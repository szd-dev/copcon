package tool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/testutil"
)

type mockTool struct {
	name        string
	description string
	schema      map[string]any
	result      *ToolResult
	execErr     error
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) InputSchema() map[string]any {
	return m.schema
}

func (m *mockTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*ToolResult, error) {
	return m.result, m.execErr
}

func TestToolManager_Register(t *testing.T) {
	mgr := NewToolManager()

	tool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		schema:      map[string]any{"type": "object"},
	}

	err := mgr.Register(tool)
	assert.NoError(t, err)

	tools := mgr.List()
	assert.Len(t, tools, 1)
	assert.Equal(t, "test_tool", tools[0].Name)
}

func TestToolManager_Register_Duplicate(t *testing.T) {
	mgr := NewToolManager()

	tool := &mockTool{name: "test_tool"}
	err := mgr.Register(tool)
	require.NoError(t, err)

	err = mgr.Register(tool)
	assert.ErrorIs(t, err, ErrToolAlreadyExists)
}

func TestToolManager_Execute(t *testing.T) {
	mgr := NewToolManager()

	tool := &mockTool{
		name:   "echo",
		result: &ToolResult{Success: true, Data: "pong"},
		schema: map[string]any{"type": "object"},
	}
	err := mgr.Register(tool)
	require.NoError(t, err)

	chatCtx := testutil.NewMockChatContext(context.Background(), "", "")
	result, err := mgr.Execute(chatCtx, "echo", map[string]any{})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "pong", result.Data)
}

func TestToolManager_Execute_NotFound(t *testing.T) {
	mgr := NewToolManager()

	chatCtx := testutil.NewMockChatContext(context.Background(), "", "")
	_, err := mgr.Execute(chatCtx, "nonexistent", map[string]any{})
	assert.ErrorIs(t, err, ErrToolNotFound)
}

func TestToolManager_GetToolDefs(t *testing.T) {
	mgr := NewToolManager()

	tool := &mockTool{
		name:        "calculator",
		description: "Performs calculations",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{"type": "string"},
			},
		},
	}
	err := mgr.Register(tool)
	require.NoError(t, err)

	tools := mgr.GetToolDefs()

	assert.Len(t, tools, 1)
}
