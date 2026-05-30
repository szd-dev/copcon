package sqlitevec

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/core/storage"
	"github.com/copcon/plugins/knowledge-base"
)

// KnowledgeStore implements knowledgebase.KnowledgeStore using SQLite + GORM
// with vector search via sqlite-vec vec0 virtual tables.
type KnowledgeStore struct {
	db        *gorm.DB
	sqlDB     *sql.DB
	dimension int
}

// Option configures a KnowledgeStore.
type Option func(*KnowledgeStore)

// WithDimension sets the vector dimension for the vec0 virtual table.
// Default is 1536 (text-embedding-3-small). Must be set before first use.
func WithDimension(d int) Option {
	return func(ks *KnowledgeStore) {
		ks.dimension = d
	}
}

func NewKnowledgeStore(db *gorm.DB, opts ...Option) (*KnowledgeStore, error) {
	ks := &KnowledgeStore{db: db, dimension: 1536}
	for _, opt := range opts {
		opt(ks)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying sql.DB: %w", err)
	}
	ks.sqlDB = sqlDB

	if err := db.AutoMigrate(
		&kbModel{},
		&docModel{},
		&chunkModel{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrate knowledge schema: %w", err)
	}

	if err := ks.initVectorTable(context.Background()); err != nil {
		return nil, fmt.Errorf("init vector table: %w", err)
	}

	return ks, nil
}

func NewKnowledgeStoreFromDSN(dsn string, opts ...Option) (*KnowledgeStore, error) {
	db, err := gorm.Open(openDialector(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	return NewKnowledgeStore(db, opts...)
}

var _ knowledgebase.KnowledgeStore = (*KnowledgeStore)(nil)

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
	rows, err := s.sqlDB.QueryContext(ctx, "SELECT rowid FROM chunks_vec WHERE kb_id = ?", id)
	if err == nil {
		var rowIDs []int64
		for rows.Next() {
			var rid int64
			if rows.Scan(&rid) == nil {
				rowIDs = append(rowIDs, rid)
			}
		}
		rows.Close()
		for _, rid := range rowIDs {
			s.sqlDB.ExecContext(ctx, "DELETE FROM chunks_vec WHERE rowid = ?", rid)
		}
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
	var chunkIDs []string
	if err := s.db.WithContext(ctx).Model(&chunkModel{}).Where("document_id = ? AND kb_id = ?", docID, kbID).Pluck("id", &chunkIDs).Error; err != nil {
		return fmt.Errorf("find chunks for document %s: %w", docID, err)
	}
	for _, cid := range chunkIDs {
		s.sqlDB.ExecContext(ctx, "DELETE FROM chunks_vec WHERE rowid = ?", chunkIDToRowID(cid))
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

	queryBlob := toBlob(query)

	type vecResult struct {
		chunkID  string
		cosineSim float32
	}
	var allResults []vecResult

	for _, kbID := range kbIDs {
		rows, err := s.sqlDB.QueryContext(ctx, `
			SELECT chunk_id, vec_distance_cosine(embedding, ?) as cosine_dist
			FROM chunks_vec
			WHERE embedding MATCH ? AND kb_id = ?
			ORDER BY distance
			LIMIT ?
		`, queryBlob, queryBlob, kbID, topK+len(kbIDs))
		if err != nil {
			return nil, fmt.Errorf("search chunks: %w", err)
		}
		for rows.Next() {
			var r vecResult
			var cosDist float64
			if err := rows.Scan(&r.chunkID, &cosDist); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan search result: %w", err)
			}
			r.cosineSim = float32(1.0 - cosDist)
			if r.cosineSim >= threshold {
				allResults = append(allResults, r)
			}
		}
		rows.Close()
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].cosineSim > allResults[j].cosineSim
	})

	if len(allResults) > topK {
		allResults = allResults[:topK]
	}

	chunks := make([]*storage.Chunk, 0, len(allResults))
	for _, r := range allResults {
		var m chunkModel
		if err := s.db.WithContext(ctx).Where("id = ?", r.chunkID).First(&m).Error; err != nil {
			continue
		}
		c := m.toDomain()
		c.Score = r.cosineSim
		chunks = append(chunks, c)
	}

	return chunks, nil
}

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

			rowID := chunkIDToRowID(chunkID)
			vecBlob := toBlob(vectors[i])
			if len(vecBlob) > 0 {
				if err := tx.Exec(
					"INSERT INTO chunks_vec(rowid, embedding, chunk_id, kb_id) VALUES (?, ?, ?, ?)",
					rowID, vecBlob, chunkID, kbID,
				).Error; err != nil {
					return fmt.Errorf("store chunk %d in vec: %w", i, err)
				}
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