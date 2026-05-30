package storage

import "time"

// MemoryType represents the semantic category of a memory.
type MemoryType string

const (
	MemoryTypeEpisodic     MemoryType = "episodic"
	MemoryTypeSemantic     MemoryType = "semantic"
	MemoryTypeProcedural   MemoryType = "procedural"
	MemoryTypeConversation MemoryType = MemoryTypeEpisodic // backward-compatible alias
)

// MemoryFilter specifies filtering criteria for listing memories.
// Zero values mean "no filter" for each field.
type MemoryFilter struct {
	AgentID    string
	MemoryType []MemoryType
	Limit      int
	Offset     int
	Since      time.Time
	Until      time.Time
}

// Memory is a pure value type with no storage backend dependencies.
type Memory struct {
	ID         string
	Content    string
	AgentID    string
	Role       string
	Timestamp  time.Time
	MemoryType string
	Metadata   map[string]any
	Score      float32
	ValidAt    *time.Time
	InvalidAt  *time.Time
	Importance float64
}