package tracing

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/tool"
)

// stubSpan records calls to Span methods for inspection in tests.
type stubSpan struct {
	mu         sync.Mutex
	ended      bool
	attributes map[string]string
	err        error
}

func newStubSpan() *stubSpan {
	return &stubSpan{attributes: make(map[string]string)}
}

func (s *stubSpan) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
}

func (s *stubSpan) SetAttribute(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes[key] = value
}

func (s *stubSpan) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *stubSpan) isEnded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended
}

func (s *stubSpan) getAttr(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attributes[key]
}

func (s *stubSpan) getErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// stubTracer is a Tracer that creates stubSpans and records their names.
type stubTracer struct {
	mu    sync.Mutex
	spans []*stubSpan
	names []string
}

func newStubTracer() *stubTracer {
	return &stubTracer{}
}

func (t *stubTracer) StartSpan(name string) Span {
	s := newStubSpan()
	t.mu.Lock()
	t.names = append(t.names, name)
	t.spans = append(t.spans, s)
	t.mu.Unlock()
	return s
}

func (t *stubTracer) lastSpan() *stubSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.spans) == 0 {
		return nil
	}
	return t.spans[len(t.spans)-1]
}

func (t *stubTracer) spanNames() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]string, len(t.names))
	copy(cp, t.names)
	return cp
}

// stubChatContext implements iface.ChatContextInterface for tests.
type stubChatContext struct {
	ctx       context.Context
	sessionID string
	agentID   string
}

func (s *stubChatContext) Context() context.Context                  { return context.Background() }
func (s *stubChatContext) SessionID() string                         { return s.sessionID }
func (s *stubChatContext) AgentID() string                           { return s.agentID }
func (s *stubChatContext) Events() <-chan entity.Event               { return nil }
func (s *stubChatContext) Emit(_ entity.Event)                       {}
func (s *stubChatContext) Close()                                    {}
func (s *stubChatContext) Closed() <-chan struct{}                   { ch := make(chan struct{}); close(ch); return ch }
func (s *stubChatContext) Depth() int                                { return 0 }
func (s *stubChatContext) Subscribe(int64) (*iface.Subscriber, bool) { return nil, false }
func (s *stubChatContext) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
	return nil, fmt.Errorf("stub: RequestInput not implemented")
}
func (s *stubChatContext) ResolveInput(interruptID string, resp *iface.InputResponse) error {
	return iface.ErrInterruptNotFound
}
func (s *stubChatContext) PendingInputs() []iface.InputRequest {
	return nil
}
func (s *stubChatContext) SetPartLocator(messageID string, stepIndex, partIndex int) {}
func (s *stubChatContext) ClearPartLocator()                                         {}

// Test helpers for building HookContext instances.
func newHookContext(point hook.HookPoint, sessionID, agentID string) *hook.HookContext {
	chatCtx := &stubChatContext{sessionID: sessionID, agentID: agentID}
	return &hook.HookContext{
		ChatCtx:      chatCtx,
		SessionID:    sessionID,
		AgentID:      agentID,
		CurrentPoint: point,
	}
}

// ─── Interface compliance ────────────────────────────────────────────────

func TestTracingPlugin_ImplementsHook(t *testing.T) {
	var _ hook.Hook = (*TracingPlugin)(nil)
}

func TestTracingPlugin_Name(t *testing.T) {
	p := NewTracingPlugin(nil)
	assert.Equal(t, "tracing", p.Name())
}

func TestTracingPlugin_Points(t *testing.T) {
	p := NewTracingPlugin(nil)
	points := p.Points()
	assert.Len(t, points, 5)
	assert.Contains(t, points, hook.BeforeLLMCall)
	assert.Contains(t, points, hook.AfterLLMCall)
	assert.Contains(t, points, hook.BeforeToolExecute)
	assert.Contains(t, points, hook.AfterToolExecute)
	assert.Contains(t, points, hook.OnToolError)
}

func TestTracingPlugin_Priority(t *testing.T) {
	p := NewTracingPlugin(nil)
	assert.Equal(t, 200, p.Priority())
}

// ─── Nil tracer no-op ────────────────────────────────────────────────────

func TestTracingPlugin_NilTracerNoOp(t *testing.T) {
	p := NewTracingPlugin(nil)

	// Before and After LLM call should not panic.
	ctx := newHookContext(hook.BeforeLLMCall, "s1", "a1")
	err := p.Execute(ctx)
	require.NoError(t, err)

	ctx = newHookContext(hook.AfterLLMCall, "s1", "a1")
	err = p.Execute(ctx)
	require.NoError(t, err)

	// Before and After tool execute should not panic.
	ctx = newHookContext(hook.BeforeToolExecute, "s1", "a1")
	ctx.ToolName = "read_file"
	err = p.Execute(ctx)
	require.NoError(t, err)

	ctx = newHookContext(hook.AfterToolExecute, "s1", "a1")
	err = p.Execute(ctx)
	require.NoError(t, err)

	// OnToolError should not panic.
	ctx = newHookContext(hook.OnToolError, "s1", "a1")
	err = p.Execute(ctx)
	require.NoError(t, err)
}

// ─── LLM call span lifecycle ─────────────────────────────────────────────

func TestTracingPlugin_LLMSpanLifecycle(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	// BeforeLLMCall → span created.
	ctx := newHookContext(hook.BeforeLLMCall, "s-llm", "a-llm")
	err := p.Execute(ctx)
	require.NoError(t, err)

	span := tr.lastSpan()
	require.NotNil(t, span, "span should have been created")
	assert.False(t, span.isEnded(), "span should not be ended before AfterLLMCall")
	assert.Equal(t, "s-llm", span.getAttr("session_id"))
	assert.Equal(t, "a-llm", span.getAttr("agent_id"))

	// AfterLLMCall → span ended.
	ctx = newHookContext(hook.AfterLLMCall, "s-llm", "a-llm")
	err = p.Execute(ctx)
	require.NoError(t, err)
	assert.True(t, span.isEnded(), "span should be ended after AfterLLMCall")
}

func TestTracingPlugin_LLMSpanName(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	ctx := newHookContext(hook.BeforeLLMCall, "s1", "a1")
	err := p.Execute(ctx)
	require.NoError(t, err)

	names := tr.spanNames()
	assert.Len(t, names, 1)
	assert.Equal(t, "agent.llm_call", names[0])
}

// ─── Tool span lifecycle ─────────────────────────────────────────────────

func TestTracingPlugin_ToolSpanLifecycle(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	// BeforeToolExecute → span created with correct name.
	ctx := newHookContext(hook.BeforeToolExecute, "s-tool", "a-tool")
	ctx.ToolName = "run_shell"
	err := p.Execute(ctx)
	require.NoError(t, err)

	span := tr.lastSpan()
	require.NotNil(t, span, "span should have been created")
	assert.False(t, span.isEnded(), "span should not be ended before AfterToolExecute")
	assert.Equal(t, "s-tool", span.getAttr("session_id"))
	assert.Equal(t, "a-tool", span.getAttr("agent_id"))
	assert.Equal(t, "run_shell", span.getAttr("tool_name"))

	// AfterToolExecute → span ended.
	ctx = newHookContext(hook.AfterToolExecute, "s-tool", "a-tool")
	err = p.Execute(ctx)
	require.NoError(t, err)
	assert.True(t, span.isEnded(), "span should be ended after AfterToolExecute")
}

func TestTracingPlugin_ToolSpanName(t *testing.T) {
	tests := []struct {
		toolName     string
		expectedName string
	}{
		{"run_shell", "agent.tool.run_shell"},
		{"read_file", "agent.tool.read_file"},
		{"todo_create", "agent.tool.todo_create"},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			tr := newStubTracer()
			p := NewTracingPlugin(tr)

			ctx := newHookContext(hook.BeforeToolExecute, "s1", "a1")
			ctx.ToolName = tt.toolName
			err := p.Execute(ctx)
			require.NoError(t, err)

			names := tr.spanNames()
			assert.Len(t, names, 1)
			assert.Equal(t, tt.expectedName, names[0])
		})
	}
}

// ─── Error span ──────────────────────────────────────────────────────────

func TestTracingPlugin_OnToolErrorEndsSpan(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	// Start a tool span.
	ctx := newHookContext(hook.BeforeToolExecute, "s-err", "a-err")
	ctx.ToolName = "failing_tool"
	err := p.Execute(ctx)
	require.NoError(t, err)

	span := tr.lastSpan()
	require.NotNil(t, span)

	// OnToolError → span ended (even without ToolResult error).
	ctx = newHookContext(hook.OnToolError, "s-err", "a-err")
	err = p.Execute(ctx)
	require.NoError(t, err)
	assert.True(t, span.isEnded(), "span should be ended after OnToolError")
}

func TestTracingPlugin_OnToolErrorWithError(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	// Start a tool span.
	ctx := newHookContext(hook.BeforeToolExecute, "s-err2", "a-err2")
	ctx.ToolName = "failing_tool"
	err := p.Execute(ctx)
	require.NoError(t, err)

	span := tr.lastSpan()
	require.NotNil(t, span)

	// OnToolError with a ToolResult containing an error.
	ctx = newHookContext(hook.OnToolError, "s-err2", "a-err2")
	ctx.ToolResult = &tool.ToolResult{
		Success: false,
		Error:   "permission denied",
	}
	err = p.Execute(ctx)
	require.NoError(t, err)

	assert.True(t, span.isEnded(), "span should be ended")
	gotErr := span.getErr()
	require.NotNil(t, gotErr, "span should have error set")
	assert.Contains(t, gotErr.Error(), "permission denied")
}

// ─── Double-start safety ─────────────────────────────────────────────────

func TestTracingPlugin_ToolSpanReplace(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	// Start first tool span.
	ctx1 := newHookContext(hook.BeforeToolExecute, "s1", "a1")
	ctx1.ToolName = "tool_a"
	err := p.Execute(ctx1)
	require.NoError(t, err)

	firstSpan := tr.lastSpan()

	// Start second tool span (replaces first).
	ctx2 := newHookContext(hook.BeforeToolExecute, "s1", "a1")
	ctx2.ToolName = "tool_b"
	err = p.Execute(ctx2)
	require.NoError(t, err)

	// First span was orphaned — it should not have been ended automatically.
	assert.False(t, firstSpan.isEnded(), "replaced span should not be auto-ended")

	// End the second span.
	ctx3 := newHookContext(hook.AfterToolExecute, "s1", "a1")
	err = p.Execute(ctx3)
	require.NoError(t, err)

	secondSpan := tr.lastSpan()
	assert.True(t, secondSpan.isEnded(), "current span should be ended")
}

// ─── Concurrent (empty hooks) no panic ───────────────────────────────────

func TestTracingPlugin_AfterLLMCallWithoutBefore(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	// Calling AfterLLMCall without a prior BeforeLLMCall should be safe.
	ctx := newHookContext(hook.AfterLLMCall, "s1", "a1")
	err := p.Execute(ctx)
	require.NoError(t, err)
	// No span was ever started — nothing to end.
}

func TestTracingPlugin_AfterToolExecuteWithoutBefore(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	ctx := newHookContext(hook.AfterToolExecute, "s1", "a1")
	err := p.Execute(ctx)
	require.NoError(t, err)
}

func TestTracingPlugin_OnToolErrorWithoutBefore(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	ctx := newHookContext(hook.OnToolError, "s1", "a1")
	err := p.Execute(ctx)
	require.NoError(t, err)
}

// ─── Hook without unknown hook points ────────────────────────────────────

func TestTracingPlugin_UnknownHookPoint(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	// A hook point the plugin does not register for should be a no-op.
	ctx := newHookContext(hook.BeforeContextBuild, "s1", "a1")
	err := p.Execute(ctx)
	require.NoError(t, err)

	// No spans should have been created.
	names := tr.spanNames()
	assert.Empty(t, names)
}

// ─── Attribute verification ──────────────────────────────────────────────

func TestTracingPlugin_LLMSpanAttributes(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	ctx := newHookContext(hook.BeforeLLMCall, "sess-42", "agent-7")
	err := p.Execute(ctx)
	require.NoError(t, err)

	span := tr.lastSpan()
	assert.Equal(t, "sess-42", span.getAttr("session_id"))
	assert.Equal(t, "agent-7", span.getAttr("agent_id"))
	// LLM spans should not have tool_name.
	assert.Equal(t, "", span.getAttr("tool_name"))
}

func TestTracingPlugin_ToolSpanAttributes(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	ctx := newHookContext(hook.BeforeToolExecute, "sess-99", "agent-3")
	ctx.ToolName = "write_file"
	err := p.Execute(ctx)
	require.NoError(t, err)

	span := tr.lastSpan()
	assert.Equal(t, "sess-99", span.getAttr("session_id"))
	assert.Equal(t, "agent-3", span.getAttr("agent_id"))
	assert.Equal(t, "write_file", span.getAttr("tool_name"))
}

// ─── Complete LLM + Tool sequence ────────────────────────────────────────

func TestTracingPlugin_FullSequence(t *testing.T) {
	tr := newStubTracer()
	p := NewTracingPlugin(tr)

	// LLM call.
	ctx := newHookContext(hook.BeforeLLMCall, "s-full", "a-full")
	err := p.Execute(ctx)
	require.NoError(t, err)

	ctx = newHookContext(hook.AfterLLMCall, "s-full", "a-full")
	err = p.Execute(ctx)
	require.NoError(t, err)

	// Tool execution.
	ctx = newHookContext(hook.BeforeToolExecute, "s-full", "a-full")
	ctx.ToolName = "read_file"
	err = p.Execute(ctx)
	require.NoError(t, err)

	ctx = newHookContext(hook.AfterToolExecute, "s-full", "a-full")
	err = p.Execute(ctx)
	require.NoError(t, err)

	names := tr.spanNames()
	assert.Equal(t, []string{"agent.llm_call", "agent.tool.read_file"}, names)

	// Verify all spans ended.
	for i, span := range tr.spans {
		assert.True(t, span.isEnded(), fmt.Sprintf("span %d (%s) should be ended", i, names[i]))
	}
}

// Compile-time check that stubTracer implements Tracer.
var _ Tracer = (*stubTracer)(nil)

// Compile-time check that stubSpan implements Span.
var _ Span = (*stubSpan)(nil)

// Compile-time check that stubChatContext implements the internal chat
// context interface by satisfying the minimum method set.
var _ interface {
	SessionID() string
	AgentID() string
} = (*stubChatContext)(nil)
