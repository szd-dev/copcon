package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// Compile-time check: OpenAIAdapter implements LLMProvider.
var _ LLMProvider = (*OpenAIAdapter)(nil)

// deltaExtraFields captures provider-specific fields not in the standard delta
// struct, such as reasoning_content for deep‑seek models.
type deltaExtraFields struct {
	ReasoningContent string `json:"reasoning_content"`
}

// OpenAIAdapter wraps *openai.Client to implement LLMProvider.
type OpenAIAdapter struct {
	client *openai.Client
	model  string
}

// NewOpenAIAdapter creates an OpenAIAdapter backed by the given client.
func NewOpenAIAdapter(client *openai.Client, model string) *OpenAIAdapter {
	return &OpenAIAdapter{client: client, model: model}
}

// Stream sends a streaming completion request and returns two channels:
// ch delivers StreamChunk values; errc delivers at most one error.
// Both channels are closed by the adapter when the stream ends.
func (a *OpenAIAdapter) Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error) {
	openAIMsgs := convertMessages(params.Messages)
	openAITools := convertTools(params.Tools)

	req := openai.ChatCompletionNewParams{
		Model:             shared.ChatModel(params.Model),
		Messages:          openAIMsgs,
		Tools:             openAITools,
		ParallelToolCalls: openai.Bool(true),
	}

	stream := a.client.Chat.Completions.NewStreaming(ctx, req)

	ch := make(chan StreamChunk)
	errc := make(chan error, 1)

	go func() {
		defer close(ch)
		defer close(errc)
		defer stream.Close()

		acc := openai.ChatCompletionAccumulator{}
		toolCallMap := make(map[int]*toolCallAccum)

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			select {
			case <-ctx.Done():
				return
			default:
			}

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				finishReason := chunk.Choices[0].FinishReason

				chunkOut := StreamChunk{}

				if delta.Content != "" {
					chunkOut.Content = delta.Content
				}

				// Extract reasoning_content from raw JSON extra fields.
				var extra deltaExtraFields
				if err := json.Unmarshal([]byte(delta.RawJSON()), &extra); err == nil {
					if extra.ReasoningContent != "" {
						chunkOut.ReasoningContent = extra.ReasoningContent
					}
				}

				// Accumulate tool call deltas by index.
				if len(delta.ToolCalls) > 0 {
					chunkOut.ToolCalls = make([]ToolCallDelta, 0, len(delta.ToolCalls))
					for _, tc := range delta.ToolCalls {
						idx := int(tc.Index)
						delta := ToolCallDelta{
							Index:     idx,
							ID:        tc.ID,
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						}
						if existing, ok := toolCallMap[idx]; ok {
							if tc.ID != "" {
								existing.ID = tc.ID
							}
							if tc.Function.Name != "" {
								existing.Name = tc.Function.Name
							}
							if tc.Function.Arguments != "" {
								existing.Arguments += tc.Function.Arguments
							}
							// Forward the delta so callers can see incremental
							// tool call data in real time.
							chunkOut.ToolCalls = append(chunkOut.ToolCalls, ToolCallDelta{
								Index:     idx,
								ID:        existing.ID,
								Name:      existing.Name,
								Arguments: tc.Function.Arguments,
							})
						} else {
							toolCallMap[idx] = &toolCallAccum{
								ID:        tc.ID,
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							}
							chunkOut.ToolCalls = append(chunkOut.ToolCalls, delta)
						}
					}
				}

				// Also handle tool calls that arrive via the accumulator
				// (JustFinishedToolCall) — these are complete tool calls that
				// appeared entirely within a single chunk.
				if finished, ok := acc.JustFinishedToolCall(); ok {
					found := false
					for _, existing := range toolCallMap {
						if existing.ID == finished.ID {
							found = true
							break
						}
					}
					if !found {
						toolCallMap[len(toolCallMap)] = &toolCallAccum{
							ID:        finished.ID,
							Name:      finished.Name,
							Arguments: finished.Arguments,
						}
						chunkOut.ToolCalls = append(chunkOut.ToolCalls, ToolCallDelta{
							Index:     len(toolCallMap) - 1,
							ID:        finished.ID,
							Name:      finished.Name,
							Arguments: finished.Arguments,
						})
					}
				}

				// Set finish reason if present.
				if finishReason != "" {
					chunkOut.FinishReason = finishReason
				}

				// Only send if the chunk has meaningful content.
				if chunkOut.Content != "" || chunkOut.ReasoningContent != "" ||
					len(chunkOut.ToolCalls) > 0 || chunkOut.FinishReason != "" {
					select {
					case ch <- chunkOut:
					case <-ctx.Done():
						return
					}
				}
			}
		}

		// Check stream error.
		if err := stream.Err(); err != nil {
			select {
			case errc <- fmt.Errorf("openai stream: %w", err):
			case <-ctx.Done():
			}
			return
		}

		// Send final chunk with usage statistics.
		final := StreamChunk{
			Usage: &Usage{
				PromptTokens:     acc.Usage.PromptTokens,
				CompletionTokens: acc.Usage.CompletionTokens,
				TotalTokens:      acc.Usage.TotalTokens,
			},
			FinishReason: acc.Choices[0].FinishReason,
		}
		select {
		case ch <- final:
		case <-ctx.Done():
		}
	}()

	return ch, errc
}

// toolCallAccum accumulates tool call deltas by index during streaming.
type toolCallAccum struct {
	ID        string
	Name      string
	Arguments string
}

// convertMessages converts the provider-agnostic Message slice to
// openai.ChatCompletionMessageParamUnion values.
func convertMessages(messages []Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			result = append(result, openai.SystemMessage(msg.Content))
		case RoleUser:
			result = append(result, openai.UserMessage(msg.Content))
		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						},
					})
				}
				asst := openai.AssistantMessage(msg.Content)
				asst.OfAssistant.ToolCalls = toolCalls
				result = append(result, asst)
			} else {
				result = append(result, openai.AssistantMessage(msg.Content))
			}
		case RoleTool:
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		default:
			result = append(result, openai.UserMessage(msg.Content))
		}
	}
	return result
}

// convertTools converts the provider-agnostic ToolDef slice to
// openai.ChatCompletionToolUnionParam values.
func convertTools(tools []ToolDef) []openai.ChatCompletionToolUnionParam {
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, td := range tools {
		// Unmarshal parameters JSON into shared.FunctionParameters (map[string]any).
		var params shared.FunctionParameters
		if len(td.Parameters) > 0 {
			_ = json.Unmarshal(td.Parameters, &params)
		}
		result = append(result, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        td.Name,
			Description: param.NewOpt(td.Description),
			Parameters:  params,
		}))
	}
	return result
}
