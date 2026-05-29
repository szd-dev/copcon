package tools

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

type MemoryRecallTool struct {
	basePath string
}

func NewMemoryRecallTool(store MemoryStoreAPI) *MemoryRecallTool {
	basePath := ""
	if bp, ok := store.(interface{ BasePath() string }); ok {
		basePath = bp.BasePath()
	}
	return &MemoryRecallTool{basePath: basePath}
}

func (t *MemoryRecallTool) Name() string {
	return "memory_recall"
}

func (t *MemoryRecallTool) Description() string {
	return "Search the agent's persistent memory by keyword. Returns matching entries from knowledge/ and archive/ directories."
}

func (t *MemoryRecallTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Keywords to search for in memory files",
			},
			"category": map[string]any{
				"type":        "string",
				"description": "Filter by category: 'user', 'system', 'archive' (optional)",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Maximum number of results to return (default: 5)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MemoryRecallTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult("query is required")
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	category, _ := args["category"].(string)

	agentIDStr := chatCtx.AgentID()

	agentDir := filepath.Join(t.basePath, agentIDStr)
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		return successResult(map[string]any{
			"results": []any{},
			"count":   0,
			"message": "No memory directory found for this agent",
		})
	}

	var results []map[string]any
	queryLower := strings.ToLower(query)

	searchDirs := []string{"knowledge", "archive"}
	if category != "" {
		searchDirs = []string{categoryToSubdir(category)}
	}

	for _, subdir := range searchDirs {
		dir := filepath.Join(agentDir, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "INDEX.md" {
				continue
			}

			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}

			content := string(data)
			if strings.Contains(strings.ToLower(content), queryLower) {
				relPath := filepath.Join(subdir, e.Name())
				results = append(results, map[string]any{
					"path":    relPath,
					"name":    strings.TrimSuffix(e.Name(), ".md"),
					"snippet": truncateString(extractBody(content), 200),
				})

				if len(results) >= limit {
					break
				}
			}
		}

		if len(results) >= limit {
			break
		}
	}

	return successResult(map[string]any{
		"results": results,
		"count":   len(results),
		"query":   query,
	})
}

func extractBody(content string) string {
	if idx := strings.Index(content, "---\n"); idx == 0 {
		end := strings.Index(content[3:], "---\n")
		if end > 0 {
			return strings.TrimSpace(content[3+end+4:])
		}
	}
	return content
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var _ tool.Tool = (*MemoryRecallTool)(nil)
