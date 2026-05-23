package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Session is a pure value type with no GORM annotations.
type Session struct {
	ID              uuid.UUID
	Title           string
	DefaultAgentID  string
	ParentSessionID *uuid.UUID
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SessionStore persists sessions. SessionID is passed explicitly (not via ChatContext)
// to keep the storage layer independent of the request lifecycle.
type SessionStore interface {
	Create(ctx context.Context, session *Session) (*Session, error)
	Get(ctx context.Context, id uuid.UUID) (*Session, error)
	List(ctx context.Context, limit, offset int) ([]*Session, int64, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateTitle(ctx context.Context, id uuid.UUID, title string) error
	UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]any) error
	GetMessageCount(ctx context.Context, sessionID uuid.UUID) (int64, error)
	AppendMetadata(ctx context.Context, id uuid.UUID, key string, value any) error
}