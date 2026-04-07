package entity

// EventType 定义事件类型
type EventType string

const (
	EventMessage    EventType = "message"
	EventReasoning  EventType = "reasoning"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventThought    EventType = "thought"
	EventDone       EventType = "done"
	EventError      EventType = "error"

	// Async tool execution events
	EventAsyncToolStarted       EventType = "async_tool_started"
	EventAsyncToolComplete      EventType = "async_tool_complete"
	EventAsyncToolFailed        EventType = "async_tool_failed"
	EventAsyncCompletionPending EventType = "async_completion_pending"
)

// Event 定义事件结构
type Event struct {
	Type EventType `json:"type"`
	Data any       `json:"data"`
}

// MessageData 消息事件数据
type MessageData struct {
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
}

// ReasoningData 推理事件数据
type ReasoningData struct {
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
}

// ToolCallData 工具调用事件数据
type ToolCallData struct {
	MessageID string         `json:"message_id"`
	ToolName  string         `json:"tool_name"`
	Args      map[string]any `json:"args"`
	ID        string         `json:"id"`
}

// ToolResultData 工具结果事件数据
type ToolResultData struct {
	MessageID string `json:"message_id"`
	ToolName  string `json:"tool_name"`
	Result    any    `json:"result"`
	ID        string `json:"id"`
}

// DoneData 完成事件数据
type DoneData struct {
	MessageID string `json:"message_id"`
}

// ErrorData 错误事件数据
type ErrorData struct {
	Error string `json:"error"`
}

// AsyncToolStartedData 异步工具开始事件数据
type AsyncToolStartedData struct {
	CallID    string `json:"call_id"`
	ToolName  string `json:"tool_name"`
	SessionID string `json:"session_id"`
}

// AsyncToolCompleteData 异步工具完成事件数据
type AsyncToolCompleteData struct {
	CallID   string `json:"call_id"`
	ToolName string `json:"tool_name"`
	Result   any    `json:"result"`
	Duration int64  `json:"duration_ms"`
}

// AsyncToolFailedData 异步工具失败事件数据
type AsyncToolFailedData struct {
	CallID   string `json:"call_id"`
	ToolName string `json:"tool_name"`
	Error    string `json:"error"`
	Duration int64  `json:"duration_ms"`
}

// AsyncCompletionPendingData 异步完成待处理事件数据 (用于前端轮询)
type AsyncCompletionPendingData struct {
	CallID      string `json:"call_id"`
	ToolName    string `json:"tool_name"`
	SessionID   string `json:"session_id"`
	CompletedAt string `json:"completed_at"`
}
