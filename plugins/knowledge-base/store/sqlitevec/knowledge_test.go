package sqlitevec

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

func newTestStore(t *testing.T, dim int) *KnowledgeStore {
	t.Helper()
	db, err := gorm.Open(openDialector(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := NewKnowledgeStore(db, WithDimension(dim))
	require.NoError(t, err)
	return ks
}

func TestNewKnowledgeStore(t *testing.T) {
	db, err := gorm.Open(openDialector(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := NewKnowledgeStore(db, WithDimension(3))
	require.NoError(t, err)
	assert.NotNil(t, ks)
}

func TestVectorBlobRoundtrip(t *testing.T) {
	original := []float32{0.1, -0.5, 0.999, 0.0, -1.0}
	blob := toBlob(original)
	recovered := fromBlob(blob)
	assert.Equal(t, len(original), len(recovered))
	for i := range original {
		assert.InDelta(t, original[i], recovered[i], 1e-6)
	}
}

func TestVectorBlobEmpty(t *testing.T) {
	assert.Nil(t, toBlob(nil))
	assert.Nil(t, toBlob([]float32{}))
	assert.Nil(t, fromBlob(nil))
	assert.Nil(t, fromBlob([]byte{}))
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		expected float32
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{"orthogonal", []float32{1, 0, 0}, []float32{0, 1, 0}, 0.0},
		{"opposite", []float32{1, 0, 0}, []float32{-1, 0, 0}, -1.0},
		{"45 degrees", []float32{1, 0}, []float32{1, 1}, float32(1.0 / math.Sqrt(2))},
		{"empty", []float32{}, []float32{1}, 0.0},
		{"different length", []float32{1, 0}, []float32{1, 0, 0}, 0.0},
		{"zero vector", []float32{0, 0}, []float32{1, 1}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sim := cosineSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.expected, sim, 1e-5)
		})
	}
}

func TestKBLifecycle(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t, 3)

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
	ks := newTestStore(t, 3)
	err := ks.DeleteKB(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestDocumentCRUD(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t, 3)

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
	ks := newTestStore(t, 3)
	err := ks.DeleteDocument(ctx, "nonexistent", "nonexistent")
	assert.Error(t, err)
}

func TestChunkCRUD(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t, 3)

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
	ks := newTestStore(t, 2)

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
	ks := newTestStore(t, 3)

	kb, _ := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "status-kb", Backend: "sqlite-vec"})
	doc := &kbtypes.Document{KBID: kb.ID, Filename: "doc.txt", Source: "upload", Status: kbtypes.DocStatusPending}
	ks.IngestDocument(ctx, kb.ID, doc, nil)

	err := ks.UpdateDocumentStatus(ctx, kb.ID, doc.ID, kbtypes.DocStatusError)
	require.NoError(t, err)

	got, _ := ks.GetDocument(ctx, kb.ID, doc.ID)
	assert.Equal(t, kbtypes.DocStatusError, got.Status)
}

func TestSearch(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t, 3)

	kb, _ := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "search-kb", Backend: "sqlite-vec"})
	doc := &kbtypes.Document{KBID: kb.ID, Filename: "doc.txt", Source: "upload", Status: kbtypes.DocStatusPending}
	ks.IngestDocument(ctx, kb.ID, doc, nil)

	chunks := []*kbtypes.Chunk{
		{DocumentID: doc.ID, KBID: kb.ID, Content: "about cats", Index: 0},
		{DocumentID: doc.ID, KBID: kb.ID, Content: "about dogs", Index: 1},
		{DocumentID: doc.ID, KBID: kb.ID, Content: "about birds", Index: 2},
	}
	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
	}
	ks.StoreChunks(ctx, kb.ID, doc.ID, chunks, vectors)

	results, err := ks.Search(ctx, []string{kb.ID}, []float32{1.0, 0.1, 0.0}, kbtypes.SearchOptions{TopK: 2})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "about cats", results[0].Content)
	assert.True(t, results[0].Score > results[1].Score)
}

func TestSearchWithThreshold(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t, 2)

	kb, _ := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "threshold-kb", Backend: "sqlite-vec"})
	doc := &kbtypes.Document{KBID: kb.ID, Filename: "doc.txt", Source: "upload", Status: kbtypes.DocStatusPending}
	ks.IngestDocument(ctx, kb.ID, doc, nil)

	chunks := []*kbtypes.Chunk{
		{DocumentID: doc.ID, KBID: kb.ID, Content: "close match", Index: 0},
		{DocumentID: doc.ID, KBID: kb.ID, Content: "far match", Index: 1},
	}
	vectors := [][]float32{
		{1.0, 0.0},
		{0.0, 1.0},
	}
	ks.StoreChunks(ctx, kb.ID, doc.ID, chunks, vectors)

	results, err := ks.Search(ctx, []string{kb.ID}, []float32{1.0, 0.0}, kbtypes.SearchOptions{
		TopK:                10,
		SimilarityThreshold: 0.99,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "close match", results[0].Content)
}

func TestSearchEmptyQuery(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t, 3)
	results, err := ks.Search(ctx, []string{"kb1"}, []float32{}, kbtypes.SearchOptions{})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestSearchCrossKB(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t, 2)

	kb1, _ := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "kb1", Backend: "sqlite-vec"})
	kb2, _ := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "kb2", Backend: "sqlite-vec"})

	doc1 := &kbtypes.Document{KBID: kb1.ID, Filename: "doc1.txt", Source: "upload", Status: kbtypes.DocStatusPending}
	ks.IngestDocument(ctx, kb1.ID, doc1, nil)
	doc2 := &kbtypes.Document{KBID: kb2.ID, Filename: "doc2.txt", Source: "upload", Status: kbtypes.DocStatusPending}
	ks.IngestDocument(ctx, kb2.ID, doc2, nil)

	ks.StoreChunks(ctx, kb1.ID, doc1.ID, []*kbtypes.Chunk{
		{DocumentID: doc1.ID, KBID: kb1.ID, Content: "kb1 content", Index: 0},
	}, [][]float32{{1.0, 0.0}})
	ks.StoreChunks(ctx, kb2.ID, doc2.ID, []*kbtypes.Chunk{
		{DocumentID: doc2.ID, KBID: kb2.ID, Content: "kb2 content", Index: 0},
	}, [][]float32{{0.0, 1.0}})

	results, err := ks.Search(ctx, []string{kb1.ID, kb2.ID}, []float32{1.0, 0.5}, kbtypes.SearchOptions{TopK: 10})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestStoreChunksCountMismatch(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t, 1)

	err := ks.StoreChunks(ctx, "kb1", "doc1", []*kbtypes.Chunk{{}}, [][]float32{{1.0}, {2.0}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "count")
}

func TestDeleteKBCascades(t *testing.T) {
	ctx := context.Background()
	ks := newTestStore(t, 1)

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
