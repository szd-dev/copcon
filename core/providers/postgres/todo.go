package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/core/storage"
)

var ErrTodoNotFound = errors.New("todo not found")

type TodoStore struct {
	db *gorm.DB
}

func (t *TodoStore) Create(ctx context.Context, todo *storage.Todo) (*storage.Todo, error) {
	model := todoFromStorage(todo)
	if model.ID == uuid.Nil {
		model.ID = uuid.New()
	}
	if model.Status == "" {
		model.Status = TodoStatusPending
	}

	if err := t.db.WithContext(ctx).Create(model).Error; err != nil {
		return nil, fmt.Errorf("create todo: %w", err)
	}
	return todoToStorage(model), nil
}

func (t *TodoStore) Get(ctx context.Context, id uuid.UUID) (*storage.Todo, error) {
	var m Todo
	if err := t.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTodoNotFound
		}
		return nil, fmt.Errorf("get todo: %w", err)
	}
	return todoToStorage(&m), nil
}

func (t *TodoStore) List(ctx context.Context, sessionID uuid.UUID) ([]*storage.Todo, error) {
	var todos []*Todo

	if err := t.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Find(&todos).Error; err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}

	result := make([]*storage.Todo, len(todos))
	for i, td := range todos {
		result[i] = todoToStorage(td)
	}
	return result, nil
}

func (t *TodoStore) UpdateStatus(ctx context.Context, id uuid.UUID, status storage.TodoStatus) (*storage.Todo, error) {
	result := t.db.WithContext(ctx).
		Model(&Todo{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     TodoStatus(status),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return nil, fmt.Errorf("update todo status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrTodoNotFound
	}
	return t.Get(ctx, id)
}

func (t *TodoStore) DeleteBySession(ctx context.Context, sessionID uuid.UUID) error {
	return t.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&Todo{}).Error
}