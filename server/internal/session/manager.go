package session

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/tool"
)

var (
	ErrSessionNotFound = errors.New("session not found")
)

type SessionManager interface {
	Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string) (*Session, error)
	Get(chatCtx iface.ChatContextInterface) (*Session, error)
	List(chatCtx iface.ChatContextInterface, limit, offset int) ([]*Session, int64, error)
	Delete(chatCtx iface.ChatContextInterface) error
	UpdateTitle(chatCtx iface.ChatContextInterface, title string) error
	UpdateMetadata(chatCtx iface.ChatContextInterface, metadata map[string]any) error
	AddAsyncCompletionPending(chatCtx iface.ChatContextInterface, event map[string]any) error
	GetMessageCount(chatCtx iface.ChatContextInterface) (int64, error)
	GetDB() *gorm.DB
}

type sessionManager struct {
	db            *gorm.DB
	asyncRegistry *tool.AsyncToolRegistry
}

func NewSessionManager(db *gorm.DB, asyncRegistry *tool.AsyncToolRegistry) SessionManager {
	return &sessionManager{db: db, asyncRegistry: asyncRegistry}
}

func (m *sessionManager) Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string) (*Session, error) {
	session := &Session{
		ID:             uuid.New(),
		Title:          title,
		DefaultAgentID: defaultAgentID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       make(map[string]any),
	}

	if err := m.db.WithContext(chatCtx.Context()).Create(session).Error; err != nil {
		return nil, err
	}

	return session, nil
}

func (m *sessionManager) Get(chatCtx iface.ChatContextInterface) (*Session, error) {
	var session Session
	if err := m.db.WithContext(chatCtx.Context()).Where("id = ?", chatCtx.SessionID()).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

func (m *sessionManager) List(chatCtx iface.ChatContextInterface, limit, offset int) ([]*Session, int64, error) {
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

func (m *sessionManager) Delete(chatCtx iface.ChatContextInterface) error {
	// Cancel all async tools associated with this session before deleting
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

func (m *sessionManager) UpdateTitle(chatCtx iface.ChatContextInterface, title string) error {
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

func (m *sessionManager) UpdateMetadata(chatCtx iface.ChatContextInterface, metadata map[string]any) error {
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
	sess, err := m.Get(chatCtx)
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

	return m.UpdateMetadata(chatCtx, sess.Metadata)
}

func (m *sessionManager) GetMessageCount(chatCtx iface.ChatContextInterface) (int64, error) {
	var count int64
	if err := m.db.WithContext(chatCtx.Context()).
		Model(&Message{}).
		Where("session_id = ?", chatCtx.SessionID()).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (m *sessionManager) GetDB() *gorm.DB {
	return m.db
}
