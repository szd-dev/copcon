package hooks

import (
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
)

type MemoryManager interface {
	Store(chatCtx iface.ChatContextInterface, memory *storage.Memory) error
	Search(chatCtx iface.ChatContextInterface, query []float32, limit int) ([]*storage.Memory, error)
}
