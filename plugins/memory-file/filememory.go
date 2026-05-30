package memoryfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/copcon/core/storage"
)

// FileMemoryStoreInterface extends MemoryStore with file-level operations
// used by the memory tools (store/recall/forget).
type FileMemoryStoreInterface interface {
	MemoryStore
	ReadFile(ctx context.Context, agentID, relPath string) (string, error)
	WriteFile(ctx context.Context, agentID, relPath, content string, metadata map[string]string) error
	DeleteFile(ctx context.Context, agentID, relPath string) error
	ListFiles(ctx context.Context, agentID, relPath string) ([]string, error)
	GetIndex(ctx context.Context, agentID string) (string, error)
	UpdateIndex(ctx context.Context, agentID string, entry IndexEntry) error
	RemoveFromIndex(ctx context.Context, agentID, relPath string) error
}

// FileMemoryStore implements both MemoryStore and FileMemoryStoreInterface
// using a filesystem-based storage with YAML frontmatter and INDEX.md.
type FileMemoryStore struct {
	basePath     string
	maxIndexLines int
	maxIndexBytes int
}

var _ FileMemoryStoreInterface = (*FileMemoryStore)(nil)

// NewFileMemoryStore creates a new file-based memory store.
func NewFileMemoryStore(basePath string, maxIndexLines, maxIndexBytes int) (*FileMemoryStore, error) {
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base path: %w", err)
	}

	if maxIndexLines <= 0 {
		maxIndexLines = defaultMaxIndexLines
	}
	if maxIndexBytes <= 0 {
		maxIndexBytes = defaultMaxIndexBytes
	}

	return &FileMemoryStore{
		basePath:      absPath,
		maxIndexLines: maxIndexLines,
		maxIndexBytes: maxIndexBytes,
	}, nil
}

// BasePath returns the resolved absolute base path.
func (s *FileMemoryStore) BasePath() string {
	return s.basePath
}

// --- MemoryStore implementation ---

func (s *FileMemoryStore) Store(ctx context.Context, memory *storage.Memory) error {
	if memory.ID == "" {
		memory.ID = uuid.New().String()
	}
	if memory.Timestamp.IsZero() {
		memory.Timestamp = time.Now()
	}
	if memory.MemoryType == "" {
		memory.MemoryType = string(storage.MemoryTypeConversation)
	}

	agentID := memory.AgentID
	if agentID == "" {
		return fmt.Errorf("memory AgentID must not be empty")
	}

	category := categoryFromType(memory.MemoryType)
	name := memory.ID

	relPath := filepath.Join(category, name+".md")
	fm := Frontmatter{
		Name:       name,
		Category:   category,
		Importance: memory.Importance,
		CreatedAt:  memory.Timestamp,
		UpdatedAt:  time.Now(),
	}

	fm.Metadata = make(map[string]string)
	if memory.Metadata != nil {
		for k, v := range memory.Metadata {
			if vs, ok := v.(string); ok {
				fm.Metadata[k] = vs
			}
		}
	}
	fm.Metadata["memory_type"] = memory.MemoryType

	data := SerializeFrontmatter(fm, memory.Content)
	fullPath := s.fullPath(agentID, relPath)

	if err := ValidatePath(relPath); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("file already exists: %s", relPath)
	}

	if err := EnsureAgentDirs(s.basePath, agentID); err != nil {
		return err
	}

	if err := WriteFileWithPerms(fullPath, data); err != nil {
		return err
	}

	return BuildIndex(s.basePath, agentID, s.maxIndexLines, s.maxIndexBytes)
}

func (s *FileMemoryStore) Search(ctx context.Context, query []float32, limit int) ([]*storage.Memory, error) {
	return nil, fmt.Errorf("file memory does not support vector search; use keyword recall instead")
}

func (s *FileMemoryStore) GetByAgentID(ctx context.Context, agentID string, limit int) ([]*storage.Memory, error) {
	agentDir := AgentDir(s.basePath, agentID)

	var results []*storage.Memory
	for _, subdir := range []string{"knowledge", "archive"} {
		dir := filepath.Join(agentDir, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			mem, err := s.readMemoryFile(filepath.Join(dir, e.Name()), agentID)
			if err != nil {
				continue
			}
			results = append(results, mem)
			if limit > 0 && len(results) >= limit {
				return results, nil
			}
		}
	}

	return results, nil
}

func (s *FileMemoryStore) DeleteByAgentID(ctx context.Context, agentID string) error {
	agentDir := AgentDir(s.basePath, agentID)
	return os.RemoveAll(agentDir)
}

func (s *FileMemoryStore) List(ctx context.Context, filter storage.MemoryFilter) ([]*storage.Memory, error) {
	var results []*storage.Memory

	agentID := filter.AgentID
	if agentID == "" {
		return nil, fmt.Errorf("MemoryFilter.AgentID is required for file memory")
	}

	agentDir := AgentDir(s.basePath, agentID)
	dirs := []string{"knowledge", "archive"}

	for _, subdir := range dirs {
		dir := filepath.Join(agentDir, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			mem, err := s.readMemoryFile(filepath.Join(dir, e.Name()), agentID)
			if err != nil {
				continue
			}

			if !matchesFilter(mem, filter) {
				continue
			}

			results = append(results, mem)
		}
	}

	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}

	return results, nil
}

func (s *FileMemoryStore) Get(ctx context.Context, id string) (*storage.Memory, error) {
	agentDir := s.basePath

	for _, subdir := range []string{"knowledge", "archive"} {
		dir := filepath.Join(agentDir, subdir)
		candidate := filepath.Join(dir, id+".md")
		if _, err := os.Stat(candidate); err == nil {
			return s.readMemoryFile(candidate, "")
		}
		candidate = filepath.Join(dir, id)
		if _, err := os.Stat(candidate); err == nil {
			return s.readMemoryFile(candidate, "")
		}
	}

	return nil, fmt.Errorf("memory not found: %s", id)
}

func (s *FileMemoryStore) Update(ctx context.Context, memory *storage.Memory) error {
	agentID := memory.AgentID
	if agentID == "" {
		return fmt.Errorf("memory AgentID must not be empty")
	}

	category := categoryFromType(memory.MemoryType)
	name := memory.ID
	relPath := filepath.Join(category, name+".md")
	fullPath := s.fullPath(agentID, relPath)

	if _, err := os.Stat(fullPath); err != nil {
		return fmt.Errorf("memory not found: %s", relPath)
	}

	fm := Frontmatter{
		Name:       name,
		Category:   category,
		Importance: memory.Importance,
		CreatedAt:  memory.Timestamp,
		UpdatedAt:  time.Now(),
	}

	fm.Metadata = make(map[string]string)
	if memory.Metadata != nil {
		for k, v := range memory.Metadata {
			if vs, ok := v.(string); ok {
				fm.Metadata[k] = vs
			}
		}
	}
	fm.Metadata["memory_type"] = memory.MemoryType

	data := SerializeFrontmatter(fm, memory.Content)
	if err := WriteFileWithPerms(fullPath, data); err != nil {
		return err
	}

	return BuildIndex(s.basePath, agentID, s.maxIndexLines, s.maxIndexBytes)
}

func (s *FileMemoryStore) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("Delete by ID requires agentID context; use DeleteFile instead")
}

// --- FileMemoryStoreInterface file-level operations ---

func (s *FileMemoryStore) ReadFile(ctx context.Context, agentID, relPath string) (string, error) {
	if err := ValidatePath(relPath); err != nil {
		return "", err
	}

	fullPath := s.fullPath(agentID, relPath)
	isSym, err := IsSymlink(fullPath)
	if err != nil {
		return "", err
	}
	if isSym {
		return "", fmt.Errorf("symlinks are not allowed: %s", relPath)
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(data), nil
}

func (s *FileMemoryStore) WriteFile(ctx context.Context, agentID, relPath, content string, metadata map[string]string) error {
	if err := ValidatePath(relPath); err != nil {
		return err
	}

	fullPath := s.fullPath(agentID, relPath)
	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("file already exists: %s", relPath)
	}

	now := time.Now()
	name := strings.TrimSuffix(filepath.Base(relPath), ".md")
	category := filepath.Dir(relPath)

	fm := Frontmatter{
		Name:      name,
		Category:  category,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  metadata,
	}

	data := SerializeFrontmatter(fm, content)

	if err := EnsureAgentDirs(s.basePath, agentID); err != nil {
		return err
	}

	if err := WriteFileWithPerms(fullPath, data); err != nil {
		return err
	}

	return BuildIndex(s.basePath, agentID, s.maxIndexLines, s.maxIndexBytes)
}

func (s *FileMemoryStore) DeleteFile(ctx context.Context, agentID, relPath string) error {
	if err := ValidatePath(relPath); err != nil {
		return err
	}

	fullPath := s.fullPath(agentID, relPath)
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return BuildIndex(s.basePath, agentID, s.maxIndexLines, s.maxIndexBytes)
}

func (s *FileMemoryStore) ListFiles(ctx context.Context, agentID, relPath string) ([]string, error) {
	dir := s.fullPath(agentID, relPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func (s *FileMemoryStore) GetIndex(ctx context.Context, agentID string) (string, error) {
	return ReadIndex(s.basePath, agentID)
}

func (s *FileMemoryStore) UpdateIndex(ctx context.Context, agentID string, entry IndexEntry) error {
	return AddToIndex(s.basePath, agentID, entry)
}

func (s *FileMemoryStore) RemoveFromIndex(ctx context.Context, agentID, relPath string) error {
	return RemoveFromIndex(s.basePath, agentID, relPath)
}

// --- internal helpers ---

func (s *FileMemoryStore) fullPath(agentID, relPath string) string {
	return filepath.Join(s.basePath, agentID, relPath)
}

func (s *FileMemoryStore) readMemoryFile(path, agentID string) (*storage.Memory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	fm, body, err := ParseFrontmatter(data)
	if err != nil {
		return nil, err
	}

	mem := &storage.Memory{
		ID:        strings.TrimSuffix(filepath.Base(path), ".md"),
		Content:   body,
		AgentID:   agentID,
		Timestamp: fm.CreatedAt,
		Importance: fm.Importance,
	}

	if mt, ok := fm.Metadata["memory_type"]; ok {
		mem.MemoryType = mt
	} else {
		mem.MemoryType = fm.Category
	}

	return mem, nil
}

func categoryFromType(memType string) string {
	switch memType {
	case string(storage.MemoryTypeEpisodic), string(storage.MemoryTypeSemantic):
		return "knowledge"
	case string(storage.MemoryTypeProcedural):
		return "archive"
	default:
		return "knowledge"
	}
}

func matchesFilter(mem *storage.Memory, filter storage.MemoryFilter) bool {
	if len(filter.MemoryType) > 0 {
		found := false
		for _, mt := range filter.MemoryType {
			if mem.MemoryType == string(mt) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if !filter.Since.IsZero() && mem.Timestamp.Before(filter.Since) {
		return false
	}
	if !filter.Until.IsZero() && mem.Timestamp.After(filter.Until) {
		return false
	}

	return true
}
