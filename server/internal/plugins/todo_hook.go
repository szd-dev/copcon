package plugins

import (
	"log/slog"

	"github.com/google/uuid"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/storage"
)

// TodoInjectionHook appends the current todo list state to the system prompt
// on the OnSystemPrompt hook point. This ensures the agent is aware of its
// task list whenever the context is built.
type TodoInjectionHook struct {
	todoStore storage.TodoStore
	logger    *slog.Logger
}

// NewTodoInjectionHook creates a new TodoInjectionHook.
func NewTodoInjectionHook(todoStore storage.TodoStore) *TodoInjectionHook {
	return &TodoInjectionHook{
		todoStore: todoStore,
		logger:    slog.Default(),
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

	sessionUUID, err := uuid.Parse(ctx.SessionID)
	if err != nil {
		h.logger.Warn("failed to parse session id for todo injection",
			"session_id", ctx.SessionID,
			"error", err,
		)
		return nil
	}

	todos, err := h.todoStore.List(ctx.ChatCtx.Context(), sessionUUID)
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
