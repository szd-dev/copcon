package tools

import (
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/google/uuid"
)

type TodoOption func(*storage.Todo)

func WithValidation(validation string) TodoOption {
	return func(t *storage.Todo) {
		t.Validation = validation
	}
}

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

func WithActiveForm(form string) TodoOption {
	return func(t *storage.Todo) {
		t.ActiveForm = form
	}
}

type TodoManager interface {
	CreateTodo(chatCtx iface.ChatContextInterface, content string, opts ...TodoOption) (*storage.Todo, error)
	GetTodo(chatCtx iface.ChatContextInterface, id string) (*storage.Todo, error)
	ListTodos(chatCtx iface.ChatContextInterface) ([]*storage.Todo, error)
	Delete(chatCtx iface.ChatContextInterface, id string) error
	Start(chatCtx iface.ChatContextInterface, id string) (*storage.Todo, error)
	Complete(chatCtx iface.ChatContextInterface, id string, result string) (*storage.Todo, error)
	Fail(chatCtx iface.ChatContextInterface, id string, reason string) (*storage.Todo, error)
}