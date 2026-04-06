package chat_context

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

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

	// Use provided system prompt or default
	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI assistant with access to tools for code execution, file operations, and shell commands. Use these tools when appropriate to help the user."
	}

	// Inject todo state into system prompt if todoMgr is available
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

	for _, msg := range history {
		messages = append(messages, MessageForLLM{
			Role:       msg.Role,
			Content:    msg.Content,
			Reasoning:  msg.Reasoning,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  msg.ToolCalls,
		})
	}

	// Only append userInput if it's not empty
	// (In the agent loop, userInput is already added to history before BuildContext is called)
	if userInput != "" {
		messages = append(messages, MessageForLLM{
			Role:    "user",
			Content: userInput,
		})
	}

	return messages, nil
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
