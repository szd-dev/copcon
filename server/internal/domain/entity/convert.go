package entity

import "strings"

// ConvertToModelMessages converts UI-layer messages into the flat sequence of
// ModelMessages expected by the OpenAI Chat Completion API.
//
// Conversion rules:
//   - User messages: all text parts are concatenated into Content.
//   - Assistant messages:
//     1. Text parts are concatenated into Content.
//     2. Tool-call parts produce ModelToolCall entries on the assistant message
//     AND a separate role="tool" ModelMessage per call (Content=Output,
//     ToolCallID=ToolCallID, Name=ToolName).
//     3. Reasoning and step-start parts are discarded (UI-only).
//   - System messages are NOT UIMessages and are not handled here;
//     the system prompt is injected separately in BuildContext.
func ConvertToModelMessages(uiMessages []UIMessage) []ModelMessage {
	result := make([]ModelMessage, 0, len(uiMessages))

	for _, msg := range uiMessages {
		switch msg.Role {
		case "user":
			result = append(result, convertUserMessage(msg))
		case "assistant":
			result = append(result, convertAssistantMessage(msg)...)
		}
	}

	return result
}

// collectParts returns the parts to iterate: Steps[].Parts if Steps is
// non-empty, otherwise falls back to the flat Parts slice for backward compat.
func collectParts(msg UIMessage) []UIPart {
	if len(msg.Steps) > 0 {
		var parts []UIPart
		for _, step := range msg.Steps {
			parts = append(parts, step.Parts...)
		}
		return parts
	}
	return msg.Parts
}

// convertUserMessage extracts all text parts from a user UIMessage and
// concatenates them into a single ModelMessage with role="user".
func convertUserMessage(msg UIMessage) ModelMessage {
	var sb strings.Builder
	for _, part := range collectParts(msg) {
		if part.Type == UIPartText {
			sb.WriteString(part.Text)
		}
	}
	return ModelMessage{
		Role:    "user",
		Content: sb.String(),
	}
}

// convertAssistantMessage flattens an assistant UIMessage into one or more
// ModelMessages following the OpenAI API convention:
//
//	[assistant (content + tool_calls), tool (result 1), tool (result 2), ...]
//
// Reasoning and step-start parts are discarded.
func convertAssistantMessage(msg UIMessage) []ModelMessage {
	var contentParts []string
	var toolCallParts []UIPart

	for _, part := range collectParts(msg) {
		switch part.Type {
		case UIPartText:
			contentParts = append(contentParts, part.Text)
		case UIPartToolCall:
			toolCallParts = append(toolCallParts, part)
		}
	}

	assistantMsg := ModelMessage{
		Role:    "assistant",
		Content: strings.Join(contentParts, ""),
	}

	if len(toolCallParts) > 0 {
		assistantMsg.ToolCalls = make([]ModelToolCall, len(toolCallParts))
		for i, part := range toolCallParts {
			assistantMsg.ToolCalls[i] = ModelToolCall{
				ID:   part.ToolCallID,
				Type: "function",
				Function: ModelFunctionCall{
					Name:      part.ToolName,
					Arguments: part.Args,
				},
			}
		}
	}

	result := make([]ModelMessage, 0, 1+len(toolCallParts))
	result = append(result, assistantMsg)

	for _, part := range toolCallParts {
		result = append(result, ModelMessage{
			Role:       "tool",
			Content:    part.Output,
			ToolCallID: part.ToolCallID,
			Name:       part.ToolName,
		})
	}

	return result
}
