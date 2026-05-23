package core

import (
	"context"
	"testing"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/copcon/core/capabilities/hooks"
	_ "github.com/copcon/core/capabilities/tools"
)

type testStoreProvider struct{}

func (testStoreProvider) Sessions() storage.SessionStore { return nil }
func (testStoreProvider) Messages() storage.MessageStore { return nil }
func (testStoreProvider) Todos() storage.TodoStore       { return nil }

func TestNewHarness_BasicBuild(t *testing.T) {
	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{
				ID:            "test-agent",
				Name:          "Test Agent",
				Model:         "gpt-4o",
				SystemPrompt:  "You are a test agent.",
				Tools:         []string{"tools.code_executor"},
				AllowDelegate: false,
			},
		},
	})

	err := h.Build()
	require.NoError(t, err)

	assert.NotNil(t, h.Engine())
	assert.NotNil(t, h.Registry())
	assert.True(t, h.built)
}

func TestNewHarness_DoubleBuild(t *testing.T) {
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

	require.NoError(t, h.Build())
	err := h.Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already built")
}

func TestNewHarness_NilProviderReturnsError(t *testing.T) {
	h := NewHarness(HarnessConfig{
		LLM: llm.NewMockProvider(),
		Agents: []AgentSpec{{ID: "a", Name: "A", Model: "gpt-4o", SystemPrompt: "test"}},
	})
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

	require.NoError(t, h.Build())

	def, err := h.Registry().Get("factory-agent")
	require.NoError(t, err)
	assert.True(t, factoryCalled)
	assert.Equal(t, "Factory Agent", def.Name)
}

func TestNewHarness_FirstAgentIsDefault(t *testing.T) {
	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{ID: "first", Name: "First", Model: "gpt-4o", SystemPrompt: "first"},
			{ID: "second", Name: "Second", Model: "gpt-4o", SystemPrompt: "second"},
		},
	})

	require.NoError(t, h.Build())

	def, err := h.Registry().Default()
	require.NoError(t, err)
	assert.Equal(t, "first", def.ID)
}

func TestNewHarness_DefaultFromFactorySpec(t *testing.T) {
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

	require.NoError(t, h.Build())

	def, err := h.Registry().Default()
	require.NoError(t, err)
	assert.Equal(t, "factory-default", def.ID)
}

func TestNewHarness_WildcardCapabilityExpansion(t *testing.T) {
	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{ID: "wildcard-agent", Name: "Wildcard Agent", Model: "gpt-4o", SystemPrompt: "test",
				Tools: []string{"code_executor"}, AllowDelegate: false},
		},
	})

	require.NoError(t, h.Build())

	def, err := h.Registry().Get("wildcard-agent")
	require.NoError(t, err)
	assert.NotNil(t, def.ToolManager)

	tools := def.ToolManager.List()
	assert.NotEmpty(t, tools, "wildcard tools.* should expand and register tools")
}

func TestNewHarness_CapabilityDependencyResolution(t *testing.T) {
	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{
				ID:           "dep-agent",
				Name:         "Dep Agent",
				Model:        "gpt-4o",
				SystemPrompt: "test",
				Tools:        []string{"tools.todo"},
			},
		},
	})

	require.NoError(t, h.Build())

	def, err := h.Registry().Get("dep-agent")
	require.NoError(t, err)

	tools := def.ToolManager.List()
	found := false
	for _, ti := range tools {
		if ti.Name == "todolist" {
			found = true
			break
		}
	}
	assert.True(t, found, "tools.todo dependency should pull in todolist tool")
}

func TestNewAgent_QuickConfig(t *testing.T) {
	engine, registry, err := NewAgent(AgentQuickConfig{
		Name:         "Quick Agent",
		Model:        "gpt-4o",
		SystemPrompt: "You are a quick agent.",
		Tools:        []string{"tools.code_executor"},
		LLM:          llm.NewMockProvider(),
	})

	require.NoError(t, err)
	assert.NotNil(t, engine)
	assert.NotNil(t, registry)

	def, err := registry.Default()
	require.NoError(t, err)
	assert.Equal(t, "default", def.ID)
	assert.Equal(t, "Quick Agent", def.Name)
	assert.Equal(t, "gpt-4o", def.Model)
}

func TestNewHarness_UnknownCapability(t *testing.T) {
	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{
				ID:           "bad-agent",
				Name:         "Bad Agent",
				Model:        "gpt-4o",
				SystemPrompt: "test",
				Tools:        []string{"tools.nonexistent_capability"},
			},
		},
	})

	err := h.Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestNewHarness_AgentSpecModelOverride(t *testing.T) {
	h := NewHarness(HarnessConfig{
		LLM:   llm.NewMockProvider(),
		Store: StoreConfig{Provider: testStoreProvider{}},
		Agents: []AgentSpec{
			{
				ID:            "override-agent",
				Name:          "Override Agent",
				Model:         "gpt-4o",
				SystemPrompt:  "test",
				Tools:         []string{"tools.code_executor"},
				AllowDelegate: true,
			},
		},
	})

	require.NoError(t, h.Build())

	def, err := h.Registry().Get("override-agent")
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", def.Model)
	assert.Equal(t, "test", def.SystemPrompt)
}
