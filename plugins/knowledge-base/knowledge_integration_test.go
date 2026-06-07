package knowledgebase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
	"github.com/copcon/core/testutil"
	kbtestutil "github.com/copcon/plugins/knowledge-base/testutil"
)

type mockKBStore struct {
	chunks []*kbtypes.Chunk
}

func (m *mockKBStore) Search(ctx context.Context, kbIDs []string, query []float32, opts kbtypes.SearchOptions) ([]*kbtypes.Chunk, error) {
	return m.chunks, nil
}

type mockHookEmbedder = kbtestutil.MockEmbedder

func TestKBRecallHook_Metadata(t *testing.T) {
	h := NewKBRecallHook(nil, nil, nil)
	assert.Equal(t, "kb_recall", h.Name())
	assert.Equal(t, []hook.HookPoint{hook.AfterContextBuild}, h.Points())
	assert.Equal(t, 60, h.Priority())
}

func TestKBRecallHookInjectsResults(t *testing.T) {
	embedder := &mockHookEmbedder{Dims: 3}
	kbStore := &mockKBStore{
		chunks: []*kbtypes.Chunk{
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
	h := NewKBRecallHook(&mockHookEmbedder{Dims: 3}, &mockKBStore{}, map[string][]string{"test-agent": {"kb-1"}})
	ctx := &hook.HookContext{
		CurrentPoint: hook.AfterContextBuild,
		AgentID:      "test-agent",
		ChatCtx:      testutil.NewMockChatContext(context.Background(), "test-session", "test-agent"),
	}
	err := h.Execute(ctx)
	assert.NoError(t, err)
}

func TestKBRecallHookNoUserMessage(t *testing.T) {
	h := NewKBRecallHook(&mockHookEmbedder{Dims: 3}, &mockKBStore{}, map[string][]string{"test-agent": {"kb-1"}})
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
