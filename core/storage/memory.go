package storage

import (
	"context"
	"time"
)

// Memory is a pure value type with no Qdrant client dependencies.
type Memory struct {
	ID         string
	Content    string
	SessionID  string
	Role       string
	Timestamp  time.Time
	MemoryType string
	Metadata   map[string]any
	Score      float32
}

// MemoryStore persists vector memories.
type MemoryStore interface {
	Store(ctx context.Context, memory *Memory) error
	Search(ctx context.Context, query []float32, limit int) ([]*Memory, error)
	GetBySession(ctx context.Context, sessionID string, limit int) ([]*Memory, error)
	DeleteBySession(ctx context.Context, sessionID string) error
}