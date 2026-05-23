package context_builder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/entity"
)

func TestBuild_EmptyInput(t *testing.T) {
	b := New()
	result, err := b.Build(context.Background(), nil, "", "")
	require.NoError(t, err)
	assert.Len(t, result, 0)
}

func TestBuild_SystemPromptOnly(t *testing.T) {
	b := New()
	result, err := b.Build(context.Background(), nil, "You are helpful.", "")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "system", result[0].Role)
	assert.Equal(t, "You are helpful.", result[0].Content)
}

func TestBuild_UserMessageOnly(t *testing.T) {
	b := New()
	uiMessages := []entity.UIMessage{
		{
			ID:   "msg1",
			Role: "user",
			Steps: []entity.UIStep{
				{Parts: []entity.UIPart{
					{Type: entity.UIPartText, Text: "Hello", State: entity.UIPartStateDone, StepIndex: 0},
				}, State: entity.UIPartStateDone},
			},
		},
	}
	result, err := b.Build(context.Background(), uiMessages, "", "")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "user", result[0].Role)
	assert.Equal(t, "Hello", result[0].Content)
}

func TestBuild_SystemAndHistory(t *testing.T) {
	b := New()
	uiMessages := []entity.UIMessage{
		{
			ID:   "msg1",
			Role: "user",
			Steps: []entity.UIStep{
				{Parts: []entity.UIPart{
					{Type: entity.UIPartText, Text: "Question", State: entity.UIPartStateDone, StepIndex: 0},
				}, State: entity.UIPartStateDone},
			},
		},
		{
			ID:   "msg2",
			Role: "assistant",
			Steps: []entity.UIStep{
				{Parts: []entity.UIPart{
					{Type: entity.UIPartText, Text: "Answer", State: entity.UIPartStateDone, StepIndex: 0},
				}, State: entity.UIPartStateDone},
			},
		},
	}
	result, err := b.Build(context.Background(), uiMessages, "System prompt", "")
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "system", result[0].Role)
	assert.Equal(t, "System prompt", result[0].Content)
	assert.Equal(t, "user", result[1].Role)
	assert.Equal(t, "Question", result[1].Content)
	assert.Equal(t, "assistant", result[2].Role)
	assert.Equal(t, "Answer", result[2].Content)
}

func TestBuild_WithUserInput(t *testing.T) {
	b := New()
	uiMessages := []entity.UIMessage{
		{
			ID:   "msg1",
			Role: "user",
			Steps: []entity.UIStep{
				{Parts: []entity.UIPart{
					{Type: entity.UIPartText, Text: "Previous", State: entity.UIPartStateDone, StepIndex: 0},
				}, State: entity.UIPartStateDone},
			},
		},
	}
	result, err := b.Build(context.Background(), uiMessages, "System", "New input")
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "system", result[0].Role)
	assert.Equal(t, "user", result[1].Role)
	assert.Equal(t, "Previous", result[1].Content)
	assert.Equal(t, "user", result[2].Role)
	assert.Equal(t, "New input", result[2].Content)
}

func TestBuild_AssistantWithToolCalls(t *testing.T) {
	b := New()
	uiMessages := []entity.UIMessage{
		{
			ID:   "msg1",
			Role: "assistant",
			Steps: []entity.UIStep{
				{Parts: []entity.UIPart{
					{Type: entity.UIPartText, Text: "Let me run that.", State: entity.UIPartStateDone, StepIndex: 0},
					{Type: entity.UIPartToolCall, ToolCallID: "call_1", ToolName: "bash", Args: `{"cmd":"ls"}`, Output: "file.txt", State: entity.UIPartStateDone, StepIndex: 0},
				}, State: entity.UIPartStateDone},
			},
		},
	}
	result, err := b.Build(context.Background(), uiMessages, "", "")
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "assistant", result[0].Role)
	assert.Equal(t, "Let me run that.", result[0].Content)
	require.Len(t, result[0].ToolCalls, 1)
	assert.Equal(t, "call_1", result[0].ToolCalls[0].ID)
	assert.Equal(t, "function", result[0].ToolCalls[0].Type)
	assert.Equal(t, "bash", result[0].ToolCalls[0].Function.Name)
	assert.Equal(t, `{"cmd":"ls"}`, result[0].ToolCalls[0].Function.Arguments)

	assert.Equal(t, "tool", result[1].Role)
	assert.Equal(t, "file.txt", result[1].Content)
	assert.Equal(t, "call_1", result[1].ToolCallID)
}

func TestBuild_ContextCanceled(t *testing.T) {
	b := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := b.Build(ctx, nil, "", "")
	require.NoError(t, err)
	assert.Len(t, result, 0)
}

func TestBuild_MultipleSteps(t *testing.T) {
	b := New()
	uiMessages := []entity.UIMessage{
		{
			ID:   "msg1",
			Role: "assistant",
			Steps: []entity.UIStep{
				{Parts: []entity.UIPart{
					{Type: entity.UIPartText, Text: "Step 1 output", State: entity.UIPartStateDone, StepIndex: 0},
				}, State: entity.UIPartStateDone},
				{Parts: []entity.UIPart{
					{Type: entity.UIPartText, Text: "Step 2 output", State: entity.UIPartStateDone, StepIndex: 1},
				}, State: entity.UIPartStateDone},
			},
		},
	}
	result, err := b.Build(context.Background(), uiMessages, "", "")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Step 1 outputStep 2 output", result[0].Content)
}

func TestBuild_NilMessages(t *testing.T) {
	b := New()
	result, err := b.Build(context.Background(), nil, "", "Hello world")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "user", result[0].Role)
	assert.Equal(t, "Hello world", result[0].Content)
}

func TestSynthesizeUIMessage_UserMessage(t *testing.T) {
	msg := LegacyMessage{
		ID:      uuid.New(),
		Role:    "user",
		Content: "Hello",
	}
	uiMsg := SynthesizeUIMessage(msg, nil)
	require.NotNil(t, uiMsg)
	assert.Equal(t, "user", uiMsg.Role)
	assert.Equal(t, msg.ID.String(), uiMsg.ID)
	require.Len(t, uiMsg.Steps, 1)
	require.Len(t, uiMsg.Steps[0].Parts, 1)
	assert.Equal(t, entity.UIPartText, uiMsg.Steps[0].Parts[0].Type)
	assert.Equal(t, "Hello", uiMsg.Steps[0].Parts[0].Text)
}

func TestSynthesizeUIMessage_AssistantWithToolCalls(t *testing.T) {
	msg := LegacyMessage{
		ID:        uuid.New(),
		Role:      "assistant",
		Reasoning: "Thinking...",
		Content:   "Let me check.",
		ToolCalls: []LegacyToolCall{
			{ID: "call_1", Type: "function", Function: LegacyFunctionCall{Name: "bash", Arguments: `{"cmd":"ls"}`}},
		},
	}
	toolResults := map[string]string{"call_1": "file.txt"}
	uiMsg := SynthesizeUIMessage(msg, toolResults)
	require.NotNil(t, uiMsg)
	assert.Equal(t, "assistant", uiMsg.Role)
	require.Len(t, uiMsg.Steps, 1)

	parts := uiMsg.Steps[0].Parts
	require.Len(t, parts, 3)
	assert.Equal(t, entity.UIPartReasoning, parts[0].Type)
	assert.Equal(t, entity.UIPartText, parts[1].Type)
	assert.Equal(t, entity.UIPartToolCall, parts[2].Type)
	assert.Equal(t, "call_1", parts[2].ToolCallID)
	assert.Equal(t, "file.txt", parts[2].Output)
}

func TestSynthesizeUIMessage_UnsupportedRole(t *testing.T) {
	msg := LegacyMessage{
		ID:      uuid.New(),
		Role:    "system",
		Content: "You are helpful.",
	}
	uiMsg := SynthesizeUIMessage(msg, nil)
	assert.Nil(t, uiMsg)
}

func TestGroupPartsByStep_SingleStep(t *testing.T) {
	parts := []entity.UIPart{
		{Type: entity.UIPartText, Text: "Hello", StepIndex: 0},
		{Type: entity.UIPartToolCall, ToolCallID: "c1", StepIndex: 0},
	}
	steps := GroupPartsByStep(parts)
	require.Len(t, steps, 1)
	assert.Equal(t, entity.UIPartStateDone, steps[0].State)
	require.Len(t, steps[0].Parts, 2)
}

func TestGroupPartsByStep_MultipleSteps(t *testing.T) {
	parts := []entity.UIPart{
		{Type: entity.UIPartText, Text: "Step 0", StepIndex: 0},
		{Type: entity.UIPartText, Text: "Step 1", StepIndex: 1},
		{Type: entity.UIPartToolCall, ToolCallID: "c1", StepIndex: 1},
	}
	steps := GroupPartsByStep(parts)
	require.Len(t, steps, 2)
	require.Len(t, steps[0].Parts, 1)
	assert.Equal(t, "Step 0", steps[0].Parts[0].Text)
	require.Len(t, steps[1].Parts, 2)
}

func TestGroupPartsByStep_Empty(t *testing.T) {
	steps := GroupPartsByStep(nil)
	assert.Len(t, steps, 0)
}

func TestConvertLegacyToolCalls_Empty(t *testing.T) {
	result := ConvertLegacyToolCalls(nil)
	assert.Nil(t, result)

	result = ConvertLegacyToolCalls([]LegacyToolCall{})
	assert.Nil(t, result)
}

func TestConvertLegacyToolCalls_Conversion(t *testing.T) {
	input := []LegacyToolCall{
		{ID: "tc1", Type: "function", Function: LegacyFunctionCall{Name: "bash", Arguments: `{"cmd":"ls"}`}},
		{ID: "tc2", Type: "function", Function: LegacyFunctionCall{Name: "python", Arguments: `{"code":"1+1"}`}},
	}
	result := ConvertLegacyToolCalls(input)
	require.Len(t, result, 2)
	assert.Equal(t, "tc1", result[0].ID)
	assert.Equal(t, "function", result[0].Type)
	assert.Equal(t, "bash", result[0].Function.Name)
	assert.Equal(t, `{"cmd":"ls"}`, result[0].Function.Arguments)
	assert.Equal(t, "tc2", result[1].ID)
	assert.Equal(t, "python", result[1].Function.Name)
}