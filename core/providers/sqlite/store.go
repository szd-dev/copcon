package sqlite

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
	AutoMigrate(db)
	return &Store{
		SessionStore: &SessionStore{db: db},
		MessageStore: &MessageStore{db: db},
		TodoStore:    &TodoStore{db: db},
	}
}

func (s *Store) AutoMigrate() error {
	return AutoMigrate(s.SessionStore.db)
}

func (s *Store) Sessions() storage.SessionStore   { return s.SessionStore }
func (s *Store) Messages() storage.MessageStore   { return s.MessageStore }
func (s *Store) Todos() storage.TodoStore         { return s.TodoStore }
func (s *Store) Knowledge() storage.KnowledgeStore { return nil }

var (
	_ storage.SessionStore  = (*SessionStore)(nil)
	_ storage.MessageStore  = (*MessageStore)(nil)
	_ storage.TodoStore     = (*TodoStore)(nil)
	_ storage.StoreProvider = (*Store)(nil)
)

func IsNotFound(err error) bool {
	return errors.Is(err, errSessionNotFound) ||
		errors.Is(err, errMessageNotFound) ||
		errors.Is(err, errTodoNotFound)
}
