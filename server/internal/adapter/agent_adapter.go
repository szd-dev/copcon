package adapter

import (
	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/session"
)

type SessionManagerAdapter struct {
	Inner session.SessionManager
}

func NewSessionManagerAdapter(inner session.SessionManager) *SessionManagerAdapter {
	return &SessionManagerAdapter{Inner: inner}
}

func (a *SessionManagerAdapter) GetSession(chatCtx iface.ChatContextInterface) (*storage.Session, error) {
	sess, err := a.Inner.GetSession(chatCtx)
	if err != nil {
		return nil, err
	}
	return session.SessionToStorage(sess), nil
}

func (a *SessionManagerAdapter) CreateSession(chatCtx iface.ChatContextInterface, title, defaultAgentID string, opts ...iface.SessionCreateOption) (*storage.Session, error) {
	var serverOpts []session.CreateOption

	if len(opts) > 0 {
		tmp := &storage.Session{}
		for _, opt := range opts {
			opt(tmp)
		}
		if tmp.ParentSessionID != nil {
			serverOpts = append(serverOpts, session.WithParentSessionID(*tmp.ParentSessionID))
		}
	}

	sess, err := a.Inner.CreateSession(chatCtx, title, defaultAgentID, serverOpts...)
	if err != nil {
		return nil, err
	}
	return session.SessionToStorage(sess), nil
}

func (a *SessionManagerAdapter) AddAsyncCompletionPending(chatCtx iface.ChatContextInterface, event map[string]any) error {
	return a.Inner.AddAsyncCompletionPending(chatCtx, event)
}

type ContextManagerAdapter struct {
	Inner chat_context.ContextManager
}

func NewContextManagerAdapter(inner chat_context.ContextManager) *ContextManagerAdapter {
	return &ContextManagerAdapter{Inner: inner}
}

func (a *ContextManagerAdapter) AddMessage(chatCtx iface.ChatContextInterface, msg *storage.Message) error {
	return a.Inner.AddMessage(chatCtx, session.MessageFromStorage(msg))
}

func (a *ContextManagerAdapter) UpdateMessage(chatCtx iface.ChatContextInterface, msg *storage.Message) error {
	return a.Inner.UpdateMessage(chatCtx, session.MessageFromStorage(msg))
}

func (a *ContextManagerAdapter) UpsertMessage(chatCtx iface.ChatContextInterface, msg *storage.Message) error {
	return a.Inner.UpsertMessage(chatCtx, session.MessageFromStorage(msg))
}

func (a *ContextManagerAdapter) BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]entity.MessageForLLM, error) {
	return a.Inner.BuildContext(chatCtx, userInput, maxTokens, systemPrompt)
}

func (a *ContextManagerAdapter) GetHistory(chatCtx iface.ChatContextInterface, limit int) ([]*storage.Message, error) {
	messages, err := a.Inner.GetHistory(chatCtx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]*storage.Message, len(messages))
	for i := range messages {
		result[i] = session.MessageToStorage(&messages[i])
	}
	return result, nil
}
