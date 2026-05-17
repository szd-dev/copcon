package entity

// EventType 定义事件类型
type EventType string

const (
	// ---- 旧版事件类型（已废弃，向后兼容保留） ----

	// Deprecated: use part_create / part_update / step_create instead
	EventMessage EventType = "message"
	// Deprecated: use part_create with partType "reasoning" instead
	EventReasoning EventType = "reasoning"
	// Deprecated: use part_create with partType "tool-call" instead
	EventToolCall EventType = "tool_call"
	// Deprecated: use part_update with output field instead
	EventToolResult EventType = "tool_result"
	// Deprecated: never emitted
	EventThought EventType = "thought"
	// Deprecated: use message_done instead
	EventDone EventType = "done"

	EventError EventType = "error"

	// ---- Step/Part级别事件类型（新版） ----

	// EventStepCreate 创建新的Step
	EventStepCreate EventType = "step_create"
	// EventPartCreate 创建新的UI Part
	EventPartCreate EventType = "part_create"
	// EventPartUpdate 更新已有Part的状态或内容
	EventPartUpdate EventType = "part_update"
	// EventMessageDone 标记消息流结束
	EventMessageDone EventType = "message_done"

	// ---- Async tool execution events ----
	EventAsyncToolStarted  EventType = "async_tool_started"
	EventAsyncToolComplete EventType = "async_tool_complete"
	EventAsyncToolFailed   EventType = "async_tool_failed"
	// Deprecated: only used in metadata, not as SSE event
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

// StepCreateData Step创建事件数据
type StepCreateData struct {
	MessageID string `json:"messageId"`
	StepIndex int    `json:"stepIndex"`
}

// PartCreateData Part创建事件数据
// PartType 可选值: "text", "reasoning", "tool-call"
type PartCreateData struct {
	MessageID  string `json:"messageId"`
	StepIndex  int    `json:"stepIndex"`
	PartIndex  int    `json:"partIndex"`
	PartType   string `json:"partType"`
	State      string `json:"state,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Args       string `json:"args,omitempty"`
}

// PartUpdateData Part更新事件数据
type PartUpdateData struct {
	MessageID string `json:"messageId"`
	StepIndex int    `json:"stepIndex"`
	PartIndex int    `json:"partIndex"`
	PartType  string `json:"partType"`
	TextDelta string `json:"textDelta,omitempty"`
	State     string `json:"state,omitempty"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

// MessageDoneData 消息完成事件数据
type MessageDoneData struct {
	MessageID string `json:"messageId"`
}

// AsyncToolStartedData 异步工具开始事件数据
type AsyncToolStartedData struct {
	MessageID string `json:"message_id"`
	CallID    string `json:"call_id"`
	ToolName  string `json:"tool_name"`
	SessionID string `json:"session_id"`
}

// AsyncToolCompleteData 异步工具完成事件数据
type AsyncToolCompleteData struct {
	MessageID string `json:"message_id"`
	CallID    string `json:"call_id"`
	ToolName  string `json:"tool_name"`
	Result    any    `json:"result"`
	Duration  int64  `json:"duration_ms"`
}

// AsyncToolFailedData 异步工具失败事件数据
type AsyncToolFailedData struct {
	MessageID string `json:"message_id"`
	CallID    string `json:"call_id"`
	ToolName  string `json:"tool_name"`
	Error     string `json:"error"`
	Duration  int64  `json:"duration_ms"`
}

// AsyncCompletionPendingData 异步完成待处理事件数据 (用于前端轮询)
type AsyncCompletionPendingData struct {
	CallID      string `json:"call_id"`
	ToolName    string `json:"tool_name"`
	SessionID   string `json:"session_id"`
	CompletedAt string `json:"completed_at"`
}
