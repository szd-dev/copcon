package memory

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"

	"github.com/copcon/core/iface"
)

var (
	ErrMemoryNotFound = errors.New("memory not found")
)

type MemoryManager interface {
	Store(chatCtx iface.ChatContextInterface, memory *Memory) error
	Search(chatCtx iface.ChatContextInterface, query []float32, limit int) ([]*Memory, error)
	GetBySession(chatCtx iface.ChatContextInterface, limit int) ([]*Memory, error)
	DeleteBySession(chatCtx iface.ChatContextInterface) error
}

type Memory struct {
	ID         string         `json:"id"`
	Content    string         `json:"content"`
	SessionID  string         `json:"session_id"`
	Role       string         `json:"role"`
	Timestamp  int64          `json:"timestamp"`
	MemoryType string         `json:"memory_type"`
	Metadata   map[string]any `json:"metadata"`
	Score      float32        `json:"score,omitempty"`
}

type memoryManager struct {
	client     *qdrant.Client
	collection string
}

func NewMemoryManager(client *qdrant.Client, collection string) MemoryManager {
	return &memoryManager{
		client:     client,
		collection: collection,
	}
}

func (m *memoryManager) Store(chatCtx iface.ChatContextInterface, memory *Memory) error {
	if memory.ID == "" {
		memory.ID = uuid.New().String()
	}
	if memory.Timestamp == 0 {
		memory.Timestamp = time.Now().Unix()
	}
	if memory.MemoryType == "" {
		memory.MemoryType = "conversation"
	}

	metadata := map[string]any{
		"content":     memory.Content,
		"session_id":  memory.SessionID,
		"role":        memory.Role,
		"timestamp":   memory.Timestamp,
		"memory_type": memory.MemoryType,
	}

	for k, v := range memory.Metadata {
		metadata[k] = v
	}

	points := []*qdrant.PointStruct{
		{
			Id:      &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: memory.ID}},
			Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vector{Vector: &qdrant.Vector{Data: []float32{}}}},
			Payload: qdrant.NewValueMap(metadata),
		},
	}

	_, err := m.client.Upsert(chatCtx.Context(), &qdrant.UpsertPoints{
		CollectionName: m.collection,
		Points:         points,
	})

	return err
}

func (m *memoryManager) Search(chatCtx iface.ChatContextInterface, query []float32, limit int) ([]*Memory, error) {
	filter := &qdrant.Filter{}

	sessionID := chatCtx.SessionID()
	if sessionID != "" {
		filter.Must = []*qdrant.Condition{
			qdrant.NewMatch("session_id", sessionID),
		}
	}

	results, err := m.client.Query(chatCtx.Context(), &qdrant.QueryPoints{
		CollectionName: m.collection,
		Query:          qdrant.NewQuery(query...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
		Filter:         filter,
	})
	if err != nil {
		return nil, err
	}

	memories := make([]*Memory, 0, len(results))
	for _, result := range results {
		memory := &Memory{
			Score: result.Score,
		}

		if result.Id != nil {
			switch id := result.Id.PointIdOptions.(type) {
			case *qdrant.PointId_Uuid:
				memory.ID = id.Uuid
			case *qdrant.PointId_Num:
				memory.ID = string(rune(id.Num))
			}
		}

		if payload := result.Payload; payload != nil {
			if v, ok := payload["content"]; ok {
				memory.Content = v.GetStringValue()
			}
			if v, ok := payload["session_id"]; ok {
				memory.SessionID = v.GetStringValue()
			}
			if v, ok := payload["role"]; ok {
				memory.Role = v.GetStringValue()
			}
			if v, ok := payload["timestamp"]; ok {
				memory.Timestamp = v.GetIntegerValue()
			}
			if v, ok := payload["memory_type"]; ok {
				memory.MemoryType = v.GetStringValue()
			}
		}

		memories = append(memories, memory)
	}

	return memories, nil
}

func (m *memoryManager) GetBySession(chatCtx iface.ChatContextInterface, limit int) ([]*Memory, error) {
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("session_id", chatCtx.SessionID()),
		},
	}

	results, err := m.client.Scroll(chatCtx.Context(), &qdrant.ScrollPoints{
		CollectionName: m.collection,
		Filter:         filter,
		Limit:          qdrant.PtrOf(uint32(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	memories := make([]*Memory, 0, len(results))
	for _, result := range results {
		memory := &Memory{}

		if result.Id != nil {
			switch id := result.Id.PointIdOptions.(type) {
			case *qdrant.PointId_Uuid:
				memory.ID = id.Uuid
			case *qdrant.PointId_Num:
				memory.ID = string(rune(id.Num))
			}
		}

		if payload := result.Payload; payload != nil {
			if v, ok := payload["content"]; ok {
				memory.Content = v.GetStringValue()
			}
			if v, ok := payload["session_id"]; ok {
				memory.SessionID = v.GetStringValue()
			}
			if v, ok := payload["role"]; ok {
				memory.Role = v.GetStringValue()
			}
			if v, ok := payload["timestamp"]; ok {
				memory.Timestamp = v.GetIntegerValue()
			}
			if v, ok := payload["memory_type"]; ok {
				memory.MemoryType = v.GetStringValue()
			}
		}

		memories = append(memories, memory)
	}

	return memories, nil
}

func (m *memoryManager) DeleteBySession(chatCtx iface.ChatContextInterface) error {
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("session_id", chatCtx.SessionID()),
		},
	}

	_, err := m.client.Delete(chatCtx.Context(), &qdrant.DeletePoints{
		CollectionName: m.collection,
		Points:         &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Filter{Filter: filter}},
	})

	return err
}

func EstimateTokens(content string) int {
	return len(content) / 4
}

func ToJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
