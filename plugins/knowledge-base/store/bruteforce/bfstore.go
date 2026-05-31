package bruteforce

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"

	"gorm.io/gorm"

	knowledgebase "github.com/copcon/plugins/knowledge-base"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

// BruteForceVectorStore implements VectorStore by loading vector blobs from
// the chunks table and computing cosine similarity in application memory.
// It requires zero external dependencies beyond GORM.
type BruteForceVectorStore struct {
	db *gorm.DB
}

// Compile-time interface check
var _ knowledgebase.VectorStore = (*BruteForceVectorStore)(nil)

// New creates a brute-force VectorStore backed by the given GORM connection.
func New(db *gorm.DB) *BruteForceVectorStore {
	return &BruteForceVectorStore{db: db}
}

// Store is a no-op — vector blobs are already persisted in the chunks table
// by KnowledgeStore's GORM write.
func (s *BruteForceVectorStore) Store(ctx context.Context, kbID, docID string, chunks []knowledgebase.VectorChunk, vectors [][]float32) error {
	return nil
}

// Search loads all vector blobs for the given KBs, decodes them, and performs
// brute-force cosine similarity ranking.
func (s *BruteForceVectorStore) Search(ctx context.Context, kbIDs []string, query []float32, opts kbtypes.SearchOptions) ([]knowledgebase.SearchResult, error) {
	if len(query) == 0 {
		return nil, nil
	}

	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}
	threshold := opts.SimilarityThreshold

	type chunkVec struct {
		ID     string
		KBID   string
		Vector []byte
	}

	var rows []chunkVec
	if err := s.db.WithContext(ctx).
		Table("chunks").
		Select("id, kb_id, vector").
		Where("kb_id IN ? AND vector IS NOT NULL", kbIDs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load chunks for search: %w", err)
	}

	type scored struct {
		chunkID string
		kbID    string
		score   float32
	}
	var results []scored

	for _, r := range rows {
		vec := fromBlob(r.Vector)
		if len(vec) == 0 {
			continue
		}
		sim := cosineSimilarity(query, vec)
		if sim >= threshold {
			results = append(results, scored{chunkID: r.ID, kbID: r.KBID, score: sim})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	out := make([]knowledgebase.SearchResult, len(results))
	for i, r := range results {
		out[i] = knowledgebase.SearchResult{
			ChunkID: r.chunkID,
			KBID:    r.kbID,
			Score:   r.score,
		}
	}
	return out, nil
}

// DeleteByKB is a no-op — GORM cascade handles chunk deletion.
func (s *BruteForceVectorStore) DeleteByKB(ctx context.Context, kbID string) error {
	return nil
}

// DeleteByDocument is a no-op — GORM cascade handles chunk deletion.
func (s *BruteForceVectorStore) DeleteByDocument(ctx context.Context, kbID, docID string) error {
	return nil
}

// Backend returns the identifier for this implementation.
func (s *BruteForceVectorStore) Backend() string {
	return "brute-force"
}

// Verify returns the count of chunks with non-null vectors per KB.
func (s *BruteForceVectorStore) Verify(ctx context.Context) (map[string]int, error) {
	type row struct {
		KBID  string `gorm:"column:kb_id"`
		Count int    `gorm:"column:cnt"`
	}
	var rows []row
	if err := s.db.WithContext(ctx).
		Table("chunks").
		Select("kb_id, COUNT(*) as cnt").
		Where("vector IS NOT NULL").
		Group("kb_id").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("verify chunks: %w", err)
	}
	result := make(map[string]int, len(rows))
	for _, r := range rows {
		result[r.KBID] = r.Count
	}
	return result, nil
}

// --- vector encoding/decoding helpers ---

func toBlob(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func fromBlob(data []byte) []float32 {
	if len(data) == 0 || len(data)%4 != 0 {
		return nil
	}
	n := len(data) / 4
	vec := make([]float32, n)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return vec
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
