package adapter

import (
	"github.com/copcon/core/capabilities/tools"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tools/todo"
)

type TodoManagerAdapter struct {
	Inner todo.TodoManager
}

func NewTodoManagerAdapter(inner todo.TodoManager) *TodoManagerAdapter {
	return &TodoManagerAdapter{Inner: inner}
}

func (a *TodoManagerAdapter) CreateTodo(chatCtx iface.ChatContextInterface, content string, opts ...tools.TodoOption) (*storage.Todo, error) {
	var serverOpts []todo.TodoOption

	if len(opts) > 0 {
		tmp := &storage.Todo{}
		for _, opt := range opts {
			opt(tmp)
		}
		if tmp.Validation != "" {
			serverOpts = append(serverOpts, todo.WithValidation(tmp.Validation))
		}
		if len(tmp.DependsOn) > 0 {
			deps := make([]string, len(tmp.DependsOn))
			for i, id := range tmp.DependsOn {
				deps[i] = id.String()
			}
			serverOpts = append(serverOpts, todo.WithDependsOn(deps...))
		}
		if tmp.ActiveForm != "" {
			serverOpts = append(serverOpts, todo.WithActiveForm(tmp.ActiveForm))
		}
	}

	t, err := a.Inner.CreateTodo(chatCtx, content, serverOpts...)
	if err != nil {
		return nil, err
	}
	return session.TodoToStorage(t), nil
}

func (a *TodoManagerAdapter) GetTodo(chatCtx iface.ChatContextInterface, id string) (*storage.Todo, error) {
	t, err := a.Inner.GetTodo(chatCtx, id)
	if err != nil {
		return nil, err
	}
	return session.TodoToStorage(t), nil
}

func (a *TodoManagerAdapter) ListTodos(chatCtx iface.ChatContextInterface) ([]*storage.Todo, error) {
	todos, err := a.Inner.ListTodos(chatCtx)
	if err != nil {
		return nil, err
	}
	result := make([]*storage.Todo, len(todos))
	for i, t := range todos {
		result[i] = session.TodoToStorage(t)
	}
	return result, nil
}

func (a *TodoManagerAdapter) Delete(chatCtx iface.ChatContextInterface, id string) error {
	return a.Inner.Delete(chatCtx, id)
}

func (a *TodoManagerAdapter) Start(chatCtx iface.ChatContextInterface, id string) (*storage.Todo, error) {
	t, err := a.Inner.Start(chatCtx, id)
	if err != nil {
		return nil, err
	}
	return session.TodoToStorage(t), nil
}

func (a *TodoManagerAdapter) Complete(chatCtx iface.ChatContextInterface, id string, result string) (*storage.Todo, error) {
	t, err := a.Inner.Complete(chatCtx, id, result)
	if err != nil {
		return nil, err
	}
	return session.TodoToStorage(t), nil
}

func (a *TodoManagerAdapter) Fail(chatCtx iface.ChatContextInterface, id string, reason string) (*storage.Todo, error) {
	t, err := a.Inner.Fail(chatCtx, id, reason)
	if err != nil {
		return nil, err
	}
	return session.TodoToStorage(t), nil
}
