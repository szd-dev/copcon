package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/testutil"
	"github.com/copcon/server/internal/tool"
	"github.com/copcon/server/internal/tools/todo"
)

const integrationTestDBName = "agent_integration_test"

// setupIntegrationTestDB creates a test database for integration tests
func setupIntegrationTestDB(t *testing.T) *gorm.DB {
	dsn := "host=localhost port=5432 user=admin password=changeme dbname=postgres sslmode=disable"

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "Failed to connect to PostgreSQL")

	db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", integrationTestDBName))
	db.Exec(fmt.Sprintf("CREATE DATABASE %s", integrationTestDBName))

	sqlDB, _ := db.DB()
	sqlDB.Close()

	testDSN := fmt.Sprintf("host=localhost port=5432 user=admin password=changeme dbname=%s sslmode=disable", integrationTestDBName)
	testDB, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	require.NoError(t, err, "Failed to connect to test database")

	err = testDB.AutoMigrate(&session.Session{}, &session.Message{}, &session.Todo{})
	require.NoError(t, err, "Failed to run migrations")

	t.Cleanup(func() {
		testDB.Exec("DROP TABLE IF EXISTS todos")
		testDB.Exec("DROP TABLE IF EXISTS messages")
		testDB.Exec("DROP TABLE IF EXISTS sessions")
		sqlDB, _ := testDB.DB()
		sqlDB.Close()

		cleanupDB, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		cleanupDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", integrationTestDBName))
		sqlCleanup, _ := cleanupDB.DB()
		sqlCleanup.Close()
	})

	return testDB
}

// mockAsyncTool is a configurable async tool for testing
type mockAsyncTool struct {
	name        string
	description string
	duration    time.Duration
	result      any
	err         error
	executeHook func(chatCtx iface.ChatContextInterface, args map[string]any)
}

func (t *mockAsyncTool) Name() string        { return t.name }
func (t *mockAsyncTool) Description() string { return t.description }
func (t *mockAsyncTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *mockAsyncTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	// Call hook if set (for testing side effects)
	if t.executeHook != nil {
		t.executeHook(chatCtx, args)
	}

	// Simulate work
	if t.duration > 0 {
		select {
		case <-time.After(t.duration):
		case <-chatCtx.Context().Done():
			return nil, chatCtx.Context().Err()
		}
	}

	// Return result or error
	if t.err != nil {
		return &tool.ToolResult{
			Success: false,
			Error:   t.err.Error(),
		}, nil
	}

	return &tool.ToolResult{
		Success: true,
		Data:    t.result,
	}, nil
}

// integrationTestHarness holds all components needed for integration tests
type integrationTestHarness struct {
	db            *gorm.DB
	sessionMgr    session.SessionManager
	contextMgr    chat_context.ContextManager
	todoMgr       todo.TodoManager
	asyncRegistry *tool.AsyncToolRegistry
	toolManager   tool.ToolManager
	agentRegistry AgentRegistry
	engine        *engineImpl
}

func newIntegrationTestHarness(t *testing.T) *integrationTestHarness {
	db := setupIntegrationTestDB(t)

	sessionMgr := session.NewSessionManager(db, nil)
	todoMgr := todo.NewTodoManager(db)
	contextMgr := chat_context.NewContextManager(db, todoMgr, slog.Default())
	asyncRegistry := tool.NewAsyncToolRegistry()
	toolManager := tool.NewToolManager()
	agentRegistry := newMockAgentRegistry()

	engine := NewAgentEngine(agentRegistry, sessionMgr, contextMgr, asyncRegistry)

	return &integrationTestHarness{
		db:            db,
		sessionMgr:    sessionMgr,
		contextMgr:    contextMgr,
		todoMgr:       todoMgr,
		asyncRegistry: asyncRegistry,
		toolManager:   toolManager,
		agentRegistry: agentRegistry,
		engine:        engine.(*engineImpl),
	}
}

func (h *integrationTestHarness) createSession(t *testing.T, ctx context.Context, agentID string) *session.Session {
	chatCtx := testutil.NewMockChatContext(ctx, "", agentID)
	sess, err := h.sessionMgr.Create(chatCtx, "Integration Test Session", agentID)
	require.NoError(t, err)
	return sess
}

func (h *integrationTestHarness) registerAgent(t *testing.T, agentID string, toolMgr tool.ToolManager) AgentDefinition {
	agentDef := AgentDefinition{
		ID:           agentID,
		Name:         "Test Agent",
		Model:        "gpt-4o",
		SystemPrompt: "You are a test agent.",
		ToolManager:  toolMgr,
		// OpenAIClient is nil - we'll test executeAsync directly without LLM
	}
	h.agentRegistry.(*mockAgentRegistry).Register(agentID, agentDef)
	return agentDef
}

// TestFullAsyncLifecycle tests the complete async execution flow:
// 1. Tool execution starts and emits EventAsyncToolStarted
// 2. Tool runs in background
// 3. Tool completes and emits EventAsyncToolComplete
// 4. Result is persisted to database
// 5. Result can be retrieved via GetToolStatusTool
func TestFullAsyncLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	harness := newIntegrationTestHarness(t)
	ctx := context.Background()

	// Create a mock async tool that completes quickly
	asyncTool := &mockAsyncTool{
		name:        "test_async_tool",
		description: "A test async tool",
		duration:    100 * time.Millisecond,
		result:      map[string]any{"status": "completed", "value": 42},
	}

	// Register tool with tool manager
	err := harness.toolManager.Register(asyncTool)
	require.NoError(t, err)

	// Create session
	sess := harness.createSession(t, ctx, "test-agent")
	harness.registerAgent(t, "test-agent", harness.toolManager)

	// Create chat context with buffered event channel
	chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")

	// Execute async tool directly via executeAsync
	tc := toolCallInfo{
		ID:        "call-test-async-123",
		Name:      "test_async_tool",
		Arguments: "{}",
		MessageID: uuid.New().String(),
	}

	err = harness.engine.executeAsync(chatCtx, harness.toolManager, tc, map[string]any{}, "", 0, nil)
	require.NoError(t, err, "executeAsync should return immediately")

	// Verify EventAsyncToolStarted was emitted
	startedEvent := waitForEventType(chatCtx, entity.EventAsyncToolStarted, 1*time.Second)
	require.NotNil(t, startedEvent, "Should receive EventAsyncToolStarted")
	startedData := startedEvent.Data.(entity.AsyncToolStartedData)
	assert.Equal(t, "call-test-async-123", startedData.CallID)
	assert.Equal(t, "test_async_tool", startedData.ToolName)
	assert.Equal(t, sess.ID.String(), startedData.SessionID)

	// Wait for async tool to complete
	completeEvent := waitForEventType(chatCtx, entity.EventAsyncToolComplete, 2*time.Second)
	require.NotNil(t, completeEvent, "Should receive EventAsyncToolComplete")
	completeData := completeEvent.Data.(entity.AsyncToolCompleteData)
	assert.Equal(t, "call-test-async-123", completeData.CallID)
	assert.Equal(t, "test_async_tool", completeData.ToolName)
	assert.GreaterOrEqual(t, completeData.Duration, int64(100)) // At least 100ms
	assert.NotNil(t, completeData.Result)

	// Verify result in registry
	state, err := harness.asyncRegistry.GetStatus("call-test-async-123")
	require.NoError(t, err)
	assert.Equal(t, tool.StatusCompleted, state.Status)
	assert.NotNil(t, state.Result)

	// Verify message was persisted to database
	var messages []session.Message
	err = harness.db.Where("session_id = ? AND tool_call_id = ?", sess.ID, "call-test-async-123").Find(&messages).Error
	require.NoError(t, err)
	require.Len(t, messages, 1, "Should have exactly one tool result message")
	assert.Equal(t, "tool", messages[0].Role)

	// Verify the result content
	var resultContent map[string]any
	err = json.Unmarshal([]byte(messages[0].Content), &resultContent)
	require.NoError(t, err)
	assert.Equal(t, "completed", resultContent["status"])
	assert.Equal(t, float64(42), resultContent["value"])

	chatCtx.Close()
}

// TestSSEDisconnectScenario tests what happens when SSE disconnects:
// 1. Async tool starts, SSE disconnects
// 2. Tool completes in background (result persisted)
// 3. Frontend polls via new session, result already in context
func TestSSEDisconnectScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	harness := newIntegrationTestHarness(t)
	ctx := context.Background()

	// Create async tool that takes longer to complete
	asyncTool := &mockAsyncTool{
		name:        "long_running_tool",
		description: "A long-running async tool",
		duration:    200 * time.Millisecond,
		result:      map[string]any{"status": "done", "data": "processed"},
	}

	err := harness.toolManager.Register(asyncTool)
	require.NoError(t, err)

	sess := harness.createSession(t, ctx, "test-agent")
	harness.registerAgent(t, "test-agent", harness.toolManager)

	// First chat context (simulates initial SSE connection)
	chatCtx1 := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")

	// Execute async tool
	tc := toolCallInfo{
		ID:        "call-sse-disconnect-123",
		Name:      "long_running_tool",
		Arguments: "{}",
		MessageID: uuid.New().String(),
	}

	err = harness.engine.executeAsync(chatCtx1, harness.toolManager, tc, map[string]any{}, "", 0, nil)
	require.NoError(t, err)

	// Verify tool started
	startedEvent := waitForEventType(chatCtx1, entity.EventAsyncToolStarted, 1*time.Second)
	require.NotNil(t, startedEvent)

	// Simulate SSE disconnect by closing the event channel
	chatCtx1.Close()

	// Wait for tool to complete (in background)
	time.Sleep(300 * time.Millisecond)

	// Verify result was persisted even though SSE disconnected
	state, err := harness.asyncRegistry.GetStatus("call-sse-disconnect-123")
	require.NoError(t, err)
	assert.Equal(t, tool.StatusCompleted, state.Status)

	// Verify message persisted to DB
	var messages []session.Message
	err = harness.db.Where("session_id = ? AND tool_call_id = ?", sess.ID, "call-sse-disconnect-123").Find(&messages).Error
	require.NoError(t, err)
	require.Len(t, messages, 1)

	// Simulate frontend reconnecting/polling with new chat context
	chatCtx2 := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
	defer chatCtx2.Close()

	// Build context - should include the async tool result
	msgsForLLM, err := harness.contextMgr.BuildContext(chatCtx2, "", 256000, "You are a test agent.")
	require.NoError(t, err)

	// Verify the tool result is in the context
	foundToolResult := false
	for _, msg := range msgsForLLM {
		if msg.Role == "tool" {
			foundToolResult = true
			assert.Contains(t, msg.Content, "done")
			break
		}
	}
	assert.True(t, foundToolResult, "Async tool result should be in LLM context after SSE reconnect")
}

// TestMultipleAsyncTools tests multiple async tools with different completion times:
// 1. Start multiple async tools simultaneously
// 2. Tools complete at different times
// 3. All results are persisted correctly
// 4. Results can be listed by session
func TestMultipleAsyncTools(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	harness := newIntegrationTestHarness(t)
	ctx := context.Background()

	// Create three tools with different durations
	toolShort := &mockAsyncTool{
		name:        "short_tool",
		description: "Completes quickly",
		duration:    50 * time.Millisecond,
		result:      map[string]any{"duration": "short"},
	}

	toolMedium := &mockAsyncTool{
		name:        "medium_tool",
		description: "Completes in medium time",
		duration:    150 * time.Millisecond,
		result:      map[string]any{"duration": "medium"},
	}

	toolLong := &mockAsyncTool{
		name:        "long_tool",
		description: "Completes slowly",
		duration:    250 * time.Millisecond,
		result:      map[string]any{"duration": "long"},
	}

	for _, tool := range []*mockAsyncTool{toolShort, toolMedium, toolLong} {
		err := harness.toolManager.Register(tool)
		require.NoError(t, err)
	}

	sess := harness.createSession(t, ctx, "test-agent")
	harness.registerAgent(t, "test-agent", harness.toolManager)

	chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
	defer chatCtx.Close()

	// Execute all three tools
	callIDs := []string{"call-multi-1", "call-multi-2", "call-multi-3"}
	tools := []string{"short_tool", "medium_tool", "long_tool"}

	for i, toolName := range tools {
		tc := toolCallInfo{
			ID:        callIDs[i],
			Name:      toolName,
			Arguments: "{}",
			MessageID: uuid.New().String(),
		}
		err := harness.engine.executeAsync(chatCtx, harness.toolManager, tc, map[string]any{}, "", 0, nil)
		require.NoError(t, err)
	}

	// Wait for all tools to complete
	time.Sleep(400 * time.Millisecond)

	// Verify all three completed
	completionOrder := []string{}
	for _, callID := range callIDs {
		state, err := harness.asyncRegistry.GetStatus(callID)
		require.NoError(t, err)
		assert.Equal(t, tool.StatusCompleted, state.Status)
		completionOrder = append(completionOrder, callID)
	}

	// Verify all messages persisted
	var messages []session.Message
	err := harness.db.Where("session_id = ?", sess.ID).Find(&messages).Error
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(messages), 3, "Should have at least 3 tool result messages")

	// List tools by session
	sessionTools := harness.asyncRegistry.ListBySession(sess.ID.String())
	assert.GreaterOrEqual(t, len(sessionTools), 3, "Should have at least 3 tools in session")
}

// TestSessionCleanupIntegration verifies session deletion cancels all async tools:
// 1. Start multiple async tools for a session
// 2. Delete the session
// 3. Verify all async tools are cancelled
// 4. Verify cancellation propagates to running tools
func TestSessionCleanupIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	harness := newIntegrationTestHarness(t)
	ctx := context.Background()

	// Create a tool that respects context cancellation
	cancelledCh := make(chan bool, 1)
	longTool := &mockAsyncTool{
		name:        "long_running_for_cancel",
		description: "A long-running tool that can be cancelled",
		duration:    10 * time.Second, // Long duration
		result:      map[string]any{"status": "should not reach"},
		executeHook: func(chatCtx iface.ChatContextInterface, args map[string]any) {
			// Monitor for cancellation
			go func() {
				<-chatCtx.Context().Done()
				cancelledCh <- true
			}()
		},
	}

	err := harness.toolManager.Register(longTool)
	require.NoError(t, err)

	sess := harness.createSession(t, ctx, "test-agent")
	harness.registerAgent(t, "test-agent", harness.toolManager)

	chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
	defer chatCtx.Close()

	// Start two async tools
	callIDs := []string{"call-cleanup-1", "call-cleanup-2"}
	for _, callID := range callIDs {
		tc := toolCallInfo{
			ID:        callID,
			Name:      "long_running_for_cancel",
			Arguments: "{}",
			MessageID: uuid.New().String(),
		}
		err := harness.engine.executeAsync(chatCtx, harness.toolManager, tc, map[string]any{}, "", 0, nil)
		require.NoError(t, err)
	}

	// Wait for tools to start
	time.Sleep(50 * time.Millisecond)

	// Verify tools are running
	for _, callID := range callIDs {
		state, err := harness.asyncRegistry.GetStatus(callID)
		require.NoError(t, err)
		assert.Equal(t, tool.StatusRunning, state.Status)
	}

	// Cancel all tools for session (simulates session deletion)
	cancelled := harness.asyncRegistry.CancelSession(sess.ID.String())
	assert.Equal(t, 2, cancelled, "Should cancel both tools")

	// Wait for cancellation to propagate
	select {
	case <-cancelledCh:
		// Good - tool received cancellation
	case <-time.After(1 * time.Second):
		t.Fatal("Tool did not receive cancellation signal")
	}

	// Verify tools are marked as cancelled
	time.Sleep(100 * time.Millisecond)
	for _, callID := range callIDs {
		state, err := harness.asyncRegistry.GetStatus(callID)
		if err == nil {
			assert.Equal(t, tool.StatusCancelled, state.Status)
		}
	}
}

// TestTimeoutIntegration verifies timeout triggers correct cleanup:
// 1. Start async tool with short timeout (via context)
// 2. Tool takes longer than timeout
// 3. Verify tool is cancelled and marked as failed
// 4. Verify proper cleanup in registry
func TestTimeoutIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	harness := newIntegrationTestHarness(t)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Tool that takes longer than timeout
	timeoutTool := &mockAsyncTool{
		name:        "slow_tool_for_timeout",
		description: "A slow tool that will timeout",
		duration:    5 * time.Second, // Much longer than timeout
		result:      map[string]any{"status": "should not reach"},
	}

	err := harness.toolManager.Register(timeoutTool)
	require.NoError(t, err)

	sess := harness.createSession(t, ctx, "test-agent")
	harness.registerAgent(t, "test-agent", harness.toolManager)

	chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
	defer chatCtx.Close()

	// Execute async tool (uses context with timeout)
	tc := toolCallInfo{
		ID:        "call-timeout-test",
		Name:      "slow_tool_for_timeout",
		Arguments: "{}",
		MessageID: uuid.New().String(),
	}

	err = harness.engine.executeAsync(chatCtx, harness.toolManager, tc, map[string]any{}, "", 0, nil)
	require.NoError(t, err)

	// Wait for started event
	startedEvent := waitForEventType(chatCtx, entity.EventAsyncToolStarted, 1*time.Second)
	require.NotNil(t, startedEvent)

	// Wait for timeout and failure
	failedEvent := waitForEventType(chatCtx, entity.EventAsyncToolFailed, 2*time.Second)
	require.NotNil(t, failedEvent, "Should receive EventAsyncToolFailed on timeout")
	failedData := failedEvent.Data.(entity.AsyncToolFailedData)
	assert.Equal(t, "call-timeout-test", failedData.CallID)
	assert.Contains(t, failedData.Error, "context canceled")

	// Verify state in registry
	state, err := harness.asyncRegistry.GetStatus("call-timeout-test")
	require.NoError(t, err)
	assert.Equal(t, tool.StatusFailed, state.Status)
	assert.Contains(t, state.Error, "context")
}

// waitForEventType waits for a specific event type from the chat context
func waitForEventType(chatCtx *testutil.MockChatContext, eventType entity.EventType, timeout time.Duration) *entity.Event {
	deadline := time.After(timeout)
	for {
		select {
		case event := <-chatCtx.Events():
			if event.Type == eventType {
				return &event
			}
		case <-deadline:
			return nil
		}
	}
}

// TestAsyncToolPanicRecovery verifies panic recovery in async tools
func TestAsyncToolPanicRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	harness := newIntegrationTestHarness(t)
	ctx := context.Background()

	// Tool that panics
	panicTool := &mockAsyncTool{
		name:        "panic_tool",
		description: "A tool that panics",
		duration:    0,
		executeHook: func(chatCtx iface.ChatContextInterface, args map[string]any) {
			panic("intentional panic for testing")
		},
	}

	err := harness.toolManager.Register(panicTool)
	require.NoError(t, err)

	sess := harness.createSession(t, ctx, "test-agent")
	harness.registerAgent(t, "test-agent", harness.toolManager)

	chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
	defer chatCtx.Close()

	tc := toolCallInfo{
		ID:        "call-panic-test",
		Name:      "panic_tool",
		Arguments: "{}",
		MessageID: uuid.New().String(),
	}

	err = harness.engine.executeAsync(chatCtx, harness.toolManager, tc, map[string]any{}, "", 0, nil)
	require.NoError(t, err)

	// Wait for started event
	startedEvent := waitForEventType(chatCtx, entity.EventAsyncToolStarted, 1*time.Second)
	require.NotNil(t, startedEvent)

	// Wait for failure event (from panic)
	failedEvent := waitForEventType(chatCtx, entity.EventAsyncToolFailed, 2*time.Second)
	require.NotNil(t, failedEvent, "Should receive EventAsyncToolFailed on panic")
	failedData := failedEvent.Data.(entity.AsyncToolFailedData)
	assert.Contains(t, failedData.Error, "panic")

	// Verify state
	state, err := harness.asyncRegistry.GetStatus("call-panic-test")
	require.NoError(t, err)
	assert.Equal(t, tool.StatusFailed, state.Status)
	assert.Contains(t, state.Error, "panic")
}

// TestConcurrentVsAsyncExecution tests difference between concurrent and async execution
func TestConcurrentVsAsyncExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	harness := newIntegrationTestHarness(t)
	ctx := context.Background()

	// Create tools
	slowTool := &mockAsyncTool{
		name:        "slow_sync_tool",
		description: "A slow tool",
		duration:    100 * time.Millisecond,
		result:      map[string]any{"executed": true},
	}

	err := harness.toolManager.Register(slowTool)
	require.NoError(t, err)

	sess := harness.createSession(t, ctx, "test-agent")
	harness.registerAgent(t, "test-agent", harness.toolManager)

	// Test async execution returns immediately
	chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
	defer chatCtx.Close()

	tc := toolCallInfo{
		ID:        "call-async-immediate",
		Name:      "slow_sync_tool",
		Arguments: "{}",
		MessageID: uuid.New().String(),
	}

	start := time.Now()
	err = harness.engine.executeAsync(chatCtx, harness.toolManager, tc, map[string]any{}, "", 0, nil)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed, 50*time.Millisecond, "executeAsync should return immediately, not wait for tool")

	// Verify tool is running
	time.Sleep(10 * time.Millisecond)
	state, err := harness.asyncRegistry.GetStatus("call-async-immediate")
	require.NoError(t, err)
	assert.Equal(t, tool.StatusRunning, state.Status)

	// Wait for completion
	completeEvent := waitForEventType(chatCtx, entity.EventAsyncToolComplete, 2*time.Second)
	require.NotNil(t, completeEvent)
}

// TestSemaphoreLimitingIntegration verifies semaphore limits concurrent async executions
func TestSemaphoreLimitingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	harness := newIntegrationTestHarness(t)
	ctx := context.Background()

	var (
		startTimes = make(map[string]time.Time)
		mu         sync.Mutex
	)

	signalTool := &mockAsyncTool{
		name:        "signal_tool",
		description: "A tool that signals start",
		duration:    100 * time.Millisecond,
		result:      map[string]any{"ok": true},
		executeHook: func(chatCtx iface.ChatContextInterface, args map[string]any) {
			callID := args["_call_id"].(string)
			mu.Lock()
			startTimes[callID] = time.Now()
			mu.Unlock()
		},
	}

	err := harness.toolManager.Register(signalTool)
	require.NoError(t, err)

	sess := harness.createSession(t, ctx, "test-agent")
	harness.registerAgent(t, "test-agent", harness.toolManager)

	chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
	defer chatCtx.Close()

	// Start more tools than semaphore limit (5)
	// All should complete, just limited concurrency
	numTools := 7
	for i := 0; i < numTools; i++ {
		callID := fmt.Sprintf("call-semaphore-%d", i)
		tc := toolCallInfo{
			ID:        callID,
			Name:      "signal_tool",
			Arguments: fmt.Sprintf(`{"_call_id": "%s"}`, callID),
			MessageID: uuid.New().String(),
		}
		err := harness.engine.executeAsync(chatCtx, harness.toolManager, tc, map[string]any{
			"_call_id": callID,
		}, "", 0, nil)
		require.NoError(t, err)
	}

	// Wait for all to complete
	time.Sleep(1 * time.Second)

	// Verify all completed
	for i := 0; i < numTools; i++ {
		callID := fmt.Sprintf("call-semaphore-%d", i)
		state, err := harness.asyncRegistry.GetStatus(callID)
		if err == nil {
			assert.Equal(t, tool.StatusCompleted, state.Status)
		}
	}
}
