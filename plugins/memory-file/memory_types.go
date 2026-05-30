package memoryfile

import (
	"github.com/copcon/core/iface"
)

type MemoryManager interface {
	Store(chatCtx iface.ChatContextInterface, memory *Memory) error
	Search(chatCtx iface.ChatContextInterface, query []float32, limit int) ([]*Memory, error)
}
