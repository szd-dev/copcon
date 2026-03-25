package session

import (
	"database/sql/driver"
	"encoding/json"

	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type JSONB map[string]any

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONB) Scan(value interface{}) error {
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

func (JSONB) GormDataType() string {
	return "jsonb"
}

type ToolCalls []ToolCall

func (tc ToolCalls) Value() (driver.Value, error) {
	if tc == nil {
		return nil, nil
	}
	return json.Marshal(tc)
}

func (tc *ToolCalls) Scan(value interface{}) error {
	if value == nil {
		*tc = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	if len(bytes) == 0 {
		*tc = nil
		return nil
	}

	var raw json.RawMessage
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return err
	}

	switch raw[0] {
	case 'n':
		*tc = nil
		return nil
	case '[':
		return json.Unmarshal(bytes, tc)
	case '{':
		*tc = nil
		return nil
	default:
		return json.Unmarshal(bytes, tc)
	}
}

func (ToolCalls) GormDataType() string {
	return "jsonb"
}

type Session struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Title     string    `gorm:"size:255" json:"title"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	Metadata  JSONB     `gorm:"type:jsonb" json:"metadata"`
	Messages  []Message `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
}

type Message struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID  uuid.UUID `gorm:"type:uuid;not null;index" json:"session_id"`
	Role       string    `gorm:"size:20;not null" json:"role"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	Reasoning  string    `gorm:"type:text" json:"reasoning,omitempty"`
	ToolCalls  ToolCalls `gorm:"type:jsonb" json:"tool_calls,omitempty"`
	ToolCallID string    `gorm:"size:255" json:"tool_call_id,omitempty"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (Session) TableName() string {
	return "sessions"
}

func (Message) TableName() string {
	return "messages"
}

func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (m *Message) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
