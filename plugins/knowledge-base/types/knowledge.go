package types

import "time"

// DocumentStatus represents the processing state of a document.
type DocumentStatus string

const (
	DocStatusPending   DocumentStatus = "pending"
	DocStatusParsing   DocumentStatus = "parsing"
	DocStatusIndexing  DocumentStatus = "indexing"
	DocStatusReady     DocumentStatus = "ready"
	DocStatusError     DocumentStatus = "error"
)

// KnowledgeBase is a pure value type representing a collection of documents
// backed by a specific vector storage engine.
type KnowledgeBase struct {
	ID        string
	Name      string
	Backend   string
	Config    map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
	Metadata  map[string]any
}

// Document is a pure value type representing an ingested file within a knowledge base.
type Document struct {
	ID         string
	KBID       string
	Filename   string
	Source     string // "upload", "api", "sync"
	Status     DocumentStatus
	Content    string
	ErrorMsg   string
	ChunkCount int
	TokenCount int
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Metadata   map[string]any
}

// Chunk is a pure value type representing a segment of a document, stored as a
// vector embedding for semantic search.
type Chunk struct {
	ID         string
	DocumentID string
	KBID       string
	Content    string
	Context    string // reserved for Contextual Retrieval
	Index      int
	TokenCount int
	Metadata   map[string]any
	Score      float32
}

// SearchOptions configures a semantic search query against one or more knowledge bases.
type SearchOptions struct {
	TopK                int
	SimilarityThreshold float32
	Filters             map[string]any
}
