package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"golang.org/x/sync/semaphore"

	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/memory"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
)

var (
	ErrNoSession = errors.New("session not found")
)

// ExecutionMode defines how a tool should be executed.
type ExecutionMode string

const (
	ExecutionModeSync       ExecutionMode = "sync"
	ExecutionModeConcurrent ExecutionMode = "concurrent"
	ExecutionModeAsync      ExecutionMode = "async"
)

type deltaExtraFields struct {
	ReasoningContent string `json:"reasoning_content"`
}

type toolCallInfo struct {
	MessageID string
	ID        string
	Name      string
	Arguments string
}

// StreamResult encapsulates the result of streaming an LLM response.
// It contains all accumulated content, reasoning, and tool calls from the stream.
type StreamResult struct {
	MessageID        string
	Content          string
	ReasoningContent string
	ToolCalls        []toolCallInfo
	Usage            openai.CompletionUsage
}

// LLMRequest encapsulates the parameters for an LLM API call.
// It groups the model, messages, and tools for cleaner function signatures.
type LLMRequest struct {
	Model    string
	Messages []openai.ChatCompletionMessageParamUnion
	Tools    []openai.ChatCompletionToolUnionParam
}

type AgentEngine struct {
	agentRegistry  AgentRegistry
	sessionMgr     session.SessionManager
	contextMgr     chat_context.ContextManager
	memoryMgr      memory.MemoryManager
	concurrencySem *semaphore.Weighted     // limit concurrent tool executions
	asyncRegistry  *tool.AsyncToolRegistry // tracks async tool executions
}

func NewAgentEngine(
	agentRegistry AgentRegistry,
	sessionMgr session.SessionManager,
	contextMgr chat_context.ContextManager,
	memoryMgr memory.MemoryManager,
	asyncRegistry *tool.AsyncToolRegistry,
) *AgentEngine {
	return &AgentEngine{
		agentRegistry:  agentRegistry,
		sessionMgr:     sessionMgr,
		contextMgr:     contextMgr,
		memoryMgr:      memoryMgr,
		concurrencySem: semaphore.NewWeighted(5), // max 5 concurrent tool executions
		asyncRegistry:  asyncRegistry,
	}
}

func (e *AgentEngine) Chat(chatCtx iface.ChatContextInterface, userInput string) error {
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
func (e *AgentEngine) prepareAgentLoop(chatCtx iface.ChatContextInterface, userInput string) (*AgentDefinition, error) {
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
// content, reasoning, and tool calls while emitting events in real-time.
func (e *AgentEngine) handleStreaming(
	chatCtx iface.ChatContextInterface,
	agentDef *AgentDefinition,
	openAIMessages []openai.ChatCompletionMessageParamUnion,
	tools []openai.ChatCompletionToolUnionParam,
	messageID string,
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
	}
	toolCallMap := make(map[int]*toolCallInfo)

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta

			if delta.Content != "" {
				result.Content += delta.Content
				chatCtx.Emit(entity.Event{
					Type: entity.EventMessage,
					Data: entity.MessageData{MessageID: messageID, Content: delta.Content},
				})
			}

			var extra deltaExtraFields
			if err := json.Unmarshal([]byte(delta.RawJSON()), &extra); err == nil {
				if extra.ReasoningContent != "" {
					result.ReasoningContent += extra.ReasoningContent
					chatCtx.Emit(entity.Event{
						Type: entity.EventReasoning,
						Data: entity.ReasoningData{MessageID: messageID, Content: extra.ReasoningContent},
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
		log.Printf("========== LLM Error ==========")
		log.Printf("Error: %v", err)
		log.Printf("===============================")
		return nil, fmt.Errorf("stream error: %w", err)
	}

	result.Usage = acc.Usage

	log.Printf("========== LLM Response ==========")
	if result.ReasoningContent != "" {
		log.Printf("Reasoning: %s", result.ReasoningContent)
	}
	if result.Content != "" {
		log.Printf("Content: %s", result.Content)
	}
	if len(result.ToolCalls) > 0 {
		log.Printf("Tool calls: %d", len(result.ToolCalls))
		for i, tc := range result.ToolCalls {
			log.Printf("  [%d] %s(%s) id=%s", i, tc.Name, tc.Arguments, tc.ID)
		}
	}
	if result.Usage.TotalTokens > 0 {
		log.Printf("Tokens - Prompt: %d, Completion: %d, Total: %d",
			result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens)
	}
	log.Printf("==================================")

	return result, nil
}

// logLLMRequest logs the LLM request parameters for debugging.
func (e *AgentEngine) logLLMRequest(agentDef *AgentDefinition, messages []chat_context.MessageForLLM, tools []openai.ChatCompletionToolUnionParam) {
	log.Printf("========== LLM Request ==========")
	log.Printf("Agent: %s", agentDef.Name)
	log.Printf("Model: %s", agentDef.Model)
	log.Printf("Message count: %d", len(messages))
	for i, msg := range messages {
		log.Printf("  [%d] role=%s content=%s", i, msg.Role, msg.Content)
	}
	if len(tools) > 0 {
		log.Printf("Tools available: %d", len(tools))
	}
	log.Printf("=================================")
}

func (e *AgentEngine) runAgentLoop(chatCtx iface.ChatContextInterface, userInput string) error {
	// Phase 1: Initialization
	agentDef, err := e.prepareAgentLoop(chatCtx, userInput)
	if err != nil {
		return err
	}

	// Tool list is static per agent, retrieve once outside loop
	tools := agentDef.ToolManager.GetOpenAITools()

	// Phase 2: Agent Loop
	for {
		messageID := uuid.New().String()

		// Build LLM context
		messages, err := e.contextMgr.BuildContext(chatCtx, "", 256000, agentDef.SystemPrompt)
		if err != nil {
			return fmt.Errorf("build context: %w", err)
		}

		openAIMessages := e.convertMessages(messages)

		// Log request
		e.logLLMRequest(agentDef, messages, tools)

		// Phase 3: Streaming
		result, err := e.handleStreaming(chatCtx, agentDef, openAIMessages, tools, messageID)
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
	}
}

// parseExecutionMode extracts the execution_mode from tool arguments.
// It returns the mode (defaulting to sync) and the args with execution_mode removed.
func parseExecutionMode(args map[string]any) (ExecutionMode, map[string]any) {
	mode := ExecutionModeSync
	if val, ok := args["execution_mode"]; ok {
		if str, ok := val.(string); ok {
			switch str {
			case "sync", "concurrent", "async":
				mode = ExecutionMode(str)
			}
		}
		delete(args, "execution_mode")
	}
	return mode, args
}

func (e *AgentEngine) executeSync(chatCtx iface.ChatContextInterface, toolMgr tool.ToolManager, tc toolCallInfo, args map[string]any) error {
	chatCtx.Emit(entity.Event{
		Type: entity.EventToolCall,
		Data: entity.ToolCallData{
			ToolName:  tc.Name,
			Args:      args,
			ID:        tc.ID,
			MessageID: tc.MessageID,
		},
	})

	result, err := toolMgr.Execute(chatCtx, tc.Name, args)

	var resultData entity.ToolResultData
	if err != nil {
		resultData = entity.ToolResultData{
			ToolName:  tc.Name,
			Result:    map[string]any{"error": err.Error()},
			ID:        tc.ID,
			MessageID: tc.MessageID,
		}
	} else {
		resultData = entity.ToolResultData{
			ToolName:  tc.Name,
			Result:    result,
			ID:        tc.ID,
			MessageID: tc.MessageID,
		}
	}

	chatCtx.Emit(entity.Event{Type: entity.EventToolResult, Data: resultData})

	resultJSON, _ := json.Marshal(resultData.Result)
	if err := e.contextMgr.AddMessage(chatCtx, &session.Message{
		Role:       "tool",
		Content:    string(resultJSON),
		ToolCallID: tc.ID,
	}); err != nil {
		return fmt.Errorf("add tool result message: %w", err)
	}

	return nil
}

func (e *AgentEngine) executeAsync(chatCtx iface.ChatContextInterface, toolMgr tool.ToolManager, tc toolCallInfo, args map[string]any) error {
	sessionID := chatCtx.SessionID()
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(chatCtx.Context(), 5*time.Minute)

	e.asyncRegistry.Register(sessionID, tc.ID, tc.Name, cancel)

	chatCtx.Emit(entity.Event{
		Type: entity.EventAsyncToolStarted,
		Data: entity.AsyncToolStartedData{
			CallID:    tc.ID,
			ToolName:  tc.Name,
			SessionID: sessionID,
		},
	})

	go func() {
		defer e.asyncRegistry.Unregister(tc.ID)
		defer cancel()

		if err := e.concurrencySem.Acquire(ctx, 1); err != nil {
			e.asyncRegistry.Fail(tc.ID, err.Error())
			chatCtx.Emit(entity.Event{
				Type: entity.EventAsyncToolFailed,
				Data: entity.AsyncToolFailedData{
					CallID:   tc.ID,
					ToolName: tc.Name,
					Error:    err.Error(),
					Duration: time.Since(startTime).Milliseconds(),
				},
			})
			return
		}
		defer e.concurrencySem.Release(1)

		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				errMsg := fmt.Sprintf("panic: %v\n%s", r, stack)
				e.asyncRegistry.Fail(tc.ID, errMsg)
				chatCtx.Emit(entity.Event{
					Type: entity.EventAsyncToolFailed,
					Data: entity.AsyncToolFailedData{
						CallID:   tc.ID,
						ToolName: tc.Name,
						Error:    errMsg,
						Duration: time.Since(startTime).Milliseconds(),
					},
				})
			}
		}()

		result, err := toolMgr.Execute(chatCtx, tc.Name, args)

		if err != nil {
			e.asyncRegistry.Fail(tc.ID, err.Error())
			chatCtx.Emit(entity.Event{
				Type: entity.EventAsyncToolFailed,
				Data: entity.AsyncToolFailedData{
					CallID:   tc.ID,
					ToolName: tc.Name,
					Error:    err.Error(),
					Duration: time.Since(startTime).Milliseconds(),
				},
			})

			resultJSON, _ := json.Marshal(map[string]any{"error": err.Error()})
			if addErr := e.contextMgr.AddMessage(chatCtx, &session.Message{
				Role:       "tool",
				Content:    string(resultJSON),
				ToolCallID: tc.ID,
			}); addErr != nil {
				log.Printf("failed to persist async tool error: %v", addErr)
			}

			pendingEvent := map[string]any{
				"id":           uuid.New().String(),
				"call_id":      tc.ID,
				"tool_name":    tc.Name,
				"session_id":   sessionID,
				"completed_at": time.Now().Format(time.RFC3339),
				"status":       "failed",
				"error":        err.Error(),
			}
			if addErr := e.sessionMgr.AddAsyncCompletionPending(chatCtx, pendingEvent); addErr != nil {
				log.Printf("failed to record async completion pending: %v", addErr)
			}
		} else {
			e.asyncRegistry.Complete(tc.ID, result)
			chatCtx.Emit(entity.Event{
				Type: entity.EventAsyncToolComplete,
				Data: entity.AsyncToolCompleteData{
					CallID:   tc.ID,
					ToolName: tc.Name,
					Result:   result,
					Duration: time.Since(startTime).Milliseconds(),
				},
			})

			resultJSON, _ := json.Marshal(result)
			if addErr := e.contextMgr.AddMessage(chatCtx, &session.Message{
				Role:       "tool",
				Content:    string(resultJSON),
				ToolCallID: tc.ID,
			}); addErr != nil {
				log.Printf("failed to persist async tool result: %v", addErr)
			}

			pendingEvent := map[string]any{
				"id":           uuid.New().String(),
				"call_id":      tc.ID,
				"tool_name":    tc.Name,
				"session_id":   sessionID,
				"completed_at": time.Now().Format(time.RFC3339),
				"status":       "completed",
			}
			if addErr := e.sessionMgr.AddAsyncCompletionPending(chatCtx, pendingEvent); addErr != nil {
				log.Printf("failed to record async completion pending: %v", addErr)
			}
		}
	}()

	return nil
}

// parsedToolCall holds a tool call with its parsed arguments (execution_mode removed).
type parsedToolCall struct {
	tc   toolCallInfo
	args map[string]any
}

// toolExecutionResult holds the result of a single tool execution.
type toolExecutionResult struct {
	tc     toolCallInfo
	result any
	err    error
}

// executeConcurrent executes multiple tool calls concurrently with semaphore limiting.
// It uses a semaphore to limit concurrency to 5 and collects results safely with mutex protection.
// One failure does not stop other executions - all tools run to completion.
// Results are sorted by tool call ID and persisted in order.
func (e *AgentEngine) executeConcurrent(
	chatCtx iface.ChatContextInterface,
	toolMgr tool.ToolManager,
	toolCalls []parsedToolCall,
) error {
	if len(toolCalls) == 0 {
		return nil
	}

	var (
		results = make([]toolExecutionResult, len(toolCalls))
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	// Launch goroutines for each tool call
	for i, p := range toolCalls {
		wg.Add(1)
		go func(idx int, p parsedToolCall) {
			defer wg.Done()

			// Acquire semaphore (blocks if 5 concurrent executions are already running)
			if err := e.concurrencySem.Acquire(chatCtx.Context(), 1); err != nil {
				// Context cancelled or semaphore error
				mu.Lock()
				results[idx] = toolExecutionResult{
					tc:  p.tc,
					err: fmt.Errorf("semaphore acquire: %w", err),
				}
				mu.Unlock()
				return
			}
			defer e.concurrencySem.Release(1)

			// Emit tool call event
			chatCtx.Emit(entity.Event{
				Type: entity.EventToolCall,
				Data: entity.ToolCallData{
					ToolName:  p.tc.Name,
					Args:      p.args,
					ID:        p.tc.ID,
					MessageID: p.tc.MessageID,
				},
			})

			// Execute tool
			execResult, execErr := toolMgr.Execute(chatCtx, p.tc.Name, p.args)

			var result any
			if execErr != nil {
				result = map[string]any{"error": execErr.Error()}
			} else {
				result = execResult
			}

			mu.Lock()
			results[idx] = toolExecutionResult{
				tc:     p.tc,
				result: result,
				err:    execErr,
			}
			mu.Unlock()

			chatCtx.Emit(entity.Event{
				Type: entity.EventToolResult,
				Data: entity.ToolResultData{
					ToolName:  p.tc.Name,
					Result:    result,
					ID:        p.tc.ID,
					MessageID: p.tc.MessageID,
				},
			})
		}(i, p)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Sort results by tool call ID for consistent ordering
	sort.Slice(results, func(i, j int) bool {
		return results[i].tc.ID < results[j].tc.ID
	})

	// Persist all results in order
	for _, r := range results {
		resultJSON, _ := json.Marshal(r.result)
		if err := e.contextMgr.AddMessage(chatCtx, &session.Message{
			Role:       "tool",
			Content:    string(resultJSON),
			ToolCallID: r.tc.ID,
		}); err != nil {
			return fmt.Errorf("add tool result message for %s: %w", r.tc.ID, err)
		}
	}

	return nil
}

func (e *AgentEngine) convertMessages(messages []chat_context.MessageForLLM) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			result = append(result, openai.SystemMessage(msg.Content))
		case "user":
			result = append(result, openai.UserMessage(msg.Content))
		case "assistant":
			result = append(result, openai.AssistantMessage(msg.Content))
		case "tool":
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		default:
			result = append(result, openai.UserMessage(msg.Content))
		}
	}
	return result
}

func (e *AgentEngine) convertToolCalls(toolCalls []toolCallInfo) []session.ToolCall {
	result := make([]session.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		result[i] = session.ToolCall{
			ID:   tc.ID,
			Type: "function",
			Function: session.FunctionCall{
				Name:      tc.Name,
				Arguments: tc.Arguments,
			},
		}
	}
	return result
}

// persistMessage saves an assistant message to the context.
// If result has ToolCalls, they are included.
// If isFinal is true, the message ID is set.
func (e *AgentEngine) persistMessage(
	chatCtx iface.ChatContextInterface,
	result *StreamResult,
	isFinal bool,
) error {
	msg := &session.Message{
		Role:      "assistant",
		Content:   result.Content,
		Reasoning: result.ReasoningContent,
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

// handleToolCalls processes tool calls from the streaming result.
// Returns true if the loop should continue (tool calls were executed),
// false if the loop should exit (no tool calls, final message persisted).
func (e *AgentEngine) handleToolCalls(
	chatCtx iface.ChatContextInterface,
	toolMgr tool.ToolManager,
	result *StreamResult,
) (bool, error) {
	if len(result.ToolCalls) > 0 {
		if err := e.persistMessage(chatCtx, result, false); err != nil {
			return false, err
		}

		var (
			syncToolCalls       []toolCallInfo
			concurrentToolCalls []parsedToolCall
			asyncToolCalls      []toolCallInfo
		)

		for _, tc := range result.ToolCalls {
			args := parseArgs(tc.Arguments)
			mode, args := parseExecutionMode(args)

			switch mode {
			case ExecutionModeSync:
				syncToolCalls = append(syncToolCalls, tc)
			case ExecutionModeConcurrent:
				concurrentToolCalls = append(concurrentToolCalls, parsedToolCall{tc: tc, args: args})
			case ExecutionModeAsync:
				asyncToolCalls = append(asyncToolCalls, tc)
			}
		}

		for _, tc := range syncToolCalls {
			args := parseArgs(tc.Arguments)
			_, args = parseExecutionMode(args)
			if err := e.executeSync(chatCtx, toolMgr, tc, args); err != nil {
				return false, fmt.Errorf("execute sync tool call: %w", err)
			}
		}

		if len(concurrentToolCalls) > 0 {
			if err := e.executeConcurrent(chatCtx, toolMgr, concurrentToolCalls); err != nil {
				return false, fmt.Errorf("execute concurrent tool calls: %w", err)
			}
		}

		for _, tc := range asyncToolCalls {
			args := parseArgs(tc.Arguments)
			_, args = parseExecutionMode(args)
			if err := e.executeAsync(chatCtx, toolMgr, tc, args); err != nil {
				return false, fmt.Errorf("execute async tool call: %w", err)
			}
		}

		return true, nil
	}

	if err := e.persistMessage(chatCtx, result, true); err != nil {
		return false, err
	}

	chatCtx.Emit(entity.Event{
		Type: entity.EventDone,
		Data: entity.DoneData{MessageID: result.MessageID},
	})

	return false, nil
}

func parseArgs(argsJSON string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return make(map[string]any)
	}
	return args
}
