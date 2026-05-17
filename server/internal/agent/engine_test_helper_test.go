package agent

import (
	"golang.org/x/sync/semaphore"

	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
)

// TestEngineOption configures an AgentEngine created by NewTestEngine.
type TestEngineOption func(*AgentEngine)

// WithTestRegistry sets a custom agent registry on the test engine.
func WithTestRegistry(reg AgentRegistry) TestEngineOption {
	return func(e *AgentEngine) {
		e.agentRegistry = reg
	}
}

// WithTestSessionMgr sets a custom session manager on the test engine.
func WithTestSessionMgr(mgr session.SessionManager) TestEngineOption {
	return func(e *AgentEngine) {
		e.sessionMgr = mgr
	}
}

// WithTestContextMgr sets a custom context manager on the test engine.
func WithTestContextMgr(mgr chat_context.ContextManager) TestEngineOption {
	return func(e *AgentEngine) {
		e.contextMgr = mgr
	}
}

// WithTestAsyncRegistry sets a custom async registry on the test engine.
func WithTestAsyncRegistry(reg *tool.AsyncToolRegistry) TestEngineOption {
	return func(e *AgentEngine) {
		e.asyncRegistry = reg
	}
}

// NewTestEngine creates a minimal AgentEngine for testing with sensible defaults.
// All fields are populated with mock implementations. Options can override defaults.
func NewTestEngine(opts ...TestEngineOption) *AgentEngine {
	e := &AgentEngine{
		agentRegistry:  newMockAgentRegistry(),
		sessionMgr:     newMockSessionManager(),
		contextMgr:     newMockContextManager(),
		concurrencySem: semaphore.NewWeighted(5),
		asyncRegistry:  tool.NewAsyncToolRegistry(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// NewTestEngineWithRegistry creates a test AgentEngine with a specific async registry.
func NewTestEngineWithRegistry(asyncRegistry *tool.AsyncToolRegistry, opts ...TestEngineOption) *AgentEngine {
	e := NewTestEngine(opts...)
	e.asyncRegistry = asyncRegistry
	return e
}
