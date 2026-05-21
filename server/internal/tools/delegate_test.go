package tools

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/agent"
	"github.com/copcon/server/internal/domain/entity"
	chatcontextpkg "github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/testutil"
	"github.com/copcon/server/internal/tool"
)

// --- Mock Agent Registry ---

type mockAgentRegistry struct {
	factories map[string]agent.AgentFactory
}

func newMockAgentRegistry() *mockAgentRegistry {
	return &mockAgentRegistry{factories: make(map[string]agent.AgentFactory)}
}

func (m *mockAgentRegistry) Get(id string) (agent.AgentDefinition, error) {
	return agent.AgentDefinition{}, agent.ErrAgentNotFound
}

func (m *mockAgentRegistry) List() []agent.AgentInfo {
	return nil
}

func (m *mockAgentRegistry) Default() (agent.AgentDefinition, error) {
	return agent.AgentDefinition{}, agent.ErrNoDefaultAgent
}

func (m *mockAgentRegistry) RegisterFactory(id, name, model string, allowDelegate bool, factory agent.AgentFactory) {
	m.factories[id] = factory
}

func (m *mockAgentRegistry) GetFactory(id string) (agent.AgentFactory, error) {
	f, ok := m.factories[id]
	if !ok {
		return nil, agent.ErrAgentNotFound
	}
	return f, nil
}

func (m *mockAgentRegistry) ListDelegatable() []agent.AgentInfo {
	return nil
}

// --- Mock Session Manager ---

type mockSessionManager struct {
	created *session.Session
}

func newMockSessionManager() *mockSessionManager {
	return &mockSessionManager{}
}

func (m *mockSessionManager) Create(chatCtx chatcontextpkg.ChatContextInterface, title, defaultAgentID string, opts ...session.CreateOption) (*session.Session, error) {
	s := &session.Session{
		ID:             uuid.New(),
		Title:          title,
		DefaultAgentID: defaultAgentID,
		Metadata:       make(map[string]any),
	}
	for _, opt := range opts {
		opt(s)
	}
	m.created = s
	return s, nil
}

func (m *mockSessionManager) Get(chatCtx chatcontextpkg.ChatContextInterface) (*session.Session, error) {
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionManager) List(chatCtx chatcontextpkg.ChatContextInterface, limit, offset int) ([]*session.Session, int64, error) {
	return nil, 0, nil
}

func (m *mockSessionManager) Delete(chatCtx chatcontextpkg.ChatContextInterface) error {
	return nil
}

func (m *mockSessionManager) UpdateTitle(chatCtx chatcontextpkg.ChatContextInterface, title string) error {
	return nil
}

func (m *mockSessionManager) UpdateMetadata(chatCtx chatcontextpkg.ChatContextInterface, metadata map[string]any) error {
	return nil
}

func (m *mockSessionManager) AddAsyncCompletionPending(chatCtx chatcontextpkg.ChatContextInterface, event map[string]any) error {
	return nil
}

func (m *mockSessionManager) GetMessageCount(chatCtx chatcontextpkg.ChatContextInterface) (int64, error) {
	return 0, nil
}

func (m *mockSessionManager) GetDB() *gorm.DB {
	return nil
}

// --- Mock Context Manager ---

type mockContextManager struct {
	messages []session.Message
}

func newMockContextManager() *mockContextManager {
	return &mockContextManager{}
}

func (m *mockContextManager) GetHistory(chatCtx chatcontextpkg.ChatContextInterface, limit int) ([]session.Message, error) {
	return m.messages, nil
}

func (m *mockContextManager) AddMessage(chatCtx chatcontextpkg.ChatContextInterface, msg *session.Message) error {
	m.messages = append(m.messages, *msg)
	return nil
}

func (m *mockContextManager) BuildContext(chatCtx chatcontextpkg.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]entity.MessageForLLM, error) {
	return nil, nil
}

func (m *mockContextManager) DeleteBySession(chatCtx chatcontextpkg.ChatContextInterface) error {
	return nil
}

// --- Mock Agent Engine ---

type mockAgentEngine struct {
	chatErr error
}

func newMockAgentEngine() *mockAgentEngine {
	return &mockAgentEngine{}
}

func (m *mockAgentEngine) Chat(chatCtx chatcontextpkg.ChatContextInterface, userInput string) error {
	err := m.chatErr
	time.Sleep(10 * time.Millisecond)
	chatCtx.Close()
	return err
}

// --- Tests ---

func TestDelegateToTool_SyncSuccess(t *testing.T) {
	agentRegistry := newMockAgentRegistry()
	sessionMgr := newMockSessionManager()
	contextMgr := newMockContextManager()
	engine := newMockAgentEngine()

	agentRegistry.RegisterFactory("sub-agent", "Sub Agent", "gpt-4o", true, func(ctx context.Context, params agent.CreateParams) (agent.AgentDefinition, error) {
		return agent.AgentDefinition{
			ID:           "sub-agent",
			Name:         "Sub Agent",
			Model:        "gpt-4o",
			SystemPrompt: params.Task,
		}, nil
	})

	tool := NewDelegateToTool(agentRegistry, sessionMgr, contextMgr, engine)

	parentSessionID := uuid.New()
	chatCtx := testutil.NewMockChatContext(context.Background(), parentSessionID.String(), "main-agent")

	result, err := tool.Execute(chatCtx, map[string]any{
		"agent_id": "sub-agent",
		"task":     "Run a diagnostic",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)

	data, ok := result.Data.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "completed", data["status"])
	assert.NotEmpty(t, data["sub_session_id"])

	assert.NotNil(t, sessionMgr.created)
	assert.Equal(t, "sub-agent", sessionMgr.created.DefaultAgentID)
	assert.NotNil(t, sessionMgr.created.ParentSessionID)
	assert.Equal(t, parentSessionID, *sessionMgr.created.ParentSessionID)

	// Verify task message was injected into sub-session
	assert.Len(t, contextMgr.messages, 1)
	assert.Equal(t, "user", contextMgr.messages[0].Role)
	assert.Equal(t, "Run a diagnostic", contextMgr.messages[0].Content)
}

func TestDelegateToTool_InvalidAgent(t *testing.T) {
	agentRegistry := newMockAgentRegistry()
	sessionMgr := newMockSessionManager()
	contextMgr := newMockContextManager()
	engine := newMockAgentEngine()

	tool := NewDelegateToTool(agentRegistry, sessionMgr, contextMgr, engine)

	chatCtx := testutil.NewMockChatContext(context.Background(), "session-id", "main-agent")

	result, err := tool.Execute(chatCtx, map[string]any{
		"agent_id": "nonexistent-agent",
		"task":     "Do something",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "agent not found")
}

func TestDelegateToTool_NoExecutionMode(t *testing.T) {
	tool := NewDelegateToTool(nil, nil, nil, nil)

	schema := tool.InputSchema()

	props, ok := schema["properties"].(map[string]any)
	assert.True(t, ok)

	_, hasExecutionMode := props["execution_mode"]
	assert.False(t, hasExecutionMode,
		"delegate_to schema must NOT contain execution_mode (collision with mode param)")

	mode, hasMode := props["mode"]
	assert.True(t, hasMode, "delegate_to schema must have its own mode param")

	modeMap, ok := mode.(map[string]any)
	assert.True(t, ok)
	assert.Contains(t, modeMap["enum"], "sync")
}

func TestDelegateToTool_MissingAgentID(t *testing.T) {
	tool := NewDelegateToTool(nil, nil, nil, nil)

	chatCtx := testutil.NewMockChatContext(context.Background(), "session-id", "main-agent")

	result, err := tool.Execute(chatCtx, map[string]any{
		"task": "Do something",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, "agent_id is required", result.Error)
}

func TestDelegateToTool_MissingTask(t *testing.T) {
	agentRegistry := newMockAgentRegistry()
	tool := NewDelegateToTool(agentRegistry, nil, nil, nil)

	agentRegistry.RegisterFactory("agent-1", "Agent 1", "gpt-4o", true, func(ctx context.Context, params agent.CreateParams) (agent.AgentDefinition, error) {
		return agent.AgentDefinition{}, nil
	})

	chatCtx := testutil.NewMockChatContext(context.Background(), "session-id", "main-agent")

	result, err := tool.Execute(chatCtx, map[string]any{
		"agent_id": "agent-1",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, "task is required", result.Error)
}

func TestDelegateToTool_IsDelegationTool(t *testing.T) {
	tool := NewDelegateToTool(nil, nil, nil, nil)
	assert.True(t, tool.IsDelegationTool())
}

func TestDelegateToTool_InterfaceCompliance(t *testing.T) {
	var _tool tool.Tool = NewDelegateToTool(nil, nil, nil, nil)
	var _delegation tool.DelegationTool = NewDelegateToTool(nil, nil, nil, nil)

	assert.Equal(t, "delegate_to", _tool.Name())
	assert.Equal(t, "Delegate a task to another agent", _tool.Description())
	assert.True(t, _delegation.IsDelegationTool())
}
