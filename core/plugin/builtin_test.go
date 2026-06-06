package plugin

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

type mockAgentEngine struct{}

func (m *mockAgentEngine) Chat(_ iface.ChatContextInterface, _ string) error { return nil }

type mockAgentRegistry struct{}

func (m *mockAgentRegistry) Get(_ string) (agent.AgentDefinition, error) {
	return agent.AgentDefinition{}, agent.ErrAgentNotFound
}
func (m *mockAgentRegistry) List() []agent.AgentInfo                         { return nil }
func (m *mockAgentRegistry) Default() (agent.AgentDefinition, error)         { return agent.AgentDefinition{}, nil }
func (m *mockAgentRegistry) RegisterFactory(_, _, _ string, _ bool, _ agent.AgentFactory) {
}
func (m *mockAgentRegistry) GetFactory(_ string) (agent.AgentFactory, error) {
	return nil, agent.ErrAgentNotFound
}
func (m *mockAgentRegistry) ListDelegatable() []agent.AgentInfo { return nil }

type mockSessionStore struct{}

func (m *mockSessionStore) Create(_ context.Context, s *storage.Session) (*storage.Session, error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return s, nil
}
func (m *mockSessionStore) Get(_ context.Context, _ uuid.UUID) (*storage.Session, error) {
	return nil, nil
}
func (m *mockSessionStore) List(_ context.Context, _, _ int) ([]*storage.Session, int64, error) {
	return nil, 0, nil
}
func (m *mockSessionStore) Delete(_ context.Context, _ uuid.UUID) error          { return nil }
func (m *mockSessionStore) UpdateTitle(_ context.Context, _ uuid.UUID, _ string) error     { return nil }
func (m *mockSessionStore) UpdateMetadata(_ context.Context, _ uuid.UUID, _ map[string]any) error {
	return nil
}
func (m *mockSessionStore) GetMessageCount(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockSessionStore) AppendMetadata(_ context.Context, _ uuid.UUID, _ string, _ any) error {
	return nil
}

type mockMessageStore struct{}

func (m *mockMessageStore) List(_ context.Context, _ uuid.UUID, _ int) ([]*storage.Message, error) {
	return nil, nil
}
func (m *mockMessageStore) Add(_ context.Context, _ *storage.Message) error      { return nil }
func (m *mockMessageStore) Update(_ context.Context, _ *storage.Message) error   { return nil }
func (m *mockMessageStore) Upsert(_ context.Context, _ *storage.Message) error   { return nil }
func (m *mockMessageStore) DeleteBySession(_ context.Context, _ uuid.UUID) error { return nil }

type mockTodoStore struct {
	todos []*storage.Todo
}

func (m *mockTodoStore) Create(_ context.Context, todo *storage.Todo) (*storage.Todo, error) {
	if todo.ID == uuid.Nil {
		todo.ID = uuid.New()
	}
	m.todos = append(m.todos, todo)
	return todo, nil
}
func (m *mockTodoStore) Get(_ context.Context, id uuid.UUID) (*storage.Todo, error) {
	for _, t := range m.todos {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, nil
}
func (m *mockTodoStore) List(_ context.Context, sessionID uuid.UUID) ([]*storage.Todo, error) {
	var result []*storage.Todo
	for _, t := range m.todos {
		if t.SessionID == sessionID {
			result = append(result, t)
		}
	}
	return result, nil
}
func (m *mockTodoStore) UpdateStatus(_ context.Context, id uuid.UUID, status storage.TodoStatus) (*storage.Todo, error) {
	for _, t := range m.todos {
		if t.ID == id {
			t.Status = status
			return t, nil
		}
	}
	return nil, nil
}
func (m *mockTodoStore) DeleteBySession(_ context.Context, _ uuid.UUID) error { return nil }

func TestBuiltin_Name(t *testing.T) {
	p := NewBuiltin()
	assert.Equal(t, "builtin", p.Name())
}

func TestBuiltin_ToolsCount(t *testing.T) {
	p := NewBuiltin()
	tools := p.Tools()
	assert.Len(t, tools, 9)
}

func TestBuiltin_HooksCount(t *testing.T) {
	p := NewBuiltin()
	hooks := p.Hooks()
	assert.Len(t, hooks, 3)
}

func TestBuiltin_ToolNames(t *testing.T) {
	p := NewBuiltin()
	ts := p.Tools()
	expected := []string{
		"builtin.tool.code_executor",
		"builtin.tool.shell_executor",
		"builtin.tool.file_ops",
		"builtin.tool.todolist",
		"builtin.tool.confirm_action",
		"builtin.tool.ask_user",
		"builtin.tool.get_tool_status",
		"builtin.tool.delegate_to",
		"builtin.tool.read_sub_session",
	}
	names := make([]string, len(ts))
	for i, t2 := range ts {
		names[i] = t2.Name()
	}
	assert.ElementsMatch(t, expected, names)

	for _, t2 := range ts {
		assert.True(t, strings.HasPrefix(t2.Name(), "builtin.tool."),
			"tool name %q should have builtin.tool. prefix", t2.Name())
	}
}

func TestBuiltin_HookNames(t *testing.T) {
	p := NewBuiltin()
	hs := p.Hooks()
	expected := []string{
		"builtin.hook.logging",
		"builtin.hook.todo_injection",
		"builtin.hook.tracing",
	}
	names := make([]string, len(hs))
	for i, h := range hs {
		names[i] = h.Name()
	}
	assert.ElementsMatch(t, expected, names)

	for _, h := range hs {
		assert.True(t, strings.HasPrefix(h.Name(), "builtin.hook."),
			"hook name %q should have builtin.hook. prefix", h.Name())
	}
}

func TestBuiltin_InitInjectsDeps(t *testing.T) {
	p := NewBuiltin()

	deps := PluginDeps{
		Engine:        &mockAgentEngine{},
		AgentRegistry: &mockAgentRegistry{},
		SessionStore:  &mockSessionStore{},
		MessageStore:  &mockMessageStore{},
		TodoStore:     &mockTodoStore{},
	}

	err := p.Init(deps)
	require.NoError(t, err)

	b := p.(*builtin)

	require.NotNil(t, b.delegateTool.inner, "delegateTool should be initialized after Init")
	require.NotNil(t, b.todoTool.inner, "todoTool should be initialized after Init")
	require.NotNil(t, b.todoInjection.inner, "todoInjection should be initialized after Init")
	require.NotNil(t, b.readSubTool.inner, "readSubTool should be initialized after Init")

	assert.Equal(t, "delegate_to", b.delegateTool.inner.Name())
	assert.Equal(t, "todolist", b.todoTool.inner.Name())
	assert.Equal(t, "todo_injection", b.todoInjection.inner.Name())
	assert.Equal(t, "read_sub_session", b.readSubTool.inner.Name())
}

func TestBuiltin_InitRequiresEngine(t *testing.T) {
	p := NewBuiltin()

	deps := PluginDeps{
		Engine:        nil,
		AgentRegistry: &mockAgentRegistry{},
		SessionStore:  &mockSessionStore{},
		MessageStore:  &mockMessageStore{},
		TodoStore:     &mockTodoStore{},
	}

	err := p.Init(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Engine")
}

func TestBuiltin_InitWithWrongEngineType(t *testing.T) {
	p := NewBuiltin()

	deps := PluginDeps{
		Engine:        "not-an-engine",
		AgentRegistry: &mockAgentRegistry{},
		SessionStore:  &mockSessionStore{},
		MessageStore:  &mockMessageStore{},
		TodoStore:     &mockTodoStore{},
	}

	err := p.Init(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Engine")
}

func TestBuiltin_InitWithoutTodoStore(t *testing.T) {
	p := NewBuiltin()

	deps := PluginDeps{
		Engine:        &mockAgentEngine{},
		AgentRegistry: &mockAgentRegistry{},
		SessionStore:  &mockSessionStore{},
		MessageStore:  &mockMessageStore{},
		TodoStore:     nil,
	}

	err := p.Init(deps)
	require.NoError(t, err)

	b := p.(*builtin)

	assert.NotNil(t, b.delegateTool.inner, "delegateTool should still be initialized")
	assert.Equal(t, "todolist", b.todoTool.inner.Name(),
		"todoTool should remain as placeholder when TodoStore is nil")
}

func TestBuiltin_DelegateToolIsDelegationTool(t *testing.T) {
	p := NewBuiltin()
	ts := p.Tools()

	var delegateTool tool.Tool
	for _, t2 := range ts {
		if t2.Name() == "builtin.tool.delegate_to" {
			delegateTool = t2
			break
		}
	}
	require.NotNil(t, delegateTool, "delegate_to tool should exist")

	dt, ok := delegateTool.(tool.DelegationTool)
	assert.True(t, ok, "delegate_to tool should implement DelegationTool")
	assert.True(t, dt.IsDelegationTool())
}

func TestBuiltin_ToolsAreFunctional(t *testing.T) {
	p := NewBuiltin()
	ts := p.Tools()

	for _, t2 := range ts {
		assert.NotEmpty(t, t2.Name(), "tool should have a name")
	}
}
