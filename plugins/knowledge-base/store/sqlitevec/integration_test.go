package sqlitevec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/copcon/core/storage"
)

func TestIntegrationKBLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := 	gorm.Open(openDialector(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := NewKnowledgeStore(db)
	require.NoError(t, err)

	kb, err := ks.CreateKB(ctx, &storage.KnowledgeBase{
		Name:    "integration-kb",
		Backend: "sqlite-vec",
		Config:  map[string]any{"purpose": "integration test"},
	})
	require.NoError(t, err)

	doc := &storage.Document{
		KBID:     kb.ID,
		Filename: "test.txt",
		Source:   "upload",
		Status:   storage.DocStatusPending,
	}
	err = ks.IngestDocument(ctx, kb.ID, doc, []byte("test content"))
	require.NoError(t, err)

	chunks := []*storage.Chunk{
		{DocumentID: doc.ID, KBID: kb.ID, Content: "Go is a programming language", Index: 0, TokenCount: 6},
		{DocumentID: doc.ID, KBID: kb.ID, Content: "Rust is a systems language", Index: 1, TokenCount: 5},
		{DocumentID: doc.ID, KBID: kb.ID, Content: "Python is a scripting language", Index: 2, TokenCount: 5},
	}
	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
	}

	err = ks.StoreChunks(ctx, kb.ID, doc.ID, chunks, vectors)
	require.NoError(t, err)

	gotDoc, err := ks.GetDocument(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.DocStatusReady, gotDoc.Status)
	assert.Equal(t, 3, gotDoc.ChunkCount)

	gotChunks, err := ks.GetChunks(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Len(t, gotChunks, 3)
	assert.Equal(t, "Go is a programming language", gotChunks[0].Content)

	results, err := ks.Search(ctx, []string{kb.ID}, []float32{1.0, 0.1, 0.0}, storage.SearchOptions{TopK: 2})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "Go is a programming language", results[0].Content)
	assert.True(t, results[0].Score > results[1].Score)

	err = ks.DeleteDocument(ctx, kb.ID, doc.ID)
	require.NoError(t, err)

	gotChunks, err = ks.GetChunks(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Empty(t, gotChunks)

	err = ks.DeleteKB(ctx, kb.ID)
	require.NoError(t, err)

	_, err = ks.GetKB(ctx, kb.ID)
	assert.Error(t, err)
}

func TestIntegrationMultiKBSearch(t *testing.T) {
	ctx := context.Background()
	db, err := 	gorm.Open(openDialector(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := NewKnowledgeStore(db)
	require.NoError(t, err)

	kb1, _ := ks.CreateKB(ctx, &storage.KnowledgeBase{Name: "kb1", Backend: "sqlite-vec"})
	kb2, _ := ks.CreateKB(ctx, &storage.KnowledgeBase{Name: "kb2", Backend: "sqlite-vec"})

	doc1 := &storage.Document{KBID: kb1.ID, Filename: "doc1.txt", Source: "upload", Status: storage.DocStatusPending}
	ks.IngestDocument(ctx, kb1.ID, doc1, nil)
	ks.StoreChunks(ctx, kb1.ID, doc1.ID, []*storage.Chunk{
		{DocumentID: doc1.ID, KBID: kb1.ID, Content: "cats are furry", Index: 0},
	}, [][]float32{{1.0, 0.0}})

	doc2 := &storage.Document{KBID: kb2.ID, Filename: "doc2.txt", Source: "upload", Status: storage.DocStatusPending}
	ks.IngestDocument(ctx, kb2.ID, doc2, nil)
	ks.StoreChunks(ctx, kb2.ID, doc2.ID, []*storage.Chunk{
		{DocumentID: doc2.ID, KBID: kb2.ID, Content: "dogs are loyal", Index: 0},
	}, [][]float32{{0.0, 1.0}})

	results, err := ks.Search(ctx, []string{kb1.ID, kb2.ID}, []float32{1.0, 0.5}, storage.SearchOptions{TopK: 10})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	results, err = ks.Search(ctx, []string{kb1.ID}, []float32{1.0, 0.0}, storage.SearchOptions{TopK: 10})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "cats are furry", results[0].Content)
}

func TestIntegrationVectorSearchAccuracy(t *testing.T) {
	ctx := context.Background()
	db, err := 	gorm.Open(openDialector(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := NewKnowledgeStore(db)
	require.NoError(t, err)

	kb, _ := ks.CreateKB(ctx, &storage.KnowledgeBase{Name: "accuracy-kb", Backend: "sqlite-vec"})
	doc := &storage.Document{KBID: kb.ID, Filename: "doc.txt", Source: "upload", Status: storage.DocStatusPending}
	ks.IngestDocument(ctx, kb.ID, doc, nil)

	chunkData := []struct {
		content string
		vector  []float32
	}{
		{"machine learning algorithms", []float32{0.9, 0.1, 0.0}},
		{"web development frameworks", []float32{0.1, 0.9, 0.0}},
		{"database optimization", []float32{0.0, 0.1, 0.9}},
	}

	var chunks []*storage.Chunk
	var vectors [][]float32
	for i, d := range chunkData {
		chunks = append(chunks, &storage.Chunk{
			DocumentID: doc.ID, KBID: kb.ID, Content: d.content, Index: i,
		})
		vectors = append(vectors, d.vector)
	}

	ks.StoreChunks(ctx, kb.ID, doc.ID, chunks, vectors)

	results, err := ks.Search(ctx, []string{kb.ID}, []float32{0.8, 0.2, 0.0}, storage.SearchOptions{TopK: 3})
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, "machine learning algorithms", results[0].Content)
	assert.True(t, results[0].Score > 0.9)
}
