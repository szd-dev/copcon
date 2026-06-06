package memoryfile

import (
	"context"
	"testing"

	"github.com/copcon/core/llm"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPlugin_Name(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	p := NewPlugin(store, nil, nil)
	assert.Equal(t, "memory", p.Name())
}

func TestNewPlugin_Tools(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	p := NewPlugin(store, nil, nil)
	tools := p.Tools()

	assert.Len(t, tools, 3)

	expectedNames := []string{
		"memory.tool.memory_store",
		"memory.tool.memory_recall",
		"memory.tool.memory_forget",
	}
	for i, expected := range expectedNames {
		assert.Equal(t, expected, tools[i].Name(), "tool[%d] name mismatch", i)
	}
}

func TestNewPlugin_Hooks_WithoutSummaryLLM(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	p := NewPlugin(store, nil, nil)
	hooks := p.Hooks()

	assert.Len(t, hooks, 3)

	expectedNames := []string{
		"memory.hook.file_memory",
		"memory.hook.memory_recall",
		"memory.hook.fact_extraction",
	}
	for i, expected := range expectedNames {
		assert.Equal(t, expected, hooks[i].Name(), "hook[%d] name mismatch", i)
	}
}

func TestNewPlugin_Hooks_WithSummaryLLM(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	p := NewPlugin(store, nil, &pluginTestLLM{})
	hooks := p.Hooks()

	assert.Len(t, hooks, 4)

	expectedNames := []string{
		"memory.hook.file_memory",
		"memory.hook.memory_recall",
		"memory.hook.fact_extraction",
		"memory.hook.memory_summary",
	}
	for i, expected := range expectedNames {
		assert.Equal(t, expected, hooks[i].Name(), "hook[%d] name mismatch", i)
	}
}

func TestNewPlugin_Init_InjectsMessageStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	p := NewPlugin(store, nil, nil)

	// Hooks() must be called to initialize factHook reference.
	_ = p.Hooks()

	msgStore := &mockMessageStore{}
	err = p.Init(plugin.PluginDeps{MessageStore: msgStore})
	require.NoError(t, err)

	mp := p.(*memoryPlugin)
	assert.Equal(t, storage.MessageStore(msgStore), mp.factHook.messageStore)
}

func TestNewPlugin_Init_NilMessageStore_NoError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	p := NewPlugin(store, nil, nil)
	_ = p.Hooks()

	err = p.Init(plugin.PluginDeps{MessageStore: nil})
	require.NoError(t, err)

	mp := p.(*memoryPlugin)
	assert.Nil(t, mp.factHook.messageStore)
}

func TestNewPlugin_GetStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	p := NewPlugin(store, nil, nil)
	mp := p.(*memoryPlugin)
	assert.Equal(t, store, mp.GetStore())
}

// pluginTestLLM is a minimal stub satisfying llm.LLMProvider for plugin tests.
type pluginTestLLM struct{}

func (m *pluginTestLLM) Stream(_ context.Context, _ llm.StreamParams) (<-chan llm.StreamChunk, <-chan error) {
	ch := make(chan llm.StreamChunk)
	errc := make(chan error)
	close(ch)
	close(errc)
	return ch, errc
}
