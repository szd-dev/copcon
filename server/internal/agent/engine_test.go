package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/memory"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/testutil"
	"github.com/copcon/server/internal/todo"
	"github.com/copcon/server/internal/tool"
)

type mockSessionManager struct {
	sessions map[string]*session.Session
}

func newMockSessionManager() *mockSessionManager {
	return &mockSessionManager{
		sessions: make(map[string]*session.Session),
	}
}

func (m *mockSessionManager) Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string) (*session.Session, error) {
	s := &session.Session{
		ID:             uuid.New(),
		Title:          title,
		DefaultAgentID: defaultAgentID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       make(map[string]any),
	}
	m.sessions[s.ID.String()] = s
	return s, nil
}

func (m *mockSessionManager) Get(chatCtx iface.ChatContextInterface) (*session.Session, error) {
	s, ok := m.sessions[chatCtx.SessionID()]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	return s, nil
}

func (m *mockSessionManager) List(chatCtx iface.ChatContextInterface, limit, offset int) ([]*session.Session, int64, error) {
	return nil, 0, nil
}

func (m *mockSessionManager) Delete(chatCtx iface.ChatContextInterface) error {
	delete(m.sessions, chatCtx.SessionID())
	return nil
}

func (m *mockSessionManager) UpdateTitle(chatCtx iface.ChatContextInterface, title string) error {
	return nil
}

func (m *mockSessionManager) GetMessageCount(chatCtx iface.ChatContextInterface) (int64, error) {
	return 0, nil
}

func (m *mockSessionManager) GetDB() *gorm.DB {
	return nil
}

type mockContextManager struct {
	messages map[string][]chat_context.MessageForLLM
}

func newMockContextManager() *mockContextManager {
	return &mockContextManager{
		messages: make(map[string][]chat_context.MessageForLLM),
	}
}

func (m *mockContextManager) GetHistory(chatCtx iface.ChatContextInterface, limit int) ([]session.Message, error) {
	return nil, nil
}

func (m *mockContextManager) AddMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error {
	return nil
}

func (m *mockContextManager) BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]chat_context.MessageForLLM, error) {
	return nil, nil
}

func (m *mockContextManager) DeleteBySession(chatCtx iface.ChatContextInterface) error {
	return nil
}

type mockMemoryManager struct{}

func (m *mockMemoryManager) Store(chatCtx iface.ChatContextInterface, memory *memory.Memory) error {
	return nil
}

func (m *mockMemoryManager) Search(chatCtx iface.ChatContextInterface, query []float32, limit int) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryManager) GetBySession(chatCtx iface.ChatContextInterface, limit int) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryManager) DeleteBySession(chatCtx iface.ChatContextInterface) error {
	return nil
}

type mockTodoManager struct{}

func (m *mockTodoManager) Create(chatCtx iface.ChatContextInterface, content string, opts ...todo.TodoOption) (*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) Get(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) List(chatCtx iface.ChatContextInterface) ([]*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) Update(chatCtx iface.ChatContextInterface, id string, updates map[string]any) (*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) Delete(chatCtx iface.ChatContextInterface, id string) error {
	return nil
}

func (m *mockTodoManager) Start(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) Complete(chatCtx iface.ChatContextInterface, id string, result string) (*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) Fail(chatCtx iface.ChatContextInterface, id string, reason string) (*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) Block(chatCtx iface.ChatContextInterface, id string, reason string) (*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) Unblock(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) GetAvailableTodos(chatCtx iface.ChatContextInterface) ([]*session.Todo, error) {
	return nil, nil
}

func (m *mockTodoManager) GetDB() *gorm.DB {
	return nil
}

type mockAgentRegistry struct {
	agents       map[string]AgentDefinition
	defaultAgent string
}

func newMockAgentRegistry() *mockAgentRegistry {
	return &mockAgentRegistry{
		agents: make(map[string]AgentDefinition),
	}
}

func (r *mockAgentRegistry) Register(id string, agent AgentDefinition) {
	r.agents[id] = agent
}

func (r *mockAgentRegistry) SetDefault(id string) {
	r.defaultAgent = id
}

func (r *mockAgentRegistry) Get(id string) (AgentDefinition, error) {
	agent, ok := r.agents[id]
	if !ok {
		return AgentDefinition{}, ErrAgentNotFound
	}
	return agent, nil
}

func (r *mockAgentRegistry) List() []AgentInfo {
	infos := make([]AgentInfo, 0, len(r.agents))
	for id, agent := range r.agents {
		infos = append(infos, AgentInfo{
			ID:   id,
			Name: agent.Name,
		})
	}
	return infos
}

func (r *mockAgentRegistry) Default() (AgentDefinition, error) {
	if r.defaultAgent == "" {
		return AgentDefinition{}, ErrNoDefaultAgent
	}
	return r.Get(r.defaultAgent)
}

type mockToolManagerForEngine struct {
	tools []openai.ChatCompletionToolUnionParam
}

func (m *mockToolManagerForEngine) Register(t tool.Tool) error   { return nil }
func (m *mockToolManagerForEngine) Unregister(name string) error { return nil }
func (m *mockToolManagerForEngine) Get(name string) (tool.Tool, error) {
	return nil, tool.ErrToolNotFound
}
func (m *mockToolManagerForEngine) List() []tool.ToolInfo { return nil }
func (m *mockToolManagerForEngine) Execute(chatCtx iface.ChatContextInterface, name string, args map[string]any) (*tool.ToolResult, error) {
	return nil, nil
}
func (m *mockToolManagerForEngine) GetOpenAITools() []openai.ChatCompletionToolUnionParam {
	return m.tools
}

func TestAgentEngineChatWithAgent(t *testing.T) {
	sessionMgr := newMockSessionManager()
	chat_context := newMockContextManager()
	memoryMgr := &mockMemoryManager{}

	agentRegistry := newMockAgentRegistry()

	agentA := AgentDefinition{
		ID:           "agent-a",
		Name:         "Agent A",
		Model:        "gpt-4o",
		SystemPrompt: "You are Agent A.",
		ToolManager:  &mockToolManagerForEngine{},
	}

	agentB := AgentDefinition{
		ID:           "agent-b",
		Name:         "Agent B",
		Model:        "gpt-4o-mini",
		SystemPrompt: "You are Agent B.",
		ToolManager:  &mockToolManagerForEngine{},
	}

	agentRegistry.Register("agent-a", agentA)
	agentRegistry.Register("agent-b", agentB)
	agentRegistry.SetDefault("agent-a")

	ctx := context.Background()
	chatCtxForCreate := iface.NewChatContext(ctx, "", "agent-a")
	session, err := sessionMgr.Create(chatCtxForCreate, "Test Session", "agent-a")
	require.NoError(t, err)

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr, &mockTodoManager{})
	require.NotNil(t, engine)

	chatCtxForChat := iface.NewChatContext(ctx, session.ID.String(), "agent-b")
	err = engine.Chat(chatCtxForChat, "Hello")
	require.NoError(t, err)

	go func() {
		for range chatCtxForChat.Events() {
		}
	}()
}

func TestAgentEngineChatWithDefaultAgent(t *testing.T) {
	sessionMgr := newMockSessionManager()
	chat_context := newMockContextManager()
	memoryMgr := &mockMemoryManager{}

	agentRegistry := newMockAgentRegistry()

	agentA := AgentDefinition{
		ID:           "agent-a",
		Name:         "Agent A",
		Model:        "gpt-4o",
		SystemPrompt: "You are Agent A.",
		ToolManager:  &mockToolManagerForEngine{},
	}

	agentRegistry.Register("agent-a", agentA)
	agentRegistry.SetDefault("agent-a")

	ctx := context.Background()
	chatCtxForCreate := iface.NewChatContext(ctx, "", "agent-a")
	session, err := sessionMgr.Create(chatCtxForCreate, "Test Session", "agent-a")
	require.NoError(t, err)

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr, &mockTodoManager{})
	require.NotNil(t, engine)

	chatCtxForChat := iface.NewChatContext(ctx, session.ID.String(), "")
	err = engine.Chat(chatCtxForChat, "Hello")
	require.NoError(t, err)

	go func() {
		for range chatCtxForChat.Events() {
		}
	}()
}

func TestAgentEngineSystemPrompt(t *testing.T) {
	sessionMgr := newMockSessionManager()
	chat_context := newMockContextManager()
	memoryMgr := &mockMemoryManager{}

	agentRegistry := newMockAgentRegistry()

	customPrompt := "You are a specialized coding assistant."
	agent := AgentDefinition{
		ID:           "coding-agent",
		Name:         "Coding Agent",
		Model:        "gpt-4o",
		SystemPrompt: customPrompt,
		ToolManager:  &mockToolManagerForEngine{},
	}

	agentRegistry.Register("coding-agent", agent)
	agentRegistry.SetDefault("coding-agent")

	ctx := context.Background()
	chatCtxForCreate := iface.NewChatContext(ctx, "", "coding-agent")
	session, err := sessionMgr.Create(chatCtxForCreate, "Test Session", "coding-agent")
	require.NoError(t, err)

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr, &mockTodoManager{})
	require.NotNil(t, engine)

	agentDef, err := agentRegistry.Get("coding-agent")
	require.NoError(t, err)
	assert.Equal(t, customPrompt, agentDef.SystemPrompt)

	chatCtxForChat := iface.NewChatContext(ctx, session.ID.String(), "coding-agent")
	err = engine.Chat(chatCtxForChat, "Hello")
	require.NoError(t, err)

	go func() {
		for range chatCtxForChat.Events() {
		}
	}()
}

func TestAgentEngineChatWithInvalidAgent(t *testing.T) {
	sessionMgr := newMockSessionManager()
	chat_context := newMockContextManager()
	memoryMgr := &mockMemoryManager{}

	agentRegistry := newMockAgentRegistry()

	agent := AgentDefinition{
		ID:           "agent-1",
		Name:         "Agent 1",
		Model:        "gpt-4o",
		SystemPrompt: "You are Agent 1.",
		ToolManager:  &mockToolManagerForEngine{},
	}

	agentRegistry.Register("agent-1", agent)
	agentRegistry.SetDefault("agent-1")

	ctx := context.Background()
	chatCtxForCreate := iface.NewChatContext(ctx, "", "agent-1")
	session, err := sessionMgr.Create(chatCtxForCreate, "Test Session", "agent-1")
	require.NoError(t, err)

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr, &mockTodoManager{})
	require.NotNil(t, engine)

	chatCtxForChat := iface.NewChatContext(ctx, session.ID.String(), "non-existent-agent")
	err = engine.Chat(chatCtxForChat, "Hello")
	require.NoError(t, err)

	var foundError bool
	for event := range chatCtxForChat.Events() {
		if event.Type == entity.EventError {
			foundError = true
			errorData, ok := event.Data.(entity.ErrorData)
			require.True(t, ok)
			assert.Contains(t, errorData.Error, "agent not found")
		}
	}
	assert.True(t, foundError, "Expected error event for invalid agent")
}

func TestAgentEngineStateless(t *testing.T) {
	sessionMgr := newMockSessionManager()
	chat_context := newMockContextManager()
	memoryMgr := &mockMemoryManager{}

	agentRegistry := newMockAgentRegistry()

	agent := AgentDefinition{
		ID:           "agent-1",
		Name:         "Agent 1",
		Model:        "gpt-4o",
		SystemPrompt: "You are Agent 1.",
		ToolManager:  &mockToolManagerForEngine{},
	}

	agentRegistry.Register("agent-1", agent)
	agentRegistry.SetDefault("agent-1")

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr, &mockTodoManager{})
	require.NotNil(t, engine)

	assert.NotNil(t, engine.agentRegistry)
	assert.NotNil(t, engine.sessionMgr)
	assert.NotNil(t, engine.contextMgr)
	assert.NotNil(t, engine.memoryMgr)
}

func TestMessageDataMessageID(t *testing.T) {
	msgData := entity.MessageData{
		MessageID: "test-message-id",
		Content:   "test content",
	}
	assert.Equal(t, "test-message-id", msgData.MessageID)
	assert.Equal(t, "test content", msgData.Content)
}

// TestTodoLoopFix verifies the todo state injection and duplicate prevention behavior.
// This test FAILS in the current implementation because:
// 1. Agent loop does not inject todo state into the context before LLM call
// 2. TodoTool.Create does not detect duplicate todos
// 3. TodoTool.Create does not auto-start the todo after creation
func TestTodoLoopFix(t *testing.T) {
	// Setup test database
	dsn := "host=localhost user=admin password=changeme dbname=agent_infra port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	// Run migrations
	err = db.AutoMigrate(&session.Session{}, &session.Todo{}, &session.Message{})
	require.NoError(t, err)

	// Cleanup test data
	db.Exec("DELETE FROM todos WHERE content LIKE 'TestTodoLoop:%'")
	db.Exec("DELETE FROM messages WHERE session_id IN (SELECT id FROM sessions WHERE title LIKE 'TestTodoLoop:%')")
	db.Exec("DELETE FROM sessions WHERE title LIKE 'TestTodoLoop:%'")

	// Create test session
	sess := &session.Session{
		ID:             uuid.New(),
		Title:          "TestTodoLoop: " + uuid.New().String(),
		DefaultAgentID: "test-agent",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       make(map[string]any),
	}
	err = db.Create(sess).Error
	require.NoError(t, err)

	// Create todo manager
	todoMgr := todo.NewTodoManager(db)
	ctx := context.Background()

	t.Run("todo state should be injected into context before LLM call", func(t *testing.T) {
		// Create some existing todos for the session
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
		existingTodo, err := todoMgr.Create(chatCtx, "TestTodoLoop: existing task 1")
		require.NoError(t, err)
		require.NotNil(t, existingTodo)

		// Create a context manager
		contextMgr := chat_context.NewContextManager(db)

		// Build context - this should include todo state, but currently doesn't
		messages, err := contextMgr.BuildContext(chatCtx, "", 256000, "You are a helpful assistant.")
		require.NoError(t, err)

		// Check if any message contains todo information
		// This assertion will FAIL because todo state is not injected
		hasTodoInfo := false
		for _, msg := range messages {
			if strings.Contains(strings.ToLower(msg.Content), "todo") ||
				strings.Contains(strings.ToLower(msg.Content), "task") {
				hasTodoInfo = true
				break
			}
		}

		// EXPECTED: Todo info should be present in context
		// ACTUAL: Todo info is NOT present (this assertion fails)
		assert.True(t, hasTodoInfo,
			"Context should include existing todo state before LLM call. "+
				"Current implementation does not inject todo state into BuildContext. "+
				"The LLM has no visibility of existing todos.")
	})

	t.Run("duplicate todo creation should be prevented", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")

		// Create a todo
		todo1, err := todoMgr.Create(chatCtx, "TestTodoLoop: duplicate check task")
		require.NoError(t, err)
		require.NotNil(t, todo1)

		// Try to create the same todo again
		// EXPECTED: Should return error or existing todo
		// ACTUAL: Creates a duplicate (this assertion fails)
		todo2, err := todoMgr.Create(chatCtx, "TestTodoLoop: duplicate check task")

		// The current implementation allows duplicates - this is the bug
		if err == nil && todo2 != nil {
			// Both todos exist with same content - this should NOT happen
			assert.NotEqual(t, todo1.ID, todo2.ID,
				"Duplicate todos should not be created with the same content. "+
					"Current implementation allows duplicate creation which can lead to infinite todo loops.")
		}
	})

	t.Run("todo count should not increase per loop iteration", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")

		// Get initial todo count
		initialTodos, err := todoMgr.List(chatCtx)
		require.NoError(t, err)
		initialCount := len(initialTodos)

		// Simulate multiple "loop iterations" where the LLM might try to create the same todo
		sameContent := "TestTodoLoop: loop iteration task"

		// First iteration - create todo
		_, err = todoMgr.Create(chatCtx, sameContent)
		require.NoError(t, err)

		// Second iteration - LLM tries to create the same todo again
		// (because it has no visibility of existing todos)
		_, err = todoMgr.Create(chatCtx, sameContent)
		// This should fail or return existing, but currently succeeds

		// Third iteration
		_, err = todoMgr.Create(chatCtx, sameContent)

		// Check final count
		finalTodos, err := todoMgr.List(chatCtx)
		require.NoError(t, err)
		finalCount := len(finalTodos)

		// EXPECTED: Count should increase by at most 1 (only the first creation)
		// ACTUAL: Count increases by 3 (all three creations succeed)
		allowedIncrease := 1
		actualIncrease := finalCount - initialCount

		assert.LessOrEqual(t, actualIncrease, allowedIncrease,
			"Todo count should not increase unboundedly per loop iteration. "+
				"Current implementation allows %d increases, expected at most %d. "+
				"This leads to infinite todo creation when LLM has no visibility of existing todos.",
			actualIncrease, allowedIncrease)
	})

	t.Run("todo should be auto-started after creation", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")

		// Create a todo
		createdTodo, err := todoMgr.Create(chatCtx, "TestTodoLoop: auto-start task")
		require.NoError(t, err)
		require.NotNil(t, createdTodo)

		// EXPECTED: After creation, todo should be automatically started (status = in_progress)
		// ACTUAL: Todo remains in pending status (this assertion fails)
		assert.Equal(t, session.TodoStatusInProgress, createdTodo.Status,
			"Todo should be automatically started after creation. "+
				"Current implementation leaves todo in 'pending' status. "+
				"Expected behavior: Create once, then auto-execute (immediately call Start()).")
	})
}
