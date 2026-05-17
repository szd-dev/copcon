# Hook 注册与运行

`HookRunner` 是 Hook 系统的调度中心。它负责存储已注册的 Hook，并在引擎触达某个 HookPoint 时按顺序执行它们。

## HookRunner 接口

```go
// 定义在 server/internal/hook/runner.go
type HookRunner interface {
    // Register 注册一个 Hook。并发安全。
    Register(hook Hook)

    // Run 在指定 HookPoint 上，按优先级降序执行所有匹配的 Hook。
    // 如果 ctx.ChatCtx 的 context 已取消，直接返回不执行。
    // 每个 Hook 都有 panic 恢复和 error 日志保护。
    Run(point HookPoint, ctx *HookContext)

    // On 是 Run 的便利方法。自动从 chatCtx 和 logger 填充
    // HookContext 的基础字段，再叠加可选的 HookExtra 字段。
    On(point HookPoint, chatCtx iface.ChatContextInterface, logger *slog.Logger, extra ...HookExtra)
}
```

### Register — 注册 Hook

把 Hook 添加到 Runner 内部列表中。注册时记录当前时间戳，用于同优先级时的排序。

```go
runner.Register(myHook)
runner.Register(anotherHook)
```

### Run — 手动触发

如果你需要直接在某个点上触发 Hook 链：

```go
ctx := &hook.HookContext{
    ChatCtx:      chatCtx,
    SessionID:    chatCtx.SessionID(),
    AgentID:      chatCtx.AgentID(),
    Logger:       logger,
    CurrentPoint: hook.AfterContextBuild,
    Messages:     &messages,
}
runner.Run(hook.AfterContextBuild, ctx)
```

### On — 引擎内部使用的便利方法

`On` 省去了手动构造 `HookContext` 的麻烦：

```go
// 引擎中的典型调用
runner.On(
    hook.AfterContextBuild,
    chatCtx,
    logger,
    hook.HookExtra{Messages: &messages},
)
```

引擎内部统一使用 `On`，因为 `On` 会自动填充 `SessionID`、`AgentID` 等基础字段，保证一致性。你的日常代码中不需要调用 `On` 或 `Run`，这两个方法是引擎在调用你的 Hook。

---

## 注册顺序与排序规则

当你注册多个 Hook 并触发某个 HookPoint 时，执行顺序遵循两条规则：

### 规则一：优先级降序（Priority 大的先执行）

```go
type HookA struct{}
func (h *HookA) Priority() int { return 200 }

type HookB struct{}
func (h *HookB) Priority() int { return 100 }

type HookC struct{}
func (h *HookC) Priority() int { return 50 }

runner.Register(a)  // Priority=200
runner.Register(b)  // Priority=100
runner.Register(c)  // Priority=50

// Run 时的执行顺序: A → B → C
```

### 规则二：同优先级时，注册时间升序（先注册的先执行）

```go
type HookX struct{}
func (h *HookX) Priority() int { return 100 }

type HookY struct{}
func (h *HookY) Priority() int { return 100 }

runner.Register(x)  // 注册时间 t1
runner.Register(y)  // 注册时间 t2

// 同 Priority=100，执行顺序: X → Y
```

### 排序源码

排序发生在 `Run` 方法内部：

```go
sort.Slice(matched, func(i, j int) bool {
    pi, pj := matched[i].hook.Priority(), matched[j].hook.Priority()
    if pi != pj {
        return pi > pj  // 优先级降序
    }
    return matched[i].createdAt.Before(matched[j].createdAt)  // 注册时间升序
})
```

### 为什么是降序？

大部分系统的"优先级"数字越大表示越优先。降序排列让数字大的 Hook 排在前面，符合直觉。

---

## 线程安全

HookRunner 内部使用 `sync.Mutex` 保证并发安全。

```go
type hookRunner struct {
    mu      sync.Mutex
    entries []hookEntry
}

func (r *hookRunner) Register(hook Hook) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.entries = append(r.entries, hookEntry{
        hook:      hook,
        createdAt: time.Now(),
    })
}
```

`Run` 在执行前会通过 `sync.Mutex` 快照（snapshot）当前的 Hook 列表，然后在锁外执行。这意味着：

- 你可以在运行时注册新的 Hook，不会影响正在执行的 Hook 链
- 正在执行 Hook 时注册的新 Hook 要在下一次 `Run` 调用时才会生效

```go
func (r *hookRunner) Run(point HookPoint, ctx *HookContext) {
    // ... 取消检查 ...

    r.mu.Lock()
    candidates := make([]hookEntry, len(r.entries))
    copy(candidates, r.entries)
    r.mu.Unlock()

    // 在锁外执行，不阻塞后续注册
    // ...
}
```

---

## NewEmptyRunner — 无 Hook 场景

当引擎不加载任何 Hook 时，使用 `NewEmptyRunner()` 创建一个空 Runner：

```go
runner := hook.NewEmptyRunner()
// 等价于
runner := hook.NewHookRunner()
```

这两个函数返回完全相同的东西。`NewEmptyRunner` 只是在语义上更清晰地表达了"当前没有 Hook"的意图。

空 Runner 的 `Run` 调用完全无开销：没有匹配的 Hook，直接返回。

---

## 完整注册示例

以下示例展示了一个典型的初始化流程：创建 Runner，注册多个 Hook，然后传给引擎。

```go
package main

import (
    "log/slog"

    "github.com/copcon/server/internal/hook"
    "github.com/copcon/server/internal/plugins"
    "github.com/copcon/server/internal/plugins/memory"
    "github.com/copcon/server/internal/tools/todo"
)

func setupHooks(todoMgr todo.TodoManager, memoryMgr memory.Manager) hook.HookRunner {
    runner := hook.NewHookRunner()

    // 1. 注册 Todo 注入 Hook（Priority=50）
    todoHook := plugins.NewTodoInjectionHook(todoMgr)
    runner.Register(todoHook)

    // 2. 注册 Memory 插件（Priority=100）
    memoryHook := memory.NewMemoryPlugin(memoryMgr)
    runner.Register(memoryHook)

    // 3. 注册自定义日志 Hook（Priority=200）
    loggerHook := NewCustomLogger(slog.Default())
    runner.Register(loggerHook)

    return runner
}

func main() {
    runner := setupHooks(todoMgr, memoryMgr)

    // 将 runner 传入引擎
    engine := agent.NewEngine(
        agent.WithHookRunner(runner),
        // ... 其他配置
    )
}
```

### CustomLogger 示例

上面用到的 `CustomLogger` 是一个简单的示例 Hook：

```go
package main

import (
    "log/slog"
    "time"

    "github.com/copcon/server/internal/hook"
)

type CustomLogger struct {
    startTime time.Time
    logger    *slog.Logger
}

func NewCustomLogger(logger *slog.Logger) *CustomLogger {
    return &CustomLogger{logger: logger}
}

func (l *CustomLogger) Name() string {
    return "custom_logger"
}

func (l *CustomLogger) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.BeforeLLMCall, hook.AfterLLMCall}
}

func (l *CustomLogger) Priority() int {
    return 200
}

func (l *CustomLogger) Execute(ctx *hook.HookContext) error {
    switch ctx.CurrentPoint {
    case hook.BeforeLLMCall:
        l.startTime = time.Now()
        l.logger.Info("llm call starting",
            "session_id", ctx.SessionID,
            "agent_id", ctx.AgentID,
        )

    case hook.AfterLLMCall:
        elapsed := time.Since(l.startTime)
        msgCount := 0
        if ctx.Messages != nil {
            msgCount = len(*ctx.Messages)
        }
        l.logger.Info("llm call completed",
            "session_id", ctx.SessionID,
            "latency_ms", elapsed.Milliseconds(),
            "message_count", msgCount,
        )
    }

    return nil
}
```

---

## 调试技巧

### 看日志确认 Hook 是否被执行

每个 Hook 执行时都会带上名字和 HookPoint：

```text
INFO hook execution started hook=custom_logger point=before_llm_call
```

如果 Hook 返回了 error：

```text
WARN hook returned error hook=sensitive_word_filter error="something went wrong" point=after_context_build
```

如果 Hook panic 了：

```text
ERROR hook panicked hook=custom_logger panic="runtime error: invalid memory address" point=before_llm_call
```

### 验证优先级排序

按 `Priority()` 从大到小注册，看日志输出顺序是否符合预期。

### 检查 HookContext 字段

在 `Execute` 开头打印所有字段：

```go
func (h *MyHook) Execute(ctx *hook.HookContext) error {
    h.logger.Debug("hook context dump",
        "session_id", ctx.SessionID,
        "agent_id", ctx.AgentID,
        "current_point", ctx.CurrentPoint,
        "tool_name", ctx.ToolName,
        "has_tool_args", ctx.ToolArgs != nil,
        "has_tool_result", ctx.ToolResult != nil,
        "has_system_prompt", ctx.SystemPrompt != nil,
        "has_messages", ctx.Messages != nil,
    )
    // ...
}
```