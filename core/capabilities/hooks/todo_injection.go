package hooks

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/storage"
)

type TodoInjectionHook struct {
	todoStore storage.TodoStore
	logger    *slog.Logger
}

func NewTodoInjectionHook(todoStore storage.TodoStore) *TodoInjectionHook {
	return &TodoInjectionHook{
		todoStore: todoStore,
		logger:    slog.Default(),
	}
}

func (h *TodoInjectionHook) Name() string {
	return "todo_injection"
}

func (h *TodoInjectionHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.OnSystemPrompt}
}

func (h *TodoInjectionHook) Priority() int {
	return 50
}

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

func formatTodoState(todos []*storage.Todo) string {
	var pending, inProgress, completed, failed, blocked []string

	for _, t := range todos {
		content := t.Content
		if t.ActiveForm != "" {
			content = t.ActiveForm
		}
		switch t.Status {
		case storage.TodoStatusPending:
			pending = append(pending, content)
		case storage.TodoStatusInProgress:
			inProgress = append(inProgress, content)
		case storage.TodoStatusCompleted:
			completed = append(completed, content)
		case storage.TodoStatusFailed:
			failed = append(failed, content)
		case storage.TodoStatusBlocked:
			blocked = append(blocked, content)
		}
	}

	var parts []string
	if len(pending) > 0 {
		parts = append(parts, "pending: "+strings.Join(pending, ", "))
	}
	if len(inProgress) > 0 {
		parts = append(parts, "in_progress: "+strings.Join(inProgress, ", "))
	}
	if len(completed) > 0 {
		parts = append(parts, "completed: "+strings.Join(completed, ", "))
	}
	if len(failed) > 0 {
		parts = append(parts, "failed: "+strings.Join(failed, ", "))
	}
	if len(blocked) > 0 {
		parts = append(parts, "blocked: "+strings.Join(blocked, ", "))
	}

	return "Current todo list: [" + strings.Join(parts, ", ") + "]"
}

type todoInjectionHookCapability struct{}

func (c *todoInjectionHookCapability) Name() string                         { return capabilities.HookTodoInjection }
func (c *todoInjectionHookCapability) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeHook }
func (c *todoInjectionHookCapability) DependsOn() []string                  { return nil }
func (c *todoInjectionHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	if deps.TodoStore == nil {
		return nil, fmt.Errorf("%w: TodoStore not configured", capabilities.ErrDependencyUnavailable)
	}
	return NewTodoInjectionHook(deps.TodoStore), nil
}