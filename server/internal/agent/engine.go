package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"

	contextmgr "github.com/copcon/server/internal/context"
	"github.com/copcon/server/internal/memory"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
)

var (
	ErrNoSession = errors.New("session not found")
)

type EventType string

const (
	EventMessage    EventType = "message"
	EventReasoning  EventType = "reasoning"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventThought    EventType = "thought"
	EventDone       EventType = "done"
	EventError      EventType = "error"
)

type Event struct {
	Type EventType `json:"type"`
	Data any       `json:"data"`
}

type MessageData struct {
	Content string `json:"content"`
}

type ReasoningData struct {
	Content string `json:"content"`
}

type ToolCallData struct {
	ToolName string         `json:"tool_name"`
	Args     map[string]any `json:"args"`
	ID       string         `json:"id"`
}

type ToolResultData struct {
	ToolName string `json:"tool_name"`
	Result   any    `json:"result"`
	ID       string `json:"id"`
}

type DoneData struct {
	MessageID string `json:"message_id"`
}

type ErrorData struct {
	Error string `json:"error"`
}

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
	contextMgr    contextmgr.ContextManager
	memoryMgr     memory.MemoryManager
}

func NewAgentEngine(
	agentRegistry AgentRegistry,
	sessionMgr session.SessionManager,
	contextMgr contextmgr.ContextManager,
	memoryMgr memory.MemoryManager,
) *AgentEngine {
	return &AgentEngine{
		agentRegistry: agentRegistry,
		sessionMgr:    sessionMgr,
		contextMgr:    contextMgr,
		memoryMgr:     memoryMgr,
	}
}

func (e *AgentEngine) Chat(ctx context.Context, sessionID string, agentID string, userInput string) (<-chan Event, error) {
	events := make(chan Event, 100)

	go func() {
		defer close(events)

		if err := e.runAgentLoop(ctx, sessionID, agentID, userInput, events); err != nil {
			events <- Event{
				Type: EventError,
				Data: ErrorData{Error: err.Error()},
			}
		}
	}()

	return events, nil
}

func (e *AgentEngine) runAgentLoop(ctx context.Context, sessionID string, agentID string, userInput string, events chan<- Event) error {
	sess, err := e.sessionMgr.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	// Determine which agent to use
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

	if err := e.contextMgr.AddMessage(ctx, sessionID, &session.Message{
		Role:    "user",
		Content: userInput,
	}); err != nil {
		return fmt.Errorf("add user message: %w", err)
	}

	for {
		messages, err := e.contextMgr.BuildContext(ctx, sessionID, "", 256000, agentDef.SystemPrompt)
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

		stream := agentDef.OpenAIClient.Chat.Completions.NewStreaming(ctx, params)
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

				log.Printf("Delta: %v", delta.RawJSON())

				if delta.Content != "" {
					content += delta.Content
					events <- Event{
						Type: EventMessage,
						Data: MessageData{Content: delta.Content},
					}
				}

				var extra deltaExtraFields
				if err := json.Unmarshal([]byte(delta.RawJSON()), &extra); err == nil {
					if extra.ReasoningContent != "" {
						reasoningContent += extra.ReasoningContent
						events <- Event{
							Type: EventReasoning,
							Data: ReasoningData{Content: extra.ReasoningContent},
						}
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
			if err := e.contextMgr.AddMessage(ctx, sessionID, &session.Message{
				Role:      "assistant",
				Content:   content,
				Reasoning: reasoningContent,
				ToolCalls: e.convertToolCalls(toolCalls),
			}); err != nil {
				return fmt.Errorf("add assistant message: %w", err)
			}

			for _, tc := range toolCalls {
				if err := e.executeToolCall(ctx, sessionID, agentDef.ToolManager, tc, events); err != nil {
					return fmt.Errorf("execute tool call: %w", err)
				}
			}

			continue
		}

		messageID := uuid.New().String()
		if err := e.contextMgr.AddMessage(ctx, sessionID, &session.Message{
			ID:        uuid.MustParse(messageID),
			Role:      "assistant",
			Content:   content,
			Reasoning: reasoningContent,
		}); err != nil {
			return fmt.Errorf("add assistant message: %w", err)
		}

		events <- Event{
			Type: EventDone,
			Data: DoneData{MessageID: messageID},
		}

		return nil
	}
}

func (e *AgentEngine) executeToolCall(ctx context.Context, sessionID string, toolMgr tool.ToolManager, tc toolCallInfo, events chan<- Event) error {
	events <- Event{
		Type: EventToolCall,
		Data: ToolCallData{
			ToolName: tc.Name,
			Args:     parseArgs(tc.Arguments),
			ID:       tc.ID,
		},
	}

	args := parseArgs(tc.Arguments)
	result, err := toolMgr.Execute(ctx, tc.Name, args)

	var resultData ToolResultData
	if err != nil {
		resultData = ToolResultData{
			ToolName: tc.Name,
			Result:   map[string]any{"error": err.Error()},
			ID:       tc.ID,
		}
	} else {
		resultData = ToolResultData{
			ToolName: tc.Name,
			Result:   result,
			ID:       tc.ID,
		}
	}

	events <- Event{Type: EventToolResult, Data: resultData}

	resultJSON, _ := json.Marshal(resultData.Result)
	if err := e.contextMgr.AddMessage(ctx, sessionID, &session.Message{
		Role:       "tool",
		Content:    string(resultJSON),
		ToolCallID: tc.ID,
	}); err != nil {
		return fmt.Errorf("add tool result message: %w", err)
	}

	return nil
}

func (e *AgentEngine) convertMessages(messages []contextmgr.MessageForLLM) []openai.ChatCompletionMessageParamUnion {
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
