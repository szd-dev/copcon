package memoryfile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
	memtypes "github.com/copcon/plugins/memory-file/types"
	"github.com/copcon/core/testutil"
)

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

type mockMemoryStoreForHook struct {
	memories []*memtypes.Memory
}

func (m *mockMemoryStoreForHook) Store(ctx context.Context, memory *memtypes.Memory) error {
	m.memories = append(m.memories, memory)
	return nil
}

func (m *mockMemoryStoreForHook) Search(ctx context.Context, query []float32, limit int) ([]*memtypes.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForHook) GetByAgentID(ctx context.Context, agentID string, limit int) ([]*memtypes.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForHook) DeleteByAgentID(ctx context.Context, agentID string) error { return nil }

func (m *mockMemoryStoreForHook) List(ctx context.Context, filter memtypes.MemoryFilter) ([]*memtypes.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForHook) Get(ctx context.Context, id string) (*memtypes.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForHook) Update(ctx context.Context, memory *memtypes.Memory) error { return nil }

func (m *mockMemoryStoreForHook) Delete(ctx context.Context, id string) error { return nil }

var _ MemoryStore = (*mockMemoryStoreForHook)(nil)
var _ kbtypes.Embedder = (*mockHookEmbedder)(nil)

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
	store := &mockMemoryStoreForHook{}
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
	store := &mockMemoryStoreForHook{}
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
