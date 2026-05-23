package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TodoStatus represents the lifecycle state of a todo item.
type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
	TodoStatusBlocked    TodoStatus = "blocked"
	TodoStatusFailed     TodoStatus = "failed"
)

// Todo is a pure value type with no GORM annotations.
type Todo struct {
	ID          uuid.UUID
	SessionID   uuid.UUID
	Content     string
	ActiveForm  string
	Status      TodoStatus
	Priority    string
	DependsOn   []uuid.UUID
	Validation  string
	Result      string
	RetryCount  int
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TodoStore persists todos.
type TodoStore interface {
	Create(ctx context.Context, todo *Todo) (*Todo, error)
	Get(ctx context.Context, id uuid.UUID) (*Todo, error)
	List(ctx context.Context, sessionID uuid.UUID) ([]*Todo, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status TodoStatus) (*Todo, error)
	DeleteBySession(ctx context.Context, sessionID uuid.UUID) error
}