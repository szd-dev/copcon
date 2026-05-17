# EngineOption 参考

`EngineOption` 是 CopCon Agent 引擎的函数式选项模式配置入口。通过组合不同的 `EngineOption`，可以在不修改引擎内部代码的情况下定制其行为。

**文件位置**: `server/internal/agent/engine.go`

## 函数式选项模式

`EngineOption` 是一个函数类型：

```go
type EngineOption func(*engineImpl)
```

每个选项函数接收引擎内部实现的指针，直接修改其字段。使用方式：

```go
engine := agent.NewAgentEngine(
    agentRegistry,
    sessionMgr,
    contextMgr,
    asyncRegistry,
    agent.WithConcurrency(10),
    agent.WithLogger(customLogger),
    agent.WithHookRunner(hookRunner),
)
```

`NewAgentEngine` 先初始化默认值，然后依次应用每个传入的 Option：

```go
e := &engineImpl{
    concurrency:   5,
    logger:        slog.New(slog.NewTextHandler(os.Stderr, nil)),
    hookRunner:    hook.NewEmptyRunner(),
    // ...
}
for _, opt := range opts {
    opt(e)
}
```

## 所有 Option

### WithHookRunner

```go
func WithHookRunner(runner hook.HookRunner) EngineOption
```

注入 HookRunner，控制所有生命周期 hook 的执行和排序。

| 默认值 | 说明 |
|--------|------|
| `hook.NewEmptyRunner()` | 空的 HookRunner，不注册任何 hook，所有 `On()` 调用无操作 |

当不传 `WithHookRunner` 时，引擎使用 `NewEmptyRunner()`，Hook 系统仍然可用但不会有任何钩子执行。这是使用 Hook 系统的前提条件。

### WithLLMProvider

```go
func WithLLMProvider(p llm.LLMProvider) EngineOption
```

设置引擎级别的 LLM Provider，覆盖 Agent 定义中的 Provider。当为 nil（默认）时，引擎使用 `AgentDefinition.LLMProvider`。

| 默认值 | 说明 |
|--------|------|
| `nil` | 使用 Agent 定义中的 Provider |

使用场景：在多 Agent 场景下统一使用同一个 LLM 后端，而非每个 Agent 各自创建 client。

### WithConcurrency

```go
func WithConcurrency(n int) EngineOption
```

设置工具执行的并发上限。`n` 必须大于 0，否则触发 panic。

| 默认值 | 说明 |
|--------|------|
| `5` | 最多同时执行 5 个工具 |

内部使用 `golang.org/x/sync/semaphore.Weighted` 实现并发控制。当 LLM 返回多个工具调用时，引擎同时启动最多 `n` 个工具的 goroutine 并行执行。

### WithLogger

```go
func WithLogger(logger *slog.Logger) EngineOption
```

设置结构化日志记录器，控制引擎及其所有 hook 的日志输出格式和目的地。

| 默认值 | 说明 |
|--------|------|
| `slog.New(slog.NewTextHandler(os.Stderr, nil))` | 文本格式，输出到 stderr |

传入的 logger 会被传递给 `HookContext.Logger`，因此所有通过 HookRunner 执行的 hook 都能使用同一个 logger。日志插件（`LoggingPlugin`、`TracingPlugin`）的输出都通过此 logger 记录。

## 组合多个 Option

Options 按传入顺序依次应用，后面的 Option 可能覆盖前面的：

```go
engine := agent.NewAgentEngine(
    agentRegistry,
    sessionMgr,
    contextMgr,
    asyncRegistry,
    // 1. 设置并发上限
    agent.WithConcurrency(10),
    // 2. 设置 JSON 格式日志
    agent.WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil))),
    // 3. 注册 hook runner（所有插件需先注册到 runner）
    agent.WithHookRunner(hookRunner),
)
```

推荐的使用顺序：
1. `WithConcurrency`：设置性能参数
2. `WithLogger`：设置日志配置
3. `WithHookRunner`：注入 Hook 系统（通常需要 logger 和 concurrency 已设置）
4. `WithLLMProvider`：覆盖 LLM 后端（如需要）

## 完整的引擎初始化示例

```go
package main

import (
    "log/slog"
    "os"

    "github.com/copcon/server/internal/agent"
    "github.com/copcon/server/internal/chat_context"
    "github.com/copcon/server/internal/config"
    "github.com/copcon/server/internal/hook"
    "github.com/copcon/server/internal/plugins"
    "github.com/copcon/server/internal/plugins/logging"
    "github.com/copcon/server/internal/plugins/tracing"
    "github.com/copcon/server/internal/session"
    "github.com/copcon/server/internal/tool"
    "github.com/copcon/server/internal/tools/todo"
)

func main() {
    cfg, _ := config.Load()
    db := initDB(cfg)

    // 基础设施
    toolRegistry := tool.NewToolRegistry()
    agentRegistry, _ := agent.NewAgentRegistry(cfg, toolRegistry)
    sessionMgr := session.NewSessionManager(db)
    contextMgr := chat_context.NewContextManager(db)
    asyncRegistry := tool.NewAsyncToolRegistry()

    // 创建 Todo Hook
    todoMgr := todo.NewTodoManager(db)
    todoHook := plugins.NewTodoInjectionHook(todoMgr)

    // 创建 Logging Plugin
    loggingPlugin := logging.NewLoggingPlugin()

    // 创建 Tracing Plugin
    tracingPlugin := tracing.NewTracingPlugin(nil) // 开发环境关闭

    // 注册 hooks
    runner := hook.NewHookRunner()
    runner.Register(todoHook)
    runner.Register(loggingPlugin)
    runner.Register(tracingPlugin)

    // 自定义 logger
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    // 创建引擎
    engine := agent.NewAgentEngine(
        agentRegistry,
        sessionMgr,
        contextMgr,
        asyncRegistry,
        agent.WithConcurrency(8),
        agent.WithLogger(logger),
        agent.WithHookRunner(runner),
    )

    // engine 就绪
}
```

## engineImpl 内部字段

完整的引擎内部字段（通过 Option 可配置的）：

```go
type engineImpl struct {
    logger         *slog.Logger               // WithLogger
    agentRegistry  AgentRegistry              // 构造函数参数
    sessionMgr     session.SessionManager     // 构造函数参数
    contextMgr     chat_context.ContextManager // 构造函数参数
    llmProvider    llm.LLMProvider            // WithLLMProvider
    concurrency    int                        // WithConcurrency
    concurrencySem *semaphore.Weighted        // 内部派生
    asyncRegistry  *tool.AsyncToolRegistry    // 构造函数参数
    hookRunner     hook.HookRunner            // WithHookRunner
}
```

`concurrencySem` 在 `NewAgentEngine` 中从 `concurrency` 字段自动派生，无需单独设置。`asyncRegistry`、`agentRegistry`、`sessionMgr`、`contextMgr` 为必需构造函数参数，不是 Option。