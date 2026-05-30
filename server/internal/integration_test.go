package internal

import (
	"bytes"
	"context"
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

	"github.com/copcon/core/agent"
	"github.com/copcon/core/chat"
	"github.com/copcon/core/entity"
	pgstore "github.com/copcon/plugins/storage-postgres"
	"github.com/copcon/core/storage"
	"github.com/copcon/server/internal/api"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/testutil"
)

type integrationHarness struct {
	store         storage.StoreProvider
	agentRegistry agent.AgentRegistry
}

func (h *integrationHarness) Store() storage.StoreProvider        { return h.store }
func (h *integrationHarness) Engine() agent.AgentEngine           { return nil }
func (h *integrationHarness) Registry() agent.AgentRegistry       { return h.agentRegistry }
func (h *integrationHarness) ActiveSessions() chat.ActiveSessions { return chat.NewActiveSessions() }

func setupIntegrationTestDB(t *testing.T) *gorm.DB {
	dsn := "host=localhost user=admin password=changeme dbname=agent_infra port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	err = db.AutoMigrate(&pgstore.Session{}, &pgstore.Message{}, &pgstore.Todo{})
	require.NoError(t, err)

	db.Exec("DELETE FROM todos WHERE content LIKE 'IntegrationTest:%'")
	db.Exec("DELETE FROM messages WHERE content LIKE 'IntegrationTest:%' OR content LIKE '%IntegrationTest%'")
	db.Exec("DELETE FROM sessions WHERE title LIKE 'IntegrationTest:%'")

	return db
}

func createIntegrationTestSession(t *testing.T, db *gorm.DB) *pgstore.Session {
	sess := &pgstore.Session{
		ID:             uuid.New(),
		Title:          "IntegrationTest: " + uuid.New().String(),
		DefaultAgentID: "test-agent",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       make(map[string]any),
	}
	err := db.Create(sess).Error
	require.NoError(t, err)
	return sess
}

func TestIntegrationAllIssues(t *testing.T) {
	db := setupIntegrationTestDB(t)
	gin.SetMode(gin.TestMode)
	ctx := context.Background()

	sess := createIntegrationTestSession(t, db)

	t.Run("GetMessages returns reasoning field", func(t *testing.T) {
		reasoningContent := "IntegrationTest: Let me think step by step..."
		msg := &pgstore.Message{
			ID:        uuid.New(),
			SessionID: sess.ID,
			Role:      "assistant",
			Content:   "IntegrationTest: message content",
			Reasoning: reasoningContent,
			CreatedAt: time.Now(),
		}
		err := db.Create(msg).Error
		require.NoError(t, err)

		cfg := &config.Config{DefaultAgentID: "test-agent"}
		pg := pgstore.NewStore(db)
		agentRegistry := &mockAgentRegistryForIntegration{defaultAgent: "test-agent"}

		handler := api.NewHandler(cfg, &integrationHarness{store: pg, agentRegistry: agentRegistry})

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

		var foundTestMsg bool
		for _, m := range messages {
			msgMap := m.(map[string]interface{})
			if msgMap["id"] == msg.ID.String() {
				foundTestMsg = true

				reasoning, hasReasoning := msgMap["reasoning"]
				require.True(t, hasReasoning, "message must include reasoning field")
				assert.Equal(t, reasoningContent, reasoning, "reasoning field should match stored value")
				break
			}
		}
		assert.True(t, foundTestMsg, "should find the test message")
	})

	t.Run("MessageID present in MessageData events", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")

		testMessageID := uuid.New().String()
		testContent := "IntegrationTest: event content"

		chatCtx.Emit(entity.Event{
			Type: entity.EventMessage,
			Data: entity.MessageData{
				MessageID: testMessageID,
				Content:   testContent,
			},
		})

		event := <-chatCtx.Events()

		assert.Equal(t, entity.EventMessage, event.Type)

		msgData, ok := event.Data.(entity.MessageData)
		require.True(t, ok, "event data should be MessageData type")

		assert.Equal(t, testMessageID, msgData.MessageID, "MessageData must include message_id field")
		assert.Equal(t, testContent, msgData.Content)
	})
}

type mockAgentRegistryForIntegration struct {
	defaultAgent string
}

func (r *mockAgentRegistryForIntegration) Get(id string) (agent.AgentDefinition, error) {
	return agent.AgentDefinition{ID: id, Name: "Test Agent", Model: "gpt-4o"}, nil
}

func (r *mockAgentRegistryForIntegration) List() []agent.AgentInfo {
	return []agent.AgentInfo{{ID: "test-agent", Name: "Test Agent", Model: "gpt-4o"}}
}

func (r *mockAgentRegistryForIntegration) Default() (agent.AgentDefinition, error) {
	return r.Get(r.defaultAgent)
}

func (r *mockAgentRegistryForIntegration) RegisterFactory(id, name, model string, allowDelegate bool, factory agent.AgentFactory) {
}

func (r *mockAgentRegistryForIntegration) GetFactory(id string) (agent.AgentFactory, error) {
	return nil, agent.ErrAgentNotFound
}

func (r *mockAgentRegistryForIntegration) ListDelegatable() []agent.AgentInfo {
	return nil
}

func setupIntegrationRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()

	db := setupIntegrationTestDB(t)
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{DefaultAgentID: "test-agent"}
	pg := pgstore.NewStore(db)
	agentRegistry := &mockAgentRegistryForIntegration{defaultAgent: "test-agent"}

	handler := api.NewHandler(cfg, &integrationHarness{store: pg, agentRegistry: agentRegistry})

	router := gin.New()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	apiGroup := router.Group("/api")
	{
		sessions := apiGroup.Group("/sessions")
		{
			sessions.POST("", handler.CreateSession)
			sessions.GET("", handler.ListSessions)
			sessions.GET("/:sessionId", handler.GetSession)
			sessions.DELETE("/:sessionId", handler.DeleteSession)
			sessions.GET("/:sessionId/messages", handler.GetMessages)
			sessions.POST("/:sessionId/chat", handler.Chat)
		}
	}

	return router, db
}

func TestIntegrationHealth(t *testing.T) {
	router, _ := setupIntegrationRouter(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response["status"])
}

func TestIntegrationCreateSession(t *testing.T) {
	router, _ := setupIntegrationRouter(t)

	reqBody := map[string]string{
		"title":            "IntegrationTest: Create Session",
		"default_agent_id": "test-agent",
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

	assert.Equal(t, "IntegrationTest: Create Session", response["title"])
	assert.Equal(t, "test-agent", response["default_agent_id"])
	assert.NotEmpty(t, response["id"])
	assert.NotNil(t, response["created_at"])
	assert.NotNil(t, response["updated_at"])
}

func TestIntegrationListSessions(t *testing.T) {
	router, _ := setupIntegrationRouter(t)

	createBody := map[string]string{
		"title":            "IntegrationTest: List Session",
		"default_agent_id": "test-agent",
	}
	jsonBody, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBuffer(jsonBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	require.Equal(t, http.StatusCreated, createW.Code)

	req, _ := http.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	sessions, ok := response["sessions"].([]interface{})
	require.True(t, ok, "response should contain sessions array")
	assert.NotEmpty(t, sessions, "sessions list should not be empty")

	total, ok := response["total"].(float64)
	require.True(t, ok, "response should contain total count")
	assert.GreaterOrEqual(t, total, float64(1))
}

func TestIntegrationGetMessages(t *testing.T) {
	router, db := setupIntegrationRouter(t)

	sess := createIntegrationTestSession(t, db)
	msg := &pgstore.Message{
		ID:        uuid.New(),
		SessionID: sess.ID,
		Role:      "user",
		Content:   "IntegrationTest: hello from integration test",
		CreatedAt: time.Now(),
	}
	err := db.Create(msg).Error
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/api/sessions/"+sess.ID.String()+"/messages", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	messages, ok := response["messages"].([]interface{})
	require.True(t, ok, "response should contain messages array")
	assert.NotEmpty(t, messages, "messages list should not be empty")
}

func TestIntegrationDeleteSession(t *testing.T) {
	router, _ := setupIntegrationRouter(t)

	createBody := map[string]string{
		"title":            "IntegrationTest: Delete Session",
		"default_agent_id": "test-agent",
	}
	jsonBody, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBuffer(jsonBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	require.Equal(t, http.StatusCreated, createW.Code)

	var createResp map[string]interface{}
	err := json.Unmarshal(createW.Body.Bytes(), &createResp)
	require.NoError(t, err)
	sessionID := createResp["id"].(string)

	deleteReq, _ := http.NewRequest("DELETE", "/api/sessions/"+sessionID, nil)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)

	assert.Equal(t, http.StatusNoContent, deleteW.Code)

	deleteReq2, _ := http.NewRequest("DELETE", "/api/sessions/"+sessionID, nil)
	deleteW2 := httptest.NewRecorder()
	router.ServeHTTP(deleteW2, deleteReq2)
	assert.Equal(t, http.StatusNotFound, deleteW2.Code)
}

func TestIntegrationChatHeaders(t *testing.T) {
	router, _ := setupIntegrationRouter(t)

	createBody := map[string]string{
		"title":            "IntegrationTest: Chat Headers",
		"default_agent_id": "test-agent",
	}
	jsonBody, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBuffer(jsonBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	require.Equal(t, http.StatusCreated, createW.Code)

	var createResp map[string]interface{}
	err := json.Unmarshal(createW.Body.Bytes(), &createResp)
	require.NoError(t, err)
	sessionID := createResp["id"].(string)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	chatBody := `{"content":"hello","agent_id":"test-agent"}`
	chatReq, _ := http.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewBufferString(chatBody))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq = chatReq.WithContext(ctx)
	chatW := httptest.NewRecorder()
	router.ServeHTTP(chatW, chatReq)

	assert.Equal(t, "text/event-stream", chatW.Header().Get("Content-Type"),
		"chat endpoint should return text/event-stream content type")
	assert.Equal(t, "no-cache", chatW.Header().Get("Cache-Control"),
		"chat endpoint should set Cache-Control: no-cache")
}
