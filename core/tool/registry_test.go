package tool

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolRegistry(t *testing.T) {
	registry := NewToolRegistry()
	require.NotNil(t, registry)

	tool1 := &mockTool{
		name:        "test_tool_1",
		description: "First test tool",
		schema:      map[string]any{"type": "object"},
	}
	err := registry.Register(tool1)
	assert.NoError(t, err)

	err = registry.Register(tool1)
	assert.NoError(t, err)

	retrieved, err := registry.Get("test_tool_1")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test_tool_1", retrieved.Name())
	assert.Equal(t, "First test tool", retrieved.Description())

	retrieved, err = registry.Get("nonexistent")
	assert.ErrorIs(t, err, ErrToolNotFound)
	assert.Nil(t, retrieved)

	tool2 := &mockTool{
		name:        "test_tool_2",
		description: "Second test tool",
		schema:      map[string]any{"type": "string"},
	}
	err = registry.Register(tool2)
	assert.NoError(t, err)

	list := registry.List()
	assert.Len(t, list, 2)

	names := make([]string, len(list))
	for i, info := range list {
		names[i] = info.Name
	}
	assert.Contains(t, names, "test_tool_1")
	assert.Contains(t, names, "test_tool_2")
}

func TestToolRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewToolRegistry()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			tool := &mockTool{
				name:        fmt.Sprintf("concurrent_tool_%d", idx),
				description: fmt.Sprintf("Tool %d", idx),
				schema:      map[string]any{"type": "object"},
			}
			_ = registry.Register(tool)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	list := registry.List()
	assert.Len(t, list, 10)
}

// AsyncToolRegistry Tests

func TestAsyncToolRegistry_Register(t *testing.T) {
	registry := NewAsyncToolRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register("session-1", "call-1", "test_tool", cancel)

	state, err := registry.GetStatus("call-1")
	require.NoError(t, err)
	assert.Equal(t, "call-1", state.CallID)
	assert.Equal(t, "test_tool", state.ToolName)
	assert.Equal(t, StatusRunning, state.Status)
	assert.Equal(t, "session-1", state.SessionID)
}

func TestAsyncToolRegistry_Complete(t *testing.T) {
	registry := NewAsyncToolRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register("session-1", "call-1", "test_tool", cancel)
	registry.Complete("call-1", map[string]any{"result": "success"})

	state, err := registry.GetStatus("call-1")
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, state.Status)
	assert.NotNil(t, state.Result)
	assert.False(t, state.EndTime.IsZero())
}

func TestAsyncToolRegistry_Fail(t *testing.T) {
	registry := NewAsyncToolRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register("session-1", "call-1", "test_tool", cancel)
	registry.Fail("call-1", "something went wrong")

	state, err := registry.GetStatus("call-1")
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, state.Status)
	assert.Equal(t, "something went wrong", state.Error)
	assert.False(t, state.EndTime.IsZero())
}

func TestAsyncToolRegistry_Cancel(t *testing.T) {
	registry := NewAsyncToolRegistry()
	ctx, cancel := context.WithCancel(context.Background())

	registry.Register("session-1", "call-1", "test_tool", cancel)

	cancelled := registry.Cancel("call-1")
	assert.True(t, cancelled)

	state, err := registry.GetStatus("call-1")
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, state.Status)
	assert.False(t, state.EndTime.IsZero())

	// Verify context was cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("context should be cancelled")
	}
}

func TestAsyncToolRegistry_Cancel_AlreadyCompleted(t *testing.T) {
	registry := NewAsyncToolRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register("session-1", "call-1", "test_tool", cancel)
	registry.Complete("call-1", "done")

	// Should not cancel already completed tool
	cancelled := registry.Cancel("call-1")
	assert.False(t, cancelled)

	state, _ := registry.GetStatus("call-1")
	assert.Equal(t, StatusCompleted, state.Status)
}

func TestAsyncToolRegistry_Cancel_NotFound(t *testing.T) {
	registry := NewAsyncToolRegistry()

	cancelled := registry.Cancel("non-existent")
	assert.False(t, cancelled)
}

func TestAsyncToolRegistry_CancelSession(t *testing.T) {
	registry := NewAsyncToolRegistry()

	// Register multiple tools for same session
	cancels := make([]context.CancelFunc, 3)
	for i := 0; i < 3; i++ {
		_, cancel := context.WithCancel(context.Background())
		cancels[i] = cancel
		registry.Register("session-1", fmt.Sprintf("call-%d", i), "test_tool", cancel)
	}

	count := registry.CancelSession("session-1")
	assert.Equal(t, 3, count)

	// Verify all are cancelled
	for i := 0; i < 3; i++ {
		state, _ := registry.GetStatus(fmt.Sprintf("call-%d", i))
		assert.Equal(t, StatusCancelled, state.Status)
	}
}

func TestAsyncToolRegistry_CancelSession_MixedSessions(t *testing.T) {
	registry := NewAsyncToolRegistry()

	// Register tools for different sessions
	_, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	_, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	registry.Register("session-1", "call-1", "test_tool", cancel1)
	registry.Register("session-2", "call-2", "test_tool", cancel2)

	count := registry.CancelSession("session-1")
	assert.Equal(t, 1, count)

	// Verify session-1 tool is cancelled
	state1, _ := registry.GetStatus("call-1")
	assert.Equal(t, StatusCancelled, state1.Status)

	// Verify session-2 tool is still running
	state2, _ := registry.GetStatus("call-2")
	assert.Equal(t, StatusRunning, state2.Status)
}

func TestAsyncToolRegistry_ListBySession(t *testing.T) {
	registry := NewAsyncToolRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register tools for different sessions
	registry.Register("session-1", "call-1", "tool_a", cancel)
	registry.Register("session-1", "call-2", "tool_b", cancel)
	registry.Register("session-2", "call-3", "tool_c", cancel)

	tools := registry.ListBySession("session-1")
	assert.Len(t, tools, 2)

	toolIDs := make(map[string]bool)
	for _, tool := range tools {
		toolIDs[tool.CallID] = true
	}
	assert.True(t, toolIDs["call-1"])
	assert.True(t, toolIDs["call-2"])

	// Verify session-2 has only one tool
	tools2 := registry.ListBySession("session-2")
	assert.Len(t, tools2, 1)
	assert.Equal(t, "call-3", tools2[0].CallID)
}

func TestAsyncToolRegistry_GetStatus_NotFound(t *testing.T) {
	registry := NewAsyncToolRegistry()

	_, err := registry.GetStatus("non-existent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool call not found")
}

func TestAsyncToolRegistry_Unregister(t *testing.T) {
	registry := NewAsyncToolRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register("session-1", "call-1", "test_tool", cancel)

	// Verify it exists
	_, err := registry.GetStatus("call-1")
	require.NoError(t, err)

	// Unregister
	registry.Unregister("call-1")

	// Verify it's gone
	_, err = registry.GetStatus("call-1")
	require.Error(t, err)
}

func TestAsyncToolRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewAsyncToolRegistry()
	var wg sync.WaitGroup

	// Concurrent registrations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			callID := fmt.Sprintf("call-%d", id)
			registry.Register("session-1", callID, "test_tool", cancel)
		}(i)
	}

	wg.Wait()

	tools := registry.ListBySession("session-1")
	assert.Len(t, tools, 100)
}

func TestAsyncToolRegistry_ConcurrentReadWrite(t *testing.T) {
	registry := NewAsyncToolRegistry()
	var wg sync.WaitGroup

	// Register initial entries
	for i := 0; i < 50; i++ {
		_, cancel := context.WithCancel(context.Background())
		registry.Register("session-1", fmt.Sprintf("call-%d", i), "test_tool", cancel)
	}

	// Concurrent reads and writes
	for i := 0; i < 50; i++ {
		// Writer goroutine
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			registry.Complete(fmt.Sprintf("call-%d", id), "done")
		}(i)

		// Reader goroutine
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _ = registry.GetStatus(fmt.Sprintf("call-%d", id))
		}(i)
	}

	wg.Wait()

	// Verify all completed
	tools := registry.ListBySession("session-1")
	for _, tool := range tools {
		assert.Equal(t, StatusCompleted, tool.Status)
	}
}
