# HookContext 字段参考

`HookContext` 是 Hook 执行时拿到的上下文对象。它包含了当前请求的所有相关数据和 HookPoint 信息。

---

## 结构体定义

```go
// 定义在 server/internal/hook/hook.go
type HookContext struct {
    ChatCtx      iface.ChatContextInterface  // 统一会话上下文 — 始终填充
    SessionID    string                      // 会话 ID — 始终填充
    AgentID      string                      // Agent ID — 始终填充
    SystemPrompt *string                     // 系统提示词指针 — 部分 HookPoint 填充
    Messages     *[]entity.MessageForLLM     // 消息列表指针 — 部分 HookPoint 填充
    ToolName     string                      // 工具名称 — 部分 HookPoint 填充
    ToolArgs     map[string]any              // 工具参数 — 部分 HookPoint 填充
    ToolResult   *tool.ToolResult            // 工具结果指针 — 部分 HookPoint 填充
    Logger       *slog.Logger                // 结构化日志 — 始终填充
    CurrentPoint HookPoint                   // 当前执行的 HookPoint — 始终填充
}
```

---

## 字段总览表

### 始终填充的字段

以下字段无论哪个 HookPoint 都保证有值：

| 字段 | 类型 | 说明 |
|------|------|------|
| `ChatCtx` | `iface.ChatContextInterface` | 统一会话上下文。提供 `SessionID()`、`AgentID()`、`Context()`、`Emit()` 等方法 |
| `SessionID` | `string` | 当前会话 ID，等价于 `ChatCtx.SessionID()` |
| `AgentID` | `string` | 当前 Agent ID，等价于 `ChatCtx.AgentID()` |
| `Logger` | `*slog.Logger` | 请求级别的结构化日志。带 `session_id` 和 `agent_id` 上下文 |
| `CurrentPoint` | `HookPoint` | 当前触发的 HookPoint。用这个字段区分注册了多个 Point 的 Hook |

### 按 HookPoint 填充的字段

以下字段只在特定 HookPoint 下有值：

| 字段 | 类型 | 可变 | 填充时机 |
|------|------|------|----------|
| `SystemPrompt` | `*string` | ✓ | `on_system_prompt`, `before_context_build` |
| `Messages` | `*[]entity.MessageForLLM` | ✓ | `after_context_build`, `before_llm_call`, `after_llm_call`, `on_message_persist` |
| `ToolName` | `string` | ✗ | `before_tool_execute`, `after_tool_execute`, `on_tool_error` |
| `ToolArgs` | `map[string]any` | ✓ | `before_tool_execute`, `after_tool_execute`, `on_tool_error` |
| `ToolResult` | `*tool.ToolResult` | ✓ | `after_tool_execute`, `on_tool_error` |

**"可变"的含义**：Hook 可以通过 `*ctx.SystemPrompt = "new value"` 修改该字段的值。修改会影响后续 Hook 和引擎流程。

**"不可变"的含义**：`ToolName` 是值类型 `string`，修改 `ctx.ToolName` 不会影响引擎流程（因为 `Run` 方法创建的 `HookContext` 是一个副本）。如果你需要改变工具调用行为，应修改 `ToolArgs` 或 `ToolResult`。

---

## 逐字段详解

### ChatCtx

```go
ChatCtx iface.ChatContextInterface
```

`ChatCtx` 是整个请求流转的统一上下文。它封装了 session 标识和事件流能力。在 Hook 中你最常用的三个方法：

- `ctx.ChatCtx.SessionID()` — 获取 Session ID（等价于 `ctx.SessionID`）
- `ctx.ChatCtx.Context()` — 获取 Go 标准 context，用于数据库操作和新 goroutine 派生
- `ctx.ChatCtx.Emit(event)` — 向客户端推送 SSE 事件

### Logger

```go
Logger *slog.Logger
```

请求级别的结构化日志。已经预填充了 `session_id` 和 `agent_id`，你调用时不需要再手动加这些字段。

```go
ctx.Logger.Info("processing hook",
    "hook_name", h.Name(),
    "point", ctx.CurrentPoint,
)
```

### Messages

```go
Messages *[]entity.MessageForLLM
```

消息列表的指针。`*Messages` 是发送给 LLM 的消息数组。修改消息列表会影响 LLM 的输入。

**nil 检查**：在 `before_tool_execute` 等非消息相关的 HookPoint 上，`Messages` 是 `nil`。使用前务必检查。

**修改方式**：直接修改指针指向的内容。

```go
// 追加一条系统消息
*ctx.Messages = append([]entity.MessageForLLM{
    {Role: "system", Content: "附加上下文...  "},
}, *ctx.Messages...)

// 修改最后一条消息的内容
last := &(*ctx.Messages)[len(*ctx.Messages)-1]
last.Content = strings.ReplaceAll(last.Content, "old", "new")
```

### ToolArgs

```go
ToolArgs map[string]any
```

工具调用的参数 map。对于 `BeforeToolExecute`，修改 `ToolArgs` 会直接影响工具收到的参数。

```go
// 在 before_tool_execute 中拦截并改写参数
if ctx.ToolName == "shell" {
    // 给所有 shell 命令加上超时限制
    ctx.ToolArgs["timeout"] = 30
}
```

### ToolResult

```go
ToolResult *tool.ToolResult
```

工具执行结果的指针。`ToolResult` 的结构：

```go
type ToolResult struct {
    Success bool   `json:"success"`
    Data    any    `json:"data,omitempty"`
    Error   string `json:"error,omitempty"`
}
```

**在 `after_tool_execute` 中**：`ToolResult` 有值，你可以修改 `Success`、`Data`、`Error` 字段。

**在 `on_tool_error` 中**：`ToolResult` 可能是 `nil`。你可以设置一个新的 `ToolResult` 作为兜底结果。

**nil 检查**：`ToolResult` 是指针，在 `on_tool_error` 中可能是 `nil`。使用前务必检查。

```go
if ctx.ToolResult == nil {
    // 工具执行出错，没有结果返回。提供一个兜底结果。
    ctx.ToolResult = &tool.ToolResult{
        Success: false,
        Data:    "工具执行超时，请重试",
    }
    return nil
}

// 正常修改结果
ctx.ToolResult.Data = truncatedData
```

### SystemPrompt

```go
SystemPrompt *string
```

系统提示词的指针。可以在 `OnSystemPrompt` 中替换整个提示词，或在 `BeforeContextBuild` 中预先修改。

```go
// 替换整个系统提示词
*ctx.SystemPrompt = customPrompt

// 在原提示词末尾追加内容
*ctx.SystemPrompt = *ctx.SystemPrompt + "\n\n当前待办列表：\n" + todoList
```

---

## 可变性总结

| 字段 | 可变 | 修改方式 | 影响范围 |
|------|------|---------|---------|
| `SystemPrompt` | ✓ | `*ctx.SystemPrompt = "..."` | 后续 Hook + 引擎 |
| `Messages` | ✓ | `*ctx.Messages = modified` | 后续 Hook + LLM 输入 |
| `ToolArgs` | ✓ | `ctx.ToolArgs["key"] = val` | 工具收到的参数 |
| `ToolResult` | ✓ | `ctx.ToolResult.Data = ...` | 后续 Hook + 返回给 LLM |
| `ToolName` | ✗ | 不可靠（值类型副本） | 不影响引擎 |
| `SessionID` | ✗ | 只读 | — |
| `AgentID` | ✗ | 只读 | — |
| `CurrentPoint` | ✗ | 只读 | — |
| `ChatCtx` | ✗ | 只读 | — |
| `Logger` | ✗ | 只读 | — |

---

## nil 处理的正确姿势

不是所有 HookPoint 都填充所有字段。以下字段在使用前**必须**做 nil 检查：

```go
func (h *MyHook) Execute(ctx *hook.HookContext) error {
    if ctx.SystemPrompt != nil {
        // 可以安全读写
        *ctx.SystemPrompt = newPrompt
    }

    if ctx.Messages != nil && len(*ctx.Messages) > 0 {
        // 可以安全读写
        for i := range *ctx.Messages {
            // ...
        }
    }

    if ctx.ToolResult != nil {
        // 可以安全读写
        ctx.ToolResult.Data = processedData
    }

    // ToolArgs 如果是 nil，读写都会 panic
    // 但引擎保证在相关 Point 上它不会是 nil

    return nil
}
```

---

## HookExtra — 引擎内部使用的辅助结构

你通常不需要直接使用 `HookExtra`，但了解它有助于理解字段是怎么传到 HookContext 里的：

```go
type HookExtra struct {
    ToolName     *string
    ToolArgs     map[string]any
    ToolResult   *tool.ToolResult
    SystemPrompt *string
    Messages     *[]entity.MessageForLLM
}
```

引擎在执行 `On` 方法时，先构造基本的 `HookContext`，然后通过 `HookExtra` 把当前调用特有的字段填进去：

```go
runner.On(
    hook.AfterContextBuild,
    chatCtx,
    logger,
    hook.HookExtra{Messages: &messages},
)
```

---

## 相关文档

- [编写一个 Hook](./writing-a-hook.md) — Execute 方法的实现指南
- [Hook 注册与运行](./hook-registry.md) — On 和 Run 的使用方式
- [Hook 系统概览](./overview.md) — 回到总览