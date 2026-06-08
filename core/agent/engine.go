package agent

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"

	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"

	"github.com/copcon/core/context_builder"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

var (
	ErrNoSession = errors.New("session not found")
)

const maxSteps = 50

const deltaPersistInterval = 10

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
	ToolResults      map[string]*ToolCallResult
	Usage            *llm.Usage
}

type AgentEngine interface {
	Chat(chatCtx iface.ChatContextInterface, userInput string) error
}

type EngineOption func(*engineImpl)

func WithConcurrency(n int) EngineOption {
	return func(e *engineImpl) {
		if n <= 0 {
			panic(fmt.Sprintf("WithConcurrency: n must be > 0, got %d", n))
		}
		e.concurrency = n
	}
}

func WithLogger(logger *slog.Logger) EngineOption {
	return func(e *engineImpl) {
		e.logger = logger
	}
}

func WithHookRunner(runner hook.HookRunner) EngineOption {
	return func(e *engineImpl) {
		e.hookRunner = runner
	}
}

func WithGlobalHooks(hooks ...hook.Hook) EngineOption {
	return func(e *engineImpl) {
		e.globalHooks = hooks
	}
}

func WithLLMProvider(p llm.LLMProvider) EngineOption {
	return func(e *engineImpl) {
		e.llmProvider = p
	}
}

type engineImpl struct {
	logger         *slog.Logger
	agentRegistry  AgentRegistry
	sessionStore   storage.SessionStore
	messageStore   storage.MessageStore
	ctxBuilder     context_builder.ContextBuilder
	llmProvider    llm.LLMProvider
	concurrency    int
	concurrencySem *semaphore.Weighted
	asyncRegistry  tool.AsyncToolTracker
	hookRunner     hook.HookRunner
	globalHooks    []hook.Hook
}

var _ AgentEngine = (*engineImpl)(nil)

func NewAgentEngine(
	agentRegistry AgentRegistry,
	sessionStore storage.SessionStore,
	messageStore storage.MessageStore,
	ctxBuilder context_builder.ContextBuilder,
	asyncRegistry tool.AsyncToolTracker,
	opts ...EngineOption,
) AgentEngine {
	e := &engineImpl{
		agentRegistry: agentRegistry,
		sessionStore:  sessionStore,
		messageStore:  messageStore,
		ctxBuilder:    ctxBuilder,
		asyncRegistry: asyncRegistry,
		hookRunner:    hook.NewEmptyRunner(),
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
	if chatCtx.Depth() >= 3 {
		return fmt.Errorf("max subagent depth exceeded")
	}
	if err := e.runAgentLoop(chatCtx, userInput); err != nil {
		chatCtx.Emit(entity.Event{
			Type: entity.EventError,
			Data: entity.ErrorData{Error: err.Error()},
		})
	}
	return nil
}

func (e *engineImpl) prepareAgentLoop(chatCtx iface.ChatContextInterface, userInput string) (*AgentDefinition, error) {
	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}
	sess, err := e.sessionStore.Get(chatCtx.Context(), sessionUUID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	e.hookRunner.On(hook.OnSessionResolve, chatCtx, e.logger)

	agentID := chatCtx.AgentID()
	if agentID == "" {
		agentID = sess.DefaultAgentID
	}
	if agentID == "" {
		defaultAgent, err := e.agentRegistry.Default()
		if err != nil {
			return nil, fmt.Errorf("no agent specified and no default agent: %w", err)
		}
		agentID = defaultAgent.ID
	}

	agentDef, err := e.agentRegistry.Get(agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}

	if userInput != "" {
		if err := e.messageStore.Add(chatCtx.Context(), &storage.Message{
			SessionID: sessionUUID,
			Role:      "user",
			Content:   userInput,
		}); err != nil {
			return nil, fmt.Errorf("add user message: %w", err)
		}
	}

	return &agentDef, nil
}

func (e *engineImpl) handleStreaming(
	chatCtx iface.ChatContextInterface,
	agentDef *AgentDefinition,
	llmMessages []llm.Message,
	tools []llm.ToolDef,
	messageID string,
	stepIndex int,
	persistedMsgUUID *string,
	accumulatedParts *[]storage.Part,
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
	textDeltaCount := 0

	for chunk := range ch {
		if chunk.Content != "" {
			result.Content += chunk.Content
			textDeltaCount++

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

			if textDeltaCount%deltaPersistInterval == 0 {
				e.checkpointStreamingParts(chatCtx, messageID, stepIndex, result, accumulatedParts, reasoningPartCreated, textPartCreated)
				if *persistedMsgUUID == "" {
					*persistedMsgUUID = messageID
				}
			}
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

	e.checkpointDoneParts(chatCtx, messageID, stepIndex, result, accumulatedParts)
	if *persistedMsgUUID == "" {
		*persistedMsgUUID = messageID
	}

	return result, nil
}

func (e *engineImpl) runAgentLoop(chatCtx iface.ChatContextInterface, userInput string) error {
	agentDef, err := e.prepareAgentLoop(chatCtx, userInput)
	if err != nil {
		return err
	}

	tools := agentDef.ToolManager.GetToolDefs()
	messageID := uuid.New().String()
	persistedMsgUUID := ""
	var accumulatedParts []storage.Part
	var accumulatedToolCalls []storage.ToolCall
	isFirstIteration := true
	stepIndex := 0

	var composedHooks []hook.Hook
	if len(agentDef.Hooks) > 0 {
		composedHooks = composeHooks(e.globalHooks, agentDef.Hooks)
	}

	for {
		if stepIndex >= maxSteps {
			chatCtx.Emit(entity.Event{
				Type: entity.EventError,
				Data: entity.ErrorData{Error: "step limit exceeded"},
			})
			return nil
		}

		if !isFirstIteration {
			chatCtx.Emit(entity.Event{
				Type: entity.EventStepCreate,
				Data: entity.StepCreateData{
					MessageID: messageID,
					StepIndex: stepIndex,
				},
			})
			if len(accumulatedParts) > 0 {
				if err := e.persistMessageUpsert(chatCtx, messageID, accumulatedParts); err != nil {
					e.logger.Warn("incremental_persist_step_create_failed",
						"session_id", chatCtx.SessionID(),
						"step_index", stepIndex,
						"error", err,
					)
				}
			}
		}
		isFirstIteration = false

		systemPrompt := agentDef.SystemPrompt
		if composedHooks != nil {
			runComposedHooks(composedHooks, hook.OnSystemPrompt, chatCtx, e.logger, hook.HookExtra{SystemPrompt: &systemPrompt})
		} else {
			e.hookRunner.On(hook.OnSystemPrompt, chatCtx, e.logger, hook.HookExtra{SystemPrompt: &systemPrompt})
		}

		if composedHooks != nil {
			runComposedHooks(composedHooks, hook.BeforeContextBuild, chatCtx, e.logger, hook.HookExtra{SystemPrompt: &systemPrompt})
		} else {
			e.hookRunner.On(hook.BeforeContextBuild, chatCtx, e.logger, hook.HookExtra{SystemPrompt: &systemPrompt})
		}

		sessionUUID, _ := uuid.Parse(chatCtx.SessionID())
		messages, err := BuildContext(chatCtx.Context(), e.messageStore, e.ctxBuilder, sessionUUID, "", 256000, systemPrompt)
		if err != nil {
			return fmt.Errorf("build context: %w", err)
		}

		if composedHooks != nil {
			runComposedHooks(composedHooks, hook.AfterContextBuild, chatCtx, e.logger, hook.HookExtra{Messages: &messages})
		} else {
			e.hookRunner.On(hook.AfterContextBuild, chatCtx, e.logger, hook.HookExtra{Messages: &messages})
		}

		llmMessages := e.convertToLLMMessages(messages)
		llmTools := tools

		if composedHooks != nil {
			runComposedHooks(composedHooks, hook.BeforeLLMCall, chatCtx, e.logger, hook.HookExtra{Messages: &messages})
		} else {
			e.hookRunner.On(hook.BeforeLLMCall, chatCtx, e.logger, hook.HookExtra{Messages: &messages})
		}

		result, err := e.handleStreaming(chatCtx, agentDef, llmMessages, llmTools, messageID, stepIndex, &persistedMsgUUID, &accumulatedParts)
		if err != nil {
			return err
		}

		llmResp := &hook.LLMResponseExtra{
			Content:          result.Content,
			ReasoningContent: result.ReasoningContent,
			ToolCallCount:    len(result.ToolCalls),
		}
		if result.Usage != nil {
			llmResp.PromptTokens = result.Usage.PromptTokens
			llmResp.CompletionTokens = result.Usage.CompletionTokens
			llmResp.TotalTokens = result.Usage.TotalTokens
		}

		if composedHooks != nil {
			runComposedHooks(composedHooks, hook.AfterLLMCall, chatCtx, e.logger, hook.HookExtra{Messages: &messages, LLMResponse: llmResp})
		} else {
			e.hookRunner.On(hook.AfterLLMCall, chatCtx, e.logger, hook.HookExtra{Messages: &messages, LLMResponse: llmResp})
		}

		shouldContinue, err := e.handleToolCalls(chatCtx, agentDef.ToolManager, result, &persistedMsgUUID, &accumulatedParts, &accumulatedToolCalls)
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

func (e *engineImpl) persistMessage(
	chatCtx iface.ChatContextInterface,
	result *StreamResult,
	isFinal bool,
	persistedMsgUUID *string,
	accumulatedParts *[]storage.Part,
	accumulatedToolCalls *[]storage.ToolCall,
) error {
	parts := e.buildUIParts(result, result.StepIndex)
	*accumulatedParts = append(*accumulatedParts, parts...)

	if len(result.ToolCalls) > 0 {
		newCalls := e.convertToolCalls(result.ToolCalls)
		*accumulatedToolCalls = append(*accumulatedToolCalls, newCalls...)
	}

	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	msg := &storage.Message{
		ID:        uuid.MustParse(result.MessageID),
		SessionID: sessionUUID,
		Role:      "assistant",
		Content:   result.Content,
		Reasoning: result.ReasoningContent,
		Parts:     *accumulatedParts,
		ToolCalls: *accumulatedToolCalls,
	}

	if *persistedMsgUUID == "" {
		if err := e.messageStore.Add(chatCtx.Context(), msg); err != nil {
			return fmt.Errorf("add assistant message: %w", err)
		}
		*persistedMsgUUID = result.MessageID
	} else {
		if err := e.messageStore.Update(chatCtx.Context(), msg); err != nil {
			return fmt.Errorf("update assistant message: %w", err)
		}
	}

	e.hookRunner.On(hook.OnMessagePersist, chatCtx, e.logger)

	return nil
}

func (e *engineImpl) persistMessageUpsert(
	chatCtx iface.ChatContextInterface,
	msgUUID string,
	parts []storage.Part,
) error {
	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	msg := &storage.Message{
		ID:        uuid.MustParse(msgUUID),
		SessionID: sessionUUID,
		Role:      "assistant",
		Parts:     parts,
	}

	if err := e.messageStore.Upsert(chatCtx.Context(), msg); err != nil {
		return fmt.Errorf("upsert assistant message: %w", err)
	}

	return nil
}

func (e *engineImpl) checkpointStreamingParts(
	chatCtx iface.ChatContextInterface,
	messageID string,
	stepIndex int,
	result *StreamResult,
	accumulatedParts *[]storage.Part,
	reasoningPartCreated, textPartCreated bool,
) {
	var parts []storage.Part
	parts = append(parts, *accumulatedParts...)

	if reasoningPartCreated {
		parts = append(parts, storage.Part{
			Type:      "reasoning",
			Text:      result.ReasoningContent,
			State:     "streaming",
			StepIndex: stepIndex,
		})
	}
	if textPartCreated {
		parts = append(parts, storage.Part{
			Type:      "text",
			Text:      result.Content,
			State:     "streaming",
			StepIndex: stepIndex,
		})
	}

	if err := e.persistMessageUpsert(chatCtx, messageID, parts); err != nil {
		e.logger.Warn("incremental_persist_delta_failed",
			"session_id", chatCtx.SessionID(),
			"step_index", stepIndex,
			"error", err,
		)
	}
}

func (e *engineImpl) checkpointDoneParts(
	chatCtx iface.ChatContextInterface,
	messageID string,
	stepIndex int,
	result *StreamResult,
	accumulatedParts *[]storage.Part,
) {
	currentParts := e.buildUIParts(result, stepIndex)
	var parts []storage.Part
	parts = append(parts, *accumulatedParts...)
	parts = append(parts, currentParts...)

	if err := e.persistMessageUpsert(chatCtx, messageID, parts); err != nil {
		e.logger.Warn("incremental_persist_done_failed",
			"session_id", chatCtx.SessionID(),
			"step_index", stepIndex,
			"error", err,
		)
	}
}

func (e *engineImpl) checkpointToolResult(
	chatCtx iface.ChatContextInterface,
	result *StreamResult,
	persistedMsgUUID *string,
	accumulatedParts *[]storage.Part,
) {
	currentParts := e.buildUIParts(result, result.StepIndex)
	var parts []storage.Part
	parts = append(parts, *accumulatedParts...)
	parts = append(parts, currentParts...)

	if err := e.persistMessageUpsert(chatCtx, result.MessageID, parts); err != nil {
		e.logger.Warn("incremental_persist_tool_result_failed",
			"session_id", chatCtx.SessionID(),
			"step_index", result.StepIndex,
			"error", err,
		)
	}
	if *persistedMsgUUID == "" {
		*persistedMsgUUID = result.MessageID
	}
}

func (e *engineImpl) buildUIParts(result *StreamResult, stepIndex int) []storage.Part {
	var parts []storage.Part

	if result.ReasoningContent != "" {
		parts = append(parts, storage.Part{
			Type:      "reasoning",
			Text:      result.ReasoningContent,
			State:     "done",
			StepIndex: stepIndex,
		})
	}

	if result.Content != "" || len(result.ToolCalls) == 0 {
		parts = append(parts, storage.Part{
			Type:      "text",
			Text:      result.Content,
			State:     "done",
			StepIndex: stepIndex,
		})
	}

	for _, tc := range result.ToolCalls {
		part := storage.Part{
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

func composeHooks(globalHooks []hook.Hook, agentHooks []hook.Hook) []hook.Hook {
	all := make([]hook.Hook, 0, len(globalHooks)+len(agentHooks))
	all = append(all, agentHooks...)
	all = append(all, globalHooks...)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Priority() < all[j].Priority()
	})
	return all
}

func runComposedHooks(hooks []hook.Hook, point hook.HookPoint, chatCtx iface.ChatContextInterface, logger *slog.Logger, extras ...hook.HookExtra) {
	for _, h := range hooks {
		matches := false
		for _, p := range h.Points() {
			if p == point {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}

		ctx := &hook.HookContext{
			ChatCtx:      chatCtx,
			SessionID:    chatCtx.SessionID(),
			AgentID:      chatCtx.AgentID(),
			Logger:       logger,
			CurrentPoint: point,
		}
		if len(extras) > 0 {
			e := extras[0]
			if e.ToolName != nil {
				ctx.ToolName = *e.ToolName
			}
			if e.ToolArgs != nil {
				ctx.ToolArgs = e.ToolArgs
			}
			if e.ToolResult != nil {
				ctx.ToolResult = e.ToolResult
			}
			if e.SystemPrompt != nil {
				ctx.SystemPrompt = e.SystemPrompt
			}
			if e.Messages != nil {
				ctx.Messages = e.Messages
			}
			if e.LLMResponse != nil {
				ctx.LLMResponse = e.LLMResponse
			}
		}

		func() {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("hook panicked",
						"hook", h.Name(),
						"panic", rec,
						"point", point,
					)
				}
			}()
			if err := h.Execute(ctx); err != nil {
				logger.Warn("hook returned error",
					"hook", h.Name(),
					"error", err,
					"point", point,
				)
			}
		}()
	}
}
