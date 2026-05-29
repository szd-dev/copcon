package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/providers/filememory"
)

type mockChatContext struct {
	ctx       context.Context
	agentID   string
	sessionID string
}

func (m *mockChatContext) Context() context.Context                          { return m.ctx }
func (m *mockChatContext) SessionID() string                                 { return m.sessionID }
func (m *mockChatContext) AgentID() string                                   { return m.agentID }
func (m *mockChatContext) Events() <-chan entity.Event                       { return nil }
func (m *mockChatContext) Emit(event entity.Event)                           {}
func (m *mockChatContext) Close()                                            {}
func (m *mockChatContext) Closed() <-chan struct{}                           { return nil }
func (m *mockChatContext) Depth() int                                        { return 0 }
func (m *mockChatContext) Subscribe(fromSeq int64) (*iface.Subscriber, bool) { return nil, false }
func (m *mockChatContext) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
	return nil, nil
}
func (m *mockChatContext) ResolveInput(id string, resp *iface.InputResponse) error { return nil }
func (m *mockChatContext) PendingInputs() []iface.InputRequest { return nil }
func (m *mockChatContext) SetPartLocator(string, int, int)     {}
func (m *mockChatContext) ClearPartLocator()                   {}

func setupTestStore(t *testing.T) *filememory.FileMemoryStore {
	t.Helper()
	tmpDir := t.TempDir()
	store, err := filememory.NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)
	return store
}

func TestMemoryStoreTool_Name(t *testing.T) {
	store := setupTestStore(t)
	tool := NewMemoryStoreTool(store)
	assert.Equal(t, "memory_store", tool.Name())
}

func TestMemoryStoreTool_Execute(t *testing.T) {
	store := setupTestStore(t)
	tool := NewMemoryStoreTool(store)

	chatCtx := &mockChatContext{
		ctx:     context.Background(),
		agentID: "agent-1",
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
		ctx:     context.Background(),
		agentID: "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "content is required")
}

func TestMemoryRecallTool_Name(t *testing.T) {
	store := setupTestStore(t)
	tool := NewMemoryRecallTool(store)
	assert.Equal(t, "memory_recall", tool.Name())
}

func TestMemoryRecallTool_Execute(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()
	err := store.WriteFile(ctx, "agent-1", "knowledge/go-pref.md", "User prefers Go programming language", nil)
	require.NoError(t, err)

	tool := NewMemoryRecallTool(store)
	chatCtx := &mockChatContext{
		ctx:     ctx,
		agentID: "agent-1",
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

	tool := NewMemoryRecallTool(store)
	chatCtx := &mockChatContext{
		ctx:     ctx,
		agentID: "agent-1",
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
	tool := NewMemoryRecallTool(store)

	chatCtx := &mockChatContext{
		ctx:     context.Background(),
		agentID: "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "query is required")
}

func TestMemoryForgetTool_Name(t *testing.T) {
	store := setupTestStore(t)
	tool := NewMemoryForgetTool(store)
	assert.Equal(t, "memory_forget", tool.Name())
}

func TestMemoryForgetTool_Execute_ByPath(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()
	err := store.WriteFile(ctx, "agent-1", "knowledge/forget-me.md", "Temporary note", nil)
	require.NoError(t, err)

	tool := NewMemoryForgetTool(store)
	chatCtx := &mockChatContext{
		ctx:     ctx,
		agentID: "agent-1",
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
		ctx:     ctx,
		agentID: "agent-1",
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
		ctx:     context.Background(),
		agentID: "agent-1",
	}

	result, err := tool.Execute(chatCtx, map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "name' or 'path'")
}

func TestMemoryToolIntegration_StoreRecallForget(t *testing.T) {
	store := setupTestStore(t)
	chatCtx := &mockChatContext{
		ctx:     context.Background(),
		agentID: "agent-1",
	}

	storeTool := NewMemoryStoreTool(store)
	result, err := storeTool.Execute(chatCtx, map[string]any{
		"content":  "User likes dark mode",
		"category": "user",
		"name":     "ui-preference",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)

	recallTool := NewMemoryRecallTool(store)
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
