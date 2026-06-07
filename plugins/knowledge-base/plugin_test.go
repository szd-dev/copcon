package knowledgebase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/plugin"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

type mockFullStore struct {
	mockKBStore
}

func (m *mockFullStore) CreateKB(_ context.Context, _ *kbtypes.KnowledgeBase) (*kbtypes.KnowledgeBase, error) {
	return nil, nil
}
func (m *mockFullStore) DeleteKB(_ context.Context, _ string) error                   { return nil }
func (m *mockFullStore) ListKBs(_ context.Context) ([]*kbtypes.KnowledgeBase, error)   { return nil, nil }
func (m *mockFullStore) GetKB(_ context.Context, _ string) (*kbtypes.KnowledgeBase, error) {
	return nil, nil
}
func (m *mockFullStore) IngestDocument(_ context.Context, _ string, _ *kbtypes.Document, _ []byte) error {
	return nil
}
func (m *mockFullStore) ListDocuments(_ context.Context, _ string) ([]*kbtypes.Document, error) {
	return nil, nil
}
func (m *mockFullStore) DeleteDocument(_ context.Context, _, _ string) error { return nil }
func (m *mockFullStore) GetDocument(_ context.Context, _, _ string) (*kbtypes.Document, error) {
	return nil, nil
}
func (m *mockFullStore) UpdateDocumentStatus(_ context.Context, _, _ string, _ kbtypes.DocumentStatus) error {
	return nil
}
func (m *mockFullStore) UpdateDocumentErrorMsg(_ context.Context, _, _ string, _ string) error {
	return nil
}
func (m *mockFullStore) ListDocumentsByStatus(_ context.Context, _ []string) ([]*kbtypes.Document, error) {
	return nil, nil
}
func (m *mockFullStore) ClaimDocumentStatus(_ context.Context, _ string, _, _ string) (bool, error) {
	return false, nil
}
func (m *mockFullStore) StoreChunks(_ context.Context, _, _ string, _ []*kbtypes.Chunk, _ [][]float32) error {
	return nil
}
func (m *mockFullStore) GetChunks(_ context.Context, _, _ string) ([]*kbtypes.Chunk, error) {
	return nil, nil
}
func (m *mockFullStore) UpdateChunk(_ context.Context, _ string, _ *kbtypes.Chunk) error { return nil }

var _ KnowledgeStore = (*mockFullStore)(nil)

func TestPluginName(t *testing.T) {
	p := NewPlugin(nil, nil)
	assert.Equal(t, "knowledge", p.Name())
}

func TestPluginToolsIsEmpty(t *testing.T) {
	p := NewPlugin(nil, nil)
	assert.Nil(t, p.Tools())
}

func TestPluginHooksCount(t *testing.T) {
	p := NewPlugin(nil, nil)
	assert.Len(t, p.Hooks(), 1)
}

func TestPluginHookName(t *testing.T) {
	p := NewPlugin(nil, nil)
	hooks := p.Hooks()
	assert.Equal(t, "knowledge.hook.kb_recall", hooks[0].Name())
}

func TestPluginInitInjectsAgentKBs(t *testing.T) {
	emb := &mockHookEmbedder{Dims: 3}
	ks := &mockFullStore{}
	p := NewPlugin(ks, emb)

	agentKBs := map[string][]string{
		"agent-1": {"kb-1", "kb-2"},
	}
	err := p.Init(plugin.PluginDeps{AgentKnowledgeBases: agentKBs})
	require.NoError(t, err)

	hooks := p.Hooks()
	require.Len(t, hooks, 1)
	assert.Equal(t, "knowledge.hook.kb_recall", hooks[0].Name())
}

func TestPluginGetStore(t *testing.T) {
	ks := &mockFullStore{}
	p := NewPlugin(ks, nil)
	kp := p.(*kbPlugin)
	assert.Equal(t, ks, kp.GetStore())
}

func TestPluginGetEmbedder(t *testing.T) {
	emb := &mockHookEmbedder{Dims: 3}
	p := NewPlugin(nil, emb)
	kp := p.(*kbPlugin)
	assert.Equal(t, emb, kp.GetEmbedder())
}

func TestPluginGetStoreNil(t *testing.T) {
	p := NewPlugin(nil, nil)
	kp := p.(*kbPlugin)
	assert.Nil(t, kp.GetStore())
}

func TestPluginGetEmbedderNil(t *testing.T) {
	p := NewPlugin(nil, nil)
	kp := p.(*kbPlugin)
	assert.Nil(t, kp.GetEmbedder())
}
