package memoryfile

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/plugins/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLLMProvider struct {
	response string
	err      error
}

func (m *mockLLMProvider) Stream(ctx context.Context, params llm.StreamParams) (<-chan llm.StreamChunk, <-chan error) {
	ch := make(chan llm.StreamChunk, 1)
	errc := make(chan error, 1)
	go func() {
		defer close(ch)
		defer close(errc)
		if m.err != nil {
			errc <- m.err
			return
		}
		ch <- llm.StreamChunk{Content: m.response}
	}()
	return ch, errc
}

func setupRecallHookStore(t *testing.T, tmpDir, agentID string) *FileMemoryStore {
	t.Helper()
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)
	return store
}

func writeMemoryFile(t *testing.T, tmpDir, agentID, subdir, name, sessionID, description, body string) {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	fm := Frontmatter{
		Name:        name,
		Category:    subdir,
		SessionID:   sessionID,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	data := SerializeFrontmatter(fm, body)
	require.NoError(t, WriteFileWithPerms(filepath.Join(tmpDir, agentID, subdir, name+".md"), data))
}

func TestMemoryRecallHook_SelectsRelevant(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-recall"
	store := setupRecallHookStore(t, tmpDir, agentID)

	writeMemoryFile(t, tmpDir, agentID, "knowledge", "user-prefs", "sess-1", "User preferences", "Dark mode preferred")

	// Build the index so the hook can read it.
	require.NoError(t, BuildIndex(tmpDir, agentID, 200, 25*1024))

	// Mock LLM returns JSON selecting the memory file.
	selectedPaths := []string{"knowledge/user-prefs.md"}
	mockResp, _ := json.Marshal(selectedPaths)
	mockLLM := &mockLLMProvider{response: string(mockResp)}

	h := NewMemoryRecallHook(store, mockLLM)

	msgs := []entity.MessageForLLM{
		{Role: "user", Content: "What are my preferences?"},
	}
	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: "sess-current",
		Messages:  &msgs,
		ChatCtx:   &mockChatCtx{},
	}

	err := h.Execute(ctx)
	require.NoError(t, err)

	// Verify injected messages: should prepend recalled memory before original messages.
	assert.GreaterOrEqual(t, len(*ctx.Messages), 2)
	assert.Equal(t, "system", (*ctx.Messages)[0].Role)
	assert.Contains(t, (*ctx.Messages)[0].Content, "[Recalled Memory:")
	assert.Contains(t, (*ctx.Messages)[0].Content, "knowledge/user-prefs.md")
	assert.Contains(t, (*ctx.Messages)[0].Content, "Dark mode preferred")
}

func TestMemoryRecallHook_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-badjson"
	store := setupRecallHookStore(t, tmpDir, agentID)

	writeMemoryFile(t, tmpDir, agentID, "knowledge", "test-fact", "sess-1", "Test", "Body")
	require.NoError(t, BuildIndex(tmpDir, agentID, 200, 25*1024))

	mockLLM := &mockLLMProvider{response: "not json at all"}
	h := NewMemoryRecallHook(store, mockLLM)

	msgs := []entity.MessageForLLM{
		{Role: "user", Content: "Hello"},
	}
	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: "sess-badjson",
		Messages:  &msgs,
		ChatCtx:   &mockChatCtx{},
	}

	err := h.Execute(ctx)
	require.NoError(t, err)

	assert.Len(t, *ctx.Messages, 1)
	assert.Equal(t, "user", (*ctx.Messages)[0].Role)
}

func TestMemoryRecallHook_EmptyIndex(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-empty"
	store := setupRecallHookStore(t, tmpDir, agentID)

	mockLLM := &mockLLMProvider{response: `["knowledge/fake.md"]`}
	h := NewMemoryRecallHook(store, mockLLM)

	msgs := []entity.MessageForLLM{
		{Role: "user", Content: "Hello"},
	}
	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: "sess-empty",
		Messages:  &msgs,
		ChatCtx:   &mockChatCtx{},
	}

	err := h.Execute(ctx)
	require.NoError(t, err)

	// No index → no injection.
	assert.Len(t, *ctx.Messages, 1)
}

func TestMemoryRecallHook_Dedup(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-dedup"
	store := setupRecallHookStore(t, tmpDir, agentID)

	writeMemoryFile(t, tmpDir, agentID, "knowledge", "dedup-fact", "sess-1", "Dedup test", "Body content")
	require.NoError(t, BuildIndex(tmpDir, agentID, 200, 25*1024))

	selectedPaths := []string{"knowledge/dedup-fact.md"}
	mockResp, _ := json.Marshal(selectedPaths)
	mockLLM := &mockLLMProvider{response: string(mockResp)}

	h := NewMemoryRecallHook(store, mockLLM)

	sessionID := "sess-dedup"

	// First call: injects the memory.
	msgs1 := []entity.MessageForLLM{
		{Role: "user", Content: "First call"},
	}
	ctx1 := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		Messages:  &msgs1,
		ChatCtx:   &mockChatCtx{},
	}
	require.NoError(t, h.Execute(ctx1))
	assert.GreaterOrEqual(t, len(*ctx1.Messages), 2)

	// Second call: same path should be deduped.
	msgs2 := []entity.MessageForLLM{
		{Role: "user", Content: "Second call"},
	}
	ctx2 := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		Messages:  &msgs2,
		ChatCtx:   &mockChatCtx{},
	}
	require.NoError(t, h.Execute(ctx2))
	assert.Len(t, *ctx2.Messages, 1)
}

func TestMemoryRecallHook_NoUserMessage(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-nouser"
	store := setupRecallHookStore(t, tmpDir, agentID)

	writeMemoryFile(t, tmpDir, agentID, "knowledge", "some-fact", "sess-1", "Desc", "Body")
	require.NoError(t, BuildIndex(tmpDir, agentID, 200, 25*1024))

	mockLLM := &mockLLMProvider{response: `["knowledge/some-fact.md"]`}
	h := NewMemoryRecallHook(store, mockLLM)

	// Only system messages, no user message.
	msgs := []entity.MessageForLLM{
		{Role: "system", Content: "System prompt"},
	}
	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: "sess-nouser",
		Messages:  &msgs,
		ChatCtx:   &mockChatCtx{},
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, *ctx.Messages, 1)
}

func TestMemoryRecallHook_NilLLM(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-nilllm"
	store := setupRecallHookStore(t, tmpDir, agentID)

	h := NewMemoryRecallHook(store, nil)

	msgs := []entity.MessageForLLM{
		{Role: "user", Content: "Hello"},
	}
	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: "sess-nil",
		Messages:  &msgs,
		ChatCtx:   &mockChatCtx{},
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, *ctx.Messages, 1)
}

func TestMemoryRecallHook_EmptyAgentID(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	mockLLM := &mockLLMProvider{response: `[]`}
	h := NewMemoryRecallHook(store, mockLLM)

	msgs := []entity.MessageForLLM{
		{Role: "user", Content: "Hello"},
	}
	ctx := &hook.HookContext{
		AgentID:   "",
		SessionID: "sess-noagent",
		Messages:  &msgs,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, *ctx.Messages, 1)
}

func TestMemoryRecallHook_LLMError(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-llmerr"
	store := setupRecallHookStore(t, tmpDir, agentID)

	writeMemoryFile(t, tmpDir, agentID, "knowledge", "err-fact", "sess-1", "Desc", "Body")
	require.NoError(t, BuildIndex(tmpDir, agentID, 200, 25*1024))

	mockLLM := &mockLLMProvider{err: context.DeadlineExceeded}
	h := NewMemoryRecallHook(store, mockLLM)

	msgs := []entity.MessageForLLM{
		{Role: "user", Content: "Hello"},
	}
	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: "sess-llmerr",
		Messages:  &msgs,
		ChatCtx:   &mockChatCtx{},
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, *ctx.Messages, 1)
}

func TestMemoryRecallHook_EmptyResponse(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-emptyresp"
	store := setupRecallHookStore(t, tmpDir, agentID)

	writeMemoryFile(t, tmpDir, agentID, "knowledge", "empty-fact", "sess-1", "Desc", "Body")
	require.NoError(t, BuildIndex(tmpDir, agentID, 200, 25*1024))

	mockLLM := &mockLLMProvider{response: `[]`}
	h := NewMemoryRecallHook(store, mockLLM)

	msgs := []entity.MessageForLLM{
		{Role: "user", Content: "Hello"},
	}
	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: "sess-emptyresp",
		Messages:  &msgs,
		ChatCtx:   &mockChatCtx{},
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, *ctx.Messages, 1)
}

func TestMemoryRecallHook_CapsAtFive(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-cap5"
	store := setupRecallHookStore(t, tmpDir, agentID)

	for i := 0; i < 7; i++ {
		name := []string{"a", "b", "c", "d", "e", "f", "g"}[i]
		writeMemoryFile(t, tmpDir, agentID, "knowledge", "fact-"+name, "sess-1", "Desc "+name, "Body "+name)
	}
	require.NoError(t, BuildIndex(tmpDir, agentID, 200, 25*1024))

	// LLM returns 7 paths.
	selectedPaths := []string{
		"knowledge/fact-a.md", "knowledge/fact-b.md", "knowledge/fact-c.md",
		"knowledge/fact-d.md", "knowledge/fact-e.md", "knowledge/fact-f.md",
		"knowledge/fact-g.md",
	}
	mockResp, _ := json.Marshal(selectedPaths)
	mockLLM := &mockLLMProvider{response: string(mockResp)}

	h := NewMemoryRecallHook(store, mockLLM)

	msgs := []entity.MessageForLLM{
		{Role: "user", Content: "Give me everything"},
	}
	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: "sess-cap5",
		Messages:  &msgs,
		ChatCtx:   &mockChatCtx{},
	}

	err := h.Execute(ctx)
	require.NoError(t, err)

	// 5 injected + 1 original = 6.
	assert.Len(t, *ctx.Messages, 6)
}

func TestMemoryRecallHook_Metadata(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	h := NewMemoryRecallHook(store, nil)
	assert.Equal(t, "memory_recall", h.Name())
	assert.Equal(t, 70, h.Priority())
	assert.Equal(t, []hook.HookPoint{hook.AfterContextBuild}, h.Points())
}

type mockChatCtx = testutil.MockChatContext
