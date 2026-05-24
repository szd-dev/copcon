# Logging Hook

The logging hook emits structured log entries at key points in the agent lifecycle. It records LLM calls and tool executions with enough detail to reconstruct what happened during a session, without being noisy.

## Purpose

Observability starts with knowing what your agents are doing. The logging hook gives you a consistent, structured record of:

- When LLM calls happen and which session/agent they belong to
- What tools are invoked, with their arguments and results
- Whether tool executions succeed or fail

This is the baseline hook. Most deployments should enable it.

## Hook Points

| Hook Point | What Gets Logged |
|------------|-----------------|
| `BeforeLLMCall` | Session ID, agent ID, message count |
| `AfterLLMCall` | Session ID, agent ID |
| `BeforeToolExecute` | Session ID, agent ID, tool name, tool arguments (truncated) |
| `AfterToolExecute` | Session ID, agent ID, tool name, success/failure, error (if any) |

Priority: **200** (runs before lower-priority hooks like memory).

## How It Works

The logging hook uses `slog` structured logging. Every log entry includes key-value pairs that make it easy to filter and search in log aggregation systems.

### Before LLM Call

```go
ctx.Logger.Info("before_llm_call",
    "session_id", ctx.SessionID,
    "agent_id", ctx.AgentID,
    "message_count", msgCount,
)
```

Output example:
```
level=INFO msg=before_llm_call session_id=sess_abc123 agent_id=assistant message_count=8
```

### After LLM Call

```go
ctx.Logger.Info("after_llm_call",
    "session_id", ctx.SessionID,
    "agent_id", ctx.AgentID,
)
```

### Before Tool Execute

```go
ctx.Logger.Info("before_tool_execute",
    "session_id", ctx.SessionID,
    "agent_id", ctx.AgentID,
    "tool_name", ctx.ToolName,
    "tool_args", truncateArgs(ctx.ToolArgs, 500),
)
```

Tool arguments are JSON-serialized and truncated to 500 characters to keep logs readable. If serialization fails, the hook logs `<marshal error: ...>` instead of crashing.

### After Tool Execute

```go
ctx.Logger.Info("after_tool_execute",
    "session_id", ctx.SessionID,
    "agent_id", ctx.AgentID,
    "tool_name", ctx.ToolName,
    "success", ctx.ToolResult.Success,
    "error", truncateString(ctx.ToolResult.Error, 200),  // if present
)
```

Error messages are truncated to 200 characters.

## Configuration

### YAML

```yaml
hooks:
  - name: "logging"
    type: "file_logger"
    enabled: true
    parameters:
      file: "./logs/copcon.log"
      level: "info"           # debug, info, warn, error
      max_size: "100mb"
      max_backups: 30
      max_age_days: 90
```

### Go

```go
harness := core.NewHarness(core.HarnessConfig{
    Hooks: []HookSpec{
        {Name: "hooks.logging", Enabled: true},
    },
})
```

The logging hook has no external dependencies. It uses the `Logger` field from `HookContext`, which the engine provides automatically.

### Environment Variable

You can override the log level without changing the config file:

```bash
export COPCON_LOG_LEVEL=debug
```

## Logged Fields Reference

| Field | When Present | Description |
|-------|-------------|-------------|
| `session_id` | Always | The session this event belongs to |
| `agent_id` | Always | The agent handling the request |
| `message_count` | `BeforeLLMCall` | Number of messages in the context window |
| `tool_name` | Tool events | Name of the tool being executed |
| `tool_args` | `BeforeToolExecute` | JSON-serialized arguments, truncated to 500 chars |
| `success` | `AfterToolExecute` | Whether the tool execution succeeded |
| `error` | `AfterToolExecute` (on failure) | Error message, truncated to 200 chars |

## Performance Impact

The logging hook is lightweight. It does no I/O beyond what `slog` already provides, and argument truncation keeps log volume bounded.

| Aspect | Impact |
|--------|--------|
| Latency per event | Sub-millisecond (struct field reads + slog write) |
| Argument truncation | JSON marshal + string slice, bounded by max length |
| Concurrency | Safe. The hook holds no mutable state. |

### Tips

- Set the log level to `warn` or `error` in production if you only care about failures. The `AfterToolExecute` event logs errors at `info` level, so consider adjusting the hook if you need error-only logging.
- For high-throughput agents, send logs to a file or log aggregation service rather than stdout to avoid interleaving with SSE output.
- The 500-character truncation for tool args is a sensible default. If you need full args for debugging, temporarily lower the log level and adjust the truncation limit in a custom hook.

## Example: Basic Agent with Logging

```yaml
agents:
  - name: "assistant"
    model: "gpt-4"
    system_prompt: "You are a helpful assistant."
    hooks:
      - "logging"

hooks:
  - name: "logging"
    enabled: true
    parameters:
      file: "./logs/agent.log"
      level: "info"
```

With this setup, every LLM call and tool execution will be logged with session and agent context.
