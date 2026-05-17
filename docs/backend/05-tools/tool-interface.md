# Tool 接口规范

CopCon 的工具系统以 `Tool` 接口为核心，所有 Agent 可调用的工具都实现该接口。工具定义、执行和 LLM 函数调用定义三者各司其职。

## Tool 接口

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*ToolResult, error)
}
```

四个方法各司其职：

| 方法 | 用途 | 何时调用 |
|------|------|----------|
| `Name()` | 返回工具的唯一名称 | 注册时、LLM 工具定义生成时 |
| `Description()` | 返回工具的功能描述 | 发给 LLM 的函数说明 |
| `InputSchema()` | 返回 JSON Schema 参数定义 | 发给 LLM 的参数格式 |
| `Execute()` | 执行工具逻辑 | LLM 决定调用此工具时 |

### 简化实现示例

```go
type EchoTool struct{}

func (t *EchoTool) Name() string { return "echo" }

func (t *EchoTool) Description() string {
    return "原样返回输入的文本"
}

func (t *EchoTool) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "text": map[string]any{
                "type":        "string",
                "description": "要回显的文本",
            },
        },
        "required": []string{"text"},
    }
}

func (t *EchoTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    text, ok := args["text"].(string)
    if !ok {
        return &tool.ToolResult{Success: false, Error: "text is required"}, nil
    }
    return &tool.ToolResult{
        Success: true,
        Data:    map[string]any{"echo": text},
    }, nil
}
```

关键约定：

- `Execute` 的第一个参数是 `ChatContextInterface`，工具通过它获取 session ID、context 等信息
- `args` 是 `map[string]any`，来自 LLM 解析的 JSON 参数
- 业务错误不要通过 `error` 返回，而是填充 `ToolResult.Error` 并将 `error` 返回值设为 nil

## ToolResult

```go
type ToolResult struct {
    Success bool   `json:"success"`
    Data    any    `json:"data,omitempty"`
    Error   string `json:"error,omitempty"`
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `Success` | `bool` | 执行是否成功 |
| `Data` | `any` | 成功时的返回数据，会被序列化为 JSON |
| `Error` | `string` | 失败时的错误消息 |

### 成功返回

```go
return &tool.ToolResult{
    Success: true,
    Data: map[string]any{
        "weather": "晴",
        "temperature": 25,
        "city": "北京",
    },
}, nil
```

### 失败返回

`Error` 字段用于工具执行失败的情况，如参数缺失、操作拒绝等。注意此时 `error` 返回值仍为 nil：

```go
// 正确：业务错误通过 ToolResult 返回
return &tool.ToolResult{Success: false, Error: "path is required"}, nil

// 错误：不要通过 error 返回值报告业务错误
return nil, fmt.Errorf("path is required")
```

`error` 返回值**仅用于系统级错误**（如数据库连接失败、上下文取消等无法继续的情况）。

## ToolDef 与 LLM 函数调用格式

工具注册后，`ToolManager.GetOpenAITools()` 会将 `Tool` 接口信息转换为 LLM 的函数调用定义。

转换发生在 `toolManager.GetOpenAITools()` 中：

```go
func (m *toolManager) GetOpenAITools() []openai.ChatCompletionToolUnionParam {
    for _, t := range m.tools {
        schema := t.InputSchema()
        // 自动注入 execution_mode 参数
        props := schemaCopy["properties"].(map[string]any)
        props["execution_mode"] = map[string]any{
            "type":        "string",
            "enum":        []string{"sync", "concurrent", "async"},
            "default":     "sync",
            "description": "执行模式...",
        }

        tools = append(tools, openai.ChatCompletionFunctionTool(
            openai.FunctionDefinitionParam{
                Name:        t.Name(),
                Description: openai.String(t.Description()),
                Parameters:  openai.FunctionParameters(schemaCopy),
            },
        ))
    }
    return tools
}
```

注意：`execution_mode` 参数由 ToolManager **自动注入**到每个工具的 schema 中。工具实现者无需在自己的 `InputSchema()` 中包含此参数。

## 注册模式

CopCon 有两层管理器：

### ToolRegistry

轻量级注册表，仅管理工具实例。Agent 引擎用它来注册已知工具。

```go
type ToolRegistry interface {
    Register(tool Tool) error
    Get(name string) (Tool, error)
    List() []ToolInfo
}
```

### ToolManager

完整管理器，在 Registry 之上增加了工具调用（Execute）和 LLM 格式输出（GetOpenAITools）。

```go
type ToolManager interface {
    Register(tool Tool) error
    Unregister(name string) error
    Get(name string) (Tool, error)
    List() []ToolInfo
    Execute(chatCtx iface.ChatContextInterface, name string, args map[string]any) (*ToolResult, error)
    GetOpenAITools() []openai.ChatCompletionToolUnionParam
}
```

### ToolInfo

`List()` 方法返回的摘要信息：

```go
type ToolInfo struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    InputSchema map[string]any `json:"input_schema"`
}
```

## 用到 ChatContext 的场景

工具通过 `chatCtx` 参数获取会话上下文。典型用法：

```go
func (t *MyTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    // 获取 session ID
    sessionID := chatCtx.SessionID()

    // 获取标准 context（用于数据库操作、HTTP 请求等）
    ctx := chatCtx.Context()

    // 获取 agent ID
    agentID := chatCtx.AgentID()

    // ... 使用这些信息执行工具逻辑
}
```

`ChatContextInterface` 完整接口：

```go
type ChatContextInterface interface {
    Context() context.Context
    SessionID() string
    AgentID() string
    Events() <-chan entity.Event
    Emit(event entity.Event)
}
```

通常工具只需要 `SessionID()` 和 `Context()`。多数内置工具的 `Execute` 方法只用到这两个。

## 内置工具占位符

CopCon 内置以下工具（详见 [内置工具参考](./builtin-tools.md)）：

| 工具 | 名称 | 用途 |
|------|------|------|
| `ShellExecutor` | `shell_executor` | 白名单 shell 命令执行 |
| `CodeExecutor` | `code_executor` | Python/JavaScript 代码执行 |
| `FileOps` | `file_ops` | 文件读写操作 |
| `TodoTool` | `todolist` | 任务管理 |

## Panic 保护

工具 `Execute` 中发生的 panic 会被 `executeAsync` 中的 recover 捕获。但在 `executeSync` 和 `executeConcurrent` 中没有内置 panic 保护。建议工具实现中考虑 defer recover：

```go
func (t *MyTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (result *tool.ToolResult, err error) {
    defer func() {
        if r := recover(); r != nil {
            result = &tool.ToolResult{
                Success: false,
                Error:   fmt.Sprintf("tool panic: %v", r),
            }
            err = nil
        }
    }()

    // ... 正常逻辑
}
```

或者可以依赖 Agent Engine 层级的屏障。已知 `executeAsync` 有完整的 recover 保护，`executeSync` 和 `executeConcurrent` 可以通过 hook 层处理。