package tools

import (
	"github.com/copcon/core/context_builder"
	"github.com/copcon/server/internal/session"
)

func sessionMsgToLegacy(msg session.Message) context_builder.LegacyMessage {
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
