package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"golang.org/x/sync/semaphore"

	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/hook"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
)

var (
	ErrNoSession = errors.New("session not found")
)

type deltaExtraFields struct {
	ReasoningContent string `json:"reasoning_content"`
}

// StreamResult encapsulates the result of streaming an LLM response.
// It contains all accumulated content, reasoning, and tool calls from the stream.
// ToolCallResult holds the output or error from a tool execution,
// used to populate PersistedPart state and output at persist time.
type ToolCallResult struct {
	Output string
	Error  string
}

type StreamResult struct {
	MessageID        string
	StepIndex        int
	Content          string
	ReasoningContent string
	ToolCalls        []toolCallInfo
	ToolResults      map[string]*ToolCallResult // callID → result
	Usage            openai.CompletionUsage
}

// LLMRequest encapsulates the parameters for an LLM API call.
// It groups the model, messages, and tools for cleaner function signatures.
type LLMRequest struct {
	Model    string
	Messages []openai.ChatCompletionMessageParamUnion
	Tools    []openai.ChatCompletionToolUnionParam
}

// AgentEngine defines the public interface for the agent engine.
type AgentEngine interface {
	Chat(chatCtx iface.ChatContextInterface, userInput string) error
}

// EngineOption configures an AgentEngine created by NewAgentEngine.
type EngineOption func(*engineImpl)

// WithConcurrency sets the maximum number of concurrent tool executions.
// n must be greater than 0, otherwise it panics.
func WithConcurrency(n int) EngineOption {
	return func(e *engineImpl) {
		if n <= 0 {
			panic(fmt.Sprintf("WithConcurrency: n must be > 0, got %d", n))
		}
		e.concurrency = n
	}
}

// WithLogger sets the structured logger on the engine.
func WithLogger(logger *slog.Logger) EngineOption {
	return func(e *engineImpl) {
		e.logger = logger
	}
}

// WithHookRunner sets the hook runner for lifecycle hook execution.
// When nil (default), hooks are not executed.
func WithHookRunner(runner hook.HookRunner) EngineOption {
	return func(e *engineImpl) {
		e.hookRunner = runner
	}
}

type engineImpl struct {
	logger         *slog.Logger
	agentRegistry  AgentRegistry
	sessionMgr     session.SessionManager
	contextMgr     chat_context.ContextManager
	concurrency    int                     // max concurrent tool executions
	concurrencySem *semaphore.Weighted     // limit concurrent tool executions
	asyncRegistry  *tool.AsyncToolRegistry // tracks async tool executions
	hookRunner     hook.HookRunner         // optional hook runner for lifecycle hooks
}

var _ AgentEngine = (*engineImpl)(nil)

func NewAgentEngine(
	agentRegistry AgentRegistry,
	sessionMgr session.SessionManager,
	contextMgr chat_context.ContextManager,
	asyncRegistry *tool.AsyncToolRegistry,
	opts ...EngineOption,
) AgentEngine {
	e := &engineImpl{
		agentRegistry: agentRegistry,
		sessionMgr:    sessionMgr,
		contextMgr:    contextMgr,
		asyncRegistry: asyncRegistry,
		concurrency:   5,
		logger:        slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	for _, opt := range opts {
		opt(e)
	}
	e.concurrencySem = semaphore.NewWeighted(int64(e.concurrency))
	return e
}

func (e *engineImpl) Chat(chatCtx iface.ChatContextInterface, userInput string) error {
	if err := e.runAgentLoop(chatCtx, userInput); err != nil {
		chatCtx.Emit(entity.Event{
			Type: entity.EventError,
			Data: entity.ErrorData{Error: err.Error()},
		})
	}
	return nil
}

// prepareAgentLoop handles the initialization phase of the agent loop.
// It retrieves the session, resolves the agent (with 3-layer fallback),
// and persists the user message.
// Original location: runAgentLoop lines 72-102.
func (e *engineImpl) prepareAgentLoop(chatCtx iface.ChatContextInterface, userInput string) (*AgentDefinition, error) {
	sess, err := e.sessionMgr.Get(chatCtx)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Determine which agent to use (3-layer fallback)
	agentID := chatCtx.AgentID()
	if agentID == "" {
		agentID = sess.DefaultAgentID
	}
	if agentID == "" {
		// Fall back to default agent from registry
		defaultAgent, err := e.agentRegistry.Default()
		if err != nil {
			return nil, fmt.Errorf("no agent specified and no default agent: %w", err)
		}
		agentID = defaultAgent.ID
	}

	// Get agent definition
	agentDef, err := e.agentRegistry.Get(agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}

	// Skip user message persistence for empty input.
	// This allows async tool completions to trigger new agent loop rounds
	// when SSE reconnects or frontend polls for pending events.
	// The context will already have the async tool result persisted by the goroutine.
	if userInput != "" {
		if err := e.contextMgr.AddMessage(chatCtx, &session.Message{
			Role:    "user",
			Content: userInput,
		}); err != nil {
			return nil, fmt.Errorf("add user message: %w", err)
		}
	}

	return &agentDef, nil
}

// handleStreaming processes the streaming response from OpenAI, accumulating
// content, reasoning, and tool calls while emitting part-level events in real-time.
// It emits only new UI-layer events (EventPartCreate, EventPartUpdate).
func (e *engineImpl) handleStreaming(
	chatCtx iface.ChatContextInterface,
	agentDef *AgentDefinition,
	openAIMessages []openai.ChatCompletionMessageParamUnion,
	tools []openai.ChatCompletionToolUnionParam,
	messageID string,
	stepIndex int,
) (*StreamResult, error) {
	params := openai.ChatCompletionNewParams{
		Model:             openai.ChatModel(agentDef.Model),
		Messages:          openAIMessages,
		Tools:             tools,
		ParallelToolCalls: openai.Bool(true),
	}

	stream := agentDef.OpenAIClient.Chat.Completions.NewStreaming(chatCtx.Context(), params)
	acc := openai.ChatCompletionAccumulator{}

	result := &StreamResult{
		MessageID: messageID,
		StepIndex: stepIndex,
	}
	toolCallMap := make(map[int]*toolCallInfo)

	partIndex := 0
	textPartCreated := false
	textPartIndex := -1
	reasoningPartCreated := false
	reasoningPartIndex := -1

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta

			if delta.Content != "" {
				result.Content += delta.Content

				if !textPartCreated {
					chatCtx.Emit(entity.Event{
						Type: entity.EventPartCreate,
						Data: entity.PartCreateData{
							MessageID: messageID,
							StepIndex: stepIndex,
							PartIndex: partIndex,
							PartType:  "text",
							State:     "streaming",
						},
					})
					textPartIndex = partIndex
					partIndex++
					textPartCreated = true
				}
				chatCtx.Emit(entity.Event{
					Type: entity.EventPartUpdate,
					Data: entity.PartUpdateData{
						MessageID: messageID,
						StepIndex: stepIndex,
						PartIndex: textPartIndex,
						PartType:  "text",
						TextDelta: delta.Content,
					},
				})
			}

			var extra deltaExtraFields
			if err := json.Unmarshal([]byte(delta.RawJSON()), &extra); err == nil {
				if extra.ReasoningContent != "" {
					result.ReasoningContent += extra.ReasoningContent

					if !reasoningPartCreated {
						chatCtx.Emit(entity.Event{
							Type: entity.EventPartCreate,
							Data: entity.PartCreateData{
								MessageID: messageID,
								StepIndex: stepIndex,
								PartIndex: partIndex,
								PartType:  "reasoning",
								State:     "streaming",
							},
						})
						reasoningPartIndex = partIndex
						partIndex++
						reasoningPartCreated = true
					}
					chatCtx.Emit(entity.Event{
						Type: entity.EventPartUpdate,
						Data: entity.PartUpdateData{
							MessageID: messageID,
							StepIndex: stepIndex,
							PartIndex: reasoningPartIndex,
							PartType:  "reasoning",
							TextDelta: extra.ReasoningContent,
						},
					})
				}
			}

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
							MessageID: messageID,
						}
					}
				}
			}
		}

		// JustFinishedToolCall handles edge cases where tool call info
		// arrives via accumulator rather than delta.ToolCalls
		if tool, ok := acc.JustFinishedToolCall(); ok {
			found := false
			for _, tc := range toolCallMap {
				if tc.ID == tool.ID {
					found = true
					break
				}
			}
			if !found {
				toolCallMap[len(toolCallMap)] = &toolCallInfo{
					ID:        tool.ID,
					Name:      tool.Name,
					Arguments: tool.Arguments,
				}
			}
		}
	}

	for i := 0; i < len(toolCallMap); i++ {
		if tc, ok := toolCallMap[i]; ok {
			result.ToolCalls = append(result.ToolCalls, *tc)
		}
	}

	if err := stream.Err(); err != nil {
		e.logger.Error("llm_stream_error",
			"session_id", chatCtx.SessionID(),
			"error", err,
		)
		return nil, fmt.Errorf("stream error: %w", err)
	}

	result.Usage = acc.Usage

	// Emit part-level state transitions for completed parts
	if reasoningPartCreated {
		chatCtx.Emit(entity.Event{
			Type: entity.EventPartUpdate,
			Data: entity.PartUpdateData{
				MessageID: messageID,
				StepIndex: stepIndex,
				PartIndex: reasoningPartIndex,
				PartType:  "reasoning",
				State:     "done",
			},
		})
	}
	if textPartCreated {
		chatCtx.Emit(entity.Event{
			Type: entity.EventPartUpdate,
			Data: entity.PartUpdateData{
				MessageID: messageID,
				StepIndex: stepIndex,
				PartIndex: textPartIndex,
				PartType:  "text",
				State:     "done",
			},
		})
	}

	e.logger.Info("llm_response",
		"session_id", chatCtx.SessionID(),
		"reasoning_len", len(result.ReasoningContent),
		"content_len", len(result.Content),
		"tool_calls", len(result.ToolCalls),
		"prompt_tokens", result.Usage.PromptTokens,
		"completion_tokens", result.Usage.CompletionTokens,
		"total_tokens", result.Usage.TotalTokens,
	)

	return result, nil
}

// logLLMRequest logs the LLM request parameters for debugging.
func (e *engineImpl) logLLMRequest(agentDef *AgentDefinition, messages []entity.MessageForLLM, tools []openai.ChatCompletionToolUnionParam, sessionID string) {
	e.logger.Info("llm_request",
		"session_id", sessionID,
		"agent", agentDef.Name,
		"model", agentDef.Model,
		"message_count", len(messages),
		"tool_count", len(tools),
	)
}

func (e *engineImpl) runAgentLoop(chatCtx iface.ChatContextInterface, userInput string) error {
	// Phase 1: Initialization
	agentDef, err := e.prepareAgentLoop(chatCtx, userInput)
	if err != nil {
		return err
	}

	// Tool list is static per agent, retrieve once outside loop
	tools := agentDef.ToolManager.GetOpenAITools()

	// Generate messageID once for the entire agent loop
	messageID := uuid.New().String()

	// Track iterations to emit step_create events on subsequent rounds
	isFirstIteration := true
	stepIndex := 0

	// Phase 2: Agent Loop
	for {
		if !isFirstIteration {
			chatCtx.Emit(entity.Event{
				Type: entity.EventStepCreate,
				Data: entity.StepCreateData{
					MessageID: messageID,
					StepIndex: stepIndex,
				},
			})
		}
		isFirstIteration = false

		// Build LLM context
		systemPrompt := agentDef.SystemPrompt
		if e.hookRunner != nil {
			e.hookRunner.Run(hook.OnSystemPrompt, &hook.HookContext{
				ChatCtx:      chatCtx,
				SessionID:    chatCtx.SessionID(),
				AgentID:      chatCtx.AgentID(),
				SystemPrompt: &systemPrompt,
				Logger:       e.logger,
				CurrentPoint: hook.OnSystemPrompt,
			})
		}
		messages, err := e.contextMgr.BuildContext(chatCtx, "", 256000, systemPrompt)
		if err != nil {
			return fmt.Errorf("build context: %w", err)
		}

		openAIMessages := e.convertMessages(messages)

		// Log request
		e.logLLMRequest(agentDef, messages, tools, chatCtx.SessionID())

		// Phase 3: Streaming
		result, err := e.handleStreaming(chatCtx, agentDef, openAIMessages, tools, messageID, stepIndex)
		if err != nil {
			return err
		}

		// Phase 4: Tool Call Handling
		shouldContinue, err := e.handleToolCalls(chatCtx, agentDef.ToolManager, result)
		if err != nil {
			return err
		}

		if !shouldContinue {
			return nil
		}

		stepIndex++
	}
}

func (e *engineImpl) convertMessages(messages []entity.MessageForLLM) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			result = append(result, openai.SystemMessage(msg.Content))
		case "user":
			result = append(result, openai.UserMessage(msg.Content))
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Build assistant message with tool_calls
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
		case "tool":
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		default:
			result = append(result, openai.UserMessage(msg.Content))
		}
	}
	return result
}

// persistMessage saves an assistant message to the context.
// If result has ToolCalls, they are included.
// If isFinal is true, the message ID is set.
// Parts JSONB is populated from the stream result for UIMessage format.
func (e *engineImpl) persistMessage(
	chatCtx iface.ChatContextInterface,
	result *StreamResult,
	isFinal bool,
) error {
	parts := e.buildUIParts(result, result.StepIndex)

	msg := &session.Message{
		Role:      "assistant",
		Content:   result.Content,
		Reasoning: result.ReasoningContent,
		Parts:     parts,
	}

	if isFinal {
		msg.ID = uuid.MustParse(result.MessageID)
	}

	if len(result.ToolCalls) > 0 {
		msg.ToolCalls = e.convertToolCalls(result.ToolCalls)
	}

	if err := e.contextMgr.AddMessage(chatCtx, msg); err != nil {
		return fmt.Errorf("add assistant message: %w", err)
	}

	return nil
}

func (e *engineImpl) buildUIParts(result *StreamResult, stepIndex int) session.PersistedParts {
	var parts session.PersistedParts

	if result.ReasoningContent != "" {
		parts = append(parts, session.PersistedPart{
			Type:      "reasoning",
			Text:      result.ReasoningContent,
			State:     "done",
			StepIndex: stepIndex,
		})
	}

	if result.Content != "" || len(result.ToolCalls) == 0 {
		parts = append(parts, session.PersistedPart{
			Type:      "text",
			Text:      result.Content,
			State:     "done",
			StepIndex: stepIndex,
		})
	}

	for _, tc := range result.ToolCalls {
		part := session.PersistedPart{
			Type:       "tool-call",
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Args:       tc.Arguments,
			StepIndex:  stepIndex,
		}

		if tr, ok := result.ToolResults[tc.ID]; ok {
			if tr.Error != "" {
				part.State = "error"
				part.Error = tr.Error
			} else {
				part.State = "complete"
				part.Output = tr.Output
			}
		} else {
			part.State = "pending"
		}

		parts = append(parts, part)
	}

	return parts
}
