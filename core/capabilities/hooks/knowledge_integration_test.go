package hooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/providers/embedding"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/testutil"
)

type mockKBStore struct {
	chunks []*storage.Chunk
}

func (m *mockKBStore) Search(ctx context.Context, kbIDs []string, query []float32, opts storage.SearchOptions) ([]*storage.Chunk, error) {
	return m.chunks, nil
}

type mockHookEmbedder struct {
	dimensions int
}

func (m *mockHookEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vec := make([]float32, m.dimensions)
	vec[0] = 0.5
	return vec, nil
}

func (m *mockHookEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.dimensions)
		vec[0] = 0.5
		results[i] = vec
	}
	return results, nil
}

func (m *mockHookEmbedder) Dimensions() int { return m.dimensions }
func (m *mockHookEmbedder) Name() string    { return "mock" }

func TestKBRecallHookName(t *testing.T) {
	h := NewKBRecallHook(nil, nil, nil)
	assert.Equal(t, "kb_recall", h.Name())
}

func TestKBRecallHookPoints(t *testing.T) {
	h := NewKBRecallHook(nil, nil, nil)
	assert.Equal(t, []hook.HookPoint{hook.AfterContextBuild}, h.Points())
}

func TestKBRecallHookPriority(t *testing.T) {
	h := NewKBRecallHook(nil, nil, nil)
	assert.Equal(t, 60, h.Priority())
}

func TestKBRecallHookInjectsResults(t *testing.T) {
	embedder := &mockHookEmbedder{dimensions: 3}
	kbStore := &mockKBStore{
		chunks: []*storage.Chunk{
			{Content: "relevant chunk", Score: 0.9},
		},
	}
	h := NewKBRecallHook(embedder, kbStore, map[string][]string{"test-agent": {"kb-1"}})

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "what is Go?"},
	}
	ctx := &hook.HookContext{
		Messages:     &messages,
		SessionID:    "test-session",
		AgentID:      "test-agent",
		CurrentPoint: hook.AfterContextBuild,
		ChatCtx:      testutil.NewMockChatContext(context.Background(), "test-session", "test-agent"),
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, "system", messages[0].Role)
	assert.Contains(t, messages[0].Content, "relevant chunk")
}

func TestKBRecallHookNilMessages(t *testing.T) {
	h := NewKBRecallHook(&mockHookEmbedder{dimensions: 3}, &mockKBStore{}, map[string][]string{"test-agent": {"kb-1"}})
	ctx := &hook.HookContext{
		CurrentPoint: hook.AfterContextBuild,
		AgentID:      "test-agent",
		ChatCtx:      testutil.NewMockChatContext(context.Background(), "test-session", "test-agent"),
	}
	err := h.Execute(ctx)
	assert.NoError(t, err)
}

func TestKBRecallHookNoUserMessage(t *testing.T) {
	h := NewKBRecallHook(&mockHookEmbedder{dimensions: 3}, &mockKBStore{}, map[string][]string{"test-agent": {"kb-1"}})
	messages := []entity.MessageForLLM{{Role: "system", Content: "sys"}}
	ctx := &hook.HookContext{
		Messages:     &messages,
		CurrentPoint: hook.AfterContextBuild,
		AgentID:      "test-agent",
		ChatCtx:      testutil.NewMockChatContext(context.Background(), "test-session", "test-agent"),
	}
	err := h.Execute(ctx)
	assert.NoError(t, err)
	assert.Len(t, messages, 1)
}

func TestKBRecallHookNoDependencies(t *testing.T) {
	h := NewKBRecallHook(nil, nil, nil)
	messages := []entity.MessageForLLM{{Role: "user", Content: "test"}}
	ctx := &hook.HookContext{
		Messages:     &messages,
		CurrentPoint: hook.AfterContextBuild,
		ChatCtx:      testutil.NewMockChatContext(context.Background(), "test-session", "test-agent"),
	}
	err := h.Execute(ctx)
	assert.NoError(t, err)
	assert.Len(t, messages, 1)
}

type mockMemoryStore struct {
	memories []*storage.Memory
}

func (m *mockMemoryStore) Store(ctx context.Context, memory *storage.Memory) error {
	m.memories = append(m.memories, memory)
	return nil
}

func (m *mockMemoryStore) Search(ctx context.Context, query []float32, limit int) ([]*storage.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) GetBySession(ctx context.Context, sessionID string, limit int) ([]*storage.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) DeleteBySession(ctx context.Context, sessionID string) error { return nil }
func (m *mockMemoryStore) List(ctx context.Context, filter storage.MemoryFilter) ([]*storage.Memory, error) {
	return nil, nil
}
func (m *mockMemoryStore) Get(ctx context.Context, id string) (*storage.Memory, error) {
	return nil, nil
}
func (m *mockMemoryStore) Update(ctx context.Context, memory *storage.Memory) error { return nil }
func (m *mockMemoryStore) Delete(ctx context.Context, id string) error              { return nil }

func TestMemoryPersistHookName(t *testing.T) {
	h := NewMemoryPersistHook(nil, nil)
	assert.Equal(t, "memory_persist", h.Name())
}

func TestMemoryPersistHookPoints(t *testing.T) {
	h := NewMemoryPersistHook(nil, nil)
	assert.Equal(t, []hook.HookPoint{hook.OnMessagePersist}, h.Points())
}

func TestMemoryPersistHookPriority(t *testing.T) {
	h := NewMemoryPersistHook(nil, nil)
	assert.Equal(t, 40, h.Priority())
}

func TestMemoryPersistHookStoresAssistantMessage(t *testing.T) {
	embedder := &mockHookEmbedder{dimensions: 3}
	store := &mockMemoryStore{}
	h := NewMemoryPersistHook(embedder, store)

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "Hello! How can I help you today?"},
	}
	ctx := &hook.HookContext{
		Messages:     &messages,
		SessionID:    "test-session",
		CurrentPoint: hook.OnMessagePersist,
		ChatCtx:      testutil.NewMockChatContext(context.Background(), "test-session", "test-agent"),
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
}

func TestMemoryPersistHookNoAssistantMessage(t *testing.T) {
	embedder := &mockHookEmbedder{dimensions: 3}
	store := &mockMemoryStore{}
	h := NewMemoryPersistHook(embedder, store)

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "hello"},
	}
	ctx := &hook.HookContext{
		Messages:     &messages,
		SessionID:    "test-session",
		CurrentPoint: hook.OnMessagePersist,
		ChatCtx:      testutil.NewMockChatContext(context.Background(), "test-session", "test-agent"),
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Empty(t, store.memories)
}

func TestMemoryPersistHookNilDeps(t *testing.T) {
	h := NewMemoryPersistHook(nil, nil)
	messages := []entity.MessageForLLM{{Role: "assistant", Content: "test"}}
	ctx := &hook.HookContext{
		Messages:     &messages,
		CurrentPoint: hook.OnMessagePersist,
		ChatCtx:      testutil.NewMockChatContext(context.Background(), "test-session", "test-agent"),
	}
	err := h.Execute(ctx)
	assert.NoError(t, err)
}

func TestExtractKeywords(t *testing.T) {
	keywords := extractKeywords("The quick brown fox jumps over the lazy dog and runs quickly", 3)
	assert.NotEmpty(t, keywords)
	assert.LessOrEqual(t, len(keywords), 3)
	for _, kw := range keywords {
		assert.Greater(t, len(kw), 1)
		assert.False(t, englishStopWords[kw])
	}
}

func TestExtractKeywordsChinese(t *testing.T) {
	keywords := extractKeywords("人工智能技术发展迅速 机器学习应用广泛", 3)
	assert.NotEmpty(t, keywords)
}

func TestExtractKeywordsEmpty(t *testing.T) {
	keywords := extractKeywords("", 5)
	assert.Empty(t, keywords)
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("Hello, World! This is a test.")
	assert.Contains(t, tokens, "hello")
	assert.Contains(t, tokens, "world")
	assert.Contains(t, tokens, "test")
}

func TestIsStopWord(t *testing.T) {
	assert.True(t, isStopWord("the"))
	assert.True(t, isStopWord("的"))
	assert.False(t, isStopWord("golang"))
	assert.True(t, isStopWord("a"))
	assert.True(t, isStopWord("x"))
}

var _ embedding.Embedder = (*mockHookEmbedder)(nil)
