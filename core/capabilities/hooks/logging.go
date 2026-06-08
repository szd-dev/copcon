package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
)

type LoggingPluginConfig struct {
	Enabled       bool
	SystemPrompt  bool
	DetailContext bool
	LLMResponse   bool
	Writer        io.Writer
}

type LoggingPlugin struct {
	cfg    LoggingPluginConfig
	logger *slog.Logger
}

func NewLoggingPlugin(cfg LoggingPluginConfig) *LoggingPlugin {
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}
	return &LoggingPlugin{
		cfg:    cfg,
		logger: slog.New(slog.NewTextHandler(cfg.Writer, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
}

func (h *LoggingPlugin) Name() string {
	return "logging"
}

func (h *LoggingPlugin) Points() []hook.HookPoint {
	return []hook.HookPoint{
		hook.OnSystemPrompt,
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
	if !h.cfg.Enabled {
		return nil
	}

	switch ctx.CurrentPoint {
	case hook.OnSystemPrompt:
		h.logSystemPrompt(ctx)
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

func (h *LoggingPlugin) logSystemPrompt(ctx *hook.HookContext) {
	if ctx.SystemPrompt == nil || *ctx.SystemPrompt == "" {
		return
	}

	attrs := []any{
		"session_id", ctx.SessionID,
		"agent_id", ctx.AgentID,
	}
	if h.cfg.SystemPrompt {
		attrs = append(attrs, "system_prompt", *ctx.SystemPrompt)
	}
	h.logger.Info("system_prompt", attrs...)
}

func (h *LoggingPlugin) logBeforeLLMCall(ctx *hook.HookContext) {
	msgCount := 0
	if ctx.Messages != nil {
		msgCount = len(*ctx.Messages)
	}

	attrs := []any{
		"session_id", ctx.SessionID,
		"agent_id", ctx.AgentID,
		"message_count", msgCount,
	}

	if h.cfg.DetailContext && ctx.Messages != nil {
		attrs = append(attrs, "messages", formatMessages(*ctx.Messages))
	}

	h.logger.Info("before_llm_call", attrs...)
}

func (h *LoggingPlugin) logAfterLLMCall(ctx *hook.HookContext) {
	if ctx.LLMResponse == nil {
		h.logger.Info("after_llm_call",
			"session_id", ctx.SessionID,
			"agent_id", ctx.AgentID,
		)
		return
	}

	resp := ctx.LLMResponse
	attrs := []any{
		"session_id", ctx.SessionID,
		"agent_id", ctx.AgentID,
		"content_len", len(resp.Content),
		"reasoning_len", len(resp.ReasoningContent),
		"tool_calls", resp.ToolCallCount,
		"prompt_tokens", resp.PromptTokens,
		"completion_tokens", resp.CompletionTokens,
		"total_tokens", resp.TotalTokens,
	}

	if h.cfg.LLMResponse {
		if resp.Content != "" {
			attrs = append(attrs, "content", resp.Content)
		}
		if resp.ReasoningContent != "" {
			attrs = append(attrs, "reasoning", resp.ReasoningContent)
		}
	}

	h.logger.Info("after_llm_call", attrs...)
}

func (h *LoggingPlugin) logBeforeToolExecute(ctx *hook.HookContext) {
	h.logger.Info("before_tool_execute",
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

	h.logger.Info("after_tool_execute", attrs...)
}

func formatMessages(msgs []entity.MessageForLLM) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, m := range msgs {
		if i > 0 {
			sb.WriteString(", ")
		}
		tcStr := ""
		if len(m.ToolCalls) > 0 {
			tcNames := make([]string, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				tcNames[j] = tc.Function.Name
			}
			tcStr = fmt.Sprintf(", tool_calls=%v", tcNames)
		}
		content := truncateString(m.Content, 200)
		sb.WriteString(fmt.Sprintf("{role=%s, content=%q%s}", m.Role, content, tcStr))
	}
	sb.WriteString("]")
	return sb.String()
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