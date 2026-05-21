package iface

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestChatContext() *ChatContext {
	return NewChatContext(nil, "test-session", "test-agent")
}

func TestRequestInput_Resolve(t *testing.T) {
	c := newTestChatContext()
	c.SetPartLocator("msg-1", 0, 1)

	req := InputRequest{
		Type:    InterruptApproval,
		Message: "Approve this action?",
	}

	var resp *InputResponse
	var err error
	done := make(chan struct{})

	go func() {
		resp, err = c.RequestInput(req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	pending := c.PendingInputs()
	require.Len(t, pending, 1)
	assert.Equal(t, InterruptApproval, pending[0].Type)
	assert.NotEmpty(t, pending[0].ID)

	resolveErr := c.ResolveInput(pending[0].ID, &InputResponse{
		Action:  "approved",
		Content: map[string]any{"reason": "looks good"},
	})
	require.NoError(t, resolveErr)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RequestInput did not return after ResolveInput")
	}

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "approved", resp.Action)
	assert.Equal(t, "looks good", resp.Content["reason"])

	pending = c.PendingInputs()
	assert.Empty(t, pending)
}

func TestRequestInput_SessionClose(t *testing.T) {
	c := newTestChatContext()
	c.SetPartLocator("msg-2", 0, 0)

	req := InputRequest{
		Type:    InterruptQuestion,
		Message: "What is your name?",
	}

	var err error
	done := make(chan struct{})

	go func() {
		_, err = c.RequestInput(req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	c.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RequestInput did not return after Close")
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session closed while waiting for input")
}

func TestPendingInputs(t *testing.T) {
	c := newTestChatContext()
	c.SetPartLocator("msg-3", 0, 0)

	assert.Empty(t, c.PendingInputs())

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		c.RequestInput(InputRequest{Type: InterruptApproval, Message: "first"})
	}()
	go func() {
		defer wg.Done()
		c.RequestInput(InputRequest{Type: InterruptQuestion, Message: "second"})
	}()

	time.Sleep(100 * time.Millisecond)

	pending := c.PendingInputs()
	require.Len(t, pending, 2)

	types := map[InterruptType]int{}
	for _, p := range pending {
		types[p.Type]++
	}
	assert.Equal(t, 1, types[InterruptApproval])
	assert.Equal(t, 1, types[InterruptQuestion])

	for _, p := range pending {
		err := c.ResolveInput(p.ID, &InputResponse{Action: "ok"})
		require.NoError(t, err)
	}

	wg.Wait()

	assert.Empty(t, c.PendingInputs())
}

func TestResolveInput_NotFound(t *testing.T) {
	c := newTestChatContext()

	err := c.ResolveInput("nonexistent-id", &InputResponse{Action: "approved"})
	assert.ErrorIs(t, err, ErrInterruptNotFound)
}

func TestRequestInput_EmitsPartUpdate(t *testing.T) {
	c := newTestChatContext()
	c.SetPartLocator("msg-emit", 2, 3)

	sub, ok := c.Subscribe(0)
	require.True(t, ok)

	// Give subscriber goroutine time to start iterating.
	time.Sleep(20 * time.Millisecond)

	req := InputRequest{
		Type:    InterruptApproval,
		Message: "Please approve",
		Summary: "Approval needed",
	}

	done := make(chan struct{})
	go func() {
		c.RequestInput(req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	select {
	case evt := <-sub.Events:
		assert.Equal(t, entity.EventPartUpdate, evt.Type)
		data, ok := evt.Data.(entity.PartUpdateData)
		require.True(t, ok)
		assert.Equal(t, "msg-emit", data.MessageID)
		assert.Equal(t, 2, data.StepIndex)
		assert.Equal(t, 3, data.PartIndex)
		assert.Equal(t, "tool-call", data.PartType)
		assert.Equal(t, string(entity.UIPartStateWaitingForInput), data.State)
		require.NotNil(t, data.Interrupt)
		assert.NotEmpty(t, data.Interrupt["interruptId"])
		assert.Equal(t, "approval", data.Interrupt["interruptType"])
		assert.Equal(t, "Please approve", data.Interrupt["message"])
	case <-time.After(2 * time.Second):
		t.Fatal("no event received from subscriber")
	}

	pending := c.PendingInputs()
	require.Len(t, pending, 1)
	c.ResolveInput(pending[0].ID, &InputResponse{Action: "approved"})

	<-done
	c.ClearPartLocator()
}

func TestRequestInput_NoPartLocator(t *testing.T) {
	c := newTestChatContext()

	sub, ok := c.Subscribe(0)
	require.True(t, ok)

	req := InputRequest{Type: InterruptQuestion, Message: "test"}

	done := make(chan struct{})
	go func() {
		c.RequestInput(req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	select {
	case evt := <-sub.Events:
		t.Fatalf("unexpected event emitted: %+v", evt)
	case <-time.After(100 * time.Millisecond):
	}

	pending := c.PendingInputs()
	require.Len(t, pending, 1)
	c.ResolveInput(pending[0].ID, &InputResponse{Action: "answered"})

	<-done
}

func TestConcurrentEmit(t *testing.T) {
	c := newTestChatContext()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			c.Emit(entity.Event{
				Type: entity.EventMessage,
				Data: entity.MessageData{MessageID: "msg", Content: "hello"},
			})
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(goroutines), c.seq.Load())
}

func TestSetPartLocator_ClearPartLocator(t *testing.T) {
	c := newTestChatContext()

	assert.Nil(t, c.partLocator)

	c.SetPartLocator("msg-a", 1, 2)
	require.NotNil(t, c.partLocator)
	assert.Equal(t, "msg-a", c.partLocator.messageID)
	assert.Equal(t, 1, c.partLocator.stepIndex)
	assert.Equal(t, 2, c.partLocator.partIndex)

	c.ClearPartLocator()
	assert.Nil(t, c.partLocator)
}

func TestChatContextInterfaceExtension(t *testing.T) {
	t.Run("Close_method_exists_on_interface", func(t *testing.T) {
		var ci ChatContextInterface = NewChatContext(context.Background(), "s", "a")
		ci.Close() // must compile and not panic
	})

	t.Run("Closed_returns_open_channel_initially", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		ch := c.Closed()
		if ch == nil {
			t.Fatal("Closed() returned nil channel")
		}
		// channel must be open initially (ringbuf-based implementation).
		select {
		case <-ch:
			t.Fatal("Closed() channel should not be closed before Close() is called")
		default:
			// expected: still open
		}
	})

	t.Run("Closed_fires_after_Close", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		c.Close()
		select {
		case <-c.Closed():
			// expected: closed after Close()
		default:
			t.Fatal("Closed() channel should be closed after Close()")
		}
	})

	t.Run("Depth_returns_zero_by_default", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		if d := c.Depth(); d != 0 {
			t.Fatalf("expected Depth()=0, got %d", d)
		}
	})

	t.Run("WithDepth_sets_depth", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		c.WithDepth(7)
		if d := c.Depth(); d != 7 {
			t.Fatalf("expected Depth()=7, got %d", d)
		}
	})

	t.Run("WithDepth_returns_self_for_chaining", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		chained := c.WithDepth(3)
		if chained != c {
			t.Fatal("WithDepth should return the same instance")
		}
	})

	t.Run("Subscribe_0_with_no_events_succeeds", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		sub, ok := c.Subscribe(0)
		if sub == nil {
			t.Fatal("expected non-nil Subscriber for seq 0")
		}
		if !ok {
			t.Fatal("expected ok=true for seq 0")
		}
		c.Emit(entity.Event{Type: entity.EventType("hello")})
		c.Close()
		received := 0
		for range sub.Events {
			received++
		}
		if received != 1 {
			t.Fatalf("expected 1 event, got %d", received)
		}
	})

	t.Run("Subscriber_struct_has_Events_channel", func(t *testing.T) {
		ch := make(chan entity.Event)
		s := &Subscriber{Events: ch}
		if s.Events == nil {
			t.Fatal("Subscriber.Events should not be nil")
		}
	})

	t.Run("existing_methods_still_work", func(t *testing.T) {
		c := NewChatContext(context.Background(), "test-session", "test-agent")

		if c.Context() == nil {
			t.Fatal("Context() should not be nil")
		}
		if c.SessionID() != "test-session" {
			t.Fatal("SessionID() mismatch")
		}
		if c.AgentID() != "test-agent" {
			t.Fatal("AgentID() mismatch")
		}

		done := make(chan bool, 1)
		go func() {
			evt := <-c.Events()
			if evt.Type != entity.EventDone {
				t.Errorf("expected EventDone, got %s", evt.Type)
			}
			done <- true
		}()

		c.Emit(entity.Event{Type: entity.EventDone})

		select {
		case <-done:
			// ok
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	})

	t.Run("Close_stops_events_stream", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")

		// Start reading Events() before closing.
		events := c.Events()

		// Emit one event then close.
		c.Emit(entity.Event{Type: entity.EventMessage})
		c.Close()

		// Should receive the event first, then channel closes.
		received := 0
		for range events {
			received++
		}
		if received != 1 {
			t.Fatalf("expected 1 event before close, got %d", received)
		}
	})

	t.Run("Close_via_interface", func(t *testing.T) {
		var ci ChatContextInterface = NewChatContext(context.Background(), "s", "a")
		ci.Close()
		select {
		case <-ci.Closed():
			// expected
		default:
			t.Fatal("Closed() should fire after Close()")
		}
	})

	t.Run("Subscribe_replays_from_correct_position", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")

		for range 10 {
			c.Emit(entity.Event{Type: entity.EventType("event")})
		}

		// Subscribe from seq 5 should get events 5,6,7,8,9.
		sub, ok := c.Subscribe(5)
		if !ok {
			t.Fatal("Subscribe(5) should succeed")
		}
		if sub == nil {
			t.Fatal("expected non-nil Subscriber")
		}

		c.Close()

		received := 0
		for range sub.Events {
			received++
		}
		if received != 5 {
			t.Fatalf("expected 5 events from Subscribe(5), got %d", received)
		}
	})

	t.Run("Subscribe_future_seq_fails", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		c.Emit(entity.Event{Type: entity.EventDone})
		sub, ok := c.Subscribe(5)
		if sub != nil {
			t.Fatal("expected nil for future seq")
		}
		if ok {
			t.Fatal("expected false for future seq")
		}
	})

	t.Run("Subscribe_negative_seq_fails", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		sub, ok := c.Subscribe(-1)
		if sub != nil {
			t.Fatal("expected nil for negative seq")
		}
		if ok {
			t.Fatal("expected false for negative seq")
		}
	})
}
