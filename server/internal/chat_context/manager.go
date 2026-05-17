package chat_context

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/context_builder"
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
	BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]entity.MessageForLLM, error)
	DeleteBySession(chatCtx iface.ChatContextInterface) error
}

type contextManager struct {
	db         *gorm.DB
	todoMgr    todo.TodoManager
	ctxBuilder context_builder.ContextBuilder
	logger     *slog.Logger
}

func NewContextManager(db *gorm.DB, todoMgr todo.TodoManager, ctxBuilder context_builder.ContextBuilder, logger *slog.Logger) ContextManager {
	return &contextManager{db: db, todoMgr: todoMgr, ctxBuilder: ctxBuilder, logger: logger}
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

func (m *contextManager) BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]entity.MessageForLLM, error) {
	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI assistant with access to tools for code execution, file operations, and shell commands. Use these tools when appropriate to help the user."
	}

	if m.todoMgr != nil {
		todos, err := m.todoMgr.List(chatCtx)
		if err != nil {
			m.logger.Warn("failed to fetch todos", "session_id", chatCtx.SessionID(), "error", err)
		} else if len(todos) > 0 {
			todoState := formatTodoState(todos)
			systemPrompt = systemPrompt + "\n\n" + todoState
		}
	}

	history, err := m.GetHistory(chatCtx, 1024)
	if err != nil {
		return nil, err
	}

	toolResultByCallID := buildToolResultLookup(history)

	uiMessages, hasFallback := convertDBMessagesToUI(history, toolResultByCallID)
	if !hasFallback {
		return m.ctxBuilder.Build(chatCtx.Context(), uiMessages, systemPrompt, userInput)
	}

	// Legacy fallback: direct DB → MessageForLLM conversion
	messages := make([]entity.MessageForLLM, 0)

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
			ToolCalls:  context_builder.ConvertSessionToolCalls(msg.ToolCalls),
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
			steps := context_builder.GroupPartsByStep(uiParts)
			uiMessages = append(uiMessages, entity.UIMessage{
				ID:    msg.ID.String(),
				Role:  msg.Role,
				Steps: steps,
			})
		} else {
			uiMsg := context_builder.SynthesizeUIMessage(msg, toolResultByCallID)
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
