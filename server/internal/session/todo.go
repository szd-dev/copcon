package session

import (
	"time"

	"github.com/google/uuid"
)

// TodoStatus represents the status of a todo item
type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
	TodoStatusBlocked    TodoStatus = "blocked"
	TodoStatusFailed     TodoStatus = "failed"
)

// Todo represents a task item associated with a session
type Todo struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID   uuid.UUID   `gorm:"type:uuid;not null;index" json:"session_id"`
	Content     string      `gorm:"not null" json:"content"`
	ActiveForm  string      `gorm:"size:255" json:"active_form,omitempty"`
	Status      TodoStatus  `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	DependsOn   []uuid.UUID `gorm:"type:uuid[];default:'{}'" json:"depends_on,omitempty"`
	Validation  string      `gorm:"type:text" json:"validation,omitempty"`
	Result      string      `gorm:"type:text" json:"result,omitempty"`
	RetryCount  int         `gorm:"default:0" json:"retry_count"`
	CreatedAt   time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time   `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`

	Session *Session `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
}

// TableName returns the table name for the Todo model
func (Todo) TableName() string {
	return "todos"
}
