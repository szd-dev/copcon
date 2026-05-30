package tools

import (
	"fmt"
	"time"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

type GetToolStatusTool struct {
	asyncRegistry *tool.AsyncToolRegistry
}

func NewGetToolStatusTool(asyncRegistry *tool.AsyncToolRegistry) *GetToolStatusTool {
	return &GetToolStatusTool{asyncRegistry: asyncRegistry}
}

func (t *GetToolStatusTool) Name() string {
	return "get_tool_status"
}

func (t *GetToolStatusTool) Description() string {
	return "查询异步工具的执行状态。用于检查后台任务是否完成。"
}

func (t *GetToolStatusTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"call_id": map[string]any{
				"type":        "string",
				"description": "工具调用的唯一标识符",
			},
		},
		"required": []string{"call_id"},
	}
}

func (t *GetToolStatusTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	callID, ok := args["call_id"].(string)
	if !ok || callID == "" {
		return errorResult("call_id is required")
	}

	state, err := t.asyncRegistry.GetStatus(callID)
	if err != nil {
		return errorResult(fmt.Sprintf("tool call not found: %s", callID))
	}

	response := map[string]any{
		"call_id":    state.CallID,
		"tool_name":  state.ToolName,
		"status":     string(state.Status),
		"start_time": state.StartTime.Format(time.RFC3339),
	}

	if !state.EndTime.IsZero() {
		response["end_time"] = state.EndTime.Format(time.RFC3339)
		response["duration"] = state.EndTime.Sub(state.StartTime).String()
	}

	if state.Result != nil {
		response["result"] = state.Result
	}

	if state.Error != "" {
		response["error"] = state.Error
	}

	return successResult(response)
}

type GetToolResultTool struct {
	asyncRegistry *tool.AsyncToolRegistry
}

func NewGetToolResultTool(registry *tool.AsyncToolRegistry) *GetToolResultTool {
	return &GetToolResultTool{asyncRegistry: registry}
}

func (t *GetToolResultTool) Name() string {
	return "get_tool_result"
}

func (t *GetToolResultTool) Description() string {
	return "获取已完成异步工具的执行结果。仅在工具状态为 completed 时可用。"
}

func (t *GetToolResultTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"call_id": map[string]any{
				"type":        "string",
				"description": "工具调用的唯一标识符",
			},
		},
		"required": []string{"call_id"},
	}
}

func (t *GetToolResultTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	callID, ok := args["call_id"].(string)
	if !ok || callID == "" {
		return &tool.ToolResult{
			Success: false,
			Error:   "call_id is required",
		}, nil
	}

	state, err := t.asyncRegistry.GetStatus(callID)
	if err != nil {
		return &tool.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool not found: %s", callID),
		}, nil
	}

	if state.Status != tool.StatusCompleted {
		return &tool.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool is not completed (status: %s)", state.Status),
		}, nil
	}

	return &tool.ToolResult{
		Success: true,
		Data:    state.Result,
	}, nil
}

type CancelToolTool struct {
	asyncRegistry *tool.AsyncToolRegistry
}

func NewCancelToolTool(asyncRegistry *tool.AsyncToolRegistry) *CancelToolTool {
	return &CancelToolTool{asyncRegistry: asyncRegistry}
}

func (t *CancelToolTool) Name() string {
	return "cancel_tool"
}

func (t *CancelToolTool) Description() string {
	return "取消正在执行的异步工具。工具会立即停止，结果不可恢复。"
}

func (t *CancelToolTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"call_id": map[string]any{
				"type":        "string",
				"description": "工具调用的唯一标识符",
			},
		},
		"required": []string{"call_id"},
	}
}

func (t *CancelToolTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	callID, ok := args["call_id"].(string)
	if !ok || callID == "" {
		return errorResult("call_id is required")
	}

	cancelled := t.asyncRegistry.Cancel(callID)
	if !cancelled {
		return errorResult(fmt.Sprintf("could not cancel tool (not running or not found): %s", callID))
	}

	return successResult(map[string]any{
		"call_id":   callID,
		"cancelled": true,
	})
}

type ListAsyncToolsTool struct {
	asyncRegistry *tool.AsyncToolRegistry
}

func NewListAsyncToolsTool(asyncRegistry *tool.AsyncToolRegistry) *ListAsyncToolsTool {
	return &ListAsyncToolsTool{asyncRegistry: asyncRegistry}
}

func (t *ListAsyncToolsTool) Name() string {
	return "list_async_tools"
}

func (t *ListAsyncToolsTool) Description() string {
	return "列出当前会话中所有异步工具的状态。用于查看后台任务进度。"
}

func (t *ListAsyncToolsTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *ListAsyncToolsTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	sessionID := chatCtx.SessionID()
	tools := t.asyncRegistry.ListBySession(sessionID)

	result := make([]map[string]any, len(tools))
	for i, state := range tools {
		result[i] = map[string]any{
			"call_id":    state.CallID,
			"tool_name":  state.ToolName,
			"status":     string(state.Status),
			"start_time": state.StartTime.Format(time.RFC3339),
		}
		if !state.EndTime.IsZero() {
			result[i]["duration_ms"] = state.EndTime.Sub(state.StartTime).Milliseconds()
		}
	}

	return successResult(map[string]any{
		"tools": result,
		"count": len(result),
	})
}

type asyncCapability struct{}

func (c *asyncCapability) Name() string                         { return capabilities.ToolAsync }
func (c *asyncCapability) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeTool }
func (c *asyncCapability) DependsOn() []string                  { return nil }
func (c *asyncCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	registry := tool.NewAsyncToolRegistry()
	return NewGetToolStatusTool(registry), nil
}