package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
)

type upsertTrackingContextManager struct {
	mu          sync.Mutex
	messages    map[uuid.UUID]*session.Message
	insertCount int
	updateCount int
	upsertCount int
	insertOrder []uuid.UUID
}

func newUpsertTrackingContextManager() *upsertTrackingContextManager {
	return &upsertTrackingContextManager{
		messages: make(map[uuid.UUID]*session.Message),
	}
}

func (m *upsertTrackingContextManager) GetHistory(chatCtx iface.ChatContextInterface, limit int) ([]session.Message, error) {
	return nil, nil
}

func (m *upsertTrackingContextManager) AddMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[msg.ID] = msg
	m.insertCount++
	m.insertOrder = append(m.insertOrder, msg.ID)
	return nil
}

func (m *upsertTrackingContextManager) UpdateMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[msg.ID] = msg
	m.updateCount++
	return nil
}

func (m *upsertTrackingContextManager) UpsertMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.messages[msg.ID]; exists {
		m.messages[msg.ID] = msg
	} else {
		m.messages[msg.ID] = msg
		m.insertOrder = append(m.insertOrder, msg.ID)
	}
	m.upsertCount++
	return nil
}

func (m *upsertTrackingContextManager) BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]entity.MessageForLLM, error) {
	return nil, nil
}

func (m *upsertTrackingContextManager) DeleteBySession(chatCtx iface.ChatContextInterface) error {
	return nil
}

func (m *upsertTrackingContextManager) MessageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func (m *upsertTrackingContextManager) GetMessage(id uuid.UUID) *session.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages[id]
}

func TestPersistMessage_InsertThenUpdate(t *testing.T) {
	ctxMgr := newUpsertTrackingContextManager()
	engine := NewTestEngine(WithTestContextMgr(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionManager()
	chatCtxCreate := iface.NewChatContext(ctx, "", "test-agent")
	sess, err := sessionMgr.Create(chatCtxCreate, "Test Session", "test-agent")
	require.NoError(t, err)

	chatCtx := iface.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	persistedMsgUUID := ""
	accumulatedParts := session.PersistedParts{}
	accumulatedToolCalls := session.ToolCalls{}

	result1 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "Step 0 content",
		ToolCalls: []toolCallInfo{
			{ID: "call-1", Name: "test_tool", Arguments: "{}", MessageID: msgUUID},
		},
		ToolResults: map[string]*ToolCallResult{
			"call-1": {Output: `{"result": "ok"}`},
		},
	}

	err = engine.persistMessage(chatCtx, result1, false, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
	require.NoError(t, err)

	assert.Equal(t, 1, ctxMgr.insertCount, "first persistMessage should INSERT")
	assert.Equal(t, 0, ctxMgr.updateCount, "first persistMessage should not UPDATE")
	assert.Equal(t, msgUUID, persistedMsgUUID, "persistedMsgUUID should be set after INSERT")

	result2 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 1,
		Content:   "Step 1 final content",
	}

	err = engine.persistMessage(chatCtx, result2, true, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
	require.NoError(t, err)

	assert.Equal(t, 1, ctxMgr.insertCount, "second persistMessage should not INSERT again")
	assert.Equal(t, 1, ctxMgr.updateCount, "second persistMessage should UPDATE")
	assert.Equal(t, 1, ctxMgr.MessageCount(), "should have exactly 1 message row")
}

func TestPersistMessage_SingleRowAfterMultiplePersists(t *testing.T) {
	ctxMgr := newUpsertTrackingContextManager()
	engine := NewTestEngine(WithTestContextMgr(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionManager()
	chatCtxCreate := iface.NewChatContext(ctx, "", "test-agent")
	sess, err := sessionMgr.Create(chatCtxCreate, "Test Session", "test-agent")
	require.NoError(t, err)

	chatCtx := iface.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	persistedMsgUUID := ""
	accumulatedParts := session.PersistedParts{}
	accumulatedToolCalls := session.ToolCalls{}

	for i := 0; i < 5; i++ {
		result := &StreamResult{
			MessageID: msgUUID,
			StepIndex: i,
			Content:   "content step " + string(rune('0'+i)),
		}
		if i < 4 {
			result.ToolCalls = []toolCallInfo{
				{ID: "call-" + string(rune('0'+i)), Name: "tool", Arguments: "{}", MessageID: msgUUID},
			}
			result.ToolResults = map[string]*ToolCallResult{
				"call-" + string(rune('0'+i)): {Output: `"ok"`},
			}
		}

		isFinal := i == 4
		err = engine.persistMessage(chatCtx, result, isFinal, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
		require.NoError(t, err)
	}

	assert.Equal(t, 1, ctxMgr.insertCount, "should INSERT exactly once")
	assert.Equal(t, 4, ctxMgr.updateCount, "should UPDATE 4 times (steps 1-4)")
	assert.Equal(t, 1, ctxMgr.MessageCount(), "should have exactly 1 message row after 5 persists")
}

func TestPersistMessage_AccumulatedParts(t *testing.T) {
	ctxMgr := newUpsertTrackingContextManager()
	engine := NewTestEngine(WithTestContextMgr(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionManager()
	chatCtxCreate := iface.NewChatContext(ctx, "", "test-agent")
	sess, err := sessionMgr.Create(chatCtxCreate, "Test Session", "test-agent")
	require.NoError(t, err)

	chatCtx := iface.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()
	msgUUIDParsed := uuid.MustParse(msgUUID)

	persistedMsgUUID := ""
	accumulatedParts := session.PersistedParts{}
	accumulatedToolCalls := session.ToolCalls{}

	result1 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "Step 0",
		ToolCalls: []toolCallInfo{
			{ID: "call-1", Name: "tool_a", Arguments: `{"x":1}`, MessageID: msgUUID},
		},
		ToolResults: map[string]*ToolCallResult{
			"call-1": {Output: `"result_a"`},
		},
	}
	err = engine.persistMessage(chatCtx, result1, false, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
	require.NoError(t, err)

	result2 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 1,
		Content:   "Step 1",
		ToolCalls: []toolCallInfo{
			{ID: "call-2", Name: "tool_b", Arguments: `{"y":2}`, MessageID: msgUUID},
		},
		ToolResults: map[string]*ToolCallResult{
			"call-2": {Output: `"result_b"`},
		},
	}
	err = engine.persistMessage(chatCtx, result2, false, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
	require.NoError(t, err)

	result3 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 2,
		Content:   "Final answer",
	}
	err = engine.persistMessage(chatCtx, result3, true, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
	require.NoError(t, err)

	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg, "message should exist")

	parts := msg.Parts
	require.NotEmpty(t, parts, "parts should not be empty")

	assert.Equal(t, 0, parts[0].StepIndex, "first part should be step 0")
	assert.Equal(t, "text", parts[0].Type)

	hasStep0ToolCall := false
	hasStep1ToolCall := false
	hasStep2Text := false
	for _, p := range parts {
		if p.StepIndex == 0 && p.Type == "tool-call" && p.ToolName == "tool_a" {
			hasStep0ToolCall = true
			assert.Equal(t, "complete", p.State)
			assert.Equal(t, `"result_a"`, p.Output)
		}
		if p.StepIndex == 1 && p.Type == "tool-call" && p.ToolName == "tool_b" {
			hasStep1ToolCall = true
			assert.Equal(t, "complete", p.State)
			assert.Equal(t, `"result_b"`, p.Output)
		}
		if p.StepIndex == 2 && p.Type == "text" {
			hasStep2Text = true
			assert.Equal(t, "Final answer", p.Text)
		}
	}
	assert.True(t, hasStep0ToolCall, "parts should contain step 0 tool-call")
	assert.True(t, hasStep1ToolCall, "parts should contain step 1 tool-call")
	assert.True(t, hasStep2Text, "parts should contain step 2 text")

	assert.Equal(t, "Final answer", msg.Content, "content should be from last step")
}

func TestPersistMessageUpsert_InsertThenUpdate(t *testing.T) {
	ctxMgr := newUpsertTrackingContextManager()
	engine := NewTestEngine(WithTestContextMgr(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionManager()
	chatCtxCreate := iface.NewChatContext(ctx, "", "test-agent")
	sess, err := sessionMgr.Create(chatCtxCreate, "Test Session", "test-agent")
	require.NoError(t, err)

	chatCtx := iface.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()
	msgUUIDParsed := uuid.MustParse(msgUUID)

	parts1 := session.PersistedParts{
		{Type: "text", Text: "Hello", State: "streaming", StepIndex: 0},
	}
	err = engine.persistMessageUpsert(chatCtx, msgUUID, parts1)
	require.NoError(t, err)

	assert.Equal(t, 1, ctxMgr.upsertCount, "first call should invoke UpsertMessage")
	assert.Equal(t, 1, ctxMgr.MessageCount(), "should have 1 row after first upsert")

	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 1)
	assert.Equal(t, "Hello", msg.Parts[0].Text)
	assert.Equal(t, "streaming", msg.Parts[0].State)

	parts2 := session.PersistedParts{
		{Type: "text", Text: "Hello World", State: "done", StepIndex: 0},
	}
	err = engine.persistMessageUpsert(chatCtx, msgUUID, parts2)
	require.NoError(t, err)

	assert.Equal(t, 2, ctxMgr.upsertCount, "second call should invoke UpsertMessage again")
	assert.Equal(t, 1, ctxMgr.MessageCount(), "should still have 1 row after second upsert")

	msg = ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 1)
	assert.Equal(t, "Hello World", msg.Parts[0].Text, "parts should be updated")
	assert.Equal(t, "done", msg.Parts[0].State, "state should be updated")
}

func TestPersistMessageUpsert_SingleRowAfterFivePersists(t *testing.T) {
	ctxMgr := newUpsertTrackingContextManager()
	engine := NewTestEngine(WithTestContextMgr(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionManager()
	chatCtxCreate := iface.NewChatContext(ctx, "", "test-agent")
	sess, err := sessionMgr.Create(chatCtxCreate, "Test Session", "test-agent")
	require.NoError(t, err)

	chatCtx := iface.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	accumulated := session.PersistedParts{}

	for i := 0; i < 5; i++ {
		accumulated = append(accumulated, session.PersistedPart{
			Type:      "text",
			Text:      "chunk " + string(rune('0'+i)),
			State:     "streaming",
			StepIndex: 0,
		})
		err = engine.persistMessageUpsert(chatCtx, msgUUID, accumulated)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, ctxMgr.upsertCount, "should call UpsertMessage 5 times")
	assert.Equal(t, 1, ctxMgr.MessageCount(), "should have exactly 1 row after 5 upserts")

	msgUUIDParsed := uuid.MustParse(msgUUID)
	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 5, "final Parts should contain all 5 accumulated chunks")

	for i, p := range msg.Parts {
		assert.Equal(t, "chunk "+string(rune('0'+i)), p.Text, "part %d text mismatch", i)
	}
}

func TestPersistMessage_FirstCallSetsUUID(t *testing.T) {
	ctxMgr := newUpsertTrackingContextManager()
	engine := NewTestEngine(WithTestContextMgr(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionManager()
	chatCtxCreate := iface.NewChatContext(ctx, "", "test-agent")
	sess, err := sessionMgr.Create(chatCtxCreate, "Test Session", "test-agent")
	require.NoError(t, err)

	chatCtx := iface.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	persistedMsgUUID := ""
	accumulatedParts := session.PersistedParts{}
	accumulatedToolCalls := session.ToolCalls{}

	assert.Equal(t, "", persistedMsgUUID, "should start empty")

	result := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "First",
	}
	err = engine.persistMessage(chatCtx, result, false, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
	require.NoError(t, err)

	assert.Equal(t, msgUUID, persistedMsgUUID, "should be set after first INSERT")

	result2 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 1,
		Content:   "Second",
	}
	err = engine.persistMessage(chatCtx, result2, true, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
	require.NoError(t, err)

	assert.Equal(t, msgUUID, persistedMsgUUID, "should remain the same UUID after UPDATE")
}

func TestPersistMessage_ToolCallDataPreservedDuringUpdate(t *testing.T) {
	ctxMgr := newUpsertTrackingContextManager()
	engine := NewTestEngine(WithTestContextMgr(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionManager()
	chatCtxCreate := iface.NewChatContext(ctx, "", "test-agent")
	sess, err := sessionMgr.Create(chatCtxCreate, "Test Session", "test-agent")
	require.NoError(t, err)

	chatCtx := iface.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()
	msgUUIDParsed := uuid.MustParse(msgUUID)

	persistedMsgUUID := ""
	accumulatedParts := session.PersistedParts{}
	accumulatedToolCalls := session.ToolCalls{}

	result1 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "Let me check",
		ToolCalls: []toolCallInfo{
			{ID: "call-abc", Name: "search", Arguments: `{"q":"test"}`, MessageID: msgUUID},
		},
		ToolResults: map[string]*ToolCallResult{
			"call-abc": {Output: `"found it"`},
		},
	}
	err = engine.persistMessage(chatCtx, result1, false, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
	require.NoError(t, err)

	result2 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 1,
		Content:   "Here is the result",
	}
	err = engine.persistMessage(chatCtx, result2, true, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
	require.NoError(t, err)

	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)

	hasToolCallPart := false
	for _, p := range msg.Parts {
		if p.Type == "tool-call" && p.ToolCallID == "call-abc" {
			hasToolCallPart = true
			assert.Equal(t, "search", p.ToolName)
			assert.Equal(t, `{"q":"test"}`, p.Args)
			assert.Equal(t, "complete", p.State)
			assert.Equal(t, `"found it"`, p.Output)
		}
	}
	assert.True(t, hasToolCallPart, "tool call data from step 0 should be preserved after UPDATE")

	require.NotEmpty(t, msg.ToolCalls)
	assert.Equal(t, "call-abc", msg.ToolCalls[0].ID)
	assert.Equal(t, "search", msg.ToolCalls[0].Function.Name)
}
