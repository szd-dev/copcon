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

// Deprecated: use PersistedParts instead.
type UIParts []map[string]any

func (p UIParts) Value() (driver.Value, error) {
	if p == nil {
		return nil, nil
	}
	return json.Marshal(p)
}

func (p *UIParts) Scan(value interface{}) error {
	if value == nil {
		*p = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	if len(bytes) == 0 {
		*p = nil
		return nil
	}

	var raw json.RawMessage
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return err
	}

	switch raw[0] {
	case 'n':
		*p = nil
		return nil
	case '[':
		return json.Unmarshal(bytes, p)
	case '{':
		*p = nil
		return nil
	default:
		return json.Unmarshal(bytes, p)
	}
}

func (UIParts) GormDataType() string {
	return "jsonb"
}

// PersistedPart represents a single message part stored in the database.
// JSON tags use camelCase matching entity.UIPart conventions.
type PersistedPart struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	State      string `json:"state,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Args       string `json:"args,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	StepIndex  int    `json:"stepIndex"`
}

// PersistedParts is a slice of PersistedPart with GORM JSONB support.
type PersistedParts []PersistedPart

func (p PersistedParts) Value() (driver.Value, error) {
	if p == nil {
		return nil, nil
	}
	return json.Marshal(p)
}

func (p *PersistedParts) Scan(value interface{}) error {
	if value == nil {
		*p = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	if len(bytes) == 0 {
		*p = nil
		return nil
	}

	var raw json.RawMessage
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return err
	}

	// Handle non-array JSON: null, object, etc.
	switch raw[0] {
	case 'n': // null
		*p = nil
		return nil
	case '{': // single object — not a valid array
		*p = nil
		return nil
	}

	// Two-pass: unmarshal into generic maps, then map keys with
	// camelCase-first, snake_case-fallback for legacy compatibility.
	var generic []map[string]any
	if err := json.Unmarshal(bytes, &generic); err != nil {
		return err
	}

	result := make(PersistedParts, 0, len(generic))
	for _, m := range generic {
		var pp PersistedPart
		pp.Type = normalizePartType(strVal(m, "type"))
		pp.Text = strValFallback(m, "text", "text_delta")
		pp.State = strVal(m, "state")
		pp.ToolCallID = strValFallback(m, "toolCallId", "tool_call_id")
		pp.ToolName = strValFallback(m, "toolName", "tool_name")
		pp.Args = strVal(m, "args")
		pp.Output = strVal(m, "output")
		pp.Error = strVal(m, "error")
		pp.StepIndex = intValFallback(m, "stepIndex", "step_index", 0)
		result = append(result, pp)
	}

	*p = result
	return nil
}

func (PersistedParts) GormDataType() string {
	return "jsonb"
}

// strVal returns the string value for key, or "" if absent.
func strVal(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// strValFallback tries primary key first, then fallback key.
func strValFallback(m map[string]any, primary, fallback string) string {
	if s := strVal(m, primary); s != "" {
		return s
	}
	return strVal(m, fallback)
}

// intValFallback tries primary key first, then fallback key, returning def if both absent.
func intValFallback(m map[string]any, primary, fallback string, def int) int {
	if v, ok := m[primary]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	if v, ok := m[fallback]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

// normalizePartType maps legacy type values to their canonical forms.
// Legacy: "text_delta" → "text", "tool_call" → "tool-call".
func normalizePartType(t string) string {
	switch t {
	case "text_delta":
		return "text"
	case "tool_call":
		return "tool-call"
	default:
		return t
	}
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
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Title          string    `gorm:"size:255" json:"title"`
	DefaultAgentID string    `gorm:"size:64" json:"default_agent_id"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	Metadata       JSONB     `gorm:"type:jsonb" json:"metadata"`
	Messages       []Message `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
	Todos          []Todo    `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
}

type Message struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID  uuid.UUID      `gorm:"type:uuid;not null;index" json:"session_id"`
	Role       string         `gorm:"size:20;not null" json:"role"`
	Content    string         `gorm:"type:text" json:"content,omitempty"`
	Reasoning  string         `gorm:"type:text" json:"reasoning,omitempty"`
	ToolCalls  ToolCalls      `gorm:"type:jsonb" json:"tool_calls,omitempty"`
	ToolCallID string         `gorm:"size:255" json:"tool_call_id,omitempty"`
	Parts      PersistedParts `gorm:"type:jsonb" json:"parts,omitempty"`
	Model      string         `gorm:"size:100" json:"model,omitempty"`
	TokenCount int            `json:"token_count,omitempty"`
	DurationMs int64          `json:"duration_ms,omitempty"`
	CreatedAt  time.Time      `gorm:"autoCreateTime" json:"created_at"`
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
