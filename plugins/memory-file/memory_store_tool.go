package memoryfile

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
			"type": map[string]any{
				"type":        "string",
				"description": "Memory type classification: user, feedback, project, reference (optional)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Brief description of the memory content (optional)",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "Session ID this memory originates from (auto-filled if not provided)",
			},
			"message_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Message IDs that contributed to this memory (optional)",
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

	memType, _ := args["type"].(string)
	description, _ := args["description"].(string)

	agentIDStr := chatCtx.AgentID()
	sessionID := chatCtx.SessionID()
	if sessionIDArg, ok := args["session_id"].(string); ok && sessionIDArg != "" {
		sessionID = sessionIDArg
	}

	var messageIDs []string
	if mids, ok := args["message_ids"].([]any); ok {
		for _, mid := range mids {
			if s, ok := mid.(string); ok {
				messageIDs = append(messageIDs, s)
			}
		}
	}

	SetManualStoreFlag(sessionID)

	metadata := map[string]string{
		"category":   category,
		"importance": fmt.Sprintf("%.2f", importance),
	}

	now := time.Now()
	subdir := categoryToSubdir(category)
	relPath := filepath.Join(subdir, name+".md")

	fm := Frontmatter{
		Name:        name,
		Category:    category,
		Importance:  importance,
		Description: description,
		Type:        memType,
		SessionID:   sessionID,
		MessageIDs:  messageIDs,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    metadata,
	}

	data := SerializeFrontmatter(fm, content)

	basePath := ""
	if bp, ok := t.store.(interface{ BasePath() string }); ok {
		basePath = bp.BasePath()
	}

	fullPath := filepath.Join(basePath, agentIDStr, relPath)
	if err := EnsureAgentDirs(basePath, agentIDStr); err != nil {
		return errorResult(fmt.Sprintf("failed to create dirs: %v", err))
	}
	if err := WriteFileWithPerms(fullPath, data); err != nil {
		return errorResult(fmt.Sprintf("failed to write: %v", err))
	}

	if fms, ok := t.store.(*FileMemoryStore); ok {
		fms.mu.Lock()
		_ = BuildIndex(basePath, agentIDStr, fms.maxIndexLines, fms.maxIndexBytes)
		_ = BuildFacts(basePath, agentIDStr)
		fms.mu.Unlock()
	}

	return successResult(map[string]any{
		"path":        relPath,
		"name":        name,
		"category":    category,
		"importance":  importance,
		"type":        memType,
		"description": description,
		"session_id":  sessionID,
		"message_ids": messageIDs,
		"message":     "Memory stored successfully",
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