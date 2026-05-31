package sqlitevec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

func TestIntegrationKBLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := NewKnowledgeStore(db, &mockVectorStore{})
	require.NoError(t, err)

	kb, err := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{
		Name:    "integration-kb",
		Backend: "sqlite-vec",
		Config:  map[string]any{"purpose": "integration test"},
	})
	require.NoError(t, err)

	doc := &kbtypes.Document{
		KBID:     kb.ID,
		Filename: "test.txt",
		Source:   "upload",
		Status:   kbtypes.DocStatusPending,
	}
	err = ks.IngestDocument(ctx, kb.ID, doc, []byte("test content"))
	require.NoError(t, err)

	chunks := []*kbtypes.Chunk{
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
	assert.Equal(t, kbtypes.DocStatusReady, gotDoc.Status)
	assert.Equal(t, 3, gotDoc.ChunkCount)

	gotChunks, err := ks.GetChunks(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Len(t, gotChunks, 3)
	assert.Equal(t, "Go is a programming language", gotChunks[0].Content)

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