package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/chatcontext"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/testutil"
)

// compositionHook records execution order.
type compositionHook struct {
	name     string
	priority int
	points   []hook.HookPoint
}

func (h *compositionHook) Name() string                        { return h.name }
func (h *compositionHook) Priority() int                       { return h.priority }
func (h *compositionHook) Points() []hook.HookPoint            { return h.points }
func (h *compositionHook) Execute(ctx *hook.HookContext) error { return nil }

// executionRecorder records the order of hook executions.
type executionRecorder struct {
	mu      sync.Mutex
	names   []string
	trigger chan struct{} // closed when a hook fires
}

func newExecutionRecorder() *executionRecorder {
	return &executionRecorder{
		trigger: make(chan struct{}),
	}
}

func (r *executionRecorder) Record(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
	if len(r.names) == 1 {
		close(r.trigger)
	}
}

func (r *executionRecorder) GetOrder() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cpy := make([]string, len(r.names))
	copy(cpy, r.names)
	return cpy
}

// recordingCompositionHook records execution in the recorder.
type recordingCompositionHook struct {
	name     string
	priority int
	points   []hook.HookPoint
	recorder *executionRecorder
}

func (h *recordingCompositionHook) Name() string             { return h.name }
func (h *recordingCompositionHook) Priority() int            { return h.priority }
func (h *recordingCompositionHook) Points() []hook.HookPoint { return h.points }
func (h *recordingCompositionHook) Execute(ctx *hook.HookContext) error {
	h.recorder.Record(h.name)
	return nil
}

// TestHookComposition_Order verifies that composeHooks sorts hooks correctly:
// agent hooks (priority 0-49) before global hooks (priority 50-99).
func TestHookComposition_Order(t *testing.T) {
	agentHooks := []hook.Hook{
		&compositionHook{name: "agent-high", priority: 40, points: []hook.HookPoint{hook.BeforeContextBuild}},
		&compositionHook{name: "agent-low", priority: 10, points: []hook.HookPoint{hook.BeforeContextBuild}},
	}
	globalHooks := []hook.Hook{
		&compositionHook{name: "global-low", priority: 60, points: []hook.HookPoint{hook.BeforeContextBuild}},
		&compositionHook{name: "global-high", priority: 90, points: []hook.HookPoint{hook.BeforeContextBuild}},
	}

	composed := composeHooks(globalHooks, agentHooks)

	require.Len(t, composed, 4, "should have 4 hooks total")

	// Verify order: ascending priority — agent hooks (10, 40) before global (60, 90)
	assert.Equal(t, "agent-low", composed[0].Name(), "lowest priority agent hook first")
	assert.Equal(t, "agent-high", composed[1].Name(), "higher priority agent hook second")
	assert.Equal(t, "global-low", composed[2].Name(), "lowest priority global hook third")
	assert.Equal(t, "global-high", composed[3].Name(), "highest priority global hook last")
}

// TestHookComposition_AgentOnly verifies composeHooks works with only agent hooks.
func TestHookComposition_AgentOnly(t *testing.T) {
	agentHooks := []hook.Hook{
		&compositionHook{name: "agent-a", priority: 10, points: []hook.HookPoint{hook.BeforeContextBuild}},
		&compositionHook{name: "agent-b", priority: 20, points: []hook.HookPoint{hook.BeforeContextBuild}},
	}

	composed := composeHooks(nil, agentHooks)
	require.Len(t, composed, 2)
	assert.Equal(t, "agent-a", composed[0].Name())
	assert.Equal(t, "agent-b", composed[1].Name())
}

// TestHookComposition_GlobalOnly verifies composeHooks works with only global hooks.
func TestHookComposition_GlobalOnly(t *testing.T) {
	globalHooks := []hook.Hook{
		&compositionHook{name: "global-a", priority: 60, points: []hook.HookPoint{hook.BeforeContextBuild}},
		&compositionHook{name: "global-b", priority: 90, points: []hook.HookPoint{hook.BeforeContextBuild}},
	}

	composed := composeHooks(globalHooks, nil)
	require.Len(t, composed, 2)
	assert.Equal(t, "global-a", composed[0].Name())
	assert.Equal(t, "global-b", composed[1].Name())
}

// TestHookComposition_Empty verifies composeHooks with no hooks returns empty slice.
func TestHookComposition_Empty(t *testing.T) {
	composed := composeHooks(nil, nil)
	require.Empty(t, composed)
}

// TestHookComposition_Engine verifies that engine.Chat() with an agent that has
// hooks executes agent hooks before global hooks when using runComposedHooks.
func TestHookComposition_Engine(t *testing.T) {
	recorder := newExecutionRecorder()

	// Agent hooks: priority 0-49
	agentHook1 := &recordingCompositionHook{
		name:     "agent-1",
		priority: 10,
		points:   []hook.HookPoint{hook.BeforeContextBuild},
		recorder: recorder,
	}
	agentHook2 := &recordingCompositionHook{
		name:     "agent-2",
		priority: 20,
		points:   []hook.HookPoint{hook.BeforeContextBuild},
		recorder: recorder,
	}

	// Global hooks: priority 50-99
	globalHook1 := &recordingCompositionHook{
		name:     "global-1",
		priority: 60,
		points:   []hook.HookPoint{hook.BeforeContextBuild},
		recorder: recorder,
	}
	globalHook2 := &recordingCompositionHook{
		name:     "global-2",
		priority: 90,
		points:   []hook.HookPoint{hook.BeforeContextBuild},
		recorder: recorder,
	}

	// Set up mock engine components
	agentRegistry := newMockAgentRegistry()
	agent := AgentDefinition{
		ID:           "composition-test",
		Name:         "Composition Test Agent",
		Model:        "gpt-4o",
		SystemPrompt: "You are a test agent.",
		ToolManager:  &mockToolManagerForEngine{},
		LLMProvider:  llm.NewMockProvider(),
		Hooks:        []hook.Hook{agentHook1, agentHook2},
	}
	agentRegistry.Register("composition-test", agent)
	agentRegistry.SetDefault("composition-test")

	sessionMgr := newMockSessionManager()
	ctx := context.Background()
	chatCtxCreate := chatcontext.NewChatContext(ctx, "", "composition-test")
	sess, err := sessionMgr.CreateSession(chatCtxCreate, "Test Session", "composition-test")
	require.NoError(t, err)

	contextMgr := newMockContextManager()

	// Create engine with global hooks
	engine := NewTestEngine(
		WithTestRegistry(agentRegistry),
		WithTestSessionMgr(sessionMgr),
		WithTestContextMgr(contextMgr),
		WithGlobalHooks(globalHook1, globalHook2),
	)

	chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "composition-test")

	// Run Chat — this will trigger hook composition in runAgentLoop
	err = engine.Chat(chatCtx, "Hello")
	require.NoError(t, err)

	// Drain events
	go func() {
		for range chatCtx.Events() {
		}
	}()
	closeMockChatContext(chatCtx)

	// Verify execution order: agent hooks before global hooks
	order := recorder.GetOrder()
	t.Logf("Hook execution order: %v", order)
	require.Len(t, order, 4, "all 4 hooks should have executed")

	assert.Equal(t, "agent-1", order[0], "agent-1 (pri=10) should execute first")
	assert.Equal(t, "agent-2", order[1], "agent-2 (pri=20) should execute second")
	assert.Equal(t, "global-1", order[2], "global-1 (pri=60) should execute third")
	assert.Equal(t, "global-2", order[3], "global-2 (pri=90) should execute last")
}

// TestHookComposition_BackwardCompat verifies that an agent WITHOUT hooks
// still works correctly (falls back to global hook runner).
func TestHookComposition_BackwardCompat(t *testing.T) {
	agentRegistry := newMockAgentRegistry()
	agent := AgentDefinition{
		ID:           "no-hooks-agent",
		Name:         "No Hooks Agent",
		Model:        "gpt-4o",
		SystemPrompt: "You are a test agent.",
		ToolManager:  &mockToolManagerForEngine{},
		LLMProvider:  llm.NewMockProvider(),
		// Hooks intentionally nil/zero-value
	}
	agentRegistry.Register("no-hooks-agent", agent)
	agentRegistry.SetDefault("no-hooks-agent")

	sessionMgr := newMockSessionManager()
	ctx := context.Background()
	chatCtxCreate := chatcontext.NewChatContext(ctx, "", "no-hooks-agent")
	sess, err := sessionMgr.CreateSession(chatCtxCreate, "Test Session", "no-hooks-agent")
	require.NoError(t, err)

	engine := NewTestEngine(
		WithTestRegistry(agentRegistry),
		WithTestSessionMgr(sessionMgr),
	)

	chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "no-hooks-agent")
	err = engine.Chat(chatCtx, "Hello")
	// Should succeed without panic or error — backward compat
	assert.NoError(t, err)

	go func() {
		for range chatCtx.Events() {
		}
	}()
	closeMockChatContext(chatCtx)
}
