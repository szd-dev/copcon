package memoryfile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/tool"
)

// MemoryRecallTool provides LLM-powered semantic selection over memory entries,
// with fallback to traditional keyword matching when the LLM is unavailable.
type MemoryRecallTool struct {
	basePath string
	store    MemoryStoreAPI
	llm      llm.LLMProvider
}

func NewMemoryRecallTool(store MemoryStoreAPI, llmProvider llm.LLMProvider) *MemoryRecallTool {
	basePath := ""
	if bp, ok := store.(interface{ BasePath() string }); ok {
		basePath = bp.BasePath()
	}
	return &MemoryRecallTool{basePath: basePath, store: store, llm: llmProvider}
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

type indexEntry struct {
	Name        string
	RelPath     string
	Description string
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

	if t.llm != nil {
		results, err := t.semanticRecall(chatCtx.Context(), agentIDStr, query, category, limit)
		if err == nil && len(results) > 0 {
			return successResult(map[string]any{
				"results": results,
				"count":   len(results),
				"query":   query,
			})
		}
	}

	return t.keywordRecall(agentIDStr, query, category, limit)
}

func (t *MemoryRecallTool) semanticRecall(ctx context.Context, agentID, query, category string, limit int) ([]map[string]any, error) {
	indexContent, err := ReadIndex(t.basePath, agentID)
	if err != nil || indexContent == "" {
		return nil, fmt.Errorf("no index available")
	}

	entries := parseIndexEntries(indexContent)
	if len(entries) == 0 {
		return nil, fmt.Errorf("no entries in index")
	}

	if category != "" {
		var filtered []indexEntry
		targetSubdir := categoryToSubdir(category)
		for _, e := range entries {
			if strings.HasPrefix(e.RelPath, targetSubdir) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
		if len(entries) == 0 {
			return nil, fmt.Errorf("no entries match category")
		}
	}

	var entryLines []string
	for i, e := range entries {
		entryLines = append(entryLines, fmt.Sprintf("%d. name: %q, path: %q, description: %q", i+1, e.Name, e.RelPath, e.Description))
	}

	prompt := fmt.Sprintf(`Select the most relevant memory entries for this query: "%s"

Available memories:
%s

Return a JSON array of up to %d paths. Example: ["knowledge/xxx.md"]
Return ONLY the JSON array.`, query, strings.Join(entryLines, "\n"), limit)

	params := llm.StreamParams{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
	}

	resp, err := Complete(ctx, t.llm, params)
	if err != nil {
		return nil, fmt.Errorf("llm completion failed: %w", err)
	}

	resp = strings.TrimSpace(resp)
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	var selectedPaths []string
	if err := json.Unmarshal([]byte(resp), &selectedPaths); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if len(selectedPaths) == 0 {
		return nil, fmt.Errorf("llm selected no paths")
	}

	var results []map[string]any
	for _, relPath := range selectedPaths {
		if len(results) >= limit {
			break
		}
		fullPath := filepath.Join(t.basePath, agentID, relPath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		name := strings.TrimSuffix(filepath.Base(relPath), ".md")

		sessionID := ""
		memType := ""
		fm, body, err := ParseFrontmatter(data)
		if err == nil {
			sessionID = fm.SessionID
			memType = fm.Type
		} else {
			body = string(data)
		}

		results = append(results, map[string]any{
			"path":       relPath,
			"name":       name,
			"snippet":    truncateString(body, 200),
			"type":       memType,
			"session_id": sessionID,
		})
	}

	return results, nil
}

// parseIndexEntries parses INDEX.md content into structured indexEntry values.
//
// INDEX.md format:
//
//	- **name** (`relpath`) — category [date]
//	  > description
func parseIndexEntries(indexContent string) []indexEntry {
	var entries []indexEntry
	lines := strings.Split(indexContent, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "- **") {
			var entry indexEntry
			rest := line[4:]
			if idx := strings.Index(rest, "** (`"); idx > 0 {
				entry.Name = rest[:idx]
				rest = rest[idx+5:]
				if idx2 := strings.Index(rest, "`)"); idx2 > 0 {
					entry.RelPath = rest[:idx2]
				}
			}
			entries = append(entries, entry)
		} else if strings.HasPrefix(line, "  > ") && len(entries) > 0 {
			entries[len(entries)-1].Description = strings.TrimPrefix(line, "  > ")
		}
	}

	return entries
}

func (t *MemoryRecallTool) keywordRecall(agentID, query, category string, limit int) (*tool.ToolResult, error) {
	agentDir := filepath.Join(t.basePath, agentID)
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
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "INDEX.md" {
				continue
			}

			relPath := filepath.Join(subdir, e.Name())
			fullPath := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}

			content := string(data)
			if !strings.Contains(strings.ToLower(content), queryLower) {
				continue
			}

			sessionID := ""
			fm, body, err := ParseFrontmatter(data)
			if err == nil {
				sessionID = fm.SessionID
				content = body
			}

			results = append(results, map[string]any{
				"path":       relPath,
				"name":       strings.TrimSuffix(e.Name(), ".md"),
				"snippet":    truncateString(extractBody(content), 200),
				"session_id": sessionID,
			})

			if len(results) >= limit {
				break
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

// extractBody returns the content after the YAML frontmatter delimiter.
// If no frontmatter is found, the full content is returned.
func extractBody(content string) string {
	if idx := strings.Index(content, "---\n"); idx == 0 {
		end := strings.Index(content[3:], "---\n")
		if end > 0 {
			return strings.TrimSpace(content[3+end+4:])
		}
	}
	return content
}

// truncateString truncates a string to maxLen characters, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

var _ tool.Tool = (*MemoryRecallTool)(nil)