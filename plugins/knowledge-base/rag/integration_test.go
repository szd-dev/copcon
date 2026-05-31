package kbrag

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kbtypes "github.com/copcon/plugins/knowledge-base/types"
	knowledgebase "github.com/copcon/plugins/knowledge-base"
)

// inMemoryPipelineStore implements knowledgebase.KnowledgeStore
type inMemoryPipelineStore struct {
	kbs       map[string]*kbtypes.KnowledgeBase
	documents map[string]map[string]*kbtypes.Document
	chunks    map[string]map[string][]*kbtypes.Chunk
	vectors   map[string]map[string][][]float32
	statuses  map[string]kbtypes.DocumentStatus
}

func newInMemoryPipelineStore() *inMemoryPipelineStore {
	return &inMemoryPipelineStore{
		kbs:       make(map[string]*kbtypes.KnowledgeBase),
		documents: make(map[string]map[string]*kbtypes.Document),
		chunks:    make(map[string]map[string][]*kbtypes.Chunk),
		vectors:   make(map[string]map[string][][]float32),
		statuses:  make(map[string]kbtypes.DocumentStatus),
	}
}

func (s *inMemoryPipelineStore) CreateKB(ctx context.Context, kb *kbtypes.KnowledgeBase) (*kbtypes.KnowledgeBase, error) {
	s.kbs[kb.ID] = kb
	s.documents[kb.ID] = make(map[string]*kbtypes.Document)
	s.chunks[kb.ID] = make(map[string][]*kbtypes.Chunk)
	s.vectors[kb.ID] = make(map[string][][]float32)
	return kb, nil
}

func (s *inMemoryPipelineStore) IngestDocument(ctx context.Context, kbID string, doc *kbtypes.Document, content []byte) error {
	if doc.ID == "" {
		doc.ID = fmt.Sprintf("doc-%d", len(s.documents[kbID])+1)
	}
	doc.KBID = kbID
	s.documents[kbID][doc.ID] = doc
	s.statuses[doc.ID] = doc.Status
	return nil
}

func (s *inMemoryPipelineStore) StoreChunks(ctx context.Context, kbID string, docID string, chunks []*kbtypes.Chunk, vectors [][]float32) error {
	s.chunks[kbID][docID] = chunks
	s.vectors[kbID][docID] = vectors
	s.statuses[docID] = kbtypes.DocStatusReady
	if doc, ok := s.documents[kbID][docID]; ok {
		doc.ChunkCount = len(chunks)
		doc.Status = kbtypes.DocStatusReady
	}
	return nil
}

func (s *inMemoryPipelineStore) UpdateDocumentStatus(ctx context.Context, kbID string, docID string, status kbtypes.DocumentStatus) error {
	s.statuses[docID] = status
	if doc, ok := s.documents[kbID][docID]; ok {
		doc.Status = status
	}
	return nil
}

func (s *inMemoryPipelineStore) GetDocument(ctx context.Context, kbID string, docID string) (*kbtypes.Document, error) {
	doc, ok := s.documents[kbID][docID]
	if !ok {
		return nil, fmt.Errorf("document not found")
	}
	return doc, nil
}

func (s *inMemoryPipelineStore) GetChunks(ctx context.Context, kbID string, docID string) ([]*kbtypes.Chunk, error) {
	chunks, ok := s.chunks[kbID][docID]
	if !ok {
		return nil, nil
	}
	return chunks, nil
}

func (s *inMemoryPipelineStore) DeleteKB(ctx context.Context, id string) error {
	delete(s.kbs, id)
	delete(s.documents, id)
	delete(s.chunks, id)
	delete(s.vectors, id)
	return nil
}

func (s *inMemoryPipelineStore) ListKBs(ctx context.Context) ([]*kbtypes.KnowledgeBase, error) {
	var result []*kbtypes.KnowledgeBase
	for _, kb := range s.kbs {
		result = append(result, kb)
	}
	return result, nil
}

func (s *inMemoryPipelineStore) GetKB(ctx context.Context, id string) (*kbtypes.KnowledgeBase, error) {
	kb, ok := s.kbs[id]
	if !ok {
		return nil, fmt.Errorf("kb not found")
	}
	return kb, nil
}

func (s *inMemoryPipelineStore) DeleteDocument(ctx context.Context, kbID string, docID string) error {
	if docs, ok := s.documents[kbID]; ok {
		delete(docs, docID)
	}
	if chunks, ok := s.chunks[kbID]; ok {
		delete(chunks, docID)
	}
	if vecs, ok := s.vectors[kbID]; ok {
		delete(vecs, docID)
	}
	delete(s.statuses, docID)
	return nil
}

func (s *inMemoryPipelineStore) ListDocuments(ctx context.Context, kbID string) ([]*kbtypes.Document, error) {
	var result []*kbtypes.Document
	if docs, ok := s.documents[kbID]; ok {
		for _, doc := range docs {
			result = append(result, doc)
		}
	}
	return result, nil
}

func (s *inMemoryPipelineStore) UpdateChunk(ctx context.Context, kbID string, chunk *kbtypes.Chunk) error {
	return nil
}

func (s *inMemoryPipelineStore) Search(ctx context.Context, kbIDs []string, query []float32, opts kbtypes.SearchOptions) ([]*kbtypes.Chunk, error) {
	return nil, nil
}

func (s *inMemoryPipelineStore) UpdateDocumentErrorMsg(ctx context.Context, kbID string, docID string, msg string) error {
	return nil
}

func (s *inMemoryPipelineStore) ListDocumentsByStatus(ctx context.Context, statuses []string) ([]*kbtypes.Document, error) {
	return nil, nil
}

func (s *inMemoryPipelineStore) ClaimDocumentStatus(ctx context.Context, docID string, newStatus string, expectedStatus string) (bool, error) {
	return false, nil
}

// Compile-time check that inMemoryPipelineStore implements KnowledgeStore
var _ knowledgebase.KnowledgeStore = (*inMemoryPipelineStore)(nil)

type integrationTestEmbedder struct {
	dimensions int
}

func (e *integrationTestEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vec := make([]float32, e.dimensions)
	for i := range vec {
		vec[i] = float32(len(text)%100) / 100.0
	}
	return vec, nil
}

func (e *integrationTestEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
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

func (e *integrationTestEmbedder) Dimensions() int { return e.dimensions }
func (e *integrationTestEmbedder) Name() string    { return "test-embedder" }

func TestIntegrationPipelineEndToEnd(t *testing.T) {
	ctx := context.Background()
	store := newInMemoryPipelineStore()

	kb, err := store.CreateKB(ctx, &kbtypes.KnowledgeBase{
		ID:      "kb-1",
		Name:    "pipeline-kb",
		Backend: "in-memory",
	})
	require.NoError(t, err)

	embedder := &integrationTestEmbedder{dimensions: 8}
	parser := NewDefaultParser()
	pipeline := NewPipeline(parser, embedder, store)

	doc := &kbtypes.Document{
		KBID:     kb.ID,
		Filename: "sample.txt",
		Source:   "upload",
		Status:   kbtypes.DocStatusPending,
	}
	content := []byte("This is the first paragraph about Go programming.\n\nThis is the second paragraph about Python scripting.\n\nThis is the third paragraph about Rust systems programming.")

	err = pipeline.Ingest(ctx, kb.ID, doc, content, "text/plain", nil)
	require.NoError(t, err)

	gotDoc, err := store.GetDocument(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, kbtypes.DocStatusReady, gotDoc.Status)
	assert.Greater(t, gotDoc.ChunkCount, 0)

	chunks, err := store.GetChunks(ctx, kb.ID, doc.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, chunks)
}

func TestIntegrationPipelineMarkdownEndToEnd(t *testing.T) {
	ctx := context.Background()
	store := newInMemoryPipelineStore()

	kb, _ := store.CreateKB(ctx, &kbtypes.KnowledgeBase{ID: "kb-md", Name: "md-kb", Backend: "in-memory"})

	embedder := &integrationTestEmbedder{dimensions: 8}
	pipeline := NewMarkdownPipeline(NewDefaultParser(), embedder, store)

	doc := &kbtypes.Document{
		KBID:     kb.ID,
		Filename: "doc.md",
		Source:   "upload",
		Status:   kbtypes.DocStatusPending,
	}
	content := []byte("# Introduction\n\nThis is the introduction section.\n\n## Details\n\nHere are the details about the system.")

	err := pipeline.Ingest(ctx, kb.ID, doc, content, "text/markdown", nil)
	require.NoError(t, err)

	gotDoc, _ := store.GetDocument(ctx, kb.ID, doc.ID)
	assert.Equal(t, kbtypes.DocStatusReady, gotDoc.Status)
}

func TestIntegrationPipelineMultipleDocuments(t *testing.T) {
	ctx := context.Background()
	store := newInMemoryPipelineStore()

	kb, _ := store.CreateKB(ctx, &kbtypes.KnowledgeBase{ID: "kb-multi", Name: "multi-kb", Backend: "in-memory"})
	embedder := &integrationTestEmbedder{dimensions: 8}
	pipeline := NewPipeline(NewDefaultParser(), embedder, store)

	for i := 0; i < 3; i++ {
		doc := &kbtypes.Document{
			KBID:     kb.ID,
			Filename: "doc" + string(rune('A'+i)) + ".txt",
			Source:   "upload",
			Status:   kbtypes.DocStatusPending,
		}
		content := []byte("Document " + string(rune('A'+i)) + " content about various topics.")
		err := pipeline.Ingest(ctx, kb.ID, doc, content, "text/plain", nil)
		require.NoError(t, err)
	}

	for docID, status := range store.statuses {
		assert.Equal(t, kbtypes.DocStatusReady, status, "doc %s should be ready", docID)
	}
}
