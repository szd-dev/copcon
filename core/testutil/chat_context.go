package testutil

import (
	"context"
	"log/slog"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
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
		slog.Warn("SSE near capacity, blocking emit", "event_type", event.Type)
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

func (m *MockChatContext) Closed() <-chan struct{} {
	closed := make(chan struct{})
	close(closed)
	return closed
}

func (m *MockChatContext) Depth() int {
	return 0
}

func (m *MockChatContext) Subscribe(fromSeq int64) (*iface.Subscriber, bool) {
	return nil, false
}

func (m *MockChatContext) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
	return nil, context.Canceled
}

func (m *MockChatContext) ResolveInput(interruptID string, resp *iface.InputResponse) error {
	return iface.ErrInterruptNotFound
}

func (m *MockChatContext) PendingInputs() []iface.InputRequest {
	return nil
}

func (m *MockChatContext) SetPartLocator(messageID string, stepIndex, partIndex int) {}
func (m *MockChatContext) ClearPartLocator()                                         {}

// Ensure MockChatContext implements iface.ChatContextInterface
var _ iface.ChatContextInterface = (*MockChatContext)(nil)
