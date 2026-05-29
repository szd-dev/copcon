package rag

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/copcon/core/providers/sqlitevec"
	"github.com/copcon/core/storage"
)

type testEmbedder struct {
	dimensions int
}

func (e *testEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vec := make([]float32, e.dimensions)
	for i := range vec {
		vec[i] = float32(len(text)%100) / 100.0
	}
	return vec, nil
}

func (e *testEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec := make([]float32, e.dimensions)
		for j := range vec {
			vec[j] = float32((len(text)+i)%100) / 100.0
		}
		results[i] = vec
	}
	return results, nil
}

func (e *testEmbedder) Dimensions() int { return e.dimensions }
func (e *testEmbedder) Name() string    { return "test-embedder" }

func TestIntegrationPipelineEndToEnd(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := sqlitevec.NewKnowledgeStore(db)
	require.NoError(t, err)

	kb, err := ks.CreateKB(ctx, &storage.KnowledgeBase{
		Name:    "pipeline-kb",
		Backend: "sqlite-vec",
	})
	require.NoError(t, err)

	embedder := &testEmbedder{dimensions: 8}
	parser := NewDefaultParser()
	pipeline := NewPipeline(parser, embedder, ks)

	doc := &storage.Document{
		KBID:     kb.ID,
		Filename: "sample.txt",
		Source:   "upload",
		Status:   storage.DocStatusPending,
	}
	content := []byte("This is the first paragraph about Go programming.\n\nThis is the second paragraph about Python scripting.\n\nThis is the third paragraph about Rust systems programming.")

	err = pipeline.Ingest(ctx, kb.ID, doc, content, "text/plain", nil)
	require.NoError(t, err)

	gotDoc, err := ks.GetDocument(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.DocStatusReady, gotDoc.Status)
	assert.Greater(t, gotDoc.ChunkCount, 0)

	chunks, err := ks.GetChunks(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, chunks)

	queryVec, _ := embedder.Embed(ctx, "Go programming")
	results, err := ks.Search(ctx, []string{kb.ID}, queryVec, storage.SearchOptions{TopK: 5})
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

func TestIntegrationPipelineMarkdownEndToEnd(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := sqlitevec.NewKnowledgeStore(db)
	require.NoError(t, err)

	kb, _ := ks.CreateKB(ctx, &storage.KnowledgeBase{Name: "md-kb", Backend: "sqlite-vec"})

	embedder := &testEmbedder{dimensions: 8}
	pipeline := NewMarkdownPipeline(NewDefaultParser(), embedder, ks)

	doc := &storage.Document{
		KBID:     kb.ID,
		Filename: "doc.md",
		Source:   "upload",
		Status:   storage.DocStatusPending,
	}
	content := []byte("# Introduction\n\nThis is the introduction section.\n\n## Details\n\nHere are the details about the system.")

	err = pipeline.Ingest(ctx, kb.ID, doc, content, "text/markdown", nil)
	require.NoError(t, err)

	gotDoc, _ := ks.GetDocument(ctx, kb.ID, doc.ID)
	assert.Equal(t, storage.DocStatusReady, gotDoc.Status)
}

func TestIntegrationPipelineMultipleDocuments(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	ks, err := sqlitevec.NewKnowledgeStore(db)
	require.NoError(t, err)

	kb, _ := ks.CreateKB(ctx, &storage.KnowledgeBase{Name: "multi-kb", Backend: "sqlite-vec"})
	embedder := &testEmbedder{dimensions: 8}
	pipeline := NewPipeline(NewDefaultParser(), embedder, ks)

	for i := 0; i < 3; i++ {
		doc := &storage.Document{
			KBID:     kb.ID,
			Filename: "doc" + string(rune('A'+i)) + ".txt",
			Source:   "upload",
			Status:   storage.DocStatusPending,
		}
		content := []byte("Document " + string(rune('A'+i)) + " content about various topics.")
		err := pipeline.Ingest(ctx, kb.ID, doc, content, "text/plain", nil)
		require.NoError(t, err)
	}

	docs, err := ks.ListDocuments(ctx, kb.ID)
	require.NoError(t, err)
	assert.Len(t, docs, 3)

	for _, d := range docs {
		assert.Equal(t, storage.DocStatusReady, d.Status)
	}
}
