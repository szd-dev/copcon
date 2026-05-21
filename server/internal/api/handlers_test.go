package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/agent"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tools/todo"
)

type mockSessionManager struct {
	sessions map[string]*session.Session
	db       *gorm.DB
}

func newMockSessionManager() *mockSessionManager {
	return &mockSessionManager{
		sessions: make(map[string]*session.Session),
	}
}

func (m *mockSessionManager) Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string, opts ...session.CreateOption) (*session.Session, error) {
	sess := &session.Session{
		ID:             uuid.New(),
		Title:          title,
		DefaultAgentID: defaultAgentID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       make(map[string]any),
	}
	m.sessions[sess.ID.String()] = sess
	return sess, nil
}

func (m *mockSessionManager) Get(chatCtx iface.ChatContextInterface) (*session.Session, error) {
	sess, ok := m.sessions[chatCtx.SessionID()]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	return sess, nil
}

func (m *mockSessionManager) List(chatCtx iface.ChatContextInterface, limit, offset int) ([]*session.Session, int64, error) {
	var list []*session.Session
	for _, s := range m.sessions {
		list = append(list, s)
	}
	return list, int64(len(list)), nil
}

func (m *mockSessionManager) Delete(chatCtx iface.ChatContextInterface) error {
	if _, ok := m.sessions[chatCtx.SessionID()]; !ok {
		return session.ErrSessionNotFound
	}
	delete(m.sessions, chatCtx.SessionID())
	return nil
}

func (m *mockSessionManager) UpdateTitle(chatCtx iface.ChatContextInterface, title string) error {
	sess, ok := m.sessions[chatCtx.SessionID()]
	if !ok {
		return session.ErrSessionNotFound
	}
	sess.Title = title
	return nil
}

func (m *mockSessionManager) GetMessageCount(chatCtx iface.ChatContextInterface) (int64, error) {
	return 0, nil
}

func (m *mockSessionManager) GetDB() *gorm.DB {
	return m.db
}

func (m *mockSessionManager) UpdateMetadata(chatCtx iface.ChatContextInterface, metadata map[string]any) error {
	return nil
}

func (m *mockSessionManager) AddAsyncCompletionPending(chatCtx iface.ChatContextInterface, event map[string]any) error {
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

	sessionMgr := newMockSessionManager()
	todoMgr := &mockTodoManager{}
	agentRegistry := newMockAgentRegistry("default-agent")

	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}
	agentRegistry.agents["code-assistant"] = agent.AgentDefinition{ID: "code-assistant", Name: "Code Assistant", Model: "gpt-4o"}

	handler := NewHandler(cfg, sessionMgr, todoMgr, nil, agentRegistry)

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

func setupTestDB(t *testing.T) *gorm.DB {
	dsn := "host=localhost user=admin password=changeme dbname=agent_infra port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	err = db.AutoMigrate(&session.Session{}, &session.Message{})
	require.NoError(t, err)

	db.Exec("DELETE FROM messages WHERE content LIKE 'Test:%'")
	db.Exec("DELETE FROM sessions WHERE title LIKE 'Test:%'")

	return db
}

func createTestSessionForMessages(t *testing.T, db *gorm.DB) *session.Session {
	sess := &session.Session{
		ID:             uuid.New(),
		Title:          "Test: " + uuid.New().String(),
		DefaultAgentID: "default-agent",
		Metadata:       make(map[string]any),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := db.Create(sess).Error
	require.NoError(t, err)
	return sess
}

func TestGetMessagesReasoning(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)

	sess := createTestSessionForMessages(t, db)

	reasoningContent := "Let me think about this step by step..."
	msg := &session.Message{
		ID:        uuid.New(),
		SessionID: sess.ID,
		Role:      "assistant",
		Content:   "Test: message with reasoning",
		Reasoning: reasoningContent,
		CreatedAt: time.Now(),
	}
	err := db.Create(msg).Error
	require.NoError(t, err)

	cfg := &config.Config{DefaultAgentID: "default-agent"}
	sessionMgr := &dbSessionManager{db: db}
	todoMgr := &mockTodoManager{}
	agentRegistry := newMockAgentRegistry("default-agent")
	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}

	handler := NewHandler(cfg, sessionMgr, todoMgr, nil, agentRegistry)

	router := gin.New()
	router.GET("/api/sessions/:sessionId/messages", handler.GetMessages)

	req, _ := http.NewRequest("GET", "/api/sessions/"+sess.ID.String()+"/messages", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	messages, ok := response["messages"].([]interface{})
	require.True(t, ok, "response should contain messages array")
	require.Len(t, messages, 1, "should have exactly one message")

	message := messages[0].(map[string]interface{})

	assert.Equal(t, msg.ID.String(), message["id"])
	assert.Equal(t, sess.ID.String(), message["session_id"])
	assert.Equal(t, "assistant", message["role"])
	assert.Equal(t, "Test: message with reasoning", message["content"])

	reasoning, hasReasoning := message["reasoning"]
	require.True(t, hasReasoning, "response MUST include 'reasoning' field")
	assert.Equal(t, reasoningContent, reasoning, "reasoning field should match stored value")
}

type dbSessionManager struct {
	db *gorm.DB
}

func (m *dbSessionManager) Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string, opts ...session.CreateOption) (*session.Session, error) {
	sess := &session.Session{
		ID:             uuid.New(),
		Title:          title,
		DefaultAgentID: defaultAgentID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       make(map[string]any),
	}
	err := m.db.Create(sess).Error
	return sess, err
}

func (m *dbSessionManager) Get(chatCtx iface.ChatContextInterface) (*session.Session, error) {
	var sess session.Session
	err := m.db.Where("id = ?", chatCtx.SessionID()).First(&sess).Error
	if err != nil {
		return nil, session.ErrSessionNotFound
	}
	return &sess, nil
}

func (m *dbSessionManager) List(chatCtx iface.ChatContextInterface, limit, offset int) ([]*session.Session, int64, error) {
	var sessions []*session.Session
	var total int64
	m.db.Model(&session.Session{}).Count(&total)
	m.db.Limit(limit).Offset(offset).Find(&sessions)
	return sessions, total, nil
}

func (m *dbSessionManager) Delete(chatCtx iface.ChatContextInterface) error {
	return m.db.Delete(&session.Session{}, "id = ?", chatCtx.SessionID()).Error
}

func (m *dbSessionManager) UpdateTitle(chatCtx iface.ChatContextInterface, title string) error {
	return m.db.Model(&session.Session{}).Where("id = ?", chatCtx.SessionID()).Update("title", title).Error
}

func (m *dbSessionManager) GetMessageCount(chatCtx iface.ChatContextInterface) (int64, error) {
	var count int64
	err := m.db.Model(&session.Message{}).Where("session_id = ?", chatCtx.SessionID()).Count(&count).Error
	return count, err
}

func (m *dbSessionManager) GetDB() *gorm.DB {
	return m.db
}

func (m *dbSessionManager) UpdateMetadata(chatCtx iface.ChatContextInterface, metadata map[string]any) error {
	return m.db.Model(&session.Session{}).Where("id = ?", chatCtx.SessionID()).Update("metadata", metadata).Error
}

func (m *dbSessionManager) AddAsyncCompletionPending(chatCtx iface.ChatContextInterface, event map[string]any) error {
	sess, err := m.Get(chatCtx)
	if err != nil {
		return err
	}

	if sess.Metadata == nil {
		sess.Metadata = make(map[string]any)
	}

	var pending []map[string]any
	if val, ok := sess.Metadata["async_completion_pending"].([]map[string]any); ok {
		pending = val
	} else {
		pending = []map[string]any{}
	}

	pending = append(pending, event)
	sess.Metadata["async_completion_pending"] = pending

	return m.UpdateMetadata(chatCtx, sess.Metadata)
}

func TestBackfillParts_UserMessage(t *testing.T) {
	msg := session.Message{
		ID:      uuid.New(),
		Role:    "user",
		Content: "Hello",
	}
	parts := backfillParts(msg, nil)
	require.Len(t, parts, 1)
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "Hello", parts[0].Text)
	assert.Equal(t, "done", parts[0].State)
	assert.Equal(t, 0, parts[0].StepIndex)
}

func TestBackfillParts_AssistantWithToolCalls(t *testing.T) {
	msg := session.Message{
		ID:        uuid.New(),
		Role:      "assistant",
		Reasoning: "Thinking...",
		Content:   "Let me check.",
		ToolCalls: session.ToolCalls{
			{ID: "call_1", Type: "function", Function: session.FunctionCall{Name: "bash", Arguments: `{"cmd":"ls"}`}},
		},
	}
	toolResults := map[string]string{"call_1": "file.txt"}
	parts := backfillParts(msg, toolResults)
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
	msg := session.Message{
		ID:   uuid.New(),
		Role: "assistant",
		ToolCalls: session.ToolCalls{
			{ID: "call_2", Type: "function", Function: session.FunctionCall{Name: "python", Arguments: `{"code":"1+1"}`}},
		},
	}
	parts := backfillParts(msg, nil)
	require.Len(t, parts, 1)
	assert.Equal(t, "tool-call", parts[0].Type)
	assert.Equal(t, "call_2", parts[0].ToolCallID)
	assert.Equal(t, 0, parts[0].StepIndex)
}

func TestGroupPartsByStep_SingleStep(t *testing.T) {
	parts := session.PersistedParts{
		{Type: "text", Text: "Hello", StepIndex: 0},
		{Type: "tool-call", ToolCallID: "c1", StepIndex: 0},
	}
	steps := groupPartsByStep(parts)
	require.Len(t, steps, 1)
	assert.Equal(t, entity.UIPartStateDone, steps[0].State)
	require.Len(t, steps[0].Parts, 2)
	assert.Equal(t, entity.UIPartText, steps[0].Parts[0].Type)
	assert.Equal(t, entity.UIPartToolCall, steps[0].Parts[1].Type)
}

func TestGroupPartsByStep_MultipleSteps(t *testing.T) {
	parts := session.PersistedParts{
		{Type: "text", Text: "Step 0", StepIndex: 0},
		{Type: "text", Text: "Step 1", StepIndex: 1},
		{Type: "tool-call", ToolCallID: "c1", StepIndex: 1},
	}
	steps := groupPartsByStep(parts)
	require.Len(t, steps, 2)
	require.Len(t, steps[0].Parts, 1)
	assert.Equal(t, "Step 0", steps[0].Parts[0].Text)
	require.Len(t, steps[1].Parts, 2)
}

func TestGroupPartsByStep_Empty(t *testing.T) {
	steps := groupPartsByStep(nil)
	assert.Len(t, steps, 0)
}

// mockAgentEngine implements agent.AgentEngine for testing.
type mockAgentEngine struct {
	chatFn func(chatCtx iface.ChatContextInterface, userInput string) error
}

func (m *mockAgentEngine) Chat(chatCtx iface.ChatContextInterface, userInput string) error {
	if m.chatFn != nil {
		return m.chatFn(chatCtx, userInput)
	}
	return nil
}

func setupChatTestHandler(t *testing.T, mockAgent *mockAgentEngine) (*Handler, *SessionAgentStore, func()) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		DefaultAgentID: "default-agent",
		Agents: []config.AgentConfig{
			{ID: "default-agent", Name: "Default"},
			{ID: "code-assistant", Name: "Code Assistant"},
		},
	}

	sessionMgr := newMockSessionManager()
	todoMgr := &mockTodoManager{}
	agentRegistry := newMockAgentRegistry("default-agent")

	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}
	agentRegistry.agents["code-assistant"] = agent.AgentDefinition{ID: "code-assistant", Name: "Code Assistant", Model: "gpt-4o"}

	handler := NewHandler(cfg, sessionMgr, todoMgr, mockAgent, agentRegistry)

	cleanup := func() {}

	return handler, handler.sessionAgentStore, cleanup
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

	chatCtx := iface.NewChatContext(context.Background(), sessionID, "")
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

	chatCtx := iface.NewChatContext(context.Background(), sessionID, "")
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

	chatCtx := iface.NewChatContext(context.Background(), sessionID, "")
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
	store := NewSessionAgentStore()

	_, ok := store.Get("nonexistent")
	assert.False(t, ok)

	chatCtx := iface.NewChatContext(context.Background(), "sess-1", "agent-1")
	store.Put("sess-1", chatCtx)

	got, ok := store.Get("sess-1")
	assert.True(t, ok)
	assert.Equal(t, "sess-1", got.SessionID())
	assert.Equal(t, "agent-1", got.AgentID())

	store.Remove("sess-1")
	_, ok = store.Get("sess-1")
	assert.False(t, ok)

	store.Put("a", iface.NewChatContext(context.Background(), "a", ""))
	store.Put("b", iface.NewChatContext(context.Background(), "b", ""))
	store.Put("c", iface.NewChatContext(context.Background(), "c", ""))

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

	// Put a closed ChatContext (no events, already closed)
	chatCtx := iface.NewChatContext(context.Background(), sessionID, "")
	chatCtx.Close()
	store.Put(sessionID, chatCtx)

	// Subscribe from seq that's beyond what's available (seq=0 but currentSeq=0, fromSeq=5)
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
