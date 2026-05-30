package memoryfile

import (
	"context"

	memtypes "github.com/copcon/plugins/memory-file/types"
)

// MemoryStore persists vector memories.
type MemoryStore interface {
	Store(ctx context.Context, memory *memtypes.Memory) error
	Search(ctx context.Context, query []float32, limit int) ([]*memtypes.Memory, error)
	GetByAgentID(ctx context.Context, agentID string, limit int) ([]*memtypes.Memory, error)
	DeleteByAgentID(ctx context.Context, agentID string) error
	List(ctx context.Context, filter memtypes.MemoryFilter) ([]*memtypes.Memory, error)
	Get(ctx context.Context, id string) (*memtypes.Memory, error)
	Update(ctx context.Context, memory *memtypes.Memory) error
	Delete(ctx context.Context, id string) error
}