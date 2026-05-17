package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/hook"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tools/todo"
)

// stubTodoManager implements TodoManager for testing the TodoInjectionHook.
type stubTodoManager struct {
	todos []*session.Todo
	err   error
}

func (m *stubTodoManager) Create(chatCtx iface.ChatContextInterface, content string, opts ...todo.TodoOption) (*session.Todo, error) {
	return nil, nil
}
func (m *stubTodoManager) Get(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error) {
	return nil, nil
}
func (m *stubTodoManager) List(chatCtx iface.ChatContextInterface) ([]*session.Todo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.todos, nil
}
func (m *stubTodoManager) Update(chatCtx iface.ChatContextInterface, id string, updates map[string]any) (*session.Todo, error) {
	return nil, nil
}
func (m *stubTodoManager) Delete(chatCtx iface.ChatContextInterface, id string) error {
	return nil
}
func (m *stubTodoManager) Start(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error) {
	return nil, nil
}
func (m *stubTodoManager) Complete(chatCtx iface.ChatContextInterface, id string, result string) (*session.Todo, error) {
	return nil, nil
}
func (m *stubTodoManager) Fail(chatCtx iface.ChatContextInterface, id string, reason string) (*session.Todo, error) {
	return nil, nil
}
func (m *stubTodoManager) Block(chatCtx iface.ChatContextInterface, id string, reason string) (*session.Todo, error) {
	return nil, nil
}
func (m *stubTodoManager) Unblock(chatCtx iface.ChatContextInterface, id string) (*session.Todo, error) {
	return nil, nil
}
func (m *stubTodoManager) GetAvailableTodos(chatCtx iface.ChatContextInterface) ([]*session.Todo, error) {
	return nil, nil
}
func (m *stubTodoManager) GetDB() *gorm.DB {
	return nil
}

// stubChatContext is a minimal ChatContextInterface for tests.
type stubChatContext struct {
	ctx       context.Context
	sessionID string
	agentID   string
}

func (s *stubChatContext) Context() context.Context    { return s.ctx }
func (s *stubChatContext) SessionID() string           { return s.sessionID }
func (s *stubChatContext) AgentID() string             { return s.agentID }
func (s *stubChatContext) Events() <-chan entity.Event { return nil }
func (s *stubChatContext) Emit(_ entity.Event)         {}

func TestTodoInjectionHook_Name(t *testing.T) {
	h := NewTodoInjectionHook(&stubTodoManager{})
	assert.Equal(t, "todo_injection", h.Name())
}

func TestTodoInjectionHook_Points(t *testing.T) {
	h := NewTodoInjectionHook(&stubTodoManager{})
	assert.Equal(t, []hook.HookPoint{hook.OnSystemPrompt}, h.Points())
}

func TestTodoInjectionHook_Priority(t *testing.T) {
	h := NewTodoInjectionHook(&stubTodoManager{})
	assert.Equal(t, 50, h.Priority())
}

func TestTodoInjectionHook_SkipNilPrompt(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s1", agentID: "a1"}

	mgr := &stubTodoManager{
		todos: []*session.Todo{
			{Content: "task 1", Status: session.TodoStatusPending},
		},
	}

	h := NewTodoInjectionHook(mgr)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s1",
		AgentID:      "a1",
		SystemPrompt: nil,
		CurrentPoint: hook.OnSystemPrompt,
	})
	assert.NoError(t, err)
}

func TestTodoInjectionHook_EmptyTodos(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s1", agentID: "a1"}

	mgr := &stubTodoManager{
		todos: []*session.Todo{},
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(mgr)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s1",
		AgentID:      "a1",
		SystemPrompt: &systemPrompt,
		CurrentPoint: hook.OnSystemPrompt,
	})
	require.NoError(t, err)
	// Prompt should be unchanged (empty todos = no-op)
	assert.Equal(t, "You are helpful.", systemPrompt)
}

func TestTodoInjectionHook_AppendsTodos(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s2", agentID: "a2"}

	mgr := &stubTodoManager{
		todos: []*session.Todo{
			{Content: "task A", Status: session.TodoStatusPending},
			{Content: "task B", Status: session.TodoStatusInProgress},
			{Content: "task C", Status: session.TodoStatusCompleted},
		},
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(mgr)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s2",
		AgentID:      "a2",
		SystemPrompt: &systemPrompt,
		CurrentPoint: hook.OnSystemPrompt,
	})
	require.NoError(t, err)

	// Prompt should be prefixed, with todo state appended
	assert.Contains(t, systemPrompt, "You are helpful.")
	assert.Contains(t, systemPrompt, "Current todo list")
	assert.Contains(t, systemPrompt, "pending: task A")
	assert.Contains(t, systemPrompt, "in_progress: task B")
	assert.Contains(t, systemPrompt, "completed: task C")
}

func TestTodoInjectionHook_ErrorGraceful(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s3", agentID: "a3"}

	mgr := &stubTodoManager{
		err: errors.New("db connection lost"),
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(mgr)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s3",
		AgentID:      "a3",
		SystemPrompt: &systemPrompt,
		CurrentPoint: hook.OnSystemPrompt,
	})
	// Error should be swallowed (graceful), hook returns nil
	assert.NoError(t, err)
	// Prompt should be unchanged
	assert.Equal(t, "You are helpful.", systemPrompt)
}

func TestTodoInjectionHook_ActiveFormPriority(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s4", agentID: "a4"}

	mgr := &stubTodoManager{
		todos: []*session.Todo{
			{Content: "raw content", ActiveForm: "polished form", Status: session.TodoStatusPending},
		},
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(mgr)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s4",
		AgentID:      "a4",
		SystemPrompt: &systemPrompt,
		CurrentPoint: hook.OnSystemPrompt,
	})
	require.NoError(t, err)

	// ActiveForm should be used, not Content
	assert.Contains(t, systemPrompt, "pending: polished form")
	assert.NotContains(t, systemPrompt, "pending: raw content")
}

func TestTodoInjectionHook_AllStatuses(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s5", agentID: "a5"}

	mgr := &stubTodoManager{
		todos: []*session.Todo{
			{Content: "P", Status: session.TodoStatusPending},
			{Content: "I", Status: session.TodoStatusInProgress},
			{Content: "C", Status: session.TodoStatusCompleted},
			{Content: "F", Status: session.TodoStatusFailed},
			{Content: "B", Status: session.TodoStatusBlocked},
		},
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(mgr)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s5",
		AgentID:      "a5",
		SystemPrompt: &systemPrompt,
		CurrentPoint: hook.OnSystemPrompt,
	})
	require.NoError(t, err)

	assert.Contains(t, systemPrompt, "pending: P")
	assert.Contains(t, systemPrompt, "in_progress: I")
	assert.Contains(t, systemPrompt, "completed: C")
	assert.Contains(t, systemPrompt, "failed: F")
	assert.Contains(t, systemPrompt, "blocked: B")
}
