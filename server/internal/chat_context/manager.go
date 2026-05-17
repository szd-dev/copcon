package chat_context

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tools/todo"
)

var (
	ErrContextTooLong = errors.New("context exceeds maximum token limit")
)

type ContextManager interface {
	GetHistory(chatCtx iface.ChatContextInterface, limit int) ([]session.Message, error)
	AddMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error
	BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]MessageForLLM, error)
	DeleteBySession(chatCtx iface.ChatContextInterface) error
}

type MessageForLLM struct {
	Role       string             `json:"role"`
	Content    string             `json:"content"`
	Reasoning  string             `json:"reasoning,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	ToolCalls  []session.ToolCall `json:"tool_calls,omitempty"`
}

type contextManager struct {
	db      *gorm.DB
	todoMgr todo.TodoManager
}

func NewContextManager(db *gorm.DB, todoMgr todo.TodoManager) ContextManager {
	return &contextManager{db: db, todoMgr: todoMgr}
}

func (m *contextManager) GetHistory(chatCtx iface.ChatContextInterface, limit int) ([]session.Message, error) {
	var messages []session.Message

	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	query := m.db.WithContext(chatCtx.Context()).
		Where("session_id = ?", sessionUUID).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}

	return messages, nil
}

func (m *contextManager) AddMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error {
	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}

	if msg.ID == uuid.Nil {
		msg.ID = uuid.New()
	}
	msg.SessionID = sessionUUID

	return m.db.WithContext(chatCtx.Context()).Create(msg).Error
}

func (m *contextManager) BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]MessageForLLM, error) {
	messages := make([]MessageForLLM, 0)

	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI assistant with access to tools for code execution, file operations, and shell commands. Use these tools when appropriate to help the user."
	}

	if m.todoMgr != nil {
		todos, err := m.todoMgr.List(chatCtx)
		if err != nil {
			log.Printf("Warning: failed to fetch todos: %v", err)
		} else if len(todos) > 0 {
			todoState := formatTodoState(todos)
			systemPrompt = systemPrompt + "\n\n" + todoState
		}
	}

	messages = append(messages, MessageForLLM{
		Role:    "system",
		Content: systemPrompt,
	})

	history, err := m.GetHistory(chatCtx, 1024)
	if err != nil {
		return nil, err
	}

	// Build a lookup of tool results by ToolCallID so we can populate
	// output in tool-call parts when converting from Parts JSONB.
	toolResultByCallID := buildToolResultLookup(history)

	// Try the UIMessage path for messages that have Parts populated with
	// complete tool-call output. Fall back to direct MessageForLLM for legacy data.
	uiMessages, hasFallback := convertDBMessagesToUI(history, toolResultByCallID)
	if !hasFallback {
		modelMessages := entity.ConvertToModelMessages(uiMessages)
		for _, mm := range modelMessages {
			messages = append(messages, MessageForLLM{
				Role:       mm.Role,
				Content:    mm.Content,
				ToolCallID: mm.ToolCallID,
				ToolCalls:  convertModelToolCalls(mm.ToolCalls),
			})
		}
	} else {
		// Legacy path: direct DB → MessageForLLM conversion.
		// This works correctly now because convertMessages() includes ToolCalls.
		for _, msg := range history {
			messages = append(messages, MessageForLLM{
				Role:       msg.Role,
				Content:    msg.Content,
				Reasoning:  msg.Reasoning,
				ToolCallID: msg.ToolCallID,
				ToolCalls:  msg.ToolCalls,
			})
		}
	}

	if userInput != "" {
		messages = append(messages, MessageForLLM{
			Role:    "user",
			Content: userInput,
		})
	}

	return messages, nil
}

// buildToolResultLookup creates a map from tool call ID to the tool result content.
func buildToolResultLookup(history []session.Message) map[string]string {
	lookup := make(map[string]string)
	for _, msg := range history {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			lookup[msg.ToolCallID] = msg.Content
		}
	}
	return lookup
}

// convertDBMessagesToUI converts database Message records to UIMessages.
// Returns the UIMessages and a boolean indicating whether any messages
// need to fall back to the legacy path (no Parts available).
// Tool-role messages are excluded since ConvertToModelMessages generates them
// from tool-call parts with output populated from toolResultByCallID.
func convertDBMessagesToUI(history []session.Message, toolResultByCallID map[string]string) ([]entity.UIMessage, bool) {
	var uiMessages []entity.UIMessage
	hasFallback := false

	for _, msg := range history {
		if msg.Role == "tool" {
			continue
		}

		if len(msg.Parts) > 0 {
			uiParts := convertDBPartsToUIPart(msg.Parts, msg.ID.String(), toolResultByCallID)
			steps := groupPartsByStep(uiParts)
			uiMessages = append(uiMessages, entity.UIMessage{
				ID:    msg.ID.String(),
				Role:  msg.Role,
				Steps: steps,
			})
		} else {
			uiMsg := synthesizeUIMessage(msg, toolResultByCallID)
			if uiMsg != nil {
				uiMessages = append(uiMessages, *uiMsg)
			} else {
				hasFallback = true
			}
		}
	}

	return uiMessages, hasFallback
}

// convertDBPartsToUIPart converts raw Parts JSONB to typed UIPart slice.
// Uses toolResultByCallID to populate output for tool-call parts that don't have output stored.
func convertDBPartsToUIPart(parts session.PersistedParts, messageID string, toolResultByCallID map[string]string) []entity.UIPart {
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

// groupPartsByStep groups UIParts by StepIndex into UIStep objects.
func groupPartsByStep(parts []entity.UIPart) []entity.UIStep {
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

// synthesizeUIMessage creates a UIMessage from legacy Content/Reasoning/ToolCalls fields.
// Returns nil for unsupported roles.
func synthesizeUIMessage(msg session.Message, toolResultByCallID map[string]string) *entity.UIMessage {
	switch msg.Role {
	case "user":
		parts := []entity.UIPart{
			{Type: entity.UIPartText, Text: msg.Content, State: entity.UIPartStateDone, StepIndex: 0},
		}
		return &entity.UIMessage{
			ID:    msg.ID.String(),
			Role:  "user",
			Steps: groupPartsByStep(parts),
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
			Steps: groupPartsByStep(parts),
		}
	default:
		return nil
	}
}

// convertModelToolCalls converts entity ModelToolCalls to session ToolCalls.
func convertModelToolCalls(toolCalls []entity.ModelToolCall) []session.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]session.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		result[i] = session.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: session.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return result
}

func (m *contextManager) DeleteBySession(chatCtx iface.ChatContextInterface) error {
	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}

	return m.db.WithContext(chatCtx.Context()).
		Where("session_id = ?", sessionUUID).
		Delete(&session.Message{}).Error
}

func EstimateTokens(content string) int {
	return len(content) / 4
}

func formatTodoState(todos []*session.Todo) string {
	var pending, inProgress, completed, failed, blocked []string

	for _, t := range todos {
		content := t.Content
		if t.ActiveForm != "" {
			content = t.ActiveForm
		}
		switch t.Status {
		case session.TodoStatusPending:
			pending = append(pending, content)
		case session.TodoStatusInProgress:
			inProgress = append(inProgress, content)
		case session.TodoStatusCompleted:
			completed = append(completed, content)
		case session.TodoStatusFailed:
			failed = append(failed, content)
		case session.TodoStatusBlocked:
			blocked = append(blocked, content)
		}
	}

	var parts []string
	if len(pending) > 0 {
		parts = append(parts, "pending: "+strings.Join(pending, ", "))
	}
	if len(inProgress) > 0 {
		parts = append(parts, "in_progress: "+strings.Join(inProgress, ", "))
	}
	if len(completed) > 0 {
		parts = append(parts, "completed: "+strings.Join(completed, ", "))
	}
	if len(failed) > 0 {
		parts = append(parts, "failed: "+strings.Join(failed, ", "))
	}
	if len(blocked) > 0 {
		parts = append(parts, "blocked: "+strings.Join(blocked, ", "))
	}

	return "Current todo list: [" + strings.Join(parts, ", ") + "]"
}
