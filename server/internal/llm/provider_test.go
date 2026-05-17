package llm

import (
	"context"
	"encoding/json"
	"testing"
)

// compileTimeCheck ensures the interface is usable (type-checks at build time).
func TestInterfaceCompliance(t *testing.T) {
	// This is a compile-time check: if mockProvider doesn't satisfy
	// LLMProvider, this file won't build.
	var _ LLMProvider = &mockProvider{}
}

// mockProvider implements LLMProvider for testing purposes.
type mockProvider struct{}

func (m *mockProvider) Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error) {
	ch := make(chan StreamChunk)
	errc := make(chan error, 1)
	close(ch)
	close(errc)
	return ch, errc
}

func TestStreamParamsMarshalUnmarshal(t *testing.T) {
	original := StreamParams{
		Model:       "gpt-4o",
		Temperature: 0.7,
		MaxTokens:   4096,
		Messages: []Message{
			{Role: RoleSystem, Content: "You are helpful."},
			{Role: RoleUser, Content: "Hello"},
			{
				Role:    RoleAssistant,
				Content: "Hi!",
				ToolCalls: []ToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: FunctionCall{
							Name:      "read_file",
							Arguments: `{"path":"/tmp/test.txt"}`,
						},
					},
				},
			},
			{Role: RoleTool, ToolCallID: "call_1", Content: "file contents here", Name: "read_file"},
		},
		Tools: []ToolDef{
			{
				Name:        "read_file",
				Description: "Read a file from disk",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal StreamParams: %v", err)
	}

	var restored StreamParams
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal StreamParams: %v", err)
	}

	if restored.Model != original.Model {
		t.Errorf("Model: got %q, want %q", restored.Model, original.Model)
	}
	if restored.Temperature != original.Temperature {
		t.Errorf("Temperature: got %v, want %v", restored.Temperature, original.Temperature)
	}
	if restored.MaxTokens != original.MaxTokens {
		t.Errorf("MaxTokens: got %v, want %v", restored.MaxTokens, original.MaxTokens)
	}
	if len(restored.Messages) != len(original.Messages) {
		t.Fatalf("Messages len: got %d, want %d", len(restored.Messages), len(original.Messages))
	}

	// Check assistant message with tool calls
	asst := restored.Messages[2]
	if len(asst.ToolCalls) != 1 {
		t.Fatalf("assistant ToolCalls len: got %d, want 1", len(asst.ToolCalls))
	}
	if asst.ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("tool call name: got %q, want read_file", asst.ToolCalls[0].Function.Name)
	}
	if asst.ToolCalls[0].Function.Arguments != `{"path":"/tmp/test.txt"}` {
		t.Errorf("tool call args: got %q", asst.ToolCalls[0].Function.Arguments)
	}

	// Check tool message
	tool := restored.Messages[3]
	if tool.Role != RoleTool {
		t.Errorf("tool role: got %q, want %q", tool.Role, RoleTool)
	}
	if tool.ToolCallID != "call_1" {
		t.Errorf("ToolCallID: got %q, want call_1", tool.ToolCallID)
	}
	if tool.Name != "read_file" {
		t.Errorf("tool Name: got %q, want read_file", tool.Name)
	}

	// Check tools
	if len(restored.Tools) != 1 {
		t.Fatalf("Tools len: got %d, want 1", len(restored.Tools))
	}
	if restored.Tools[0].Name != "read_file" {
		t.Errorf("tool Name: got %q, want read_file", restored.Tools[0].Name)
	}
}

func TestStreamChunkMarshalUnmarshal(t *testing.T) {
	usage := &Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	original := StreamChunk{
		Content:          "Hello",
		ReasoningContent: "I should say hello",
		ToolCalls: []ToolCallDelta{
			{ID: "call_abc", Name: "read_file", Arguments: `{"path":"`},
			{ID: "call_abc", Name: "", Arguments: `/tmp/test.txt"}`},
		},
		Usage:        usage,
		FinishReason: "stop",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal StreamChunk: %v", err)
	}

	var restored StreamChunk
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal StreamChunk: %v", err)
	}

	if restored.Content != "Hello" {
		t.Errorf("Content: got %q, want Hello", restored.Content)
	}
	if restored.ReasoningContent != "I should say hello" {
		t.Errorf("ReasoningContent: got %q, want %q", restored.ReasoningContent, "I should say hello")
	}
	if restored.FinishReason != "stop" {
		t.Errorf("FinishReason: got %q, want stop", restored.FinishReason)
	}
	if len(restored.ToolCalls) != 2 {
		t.Fatalf("ToolCalls len: got %d, want 2", len(restored.ToolCalls))
	}
	if restored.Usage == nil {
		t.Fatal("Usage is nil")
	}
	if restored.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens: got %d, want 100", restored.Usage.PromptTokens)
	}
	if restored.Usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens: got %d, want 50", restored.Usage.CompletionTokens)
	}
	if restored.Usage.TotalTokens != 150 {
		t.Errorf("TotalTokens: got %d, want 150", restored.Usage.TotalTokens)
	}
}

func TestStreamChunkNilUsage(t *testing.T) {
	// Usage is nil for intermediate chunks — verify marshal/unmarshal handles it.
	original := StreamChunk{
		Content: "partial",
		Usage:   nil,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored StreamChunk
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.Usage != nil {
		t.Errorf("Usage should be nil, got %+v", restored.Usage)
	}
}

func TestToolDefParametersRawMessage(t *testing.T) {
	// Verify json.RawMessage preserves raw JSON faithfully.
	td := ToolDef{
		Name:        "search",
		Description: "Search the web",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"}},"required":["query"]}`),
	}

	data, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Re-parse as generic map to check round-trip
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	params, ok := raw["parameters"]
	if !ok {
		t.Fatal("missing parameters key")
	}
	paramsMap, ok := params.(map[string]any)
	if !ok {
		t.Fatal("parameters is not a JSON object")
	}
	if paramsMap["type"] != "object" {
		t.Errorf("parameters.type: got %v, want object", paramsMap["type"])
	}
}
