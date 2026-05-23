package plugins

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/storage"
)

// stubTodoStore implements storage.TodoStore for testing the TodoInjectionHook.
type stubTodoStore struct {
	todos []*storage.Todo
	err   error
}

func (m *stubTodoStore) Create(_ context.Context, todo *storage.Todo) (*storage.Todo, error) {
	return todo, nil
}
func (m *stubTodoStore) Get(_ context.Context, _ uuid.UUID) (*storage.Todo, error) {
	return nil, nil
}
func (m *stubTodoStore) List(_ context.Context, _ uuid.UUID) ([]*storage.Todo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.todos, nil
}
func (m *stubTodoStore) UpdateStatus(_ context.Context, _ uuid.UUID, _ storage.TodoStatus) (*storage.Todo, error) {
	return nil, nil
}
func (m *stubTodoStore) DeleteBySession(_ context.Context, _ uuid.UUID) error {
	return nil
}

// stubChatContext is a minimal ChatContextInterface for tests.
type stubChatContext struct {
	ctx       context.Context
	sessionID string
	agentID   string
}

func (s *stubChatContext) Context() context.Context                  { return s.ctx }
func (s *stubChatContext) SessionID() string                         { return s.sessionID }
func (s *stubChatContext) AgentID() string                           { return s.agentID }
func (s *stubChatContext) Events() <-chan entity.Event               { return nil }
func (s *stubChatContext) Emit(_ entity.Event)                       {}
func (s *stubChatContext) Close()                                    {}
func (s *stubChatContext) Closed() <-chan struct{}                   { ch := make(chan struct{}); close(ch); return ch }
func (s *stubChatContext) Depth() int                                { return 0 }
func (s *stubChatContext) Subscribe(int64) (*iface.Subscriber, bool) { return nil, false }
func (s *stubChatContext) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
	return nil, fmt.Errorf("stub: RequestInput not implemented")
}
func (s *stubChatContext) ResolveInput(interruptID string, resp *iface.InputResponse) error {
	return iface.ErrInterruptNotFound
}
func (s *stubChatContext) PendingInputs() []iface.InputRequest {
	return nil
}
func (s *stubChatContext) SetPartLocator(messageID string, stepIndex, partIndex int) {}
func (s *stubChatContext) ClearPartLocator()                                         {}

func TestTodoInjectionHook_Name(t *testing.T) {
	h := NewTodoInjectionHook(&stubTodoStore{})
	assert.Equal(t, "todo_injection", h.Name())
}

func TestTodoInjectionHook_Points(t *testing.T) {
	h := NewTodoInjectionHook(&stubTodoStore{})
	assert.Equal(t, []hook.HookPoint{hook.OnSystemPrompt}, h.Points())
}

func TestTodoInjectionHook_Priority(t *testing.T) {
	h := NewTodoInjectionHook(&stubTodoStore{})
	assert.Equal(t, 50, h.Priority())
}

func TestTodoInjectionHook_SkipNilPrompt(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "00000000-0000-0000-0000-000000000001", agentID: "a1"}

	store := &stubTodoStore{
		todos: []*storage.Todo{
			{Content: "task 1", Status: storage.TodoStatusPending},
		},
	}

	h := NewTodoInjectionHook(store)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "00000000-0000-0000-0000-000000000001",
		AgentID:      "a1",
		SystemPrompt: nil,
		CurrentPoint: hook.OnSystemPrompt,
	})
	assert.NoError(t, err)
}

func TestTodoInjectionHook_EmptyTodos(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "00000000-0000-0000-0000-000000000001", agentID: "a1"}

	store := &stubTodoStore{
		todos: []*storage.Todo{},
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(store)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "00000000-0000-0000-0000-000000000001",
		AgentID:      "a1",
		SystemPrompt: &systemPrompt,
		CurrentPoint: hook.OnSystemPrompt,
	})
	require.NoError(t, err)
	assert.Equal(t, "You are helpful.", systemPrompt)
}

func TestTodoInjectionHook_AppendsTodos(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "00000000-0000-0000-0000-000000000002", agentID: "a2"}

	store := &stubTodoStore{
		todos: []*storage.Todo{
			{Content: "task A", Status: storage.TodoStatusPending},
			{Content: "task B", Status: storage.TodoStatusInProgress},
			{Content: "task C", Status: storage.TodoStatusCompleted},
		},
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(store)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "00000000-0000-0000-0000-000000000002",
		AgentID:      "a2",
		SystemPrompt: &systemPrompt,
		CurrentPoint: hook.OnSystemPrompt,
	})
	require.NoError(t, err)

	assert.Contains(t, systemPrompt, "You are helpful.")
	assert.Contains(t, systemPrompt, "Current todo list")
	assert.Contains(t, systemPrompt, "pending: task A")
	assert.Contains(t, systemPrompt, "in_progress: task B")
	assert.Contains(t, systemPrompt, "completed: task C")
}

func TestTodoInjectionHook_ErrorGraceful(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "00000000-0000-0000-0000-000000000003", agentID: "a3"}

	store := &stubTodoStore{
		err: errors.New("db connection lost"),
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(store)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "00000000-0000-0000-0000-000000000003",
		AgentID:      "a3",
		SystemPrompt: &systemPrompt,
		CurrentPoint: hook.OnSystemPrompt,
	})
	assert.NoError(t, err)
	assert.Equal(t, "You are helpful.", systemPrompt)
}

func TestTodoInjectionHook_ActiveFormPriority(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "00000000-0000-0000-0000-000000000004", agentID: "a4"}

	store := &stubTodoStore{
		todos: []*storage.Todo{
			{Content: "raw content", ActiveForm: "polished form", Status: storage.TodoStatusPending},
		},
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(store)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "00000000-0000-0000-0000-000000000004",
		AgentID:      "a4",
		SystemPrompt: &systemPrompt,
		CurrentPoint: hook.OnSystemPrompt,
	})
	require.NoError(t, err)

	assert.Contains(t, systemPrompt, "pending: polished form")
	assert.NotContains(t, systemPrompt, "pending: raw content")
}

func TestTodoInjectionHook_AllStatuses(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "00000000-0000-0000-0000-000000000005", agentID: "a5"}

	store := &stubTodoStore{
		todos: []*storage.Todo{
			{Content: "P", Status: storage.TodoStatusPending},
			{Content: "I", Status: storage.TodoStatusInProgress},
			{Content: "C", Status: storage.TodoStatusCompleted},
			{Content: "F", Status: storage.TodoStatusFailed},
			{Content: "B", Status: storage.TodoStatusBlocked},
		},
	}

	systemPrompt := "You are helpful."
	h := NewTodoInjectionHook(store)
	err := h.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "00000000-0000-0000-0000-000000000005",
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
