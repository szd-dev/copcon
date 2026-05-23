package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/copcon/core/storage"
)

var ErrMessageNotFound = errors.New("message not found")

type MessageStore struct {
	db *gorm.DB
}

func (m *MessageStore) List(ctx context.Context, sessionID uuid.UUID, limit int) ([]*storage.Message, error) {
	var messages []Message

	query := m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}

	result := make([]*storage.Message, len(messages))
	for i := range messages {
		result[i] = messageToStorage(&messages[i])
	}
	return result, nil
}

func (m *MessageStore) Add(ctx context.Context, message *storage.Message) error {
	model := messageFromStorage(message)
	if model.ID == uuid.Nil {
		model.ID = uuid.New()
	}
	return m.db.WithContext(ctx).Create(model).Error
}

func (m *MessageStore) Update(ctx context.Context, message *storage.Message) error {
	model := messageFromStorage(message)
	result := m.db.WithContext(ctx).
		Model(&Message{}).
		Where("id = ? AND session_id = ?", model.ID, model.SessionID).
		Updates(map[string]any{
			"content":    model.Content,
			"reasoning":  model.Reasoning,
			"parts":      model.Parts,
			"tool_calls": model.ToolCalls,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrMessageNotFound, model.ID)
	}
	return nil
}

func (m *MessageStore) Upsert(ctx context.Context, message *storage.Message) error {
	model := messageFromStorage(message)
	if model.ID == uuid.Nil {
		model.ID = uuid.New()
	}

	result := m.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content", "reasoning", "parts", "tool_calls"}),
		}).
		Create(model)
	return result.Error
}

func (m *MessageStore) DeleteBySession(ctx context.Context, sessionID uuid.UUID) error {
	return m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&Message{}).Error
}