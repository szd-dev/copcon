package iface

import (
	"context"
	"testing"
	"time"

	"github.com/copcon/server/internal/domain/entity"
)

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
		ctx := context.Background()
		c := NewChatContext(ctx, "test-session", "test-agent")

		if c.Context() != ctx {
			t.Fatal("Context() mismatch")
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
