# 能力系统 (Capabilities)

CopCon 的核心设计是基于**能力 (Capabilities)** 的插件架构。能力分为两类:

- **Tools**: 可执行的操作 (如代码执行、文件操作)
- **Hooks**: 在特定时机触发的逻辑 (如日志记录、记忆检索)

所有能力通过 `init()` 自动注册,无需手动调用。

## 架构概览

```
core/capabilities/
├── registry.go          # 能力注册表 (全局单例)
├── tools/
│   ├── code_executor/   # 代码执行
│   ├── shell_executor/  # Shell 命令
│   ├── file_ops/        # 文件操作
│   ├── todo/            # 待办管理
│   ├── web_search/      # 网络搜索
│   ├── ask_user/        # 用户交互
│   ├── api_call/        # API 调用
│   └── database_query/  # 数据库查询
└── hooks/
    ├── logging/         # 日志记录
    ├── tracing/         # 链路追踪
    ├── memory/          # 记忆管理
    └── todo_injection/  # 待办注入
```

## 内置 Tools

### 1. Code Executor

执行多语言代码 (Python, JavaScript, Go, Ruby 等)。

```go
package code_executor

func init() {
    capabilities.RegisterTool(&CodeExecutor{})
}

type CodeExecutor struct{}

func (t *CodeExecutor) Name() string {
    return "code_executor"
}

func (t *CodeExecutor) Description() string {
    return "Execute code in multiple languages"
}

func (t *CodeExecutor) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    language := args["language"].(string)
    code := args["code"].(string)
    
    // 执行代码...
    return result, nil
}
```

**输入参数**:
```json
{
  "language": "python",
  "code": "print('Hello, World!')"
}
```

**输出**:
```json
{
  "stdout": "Hello, World!\n",
  "stderr": "",
  "exit_code": 0
}
```

### 2. Shell Executor

执行系统命令。

```json
{
  "command": "ls -la",
  "timeout": 30
}
```

### 3. File Operations

文件的读取、写入、列举、移动、删除。

```json
{
  "action": "read",
  "path": "/path/to/file.txt"
}
```

### 4. Web Search

网络搜索功能 (默认使用 Tavily API)。

```json
{
  "query": "latest AI news",
  "num_results": 5
}
```

### 5. Todo Management

任务创建、列出、更新、删除。

```json
{
  "action": "create",
  "content": "Write documentation",
  "priority": "high"
}
```

### 6. User Interaction

暂停执行,向用户请求输入。

```json
{
  "prompt": "请确认是否继续?",
  "options": ["yes", "no"]
}
```

### 7. API Call

调用 HTTP API。

```json
{
  "method": "GET",
  "url": "https://api.example.com/data",
  "headers": {"Authorization": "Bearer token"}
}
```

### 8. Database Query

执行数据库查询 (PostgreSQL, MySQL, SQLite)。

```json
{
  "database": "copcon_prod",
  "query": "SELECT * FROM users WHERE id = ?",
  "params": [123]
}
```

## 内置 Hooks

### 1. Logging Hook

记录每次对话的请求和响应。

```go
package logging

func init() {
    capabilities.RegisterHook(&LoggingHook{})
}

type LoggingHook struct{}

func (h *LoggingHook) Name() string {
    return "logging"
}

func (h *LoggingHook) OnRequest(ctx context.Context, req *Request) error {
    log.Printf("[Request] %s: %s", req.AgentName, req.Message)
    return nil
}

func (h *LoggingHook) OnResponse(ctx context.Context, resp *Response) error {
    log.Printf("[Response] %s: %s", resp.AgentName, resp.Content)
    return nil
}
```

**Hook Points**:
- `OnRequest`: 请求发送前
- `OnResponse`: 响应返回后
- `OnError`: 错误发生时
- `OnTimeout`: 超时时

### 2. Tracing Hook

链路追踪,记录 Tool 调用的执行时间。

```go
type TracingHook struct{}

func (h *TracingHook) OnToolCall(ctx context.Context, tool string, args map[string]interface{}) error {
    // 开始追踪 Tool 调用
    return nil
}

func (h *TracingHook) OnToolComplete(ctx context.Context, result interface{}, duration time.Duration) error {
    // 记录执行时间
    return nil
}
```

### 3. Memory Hook

自动将对话历史保存到长期记忆,并在需要时检索。

```go
type MemoryHook struct{}

func (h *MemoryHook) OnResponse(ctx context.Context, resp *Response) error {
    // 将响应保存到 Qdrant 向量数据库
    err := memory.SaveToVectorDB(ctx, resp.Content)
    return err
}

func (h *MemoryHook) OnRequest(ctx context.Context, req *Request) error {
    // 从长期记忆中检索相关上下文
    related, err := memory.SearchRelated(ctx, req.Message)
    if err == nil && len(related) > 0 {
        req.Context = append(req.Context, related...)
    }
    return nil
}
```

**配置**:
```go
harness := core.NewHarness(core.HarnessConfig{
    Hooks: []interface{}{&capabilities.MemoryHook{}},
    VectorDB: "qdrant://localhost:6333",
})
```

### 4. Todo Injection Hook

自动将当前 Agent 的待办事项注入到请求上下文中。

```go
type TodoInjectionHook struct{}

func (h *TodoInjectionHook) OnRequest(ctx context.Context, req *Request) error {
    // 获取当前 Agent 的待办
    todos, _ := db.QueryTodos(req.AgentName)
    
    // 注入到上下文
    if len(todos) > 0 {
        req.Context = append(req.Context, "Current todos:")
        for _, todo := range todos {
            req.Context = append(req.Context, fmt.Sprintf("- %s", todo.Content))
        }
    }
    
    return nil
}
```

## 自定义 Tool

### 实现自定义 Tool

1. 创建新包 `core/capabilities/tools/my_tool/`
2. 实现 Tool 接口
3. 在 `init()` 中注册

```go
package my_tool

import (
    "context"
    "github.com/copcon/core/capabilities"
)

type MyCustomTool struct{}

func init() {
    capabilities.RegisterTool(&MyCustomTool{
        Name: "my_custom_tool",
        Description: "Custom tool for specific purpose",
        Arguments: map[string]string{
            "param1": "Description of parameter 1",
            "param2": "Description of parameter 2",
        },
    })
}

func (t *MyCustomTool) Name() string {
    return "my_custom_tool"
}

func (t *MyCustomTool) Description() string {
    return "Custom tool for specific purpose"
}

func (t *MyCustomTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    // 实现你的逻辑
    return result, nil
}
```

### 使用自定义 Tool

```go
harness := core.NewHarness(core.HarnessConfig{
    Tools: []string{"my_custom_tool", "code_executor"},
})
```

## 自定义 Hook

### 实现自定义 Hook

```go
package my_hook

import (
    "context"
    "github.com/copcon/core/capabilities"
)

type MyCustomHook struct{}

func init() {
    capabilities.RegisterHook(&MyCustomHook{
        Name: "my_custom_hook",
    })
}

func (h *MyCustomHook) Name() string {
    return "my_custom_hook"
}

func (h *MyCustomHook) OnRequest(ctx context.Context, req *Request) error {
    // 请求前处理
    return nil
}

func (h *MyCustomHook) OnResponse(ctx context.Context, resp *Response) error {
    // 响应后处理
    return nil
}
```

### 使用自定义 Hook

```go
harness := core.NewHarness(core.HarnessConfig{
    Hooks: []interface{}{&my_hook.MyCustomHook{}},
})
```

## Hook 执行顺序

多个 Hook 按注册顺序触发:

```go
harness := core.NewHarness(core.HarnessConfig{
    Hooks: []interface{}{
        &logging.LoggingHook{},      // 第 1 个
        &tracing.TracingHook{},      // 第 2 个
        &memory.MemoryHook{},        // 第 3 个
    },
})
```

**执行流程**:

```
Request 进入
    ↓
Logging.OnRequest()
    ↓
Tracing.OnRequest()
    ↓
Memory.OnRequest()
    ↓
[Tool 执行]
    ↓
Memory.OnResponse()
    ↓
Tracing.OnResponse()
    ↓
Logging.OnResponse()
    ↓
Response 返回
```

## Tool 参数验证

```go
type ValidatedTool struct{}

func (t *ValidatedTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    // 验证必需参数
    if _, ok := args["required_field"]; !ok {
        return nil, fmt.Errorf("required_field is missing")
    }
    
    // 类型检查
    num, ok := args["number"].(float64)
    if !ok {
        return nil, fmt.Errorf("number must be a numeric value")
    }
    
    // 范围检查
    if num < 0 || num > 100 {
        return nil, fmt.Errorf("number must be between 0 and 100")
    }
    
    return t.process(ctx, args)
}
```

## 错误处理

### Tool 错误

```go
type MyTool struct{}

func (t *MyTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    if err := validate(args); err != nil {
        return nil, fmt.Errorf("invalid arguments: %w", err)
    }
    
    result, err := t.process(args)
    if err != nil {
        // 返回详细错误信息
        return nil, fmt.Errorf("processing failed: %w", err)
    }
    
    return result, nil
}
```

### Hook 错误

```go
type MyHook struct{}

func (h *MyHook) OnRequest(ctx context.Context, req *Request) error {
    if err := h.validate(req); err != nil {
        // 返回错误会中断后续 Hook 执行
        return fmt.Errorf("validation failed: %w", err)
    }
    
    return nil
}
```

## 能力配置示例

### 完整配置

```go
harness := core.NewHarness(core.HarnessConfig{
    // 启用的 Tools (使用 init 注册的所有 Tool)
    Tools: []string{
        "code_executor",
        "file_ops",
        "web_search",
        "todo",
    },
    
    // 启用的 Hooks (按顺序执行)
    Hooks: []interface{}{
        &logging.LoggingHook{},
        &tracing.TracingHook{},
        &memory.MemoryHook{},
        &todo_injection.TodoInjectionHook{},
    },
    
    // 向量数据库 (用于 Memory Hook)
    VectorDB: "qdrant://localhost:6333",
    
    // 长期记忆配置
    Memory: core.MemoryConfig{
        Enabled: true,
        TopK: 5,  // 检索前 5 条最相关的记忆
        MinScore: 0.7,  // 最小相似度
    },
})
```

### 最小配置

```go
harness := core.NewHarness(core.HarnessConfig{
    Tools: []string{"code_executor"},
    Hooks: []interface{}{},
})
```

## 调试能力

### 启用调试日志

```go
log.SetLevel(log.DebugLevel)  // 全局调试级别
```

### 追踪能力执行

```go
tracer := tracing.NewTracer("stdout")
defer tracer.Close()

harness := core.NewHarness(core.HarnessConfig{
    Tracer: tracer,
    Hooks: []interface{}{
        &tracing.TracingHook{},  // 自动记录追踪
    },
})
```

### 性能监控

```go
import (
    "github.com/prometheus/client_golang/prometheus"
)

// 注册 Prometheus 指标
harness := core.NewHarness(core.HarnessConfig{
    Metrics: &prometheus.Registry{},
    Hooks: []interface{}{
        &metrics.MetricsHook{},  // 自动暴露指标
    },
})
```

## 最佳实践

1. **单一职责**: 每个 Tool 只负责一项功能
2. **参数验证**: 在 Tool 入口处验证所有输入
3. **错误处理**: 明确返回错误,不要默默失败
4. **Hook 顺序**: 注意注册顺序,因为 Hook 按注册顺序执行
5. **性能**: Tool 和 Hook 应该是轻量级的,避免阻塞
6. **文档**: 每个 Tool 都应该有清晰的描述,帮助 LLM 理解如何使用

## 下一步

- [内置 Tool 详细文档](./built-in-tools.md)
- [内置 Hook 详细文档](./built-in-hooks.md)
- [自定义 Tool 完整指南](../06-extending/custom-tool.md)
