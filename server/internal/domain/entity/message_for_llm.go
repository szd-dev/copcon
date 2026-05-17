package entity

// MessageForLLM represents a message formatted for the LLM API.
// It flattens the rich UIMessage structure into the role/content/tool_calls
// format expected by LLM providers.
type MessageForLLM struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Reasoning  string          `json:"reasoning,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []ModelToolCall `json:"tool_calls,omitempty"`
}
