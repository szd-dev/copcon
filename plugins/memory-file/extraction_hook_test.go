package memoryfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMessageStore struct {
	messages []*storage.Message
}

func (m *mockMessageStore) List(ctx context.Context, sessionID uuid.UUID, limit int) ([]*storage.Message, error) {
	if limit > 0 && len(m.messages) > limit {
		return m.messages[:limit], nil
	}
	return m.messages, nil
}
func (m *mockMessageStore) Add(ctx context.Context, message *storage.Message) error    { return nil }
func (m *mockMessageStore) Update(ctx context.Context, message *storage.Message) error  { return nil }
func (m *mockMessageStore) Upsert(ctx context.Context, message *storage.Message) error  { return nil }
func (m *mockMessageStore) DeleteBySession(ctx context.Context, sessionID uuid.UUID) error { return nil }

func makeMessages(sessionID uuid.UUID, pairs ...string) []*storage.Message {
	var msgs []*storage.Message
	for i := 0; i < len(pairs); i += 2 {
		role := pairs[i]
		content := pairs[i+1]
		msgs = append(msgs, &storage.Message{
			ID:        uuid.New(),
			SessionID: sessionID,
			Role:       role,
			Content:    content,
		})
	}
	return msgs
}

func TestFactExtractionHook_ExtractsFacts(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-extract"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionUUID := uuid.New()
	sessionID := sessionUUID.String()
	msgStore := &mockMessageStore{
		messages: makeMessages(sessionUUID, "user", "I prefer Go over Python", "assistant", "Noted, you prefer Go."),
	}

	facts := []extractedFact{
		{Content: "User prefers Go over Python", Type: "user", Name: "go_preference", Description: "Language preference", Importance: 0.8},
	}
	mockResp, _ := json.Marshal(facts)
	mockLLM := &mockLLMProvider{response: string(mockResp)}

	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	// Wait for goroutine to complete.
	time.Sleep(500 * time.Millisecond)

	// Verify file created in knowledge/.
	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, err := os.ReadDir(knowledgeDir)
	require.NoError(t, err)
	assert.NotEmpty(t, entries)

	found := false
	for _, e := range entries {
		if e.Name() == "go_preference.md" {
			found = true
			data, err := os.ReadFile(filepath.Join(knowledgeDir, e.Name()))
			require.NoError(t, err)
			assert.Contains(t, string(data), "User prefers Go over Python")
			break
		}
	}
	assert.True(t, found, "expected go_preference.md to be created")
}

func TestFactExtractionHook_SkipWhenManualStore(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-manual"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionID := uuid.New().String()
	msgStore := &mockMessageStore{}
	mockLLM := &mockLLMProvider{response: `[{"content":"x","type":"user","name":"y","description":"d","importance":0.5}]`}

	SetManualStoreFlag(sessionID)
	defer ClearManualStoreFlag(sessionID)

	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// No files should be created.
	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, _ := os.ReadDir(knowledgeDir)
	assert.Empty(t, entries)
}

func TestFactExtractionHook_EmptyResponse(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-empty"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionUUID := uuid.New()
	sessionID := sessionUUID.String()
	msgStore := &mockMessageStore{
		messages: makeMessages(sessionUUID, "user", "Hello", "assistant", "Hi"),
	}

	mockLLM := &mockLLMProvider{response: `[]`}
	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, _ := os.ReadDir(knowledgeDir)
	assert.Empty(t, entries)
}

func TestFactExtractionHook_LLMFailure(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-llmfail"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionUUID := uuid.New()
	sessionID := sessionUUID.String()
	msgStore := &mockMessageStore{
		messages: makeMessages(sessionUUID, "user", "Hello", "assistant", "Hi"),
	}

	mockLLM := &mockLLMProvider{err: context.DeadlineExceeded}
	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, _ := os.ReadDir(knowledgeDir)
	assert.Empty(t, entries)
}

func TestFactExtractionHook_MaxFacts(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-maxfacts"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionUUID := uuid.New()
	sessionID := sessionUUID.String()
	msgStore := &mockMessageStore{
		messages: makeMessages(sessionUUID, "user", "Tell me facts", "assistant", "Here they are"),
	}

	var facts []extractedFact
	for i := 0; i < 10; i++ {
		facts = append(facts, extractedFact{
			Content:     "Fact content " + string(rune('A'+i)),
			Type:        "user",
			Name:        "fact_" + string(rune('A'+i)),
			Description: "Description " + string(rune('A'+i)),
			Importance:  0.5,
		})
	}
	mockResp, _ := json.Marshal(facts)
	mockLLM := &mockLLMProvider{response: string(mockResp)}

	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, err := os.ReadDir(knowledgeDir)
	require.NoError(t, err)

	// Only 5 facts should be written (max).
	assert.LessOrEqual(t, len(entries), 5)
}

func TestFactExtractionHook_NilLLM(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-nilllm"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionID := uuid.New().String()
	msgStore := &mockMessageStore{}

	h := NewFactExtractionHook(store, nil, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, _ := os.ReadDir(knowledgeDir)
	assert.Empty(t, entries)
}

func TestFactExtractionHook_InvalidSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-invalid-sess"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	msgStore := &mockMessageStore{}
	mockLLM := &mockLLMProvider{response: `[]`}

	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: "not-a-valid-uuid",
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Should not crash, no files created.
	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, _ := os.ReadDir(knowledgeDir)
	assert.Empty(t, entries)
}

func TestFactExtractionHook_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-badjson"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionUUID := uuid.New()
	sessionID := sessionUUID.String()
	msgStore := &mockMessageStore{
		messages: makeMessages(sessionUUID, "user", "Hello", "assistant", "Hi"),
	}

	mockLLM := &mockLLMProvider{response: "this is not json"}
	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, _ := os.ReadDir(knowledgeDir)
	assert.Empty(t, entries)
}

func TestFactExtractionHook_FactWithMissingFields(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-missing-fields"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionUUID := uuid.New()
	sessionID := sessionUUID.String()
	msgStore := &mockMessageStore{
		messages: makeMessages(sessionUUID, "user", "Tell me", "assistant", "OK"),
	}

	facts := []extractedFact{
		{Content: "", Type: "user", Name: "empty_content", Description: "No content", Importance: 0.5},
		{Content: "Has content", Type: "user", Name: "", Description: "No name", Importance: 0.5},
		{Content: "Valid fact", Type: "user", Name: "valid_fact", Description: "A valid fact", Importance: 0.8},
	}
	mockResp, _ := json.Marshal(facts)
	mockLLM := &mockLLMProvider{response: string(mockResp)}

	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, err := os.ReadDir(knowledgeDir)
	require.NoError(t, err)

	// Only the valid fact (with both content and name) should be written.
	assert.Len(t, entries, 1)
	assert.Equal(t, "valid_fact.md", entries[0].Name())
}

func TestFactExtractionHook_CodeBlockJSON(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-codeblock"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionUUID := uuid.New()
	sessionID := sessionUUID.String()
	msgStore := &mockMessageStore{
		messages: makeMessages(sessionUUID, "user", "Remember this", "assistant", "Got it"),
	}

	facts := []extractedFact{
		{Content: "Code block fact", Type: "reference", Name: "codeblock_fact", Description: "From code block", Importance: 0.7},
	}
	raw, _ := json.Marshal(facts)
	// Simulate LLM wrapping JSON in a code block.
	codeBlockResp := "```json\n" + string(raw) + "\n```"
	mockLLM := &mockLLMProvider{response: codeBlockResp}

	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, err := os.ReadDir(knowledgeDir)
	require.NoError(t, err)
	assert.NotEmpty(t, entries)
}

func TestFactExtractionHook_PriorityAndName(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	h := NewFactExtractionHook(store, nil, nil, "test-model")
	assert.Equal(t, "fact_extraction", h.Name())
	assert.Equal(t, 100, h.Priority())
	assert.Equal(t, []hook.HookPoint{hook.OnMessagePersist}, h.Points())
}

func TestSetManualStoreFlag(t *testing.T) {
	sessionID := "flag-test-session"

	SetManualStoreFlag(sessionID)
	_, loaded := manualStoreFlags.Load(sessionID)
	assert.True(t, loaded)

	ClearManualStoreFlag(sessionID)
	_, loaded = manualStoreFlags.Load(sessionID)
	assert.False(t, loaded)
}

func TestBuildExtractionPrompt(t *testing.T) {
	prompt := buildExtractionPrompt("existing mem", "user: hello")
	assert.Contains(t, prompt, "fact extraction assistant")
	assert.Contains(t, prompt, "existing mem")
	assert.Contains(t, prompt, "user: hello")
	assert.Contains(t, prompt, "JSON array")
}

func TestBuildExtractionPrompt_NoExistingMemories(t *testing.T) {
	prompt := buildExtractionPrompt("", "user: hello")
	assert.NotContains(t, prompt, "Existing memories")
	assert.Contains(t, prompt, "user: hello")
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hel...", truncate("hello world", 3))
}

func TestFactExtractionHook_EmptyMessages(t *testing.T) {
	tmpDir := t.TempDir()
	agentID := "agent-emptymsgs"
	require.NoError(t, EnsureAgentDirs(tmpDir, agentID))

	store, err := NewFileMemoryStore(tmpDir, 200, 25*1024)
	require.NoError(t, err)

	sessionUUID := uuid.New()
	sessionID := sessionUUID.String()
	msgStore := &mockMessageStore{
		messages: []*storage.Message{
			{ID: uuid.New(), SessionID: sessionUUID, Role: "user", Content: ""},
		},
	}

	mockLLM := &mockLLMProvider{response: `[]`}
	h := NewFactExtractionHook(store, mockLLM, msgStore, "test-model")

	ctx := &hook.HookContext{
		AgentID:   agentID,
		SessionID: sessionID,
		ChatCtx:   &mockChatCtx{},
	}

	err = h.Execute(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	knowledgeDir := filepath.Join(tmpDir, agentID, "knowledge")
	entries, _ := os.ReadDir(knowledgeDir)
	assert.Empty(t, entries)
}

// extractionMockChatCtx satisfies iface.ChatContextInterface.
type extractionMockChatCtx struct{}

func (m *extractionMockChatCtx) Context() context.Context                           { return context.Background() }
func (m *extractionMockChatCtx) SessionID() string                                   { return "" }
func (m *extractionMockChatCtx) AgentID() string                                     { return "" }
func (m *extractionMockChatCtx) Events() <-chan entity.Event                         { return nil }
func (m *extractionMockChatCtx) Emit(event entity.Event)                             {}
func (m *extractionMockChatCtx) Close()                                              {}
func (m *extractionMockChatCtx) Closed() <-chan struct{}                             { return nil }
func (m *extractionMockChatCtx) Depth() int                                          { return 0 }
func (m *extractionMockChatCtx) Subscribe(fromSeq int64) (*iface.Subscriber, bool)   { return nil, false }
func (m *extractionMockChatCtx) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
	return nil, nil
}
func (m *extractionMockChatCtx) ResolveInput(interruptID string, resp *iface.InputResponse) error {
	return nil
}
func (m *extractionMockChatCtx) PendingInputs() []iface.InputRequest                 { return nil }
func (m *extractionMockChatCtx) SetPartLocator(messageID string, stepIndex, partIndex int) {}
func (m *extractionMockChatCtx) ClearPartLocator()                                   {}
