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
	"github.com/stretchr/testify/require"
)

func TestNewFileMemoryStore(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, store.BasePath())
}

func TestNewFileMemoryStore_DefaultLimits(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := NewFileMemoryStore(tmpDir, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, defaultMaxIndexLines, store.maxIndexLines)
	assert.Equal(t, defaultMaxIndexBytes, store.maxIndexBytes)
}

func TestFileMemoryStore_StoreAndGetBySession(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	mem := &Memory{
		Content:    "test content",
		SessionID:  "agent-1",
		Role:       "assistant",
		MemoryType: string(MemoryTypeEpisodic),
	}

	err = store.Store(ctx, mem)
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)

	results, err := store.GetBySession(ctx, "agent-1", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "test content", results[0].Content)
}

func TestFileMemoryStore_Store_NoOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	mem := &Memory{
		ID:         "fixed-id",
		Content:    "original",
		SessionID:  "agent-1",
		MemoryType: string(MemoryTypeEpisodic),
	}

	err = store.Store(ctx, mem)
	require.NoError(t, err)

	mem.Content = "modified"
	err = store.Store(ctx, mem)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestFileMemoryStore_Update(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	mem := &Memory{
		Content:    "original content",
		SessionID:  "agent-1",
		Role:       "assistant",
		MemoryType: string(MemoryTypeEpisodic),
	}

	err = store.Store(ctx, mem)
	require.NoError(t, err)

	mem.Content = "updated content"
	err = store.Update(ctx, mem)
	require.NoError(t, err)

	results, err := store.GetBySession(ctx, "agent-1", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Content, "updated content")
}

func TestFileMemoryStore_DeleteBySession(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	mem := &Memory{
		Content:    "to be deleted",
		SessionID:  "agent-1",
		MemoryType: string(MemoryTypeEpisodic),
	}

	err = store.Store(ctx, mem)
	require.NoError(t, err)

	err = store.DeleteBySession(ctx, "agent-1")
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, "agent-1"))
	assert.True(t, os.IsNotExist(err))
}

func TestFileMemoryStore_Search_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	_, err = store.Search(context.Background(), []float32{0.1}, 5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vector search")
}

func TestFileMemoryStore_WriteFileAndReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	err = store.WriteFile(ctx, "agent-1", "knowledge/test-file.md", "hello world", nil)
	require.NoError(t, err)

	content, err := store.ReadFile(ctx, "agent-1", "knowledge/test-file.md")
	require.NoError(t, err)
	assert.Contains(t, content, "hello world")
}

func TestFileMemoryStore_WriteFile_NoOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	err = store.WriteFile(ctx, "agent-1", "knowledge/test.md", "first", nil)
	require.NoError(t, err)

	err = store.WriteFile(ctx, "agent-1", "knowledge/test.md", "second", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestFileMemoryStore_DeleteFile(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	err = store.WriteFile(ctx, "agent-1", "knowledge/del.md", "delete me", nil)
	require.NoError(t, err)

	err = store.DeleteFile(ctx, "agent-1", "knowledge/del.md")
	require.NoError(t, err)

	_, err = store.ReadFile(ctx, "agent-1", "knowledge/del.md")
	assert.Error(t, err)
}

func TestFileMemoryStore_ListFiles(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	err = store.WriteFile(ctx, "agent-1", "knowledge/a.md", "a", nil)
	require.NoError(t, err)
	err = store.WriteFile(ctx, "agent-1", "knowledge/b.md", "b", nil)
	require.NoError(t, err)

	files, err := store.ListFiles(ctx, "agent-1", "knowledge")
	require.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestFileMemoryStore_GetIndex(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	err = store.WriteFile(ctx, "agent-1", "knowledge/test.md", "indexed content", nil)
	require.NoError(t, err)

	index, err := store.GetIndex(ctx, "agent-1")
	require.NoError(t, err)
	assert.Contains(t, index, "Memory Index")
	assert.Contains(t, index, "test")
}

func TestFileMemoryStore_List_WithFilter(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	mem1 := &Memory{
		Content:    "episodic memory",
		SessionID:  "agent-1",
		MemoryType: string(MemoryTypeEpisodic),
	}
	err = store.Store(ctx, mem1)
	require.NoError(t, err)

	mem2 := &Memory{
		Content:    "procedural memory",
		SessionID:  "agent-1",
		MemoryType: string(MemoryTypeProcedural),
	}
	err = store.Store(ctx, mem2)
	require.NoError(t, err)

	filter := MemoryFilter{
		SessionID:  "agent-1",
		MemoryType: []MemoryType{MemoryTypeEpisodic},
	}
	results, err := store.List(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, string(MemoryTypeEpisodic), results[0].MemoryType)
}

// --- Frontmatter tests ---

func TestParseFrontmatter(t *testing.T) {
	data := []byte("---\nname: test\ncategory: knowledge\ncreated_at: 2026-01-01T00:00:00Z\nupdated_at: 2026-01-01T00:00:00Z\n---\nHello world")

	fm, body, err := ParseFrontmatter(data)
	require.NoError(t, err)
	assert.Equal(t, "test", fm.Name)
	assert.Equal(t, "knowledge", fm.Category)
	assert.Equal(t, "Hello world", body)
}

func TestParseFrontmatter_NoDelimiters(t *testing.T) {
	data := []byte("Just plain content")

	fm, body, err := ParseFrontmatter(data)
	require.NoError(t, err)
	assert.Equal(t, Frontmatter{}, fm)
	assert.Equal(t, "Just plain content", body)
}

func TestSerializeAndParse_Roundtrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	fm := Frontmatter{
		Name:      "test-roundtrip",
		Category:  "knowledge",
		Importance: 0.8,
		CreatedAt: now,
		UpdatedAt: now,
		Tags:      []string{"go", "test"},
		Metadata:  map[string]string{"source": "unit-test"},
	}
	body := "This is the body content."

	data := SerializeFrontmatter(fm, body)

	parsedFm, parsedBody, err := ParseFrontmatter(data)
	require.NoError(t, err)
	assert.Equal(t, fm.Name, parsedFm.Name)
	assert.Equal(t, fm.Category, parsedFm.Category)
	assert.Equal(t, fm.Importance, parsedFm.Importance)
	assert.Equal(t, body, parsedBody)
}

// --- Path validator tests ---

func TestValidatePath_RejectsTraversal(t *testing.T) {
	err := ValidatePath("../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "traversal")
}

func TestValidatePath_RejectsAbsolute(t *testing.T) {
	err := ValidatePath("/etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

func TestValidatePath_RejectsDotfile(t *testing.T) {
	err := ValidatePath(".hidden")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dotfile")
}

func TestValidatePath_RejectsEmpty(t *testing.T) {
	err := ValidatePath("")
	assert.Error(t, err)
}

func TestValidatePath_AcceptsValid(t *testing.T) {
	err := ValidatePath("knowledge/test-file.md")
	assert.NoError(t, err)
}

func TestValidatePath_RejectsEmbeddedTraversal(t *testing.T) {
	err := ValidatePath("knowledge/../../../etc/passwd")
	assert.Error(t, err)
}

// --- Index tests ---

func TestBuildIndex_Truncation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 10, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("file-%03d", i)
		err := store.WriteFile(ctx, "agent-1", "knowledge/"+name+".md", "content "+name, nil)
		require.NoError(t, err)
	}

	index, err := store.GetIndex(ctx, "agent-1")
	require.NoError(t, err)
	lines := strings.Count(index, "\n")
	assert.LessOrEqual(t, lines, 15) // 10 lines + header + truncation notice
	assert.Contains(t, index, "truncated")
}

// --- Directory permission tests ---

func TestEnsureAgentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	err := EnsureAgentDirs(tmpDir, "agent-1")
	require.NoError(t, err)

	for _, sub := range []string{"system", "knowledge", "archive"} {
		info, err := os.Stat(filepath.Join(tmpDir, "agent-1", sub))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestWriteFileWithPerms(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.md")

	err := WriteFileWithPerms(path, []byte("hello"))
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// --- Integration: store→recall→forget cycle ---

func TestIntegration_StoreRecallForget(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()

	// Store
	err = store.WriteFile(ctx, "agent-1", "knowledge/preference.md", "User prefers dark mode", map[string]string{
		"category": "user",
	})
	require.NoError(t, err)

	content, err := store.ReadFile(ctx, "agent-1", "knowledge/preference.md")
	require.NoError(t, err)
	assert.Contains(t, content, "User prefers dark mode")

	index, err := store.GetIndex(ctx, "agent-1")
	require.NoError(t, err)
	assert.Contains(t, index, "preference")

	err = store.DeleteFile(ctx, "agent-1", "knowledge/preference.md")
	require.NoError(t, err)

	_, err = store.ReadFile(ctx, "agent-1", "knowledge/preference.md")
	assert.Error(t, err)

	index, err = store.GetIndex(ctx, "agent-1")
	require.NoError(t, err)
	assert.NotContains(t, index, "preference")
}

func TestIntegration_StoreViaMemoryInterface_RecallViaFileInterface(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()

	mem := &Memory{
		Content:    "The user works in Go",
		SessionID:  "agent-1",
		MemoryType: string(MemoryTypeSemantic),
	}
	err = store.Store(ctx, mem)
	require.NoError(t, err)

	files, err := store.ListFiles(ctx, "agent-1", "knowledge")
	require.NoError(t, err)
	assert.NotEmpty(t, files)

	for _, f := range files {
		content, err := store.ReadFile(ctx, "agent-1", "knowledge/"+f)
		require.NoError(t, err)
		assert.Contains(t, content, "The user works in Go")
	}
}

// --- Path security: attack vectors ---

func TestPathSecurity_TraversalVariants(t *testing.T) {
	attackVectors := []struct {
		name     string
		path     string
		contains string
	}{
		{"parent traversal", "../etc/passwd", "traversal"},
		{"double parent", "../../etc/passwd", "traversal"},
		{"embedded traversal", "knowledge/../../../etc/passwd", "unclean"},
		{"absolute unix", "/etc/passwd", "absolute"},
		{"dotfile", ".env", "dotfile"},
		{"dotfile in path", "knowledge/.secret", "dotfile"},
		{"git directory", ".git/config", "dotfile"},
	}

	for _, tc := range attackVectors {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePath(tc.path)
			assert.Error(t, err, "expected error for path: %s", tc.path)
			if err != nil {
				assert.Contains(t, err.Error(), tc.contains)
			}
		})
	}
}

func TestPathSecurity_StoreRejectsTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	err = store.WriteFile(ctx, "agent-1", "../escape.md", "stolen", nil)
	assert.Error(t, err)
}

func TestPathSecurity_ReadFileRejectsTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.ReadFile(ctx, "agent-1", "../../etc/passwd")
	assert.Error(t, err)
}

// --- INDEX truncation with >200 files ---

func TestIndexTruncation_Over200Files(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	for i := 0; i < 250; i++ {
		name := fmt.Sprintf("entry-%03d", i)
		err := store.WriteFile(ctx, "agent-1", "knowledge/"+name+".md", "content "+name, nil)
		require.NoError(t, err)
	}

	index, err := store.GetIndex(ctx, "agent-1")
	require.NoError(t, err)
	assert.Contains(t, index, "Memory Index")
	assert.Contains(t, index, "truncated")

	lines := strings.Count(index, "\n")
	assert.LessOrEqual(t, lines, 210) // 200 max + header + notice
}

func TestIndexTruncation_ByteLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 500)
	require.NoError(t, err)

	ctx := context.Background()
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("long-name-entry-%03d-with-verbose-description", i)
		err := store.WriteFile(ctx, "agent-1", "knowledge/"+name+".md", "content", nil)
		require.NoError(t, err)
	}

	index, err := store.GetIndex(ctx, "agent-1")
	require.NoError(t, err)
	assert.LessOrEqual(t, len(index), 600)
}

// --- Empty/missing directory tests ---

func TestGetIndex_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	index, err := store.GetIndex(ctx, "nonexistent-agent")
	require.NoError(t, err)
	assert.Equal(t, "", index)
}

func TestListFiles_NonexistentDir(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	ctx := context.Background()
	files, err := store.ListFiles(ctx, "agent-1", "knowledge")
	require.NoError(t, err)
	assert.Nil(t, files)
}

// --- Frontmatter edge cases ---

func TestParseFrontmatter_UnclosedDelimiter(t *testing.T) {
	data := []byte("---\nname: test\nno closing delimiter")
	_, _, err := ParseFrontmatter(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closing delimiter")
}

func TestParseFrontmatter_InvalidYAML(t *testing.T) {
	data := []byte("---\n: invalid yaml {{\n---\nbody")
	_, _, err := ParseFrontmatter(data)
	assert.Error(t, err)
}

func TestSerializeFrontmatter_EmptyBody(t *testing.T) {
	fm := Frontmatter{
		Name:      "empty-body",
		Category:  "knowledge",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	data := SerializeFrontmatter(fm, "")

	parsedFm, body, err := ParseFrontmatter(data)
	require.NoError(t, err)
	assert.Equal(t, "empty-body", parsedFm.Name)
	assert.Equal(t, "", body)
}

// --- MemoryStore interface compliance ---

func TestFileMemoryStore_ImplementsMemoryStore(t *testing.T) {
	var _ MemoryStore = (*FileMemoryStore)(nil)
}

func TestFileMemoryStore_ImplementsFileMemoryStoreInterface(t *testing.T) {
	var _ FileMemoryStoreInterface = (*FileMemoryStore)(nil)
}
