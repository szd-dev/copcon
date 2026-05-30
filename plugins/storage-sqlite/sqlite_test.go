package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sqlitesql "github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/copcon/core/storage"
)

// --- Compile-time interface checks ---
// These will fail until Store, SessionStore, MessageStore, TodoStore are implemented.

var _ storage.StoreProvider = (*Store)(nil)
var _ storage.SessionStore = (*SessionStore)(nil)
var _ storage.MessageStore = (*MessageStore)(nil)
var _ storage.TodoStore = (*TodoStore)(nil)

// --- Test helpers ---

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlitesql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	db := setupTestDB(t)
	return NewStore(db)
}

// --- SessionStore tests ---

func TestSessionStore_Create(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Test Session",
		DefaultAgentID: "test-agent",
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, "Test Session", session.Title)
	assert.Equal(t, "test-agent", session.DefaultAgentID)
}

func TestSessionStore_Create_WithMetadata(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Meta Session",
		DefaultAgentID: "agent-1",
		Metadata:       map[string]any{"key": "value", "count": float64(42)},
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, "value", session.Metadata["key"])
	assert.Equal(t, float64(42), session.Metadata["count"])
}

func TestSessionStore_Get(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	created, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Get Test",
		DefaultAgentID: "agent-1",
		Metadata:       map[string]any{"foo": "bar"},
	})
	require.NoError(t, err)

	got, err := s.Sessions().Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "Get Test", got.Title)
	assert.Equal(t, "agent-1", got.DefaultAgentID)
	assert.Equal(t, "bar", got.Metadata["foo"])
}

func TestSessionStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	_, err := s.Sessions().Get(ctx, uuid.New())
	assert.ErrorIs(t, err, errSessionNotFound)
}

func TestSessionStore_List(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	for i := 0; i < 3; i++ {
		_, err := s.Sessions().Create(ctx, &storage.Session{
			Title:          "Session " + string(rune('A'+i)),
			DefaultAgentID: "agent-1",
		})
		require.NoError(t, err)
	}

	sessions, total, err := s.Sessions().List(ctx, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, sessions, 3)

	// List should be ordered by updated_at DESC
	assert.Equal(t, "Session C", sessions[0].Title)
	assert.Equal(t, "Session B", sessions[1].Title)
	assert.Equal(t, "Session A", sessions[2].Title)
}

func TestSessionStore_Delete(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	created, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Delete Me",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	err = s.Sessions().Delete(ctx, created.ID)
	require.NoError(t, err)

	_, err = s.Sessions().Get(ctx, created.ID)
	assert.ErrorIs(t, err, errSessionNotFound)
}

func TestSessionStore_Delete_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	err := s.Sessions().Delete(ctx, uuid.New())
	assert.ErrorIs(t, err, errSessionNotFound)
}

func TestSessionStore_UpdateTitle(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	created, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Old Title",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	err = s.Sessions().UpdateTitle(ctx, created.ID, "New Title")
	require.NoError(t, err)

	got, err := s.Sessions().Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "New Title", got.Title)
}

func TestSessionStore_UpdateTitle_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	err := s.Sessions().UpdateTitle(ctx, uuid.New(), "New Title")
	assert.ErrorIs(t, err, errSessionNotFound)
}

func TestSessionStore_UpdateMetadata(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	created, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Meta Update",
		DefaultAgentID: "agent-1",
		Metadata:       map[string]any{"old": "data"},
	})
	require.NoError(t, err)

	newMeta := map[string]any{"new": "value", "count": float64(99)}
	err = s.Sessions().UpdateMetadata(ctx, created.ID, newMeta)
	require.NoError(t, err)

	got, err := s.Sessions().Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "value", got.Metadata["new"])
	assert.Equal(t, float64(99), got.Metadata["count"])
}

func TestSessionStore_GetMessageCount(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Msg Count",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err := s.Messages().Add(ctx, &storage.Message{
			SessionID: session.ID,
			Role:      "user",
			Content:   "msg " + string(rune('0'+i)),
		})
		require.NoError(t, err)
	}

	count, err := s.Sessions().GetMessageCount(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestSessionStore_AppendMetadata(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Append Meta",
		DefaultAgentID: "agent-1",
		Metadata:       map[string]any{},
	})
	require.NoError(t, err)

	err = s.Sessions().AppendMetadata(ctx, session.ID, "tags", "go")
	require.NoError(t, err)

	got, err := s.Sessions().Get(ctx, session.ID)
	require.NoError(t, err)
	tags, ok := got.Metadata["tags"].([]any)
	require.True(t, ok, "metadata key 'tags' should be a []any")
	assert.Equal(t, []any{"go"}, tags)
}

func TestSessionStore_AppendMetadata_ExistingKey(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Append Existing",
		DefaultAgentID: "agent-1",
		Metadata:       map[string]any{},
	})
	require.NoError(t, err)

	err = s.Sessions().AppendMetadata(ctx, session.ID, "tags", "go")
	require.NoError(t, err)

	err = s.Sessions().AppendMetadata(ctx, session.ID, "tags", "sqlite")
	require.NoError(t, err)

	got, err := s.Sessions().Get(ctx, session.ID)
	require.NoError(t, err)
	tags, ok := got.Metadata["tags"].([]any)
	require.True(t, ok, "metadata key 'tags' should be a []any")
	assert.Equal(t, []any{"go", "sqlite"}, tags)
}

// --- MessageStore tests ---

func TestMessageStore_Add(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Msg Add",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	err = s.Messages().Add(ctx, &storage.Message{
		SessionID: session.ID,
		Role:      "user",
		Content:   "Hello, world!",
	})
	require.NoError(t, err)

	messages, err := s.Messages().List(ctx, session.ID, 0)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "Hello, world!", messages[0].Content)
}

func TestMessageStore_List(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Msg List",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err := s.Messages().Add(ctx, &storage.Message{
			SessionID: session.ID,
			Role:      "user",
			Content:   "msg " + string(rune('0'+i)),
		})
		require.NoError(t, err)
	}

	messages, err := s.Messages().List(ctx, session.ID, 0)
	require.NoError(t, err)
	assert.Len(t, messages, 3)

	// Messages should be ordered by created_at ASC
	assert.Equal(t, "msg 0", messages[0].Content)
	assert.Equal(t, "msg 1", messages[1].Content)
	assert.Equal(t, "msg 2", messages[2].Content)
}

func TestMessageStore_List_WithLimit(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Msg Limit",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		err := s.Messages().Add(ctx, &storage.Message{
			SessionID: session.ID,
			Role:      "user",
			Content:   "msg " + string(rune('0'+i)),
		})
		require.NoError(t, err)
	}

	messages, err := s.Messages().List(ctx, session.ID, 3)
	require.NoError(t, err)
	assert.Len(t, messages, 3)
}

func TestMessageStore_Update(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Msg Update",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	msg := &storage.Message{
		SessionID: session.ID,
		Role:      "assistant",
		Content:   "original",
		Parts: []storage.Part{
			{Type: "text", Text: "original"},
		},
	}
	err = s.Messages().Add(ctx, msg)
	require.NoError(t, err)

	messages, err := s.Messages().List(ctx, session.ID, 0)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	messages[0].Content = "updated"
	messages[0].Parts = []storage.Part{
		{Type: "text", Text: "updated"},
	}
	err = s.Messages().Update(ctx, messages[0])
	require.NoError(t, err)

	updated, err := s.Messages().List(ctx, session.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, "updated", updated[0].Content)
	require.Len(t, updated[0].Parts, 1)
	assert.Equal(t, "updated", updated[0].Parts[0].Text)
}

func TestMessageStore_Update_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	msg := &storage.Message{
		ID:        uuid.New(),
		SessionID: uuid.New(),
		Role:      "user",
		Content:   "ghost",
	}
	err := s.Messages().Update(ctx, msg)
	assert.ErrorIs(t, err, errMessageNotFound)
}

func TestMessageStore_Upsert_Create(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Msg Upsert Create",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	msg := &storage.Message{
		SessionID: session.ID,
		Role:      "user",
		Content:   "upserted new",
	}
	err = s.Messages().Upsert(ctx, msg)
	require.NoError(t, err)

	messages, err := s.Messages().List(ctx, session.ID, 0)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, "upserted new", messages[0].Content)
}

func TestMessageStore_Upsert_Update(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Msg Upsert Update",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	err = s.Messages().Add(ctx, &storage.Message{
		SessionID: session.ID,
		Role:      "user",
		Content:   "original",
	})
	require.NoError(t, err)

	messages, err := s.Messages().List(ctx, session.ID, 0)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	messages[0].Content = "upserted updated"
	err = s.Messages().Upsert(ctx, messages[0])
	require.NoError(t, err)

	updated, err := s.Messages().List(ctx, session.ID, 0)
	require.NoError(t, err)
	assert.Len(t, updated, 1)
	assert.Equal(t, "upserted updated", updated[0].Content)
}

func TestMessageStore_DeleteBySession(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session1, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Session 1",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	session2, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Session 2",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		err := s.Messages().Add(ctx, &storage.Message{
			SessionID: session1.ID,
			Role:      "user",
			Content:   "s1 msg",
		})
		require.NoError(t, err)

		err = s.Messages().Add(ctx, &storage.Message{
			SessionID: session2.ID,
			Role:      "user",
			Content:   "s2 msg",
		})
		require.NoError(t, err)
	}

	err = s.Messages().DeleteBySession(ctx, session1.ID)
	require.NoError(t, err)

	s1Msgs, err := s.Messages().List(ctx, session1.ID, 0)
	require.NoError(t, err)
	assert.Len(t, s1Msgs, 0)

	s2Msgs, err := s.Messages().List(ctx, session2.ID, 0)
	require.NoError(t, err)
	assert.Len(t, s2Msgs, 2)
}

// --- TodoStore tests ---

func TestTodoStore_Create(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Todo Create",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	todo, err := s.Todos().Create(ctx, &storage.Todo{
		SessionID: session.ID,
		Content:   "Write tests",
		Status:    storage.TodoStatusPending,
		Priority:  "high",
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, todo.ID)
	assert.Equal(t, "Write tests", todo.Content)
	assert.Equal(t, storage.TodoStatusPending, todo.Status)
	assert.Equal(t, session.ID, todo.SessionID)
}

func TestTodoStore_Create_DefaultStatus(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Todo Default",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	todo, err := s.Todos().Create(ctx, &storage.Todo{
		SessionID: session.ID,
		Content:   "Default status todo",
	})
	require.NoError(t, err)
	assert.Equal(t, storage.TodoStatusPending, todo.Status)
}

func TestTodoStore_Get(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Todo Get",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	created, err := s.Todos().Create(ctx, &storage.Todo{
		SessionID: session.ID,
		Content:   "Fetch me",
		Status:    storage.TodoStatusInProgress,
	})
	require.NoError(t, err)

	got, err := s.Todos().Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "Fetch me", got.Content)
	assert.Equal(t, storage.TodoStatusInProgress, got.Status)
}

func TestTodoStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	_, err := s.Todos().Get(ctx, uuid.New())
	assert.ErrorIs(t, err, errTodoNotFound)
}

func TestTodoStore_List(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Todo List",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err := s.Todos().Create(ctx, &storage.Todo{
			SessionID: session.ID,
			Content:   "Todo " + string(rune('A'+i)),
			Status:    storage.TodoStatusPending,
		})
		require.NoError(t, err)
	}

	todos, err := s.Todos().List(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, todos, 3)

	// Todos should be ordered by created_at DESC
	assert.Equal(t, "Todo C", todos[0].Content)
	assert.Equal(t, "Todo B", todos[1].Content)
	assert.Equal(t, "Todo A", todos[2].Content)
}

func TestTodoStore_UpdateStatus(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Todo Status",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	created, err := s.Todos().Create(ctx, &storage.Todo{
		SessionID: session.ID,
		Content:   "Update my status",
		Status:    storage.TodoStatusPending,
	})
	require.NoError(t, err)

	updated, err := s.Todos().UpdateStatus(ctx, created.ID, storage.TodoStatusInProgress)
	require.NoError(t, err)
	assert.Equal(t, storage.TodoStatusInProgress, updated.Status)
}

func TestTodoStore_UpdateStatus_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	_, err := s.Todos().UpdateStatus(ctx, uuid.New(), storage.TodoStatusInProgress)
	assert.ErrorIs(t, err, errTodoNotFound)
}

func TestTodoStore_DeleteBySession(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t)

	session1, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Todo S1",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	session2, err := s.Sessions().Create(ctx, &storage.Session{
		Title:          "Todo S2",
		DefaultAgentID: "agent-1",
	})
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		_, err := s.Todos().Create(ctx, &storage.Todo{
			SessionID: session1.ID,
			Content:   "S1 todo",
			Status:    storage.TodoStatusPending,
		})
		require.NoError(t, err)

		_, err = s.Todos().Create(ctx, &storage.Todo{
			SessionID: session2.ID,
			Content:   "S2 todo",
			Status:    storage.TodoStatusPending,
		})
		require.NoError(t, err)
	}

	err = s.Todos().DeleteBySession(ctx, session1.ID)
	require.NoError(t, err)

	s1Todos, err := s.Todos().List(ctx, session1.ID)
	require.NoError(t, err)
	assert.Len(t, s1Todos, 0)

	s2Todos, err := s.Todos().List(ctx, session2.ID)
	require.NoError(t, err)
	assert.Len(t, s2Todos, 2)
}
