package memoryfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/copcon/core/hook"
)

func TestMemorySummaryHook_GeneratesSummary(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileMemoryStore(dir, 0, 0)
	err := EnsureAgentDirs(dir, "agent1")
	assert.NoError(t, err)

	// Use low MaxMemories so 3 files trigger ShouldTrigger
	config := SummarizerConfig{MaxMemories: 2, MaxAgeHours: 999, CooldownMinutes: 0}
	mock := &summarizerMockLLM{response: "# Summary"}
	summarizer := NewFileSummarizer(store, mock, config)

	// Write 3 knowledge files so ShouldTrigger returns true
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, "agent1", "knowledge", fmt.Sprintf("mem-%d.md", i))
		err := WriteFileWithPerms(path, []byte("---\nname: test\n---\ncontent"))
		assert.NoError(t, err)
	}

	h := NewMemorySummaryHook(summarizer)

	sp := "system prompt"
	ctx := &hook.HookContext{
		AgentID:      "agent1",
		SystemPrompt: &sp,
	}

	err = h.Execute(ctx)
	assert.NoError(t, err)

	// Wait for async goroutine to complete
	time.Sleep(200 * time.Millisecond)

	// Check that summary was generated
	systemDir := filepath.Join(dir, "agent1", "system")
	entries, err := os.ReadDir(systemDir)
	assert.NoError(t, err)
	var found bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "summary-") {
			found = true
			break
		}
	}
	assert.True(t, found, "summary file should exist in system/")
}

func TestMemorySummaryHook_NoopWithoutSummarizer(t *testing.T) {
	h := NewMemorySummaryHook(nil)
	sp := "prompt"
	ctx := &hook.HookContext{AgentID: "agent1", SystemPrompt: &sp}
	err := h.Execute(ctx)
	assert.NoError(t, err)
}

func TestMemorySummaryHook_NoopWithEmptyAgentID(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileMemoryStore(dir, 0, 0)

	config := SummarizerConfig{MaxMemories: 2, MaxAgeHours: 999, CooldownMinutes: 0}
	mock := &summarizerMockLLM{response: "# Summary"}
	summarizer := NewFileSummarizer(store, mock, config)

	h := NewMemorySummaryHook(summarizer)
	sp := "prompt"
	ctx := &hook.HookContext{AgentID: "", SystemPrompt: &sp}
	err := h.Execute(ctx)
	assert.NoError(t, err)
}

func TestMemorySummaryHook_Name(t *testing.T) {
	h := NewMemorySummaryHook(nil)
	assert.Equal(t, "memory_summary", h.Name())
}

func TestMemorySummaryHook_Points(t *testing.T) {
	h := NewMemorySummaryHook(nil)
	assert.Equal(t, []hook.HookPoint{hook.OnSystemPrompt}, h.Points())
}

func TestMemorySummaryHook_Priority(t *testing.T) {
	h := NewMemorySummaryHook(nil)
	assert.Equal(t, 90, h.Priority())
}