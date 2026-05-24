# 自定义 Hook

Hook 是在 Agent 生命周期特定阶段执行的拦截逻辑。与 Tool 不同，Hook 不是由 LLM 主动调用的，而是由引擎在关键节点自动触发。Hook 可以观察、修改甚至阻断请求流。

## Hook 接口

每个 Hook 必须实现 `hook.Hook` 接口：

```go
// core/hook/hook.go
type Hook interface {
    Name() string
    Points() []HookPoint
    Priority() int
    Execute(ctx *HookContext) error
}
```

### 方法说明

| 方法 | 返回值 | 说明 |
|------|--------|------|
| `Name()` | `string` | Hook 的标识符，用于日志和调试 |
| `Points()` | `[]HookPoint` | 声明此 Hook 关注哪些生命周期节点 |
| `Priority()` | `int` | 执行优先级，数值越大越先执行。默认 100 |
| `Execute()` | `error` | Hook 的执行逻辑。返回 error 不会中断管线，但会被记录 |

## 生命周期节点

CopCon 定义了以下 HookPoint，覆盖 Agent 引擎的完整请求处理流程：

| HookPoint | 触发时机 | 可修改字段 |
|-----------|---------|-----------|
| `BeforeContextBuild` | 组装上下文窗口之前 | `SystemPrompt` |
| `AfterContextBuild` | 上下文窗口组装完成，发送给 LLM 之前 | `Messages` |
| `OnSystemPrompt` | 系统提示词解析时 | `SystemPrompt` |
| `OnMessagePersist` | 消息持久化之前 | `Messages` |
| `BeforeToolExecute` | Tool 执行之前 | `ToolArgs` |
| `AfterToolExecute` | Tool 执行成功之后 | `ToolResult` |
| `OnToolError` | Tool 执行失败时 | `ToolResult`（可设置回退结果） |
| `BeforeLLMCall` | 发送 LLM API 请求之前 | `Messages` |
| `AfterLLMCall` | 收到 LLM API 响应之后 | `Messages` |
| `OnSessionResolve` | Session ID 解析时 | 无 |

## HookContext 结构

`HookContext` 携带了当前请求的所有上下文信息。不同 HookPoint 下，可用字段不同。

```go
type HookContext struct {
    ChatCtx      iface.ChatContextInterface  // 始终可用
    SessionID    string                       // 始终可用
    AgentID      string                       // 始终可用
    Logger       *slog.Logger                 // 始终可用
    CurrentPoint HookPoint                    // 始终可用

    SystemPrompt *string                      // OnSystemPrompt, BeforeContextBuild
    Messages     *[]entity.MessageForLLM      // AfterContextBuild, BeforeLLMCall, AfterLLMCall
    ToolName     string                       // BeforeToolExecute, AfterToolExecute, OnToolError
    ToolArgs     map[string]any               // BeforeToolExecute, AfterToolExecute, OnToolError
    ToolResult   *tool.ToolResult             // AfterToolExecute, OnToolError
}
```

指针字段（`*string`、`*[]MessageForLLM`、`*tool.ToolResult`）表示 Hook 可以修改的值。设置指针字段即可影响下游行为。

## 执行顺序

多个 Hook 在同一 HookPoint 上按 `Priority()` 降序执行（数值越大越先执行）。相同优先级时，按注册顺序执行。

```
Hook A (Priority: 300)  →  最先执行
Hook B (Priority: 200)  →  其次执行
Hook C (Priority: 100)  →  最后执行
```

内置 Hook 的优先级参考：
- `todo_injection`: 50（最晚执行，确保系统提示词已经定型）
- `logging`: 200
- `tracing`: 200

## 示例一：审计日志 Hook

记录所有 Tool 调用的详细信息，用于合规审计。

```go
package audit

import (
    "encoding/json"
    "log/slog"
    "os"

    "github.com/copcon/core/capabilities"
    "github.com/copcon/core/hook"
    "github.com/copcon/core/tool"
)

type AuditHook struct {
    logger *slog.Logger
}

func NewAuditHook() *AuditHook {
    file, _ := os.OpenFile("audit.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    return &AuditHook{
        logger: slog.New(slog.NewJSONHandler(file, nil)),
    }
}

func (h *AuditHook) Name() string { return "audit" }

func (h *AuditHook) Points() []hook.HookPoint {
    return []hook.HookPoint{
        hook.BeforeToolExecute,
        hook.AfterToolExecute,
        hook.OnToolError,
    }
}

func (h *AuditHook) Priority() int { return 300 } // 最先执行，确保记录所有操作

func (h *AuditHook) Execute(ctx *hook.HookContext) error {
    switch ctx.CurrentPoint {
    case hook.BeforeToolExecute:
        args, _ := json.Marshal(ctx.ToolArgs)
        h.logger.Info("tool_call_start",
            "session_id", ctx.SessionID,
            "agent_id", ctx.AgentID,
            "tool", ctx.ToolName,
            "args", string(args),
        )
    case hook.AfterToolExecute:
        h.logger.Info("tool_call_end",
            "session_id", ctx.SessionID,
            "agent_id", ctx.AgentID,
            "tool", ctx.ToolName,
            "success", ctx.ToolResult.Success,
        )
    case hook.OnToolError:
        errMsg := ""
        if ctx.ToolResult != nil {
            errMsg = ctx.ToolResult.Error
        }
        h.logger.Error("tool_call_error",
            "session_id", ctx.SessionID,
            "agent_id", ctx.AgentID,
            "tool", ctx.ToolName,
            "error", errMsg,
        )
    }
    return nil
}
```

## 示例二：速率限制 Hook

在 LLM 调用前检查速率限制，防止过载。

```go
package ratelimit

import (
    "fmt"
    "sync"
    "time"

    "github.com/copcon/core/capabilities"
    "github.com/copcon/core/hook"
)

type RateLimiter struct {
    mu       sync.Mutex
    requests map[string][]time.Time // sessionID → timestamps
    limit    int                    // 每分钟最大请求数
    window   time.Duration
}

type RateLimitHook struct {
    limiter *RateLimiter
}

func NewRateLimitHook(limit int, window time.Duration) *RateLimitHook {
    return &RateLimitHook{
        limiter: &RateLimiter{
            requests: make(map[string][]time.Time),
            limit:    limit,
            window:   window,
        },
    }
}

func (h *RateLimitHook) Name() string     { return "rate_limit" }
func (h *RateLimitHook) Priority() int    { return 500 } // 高优先级，在其他 Hook 之前执行

func (h *RateLimitHook) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.BeforeLLMCall}
}

func (h *RateLimitHook) Execute(ctx *hook.HookContext) error {
    if !h.limiter.Allow(ctx.SessionID) {
        // 返回 error 会被记录但不会中断管线。
        // 要真正阻断请求，需要修改 Messages 为空或设置标志。
        // 当前框架设计中，Hook error 仅做日志记录。
        ctx.Logger.Warn("rate limit exceeded",
            "session_id", ctx.SessionID,
            "limit", h.limiter.limit,
            "window", h.limiter.window,
        )
        return fmt.Errorf("rate limit exceeded: session %s", ctx.SessionID)
    }
    return nil
}

func (r *RateLimiter) Allow(sessionID string) bool {
    r.mu.Lock()
    defer r.mu.Unlock()

    now := time.Now()
    cutoff := now.Add(-r.window)

    // 清理过期记录
    timestamps := r.requests[sessionID]
    valid := timestamps[:0]
    for _, ts := range timestamps {
        if ts.After(cutoff) {
            valid = append(valid, ts)
        }
    }

    if len(valid) >= r.limit {
        r.requests[sessionID] = valid
        return false
    }

    r.requests[sessionID] = append(valid, now)
    return true
}
```

## 示例三：数据脱敏 Hook

在消息持久化之前，自动脱敏敏感信息。

```go
package masking

import (
    "regexp"
    "strings"

    "github.com/copcon/core/capabilities"
    "github.com/copcon/core/hook"
)

type DataMaskingHook struct {
    patterns []*regexp.Regexp
}

func NewDataMaskingHook() *DataMaskingHook {
    return &DataMaskingHook{
        patterns: []*regexp.Regexp{
            regexp.MustCompile(`\b\d{16,19}\b`),         // 银行卡号
            regexp.MustCompile(`\b\d{6}\d{12}\b`),       // 身份证号
            regexp.MustCompile(`1[3-9]\d{9}`),           // 手机号
            regexp.MustCompile(`(?i)password[=:]\s*\S+`), // 密码
        },
    }
}

func (h *DataMaskingHook) Name() string  { return "data_masking" }
func (h *DataMaskingHook) Priority() int { return 100 }

func (h *DataMaskingHook) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.OnMessagePersist}
}

func (h *DataMaskingHook) Execute(ctx *hook.HookContext) error {
    if ctx.Messages == nil {
        return nil
    }

    for i, msg := range *ctx.Messages {
        (*ctx.Messages)[i].Content = h.mask(msg.Content)
    }
    return nil
}

func (h *DataMaskingHook) mask(text string) string {
    result := text
    for _, p := range h.patterns {
        result = p.ReplaceAllStringFunc(result, func(match string) string {
            if len(match) <= 4 {
                return strings.Repeat("*", len(match))
            }
            return match[:2] + strings.Repeat("*", len(match)-4) + match[len(match)-2:]
        })
    }
    return result
}
```

## 示例四：系统提示词增强 Hook

动态追加上下文信息到系统提示词。

```go
package prompt_enhance

import (
    "fmt"
    "os"
    "time"

    "github.com/copcon/core/capabilities"
    "github.com/copcon/core/hook"
)

type PromptEnhanceHook struct {
    extraInfo string
}

func NewPromptEnhanceHook() *PromptEnhanceHook {
    hostname, _ := os.Hostname()
    return &PromptEnhanceHook{
        extraInfo: fmt.Sprintf(
            "\n\nEnvironment: host=%s, time=%s",
            hostname,
            time.Now().Format("2006-01-02"),
        ),
    }
}

func (h *PromptEnhanceHook) Name() string  { return "prompt_enhance" }
func (h *PromptEnhanceHook) Priority() int { return 90 } // 低优先级，在其他 Hook 修改之后追加

func (h *PromptEnhanceHook) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.OnSystemPrompt}
}

func (h *PromptEnhanceHook) Execute(ctx *hook.HookContext) error {
    if ctx.SystemPrompt == nil {
        return nil
    }
    *ctx.SystemPrompt += h.extraInfo
    return nil
}
```

## 注册自定义 Hook

### 方式一：Capability 自动注册（推荐）

```go
package audit

import (
    "github.com/copcon/core/capabilities"
    "github.com/copcon/core/hook"
)

func init() {
    capabilities.Register(&auditHookCapability{})
}

type auditHookCapability struct{}

func (c *auditHookCapability) Name() string                      { return "hooks.audit" }
func (c *auditHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *auditHookCapability) DependsOn() []string               { return nil }
func (c *auditHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
    return NewAuditHook(), nil
}
```

命名约定：`hooks.<hook_name>`。

### 方式二：直接注册到 HookRunner

```go
runner := hook.NewHookRunner()
runner.Register(&RateLimitHook{limit: 60, window: time.Minute})
runner.Register(&DataMaskingHook{})
```

### Capability 依赖声明

如果 Hook 依赖存储层，通过 `DependsOn()` 和 `CapabilityDeps` 获取：

```go
func (c *todoInjectionCapability) DependsOn() []string { return nil }

func (c *todoInjectionCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
    // deps.TodoStore 已由框架注入
    return NewTodoInjectionHook(deps.TodoStore), nil
}
```

## 访问对话和工具上下文

`HookContext` 提供了不同粒度的上下文访问：

### 通过 ChatCtx

```go
func (h *MyHook) Execute(ctx *hook.HookContext) error {
    // 获取标准上下文（用于超时和取消）
    stdCtx := ctx.ChatCtx.Context()

    // 获取会话 ID
    sessionID := ctx.ChatCtx.SessionID()

    // 获取 Agent ID
    agentID := ctx.ChatCtx.AgentID()

    // 请求用户输入（HITL）
    resp, err := ctx.ChatCtx.RequestInput(iface.InputRequest{
        Type:    iface.InterruptApproval,
        Message: "确认执行此操作？",
    })

    return nil
}
```

### 通过 Messages 指针

在 `AfterContextBuild` 或 `BeforeLLMCall` 阶段，可以读取和修改发送给 LLM 的消息列表：

```go
func (h *MyHook) Execute(ctx *hook.HookContext) error {
    if ctx.Messages == nil {
        return nil
    }

    // 在消息列表前面插入一条系统消息
    *ctx.Messages = append([]entity.MessageForLLM{
        {Role: "system", Content: "Additional context here"},
    }, *ctx.Messages...)

    return nil
}
```

### 通过 ToolResult 指针

在 `AfterToolExecute` 阶段，可以修改工具返回的结果：

```go
func (h *MyHook) Execute(ctx *hook.HookContext) error {
    if ctx.ToolResult != nil && ctx.ToolResult.Success {
        // 给成功结果追加元数据
        if data, ok := ctx.ToolResult.Data.(map[string]any); ok {
            data["hook_processed"] = true
        }
    }
    return nil
}
```

在 `OnToolError` 阶段，可以设置回退结果：

```go
func (h *FallbackHook) Execute(ctx *hook.HookContext) error {
    if ctx.CurrentPoint == hook.OnToolError && ctx.ToolResult != nil {
        // 提供回退结果
        ctx.ToolResult.Success = true
        ctx.ToolResult.Data = map[string]any{"fallback": "cached value"}
        ctx.ToolResult.Error = ""
    }
    return nil
}
```

## 错误处理

Hook 的错误处理与 Tool 不同：

- Hook 返回 `error` **不会中断管线**。错误会被记录到日志，然后继续执行后续 Hook
- 如果 Hook 发生 panic，框架会恢复并记录日志，不会崩溃
- 要真正阻断请求流，Hook 需要通过修改上下文数据来实现（例如清空 `Messages`）

```go
func (h *MyHook) Execute(ctx *hook.HookContext) error {
    // 这个 error 只会被记录，不会停止管线
    if err := h.doSomething(); err != nil {
        return fmt.Errorf("my_hook failed: %w", err)
    }
    return nil
}
```

## 性能考虑

1. **避免阻塞操作**。Hook 在请求的关键路径上执行。如果需要做耗时操作（如写文件、网络请求），考虑用 goroutine 异步处理

```go
func (h *MyHook) Execute(ctx *hook.HookContext) error {
    // 异步写入审计日志，不阻塞请求
    go func() {
        if err := writeAuditLog(ctx); err != nil {
            ctx.Logger.Error("audit log write failed", "error", err)
        }
    }()
    return nil
}
```

2. **并发安全**。如果 Hook 有可变状态（如速率限制器的计数器），必须用 `sync.Mutex` 或 `sync.Map` 保护

3. **优先级设计**。把"阻断类" Hook 设高优先级（如速率限制），把"观察类" Hook 设低优先级（如审计日志），这样阻断判断先于日志记录

4. **空检查**。始终检查指针字段是否为 `nil`。`Messages`、`SystemPrompt`、`ToolResult` 在某些 HookPoint 下不会填充

5. **上下文取消**。如果 Hook 做耗时工作，检查 `ctx.ChatCtx.Context().Err()`，在请求被取消时及时退出

## 最佳实践

1. **单一职责**。一个 Hook 只做一件事。需要多个功能时，注册多个 Hook
2. **幂等执行**。Hook 可能因为重试被多次执行，确保逻辑幂等
3. **不要依赖执行顺序**。虽然优先级提供了确定性排序，但逻辑上应尽量独立
4. **结构化日志**。使用 `ctx.Logger` 而非全局 logger，确保日志带上 session 和 agent 上下文
5. **合理的优先级**。100 是默认值，小于 100 的 Hook 在大多数内置 Hook 之后执行

## 下一步

- [自定义 Tool](custom-tool.md) - 编写 Agent 可调用的自定义工具
- [自定义 Provider](custom-provider.md) - 实现自定义存储后端
- [内置 Hook 详解](../05-built-in-capabilities/hooks/overview.md) - 了解内置 Hook 的实现
