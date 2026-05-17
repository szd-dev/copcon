package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/hook"
	"github.com/copcon/server/internal/memory"
)

// stubMemoryManager implements memory.MemoryManager for testing.
type stubMemoryManager struct {
	searchResults  []*memory.Memory
	searchErr      error
	storedMemories []*memory.Memory
	storeErr       error
}

func (m *stubMemoryManager) Store(chatCtx iface.ChatContextInterface, mem *memory.Memory) error {
	if m.storeErr != nil {
		return m.storeErr
	}
	m.storedMemories = append(m.storedMemories, mem)
	return nil
}

func (m *stubMemoryManager) Search(chatCtx iface.ChatContextInterface, query []float32, limit int) ([]*memory.Memory, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResults, nil
}

func (m *stubMemoryManager) GetBySession(chatCtx iface.ChatContextInterface, limit int) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *stubMemoryManager) DeleteBySession(chatCtx iface.ChatContextInterface) error {
	return nil
}

// stubChatContext is a minimal ChatContextInterface for tests.
type stubChatContext struct {
	ctx       context.Context
	sessionID string
	agentID   string
}

func (s *stubChatContext) Context() context.Context    { return s.ctx }
func (s *stubChatContext) SessionID() string           { return s.sessionID }
func (s *stubChatContext) AgentID() string             { return s.agentID }
func (s *stubChatContext) Events() <-chan entity.Event { return nil }
func (s *stubChatContext) Emit(_ entity.Event)         {}

func TestMemoryPlugin_Name(t *testing.T) {
	p := NewMemoryPlugin(&stubMemoryManager{})
	assert.Equal(t, "memory_plugin", p.Name())
}

func TestMemoryPlugin_Points(t *testing.T) {
	p := NewMemoryPlugin(&stubMemoryManager{})
	assert.Equal(t, []hook.HookPoint{hook.AfterContextBuild, hook.OnMessagePersist}, p.Points())
}

func TestMemoryPlugin_Priority(t *testing.T) {
	p := NewMemoryPlugin(&stubMemoryManager{})
	assert.Equal(t, 100, p.Priority())
}

// TestMemoryPlugin_NilManagerNoOp verifies that the plugin is a no-op
// when the memory manager is nil.
func TestMemoryPlugin_NilManagerNoOp(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s1", agentID: "a1"}

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "hello"},
	}

	p := NewMemoryPlugin(nil)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s1",
		AgentID:      "a1",
		Messages:     &messages,
		CurrentPoint: hook.AfterContextBuild,
	})
	assert.NoError(t, err)
	// Messages should be unchanged
	assert.Len(t, messages, 1)
}

// TestMemoryPlugin_AfterContextBuild_InjectsMemories verifies that search
// results are formatted as a system message and prepended to the context.
func TestMemoryPlugin_AfterContextBuild_InjectsMemories(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s2", agentID: "a2"}

	mgr := &stubMemoryManager{
		searchResults: []*memory.Memory{
			{Content: "User asked about Go generics last time"},
			{Content: "User prefers concise error messages"},
		},
	}

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "Tell me about generics"},
		{Role: "assistant", Content: "Sure, here's an overview..."},
		{Role: "user", Content: "Can you show me an example?"},
	}

	p := NewMemoryPlugin(mgr)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s2",
		AgentID:      "a2",
		Messages:     &messages,
		CurrentPoint: hook.AfterContextBuild,
	})
	require.NoError(t, err)

	// The messages list should now have one extra entry (system message prepended).
	assert.Len(t, messages, 4)

	// First message should be the injected system message.
	assert.Equal(t, "system", messages[0].Role)
	assert.Contains(t, messages[0].Content, "Relevant context from previous conversations")
	assert.Contains(t, messages[0].Content, "User asked about Go generics last time")
	assert.Contains(t, messages[0].Content, "User prefers concise error messages")
}

// TestMemoryPlugin_AfterContextBuild_EmptyResults verifies that when
// search returns no results, no system message is injected.
func TestMemoryPlugin_AfterContextBuild_EmptyResults(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s3", agentID: "a3"}

	mgr := &stubMemoryManager{
		searchResults: []*memory.Memory{},
	}

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "hello"},
	}

	p := NewMemoryPlugin(mgr)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s3",
		AgentID:      "a3",
		Messages:     &messages,
		CurrentPoint: hook.AfterContextBuild,
	})
	require.NoError(t, err)
	// Should be unchanged — no extra entry.
	assert.Len(t, messages, 1)
}

// TestMemoryPlugin_AfterContextBuild_SearchError verifies that search
// errors are logged but don't abort the pipeline or modify messages.
func TestMemoryPlugin_AfterContextBuild_SearchError(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s4", agentID: "a4"}

	mgr := &stubMemoryManager{
		searchErr: errors.New("qdrant unavailable"),
	}

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "hello"},
	}

	p := NewMemoryPlugin(mgr)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s4",
		AgentID:      "a4",
		Messages:     &messages,
		CurrentPoint: hook.AfterContextBuild,
	})
	// Error should be swallowed — hook returns nil.
	assert.NoError(t, err)
	assert.Len(t, messages, 1)
}

// TestMemoryPlugin_AfterContextBuild_NilMessages verifies no crash when
// Messages is nil.
func TestMemoryPlugin_AfterContextBuild_NilMessages(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "s5", agentID: "a5"}

	mgr := &stubMemoryManager{
		searchResults: []*memory.Memory{
			{Content: "past context"},
		},
	}

	p := NewMemoryPlugin(mgr)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s5",
		AgentID:      "a5",
		Messages:     nil,
		CurrentPoint: hook.AfterContextBuild,
	})
	assert.NoError(t, err)
}

// TestMemoryPlugin_OnMessagePersist_StoresAssistant verifies that
// the last assistant message with content is stored to the memory manager.
func TestMemoryPlugin_OnMessagePersist_StoresAssistant(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "p1", agentID: "a1"}

	mgr := &stubMemoryManager{}

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "Hi, how can I help?"},
	}

	p := NewMemoryPlugin(mgr)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "p1",
		AgentID:      "a1",
		Messages:     &messages,
		CurrentPoint: hook.OnMessagePersist,
	})
	assert.NoError(t, err)

	// Store happens asynchronously; wait briefly for the goroutine.
	time.Sleep(50 * time.Millisecond)

	assert.Len(t, mgr.storedMemories, 1)
	assert.Equal(t, "Hi, how can I help?", mgr.storedMemories[0].Content)
	assert.Equal(t, "p1", mgr.storedMemories[0].SessionID)
	assert.Equal(t, "assistant", mgr.storedMemories[0].Role)
	assert.Equal(t, "conversation", mgr.storedMemories[0].MemoryType)
}

// TestMemoryPlugin_OnMessagePersist_SkipsEmptyContent verifies that
// assistant messages with empty content are not stored.
func TestMemoryPlugin_OnMessagePersist_SkipsEmptyContent(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "p2", agentID: "a2"}

	mgr := &stubMemoryManager{}

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: ""},
	}

	p := NewMemoryPlugin(mgr)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "p2",
		AgentID:      "a2",
		Messages:     &messages,
		CurrentPoint: hook.OnMessagePersist,
	})
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Len(t, mgr.storedMemories, 0)
}

// TestMemoryPlugin_OnMessagePersist_SkipsUserMessages verifies that
// user messages are never stored.
func TestMemoryPlugin_OnMessagePersist_SkipsUserMessages(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "p3", agentID: "a3"}

	mgr := &stubMemoryManager{}

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "my message"},
	}

	p := NewMemoryPlugin(mgr)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "p3",
		AgentID:      "a3",
		Messages:     &messages,
		CurrentPoint: hook.OnMessagePersist,
	})
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Len(t, mgr.storedMemories, 0)
}

// TestMemoryPlugin_OnMessagePersist_NilMessages verifies no crash when
// Messages is nil on persist.
func TestMemoryPlugin_OnMessagePersist_NilMessages(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "p5", agentID: "a5"}

	mgr := &stubMemoryManager{}

	p := NewMemoryPlugin(mgr)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "p5",
		AgentID:      "a5",
		Messages:     nil,
		CurrentPoint: hook.OnMessagePersist,
	})
	assert.NoError(t, err)
}

// TestMemoryPlugin_Execute_UnknownPoint verifies that unknown hook points
// are silently ignored.
func TestMemoryPlugin_Execute_UnknownPoint(t *testing.T) {
	ctx := context.Background()
	stubCtx := &stubChatContext{ctx: ctx, sessionID: "u1", agentID: "u1"}

	mgr := &stubMemoryManager{
		searchResults: []*memory.Memory{{Content: "should not inject"}},
	}

	messages := []entity.MessageForLLM{
		{Role: "user", Content: "test"},
	}

	p := NewMemoryPlugin(mgr)
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "u1",
		AgentID:      "u1",
		Messages:     &messages,
		CurrentPoint: hook.BeforeLLMCall,
	})
	assert.NoError(t, err)
	assert.Len(t, messages, 1)
}
