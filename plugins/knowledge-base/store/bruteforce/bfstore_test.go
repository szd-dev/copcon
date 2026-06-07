package bruteforce

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	knowledgebase "github.com/copcon/plugins/knowledge-base"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

// chunkRow mirrors the chunks table schema for GORM writes in tests.
type chunkRow struct {
	ID         string `gorm:"primaryKey;size:64"`
	DocumentID string `gorm:"size:64;not null;index"`
	KBID       string `gorm:"size:64;not null;index"`
	Content    string `gorm:"type:text;not null"`
	Vector     []byte `gorm:"type:blob"`
}

func (chunkRow) TableName() string { return "chunks" }

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&chunkRow{}))
	return db
}

func insertChunks(t *testing.T, db *gorm.DB, kbID string, chunks []knowledgebase.VectorChunk, vectors [][]float32) {
	t.Helper()
	for i, c := range chunks {
		row := &chunkRow{
			ID:         c.ID,
			DocumentID: c.DocumentID,
			KBID:       kbID,
			Content:    c.Content,
			Vector:     toBlob(vectors[i]),
		}
		require.NoError(t, db.Create(row).Error)
	}
}

func TestBackend(t *testing.T) {
	s := New(nil)
	assert.Equal(t, "brute-force", s.Backend())
}

func TestSearchEmpty(t *testing.T) {
	db := setupTestDB(t)
	s := New(db)

	results, err := s.Search(context.Background(), []string{"kb1"}, []float32{}, kbtypes.SearchOptions{})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestSearchBasic(t *testing.T) {
	db := setupTestDB(t)
	s := New(db)

	chunks := []knowledgebase.VectorChunk{
		{ID: "c1", DocumentID: "d1", KBID: "kb1", Content: "cats"},
		{ID: "c2", DocumentID: "d1", KBID: "kb1", Content: "dogs"},
		{ID: "c3", DocumentID: "d2", KBID: "kb1", Content: "birds"},
	}
	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
	}
	insertChunks(t, db, "kb1", chunks, vectors)

	results, err := s.Search(context.Background(), []string{"kb1"}, []float32{1.0, 0.1, 0.0}, kbtypes.SearchOptions{TopK: 2})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "c1", results[0].ChunkID)
	assert.True(t, results[0].Score > results[1].Score)
}

func TestSearchWithThreshold(t *testing.T) {
	db := setupTestDB(t)
	s := New(db)

	chunks := []knowledgebase.VectorChunk{
		{ID: "c1", DocumentID: "d1", KBID: "kb1", Content: "close"},
		{ID: "c2", DocumentID: "d1", KBID: "kb1", Content: "far"},
	}
	vectors := [][]float32{
		{1.0, 0.0},
		{0.0, 1.0},
	}
	insertChunks(t, db, "kb1", chunks, vectors)

	results, err := s.Search(context.Background(), []string{"kb1"}, []float32{1.0, 0.0}, kbtypes.SearchOptions{
		TopK:                10,
		SimilarityThreshold: 0.99,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "c1", results[0].ChunkID)
}

func TestVerify(t *testing.T) {
	db := setupTestDB(t)
	s := New(db)

	chunks := []knowledgebase.VectorChunk{
		{ID: "c1", DocumentID: "d1", KBID: "kb1"},
		{ID: "c2", DocumentID: "d1", KBID: "kb1"},
		{ID: "c3", DocumentID: "d2", KBID: "kb2"},
	}
	vectors := [][]float32{
		{1.0, 0.0},
		{0.0, 1.0},
		{1.0, 1.0},
	}
	insertChunks(t, db, "kb1", chunks[:2], vectors[:2])
	insertChunks(t, db, "kb2", chunks[2:], vectors[2:])

	verify, err := s.Verify(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, verify["kb1"])
	assert.Equal(t, 1, verify["kb2"])
}

func TestVectorBlobRoundtrip(t *testing.T) {
	original := []float32{0.1, -0.5, 0.999, 0.0, -1.0}
	blob := toBlob(original)
	recovered := fromBlob(blob)
	require.Equal(t, len(original), len(recovered))
	for i := range original {
		assert.InDelta(t, original[i], recovered[i], 1e-6)
	}
}

func TestCosineSimilarityAccuracy(t *testing.T) {
	assert.InDelta(t, 1.0, cosineSimilarity([]float32{1, 0, 0}, []float32{1, 0, 0}), 1e-5)
	assert.InDelta(t, 0.0, cosineSimilarity([]float32{1, 0, 0}, []float32{0, 1, 0}), 1e-5)
	assert.InDelta(t, -1.0, cosineSimilarity([]float32{1, 0, 0}, []float32{-1, 0, 0}), 1e-5)
}
