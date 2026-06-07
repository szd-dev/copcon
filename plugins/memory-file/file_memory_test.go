package memoryfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/hook"
)

type mockFileMemoryStore struct {
	basePath string
}

func (m *mockFileMemoryStore) BasePath() string { return m.basePath }

func TestFileMemoryHook_Metadata(t *testing.T) {
	h := NewFileMemoryHook(&mockFileMemoryStore{basePath: t.TempDir()})
	assert.Equal(t, "file_memory", h.Name())
	assert.Equal(t, []hook.HookPoint{hook.OnSystemPrompt}, h.Points())
	assert.Equal(t, 80, h.Priority())
}

func TestFileMemoryHook_InjectsSystemFiles(t *testing.T) {
	tmpDir := t.TempDir()
	agentDir := filepath.Join(tmpDir, "agent-1", "system")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "guidelines.md"), []byte("Be helpful"), 0o644))

	h := NewFileMemoryHook(&mockFileMemoryStore{basePath: tmpDir})

	prompt := "You are an assistant."
	ctx := &hook.HookContext{
		AgentID:      "agent-1",
		SystemPrompt: &prompt,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, *ctx.SystemPrompt, "Agent Memory")
	assert.Contains(t, *ctx.SystemPrompt, "guidelines.md")
	assert.Contains(t, *ctx.SystemPrompt, "Be helpful")
	assert.Contains(t, *ctx.SystemPrompt, "Memory Protocol")
}

func TestFileMemoryHook_InjectsIndex(t *testing.T) {
	tmpDir := t.TempDir()
	agentDir := filepath.Join(tmpDir, "agent-1", "system")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "INDEX.md"), []byte("# Memory Index\n- test entry"), 0o644))

	h := NewFileMemoryHook(&mockFileMemoryStore{basePath: tmpDir})

	prompt := "You are an assistant."
	ctx := &hook.HookContext{
		AgentID:      "agent-1",
		SystemPrompt: &prompt,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, *ctx.SystemPrompt, "Memory Index")
	assert.Contains(t, *ctx.SystemPrompt, "test entry")
}

func TestFileMemoryHook_SkipsWhenNoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	h := NewFileMemoryHook(&mockFileMemoryStore{basePath: tmpDir})

	prompt := "You are an assistant."
	ctx := &hook.HookContext{
		AgentID:      "agent-1",
		SystemPrompt: &prompt,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "You are an assistant.", *ctx.SystemPrompt)
}

func TestFileMemoryHook_SkipsWhenNilPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewFileMemoryHook(&mockFileMemoryStore{basePath: tmpDir})

	ctx := &hook.HookContext{
		AgentID:      "agent-1",
		SystemPrompt: nil,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
}

func TestFileMemoryHook_SkipsWhenEmptyAgentID(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewFileMemoryHook(&mockFileMemoryStore{basePath: tmpDir})

	prompt := "You are an assistant."
	ctx := &hook.HookContext{
		AgentID:      "",
		SystemPrompt: &prompt,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "You are an assistant.", *ctx.SystemPrompt)
}

func TestFileMemoryHook_DoesNotInjectINDEXAsSystemFile(t *testing.T) {
	tmpDir := t.TempDir()
	agentDir := filepath.Join(tmpDir, "agent-1", "system")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "INDEX.md"), []byte("# Memory Index"), 0o644))

	h := NewFileMemoryHook(&mockFileMemoryStore{basePath: tmpDir})

	prompt := "You are an assistant."
	ctx := &hook.HookContext{
		AgentID:      "agent-1",
		SystemPrompt: &prompt,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, *ctx.SystemPrompt, "Memory Index")
	assert.NotContains(t, *ctx.SystemPrompt, "System Context")
}
