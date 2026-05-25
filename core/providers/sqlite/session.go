package sqlite

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/core/storage"
)

var errSessionNotFound = errors.New("session not found")

type SessionStore struct {
	db *gorm.DB
}

func (s *SessionStore) Create(ctx context.Context, session *storage.Session) (*storage.Session, error) {
	model := sessionFromStorage(session)
	if model.ID == uuid.Nil {
		model.ID = uuid.New()
	}
	if model.Metadata == nil {
		model.Metadata = make(JSONB)
	}
	if err := s.db.WithContext(ctx).Create(model).Error; err != nil {
		return nil, err
	}
	return sessionToStorage(model), nil
}

func (s *SessionStore) Get(ctx context.Context, id uuid.UUID) (*storage.Session, error) {
	var m Session
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errSessionNotFound
		}
		return nil, err
	}
	return sessionToStorage(&m), nil
}

func (s *SessionStore) List(ctx context.Context, limit, offset int) ([]*storage.Session, int64, error) {
	var sessions []*Session
	var total int64

	if err := s.db.WithContext(ctx).Model(&Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := s.db.WithContext(ctx).
		Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&sessions).Error; err != nil {
		return nil, 0, err
	}

	result := make([]*storage.Session, len(sessions))
	for i, ss := range sessions {
		result[i] = sessionToStorage(ss)
	}
	return result, total, nil
}

func (s *SessionStore) Delete(ctx context.Context, id uuid.UUID) error {
	result := s.db.WithContext(ctx).Delete(&Session{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errSessionNotFound
	}
	return nil
}

func (s *SessionStore) UpdateTitle(ctx context.Context, id uuid.UUID, title string) error {
	result := s.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", id).
		Update("title", title)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errSessionNotFound
	}
	return nil
}

func (s *SessionStore) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]any) error {
	result := s.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", id).
		Update("metadata", JSONB(metadata))

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errSessionNotFound
	}
	return nil
}

func (s *SessionStore) GetMessageCount(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&Message{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *SessionStore) AppendMetadata(ctx context.Context, id uuid.UUID, key string, value any) error {
	var m Session
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errSessionNotFound
		}
		return err
	}

	if m.Metadata == nil {
		m.Metadata = make(JSONB)
	}

	existing, ok := m.Metadata[key]
	if !ok {
		m.Metadata[key] = []any{value}
	} else {
		arr, ok := existing.([]any)
		if !ok {
			arr = []any{existing}
		}
		m.Metadata[key] = append(arr, value)
	}

	result := s.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", id).
		Update("metadata", m.Metadata)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errSessionNotFound
	}
	return nil
}