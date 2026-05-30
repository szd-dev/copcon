package core

import (
	"testing"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/capabilities/hooks"
	"github.com/copcon/core/capabilities/tools"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/llm"
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

type stubHookCap struct {
	name string
}

func (c *stubHookCap) Name() string                         { return c.name }
func (c *stubHookCap) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeHook }
func (c *stubHookCap) DependsOn() []string                  { return nil }
func (c *stubHookCap) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return &stubHook{name: c.name}, nil
}

type stubHook struct {
	name string
}

func (h *stubHook) Name() string     { return h.name }
func (h *stubHook) Points() []hook.HookPoint { return nil }
func (h *stubHook) Priority() int    { return 100 }
func (h *stubHook) Execute(ctx *hook.HookContext) error { return nil }

type stubToolCap struct {
	capName  string
	toolName string
}

func (c *stubToolCap) Name() string                         { return c.capName }
func (c *stubToolCap) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeTool }
func (c *stubToolCap) DependsOn() []string                  { return nil }
func (c *stubToolCap) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	return &stubTool{name: c.toolName}, nil
}

type stubTool struct {
	name string
}

func (t *stubTool) Name() string                      { return t.name }
func (t *stubTool) Description() string               { return "stub" }
func (t *stubTool) InputSchema() map[string]any        { return nil }
func (t *stubTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	return &tool.ToolResult{Success: true}, nil
}

func newRegistryWithPlugins() *capabilities.Registry {
	r := capabilities.NewRegistry()
	hooks.RegisterAll(r)
	tools.RegisterAll(r)

	r.Register(&stubHookCap{name: capabilities.HookMemory})
	r.Register(&stubHookCap{name: capabilities.HookFileMemory})
	r.Register(&stubToolCap{capName: capabilities.ToolMemoryStore, toolName: "memory_store"})
	r.Register(&stubToolCap{capName: capabilities.ToolMemoryRecall, toolName: "memory_recall"})
	r.Register(&stubToolCap{capName: capabilities.ToolMemoryForget, toolName: "memory_forget"})
	r.Register(&stubHookCap{name: capabilities.HookKBRecall})
	r.Register(&stubHookCap{name: capabilities.HookMemoryPersist})

	return r
}

func TestHarness_NoneEnabled(t *testing.T) {
	cfg := baseHarnessConfig()
	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	def, err := h.Registry().Get("test-agent")
	require.NoError(t, err)

	toolNames := collectToolNames(def.ToolManager.List())
	assert.Contains(t, toolNames, "confirm_action")
	assert.Contains(t, toolNames, "ask_user")
	assert.NotContains(t, toolNames, "memory_store")
	assert.NotContains(t, toolNames, "memory_recall")
	assert.NotContains(t, toolNames, "memory_forget")
}

func TestHarness_MemoryOnly(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := baseHarnessConfig()
	cfg.Registry = newRegistryWithPlugins()
	cfg.Agents[0].Memory = MemorySpec{
		Enabled:       true,
		BasePath:      tmpDir,
		MaxIndexLines: 100,
		MaxIndexBytes: 10240,
	}

	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	def, err := h.Registry().Get("test-agent")
	require.NoError(t, err)

	toolNames := collectToolNames(def.ToolManager.List())
	assert.Contains(t, toolNames, "memory_store")
	assert.Contains(t, toolNames, "memory_recall")
	assert.Contains(t, toolNames, "memory_forget")
}

func TestHarness_KBOnly(t *testing.T) {
	cfg := baseHarnessConfig()
	cfg.Registry = newRegistryWithPlugins()
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	def, err := h.Registry().Get("test-agent")
	require.NoError(t, err)

	toolNames := collectToolNames(def.ToolManager.List())
	assert.NotContains(t, toolNames, "memory_store")
	assert.NotContains(t, toolNames, "memory_recall")
	assert.NotContains(t, toolNames, "memory_forget")
}

func TestHarness_BothEnabled(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := baseHarnessConfig()
	cfg.Registry = newRegistryWithPlugins()
	cfg.Agents[0].Memory = MemorySpec{
		Enabled:       true,
		BasePath:      tmpDir,
		MaxIndexLines: 100,
		MaxIndexBytes: 10240,
	}
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	def, err := h.Registry().Get("test-agent")
	require.NoError(t, err)

	toolNames := collectToolNames(def.ToolManager.List())
	assert.Contains(t, toolNames, "memory_store")
	assert.Contains(t, toolNames, "memory_recall")
	assert.Contains(t, toolNames, "memory_forget")
}

func TestHarness_CollectCapabilityNames_MemoryBundle(t *testing.T) {
	cfg := baseHarnessConfig()
	cfg.Agents[0].Memory = MemorySpec{Enabled: true}

	h := NewHarness(cfg)
	names := h.collectCapabilityNames()

	assert.Contains(t, names, capabilities.HookMemory)
	assert.Contains(t, names, capabilities.HookFileMemory)
	assert.Contains(t, names, capabilities.ToolMemoryStore)
	assert.Contains(t, names, capabilities.ToolMemoryRecall)
	assert.Contains(t, names, capabilities.ToolMemoryForget)
	assert.NotContains(t, names, capabilities.HookKBRecall)
	assert.NotContains(t, names, capabilities.HookMemoryPersist)
}

func TestHarness_CollectCapabilityNames_KBBundle(t *testing.T) {
	cfg := baseHarnessConfig()
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	names := h.collectCapabilityNames()

	assert.Contains(t, names, capabilities.HookKBRecall)
	assert.Contains(t, names, capabilities.HookMemoryPersist)
	assert.NotContains(t, names, capabilities.HookFileMemory)
	assert.NotContains(t, names, capabilities.ToolMemoryStore)
}

func TestHarness_CollectCapabilityNames_BothBundles(t *testing.T) {
	cfg := baseHarnessConfig()
	cfg.Agents[0].Memory = MemorySpec{Enabled: true}
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	names := h.collectCapabilityNames()

	assert.Contains(t, names, capabilities.HookMemory)
	assert.Contains(t, names, capabilities.HookFileMemory)
	assert.Contains(t, names, capabilities.ToolMemoryStore)
	assert.Contains(t, names, capabilities.ToolMemoryRecall)
	assert.Contains(t, names, capabilities.ToolMemoryForget)
	assert.Contains(t, names, capabilities.HookKBRecall)
	assert.Contains(t, names, capabilities.HookMemoryPersist)
}

func TestHarness_CollectCapabilityNames_Deduplication(t *testing.T) {
	cfg := baseHarnessConfig()
	cfg.Agents = append(cfg.Agents, AgentSpec{
		ID:             "agent-2",
		Name:           "Agent 2",
		Model:          "gpt-4o",
		SystemPrompt:   "second agent",
		Memory:         MemorySpec{Enabled: true},
		KnowledgeBases: []string{"kb-2"},
	})
	cfg.Agents[0].Memory = MemorySpec{Enabled: true}
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	names := h.collectCapabilityNames()

	counts := make(map[string]int)
	for _, n := range names {
		counts[n]++
	}

	for _, bundleName := range capabilities.MemoryBundleNames() {
		assert.Equal(t, 1, counts[bundleName], "bundle name %q should appear exactly once", bundleName)
	}
	for _, bundleName := range capabilities.KnowledgeBaseBundleNames() {
		assert.Equal(t, 1, counts[bundleName], "bundle name %q should appear exactly once", bundleName)
	}
}

func TestHarness_AgentKBsMap(t *testing.T) {
	cfg := baseHarnessConfig()
	cfg.Registry = newRegistryWithPlugins()
	cfg.Agents = append(cfg.Agents, AgentSpec{
		ID:             "agent-2",
		Name:           "Agent 2",
		Model:          "gpt-4o",
		SystemPrompt:   "second agent",
		KnowledgeBases: []string{"kb-2", "kb-3"},
	})
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	def1, err := h.Registry().Get("test-agent")
	require.NoError(t, err)
	assert.NotNil(t, def1)

	def2, err := h.Registry().Get("agent-2")
	require.NoError(t, err)
	assert.NotNil(t, def2)
}

func TestHarness_SkipHookOnMissingDependency(t *testing.T) {
	cfg := baseHarnessConfig()
	h := NewHarness(cfg)
	require.NoError(t, h.Build())
	assert.True(t, h.built)
}

func TestHarness_ErrDependencyUnavailable(t *testing.T) {
	assert.Equal(t, "dependency unavailable", capabilities.ErrDependencyUnavailable.Error())
}

// TestHarness_RegistryAutoCreated verifies that Build() auto-creates a
// Registry when HarnessConfig.Registry is nil, and that core capabilities
// are registered automatically.
func TestHarness_RegistryAutoCreated(t *testing.T) {
	cfg := baseHarnessConfig()
	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	r := h.CapRegistry()
	require.NotNil(t, r)

	_, ok := r.Get(capabilities.HookLogging)
	assert.True(t, ok, "core hook should be auto-registered")

	_, ok = r.Get(capabilities.ToolAskUser)
	assert.True(t, ok, "core tool should be auto-registered")
}

// TestHarness_CustomRegistry verifies that a caller-provided Registry is
// used by Build() without modification (no duplicate registration).
func TestHarness_CustomRegistry(t *testing.T) {
	r := capabilities.NewRegistry()
	hooks.RegisterAll(r)
	tools.RegisterAll(r)

	cfg := baseHarnessConfig()
	cfg.Registry = r
	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	assert.Equal(t, r, h.CapRegistry(), "should use the provided registry")
}

func collectToolNames(tools []tool.ToolInfo) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}
