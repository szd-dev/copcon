package testutil

import (
	"context"
	"log"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
)

// MockChatContext is a mock implementation of ChatContextInterface for testing
type MockChatContext struct {
	ctx       context.Context
	sessionID string
	agentID   string
	events    chan entity.Event
}

// NewMockChatContext creates a new MockChatContext for testing
func NewMockChatContext(ctx context.Context, sessionID, agentID string) *MockChatContext {
	return &MockChatContext{
		ctx:       ctx,
		sessionID: sessionID,
		agentID:   agentID,
		events:    make(chan entity.Event, 256),
	}
}

func (m *MockChatContext) Context() context.Context    { return m.ctx }
func (m *MockChatContext) SessionID() string           { return m.sessionID }
func (m *MockChatContext) AgentID() string             { return m.agentID }
func (m *MockChatContext) Events() <-chan entity.Event { return m.events }
func (m *MockChatContext) Emit(event entity.Event) {
	select {
	case m.events <- event:
	default:
		log.Printf("WARNING: SSE event channel near capacity, event type=%s", event.Type)
		select {
		case m.events <- event:
		case <-m.ctx.Done():
		}
	}
}

// Close closes the events channel
func (m *MockChatContext) Close() {
	close(m.events)
}

// Ensure MockChatContext implements iface.ChatContextInterface
var _ iface.ChatContextInterface = (*MockChatContext)(nil)
