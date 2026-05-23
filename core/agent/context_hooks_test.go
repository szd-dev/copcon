package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/chatcontext"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/testutil"
	"github.com/copcon/core/tool"
)

// contextHookRecorder records all hook calls with point, session ID, and agent ID.
type contextHookRecorder struct {
	mu      sync.Mutex
	records []contextHookRecord
}

type contextHookRecord struct {
	Point     hook.HookPoint
	SessionID string
	AgentID   string
}

func (r *contextHookRecorder) Name() string  { return "context-recorder" }
func (r *contextHookRecorder) Priority() int { return 100 }

func (r *contextHookRecorder) Points() []hook.HookPoint {
	return []hook.HookPoint{
		hook.OnSessionResolve,
		hook.BeforeContextBuild,
		hook.AfterContextBuild,
		hook.BeforeLLMCall,
		hook.AfterLLMCall,
		hook.OnMessagePersist,
	}
}

func (r *contextHookRecorder) Execute(ctx *hook.HookContext) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, contextHookRecord{
		Point:     ctx.CurrentPoint,
		SessionID: ctx.SessionID,
		AgentID:   ctx.AgentID,
	})
	return nil
}

func (r *contextHookRecorder) Records() []contextHookRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	cpy := make([]contextHookRecord, len(r.records))
	copy(cpy, r.records)
	return cpy
}

func (r *contextHookRecorder) PointsInOrder() []hook.HookPoint {
	recs := r.Records()
	pts := make([]hook.HookPoint, len(recs))
	for i, rec := range recs {
		pts[i] = rec.Point
	}
	return pts
}

// mockOpenAIServer creates an httptest server that mimics the OpenAI streaming API.
// It returns a single content chunk followed by a finish reason.
func mockOpenAIServer(content string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "not found", 404)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", 500)
			return
		}

		if content != "" {
			chunk := map[string]any{
				"id":      "chatcmpl-test",
				"object":  "chat.completion.chunk",
				"created": 1234567890,
				"model":   "gpt-4o",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"content": content,
						},
					},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		finishChunk := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion.chunk",
			"created": 1234567890,
			"model":   "gpt-4o",
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         map[string]any{},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		data, _ := json.Marshal(finishChunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

// TestContextHooks tests that all context lifecycle hooks fire in the correct order.
func TestContextHooks(t *testing.T) {
	t.Run("full lifecycle hook order", func(t *testing.T) {
		server := mockOpenAIServer("Hello, I am an AI assistant.")
		defer server.Close()

		// Create recording hook
		recorder := &contextHookRecorder{}
		runner := hook.NewHookRunner()
		runner.Register(recorder)

		sessionMgr := newMockSessionStore()
		ctx := context.Background()
		sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
		require.NoError(t, err)

		contextMgr := &recordingMessageStore{}

		// Create agent registry with a mock OpenAI client pointing at our test server
		agentRegistry := newMockAgentRegistry()
		mockClient := openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		)
		agent := AgentDefinition{
			ID:           "test-agent",
			Name:         "Test Agent",
			Model:        "gpt-4o",
			SystemPrompt: "You are a test assistant.",
			ToolManager:  &mockToolManagerForEngine{},
			LLMProvider:  llm.NewOpenAIAdapter(&mockClient, "gpt-4o"),
		}
		agentRegistry.Register("test-agent", agent)
		agentRegistry.SetDefault("test-agent")

		// Create engine with hook runner
		engine := NewTestEngine(
			WithTestRegistry(agentRegistry),
			WithTestSessionStore(sessionMgr),
			WithTestMessageStore(contextMgr),
			WithHookRunner(runner),
		)

		// Run Chat
		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
		err = engine.Chat(chatCtx, "Hello")
		require.NoError(t, err)

		// Drain events channel
		go func() {
			for range chatCtx.Events() {
			}
		}()
		time.Sleep(500 * time.Millisecond)
		closeMockChatContext(chatCtx)

		// Verify records
		records := recorder.Records()
		pointsInOrder := make([]hook.HookPoint, len(records))
		for i, rec := range records {
			pointsInOrder[i] = rec.Point
		}

		// Expected order for first iteration:
		// 1. OnSessionResolve (in prepareAgentLoop)
		// 2. OnSystemPrompt, BeforeContextBuild, AfterContextBuild, BeforeLLMCall, AfterLLMCall (in loop)
		// 3. OnMessagePersist (in persistMessage after handleToolCalls)
		t.Logf("Hook call order: %v", pointsInOrder)

		// Verify all required hook points are present
		assert.Contains(t, pointsInOrder, hook.OnSessionResolve, "OnSessionResolve should fire")
		assert.Contains(t, pointsInOrder, hook.BeforeContextBuild, "BeforeContextBuild should fire")
		assert.Contains(t, pointsInOrder, hook.AfterContextBuild, "AfterContextBuild should fire")
		assert.Contains(t, pointsInOrder, hook.BeforeLLMCall, "BeforeLLMCall should fire")
		assert.Contains(t, pointsInOrder, hook.AfterLLMCall, "AfterLLMCall should fire")
		assert.Contains(t, pointsInOrder, hook.OnMessagePersist, "OnMessagePersist should fire")

		// Verify order: OnSessionResolve must come first
		onSessionResolveIdx := -1
		beforeContextBuildIdx := -1
		afterContextBuildIdx := -1
		beforeLLMCallIdx := -1
		afterLLMCallIdx := -1
		onMessagePersistIdx := -1

		for i, rec := range records {
			switch rec.Point {
			case hook.OnSessionResolve:
				onSessionResolveIdx = i
			case hook.BeforeContextBuild:
				beforeContextBuildIdx = i
			case hook.AfterContextBuild:
				afterContextBuildIdx = i
			case hook.BeforeLLMCall:
				beforeLLMCallIdx = i
			case hook.AfterLLMCall:
				afterLLMCallIdx = i
			case hook.OnMessagePersist:
				onMessagePersistIdx = i
			}
		}

		assert.Greater(t, beforeContextBuildIdx, onSessionResolveIdx,
			"BeforeContextBuild should fire after OnSessionResolve")
		assert.Greater(t, afterContextBuildIdx, beforeContextBuildIdx,
			"AfterContextBuild should fire after BeforeContextBuild")
		assert.Greater(t, beforeLLMCallIdx, afterContextBuildIdx,
			"BeforeLLMCall should fire after AfterContextBuild")
		assert.Greater(t, afterLLMCallIdx, beforeLLMCallIdx,
			"AfterLLMCall should fire after BeforeLLMCall")
		assert.Greater(t, onMessagePersistIdx, afterLLMCallIdx,
			"OnMessagePersist should fire after AfterLLMCall")

		// Verify session ID and agent ID are correct in all records
		for _, rec := range records {
			assert.Equal(t, sess.ID.String(), rec.SessionID,
				"SessionID should be correct for hook %s", rec.Point)
			assert.Equal(t, "test-agent", rec.AgentID,
				"AgentID should be correct for hook %s", rec.Point)
		}
	})

	t.Run("hooks fire correctly on multi-iteration loop", func(t *testing.T) {
		// Server that returns tool calls on first iteration, then content on second
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", 500)
				return
			}

			callCount++
			if callCount == 1 {
				toolCallChunk := map[string]any{
					"id":      "chatcmpl-test",
					"object":  "chat.completion.chunk",
					"created": 1234567890,
					"model":   "gpt-4o",
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"tool_calls": []map[string]any{
									{
										"index": 0,
										"id":    "call-test-001",
										"type":  "function",
										"function": map[string]any{
											"name":      "noop",
											"arguments": "{}",
										},
									},
								},
							},
						},
					},
				}
				data, _ := json.Marshal(toolCallChunk)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}

			finishChunk := map[string]any{
				"id":      "chatcmpl-test",
				"object":  "chat.completion.chunk",
				"created": 1234567890,
				"model":   "gpt-4o",
				"choices": []map[string]any{
					{
						"index":         0,
						"delta":         map[string]any{},
						"finish_reason": "stop",
					},
				},
			}
			data, _ := json.Marshal(finishChunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
		defer server.Close()

		recorder := &contextHookRecorder{}
		runner := hook.NewHookRunner()
		runner.Register(recorder)

		sessionMgr := newMockSessionStore()
		ctx := context.Background()
		sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
		require.NoError(t, err)

		contextMgr := &recordingMessageStore{}

		agentRegistry := newMockAgentRegistry()
		mockClient := openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		)

		// Need a tool manager that can actually handle "noop" tool calls
		toolMgr := newMockToolManagerWithTools()
		noopTool := &mockControllableTool{
			name:     "noop",
			duration: 10 * time.Millisecond,
			result:   &tool.ToolResult{Success: true, Data: map[string]any{"ok": true}},
		}
		toolMgr.Register(noopTool)

		agent := AgentDefinition{
			ID:           "test-agent",
			Name:         "Test Agent",
			Model:        "gpt-4o",
			SystemPrompt: "You are a test assistant.",
			ToolManager:  toolMgr,
			LLMProvider:  llm.NewOpenAIAdapter(&mockClient, "gpt-4o"),
		}
		agentRegistry.Register("test-agent", agent)
		agentRegistry.SetDefault("test-agent")

		engine := NewTestEngine(
			WithTestRegistry(agentRegistry),
			WithTestSessionStore(sessionMgr),
			WithTestMessageStore(contextMgr),
			WithHookRunner(runner),
		)

		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
		err = engine.Chat(chatCtx, "Hello")
		require.NoError(t, err)

		// Drain events
		go func() {
			for range chatCtx.Events() {
			}
		}()
		time.Sleep(500 * time.Millisecond)
		closeMockChatContext(chatCtx)

		records := recorder.Records()

		// On multi-iteration, OnSessionResolve fires once.
		// Context hooks + LLM hooks + MessagePersist fire twice (once per iteration).
		countByPoint := make(map[hook.HookPoint]int)
		for _, rec := range records {
			countByPoint[rec.Point]++
		}

		assert.Equal(t, 1, countByPoint[hook.OnSessionResolve],
			"OnSessionResolve should fire exactly once")
		assert.Equal(t, 2, countByPoint[hook.BeforeContextBuild],
			"BeforeContextBuild should fire twice (once per loop iteration)")
		assert.Equal(t, 2, countByPoint[hook.AfterContextBuild],
			"AfterContextBuild should fire twice")
		assert.Equal(t, 2, countByPoint[hook.BeforeLLMCall],
			"BeforeLLMCall should fire twice")
		assert.Equal(t, 2, countByPoint[hook.AfterLLMCall],
			"AfterLLMCall should fire twice")
		assert.Equal(t, 2, countByPoint[hook.OnMessagePersist],
			"OnMessagePersist should fire twice")

		t.Logf("Multi-iteration hook counts: %v", countByPoint)
	})

	t.Run("OnSessionResolve fires when prepareAgentLoop is called directly", func(t *testing.T) {
		recorder := &contextHookRecorder{}
		runner := hook.NewHookRunner()
		runner.Register(recorder)

		sessionMgr := newMockSessionStore()
		ctx := context.Background()
		sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
		require.NoError(t, err)

		agentRegistry := newMockAgentRegistry()
		agentRegistry.Register("test-agent", AgentDefinition{
			ID:           "test-agent",
			Name:         "Test Agent",
			Model:        "gpt-4o",
			SystemPrompt: "You are a test assistant.",
			ToolManager:  &mockToolManagerForEngine{},
		})
		agentRegistry.SetDefault("test-agent")

		engine := NewTestEngine(
			WithTestRegistry(agentRegistry),
			WithTestSessionStore(sessionMgr),
			WithHookRunner(runner),
		)

		chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
		_, err = engine.prepareAgentLoop(chatCtx, "Hello")
		require.NoError(t, err)

		records := recorder.Records()
		require.Len(t, records, 1, "expected only OnSessionResolve")
		assert.Equal(t, hook.OnSessionResolve, records[0].Point)
		assert.Equal(t, sess.ID.String(), records[0].SessionID)
		assert.Equal(t, "test-agent", records[0].AgentID)
	})

	t.Run("OnMessagePersist fires after persistMessage", func(t *testing.T) {
		recorder := &contextHookRecorder{}
		runner := hook.NewHookRunner()
		runner.Register(recorder)

		sessionMgr := newMockSessionStore()
		ctx := context.Background()
		sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
		require.NoError(t, err)

		contextMgr := &recordingMessageStore{}

		engine := NewTestEngine(
			WithTestSessionStore(sessionMgr),
			WithTestMessageStore(contextMgr),
			WithHookRunner(runner),
		)

		chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")

		result := &StreamResult{
			MessageID: "00000000-0000-0000-0000-000000000001",
			StepIndex: 0,
			Content:   "Hello world",
		}
		err = engine.persistMessage(chatCtx, result, true, new(string), new([]storage.Part), new([]storage.ToolCall))
		require.NoError(t, err)

		records := recorder.Records()
		require.Len(t, records, 1, "expected only OnMessagePersist")
		assert.Equal(t, hook.OnMessagePersist, records[0].Point)
		assert.Equal(t, sess.ID.String(), records[0].SessionID)
	})

	t.Run("default empty runner does not panic", func(t *testing.T) {
		// Engine without WithHookRunner — uses default NewEmptyRunner, should not panic
		sessionMgr := newMockSessionStore()
		ctx := context.Background()
		sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
		require.NoError(t, err)

		agentRegistry := newMockAgentRegistry()
		agentRegistry.Register("test-agent", AgentDefinition{
			ID:           "test-agent",
			Name:         "Test Agent",
			Model:        "gpt-4o",
			SystemPrompt: "You are a test assistant.",
			ToolManager:  &mockToolManagerForEngine{},
		})
		agentRegistry.SetDefault("test-agent")

		engine := NewTestEngine(
			WithTestRegistry(agentRegistry),
			WithTestSessionStore(sessionMgr),
		)

		chatCtx := chatcontext.NewChatContext(ctx, sess.ID.String(), "test-agent")
		_, err = engine.prepareAgentLoop(chatCtx, "Hello")
		require.NoError(t, err)

		result := &StreamResult{
			MessageID: "00000000-0000-0000-0000-000000000001",
			StepIndex: 0,
			Content:   "Hello world",
		}
		err = engine.persistMessage(chatCtx, result, true, new(string), new([]storage.Part), new([]storage.ToolCall))
		require.NoError(t, err)
		// No panic — success
	})

	t.Run("afterContextBuild hook can modify messages", func(t *testing.T) {
		server := mockOpenAIServer("response")
		defer server.Close()

		messageModifierHook := &messageModifierHook{}
		runner := hook.NewHookRunner()
		runner.Register(messageModifierHook)

		sessionMgr := newMockSessionStore()
		ctx := context.Background()
		sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
		require.NoError(t, err)

		contextMgr := &recordingMessageStore{}

		agentRegistry := newMockAgentRegistry()
		mockClient := openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		)
		agent := AgentDefinition{
			ID:           "test-agent",
			Name:         "Test Agent",
			Model:        "gpt-4o",
			SystemPrompt: "System prompt",
			ToolManager:  &mockToolManagerForEngine{},
			LLMProvider:  llm.NewOpenAIAdapter(&mockClient, "gpt-4o"),
		}
		agentRegistry.Register("test-agent", agent)
		agentRegistry.SetDefault("test-agent")

		engine := NewTestEngine(
			WithTestRegistry(agentRegistry),
			WithTestSessionStore(sessionMgr),
			WithTestMessageStore(contextMgr),
			WithHookRunner(runner),
		)

		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
		err = engine.Chat(chatCtx, "Hello")
		require.NoError(t, err)

		go func() {
			for range chatCtx.Events() {
			}
		}()
		time.Sleep(500 * time.Millisecond)
		closeMockChatContext(chatCtx)

		assert.True(t, messageModifierHook.wasCalled, "AfterContextBuild hook should be called")
		assert.True(t, messageModifierHook.modified, "Messages should have been modified")
	})

	t.Run("beforeContextBuild hook can modify system prompt", func(t *testing.T) {
		server := mockOpenAIServer("response")
		defer server.Close()

		promptModified := false
		promptHook := &promptModifierHook{
			onModify: func() { promptModified = true },
		}
		runner := hook.NewHookRunner()
		runner.Register(promptHook)

		sessionMgr := newMockSessionStore()
		ctx := context.Background()
		sess, err := sessionMgr.Create(context.Background(), &storage.Session{Title: "Test Session", DefaultAgentID: "test-agent"})
		require.NoError(t, err)

		contextMgr := &recordingMessageStore{}

		agentRegistry := newMockAgentRegistry()
		mockClient := openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		)
		agent := AgentDefinition{
			ID:           "test-agent",
			Name:         "Test Agent",
			Model:        "gpt-4o",
			SystemPrompt: "Original system prompt",
			ToolManager:  &mockToolManagerForEngine{},
			LLMProvider:  llm.NewOpenAIAdapter(&mockClient, "gpt-4o"),
		}
		agentRegistry.Register("test-agent", agent)
		agentRegistry.SetDefault("test-agent")

		engine := NewTestEngine(
			WithTestRegistry(agentRegistry),
			WithTestSessionStore(sessionMgr),
			WithTestMessageStore(contextMgr),
			WithHookRunner(runner),
		)

		chatCtx := testutil.NewMockChatContext(ctx, sess.ID.String(), "test-agent")
		err = engine.Chat(chatCtx, "Hello")
		require.NoError(t, err)

		go func() {
			for range chatCtx.Events() {
			}
		}()
		time.Sleep(500 * time.Millisecond)
		closeMockChatContext(chatCtx)

		assert.True(t, promptModified, "System prompt should have been modified by hook")
		assert.Equal(t, "Modified: Original system prompt", promptHook.modifiedPrompt,
			"Prompt should be modified")
	})
}

// messageModifierHook modifies messages during AfterContextBuild.
type messageModifierHook struct {
	wasCalled bool
	modified  bool
}

func (h *messageModifierHook) Name() string  { return "message-modifier" }
func (h *messageModifierHook) Priority() int { return 100 }

func (h *messageModifierHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.AfterContextBuild}
}

func (h *messageModifierHook) Execute(ctx *hook.HookContext) error {
	h.wasCalled = true
	if ctx.Messages != nil {
		*ctx.Messages = append(*ctx.Messages, entity.MessageForLLM{
			Role:    "user",
			Content: "[Injected by hook]",
		})
		h.modified = true
	}
	return nil
}

// promptModifierHook modifies the system prompt during BeforeContextBuild.
type promptModifierHook struct {
	onModify       func()
	modifiedPrompt string
}

func (h *promptModifierHook) Name() string  { return "prompt-modifier" }
func (h *promptModifierHook) Priority() int { return 100 }

func (h *promptModifierHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.BeforeContextBuild}
}

func (h *promptModifierHook) Execute(ctx *hook.HookContext) error {
	if ctx.SystemPrompt != nil {
		h.modifiedPrompt = "Modified: " + *ctx.SystemPrompt
		*ctx.SystemPrompt = h.modifiedPrompt
		h.onModify()
	}
	return nil
}

type recordingMessageStore struct {
	addMessages []*storage.Message
	mu          sync.Mutex
}

func (m *recordingMessageStore) List(_ context.Context, _ uuid.UUID, _ int) ([]*storage.Message, error) {
	return nil, nil
}

func (m *recordingMessageStore) Add(_ context.Context, msg *storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addMessages = append(m.addMessages, msg)
	return nil
}

func (m *recordingMessageStore) Update(_ context.Context, msg *storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.addMessages {
		if existing.ID == msg.ID {
			m.addMessages[i] = msg
			return nil
		}
	}
	m.addMessages = append(m.addMessages, msg)
	return nil
}

func (m *recordingMessageStore) Upsert(_ context.Context, msg *storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.addMessages {
		if existing.ID == msg.ID {
			m.addMessages[i] = msg
			return nil
		}
	}
	m.addMessages = append(m.addMessages, msg)
	return nil
}

func (m *recordingMessageStore) DeleteBySession(_ context.Context, _ uuid.UUID) error {
	return nil
}
