package adapter

import (
	"time"

	"github.com/copcon/core/capabilities/hooks"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/server/internal/memory"
)

type MemoryManagerAdapter struct {
	Inner memory.MemoryManager
}

func NewMemoryManagerAdapter(inner memory.MemoryManager) *MemoryManagerAdapter {
	return &MemoryManagerAdapter{Inner: inner}
}

func (a *MemoryManagerAdapter) Store(chatCtx iface.ChatContextInterface, m *storage.Memory) error {
	return a.Inner.Store(chatCtx, storageMemoryToServer(m))
}

func (a *MemoryManagerAdapter) Search(chatCtx iface.ChatContextInterface, query []float32, limit int) ([]*storage.Memory, error) {
	results, err := a.Inner.Search(chatCtx, query, limit)
	if err != nil {
		return nil, err
	}
	memories := make([]*storage.Memory, len(results))
	for i, r := range results {
		memories[i] = serverMemoryToStorage(r)
	}
	return memories, nil
}

func storageMemoryToServer(m *storage.Memory) *memory.Memory {
	if m == nil {
		return nil
	}
	var ts int64
	if !m.Timestamp.IsZero() {
		ts = m.Timestamp.Unix()
	}
	return &memory.Memory{
		ID:         m.ID,
		Content:    m.Content,
		SessionID:  m.SessionID,
		Role:       m.Role,
		Timestamp:  ts,
		MemoryType: m.MemoryType,
		Metadata:   m.Metadata,
		Score:      m.Score,
	}
}

func serverMemoryToStorage(m *memory.Memory) *storage.Memory {
	if m == nil {
		return nil
	}
	return &storage.Memory{
		ID:         m.ID,
		Content:    m.Content,
		SessionID:  m.SessionID,
		Role:       m.Role,
		Timestamp:  time.Unix(m.Timestamp, 0),
		MemoryType: m.MemoryType,
		Metadata:   m.Metadata,
		Score:      m.Score,
	}
}

var _ hooks.MemoryManager = (*MemoryManagerAdapter)(nil)
