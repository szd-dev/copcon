package hook

import (
	"log/slog"
	"sort"
	"sync"
	"time"
)

// HookRunner provides a concurrency-safe, ordered executor for registered
// hooks. Hooks are sorted by priority (descending) and then by registration
// timestamp (ascending for ties). Individual hook failures or panics are
// contained — they are logged and the chain continues.
type HookRunner interface {
	// Register adds a hook to the runner. Safe for concurrent use.
	Register(hook Hook)

	// Run dispatches all hooks registered for the given HookPoint.
	// Hooks are executed in priority-descending order (higher priority
	// first), with ties broken by registration order (earlier first).
	//
	// If the context in ctx.ChatCtx is already cancelled, Run returns
	// immediately without executing any hooks.
	//
	// Each hook is wrapped in panic recovery and error logging; a
	// failing hook never aborts the chain.
	Run(point HookPoint, ctx *HookContext)
}

// hookEntry associates a registered hook with its registration timestamp
// for deterministic sort ordering when priorities are equal.
type hookEntry struct {
	hook      Hook
	createdAt time.Time
}

// hookRunner is the default HookRunner implementation.
type hookRunner struct {
	mu      sync.Mutex
	entries []hookEntry
}

// NewHookRunner creates a new, empty HookRunner.
func NewHookRunner() HookRunner {
	return &hookRunner{
		entries: make([]hookEntry, 0),
	}
}

// Register adds a hook. Safe for concurrent use.
func (r *hookRunner) Register(hook Hook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, hookEntry{
		hook:      hook,
		createdAt: time.Now(),
	})
}

// Run executes all hooks registered for the given point.
func (r *hookRunner) Run(point HookPoint, ctx *HookContext) {
	// Context cancellation check: if the underlying context is already
	// done (cancelled or timed out), skip the entire chain.
	if err := ctx.ChatCtx.Context().Err(); err != nil {
		return
	}

	// Snapshot registered hooks under lock, then execute outside lock
	// so long-running hooks don't block further registrations.
	r.mu.Lock()
	candidates := make([]hookEntry, len(r.entries))
	copy(candidates, r.entries)
	r.mu.Unlock()

	// Filter to hooks that registered for this point.
	var matched []hookEntry
	for _, e := range candidates {
		for _, p := range e.hook.Points() {
			if p == point {
				matched = append(matched, e)
				break
			}
		}
	}

	if len(matched) == 0 {
		return
	}

	// Sort: priority descending, then registration time ascending.
	sort.Slice(matched, func(i, j int) bool {
		pi, pj := matched[i].hook.Priority(), matched[j].hook.Priority()
		if pi != pj {
			return pi > pj
		}
		return matched[i].createdAt.Before(matched[j].createdAt)
	})

	for _, e := range matched {
		r.executeHook(e.hook, ctx)
	}
}

// executeHook wraps a single hook execution with panic recovery and
// error logging. Neither panics nor errors abort the chain.
func (r *hookRunner) executeHook(h Hook, ctx *HookContext) {
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("hook panicked",
					"hook", h.Name(),
					"panic", rec,
					"point", ctx.CurrentPoint,
				)
			}
		}()
		if err := h.Execute(ctx); err != nil {
			slog.Warn("hook returned error",
				"hook", h.Name(),
				"error", err,
				"point", ctx.CurrentPoint,
			)
		}
	}()
}
