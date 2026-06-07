package memoryfile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/plugins/testutil"
)

type mockChatContext = testutil.MockChatContext

func setupTestStore(t *testing.T) *FileMemoryStore {
	t.Helper()
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)
	return store
}

func TestMemoryTools_Metadata(t *testing.T) {
	store := setupTestStore(t)

	assert.Equal(t, "memory_store", NewMemoryStoreTool(store).Name())
	assert.Equal(t, "memory_recall", NewMemoryRecallTool(store, nil).Name())
	assert.Equal(t, "memory_forget", NewMemoryForgetTool(store).Name())
}

func TestMemoryStoreTool_Execute(t *testing.T) {
	store := setupTestStore(t)
	tool := NewMemoryStoreTool(store)

	chatCtx := &mockChatContext{
		Ctx:     context.Background(),
		Agent:   "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{
		"content":    "User prefers Go over Python",
		"category":   "user",
		"name":       "language-preference",
		"importance": 0.9,
	})
	require.NoError(t, err)
	assert.True(t, result.Success)

	data := result.Data.(map[string]any)
	response := data["response"].(string)
	assert.Contains(t, response, "language-preference")
	assert.Contains(t, response, "knowledge")
}

func TestMemoryStoreTool_Execute_MissingContent(t *testing.T) {
	store := setupTestStore(t)
	tool := NewMemoryStoreTool(store)

	chatCtx := &mockChatContext{
		Ctx:     context.Background(),
		Agent:   "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "content is required")
}

func TestMemoryRecallTool_Execute(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()
	err := store.WriteFile(ctx, "agent-1", "knowledge/go-pref.md", "User prefers Go programming language", nil)
	require.NoError(t, err)

	tool := NewMemoryRecallTool(store, nil)
	chatCtx := &mockChatContext{
		Ctx:     ctx,
		Agent:   "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{
		"query": "Go programming",
		"limit": 5,
	})
	require.NoError(t, err)
	assert.True(t, result.Success)

	data := result.Data.(map[string]any)
	response := data["response"].(string)
	assert.Contains(t, response, "go-pref")
}

func TestMemoryRecallTool_Execute_NoMatch(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()
	err := store.WriteFile(ctx, "agent-1", "knowledge/go-pref.md", "User prefers Go", nil)
	require.NoError(t, err)

	tool := NewMemoryRecallTool(store, nil)
	chatCtx := &mockChatContext{
		Ctx:     ctx,
		Agent:   "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{
		"query": "Rust programming",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)

	data := result.Data.(map[string]any)
	response := data["response"].(string)
	assert.Contains(t, response, `"count":0`)
}

func TestMemoryRecallTool_Execute_MissingQuery(t *testing.T) {
	store := setupTestStore(t)
	tool := NewMemoryRecallTool(store, nil)

	chatCtx := &mockChatContext{
		Ctx:     context.Background(),
		Agent:   "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "query is required")
}

func TestMemoryForgetTool_Execute_ByPath(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()
	err := store.WriteFile(ctx, "agent-1", "knowledge/forget-me.md", "Temporary note", nil)
	require.NoError(t, err)

	tool := NewMemoryForgetTool(store)
	chatCtx := &mockChatContext{
		Ctx:     ctx,
		Agent:   "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{
		"path": "knowledge/forget-me.md",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)

	_, err = store.ReadFile(ctx, "agent-1", "knowledge/forget-me.md")
	assert.Error(t, err)
}

func TestMemoryForgetTool_Execute_ByName(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()
	err := store.WriteFile(ctx, "agent-1", "knowledge/find-me.md", "Named note", nil)
	require.NoError(t, err)

	tool := NewMemoryForgetTool(store)
	chatCtx := &mockChatContext{
		Ctx:     ctx,
		Agent:   "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{
		"name": "find-me",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestMemoryForgetTool_Execute_MissingBothNameAndPath(t *testing.T) {
	store := setupTestStore(t)
	tool := NewMemoryForgetTool(store)

	chatCtx := &mockChatContext{
		Ctx:     context.Background(),
		Agent:   "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "name' or 'path'")
}

func TestMemoryToolIntegration_StoreRecallForget(t *testing.T) {
	store := setupTestStore(t)
	chatCtx := &mockChatContext{
		Ctx:     context.Background(),
		Agent:   "agent-1",
	}

	storeTool := NewMemoryStoreTool(store)
	result, err := storeTool.Execute(chatCtx, map[string]any{
		"content":  "User likes dark mode",
		"category": "user",
		"name":     "ui-preference",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)

	recallTool := NewMemoryRecallTool(store, nil)
	result, err = recallTool.Execute(chatCtx, map[string]any{
		"query": "dark mode",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)

	data := result.Data.(map[string]any)
	response := data["response"].(string)
	assert.Contains(t, response, "ui-preference")

	forgetTool := NewMemoryForgetTool(store)
	result, err = forgetTool.Execute(chatCtx, map[string]any{
		"path": "knowledge/ui-preference.md",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)

	result, err = recallTool.Execute(chatCtx, map[string]any{
		"query": "dark mode",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)

	data = result.Data.(map[string]any)
	response = data["response"].(string)
	assert.Contains(t, response, `"count":0`)
}
