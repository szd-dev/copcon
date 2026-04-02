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
	Content string `json:"content"`
}

// ToolCallData 工具调用事件数据
type ToolCallData struct {
	ToolName string         `json:"tool_name"`
	Args     map[string]any `json:"args"`
	ID       string         `json:"id"`
}

// ToolResultData 工具结果事件数据
type ToolResultData struct {
	ToolName string `json:"tool_name"`
	Result   any    `json:"result"`
	ID       string `json:"id"`
}

// DoneData 完成事件数据
type DoneData struct {
	MessageID string `json:"message_id"`
}

// ErrorData 错误事件数据
type ErrorData struct {
	Error string `json:"error"`
}
