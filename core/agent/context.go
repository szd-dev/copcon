package agent

import (
	"context"

	"github.com/google/uuid"

	"github.com/copcon/core/context_builder"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/storage"
)

// BuildContext constructs the message sequence for an LLM call.
// It reads history from the MessageStore, converts to UIMessages (using Parts
// when available, falling back to legacy Content/ToolCalls when not), and calls
// the ContextBuilder to produce the final flat MessageForLLM list.
func BuildContext(
	ctx context.Context,
	store storage.MessageStore,
	builder context_builder.ContextBuilder,
	sessionID uuid.UUID,
	userInput string,
	maxTokens int,
	systemPrompt string,
) ([]entity.MessageForLLM, error) {
	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI assistant with access to tools for code execution, file operations, and shell commands. Use these tools when appropriate to help the user."
	}

	history, err := store.List(ctx, sessionID, 1024)
	if err != nil {
		return nil, err
	}

	toolResultByCallID := buildToolResultLookup(history)

	uiMessages, hasFallback := convertMessagesToUI(history, toolResultByCallID)
	if !hasFallback {
		return builder.Build(ctx, uiMessages, systemPrompt, userInput)
	}

	messages := make([]entity.MessageForLLM, 0, len(history)+1)
	messages = append(messages, entity.MessageForLLM{
		Role:    "system",
		Content: systemPrompt,
	})

	for _, msg := range history {
		messages = append(messages, entity.MessageForLLM{
			Role:       msg.Role,
			Content:    msg.Content,
			Reasoning:  msg.Reasoning,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  context_builder.ConvertLegacyToolCalls(storageToolCallsToLegacy(msg.ToolCalls)),
		})
	}

	if userInput != "" {
		messages = append(messages, entity.MessageForLLM{
			Role:    "user",
			Content: userInput,
		})
	}

	return messages, nil
}

func buildToolResultLookup(history []*storage.Message) map[string]string {
	lookup := make(map[string]string)
	for _, msg := range history {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			lookup[msg.ToolCallID] = msg.Content
		}
	}
	return lookup
}

// convertMessagesToUI converts storage.Message records to UIMessages, returning
// false when all messages have Parts and true when any need the legacy fallback path.
func convertMessagesToUI(history []*storage.Message, toolResultByCallID map[string]string) ([]entity.UIMessage, bool) {
	var uiMessages []entity.UIMessage
	hasFallback := false

	for _, msg := range history {
		if msg.Role == "tool" {
			continue
		}

		if len(msg.Parts) > 0 {
			uiParts := convertPartsToUI(msg.Parts, msg.ID.String(), toolResultByCallID)
			steps := context_builder.GroupPartsByStep(uiParts)
			uiMessages = append(uiMessages, entity.UIMessage{
				ID:    msg.ID.String(),
				Role:  msg.Role,
				Steps: steps,
			})
		} else {
			uiMsg := context_builder.SynthesizeUIMessage(storageMsgToLegacy(msg), toolResultByCallID)
			if uiMsg != nil {
				uiMessages = append(uiMessages, *uiMsg)
			} else {
				hasFallback = true
			}
		}
	}

	return uiMessages, hasFallback
}

func convertPartsToUI(parts []storage.Part, messageID string, toolResultByCallID map[string]string) []entity.UIPart {
	var uiParts []entity.UIPart
	for _, p := range parts {
		uiPart := entity.UIPart{
			Type:       entity.UIPartType(p.Type),
			Text:       p.Text,
			State:      entity.UIPartState(p.State),
			ToolCallID: p.ToolCallID,
			ToolName:   p.ToolName,
			Args:       p.Args,
			Output:     p.Output,
			Error:      p.Error,
			StepIndex:  p.StepIndex,
		}

		if p.Type == "tool-call" && uiPart.ToolCallID != "" && uiPart.Output == "" {
			if result, ok := toolResultByCallID[uiPart.ToolCallID]; ok {
				uiPart.Output = result
			}
		}

		uiParts = append(uiParts, uiPart)
	}
	return uiParts
}

func storageMsgToLegacy(msg *storage.Message) context_builder.LegacyMessage {
	tcs := make([]context_builder.LegacyToolCall, len(msg.ToolCalls))
	for i, tc := range msg.ToolCalls {
		tcs[i] = context_builder.LegacyToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: context_builder.LegacyFunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return context_builder.LegacyMessage{
		ID:        msg.ID,
		Role:      msg.Role,
		Content:   msg.Content,
		Reasoning: msg.Reasoning,
		ToolCalls: tcs,
	}
}

func storageToolCallsToLegacy(toolCalls []storage.ToolCall) []context_builder.LegacyToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]context_builder.LegacyToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		result[i] = context_builder.LegacyToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: context_builder.LegacyFunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return result
}