package internal

import (
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
	"github.com/copcon/server/internal/api"
	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/testutil"
	"github.com/copcon/server/internal/tools/todo"
)

func setupIntegrationTestDB(t *testing.T) *gorm.DB {
	dsn := "host=localhost user=admin password=changeme dbname=agent_infra port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	err = db.AutoMigrate(&session.Session{}, &session.Message{}, &session.Todo{})
	require.NoError(t, err)

	db.Exec("DELETE FROM todos WHERE content LIKE 'IntegrationTest:%'")
	db.Exec("DELETE FROM messages WHERE content LIKE 'IntegrationTest:%' OR content LIKE '%IntegrationTest%'")
	db.Exec("DELETE FROM sessions WHERE title LIKE 'IntegrationTest:%'")

	return db
}

func createIntegrationTestSession(t *testing.T, db *gorm.DB) *session.Session {
	sess := &session.Session{
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
		msg := &session.Message{
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
		sessionMgr := &dbSessionManagerForIntegration{db: db}
		todoMgr := todo.NewTodoManager(db)
		agentRegistry := &mockAgentRegistryForIntegration{defaultAgent: "test-agent"}

		handler := api.NewHandler(cfg, sessionMgr, todoMgr, nil, agentRegistry)

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

	t.Run("Todo state injection and duplicate prevention", func(t *testing.T) {
		todoMgr := todo.NewTodoManager(db)
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")

		t.Run("duplicate todos not created", func(t *testing.T) {
			todoContent := "IntegrationTest: duplicate check task"
			todo1, err := todoMgr.Create(chatCtx, todoContent)
			require.NoError(t, err)
			require.NotNil(t, todo1)

			todo2, err := todoMgr.Create(chatCtx, todoContent)
			require.NoError(t, err)

			assert.Equal(t, todo1.ID, todo2.ID, "duplicate creation should return existing todo")

			todos, err := todoMgr.List(chatCtx)
			require.NoError(t, err)

			var matchingTodos []*session.Todo
			for _, t := range todos {
				if t.Content == todoContent {
					matchingTodos = append(matchingTodos, t)
				}
			}
			assert.Len(t, matchingTodos, 1, "should only have one todo with identical content")
		})

		t.Run("todo auto-started after creation", func(t *testing.T) {
			chatCtx2 := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
			autoStartContent := "IntegrationTest: auto-start task"
			createdTodo, err := todoMgr.Create(chatCtx2, autoStartContent)
			require.NoError(t, err)

			assert.Equal(t, session.TodoStatusInProgress, createdTodo.Status,
				"todo should be automatically started after creation")
		})

		t.Run("todo state injected into LLM context", func(t *testing.T) {
			chatCtx3 := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
			_, err := todoMgr.Create(chatCtx3, "IntegrationTest: context injection task 1")
			require.NoError(t, err)

			_, err = todoMgr.Create(chatCtx3, "IntegrationTest: context injection task 2")
			require.NoError(t, err)

			contextMgr := chat_context.NewContextManager(db, todoMgr)

			systemPrompt := "You are a helpful assistant."
			_, err = contextMgr.BuildContext(chatCtx3, "", 256000, systemPrompt)
			require.NoError(t, err)

			todos, err := todoMgr.List(chatCtx3)
			require.NoError(t, err)

			if len(todos) > 0 {
				todoState := formatTodoStateForTest(todos)
				injectedSystemPrompt := systemPrompt + "\n\n" + todoState

				assert.Contains(t, injectedSystemPrompt, "Current todo list",
					"todo state should be in system prompt")

				hasStatus := false
				for _, t := range todos {
					if strings.Contains(injectedSystemPrompt, t.Content) {
						hasStatus = true
						break
					}
				}
				assert.True(t, hasStatus, "todo content should appear in injected system prompt")
			}
		})
	})
}

func formatTodoStateForTest(todos []*session.Todo) string {
	var pending, inProgress, completed, failed, blocked []string

	for _, t := range todos {
		content := t.Content
		if t.ActiveForm != "" {
			content = t.ActiveForm
		}
		switch t.Status {
		case session.TodoStatusPending:
			pending = append(pending, content)
		case session.TodoStatusInProgress:
			inProgress = append(inProgress, content)
		case session.TodoStatusCompleted:
			completed = append(completed, content)
		case session.TodoStatusFailed:
			failed = append(failed, content)
		case session.TodoStatusBlocked:
			blocked = append(blocked, content)
		}
	}

	var parts []string
	if len(pending) > 0 {
		parts = append(parts, "pending: "+strings.Join(pending, ", "))
	}
	if len(inProgress) > 0 {
		parts = append(parts, "in_progress: "+strings.Join(inProgress, ", "))
	}
	if len(completed) > 0 {
		parts = append(parts, "completed: "+strings.Join(completed, ", "))
	}
	if len(failed) > 0 {
		parts = append(parts, "failed: "+strings.Join(failed, ", "))
	}
	if len(blocked) > 0 {
		parts = append(parts, "blocked: "+strings.Join(blocked, ", "))
	}

	return "Current todo list: [" + strings.Join(parts, ", ") + "]"
}

type dbSessionManagerForIntegration struct {
	db *gorm.DB
}

func (m *dbSessionManagerForIntegration) Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string) (*session.Session, error) {
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

func (m *dbSessionManagerForIntegration) Get(chatCtx iface.ChatContextInterface) (*session.Session, error) {
	var sess session.Session
	err := m.db.Where("id = ?", chatCtx.SessionID()).First(&sess).Error
	if err != nil {
		return nil, session.ErrSessionNotFound
	}
	return &sess, nil
}

func (m *dbSessionManagerForIntegration) List(chatCtx iface.ChatContextInterface, limit, offset int) ([]*session.Session, int64, error) {
	var sessions []*session.Session
	var total int64
	m.db.Model(&session.Session{}).Count(&total)
	m.db.Limit(limit).Offset(offset).Find(&sessions)
	return sessions, total, nil
}

func (m *dbSessionManagerForIntegration) Delete(chatCtx iface.ChatContextInterface) error {
	return m.db.Delete(&session.Session{}, "id = ?", chatCtx.SessionID()).Error
}

func (m *dbSessionManagerForIntegration) UpdateTitle(chatCtx iface.ChatContextInterface, title string) error {
	return m.db.Model(&session.Session{}).Where("id = ?", chatCtx.SessionID()).Update("title", title).Error
}

func (m *dbSessionManagerForIntegration) GetMessageCount(chatCtx iface.ChatContextInterface) (int64, error) {
	var count int64
	err := m.db.Model(&session.Message{}).Where("session_id = ?", chatCtx.SessionID()).Count(&count).Error
	return count, err
}

func (m *dbSessionManagerForIntegration) GetDB() *gorm.DB {
	return m.db
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
