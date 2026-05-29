package core

import (
	"context"
	"os"
	"testing"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/testutil"
	"github.com/copcon/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/copcon/core/capabilities/hooks"
	_ "github.com/copcon/core/capabilities/tools"
)

type integrationStoreProvider struct {
	sessionStore   storage.SessionStore
	messageStore   storage.MessageStore
	todoStore      storage.TodoStore
	knowledgeStore storage.KnowledgeStore
}

func (p integrationStoreProvider) Sessions() storage.SessionStore   { return p.sessionStore }
func (p integrationStoreProvider) Messages() storage.MessageStore   { return p.messageStore }
func (p integrationStoreProvider) Todos() storage.TodoStore         { return p.todoStore }
func (p integrationStoreProvider) Knowledge() storage.KnowledgeStore { return p.knowledgeStore }

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
	cfg.Agents[0].Memory = MemorySpec{
		Enabled:       true,
		BasePath:      tmpDir,
		MaxIndexLines: 100,
		MaxIndexBytes: 10240,
	}

	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	assert.NotNil(t, h.config.Store.FileMemory)

	def, err := h.Registry().Get("test-agent")
	require.NoError(t, err)

	toolNames := collectToolNames(def.ToolManager.List())
	assert.Contains(t, toolNames, "memory_store")
	assert.Contains(t, toolNames, "memory_recall")
	assert.Contains(t, toolNames, "memory_forget")
}

func TestHarness_KBOnly(t *testing.T) {
	cfg := baseHarnessConfig()
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	assert.NotNil(t, h.config.Store.KnowledgeStore)

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
	cfg.Agents[0].Memory = MemorySpec{
		Enabled:       true,
		BasePath:      tmpDir,
		MaxIndexLines: 100,
		MaxIndexBytes: 10240,
	}
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	assert.NotNil(t, h.config.Store.FileMemory)
	assert.NotNil(t, h.config.Store.KnowledgeStore)

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

	assert.Contains(t, names, "hooks.file_memory")
	assert.Contains(t, names, "tools.memory_store")
	assert.Contains(t, names, "tools.memory_recall")
	assert.Contains(t, names, "tools.memory_forget")
	assert.NotContains(t, names, "hooks.kb_recall")
	assert.NotContains(t, names, "hooks.memory_persist")
}

func TestHarness_CollectCapabilityNames_KBBundle(t *testing.T) {
	cfg := baseHarnessConfig()
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	names := h.collectCapabilityNames()

	assert.Contains(t, names, "hooks.kb_recall")
	assert.Contains(t, names, "hooks.memory_persist")
	assert.NotContains(t, names, "hooks.file_memory")
	assert.NotContains(t, names, "tools.memory_store")
}

func TestHarness_CollectCapabilityNames_BothBundles(t *testing.T) {
	cfg := baseHarnessConfig()
	cfg.Agents[0].Memory = MemorySpec{Enabled: true}
	cfg.Agents[0].KnowledgeBases = []string{"kb-1"}

	h := NewHarness(cfg)
	names := h.collectCapabilityNames()

	assert.Contains(t, names, "hooks.file_memory")
	assert.Contains(t, names, "tools.memory_store")
	assert.Contains(t, names, "tools.memory_recall")
	assert.Contains(t, names, "tools.memory_forget")
	assert.Contains(t, names, "hooks.kb_recall")
	assert.Contains(t, names, "hooks.memory_persist")
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

func TestHarness_FileMemoryHookInjection(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := baseHarnessConfig()
	cfg.Agents[0].Memory = MemorySpec{
		Enabled:       true,
		BasePath:      tmpDir,
		MaxIndexLines: 100,
		MaxIndexBytes: 10240,
	}

	h := NewHarness(cfg)
	require.NoError(t, h.Build())

	agentDir := tmpDir + "/test-agent/system"
	require.NoError(t, os.MkdirAll(agentDir, 0o700))
	require.NoError(t, os.WriteFile(agentDir+"/guidelines.md", []byte("Always be helpful and concise."), 0o600))

	chatCtx := testutil.NewMockChatContext(context.Background(), "test-session", "test-agent")
	prompt := "You are a test agent."
	hookCtx := &hook.HookContext{
		ChatCtx:      chatCtx,
		SessionID:    "test-session",
		AgentID:      "test-agent",
		SystemPrompt: &prompt,
		CurrentPoint: hook.OnSystemPrompt,
		Logger:       h.config.Logger,
	}
	h.HookRunner().Run(hook.OnSystemPrompt, hookCtx)

	assert.Contains(t, prompt, "Agent Memory")
	assert.Contains(t, prompt, "guidelines.md")
}

func TestHarness_ErrDependencyUnavailable(t *testing.T) {
	assert.Equal(t, "dependency unavailable", capabilities.ErrDependencyUnavailable.Error())
}

func collectToolNames(tools []tool.ToolInfo) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}
