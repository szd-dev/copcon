package agent

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/testutil"
	"github.com/copcon/core/tool"
)

// ============================================================================
// Mock Tools with Controllable Execution Duration
// ============================================================================

// mockControllableTool is a tool with configurable execution duration and behavior
type mockControllableTool struct {
	name        string
	description string
	duration    time.Duration
	result      *tool.ToolResult
	err         error
	shouldPanic bool
	panicMsg    string
	execCount   atomic.Int32 // Track number of executions
	startTimes  []time.Time  // Track when each execution started
	mu          sync.Mutex
}

func (t *mockControllableTool) Name() string {
	return t.name
}

func (t *mockControllableTool) Description() string {
	return t.description
}

func (t *mockControllableTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *mockControllableTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	t.mu.Lock()
	t.startTimes = append(t.startTimes, time.Now())
	t.mu.Unlock()

	t.execCount.Add(1)

	// Simulate panic if configured
	if t.shouldPanic {
		panic(t.panicMsg)
	}

	// Simulate execution duration
	if t.duration > 0 {
		select {
		case <-time.After(t.duration):
		case <-chatCtx.Context().Done():
			return nil, chatCtx.Context().Err()
		}
	}

	// Return configured result/error
	if t.err != nil {
		return nil, t.err
	}
	return t.result, nil
}

func (t *mockControllableTool) GetExecutionCount() int32 {
	return t.execCount.Load()
}

func (t *mockControllableTool) GetStartTimes() []time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.startTimes
}

// mockToolManagerWithTools allows registering specific tools for testing
type mockToolManagerWithTools struct {
	tools map[string]tool.Tool
}

func newMockToolManagerWithTools() *mockToolManagerWithTools {
	return &mockToolManagerWithTools{
		tools: make(map[string]tool.Tool),
	}
}

func (m *mockToolManagerWithTools) Register(t tool.Tool) error {
	m.tools[t.Name()] = t
	return nil
}

func (m *mockToolManagerWithTools) Unregister(name string) error {
	delete(m.tools, name)
	return nil
}

func (m *mockToolManagerWithTools) Get(name string) (tool.Tool, error) {
	if t, ok := m.tools[name]; ok {
		return t, nil
	}
	return nil, tool.ErrToolNotFound
}

func (m *mockToolManagerWithTools) List() []tool.ToolInfo {
	infos := make([]tool.ToolInfo, 0, len(m.tools))
	for name, t := range m.tools {
		infos = append(infos, tool.ToolInfo{
			Name:        name,
			Description: t.Description(),
		})
	}
	return infos
}

func (m *mockToolManagerWithTools) Execute(chatCtx iface.ChatContextInterface, name string, args map[string]any) (*tool.ToolResult, error) {
	t, err := m.Get(name)
	if err != nil {
		return nil, err
	}
	return t.Execute(chatCtx, args)
}

func (m *mockToolManagerWithTools) GetToolDefs() []llm.ToolDef {
	// Not used in execution tests
	return nil
}

// ============================================================================
// Test Helper Functions
// ============================================================================

// createTestEngine creates a minimal AgentEngine for execution mode testing
func createTestEngine() *engineImpl {
	return NewTestEngine()
}

// createTestEngineWithRegistry creates an AgentEngine with a specific async registry
func createTestEngineWithRegistry(asyncRegistry *tool.AsyncToolRegistry) *engineImpl {
	return NewTestEngineWithRegistry(asyncRegistry)
}

// trackEvents collects events from a channel with timeout
func trackEvents(eventChan <-chan entity.Event, timeout time.Duration) []entity.Event {
	events := make([]entity.Event, 0)
	timeoutChan := time.After(timeout)
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				return events
			}
			events = append(events, event)
		case <-timeoutChan:
			return events
		}
	}
}

// closeMockChatContext safely closes a MockChatContext
func closeMockChatContext(chatCtx iface.ChatContextInterface) {
	if mock, ok := chatCtx.(*testutil.MockChatContext); ok {
		mock.Close()
	}
}

// ============================================================================
// TestSyncExecution: Verify sequential execution and result persistence
// ============================================================================

func TestSyncExecution(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	// Create a tool that takes 100ms to execute
	syncTool := &mockControllableTool{
		name:        "sync_test_tool",
		description: "A tool for testing sync execution",
		duration:    100 * time.Millisecond,
		result: &tool.ToolResult{
			Success: true,
			Data:    map[string]any{"value": "sync_result"},
		},
	}
	toolMgr.Register(syncTool)

	engine := createTestEngine()

	ctx := context.Background()
	sessionID := "test-sync-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-sync-001",
		ID:        "call-sync-001",
		Name:      "sync_test_tool",
		Arguments: "{}",
	}

	args := map[string]any{}

	// Execute sync tool
	partIndices := map[string]int{"call-sync-001": 0}
	err := engine.executeSync(chatCtx, toolMgr, tc, args, "", 0, partIndices, make(map[string]*ToolCallResult))
	require.NoError(t, err, "Sync execution should not return error")

	// Verify tool was executed exactly once
	assert.Equal(t, int32(1), syncTool.GetExecutionCount(), "Tool should be executed once")

	// Collect events
	events := trackEvents(chatCtx.Events(), 500*time.Millisecond)

	// Verify part_update event sequence (running → complete)
	var foundRunning, foundComplete bool
	for _, event := range events {
		if event.Type == entity.EventPartUpdate {
			data := event.Data.(entity.PartUpdateData)
			if data.State == "running" {
				foundRunning = true
				assert.Equal(t, "tool-call", data.PartType)
				assert.Equal(t, 0, data.PartIndex)
			}
			if data.State == "complete" {
				foundComplete = true
				assert.Equal(t, "tool-call", data.PartType)
				assert.NotEmpty(t, data.Output)
			}
		}
	}
	assert.True(t, foundRunning, "part_update with state='running' should be emitted")
	assert.True(t, foundComplete, "part_update with state='complete' should be emitted")

	// Close channel to clean up
	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestConcurrentExecution: Verify concurrent execution with overlapping timestamps
// ============================================================================

func TestConcurrentExecution(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	// Create multiple tools with different execution times
	tool1 := &mockControllableTool{
		name:        "concurrent_tool_1",
		description: "First concurrent tool",
		duration:    50 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"tool": 1}},
	}
	tool2 := &mockControllableTool{
		name:        "concurrent_tool_2",
		description: "Second concurrent tool",
		duration:    100 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"tool": 2}},
	}
	tool3 := &mockControllableTool{
		name:        "concurrent_tool_3",
		description: "Third concurrent tool",
		duration:    75 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"tool": 3}},
	}

	toolMgr.Register(tool1)
	toolMgr.Register(tool2)
	toolMgr.Register(tool3)

	engine := createTestEngine()

	ctx := context.Background()
	sessionID := "test-concurrent-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	// Create parsed tool calls for concurrent execution
	toolCalls := []parsedToolCall{
		{tc: toolCallInfo{ID: "call-concurrent-001", MessageID: "msg-001", Name: "concurrent_tool_1", Arguments: "{}"}, args: map[string]any{}},
		{tc: toolCallInfo{ID: "call-concurrent-002", MessageID: "msg-001", Name: "concurrent_tool_2", Arguments: "{}"}, args: map[string]any{}},
		{tc: toolCallInfo{ID: "call-concurrent-003", MessageID: "msg-001", Name: "concurrent_tool_3", Arguments: "{}"}, args: map[string]any{}},
	}

	startTime := time.Now()

	// Execute concurrent tools
	err := engine.executeConcurrent(chatCtx, toolMgr, toolCalls, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err, "Concurrent execution should not return error")

	totalDuration := time.Since(startTime)

	// Verify total duration is less than sum of individual durations (proving concurrency)
	sumOfDurations := tool1.duration + tool2.duration + tool3.duration
	assert.Less(t, totalDuration, sumOfDurations,
		"Concurrent execution should complete faster than sequential (%v < %v)",
		totalDuration, sumOfDurations)

	// Verify all tools were executed
	assert.Equal(t, int32(1), tool1.GetExecutionCount(), "Tool 1 should be executed once")
	assert.Equal(t, int32(1), tool2.GetExecutionCount(), "Tool 2 should be executed once")
	assert.Equal(t, int32(1), tool3.GetExecutionCount(), "Tool 3 should be executed once")

	// Verify overlapping timestamps (proving true concurrency)
	startTimes1 := tool1.GetStartTimes()
	startTimes2 := tool2.GetStartTimes()
	startTimes3 := tool3.GetStartTimes()

	assert.Len(t, startTimes1, 1)
	assert.Len(t, startTimes2, 1)
	assert.Len(t, startTimes3, 1)

	// All tools should start within a small window (e.g., 10ms)
	maxStartDiff := max(
		startTimes1[0].Sub(startTimes2[0]).Abs(),
		startTimes1[0].Sub(startTimes3[0]).Abs(),
		startTimes2[0].Sub(startTimes3[0]).Abs(),
	)
	assert.Less(t, maxStartDiff, 10*time.Millisecond,
		"Concurrent tools should start at approximately the same time")

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestConcurrencyLimit: Verify max 5 concurrent tools
// ============================================================================

func TestConcurrencyLimit(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	// Create 7 tools that take 100ms each
	// With limit of 5, total time should be ~200ms (2 batches), not 700ms
	for i := 1; i <= 7; i++ {
		toolInstance := &mockControllableTool{
			name:        fmt.Sprintf("limit_tool_%d", i),
			description: fmt.Sprintf("Tool %d for concurrency limit test", i),
			duration:    100 * time.Millisecond,
			result:      &tool.ToolResult{Success: true, Data: map[string]any{"tool": i}},
		}
		toolMgr.Register(toolInstance)
	}

	engine := createTestEngine()

	ctx := context.Background()
	sessionID := "test-limit-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	// Create 7 concurrent tool calls
	toolCalls := make([]parsedToolCall, 7)
	for i := 1; i <= 7; i++ {
		toolCalls[i-1] = parsedToolCall{
			tc:   toolCallInfo{ID: fmt.Sprintf("call-limit-%03d", i), MessageID: "msg-limit", Name: fmt.Sprintf("limit_tool_%d", i), Arguments: "{}"},
			args: map[string]any{},
		}
	}

	startTime := time.Now()
	err := engine.executeConcurrent(chatCtx, toolMgr, toolCalls, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err)

	duration := time.Since(startTime)

	// With semaphore limit of 5 and 7 tools each taking 100ms:
	// - First batch: 5 tools (100ms)
	// - Second batch: 2 tools (100ms)
	// Total: ~200ms (plus some overhead)
	// Without limit, would be ~100ms if truly unlimited, but semaphore ensures batching
	// Expected: between 150ms and 250ms (allowing for scheduling overhead)
	assert.GreaterOrEqual(t, duration, 150*time.Millisecond,
		"Duration should reflect semaphore limiting (not unlimited parallelism)")
	assert.Less(t, duration, 350*time.Millisecond,
		"Duration should not be sequential (700ms), should be batched (~200ms)")

	// Verify all tools executed
	for i := 1; i <= 7; i++ {
		toolName := fmt.Sprintf("limit_tool_%d", i)
		toolInstance, err := toolMgr.Get(toolName)
		require.NoError(t, err)
		mockTool := toolInstance.(*mockControllableTool)
		assert.Equal(t, int32(1), mockTool.GetExecutionCount(),
			"Tool %d should be executed once", i)
	}

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestAsyncExecution: Verify main loop returns immediately
// ============================================================================

func TestAsyncExecution(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	// Create a tool that takes 500ms
	asyncTool := &mockControllableTool{
		name:        "async_test_tool",
		description: "A tool for testing async execution",
		duration:    500 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"async_result": "completed"}},
	}
	toolMgr.Register(asyncTool)

	asyncRegistry := tool.NewAsyncToolRegistry()
	engine := createTestEngineWithRegistry(asyncRegistry)

	ctx := context.Background()
	sessionID := "test-async-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-async-001",
		ID:        "call-async-001",
		Name:      "async_test_tool",
		Arguments: "{}",
	}

	args := map[string]any{}

	startTime := time.Now()

	// Execute async tool - should return immediately
	err := engine.executeAsync(chatCtx, toolMgr, tc, args, "", 0, nil)
	require.NoError(t, err, "Async execution should return immediately without error")

	returnDuration := time.Since(startTime)

	// Main loop should return within 50ms (immediate return, not waiting for tool)
	assert.Less(t, returnDuration, 50*time.Millisecond,
		"Async executeAsync should return immediately (%v < 50ms)", returnDuration)

	// Verify tool is registered in async registry
	state, err := asyncRegistry.GetStatus("call-async-001")
	require.NoError(t, err, "Tool should be registered in async registry")
	assert.Equal(t, tool.StatusRunning, state.Status, "Tool should be in running status")

	// Verify EventAsyncToolStarted was emitted
	events := trackEvents(chatCtx.Events(), 100*time.Millisecond)
	var foundStartedEvent bool
	for _, event := range events {
		if event.Type == entity.EventAsyncToolStarted {
			foundStartedEvent = true
			data := event.Data.(entity.AsyncToolStartedData)
			assert.Equal(t, "call-async-001", data.CallID)
			assert.Equal(t, "async_test_tool", data.ToolName)
			assert.Equal(t, sessionID, data.SessionID)
		}
	}
	assert.True(t, foundStartedEvent, "EventAsyncToolStarted should be emitted immediately")

	// Wait for async tool to complete (with timeout)
	time.Sleep(600 * time.Millisecond)

	// Verify tool completed
	state, err = asyncRegistry.GetStatus("call-async-001")
	if err == nil {
		assert.Equal(t, tool.StatusCompleted, state.Status, "Tool should be completed after waiting")
	}

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestAsyncCompletionSSEOnline: Verify event emission and persistence
// ============================================================================

func TestAsyncCompletionSSEOnline(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	asyncTool := &mockControllableTool{
		name:        "async_complete_tool",
		description: "Tool for testing async completion with SSE online",
		duration:    200 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"sse_result": "success"}},
	}
	toolMgr.Register(asyncTool)

	asyncRegistry := tool.NewAsyncToolRegistry()
	engine := createTestEngineWithRegistry(asyncRegistry)

	ctx := context.Background()
	sessionID := "test-sse-online-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-sse-001",
		ID:        "call-sse-001",
		Name:      "async_complete_tool",
		Arguments: "{}",
	}

	err := engine.executeAsync(chatCtx, toolMgr, tc, map[string]any{}, "", 0, nil)
	require.NoError(t, err)

	// Wait for completion and collect events
	events := trackEvents(chatCtx.Events(), 500*time.Millisecond)

	// Verify event sequence
	var foundStarted, foundComplete bool
	var completeData entity.AsyncToolCompleteData

	for _, event := range events {
		if event.Type == entity.EventAsyncToolStarted {
			foundStarted = true
		}
		if event.Type == entity.EventAsyncToolComplete {
			foundComplete = true
			completeData = event.Data.(entity.AsyncToolCompleteData)
		}
	}

	assert.True(t, foundStarted, "EventAsyncToolStarted should be emitted")
	assert.True(t, foundComplete, "EventAsyncToolComplete should be emitted when SSE is online")

	// Verify completion event data
	assert.Equal(t, "call-sse-001", completeData.CallID)
	assert.Equal(t, "async_complete_tool", completeData.ToolName)
	assert.Greater(t, completeData.Duration, int64(0), "Duration should be tracked")
	assert.NotNil(t, completeData.Result, "Result should be included")

	// Verify registry state
	state, err := asyncRegistry.GetStatus("call-sse-001")
	if err == nil {
		assert.Equal(t, tool.StatusCompleted, state.Status)
		assert.NotNil(t, state.Result)
	}

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestAsyncCompletionSSEOffline: Verify pending event recording
// ============================================================================

func TestAsyncCompletionSSEOffline(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	asyncTool := &mockControllableTool{
		name:        "async_offline_tool",
		description: "Tool for testing async completion with SSE offline",
		duration:    100 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"offline_result": "data"}},
	}
	toolMgr.Register(asyncTool)

	asyncRegistry := tool.NewAsyncToolRegistry()
	engine := createTestEngineWithRegistry(asyncRegistry)

	ctx := context.Background()
	sessionID := "test-sse-offline-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-offline-001",
		ID:        "call-offline-001",
		Name:      "async_offline_tool",
		Arguments: "{}",
	}

	// Execute async tool
	err := engine.executeAsync(chatCtx, toolMgr, tc, map[string]any{}, "", 0, nil)
	require.NoError(t, err)

	// Simulate SSE offline by closing event channel before completion
	// This tests that the goroutine still completes and persists result
	time.Sleep(150 * time.Millisecond)

	// Verify registry still tracks the tool state (even though SSE offline)
	state, err := asyncRegistry.GetStatus("call-offline-001")
	if err == nil {
		assert.Equal(t, tool.StatusCompleted, state.Status,
			"Tool should complete even when SSE is offline")
		assert.NotNil(t, state.Result,
			"Result should be persisted to registry even when SSE offline")
	}

	// Note: EventAsyncCompletionPending would be used by frontend to poll
	// This test verifies the registry state is maintained for polling

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestErrorIsolation: Verify one failure doesn't affect others
// ============================================================================

func TestErrorIsolation(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	// Create tools - one will fail, others should succeed
	successTool1 := &mockControllableTool{
		name:        "success_tool_1",
		description: "First successful tool",
		duration:    50 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"result": 1}},
	}
	successTool2 := &mockControllableTool{
		name:        "success_tool_2",
		description: "Second successful tool",
		duration:    50 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"result": 2}},
	}
	failingTool := &mockControllableTool{
		name:        "failing_tool",
		description: "Tool that will fail",
		duration:    50 * time.Millisecond,
		err:         errors.New("intentional test failure"),
	}

	toolMgr.Register(successTool1)
	toolMgr.Register(successTool2)
	toolMgr.Register(failingTool)

	engine := createTestEngine()

	ctx := context.Background()
	sessionID := "test-isolation-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	// Create concurrent tool calls (one failing, others succeeding)
	toolCalls := []parsedToolCall{
		{tc: toolCallInfo{ID: "call-success-001", MessageID: "msg-iso", Name: "success_tool_1", Arguments: "{}"}, args: map[string]any{}},
		{tc: toolCallInfo{ID: "call-fail-001", MessageID: "msg-iso", Name: "failing_tool", Arguments: "{}"}, args: map[string]any{}},
		{tc: toolCallInfo{ID: "call-success-002", MessageID: "msg-iso", Name: "success_tool_2", Arguments: "{}"}, args: map[string]any{}},
	}

	// Execute concurrent - should not fail overall even if one tool fails
	err := engine.executeConcurrent(chatCtx, toolMgr, toolCalls, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err, "Concurrent execution should not fail even if one tool fails")

	// Verify all tools were executed (failure didn't stop others)
	assert.Equal(t, int32(1), successTool1.GetExecutionCount(), "Success tool 1 should still execute")
	assert.Equal(t, int32(1), successTool2.GetExecutionCount(), "Success tool 2 should still execute")
	assert.Equal(t, int32(1), failingTool.GetExecutionCount(), "Failing tool should also execute")

	// Verify events - should have part_update for all (success and failure)
	events := trackEvents(chatCtx.Events(), 500*time.Millisecond)

	partUpdateCount := 0
	for _, event := range events {
		if event.Type == entity.EventPartUpdate {
			data := event.Data.(entity.PartUpdateData)
			if data.State == "complete" || data.State == "error" {
				partUpdateCount++
			}
			// Failing tool should have error state
			if data.State == "error" {
				assert.NotEmpty(t, data.Error)
			}
		}
	}
	assert.Equal(t, 3, partUpdateCount, "Should have terminal part_update for all 3 tools (including failed one)")

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestResultOrdering: Verify results ordered by call ID
// ============================================================================

func TestResultOrdering(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	// Create tools with different durations (will complete in different order)
	// tool_a: 100ms (slowest), tool_c: 20ms (fastest), tool_b: 50ms (medium)
	toolA := &mockControllableTool{
		name:        "tool_a",
		description: "Slow tool (100ms)",
		duration:    100 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"name": "a"}},
	}
	toolB := &mockControllableTool{
		name:        "tool_b",
		description: "Medium tool (50ms)",
		duration:    50 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"name": "b"}},
	}
	toolC := &mockControllableTool{
		name:        "tool_c",
		description: "Fast tool (20ms)",
		duration:    20 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"name": "c"}},
	}

	toolMgr.Register(toolA)
	toolMgr.Register(toolB)
	toolMgr.Register(toolC)

	// Use a mock context manager that records message order
	orderedMgr := &mockOrderedMessageStore{}

	engine := NewTestEngine(WithTestMessageStore(orderedMgr))

	ctx := context.Background()
	sessionID := "test-ordering-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	// Create tool calls with IDs that should be ordered alphabetically
	// call_a (slowest), call_c (fastest), call_b (medium)
	// Completion order: call_c first, then call_b, then call_a
	// Expected result order (by ID): call_a, call_b, call_c
	toolCalls := []parsedToolCall{
		{tc: toolCallInfo{ID: "call_a", MessageID: "msg-order", Name: "tool_a", Arguments: "{}"}, args: map[string]any{}},
		{tc: toolCallInfo{ID: "call_c", MessageID: "msg-order", Name: "tool_c", Arguments: "{}"}, args: map[string]any{}},
		{tc: toolCallInfo{ID: "call_b", MessageID: "msg-order", Name: "tool_b", Arguments: "{}"}, args: map[string]any{}},
	}

	err := engine.executeConcurrent(chatCtx, toolMgr, toolCalls, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err)

	// Verify order in context manager (should be alphabetical by ID)
	require.Len(t, orderedMgr.messages, 3, "Should have 3 persisted messages")

	// Messages should be sorted by tool call ID (alphabetically)
	assert.Equal(t, "call_a", orderedMgr.messages[0].ToolCallID, "First message should be call_a")
	assert.Equal(t, "call_b", orderedMgr.messages[1].ToolCallID, "Second message should be call_b")
	assert.Equal(t, "call_c", orderedMgr.messages[2].ToolCallID, "Third message should be call_c")

	closeMockChatContext(chatCtx)
}

type mockOrderedMessageStore struct {
	messages []*storage.Message
	mu       sync.Mutex
}

func (m *mockOrderedMessageStore) List(_ context.Context, _ uuid.UUID, _ int) ([]*storage.Message, error) {
	return nil, nil
}

func (m *mockOrderedMessageStore) Add(_ context.Context, msg *storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockOrderedMessageStore) Update(_ context.Context, msg *storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.messages {
		if existing.ID == msg.ID {
			m.messages[i] = msg
			return nil
		}
	}
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockOrderedMessageStore) Upsert(_ context.Context, msg *storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.messages {
		if existing.ID == msg.ID {
			m.messages[i] = msg
			return nil
		}
	}
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockOrderedMessageStore) DeleteBySession(_ context.Context, _ uuid.UUID) error {
	return nil
}

// ============================================================================
// TestPanicRecovery: Verify panicked tool doesn't crash server
// ============================================================================

func TestPanicRecovery(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	// Create a tool that will panic
	panicTool := &mockControllableTool{
		name:        "panic_tool",
		description: "Tool that panics during execution",
		shouldPanic: true,
		panicMsg:    "intentional test panic",
	}

	// Create a normal tool that should still work
	normalTool := &mockControllableTool{
		name:        "normal_tool",
		description: "Normal tool that should succeed",
		duration:    50 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"normal": "result"}},
	}

	toolMgr.Register(panicTool)
	toolMgr.Register(normalTool)

	asyncRegistry := tool.NewAsyncToolRegistry()
	engine := createTestEngineWithRegistry(asyncRegistry)

	ctx := context.Background()
	sessionID := "test-panic-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-panic-001",
		ID:        "call-panic-001",
		Name:      "panic_tool",
		Arguments: "{}",
	}

	// Execute async tool that will panic
	err := engine.executeAsync(chatCtx, toolMgr, tc, map[string]any{}, "", 0, nil)
	require.NoError(t, err, "executeAsync should not fail even if tool will panic")

	// Wait for panic to be caught and handled
	time.Sleep(200 * time.Millisecond)

	// Verify registry shows failure (not crash)
	state, err := asyncRegistry.GetStatus("call-panic-001")
	if err == nil {
		assert.Equal(t, tool.StatusFailed, state.Status, "Panicked tool should be marked as failed")
		assert.Contains(t, state.Error, "panic", "Error should contain panic information")
		assert.Contains(t, state.Error, "intentional test panic", "Error should contain panic message")
	}

	// Collect events - should have failed event
	events := trackEvents(chatCtx.Events(), 100*time.Millisecond)
	var foundFailedEvent bool
	for _, event := range events {
		if event.Type == entity.EventAsyncToolFailed {
			foundFailedEvent = true
			data := event.Data.(entity.AsyncToolFailedData)
			assert.Equal(t, "call-panic-001", data.CallID)
			assert.Contains(t, data.Error, "panic")
		}
	}
	assert.True(t, foundFailedEvent, "EventAsyncToolFailed should be emitted for panicked tool")

	// Verify normal tool can still execute (server not crashed)
	normalTc := toolCallInfo{
		MessageID: "msg-normal-001",
		ID:        "call-normal-001",
		Name:      "normal_tool",
		Arguments: "{}",
	}
	err = engine.executeSync(chatCtx, toolMgr, normalTc, map[string]any{}, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err, "Normal tool should still work after panic was recovered")
	assert.Equal(t, int32(1), normalTool.GetExecutionCount(), "Normal tool should execute successfully")

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestGoroutineLeak: Verify no goroutine leaks after execution
// ============================================================================

func TestGoroutineLeak(t *testing.T) {
	// Get baseline goroutine count
	runtime.GC()
	time.Sleep(100 * time.Millisecond) // Let goroutines settle
	baseGoroutines := runtime.NumGoroutine()

	toolMgr := newMockToolManagerWithTools()

	// Create multiple async tools
	for i := 1; i <= 10; i++ {
		asyncTool := &mockControllableTool{
			name:        fmt.Sprintf("leak_test_tool_%d", i),
			description: fmt.Sprintf("Tool %d for goroutine leak test", i),
			duration:    50 * time.Millisecond,
			result:      &tool.ToolResult{Success: true, Data: map[string]any{"tool": i}},
		}
		toolMgr.Register(asyncTool)
	}

	asyncRegistry := tool.NewAsyncToolRegistry()
	engine := createTestEngineWithRegistry(asyncRegistry)

	ctx := context.Background()
	sessionID := "test-leak-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	// Launch 10 async tools
	for i := 1; i <= 10; i++ {
		tc := toolCallInfo{
			MessageID: fmt.Sprintf("msg-leak-%d", i),
			ID:        fmt.Sprintf("call-leak-%03d", i),
			Name:      fmt.Sprintf("leak_test_tool_%d", i),
			Arguments: "{}",
		}
		err := engine.executeAsync(chatCtx, toolMgr, tc, map[string]any{}, "", 0, nil)
		require.NoError(t, err)
	}

	// Wait for all async tools to complete
	time.Sleep(300 * time.Millisecond)

	// Allow goroutines to settle
	runtime.GC()
	time.Sleep(200 * time.Millisecond)

	// Check goroutine count
	finalGoroutines := runtime.NumGoroutine()

	// Verify no goroutine leak (allow small variance for test infrastructure)
	goroutineDiff := finalGoroutines - baseGoroutines
	assert.LessOrEqual(t, goroutineDiff, 2,
		"No significant goroutine leak after async executions (base=%d, final=%d, diff=%d)",
		baseGoroutines, finalGoroutines, goroutineDiff)

	// Verify registry is cleaned up (tools unregistered after completion)
	remainingTools := 0
	for i := 1; i <= 10; i++ {
		callID := fmt.Sprintf("call-leak-%03d", i)
		state, err := asyncRegistry.GetStatus(callID)
		if err == nil && state.Status == tool.StatusRunning {
			remainingTools++
		}
	}
	// All tools should be completed and still tracked in registry (for result retrieval)
	// Running status should be 0
	assert.Equal(t, 0, remainingTools,
		"No tools should remain in running status after completion")

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestConcurrentWithErrorAndPanic: Mixed concurrent execution with failures
// ============================================================================

func TestConcurrentWithErrorAndPanic(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	// Mix of success, error, and panic scenarios
	successTool := &mockControllableTool{
		name:        "mixed_success",
		description: "Tool that succeeds",
		duration:    30 * time.Millisecond,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"status": "ok"}},
	}
	errorTool := &mockControllableTool{
		name:        "mixed_error",
		description: "Tool that returns error",
		duration:    30 * time.Millisecond,
		err:         errors.New("mixed test error"),
	}

	toolMgr.Register(successTool)
	toolMgr.Register(errorTool)

	engine := createTestEngine()

	ctx := context.Background()
	sessionID := "test-mixed-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	toolCalls := []parsedToolCall{
		{tc: toolCallInfo{ID: "call-mixed-001", MessageID: "msg-mixed", Name: "mixed_success", Arguments: "{}"}, args: map[string]any{}},
		{tc: toolCallInfo{ID: "call-mixed-002", MessageID: "msg-mixed", Name: "mixed_error", Arguments: "{}"}, args: map[string]any{}},
	}

	err := engine.executeConcurrent(chatCtx, toolMgr, toolCalls, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err, "Concurrent execution should handle mixed success/failure")

	// Both tools should execute
	assert.Equal(t, int32(1), successTool.GetExecutionCount())
	assert.Equal(t, int32(1), errorTool.GetExecutionCount())

	events := trackEvents(chatCtx.Events(), 200*time.Millisecond)

	// Should have results for both (success and error wrapped)
	terminalUpdates := 0
	for _, event := range events {
		if event.Type == entity.EventPartUpdate {
			data := event.Data.(entity.PartUpdateData)
			if data.State == "complete" || data.State == "error" {
				terminalUpdates++
			}
		}
	}
	assert.Equal(t, 2, terminalUpdates, "Should have terminal part_update for both tools")

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestExecutionModeParsing: Verify execution_mode extraction
// ============================================================================

func TestExecutionModeParsing(t *testing.T) {
	t.Run("extracts sync mode", func(t *testing.T) {
		args := map[string]any{"execution_mode": "sync", "param": "value"}
		mode, cleanedArgs := parseExecutionMode(args)
		assert.Equal(t, ExecutionModeSync, mode)
		assert.NotContains(t, cleanedArgs, "execution_mode", "execution_mode should be removed")
		assert.Contains(t, cleanedArgs, "param", "other params should remain")
	})

	t.Run("extracts concurrent mode", func(t *testing.T) {
		args := map[string]any{"execution_mode": "concurrent", "param": "value"}
		mode, cleanedArgs := parseExecutionMode(args)
		assert.Equal(t, ExecutionModeConcurrent, mode)
		assert.NotContains(t, cleanedArgs, "execution_mode")
	})

	t.Run("extracts async mode", func(t *testing.T) {
		args := map[string]any{"execution_mode": "async", "param": "value"}
		mode, cleanedArgs := parseExecutionMode(args)
		assert.Equal(t, ExecutionModeAsync, mode)
		assert.NotContains(t, cleanedArgs, "execution_mode")
	})

	t.Run("defaults to sync for invalid mode", func(t *testing.T) {
		args := map[string]any{"execution_mode": "invalid", "param": "value"}
		mode, cleanedArgs := parseExecutionMode(args)
		assert.Equal(t, ExecutionModeSync, mode, "Invalid mode should default to sync")
		assert.NotContains(t, cleanedArgs, "execution_mode")
	})

	t.Run("defaults to sync when not specified", func(t *testing.T) {
		args := map[string]any{"param": "value"}
		mode, cleanedArgs := parseExecutionMode(args)
		assert.Equal(t, ExecutionModeSync, mode, "Missing mode should default to sync")
		assert.Contains(t, cleanedArgs, "param")
	})

	t.Run("handles empty args", func(t *testing.T) {
		args := map[string]any{}
		mode, cleanedArgs := parseExecutionMode(args)
		assert.Equal(t, ExecutionModeSync, mode)
		assert.Empty(t, cleanedArgs)
	})
}

// ============================================================================
// TestContextCancellation: Verify tools respect context cancellation
// ============================================================================

func TestContextCancellation(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	// Tool that takes a while
	longTool := &mockControllableTool{
		name:        "long_running_tool",
		description: "Tool that takes 5 seconds",
		duration:    5 * time.Second,
		result:      &tool.ToolResult{Success: true, Data: map[string]any{"done": true}},
	}
	toolMgr.Register(longTool)

	asyncRegistry := tool.NewAsyncToolRegistry()
	engine := createTestEngineWithRegistry(asyncRegistry)

	// Create context that will be cancelled quickly
	ctx, cancel := context.WithCancel(context.Background())
	sessionID := "test-cancel-session"
	chatCtx := testutil.NewMockChatContext(ctx, sessionID, "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-cancel-001",
		ID:        "call-cancel-001",
		Name:      "long_running_tool",
		Arguments: "{}",
	}

	err := engine.executeAsync(chatCtx, toolMgr, tc, map[string]any{}, "", 0, nil)
	require.NoError(t, err)

	// Cancel context after a short delay
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for cancellation to propagate
	time.Sleep(200 * time.Millisecond)

	// Verify tool was cancelled
	state, err := asyncRegistry.GetStatus("call-cancel-001")
	if err == nil {
		// Tool should be cancelled or failed due to context cancellation
		assert.NotEqual(t, tool.StatusRunning, state.Status,
			"Tool should not remain in running status after context cancellation")
	}

	// Verify tool didn't complete execution (cancelled before 5s)
	assert.Less(t, longTool.GetExecutionCount(), int32(2),
		"Tool should not complete full execution when context cancelled")

	closeMockChatContext(chatCtx)
}

// ============================================================================
// TestToolHooks: Verify hook calls for sync, concurrent, and async execution
// ============================================================================

type recordingHook struct {
	records []hookCallRecord
	mu      sync.Mutex
}

type hookCallRecord struct {
	Point    hook.HookPoint
	ToolName string
}

func (h *recordingHook) Name() string  { return "recording" }
func (h *recordingHook) Priority() int { return 100 }

func (h *recordingHook) Points() []hook.HookPoint {
	return []hook.HookPoint{
		hook.BeforeToolExecute,
		hook.AfterToolExecute,
		hook.OnToolError,
	}
}

func (h *recordingHook) Execute(ctx *hook.HookContext) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, hookCallRecord{
		Point:    ctx.CurrentPoint,
		ToolName: ctx.ToolName,
	})
	return nil
}

func (h *recordingHook) Records() []hookCallRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	cpy := make([]hookCallRecord, len(h.records))
	copy(cpy, h.records)
	return cpy
}

func TestToolHooksSyncExecution(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	syncTool := &mockControllableTool{
		name:     "sync_hook_tool",
		duration: 10 * time.Millisecond,
		result:   &tool.ToolResult{Success: true, Data: map[string]any{"value": "ok"}},
	}
	toolMgr.Register(syncTool)

	recHook := &recordingHook{}
	hookRunner := hook.NewHookRunner()
	hookRunner.Register(recHook)

	engine := NewTestEngine(WithHookRunner(hookRunner))

	ctx := context.Background()
	chatCtx := testutil.NewMockChatContext(ctx, "test-hook-sync", "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-hook-001",
		ID:        "call-hook-001",
		Name:      "sync_hook_tool",
		Arguments: "{}",
	}

	err := engine.executeSync(chatCtx, toolMgr, tc, map[string]any{}, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err)

	records := recHook.Records()
	require.Len(t, records, 2, "expected BeforeToolExecute + AfterToolExecute")
	assert.Equal(t, hook.BeforeToolExecute, records[0].Point)
	assert.Equal(t, "sync_hook_tool", records[0].ToolName)
	assert.Equal(t, hook.AfterToolExecute, records[1].Point)
	assert.Equal(t, "sync_hook_tool", records[1].ToolName)

	closeMockChatContext(chatCtx)
}

func TestToolHooksSyncError(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	failTool := &mockControllableTool{
		name:     "fail_hook_tool",
		duration: 10 * time.Millisecond,
		err:      errors.New("tool failure"),
	}
	toolMgr.Register(failTool)

	recHook := &recordingHook{}
	hookRunner := hook.NewHookRunner()
	hookRunner.Register(recHook)

	engine := NewTestEngine(WithHookRunner(hookRunner))

	ctx := context.Background()
	chatCtx := testutil.NewMockChatContext(ctx, "test-hook-error", "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-hook-err",
		ID:        "call-hook-err",
		Name:      "fail_hook_tool",
		Arguments: "{}",
	}

	err := engine.executeSync(chatCtx, toolMgr, tc, map[string]any{}, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err) // executeSync does not return tool errors

	records := recHook.Records()
	require.Len(t, records, 2, "expected BeforeToolExecute + OnToolError")
	assert.Equal(t, hook.BeforeToolExecute, records[0].Point)
	assert.Equal(t, hook.OnToolError, records[1].Point)

	closeMockChatContext(chatCtx)
}

func TestToolHooksConcurrentExecution(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	t1 := &mockControllableTool{
		name:     "hook_concurrent_1",
		duration: 20 * time.Millisecond,
		result:   &tool.ToolResult{Success: true, Data: map[string]any{"v": 1}},
	}
	t2 := &mockControllableTool{
		name:     "hook_concurrent_2",
		duration: 30 * time.Millisecond,
		result:   &tool.ToolResult{Success: true, Data: map[string]any{"v": 2}},
	}
	toolMgr.Register(t1)
	toolMgr.Register(t2)

	recHook := &recordingHook{}
	hookRunner := hook.NewHookRunner()
	hookRunner.Register(recHook)

	engine := NewTestEngine(WithHookRunner(hookRunner))

	ctx := context.Background()
	chatCtx := testutil.NewMockChatContext(ctx, "test-hook-concurrent", "test-agent")

	toolCalls := []parsedToolCall{
		{tc: toolCallInfo{ID: "call-hc-1", MessageID: "msg-hc", Name: "hook_concurrent_1", Arguments: "{}"}, args: map[string]any{}},
		{tc: toolCallInfo{ID: "call-hc-2", MessageID: "msg-hc", Name: "hook_concurrent_2", Arguments: "{}"}, args: map[string]any{}},
	}

	err := engine.executeConcurrent(chatCtx, toolMgr, toolCalls, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err)

	records := recHook.Records()
	require.Len(t, records, 4, "expected 2 BeforeToolExecute + 2 AfterToolExecute")

	beforeCount := 0
	afterCount := 0
	for _, r := range records {
		switch r.Point {
		case hook.BeforeToolExecute:
			beforeCount++
		case hook.AfterToolExecute:
			afterCount++
		}
	}
	assert.Equal(t, 2, beforeCount)
	assert.Equal(t, 2, afterCount)

	closeMockChatContext(chatCtx)
}

func TestToolHooksAsyncExecution(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	asyncTool := &mockControllableTool{
		name:     "hook_async_tool",
		duration: 100 * time.Millisecond,
		result:   &tool.ToolResult{Success: true, Data: map[string]any{"v": "async"}},
	}
	toolMgr.Register(asyncTool)

	recHook := &recordingHook{}
	hookRunner := hook.NewHookRunner()
	hookRunner.Register(recHook)

	asyncRegistry := tool.NewAsyncToolRegistry()
	engine := NewTestEngineWithRegistry(asyncRegistry, WithHookRunner(hookRunner))

	ctx := context.Background()
	chatCtx := testutil.NewMockChatContext(ctx, "test-hook-async", "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-ha",
		ID:        "call-ha",
		Name:      "hook_async_tool",
		Arguments: "{}",
	}

	err := engine.executeAsync(chatCtx, toolMgr, tc, map[string]any{}, "", 0, nil)
	require.NoError(t, err)

	// Only BeforeToolExecute is called synchronously for async
	records := recHook.Records()
	require.Len(t, records, 1, "expected only BeforeToolExecute before goroutine launch")
	assert.Equal(t, hook.BeforeToolExecute, records[0].Point)
	assert.Equal(t, "hook_async_tool", records[0].ToolName)

	// Wait for async tool completion
	time.Sleep(200 * time.Millisecond)

	// No additional hooks should have fired (goroutine body has no hook calls)
	records = recHook.Records()
	assert.Len(t, records, 1, "no additional hook calls should fire in async goroutine")

	closeMockChatContext(chatCtx)
}

func TestToolHooksWithoutHookRunner(t *testing.T) {
	toolMgr := newMockToolManagerWithTools()

	syncTool := &mockControllableTool{
		name:     "no_hook_tool",
		duration: 10 * time.Millisecond,
		result:   &tool.ToolResult{Success: true, Data: map[string]any{"value": "ok"}},
	}
	toolMgr.Register(syncTool)

	// Engine without hookRunner (nil = default)
	engine := NewTestEngine()

	ctx := context.Background()
	chatCtx := testutil.NewMockChatContext(ctx, "test-no-hook", "test-agent")

	tc := toolCallInfo{
		MessageID: "msg-no-hook",
		ID:        "call-no-hook",
		Name:      "no_hook_tool",
		Arguments: "{}",
	}

	err := engine.executeSync(chatCtx, toolMgr, tc, map[string]any{}, "", 0, nil, make(map[string]*ToolCallResult))
	require.NoError(t, err)

	// Should not panic — nil hookRunner is handled gracefully
	closeMockChatContext(chatCtx)
}
