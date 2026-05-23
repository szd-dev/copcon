package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/tool"
)

type registryMockTool struct {
	name        string
	description string
}

func (m *registryMockTool) Name() string                { return m.name }
func (m *registryMockTool) Description() string         { return m.description }
func (m *registryMockTool) InputSchema() map[string]any { return map[string]any{} }
func (m *registryMockTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	return nil, nil
}

type registryMockToolManager struct {
	tools map[string]tool.Tool
}

func (m *registryMockToolManager) Register(t tool.Tool) error   { return nil }
func (m *registryMockToolManager) Unregister(name string) error { return nil }
func (m *registryMockToolManager) Get(name string) (tool.Tool, error) {
	if t, ok := m.tools[name]; ok {
		return t, nil
	}
	return nil, tool.ErrToolNotFound
}
func (m *registryMockToolManager) List() []tool.ToolInfo {
	infos := make([]tool.ToolInfo, 0, len(m.tools))
	for _, t := range m.tools {
		infos = append(infos, tool.ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return infos
}
func (m *registryMockToolManager) Execute(chatCtx iface.ChatContextInterface, name string, args map[string]any) (*tool.ToolResult, error) {
	return nil, nil
}
func (m *registryMockToolManager) GetToolDefs() []llm.ToolDef { return nil }

func TestNewAgentRegistryCreatesEmptyRegistry(t *testing.T) {
	registry := NewAgentRegistry("default-agent")
	require.NotNil(t, registry)

	agents := registry.List()
	assert.Len(t, agents, 0)
}

func TestAgentRegistryWithFactoryRegistration(t *testing.T) {
	registry := NewAgentRegistry("agent-1")
	require.NotNil(t, registry)

	factory := func(ctx context.Context, params CreateParams) (AgentDefinition, error) {
		return AgentDefinition{
			ID:           "agent-1",
			Name:         "Test Agent 1",
			Model:        "gpt-4o",
			SystemPrompt: "You are agent 1.",
			ToolManager:  &registryMockToolManager{tools: map[string]tool.Tool{}},
		}, nil
	}

	registry.RegisterFactory("agent-1", "Test Agent 1", "gpt-4o", true, factory)

	agents := registry.List()
	assert.Len(t, agents, 1)
	assert.Equal(t, "agent-1", agents[0].ID)
	assert.Equal(t, "Test Agent 1", agents[0].Name)
	assert.Equal(t, "gpt-4o", agents[0].Model)
}

func TestAgentRegistryGetWithFactory(t *testing.T) {
	registry := NewAgentRegistry("agent-1")
	require.NotNil(t, registry)

	factory := func(ctx context.Context, params CreateParams) (AgentDefinition, error) {
		return AgentDefinition{
			ID:           "agent-1",
			Name:         "Test Agent 1",
			Model:        "gpt-4o",
			SystemPrompt: "You are agent 1.",
			ToolManager:  &registryMockToolManager{tools: map[string]tool.Tool{}},
		}, nil
	}

	registry.RegisterFactory("agent-1", "Test Agent 1", "gpt-4o", false, factory)

	agentDef, err := registry.Get("agent-1")
	require.NoError(t, err)
	assert.Equal(t, "agent-1", agentDef.ID)
	assert.Equal(t, "Test Agent 1", agentDef.Name)
	assert.Equal(t, "gpt-4o", agentDef.Model)
	assert.Equal(t, "You are agent 1.", agentDef.SystemPrompt)

	_, err = registry.Get("non-existent")
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestAgentRegistryDefault(t *testing.T) {
	registry := NewAgentRegistry("agent-1")
	require.NotNil(t, registry)

	factory1 := func(ctx context.Context, params CreateParams) (AgentDefinition, error) {
		return AgentDefinition{
			ID:           "agent-1",
			Name:         "Test Agent 1",
			Model:        "gpt-4o",
			SystemPrompt: "You are agent 1.",
			ToolManager:  &registryMockToolManager{tools: map[string]tool.Tool{}},
		}, nil
	}

	factory2 := func(ctx context.Context, params CreateParams) (AgentDefinition, error) {
		return AgentDefinition{
			ID:           "agent-2",
			Name:         "Test Agent 2",
			Model:        "gpt-4o-mini",
			SystemPrompt: "You are agent 2.",
			ToolManager:  &registryMockToolManager{tools: map[string]tool.Tool{}},
		}, nil
	}

	registry.RegisterFactory("agent-1", "Test Agent 1", "gpt-4o", true, factory1)
	registry.RegisterFactory("agent-2", "Test Agent 2", "gpt-4o-mini", false, factory2)

	defaultAgent, err := registry.Default()
	require.NoError(t, err)
	assert.Equal(t, "agent-1", defaultAgent.ID)
	assert.Equal(t, "Test Agent 1", defaultAgent.Name)

	registryNoDefault := NewAgentRegistry("")
	registryNoDefault.RegisterFactory("agent-1", "Test Agent 1", "gpt-4o", true, factory1)

	_, err = registryNoDefault.Default()
	assert.ErrorIs(t, err, ErrNoDefaultAgent)
}

func TestAgentRegistryEmpty(t *testing.T) {
	registry := NewAgentRegistry("")

	agents := registry.List()
	assert.Len(t, agents, 0)

	_, err := registry.Get("any-id")
	assert.ErrorIs(t, err, ErrAgentNotFound)

	_, err = registry.Default()
	assert.ErrorIs(t, err, ErrNoDefaultAgent)
}

func TestAgentDefinitionStruct(t *testing.T) {
	def := AgentDefinition{
		ID:           "test-agent",
		Name:         "Test Agent",
		Model:        "gpt-4o",
		SystemPrompt: "You are a helpful assistant.",
		ToolManager:  &registryMockToolManager{},
	}

	assert.Equal(t, "test-agent", def.ID)
	assert.Equal(t, "Test Agent", def.Name)
	assert.Equal(t, "gpt-4o", def.Model)
	assert.Equal(t, "You are a helpful assistant.", def.SystemPrompt)
	assert.NotNil(t, def.ToolManager)
}

func TestAgentInfoEquality(t *testing.T) {
	info1 := AgentInfo{ID: "test", Name: "Test Agent"}
	info2 := AgentInfo{ID: "test", Name: "Test Agent"}
	info3 := AgentInfo{ID: "other", Name: "Other Agent"}

	assert.Equal(t, info1, info2)
	assert.NotEqual(t, info1, info3)
}

func TestFactoryRegistry(t *testing.T) {
	t.Run("register factory and create with task", func(t *testing.T) {
		reg := &agentRegistry{
			factories: make(map[string]factoryEntry),
		}

		factory := func(ctx context.Context, params CreateParams) (AgentDefinition, error) {
			sp := "You are a helpful assistant."
			if params.Task != "" {
				sp += "\n\nCurrent Task: " + params.Task
			}
			return AgentDefinition{
				ID:           "test-agent",
				Name:         "Test Agent",
				Model:        "gpt-4o",
				SystemPrompt: sp,
				ToolManager:  &registryMockToolManager{tools: map[string]tool.Tool{}},
			}, nil
		}

		reg.RegisterFactory("test-agent", "Test Agent", "gpt-4o", true, factory)

		f, err := reg.GetFactory("test-agent")
		require.NoError(t, err)
		require.NotNil(t, f)

		def, err := f(context.Background(), CreateParams{Task: "Solve this problem"})
		require.NoError(t, err)
		assert.Contains(t, def.SystemPrompt, "Solve this problem")
		assert.Contains(t, def.SystemPrompt, "Current Task:")

		defNoTask, err := f(context.Background(), CreateParams{})
		require.NoError(t, err)
		assert.NotContains(t, defNoTask.SystemPrompt, "Current Task:")
	})

	t.Run("GetFactory for unregistered agent returns error", func(t *testing.T) {
		reg := &agentRegistry{
			factories: make(map[string]factoryEntry),
		}

		f, err := reg.GetFactory("nonexistent")
		assert.ErrorIs(t, err, ErrAgentNotFound)
		assert.Nil(t, f)
	})

	t.Run("ListDelegatable only returns allowDelegate agents", func(t *testing.T) {
		reg := &agentRegistry{
			factories: make(map[string]factoryEntry),
		}

		noopFactory := func(ctx context.Context, params CreateParams) (AgentDefinition, error) {
			return AgentDefinition{}, nil
		}

		reg.RegisterFactory("agent-a", "Agent A", "gpt-4o", true, noopFactory)
		reg.RegisterFactory("agent-b", "Agent B", "gpt-4o-mini", false, noopFactory)
		reg.RegisterFactory("agent-c", "Agent C", "gpt-3.5-turbo", true, noopFactory)

		delegatable := reg.ListDelegatable()
		require.Len(t, delegatable, 2)

		ids := make(map[string]bool)
		for _, info := range delegatable {
			ids[info.ID] = true
		}
		assert.True(t, ids["agent-a"])
		assert.False(t, ids["agent-b"])
		assert.True(t, ids["agent-c"])
	})
}