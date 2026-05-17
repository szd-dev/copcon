package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	openai "github.com/openai/openai-go/v3"
	"golang.org/x/sync/semaphore"

	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/hook"
	"github.com/copcon/server/internal/llm"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
)

var (
	ErrNoSession = errors.New("session not found")
)

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
	Usage            *llm.Usage
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

// WithLLMProvider sets the LLM provider for the engine.
// When nil (default), an OpenAIClient from the agent definition is used.
func WithLLMProvider(p llm.LLMProvider) EngineOption {
	return func(e *engineImpl) {
		e.llmProvider = p
	}
}

type engineImpl struct {
	logger         *slog.Logger
	agentRegistry  AgentRegistry
	sessionMgr     session.SessionManager
	contextMgr     chat_context.ContextManager
	llmProvider    llm.LLMProvider         // LLM provider (defaults to agent definition's provider)
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

	// Hook: OnSessionResolve
	if e.hookRunner != nil {
		e.hookRunner.Run(hook.OnSessionResolve, &hook.HookContext{
			ChatCtx:      chatCtx,
			SessionID:    chatCtx.SessionID(),
			AgentID:      chatCtx.AgentID(),
			Logger:       e.logger,
			CurrentPoint: hook.OnSessionResolve,
		})
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

// handleStreaming processes the streaming response from the LLM provider,
// accumulating content, reasoning, and tool calls while emitting part-level
// events in real time (EventPartCreate, EventPartUpdate).
func (e *engineImpl) handleStreaming(
	chatCtx iface.ChatContextInterface,
	agentDef *AgentDefinition,
	llmMessages []llm.Message,
	tools []llm.ToolDef,
	messageID string,
	stepIndex int,
) (*StreamResult, error) {
	provider := e.llmProvider
	if provider == nil {
		provider = agentDef.LLMProvider
	}

	params := llm.StreamParams{
		Model:    agentDef.Model,
		Messages: llmMessages,
		Tools:    tools,
	}

	ch, errc := provider.Stream(chatCtx.Context(), params)

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

	for chunk := range ch {
		if chunk.Content != "" {
			result.Content += chunk.Content

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
					TextDelta: chunk.Content,
				},
			})
		}

		if chunk.ReasoningContent != "" {
			result.ReasoningContent += chunk.ReasoningContent

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
					TextDelta: chunk.ReasoningContent,
				},
			})
		}

		if len(chunk.ToolCalls) > 0 {
			for _, tc := range chunk.ToolCalls {
				idx := tc.Index
				if existing, ok := toolCallMap[idx]; ok {
					if tc.Name != "" {
						existing.Name = tc.Name
					}
					if tc.Arguments != "" {
						existing.Arguments += tc.Arguments
					}
					if tc.ID != "" {
						existing.ID = tc.ID
					}
				} else {
					toolCallMap[idx] = &toolCallInfo{
						ID:        tc.ID,
						Name:      tc.Name,
						Arguments: tc.Arguments,
						MessageID: messageID,
					}
				}
			}
		}

		if chunk.Usage != nil {
			result.Usage = chunk.Usage
		}
	}

	for i := 0; i < len(toolCallMap); i++ {
		if tc, ok := toolCallMap[i]; ok {
			result.ToolCalls = append(result.ToolCalls, *tc)
		}
	}

	// Check for stream errors (consumed concurrently via errc)
	var streamErr error
	select {
	case err, ok := <-errc:
		if ok && err != nil {
			streamErr = err
		}
	default:
	}

	if streamErr != nil {
		e.logger.Error("llm_stream_error",
			"session_id", chatCtx.SessionID(),
			"error", streamErr,
		)
		return nil, fmt.Errorf("stream error: %w", streamErr)
	}

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

	if result.Usage != nil {
		e.logger.Info("llm_response",
			"session_id", chatCtx.SessionID(),
			"reasoning_len", len(result.ReasoningContent),
			"content_len", len(result.Content),
			"tool_calls", len(result.ToolCalls),
			"prompt_tokens", result.Usage.PromptTokens,
			"completion_tokens", result.Usage.CompletionTokens,
			"total_tokens", result.Usage.TotalTokens,
		)
	}

	return result, nil
}

// logLLMRequest logs the LLM request parameters for debugging.
func (e *engineImpl) logLLMRequest(agentDef *AgentDefinition, messages []entity.MessageForLLM, tools []llm.ToolDef, sessionID string) {
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

		// OnSystemPrompt hook (already wired in T11)
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

		// Hook: BeforeContextBuild
		if e.hookRunner != nil {
			e.hookRunner.Run(hook.BeforeContextBuild, &hook.HookContext{
				ChatCtx:      chatCtx,
				SessionID:    chatCtx.SessionID(),
				AgentID:      chatCtx.AgentID(),
				SystemPrompt: &systemPrompt,
				Logger:       e.logger,
				CurrentPoint: hook.BeforeContextBuild,
			})
		}

		messages, err := e.contextMgr.BuildContext(chatCtx, "", 256000, systemPrompt)
		if err != nil {
			return fmt.Errorf("build context: %w", err)
		}

		// Hook: AfterContextBuild — hooks may inspect or modify the assembled messages
		if e.hookRunner != nil {
			e.hookRunner.Run(hook.AfterContextBuild, &hook.HookContext{
				ChatCtx:      chatCtx,
				SessionID:    chatCtx.SessionID(),
				AgentID:      chatCtx.AgentID(),
				Messages:     &messages,
				Logger:       e.logger,
				CurrentPoint: hook.AfterContextBuild,
			})
		}

		llmMessages := e.convertToLLMMessages(messages)
		llmTools := convertToLLMTools(tools)

		// Log request
		e.logLLMRequest(agentDef, messages, llmTools, chatCtx.SessionID())

		// Hook: BeforeLLMCall
		if e.hookRunner != nil {
			e.hookRunner.Run(hook.BeforeLLMCall, &hook.HookContext{
				ChatCtx:      chatCtx,
				SessionID:    chatCtx.SessionID(),
				AgentID:      chatCtx.AgentID(),
				Messages:     &messages,
				Logger:       e.logger,
				CurrentPoint: hook.BeforeLLMCall,
			})
		}

		// Phase 3: Streaming
		result, err := e.handleStreaming(chatCtx, agentDef, llmMessages, llmTools, messageID, stepIndex)
		if err != nil {
			return err
		}

		// Hook: AfterLLMCall
		if e.hookRunner != nil {
			e.hookRunner.Run(hook.AfterLLMCall, &hook.HookContext{
				ChatCtx:      chatCtx,
				SessionID:    chatCtx.SessionID(),
				AgentID:      chatCtx.AgentID(),
				Messages:     &messages,
				Logger:       e.logger,
				CurrentPoint: hook.AfterLLMCall,
			})
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

func (e *engineImpl) convertToLLMMessages(messages []entity.MessageForLLM) []llm.Message {
	result := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			result = append(result, llm.Message{Role: llm.RoleSystem, Content: msg.Content})
		case "user":
			result = append(result, llm.Message{Role: llm.RoleUser, Content: msg.Content})
		case "assistant":
			m := llm.Message{Role: llm.RoleAssistant, Content: msg.Content}
			if len(msg.ToolCalls) > 0 {
				m.ToolCalls = make([]llm.ToolCall, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					m.ToolCalls = append(m.ToolCalls, llm.ToolCall{
						ID:   tc.ID,
						Type: "function",
						Function: llm.FunctionCall{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					})
				}
			}
			result = append(result, m)
		case "tool":
			result = append(result, llm.Message{
				Role:       llm.RoleTool,
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
		default:
			result = append(result, llm.Message{Role: llm.RoleUser, Content: msg.Content})
		}
	}
	return result
}

// convertToLLMTools converts OpenAI tool definitions to provider-agnostic ToolDef values.
func convertToLLMTools(tools []openai.ChatCompletionToolUnionParam) []llm.ToolDef {
	result := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		if t.OfFunction != nil {
			fn := t.OfFunction.Function
			td := llm.ToolDef{
				Name:        fn.Name,
				Description: fn.Description.Value,
			}
			if fn.Parameters != nil {
				paramsJSON, _ := json.Marshal(fn.Parameters)
				td.Parameters = paramsJSON
			}
			result = append(result, td)
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

	if e.hookRunner != nil {
		e.hookRunner.Run(hook.OnMessagePersist, &hook.HookContext{
			ChatCtx:      chatCtx,
			SessionID:    chatCtx.SessionID(),
			AgentID:      chatCtx.AgentID(),
			Logger:       e.logger,
			CurrentPoint: hook.OnMessagePersist,
		})
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
