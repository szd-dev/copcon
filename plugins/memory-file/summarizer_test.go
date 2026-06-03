package memoryfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/copcon/core/llm"
)

// summarizerMockLLM is a mock LLM provider for FileSummarizer tests.
type summarizerMockLLM struct {
	response string
	err      error
}

func (m *summarizerMockLLM) Stream(ctx context.Context, params llm.StreamParams) (<-chan llm.StreamChunk, <-chan error) {
	ch := make(chan llm.StreamChunk, 1)
	errc := make(chan error, 1)
	go func() {
		defer close(ch)
		defer close(errc)
		if m.err != nil {
			errc <- m.err
			return
		}
		ch <- llm.StreamChunk{Content: m.response}
	}()
	return ch, errc
}

func TestFileSummarizer_ShouldTrigger_ByCount(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileMemoryStore(dir, 0, 0)
	err := EnsureAgentDirs(dir, "agent1")
	assert.NoError(t, err)

	// Config with low MaxMemories so 3 files trigger it
	config := SummarizerConfig{MaxMemories: 2, MaxAgeHours: 999, CooldownMinutes: 0}
	mock := &summarizerMockLLM{response: "summary"}
	s := NewFileSummarizer(store, mock, config)

	// Write 3 knowledge files
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, "agent1", "knowledge", fmt.Sprintf("mem-%d.md", i))
		err := WriteFileWithPerms(path, []byte("---\nname: test\n---\ncontent"))
		assert.NoError(t, err)
	}

	assert.True(t, s.ShouldTrigger("agent1"))
}

func TestFileSummarizer_ShouldTrigger_ByAge(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileMemoryStore(dir, 0, 0)
	err := EnsureAgentDirs(dir, "agent1")
	assert.NoError(t, err)

	config := SummarizerConfig{MaxMemories: 999, MaxAgeHours: 1, CooldownMinutes: 0}
	mock := &summarizerMockLLM{response: "summary"}
	s := NewFileSummarizer(store, mock, config)

	// Write a file and set mod time to 2 hours ago
	path := filepath.Join(dir, "agent1", "knowledge", "old.md")
	err = WriteFileWithPerms(path, []byte("---\nname: test\n---\ncontent"))
	assert.NoError(t, err)
	oldTime := time.Now().Add(-2 * time.Hour)
	err = os.Chtimes(path, oldTime, oldTime)
	assert.NoError(t, err)

	assert.True(t, s.ShouldTrigger("agent1"))
}

func TestFileSummarizer_Cooldown(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileMemoryStore(dir, 0, 0)
	err := EnsureAgentDirs(dir, "agent1")
	assert.NoError(t, err)

	config := SummarizerConfig{MaxMemories: 0, MaxAgeHours: 0, CooldownMinutes: 60}
	mock := &summarizerMockLLM{response: "summary"}
	s := NewFileSummarizer(store, mock, config)

	// Write .last_summary with current time (cooldown not expired)
	systemDir := filepath.Join(dir, "agent1", "system")
	err = os.MkdirAll(systemDir, 0o700)
	assert.NoError(t, err)
	lastSummaryPath := filepath.Join(systemDir, ".last_summary")
	err = WriteFileWithPerms(lastSummaryPath, []byte(time.Now().Format(time.RFC3339)))
	assert.NoError(t, err)

	// Write knowledge files
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, "agent1", "knowledge", fmt.Sprintf("mem-%d.md", i))
		err := WriteFileWithPerms(path, []byte("---\nname: test\n---\ncontent"))
		assert.NoError(t, err)
	}

	assert.False(t, s.ShouldTrigger("agent1")) // cooldown not expired
}

func TestFileSummarizer_Summarize(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileMemoryStore(dir, 0, 0)
	err := EnsureAgentDirs(dir, "agent1")
	assert.NoError(t, err)

	config := DefaultSummarizerConfig()
	mock := &summarizerMockLLM{response: "# Summary\nThis is a test summary."}
	s := NewFileSummarizer(store, mock, config)

	// Write knowledge files
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, "agent1", "knowledge", fmt.Sprintf("mem-%d.md", i))
		err := WriteFileWithPerms(path, []byte("---\nname: test\n---\ncontent"))
		assert.NoError(t, err)
	}

	err = s.Summarize(context.Background(), "agent1")
	assert.NoError(t, err)

	// Check summary file exists in system/
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

	// Verify .last_summary was written
	data, err := os.ReadFile(filepath.Join(systemDir, ".last_summary"))
	assert.NoError(t, err)
	_, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	assert.NoError(t, parseErr, ".last_summary should contain valid timestamp")
}

func TestFileSummarizer_ShouldTrigger_NilLLM(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileMemoryStore(dir, 0, 0)
	_ = EnsureAgentDirs(dir, "agent1")

	config := DefaultSummarizerConfig()
	s := NewFileSummarizer(store, nil, config)

	assert.False(t, s.ShouldTrigger("agent1"))
}

func TestFileSummarizer_Summarize_NoFiles(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileMemoryStore(dir, 0, 0)
	_ = EnsureAgentDirs(dir, "agent1")

	config := DefaultSummarizerConfig()
	mock := &summarizerMockLLM{response: "summary"}
	s := NewFileSummarizer(store, mock, config)

	// No files written — Summarize should return nil without calling LLM
	err := s.Summarize(context.Background(), "agent1")
	assert.NoError(t, err)
}

func TestFileSummarizer_Summarize_LLMError(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileMemoryStore(dir, 0, 0)
	_ = EnsureAgentDirs(dir, "agent1")

	config := DefaultSummarizerConfig()
	mock := &summarizerMockLLM{err: fmt.Errorf("LLM unavailable")}
	s := NewFileSummarizer(store, mock, config)

	// Write a knowledge file
	path := filepath.Join(dir, "agent1", "knowledge", "test.md")
	_ = WriteFileWithPerms(path, []byte("---\nname: test\n---\ncontent"))

	err := s.Summarize(context.Background(), "agent1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM summary call failed")
}