package chat

import (
	"sync"

	"github.com/copcon/core/chatcontext"
	"github.com/copcon/core/iface"
)

type ActiveSessions interface {
	Get(sessionID string) (*chatcontext.ChatContext, bool)
	Put(sessionID string, chatCtx *chatcontext.ChatContext)
	Remove(sessionID string)
}

type sessionStore struct {
	mu     sync.RWMutex
	active map[string]*chatcontext.ChatContext
}

func NewActiveSessions() ActiveSessions {
	return &sessionStore{active: make(map[string]*chatcontext.ChatContext)}
}

func (s *sessionStore) Put(sessionID string, chatCtx *chatcontext.ChatContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[sessionID] = chatCtx
}

func (s *sessionStore) Get(sessionID string) (*chatcontext.ChatContext, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ctx, ok := s.active[sessionID]
	return ctx, ok
}

func (s *sessionStore) Remove(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, sessionID)
}

var _ iface.Storer = (*sessionStore)(nil)
