package agent

import (
	"context"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/tool"
)

// mockTool implements tool.Tool interface for testing
type mockTool struct {
	name        string
	description string
}

func (m *mockTool) Name() string                { return m.name }
func (m *mockTool) Description() string         { return m.description }
func (m *mockTool) InputSchema() map[string]any { return map[string]any{} }
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (*tool.ToolResult, error) {
	return nil, nil
}

// mockToolManager is a mock implementation of tool.ToolManager for testing
type mockToolManager struct {
	tools map[string]tool.Tool
}

func (m *mockToolManager) Register(t tool.Tool) error   { return nil }
func (m *mockToolManager) Unregister(name string) error { return nil }
func (m *mockToolManager) Get(name string) (tool.Tool, error) {
	if t, ok := m.tools[name]; ok {
		return t, nil
	}
	return nil, tool.ErrToolNotFound
}
func (m *mockToolManager) List() []tool.ToolInfo {
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
func (m *mockToolManager) Execute(ctx context.Context, name string, args map[string]any) (*tool.ToolResult, error) {
	return nil, nil
}
func (m *mockToolManager) GetOpenAITools() []openai.ChatCompletionToolUnionParam { return nil }

// mockToolRegistry is a mock implementation of tool.ToolRegistry for testing
type mockToolRegistry struct {
	tools map[string]tool.Tool
}

func (r *mockToolRegistry) Register(t tool.Tool) error { return nil }
func (r *mockToolRegistry) Get(name string) (tool.Tool, error) {
	if t, ok := r.tools[name]; ok {
		return t, nil
	}
	return nil, tool.ErrToolNotFound
}
func (r *mockToolRegistry) List() []tool.ToolInfo {
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

func newMockToolRegistry(tools []tool.Tool) tool.ToolRegistry {
	registry := &mockToolRegistry{tools: make(map[string]tool.Tool)}
	for _, t := range tools {
		registry.tools[t.Name()] = t
	}
	return registry
}

func TestAgentRegistryLoad(t *testing.T) {
	// Create mock tools
	bashTool := &mockTool{name: "bash", description: "Bash tool"}
	pythonTool := &mockTool{name: "python", description: "Python tool"}
	toolRegistry := newMockToolRegistry([]tool.Tool{bashTool, pythonTool})

	// Create config with agents
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

	// Create registry
	registry, err := NewAgentRegistry(cfg, toolRegistry)
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Verify agents were loaded
	agents := registry.List()
	assert.Len(t, agents, 2)

	// Check agent info contains expected data
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
	// Create mock tools
	bashTool := &mockTool{name: "bash", description: "Bash tool"}
	toolRegistry := newMockToolRegistry([]tool.Tool{bashTool})

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

	// Test Get existing agent
	agent, err := registry.Get("agent-1")
	require.NoError(t, err)
	assert.Equal(t, "agent-1", agent.ID)
	assert.Equal(t, "Test Agent 1", agent.Name)
	assert.Equal(t, "gpt-4o", agent.Model)
	assert.Equal(t, "You are agent 1.", agent.SystemPrompt)
	assert.NotNil(t, agent.ToolManager)
	assert.NotNil(t, agent.OpenAIClient)

	// Test Get non-existent agent
	_, err = registry.Get("non-existent")
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestAgentRegistryDefault(t *testing.T) {
	bashTool := &mockTool{name: "bash", description: "Bash tool"}
	toolRegistry := newMockToolRegistry([]tool.Tool{bashTool})

	// Test with default agent configured
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

	// Test Default returns configured default
	defaultAgent, err := registry.Default()
	require.NoError(t, err)
	assert.Equal(t, "agent-1", defaultAgent.ID)
	assert.Equal(t, "Test Agent 1", defaultAgent.Name)

	// Test without default agent configured
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
	// Create mock tools - only bash is available
	bashTool := &mockTool{name: "bash", description: "Bash tool"}
	toolRegistry := newMockToolRegistry([]tool.Tool{bashTool})

	// Config with valid tools
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

	// Should succeed with valid tools
	registry, err := NewAgentRegistry(cfgValid, toolRegistry)
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Config with invalid tool
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

	// Should fail with invalid tool
	_, err = NewAgentRegistry(cfgInvalid, toolRegistry)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool not found")

	// Config with mix of valid and invalid tools
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

	// Should fail with invalid tool in mix
	_, err = NewAgentRegistry(cfgMixed, toolRegistry)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-tool")
}

func TestAgentRegistryEmpty(t *testing.T) {
	toolRegistry := newMockToolRegistry([]tool.Tool{})

	// Config with no agents
	cfg := &config.Config{
		Agents: []config.AgentConfig{},
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
		},
	}

	registry, err := NewAgentRegistry(cfg, toolRegistry)
	require.NoError(t, err)
	require.NotNil(t, registry)

	// List should return empty
	agents := registry.List()
	assert.Len(t, agents, 0)

	// Get should return error
	_, err = registry.Get("any-id")
	assert.ErrorIs(t, err, ErrAgentNotFound)

	// Default should return error
	_, err = registry.Default()
	assert.ErrorIs(t, err, ErrNoDefaultAgent)
}

func TestAgentDefinitionStruct(t *testing.T) {
	def := AgentDefinition{
		ID:           "test-agent",
		Name:         "Test Agent",
		Model:        "gpt-4o",
		SystemPrompt: "You are a helpful assistant.",
		ToolManager:  &mockToolManager{},
	}

	assert.Equal(t, "test-agent", def.ID)
	assert.Equal(t, "Test Agent", def.Name)
	assert.Equal(t, "gpt-4o", def.Model)
	assert.Equal(t, "You are a helpful assistant.", def.SystemPrompt)
	assert.NotNil(t, def.ToolManager)
}

func TestAgentInfoEquality(t *testing.T) {
	// Test AgentInfo struct
	info1 := AgentInfo{ID: "test", Name: "Test Agent"}
	info2 := AgentInfo{ID: "test", Name: "Test Agent"}
	info3 := AgentInfo{ID: "other", Name: "Other Agent"}

	assert.Equal(t, info1, info2)
	assert.NotEqual(t, info1, info3)
}
