package memoryfile

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFacts_WithSessions(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-1"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	now := time.Now().Truncate(time.Second)

	// Write a memory file with session_id in frontmatter.
	fm := Frontmatter{
		Name:       "user-prefs",
		Category:   "knowledge",
		SessionID:  "sess-001",
		MessageIDs: []string{"msg-001", "msg-002"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	data := SerializeFrontmatter(fm, "User prefers dark mode")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "knowledge", "user-prefs.md"), data))

	err := BuildFacts(tmpDir, agentID)
	require.NoError(t, err)

	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)

	assert.Contains(t, content, "# Fact Index")
	assert.Contains(t, content, "## session: sess-001")
	assert.Contains(t, content, "user-prefs")
	assert.Contains(t, content, "knowledge/user-prefs.md")
}

func TestBuildFacts_NoSessions(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-2"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	now := time.Now().Truncate(time.Second)

	// Write a memory file WITHOUT session_id.
	fm := Frontmatter{
		Name:      "no-session",
		Category:  "knowledge",
		CreatedAt: now,
		UpdatedAt: now,
	}
	data := SerializeFrontmatter(fm, "Some content")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "knowledge", "no-session.md"), data))

	err := BuildFacts(tmpDir, agentID)
	require.NoError(t, err)

	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)

	assert.Contains(t, content, "# Fact Index")
	assert.Contains(t, content, "No fact entries")
}

func TestBuildFacts_MultipleSessions(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-3"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	now := time.Now().Truncate(time.Second)

	// File for session A.
	fmA := Frontmatter{
		Name:       "prefs-a",
		Category:   "knowledge",
		SessionID:  "sess-alpha",
		MessageIDs: []string{"msg-a1"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	dataA := SerializeFrontmatter(fmA, "Alpha preference")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "knowledge", "prefs-a.md"), dataA))

	// File for session B.
	fmB := Frontmatter{
		Name:       "prefs-b",
		Category:   "knowledge",
		SessionID:  "sess-beta",
		MessageIDs: []string{"msg-b1"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	dataB := SerializeFrontmatter(fmB, "Beta preference")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "knowledge", "prefs-b.md"), dataB))

	err := BuildFacts(tmpDir, agentID)
	require.NoError(t, err)

	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)

	assert.Contains(t, content, "## session: sess-alpha")
	assert.Contains(t, content, "## session: sess-beta")
	assert.Contains(t, content, "prefs-a")
	assert.Contains(t, content, "prefs-b")
}

func TestBuildFacts_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-concurrent"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	now := time.Now().Truncate(time.Second)

	// Pre-create a memory file so BuildFacts has something to work with.
	fm := Frontmatter{
		Name:       "concurrent-fact",
		Category:   "knowledge",
		SessionID:  "sess-conc",
		MessageIDs: []string{"msg-c1"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	data := SerializeFrontmatter(fm, "Concurrent content")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "knowledge", "concurrent-fact.md"), data))

	var wg sync.WaitGroup
	errCount := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := BuildFacts(tmpDir, agentID); err != nil {
				errCount <- err
			}
		}()
	}
	wg.Wait()
	close(errCount)

	for err := range errCount {
		t.Errorf("concurrent BuildFacts failed: %v", err)
	}

	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)
	assert.Contains(t, content, "# Fact Index")
	assert.Contains(t, content, "concurrent-fact")
}

func TestBuildFacts_ArchiveDir(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-archive"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	now := time.Now().Truncate(time.Second)

	// Write to archive/ directory.
	fm := Frontmatter{
		Name:       "archived-fact",
		Category:   "archive",
		SessionID:  "sess-arch",
		MessageIDs: []string{"msg-ar1"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	data := SerializeFrontmatter(fm, "Archived content")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "archive", "archived-fact.md"), data))

	err := BuildFacts(tmpDir, agentID)
	require.NoError(t, err)

	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)
	assert.Contains(t, content, "archive/archived-fact.md")
}

func TestBuildFacts_NoMessageIDs(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-nomsgid"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	now := time.Now().Truncate(time.Second)

	fm := Frontmatter{
		Name:      "fact-no-msgid",
		Category:  "knowledge",
		SessionID: "sess-nomsgid",
		CreatedAt: now,
		UpdatedAt: now,
	}
	data := SerializeFrontmatter(fm, "No message IDs")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "knowledge", "fact-no-msgid.md"), data))

	err := BuildFacts(tmpDir, agentID)
	require.NoError(t, err)

	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)
	// When no messageIDs, the entry should use "- " prefix.
	assert.Contains(t, content, "| - |")
	assert.Contains(t, content, "fact-no-msgid")
}

func TestReadFacts_NonexistentAgent(t *testing.T) {
	tmpDir := t.TempDir()

	content, err := ReadFacts(tmpDir, "nonexistent-agent")
	require.NoError(t, err)
	assert.Equal(t, "", content)
}

func TestFormatFacts_Empty(t *testing.T) {
	result := formatFacts(nil)
	assert.Contains(t, result, "# Fact Index")
	assert.Contains(t, result, "No fact entries")
}

func TestFormatFacts_WithEntries(t *testing.T) {
	entries := []FactEntry{
		{
			SessionID:  "sess-1",
			MessageIDs: []string{"msg-a", "msg-b"},
			Name:       "test-fact",
			RelPath:    "knowledge/test.md",
		},
	}
	result := formatFacts(entries)
	assert.Contains(t, result, "## session: sess-1")
	assert.Contains(t, result, "| msg-a |")
	assert.Contains(t, result, "| msg-b |")
	assert.Contains(t, result, "[test-fact](knowledge/test.md)")
}

func TestBuildFacts_SkipsFactsMD(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-skip"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	// Manually create a FACTS.md in knowledge/ — it should be skipped by scanMDFiles
	// (scanMDFiles only skips INDEX.md, but FACTS.md is in system/ so it won't be scanned anyway).
	// This test verifies FACTS.md does not appear in its own output.
	now := time.Now().Truncate(time.Second)
	fm := Frontmatter{
		Name:       "real-fact",
		Category:   "knowledge",
		SessionID:  "sess-skip",
		MessageIDs: []string{"msg-s1"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	data := SerializeFrontmatter(fm, "Real fact content")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "knowledge", "real-fact.md"), data))

	err := BuildFacts(tmpDir, agentID)
	require.NoError(t, err)

	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)

	// Verify no self-reference to system/FACTS.md.
	assert.NotContains(t, content, "system/FACTS.md")
	// Verify only the real fact is listed.
	factCount := strings.Count(content, "| msg-s1 |")
	assert.Equal(t, 1, factCount)
}

func TestBuildFacts_NonexistentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-nodirs"
	// Don't create agent dirs — BuildFacts should handle gracefully.

	err := BuildFacts(tmpDir, agentID)
	require.NoError(t, err)

	// system/ dir should have been created by WriteFileWithPerms
	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)
	assert.Contains(t, content, "No fact entries")
}

func TestAddToFacts(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-add"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	now := time.Now().Truncate(time.Second)
	fm := Frontmatter{
		Name:       "add-fact",
		Category:   "knowledge",
		SessionID:  "sess-add",
		MessageIDs: []string{"msg-add1"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	data := SerializeFrontmatter(fm, "Add fact content")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "knowledge", "add-fact.md"), data))

	err := AddToFacts(tmpDir, agentID)
	require.NoError(t, err)

	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)
	assert.Contains(t, content, "add-fact")
}

func TestRemoveFromFacts(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-remove"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	now := time.Now().Truncate(time.Second)
	fm := Frontmatter{
		Name:       "rm-fact",
		Category:   "knowledge",
		SessionID:  "sess-rm",
		MessageIDs: []string{"msg-rm1"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	data := SerializeFrontmatter(fm, "Remove fact content")
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, "knowledge", "rm-fact.md"), data))

	// Build first.
	require.NoError(t, BuildFacts(tmpDir, agentID))

	// Delete the file.
	require.NoError(t, os.Remove(filepath.Join(tmpDir, agentID, "knowledge", "rm-fact.md")))

	// RemoveFromFacts is just a rebuild.
	err := RemoveFromFacts(tmpDir, agentID)
	require.NoError(t, err)

	content, err := ReadFacts(tmpDir, agentID)
	require.NoError(t, err)
	assert.Contains(t, content, "No fact entries")
}
