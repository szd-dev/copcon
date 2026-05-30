package kbrag

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/storage"
	knowledgebase "github.com/copcon/plugins/knowledge-base"
)

var errEmptyText = fmt.Errorf("empty text provided for embedding")

type mockEmbedder struct {
	dimensions int
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, errEmptyText
	}
	vec := make([]float32, m.dimensions)
	vec[0] = 1.0
	return vec, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errEmptyText
	}
	results := make([][]float32, len(texts))
	for i, text := range texts {
		if text == "" {
			return nil, errEmptyText
		}
		vec := make([]float32, m.dimensions)
		vec[0] = float32(i) / float32(len(texts))
		results[i] = vec
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dimensions }
func (m *mockEmbedder) Name() string    { return "mock" }

type mockPipelineStore struct {
	documents map[string]*storage.Document
	chunks    map[string][]*storage.Chunk
	vectors   map[string][][]float32
	statuses  map[string]storage.DocumentStatus
}

func newMockPipelineStore() *mockPipelineStore {
	return &mockPipelineStore{
		documents: make(map[string]*storage.Document),
		chunks:    make(map[string][]*storage.Chunk),
		vectors:   make(map[string][][]float32),
		statuses:  make(map[string]storage.DocumentStatus),
	}
}

func (s *mockPipelineStore) IngestDocument(ctx context.Context, kbID string, doc *storage.Document, content []byte) error {
	if doc.ID == "" {
		doc.ID = "doc-1"
	}
	s.documents[doc.ID] = doc
	s.statuses[doc.ID] = doc.Status
	return nil
}

func (s *mockPipelineStore) StoreChunks(ctx context.Context, kbID string, docID string, chunks []*storage.Chunk, vectors [][]float32) error {
	s.chunks[docID] = chunks
	s.vectors[docID] = vectors
	s.statuses[docID] = storage.DocStatusReady
	return nil
}

func (s *mockPipelineStore) UpdateDocumentStatus(ctx context.Context, kbID string, docID string, status storage.DocumentStatus) error {
	s.statuses[docID] = status
	return nil
}

func (s *mockPipelineStore) CreateKB(ctx context.Context, kb *storage.KnowledgeBase) (*storage.KnowledgeBase, error) {
	return kb, nil
}

func (s *mockPipelineStore) DeleteKB(ctx context.Context, id string) error {
	return nil
}

func (s *mockPipelineStore) ListKBs(ctx context.Context) ([]*storage.KnowledgeBase, error) {
	return nil, nil
}

func (s *mockPipelineStore) GetKB(ctx context.Context, id string) (*storage.KnowledgeBase, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *mockPipelineStore) DeleteDocument(ctx context.Context, kbID string, docID string) error {
	delete(s.documents, docID)
	delete(s.chunks, docID)
	delete(s.statuses, docID)
	return nil
}

func (s *mockPipelineStore) GetDocument(ctx context.Context, kbID string, docID string) (*storage.Document, error) {
	if doc, ok := s.documents[docID]; ok {
		return doc, nil
	}
	return nil, fmt.Errorf("not found")
}

func (s *mockPipelineStore) ListDocuments(ctx context.Context, kbID string) ([]*storage.Document, error) {
	var docs []*storage.Document
	for _, doc := range s.documents {
		docs = append(docs, doc)
	}
	return docs, nil
}

func (s *mockPipelineStore) GetChunks(ctx context.Context, kbID string, docID string) ([]*storage.Chunk, error) {
	return s.chunks[docID], nil
}

func (s *mockPipelineStore) UpdateChunk(ctx context.Context, kbID string, chunk *storage.Chunk) error {
	return nil
}

func (s *mockPipelineStore) Search(ctx context.Context, kbIDs []string, query []float32, opts storage.SearchOptions) ([]*storage.Chunk, error) {
	return nil, nil
}

func TestPipelineIngest(t *testing.T) {
	parser := NewDefaultParser()
	embedder := &mockEmbedder{dimensions: 3}
	store := newMockPipelineStore()
	pipeline := NewPipeline(parser, embedder, store)

	var progressCalls atomic.Int32
	progress := func(stage string, current, total int) {
		progressCalls.Add(1)
	}

	doc := &storage.Document{
		Filename: "test.txt",
		Source:   "upload",
		Status:   storage.DocStatusPending,
	}
	content := []byte("This is a test document. It has multiple sentences. Each one is important.")

	err := pipeline.Ingest(context.Background(), "kb-1", doc, content, "text/plain", progress)
	require.NoError(t, err)
	assert.Equal(t, "doc-1", doc.ID)
	assert.Equal(t, storage.DocStatusReady, store.statuses[doc.ID])
	assert.Equal(t, int32(4), progressCalls.Load())
	assert.NotEmpty(t, store.chunks[doc.ID])
	assert.NotEmpty(t, store.vectors[doc.ID])
}

func TestPipelineIngestMarkdown(t *testing.T) {
	parser := NewDefaultParser()
	embedder := &mockEmbedder{dimensions: 3}
	store := newMockPipelineStore()
	pipeline := NewPipeline(parser, embedder, store)

	doc := &storage.Document{
		Filename: "test.md",
		Source:   "upload",
		Status:   storage.DocStatusPending,
	}
	content := []byte("# Title\n\nParagraph one.\n\n## Section\n\nParagraph two.")

	err := pipeline.Ingest(context.Background(), "kb-1", doc, content, "text/markdown", nil)
	require.NoError(t, err)
	assert.Equal(t, storage.DocStatusReady, store.statuses[doc.ID])
}

func TestPipelineIngestParseError(t *testing.T) {
	parser := NewDefaultParser()
	embedder := &mockEmbedder{dimensions: 3}
	store := newMockPipelineStore()
	pipeline := NewPipeline(parser, embedder, store)

	doc := &storage.Document{
		Filename: "test.xyz",
		Source:   "upload",
		Status:   storage.DocStatusPending,
	}

	err := pipeline.Ingest(context.Background(), "kb-1", doc, []byte("data"), "application/octet-stream", nil)
	assert.Error(t, err)
	assert.Equal(t, storage.DocStatusError, store.statuses[doc.ID])
}

func TestPipelineIngestNoProgress(t *testing.T) {
	parser := NewDefaultParser()
	embedder := &mockEmbedder{dimensions: 3}
	store := newMockPipelineStore()
	pipeline := NewPipeline(parser, embedder, store)

	doc := &storage.Document{
		Filename: "test.txt",
		Source:   "upload",
		Status:   storage.DocStatusPending,
	}

	err := pipeline.Ingest(context.Background(), "kb-1", doc, []byte("Hello world."), "text/plain", nil)
	require.NoError(t, err)
}

func TestPipelineIngestEmptyContent(t *testing.T) {
	parser := NewDefaultParser()
	embedder := &mockEmbedder{dimensions: 3}
	store := newMockPipelineStore()
	pipeline := NewPipeline(parser, embedder, store)

	doc := &storage.Document{
		Filename: "empty.txt",
		Source:   "upload",
		Status:   storage.DocStatusPending,
	}

	err := pipeline.Ingest(context.Background(), "kb-1", doc, []byte(""), "text/plain", nil)
	require.NoError(t, err)
}

func TestPipelineStoreInterface(t *testing.T) {
	store := newMockPipelineStore()
	var _ knowledgebase.KnowledgeStore = store
}

func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 9, estimateTokens("a relatively short piece of text here"))
	assert.Equal(t, 0, estimateTokens(""))
}

func TestIsMarkdownMimetype(t *testing.T) {
	assert.True(t, isMarkdownMimetype("text/markdown"))
	assert.True(t, isMarkdownMimetype("text/x-markdown"))
	assert.False(t, isMarkdownMimetype("text/plain"))
	assert.False(t, isMarkdownMimetype("application/pdf"))
}

func TestNewMarkdownPipeline(t *testing.T) {
	parser := NewDefaultParser()
	embedder := &mockEmbedder{dimensions: 3}
	store := newMockPipelineStore()
	pipeline := NewMarkdownPipeline(parser, embedder, store)
	assert.NotNil(t, pipeline)
	assert.IsType(t, &MarkdownAwareChunker{}, pipeline.chunker)
}

type failingEmbedder struct{}

func (f *failingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("embedding failed")
}

func (f *failingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("embedding failed")
}

func (f *failingEmbedder) Dimensions() int { return 3 }
func (f *failingEmbedder) Name() string    { return "failing" }

func TestPipelineEmbedRetryError(t *testing.T) {
	parser := NewDefaultParser()
	embedder := &failingEmbedder{}
	store := newMockPipelineStore()
	pipeline := NewPipeline(parser, embedder, store)

	doc := &storage.Document{
		Filename: "test.txt",
		Source:   "upload",
		Status:   storage.DocStatusPending,
	}

	err := pipeline.Ingest(context.Background(), "kb-1", doc, []byte("Hello world."), "text/plain", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "after 3 attempts")
	assert.Equal(t, storage.DocStatusError, store.statuses[doc.ID])
}
