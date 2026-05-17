package entity

// ModelMessage 表示 OpenAI Chat Completion API 消息格式。
// 此类型是 OpenAI Chat Completion API 消息格式的 Go 映射，用于解耦内部逻辑与 go-openai SDK。
//
// Role 取值:
//   - "system": 系统指令
//   - "user": 用户输入
//   - "assistant": 模型回复（可能包含 ToolCalls）
//   - "tool": 工具执行结果（必须设置 ToolCallID 和 Content）
//
// 当 Role 为 "assistant" 时，ToolCalls 可能非空，表示模型请求的工具调用。
// 当 Role 为 "tool" 时，ToolCallID 和 Content 必须有值，表示对应工具调用的执行结果。
type ModelMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []ModelToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

// ModelToolCall 表示模型请求的单个工具调用。
// Type 字段默认值为 "function"，对应 OpenAI API 中 tool_call.type 的唯一合法值。
type ModelToolCall struct {
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function ModelFunctionCall `json:"function"`
}

// ModelFunctionCall 表示工具调用中的函数名和参数。
type ModelFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
