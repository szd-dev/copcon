package testutil

import (
	"context"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
)

type MockChatContext struct {
	Ctx       context.Context
	Agent     string
	Session   string
}

func (m *MockChatContext) Context() context.Context {
	if m.Ctx != nil {
		return m.Ctx
	}
	return context.Background()
}

func (m *MockChatContext) SessionID() string  { return m.Session }
func (m *MockChatContext) AgentID() string    { return m.Agent }

func (m *MockChatContext) Events() <-chan entity.Event       { return nil }
func (m *MockChatContext) Emit(event entity.Event)           {}
func (m *MockChatContext) Close()                            {}
func (m *MockChatContext) Closed() <-chan struct{}           { return nil }
func (m *MockChatContext) Depth() int                        { return 0 }

func (m *MockChatContext) Subscribe(fromSeq int64) (*iface.Subscriber, bool) {
	return nil, false
}

func (m *MockChatContext) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
	return nil, nil
}

func (m *MockChatContext) ResolveInput(id string, resp *iface.InputResponse) error {
	return nil
}

func (m *MockChatContext) PendingInputs() []iface.InputRequest { return nil }
func (m *MockChatContext) SetPartLocator(string, int, int)     {}
func (m *MockChatContext) ClearPartLocator()                   {}