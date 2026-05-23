package session

import (
	"github.com/google/uuid"

	"github.com/copcon/core/storage"
)

func SessionToStorage(s *Session) *storage.Session {
	if s == nil {
		return nil
	}
	return &storage.Session{
		ID:              s.ID,
		Title:           s.Title,
		DefaultAgentID:  s.DefaultAgentID,
		ParentSessionID: s.ParentSessionID,
		Metadata:        map[string]any(s.Metadata),
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
}

func SessionFromStorage(s *storage.Session) *Session {
	if s == nil {
		return nil
	}
	return &Session{
		ID:              s.ID,
		Title:           s.Title,
		DefaultAgentID:  s.DefaultAgentID,
		ParentSessionID: s.ParentSessionID,
		Metadata:        JSONB(s.Metadata),
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
}

func MessageToStorage(m *Message) *storage.Message {
	if m == nil {
		return nil
	}
	return &storage.Message{
		ID:         m.ID,
		SessionID:  m.SessionID,
		Role:       m.Role,
		Content:    m.Content,
		Reasoning:  m.Reasoning,
		ToolCalls:  toolCallsToStorage(m.ToolCalls),
		ToolCallID: m.ToolCallID,
		Parts:      partsToStorage(m.Parts),
		Model:      m.Model,
		TokenCount: m.TokenCount,
		DurationMs: m.DurationMs,
		CreatedAt:  m.CreatedAt,
	}
}

func MessageFromStorage(m *storage.Message) *Message {
	if m == nil {
		return nil
	}
	return &Message{
		ID:         m.ID,
		SessionID:  m.SessionID,
		Role:       m.Role,
		Content:    m.Content,
		Reasoning:  m.Reasoning,
		ToolCalls:  toolCallsFromStorage(m.ToolCalls),
		ToolCallID: m.ToolCallID,
		Parts:      partsFromStorage(m.Parts),
		Model:      m.Model,
		TokenCount: m.TokenCount,
		DurationMs: m.DurationMs,
		CreatedAt:  m.CreatedAt,
	}
}

func toolCallsToStorage(tcs ToolCalls) []storage.ToolCall {
	if tcs == nil {
		return nil
	}
	result := make([]storage.ToolCall, len(tcs))
	for i, tc := range tcs {
		result[i] = storage.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: storage.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return result
}

func toolCallsFromStorage(tcs []storage.ToolCall) ToolCalls {
	if tcs == nil {
		return nil
	}
	result := make(ToolCalls, len(tcs))
	for i, tc := range tcs {
		result[i] = ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return result
}

func partsToStorage(pp PersistedParts) []storage.Part {
	if pp == nil {
		return nil
	}
	result := make([]storage.Part, len(pp))
	for i, p := range pp {
		result[i] = storage.Part{
			Type:       p.Type,
			Text:       p.Text,
			State:      p.State,
			ToolCallID: p.ToolCallID,
			ToolName:   p.ToolName,
			Args:       p.Args,
			Output:     p.Output,
			Error:      p.Error,
			Interrupt:  p.Interrupt,
			StepIndex:  p.StepIndex,
		}
	}
	return result
}

func partsFromStorage(pp []storage.Part) PersistedParts {
	if pp == nil {
		return nil
	}
	result := make(PersistedParts, len(pp))
	for i, p := range pp {
		result[i] = PersistedPart{
			Type:       p.Type,
			Text:       p.Text,
			State:      p.State,
			ToolCallID: p.ToolCallID,
			ToolName:   p.ToolName,
			Args:       p.Args,
			Output:     p.Output,
			Error:      p.Error,
			Interrupt:  interruptFromStorage(p.Interrupt),
			StepIndex:  p.StepIndex,
		}
	}
	return result
}

func interruptFromStorage(v any) map[string]any {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

func TodoToStorage(t *Todo) *storage.Todo {
	if t == nil {
		return nil
	}
	return &storage.Todo{
		ID:          t.ID,
		SessionID:   t.SessionID,
		Content:     t.Content,
		ActiveForm:  t.ActiveForm,
		Status:      storage.TodoStatus(t.Status),
		Priority:    "",
		DependsOn:   []uuid.UUID(t.DependsOn),
		Validation:  t.Validation,
		Result:      t.Result,
		RetryCount:  t.RetryCount,
		CompletedAt: t.CompletedAt,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func TodoFromStorage(t *storage.Todo) *Todo {
	if t == nil {
		return nil
	}
	return &Todo{
		ID:          t.ID,
		SessionID:   t.SessionID,
		Content:     t.Content,
		ActiveForm:  t.ActiveForm,
		Status:      TodoStatus(t.Status),
		DependsOn:   UUIDArray(t.DependsOn),
		Validation:  t.Validation,
		Result:      t.Result,
		RetryCount:  t.RetryCount,
		CompletedAt: t.CompletedAt,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}
