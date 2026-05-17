// Package llm defines the provider abstraction for large language model backends.
//
// It provides a single LLMProvider interface with a streaming API, along with
// the shared types (StreamParams, Message, ToolDef, etc.) that decouple the
// agent engine from any concrete LLM SDK.
package llm

import (
	"context"
	"encoding/json"
)

// LLMProvider is the interface for any LLM backend (OpenAI, Anthropic, local, etc.).
// Implementations stream response chunks over a data channel, and report errors
// on a separate error channel. Both channels are owned by the provider: the data
// channel is closed when the stream ends (success or error), and the error channel
// receives at most one error before being closed.
type LLMProvider interface {
	// Stream sends a completion request and returns two channels:
	//   ch  – delivers StreamChunk values as they arrive from the LLM.
	//   errc – delivers a single error if the stream fails; closed after.
	//
	// The caller MUST read from ch until it is closed. The errc channel may be
	// checked after ch is exhausted, or concurrently via select.
	Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error)
}

// StreamParams groups the parameters for a streaming LLM completion request.
type StreamParams struct {
	// Model is the LLM model identifier (e.g. "gpt-4o", "claude-3-opus-20240229").
	Model string `json:"model"`

	// Messages is the conversation history, including the system prompt.
	Messages []Message `json:"messages"`

	// Tools is the list of tool/function definitions the LLM may call.
	// An empty or nil slice means no tools are available.
	Tools []ToolDef `json:"tools,omitempty"`

	// Temperature controls randomness (0.0–2.0). A value of 0 means no value
	// was set and the provider should use its default.
	Temperature float64 `json:"temperature,omitempty"`

	// MaxTokens limits the maximum number of tokens in the completion.
	// A value of 0 means no explicit limit was set.
	MaxTokens int `json:"max_tokens,omitempty"`
}

// StreamChunk represents a single incremental chunk from a streaming LLM response.
type StreamChunk struct {
	// Content is a delta of the main text response. Empty when there is no text
	// in this chunk (e.g. tool-call-only chunks).
	Content string `json:"content,omitempty"`

	// ReasoningContent is a delta of the model's internal reasoning / chain-of-thought.
	// Not all providers support this; it will be empty when unsupported or absent.
	ReasoningContent string `json:"reasoning_content,omitempty"`

	// ToolCalls holds incremental tool call deltas for this chunk. Multiple
	// tool calls may appear in a single chunk (e.g. when the model starts
	// several tool calls in parallel). The caller is responsible for accumulating
	// deltas by ID.
	ToolCalls []ToolCallDelta `json:"tool_calls,omitempty"`

	// Usage contains token usage statistics for the entire request. It is
	// typically nil for intermediate chunks and populated only in the final chunk.
	Usage *Usage `json:"usage,omitempty"`

	// FinishReason indicates why the LLM stopped (e.g. "stop", "length",
	// "tool_calls"). Empty for intermediate chunks.
	FinishReason string `json:"finish_reason,omitempty"`
}

// StreamRole enumerates the possible roles of a Message.
type StreamRole string

const (
	RoleSystem    StreamRole = "system"
	RoleUser      StreamRole = "user"
	RoleAssistant StreamRole = "assistant"
	RoleTool      StreamRole = "tool"
)

// Message represents a single message in a conversation.
type Message struct {
	// Role is the speaker role: system, user, assistant, or tool.
	Role StreamRole `json:"role"`

	// Content is the message body. For assistant messages with tool calls, this
	// may be empty or contain text that accompanies the tool calls.
	Content string `json:"content"`

	// ToolCalls is present only on assistant messages that request tool
	// invocations.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID is present only on tool messages; it links the result back to
	// the assistant's tool call request.
	ToolCallID string `json:"tool_call_id,omitempty"`

	// Name is an optional participant name. It is rarely used but supported for
	// providers that differentiate named participants.
	Name string `json:"name,omitempty"`
}

// ToolDef describes a tool/function that the LLM may request to call.
type ToolDef struct {
	// Name is the function name the LLM will use in tool call requests.
	Name string `json:"name"`

	// Description explains what the tool does; used by the LLM to decide
	// whether and when to call it.
	Description string `json:"description"`

	// Parameters is the JSON Schema describing the tool's input parameters.
	// RawMessage is used to avoid coupling to any specific provider's schema format.
	Parameters json.RawMessage `json:"parameters"`
}

// ToolCall represents a complete (non-streaming) tool call, typically found
// in assistant messages.
type ToolCall struct {
	// ID uniquely identifies this tool call within the conversation.
	ID string `json:"id"`

	// Type is the kind of tool call; for OpenAI-compatible providers this is
	// always "function".
	Type string `json:"type"`

	// Function contains the function name and arguments.
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the name and JSON-encoded arguments for a single
// function invocation.
type FunctionCall struct {
	// Name is the name of the function to call.
	Name string `json:"name"`

	// Arguments is the JSON-encoded arguments string (e.g. `{"file": "main.go"}`).
	Arguments string `json:"arguments"`
}

// ToolCallDelta is an incremental update to a tool call during streaming.
// It is used inside StreamChunk.ToolCalls. The caller accumulates deltas by
// matching on ID.
type ToolCallDelta struct {
	// ID is the unique tool call identifier. It may arrive in a later chunk
	// than Name/Arguments, so the caller should accumulate by index until the
	// ID is known.
	ID string `json:"id,omitempty"`

	// Name is the function name (may arrive incrementally).
	Name string `json:"name,omitempty"`

	// Arguments is a JSON fragment that should be concatenated with previous
	// deltas for the same tool call.
	Arguments string `json:"arguments,omitempty"`
}

// Usage holds token usage statistics for a completion request.
type Usage struct {
	// PromptTokens is the number of tokens in the prompt.
	PromptTokens int64 `json:"prompt_tokens"`

	// CompletionTokens is the number of tokens in the generated completion.
	CompletionTokens int64 `json:"completion_tokens"`

	// TotalTokens is the total number of tokens (prompt + completion).
	TotalTokens int64 `json:"total_tokens"`
}
