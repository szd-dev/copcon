// Package logging provides a LoggingPlugin for structured observability
// of the agent engine lifecycle. It logs metadata at key hook points
// without logging message content (privacy-preserving).
package logging

import (
	"encoding/json"
	"fmt"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/tool"
)

// LoggingPlugin implements hook.Hook to provide structured logging at
// BeforeLLMCall, AfterLLMCall, BeforeToolExecute, and AfterToolExecute
// hook points. Only metadata is logged — no message content or tool
// result data is exposed.
type LoggingPlugin struct{}

// NewLoggingPlugin creates a new LoggingPlugin.
func NewLoggingPlugin() *LoggingPlugin {
	return &LoggingPlugin{}
}

// Name returns a human-readable identifier for logging and debugging.
func (h *LoggingPlugin) Name() string {
	return "logging"
}

// Points returns the hook points at which this plugin should execute.
func (h *LoggingPlugin) Points() []hook.HookPoint {
	return []hook.HookPoint{
		hook.BeforeLLMCall,
		hook.AfterLLMCall,
		hook.BeforeToolExecute,
		hook.AfterToolExecute,
	}
}

// Priority returns the execution order. 200 means this plugin runs
// after most other hooks, ensuring they have finished their work
// before observability data is captured.
func (h *LoggingPlugin) Priority() int {
	return 200
}

// Execute dispatches to the appropriate logging method based on
// the current hook point. It never returns an error — logging
// failures are handled internally and never block the pipeline.
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

// logBeforeLLMCall logs metadata before an LLM request is dispatched.
// Logs session, agent, and message count.
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

// logAfterLLMCall logs metadata after an LLM response is received.
// Token usage and duration are logged when available (currently
// not tracked by the engine).
func (h *LoggingPlugin) logAfterLLMCall(ctx *hook.HookContext) {
	ctx.Logger.Info("after_llm_call",
		"session_id", ctx.SessionID,
		"agent_id", ctx.AgentID,
	)
}

// logBeforeToolExecute logs metadata before a tool is invoked.
// Tool args are serialized and truncated if they exceed 500 characters.
func (h *LoggingPlugin) logBeforeToolExecute(ctx *hook.HookContext) {
	ctx.Logger.Info("before_tool_execute",
		"session_id", ctx.SessionID,
		"agent_id", ctx.AgentID,
		"tool_name", ctx.ToolName,
		"tool_args", truncateArgs(ctx.ToolArgs, 500),
	)
}

// logAfterToolExecute logs metadata after a tool has completed.
// The result status (success/error) is logged, but result data is not.
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

// truncateArgs serializes a tool args map to JSON and truncates
// the result if it exceeds maxLen characters. This prevents
// large argument payloads from flooding log output.
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

// truncateString returns the input string truncated to maxLen
// characters with a "..." suffix if truncation occurred.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Compile-time interface check.
var _ hook.Hook = (*LoggingPlugin)(nil)

// Compile-time import check for tool package.
var _ = tool.ToolResult{}
