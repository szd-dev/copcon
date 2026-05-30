package memoryfile

import (
	"context"

	"github.com/copcon/core/storage"
)

// MemoryStore persists vector memories.
type MemoryStore interface {
	Store(ctx context.Context, memory *storage.Memory) error
	Search(ctx context.Context, query []float32, limit int) ([]*storage.Memory, error)
	GetBySession(ctx context.Context, sessionID string, limit int) ([]*storage.Memory, error)
	DeleteBySession(ctx context.Context, sessionID string) error
	List(ctx context.Context, filter storage.MemoryFilter) ([]*storage.Memory, error)
	Get(ctx context.Context, id string) (*storage.Memory, error)
	Update(ctx context.Context, memory *storage.Memory) error
	Delete(ctx context.Context, id string) error
}