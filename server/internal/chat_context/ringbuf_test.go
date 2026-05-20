package chat_context

import (
	"context"
	"testing"
	"time"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
)

func TestRingbufChatContext(t *testing.T) {
	t.Run("Emit_and_Events_fan_out", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		events := c.Events()

		const n = 50
		go func() {
			for i := range n {
				c.Emit(entity.Event{Type: entity.EventType("event")})
				_ = i
			}
			c.Close()
		}()

		received := 0
		for range events {
			received++
		}
		if received != n {
			t.Fatalf("expected %d events, got %d", n, received)
		}
	})

	t.Run("Close_then_Events_channel_closes", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		events := c.Events()
		c.Close()

		_, ok := <-events
		if ok {
			t.Fatal("expected Events channel to close after Close()")
		}
	})

	t.Run("Close_signals_Closed_channel", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		c.Close()

		select {
		case <-c.Closed():
		default:
			t.Fatal("Closed() should fire after Close()")
		}
	})

	t.Run("Closed_not_signaled_before_Close", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		select {
		case <-c.Closed():
			t.Fatal("Closed() should not fire before Close()")
		default:
		}
	})

	t.Run("Events_drains_remaining_after_Close", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		events := c.Events()

		c.Emit(entity.Event{Type: entity.EventMessage})
		c.Emit(entity.Event{Type: entity.EventDone})
		c.Close()

		received := 0
		for range events {
			received++
		}
		if received != 2 {
			t.Fatalf("expected 2 events drained after Close, got %d", received)
		}
	})

	t.Run("Subscribe_replays_from_position", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		for range 10 {
			c.Emit(entity.Event{Type: entity.EventType("pre")})
		}

		sub, ok := c.Subscribe(5)
		if !ok {
			t.Fatal("Subscribe(5) should succeed")
		}

		c.Close()

		received := 0
		for range sub.Events {
			received++
		}
		if received != 5 {
			t.Fatalf("expected 5 replayed events, got %d", received)
		}
	})

	t.Run("Subscribe_evicted_seq_returns_false", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		for range 2000 {
			c.Emit(entity.Event{Type: entity.EventType("fill")})
		}

		sub, ok := c.Subscribe(500)
		if sub != nil || ok {
			t.Fatal("Subscribe(500) should fail after eviction")
		}
	})

	t.Run("Depth_propagates", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")
		c.WithDepth(42)
		if c.Depth() != 42 {
			t.Fatalf("expected Depth()=42, got %d", c.Depth())
		}
	})

	t.Run("Multiple_Events_calls_create_independent_streams", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		ch1 := c.Events()
		ch2 := c.Events()

		go func() {
			for range 5 {
				c.Emit(entity.Event{Type: entity.EventMessage})
			}
			c.Close()
		}()

		received1 := 0
		received2 := 0

		done := make(chan struct{})
		go func() {
			for range ch1 {
				received1++
			}
			done <- struct{}{}
		}()
		go func() {
			for range ch2 {
				received2++
			}
			done <- struct{}{}
		}()

		<-done
		<-done

		if received1 != 5 || received2 != 5 {
			t.Fatalf("both streams should get 5 events: got %d and %d", received1, received2)
		}
	})

	t.Run("Subscribe_negative_seq_fails", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")
		sub, ok := c.Subscribe(-1)
		if sub != nil || ok {
			t.Fatal("Subscribe(-1) should fail")
		}
	})

	t.Run("subscribe_receives_future_events", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		c.Emit(entity.Event{Type: entity.EventMessage})
		c.Emit(entity.Event{Type: entity.EventMessage})

		sub, ok := c.Subscribe(1)
		if !ok {
			t.Fatal("Subscribe(1) should succeed")
		}

		c.Emit(entity.Event{Type: entity.EventMessage})
		c.Emit(entity.Event{Type: entity.EventMessage})
		c.Close()

		received := 0
		for range sub.Events {
			received++
		}
		if received != 3 {
			t.Fatalf("expected 3 events (1 replay + 2 future), got %d", received)
		}
	})

	t.Run("ringbuf_subscriber_gets_io_EOF", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")
		c.Close()

		events := c.Events()
		for range events {
			t.Log("draining")
		}
	})

	t.Run("concurrent_Emit_and_Events_no_deadlock", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		events := c.Events()

		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := range 100 {
				c.Emit(entity.Event{Type: entity.EventType("msg")})
				_ = i
			}
			c.Close()
		}()

		<-done

		count := 0
		for range events {
			count++
		}
		if count != 100 {
			t.Fatalf("expected 100 events, got %d", count)
		}
	})

	t.Run("Subscribe_immediately_after_create_gets_future_events", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		ch := make(chan struct{})
		go func() {
			events := c.Events()
			n := 0
			for range events {
				n++
			}
			if n != 3 {
				t.Errorf("expected 3 events, got %d", n)
			}
			close(ch)
		}()

		time.Sleep(10 * time.Millisecond)

		c.Emit(entity.Event{Type: entity.EventType("a")})
		c.Emit(entity.Event{Type: entity.EventType("b")})
		c.Emit(entity.Event{Type: entity.EventType("c")})
		c.Close()

		<-ch
	})

	t.Run("ringbuf_defaults", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s", "a")

		if c.SessionID() != "s" {
			t.Fatal("sessionID mismatch")
		}
		if c.AgentID() != "a" {
			t.Fatal("agentID mismatch")
		}
		if c.Depth() != 0 {
			t.Fatal("depth should be 0 by default")
		}
	})
}

func TestRingbufSSEHandlerPattern(t *testing.T) {
	c := iface.NewChatContext(context.Background(), "s", "a")

	go func() {
		defer c.Close()
		for range 5 {
			c.Emit(entity.Event{Type: entity.EventType("sse-event")})
		}
	}()

	events := c.Events()
	received := 0
	for range events {
		received++
	}
	if received != 5 {
		t.Fatalf("expected 5 SSE events, got %d", received)
	}
}
