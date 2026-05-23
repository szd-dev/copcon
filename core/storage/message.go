package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ToolCall represents a single tool invocation within a message.
type ToolCall struct {
	ID       string
	Type     string
	Function FunctionCall
}

// FunctionCall describes the function name and arguments for a tool call.
type FunctionCall struct {
	Name      string
	Arguments string
}

// Part represents a single message part (text, tool-call, etc.).
type Part struct {
	Type       string
	Text       string
	State      string
	ToolCallID string
	ToolName   string
	Args       string
	Output     string
	Error      string
	Interrupt  any
	StepIndex  int
}

// Message is a pure value type with no GORM annotations.
type Message struct {
	ID         uuid.UUID
	SessionID  uuid.UUID
	Role       string
	Content    string
	Reasoning  string
	ToolCalls  []ToolCall
	ToolCallID string
	Parts      []Part
	Model      string
	TokenCount int
	DurationMs int64
	CreatedAt  time.Time
}

// MessageStore persists messages.
type MessageStore interface {
	List(ctx context.Context, sessionID uuid.UUID, limit int) ([]*Message, error)
	Add(ctx context.Context, message *Message) error
	Update(ctx context.Context, message *Message) error
	Upsert(ctx context.Context, message *Message) error
	DeleteBySession(ctx context.Context, sessionID uuid.UUID) error
}