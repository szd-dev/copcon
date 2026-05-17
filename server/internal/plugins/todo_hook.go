package plugins

import (
	"log/slog"

	"github.com/copcon/server/internal/hook"
	"github.com/copcon/server/internal/tools/todo"
)

// TodoInjectionHook appends the current todo list state to the system prompt
// on the OnSystemPrompt hook point. This ensures the agent is aware of its
// task list whenever the context is built.
type TodoInjectionHook struct {
	todoMgr todo.TodoManager
	logger  *slog.Logger
}

// NewTodoInjectionHook creates a new TodoInjectionHook.
func NewTodoInjectionHook(todoMgr todo.TodoManager) *TodoInjectionHook {
	return &TodoInjectionHook{
		todoMgr: todoMgr,
		logger:  slog.Default(),
	}
}

// Name returns a human-readable identifier for logging and debugging.
func (h *TodoInjectionHook) Name() string {
	return "todo_injection"
}

// Points returns the hook points at which this hook should execute.
func (h *TodoInjectionHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.OnSystemPrompt}
}

// Priority returns the execution order. 50 means it runs before
// other hooks that may further modify the system prompt.
func (h *TodoInjectionHook) Priority() int {
	return 50
}

// Execute fetches the current todo list and appends a formatted summary
// to the system prompt. If the system prompt is nil, or fetching todos
// fails, the hook returns nil so the pipeline continues uninterrupted.
func (h *TodoInjectionHook) Execute(ctx *hook.HookContext) error {
	if ctx.SystemPrompt == nil {
		return nil
	}

	todos, err := h.todoMgr.List(ctx.ChatCtx)
	if err != nil {
		h.logger.Warn("failed to fetch todos",
			"session_id", ctx.SessionID,
			"error", err,
		)
		return nil
	}

	if len(todos) > 0 {
		todoState := formatTodoState(todos)
		*ctx.SystemPrompt = *ctx.SystemPrompt + "\n\n" + todoState
	}

	return nil
}
