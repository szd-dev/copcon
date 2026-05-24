# 自定义 Tool

Tool 是 CopCon Agent 可以调用的可执行操作。内置的 Tool 包括代码执行、Shell 命令、文件操作等。当内置工具无法满足业务需求时，你可以编写自定义 Tool。

## Tool 接口

每个 Tool 必须实现 `tool.Tool` 接口：

```go
// core/tool/manager.go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*ToolResult, error)
}
```

### 方法说明

| 方法 | 返回值 | 说明 |
|------|--------|------|
| `Name()` | `string` | 工具的唯一标识符，LLM 通过这个名字调用工具 |
| `Description()` | `string` | 工具功能的自然语言描述，帮助 LLM 判断何时使用 |
| `InputSchema()` | `map[string]any` | JSON Schema 格式的参数定义 |
| `Execute()` | `(*ToolResult, error)` | 工具的执行逻辑 |

### ToolResult 结构

```go
type ToolResult struct {
    Success bool   `json:"success"`
    Data    any    `json:"data,omitempty"`
    Error   string `json:"error,omitempty"`
}
```

- `Success` 为 `true` 时，`Data` 携带结果数据
- `Success` 为 `false` 时，`Error` 描述失败原因
- `Data` 字段通常使用 `map[string]any`，框架会自动序列化为 JSON

## 参数 Schema 定义

`InputSchema()` 返回一个 JSON Schema 对象，LLM 据此生成调用参数。

```go
func (t *MyTool) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "query": map[string]any{
                "type":        "string",
                "description": "搜索关键词",
            },
            "limit": map[string]any{
                "type":        "integer",
                "description": "返回结果数量上限",
                "default":     10,
            },
            "category": map[string]any{
                "type":        "string",
                "enum":        []string{"news", "blog", "paper"},
                "description": "搜索类别",
            },
        },
        "required": []string{"query"},
    }
}
```

### Schema 要点

1. **`type`** 必须是 `"object"`
2. **`properties`** 定义每个参数的类型和描述
3. **`required`** 列出必填参数
4. **`description`** 至关重要，LLM 依据它判断如何使用参数
5. 使用 `enum` 约束取值范围，减少 LLM 猜测
6. 框架会自动注入 `execution_mode` 参数（`sync`/`concurrent`/`async`），无需手动添加

## 输入验证

LLM 生成的参数不一定可靠，必须在 `Execute` 入口处验证所有输入。

### 基本验证模式

```go
func (t *MyTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    // 1. 提取并验证必填参数
    query, ok := args["query"].(string)
    if !ok || query == "" {
        return &tool.ToolResult{Success: false, Error: "query is required"}, nil
    }

    // 2. 提取可选参数，提供默认值
    limit := 10
    if v, ok := args["limit"].(float64); ok {
        limit = int(v)
    }

    // 3. 范围校验
    if limit < 1 || limit > 100 {
        return &tool.ToolResult{Success: false, Error: "limit must be between 1 and 100"}, nil
    }

    // 4. 枚举校验
    category, _ := args["category"].(string)
    validCategories := map[string]bool{"news": true, "blog": true, "paper": true}
    if category != "" && !validCategories[category] {
        return &tool.ToolResult{
            Success: false,
            Error:   fmt.Sprintf("invalid category: %s, must be one of news, blog, paper", category),
        }, nil
    }

    // 执行业务逻辑...
}
```

### 注意事项

- JSON 数字从 LLM 传来时是 `float64`，不是 `int`。使用 `args["limit"].(float64)` 然后转换
- 切片参数类型是 `[]any`，不是 `[]string`。需要逐个断言
- 返回验证错误时用 `ToolResult{Success: false, Error: ...}`，不要用 `error` 返回值。框架会把 `error` 当作系统级异常，而不是业务级失败
- `ToolResult` 和 `error` 同时返回时的约定：
  - 业务逻辑失败 → `ToolResult{Success: false, Error: "reason"}, nil`
  - 系统级异常 → `nil, fmt.Errorf("context: %w", err)`

## 简单示例：天气查询工具

```go
package weather

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/copcon/core/capabilities"
    "github.com/copcon/core/iface"
    "github.com/copcon/core/tool"
)

type WeatherTool struct {
    apiKey string
}

func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{apiKey: apiKey}
}

func (t *WeatherTool) Name() string {
    return "weather"
}

func (t *WeatherTool) Description() string {
    return "查询指定城市的当前天气信息，包括温度、湿度和天气状况"
}

func (t *WeatherTool) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{
                "type":        "string",
                "description": "城市名称，如 '北京' 或 'Tokyo'",
            },
            "unit": map[string]any{
                "type":        "string",
                "enum":        []string{"celsius", "fahrenheit"},
                "description": "温度单位，默认摄氏度",
            },
        },
        "required": []string{"city"},
    }
}

func (t *WeatherTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    city, ok := args["city"].(string)
    if !ok || city == "" {
        return &tool.ToolResult{Success: false, Error: "city is required"}, nil
    }

    unit, _ := args["unit"].(string)
    if unit == "" {
        unit = "celsius"
    }

    // 带超时的 HTTP 请求
    ctx, cancel := context.WithTimeout(chatCtx.Context(), 10*time.Second)
    defer cancel()

    result, err := t.fetchWeather(ctx, city, unit)
    if err != nil {
        return nil, fmt.Errorf("weather api call failed: %w", err)
    }

    return &tool.ToolResult{
        Success: true,
        Data:    result,
    }, nil
}

func (t *WeatherTool) fetchWeather(ctx context.Context, city, unit string) (map[string]any, error) {
    url := fmt.Sprintf("https://api.weather.example.com/current?city=%s&unit=%s&key=%s", city, unit, t.apiKey)
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    // 解析响应...
    return map[string]any{
        "city":        city,
        "temperature": 22,
        "humidity":    65,
        "condition":   "晴",
        "unit":        unit,
    }, nil
}
```

## 高级示例：数据库查询工具

这个示例展示了更复杂的模式：多操作分发、依赖注入、上下文传播。

```go
package dbquery

import (
    "database/sql"
    "fmt"
    "strings"

    "github.com/copcon/core/capabilities"
    "github.com/copcon/core/iface"
    "github.com/copcon/core/storage"
    "github.com/copcon/core/tool"
)

type DBQueryTool struct {
    db *sql.DB
}

func NewDBQueryTool(db *sql.DB) *DBQueryTool {
    return &DBQueryTool{db: db}
}

func (t *DBQueryTool) Name() string {
    return "db_query"
}

func (t *DBQueryTool) Description() string {
    return "执行只读 SQL 查询。仅支持 SELECT 语句，不允许修改数据。"
}

func (t *DBQueryTool) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "sql": map[string]any{
                "type":        "string",
                "description": "要执行的 SQL 查询语句，必须是 SELECT",
            },
            "max_rows": map[string]any{
                "type":        "integer",
                "description": "最大返回行数，默认 100",
                "default":     100,
            },
        },
        "required": []string{"sql"},
    }
}

func (t *DBQueryTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    query, ok := args["sql"].(string)
    if !ok || query == "" {
        return &tool.ToolResult{Success: false, Error: "sql is required"}, nil
    }

    // 安全检查：只允许 SELECT
    normalized := strings.TrimSpace(strings.ToUpper(query))
    if !strings.HasPrefix(normalized, "SELECT") {
        return &tool.ToolResult{Success: false, Error: "only SELECT queries are allowed"}, nil
    }

    maxRows := 100
    if v, ok := args["max_rows"].(float64); ok {
        maxRows = int(v)
    }

    // 使用 chatCtx 的 context 传播取消信号
    rows, err := t.db.QueryContext(chatCtx.Context(), query)
    if err != nil {
        return &tool.ToolResult{Success: false, Error: fmt.Sprintf("query failed: %v", err)}, nil
    }
    defer rows.Close()

    cols, _ := rows.Columns()
    results := make([]map[string]any, 0, maxRows)

    for rows.Next() && len(results) < maxRows {
        values := make([]any, len(cols))
        ptrs := make([]any, len(cols))
        for i := range values {
            ptrs[i] = &values[i]
        }
        if err := rows.Scan(ptrs...); err != nil {
            return &tool.ToolResult{Success: false, Error: fmt.Sprintf("scan failed: %v", err)}, nil
        }
        row := make(map[string]any)
        for i, col := range cols {
            row[col] = values[i]
        }
        results = append(results, row)
    }

    return &tool.ToolResult{
        Success: true,
        Data: map[string]any{
            "columns": cols,
            "rows":    results,
            "count":   len(results),
        },
    }, nil
}
```

## 注册自定义 Tool

有两种注册方式：通过 Capability 系统自动注册，或手动注册到 ToolManager。

### 方式一：Capability 自动注册（推荐）

创建一个 `ToolCapability` 实现并在 `init()` 中注册。框架启动时会自动发现并初始化。

```go
package weather

import (
    "github.com/copcon/core/capabilities"
    "github.com/copcon/core/tool"
)

func init() {
    capabilities.Register(&weatherCapability{})
}

type weatherCapability struct{}

func (c *weatherCapability) Name() string                      { return "tools.weather" }
func (c *weatherCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *weatherCapability) DependsOn() []string               { return nil }
func (c *weatherCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
    return NewWeatherTool(""), nil // 从环境变量读取 apiKey
}
```

命名约定：`tools.<tool_name>`，例如 `tools.weather`、`tools.db_query`。

然后在 Harness 所在的包中用空白导入触发 `init()`：

```go
import (
    _ "github.com/yourorg/copcon-weather/weather"
)
```

### 方式二：手动注册到 ToolManager

如果你不想用 Capability 系统，可以直接操作 `ToolManager`：

```go
tm := tool.NewToolManager()
tm.Register(&WeatherTool{apiKey: "your-key"})
tm.Register(&DBQueryTool{db: myDB})
```

### Capability 依赖声明

如果你的 Tool 依赖存储层，通过 `DependsOn()` 声明：

```go
func (c *dbQueryCapability) DependsOn() []string {
    return nil // 不依赖其他 capability
}

// 但可以通过 deps 获取存储实例
func (c *dbQueryCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
    // deps.SessionStore, deps.MessageStore, deps.TodoStore 均可用
    return NewDBQueryTool(myDB), nil
}
```

## 错误处理模式

### 分层错误处理

```go
func (t *MyTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    // 验证错误：业务层面，LLM 可以重试
    if err := validate(args); err != nil {
        return &tool.ToolResult{Success: false, Error: err.Error()}, nil
    }

    // 执行逻辑
    result, err := t.doWork(chatCtx, args)
    if err != nil {
        // 判断是否为可重试的业务错误
        if isRetryable(err) {
            return &tool.ToolResult{
                Success: false,
                Error:   fmt.Sprintf("temporary failure, please retry: %v", err),
            }, nil
        }
        // 系统级异常，不应由 LLM 重试
        return nil, fmt.Errorf("my_tool execution failed: %w", err)
    }

    return &tool.ToolResult{Success: true, Data: result}, nil
}
```

### 超时处理

始终使用 `chatCtx.Context()` 创建超时上下文，这样当用户断开连接时工具也会被取消：

```go
func (t *MyTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    ctx, cancel := context.WithTimeout(chatCtx.Context(), 30*time.Second)
    defer cancel()

    // 使用 ctx 而非 context.Background()
    result, err := someHTTPCall(ctx, ...)
    if ctx.Err() == context.DeadlineExceeded {
        return &tool.ToolResult{Success: false, Error: "operation timed out after 30s"}, nil
    }
    // ...
}
```

## DelegationTool 接口

如果你的 Tool 将工作委托给子 Agent（例如多 Agent 协作场景），实现 `DelegationTool` 接口以避免框架自动注入 `execution_mode` 参数造成 Schema 冲突：

```go
type DelegationTool interface {
    IsDelegationTool() bool
}

type AgentCallTool struct{}

func (t *AgentCallTool) IsDelegationTool() bool { return true }
```

## 测试自定义 Tool

### 单元测试

```go
package weather_test

import (
    "context"
    "testing"

    "github.com/copcon/core/iface"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/yourorg/copcon-weather/weather"
)

// mockChatCtx 实现 iface.ChatContextInterface
type mockChatCtx struct {
    ctx context.Context
}

func (m *mockChatCtx) Context() context.Context  { return m.ctx }
func (m *mockChatCtx) SessionID() string          { return "test-session" }
func (m *mockChatCtx) AgentID() string            { return "test-agent" }
// ... 其他方法留空即可

func TestWeatherTool_InputValidation(t *testing.T) {
    tool := weather.NewWeatherTool("test-key")
    chatCtx := &mockChatCtx{ctx: context.Background()}

    t.Run("missing city returns error", func(t *testing.T) {
        result, err := tool.Execute(chatCtx, map[string]any{})
        require.NoError(t, err)
        assert.False(t, result.Success)
        assert.Contains(t, result.Error, "city is required")
    })

    t.Run("empty city returns error", func(t *testing.T) {
        result, err := tool.Execute(chatCtx, map[string]any{"city": ""})
        require.NoError(t, err)
        assert.False(t, result.Success)
    })

    t.Run("invalid unit returns error", func(t *testing.T) {
        result, err := tool.Execute(chatCtx, map[string]any{
            "city": "Tokyo",
            "unit": "kelvin",
        })
        require.NoError(t, err)
        assert.False(t, result.Success)
        assert.Contains(t, result.Error, "invalid unit")
    })
}

func TestWeatherTool_Schema(t *testing.T) {
    tool := weather.NewWeatherTool("test-key")

    assert.Equal(t, "weather", tool.Name())
    assert.NotEmpty(t, tool.Description())

    schema := tool.InputSchema()
    assert.Equal(t, "object", schema["type"])

    props, ok := schema["properties"].(map[string]any)
    require.True(t, ok)
    assert.Contains(t, props, "city")
    assert.Contains(t, props, "unit")
}
```

## 最佳实践

1. **描述写清楚**。`Description()` 是 LLM 判断何时调用工具的唯一依据。写清楚工具的适用场景和限制，例如"仅支持 SELECT 查询"比"执行 SQL"好得多

2. **参数名自解释**。用 `max_rows` 而不是 `n`，用 `include_archived` 而不是 `archived`

3. **验证所有输入**。不要信任 LLM 生成的参数。缺少验证会导致运行时 panic

4. **结果结构化**。返回 `map[string]any` 而不是自由文本，LLM 更容易解析结构化数据

5. **尊重上下文取消**。用 `chatCtx.Context()` 传递取消信号，不要用 `context.Background()`

6. **一个工具一个职责**。不要做"瑞士军刀"式的工具。拆分成多个专注的工具比一个万能工具更容易被 LLM 正确使用

7. **幂等设计**。LLM 可能重复调用同一个工具，尽量让调用幂等或至少安全

8. **敏感操作加确认**。对于不可逆操作，使用 `chatCtx.RequestInput()` 请求用户确认

## 下一步

- [自定义 Hook](custom-hook.md) - 在 Agent 生命周期中注入自定义逻辑
- [自定义 Provider](custom-provider.md) - 实现自定义存储后端
- [内置 Tool 详解](../05-built-in-capabilities/tools/overview.md) - 了解内置工具的实现
