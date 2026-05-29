package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

// MemoryStoreAPI defines the interface needed by memory tools to interact
// with the file-based memory store. This avoids direct imports of the
// filememory package from the capabilities/tools package.
type MemoryStoreAPI interface {
	WriteFile(ctx context.Context, agentID, relPath, content string, metadata map[string]string) error
	ReadFile(ctx context.Context, agentID, relPath string) (string, error)
	DeleteFile(ctx context.Context, agentID, relPath string) error
	ListFiles(ctx context.Context, agentID, relPath string) ([]string, error)
	GetIndex(ctx context.Context, agentID string) (string, error)
	RemoveFromIndex(ctx context.Context, agentID, relPath string) error
}

type MemoryStoreTool struct {
	store MemoryStoreAPI
}

func NewMemoryStoreTool(store MemoryStoreAPI) *MemoryStoreTool {
	return &MemoryStoreTool{store: store}
}

func (t *MemoryStoreTool) Name() string {
	return "memory_store"
}

func (t *MemoryStoreTool) Description() string {
	return "Store important information to the agent's persistent file-based memory. Saves content as a markdown file with metadata."
}

func (t *MemoryStoreTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The content to store in memory",
			},
			"category": map[string]any{
				"type":        "string",
				"description": "Category for the memory (default: 'user'). Determines subdirectory: 'user' -> knowledge/, 'system' -> system/, 'archive' -> archive/",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Descriptive name for the memory file (optional, auto-generated if omitted)",
			},
			"importance": map[string]any{
				"type":        "number",
				"description": "Importance score 0-1 (optional, default 0.5)",
			},
		},
		"required": []string{"content"},
	}
}

func (t *MemoryStoreTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	content, ok := args["content"].(string)
	if !ok || content == "" {
		return errorResult("content is required")
	}

	category, _ := args["category"].(string)
	if category == "" {
		category = "user"
	}

	name, _ := args["name"].(string)
	if name == "" {
		name = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}

	importance := 0.5
	if imp, ok := args["importance"].(float64); ok {
		importance = imp
	}

	agentIDStr := chatCtx.AgentID()

	subdir := categoryToSubdir(category)
	relPath := filepath.Join(subdir, name+".md")

	metadata := map[string]string{
		"category":   category,
		"importance": fmt.Sprintf("%.2f", importance),
	}

	ctx := chatCtx.Context()

	err := t.store.WriteFile(ctx, agentIDStr, relPath, content, metadata)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to store memory: %v", err))
	}

	return successResult(map[string]any{
		"path":       relPath,
		"name":       name,
		"category":   category,
		"importance": importance,
		"message":    "Memory stored successfully",
	})
}

func categoryToSubdir(category string) string {
	switch strings.ToLower(category) {
	case "system":
		return "system"
	case "archive":
		return "archive"
	default:
		return "knowledge"
	}
}

var _ tool.Tool = (*MemoryStoreTool)(nil)
