package core

import (
	"testing"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type integrationStoreProvider struct {
	sessionStore   storage.SessionStore
	messageStore   storage.MessageStore
	todoStore      storage.TodoStore
}

func (p integrationStoreProvider) Sessions() storage.SessionStore   { return p.sessionStore }
func (p integrationStoreProvider) Messages() storage.MessageStore   { return p.messageStore }
func (p integrationStoreProvider) Todos() storage.TodoStore         { return p.todoStore }

func baseHarnessConfig() HarnessConfig {
	return HarnessConfig{
		LLM: llm.NewMockProvider(),
		Store: StoreConfig{
			Provider: integrationStoreProvider{},
		},
		Agents: []AgentSpec{
			{
				ID:            "test-agent",
				Name:          "Test Agent",
				Model:         "gpt-4o",
				SystemPrompt:  "You are a test agent.",
				AllowDelegate: false,
			},
		},
	}
}

type intStubTool struct {
	name string
}

func (t *intStubTool) Name() string                      { return t.name }
func (t *intStubTool) Description() string               { return "stub" }
func (t *intStubTool) InputSchema() map[string]any        { return nil }
func (t *intStubTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	return &tool.ToolResult{Success: true}, nil
}

type intStubHook struct {
	name string
}

func (h *intStubHook) Name() string     { return h.name }
func (h *intStubHook) Points() []hook.HookPoint { return nil }
func (h *intStubHook) Priority() int    { return 100 }
func (h *intStubHook) Execute(ctx *hook.HookContext) error { return nil }

type intStubPlugin struct {
	name  string
	tools []tool.Tool
	hooks []hook.Hook
}

func (p *intStubPlugin) Name() string        { return p.name }
func (p *intStubPlugin) Tools() []tool.Tool  { return p.tools }
func (p *intStubPlugin) Hooks() []hook.Hook  { return p.hooks }
func (p *intStubPlugin) Init(deps plugin.PluginDeps) error { return nil }

func TestHarness_PluginToolsRegistered(t *testing.T) {
	p := &intStubPlugin{
		name: "test-plugin",
		tools: []tool.Tool{
			&intStubTool{name: "memory.tool.memory_store"},
			&intStubTool{name: "memory.tool.memory_recall"},
			&intStubTool{name: "memory.tool.memory_forget"},
		},
	}

	cfg := baseHarnessConfig()
	cfg.Agents[0].Tools = []string{"memory.*"}
	h := NewHarness(cfg)
	h.Register(p)

	require.NoError(t, h.Build())

	def, err := h.Registry().Get("test-agent")
	require.NoError(t, err)

	toolNames := collectToolNames(def.ToolManager.List())
	assert.Contains(t, toolNames, "memory.tool.memory_store")
	assert.Contains(t, toolNames, "memory.tool.memory_recall")
	assert.Contains(t, toolNames, "memory.tool.memory_forget")
}

func TestHarness_PluginHooksRegistered(t *testing.T) {
	p := &intStubPlugin{
		name: "test-plugin",
		hooks: []hook.Hook{
			&intStubHook{name: "knowledge.hook.kb_recall"},
		},
	}

	cfg := baseHarnessConfig()
	h := NewHarness(cfg)
	h.Register(p)

	require.NoError(t, h.Build())

	allHooks := h.HookPool().All()
	assert.Len(t, allHooks, 1)
	assert.Equal(t, "knowledge.hook.kb_recall", allHooks[0].Name())
}

func TestHarness_AgentKBsMap(t *testing.T) {
	p := &intStubPlugin{
		name:  "test-plugin",
		tools: []tool.Tool{&intStubTool{name: "test.tool"}},
	}

	cfg := baseHarnessConfig()
	cfg.Agents = append(cfg.Agents, AgentSpec{
		ID:             "agent-2",
		Name:           "Agent 2",
		Model:          "gpt-4o",
		SystemPrompt:   "second agent",
		KnowledgeBases: []string{"kb-2", "kb-3"},
	})
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	h.Register(p)

	require.NoError(t, h.Build())

	def1, err := h.Registry().Get("test-agent")
	require.NoError(t, err)
	assert.NotNil(t, def1)

	def2, err := h.Registry().Get("agent-2")
	require.NoError(t, err)
	assert.NotNil(t, def2)
}

func collectToolNames(tools []tool.ToolInfo) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}
