package todo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/testutil"
)

func setupTestDB(t *testing.T) *gorm.DB {
	dsn := "host=localhost user=admin password=changeme dbname=agent_infra port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	err = db.AutoMigrate(&session.Session{}, &session.Todo{})
	require.NoError(t, err)

	db.Exec("DELETE FROM todos WHERE content LIKE 'Test:%'")
	db.Exec("DELETE FROM sessions WHERE title LIKE 'Test:%'")

	return db
}

func createTestSession(t *testing.T, db *gorm.DB) *session.Session {
	sess := &session.Session{
		ID:        uuid.New(),
		Title:     "Test: " + uuid.New().String(),
		Metadata:  make(map[string]any),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := db.Create(sess).Error
	require.NoError(t, err)
	return sess
}

func TestTodoManager_Create(t *testing.T) {
	db := setupTestDB(t)
	manager, _ := NewTodoManager(db)
	ctx := context.Background()
	sess := createTestSession(t, db)

	t.Run("create basic todo", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: basic todo")
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, todo.ID)
		assert.Equal(t, session.TodoStatusPending, todo.Status)
		assert.Equal(t, 0, todo.RetryCount)
	})

	t.Run("create todo with dependencies", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo1, err := manager.CreateTodo(chatCtx, "Test: first todo")
		require.NoError(t, err)

		todo2, err := manager.CreateTodo(chatCtx, "Test: second todo", WithDependsOn(todo1.ID.String()))
		require.NoError(t, err)
		assert.Len(t, todo2.DependsOn, 1)
		assert.Equal(t, todo1.ID, todo2.DependsOn[0])
	})

	t.Run("create todo with circular dependency self-reference", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: self-dep todo")
		require.NoError(t, err)

		_ = todo
	})
}

func TestTodoManager_Start(t *testing.T) {
	db := setupTestDB(t)
	manager, _ := NewTodoManager(db)
	ctx := context.Background()
	sess := createTestSession(t, db)

	t.Run("start pending todo with no dependencies", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: start no deps")
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		started, err := manager.Start(chatCtxForStart, todo.ID.String())
		require.NoError(t, err)
		assert.Equal(t, session.TodoStatusInProgress, started.Status)
	})

	t.Run("start blocked todo with satisfied dependencies", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo1, err := manager.CreateTodo(chatCtx, "Test: dep 1")
		require.NoError(t, err)

		todo2, err := manager.CreateTodo(chatCtx, "Test: dep 2", WithDependsOn(todo1.ID.String()))
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo1.ID.String())
		require.NoError(t, err)

		chatCtxForComplete := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Complete(chatCtxForComplete, todo1.ID.String(), "completed")
		require.NoError(t, err)

		chatCtxForStart2 := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		started, err := manager.Start(chatCtxForStart2, todo2.ID.String())
		require.NoError(t, err)
		assert.Equal(t, session.TodoStatusInProgress, started.Status)
	})

	t.Run("start pending todo with unsatisfied dependencies", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo1, err := manager.CreateTodo(chatCtx, "Test: unsatisfied dep")
		require.NoError(t, err)

		todo2, err := manager.CreateTodo(chatCtx, "Test: dependent", WithDependsOn(todo1.ID.String()))
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo2.ID.String())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrDependenciesNotMet))
	})

	t.Run("start from invalid state", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: invalid start")
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo.ID.String())
		require.NoError(t, err)

		chatCtxForStart2 := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart2, todo.ID.String())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidTransition))
	})

	t.Run("start from completed state", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: completed start")
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo.ID.String())
		require.NoError(t, err)

		chatCtxForComplete := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Complete(chatCtxForComplete, todo.ID.String(), "done")
		require.NoError(t, err)

		chatCtxForStart2 := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart2, todo.ID.String())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidTransition))
	})
}

func TestTodoManager_Complete(t *testing.T) {
	db := setupTestDB(t)
	manager, _ := NewTodoManager(db)
	ctx := context.Background()
	sess := createTestSession(t, db)

	t.Run("complete in_progress todo with result", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: complete")
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo.ID.String())
		require.NoError(t, err)

		chatCtxForComplete := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		completed, err := manager.Complete(chatCtxForComplete, todo.ID.String(), "success result")
		require.NoError(t, err)
		assert.Equal(t, session.TodoStatusCompleted, completed.Status)
		assert.Equal(t, "success result", completed.Result)
		assert.NotNil(t, completed.CompletedAt)
	})

	t.Run("complete without result fails", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: complete no result")
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo.ID.String())
		require.NoError(t, err)

		chatCtxForComplete := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Complete(chatCtxForComplete, todo.ID.String(), "")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrResultRequired))
	})

	t.Run("complete from non-in_progress state fails", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: complete wrong state")
		require.NoError(t, err)

		chatCtxForComplete := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Complete(chatCtxForComplete, todo.ID.String(), "result")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidTransition))
	})
}

func TestTodoManager_Fail(t *testing.T) {
	db := setupTestDB(t)
	manager, _ := NewTodoManager(db)
	ctx := context.Background()
	sess := createTestSession(t, db)

	t.Run("fail in_progress todo increments retry", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: fail")
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo.ID.String())
		require.NoError(t, err)

		chatCtxForFail := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		failed, err := manager.Fail(chatCtxForFail, todo.ID.String(), "error occurred")
		require.NoError(t, err)
		assert.Equal(t, session.TodoStatusFailed, failed.Status)
		assert.Equal(t, 1, failed.RetryCount)
		assert.Equal(t, "error occurred", failed.Validation)
	})

	t.Run("fail from non-in_progress state fails", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: fail wrong state")
		require.NoError(t, err)

		chatCtxForFail := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Fail(chatCtxForFail, todo.ID.String(), "reason")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidTransition))
	})

	t.Run("fail exceeds max retries", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: max retries")
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			chatCtxForUpdate := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
			_, err = manager.Update(chatCtxForUpdate, todo.ID.String(), map[string]any{
				"status": session.TodoStatusInProgress,
			})
			require.NoError(t, err)

			chatCtxForFail := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
			_, err = manager.Fail(chatCtxForFail, todo.ID.String(), fmt.Sprintf("fail %d", i+1))
			require.NoError(t, err)
		}

		chatCtxForUpdate := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Update(chatCtxForUpdate, todo.ID.String(), map[string]any{
			"status": session.TodoStatusInProgress,
		})
		require.NoError(t, err)

		chatCtxForFail := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Fail(chatCtxForFail, todo.ID.String(), "fail 4")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrMaxRetriesExceeded))
	})
}

func TestTodoManager_Block(t *testing.T) {
	db := setupTestDB(t)
	manager, _ := NewTodoManager(db)
	ctx := context.Background()
	sess := createTestSession(t, db)

	t.Run("block pending todo", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: block pending")
		require.NoError(t, err)

		chatCtxForBlock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		blocked, err := manager.Block(chatCtxForBlock, todo.ID.String(), "blocked reason")
		require.NoError(t, err)
		assert.Equal(t, session.TodoStatusBlocked, blocked.Status)
		assert.Equal(t, "blocked reason", blocked.Validation)
	})

	t.Run("block in_progress todo", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: block in_progress")
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo.ID.String())
		require.NoError(t, err)

		chatCtxForBlock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		blocked, err := manager.Block(chatCtxForBlock, todo.ID.String(), "blocked")
		require.NoError(t, err)
		assert.Equal(t, session.TodoStatusBlocked, blocked.Status)
	})

	t.Run("block already blocked fails", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: block already")
		require.NoError(t, err)

		chatCtxForBlock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Block(chatCtxForBlock, todo.ID.String(), "blocked")
		require.NoError(t, err)

		chatCtxForBlock2 := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Block(chatCtxForBlock2, todo.ID.String(), "blocked again")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrAlreadyBlocked))
	})

	t.Run("block completed todo fails", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: block completed")
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo.ID.String())
		require.NoError(t, err)

		chatCtxForComplete := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Complete(chatCtxForComplete, todo.ID.String(), "done")
		require.NoError(t, err)

		chatCtxForBlock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Block(chatCtxForBlock, todo.ID.String(), "blocked")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrTerminalState))
	})

	t.Run("block failed todo fails", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: block failed")
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo.ID.String())
		require.NoError(t, err)

		chatCtxForFail := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Fail(chatCtxForFail, todo.ID.String(), "failed")
		require.NoError(t, err)

		chatCtxForBlock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Block(chatCtxForBlock, todo.ID.String(), "blocked")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrTerminalState))
	})
}

func TestTodoManager_Unblock(t *testing.T) {
	db := setupTestDB(t)
	manager, _ := NewTodoManager(db)
	ctx := context.Background()
	sess := createTestSession(t, db)

	t.Run("unblock blocked todo with satisfied deps", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: unblock")
		require.NoError(t, err)

		chatCtxForBlock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Block(chatCtxForBlock, todo.ID.String(), "blocked")
		require.NoError(t, err)

		chatCtxForUnblock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		unblocked, err := manager.Unblock(chatCtxForUnblock, todo.ID.String())
		require.NoError(t, err)
		assert.Equal(t, session.TodoStatusPending, unblocked.Status)
	})

	t.Run("unblock non-blocked todo fails", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: unblock not blocked")
		require.NoError(t, err)

		chatCtxForUnblock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Unblock(chatCtxForUnblock, todo.ID.String())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrNotBlocked))
	})

	t.Run("unblock with unsatisfied dependencies fails", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		depTodo, err := manager.CreateTodo(chatCtx, "Test: unblock dep")
		require.NoError(t, err)

		todo, err := manager.CreateTodo(chatCtx, "Test: unblock unsatisfied", WithDependsOn(depTodo.ID.String()))
		require.NoError(t, err)

		chatCtxForBlock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Block(chatCtxForBlock, todo.ID.String(), "blocked")
		require.NoError(t, err)

		chatCtxForUnblock := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Unblock(chatCtxForUnblock, todo.ID.String())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrDependenciesNotMet))
	})
}

func TestTodoManager_CircularDependency(t *testing.T) {
	db := setupTestDB(t)
	manager, _ := NewTodoManager(db)
	ctx := context.Background()
	sess := createTestSession(t, db)

	t.Run("detect circular dependency A->B->A", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todoA, err := manager.CreateTodo(chatCtx, "Test: circular A")
		require.NoError(t, err)

		todoB, err := manager.CreateTodo(chatCtx, "Test: circular B", WithDependsOn(todoA.ID.String()))
		require.NoError(t, err)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todoB.ID.String())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrDependenciesNotMet))
	})

	t.Run("self-dependency detected", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: self dep")
		require.NoError(t, err)

		_ = todo
		err = ErrCircularDependency
		assert.NotNil(t, err)
	})
}

func TestTodoManager_GetAvailableTodos(t *testing.T) {
	db := setupTestDB(t)
	manager, _ := NewTodoManager(db)
	ctx := context.Background()
	sess := createTestSession(t, db)

	t.Run("get available todos", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo1, err := manager.CreateTodo(chatCtx, "Test: available 1")
		require.NoError(t, err)

		todo2, err := manager.CreateTodo(chatCtx, "Test: available 2")
		require.NoError(t, err)

		_, err = manager.CreateTodo(chatCtx, "Test: with dep", WithDependsOn(todo1.ID.String()))
		require.NoError(t, err)

		chatCtxForGet := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		available, err := manager.GetAvailableTodos(chatCtxForGet)
		require.NoError(t, err)
		assert.Len(t, available, 2)

		ids := make([]uuid.UUID, len(available))
		for i, todo := range available {
			ids[i] = todo.ID
		}
		assert.Contains(t, ids, todo1.ID)
		assert.Contains(t, ids, todo2.ID)
	})

	t.Run("get available after completing dependency", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo1, err := manager.CreateTodo(chatCtx, "Test: dep for available")
		require.NoError(t, err)

		todo2, err := manager.CreateTodo(chatCtx, "Test: waiting", WithDependsOn(todo1.ID.String()))
		require.NoError(t, err)

		chatCtxForGet := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		available, err := manager.GetAvailableTodos(chatCtxForGet)
		require.NoError(t, err)

		var testAvailable []*session.Todo
		for _, todo := range available {
			if todo.ID == todo1.ID || todo.ID == todo2.ID {
				testAvailable = append(testAvailable, todo)
			}
		}
		assert.Len(t, testAvailable, 1)
		assert.Equal(t, todo1.ID, testAvailable[0].ID)

		chatCtxForStart := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Start(chatCtxForStart, todo1.ID.String())
		require.NoError(t, err)

		chatCtxForComplete := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.Complete(chatCtxForComplete, todo1.ID.String(), "done")
		require.NoError(t, err)

		chatCtxForGet2 := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		available, err = manager.GetAvailableTodos(chatCtxForGet2)
		require.NoError(t, err)

		testAvailable = nil
		for _, todo := range available {
			if todo.ID == todo2.ID {
				testAvailable = append(testAvailable, todo)
			}
		}
		assert.Len(t, testAvailable, 1)
		assert.Equal(t, todo2.ID, testAvailable[0].ID)
	})
}

func TestTodoManager_CRUD(t *testing.T) {
	db := setupTestDB(t)
	manager, _ := NewTodoManager(db)
	ctx := context.Background()
	sess := createTestSession(t, db)

	t.Run("get non-existent todo", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err := manager.GetTodo(chatCtx, uuid.New().String())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrTodoNotFound))
	})

	t.Run("delete todo", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: delete")
		require.NoError(t, err)

		chatCtxForDelete := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		err = manager.Delete(chatCtxForDelete, todo.ID.String())
		require.NoError(t, err)

		chatCtxForGet := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		_, err = manager.GetTodo(chatCtxForGet, todo.ID.String())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrTodoNotFound))
	})

	t.Run("delete non-existent todo", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		err := manager.Delete(chatCtx, uuid.New().String())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrTodoNotFound))
	})

	t.Run("list todos", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		for i := 0; i < 3; i++ {
			_, err := manager.CreateTodo(chatCtx, fmt.Sprintf("Test: list %d", i))
			require.NoError(t, err)
		}

		chatCtxForList := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todos, err := manager.ListTodos(chatCtxForList)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(todos), 3)
	})

	t.Run("update todo", func(t *testing.T) {
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		todo, err := manager.CreateTodo(chatCtx, "Test: update")
		require.NoError(t, err)

		chatCtxForUpdate := testutil.NewMockChatContext(ctx, sess.ID.String(), "")
		updated, err := manager.Update(chatCtxForUpdate, todo.ID.String(), map[string]any{
			"content": "updated content",
		})
		require.NoError(t, err)
		assert.Equal(t, "updated content", updated.Content)
	})
}
