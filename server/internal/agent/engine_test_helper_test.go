package agent

import (
	"log/slog"
	"os"

	"golang.org/x/sync/semaphore"

	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
)

// WithTestRegistry sets a custom agent registry on the test engine.
func WithTestRegistry(reg AgentRegistry) EngineOption {
	return func(e *engineImpl) {
		e.agentRegistry = reg
	}
}

// WithTestSessionMgr sets a custom session manager on the test engine.
func WithTestSessionMgr(mgr session.SessionManager) EngineOption {
	return func(e *engineImpl) {
		e.sessionMgr = mgr
	}
}

// WithTestContextMgr sets a custom context manager on the test engine.
func WithTestContextMgr(mgr chat_context.ContextManager) EngineOption {
	return func(e *engineImpl) {
		e.contextMgr = mgr
	}
}

// WithTestAsyncRegistry sets a custom async registry on the test engine.
func WithTestAsyncRegistry(reg *tool.AsyncToolRegistry) EngineOption {
	return func(e *engineImpl) {
		e.asyncRegistry = reg
	}
}

// NewTestEngine creates a minimal AgentEngine for testing with sensible defaults.
// All fields are populated with mock implementations. Options can override defaults.
func NewTestEngine(opts ...EngineOption) *engineImpl {
	e := &engineImpl{
		agentRegistry: newMockAgentRegistry(),
		sessionMgr:    newMockSessionManager(),
		contextMgr:    newMockContextManager(),
		concurrency:   5,
		asyncRegistry: tool.NewAsyncToolRegistry(),
		logger:        slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	for _, opt := range opts {
		opt(e)
	}
	e.concurrencySem = semaphore.NewWeighted(int64(e.concurrency))
	return e
}

// NewTestEngineWithRegistry creates a test AgentEngine with a specific async registry.
func NewTestEngineWithRegistry(asyncRegistry *tool.AsyncToolRegistry, opts ...EngineOption) *engineImpl {
	e := NewTestEngine(opts...)
	e.asyncRegistry = asyncRegistry
	return e
}
