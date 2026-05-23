package agent

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
)

type deltaExtraFields struct {
	ReasoningContent string `json:"reasoning_content"`
}

// MockOpenAIStream simulates OpenAI streaming behavior for testing.
// It implements the streaming interface used by engine.go (Next, Current, Err).
type MockOpenAIStream struct {
	chunks  []openai.ChatCompletionChunk
	current openai.ChatCompletionChunk
	index   int
	err     error
	chunkID string
	model   string
	created int64
}

// NewMockOpenAIStream creates a new mock stream with default values.
func NewMockOpenAIStream() *MockOpenAIStream {
	return &MockOpenAIStream{
		chunks:  make([]openai.ChatCompletionChunk, 0),
		chunkID: "chatcmpl-mock",
		model:   "gpt-4o",
		created: 1234567890,
	}
}

// Next advances to the next chunk. Returns false when no more chunks or error.
func (s *MockOpenAIStream) Next() bool {
	if s.err != nil {
		return false
	}
	if s.index >= len(s.chunks) {
		return false
	}
	s.current = s.chunks[s.index]
	s.index++
	return true
}

// Current returns the current chunk.
func (s *MockOpenAIStream) Current() openai.ChatCompletionChunk {
	return s.current
}

// Err returns any stream error that was set.
func (s *MockOpenAIStream) Err() error {
	return s.err
}

// SetError injects an error that will be returned by Err() and stops Next().
func (s *MockOpenAIStream) SetError(err error) {
	s.err = err
}

// AddContentChunk adds a chunk with content delta.
func (s *MockOpenAIStream) AddContentChunk(content string) {
	chunk := s.createBaseChunk()
	chunk.Choices = []openai.ChatCompletionChunkChoice{
		{
			Index: 0,
			Delta: openai.ChatCompletionChunkChoiceDelta{
				Content: content,
			},
		},
	}
	s.chunks = append(s.chunks, chunk)
}

// AddReasoningChunk adds a chunk with reasoning_content in the extra fields.
// This simulates the DeepSeek-style reasoning output.
func (s *MockOpenAIStream) AddReasoningChunk(content string) {
	// Build the full chunk JSON with reasoning_content in the delta
	chunkJSON := `{
		"id": "` + s.chunkID + `",
		"model": "` + s.model + `",
		"created": ` + strconv.FormatInt(s.created, 10) + `,
		"object": "chat.completion.chunk",
		"choices": [{
			"index": 0,
			"delta": {
				"reasoning_content": ` + jsonEscapeString(content) + `
			}
		}]
	}`

	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal([]byte(chunkJSON), &chunk); err == nil {
		s.chunks = append(s.chunks, chunk)
	}
}

// AddToolCallDelta adds a chunk with tool call delta.
// Multiple deltas with the same index will accumulate (name, arguments, id).
func (s *MockOpenAIStream) AddToolCallDelta(idx int, id, name, args string) {
	chunk := s.createBaseChunk()
	chunk.Choices = []openai.ChatCompletionChunkChoice{
		{
			Index: 0,
			Delta: openai.ChatCompletionChunkChoiceDelta{
				ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
					{
						Index: int64(idx),
						ID:    id,
						Type:  "function",
						Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
							Name:      name,
							Arguments: args,
						},
					},
				},
			},
		},
	}
	s.chunks = append(s.chunks, chunk)
}

// AddFinishChunk adds a chunk with finish_reason set.
func (s *MockOpenAIStream) AddFinishChunk(reason string) {
	chunk := s.createBaseChunk()
	chunk.Choices = []openai.ChatCompletionChunkChoice{
		{
			Index:        0,
			FinishReason: reason,
			Delta:        openai.ChatCompletionChunkChoiceDelta{},
		},
	}
	s.chunks = append(s.chunks, chunk)
}

// AddEmptyChunk adds a chunk with no delta content (heartbeat/keepalive style).
func (s *MockOpenAIStream) AddEmptyChunk() {
	chunk := s.createBaseChunk()
	chunk.Choices = []openai.ChatCompletionChunkChoice{
		{
			Index: 0,
			Delta: openai.ChatCompletionChunkChoiceDelta{},
		},
	}
	s.chunks = append(s.chunks, chunk)
}

// SetChunkID sets the ID used for all chunks (default: "chatcmpl-mock").
func (s *MockOpenAIStream) SetChunkID(id string) {
	s.chunkID = id
}

// SetModel sets the model name used for all chunks (default: "gpt-4o").
func (s *MockOpenAIStream) SetModel(model string) {
	s.model = model
}

// SetCreated sets the created timestamp used for all chunks.
func (s *MockOpenAIStream) SetCreated(created int64) {
	s.created = created
}

// createBaseChunk creates a chunk with common fields populated.
func (s *MockOpenAIStream) createBaseChunk() openai.ChatCompletionChunk {
	return openai.ChatCompletionChunk{
		ID:      s.chunkID,
		Model:   s.model,
		Created: s.created,
		Object:  "chat.completion.chunk",
	}
}

// jsonEscapeString properly escapes a string for JSON.
func jsonEscapeString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Reset clears all chunks and state, allowing reuse of the mock.
func (s *MockOpenAIStream) Reset() {
	s.chunks = make([]openai.ChatCompletionChunk, 0)
	s.current = openai.ChatCompletionChunk{}
	s.index = 0
	s.err = nil
}

// ChunkCount returns the number of queued chunks.
func (s *MockOpenAIStream) ChunkCount() int {
	return len(s.chunks)
}

// TestMockOpenAIStreamBasic verifies the mock stream works correctly.
func TestMockOpenAIStreamBasic(t *testing.T) {
	stream := NewMockOpenAIStream()
	stream.AddContentChunk("Hello")
	stream.AddContentChunk(" World")
	stream.AddFinishChunk("stop")

	assert.Equal(t, 3, stream.ChunkCount())

	var content string
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			content += chunk.Choices[0].Delta.Content
		}
	}
	assert.NoError(t, stream.Err())
	assert.Equal(t, "Hello World", content)
}

// TestMockOpenAIStreamReasoning verifies reasoning content extraction.
func TestMockOpenAIStreamReasoning(t *testing.T) {
	stream := NewMockOpenAIStream()
	stream.AddReasoningChunk("Thinking...")
	stream.AddReasoningChunk(" About something")
	stream.AddContentChunk("Final answer")
	stream.AddFinishChunk("stop")

	var reasoning, content string
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			if delta.Content != "" {
				content += delta.Content
			}
			var extra deltaExtraFields
			if err := json.Unmarshal([]byte(delta.RawJSON()), &extra); err == nil {
				if extra.ReasoningContent != "" {
					reasoning += extra.ReasoningContent
				}
			}
		}
	}
	assert.NoError(t, stream.Err())
	assert.Equal(t, "Thinking... About something", reasoning)
	assert.Equal(t, "Final answer", content)
}

// TestMockOpenAIStreamToolCall verifies tool call delta accumulation.
func TestMockOpenAIStreamToolCall(t *testing.T) {
	stream := NewMockOpenAIStream()
	stream.AddToolCallDelta(0, "call-123", "", "")
	stream.AddToolCallDelta(0, "", "get_weather", "")
	stream.AddToolCallDelta(0, "", "", "{\"location\":\"NYC\"}")
	stream.AddFinishChunk("tool_calls")

	toolCallMap := make(map[int]*toolCallInfo)
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			if len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {
					idx := int(tc.Index)
					if existing, ok := toolCallMap[idx]; ok {
						if tc.Function.Name != "" {
							existing.Name = tc.Function.Name
						}
						if tc.Function.Arguments != "" {
							existing.Arguments += tc.Function.Arguments
						}
						if tc.ID != "" {
							existing.ID = tc.ID
						}
					} else {
						toolCallMap[idx] = &toolCallInfo{
							ID:        tc.ID,
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						}
					}
				}
			}
		}
	}
	assert.NoError(t, stream.Err())
	assert.Equal(t, 1, len(toolCallMap))
	assert.Equal(t, "call-123", toolCallMap[0].ID)
	assert.Equal(t, "get_weather", toolCallMap[0].Name)
	assert.Equal(t, "{\"location\":\"NYC\"}", toolCallMap[0].Arguments)
}

// TestMockOpenAIStreamError verifies error injection.
func TestMockOpenAIStreamError(t *testing.T) {
	stream := NewMockOpenAIStream()
	stream.AddContentChunk("Hello")
	stream.SetError(assert.AnError)

	count := 0
	for stream.Next() {
		count++
	}
	assert.Equal(t, 0, count)
	assert.Error(t, stream.Err())
}
