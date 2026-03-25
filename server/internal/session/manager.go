package session

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrSessionNotFound = errors.New("session not found")
)

type SessionManager interface {
	Create(ctx context.Context, title string) (*Session, error)
	Get(ctx context.Context, id string) (*Session, error)
	List(ctx context.Context, limit, offset int) ([]*Session, int64, error)
	Delete(ctx context.Context, id string) error
	UpdateTitle(ctx context.Context, id, title string) error
	GetMessageCount(ctx context.Context, sessionID string) (int64, error)
	GetDB() *gorm.DB
}

type sessionManager struct {
	db *gorm.DB
}

func NewSessionManager(db *gorm.DB) SessionManager {
	return &sessionManager{db: db}
}

func (m *sessionManager) Create(ctx context.Context, title string) (*Session, error) {
	session := &Session{
		ID:        uuid.New(),
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]any),
	}

	if err := m.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, err
	}

	return session, nil
}

func (m *sessionManager) Get(ctx context.Context, id string) (*Session, error) {
	var session Session
	if err := m.db.WithContext(ctx).Where("id = ?", id).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

func (m *sessionManager) List(ctx context.Context, limit, offset int) ([]*Session, int64, error) {
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

	return sessions, total, nil
}

func (m *sessionManager) Delete(ctx context.Context, id string) error {
	result := m.db.WithContext(ctx).Delete(&Session{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (m *sessionManager) UpdateTitle(ctx context.Context, id, title string) error {
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

func (m *sessionManager) GetMessageCount(ctx context.Context, sessionID string) (int64, error) {
	var count int64
	if err := m.db.WithContext(ctx).
		Model(&Message{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (m *sessionManager) GetDB() *gorm.DB {
	return m.db
}
