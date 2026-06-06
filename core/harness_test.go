package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testStoreProvider struct{}

func (testStoreProvider) Sessions() storage.SessionStore   { return nil }
func (testStoreProvider) Messages() storage.MessageStore   { return nil }
func (testStoreProvider) Todos() storage.TodoStore         { return nil }

func TestNewHarness_BasicBuild(t *testing.T) {
	p := &mockPlugin{
		name: "test-plugin",
		tools: []tool.Tool{
			&mockPluginTool{name: "test.tool.alpha"},
		},
		hooks: []hook.Hook{
			&mockPluginHook{name: "test.hook.beta"},
		},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{
				ID:            "test-agent",
				Name:          "Test Agent",
				Model:         "gpt-4o",
				SystemPrompt:  "You are a test agent.",
				Tools:         []string{"test.*"},
				AllowDelegate: false,
			},
		},
	})
	h.Register(p)

	err := h.Build()
	require.NoError(t, err)

	assert.NotNil(t, h.Engine())
	assert.NotNil(t, h.Registry())
	assert.True(t, h.built)
}

func TestNewHarness_DoubleBuild(t *testing.T) {
	p := &mockPlugin{
		name:  "test-plugin",
		tools: []tool.Tool{&mockPluginTool{name: "test.tool"}},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{
				ID:           "a",
				Name:         "A",
				Model:        "gpt-4o",
				SystemPrompt: "test",
			},
		},
	})
	h.Register(p)

	require.NoError(t, h.Build())
	err := h.Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already built")
}

func TestNewHarness_NilProviderReturnsError(t *testing.T) {
	p := &mockPlugin{
		name:  "test-plugin",
		tools: []tool.Tool{&mockPluginTool{name: "test.tool"}},
	}

	h := NewHarness(HarnessConfig{
		LLM: llm.NewMockProvider(),
		Agents: []AgentSpec{{ID: "a", Name: "A", Model: "gpt-4o", SystemPrompt: "test"}},
	})
	h.Register(p)

	err := h.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Provider")
}

func TestNewHarness_AgentFactorySpec(t *testing.T) {
	factoryCalled := false
	factory := func(_ context.Context, _ agent.CreateParams) (agent.AgentDefinition, error) {
		factoryCalled = true
		return agent.AgentDefinition{
			ID:           "factory-agent",
			Name:         "Factory Agent",
			Model:        "gpt-4o",
			SystemPrompt: "from factory",
		}, nil
	}

	p := &mockPlugin{
		name:  "test-plugin",
		tools: []tool.Tool{&mockPluginTool{name: "test.tool"}},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		AgentFactories: []AgentFactorySpec{
			{
				ID:            "factory-agent",
				Name:          "Factory Agent",
				Model:         "gpt-4o",
				Factory:       factory,
				AllowDelegate: true,
			},
		},
	})
	h.Register(p)

	require.NoError(t, h.Build())

	def, err := h.Registry().Get("factory-agent")
	require.NoError(t, err)
	assert.True(t, factoryCalled)
	assert.Equal(t, "Factory Agent", def.Name)
}

func TestNewHarness_FirstAgentIsDefault(t *testing.T) {
	p := &mockPlugin{
		name:  "test-plugin",
		tools: []tool.Tool{&mockPluginTool{name: "test.tool"}},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{ID: "first", Name: "First", Model: "gpt-4o", SystemPrompt: "first"},
			{ID: "second", Name: "Second", Model: "gpt-4o", SystemPrompt: "second"},
		},
	})
	h.Register(p)

	require.NoError(t, h.Build())

	def, err := h.Registry().Default()
	require.NoError(t, err)
	assert.Equal(t, "first", def.ID)
}

func TestNewHarness_DefaultFromFactorySpec(t *testing.T) {
	p := &mockPlugin{
		name:  "test-plugin",
		tools: []tool.Tool{&mockPluginTool{name: "test.tool"}},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		AgentFactories: []AgentFactorySpec{
			{
				ID:            "factory-default",
				Name:          "Factory Default",
				Model:         "gpt-4o",
				Factory:       func(_ context.Context, _ agent.CreateParams) (agent.AgentDefinition, error) {
					return agent.AgentDefinition{ID: "factory-default", Name: "Factory Default", Model: "gpt-4o"}, nil
				},
				AllowDelegate: false,
			},
		},
	})
	h.Register(p)

	require.NoError(t, h.Build())

	def, err := h.Registry().Default()
	require.NoError(t, err)
	assert.Equal(t, "factory-default", def.ID)
}

type mockPluginTool struct {
	name string
}

func (m *mockPluginTool) Name() string                           { return m.name }
func (m *mockPluginTool) Description() string                    { return "mock tool" }
func (m *mockPluginTool) InputSchema() map[string]any            { return nil }
func (m *mockPluginTool) Execute(iface.ChatContextInterface, map[string]any) (*tool.ToolResult, error) {
	return &tool.ToolResult{Success: true}, nil
}

type mockPluginHook struct {
	name string
}

func (m *mockPluginHook) Name() string              { return m.name }
func (m *mockPluginHook) Points() []hook.HookPoint  { return nil }
func (m *mockPluginHook) Priority() int              { return 100 }
func (m *mockPluginHook) Execute(_ *hook.HookContext) error { return nil }

type mockPlugin struct {
	name       string
	tools      []tool.Tool
	hooks      []hook.Hook
	initErr    error
	initCalled bool
	onInit     func(deps plugin.PluginDeps)
}

func (m *mockPlugin) Name() string        { return m.name }
func (m *mockPlugin) Tools() []tool.Tool  { return m.tools }
func (m *mockPlugin) Hooks() []hook.Hook  { return m.hooks }
func (m *mockPlugin) Init(deps plugin.PluginDeps) error {
	m.initCalled = true
	if m.onInit != nil {
		m.onInit(deps)
	}
	return m.initErr
}

func TestHarness_RegisterAndBuildFromPlugins(t *testing.T) {
	p := &mockPlugin{
		name: "test-plugin",
		tools: []tool.Tool{
			&mockPluginTool{name: "test.tool.alpha"},
			&mockPluginTool{name: "test.tool.beta"},
		},
		hooks: []hook.Hook{
			&mockPluginHook{name: "test.hook.gamma"},
		},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{
				ID:           "plugin-agent",
				Name:         "Plugin Agent",
				Model:        "gpt-4o",
				SystemPrompt: "test",
				Tools:        []string{"test.*"},
			},
		},
	})
	h.Register(p)

	err := h.Build()
	require.NoError(t, err)

	assert.NotNil(t, h.Engine())
	assert.NotNil(t, h.Registry())
	assert.NotNil(t, h.ToolPool())
	assert.NotNil(t, h.HookPool())
	assert.True(t, p.initCalled)

	def, err := h.Registry().Get("plugin-agent")
	require.NoError(t, err)
	assert.NotNil(t, def.ToolManager)

	tools := def.ToolManager.List()
	assert.Len(t, tools, 2, "agent should get tools matching test.* pattern")
}

func TestHarness_BuildFromPlugins_NamespaceSelect(t *testing.T) {
	p1 := &mockPlugin{
		name: "ns-a",
		tools: []tool.Tool{
			&mockPluginTool{name: "ns_a.tool.x"},
			&mockPluginTool{name: "ns_a.tool.y"},
		},
	}
	p2 := &mockPlugin{
		name: "ns-b",
		tools: []tool.Tool{
			&mockPluginTool{name: "ns_b.tool.z"},
		},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{
				ID: "agent-a", Name: "A", Model: "gpt-4o", SystemPrompt: "a",
				Tools: []string{"ns_a.*"},
			},
			{
				ID: "agent-b", Name: "B", Model: "gpt-4o", SystemPrompt: "b",
				Tools: []string{"ns_b.*"},
			},
			{
				ID: "agent-all", Name: "All", Model: "gpt-4o", SystemPrompt: "all",
				Tools: []string{"*"},
			},
		},
	})
	h.Register(p1)
	h.Register(p2)

	require.NoError(t, h.Build())

	defA, _ := h.Registry().Get("agent-a")
	toolsA := defA.ToolManager.List()
	assert.Len(t, toolsA, 2, "agent-a should get ns_a.* tools")

	defB, _ := h.Registry().Get("agent-b")
	toolsB := defB.ToolManager.List()
	assert.Len(t, toolsB, 1, "agent-b should get ns_b.* tools")

	defAll, _ := h.Registry().Get("agent-all")
	toolsAll := defAll.ToolManager.List()
	assert.Len(t, toolsAll, 3, "agent-all should get all tools")
}

func TestHarness_BuildFromPlugins_InitError(t *testing.T) {
	p := &mockPlugin{
		name:    "failing-plugin",
		tools:   []tool.Tool{&mockPluginTool{name: "fail.tool"}},
		initErr: fmt.Errorf("init failed"),
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{ID: "a", Name: "A", Model: "gpt-4o", SystemPrompt: "a"},
		},
	})
	h.Register(p)

	err := h.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "init plugin failing-plugin")
	assert.Contains(t, err.Error(), "init failed")
}

func TestHarness_BuildFromPlugins_ExactMatch(t *testing.T) {
	p := &mockPlugin{
		name: "exact-plugin",
		tools: []tool.Tool{
			&mockPluginTool{name: "exact.tool.one"},
			&mockPluginTool{name: "exact.tool.two"},
		},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{
				ID: "exact-agent", Name: "Exact", Model: "gpt-4o", SystemPrompt: "exact",
				Tools: []string{"exact.tool.one"},
			},
		},
	})
	h.Register(p)

	require.NoError(t, h.Build())

	def, _ := h.Registry().Get("exact-agent")
	tools := def.ToolManager.List()
	assert.Len(t, tools, 1, "exact match should return only the specified tool")
	assert.Equal(t, "exact.tool.one", tools[0].Name)
}

func TestHarness_BuildFromPlugins_HooksRegistered(t *testing.T) {
	p := &mockPlugin{
		name:  "hook-plugin",
		tools: []tool.Tool{},
		hooks: []hook.Hook{
			&mockPluginHook{name: "test.hook.before"},
			&mockPluginHook{name: "test.hook.after"},
		},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{ID: "a", Name: "A", Model: "gpt-4o", SystemPrompt: "a"},
		},
	})
	h.Register(p)

	require.NoError(t, h.Build())
	assert.NotNil(t, h.HookRunner())
	assert.NotNil(t, h.HookPool())

	allHooks := h.HookPool().All()
	assert.Len(t, allHooks, 2)
}

func TestHarness_BuildFromPlugins_PluginDepsInjected(t *testing.T) {
	var capturedDeps plugin.PluginDeps
	p := &mockPlugin{
		name:  "deps-plugin",
		tools: []tool.Tool{&mockPluginTool{name: "deps.tool"}},
		onInit: func(deps plugin.PluginDeps) {
			capturedDeps = deps
		},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{ID: "a", Name: "A", Model: "gpt-4o", SystemPrompt: "a"},
		},
	})
	h.Register(p)

	require.NoError(t, h.Build())
	assert.True(t, p.initCalled, "Init should have been called")
	assert.NotNil(t, capturedDeps.AgentRegistry)
	assert.NotNil(t, capturedDeps.Engine)
	assert.NotNil(t, capturedDeps.Logger)
}

func TestHarness_BuildFromPlugins_AgentFactorySpec(t *testing.T) {
	factoryCalled := false
	p := &mockPlugin{
		name:  "factory-plugin",
		tools: []tool.Tool{&mockPluginTool{name: "factory.tool"}},
	}

	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		AgentFactories: []AgentFactorySpec{
			{
				ID: "factory-agent", Name: "Factory Agent", Model: "gpt-4o",
				Factory: func(_ context.Context, _ agent.CreateParams) (agent.AgentDefinition, error) {
					factoryCalled = true
					return agent.AgentDefinition{ID: "factory-agent", Name: "Factory Agent", Model: "gpt-4o"}, nil
				},
				AllowDelegate: true,
			},
		},
	})
	h.Register(p)

	require.NoError(t, h.Build())

	def, err := h.Registry().Get("factory-agent")
	require.NoError(t, err)
	assert.True(t, factoryCalled)
	assert.Equal(t, "Factory Agent", def.Name)
}
