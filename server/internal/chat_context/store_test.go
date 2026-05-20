package chat_context

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/server/internal/domain/iface"
)

func TestSessionAgentStore(t *testing.T) {
	t.Run("Put_and_Get", func(t *testing.T) {
		s := NewSessionAgentStore()
		c := iface.NewChatContext(context.Background(), "s1", "a1")
		err := s.Put("s1", c)
		require.NoError(t, err)

		got, ok := s.Get("s1")
		assert.True(t, ok)
		assert.Same(t, c, got)
	})

	t.Run("Get_missing_returns_false", func(t *testing.T) {
		s := NewSessionAgentStore()
		got, ok := s.Get("nonexistent")
		assert.False(t, ok)
		assert.Nil(t, got)
	})

	t.Run("Put_duplicate_returns_error", func(t *testing.T) {
		s := NewSessionAgentStore()
		c1 := iface.NewChatContext(context.Background(), "s1", "a1")
		c2 := iface.NewChatContext(context.Background(), "s1", "a2")

		err := s.Put("s1", c1)
		require.NoError(t, err)

		err = s.Put("s1", c2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already active")
	})

	t.Run("Remove_deletes_from_store", func(t *testing.T) {
		s := NewSessionAgentStore()
		c := iface.NewChatContext(context.Background(), "s1", "a1")

		err := s.Put("s1", c)
		require.NoError(t, err)

		s.Remove("s1")

		_, ok := s.Get("s1")
		assert.False(t, ok)
	})

	t.Run("Remove_idempotent", func(t *testing.T) {
		s := NewSessionAgentStore()
		s.Remove("nonexistent")
		s.Remove("nonexistent")
	})

	t.Run("Close_auto_removes_from_store", func(t *testing.T) {
		s := NewSessionAgentStore()
		c := iface.NewChatContext(context.Background(), "s1", "a1")
		c.SetStore(s)

		err := s.Put("s1", c)
		require.NoError(t, err)

		c.Close()

		_, ok := s.Get("s1")
		assert.False(t, ok, "session should be removed after Close()")
	})

	t.Run("Close_without_store_does_not_panic", func(t *testing.T) {
		c := iface.NewChatContext(context.Background(), "s1", "a1")
		c.Close()
	})

	t.Run("concurrent_Put_Get_Remove", func(t *testing.T) {
		s := NewSessionAgentStore()
		var wg sync.WaitGroup

		// 10 goroutines: Put + Get
		for i := range 10 {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				sessionID := "s-" + string(rune('0'+n))
				c := iface.NewChatContext(context.Background(), sessionID, "a")
				err := s.Put(sessionID, c)
				if err == nil {
					got, ok := s.Get(sessionID)
					if ok {
						assert.Same(t, c, got)
					}
				}
			}(i)
		}
		wg.Wait()

		for i := range 5 {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				sessionID := "s-" + string(rune('0'+n))
				s.Remove(sessionID)
			}(i)
		}
		wg.Wait()
	})
}
