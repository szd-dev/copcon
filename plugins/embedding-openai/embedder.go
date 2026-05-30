// Package embedding defines the provider abstraction for text embedding backends.
//
// It provides a single Embedder interface that decouples the RAG pipeline from
// concrete embedding implementations (OpenAI, BGE-M3, etc.).
package embedding

import "context"

// Embedder is the interface for any text embedding backend.
// Implementations convert text inputs into dense vector representations.
type Embedder interface {
	// Embed converts a single text string into a dense vector.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch converts a batch of texts into dense vectors.
	// The returned slice has the same length as texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the number of dimensions in the embedding vectors.
	// For example, text-embedding-3-small produces 1536-dimensional vectors.
	Dimensions() int

	// Name returns a human-readable identifier for this embedder instance.
	// Example: "openai:text-embedding-3-small".
	Name() string
}