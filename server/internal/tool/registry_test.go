package tool

import (
	"fmt"
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
