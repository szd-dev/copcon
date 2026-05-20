package chat_context

import (
	"fmt"
	"sync"

	"github.com/copcon/server/internal/domain/iface"
)

// SessionAgentStore is a concurrency-safe map from sessionID to active ChatContext.
// It uses sync.RWMutex for explicit read/write lock semantics.
type SessionAgentStore struct {
	mu     sync.RWMutex
	active map[string]*iface.ChatContext
}

// NewSessionAgentStore creates a new SessionAgentStore.
func NewSessionAgentStore() *SessionAgentStore {
	return &SessionAgentStore{
		active: make(map[string]*iface.ChatContext),
	}
}

// Put stores a ChatContext for the given sessionID.
// Returns an error if the sessionID is already active.
func (s *SessionAgentStore) Put(sessionID string, chatCtx *iface.ChatContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.active[sessionID]; exists {
		return fmt.Errorf("session %s already active", sessionID)
	}

	s.active[sessionID] = chatCtx
	return nil
}

// Get retrieves the ChatContext for the given sessionID.
// Returns the ChatContext and true if found, nil and false otherwise.
func (s *SessionAgentStore) Get(sessionID string) (*iface.ChatContext, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chatCtx, ok := s.active[sessionID]
	return chatCtx, ok
}

// Remove deletes the ChatContext for the given sessionID from the store.
func (s *SessionAgentStore) Remove(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.active, sessionID)
}

var _ iface.Storer = (*SessionAgentStore)(nil)
