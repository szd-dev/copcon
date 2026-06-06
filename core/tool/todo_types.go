package tool

import (
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/google/uuid"
)

// TodoOption configures a storage.Todo when creating a new todo item.
type TodoOption func(*storage.Todo)

// WithValidation sets the validation expression for a todo.
func WithValidation(validation string) TodoOption {
	return func(t *storage.Todo) {
		t.Validation = validation
	}
}

// WithDependsOn sets the dependency list for a todo.
func WithDependsOn(depIDs ...string) TodoOption {
	return func(t *storage.Todo) {
		deps := make([]uuid.UUID, 0, len(depIDs))
		for _, id := range depIDs {
			if uid, err := uuid.Parse(id); err == nil {
				deps = append(deps, uid)
			}
		}
		t.DependsOn = deps
	}
}

// WithActiveForm sets the active form identifier for a todo.
func WithActiveForm(form string) TodoOption {
	return func(t *storage.Todo) {
		t.ActiveForm = form
	}
}

// TodoManager defines the interface for todo lifecycle management.
type TodoManager interface {
	CreateTodo(chatCtx iface.ChatContextInterface, content string, opts ...TodoOption) (*storage.Todo, error)
	GetTodo(chatCtx iface.ChatContextInterface, id string) (*storage.Todo, error)
	ListTodos(chatCtx iface.ChatContextInterface) ([]*storage.Todo, error)
	Delete(chatCtx iface.ChatContextInterface, id string) error
	Start(chatCtx iface.ChatContextInterface, id string) (*storage.Todo, error)
	Complete(chatCtx iface.ChatContextInterface, id string, result string) (*storage.Todo, error)
	Fail(chatCtx iface.ChatContextInterface, id string, reason string) (*storage.Todo, error)
}