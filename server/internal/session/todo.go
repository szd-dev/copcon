package session

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
	TodoStatusBlocked    TodoStatus = "blocked"
	TodoStatusFailed     TodoStatus = "failed"
)

// UUIDArray stores []uuid.UUID in a PostgreSQL uuid[] column.
// pgx returns uuid[] as a string in PostgreSQL array literal format
// (e.g. "{uuid1,uuid2}"), so Scan parses that format. Value produces
// the same format for writes.
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

func (Todo) TableName() string {
	return "todos"
}
