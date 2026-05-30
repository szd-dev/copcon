package knowledgebase

import (
	"context"

	"github.com/copcon/core/storage"
)

// KnowledgeStore persists knowledge bases, documents, and chunks with vector search.
type KnowledgeStore interface {
	// Knowledge base lifecycle
	CreateKB(ctx context.Context, kb *storage.KnowledgeBase) (*storage.KnowledgeBase, error)
	DeleteKB(ctx context.Context, id string) error
	ListKBs(ctx context.Context) ([]*storage.KnowledgeBase, error)
	GetKB(ctx context.Context, id string) (*storage.KnowledgeBase, error)

	// Document ingestion and management
	IngestDocument(ctx context.Context, kbID string, doc *storage.Document, content []byte) error
	ListDocuments(ctx context.Context, kbID string) ([]*storage.Document, error)
	DeleteDocument(ctx context.Context, kbID string, docID string) error
	GetDocument(ctx context.Context, kbID string, docID string) (*storage.Document, error)

	// Chunk access and mutation
	GetChunks(ctx context.Context, kbID string, docID string) ([]*storage.Chunk, error)
	UpdateChunk(ctx context.Context, kbID string, chunk *storage.Chunk) error

	// Semantic search across one or more knowledge bases
	Search(ctx context.Context, kbIDs []string, query []float32, opts storage.SearchOptions) ([]*storage.Chunk, error)
}
