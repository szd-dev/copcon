package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockKnowledgeStore struct{}

var _ KnowledgeStore = (*mockKnowledgeStore)(nil)

func (m *mockKnowledgeStore) CreateKB(_ context.Context, kb *KnowledgeBase) (*KnowledgeBase, error) {
	return kb, nil
}

func (m *mockKnowledgeStore) DeleteKB(_ context.Context, id string) error {
	return nil
}

func (m *mockKnowledgeStore) ListKBs(_ context.Context) ([]*KnowledgeBase, error) {
	return nil, nil
}

func (m *mockKnowledgeStore) GetKB(_ context.Context, id string) (*KnowledgeBase, error) {
	return nil, nil
}

func (m *mockKnowledgeStore) IngestDocument(_ context.Context, kbID string, doc *Document, _ []byte) error {
	return nil
}

func (m *mockKnowledgeStore) ListDocuments(_ context.Context, kbID string) ([]*Document, error) {
	return nil, nil
}

func (m *mockKnowledgeStore) DeleteDocument(_ context.Context, kbID string, docID string) error {
	return nil
}

func (m *mockKnowledgeStore) GetDocument(_ context.Context, kbID string, docID string) (*Document, error) {
	return nil, nil
}

func (m *mockKnowledgeStore) GetChunks(_ context.Context, kbID string, docID string) ([]*Chunk, error) {
	return nil, nil
}

func (m *mockKnowledgeStore) UpdateChunk(_ context.Context, kbID string, chunk *Chunk) error {
	return nil
}

func (m *mockKnowledgeStore) Search(_ context.Context, kbIDs []string, query []float32, opts SearchOptions) ([]*Chunk, error) {
	return nil, nil
}

func TestKnowledgeStoreInterface(t *testing.T) {
	var store KnowledgeStore = &mockKnowledgeStore{}
	assert.NotNil(t, store)
}

func TestDocumentStatusConstants(t *testing.T) {
	assert.Equal(t, DocumentStatus("pending"), DocStatusPending)
	assert.Equal(t, DocumentStatus("parsing"), DocStatusParsing)
	assert.Equal(t, DocumentStatus("ready"), DocStatusReady)
	assert.Equal(t, DocumentStatus("error"), DocStatusError)
}

func TestKnowledgeStoreMethodSignatures(t *testing.T) {
	store := &mockKnowledgeStore{}
	ctx := context.Background()

	kb, err := store.CreateKB(ctx, &KnowledgeBase{})
	assert.NoError(t, err)
	assert.NotNil(t, kb)

	err = store.DeleteKB(ctx, "kb1")
	assert.NoError(t, err)

	kbs, err := store.ListKBs(ctx)
	assert.NoError(t, err)
	assert.Nil(t, kbs)

	fetched, err := store.GetKB(ctx, "kb1")
	assert.NoError(t, err)
	assert.Nil(t, fetched)

	err = store.IngestDocument(ctx, "kb1", &Document{}, []byte("content"))
	assert.NoError(t, err)

	docs, err := store.ListDocuments(ctx, "kb1")
	assert.NoError(t, err)
	assert.Nil(t, docs)

	err = store.DeleteDocument(ctx, "kb1", "doc1")
	assert.NoError(t, err)

	doc, err := store.GetDocument(ctx, "kb1", "doc1")
	assert.NoError(t, err)
	assert.Nil(t, doc)

	chunks, err := store.GetChunks(ctx, "kb1", "doc1")
	assert.NoError(t, err)
	assert.Nil(t, chunks)

	err = store.UpdateChunk(ctx, "kb1", &Chunk{})
	assert.NoError(t, err)

	results, err := store.Search(ctx, []string{"kb1"}, []float32{0.1, 0.2}, SearchOptions{TopK: 5})
	assert.NoError(t, err)
	assert.Nil(t, results)
}
