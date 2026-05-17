package chat_context

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/server/internal/context_builder"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/session"
)

func TestSynthesizeUIMessage_UserMessage(t *testing.T) {
	msg := session.Message{
		ID:      uuid.New(),
		Role:    "user",
		Content: "Hello",
	}
	uiMsg := context_builder.SynthesizeUIMessage(msg, nil)
	require.NotNil(t, uiMsg)
	assert.Equal(t, "user", uiMsg.Role)
	assert.Equal(t, msg.ID.String(), uiMsg.ID)
	require.Len(t, uiMsg.Steps, 1, "user message should have one step")
	require.Len(t, uiMsg.Steps[0].Parts, 1)
	assert.Equal(t, entity.UIPartText, uiMsg.Steps[0].Parts[0].Type)
	assert.Equal(t, "Hello", uiMsg.Steps[0].Parts[0].Text)
	assert.Equal(t, entity.UIPartStateDone, uiMsg.Steps[0].Parts[0].State)
	assert.Equal(t, 0, uiMsg.Steps[0].Parts[0].StepIndex)
}

func TestSynthesizeUIMessage_AssistantWithToolCalls(t *testing.T) {
	msg := session.Message{
		ID:        uuid.New(),
		Role:      "assistant",
		Reasoning: "Thinking...",
		Content:   "Let me check.",
		ToolCalls: session.ToolCalls{
			{ID: "call_1", Type: "function", Function: session.FunctionCall{Name: "bash", Arguments: `{"cmd":"ls"}`}},
		},
	}
	toolResults := map[string]string{"call_1": "file.txt"}
	uiMsg := context_builder.SynthesizeUIMessage(msg, toolResults)
	require.NotNil(t, uiMsg)
	assert.Equal(t, "assistant", uiMsg.Role)
	require.Len(t, uiMsg.Steps, 1, "legacy assistant message should have one step (StepIndex=0)")

	parts := uiMsg.Steps[0].Parts
	require.Len(t, parts, 3)
	assert.Equal(t, entity.UIPartReasoning, parts[0].Type)
	assert.Equal(t, "Thinking...", parts[0].Text)
	assert.Equal(t, 0, parts[0].StepIndex)

	assert.Equal(t, entity.UIPartText, parts[1].Type)
	assert.Equal(t, "Let me check.", parts[1].Text)
	assert.Equal(t, 0, parts[1].StepIndex)

	assert.Equal(t, entity.UIPartToolCall, parts[2].Type)
	assert.Equal(t, "call_1", parts[2].ToolCallID)
	assert.Equal(t, "bash", parts[2].ToolName)
	assert.Equal(t, "file.txt", parts[2].Output)
	assert.Equal(t, 0, parts[2].StepIndex)
}

func TestSynthesizeUIMessage_AssistantToolCallOnly(t *testing.T) {
	msg := session.Message{
		ID:   uuid.New(),
		Role: "assistant",
		ToolCalls: session.ToolCalls{
			{ID: "call_2", Type: "function", Function: session.FunctionCall{Name: "python", Arguments: `{"code":"1+1"}`}},
		},
	}
	uiMsg := context_builder.SynthesizeUIMessage(msg, nil)
	require.NotNil(t, uiMsg)
	require.Len(t, uiMsg.Steps, 1)
	require.Len(t, uiMsg.Steps[0].Parts, 1)
	assert.Equal(t, entity.UIPartToolCall, uiMsg.Steps[0].Parts[0].Type)
	assert.Equal(t, 0, uiMsg.Steps[0].Parts[0].StepIndex)
}

func TestSynthesizeUIMessage_UnsupportedRole(t *testing.T) {
	msg := session.Message{
		ID:      uuid.New(),
		Role:    "system",
		Content: "You are helpful.",
	}
	uiMsg := context_builder.SynthesizeUIMessage(msg, nil)
	assert.Nil(t, uiMsg, "unsupported roles should return nil")
}

func TestGroupPartsByStep_SingleStep(t *testing.T) {
	parts := []entity.UIPart{
		{Type: entity.UIPartText, Text: "Hello", StepIndex: 0},
		{Type: entity.UIPartToolCall, ToolCallID: "c1", StepIndex: 0},
	}
	steps := context_builder.GroupPartsByStep(parts)
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
	steps := context_builder.GroupPartsByStep(parts)
	require.Len(t, steps, 2)
	require.Len(t, steps[0].Parts, 1)
	assert.Equal(t, "Step 0", steps[0].Parts[0].Text)
	require.Len(t, steps[1].Parts, 2)
}

func TestGroupPartsByStep_Empty(t *testing.T) {
	steps := context_builder.GroupPartsByStep(nil)
	assert.Len(t, steps, 0)
}

func TestConvertDBMessagesToUI_LegacyData(t *testing.T) {
	sessionID := uuid.New()
	userMsg := session.Message{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      "user",
		Content:   "Hi",
	}
	assistantMsg := session.Message{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      "assistant",
		Content:   "Hello!",
	}

	uiMessages, hasFallback := convertDBMessagesToUI(
		[]session.Message{userMsg, assistantMsg},
		nil,
	)
	assert.False(t, hasFallback, "legacy user/assistant messages should not cause fallback")
	require.Len(t, uiMessages, 2)

	assert.Equal(t, "user", uiMessages[0].Role)
	require.Len(t, uiMessages[0].Steps, 1)
	assert.Equal(t, "Hi", uiMessages[0].Steps[0].Parts[0].Text)

	assert.Equal(t, "assistant", uiMessages[1].Role)
	require.Len(t, uiMessages[1].Steps, 1)
	assert.Equal(t, "Hello!", uiMessages[1].Steps[0].Parts[0].Text)
}

func TestConvertDBMessagesToUI_WithParts(t *testing.T) {
	sessionID := uuid.New()
	msg := session.Message{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      "assistant",
		Parts: session.PersistedParts{
			{Type: "text", Text: "Result", State: "done", StepIndex: 0},
			{Type: "tool-call", ToolCallID: "c1", ToolName: "bash", State: "done", StepIndex: 0},
		},
	}
	toolResults := map[string]string{"c1": "output"}

	uiMessages, hasFallback := convertDBMessagesToUI([]session.Message{msg}, toolResults)
	assert.False(t, hasFallback)
	require.Len(t, uiMessages, 1)
	require.Len(t, uiMessages[0].Steps, 1)
	require.Len(t, uiMessages[0].Steps[0].Parts, 2)
	assert.Equal(t, "output", uiMessages[0].Steps[0].Parts[1].Output)
}

func TestConvertDBMessagesToUI_ToolRoleSkipped(t *testing.T) {
	sessionID := uuid.New()
	toolMsg := session.Message{
		ID:         uuid.New(),
		SessionID:  sessionID,
		Role:       "tool",
		ToolCallID: "c1",
		Content:    "result",
	}

	uiMessages, hasFallback := convertDBMessagesToUI([]session.Message{toolMsg}, nil)
	assert.False(t, hasFallback)
	assert.Len(t, uiMessages, 0, "tool-role messages should be skipped")
}

func TestConvertDBMessagesToUI_MixedLegacyAndNew(t *testing.T) {
	sessionID := uuid.New()
	userMsg := session.Message{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      "user",
		Content:   "Old format",
	}
	newAssistantMsg := session.Message{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      "assistant",
		Parts: session.PersistedParts{
			{Type: "text", Text: "New format", State: "done", StepIndex: 0},
		},
	}

	uiMessages, hasFallback := convertDBMessagesToUI(
		[]session.Message{userMsg, newAssistantMsg},
		nil,
	)
	assert.False(t, hasFallback)
	require.Len(t, uiMessages, 2)

	assert.Equal(t, "user", uiMessages[0].Role)
	require.Len(t, uiMessages[0].Steps, 1)

	assert.Equal(t, "assistant", uiMessages[1].Role)
	require.Len(t, uiMessages[1].Steps, 1)
	assert.Equal(t, "New format", uiMessages[1].Steps[0].Parts[0].Text)
}
