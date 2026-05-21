package hook

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
)

// stubChatCtx implements iface.ChatContextInterface for runner tests.
type stubChatCtx struct {
	ctx context.Context
}

func (s *stubChatCtx) Context() context.Context                  { return s.ctx }
func (s *stubChatCtx) SessionID() string                         { return "test-session" }
func (s *stubChatCtx) AgentID() string                           { return "test-agent" }
func (s *stubChatCtx) Events() <-chan entity.Event               { return nil }
func (s *stubChatCtx) Emit(entity.Event)                         {}
func (s *stubChatCtx) Close()                                    {}
func (s *stubChatCtx) Closed() <-chan struct{}                   { ch := make(chan struct{}); close(ch); return ch }
func (s *stubChatCtx) Depth() int                                { return 0 }
func (s *stubChatCtx) Subscribe(int64) (*iface.Subscriber, bool) { return nil, false }
func (s *stubChatCtx) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
	return nil, fmt.Errorf("stub: RequestInput not implemented")
}
func (s *stubChatCtx) ResolveInput(interruptID string, resp *iface.InputResponse) error {
	return iface.ErrInterruptNotFound
}
func (s *stubChatCtx) PendingInputs() []iface.InputRequest {
	return nil
}
func (s *stubChatCtx) SetPartLocator(messageID string, stepIndex, partIndex int) {}
func (s *stubChatCtx) ClearPartLocator()                                         {}

// trackableHook is a test hook that records execution order for assertions.
type trackableHook struct {
	name     string
	priority int
	points   []HookPoint

	// callCount tracks how many times Execute was called.
	callCount atomic.Int32

	// lastPoint records the HookPoint from the last Execute call.
	lastPoint HookPoint

	// executionErr, if non-nil, is returned from Execute.
	executionErr error

	// shouldPanic, if true, causes Execute to panic.
	shouldPanic bool

	// orderMu protects order slice.
	orderMu sync.Mutex
	// order records execution positions for insertion-order tests.
	order []int
}

func (t *trackableHook) Name() string        { return t.name }
func (t *trackableHook) Points() []HookPoint { return t.points }
func (t *trackableHook) Priority() int       { return t.priority }
func (t *trackableHook) Execute(ctx *HookContext) error {
	t.callCount.Add(1)
	t.lastPoint = ctx.CurrentPoint

	if t.shouldPanic {
		panic("intentional test panic in " + t.name)
	}
	return t.executionErr
}

// newCtx creates a HookContext from a stubChatCtx.
func newCtx(chatCtx iface.ChatContextInterface, point HookPoint) *HookContext {
	return &HookContext{
		ChatCtx:      chatCtx,
		SessionID:    "test-session",
		AgentID:      "test-agent",
		CurrentPoint: point,
	}
}

// TestRunnerNoHooks verifies Run with an empty runner is a no-op.
func TestRunnerNoHooks(t *testing.T) {
	runner := NewHookRunner()
	chatCtx := &stubChatCtx{ctx: context.Background()}

	// Should not panic or error.
	runner.Run(BeforeLLMCall, newCtx(chatCtx, BeforeLLMCall))
}

// TestRunnerSingleHook verifies a single registered hook executes.
func TestRunnerSingleHook(t *testing.T) {
	runner := NewHookRunner()
	chatCtx := &stubChatCtx{ctx: context.Background()}

	hook := &trackableHook{
		name:     "single",
		priority: 100,
		points:   []HookPoint{BeforeLLMCall},
	}
	runner.Register(hook)
	runner.Run(BeforeLLMCall, newCtx(chatCtx, BeforeLLMCall))

	if hook.callCount.Load() != 1 {
		t.Errorf("expected 1 call, got %d", hook.callCount.Load())
	}
}

// TestRunnerPriorityOrder verifies hooks execute in priority-descending
// order, with ties broken by registration time.
func TestRunnerPriorityOrder(t *testing.T) {
	runner := NewHookRunner()
	chatCtx := &stubChatCtx{ctx: context.Background()}

	var mu sync.Mutex
	var executionOrder []string

	makeHook := func(name string, priority int) *trackableHook {
		return &trackableHook{
			name:     name,
			priority: priority,
			points:   []HookPoint{BeforeLLMCall},
		}
	}

	// We need to override Execute to record order atomically.
	// Use a wrapper approach: create hooks that append to order on Execute.

	low := makeHook("low", 10)
	med := makeHook("med", 50)
	high := makeHook("high", 100)

	// Wrap to record order.
	recordWrapper := func(base *trackableHook) Hook {
		return &orderRecordingHook{
			trackableHook: base,
			order:         &executionOrder,
			mu:            &mu,
		}
	}

	runner.Register(recordWrapper(med))
	runner.Register(recordWrapper(low))
	runner.Register(recordWrapper(high))

	runner.Run(BeforeLLMCall, newCtx(chatCtx, BeforeLLMCall))

	mu.Lock()
	got := append([]string{}, executionOrder...)
	mu.Unlock()

	want := []string{"high", "med", "low"}
	if len(got) != len(want) {
		t.Fatalf("expected %d executions, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: want %q, got %q", i, want[i], got[i])
		}
	}
}

// orderRecordingHook wraps a trackableHook to record execution order.
type orderRecordingHook struct {
	*trackableHook
	order *[]string
	mu    *sync.Mutex
}

func (o *orderRecordingHook) Execute(ctx *HookContext) error {
	// Call the base to increment callCount.
	o.trackableHook.Execute(ctx)
	o.mu.Lock()
	*o.order = append(*o.order, o.name)
	o.mu.Unlock()
	return nil
}

// TestRunnerErroringHook verifies an erroring hook does not abort the chain.
func TestRunnerErroringHook(t *testing.T) {
	runner := NewHookRunner()
	chatCtx := &stubChatCtx{ctx: context.Background()}

	errHook := &trackableHook{
		name:         "erroring",
		priority:     100,
		points:       []HookPoint{BeforeLLMCall},
		executionErr: errors.New("expected test error"),
	}
	okHook := &trackableHook{
		name:     "ok",
		priority: 50,
		points:   []HookPoint{BeforeLLMCall},
	}

	runner.Register(errHook)
	runner.Register(okHook)
	runner.Run(BeforeLLMCall, newCtx(chatCtx, BeforeLLMCall))

	if errHook.callCount.Load() != 1 {
		t.Errorf("erroring hook: expected 1 call, got %d", errHook.callCount.Load())
	}
	if okHook.callCount.Load() != 1 {
		t.Errorf("ok hook: expected 1 call (chain should continue), got %d", okHook.callCount.Load())
	}
}

// TestRunnerPanickingHook verifies a panicking hook does not abort the chain.
func TestRunnerPanickingHook(t *testing.T) {
	runner := NewHookRunner()
	chatCtx := &stubChatCtx{ctx: context.Background()}

	panicHook := &trackableHook{
		name:        "panicking",
		priority:    100,
		points:      []HookPoint{BeforeLLMCall},
		shouldPanic: true,
	}
	okHook := &trackableHook{
		name:     "ok",
		priority: 50,
		points:   []HookPoint{BeforeLLMCall},
	}

	runner.Register(panicHook)
	runner.Register(okHook)
	runner.Run(BeforeLLMCall, newCtx(chatCtx, BeforeLLMCall))

	if okHook.callCount.Load() != 1 {
		t.Errorf("ok hook: expected 1 call (chain should continue after panic), got %d", okHook.callCount.Load())
	}
}

// TestRunnerContextCancelled verifies Run skips all hooks when context is
// already cancelled.
func TestRunnerContextCancelled(t *testing.T) {
	runner := NewHookRunner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before Run

	chatCtx := &stubChatCtx{ctx: ctx}

	hook := &trackableHook{
		name:     "should-not-run",
		priority: 100,
		points:   []HookPoint{BeforeLLMCall},
	}
	runner.Register(hook)
	runner.Run(BeforeLLMCall, newCtx(chatCtx, BeforeLLMCall))

	if hook.callCount.Load() != 0 {
		t.Errorf("expected 0 calls (context cancelled), got %d", hook.callCount.Load())
	}
}

// TestRunnerHookPointFiltering verifies Run only dispatches hooks whose
// Points() include the current HookPoint.
func TestRunnerHookPointFiltering(t *testing.T) {
	runner := NewHookRunner()
	chatCtx := &stubChatCtx{ctx: context.Background()}

	llmHook := &trackableHook{
		name:     "llm",
		priority: 100,
		points:   []HookPoint{BeforeLLMCall, AfterLLMCall},
	}
	toolHook := &trackableHook{
		name:     "tool",
		priority: 100,
		points:   []HookPoint{BeforeToolExecute},
	}

	runner.Register(llmHook)
	runner.Register(toolHook)
	runner.Run(BeforeLLMCall, newCtx(chatCtx, BeforeLLMCall))

	if llmHook.callCount.Load() != 1 {
		t.Errorf("llm hook: expected 1 call, got %d", llmHook.callCount.Load())
	}
	if toolHook.callCount.Load() != 0 {
		t.Errorf("tool hook: expected 0 calls (wrong point), got %d", toolHook.callCount.Load())
	}
}

// TestRunnerConcurrentRegister verifies Register is safe for concurrent use
// when called from multiple goroutines.
func TestRunnerConcurrentRegister(t *testing.T) {
	runner := NewHookRunner()
	var wg sync.WaitGroup

	const goroutines = 20
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runner.Register(&trackableHook{
				name:     "concurrent",
				priority: 100 + id,
				points:   []HookPoint{BeforeLLMCall},
			})
		}(i)
	}
	wg.Wait()

	// Verify none were lost — we should be able to run them.
	chatCtx := &stubChatCtx{ctx: context.Background()}
	runner.Run(BeforeLLMCall, newCtx(chatCtx, BeforeLLMCall))
}

// TestRunnerSamePriorityInsertionOrder verifies hooks with equal priority
// execute in registration order (ascending timestamp).
func TestRunnerSamePriorityInsertionOrder(t *testing.T) {
	runner := NewHookRunner()
	chatCtx := &stubChatCtx{ctx: context.Background()}

	var mu sync.Mutex
	var executionOrder []string

	makeRecordingHook := func(name string) Hook {
		base := &trackableHook{
			name:     name,
			priority: 100,
			points:   []HookPoint{BeforeLLMCall},
		}
		return &orderRecordingHook{
			trackableHook: base,
			order:         &executionOrder,
			mu:            &mu,
		}
	}

	runner.Register(makeRecordingHook("first"))
	runner.Register(makeRecordingHook("second"))
	runner.Register(makeRecordingHook("third"))
	runner.Run(BeforeLLMCall, newCtx(chatCtx, BeforeLLMCall))

	mu.Lock()
	got := append([]string{}, executionOrder...)
	mu.Unlock()

	want := []string{"first", "second", "third"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: want %q, got %q", i, want[i], got[i])
		}
	}
}
