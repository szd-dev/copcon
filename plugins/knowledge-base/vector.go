package knowledgebase

import (
	"context"

	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

// VectorChunk is a lightweight value type for vector storage operations.
// Unlike types.Chunk, it contains only fields needed by the vector layer.
type VectorChunk struct {
	ID         string
	DocumentID string
	KBID       string
	Content    string
	Index      int
	Score      float32
}

// SearchResult represents a single vector search hit.
type SearchResult struct {
	ChunkID string
	KBID    string
	Score   float32
}

// VectorStore abstracts vector storage and similarity search backends.
// Implementations may use sqlite-vec, brute-force cosine similarity,
// or external vector databases like Qdrant.
type VectorStore interface {
	// Store persists vector embeddings for the given chunks.
	Store(ctx context.Context, kbID, docID string, chunks []VectorChunk, vectors [][]float32) error

	// Search performs similarity search across one or more knowledge bases.
	Search(ctx context.Context, kbIDs []string, query []float32, opts kbtypes.SearchOptions) ([]SearchResult, error)

	// DeleteByKB removes all vectors associated with a knowledge base.
	DeleteByKB(ctx context.Context, kbID string) error

	// DeleteByDocument removes all vectors associated with a document.
	DeleteByDocument(ctx context.Context, kbID, docID string) error

	// Backend returns a human-readable identifier for this implementation.
	Backend() string

	// Verify returns the actual chunk count per KB in the vector store.
	// Used for consistency checking against metadata.
	Verify(ctx context.Context) (map[string]int, error)
}