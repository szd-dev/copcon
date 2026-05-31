package sqlitevec

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	knowledgebase "github.com/copcon/plugins/knowledge-base"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

// KnowledgeStore implements knowledgebase.KnowledgeStore using SQLite + GORM.
type KnowledgeStore struct {
	db  *gorm.DB
	vec knowledgebase.VectorStore
}

var _ knowledgebase.KnowledgeStore = (*KnowledgeStore)(nil)

func NewKnowledgeStore(db *gorm.DB, vec knowledgebase.VectorStore) (*KnowledgeStore, error) {
	ks := &KnowledgeStore{db: db, vec: vec}

	if err := db.AutoMigrate(
		&kbModel{},
		&docModel{},
		&chunkModel{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrate knowledge schema: %w", err)
	}

	ks.verifyConsistency(context.Background())

	return ks, nil
}

func (s *KnowledgeStore) CreateKB(ctx context.Context, kb *kbtypes.KnowledgeBase) (*kbtypes.KnowledgeBase, error) {
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
	if err := s.vec.DeleteByKB(ctx, id); err != nil {
		return fmt.Errorf("delete vector data for kb %s: %w", id, err)
	}

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

func (s *KnowledgeStore) ListKBs(ctx context.Context) ([]*kbtypes.KnowledgeBase, error) {
	var models []kbModel
	if err := s.db.WithContext(ctx).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list knowledge bases: %w", err)
	}
	result := make([]*kbtypes.KnowledgeBase, len(models))
	for i, m := range models {
		result[i] = m.toDomain()
	}
	return result, nil
}

func (s *KnowledgeStore) GetKB(ctx context.Context, id string) (*kbtypes.KnowledgeBase, error) {
	var m kbModel
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		return nil, fmt.Errorf("get knowledge base %s: %w", id, err)
	}
	return m.toDomain(), nil
}

func (s *KnowledgeStore) IngestDocument(ctx context.Context, kbID string, doc *kbtypes.Document, content []byte) error {
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
		Content:    string(content),
		ErrorMsg:   "",
		ChunkCount: doc.ChunkCount,
		TokenCount: doc.TokenCount,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata:   toJSONB(doc.Metadata),
	}

	if err := s.db.WithContext(ctx).Where("id = ?", docID).FirstOrCreate(m).Error; err != nil {
		return fmt.Errorf("ingest document: %w", err)
	}
	doc.ID = m.ID
	return nil
}

func (s *KnowledgeStore) ListDocuments(ctx context.Context, kbID string) ([]*kbtypes.Document, error) {
	var models []docModel
	if err := s.db.WithContext(ctx).Where("kb_id = ?", kbID).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list documents for kb %s: %w", kbID, err)
	}
	result := make([]*kbtypes.Document, len(models))
	for i, m := range models {
		result[i] = m.toDomain()
	}
	return result, nil
}

func (s *KnowledgeStore) DeleteDocument(ctx context.Context, kbID string, docID string) error {
	if err := s.vec.DeleteByDocument(ctx, kbID, docID); err != nil {
		return fmt.Errorf("delete vector data for document %s: %w", docID, err)
	}

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

func (s *KnowledgeStore) GetDocument(ctx context.Context, kbID string, docID string) (*kbtypes.Document, error) {
	var m docModel
	if err := s.db.WithContext(ctx).Where("id = ? AND kb_id = ?", docID, kbID).First(&m).Error; err != nil {
		return nil, fmt.Errorf("get document %s in kb %s: %w", docID, kbID, err)
	}
	return m.toDomain(), nil
}

func (s *KnowledgeStore) GetChunks(ctx context.Context, kbID string, docID string) ([]*kbtypes.Chunk, error) {
	var models []chunkModel
	if err := s.db.WithContext(ctx).Where("document_id = ? AND kb_id = ?", docID, kbID).Order("chunk_index ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("get chunks for document %s: %w", docID, err)
	}
	result := make([]*kbtypes.Chunk, len(models))
	for i, m := range models {
		result[i] = m.toDomain()
	}
	return result, nil
}

func (s *KnowledgeStore) UpdateChunk(ctx context.Context, kbID string, chunk *kbtypes.Chunk) error {
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

func (s *KnowledgeStore) StoreChunks(ctx context.Context, kbID string, docID string, chunks []*kbtypes.Chunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunks count (%d) != vectors count (%d)", len(chunks), len(vectors))
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		vecChunks := make([]knowledgebase.VectorChunk, len(chunks))
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
			vecChunks[i] = knowledgebase.VectorChunk{
				ID: chunkID, DocumentID: docID, KBID: kbID,
				Content: c.Content, Index: c.Index,
			}
		}

		if err := s.vec.Store(ctx, kbID, docID, vecChunks, vectors); err != nil {
			return fmt.Errorf("store vectors: %w", err)
		}

		now := time.Now().UTC()
		if err := tx.Model(&docModel{}).Where("id = ? AND kb_id = ?", docID, kbID).Updates(map[string]any{
			"status":      string(kbtypes.DocStatusReady),
			"chunk_count": len(chunks),
			"updated_at":  now,
		}).Error; err != nil {
			return fmt.Errorf("update document status: %w", err)
		}

		return nil
	})
}

func (s *KnowledgeStore) UpdateDocumentStatus(ctx context.Context, kbID string, docID string, status kbtypes.DocumentStatus) error {
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

func (s *KnowledgeStore) Search(ctx context.Context, kbIDs []string, query []float32, opts kbtypes.SearchOptions) ([]*kbtypes.Chunk, error) {
	results, err := s.vec.Search(ctx, kbIDs, query, opts)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	chunks := make([]*kbtypes.Chunk, 0, len(results))
	for _, r := range results {
		var m chunkModel
		if err := s.db.WithContext(ctx).Where("id = ?", r.ChunkID).First(&m).Error; err != nil {
			continue
		}
		c := m.toDomain()
		c.Score = r.Score
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// verifyConsistency checks each KB's metadata against actual vector store counts.
// KBs with mismatched counts are marked unavailable.
// This does NOT block startup — it only updates Config flags.
func (s *KnowledgeStore) verifyConsistency(ctx context.Context) {
	verify, err := s.vec.Verify(ctx)
	if err != nil {
		slog.Warn("consistency check: vector verify failed", "error", err)
		return
	}

	kbs, err := s.ListKBs(ctx)
	if err != nil {
		slog.Warn("consistency check: list KBs failed", "error", err)
		return
	}

	for _, kb := range kbs {
		docs, err := s.ListDocuments(ctx, kb.ID)
		if err != nil {
			continue
		}

		var expectedChunks int
		for _, doc := range docs {
			if doc.Status == kbtypes.DocStatusReady {
				expectedChunks += doc.ChunkCount
			}
		}

		actualChunks := verify[kb.ID]

		if expectedChunks != actualChunks {
			config := kb.Config
			if config == nil {
				config = make(map[string]any)
			}
			config["available"] = false
			config["unavailable_reason"] = fmt.Sprintf(
				"vector count mismatch: expected %d chunks, vector store has %d",
				expectedChunks, actualChunks,
			)

			s.db.WithContext(ctx).Model(&kbModel{}).Where("id = ?", kb.ID).
				Update("config", toJSONB(config))

			slog.Warn("consistency check: KB marked unavailable",
				"kb_id", kb.ID,
				"expected", expectedChunks,
				"actual", actualChunks,
			)
		} else {
			config := kb.Config
			if config == nil {
				config = make(map[string]any)
			}
			config["available"] = true
			delete(config, "unavailable_reason")

			s.db.WithContext(ctx).Model(&kbModel{}).Where("id = ?", kb.ID).
				Update("config", toJSONB(config))
		}
	}
}

func toJSONB(m map[string]any) jsonb {
	if m == nil {
		return jsonb{}
	}
	return jsonb(m)
}