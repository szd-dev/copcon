package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"

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

type toolCallInfo struct {
	ID        string
	Name      string
	Arguments string
}

type AgentEngine struct {
	agentRegistry AgentRegistry
	sessionMgr    session.SessionManager
	contextMgr    chat_context.ContextManager
	memoryMgr     memory.MemoryManager
}

func NewAgentEngine(
	agentRegistry AgentRegistry,
	sessionMgr session.SessionManager,
	contextMgr chat_context.ContextManager,
	memoryMgr memory.MemoryManager,
) *AgentEngine {
	return &AgentEngine{
		agentRegistry: agentRegistry,
		sessionMgr:    sessionMgr,
		contextMgr:    contextMgr,
		memoryMgr:     memoryMgr,
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

func (e *AgentEngine) runAgentLoop(chatCtx iface.ChatContextInterface, userInput string) error {
	sess, err := e.sessionMgr.Get(chatCtx)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	// Determine which agent to use
	agentID := chatCtx.AgentID()
	if agentID == "" {
		agentID = sess.DefaultAgentID
	}
	if agentID == "" {
		// Fall back to default agent from registry
		defaultAgent, err := e.agentRegistry.Default()
		if err != nil {
			return fmt.Errorf("no agent specified and no default agent: %w", err)
		}
		agentID = defaultAgent.ID
	}

	// Get agent definition
	agentDef, err := e.agentRegistry.Get(agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}

	if err := e.contextMgr.AddMessage(chatCtx, &session.Message{
		Role:    "user",
		Content: userInput,
	}); err != nil {
		return fmt.Errorf("add user message: %w", err)
	}

	for {
		messages, err := e.contextMgr.BuildContext(chatCtx, "", 256000, agentDef.SystemPrompt)
		if err != nil {
			return fmt.Errorf("build context: %w", err)
		}

		openAIMessages := e.convertMessages(messages)

		log.Printf("========== LLM Request ==========")
		log.Printf("Agent: %s", agentDef.Name)
		log.Printf("Model: %s", agentDef.Model)
		log.Printf("Message count: %d", len(messages))
		for i, msg := range messages {
			log.Printf("  [%d] role=%s content=%s", i, msg.Role, msg.Content)
		}
		tools := agentDef.ToolManager.GetOpenAITools()
		if len(tools) > 0 {
			log.Printf("Tools available: %d", len(tools))
		}
		log.Printf("=================================")

		params := openai.ChatCompletionNewParams{
			Model:             openai.ChatModel(agentDef.Model),
			Messages:          openAIMessages,
			Tools:             tools,
			ParallelToolCalls: openai.Bool(true),
		}

		stream := agentDef.OpenAIClient.Chat.Completions.NewStreaming(chatCtx.Context(), params)
		acc := openai.ChatCompletionAccumulator{}

		var content string
		var reasoningContent string
		var toolCalls []toolCallInfo
		toolCallMap := make(map[int]*toolCallInfo)

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta

				// log.Printf("Delta: %v", delta.RawJSON())

				if delta.Content != "" {
					content += delta.Content
					chatCtx.Emit(entity.Event{
						Type: entity.EventMessage,
						Data: entity.MessageData{Content: delta.Content},
					})
				}

				var extra deltaExtraFields
				if err := json.Unmarshal([]byte(delta.RawJSON()), &extra); err == nil {
					if extra.ReasoningContent != "" {
						reasoningContent += extra.ReasoningContent
						chatCtx.Emit(entity.Event{
							Type: entity.EventReasoning,
							Data: entity.ReasoningData{Content: extra.ReasoningContent},
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
							}
						}
					}
				}
			}

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
				toolCalls = append(toolCalls, *tc)
			}
		}

		if err := stream.Err(); err != nil {
			log.Printf("========== LLM Error ==========")
			log.Printf("Error: %v", err)
			log.Printf("===============================")
			return fmt.Errorf("stream error: %w", err)
		}

		log.Printf("========== LLM Response ==========")
		if reasoningContent != "" {
			log.Printf("Reasoning: %s", reasoningContent)
		}
		if content != "" {
			log.Printf("Content: %s", content)
		}
		if len(toolCalls) > 0 {
			log.Printf("Tool calls: %d", len(toolCalls))
			for i, tc := range toolCalls {
				log.Printf("  [%d] %s(%s) id=%s", i, tc.Name, tc.Arguments, tc.ID)
			}
		}
		if acc.Usage.TotalTokens > 0 {
			log.Printf("Tokens - Prompt: %d, Completion: %d, Total: %d",
				acc.Usage.PromptTokens, acc.Usage.CompletionTokens, acc.Usage.TotalTokens)
		}
		log.Printf("==================================")

		if len(toolCalls) > 0 {
			if err := e.contextMgr.AddMessage(chatCtx, &session.Message{
				Role:      "assistant",
				Content:   content,
				Reasoning: reasoningContent,
				ToolCalls: e.convertToolCalls(toolCalls),
			}); err != nil {
				return fmt.Errorf("add assistant message: %w", err)
			}

			for _, tc := range toolCalls {
				if err := e.executeToolCall(chatCtx, agentDef.ToolManager, tc); err != nil {
					return fmt.Errorf("execute tool call: %w", err)
				}
			}

			continue
		}

		messageID := uuid.New().String()
		if err := e.contextMgr.AddMessage(chatCtx, &session.Message{
			ID:        uuid.MustParse(messageID),
			Role:      "assistant",
			Content:   content,
			Reasoning: reasoningContent,
		}); err != nil {
			return fmt.Errorf("add assistant message: %w", err)
		}

		chatCtx.Emit(entity.Event{
			Type: entity.EventDone,
			Data: entity.DoneData{MessageID: messageID},
		})

		return nil
	}
}

func (e *AgentEngine) executeToolCall(chatCtx iface.ChatContextInterface, toolMgr tool.ToolManager, tc toolCallInfo) error {
	chatCtx.Emit(entity.Event{
		Type: entity.EventToolCall,
		Data: entity.ToolCallData{
			ToolName: tc.Name,
			Args:     parseArgs(tc.Arguments),
			ID:       tc.ID,
		},
	})

	args := parseArgs(tc.Arguments)
	result, err := toolMgr.Execute(chatCtx, tc.Name, args)

	var resultData entity.ToolResultData
	if err != nil {
		resultData = entity.ToolResultData{
			ToolName: tc.Name,
			Result:   map[string]any{"error": err.Error()},
			ID:       tc.ID,
		}
	} else {
		resultData = entity.ToolResultData{
			ToolName: tc.Name,
			Result:   result,
			ID:       tc.ID,
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

func parseArgs(argsJSON string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return make(map[string]any)
	}
	return args
}
