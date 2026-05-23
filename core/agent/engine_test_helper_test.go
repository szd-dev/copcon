package agent

import (
	"log/slog"
	"os"

	"golang.org/x/sync/semaphore"

	"github.com/copcon/core/context_builder"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

func WithTestRegistry(reg AgentRegistry) EngineOption {
	return func(e *engineImpl) {
		e.agentRegistry = reg
	}
}

func WithTestSessionStore(store storage.SessionStore) EngineOption {
	return func(e *engineImpl) {
		e.sessionStore = store
	}
}

func WithTestMessageStore(store storage.MessageStore) EngineOption {
	return func(e *engineImpl) {
		e.messageStore = store
	}
}

func WithTestAsyncRegistry(reg *tool.AsyncToolRegistry) EngineOption {
	return func(e *engineImpl) {
		e.asyncRegistry = reg
	}
}

func NewTestEngine(opts ...EngineOption) *engineImpl {
	e := &engineImpl{
		agentRegistry:  newMockAgentRegistry(),
		sessionStore:   newMockSessionStore(),
		messageStore:   newMockMessageStore(),
		ctxBuilder:     context_builder.New(),
		hookRunner:     hook.NewEmptyRunner(),
		concurrency:    5,
		asyncRegistry:  tool.NewAsyncToolRegistry(),
		logger:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	for _, opt := range opts {
		opt(e)
	}
	e.concurrencySem = semaphore.NewWeighted(int64(e.concurrency))
	return e
}

func NewTestEngineWithRegistry(asyncRegistry *tool.AsyncToolRegistry, opts ...EngineOption) *engineImpl {
	e := NewTestEngine(opts...)
	e.asyncRegistry = asyncRegistry
	return e
}
