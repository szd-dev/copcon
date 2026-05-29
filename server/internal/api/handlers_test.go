package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/chat"
	"github.com/copcon/core/chatcontext"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/server/internal/config"
)

type testStoreProvider struct {
	sessionStore *mockSessionStore
	messageStore *mockMessageStore
	todoStore    *mockTodoStore
}

func (p *testStoreProvider) Sessions() storage.SessionStore   { return p.sessionStore }
func (p *testStoreProvider) Messages() storage.MessageStore   { return p.messageStore }
func (p *testStoreProvider) Todos() storage.TodoStore         { return p.todoStore }
func (p *testStoreProvider) Knowledge() storage.KnowledgeStore { return nil }

type testHarness struct {
	store         *testStoreProvider
	engine        agent.AgentEngine
	agentRegistry agent.AgentRegistry
}

func (h *testHarness) Store() storage.StoreProvider        { return h.store }
func (h *testHarness) Engine() agent.AgentEngine           { return h.engine }
func (h *testHarness) Registry() agent.AgentRegistry       { return h.agentRegistry }
func (h *testHarness) ActiveSessions() chat.ActiveSessions { return chat.NewActiveSessions() }

type mockSessionStore struct {
	sessions map[uuid.UUID]*storage.Session
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{
		sessions: make(map[uuid.UUID]*storage.Session),
	}
}

func (m *mockSessionStore) Create(_ context.Context, s *storage.Session) (*storage.Session, error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	m.sessions[s.ID] = s
	return s, nil
}

func (m *mockSessionStore) Get(_ context.Context, id uuid.UUID) (*storage.Session, error) {
	sess, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return sess, nil
}

func (m *mockSessionStore) List(_ context.Context, limit, offset int) ([]*storage.Session, int64, error) {
	var list []*storage.Session
	for _, s := range m.sessions {
		list = append(list, s)
	}
	return list, int64(len(list)), nil
}

func (m *mockSessionStore) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.sessions[id]; !ok {
		return fmt.Errorf("session not found")
	}
	delete(m.sessions, id)
	return nil
}

func (m *mockSessionStore) UpdateTitle(_ context.Context, id uuid.UUID, title string) error {
	sess, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.Title = title
	return nil
}

func (m *mockSessionStore) UpdateMetadata(_ context.Context, id uuid.UUID, metadata map[string]any) error {
	return nil
}

func (m *mockSessionStore) GetMessageCount(_ context.Context, sessionID uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockSessionStore) AppendMetadata(_ context.Context, id uuid.UUID, key string, value any) error {
	return nil
}

type mockTodoStore struct{}

func (m *mockTodoStore) Create(_ context.Context, t *storage.Todo) (*storage.Todo, error) {
	return t, nil
}
func (m *mockTodoStore) Get(_ context.Context, id uuid.UUID) (*storage.Todo, error) {
	return nil, fmt.Errorf("todo not found")
}
func (m *mockTodoStore) List(_ context.Context, sessionID uuid.UUID) ([]*storage.Todo, error) {
	return nil, nil
}
func (m *mockTodoStore) UpdateStatus(_ context.Context, id uuid.UUID, status storage.TodoStatus) (*storage.Todo, error) {
	return nil, fmt.Errorf("todo not found")
}
func (m *mockTodoStore) DeleteBySession(_ context.Context, sessionID uuid.UUID) error {
	return nil
}

type mockMessageStore struct{}

func (m *mockMessageStore) List(_ context.Context, sessionID uuid.UUID, limit int) ([]*storage.Message, error) {
	return nil, nil
}
func (m *mockMessageStore) Add(_ context.Context, message *storage.Message) error {
	return nil
}
func (m *mockMessageStore) Update(_ context.Context, message *storage.Message) error {
	return nil
}
func (m *mockMessageStore) Upsert(_ context.Context, message *storage.Message) error {
	return nil
}
func (m *mockMessageStore) DeleteBySession(_ context.Context, sessionID uuid.UUID) error {
	return nil
}

type mockAgentRegistry struct {
	agents       map[string]agent.AgentDefinition
	defaultAgent string
}

func newMockAgentRegistry(defaultAgent string) *mockAgentRegistry {
	return &mockAgentRegistry{
		agents:       make(map[string]agent.AgentDefinition),
		defaultAgent: defaultAgent,
	}
}

func (r *mockAgentRegistry) Get(id string) (agent.AgentDefinition, error) {
	def, ok := r.agents[id]
	if !ok {
		return agent.AgentDefinition{}, agent.ErrAgentNotFound
	}
	return def, nil
}

func (r *mockAgentRegistry) List() []agent.AgentInfo {
	var list []agent.AgentInfo
	for id, def := range r.agents {
		list = append(list, agent.AgentInfo{
			ID:    id,
			Name:  def.Name,
			Model: def.Model,
		})
	}
	return list
}

func (r *mockAgentRegistry) Default() (agent.AgentDefinition, error) {
	if r.defaultAgent == "" {
		return agent.AgentDefinition{}, agent.ErrNoDefaultAgent
	}
	return r.Get(r.defaultAgent)
}

func (r *mockAgentRegistry) RegisterFactory(id, name, model string, allowDelegate bool, factory agent.AgentFactory) {
}

func (r *mockAgentRegistry) GetFactory(id string) (agent.AgentFactory, error) {
	return nil, agent.ErrAgentNotFound
}

func (r *mockAgentRegistry) ListDelegatable() []agent.AgentInfo {
	return nil
}

func setupTestHandler(t *testing.T) (*Handler, func()) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		DefaultAgentID: "default-agent",
		Agents: []config.AgentConfig{
			{ID: "default-agent", Name: "Default"},
			{ID: "code-assistant", Name: "Code Assistant"},
		},
	}

	sessionStore := newMockSessionStore()
	todoStore := &mockTodoStore{}
	messageStore := &mockMessageStore{}
	agentRegistry := newMockAgentRegistry("default-agent")

	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}
	agentRegistry.agents["code-assistant"] = agent.AgentDefinition{ID: "code-assistant", Name: "Code Assistant", Model: "gpt-4o"}

	handler := NewHandler(cfg, &testHarness{store: &testStoreProvider{sessionStore, messageStore, todoStore}, agentRegistry: agentRegistry})

	cleanup := func() {}

	return handler, cleanup
}

func TestCreateSessionWithAgent(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)

	reqBody := map[string]string{
		"title":            "Test Chat",
		"default_agent_id": "code-assistant",
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Test Chat", response["title"])
	assert.Equal(t, "code-assistant", response["default_agent_id"])
	assert.NotEmpty(t, response["id"])
	assert.NotNil(t, response["created_at"])
	assert.NotNil(t, response["updated_at"])
}

func TestCreateSessionWithDefaultAgent(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)

	reqBody := map[string]string{
		"title": "Test Chat",
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Test Chat", response["title"])
	assert.Equal(t, "default-agent", response["default_agent_id"])
}

func TestCreateSessionWithEmptyBody(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)

	req, _ := http.NewRequest("POST", "/api/sessions", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "New Chat", response["title"])
	assert.Equal(t, "default-agent", response["default_agent_id"])
}

func TestCreateSessionOnlyTitle(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)

	reqBody := map[string]string{
		"title": "Custom Title",
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Custom Title", response["title"])
	assert.Equal(t, "default-agent", response["default_agent_id"])
}

func TestCreateSessionOnlyAgentID(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)

	reqBody := map[string]string{
		"default_agent_id": "code-assistant",
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "New Chat", response["title"])
	assert.Equal(t, "code-assistant", response["default_agent_id"])
}

func TestCreateSessionInvalidJSON(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)

	req, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response["error"], "invalid")
}

func TestListAgents(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	router := gin.New()
	router.GET("/api/agents", handler.ListAgents)

	req, _ := http.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	agents, ok := response["agents"].([]interface{})
	require.True(t, ok)
	assert.Len(t, agents, 2)

	agent0 := agents[0].(map[string]interface{})
	assert.Equal(t, "default-agent", agent0["id"])
	assert.Equal(t, "Default", agent0["name"])
	assert.Equal(t, "gpt-4o", agent0["model"])

	agent1 := agents[1].(map[string]interface{})
	assert.Equal(t, "code-assistant", agent1["id"])
	assert.Equal(t, "Code Assistant", agent1["name"])
	assert.Equal(t, "gpt-4o", agent1["model"])
}

func TestBackfillParts_UserMessage(t *testing.T) {
	msg := storage.Message{
		ID:      uuid.New(),
		Role:    "user",
		Content: "Hello",
	}
	parts := BackfillParts(msg, nil)
	require.Len(t, parts, 1)
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "Hello", parts[0].Text)
	assert.Equal(t, "done", parts[0].State)
	assert.Equal(t, 0, parts[0].StepIndex)
}

func TestBackfillParts_AssistantWithToolCalls(t *testing.T) {
	msg := storage.Message{
		ID:        uuid.New(),
		Role:      "assistant",
		Reasoning: "Thinking...",
		Content:   "Let me check.",
		ToolCalls: []storage.ToolCall{
			{ID: "call_1", Type: "function", Function: storage.FunctionCall{Name: "bash", Arguments: `{"cmd":"ls"}`}},
		},
	}
	toolResults := map[string]string{"call_1": "file.txt"}
	parts := BackfillParts(msg, toolResults)
	require.Len(t, parts, 3)

	assert.Equal(t, "reasoning", parts[0].Type)
	assert.Equal(t, "Thinking...", parts[0].Text)
	assert.Equal(t, 0, parts[0].StepIndex)

	assert.Equal(t, "text", parts[1].Type)
	assert.Equal(t, "Let me check.", parts[1].Text)
	assert.Equal(t, 0, parts[1].StepIndex)

	assert.Equal(t, "tool-call", parts[2].Type)
	assert.Equal(t, "call_1", parts[2].ToolCallID)
	assert.Equal(t, "bash", parts[2].ToolName)
	assert.Equal(t, "file.txt", parts[2].Output)
	assert.Equal(t, 0, parts[2].StepIndex)
}

func TestBackfillParts_AssistantToolCallOnly(t *testing.T) {
	msg := storage.Message{
		ID:   uuid.New(),
		Role: "assistant",
		ToolCalls: []storage.ToolCall{
			{ID: "call_2", Type: "function", Function: storage.FunctionCall{Name: "python", Arguments: `{"code":"1+1"}`}},
		},
	}
	parts := BackfillParts(msg, nil)
	require.Len(t, parts, 1)
	assert.Equal(t, "tool-call", parts[0].Type)
	assert.Equal(t, "call_2", parts[0].ToolCallID)
	assert.Equal(t, 0, parts[0].StepIndex)
}

func TestGroupPartsByStep_SingleStep(t *testing.T) {
	parts := []storage.Part{
		{Type: "text", Text: "Hello", StepIndex: 0},
		{Type: "tool-call", ToolCallID: "c1", StepIndex: 0},
	}
	steps := GroupPartsByStep(parts)
	require.Len(t, steps, 1)
	assert.Equal(t, entity.UIPartStateDone, steps[0].State)
	require.Len(t, steps[0].Parts, 2)
	assert.Equal(t, entity.UIPartText, steps[0].Parts[0].Type)
	assert.Equal(t, entity.UIPartToolCall, steps[0].Parts[1].Type)
}

func TestGroupPartsByStep_MultipleSteps(t *testing.T) {
	parts := []storage.Part{
		{Type: "text", Text: "Step 0", StepIndex: 0},
		{Type: "text", Text: "Step 1", StepIndex: 1},
		{Type: "tool-call", ToolCallID: "c1", StepIndex: 1},
	}
	steps := GroupPartsByStep(parts)
	require.Len(t, steps, 2)
	require.Len(t, steps[0].Parts, 1)
	assert.Equal(t, "Step 0", steps[0].Parts[0].Text)
	require.Len(t, steps[1].Parts, 2)
}

func TestGroupPartsByStep_Empty(t *testing.T) {
	steps := GroupPartsByStep(nil)
	assert.Len(t, steps, 0)
}

type mockAgentEngine struct {
	chatFn func(chatCtx iface.ChatContextInterface, userInput string) error
}

func (m *mockAgentEngine) Chat(chatCtx iface.ChatContextInterface, userInput string) error {
	if m.chatFn != nil {
		return m.chatFn(chatCtx, userInput)
	}
	return nil
}

func setupChatTestHandler(t *testing.T, mockAgent *mockAgentEngine) (*Handler, chat.ActiveSessions, func()) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		DefaultAgentID: "default-agent",
		Agents: []config.AgentConfig{
			{ID: "default-agent", Name: "Default"},
			{ID: "code-assistant", Name: "Code Assistant"},
		},
	}

	sessionStore := newMockSessionStore()
	todoStore := &mockTodoStore{}
	messageStore := &mockMessageStore{}
	agentRegistry := newMockAgentRegistry("default-agent")

	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}
	agentRegistry.agents["code-assistant"] = agent.AgentDefinition{ID: "code-assistant", Name: "Code Assistant", Model: "gpt-4o"}

	handler := NewHandler(cfg, &testHarness{store: &testStoreProvider{sessionStore, messageStore, todoStore}, engine: mockAgent, agentRegistry: agentRegistry})

	cleanup := func() {}

	return handler, handler.chatStore, cleanup
}

func createSessionViaHandler(t *testing.T, router *gin.Engine) string {
	req, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"title":"Test Session"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	return resp["id"].(string)
}

func parseSSEEvents(t *testing.T, body string) []map[string]interface{} {
	t.Helper()
	var events []map[string]interface{}
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			var event map[string]interface{}
			err := json.Unmarshal([]byte(line[6:]), &event)
			require.NoError(t, err, "failed to parse SSE event: %s", line[6:])
			events = append(events, event)
		}
	}
	return events
}

func TestChat_FirstConnect(t *testing.T) {
	emitDone := make(chan struct{})
	agent := &mockAgentEngine{
		chatFn: func(chatCtx iface.ChatContextInterface, userInput string) error {
			chatCtx.Emit(entity.Event{Type: entity.EventMessage, Data: entity.MessageData{MessageID: "m1", Content: "hello"}})
			chatCtx.Emit(entity.Event{Type: entity.EventDone, Data: entity.DoneData{MessageID: "m1"}})
			close(emitDone)
			return nil
		},
	}
	handler, _, cleanup := setupChatTestHandler(t, agent)
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)
	router.POST("/api/sessions/:sessionId/chat", handler.Chat)

	sessionID := createSessionViaHandler(t, router)

	body := `{"content":"hello","agent_id":"default-agent"}`
	req, _ := http.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

	events := parseSSEEvents(t, w.Body.String())
	require.Len(t, events, 2, "expected 2 SSE events")
	assert.Equal(t, string(entity.EventMessage), events[0]["type"])
	assert.Equal(t, string(entity.EventDone), events[1]["type"])
}

func TestChat_Reconnect(t *testing.T) {
	handler, store, cleanup := setupChatTestHandler(t, &mockAgentEngine{})
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)
	router.POST("/api/sessions/:sessionId/chat", handler.Chat)

	sessionID := createSessionViaHandler(t, router)

	chatCtx := chatcontext.NewChatContext(context.Background(), sessionID, "")
	chatCtx.Emit(entity.Event{Type: entity.EventMessage, Data: entity.MessageData{MessageID: "m1", Content: "existing"}})
	chatCtx.Emit(entity.Event{Type: entity.EventDone, Data: entity.DoneData{MessageID: "m1"}})
	store.Put(sessionID, chatCtx)

	body := `{"reconnect":true}`
	req, _ := http.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

	events := parseSSEEvents(t, w.Body.String())
	require.Len(t, events, 2, "expected 2 SSE events from reconnect")
	assert.Equal(t, string(entity.EventMessage), events[0]["type"])
	assert.Equal(t, string(entity.EventDone), events[1]["type"])
}

func TestChat_ReconnectNoActiveAgent(t *testing.T) {
	handler, _, cleanup := setupChatTestHandler(t, &mockAgentEngine{})
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)
	router.POST("/api/sessions/:sessionId/chat", handler.Chat)

	sessionID := createSessionViaHandler(t, router)

	body := `{"reconnect":true}`
	req, _ := http.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestChat_ContentWithActiveAgentConflict(t *testing.T) {
	handler, store, cleanup := setupChatTestHandler(t, &mockAgentEngine{})
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)
	router.POST("/api/sessions/:sessionId/chat", handler.Chat)

	sessionID := createSessionViaHandler(t, router)

	chatCtx := chatcontext.NewChatContext(context.Background(), sessionID, "")
	store.Put(sessionID, chatCtx)

	body := `{"content":"hello"}`
	req, _ := http.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "active agent")
}

func TestChat_ReconnectWithLastEventSeq(t *testing.T) {
	handler, store, cleanup := setupChatTestHandler(t, &mockAgentEngine{})
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)
	router.POST("/api/sessions/:sessionId/chat", handler.Chat)

	sessionID := createSessionViaHandler(t, router)

	chatCtx := chatcontext.NewChatContext(context.Background(), sessionID, "")
	chatCtx.Emit(entity.Event{Type: "first", Data: "data0"})
	chatCtx.Emit(entity.Event{Type: "second", Data: "data1"})
	chatCtx.Emit(entity.Event{Type: "third", Data: "data2"})
	store.Put(sessionID, chatCtx)

	body := `{"reconnect":true,"last_event_seq":1}`
	req, _ := http.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

	events := parseSSEEvents(t, w.Body.String())
	require.Len(t, events, 1, "expected 1 SSE event from last_event_seq=1")
	assert.Equal(t, "third", events[0]["type"])
}

func TestChat_FirstConnectInvalidJSON(t *testing.T) {
	handler, _, cleanup := setupChatTestHandler(t, &mockAgentEngine{})
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions/:sessionId/chat", handler.Chat)

	req, _ := http.NewRequest("POST", "/api/sessions/"+uuid.New().String()+"/chat", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "invalid")
}

func TestChat_FirstConnectEmptyContent(t *testing.T) {
	handler, _, cleanup := setupChatTestHandler(t, &mockAgentEngine{})
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)
	router.POST("/api/sessions/:sessionId/chat", handler.Chat)

	sessionID := createSessionViaHandler(t, router)

	body := `{"content":""}`
	req, _ := http.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "content is required")
}

func TestSessionAgentStore(t *testing.T) {
	store := chat.NewActiveSessions()

	_, ok := store.Get("nonexistent")
	assert.False(t, ok)

	chatCtx := chatcontext.NewChatContext(context.Background(), "sess-1", "agent-1")
	store.Put("sess-1", chatCtx)

	got, ok := store.Get("sess-1")
	assert.True(t, ok)
	assert.Equal(t, "sess-1", got.SessionID())
	assert.Equal(t, "agent-1", got.AgentID())

	store.Remove("sess-1")
	_, ok = store.Get("sess-1")
	assert.False(t, ok)

	store.Put("a", chatcontext.NewChatContext(context.Background(), "a", ""))
	store.Put("b", chatcontext.NewChatContext(context.Background(), "b", ""))
	store.Put("c", chatcontext.NewChatContext(context.Background(), "c", ""))

	got, ok = store.Get("b")
	assert.True(t, ok)
	assert.Equal(t, "b", got.SessionID())

	store.Remove("b")
	_, ok = store.Get("b")
	assert.False(t, ok)

	_, ok = store.Get("a")
	assert.True(t, ok)
	_, ok = store.Get("c")
	assert.True(t, ok)
}

func TestChat_EventsLostOnReconnect(t *testing.T) {
	handler, store, cleanup := setupChatTestHandler(t, &mockAgentEngine{})
	defer cleanup()

	router := gin.New()
	router.POST("/api/sessions", handler.CreateSession)
	router.POST("/api/sessions/:sessionId/chat", handler.Chat)

	sessionID := createSessionViaHandler(t, router)

	chatCtx := chatcontext.NewChatContext(context.Background(), sessionID, "")
	chatCtx.Close()
	store.Put(sessionID, chatCtx)

	body := `{"reconnect":true,"last_event_seq":5}`
	req, _ := http.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

	events := parseSSEEvents(t, w.Body.String())
	require.Len(t, events, 1, "expected 1 SSE event (events_lost)")
	assert.Equal(t, "events_lost", events[0]["type"])
}
