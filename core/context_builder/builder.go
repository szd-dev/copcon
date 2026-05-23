package context_builder

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/copcon/core/entity"
)

// LegacyMessage mirrors the fields of session.Message that SynthesizeUIMessage
// needs. It decouples context_builder from the server/session package.
type LegacyMessage struct {
	ID        uuid.UUID
	Role      string
	Content   string
	Reasoning string
	ToolCalls []LegacyToolCall
}

// LegacyToolCall mirrors session.ToolCall.
type LegacyToolCall struct {
	ID       string
	Type     string
	Function LegacyFunctionCall
}

// LegacyFunctionCall mirrors session.FunctionCall.
type LegacyFunctionCall struct {
	Name      string
	Arguments string
}

// ContextBuilder converts UI-layer messages into the flat MessageForLLM
// sequence expected by LLM providers. It is a pure function with no
// persistence or side effects.
type ContextBuilder interface {
	Build(ctx context.Context, messages []entity.UIMessage, systemPrompt string, userInput string) ([]entity.MessageForLLM, error)
}

type builder struct{}

func New() ContextBuilder {
	return &builder{}
}

func (b *builder) Build(ctx context.Context, uiMessages []entity.UIMessage, systemPrompt string, userInput string) ([]entity.MessageForLLM, error) {
	messages := make([]entity.MessageForLLM, 0)

	if systemPrompt != "" {
		messages = append(messages, entity.MessageForLLM{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	modelMessages := entity.ConvertToModelMessages(uiMessages)
	for _, mm := range modelMessages {
		messages = append(messages, entity.MessageForLLM{
			Role:       mm.Role,
			Content:    mm.Content,
			ToolCallID: mm.ToolCallID,
			ToolCalls:  mm.ToolCalls,
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

// SynthesizeUIMessage creates a UIMessage from legacy Content/Reasoning/ToolCalls fields.
// Returns nil for unsupported roles.
func SynthesizeUIMessage(msg LegacyMessage, toolResultByCallID map[string]string) *entity.UIMessage {
	switch msg.Role {
	case "user":
		parts := []entity.UIPart{
			{Type: entity.UIPartText, Text: msg.Content, State: entity.UIPartStateDone, StepIndex: 0},
		}
		return &entity.UIMessage{
			ID:    msg.ID.String(),
			Role:  "user",
			Steps: GroupPartsByStep(parts),
		}
	case "assistant":
		var parts []entity.UIPart
		if msg.Reasoning != "" {
			parts = append(parts, entity.UIPart{
				Type:      entity.UIPartReasoning,
				Text:      msg.Reasoning,
				State:     entity.UIPartStateDone,
				StepIndex: 0,
			})
		}
		if msg.Content != "" || len(msg.ToolCalls) == 0 {
			parts = append(parts, entity.UIPart{
				Type:      entity.UIPartText,
				Text:      msg.Content,
				State:     entity.UIPartStateDone,
				StepIndex: 0,
			})
		}
		for _, tc := range msg.ToolCalls {
			output := toolResultByCallID[tc.ID]
			parts = append(parts, entity.UIPart{
				Type:       entity.UIPartToolCall,
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Args:       tc.Function.Arguments,
				Output:     output,
				State:      entity.UIPartStateDone,
				StepIndex:  0,
			})
		}
		return &entity.UIMessage{
			ID:    msg.ID.String(),
			Role:  "assistant",
			Steps: GroupPartsByStep(parts),
		}
	default:
		return nil
	}
}

// GroupPartsByStep groups UIParts by StepIndex into UIStep objects.
func GroupPartsByStep(parts []entity.UIPart) []entity.UIStep {
	stepMap := make(map[int][]entity.UIPart)
	var stepIndices []int
	for _, p := range parts {
		if _, exists := stepMap[p.StepIndex]; !exists {
			stepIndices = append(stepIndices, p.StepIndex)
		}
		stepMap[p.StepIndex] = append(stepMap[p.StepIndex], p)
	}
	sort.Ints(stepIndices)
	var steps []entity.UIStep
	for _, idx := range stepIndices {
		steps = append(steps, entity.UIStep{
			Parts: stepMap[idx],
			State: entity.UIPartStateDone,
		})
	}
	return steps
}

// ConvertLegacyToolCalls converts LegacyToolCalls to entity ModelToolCalls.
// Used when building legacy messages that have LegacyToolCall instead of entity.ModelToolCall.
func ConvertLegacyToolCalls(toolCalls []LegacyToolCall) []entity.ModelToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]entity.ModelToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		result[i] = entity.ModelToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: entity.ModelFunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return result
}
