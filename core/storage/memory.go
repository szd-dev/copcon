package storage

import (
	"context"
	"time"
)

// MemoryType represents the semantic category of a memory.
type MemoryType string

const (
	MemoryTypeEpisodic    MemoryType = "episodic"
	MemoryTypeSemantic    MemoryType = "semantic"
	MemoryTypeProcedural  MemoryType = "procedural"
	MemoryTypeConversation MemoryType = MemoryTypeEpisodic // backward-compatible alias
)

// MemoryFilter specifies filtering criteria for listing memories.
// Zero values mean "no filter" for each field.
type MemoryFilter struct {
	SessionID  string
	MemoryType []MemoryType
	Limit      int
	Offset     int
	Since      time.Time
	Until      time.Time
}

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
	ValidAt    *time.Time
	InvalidAt  *time.Time
	Importance float64
}

// MemoryStore persists vector memories.
type MemoryStore interface {
	Store(ctx context.Context, memory *Memory) error
	Search(ctx context.Context, query []float32, limit int) ([]*Memory, error)
	GetBySession(ctx context.Context, sessionID string, limit int) ([]*Memory, error)
	DeleteBySession(ctx context.Context, sessionID string) error
	List(ctx context.Context, filter MemoryFilter) ([]*Memory, error)
	Get(ctx context.Context, id string) (*Memory, error)
	Update(ctx context.Context, memory *Memory) error
	Delete(ctx context.Context, id string) error
}