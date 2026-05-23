package iface

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/storage"
)

type ChatContextInterface interface {
	Context() context.Context
	SessionID() string
	AgentID() string
	Events() <-chan entity.Event
	Emit(event entity.Event)
	Close()
	Closed() <-chan struct{}
	Depth() int
	Subscribe(fromSeq int64) (*Subscriber, bool)
	RequestInput(req InputRequest) (*InputResponse, error)
	ResolveInput(interruptID string, resp *InputResponse) error
	PendingInputs() []InputRequest
	SetPartLocator(messageID string, stepIndex, partIndex int)
	ClearPartLocator()
}

// Storer removes a session from a store.
type Storer interface {
	Remove(sessionID string)
}

// Subscriber receives a filtered view of events emitted on a ChatContext.
type Subscriber struct {
	Events <-chan entity.Event
}

type InterruptType string

const (
	InterruptApproval InterruptType = "approval"
	InterruptQuestion InterruptType = "question"
)

// InputRequest represents a request for human input during an agent execution.
type InputRequest struct {
	ID          string         `json:"id"`
	Type        InterruptType  `json:"type"`
	Message     string         `json:"message"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	ToolArgs    map[string]any `json:"tool_args,omitempty"`
}

// InputResponse represents a human's response to an InputRequest.
type InputResponse struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

var ErrInterruptNotFound = errors.New("interrupt not found")

// SessionCreateOption configures session creation.
type SessionCreateOption func(*storage.Session)

// WithParentSessionID sets the parent session for the new session.
func WithParentSessionID(id uuid.UUID) SessionCreateOption {
	return func(s *storage.Session) {
		s.ParentSessionID = &id
	}
}

// SessionManager provides session lifecycle operations for the agent engine.
// It uses pure value types from the storage package (no GORM dependencies).
type SessionManager interface {
	GetSession(chatCtx ChatContextInterface) (*storage.Session, error)
	CreateSession(chatCtx ChatContextInterface, title, defaultAgentID string, opts ...SessionCreateOption) (*storage.Session, error)
	AddAsyncCompletionPending(chatCtx ChatContextInterface, event map[string]any) error
}

// ContextManager provides message and context operations for the agent engine.
// It uses pure value types from the storage package (no GORM dependencies).
type ContextManager interface {
	AddMessage(chatCtx ChatContextInterface, msg *storage.Message) error
	UpdateMessage(chatCtx ChatContextInterface, msg *storage.Message) error
	UpsertMessage(chatCtx ChatContextInterface, msg *storage.Message) error
	GetHistory(chatCtx ChatContextInterface, limit int) ([]*storage.Message, error)
	BuildContext(chatCtx ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]entity.MessageForLLM, error)
}