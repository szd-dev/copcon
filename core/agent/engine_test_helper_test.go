package agent

import (
	"log/slog"
	"os"

	"golang.org/x/sync/semaphore"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

func WithTestRegistry(reg AgentRegistry) EngineOption {
	return func(e *engineImpl) {
		e.agentRegistry = reg
	}
}

func WithTestSessionMgr(mgr iface.SessionManager) EngineOption {
	return func(e *engineImpl) {
		e.sessionMgr = mgr
	}
}

func WithTestContextMgr(mgr iface.ContextManager) EngineOption {
	return func(e *engineImpl) {
		e.contextMgr = mgr
	}
}

func WithTestAsyncRegistry(reg *tool.AsyncToolRegistry) EngineOption {
	return func(e *engineImpl) {
		e.asyncRegistry = reg
	}
}

func NewTestEngine(opts ...EngineOption) *engineImpl {
	e := &engineImpl{
		agentRegistry: newMockAgentRegistry(),
		sessionMgr:    newMockSessionManager(),
		contextMgr:    newMockContextManager(),
		hookRunner:    hook.NewEmptyRunner(),
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

func NewTestEngineWithRegistry(asyncRegistry *tool.AsyncToolRegistry, opts ...EngineOption) *engineImpl {
	e := NewTestEngine(opts...)
	e.asyncRegistry = asyncRegistry
	return e
}
