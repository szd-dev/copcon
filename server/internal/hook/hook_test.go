package hook

import (
	"testing"
)

// TestHookPointUniqueness verifies that all HookPoint constants have
// distinct string values. Duplicate constants would cause hook
// registration and dispatch bugs.
func TestHookPointUniqueness(t *testing.T) {
	points := []HookPoint{
		BeforeContextBuild,
		AfterContextBuild,
		OnSystemPrompt,
		OnMessagePersist,
		BeforeToolExecute,
		AfterToolExecute,
		OnToolError,
		BeforeLLMCall,
		AfterLLMCall,
		OnSessionResolve,
	}

	seen := make(map[HookPoint]bool, len(points))
	for _, p := range points {
		if seen[p] {
			t.Errorf("duplicate HookPoint value: %q", p)
		}
		seen[p] = true
	}

	if len(seen) != len(points) {
		t.Errorf("expected %d unique HookPoints, got %d", len(points), len(seen))
	}
	if len(seen) != 10 {
		t.Errorf("expected 10 HookPoint constants, got %d", len(seen))
	}
}

// TestHookInterfaceCompliance verifies that a dummy implementation
// satisfies the Hook interface at compile time and that all four
// interface methods are callable.
func TestHookInterfaceCompliance(t *testing.T) {
	var h Hook = &dummyHook{}

	if h.Name() != "dummy" {
		t.Errorf("unexpected name: %s", h.Name())
	}

	gotPoints := h.Points()
	if len(gotPoints) != 2 {
		t.Errorf("expected 2 points, got %d", len(gotPoints))
	}

	if h.Priority() != 100 {
		t.Errorf("expected priority 100, got %d", h.Priority())
	}

	ctx := &HookContext{CurrentPoint: BeforeLLMCall}
	if err := h.Execute(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// dummyHook is a minimal Hook implementation used to verify the
// interface contract in tests.
type dummyHook struct{}

func (d *dummyHook) Name() string                 { return "dummy" }
func (d *dummyHook) Points() []HookPoint          { return []HookPoint{BeforeLLMCall, AfterLLMCall} }
func (d *dummyHook) Priority() int                { return 100 }
func (d *dummyHook) Execute(_ *HookContext) error { return nil }
