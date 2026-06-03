package memoryfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

type MemoryForgetTool struct {
	basePath string
	store    MemoryStoreAPI
}

func NewMemoryForgetTool(store MemoryStoreAPI) *MemoryForgetTool {
	basePath := ""
	if bp, ok := store.(interface{ BasePath() string }); ok {
		basePath = bp.BasePath()
	}
	return &MemoryForgetTool{basePath: basePath, store: store}
}

func (t *MemoryForgetTool) Name() string {
	return "memory_forget"
}

func (t *MemoryForgetTool) Description() string {
	return "Remove a memory entry from the agent's persistent file-based memory. Provide either a name or path to identify the file to delete."
}

func (t *MemoryForgetTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Name of the memory file to delete (searches knowledge/ and archive/)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Exact relative path of the memory file to delete (e.g., 'knowledge/my-file.md')",
			},
		},
	}
}

func (t *MemoryForgetTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	name, _ := args["name"].(string)
	path, _ := args["path"].(string)

	if name == "" && path == "" {
		return errorResult("either 'name' or 'path' must be provided")
	}

	agentIDStr := chatCtx.AgentID()

	var relPath string
	if path != "" {
		relPath = path
	} else {
		found, err := t.findFileByName(agentIDStr, name)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to find memory: %v", err))
		}
		if found == "" {
			return errorResult(fmt.Sprintf("memory not found: %s", name))
		}
		relPath = found
	}

	fullPath := filepath.Join(t.basePath, agentIDStr, relPath)
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return errorResult(fmt.Sprintf("memory file not found: %s", relPath))
		}
		return errorResult(fmt.Sprintf("failed to delete memory: %v", err))
	}

	// Rebuild INDEX.md and FACTS.md after deletion
	if fms, ok := t.store.(*FileMemoryStore); ok {
		fms.mu.Lock()
		_ = BuildIndex(t.basePath, agentIDStr, fms.maxIndexLines, fms.maxIndexBytes)
		_ = BuildFacts(t.basePath, agentIDStr)
		fms.mu.Unlock()
	}

	return successResult(map[string]any{
		"path":    relPath,
		"message": "Memory forgotten successfully",
	})
}

func (t *MemoryForgetTool) findFileByName(agentID, name string) (string, error) {
	agentDir := filepath.Join(t.basePath, agentID)
	targetName := name
	if !strings.HasSuffix(targetName, ".md") {
		targetName += ".md"
	}

	for _, subdir := range []string{"knowledge", "archive"} {
		dir := filepath.Join(agentDir, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.IsDir() && e.Name() == targetName {
				return filepath.Join(subdir, e.Name()), nil
			}
		}

		nameLower := strings.ToLower(targetName)
		for _, e := range entries {
			if !e.IsDir() && strings.ToLower(e.Name()) == nameLower {
				return filepath.Join(subdir, e.Name()), nil
			}
		}
	}

	return "", nil
}

var _ tool.Tool = (*MemoryForgetTool)(nil)