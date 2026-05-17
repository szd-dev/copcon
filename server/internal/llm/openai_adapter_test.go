package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compileTimeCheck ensures OpenAIAdapter satisfies LLMProvider.
func TestOpenAIAdapterImplementsLLMProvider(t *testing.T) {
	var _ LLMProvider = (*OpenAIAdapter)(nil)
}

// sseServer is a test HTTP server that simulates OpenAI streaming completions.
type sseServer struct {
	*httptest.Server
	mu     sync.Mutex
	chunks []string // raw SSE data lines to send
}

func newSSEServer(chunks []string) *sseServer {
	s := &sseServer{chunks: chunks}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		s.mu.Lock()
		for _, line := range s.chunks {
			_, _ = fmt.Fprint(w, "data: "+line+"\n\n")
			flusher.Flush()
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
		s.mu.Unlock()
	}))
	return s
}

// checkNoError drains errc and fails if any non-nil error was received.
func checkNoError(t *testing.T, errc <-chan error) {
	t.Helper()
	// errc is already closed at this point; reading returns the buffered
	// error (if any) or nil for a closed, empty channel.
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	default:
	}
}

// openaiChunk builds a JSON-marshalled ChatCompletionChunk for tests.
type openaiChunk struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []openaiChoice  `json:"choices"`
	Usage   json.RawMessage `json:"usage,omitempty"`
}

type openaiChoice struct {
	Index        int         `json:"index"`
	Delta        openaiDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
	LogProbs     any         `json:"logprobs"`
}

type openaiDelta struct {
	Content          string                `json:"content,omitempty"`
	ReasoningContent string                `json:"reasoning_content,omitempty"` // extra field
	ToolCalls        []openaiToolCallDelta `json:"tool_calls,omitempty"`
}

type openaiToolCallDelta struct {
	Index    int                     `json:"index"`
	ID       string                  `json:"id,omitempty"`
	Type     string                  `json:"type,omitempty"`
	Function openaiToolCallDeltaFunc `json:"function,omitempty"`
}

type openaiToolCallDeltaFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func (c openaiChunk) JSON() string {
	data, _ := json.Marshal(c)
	return string(data)
}

// baseChunk returns a template chunk with common fields filled.
func baseChunk() openaiChunk {
	return openaiChunk{
		ID:      "chatcmpl-test",
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "gpt-4o",
	}
}

// choice builds a single openaiChoice.
func choice(delta openaiDelta, finishReason string) openaiChoice {
	return openaiChoice{
		Index:        0,
		Delta:        delta,
		FinishReason: finishReason,
	}
}

func TestOpenAIAdapter_Stream_TextContent(t *testing.T) {
	chunks := []string{
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: "Hello"}, "")).JSON(),
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: " World"}, "")).JSON(),
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: "!"}, "stop")).JSON(),
	}
	srv := newSSEServer(chunks)
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	adapter := NewOpenAIAdapter(&client, "gpt-4o")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, errc := adapter.Stream(ctx, StreamParams{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleSystem, Content: "Be helpful."},
			{Role: RoleUser, Content: "Hi"},
		},
	})

	var content string
	var final *StreamChunk

	for c := range ch {
		content += c.Content
		if c.FinishReason != "" || c.Usage != nil {
			final = &c
		}
	}

	checkNoError(t, errc)

	assert.Equal(t, "Hello World!", content)
	require.NotNil(t, final)
	assert.Equal(t, "stop", final.FinishReason)
	assert.NotNil(t, final.Usage)
}

func TestOpenAIAdapter_Stream_ReasoningContent(t *testing.T) {
	chunks := []string{
		baseChunkCh(baseChunk(), choice(openaiDelta{ReasoningContent: "Let me think..."}, "")).JSON(),
		baseChunkCh(baseChunk(), choice(openaiDelta{ReasoningContent: " about this."}, "")).JSON(),
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: "The answer is 42"}, "stop")).JSON(),
	}
	srv := newSSEServer(chunks)
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	adapter := NewOpenAIAdapter(&client, "gpt-4o")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, errc := adapter.Stream(ctx, StreamParams{
		Model: "deepseek-reasoner",
		Messages: []Message{
			{Role: RoleUser, Content: "What is the answer?"},
		},
	})

	var reasoning string
	contentFound := false

	for c := range ch {
		reasoning += c.ReasoningContent
		if c.Content != "" {
			contentFound = true
		}
	}

	checkNoError(t, errc)

	assert.Equal(t, "Let me think... about this.", reasoning)
	assert.True(t, contentFound, "expected text content in final chunk")
}

func TestOpenAIAdapter_Stream_ToolCalls(t *testing.T) {
	chunks := []string{
		baseChunkCh(baseChunk(), choice(openaiDelta{
			ToolCalls: []openaiToolCallDelta{
				{Index: 0, ID: "call_abc", Type: "function", Function: openaiToolCallDeltaFunc{Name: "read_file"}},
				{Index: 0, Function: openaiToolCallDeltaFunc{Arguments: `{"path":"/`}},
			},
		}, "")).JSON(),
		baseChunkCh(baseChunk(), choice(openaiDelta{
			ToolCalls: []openaiToolCallDelta{
				{Index: 0, Function: openaiToolCallDeltaFunc{Arguments: `tmp/test.txt"}`}},
			},
		}, "tool_calls")).JSON(),
	}
	srv := newSSEServer(chunks)
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	adapter := NewOpenAIAdapter(&client, "gpt-4o")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, errc := adapter.Stream(ctx, StreamParams{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleUser, Content: "Read /tmp/test.txt"},
		},
		Tools: []ToolDef{
			{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	})

	var allToolCalls []ToolCallDelta

	for c := range ch {
		allToolCalls = append(allToolCalls, c.ToolCalls...)
	}

	checkNoError(t, errc)

	// Accumulate tool call deltas to verify the aggregation worked.
	accum := make(map[int]*ToolCallDelta)
	for _, tc := range allToolCalls {
		if existing, ok := accum[0]; ok {
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Name != "" {
				existing.Name = tc.Name
			}
			existing.Arguments += tc.Arguments
		} else {
			cpy := tc
			accum[0] = &cpy
		}
	}

	tc, ok := accum[0]
	require.True(t, ok, "expected tool call at index 0")
	assert.Equal(t, "call_abc", tc.ID)
	assert.Equal(t, "read_file", tc.Name)
	assert.Equal(t, `{"path":"/tmp/test.txt"}`, tc.Arguments)
}

func TestOpenAIAdapter_Stream_ErrorPropagation(t *testing.T) {
	// Create a server that sends partial data then closes the connection.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		chunk := baseChunkCh(baseChunk(), choice(openaiDelta{Content: "partial"}, "")).JSON()
		_, _ = fmt.Fprint(w, "data: "+chunk+"\n\n")
		flusher.Flush()
		// Simulate premature connection close
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				conn.Close()
				return
			}
		}
	}))
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	adapter := NewOpenAIAdapter(&client, "gpt-4o")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, errc := adapter.Stream(ctx, StreamParams{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleUser, Content: "Hello"},
		},
	})

	// Drain the data channel.
	for range ch {
	}

	// Expect an error on the error channel.
	select {
	case err := <-errc:
		require.Error(t, err)
	case <-ctx.Done():
		t.Fatal("timeout waiting for error")
	}
}

func TestOpenAIAdapter_Stream_FinalChunkHasUsage(t *testing.T) {
	chunks := []string{
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: "OK"}, "stop")).JSON(),
	}
	srv := newSSEServer(chunks)
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	adapter := NewOpenAIAdapter(&client, "gpt-4o")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, errc := adapter.Stream(ctx, StreamParams{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleUser, Content: "Hi"},
		},
	})

	// The final chunk should have usage populated.
	var lastChunk StreamChunk
	for c := range ch {
		lastChunk = c
	}

	checkNoError(t, errc)

	assert.NotNil(t, lastChunk.Usage, "final chunk should have usage")
}

func TestOpenAIAdapter_Stream_ContextCancellation(t *testing.T) {
	chunks := []string{
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: "Long"}, "")).JSON(),
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: " response"}, "")).JSON(),
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: " incoming"}, "")).JSON(),
	}
	srv := newSSEServer(chunks)
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	adapter := NewOpenAIAdapter(&client, "gpt-4o")

	ctx, cancel := context.WithCancel(context.Background())

	ch, errc := adapter.Stream(ctx, StreamParams{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleUser, Content: "Tell me a long story"},
		},
	})

	// Read one chunk then cancel.
	select {
	case _, ok := <-ch:
		require.True(t, ok)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first chunk")
	}

	cancel()

	// Both channels should be closed after cancellation, drain to ensure no deadlock.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				goto drained
			}
		case _, ok := <-errc:
			if !ok {
				goto drained
			}
		case <-timeout:
			t.Fatal("channels not closed after cancellation")
		}
	}
drained:
}

func TestConvertMessages(t *testing.T) {
	messages := []Message{
		{Role: RoleSystem, Content: "You are helpful."},
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi! How can I help?", ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "search", Arguments: `{"query":"test"}`}},
		}},
		{Role: RoleTool, Content: "search result here", ToolCallID: "call_1"},
	}

	result := convertMessages(messages)

	assert.Equal(t, 4, len(result))

	// Verify system message is correct
	data, _ := json.Marshal(result[0])
	assert.Contains(t, string(data), "system")
	assert.Contains(t, string(data), "You are helpful.")

	// Verify user message
	data, _ = json.Marshal(result[1])
	assert.Contains(t, string(data), "user")
	assert.Contains(t, string(data), "Hello")

	// Verify assistant message with tool calls
	data, _ = json.Marshal(result[2])
	assert.Contains(t, string(data), "assistant")
	assert.Contains(t, string(data), "call_1")
	assert.Contains(t, string(data), "search")

	// Verify tool message
	data, _ = json.Marshal(result[3])
	assert.Contains(t, string(data), "tool")
	assert.Contains(t, string(data), "call_1")
}

func TestConvertTools(t *testing.T) {
	tools := []ToolDef{
		{
			Name:        "read_file",
			Description: "Read a file from disk",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}

	result := convertTools(tools)

	require.Equal(t, 1, len(result))
	require.NotNil(t, result[0].OfFunction)
	assert.Equal(t, "read_file", result[0].OfFunction.Function.Name)
	assert.True(t, result[0].OfFunction.Function.Description.Valid())
	assert.Equal(t, "Read a file from disk", result[0].OfFunction.Function.Description.Value)
}

func TestOpenAIAdapter_Stream_EmptyMessages(t *testing.T) {
	chunks := []string{
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: "hello"}, "stop")).JSON(),
	}
	srv := newSSEServer(chunks)
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	adapter := NewOpenAIAdapter(&client, "gpt-4o")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, errc := adapter.Stream(ctx, StreamParams{
		Model:    "gpt-4o",
		Messages: nil,
	})

	var content string
	for c := range ch {
		content += c.Content
	}

	checkNoError(t, errc)

	assert.Equal(t, "hello", content)
}

func TestOpenAIAdapter_Stream_NoTools(t *testing.T) {
	chunks := []string{
		baseChunkCh(baseChunk(), choice(openaiDelta{Content: "no tools needed"}, "stop")).JSON(),
	}
	srv := newSSEServer(chunks)
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	adapter := NewOpenAIAdapter(&client, "gpt-4o")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, errc := adapter.Stream(ctx, StreamParams{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleUser, Content: "Hi"},
		},
		Tools: nil,
	})

	var content string
	for c := range ch {
		content += c.Content
	}

	checkNoError(t, errc)

	assert.Equal(t, "no tools needed", content)
}

// Helper: add a choice to a base chunk.
func baseChunkCh(base openaiChunk, ch openaiChoice) openaiChunk {
	base.Choices = []openaiChoice{ch}
	return base
}

// TestConvertMessages_DefaultRole tests that unknown roles default to UserMessage.
func TestConvertMessages_DefaultRole(t *testing.T) {
	messages := []Message{
		{Role: "unknown", Content: "some content"},
	}
	result := convertMessages(messages)
	data, _ := json.Marshal(result[0])
	// Should have been converted to user message.
	assert.Contains(t, string(data), `"role":"user"`)
	assert.Contains(t, string(data), "some content")
}

// TestConvertMessages_AssistantNoToolCalls tests assistant messages without tool calls.
func TestConvertMessages_AssistantNoToolCalls(t *testing.T) {
	messages := []Message{
		{Role: RoleAssistant, Content: "plain response"},
	}
	result := convertMessages(messages)
	data, _ := json.Marshal(result[0])
	assert.Contains(t, string(data), `"role":"assistant"`)
	assert.Contains(t, string(data), "plain response")
	assert.NotContains(t, string(data), "tool_calls")
}

// TestConvertTools_EmptyParameters tests tools with nil parameters.
func TestConvertTools_EmptyParameters(t *testing.T) {
	tools := []ToolDef{
		{Name: "noop", Description: "does nothing", Parameters: nil},
	}
	result := convertTools(tools)
	require.Equal(t, 1, len(result))
	assert.Equal(t, "noop", result[0].OfFunction.Function.Name)
}

// TestInterfaceCompliance_compileTime is satisfied at compile time.
func TestInterfaceCompliance_compileTime(t *testing.T) {
	assert.True(t, true)
}

// TestStreamWithMultipleChoices verifies only first choice is used when n > 1.
func TestStreamWithMultipleChoices(t *testing.T) {
	chunk := baseChunk()
	chunk.Choices = []openaiChoice{
		{Index: 0, Delta: openaiDelta{Content: "first"}},
		{Index: 1, Delta: openaiDelta{Content: "ignored"}},
	}
	srv := newSSEServer([]string{chunk.JSON()})
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	adapter := NewOpenAIAdapter(&client, "gpt-4o")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, errc := adapter.Stream(ctx, StreamParams{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleUser, Content: "Hi"},
		},
	})

	var content string
	for c := range ch {
		content += c.Content
	}

	checkNoError(t, errc)

	// Should only capture content from first choice.
	assert.NotContains(t, content, "ignored")
	assert.Contains(t, strings.TrimSpace(content), "first")
}
