package agent

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	contextmgr "github.com/copcon/server/internal/context"
	"github.com/copcon/server/internal/memory"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
)

// mockSessionManager is a mock implementation of session.SessionManager
type mockSessionManager struct {
	sessions map[string]*session.Session
}

func newMockSessionManager() *mockSessionManager {
	return &mockSessionManager{
		sessions: make(map[string]*session.Session),
	}
}

func (m *mockSessionManager) Create(ctx context.Context, title, defaultAgentID string) (*session.Session, error) {
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

func (m *mockSessionManager) Get(ctx context.Context, id string) (*session.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	return s, nil
}

func (m *mockSessionManager) List(ctx context.Context, limit, offset int) ([]*session.Session, int64, error) {
	return nil, 0, nil
}

func (m *mockSessionManager) Delete(ctx context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

func (m *mockSessionManager) UpdateTitle(ctx context.Context, id, title string) error {
	return nil
}

func (m *mockSessionManager) GetMessageCount(ctx context.Context, sessionID string) (int64, error) {
	return 0, nil
}

func (m *mockSessionManager) GetDB() *gorm.DB {
	return nil
}

// mockContextManager is a mock implementation of context.ContextManager
type mockContextManager struct {
	messages map[string][]contextmgr.MessageForLLM
}

func newMockContextManager() *mockContextManager {
	return &mockContextManager{
		messages: make(map[string][]contextmgr.MessageForLLM),
	}
}

func (m *mockContextManager) GetHistory(ctx context.Context, sessionID string, limit int) ([]session.Message, error) {
	return nil, nil
}

func (m *mockContextManager) AddMessage(ctx context.Context, sessionID string, msg *session.Message) error {
	return nil
}

func (m *mockContextManager) BuildContext(ctx context.Context, sessionID string, userInput string, maxTokens int, systemPrompt string) ([]contextmgr.MessageForLLM, error) {
	return nil, nil
}

func (m *mockContextManager) DeleteBySession(ctx context.Context, sessionID string) error {
	return nil
}

// mockMemoryManager is a mock implementation of memory.MemoryManager
type mockMemoryManager struct{}

func (m *mockMemoryManager) Store(ctx context.Context, memory *memory.Memory) error {
	return nil
}

func (m *mockMemoryManager) Search(ctx context.Context, query []float32, limit int, sessionID string) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryManager) GetBySession(ctx context.Context, sessionID string, limit int) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryManager) DeleteBySession(ctx context.Context, sessionID string) error {
	return nil
}

// mockAgentRegistry is a mock implementation of AgentRegistry
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

// mockToolManagerForEngine is a mock tool manager for engine tests
type mockToolManagerForEngine struct {
	tools []openai.ChatCompletionToolUnionParam
}

func (m *mockToolManagerForEngine) Register(t tool.Tool) error   { return nil }
func (m *mockToolManagerForEngine) Unregister(name string) error { return nil }
func (m *mockToolManagerForEngine) Get(name string) (tool.Tool, error) {
	return nil, tool.ErrToolNotFound
}
func (m *mockToolManagerForEngine) List() []tool.ToolInfo { return nil }
func (m *mockToolManagerForEngine) Execute(ctx context.Context, name string, args map[string]any) (*tool.ToolResult, error) {
	return nil, nil
}
func (m *mockToolManagerForEngine) GetOpenAITools() []openai.ChatCompletionToolUnionParam {
	return m.tools
}

func TestAgentEngineChatWithAgent(t *testing.T) {
	sessionMgr := newMockSessionManager()
	contextMgr := newMockContextManager()
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
	session, err := sessionMgr.Create(ctx, "Test Session", "agent-a")
	require.NoError(t, err)

	engine := NewAgentEngine(agentRegistry, sessionMgr, contextMgr, memoryMgr)
	require.NotNil(t, engine)

	events, err := engine.Chat(ctx, session.ID.String(), "agent-b", "Hello")
	require.NoError(t, err)
	require.NotNil(t, events)

	go func() {
		for range events {
		}
	}()
}

func TestAgentEngineChatWithDefaultAgent(t *testing.T) {
	sessionMgr := newMockSessionManager()
	contextMgr := newMockContextManager()
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
	session, err := sessionMgr.Create(ctx, "Test Session", "agent-a")
	require.NoError(t, err)

	engine := NewAgentEngine(agentRegistry, sessionMgr, contextMgr, memoryMgr)
	require.NotNil(t, engine)

	events, err := engine.Chat(ctx, session.ID.String(), "", "Hello")
	require.NoError(t, err)
	require.NotNil(t, events)

	go func() {
		for range events {
		}
	}()
}

func TestAgentEngineSystemPrompt(t *testing.T) {
	sessionMgr := newMockSessionManager()
	contextMgr := newMockContextManager()
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
	session, err := sessionMgr.Create(ctx, "Test Session", "coding-agent")
	require.NoError(t, err)

	engine := NewAgentEngine(agentRegistry, sessionMgr, contextMgr, memoryMgr)
	require.NotNil(t, engine)

	agentDef, err := agentRegistry.Get("coding-agent")
	require.NoError(t, err)
	assert.Equal(t, customPrompt, agentDef.SystemPrompt)

	events, err := engine.Chat(ctx, session.ID.String(), "coding-agent", "Hello")
	require.NoError(t, err)
	require.NotNil(t, events)

	go func() {
		for range events {
		}
	}()
}

func TestAgentEngineChatWithInvalidAgent(t *testing.T) {
	sessionMgr := newMockSessionManager()
	contextMgr := newMockContextManager()
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
	session, err := sessionMgr.Create(ctx, "Test Session", "agent-1")
	require.NoError(t, err)

	engine := NewAgentEngine(agentRegistry, sessionMgr, contextMgr, memoryMgr)
	require.NotNil(t, engine)

	events, err := engine.Chat(ctx, session.ID.String(), "non-existent-agent", "Hello")
	require.NoError(t, err)
	require.NotNil(t, events)

	// Error should be sent through events channel as EventError
	var foundError bool
	for event := range events {
		if event.Type == EventError {
			foundError = true
			errorData, ok := event.Data.(ErrorData)
			require.True(t, ok)
			assert.Contains(t, errorData.Error, "agent not found")
		}
	}
	assert.True(t, foundError, "Expected error event for invalid agent")
}

func TestAgentEngineStateless(t *testing.T) {
	sessionMgr := newMockSessionManager()
	contextMgr := newMockContextManager()
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

	engine := NewAgentEngine(agentRegistry, sessionMgr, contextMgr, memoryMgr)
	require.NotNil(t, engine)

	assert.NotNil(t, engine.agentRegistry)
	assert.NotNil(t, engine.sessionMgr)
	assert.NotNil(t, engine.contextMgr)
	assert.NotNil(t, engine.memoryMgr)
}
