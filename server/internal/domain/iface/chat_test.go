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

	t.Run("Closed_returns_signal_channel", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		ch := c.Closed()
		if ch == nil {
			t.Fatal("Closed() returned nil channel")
		}
		// channel must be closed (stub behavior)
		select {
		case <-ch:
			// expected: already closed
		default:
			t.Fatal("Closed() channel should be closed (stub)")
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

	t.Run("Subscribe_returns_nil_false", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		sub, ok := c.Subscribe(0)
		if sub != nil {
			t.Fatal("expected nil Subscriber (stub)")
		}
		if ok {
			t.Fatal("expected ok=false (stub)")
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

	t.Run("Close_closes_events_channel", func(t *testing.T) {
		c := NewChatContext(context.Background(), "s", "a")
		c.Close()
		_, ok := <-c.Events()
		if ok {
			t.Fatal("expected events channel to be closed after Close()")
		}
	})
}
