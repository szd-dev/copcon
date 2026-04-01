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

	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/memory"
	"github.com/copcon/server/internal/session"
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

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr)
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

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr)
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

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr)
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

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr)
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

	engine := NewAgentEngine(agentRegistry, sessionMgr, chat_context, memoryMgr)
	require.NotNil(t, engine)

	assert.NotNil(t, engine.agentRegistry)
	assert.NotNil(t, engine.sessionMgr)
	assert.NotNil(t, engine.contextMgr)
	assert.NotNil(t, engine.memoryMgr)
}
