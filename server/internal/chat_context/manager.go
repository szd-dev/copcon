package chat_context

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/copcon/core/context_builder"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/core/storage"
)

// Compile-time check: contextManager satisfies storage.MessageStore.
var _ storage.MessageStore = (*contextManager)(nil)

var (
	ErrContextTooLong = errors.New("context exceeds maximum token limit")
)

type ContextManager interface {
	GetHistory(chatCtx iface.ChatContextInterface, limit int) ([]session.Message, error)
	AddMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error
	UpdateMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error
	UpsertMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error
	BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]entity.MessageForLLM, error)
	ClearMessages(chatCtx iface.ChatContextInterface) error
}

type contextManager struct {
	db         *gorm.DB
	ctxBuilder context_builder.ContextBuilder
	logger     *slog.Logger
}

func NewContextManager(db *gorm.DB, ctxBuilder context_builder.ContextBuilder, logger *slog.Logger) (ContextManager, storage.MessageStore) {
	cm := &contextManager{db: db, ctxBuilder: ctxBuilder, logger: logger}
	return cm, cm
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

// UpdateMessage updates content, reasoning, parts, tool_calls by primary key.
func (m *contextManager) UpdateMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error {
	result := m.db.WithContext(chatCtx.Context()).
		Model(&session.Message{}).
		Where("id = ? AND session_id = ?", msg.ID, msg.SessionID).
		Updates(map[string]any{
			"content":    msg.Content,
			"reasoning":  msg.Reasoning,
			"parts":      msg.Parts,
			"tool_calls": msg.ToolCalls,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("message not found: %s", msg.ID)
	}
	return nil
}

// UpsertMessage INSERTs or ON CONFLICT UPDATEs content, reasoning, parts, tool_calls.
func (m *contextManager) UpsertMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error {
	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}

	if msg.ID == uuid.Nil {
		msg.ID = uuid.New()
	}
	msg.SessionID = sessionUUID

	result := m.db.WithContext(chatCtx.Context()).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content", "reasoning", "parts", "tool_calls"}),
		}).
		Create(msg)
	return result.Error
}

func (m *contextManager) BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]entity.MessageForLLM, error) {
	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI assistant with access to tools for code execution, file operations, and shell commands. Use these tools when appropriate to help the user."
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
			ToolCalls:  context_builder.ConvertLegacyToolCalls(sessionToolCallsToLegacy(msg.ToolCalls)),
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
			uiMsg := context_builder.SynthesizeUIMessage(sessionMsgToLegacy(msg), toolResultByCallID)
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

func (m *contextManager) ClearMessages(chatCtx iface.ChatContextInterface) error {
	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}

	return m.db.WithContext(chatCtx.Context()).
		Where("session_id = ?", sessionUUID).
		Delete(&session.Message{}).Error
}

// storage.MessageStore interface methods

func (m *contextManager) List(ctx context.Context, sessionID uuid.UUID, limit int) ([]*storage.Message, error) {
	var messages []session.Message

	query := m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}

	result := make([]*storage.Message, len(messages))
	for i := range messages {
		result[i] = session.MessageToStorage(&messages[i])
	}
	return result, nil
}

func (m *contextManager) Add(ctx context.Context, message *storage.Message) error {
	model := session.MessageFromStorage(message)
	if model.ID == uuid.Nil {
		model.ID = uuid.New()
	}
	return m.db.WithContext(ctx).Create(model).Error
}

func (m *contextManager) Update(ctx context.Context, message *storage.Message) error {
	model := session.MessageFromStorage(message)
	result := m.db.WithContext(ctx).
		Model(&session.Message{}).
		Where("id = ? AND session_id = ?", model.ID, model.SessionID).
		Updates(map[string]any{
			"content":    model.Content,
			"reasoning":  model.Reasoning,
			"parts":      model.Parts,
			"tool_calls": model.ToolCalls,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("message not found: %s", model.ID)
	}
	return nil
}

func (m *contextManager) Upsert(ctx context.Context, message *storage.Message) error {
	model := session.MessageFromStorage(message)
	if model.ID == uuid.Nil {
		model.ID = uuid.New()
	}

	result := m.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content", "reasoning", "parts", "tool_calls"}),
		}).
		Create(model)
	return result.Error
}

func (m *contextManager) DeleteBySession(ctx context.Context, sessionID uuid.UUID) error {
	return m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&session.Message{}).Error
}

func EstimateTokens(content string) int {
	return len(content) / 4
}
