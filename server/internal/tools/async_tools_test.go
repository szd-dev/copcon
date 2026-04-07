package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/server/internal/testutil"
	"github.com/copcon/server/internal/tool"
)

// parseResponse extracts and unmarshals the response from successResult
func parseResponse(t *testing.T, result *tool.ToolResult) map[string]any {
	data := result.Data.(map[string]any)
	responseStr := data["response"].(string)
	var parsed map[string]any
	err := json.Unmarshal([]byte(responseStr), &parsed)
	require.NoError(t, err)
	return parsed
}

// TestGetToolStatusRunning verifies status query for a running tool
func TestGetToolStatusRunning(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	statusTool := NewGetToolStatusTool(registry)

	callID := "call_running_123"
	sessionID := "session_abc"
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register(sessionID, callID, "execute_code", cancel)

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := statusTool.Execute(chatCtx, map[string]any{
		"call_id": callID,
	})

	require.NoError(t, err)
	require.True(t, result.Success)

	data := parseResponse(t, result)
	assert.Equal(t, callID, data["call_id"])
	assert.Equal(t, "execute_code", data["tool_name"])
	assert.Equal(t, "running", data["status"])
	assert.NotEmpty(t, data["start_time"])
	_, hasEndTime := data["end_time"]
	assert.False(t, hasEndTime)
}

// TestGetToolStatusCompleted verifies status query for a completed tool
func TestGetToolStatusCompleted(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	statusTool := NewGetToolStatusTool(registry)

	callID := "call_completed_456"
	sessionID := "session_abc"
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register(sessionID, callID, "execute_code", cancel)
	registry.Complete(callID, map[string]any{"output": "success"})

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := statusTool.Execute(chatCtx, map[string]any{
		"call_id": callID,
	})

	require.NoError(t, err)
	require.True(t, result.Success)

	data := parseResponse(t, result)
	assert.Equal(t, callID, data["call_id"])
	assert.Equal(t, "completed", data["status"])
	assert.NotEmpty(t, data["end_time"])
	assert.NotEmpty(t, data["duration"])
	assert.NotNil(t, data["result"])
}

// TestGetToolStatusNotFound verifies error when tool not found
func TestGetToolStatusNotFound(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	statusTool := NewGetToolStatusTool(registry)

	chatCtx := testutil.NewMockChatContext(context.Background(), "session_abc", "agent_1")
	result, err := statusTool.Execute(chatCtx, map[string]any{
		"call_id": "nonexistent_call",
	})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "tool call not found")
}

// TestGetToolStatusMissingCallID verifies error when call_id is missing
func TestGetToolStatusMissingCallID(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	statusTool := NewGetToolStatusTool(registry)

	chatCtx := testutil.NewMockChatContext(context.Background(), "session_abc", "agent_1")
	result, err := statusTool.Execute(chatCtx, map[string]any{})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "call_id is required")
}

// TestGetToolResultCompleted verifies result retrieval for a completed tool
func TestGetToolResultCompleted(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	resultTool := NewGetToolResultTool(registry)

	callID := "call_result_789"
	sessionID := "session_abc"
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	expectedResult := map[string]any{
		"stdout":    "Hello, World!\n",
		"exit_code": 0,
	}

	registry.Register(sessionID, callID, "execute_code", cancel)
	registry.Complete(callID, expectedResult)

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := resultTool.Execute(chatCtx, map[string]any{
		"call_id": callID,
	})

	require.NoError(t, err)
	assert.True(t, result.Success)

	resultData := result.Data.(map[string]any)
	assert.Equal(t, "Hello, World!\n", resultData["stdout"])
	assert.Equal(t, 0, resultData["exit_code"])
}

// TestGetToolResultRunning verifies error when trying to get result of running tool
func TestGetToolResultRunning(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	resultTool := NewGetToolResultTool(registry)

	callID := "call_running_result"
	sessionID := "session_abc"
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register(sessionID, callID, "execute_code", cancel)

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := resultTool.Execute(chatCtx, map[string]any{
		"call_id": callID,
	})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "tool is not completed")
	assert.Contains(t, result.Error, "running")
}

// TestGetToolResultFailed verifies error when trying to get result of failed tool
func TestGetToolResultFailed(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	resultTool := NewGetToolResultTool(registry)

	callID := "call_failed_result"
	sessionID := "session_abc"
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register(sessionID, callID, "execute_code", cancel)
	registry.Fail(callID, "execution timed out")

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := resultTool.Execute(chatCtx, map[string]any{
		"call_id": callID,
	})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "tool is not completed")
	assert.Contains(t, result.Error, "failed")
}

// TestGetToolResultNotFound verifies error when tool not found
func TestGetToolResultNotFound(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	resultTool := NewGetToolResultTool(registry)

	chatCtx := testutil.NewMockChatContext(context.Background(), "session_abc", "agent_1")
	result, err := resultTool.Execute(chatCtx, map[string]any{
		"call_id": "nonexistent",
	})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "tool not found")
}

// TestGetToolResultMissingCallID verifies error when call_id is missing
func TestGetToolResultMissingCallID(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	resultTool := NewGetToolResultTool(registry)

	chatCtx := testutil.NewMockChatContext(context.Background(), "session_abc", "agent_1")
	result, err := resultTool.Execute(chatCtx, map[string]any{})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "call_id is required")
}

// TestCancelToolRunning verifies cancellation works for running tools
func TestCancelToolRunning(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	cancelTool := NewCancelToolTool(registry)

	callID := "call_cancel_running"
	sessionID := "session_abc"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register(sessionID, callID, "execute_code", cancel)

	assert.NoError(t, ctx.Err())

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := cancelTool.Execute(chatCtx, map[string]any{
		"call_id": callID,
	})

	require.NoError(t, err)
	assert.True(t, result.Success)

	data := parseResponse(t, result)
	assert.Equal(t, callID, data["call_id"])
	assert.True(t, data["cancelled"].(bool))

	state, _ := registry.GetStatus(callID)
	assert.Equal(t, tool.StatusCancelled, state.Status)

	assert.Error(t, ctx.Err())
}

// TestCancelToolCompleted verifies error when trying to cancel completed tool
func TestCancelToolCompleted(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	cancelTool := NewCancelToolTool(registry)

	callID := "call_cancel_completed"
	sessionID := "session_abc"
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register(sessionID, callID, "execute_code", cancel)
	registry.Complete(callID, map[string]any{"output": "done"})

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := cancelTool.Execute(chatCtx, map[string]any{
		"call_id": callID,
	})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "could not cancel tool")
}

// TestCancelToolNotFound verifies error when tool not found
func TestCancelToolNotFound(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	cancelTool := NewCancelToolTool(registry)

	chatCtx := testutil.NewMockChatContext(context.Background(), "session_abc", "agent_1")
	result, err := cancelTool.Execute(chatCtx, map[string]any{
		"call_id": "nonexistent",
	})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "could not cancel tool")
}

// TestCancelToolMissingCallID verifies error when call_id is missing
func TestCancelToolMissingCallID(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	cancelTool := NewCancelToolTool(registry)

	chatCtx := testutil.NewMockChatContext(context.Background(), "session_abc", "agent_1")
	result, err := cancelTool.Execute(chatCtx, map[string]any{})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "call_id is required")
}

// TestListAsyncTools verifies listing multiple tools for a session
func TestListAsyncTools(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	listTool := NewListAsyncToolsTool(registry)

	sessionID := "session_list_test"
	otherSessionID := "session_other"

	_, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	registry.Register(sessionID, "call_1", "execute_code", cancel1)

	_, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	registry.Register(sessionID, "call_2", "execute_shell", cancel2)

	registry.Complete("call_1", map[string]any{"output": "done"})

	_, cancel3 := context.WithCancel(context.Background())
	defer cancel3()
	registry.Register(otherSessionID, "call_other", "execute_code", cancel3)

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := listTool.Execute(chatCtx, map[string]any{})

	require.NoError(t, err)
	assert.True(t, result.Success)

	data := parseResponse(t, result)
	tools := data["tools"].([]any)
	count := int(data["count"].(float64))

	assert.Equal(t, 2, count)
	assert.Len(t, tools, 2)

	for _, td := range tools {
		toolData := td.(map[string]any)
		assert.Contains(t, toolData, "call_id")
		assert.Contains(t, toolData, "tool_name")
		assert.Contains(t, toolData, "status")
		assert.Contains(t, toolData, "start_time")
	}

	callIDs := make([]string, len(tools))
	for i, td := range tools {
		toolData := td.(map[string]any)
		callIDs[i] = toolData["call_id"].(string)
	}
	assert.Contains(t, callIDs, "call_1")
	assert.Contains(t, callIDs, "call_2")
	assert.NotContains(t, callIDs, "call_other")
}

// TestListAsyncToolsEmpty verifies empty list when no tools in session
func TestListAsyncToolsEmpty(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	listTool := NewListAsyncToolsTool(registry)

	chatCtx := testutil.NewMockChatContext(context.Background(), "session_empty", "agent_1")
	result, err := listTool.Execute(chatCtx, map[string]any{})

	require.NoError(t, err)
	assert.True(t, result.Success)

	data := parseResponse(t, result)
	tools := data["tools"].([]any)
	count := int(data["count"].(float64))

	assert.Equal(t, 0, count)
	assert.Empty(t, tools)
}

// TestListAsyncToolsWithDuration verifies duration is included for completed tools
func TestListAsyncToolsWithDuration(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	listTool := NewListAsyncToolsTool(registry)

	sessionID := "session_duration_test"

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry.Register(sessionID, "call_with_duration", "execute_code", cancel)

	time.Sleep(10 * time.Millisecond)

	registry.Complete("call_with_duration", map[string]any{"output": "done"})

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := listTool.Execute(chatCtx, map[string]any{})

	require.NoError(t, err)
	assert.True(t, result.Success)

	data := parseResponse(t, result)
	tools := data["tools"].([]any)

	require.Len(t, tools, 1)
	toolData := tools[0].(map[string]any)
	assert.Contains(t, toolData, "duration_ms")
	assert.Greater(t, int64(toolData["duration_ms"].(float64)), int64(0))
}

// TestToolNames verifies all tool names are correct
func TestToolNames(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()

	assert.Equal(t, "get_tool_status", NewGetToolStatusTool(registry).Name())
	assert.Equal(t, "get_tool_result", NewGetToolResultTool(registry).Name())
	assert.Equal(t, "cancel_tool", NewCancelToolTool(registry).Name())
	assert.Equal(t, "list_async_tools", NewListAsyncToolsTool(registry).Name())
}

// TestToolDescriptions verifies all tool descriptions are non-empty
func TestToolDescriptions(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()

	assert.NotEmpty(t, NewGetToolStatusTool(registry).Description())
	assert.NotEmpty(t, NewGetToolResultTool(registry).Description())
	assert.NotEmpty(t, NewCancelToolTool(registry).Description())
	assert.NotEmpty(t, NewListAsyncToolsTool(registry).Description())
}

// TestToolInputSchemas verifies all tool input schemas are valid
func TestToolInputSchemas(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()

	schemas := []map[string]any{
		NewGetToolStatusTool(registry).InputSchema(),
		NewGetToolResultTool(registry).InputSchema(),
		NewCancelToolTool(registry).InputSchema(),
		NewListAsyncToolsTool(registry).InputSchema(),
	}

	for _, schema := range schemas {
		assert.Equal(t, "object", schema["type"])
		assert.NotNil(t, schema["properties"])
	}
}

// TestGetToolStatusFailed verifies status query for a failed tool
func TestGetToolStatusFailed(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	statusTool := NewGetToolStatusTool(registry)

	callID := "call_failed_status"
	sessionID := "session_abc"
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register(sessionID, callID, "execute_code", cancel)
	registry.Fail(callID, "execution error: timeout")

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := statusTool.Execute(chatCtx, map[string]any{
		"call_id": callID,
	})

	require.NoError(t, err)
	require.True(t, result.Success)

	data := parseResponse(t, result)
	assert.Equal(t, "failed", data["status"])
	assert.Equal(t, "execution error: timeout", data["error"])
}

// TestGetToolStatusCancelled verifies status query for a cancelled tool
func TestGetToolStatusCancelled(t *testing.T) {
	registry := tool.NewAsyncToolRegistry()
	statusTool := NewGetToolStatusTool(registry)

	callID := "call_cancelled_status"
	sessionID := "session_abc"
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Register(sessionID, callID, "execute_code", cancel)
	registry.Cancel(callID)

	chatCtx := testutil.NewMockChatContext(context.Background(), sessionID, "agent_1")
	result, err := statusTool.Execute(chatCtx, map[string]any{
		"call_id": callID,
	})

	require.NoError(t, err)
	require.True(t, result.Success)

	data := parseResponse(t, result)
	assert.Equal(t, "cancelled", data["status"])
}
