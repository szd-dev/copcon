# 日志插件

`LoggingPlugin` 提供 Agent 引擎生命周期的结构化可观测性。它在关键 hook 点记录元数据，不记录消息内容，确保隐私安全。

**文件位置**: `server/internal/plugins/logging/logging_plugin.go`

## 核心设计

```go
type LoggingPlugin struct{}
```

`LoggingPlugin` 是一个无状态结构体，仅依赖 `HookContext.Logger` 进行日志输出。不持有任何外部依赖。

### Hook 元数据

| 属性 | 值 | 说明 |
|------|-----|------|
| `Name()` | `"logging"` | 标识符 |
| `Points()` | `[BeforeLLMCall, AfterLLMCall, BeforeToolExecute, AfterToolExecute]` | 4 个 hook 点 |
| `Priority()` | `200` | 高优先级，在所有业务逻辑 hook 之后运行 |

Priority 200 确保日志记录发生在其他 hook 完成工作之后，捕获的是最终状态。

## 四个 Hook 点的日志内容

### BeforeLLMCall

LLM 请求发送前触发。记录请求规模信息：

```go
ctx.Logger.Info("before_llm_call",
    "session_id", ctx.SessionID,
    "agent_id",   ctx.AgentID,
    "message_count", msgCount,
)
```

| 字段 | 来源 | 说明 |
|------|------|------|
| `session_id` | `ctx.SessionID` | 会话标识 |
| `agent_id` | `ctx.AgentID` | 处理请求的 Agent |
| `message_count` | `len(*ctx.Messages)` | 上下文窗口中的消息数量 |

请注意这里只记录消息数量，不记录消息内容。消息本身可能包含敏感的用户数据。

### AfterLLMCall

LLM 响应返回后触发。当前记录基础标识信息：

```go
ctx.Logger.Info("after_llm_call",
    "session_id", ctx.SessionID,
    "agent_id",   ctx.AgentID,
)
```

Token 用量和响应耗时字段已预留，将在引擎增加追踪后接入。

### BeforeToolExecute

工具调用执行前触发。记录工具名称和截断后的参数：

```go
ctx.Logger.Info("before_tool_execute",
    "session_id", ctx.SessionID,
    "agent_id",   ctx.AgentID,
    "tool_name",  ctx.ToolName,
    "tool_args",  truncateArgs(ctx.ToolArgs, 500),
)
```

| 字段 | 来源 | 说明 |
|------|------|------|
| `tool_name` | `ctx.ToolName` | 工具名称 |
| `tool_args` | `ctx.ToolArgs` | 工具参数 JSON，截断至 500 字符 |

`truncateArgs` 将 `map[string]any` 序列化为 JSON 后截断，防止大参数负载淹没日志输出。

### AfterToolExecute

工具执行完成后触发。记录执行结果状态：

```go
ctx.Logger.Info("after_tool_execute",
    "session_id", ctx.SessionID,
    "agent_id",   ctx.AgentID,
    "tool_name",  ctx.ToolName,
    "success",    ctx.ToolResult.Success,
    "error",      truncateString(ctx.ToolResult.Error, 200),  // 仅在失败时
)
```

仅记录成功/失败状态和截断后的错误信息，不记录工具执行的实际输出（可能包含文件内容等敏感数据）。

## 结构化日志格式

日志使用 Go 标准库 `log/slog` 输出。默认 handler 为 `slog.NewTextHandler`，输出格式为：

```
time=2026-05-17T10:30:00.000+08:00 level=INFO msg=before_llm_call session_id=abc123 agent_id=code-assistant message_count=15
time=2026-05-17T10:30:05.000+08:00 level=INFO msg=after_llm_call session_id=abc123 agent_id=code-assistant
time=2026-05-17T10:30:06.000+08:00 level=INFO msg=before_tool_execute session_id=abc123 agent_id=code-assistant tool_name=read_file tool_args={"path":"main.go"}
time=2026-05-17T10:30:07.000+08:00 level=INFO msg=after_tool_execute session_id=abc123 agent_id=code-assistant tool_name=read_file success=true
```

所有日志条目均为 `Info` 级别。成功和失败通过 `success` 字段区分，非通过日志级别。

## 隐私保护

`LoggingPlugin` 遵循以下隐私原则：

1. **消息内容不记录**：`BeforeLLMCall` 和 `AfterLLMCall` 不包含任何消息文本
2. **工具输出不记录**：`AfterToolExecute` 不记录 `Result` 字段内容
3. **参数截断**：`BeforeToolExecute` 中工具参数截断至 500 字符
4. **错误截断**：工具错误信息截断至 200 字符

## 自定义日志 Handler

默认使用 `slog.NewTextHandler(os.Stderr, nil)` 输出到 stderr。可通过 `WithLogger` 引擎选项替换为自定义 handler。

### JSON 输出

```go
import "os"

handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
logger := slog.New(handler)

engine := agent.NewAgentEngine(
    // ...
    agent.WithLogger(logger),
)
```

JSON 格式输出示例：

```json
{"time":"2026-05-17T10:30:00.000+08:00","level":"INFO","msg":"before_llm_call","session_id":"abc123","agent_id":"code-assistant","message_count":15}
```

### 文件输出

```go
f, err := os.OpenFile("/var/log/copcon/agent.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
if err != nil {
    panic(err)
}
handler := slog.NewJSONHandler(f, nil)
logger := slog.New(handler)

engine := agent.NewAgentEngine(
    // ...
    agent.WithLogger(logger),
)
```

### 带上下文的 Logger

`HookContext.Logger` 由引擎在 hook 执行前根据引擎级别的 logger 派生。如果需要为每个请求附加额外字段（如 request_id），可以在调用 `hookRunner.On()` 之前包装：

```go
requestLogger := engineLogger.With("request_id", requestID)
hookRunner.On(hook.BeforeLLMCall, chatCtx, requestLogger, hookExtra)
```