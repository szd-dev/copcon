package tools

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/copcon/server/internal/agent"
	"github.com/copcon/server/internal/chat_context"
	chatcontextpkg "github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
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

func (t *DelegateToTool) Execute(chatCtx chatcontextpkg.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
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

	subSession, err := t.sessionMgr.Create(
		chatCtx,
		fmt.Sprintf("Sub-agent: %s", agentID),
		agentID,
		session.WithParentSessionID(parentSessionID),
	)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("failed to create sub-session: %v", err)}, nil
	}

	subChatCtx := chatcontextpkg.NewChatContext(chatCtx.Context(), subSession.ID.String(), agentID)
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

func buildSummary(chatCtx chatcontextpkg.ChatContextInterface) string {
	msg := map[string]string{
		"session_id": chatCtx.SessionID(),
		"agent_id":   chatCtx.AgentID(),
		"depth":      fmt.Sprintf("%d", chatCtx.Depth()),
	}
	data, _ := json.Marshal(msg)
	return string(data)
}

func collectSummary(contextMgr chat_context.ContextManager, chatCtx chatcontextpkg.ChatContextInterface) string {
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
