// Package sqlitevec provides a SQLite-backed KnowledgeStore implementation
// that stores vector embeddings as BLOBs and computes cosine similarity
// in Go code. It uses the glebarez/sqlite driver (pure Go, no CGO) via GORM.
package sqlitevec

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/core/storage"
)

// KnowledgeStore implements storage.KnowledgeStore using SQLite + GORM
// with brute-force vector search via cosine similarity.
type KnowledgeStore struct {
	db *gorm.DB
}

// NewKnowledgeStore creates a new KnowledgeStore backed by the given GORM DB.
// It auto-migrates the schema tables.
func NewKnowledgeStore(db *gorm.DB) (*KnowledgeStore, error) {
	if err := db.AutoMigrate(
		&kbModel{},
		&docModel{},
		&chunkModel{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrate knowledge schema: %w", err)
	}
	return &KnowledgeStore{db: db}, nil
}

// NewKnowledgeStoreFromDSN creates a new KnowledgeStore using a DSN string.
// Use ":memory:" for an in-memory database.
func NewKnowledgeStoreFromDSN(dsn string) (*KnowledgeStore, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	return NewKnowledgeStore(db)
}

// Compile-time interface check.
var _ storage.KnowledgeStore = (*KnowledgeStore)(nil)

func (s *KnowledgeStore) CreateKB(ctx context.Context, kb *storage.KnowledgeBase) (*storage.KnowledgeBase, error) {
	id := kb.ID
	if id == "" {
		id = uuid.New().String()
	}

	now := time.Now().UTC()
	m := &kbModel{
		ID:        id,
		Name:      kb.Name,
		Backend:   kb.Backend,
		Config:    toJSONB(kb.Config),
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  toJSONB(kb.Metadata),
	}

	if err := s.db.WithContext(ctx).Create(m).Error; err != nil {
		return nil, fmt.Errorf("create knowledge base: %w", err)
	}

	return m.toDomain(), nil
}

func (s *KnowledgeStore) DeleteKB(ctx context.Context, id string) error {
	if err := s.db.WithContext(ctx).Where("kb_id = ?", id).Delete(&chunkModel{}).Error; err != nil {
		return fmt.Errorf("delete chunks for kb %s: %w", id, err)
	}
	if err := s.db.WithContext(ctx).Where("kb_id = ?", id).Delete(&docModel{}).Error; err != nil {
		return fmt.Errorf("delete documents for kb %s: %w", id, err)
	}
	result := s.db.WithContext(ctx).Where("id = ?", id).Delete(&kbModel{})
	if result.Error != nil {
		return fmt.Errorf("delete knowledge base %s: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("knowledge base %s not found", id)
	}
	return nil
}

func (s *KnowledgeStore) ListKBs(ctx context.Context) ([]*storage.KnowledgeBase, error) {
	var models []kbModel
	if err := s.db.WithContext(ctx).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list knowledge bases: %w", err)
	}
	result := make([]*storage.KnowledgeBase, len(models))
	for i, m := range models {
		result[i] = m.toDomain()
	}
	return result, nil
}

func (s *KnowledgeStore) GetKB(ctx context.Context, id string) (*storage.KnowledgeBase, error) {
	var m kbModel
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		return nil, fmt.Errorf("get knowledge base %s: %w", id, err)
	}
	return m.toDomain(), nil
}

func (s *KnowledgeStore) IngestDocument(ctx context.Context, kbID string, doc *storage.Document, content []byte) error {
	docID := doc.ID
	if docID == "" {
		docID = uuid.New().String()
	}

	now := time.Now().UTC()
	m := &docModel{
		ID:         docID,
		KBID:       kbID,
		Filename:   doc.Filename,
		Source:     doc.Source,
		Status:     string(doc.Status),
		ChunkCount: doc.ChunkCount,
		TokenCount: doc.TokenCount,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata:   toJSONB(doc.Metadata),
	}

	if err := s.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("ingest document: %w", err)
	}
	doc.ID = m.ID
	return nil
}

func (s *KnowledgeStore) ListDocuments(ctx context.Context, kbID string) ([]*storage.Document, error) {
	var models []docModel
	if err := s.db.WithContext(ctx).Where("kb_id = ?", kbID).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list documents for kb %s: %w", kbID, err)
	}
	result := make([]*storage.Document, len(models))
	for i, m := range models {
		result[i] = m.toDomain()
	}
	return result, nil
}

func (s *KnowledgeStore) DeleteDocument(ctx context.Context, kbID string, docID string) error {
	if err := s.db.WithContext(ctx).Where("document_id = ? AND kb_id = ?", docID, kbID).Delete(&chunkModel{}).Error; err != nil {
		return fmt.Errorf("delete chunks for document %s: %w", docID, err)
	}
	result := s.db.WithContext(ctx).Where("id = ? AND kb_id = ?", docID, kbID).Delete(&docModel{})
	if result.Error != nil {
		return fmt.Errorf("delete document %s: %w", docID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("document %s not found in kb %s", docID, kbID)
	}
	return nil
}

func (s *KnowledgeStore) GetDocument(ctx context.Context, kbID string, docID string) (*storage.Document, error) {
	var m docModel
	if err := s.db.WithContext(ctx).Where("id = ? AND kb_id = ?", docID, kbID).First(&m).Error; err != nil {
		return nil, fmt.Errorf("get document %s in kb %s: %w", docID, kbID, err)
	}
	return m.toDomain(), nil
}

func (s *KnowledgeStore) GetChunks(ctx context.Context, kbID string, docID string) ([]*storage.Chunk, error) {
	var models []chunkModel
	if err := s.db.WithContext(ctx).Where("document_id = ? AND kb_id = ?", docID, kbID).Order("chunk_index ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("get chunks for document %s: %w", docID, err)
	}
	result := make([]*storage.Chunk, len(models))
	for i, m := range models {
		result[i] = m.toDomain()
	}
	return result, nil
}

func (s *KnowledgeStore) UpdateChunk(ctx context.Context, kbID string, chunk *storage.Chunk) error {
	updates := map[string]any{
		"content":     chunk.Content,
		"context":     chunk.Context,
		"token_count": chunk.TokenCount,
		"metadata":    toJSONB(chunk.Metadata),
	}
	result := s.db.WithContext(ctx).Model(&chunkModel{}).Where("id = ? AND kb_id = ?", chunk.ID, kbID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update chunk %s: %w", chunk.ID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("chunk %s not found in kb %s", chunk.ID, kbID)
	}
	return nil
}

func (s *KnowledgeStore) Search(ctx context.Context, kbIDs []string, query []float32, opts storage.SearchOptions) ([]*storage.Chunk, error) {
	if len(query) == 0 {
		return nil, nil
	}

	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}
	threshold := opts.SimilarityThreshold
	if threshold <= 0 {
		threshold = 0.0
	}

	var models []chunkModel
	if err := s.db.WithContext(ctx).Where("kb_id IN ?", kbIDs).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}

	type scored struct {
		chunk *storage.Chunk
		score float32
	}
	var results []scored

	for _, m := range models {
		vec := fromBlob(m.Vector)
		if len(vec) == 0 {
			continue
		}
		sim := cosineSimilarity(query, vec)
		if sim >= threshold {
			c := m.toDomain()
			c.Score = sim
			results = append(results, scored{chunk: c, score: sim})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	chunks := make([]*storage.Chunk, len(results))
	for i, r := range results {
		chunks[i] = r.chunk
	}
	return chunks, nil
}

// StoreChunks persists pre-processed chunks with their embeddings.
// Used by the RAG pipeline after parsing, chunking, and embedding.
func (s *KnowledgeStore) StoreChunks(ctx context.Context, kbID string, docID string, chunks []*storage.Chunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunks count (%d) != vectors count (%d)", len(chunks), len(vectors))
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, c := range chunks {
			chunkID := c.ID
			if chunkID == "" {
				chunkID = uuid.New().String()
			}
			m := &chunkModel{
				ID:         chunkID,
				DocumentID: docID,
				KBID:       kbID,
				Content:    c.Content,
				Context:    c.Context,
				ChunkIndex: c.Index,
				TokenCount: c.TokenCount,
				Vector:     toBlob(vectors[i]),
				Metadata:   toJSONB(c.Metadata),
			}
			if err := tx.Create(m).Error; err != nil {
				return fmt.Errorf("store chunk %d: %w", i, err)
			}
		}

		now := time.Now().UTC()
		if err := tx.Model(&docModel{}).Where("id = ? AND kb_id = ?", docID, kbID).Updates(map[string]any{
			"status":      string(storage.DocStatusReady),
			"chunk_count": len(chunks),
			"updated_at":  now,
		}).Error; err != nil {
			return fmt.Errorf("update document status: %w", err)
		}

		return nil
	})
}

// UpdateDocumentStatus sets the processing status of a document.
func (s *KnowledgeStore) UpdateDocumentStatus(ctx context.Context, kbID string, docID string, status storage.DocumentStatus) error {
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&docModel{}).Where("id = ? AND kb_id = ?", docID, kbID).Updates(map[string]any{
		"status":     string(status),
		"updated_at": now,
	})
	if result.Error != nil {
		return fmt.Errorf("update document status: %w", result.Error)
	}
	return nil
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

func toJSONB(m map[string]any) jsonb {
	if m == nil {
		return jsonb{}
	}
	return jsonb(m)
}
