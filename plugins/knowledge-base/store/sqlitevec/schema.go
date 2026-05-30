package sqlitevec

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/copcon/core/storage"
)

type jsonb map[string]any

func (j jsonb) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *jsonb) Scan(value any) error {
	if value == nil {
		*j = make(map[string]any)
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

func (jsonb) GormDataType() string { return "text" }

type kbModel struct {
	ID        string    `gorm:"primaryKey;size:64"`
	Name      string    `gorm:"size:255;not null"`
	Backend   string    `gorm:"size:64;not null"`
	Config    jsonb     `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	Metadata  jsonb     `gorm:"type:text"`
}

func (kbModel) TableName() string { return "knowledge_bases" }

func (m *kbModel) toDomain() *storage.KnowledgeBase {
	return &storage.KnowledgeBase{
		ID:        m.ID,
		Name:      m.Name,
		Backend:   m.Backend,
		Config:    map[string]any(m.Config),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
		Metadata:  map[string]any(m.Metadata),
	}
}

type docModel struct {
	ID         string    `gorm:"primaryKey;size:64"`
	KBID       string    `gorm:"size:64;not null;index"`
	Filename   string    `gorm:"size:512"`
	Source     string    `gorm:"size:64"`
	Status     string    `gorm:"size:32;not null;default:'pending'"`
	ChunkCount int
	TokenCount int
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
	Metadata   jsonb     `gorm:"type:text"`
}

func (docModel) TableName() string { return "documents" }

func (m *docModel) toDomain() *storage.Document {
	return &storage.Document{
		ID:         m.ID,
		KBID:       m.KBID,
		Filename:   m.Filename,
		Source:     m.Source,
		Status:     storage.DocumentStatus(m.Status),
		ChunkCount: m.ChunkCount,
		TokenCount: m.TokenCount,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
		Metadata:   map[string]any(m.Metadata),
	}
}

type chunkModel struct {
	ID         string `gorm:"primaryKey;size:64"`
	DocumentID string `gorm:"size:64;not null;index"`
	KBID       string `gorm:"size:64;not null;index"`
	Content    string `gorm:"type:text;not null"`
	Context    string `gorm:"type:text"`
	ChunkIndex int    `gorm:"not null"`
	TokenCount int
	Vector     []byte `gorm:"type:blob"`
	Metadata   jsonb  `gorm:"type:text"`
}

func (chunkModel) TableName() string { return "chunks" }

func (m *chunkModel) toDomain() *storage.Chunk {
	return &storage.Chunk{
		ID:         m.ID,
		DocumentID: m.DocumentID,
		KBID:       m.KBID,
		Content:    m.Content,
		Context:    m.Context,
		Index:      m.ChunkIndex,
		TokenCount: m.TokenCount,
		Metadata:   map[string]any(m.Metadata),
	}
}