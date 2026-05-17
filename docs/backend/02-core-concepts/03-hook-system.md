# Hook 系统

## 概述

Hook 系统是核心-外设架构的基础设施，定义在 `internal/hook/`。它允许外部代码在 Agent 引擎生命周期的关键节点注入自定义逻辑，而不需要修改核心引擎代码。

**核心思想**：引擎提供拦截点，外设通过 Hook 注册自己的行为。

## Hook 接口

定义在 `internal/hook/hook.go`：

```go
type Hook interface {
    // Name 返回 Hook 的人类可读标识，用于日志和调试。
    Name() string

    // Points 返回此 Hook 注册的 HookPoint 集合。
    // 一个 Hook 可以注册多个 HookPoint。
    Points() []HookPoint

    // Priority 返回执行顺序。数值越小越先执行。
    // 默认值为 100。
    Priority() int

    // Execute 在引擎到达此 Hook 注册的 HookPoint 时被调用。
    // 返回的 error 不会中止管道——仅记录日志。
    Execute(ctx *HookContext) error
}
```

### 实现示例

```go
// 一个简单的日志 Hook，在所有 LLM 调用前后打印日志
type LoggingHook struct{}

func (h *LoggingHook) Name() string {
    return "logging-hook"
}

func (h *LoggingHook) Points() []hook.HookPoint {
    return []hook.HookPoint{
        hook.BeforeLLMCall,
        hook.AfterLLMCall,
    }
}

func (h *LoggingHook) Priority() int {
    return 50 // 较早执行
}

func (h *LoggingHook) Execute(ctx *hook.HookContext) error {
    switch ctx.CurrentPoint {
    case hook.BeforeLLMCall:
        ctx.Logger.Info("about to call LLM",
            "session_id", ctx.SessionID,
            "message_count", len(*ctx.Messages),
        )
    case hook.AfterLLMCall:
        ctx.Logger.Info("LLM call completed",
            "session_id", ctx.SessionID,
        )
    }
    return nil
}
```

## Hook 注册

```go
runner := hook.NewHookRunner()
runner.Register(&LoggingHook{})
runner.Register(&MemoryHook{})
runner.Register(&TodoHook{})

engine := agent.NewAgentEngine(
    registry, sessionMgr, contextMgr, asyncReg,
    agent.WithHookRunner(runner),
)
```

## 10 个 HookPoint 常量

全部定义在 `internal/hook/hook.go`，下面逐一说明。

| HookPoint | 常量值 | 触发时机 | 用途 |
|-----------|--------|---------|------|
| `OnSessionResolve` | `"on_session_resolve"` | 会话加载后 | 会话级别的初始化：注入会话上下文、设置 metadata |
| `OnSystemPrompt` | `"on_system_prompt"` | 系统提示解析时 | 替换或增强系统提示（如注入 Todo 列表、用户偏好） |
| `BeforeContextBuild` | `"before_context_build"` | 构建上下文窗口前 | 预先修改 system prompt，影响后续的消息检索 |
| `AfterContextBuild` | `"after_context_build"` | 上下文窗口构建后 | 检查或修改组装好的消息列表（如注入记忆） |
| `BeforeLLMCall` | `"before_llm_call"` | LLM API 调用前 | 最终检查消息列表、统计 token、记录请求日志 |
| `AfterLLMCall` | `"after_llm_call"` | LLM API 返回后 | 检查响应、记录延迟、触发异步任务 |
| `BeforeToolExecute` | `"before_tool_execute"` | 工具执行前 | 修改工具参数、权限检查、速率限制 |
| `AfterToolExecute` | `"after_tool_execute"` | 工具执行成功后 | 转换工具结果、记录审计日志 |
| `OnToolError` | `"on_tool_error"` | 工具执行失败时 | 提供降级结果、重试逻辑、错误上报 |
| `OnMessagePersist` | `"on_message_persist"` | 消息持久化到存储时 | 转换或过滤消息内容、触发索引更新 |

## HookContext

Hook 执行时接收的上下文对象：

```go
type HookContext struct {
    ChatCtx      iface.ChatContextInterface  // 始终可用
    SessionID    string                       // 始终可用
    AgentID      string                       // 始终可用
    Logger       *slog.Logger                 // 始终可用
    CurrentPoint HookPoint                    // 始终可用

    // 可选字段 — 只在特定 HookPoint 可用
    SystemPrompt *string                       // OnSystemPrompt, BeforeContextBuild
    Messages     *[]entity.MessageForLLM       // AfterContextBuild, BeforeLLMCall, AfterLLMCall
    ToolName     string                        // BeforeToolExecute, AfterToolExecute, OnToolError
    ToolArgs     map[string]any                // BeforeToolExecute, AfterToolExecute, OnToolError
    ToolResult   *tool.ToolResult              // AfterToolExecute, OnToolError
}
```

关键设计：
- 指针字段（`*string`, `*[]MessageForLLM`, `*ToolResult`）用于**可变性**。Hook 可以通过设置这些字段来改变后续管道行为。
- 非指针字段只读。Hook 不应修改它们。
- 字段可用性取决于 `CurrentPoint`。访问未填充的字段不会 panic，但值为零值/nil。

## 执行语义

### 优先级排序

Hook 按优先级从高到低执行（数值越小越优先），同优先级按注册时间从早到晚：

```go
// runner.go 中的排序逻辑
sort.Slice(matched, func(i, j int) bool {
    pi, pj := matched[i].hook.Priority(), matched[j].hook.Priority()
    if pi != pj {
        return pi > pj // 优先级高（数值小）的先执行
    }
    return matched[i].createdAt.Before(matched[j].createdAt) // 先注册的先执行
})
```

### Panic 恢复

每个 Hook 的 `Execute()` 被包裹在 `recover()` 中。Hook 的 panic 不会中止管道：

```go
func (r *hookRunner) executeHook(h Hook, ctx *HookContext) {
    func() {
        defer func() {
            if rec := recover(); rec != nil {
                slog.Error("hook panicked", "hook", h.Name(), "panic", rec)
            }
        }()
        if err := h.Execute(ctx); err != nil {
            slog.Warn("hook returned error", "hook", h.Name(), "error", err)
        }
    }()
}
```

### 错误处理

Hook 返回的 `error` 只记录日志，不会中止链。如果需要中止引擎执行，Hook 应该通过其他机制（如设置 context 取消、或通过 ChatContext 发送错误事件）来实现。

### Context 取消

如果 `ctx.ChatCtx.Context()` 已取消（连接断开或超时），`Run()` 立即返回，不执行任何 Hook：

```go
func (r *hookRunner) Run(point HookPoint, ctx *HookContext) {
    if err := ctx.ChatCtx.Context().Err(); err != nil {
        return // context 已取消，跳过
    }
    // ...
}
```

## HookPoint 使用决策指南

| 你想要... | 使用这个 HookPoint |
|-----------|-------------------|
| 在 System Prompt 中注入当前 Todo 列表 | `OnSystemPrompt` |
| 将最近的记忆注入上下文窗口 | `AfterContextBuild` |
| 记录每次 LLM 请求的消息数和工具数 | `BeforeLLMCall` |
| 统计 LLM 延迟和 token 消耗 | `AfterLLMCall` |
| 工具调用前检查权限 | `BeforeToolExecute` |
| 记录工具调用结果用于审计 | `AfterToolExecute` |
| 工具失败时提供缓存降级结果 | `OnToolError` |
| 消息持久化时更新向量索引 | `OnMessagePersist` |
| 会话创建时初始化 metadata | `OnSessionResolve` |
| 过滤或重写上下文中的敏感信息 | `AfterContextBuild` |

## Hook vs Middleware 类比

Hook 系统与传统 Web 框架中的 Middleware 模式有相似之处，但有关键差异：

| 特性 | Web Middleware | CopCon Hook |
|------|---------------|-------------|
| 拦截点 | 请求/响应链 | Agent 生命周期中的 10 个精确节点 |
| 控制流 | 链式 next() 调用 | 引擎决定何时执行，Hook 不控制流 |
| 错误传播 | 通常可以中止请求 | 错误只记录，不中止 |
| 数据修改 | 修改 request/response | 修改 HookContext 指针字段 |
| 注册方式 | 中间件函数 | 实现 Hook 接口 |
| 执行顺序 | 注册顺序 | 优先级 + 注册时间 |
| 可扩展性 | 单一路径 | 多个 HookPoint 组合 |

二者的本质区别：Middleware 控制请求处理流（"在请求处理前后做某事"）；Hook 是观察者模式的变体，Hook 不控制引擎何时继续，引擎在 Hook 执行完毕后自主决定下一步。

## HookRunner 接口

```go
type HookRunner interface {
    // Register 注册一个 Hook。并发安全。
    Register(hook Hook)

    // Run 分发给定 HookPoint 的所有已注册 Hook。
    Run(point HookPoint, ctx *HookContext)

    // On 是便捷方法，自动填充 ChatCtx、SessionID、AgentID、
    // Logger 和 CurrentPoint，然后调用 Run。
    On(point HookPoint, chatCtx iface.ChatContextInterface, logger *slog.Logger, extra ...HookExtra)
}
```

引擎代码中使用 `On` 方法的典型模式：

```go
// engine.go 中
e.hookRunner.On(hook.BeforeContextBuild, chatCtx, e.logger,
    hook.HookExtra{SystemPrompt: &systemPrompt},
)
```

`HookExtra` 携带当前调用点的特定数据：

```go
type HookExtra struct {
    ToolName     *string
    ToolArgs     map[string]any
    ToolResult   *tool.ToolResult
    SystemPrompt *string
    Messages     *[]entity.MessageForLLM
}
```

---

上一篇：[02-agent-engine.md](./02-agent-engine.md)
下一篇：[04-llm-provider.md](./04-llm-provider.md)