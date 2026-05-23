package todo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/core/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/core/storage"
)

// Compile-time check: todoManager satisfies storage.TodoStore.
var _ storage.TodoStore = (*todoManager)(nil)

var (
	ErrTodoNotFound       = errors.New("todo not found")
	ErrInvalidTransition  = errors.New("invalid state transition")
	ErrDependenciesNotMet = errors.New("dependencies not satisfied")
	ErrCircularDependency = errors.New("circular dependency detected")
	ErrResultRequired     = errors.New("result is required for completion")
	ErrTerminalState      = errors.New("cannot transition from terminal state")
	ErrMaxRetriesExceeded = errors.New("maximum retry count exceeded")
	ErrNotBlocked         = errors.New("todo is not blocked")
	ErrAlreadyBlocked     = errors.New("todo is already blocked")
)

// TodoOption is a functional option for creating todos
type TodoOption func(*session.Todo)

type TodoManager interface {
	CreateTodo(chatCtx iface.ChatContextInterface, content string, opts ...TodoOption) (*session.Todo, error)
	GetTodo(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error)
	ListTodos(chatCtx iface.ChatContextInterface) ([]*session.Todo, error)
	Update(chatCtx iface.ChatContextInterface, id string, updates map[string]any) (*session.Todo, error)
	Delete(chatCtx iface.ChatContextInterface, id string) error
	Start(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error)
	Complete(chatCtx iface.ChatContextInterface, id string, result string) (*session.Todo, error)
	Fail(chatCtx iface.ChatContextInterface, id string, reason string) (*session.Todo, error)
	Block(chatCtx iface.ChatContextInterface, id string, reason string) (*session.Todo, error)
	Unblock(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error)
	GetAvailableTodos(chatCtx iface.ChatContextInterface) ([]*session.Todo, error)
}

// todoManager implements TodoManager interface with state machine validation
type todoManager struct {
	db *gorm.DB
}

// NewTodoManager creates a new TodoManager instance.
func NewTodoManager(db *gorm.DB) (TodoManager, storage.TodoStore) {
	m := &todoManager{db: db}
	return m, m
}

func (m *todoManager) CreateTodo(chatCtx iface.ChatContextInterface, content string, opts ...TodoOption) (*session.Todo, error) {
	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return nil, fmt.Errorf("parse session id: %w", err)
	}

	var existing session.Todo
	err = m.db.WithContext(chatCtx.Context()).
		Where("session_id = ? AND content = ? AND status != ?", sessionUUID, content, session.TodoStatusCompleted).
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}

	todo := &session.Todo{
		ID:         uuid.New(),
		SessionID:  sessionUUID,
		Content:    content,
		Status:     session.TodoStatusPending,
		RetryCount: 0,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	for _, opt := range opts {
		opt(todo)
	}

	if len(todo.DependsOn) > 0 {
		if err := m.validateNoCircularDeps(chatCtx, todo.ID, todo.DependsOn); err != nil {
			return nil, err
		}
	}

	result := m.db.WithContext(chatCtx.Context()).Create(todo)
	if result.Error != nil {
		return nil, fmt.Errorf("create todo: %w", result.Error)
	}

	if len(todo.DependsOn) == 0 {
		started, err := m.Start(chatCtx, todo.ID.String())
		if err != nil {
			slog.Warn("failed to auto-start todo", "todo_id", todo.ID.String(), "error", err)
			return todo, nil
		}
		return started, nil
	}

	return todo, nil
}

func (m *todoManager) GetTodo(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error) {
	var todo session.Todo
	if err := m.db.WithContext(chatCtx.Context()).Where("id = ?", id).First(&todo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTodoNotFound
		}
		return nil, fmt.Errorf("get todo: %w", err)
	}
	return &todo, nil
}

func (m *todoManager) ListTodos(chatCtx iface.ChatContextInterface) ([]*session.Todo, error) {
	var todos []*session.Todo

	if err := m.db.WithContext(chatCtx.Context()).
		Where("session_id = ?", chatCtx.SessionID()).
		Order("created_at DESC").
		Find(&todos).Error; err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}

	return todos, nil
}

func (m *todoManager) Update(chatCtx iface.ChatContextInterface, id string, updates map[string]any) (*session.Todo, error) {
	todo, err := m.GetTodo(chatCtx, id)
	if err != nil {
		return nil, err
	}

	result := m.db.WithContext(chatCtx.Context()).Model(todo).Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("update todo: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrTodoNotFound
	}

	return m.GetTodo(chatCtx, id)
}

func (m *todoManager) Delete(chatCtx iface.ChatContextInterface, id string) error {
	result := m.db.WithContext(chatCtx.Context()).Delete(&session.Todo{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("delete todo: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrTodoNotFound
	}
	return nil
}

func (m *todoManager) Start(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error) {
	todo, err := m.GetTodo(chatCtx, id)
	if err != nil {
		return nil, err
	}

	currentStatus := session.TodoStatus(todo.Status)
	if currentStatus != session.TodoStatusPending && currentStatus != session.TodoStatusBlocked {
		return nil, fmt.Errorf("%w: cannot start todo in status %s", ErrInvalidTransition, todo.Status)
	}

	if err := m.checkDependencies(chatCtx, todo); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDependenciesNotMet, err)
	}

	return m.Update(chatCtx, id, map[string]any{
		"status":     session.TodoStatusInProgress,
		"updated_at": time.Now(),
	})
}

func (m *todoManager) Complete(chatCtx iface.ChatContextInterface, id string, result string) (*session.Todo, error) {
	if result == "" {
		return nil, ErrResultRequired
	}

	todo, err := m.GetTodo(chatCtx, id)
	if err != nil {
		return nil, err
	}

	currentStatus := session.TodoStatus(todo.Status)
	if currentStatus != session.TodoStatusInProgress {
		return nil, fmt.Errorf("%w: cannot complete todo in status %s", ErrInvalidTransition, todo.Status)
	}

	now := time.Now()
	return m.Update(chatCtx, id, map[string]any{
		"status":       session.TodoStatusCompleted,
		"result":       result,
		"completed_at": now,
		"updated_at":   now,
	})
}

func (m *todoManager) Fail(chatCtx iface.ChatContextInterface, id string, reason string) (*session.Todo, error) {
	todo, err := m.GetTodo(chatCtx, id)
	if err != nil {
		return nil, err
	}

	currentStatus := session.TodoStatus(todo.Status)
	if currentStatus != session.TodoStatusInProgress {
		return nil, fmt.Errorf("%w: cannot fail todo in status %s", ErrInvalidTransition, todo.Status)
	}

	newRetryCount := todo.RetryCount + 1

	if newRetryCount > 3 {
		return nil, fmt.Errorf("%w: retry count would exceed maximum (3)", ErrMaxRetriesExceeded)
	}

	updates := map[string]any{
		"status":      session.TodoStatusFailed,
		"retry_count": newRetryCount,
		"updated_at":  time.Now(),
	}

	if reason != "" {
		updates["validation"] = reason
	}

	return m.Update(chatCtx, id, updates)
}

func (m *todoManager) Block(chatCtx iface.ChatContextInterface, id string, reason string) (*session.Todo, error) {
	todo, err := m.GetTodo(chatCtx, id)
	if err != nil {
		return nil, err
	}

	currentStatus := session.TodoStatus(todo.Status)
	if currentStatus == session.TodoStatusBlocked {
		return nil, ErrAlreadyBlocked
	}
	if currentStatus == session.TodoStatusCompleted || currentStatus == session.TodoStatusFailed {
		return nil, fmt.Errorf("%w: cannot block todo in terminal state %s", ErrTerminalState, todo.Status)
	}

	updates := map[string]any{
		"status":     session.TodoStatusBlocked,
		"updated_at": time.Now(),
	}

	if reason != "" {
		updates["validation"] = reason
	}

	return m.Update(chatCtx, id, updates)
}

func (m *todoManager) Unblock(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error) {
	todo, err := m.GetTodo(chatCtx, id)
	if err != nil {
		return nil, err
	}

	currentStatus := session.TodoStatus(todo.Status)
	if currentStatus != session.TodoStatusBlocked {
		return nil, ErrNotBlocked
	}

	if err := m.checkDependencies(chatCtx, todo); err != nil {
		return nil, fmt.Errorf("%w: still has unsatisfied dependencies", ErrDependenciesNotMet)
	}

	return m.Update(chatCtx, id, map[string]any{
		"status":     session.TodoStatusPending,
		"updated_at": time.Now(),
	})
}

func (m *todoManager) GetAvailableTodos(chatCtx iface.ChatContextInterface) ([]*session.Todo, error) {
	var todos []*session.Todo

	if err := m.db.WithContext(chatCtx.Context()).
		Where("session_id = ? AND status = ?", chatCtx.SessionID(), session.TodoStatusPending).
		Find(&todos).Error; err != nil {
		return nil, fmt.Errorf("get available todos: %w", err)
	}

	var available []*session.Todo
	for _, todo := range todos {
		if len(todo.DependsOn) == 0 {
			available = append(available, todo)
			continue
		}

		var completedCount int64
		err := m.db.WithContext(chatCtx.Context()).
			Model(&session.Todo{}).
			Where("id IN ? AND status = ?", todo.DependsOn, session.TodoStatusCompleted).
			Count(&completedCount).Error
		if err != nil {
			return nil, fmt.Errorf("check dependencies: %w", err)
		}

		if int(completedCount) == len(todo.DependsOn) {
			available = append(available, todo)
		}
	}

	return available, nil
}

func (m *todoManager) checkDependencies(chatCtx iface.ChatContextInterface, todo *session.Todo) error {
	if len(todo.DependsOn) == 0 {
		return nil
	}

	for _, depID := range todo.DependsOn {
		dep, err := m.GetTodo(chatCtx, depID.String())
		if err != nil {
			if errors.Is(err, ErrTodoNotFound) {
				return fmt.Errorf("dependency %s not found", depID)
			}
			return fmt.Errorf("failed to check dependency %s: %w", depID, err)
		}

		if session.TodoStatus(dep.Status) != session.TodoStatusCompleted {
			return fmt.Errorf("dependency %s not completed (status: %s)", depID, dep.Status)
		}
	}

	return nil
}

func (m *todoManager) validateNoCircularDeps(chatCtx iface.ChatContextInterface, todoID uuid.UUID, deps []uuid.UUID) error {
	visited := make(map[uuid.UUID]bool)
	recStack := make(map[uuid.UUID]bool)

	var dfs func(id uuid.UUID) error
	dfs = func(id uuid.UUID) error {
		visited[id] = true
		recStack[id] = true

		todo, err := m.GetTodo(chatCtx, id.String())
		if err != nil {
			if errors.Is(err, ErrTodoNotFound) {
				recStack[id] = false
				return nil
			}
			return err
		}

		for _, depID := range todo.DependsOn {
			if depID == todoID {
				return fmt.Errorf("%w: %s -> %s", ErrCircularDependency, id, todoID)
			}

			if !visited[depID] {
				if err := dfs(depID); err != nil {
					return err
				}
			} else if recStack[depID] {
				return fmt.Errorf("%w: cycle detected involving %s", ErrCircularDependency, depID)
			}
		}

		recStack[id] = false
		return nil
	}

	for _, depID := range deps {
		if depID == todoID {
			return fmt.Errorf("%w: todo cannot depend on itself", ErrCircularDependency)
		}
		if err := dfs(depID); err != nil {
			return err
		}
	}

	return nil
}

// WithDependsOn sets dependencies for a todo
func WithDependsOn(depIDs ...string) TodoOption {
	return func(t *session.Todo) {
		deps := make(session.UUIDArray, 0, len(depIDs))
		for _, id := range depIDs {
			if uid, err := uuid.Parse(id); err == nil {
				deps = append(deps, uid)
			}
		}
		t.DependsOn = deps
	}
}

// WithActiveForm sets the active form for a todo
func WithActiveForm(form string) TodoOption {
	return func(t *session.Todo) {
		t.ActiveForm = form
	}
}

// WithValidation sets validation rules for a todo
func WithValidation(validation string) TodoOption {
	return func(t *session.Todo) {
		t.Validation = validation
	}
}

// storage.TodoStore interface methods

func (m *todoManager) Create(ctx context.Context, todo *storage.Todo) (*storage.Todo, error) {
	model := session.TodoFromStorage(todo)
	if model.ID == uuid.Nil {
		model.ID = uuid.New()
	}
	if model.Status == "" {
		model.Status = session.TodoStatusPending
	}

	if err := m.db.WithContext(ctx).Create(model).Error; err != nil {
		return nil, fmt.Errorf("create todo: %w", err)
	}
	return session.TodoToStorage(model), nil
}

func (m *todoManager) Get(ctx context.Context, id uuid.UUID) (*storage.Todo, error) {
	var t session.Todo
	if err := m.db.WithContext(ctx).Where("id = ?", id).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTodoNotFound
		}
		return nil, fmt.Errorf("get todo: %w", err)
	}
	return session.TodoToStorage(&t), nil
}

func (m *todoManager) List(ctx context.Context, sessionID uuid.UUID) ([]*storage.Todo, error) {
	var todos []*session.Todo

	if err := m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Find(&todos).Error; err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}

	result := make([]*storage.Todo, len(todos))
	for i, t := range todos {
		result[i] = session.TodoToStorage(t)
	}
	return result, nil
}

func (m *todoManager) UpdateStatus(ctx context.Context, id uuid.UUID, status storage.TodoStatus) (*storage.Todo, error) {
	result := m.db.WithContext(ctx).
		Model(&session.Todo{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     session.TodoStatus(status),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return nil, fmt.Errorf("update todo status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrTodoNotFound
	}
	return m.Get(ctx, id)
}

func (m *todoManager) DeleteBySession(ctx context.Context, sessionID uuid.UUID) error {
	return m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&session.Todo{}).Error
}
