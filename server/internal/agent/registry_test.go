package agent

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/tool"
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
func (m *registryMockToolManager) GetOpenAITools() []openai.ChatCompletionToolUnionParam { return nil }

type registryMockToolRegistry struct {
	tools map[string]tool.Tool
}

func (r *registryMockToolRegistry) Register(t tool.Tool) error { return nil }
func (r *registryMockToolRegistry) Get(name string) (tool.Tool, error) {
	if t, ok := r.tools[name]; ok {
		return t, nil
	}
	return nil, tool.ErrToolNotFound
}
func (r *registryMockToolRegistry) List() []tool.ToolInfo {
	infos := make([]tool.ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		infos = append(infos, tool.ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return infos
}

func newRegistryMockToolRegistry(tools []tool.Tool) tool.ToolRegistry {
	registry := &registryMockToolRegistry{tools: make(map[string]tool.Tool)}
	for _, t := range tools {
		registry.tools[t.Name()] = t
	}
	return registry
}

func TestAgentRegistryLoad(t *testing.T) {
	bashTool := &registryMockTool{name: "bash", description: "Bash tool"}
	pythonTool := &registryMockTool{name: "python", description: "Python tool"}
	toolRegistry := newRegistryMockToolRegistry([]tool.Tool{bashTool, pythonTool})

	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{
				ID:           "agent-1",
				Name:         "Test Agent 1",
				Model:        "gpt-4o",
				SystemPrompt: "You are agent 1.",
				Tools:        []string{"bash"},
				BaseURL:      "",
			},
			{
				ID:           "agent-2",
				Name:         "Test Agent 2",
				Model:        "gpt-4o-mini",
				SystemPrompt: "You are agent 2.",
				Tools:        []string{"bash", "python"},
				BaseURL:      "https://custom.openai.com",
			},
		},
		DefaultAgentID: "agent-1",
		OpenAI: config.OpenAIConfig{
			APIKey:  "test-key",
			BaseURL: "https://api.openai.com",
			Model:   "gpt-4o",
		},
	}

	registry, err := NewAgentRegistry(cfg, toolRegistry)
	require.NoError(t, err)
	require.NotNil(t, registry)

	agents := registry.List()
	assert.Len(t, agents, 2)

	agentMap := make(map[string]AgentInfo)
	for _, info := range agents {
		agentMap[info.ID] = info
	}

	assert.Contains(t, agentMap, "agent-1")
	assert.Equal(t, "Test Agent 1", agentMap["agent-1"].Name)
	assert.Contains(t, agentMap, "agent-2")
	assert.Equal(t, "Test Agent 2", agentMap["agent-2"].Name)
}

func TestAgentRegistryGet(t *testing.T) {
	bashTool := &registryMockTool{name: "bash", description: "Bash tool"}
	toolRegistry := newRegistryMockToolRegistry([]tool.Tool{bashTool})

	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{
				ID:           "agent-1",
				Name:         "Test Agent 1",
				Model:        "gpt-4o",
				SystemPrompt: "You are agent 1.",
				Tools:        []string{"bash"},
			},
		},
		DefaultAgentID: "agent-1",
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
		},
	}

	registry, err := NewAgentRegistry(cfg, toolRegistry)
	require.NoError(t, err)

	agent, err := registry.Get("agent-1")
	require.NoError(t, err)
	assert.Equal(t, "agent-1", agent.ID)
	assert.Equal(t, "Test Agent 1", agent.Name)
	assert.Equal(t, "gpt-4o", agent.Model)
	assert.Equal(t, "You are agent 1.", agent.SystemPrompt)
	assert.NotNil(t, agent.ToolManager)
	assert.NotNil(t, agent.OpenAIClient)

	_, err = registry.Get("non-existent")
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestAgentRegistryDefault(t *testing.T) {
	bashTool := &registryMockTool{name: "bash", description: "Bash tool"}
	toolRegistry := newRegistryMockToolRegistry([]tool.Tool{bashTool})

	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{
				ID:           "agent-1",
				Name:         "Test Agent 1",
				Model:        "gpt-4o",
				SystemPrompt: "You are agent 1.",
				Tools:        []string{"bash"},
			},
			{
				ID:           "agent-2",
				Name:         "Test Agent 2",
				Model:        "gpt-4o-mini",
				SystemPrompt: "You are agent 2.",
				Tools:        []string{"bash"},
			},
		},
		DefaultAgentID: "agent-1",
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
		},
	}

	registry, err := NewAgentRegistry(cfg, toolRegistry)
	require.NoError(t, err)

	defaultAgent, err := registry.Default()
	require.NoError(t, err)
	assert.Equal(t, "agent-1", defaultAgent.ID)
	assert.Equal(t, "Test Agent 1", defaultAgent.Name)

	cfgNoDefault := &config.Config{
		Agents: []config.AgentConfig{
			{
				ID:           "agent-1",
				Name:         "Test Agent 1",
				Model:        "gpt-4o",
				SystemPrompt: "You are agent 1.",
				Tools:        []string{"bash"},
			},
		},
		DefaultAgentID: "",
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
		},
	}

	registryNoDefault, err := NewAgentRegistry(cfgNoDefault, toolRegistry)
	require.NoError(t, err)

	_, err = registryNoDefault.Default()
	assert.ErrorIs(t, err, ErrNoDefaultAgent)
}

func TestAgentRegistryValidateTools(t *testing.T) {
	bashTool := &registryMockTool{name: "bash", description: "Bash tool"}
	toolRegistry := newRegistryMockToolRegistry([]tool.Tool{bashTool})

	cfgValid := &config.Config{
		Agents: []config.AgentConfig{
			{
				ID:           "agent-1",
				Name:         "Test Agent",
				Model:        "gpt-4o",
				SystemPrompt: "You are a test agent.",
				Tools:        []string{"bash"},
			},
		},
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
		},
	}

	registry, err := NewAgentRegistry(cfgValid, toolRegistry)
	require.NoError(t, err)
	require.NotNil(t, registry)

	cfgInvalid := &config.Config{
		Agents: []config.AgentConfig{
			{
				ID:           "agent-1",
				Name:         "Test Agent",
				Model:        "gpt-4o",
				SystemPrompt: "You are a test agent.",
				Tools:        []string{"nonexistent-tool"},
			},
		},
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
		},
	}

	_, err = NewAgentRegistry(cfgInvalid, toolRegistry)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool not found")

	cfgMixed := &config.Config{
		Agents: []config.AgentConfig{
			{
				ID:           "agent-1",
				Name:         "Test Agent",
				Model:        "gpt-4o",
				SystemPrompt: "You are a test agent.",
				Tools:        []string{"bash", "nonexistent-tool"},
			},
		},
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
		},
	}

	_, err = NewAgentRegistry(cfgMixed, toolRegistry)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-tool")
}

func TestAgentRegistryEmpty(t *testing.T) {
	toolRegistry := newRegistryMockToolRegistry([]tool.Tool{})

	cfg := &config.Config{
		Agents: []config.AgentConfig{},
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
		},
	}

	registry, err := NewAgentRegistry(cfg, toolRegistry)
	require.NoError(t, err)
	require.NotNil(t, registry)

	agents := registry.List()
	assert.Len(t, agents, 0)

	_, err = registry.Get("any-id")
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
