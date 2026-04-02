package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/todo"
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

func (m *mockSessionManager) Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string) (*session.Session, error) {
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

func (m *dbSessionManager) Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string) (*session.Session, error) {
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
