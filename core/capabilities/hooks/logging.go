package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/tool"
)

type LoggingPlugin struct{}

func NewLoggingPlugin() *LoggingPlugin {
	return &LoggingPlugin{}
}

func (h *LoggingPlugin) Name() string {
	return "logging"
}

func (h *LoggingPlugin) Points() []hook.HookPoint {
	return []hook.HookPoint{
		hook.BeforeLLMCall,
		hook.AfterLLMCall,
		hook.BeforeToolExecute,
		hook.AfterToolExecute,
	}
}

func (h *LoggingPlugin) Priority() int {
	return 200
}

func (h *LoggingPlugin) Execute(ctx *hook.HookContext) error {
	switch ctx.CurrentPoint {
	case hook.BeforeLLMCall:
		h.logBeforeLLMCall(ctx)
	case hook.AfterLLMCall:
		h.logAfterLLMCall(ctx)
	case hook.BeforeToolExecute:
		h.logBeforeToolExecute(ctx)
	case hook.AfterToolExecute:
		h.logAfterToolExecute(ctx)
	}
	return nil
}

func (h *LoggingPlugin) logBeforeLLMCall(ctx *hook.HookContext) {
	msgCount := 0
	if ctx.Messages != nil {
		msgCount = len(*ctx.Messages)
	}

	ctx.Logger.Info("before_llm_call",
		"session_id", ctx.SessionID,
		"agent_id", ctx.AgentID,
		"message_count", msgCount,
	)
}

func (h *LoggingPlugin) logAfterLLMCall(ctx *hook.HookContext) {
	ctx.Logger.Info("after_llm_call",
		"session_id", ctx.SessionID,
		"agent_id", ctx.AgentID,
	)
}

func (h *LoggingPlugin) logBeforeToolExecute(ctx *hook.HookContext) {
	ctx.Logger.Info("before_tool_execute",
		"session_id", ctx.SessionID,
		"agent_id", ctx.AgentID,
		"tool_name", ctx.ToolName,
		"tool_args", truncateArgs(ctx.ToolArgs, 500),
	)
}

func (h *LoggingPlugin) logAfterToolExecute(ctx *hook.HookContext) {
	attrs := []any{
		"session_id", ctx.SessionID,
		"agent_id", ctx.AgentID,
		"tool_name", ctx.ToolName,
	}

	if ctx.ToolResult != nil {
		attrs = append(attrs, "success", ctx.ToolResult.Success)
		if ctx.ToolResult.Error != "" {
			attrs = append(attrs, "error", truncateString(ctx.ToolResult.Error, 200))
		}
	} else {
		attrs = append(attrs, "success", false)
	}

	ctx.Logger.Info("after_tool_execute", attrs...)
}

func truncateArgs(args map[string]any, maxLen int) string {
	if args == nil {
		return "{}"
	}

	data, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}

	return truncateString(string(data), maxLen)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var _ hook.Hook = (*LoggingPlugin)(nil)

var _ = tool.ToolResult{}

func init() {
	capabilities.Register(&loggingHookCapability{})
}

type loggingHookCapability struct{}

func (c *loggingHookCapability) Name() string                         { return capabilities.HookLogging }
func (c *loggingHookCapability) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeHook }
func (c *loggingHookCapability) DependsOn() []string                  { return nil }
func (c *loggingHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return NewLoggingPlugin(), nil
}