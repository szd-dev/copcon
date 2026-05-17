# Todo 注入插件

`TodoInjectionHook` 是一个 Hook 插件，在系统提示词构建阶段将会话的 todo 任务列表注入到上下文窗口。Agent 在每轮对话中都能看到自己的待办事项，从而自主管理任务进度。

## 核心设计

**文件位置**: `server/internal/plugins/todo_hook.go`

```go
type TodoInjectionHook struct {
    todoMgr todo.TodoManager
    logger  *slog.Logger
}

func NewTodoInjectionHook(todoMgr todo.TodoManager) *TodoInjectionHook {
    return &TodoInjectionHook{
        todoMgr: todoMgr,
        logger:  slog.Default(),
    }
}
```

### Hook 元数据

| 属性 | 值 | 说明 |
|------|-----|------|
| `Name()` | `"todo_injection"` | 用于日志和调试的标识符 |
| `Points()` | `[OnSystemPrompt]` | 仅在系统提示词解析时执行 |
| `Priority()` | `50` | 较低优先级，在大多数提示词修改 hook 之前运行 |

## 执行流程

`TodoInjectionHook` 在 `OnSystemPrompt` hook 点触发时执行以下步骤：

### 1. 空指针保护

```go
if ctx.SystemPrompt == nil {
    return nil
}
```

如果当前请求没有系统提示词（例如纯 chat 场景），hook 直接跳过。

### 2. 获取 Todo 列表

```go
todos, err := h.todoMgr.List(ctx.ChatCtx)
```

通过 `TodoManager.List` 查询当前会话的所有 todo 项。查询按 `created_at DESC` 排序。

### 3. 错误处理

```go
if err != nil {
    h.logger.Warn("failed to fetch todos",
        "session_id", ctx.SessionID,
        "error", err,
    )
    return nil  // 返回 nil，管道继续
}
```

获取失败时记录 Warn 级别日志，返回 `nil` 不阻塞管道。Agent 将在没有 todo 上下文的情况下继续运行。

### 4. 注入提示词

```go
if len(todos) > 0 {
    todoState := formatTodoState(todos)
    *ctx.SystemPrompt = *ctx.SystemPrompt + "\n\n" + todoState
}
```

仅在有 todo 项时才追加。通过在现有提示词末尾追加 `"\n\n"` 分隔符后附加 todo 状态文本。

## formatTodoState 输出格式

**文件位置**: `server/internal/plugins/todo_format.go`

`formatTodoState` 将 todo 列表格式化为简洁的状态摘要文本。它按状态分组，优先使用 `ActiveForm` 字段（设置时替代原始 `Content`）。

```
Current todo list: [pending: 编写单元测试, 更新文档, in_progress: 实现用户认证, completed: 设计数据库模型, failed: 不必要的重构, blocked: 等待上游 API 修复]
```

### 分组规则

| Todo 状态 | 输出标签 | 说明 |
|-----------|---------|------|
| `pending` | `pending` | 待开始的任务 |
| `in_progress` | `in_progress` | 正在执行的任务 |
| `completed` | `completed` | 已完成的任务 |
| `failed` | `failed` | 执行失败的任务 |
| `blocked` | `blocked` | 被阻塞的任务 |

每个分组的任务用逗号分隔，分组之间用逗号 + 空格连接，整体包裹在 `Current todo list: [...]` 中。

### ActiveForm 优先

如果 Todo 的 `ActiveForm` 字段不为空，使用该字段替代 `Content`。`ActiveForm` 通常是对任务更精确的执行描述。

## 注册示例

在引擎初始化时将 hook 注册到 HookRunner：

```go
// 创建 TodoManager
todoMgr := todo.NewTodoManager(db)

// 创建 hook
todoHook := plugins.NewTodoInjectionHook(todoMgr)

// 创建 HookRunner 并注册
runner := hook.NewHookRunner()
runner.Register(todoHook)

// 将 runner 注入引擎
engine := agent.NewAgentEngine(
    agentRegistry,
    sessionMgr,
    contextMgr,
    asyncRegistry,
    agent.WithHookRunner(runner),
)
```

## 错误处理策略

`TodoInjectionHook` 采用"尽力而为"的错误处理策略：

- **SystemPrompt 为 nil**：跳过注入，返回 nil
- **TodoManager.List 失败**：记录 `Warn` 日志，返回 nil，不阻塞管道
- **Todo 列表为空**：不追加任何文本，保持原始 system prompt

所有错误场景都不会导致 Agent 请求失败。Hook 的设计原则是不应成为单点故障。