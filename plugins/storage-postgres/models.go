package postgres

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// --- JSONB custom types ---

// JSONB is a map[string]any with GORM JSONB support.
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

// PersistedPart represents a single message part stored in the database.
type PersistedPart struct {
	Type       string         `json:"type"`
	Text       string         `json:"text,omitempty"`
	State      string         `json:"state,omitempty"`
	ToolCallID string         `json:"toolCallId,omitempty"`
	ToolName   string         `json:"toolName,omitempty"`
	Args       string         `json:"args,omitempty"`
	Output     string         `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	Interrupt  map[string]any `json:"interrupt,omitempty"`
	StepIndex  int            `json:"stepIndex"`
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

	switch raw[0] {
	case 'n':
		*p = nil
		return nil
	case '{':
		*p = nil
		return nil
	}

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
		pp.Interrupt = mapVal(m, "interrupt")
		pp.StepIndex = intValFallback(m, "stepIndex", "step_index", 0)
		result = append(result, pp)
	}

	*p = result
	return nil
}

func (PersistedParts) GormDataType() string {
	return "jsonb"
}

// ToolCallModel represents a single tool invocation within a message.
type ToolCallModel struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function FunctionCallModel `json:"function"`
}

// FunctionCallModel describes the function name and arguments for a tool call.
type FunctionCallModel struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCallsModel is a slice of ToolCallModel with GORM JSONB support.
type ToolCallsModel []ToolCallModel

func (tc ToolCallsModel) Value() (driver.Value, error) {
	if tc == nil {
		return nil, nil
	}
	return json.Marshal(tc)
}

func (tc *ToolCallsModel) Scan(value interface{}) error {
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

func (ToolCallsModel) GormDataType() string {
	return "jsonb"
}

// UUIDArray stores []uuid.UUID in a PostgreSQL uuid[] column.
type UUIDArray []uuid.UUID

func (a UUIDArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	strs := make([]string, len(a))
	for i, u := range a {
		strs[i] = u.String()
	}
	return fmt.Sprintf("{%s}", strings.Join(strs, ",")), nil
}

func (a *UUIDArray) Scan(value any) error {
	if value == nil {
		*a = nil
		return nil
	}

	var str string
	switch v := value.(type) {
	case []byte:
		str = string(v)
	case string:
		str = v
	default:
		return fmt.Errorf("UUIDArray.Scan: unsupported type %T", value)
	}

	str = strings.Trim(str, "{}")
	if str == "" {
		*a = UUIDArray{}
		return nil
	}

	parts := strings.Split(str, ",")
	result := make(UUIDArray, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, " \"")
		if part == "" {
			continue
		}
		id, err := uuid.Parse(part)
		if err != nil {
			return fmt.Errorf("UUIDArray.Scan: parse uuid %q: %w", part, err)
		}
		result = append(result, id)
	}
	*a = result
	return nil
}

func (UUIDArray) GormDataType() string {
	return "uuid[]"
}

// --- GORM Models ---

// Session is the GORM model for the sessions table.
type Session struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Title           string     `gorm:"size:255" json:"title"`
	DefaultAgentID  string     `gorm:"size:64" json:"default_agent_id"`
	ParentSessionID *uuid.UUID `gorm:"type:uuid;index;constraint:OnDelete:RESTRICT" json:"parent_session_id,omitempty"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	Metadata        JSONB      `gorm:"type:jsonb" json:"metadata"`
	Messages        []Message  `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
	Todos           []Todo     `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Session) TableName() string { return "sessions" }

func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// Message is the GORM model for the messages table.
type Message struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID  uuid.UUID      `gorm:"type:uuid;not null;index" json:"session_id"`
	Role       string         `gorm:"size:20;not null" json:"role"`
	Content    string         `gorm:"type:text" json:"content,omitempty"`
	Reasoning  string         `gorm:"type:text" json:"reasoning,omitempty"`
	ToolCalls  ToolCallsModel `gorm:"type:jsonb" json:"tool_calls,omitempty"`
	ToolCallID string         `gorm:"size:255" json:"tool_call_id,omitempty"`
	Parts      PersistedParts `gorm:"type:jsonb" json:"parts,omitempty"`
	Model      string         `gorm:"size:100" json:"model,omitempty"`
	TokenCount int            `json:"token_count,omitempty"`
	DurationMs int64          `json:"duration_ms,omitempty"`
	CreatedAt  time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (Message) TableName() string { return "messages" }

func (m *Message) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// TodoStatus is the type for todo status values.
type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
	TodoStatusBlocked    TodoStatus = "blocked"
	TodoStatusFailed     TodoStatus = "failed"
)

// Todo is the GORM model for the todos table.
type Todo struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"session_id"`
	Content     string     `gorm:"not null" json:"content"`
	ActiveForm  string     `gorm:"size:255" json:"active_form,omitempty"`
	Status      TodoStatus `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	DependsOn   UUIDArray  `gorm:"type:uuid[];default:'{}'" json:"depends_on,omitempty"`
	Validation  string     `gorm:"type:text" json:"validation,omitempty"`
	Result      string     `gorm:"type:text" json:"result,omitempty"`
	RetryCount  int        `gorm:"default:0" json:"retry_count"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	Session *Session `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Todo) TableName() string { return "todos" }

// AutoMigrate runs GORM auto-migration for all models.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Session{}, &Message{}, &Todo{})
}

// --- Helper functions for PersistedParts.Scan ---

func strVal(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func strValFallback(m map[string]any, primary, fallback string) string {
	if s := strVal(m, primary); s != "" {
		return s
	}
	return strVal(m, fallback)
}

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

func mapVal(m map[string]any, key string) map[string]any {
	v, ok := m[key]
	if !ok {
		return nil
	}
	result, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return result
}
