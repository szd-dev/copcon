package session

import (
	"sort"

	"github.com/copcon/core/entity"
)

// GroupPartsByStep groups PersistedParts by StepIndex into UIStep objects.
func GroupPartsByStep(parts PersistedParts) []entity.UIStep {
	stepMap := make(map[int][]entity.UIPart)
	var stepIndices []int
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
			Interrupt:  p.Interrupt,
			StepIndex:  p.StepIndex,
		}
		if _, exists := stepMap[p.StepIndex]; !exists {
			stepIndices = append(stepIndices, p.StepIndex)
		}
		stepMap[p.StepIndex] = append(stepMap[p.StepIndex], uiPart)
	}
	sort.Ints(stepIndices)
	var steps []entity.UIStep
	for _, idx := range stepIndices {
		steps = append(steps, entity.UIStep{
			Parts: stepMap[idx],
			State: entity.UIPartStateDone, // all persisted data is done
		})
	}
	return steps
}

// BackfillParts creates PersistedParts from legacy Content/Reasoning/ToolCalls fields
// when the Parts JSONB column is empty.
func BackfillParts(msg Message, toolResults map[string]string) PersistedParts {
	var parts PersistedParts

	if msg.Role == "user" {
		if msg.Content != "" {
			parts = append(parts, PersistedPart{
				Type:      "text",
				Text:      msg.Content,
				State:     "done",
				StepIndex: 0,
			})
		}
		return parts
	}

	if msg.Role == "assistant" {
		if msg.Reasoning != "" {
			parts = append(parts, PersistedPart{
				Type:      "reasoning",
				Text:      msg.Reasoning,
				State:     "done",
				StepIndex: 0,
			})
		}
		if msg.Content != "" || len(msg.ToolCalls) == 0 {
			parts = append(parts, PersistedPart{
				Type:      "text",
				Text:      msg.Content,
				State:     "done",
				StepIndex: 0,
			})
		}
		for _, tc := range msg.ToolCalls {
			pp := PersistedPart{
				Type:       "tool-call",
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Args:       tc.Function.Arguments,
				State:      "complete",
				StepIndex:  0,
			}
			if output, ok := toolResults[tc.ID]; ok && output != "" {
				pp.Output = output
			}
			parts = append(parts, pp)
		}
	}

	return parts
}