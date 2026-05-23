package postgres

import (
	"errors"

	"gorm.io/gorm"

	"github.com/copcon/core/storage"
)

type Store struct {
	SessionStore *SessionStore
	MessageStore *MessageStore
	TodoStore    *TodoStore
}

func NewStore(db *gorm.DB) *Store {
	return &Store{
		SessionStore: &SessionStore{db: db},
		MessageStore: &MessageStore{db: db},
		TodoStore:    &TodoStore{db: db},
	}
}

func (s *Store) AutoMigrate() error {
	return AutoMigrate(s.SessionStore.db)
}

var (
	_ storage.SessionStore = (*SessionStore)(nil)
	_ storage.MessageStore = (*MessageStore)(nil)
	_ storage.TodoStore    = (*TodoStore)(nil)
)

func IsNotFound(err error) bool {
	return errors.Is(err, ErrSessionNotFound) ||
		errors.Is(err, ErrMessageNotFound) ||
		errors.Is(err, ErrTodoNotFound)
}
