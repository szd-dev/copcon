package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/chatcontext"
	"github.com/copcon/core/storage"
)

type upsertTrackingMessageStore struct {
	mu          sync.Mutex
	messages    map[uuid.UUID]*storage.Message
	insertCount int
	updateCount int
	upsertCount int
	insertOrder []uuid.UUID
}

func newUpsertTrackingMessageStore() *upsertTrackingMessageStore {
	return &upsertTrackingMessageStore{
		messages: make(map[uuid.UUID]*storage.Message),
	}
}

func (m *upsertTrackingMessageStore) List(_ context.Context, _ uuid.UUID, _ int) ([]*storage.Message, error) {
	return nil, nil
}

func (m *upsertTrackingMessageStore) Add(_ context.Context, msg *storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[msg.ID] = msg
	m.insertCount++
	m.insertOrder = append(m.insertOrder, msg.ID)
	return nil
}

func (m *upsertTrackingMessageStore) Update(_ context.Context, msg *storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[msg.ID] = msg
	m.updateCount++
	return nil
}

func (m *upsertTrackingMessageStore) Upsert(_ context.Context, msg *storage.Message) error {
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

func (m *upsertTrackingMessageStore) DeleteBySession(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *upsertTrackingMessageStore) MessageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func (m *upsertTrackingMessageStore) GetMessage(id uuid.UUID) *storage.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages[id]
}

func TestPersistMessage_InsertThenUpdate(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	persistedMsgUUID := ""
	accumulatedParts := []storage.Part{}
	accumulatedToolCalls := []storage.ToolCall{}

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
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	persistedMsgUUID := ""
	accumulatedParts := []storage.Part{}
	accumulatedToolCalls := []storage.ToolCall{}

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
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()
	msgUUIDParsed := uuid.MustParse(msgUUID)

	persistedMsgUUID := ""
	accumulatedParts := []storage.Part{}
	accumulatedToolCalls := []storage.ToolCall{}

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
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()
	msgUUIDParsed := uuid.MustParse(msgUUID)

	parts1 := []storage.Part{
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

	parts2 := []storage.Part{
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
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	accumulated := []storage.Part{}

	for i := 0; i < 5; i++ {
		accumulated = append(accumulated, storage.Part{
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
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	persistedMsgUUID := ""
	accumulatedParts := []storage.Part{}
	accumulatedToolCalls := []storage.ToolCall{}

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
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()
	msgUUIDParsed := uuid.MustParse(msgUUID)

	persistedMsgUUID := ""
	accumulatedParts := []storage.Part{}
	accumulatedToolCalls := []storage.ToolCall{}

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

func TestCheckpointStreamingParts_PersistAtDelta10(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	accumulatedParts := []storage.Part{}

	result := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "Hello World! This is accumulated text after 10 deltas.",
	}

	engine.checkpointStreamingParts(chatCtx, msgUUID, 0, result, &accumulatedParts, false, true)

	assert.Equal(t, 1, ctxMgr.upsertCount, "checkpointStreamingParts should call UpsertMessage once")
	assert.Equal(t, 1, ctxMgr.MessageCount(), "should have exactly 1 message row")

	msgUUIDParsed := uuid.MustParse(msgUUID)
	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 1)
	assert.Equal(t, "text", msg.Parts[0].Type)
	assert.Equal(t, "streaming", msg.Parts[0].State)
	assert.Equal(t, "Hello World! This is accumulated text after 10 deltas.", msg.Parts[0].Text)
}

func TestCheckpointStreamingParts_WithPreviousAccumulatedParts(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	accumulatedParts := []storage.Part{
		{Type: "text", Text: "Previous step text", State: "done", StepIndex: 0},
		{Type: "tool-call", ToolCallID: "call-1", ToolName: "search", Args: `{"q":"test"}`, State: "complete", StepIndex: 0},
	}

	result := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 1,
		Content:   "Streaming text in step 1",
	}

	engine.checkpointStreamingParts(chatCtx, msgUUID, 1, result, &accumulatedParts, false, true)

	assert.Equal(t, 1, ctxMgr.upsertCount)

	msgUUIDParsed := uuid.MustParse(msgUUID)
	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 3)
	assert.Equal(t, "Previous step text", msg.Parts[0].Text)
	assert.Equal(t, "done", msg.Parts[0].State)
	assert.Equal(t, "tool-call", msg.Parts[1].Type)
	assert.Equal(t, "text", msg.Parts[2].Type)
	assert.Equal(t, "streaming", msg.Parts[2].State)
	assert.Equal(t, "Streaming text in step 1", msg.Parts[2].Text)
}

func TestCheckpointDoneParts_TextAndReasoning(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	accumulatedParts := []storage.Part{}

	result := &StreamResult{
		MessageID:        msgUUID,
		StepIndex:        0,
		Content:          "Final text",
		ReasoningContent: "I thought about it",
	}

	engine.checkpointDoneParts(chatCtx, msgUUID, 0, result, &accumulatedParts)

	assert.Equal(t, 1, ctxMgr.upsertCount)

	msgUUIDParsed := uuid.MustParse(msgUUID)
	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 2)
	assert.Equal(t, "reasoning", msg.Parts[0].Type)
	assert.Equal(t, "done", msg.Parts[0].State)
	assert.Equal(t, "I thought about it", msg.Parts[0].Text)
	assert.Equal(t, "text", msg.Parts[1].Type)
	assert.Equal(t, "done", msg.Parts[1].State)
	assert.Equal(t, "Final text", msg.Parts[1].Text)
}

func TestCheckpointDoneParts_WithToolCallsPending(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	accumulatedParts := []storage.Part{}

	result := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "Let me search",
		ToolCalls: []toolCallInfo{
			{ID: "call-1", Name: "search", Arguments: `{"q":"test"}`, MessageID: msgUUID},
		},
	}

	engine.checkpointDoneParts(chatCtx, msgUUID, 0, result, &accumulatedParts)

	msgUUIDParsed := uuid.MustParse(msgUUID)
	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)

	hasPendingToolCall := false
	for _, p := range msg.Parts {
		if p.Type == "tool-call" && p.ToolCallID == "call-1" {
			hasPendingToolCall = true
			assert.Equal(t, "pending", p.State)
			assert.Equal(t, "search", p.ToolName)
		}
	}
	assert.True(t, hasPendingToolCall, "done checkpoint should include tool-call in pending state")
}

func TestCheckpointToolResult_AfterSyncToolComplete(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	persistedMsgUUID := ""
	accumulatedParts := []storage.Part{}

	result := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "Let me search",
		ToolCalls: []toolCallInfo{
			{ID: "call-1", Name: "search", Arguments: `{"q":"test"}`, MessageID: msgUUID},
		},
		ToolResults: map[string]*ToolCallResult{
			"call-1": {Output: `"found it"`},
		},
	}

	engine.checkpointToolResult(chatCtx, result, &persistedMsgUUID, &accumulatedParts)

	assert.Equal(t, 1, ctxMgr.upsertCount)
	assert.Equal(t, msgUUID, persistedMsgUUID, "checkpointToolResult should set persistedMsgUUID on first call")

	msgUUIDParsed := uuid.MustParse(msgUUID)
	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)

	hasCompleteToolCall := false
	for _, p := range msg.Parts {
		if p.Type == "tool-call" && p.ToolCallID == "call-1" {
			hasCompleteToolCall = true
			assert.Equal(t, "complete", p.State)
			assert.Equal(t, `"found it"`, p.Output)
		}
	}
	assert.True(t, hasCompleteToolCall, "tool result checkpoint should show complete state")
}

func TestCheckpointToolResult_AfterSyncToolError(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	persistedMsgUUID := ""
	accumulatedParts := []storage.Part{}

	result := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "Let me try",
		ToolCalls: []toolCallInfo{
			{ID: "call-1", Name: "failing_tool", Arguments: "{}", MessageID: msgUUID},
		},
		ToolResults: map[string]*ToolCallResult{
			"call-1": {Error: "tool execution failed"},
		},
	}

	engine.checkpointToolResult(chatCtx, result, &persistedMsgUUID, &accumulatedParts)

	msgUUIDParsed := uuid.MustParse(msgUUID)
	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)

	hasErrorToolCall := false
	for _, p := range msg.Parts {
		if p.Type == "tool-call" && p.ToolCallID == "call-1" {
			hasErrorToolCall = true
			assert.Equal(t, "error", p.State)
			assert.Equal(t, "tool execution failed", p.Error)
		}
	}
	assert.True(t, hasErrorToolCall, "tool error checkpoint should show error state")
}

func TestIncrementalPersist_MultipleSyncTools(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()

	persistedMsgUUID := ""
	accumulatedParts := []storage.Part{}

	result1 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "Let me help",
		ToolCalls: []toolCallInfo{
			{ID: "call-1", Name: "tool_a", Arguments: "{}", MessageID: msgUUID},
			{ID: "call-2", Name: "tool_b", Arguments: "{}", MessageID: msgUUID},
		},
		ToolResults: map[string]*ToolCallResult{
			"call-1": {Output: `"result_a"`},
		},
	}
	engine.checkpointToolResult(chatCtx, result1, &persistedMsgUUID, &accumulatedParts)
	assert.Equal(t, 1, ctxMgr.upsertCount, "first tool checkpoint")

	msgUUIDParsed := uuid.MustParse(msgUUID)
	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	assert.Len(t, msg.Parts, 3)
	hasToolAComplete := false
	hasToolBPending := false
	for _, p := range msg.Parts {
		if p.ToolCallID == "call-1" && p.Type == "tool-call" {
			hasToolAComplete = true
			assert.Equal(t, "complete", p.State)
		}
		if p.ToolCallID == "call-2" && p.Type == "tool-call" {
			hasToolBPending = true
			assert.Equal(t, "pending", p.State)
		}
	}
	assert.True(t, hasToolAComplete)
	assert.True(t, hasToolBPending)

	result2 := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   "Let me help",
		ToolCalls: []toolCallInfo{
			{ID: "call-1", Name: "tool_a", Arguments: "{}", MessageID: msgUUID},
			{ID: "call-2", Name: "tool_b", Arguments: "{}", MessageID: msgUUID},
		},
		ToolResults: map[string]*ToolCallResult{
			"call-1": {Output: `"result_a"`},
			"call-2": {Output: `"result_b"`},
		},
	}
	engine.checkpointToolResult(chatCtx, result2, &persistedMsgUUID, &accumulatedParts)
	assert.Equal(t, 2, ctxMgr.upsertCount, "second tool checkpoint")
	assert.Equal(t, 1, ctxMgr.MessageCount(), "should still have exactly 1 row")

	msg = ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	hasToolBComplete := false
	for _, p := range msg.Parts {
		if p.ToolCallID == "call-2" && p.Type == "tool-call" {
			hasToolBComplete = true
			assert.Equal(t, "complete", p.State)
		}
	}
	assert.True(t, hasToolBComplete)
}

func TestIncrementalPersist_FullFlow15Deltas(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()
	msgUUIDParsed := uuid.MustParse(msgUUID)

	persistedMsgUUID := ""
	accumulatedParts := []storage.Part{}

	var accumulatedText string
	for i := 1; i <= 15; i++ {
		delta := fmt.Sprintf("delta%d ", i)
		accumulatedText += delta

		result := &StreamResult{
			MessageID: msgUUID,
			StepIndex: 0,
			Content:   accumulatedText,
		}

		if i%deltaPersistInterval == 0 {
			engine.checkpointStreamingParts(chatCtx, msgUUID, 0, result, &accumulatedParts, false, true)
			if persistedMsgUUID == "" {
				persistedMsgUUID = msgUUID
			}

			msg := ctxMgr.GetMessage(msgUUIDParsed)
			require.NotNil(t, msg, "message should exist after delta %d checkpoint", i)
			require.Len(t, msg.Parts, 1)
			assert.Equal(t, "text", msg.Parts[0].Type)
			assert.Equal(t, "streaming", msg.Parts[0].State)
			assert.Equal(t, accumulatedText, msg.Parts[0].Text)
		}
	}

	assert.Equal(t, 1, ctxMgr.upsertCount, "should have 1 upsert after delta 10 checkpoint")
	assert.Equal(t, 1, ctxMgr.MessageCount())

	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 1)
	assert.Equal(t, "delta1 delta2 delta3 delta4 delta5 delta6 delta7 delta8 delta9 delta10 ", msg.Parts[0].Text)

	result := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 0,
		Content:   accumulatedText,
	}
	engine.checkpointDoneParts(chatCtx, msgUUID, 0, result, &accumulatedParts)

	assert.Equal(t, 2, ctxMgr.upsertCount, "should have 2 upserts: delta checkpoint + done checkpoint")
	assert.Equal(t, 1, ctxMgr.MessageCount(), "should still have exactly 1 row")

	msg = ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 1)
	assert.Equal(t, "done", msg.Parts[0].State)
	assert.Equal(t, accumulatedText, msg.Parts[0].Text)
}

func TestIncrementalPersist_DeltaCheckpointPreservesPreviousSteps(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()
	msgUUIDParsed := uuid.MustParse(msgUUID)

	accumulatedParts := []storage.Part{
		{Type: "text", Text: "Step 0 answer", State: "done", StepIndex: 0},
		{Type: "tool-call", ToolCallID: "call-1", ToolName: "search", State: "complete", StepIndex: 0},
	}

	result := &StreamResult{
		MessageID: msgUUID,
		StepIndex: 1,
		Content:   "Streaming in step 1 after 10 deltas",
	}

	engine.checkpointStreamingParts(chatCtx, msgUUID, 1, result, &accumulatedParts, false, true)

	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 3)
	assert.Equal(t, "Step 0 answer", msg.Parts[0].Text, "previous step parts preserved")
	assert.Equal(t, "done", msg.Parts[0].State)
	assert.Equal(t, "tool-call", msg.Parts[1].Type)
	assert.Equal(t, "text", msg.Parts[2].Type)
	assert.Equal(t, "streaming", msg.Parts[2].State)
}

func TestIncrementalPersist_StepCreateCheckpoint(t *testing.T) {
	ctxMgr := newUpsertTrackingMessageStore()
	engine := NewTestEngine(WithTestMessageStore(ctxMgr))

	ctx := context.Background()
	sessionMgr := newMockSessionStore()
	sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
	require.NoError(t, err)

	chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
	msgUUID := uuid.New().String()
	msgUUIDParsed := uuid.MustParse(msgUUID)

	accumulatedParts := []storage.Part{
		{Type: "text", Text: "Step 0 text", State: "done", StepIndex: 0},
	}

	err = engine.persistMessageUpsert(chatCtx, msgUUID, accumulatedParts)
	require.NoError(t, err)
	assert.Equal(t, 1, ctxMgr.upsertCount)

	msg := ctxMgr.GetMessage(msgUUIDParsed)
	require.NotNil(t, msg)
	require.Len(t, msg.Parts, 1)
	assert.Equal(t, "Step 0 text", msg.Parts[0].Text)
}
