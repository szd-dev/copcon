package iface

import (
	"context"
	"log/slog"

	"github.com/copcon/server/internal/domain/entity"
)

type ChatContextInterface interface {
	Context() context.Context
	SessionID() string
	AgentID() string
	Events() <-chan entity.Event
	Emit(event entity.Event)
}

type ChatContext struct {
	ctx       context.Context
	sessionID string
	agentID   string
	events    chan entity.Event
}

func (c *ChatContext) Context() context.Context    { return c.ctx }
func (c *ChatContext) SessionID() string           { return c.sessionID }
func (c *ChatContext) AgentID() string             { return c.agentID }
func (c *ChatContext) Events() <-chan entity.Event { return c.events }

func (c *ChatContext) Emit(event entity.Event) {
	select {
	case c.events <- event:
		// Event sent successfully
	default:
		// Channel would block - log warning then block with context check
		slog.Warn("SSE event channel near capacity, blocking emit", "event_type", event.Type)
		select {
		case c.events <- event:
		case <-c.ctx.Done():
		}
	}
}

func (c *ChatContext) Close() {
	close(c.events)
}

func NewChatContext(ctx context.Context, sessionID, agentID string) *ChatContext {
	return &ChatContext{
		ctx:       ctx,
		sessionID: sessionID,
		agentID:   agentID,
		events:    make(chan entity.Event, 256),
	}
}
