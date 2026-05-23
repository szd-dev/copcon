package session

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

// Compile-time check: sessionManager satisfies storage.SessionStore.
var _ storage.SessionStore = (*sessionManager)(nil)

var (
	ErrSessionNotFound = errors.New("session not found")
)

// CreateOption configures the session creation.
type CreateOption func(*Session)

// WithParentSessionID sets the parent session for the new session.
func WithParentSessionID(id uuid.UUID) CreateOption {
	return func(s *Session) {
		s.ParentSessionID = &id
	}
}

type SessionManager interface {
	CreateSession(chatCtx iface.ChatContextInterface, title, defaultAgentID string, opts ...CreateOption) (*Session, error)
	GetSession(chatCtx iface.ChatContextInterface) (*Session, error)
	ListSessions(chatCtx iface.ChatContextInterface, limit, offset int) ([]*Session, int64, error)
	DeleteSession(chatCtx iface.ChatContextInterface) error
	UpdateSessionTitle(chatCtx iface.ChatContextInterface, title string) error
	UpdateSessionMetadata(chatCtx iface.ChatContextInterface, metadata map[string]any) error
	AddAsyncCompletionPending(chatCtx iface.ChatContextInterface, event map[string]any) error
	GetSessionMessageCount(chatCtx iface.ChatContextInterface) (int64, error)
}

type sessionManager struct {
	db            *gorm.DB
	asyncRegistry tool.AsyncToolTracker
}

func NewSessionManager(db *gorm.DB, asyncRegistry tool.AsyncToolTracker) SessionManager {
	return &sessionManager{db: db, asyncRegistry: asyncRegistry}
}

// SessionManager interface methods (ChatContext-based API)

func (m *sessionManager) CreateSession(chatCtx iface.ChatContextInterface, title, defaultAgentID string, opts ...CreateOption) (*Session, error) {
	session := &Session{
		ID:             uuid.New(),
		Title:          title,
		DefaultAgentID: defaultAgentID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       make(map[string]any),
	}

	for _, opt := range opts {
		opt(session)
	}

	if err := m.db.WithContext(chatCtx.Context()).Create(session).Error; err != nil {
		return nil, err
	}

	return session, nil
}

func (m *sessionManager) GetSession(chatCtx iface.ChatContextInterface) (*Session, error) {
	var session Session
	if err := m.db.WithContext(chatCtx.Context()).Where("id = ?", chatCtx.SessionID()).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

func (m *sessionManager) ListSessions(chatCtx iface.ChatContextInterface, limit, offset int) ([]*Session, int64, error) {
	var sessions []*Session
	var total int64

	if err := m.db.WithContext(chatCtx.Context()).Model(&Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := m.db.WithContext(chatCtx.Context()).
		Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&sessions).Error; err != nil {
		return nil, 0, err
	}

	return sessions, total, nil
}

func (m *sessionManager) DeleteSession(chatCtx iface.ChatContextInterface) error {
	if m.asyncRegistry != nil {
		m.asyncRegistry.CancelSession(chatCtx.SessionID())
	}

	result := m.db.WithContext(chatCtx.Context()).Delete(&Session{}, "id = ?", chatCtx.SessionID())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (m *sessionManager) UpdateSessionTitle(chatCtx iface.ChatContextInterface, title string) error {
	result := m.db.WithContext(chatCtx.Context()).
		Model(&Session{}).
		Where("id = ?", chatCtx.SessionID()).
		Update("title", title)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (m *sessionManager) UpdateSessionMetadata(chatCtx iface.ChatContextInterface, metadata map[string]any) error {
	result := m.db.WithContext(chatCtx.Context()).
		Model(&Session{}).
		Where("id = ?", chatCtx.SessionID()).
		Update("metadata", metadata)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (m *sessionManager) AddAsyncCompletionPending(chatCtx iface.ChatContextInterface, event map[string]any) error {
	sess, err := m.GetSession(chatCtx)
	if err != nil {
		return err
	}

	if sess.Metadata == nil {
		sess.Metadata = make(map[string]any)
	}

	var pending []map[string]any
	if val, ok := sess.Metadata["async_completion_pending"].([]map[string]any); ok {
		pending = val
	} else {
		pending = []map[string]any{}
	}

	pending = append(pending, event)
	sess.Metadata["async_completion_pending"] = pending

	return m.UpdateSessionMetadata(chatCtx, sess.Metadata)
}

func (m *sessionManager) GetSessionMessageCount(chatCtx iface.ChatContextInterface) (int64, error) {
	var count int64
	if err := m.db.WithContext(chatCtx.Context()).
		Model(&Message{}).
		Where("session_id = ?", chatCtx.SessionID()).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// storage.SessionStore interface methods (context.Context-based API)

func (m *sessionManager) Create(ctx context.Context, session *storage.Session) (*storage.Session, error) {
	model := SessionFromStorage(session)
	if model.ID == uuid.Nil {
		model.ID = uuid.New()
	}
	if model.Metadata == nil {
		model.Metadata = make(JSONB)
	}
	if err := m.db.WithContext(ctx).Create(model).Error; err != nil {
		return nil, err
	}
	return SessionToStorage(model), nil
}

func (m *sessionManager) Get(ctx context.Context, id uuid.UUID) (*storage.Session, error) {
	var s Session
	if err := m.db.WithContext(ctx).Where("id = ?", id).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return SessionToStorage(&s), nil
}

func (m *sessionManager) List(ctx context.Context, limit, offset int) ([]*storage.Session, int64, error) {
	var sessions []*Session
	var total int64

	if err := m.db.WithContext(ctx).Model(&Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := m.db.WithContext(ctx).
		Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&sessions).Error; err != nil {
		return nil, 0, err
	}

	result := make([]*storage.Session, len(sessions))
	for i, s := range sessions {
		result[i] = SessionToStorage(s)
	}
	return result, total, nil
}

func (m *sessionManager) Delete(ctx context.Context, id uuid.UUID) error {
	if m.asyncRegistry != nil {
		m.asyncRegistry.CancelSession(id.String())
	}

	result := m.db.WithContext(ctx).Delete(&Session{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (m *sessionManager) UpdateTitle(ctx context.Context, id uuid.UUID, title string) error {
	result := m.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", id).
		Update("title", title)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (m *sessionManager) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]any) error {
	result := m.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", id).
		Update("metadata", metadata)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (m *sessionManager) GetMessageCount(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	var count int64
	if err := m.db.WithContext(ctx).
		Model(&Message{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
