package sqlitevec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	knowledgebase "github.com/copcon/plugins/knowledge-base"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

type mockVectorStore struct{}

func (m *mockVectorStore) Store(ctx context.Context, kbID, docID string, chunks []knowledgebase.VectorChunk, vectors [][]float32) error {
	return nil
}
func (m *mockVectorStore) Search(ctx context.Context, kbIDs []string, query []float32, opts kbtypes.SearchOptions) ([]knowledgebase.SearchResult, error) {
	return nil, nil
}
func (m *mockVectorStore) DeleteByKB(ctx context.Context, kbID string) error        { return nil }
func (m *mockVectorStore) DeleteByDocument(ctx context.Context, kbID, docID string) error { return nil }
func (m *mockVectorStore) Backend() string                                            { return "mock" }
func (m *mockVectorStore) Verify(ctx context.Context) (map[string]int, error)         { return nil, nil }

var _ knowledgebase.VectorStore = (*mockVectorStore)(nil)

func newTestStore(t *testing.T) *KnowledgeStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := NewKnowledgeStore(db, &mockVectorStore{})
	require.NoError(t, err)
	return ks
}

func TestNewKnowledgeStore(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := NewKnowledgeStore(db, &mockVectorStore{})
	require.NoError(t, err)
	assert.NotNil(t, ks)
}

func TestKBLifecycle(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t)

	kb, err := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{
		Name:    "test-kb",
		Backend: "sqlite-vec",
		Config:  map[string]any{"key": "value"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, kb.ID)
	assert.Equal(t, "test-kb", kb.Name)
	assert.Equal(t, "sqlite-vec", kb.Backend)

	got, err := ks.GetKB(ctx, kb.ID)
	require.NoError(t, err)
	assert.Equal(t, kb.Name, got.Name)

	list, err := ks.ListKBs(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	err = ks.DeleteKB(ctx, kb.ID)
	require.NoError(t, err)

	_, err = ks.GetKB(ctx, kb.ID)
	assert.Error(t, err)
}

func TestDeleteKBNotFound(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t)
	err := ks.DeleteKB(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestDocumentCRUD(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t)

	kb, err := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "docs-kb", Backend: "sqlite-vec"})
	require.NoError(t, err)

	doc := &kbtypes.Document{
		KBID:     kb.ID,
		Filename: "test.txt",
		Source:   "upload",
		Status:   kbtypes.DocStatusPending,
	}
	err = ks.IngestDocument(ctx, kb.ID, doc, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, doc.ID)

	got, err := ks.GetDocument(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "test.txt", got.Filename)
	assert.Equal(t, kbtypes.DocStatusPending, got.Status)

	list, err := ks.ListDocuments(ctx, kb.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	err = ks.DeleteDocument(ctx, kb.ID, doc.ID)
	require.NoError(t, err)

	_, err = ks.GetDocument(ctx, kb.ID, doc.ID)
	assert.Error(t, err)
}

func TestDeleteDocumentNotFound(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t)
	err := ks.DeleteDocument(ctx, "nonexistent", "nonexistent")
	assert.Error(t, err)
}

func TestChunkCRUD(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t)

	kb, err := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "chunks-kb", Backend: "sqlite-vec"})
	require.NoError(t, err)

	doc := &kbtypes.Document{
		KBID:     kb.ID,
		Filename: "doc.txt",
		Source:   "upload",
		Status:   kbtypes.DocStatusPending,
	}
	err = ks.IngestDocument(ctx, kb.ID, doc, nil)
	require.NoError(t, err)

	chunks := []*kbtypes.Chunk{
		{DocumentID: doc.ID, KBID: kb.ID, Content: "chunk 0", Index: 0, TokenCount: 2},
		{DocumentID: doc.ID, KBID: kb.ID, Content: "chunk 1", Index: 1, TokenCount: 2},
	}
	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
	}

	err = ks.StoreChunks(ctx, kb.ID, doc.ID, chunks, vectors)
	require.NoError(t, err)

	got, err := ks.GetChunks(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, "chunk 0", got[0].Content)
	assert.Equal(t, 0, got[0].Index)

	gotDoc, err := ks.GetDocument(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, kbtypes.DocStatusReady, gotDoc.Status)
	assert.Equal(t, 2, gotDoc.ChunkCount)
}

func TestUpdateChunk(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t)

	kb, _ := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "update-kb", Backend: "sqlite-vec"})
	doc := &kbtypes.Document{KBID: kb.ID, Filename: "doc.txt", Source: "upload", Status: kbtypes.DocStatusPending}
	ks.IngestDocument(ctx, kb.ID, doc, nil)

	chunks := []*kbtypes.Chunk{
		{DocumentID: doc.ID, KBID: kb.ID, Content: "original", Index: 0},
	}
	vectors := [][]float32{{1.0, 0.0}}
	ks.StoreChunks(ctx, kb.ID, doc.ID, chunks, vectors)

	got, _ := ks.GetChunks(ctx, kb.ID, doc.ID)
	got[0].Content = "updated"
	err := ks.UpdateChunk(ctx, kb.ID, got[0])
	require.NoError(t, err)

	got2, _ := ks.GetChunks(ctx, kb.ID, doc.ID)
	assert.Equal(t, "updated", got2[0].Content)
}

func TestUpdateDocumentStatus(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t)

	kb, _ := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "status-kb", Backend: "sqlite-vec"})
	doc := &kbtypes.Document{KBID: kb.ID, Filename: "doc.txt", Source: "upload", Status: kbtypes.DocStatusPending}
	ks.IngestDocument(ctx, kb.ID, doc, nil)

	err := ks.UpdateDocumentStatus(ctx, kb.ID, doc.ID, kbtypes.DocStatusError)
	require.NoError(t, err)

	got, _ := ks.GetDocument(ctx, kb.ID, doc.ID)
	assert.Equal(t, kbtypes.DocStatusError, got.Status)
}

func TestStoreChunksCountMismatch(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t)

	err := ks.StoreChunks(ctx, "kb1", "doc1", []*kbtypes.Chunk{{}}, [][]float32{{1.0}, {2.0}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "count")
}

func TestDeleteKBCascades(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t)

	kb, _ := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "cascade-kb", Backend: "sqlite-vec"})
	doc := &kbtypes.Document{KBID: kb.ID, Filename: "doc.txt", Source: "upload", Status: kbtypes.DocStatusPending}
	ks.IngestDocument(ctx, kb.ID, doc, nil)
	ks.StoreChunks(ctx, kb.ID, doc.ID, []*kbtypes.Chunk{
		{DocumentID: doc.ID, KBID: kb.ID, Content: "chunk", Index: 0},
	}, [][]float32{{1.0}})

	err := ks.DeleteKB(ctx, kb.ID)
	require.NoError(t, err)

	chunks, err := ks.GetChunks(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Empty(t, chunks)
}