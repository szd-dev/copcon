package sqlitevec

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"sort"

	knowledgebase "github.com/copcon/plugins/knowledge-base"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

// SQLiteVecStore implements VectorStore using the sqlite-vec extension.
// It stores vectors in a vec0 virtual table (chunks_vec) for KNN search.
type SQLiteVecStore struct {
	sqlDB *sql.DB
	dim   int
}

// Compile-time interface check
var _ knowledgebase.VectorStore = (*SQLiteVecStore)(nil)

// New creates a SQLiteVecStore backed by the given *sql.DB with sqlite-vec extension.
func New(sqlDB *sql.DB, dim int) *SQLiteVecStore {
	return &SQLiteVecStore{sqlDB: sqlDB, dim: dim}
}

// InitVectorTable creates the vec0 virtual table if it doesn't exist.
// Must be called once after construction.
func (s *SQLiteVecStore) InitVectorTable(ctx context.Context) error {
	_, err := s.sqlDB.ExecContext(ctx, fmt.Sprintf(
		"CREATE VIRTUAL TABLE IF NOT EXISTS chunks_vec USING vec0(embedding float[%d], chunk_id TEXT, kb_id TEXT)",
		s.dim,
	))
	return err
}

func chunkIDToRowID(id string) int64 {
	h := fnv.New64a()
	h.Write([]byte(id))
	return int64(h.Sum64() >> 1)
}

// Store inserts vectors into the chunks_vec virtual table.
func (s *SQLiteVecStore) Store(ctx context.Context, kbID, docID string, chunks []knowledgebase.VectorChunk, vectors [][]float32) error {
	for i, c := range chunks {
		rowID := chunkIDToRowID(c.ID)
		vecBlob := toBlob(vectors[i])
		if len(vecBlob) > 0 {
			if _, err := s.sqlDB.ExecContext(ctx,
				"INSERT INTO chunks_vec(rowid, embedding, chunk_id, kb_id) VALUES (?, ?, ?, ?)",
				rowID, vecBlob, c.ID, kbID,
			); err != nil {
				return fmt.Errorf("store chunk %d in vec: %w", i, err)
			}
		}
	}
	return nil
}

// Search performs KNN search using sqlite-vec's vec_distance_cosine.
func (s *SQLiteVecStore) Search(ctx context.Context, kbIDs []string, query []float32, opts kbtypes.SearchOptions) ([]knowledgebase.SearchResult, error) {
	if len(query) == 0 {
		return nil, nil
	}

	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}
	threshold := opts.SimilarityThreshold

	queryBlob := toBlob(query)

	type vecResult struct {
		chunkID   string
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
			return nil, fmt.Errorf("search chunks_vec: %w", err)
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

	out := make([]knowledgebase.SearchResult, len(allResults))
	for i, r := range allResults {
		out[i] = knowledgebase.SearchResult{
			ChunkID: r.chunkID,
			KBID:    "", // not available from vec0 directly, caller fills from chunks table
			Score:   r.cosineSim,
		}
	}
	return out, nil
}

// DeleteByKB removes all vectors for a knowledge base from chunks_vec.
func (s *SQLiteVecStore) DeleteByKB(ctx context.Context, kbID string) error {
	rows, err := s.sqlDB.QueryContext(ctx, "SELECT rowid FROM chunks_vec WHERE kb_id = ?", kbID)
	if err != nil {
		return nil // table may not exist yet
	}
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
	return nil
}

// DeleteByDocument removes all vectors for a document from chunks_vec.
// It queries the chunks table to find chunk IDs, then deletes corresponding rows from chunks_vec,
// because chunks_vec lacks a doc_id column.
func (s *SQLiteVecStore) DeleteByDocument(ctx context.Context, kbID, docID string) error {
	rows, err := s.sqlDB.QueryContext(ctx,
		"SELECT id FROM chunks WHERE document_id = ? AND kb_id = ?", docID, kbID)
	if err != nil {
		return nil // chunks table may not have these yet
	}
	var chunkIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			chunkIDs = append(chunkIDs, id)
		}
	}
	rows.Close()

	for _, cid := range chunkIDs {
		s.sqlDB.ExecContext(ctx, "DELETE FROM chunks_vec WHERE rowid = ?", chunkIDToRowID(cid))
	}
	return nil
}

// Backend returns the identifier for this implementation.
func (s *SQLiteVecStore) Backend() string {
	return "sqlite-vec"
}

// Verify returns actual chunk count in chunks_vec per KB.
func (s *SQLiteVecStore) Verify(ctx context.Context) (map[string]int, error) {
	type row struct {
		KBID  string
		Count int
	}
	rows, err := s.sqlDB.QueryContext(ctx, "SELECT kb_id, COUNT(*) FROM chunks_vec GROUP BY kb_id")
	if err != nil {
		return nil, fmt.Errorf("verify chunks_vec: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.KBID, &r.Count); err != nil {
			return nil, fmt.Errorf("scan verify row: %w", err)
		}
		result[r.KBID] = r.Count
	}
	return result, nil
}
