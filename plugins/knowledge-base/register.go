package knowledgebase

import (
	"context"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/storage"
)

// MemoryStoreDeps defines the narrow interface knowledge-base needs
// from a memory store, using shared types from core/storage.
type MemoryStoreDeps interface {
	Store(ctx context.Context, memory *storage.Memory) error
	Search(ctx context.Context, query []float32, limit int) ([]*storage.Memory, error)
}

func RegisterCapabilities(r *capabilities.Registry, ks KnowledgeStore, emb storage.Embedder, ms MemoryStoreDeps) {
	r.Register(&kbRecallHookCapabilityClosure{ks: ks, emb: emb})
	r.Register(&memoryPersistHookCapabilityClosure{emb: emb, ms: ms})
}
