package session

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/domain/iface"
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
	GetMessageCount(chatCtx iface.ChatContextInterface) (int64, error)
	GetDB() *gorm.DB
}

type sessionManager struct {
	db *gorm.DB
}

func NewSessionManager(db *gorm.DB) SessionManager {
	return &sessionManager{db: db}
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
