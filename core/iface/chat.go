package iface

import (
	"context"
	"errors"

	"github.com/copcon/core/entity"
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