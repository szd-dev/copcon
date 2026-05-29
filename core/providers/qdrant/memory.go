package qdrant

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"

	"github.com/copcon/core/storage"
)

var ErrMemoryNotFound = errors.New("memory not found")

type MemoryStore struct {
	client     *qdrant.Client
	collection string
}

var _ storage.MemoryStore = (*MemoryStore)(nil)

func NewMemoryStore(client *qdrant.Client, collection string) *MemoryStore {
	return &MemoryStore{
		client:     client,
		collection: collection,
	}
}

func (m *MemoryStore) Store(ctx context.Context, memory *storage.Memory) error {
	if memory.ID == "" {
		memory.ID = uuid.New().String()
	}
	if memory.Timestamp.IsZero() {
		memory.Timestamp = time.Now()
	}
	if memory.MemoryType == "" {
		memory.MemoryType = "conversation"
	}

	metadata := map[string]any{
		"content":     memory.Content,
		"session_id":  memory.SessionID,
		"role":        memory.Role,
		"timestamp":   memory.Timestamp.Unix(),
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

	_, err := m.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: m.collection,
		Points:         points,
	})

	return err
}

func (m *MemoryStore) Search(ctx context.Context, query []float32, limit int) ([]*storage.Memory, error) {
	filter := &qdrant.Filter{}

	results, err := m.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: m.collection,
		Query:          qdrant.NewQuery(query...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
		Filter:         filter,
	})
	if err != nil {
		return nil, err
	}

	memories := make([]*storage.Memory, 0, len(results))
	for _, result := range results {
		memory := pointToMemory(result.Id, result.Payload)
		memory.Score = result.Score
		memories = append(memories, memory)
	}

	return memories, nil
}

func (m *MemoryStore) GetBySession(ctx context.Context, sessionID string, limit int) ([]*storage.Memory, error) {
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("session_id", sessionID),
		},
	}

	results, err := m.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: m.collection,
		Filter:         filter,
		Limit:          qdrant.PtrOf(uint32(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	memories := make([]*storage.Memory, 0, len(results))
	for _, result := range results {
		memories = append(memories, pointToMemory(result.Id, result.Payload))
	}

	return memories, nil
}

func (m *MemoryStore) DeleteBySession(ctx context.Context, sessionID string) error {
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("session_id", sessionID),
		},
	}

	_, err := m.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: m.collection,
		Points:         &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Filter{Filter: filter}},
	})

	return err
}

func (m *MemoryStore) List(ctx context.Context, filter storage.MemoryFilter) ([]*storage.Memory, error) {
	return nil, errors.New("not yet implemented: List")
}

func (m *MemoryStore) Get(ctx context.Context, id string) (*storage.Memory, error) {
	return nil, errors.New("not yet implemented: Get")
}

func (m *MemoryStore) Update(ctx context.Context, memory *storage.Memory) error {
	return errors.New("not yet implemented: Update")
}

func (m *MemoryStore) Delete(ctx context.Context, id string) error {
	return errors.New("not yet implemented: Delete")
}

func pointToMemory(id *qdrant.PointId, payload map[string]*qdrant.Value) *storage.Memory {
	memory := &storage.Memory{}

	if id != nil {
		switch pid := id.PointIdOptions.(type) {
		case *qdrant.PointId_Uuid:
			memory.ID = pid.Uuid
		case *qdrant.PointId_Num:
			memory.ID = string(rune(pid.Num))
		}
	}

	if payload != nil {
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
			memory.Timestamp = time.Unix(v.GetIntegerValue(), 0)
		}
		if v, ok := payload["memory_type"]; ok {
			memory.MemoryType = v.GetStringValue()
		}
	}

	return memory
}
