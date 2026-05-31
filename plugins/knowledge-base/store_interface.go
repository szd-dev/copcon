package knowledgebase

import (
	"context"

	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

// KnowledgeStore persists knowledge bases, documents, and chunks with vector search.
type KnowledgeStore interface {
	// Knowledge base lifecycle
	CreateKB(ctx context.Context, kb *kbtypes.KnowledgeBase) (*kbtypes.KnowledgeBase, error)
	DeleteKB(ctx context.Context, id string) error
	ListKBs(ctx context.Context) ([]*kbtypes.KnowledgeBase, error)
	GetKB(ctx context.Context, id string) (*kbtypes.KnowledgeBase, error)

	// Document ingestion and management
	IngestDocument(ctx context.Context, kbID string, doc *kbtypes.Document, content []byte) error
	ListDocuments(ctx context.Context, kbID string) ([]*kbtypes.Document, error)
	DeleteDocument(ctx context.Context, kbID string, docID string) error
	GetDocument(ctx context.Context, kbID string, docID string) (*kbtypes.Document, error)
	UpdateDocumentStatus(ctx context.Context, kbID string, docID string, status kbtypes.DocumentStatus) error
	UpdateDocumentErrorMsg(ctx context.Context, kbID string, docID string, msg string) error
	ListDocumentsByStatus(ctx context.Context, statuses []string) ([]*kbtypes.Document, error)
	ClaimDocumentStatus(ctx context.Context, docID string, newStatus string, expectedStatus string) (bool, error)

	// Chunk access and mutation
	StoreChunks(ctx context.Context, kbID string, docID string, chunks []*kbtypes.Chunk, vectors [][]float32) error
	GetChunks(ctx context.Context, kbID string, docID string) ([]*kbtypes.Chunk, error)
	UpdateChunk(ctx context.Context, kbID string, chunk *kbtypes.Chunk) error

	// Semantic search across one or more knowledge bases
	Search(ctx context.Context, kbIDs []string, query []float32, opts kbtypes.SearchOptions) ([]*kbtypes.Chunk, error)
}
