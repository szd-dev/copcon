package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

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

type deltaExtraFields struct {
	ReasoningContent string `json:"reasoning_content"`
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

	// Generate messageID once for the entire agent loop
	messageID := uuid.New().String()

	// Phase 2: Agent Loop
	for {

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
