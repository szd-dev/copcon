package entity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertPlainText(t *testing.T) {
	messages := []UIMessage{
		{
			ID:    "msg-1",
			Role:  "user",
			Steps: []UIStep{{Parts: []UIPart{{Type: UIPartText, Text: "Hello"}}}},
		},
		{
			ID:    "msg-2",
			Role:  "assistant",
			Steps: []UIStep{{Parts: []UIPart{{Type: UIPartText, Text: "Hi there!"}}}},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 2)
	assert.Equal(t, ModelMessage{Role: "user", Content: "Hello"}, result[0])
	assert.Equal(t, ModelMessage{Role: "assistant", Content: "Hi there!"}, result[1])
}

func TestConvertToolCalls(t *testing.T) {
	messages := []UIMessage{
		{
			ID:   "msg-1",
			Role: "assistant",
			Steps: []UIStep{{Parts: []UIPart{
				{Type: UIPartText, Text: "Let me check that."},
				{Type: UIPartToolCall, ToolCallID: "call-1", ToolName: "get_weather", Args: `{"city":"SF"}`, Output: `{"temp":72}`},
			}}},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 2)

	assert.Equal(t, "assistant", result[0].Role)
	assert.Equal(t, "Let me check that.", result[0].Content)
	assert.Len(t, result[0].ToolCalls, 1)
	assert.Equal(t, ModelToolCall{
		ID:   "call-1",
		Type: "function",
		Function: ModelFunctionCall{
			Name:      "get_weather",
			Arguments: `{"city":"SF"}`,
		},
	}, result[0].ToolCalls[0])

	assert.Equal(t, ModelMessage{
		Role:       "tool",
		Content:    `{"temp":72}`,
		ToolCallID: "call-1",
		Name:       "get_weather",
	}, result[1])
}

func TestConvertMultiTurn(t *testing.T) {
	messages := []UIMessage{
		{
			ID:    "msg-1",
			Role:  "user",
			Steps: []UIStep{{Parts: []UIPart{{Type: UIPartText, Text: "What's the weather?"}}}},
		},
		{
			ID:   "msg-2",
			Role: "assistant",
			Steps: []UIStep{{Parts: []UIPart{
				{Type: UIPartToolCall, ToolCallID: "call-1", ToolName: "get_weather", Args: `{"city":"NYC"}`, Output: `{"temp":55}`},
			}}},
		},
		{
			ID:   "msg-3",
			Role: "assistant",
			Steps: []UIStep{{Parts: []UIPart{
				{Type: UIPartText, Text: "It's 55°F in NYC."},
			}}},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 4)

	assert.Equal(t, "user", result[0].Role)
	assert.Equal(t, "What's the weather?", result[0].Content)

	assert.Equal(t, "assistant", result[1].Role)
	assert.Len(t, result[1].ToolCalls, 1)
	assert.Equal(t, "call-1", result[1].ToolCalls[0].ID)

	assert.Equal(t, "tool", result[2].Role)
	assert.Equal(t, "call-1", result[2].ToolCallID)
	assert.Equal(t, `{"temp":55}`, result[2].Content)

	assert.Equal(t, "assistant", result[3].Role)
	assert.Equal(t, "It's 55°F in NYC.", result[3].Content)
	assert.Empty(t, result[3].ToolCalls)
}

func TestConvertDropsReasoning(t *testing.T) {
	messages := []UIMessage{
		{
			ID:   "msg-1",
			Role: "assistant",
			Steps: []UIStep{{Parts: []UIPart{
				{Type: UIPartReasoning, Text: "I need to think about this..."},
				{Type: UIPartText, Text: "Here is my answer."},
			}}},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 1)
	assert.Equal(t, "assistant", result[0].Role)
	assert.Equal(t, "Here is my answer.", result[0].Content)
}

func TestConvertDropsStepStart(t *testing.T) {
	messages := []UIMessage{
		{
			ID:   "msg-1",
			Role: "assistant",
			Steps: []UIStep{{Parts: []UIPart{
				{Type: UIPartStepStart},
				{Type: UIPartText, Text: "Processing..."},
			}}},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 1)
	assert.Equal(t, "assistant", result[0].Role)
	assert.Equal(t, "Processing...", result[0].Content)
}

func TestConvertEmptyContent(t *testing.T) {
	messages := []UIMessage{
		{
			ID:   "msg-1",
			Role: "assistant",
			Steps: []UIStep{{Parts: []UIPart{
				{Type: UIPartToolCall, ToolCallID: "call-1", ToolName: "run_code", Args: `{"code":"print(1)"}`, Output: "1"},
			}}},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 2)

	assert.Equal(t, "assistant", result[0].Role)
	assert.Equal(t, "", result[0].Content)
	assert.Len(t, result[0].ToolCalls, 1)

	assert.Equal(t, "tool", result[1].Role)
	assert.Equal(t, "1", result[1].Content)
	assert.Equal(t, "call-1", result[1].ToolCallID)
	assert.Equal(t, "run_code", result[1].Name)
}

func TestConvertMultipleToolCalls(t *testing.T) {
	messages := []UIMessage{
		{
			ID:   "msg-1",
			Role: "assistant",
			Steps: []UIStep{{Parts: []UIPart{
				{Type: UIPartText, Text: "Running both."},
				{Type: UIPartToolCall, ToolCallID: "call-1", ToolName: "tool_a", Args: `{"x":1}`, Output: "result_a"},
				{Type: UIPartToolCall, ToolCallID: "call-2", ToolName: "tool_b", Args: `{"y":2}`, Output: "result_b"},
			}}},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 3)

	assert.Equal(t, "assistant", result[0].Role)
	assert.Equal(t, "Running both.", result[0].Content)
	assert.Len(t, result[0].ToolCalls, 2)
	assert.Equal(t, "call-1", result[0].ToolCalls[0].ID)
	assert.Equal(t, "call-2", result[0].ToolCalls[1].ID)

	assert.Equal(t, ModelMessage{Role: "tool", Content: "result_a", ToolCallID: "call-1", Name: "tool_a"}, result[1])
	assert.Equal(t, ModelMessage{Role: "tool", Content: "result_b", ToolCallID: "call-2", Name: "tool_b"}, result[2])
}

func TestConvertEmpty(t *testing.T) {
	result := ConvertToModelMessages([]UIMessage{})
	assert.Empty(t, result)
}

func TestConvertUserMultipleTextParts(t *testing.T) {
	messages := []UIMessage{
		{
			ID:   "msg-1",
			Role: "user",
			Steps: []UIStep{{Parts: []UIPart{
				{Type: UIPartText, Text: "Hello "},
				{Type: UIPartText, Text: "World"},
			}}},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 1)
	assert.Equal(t, "user", result[0].Role)
	assert.Equal(t, "Hello World", result[0].Content)
}

func TestConvertMultiStepAssistant(t *testing.T) {
	messages := []UIMessage{
		{
			ID:   "msg-1",
			Role: "assistant",
			Steps: []UIStep{
				{Parts: []UIPart{
					{Type: UIPartText, Text: "First step answer. "},
					{Type: UIPartToolCall, ToolCallID: "call-1", ToolName: "search", Args: `{"q":"test"}`, Output: "found"},
				}},
				{Parts: []UIPart{
					{Type: UIPartText, Text: "Second step answer."},
					{Type: UIPartToolCall, ToolCallID: "call-2", ToolName: "compute", Args: `{"x":1}`, Output: "42"},
				}},
			},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 3)

	assert.Equal(t, "assistant", result[0].Role)
	assert.Equal(t, "First step answer. Second step answer.", result[0].Content)
	assert.Len(t, result[0].ToolCalls, 2)
	assert.Equal(t, "call-1", result[0].ToolCalls[0].ID)
	assert.Equal(t, "search", result[0].ToolCalls[0].Function.Name)
	assert.Equal(t, "call-2", result[0].ToolCalls[1].ID)
	assert.Equal(t, "compute", result[0].ToolCalls[1].Function.Name)

	assert.Equal(t, ModelMessage{Role: "tool", Content: "found", ToolCallID: "call-1", Name: "search"}, result[1])
	assert.Equal(t, ModelMessage{Role: "tool", Content: "42", ToolCallID: "call-2", Name: "compute"}, result[2])
}

func TestConvertFallbackToParts(t *testing.T) {
	messages := []UIMessage{
		{
			ID:    "msg-1",
			Role:  "user",
			Parts: []UIPart{{Type: UIPartText, Text: "Legacy message"}},
		},
	}

	result := ConvertToModelMessages(messages)

	assert.Len(t, result, 1)
	assert.Equal(t, "user", result[0].Role)
	assert.Equal(t, "Legacy message", result[0].Content)
}
