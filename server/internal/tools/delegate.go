package tools

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/copcon/core/agent"
	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/core/chatcontext"
	"github.com/copcon/core/context_builder"
	"github.com/copcon/core/entity"
	iface "github.com/copcon/core/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/core/tool"
)

// DelegateToTool delegates a task to a sub-agent running in its own session.
type DelegateToTool struct {
	agentRegistry agent.AgentRegistry
	sessionMgr    session.SessionManager
	contextMgr    chat_context.ContextManager
	engine        agent.AgentEngine
}

func NewDelegateToTool(
	agentRegistry agent.AgentRegistry,
	sessionMgr session.SessionManager,
	contextMgr chat_context.ContextManager,
	engine agent.AgentEngine,
) *DelegateToTool {
	return &DelegateToTool{
		agentRegistry: agentRegistry,
		sessionMgr:    sessionMgr,
		contextMgr:    contextMgr,
		engine:        engine,
	}
}

func (t *DelegateToTool) Name() string {
	return "delegate_to"
}

func (t *DelegateToTool) Description() string {
	return "Delegate a task to another agent"
}

func (t *DelegateToTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "ID of the agent to delegate to",
			},
			"task": map[string]any{
				"type":        "string",
				"description": "Task description for the sub-agent",
			},
			"mode": map[string]any{
				"type":    "string",
				"default": "sync",
				"enum":    []string{"sync"},
			},
			"extra": map[string]any{
				"type": "object",
			},
		},
		"required": []string{"agent_id", "task"},
	}
}

func (t *DelegateToTool) IsDelegationTool() bool {
	return true
}

func (t *DelegateToTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	agentID, _ := args["agent_id"].(string)
	task, _ := args["task"].(string)

	if agentID == "" {
		return &tool.ToolResult{Success: false, Error: "agent_id is required"}, nil
	}
	if task == "" {
		return &tool.ToolResult{Success: false, Error: "task is required"}, nil
	}

	factory, err := t.agentRegistry.GetFactory(agentID)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("agent not found: %s", agentID)}, nil
	}

	parentSummary := buildSummary(chatCtx)

	_, err = factory(chatCtx.Context(), agent.CreateParams{
		Task:          task,
		ParentContext: parentSummary,
	})
	if err != nil {
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("failed to create agent: %v", err)}, nil
	}

	parentSessionID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("invalid parent session ID: %v", err)}, nil
	}

	subSession, err := t.sessionMgr.CreateSession(
		chatCtx,
		fmt.Sprintf("Sub-agent: %s", agentID),
		agentID,
		session.WithParentSessionID(parentSessionID),
	)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("failed to create sub-session: %v", err)}, nil
	}

	subChatCtx := chatcontext.NewChatContext(chatCtx.Context(), subSession.ID.String(), agentID)
	subChatCtx.WithDepth(chatCtx.Depth() + 1)

	if err := t.contextMgr.AddMessage(subChatCtx, &session.Message{
		Role:    "user",
		Content: task,
	}); err != nil {
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("failed to add task message: %v", err)}, nil
	}

	go t.engine.Chat(subChatCtx, task)

	<-subChatCtx.Closed()

	summary := collectSummary(t.contextMgr, subChatCtx)

	return &tool.ToolResult{
		Success: true,
		Data: map[string]any{
			"sub_session_id": subSession.ID.String(),
			"summary":        summary,
			"status":         "completed",
		},
	}, nil
}

func buildSummary(chatCtx iface.ChatContextInterface) string {
	msg := map[string]string{
		"session_id": chatCtx.SessionID(),
		"agent_id":   chatCtx.AgentID(),
		"depth":      fmt.Sprintf("%d", chatCtx.Depth()),
	}
	data, _ := json.Marshal(msg)
	return string(data)
}

func collectSummary(contextMgr chat_context.ContextManager, chatCtx iface.ChatContextInterface) string {
	messages, err := contextMgr.GetHistory(chatCtx, 20)
	if err != nil || len(messages) == 0 {
		return "Task completed"
	}

	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			return messages[i].Content
		}
	}

	return "Task completed"
}

var _ tool.Tool = (*DelegateToTool)(nil)
var _ tool.DelegationTool = (*DelegateToTool)(nil)

// ReadSubSessionTool retrieves messages from a sub-session created by delegate_to.
type ReadSubSessionTool struct {
	sessionMgr session.SessionManager
	contextMgr chat_context.ContextManager
}

func NewReadSubSessionTool(sessionMgr session.SessionManager, contextMgr chat_context.ContextManager) *ReadSubSessionTool {
	return &ReadSubSessionTool{
		sessionMgr: sessionMgr,
		contextMgr: contextMgr,
	}
}

func (t *ReadSubSessionTool) Name() string {
	return "read_sub_session"
}

func (t *ReadSubSessionTool) Description() string {
	return "Read messages from a sub-session created by delegate_to"
}

func (t *ReadSubSessionTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"sub_session_id": map[string]any{
				"type":        "string",
				"description": "ID of the sub-session to read",
			},
		},
		"required": []string{"sub_session_id"},
	}
}

func (t *ReadSubSessionTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	subSessionID, _ := args["sub_session_id"].(string)
	if subSessionID == "" {
		return &tool.ToolResult{Success: false, Error: "sub_session_id is required"}, nil
	}

	subChatCtx := chatcontext.NewChatContext(chatCtx.Context(), subSessionID, "")
	subSession, err := t.sessionMgr.GetSession(subChatCtx)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: "sub-session not found"}, nil
	}

	// Verify the sub-session is a child of the current session
	if subSession.ParentSessionID == nil || subSession.ParentSessionID.String() != chatCtx.SessionID() {
		return &tool.ToolResult{Success: false, Error: "sub-session not found"}, nil
	}

	messages, err := t.contextMgr.GetHistory(subChatCtx, 0)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("failed to get messages: %v", err)}, nil
	}

	uiMessages := convertMessagesToUI(messages)

	return &tool.ToolResult{
		Success: true,
		Data: map[string]any{
			"messages": uiMessages,
		},
	}, nil
}

// convertMessagesToUI converts session.Message records to entity.UIMessage format,
// matching the structure returned by the GET /messages endpoint.
func convertMessagesToUI(messages []session.Message) []entity.UIMessage {
	// Build tool-result lookup: ToolCallID → Content
	toolResultByCallID := make(map[string]string)
	for _, msg := range messages {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			toolResultByCallID[msg.ToolCallID] = msg.Content
		}
	}

	var uiMessages []entity.UIMessage
	for _, msg := range messages {
		// Skip tool-role messages; their content is embedded in tool-call parts
		if msg.Role == "tool" {
			continue
		}

		if len(msg.Parts) > 0 {
			uiParts := make([]entity.UIPart, 0, len(msg.Parts))
			for _, p := range msg.Parts {
				uiPart := entity.UIPart{
					Type:       entity.UIPartType(p.Type),
					Text:       p.Text,
					State:      entity.UIPartState(p.State),
					ToolCallID: p.ToolCallID,
					ToolName:   p.ToolName,
					Args:       p.Args,
					Output:     p.Output,
					Error:      p.Error,
					StepIndex:  p.StepIndex,
				}
				// Populate output from tool-result lookup for tool-call parts
				if p.Type == "tool-call" && uiPart.ToolCallID != "" && uiPart.Output == "" {
					if result, ok := toolResultByCallID[uiPart.ToolCallID]; ok {
						uiPart.Output = result
					}
				}
				uiParts = append(uiParts, uiPart)
			}
			steps := context_builder.GroupPartsByStep(uiParts)
			uiMessages = append(uiMessages, entity.UIMessage{
				ID:        msg.ID.String(),
				SessionID: msg.SessionID.String(),
				Role:      msg.Role,
				Steps:     steps,
				Metadata: entity.UIMetadata{
					CreatedAt:  msg.CreatedAt,
					Model:      msg.Model,
					TokenCount: msg.TokenCount,
					DurationMs: msg.DurationMs,
				},
			})
		} else {
			uiMsg := context_builder.SynthesizeUIMessage(sessionMsgToLegacy(msg), toolResultByCallID)
			if uiMsg != nil {
				uiMsg.SessionID = msg.SessionID.String()
				uiMsg.Metadata = entity.UIMetadata{
					CreatedAt:  msg.CreatedAt,
					Model:      msg.Model,
					TokenCount: msg.TokenCount,
					DurationMs: msg.DurationMs,
				}
				uiMessages = append(uiMessages, *uiMsg)
			}
		}
	}

	return uiMessages
}

var _ tool.Tool = (*ReadSubSessionTool)(nil)
