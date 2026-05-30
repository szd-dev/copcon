package memoryfile

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/copcon/core/hook"
)

const memoryProtocol = `## Memory Protocol

You have a persistent file-based memory system. You can:

1. **Store** important information using the memory_store tool — write observations, decisions, user preferences, or any knowledge worth retaining.
2. **Recall** relevant memories using the memory_recall tool — search your knowledge base by keyword when you need to reference past context.
3. **Forget** outdated or incorrect memories using the memory_forget tool — remove entries that are no longer accurate or relevant.

Memory files are organized under knowledge/ (active) and archive/ (historical). The INDEX.md provides an overview of all stored memories.

Guidelines:
- Store information proactively when you learn something new about the user or task.
- Recall before making assumptions — check if relevant context already exists.
- Forget information that has been superseded or explicitly requested for removal.
- Use descriptive names and categories to keep memories organized.`

type FileMemoryHook struct {
	fileMemoryStore FileMemoryStoreReader
	basePath        string
	logger          *slog.Logger
}

// FileMemoryStoreReader defines the read interface needed by the hook
// to avoid importing the filememory package directly.
type FileMemoryStoreReader interface {
	BasePath() string
}

func NewFileMemoryHook(store FileMemoryStoreReader) *FileMemoryHook {
	return &FileMemoryHook{
		fileMemoryStore: store,
		basePath:        store.BasePath(),
		logger:          slog.Default(),
	}
}

func (h *FileMemoryHook) Name() string {
	return "file_memory"
}

func (h *FileMemoryHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.OnSystemPrompt}
}

func (h *FileMemoryHook) Priority() int {
	return 80
}

func (h *FileMemoryHook) Execute(ctx *hook.HookContext) error {
	if ctx.SystemPrompt == nil {
		return nil
	}

	agentID := ctx.AgentID
	if agentID == "" {
		return nil
	}

	agentDir := filepath.Join(h.basePath, agentID)
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		return nil
	}

	var sections []string

	systemContent := h.readSystemFiles(agentDir)
	if systemContent != "" {
		sections = append(sections, "### System Context\n\n"+systemContent)
	}

	indexContent := h.readIndex(agentDir)
	if indexContent != "" {
		sections = append(sections, "### Memory Index\n\n"+indexContent)
	}

	if len(sections) == 0 {
		return nil
	}

	sections = append(sections, "### Memory Protocol\n\n"+memoryProtocol)

	injection := "## Agent Memory\n\n" + strings.Join(sections, "\n\n")
	*ctx.SystemPrompt = *ctx.SystemPrompt + "\n\n" + injection

	return nil
}

func (h *FileMemoryHook) readSystemFiles(agentDir string) string {
	systemDir := filepath.Join(agentDir, "system")
	entries, err := os.ReadDir(systemDir)
	if err != nil {
		return ""
	}

	var parts []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "INDEX.md" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(systemDir, e.Name()))
		if err != nil {
			h.logger.Warn("failed to read system file",
				"file", e.Name(),
				"error", err,
			)
			continue
		}
		parts = append(parts, fmt.Sprintf("**%s:**\n%s", e.Name(), string(data)))
	}

	return strings.Join(parts, "\n\n")
}

func (h *FileMemoryHook) readIndex(agentDir string) string {
	indexPath := filepath.Join(agentDir, "system", "INDEX.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return ""
	}
	return string(data)
}
