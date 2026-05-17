# 注册工具

本文演示如何创建一个完整的工具实现并注册到 CopCon 系统中。

## 步骤一：创建工具实现

每个工具需要实现 `tool.Tool` 接口的四个方法。

以天气查询工具为例：

```go
// /server/internal/tools/weather.go
package tools

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/copcon/server/internal/domain/iface"
    "github.com/copcon/server/internal/tool"
)

type WeatherTool struct {
    apiKey  string
    client  *http.Client
}

func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{
        apiKey: apiKey,
        client: &http.Client{Timeout: 10 * time.Second},
    }
}
```

### Name() — 工具名称

```go
func (t *WeatherTool) Name() string {
    return "get_weather"
}
```

名称是唯一的。LLM 会根据此名称在 function calling 中进行匹配。使用 snake_case 命名，保持与内置工具风格一致。

### Description() — 功能描述

```go
func (t *WeatherTool) Description() string {
    return "查询指定城市的天气信息，返回温度、天气状况和湿度"
}
```

描述文字直接发给 LLM，LLM 据此判断是否调用此工具。尽量精确，写明输入和输出。

### InputSchema() — 参数 Schema

```go
func (t *WeatherTool) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{
                "type":        "string",
                "description": "城市名称，如 '北京'、'上海'",
            },
            "province": map[string]any{
                "type":        "string",
                "description": "省份（可选），当城市名有歧义时使用",
            },
        },
        "required": []string{"city"},
    }
}
```

Schema 遵循 JSON Schema 规范。注意：

- `required` 数组列出必填字段
- 每个 property 中写明 `type` 和 `description`
- **不要**在 Schema 中添加 `execution_mode` 参数 — `ToolManager.GetOpenAITools()` 会自动注入

### Execute() — 执行逻辑

```go
func (t *WeatherTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    city, ok := args["city"].(string)
    if !ok || city == "" {
        return &tool.ToolResult{
            Success: false,
            Error:   "city is required",
        }, nil
    }

    province, _ := args["province"].(string)

    // 调用天气 API
    ctx := chatCtx.Context()
    weather, err := t.fetchWeather(ctx, city, province)
    if err != nil {
        return &tool.ToolResult{
            Success: false,
            Error:   fmt.Sprintf("查询天气失败: %v", err),
        }, nil
    }

    return &tool.ToolResult{
        Success: true,
        Data: map[string]any{
            "city":        weather.City,
            "temperature": weather.Temperature,
            "condition":   weather.Condition,
            "humidity":    weather.Humidity,
        },
    }, nil
}

type weatherResult struct {
    City        string  `json:"city"`
    Temperature float64 `json:"temperature"`
    Condition   string  `json:"condition"`
    Humidity    int     `json:"humidity"`
}

func (t *WeatherTool) fetchWeather(ctx context.Context, city, province string) (*weatherResult, error) {
    // 实际 HTTP 请求代码省略
    return &weatherResult{
        City: city, Temperature: 25.0, Condition: "晴", Humidity: 60,
    }, nil
}
```

关键点：

- 参数校验通过 `map[string]any` 的 type assertion 完成
- 参数错误返回 `ToolResult.Error`，**不**返回 Go error
- 使用 `chatCtx.Context()` 进行网络请求，确保上下文传播
- 工具调用时不需要 `execution_mode` 参数 — 它已被 Agent Engine 提取并处理

## 步骤二：注册到 ToolRegistry

在 Engine 初始化时注册工具：

```go
// 通常在 main.go 或初始化代码中
func setupEngine() agent.AgentEngine {
    // 创建工具注册表
    registry := tool.NewToolRegistry()

    // 注册天气工具
    weatherTool := tools.NewWeatherTool("your-api-key")
    registry.Register(weatherTool)

    // 注册其他工具...
    registry.Register(tools.NewFileOps(""))
    registry.Register(tools.NewCodeExecutor())
    registry.Register(tools.NewShellExecutor())

    // 创建工具管理器
    toolMgr := tool.NewToolManager()
    for _, info := range registry.List() {
        t, _ := registry.Get(info.Name)
        toolMgr.Register(t)
    }

    // 创建 Agent Engine
    engine := agent.NewAgentEngine(
        agent.WithLLMProvider(llmAdapter),
        agent.WithToolManager(toolMgr),
        agent.WithSessionManager(sessionMgr),
        agent.WithContextManager(contextMgr),
    )

    return engine
}
```

`ToolRegistry.Register()` 允许同名覆盖：

```go
func (r *toolRegistry) Register(tool Tool) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    name := tool.Name()
    r.tools[name] = tool  // 直接覆盖
    return nil
}
```

而 `ToolManager.Register()` 会拒绝重复注册：

```go
func (m *toolManager) Register(tool Tool) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    name := tool.Name()
    if _, exists := m.tools[name]; exists {
        return fmt.Errorf("%w: %s", ErrToolAlreadyExists, name)
    }
    m.tools[name] = tool
    return nil
}
```

## 步骤三：LLM 工具定义自动生成

工具注册完成后，Agent Engine 在每次对话中通过 `GetOpenAITools()` 获取工具列表并发送给 LLM：

```go
// agent/engine.go 内部调用
tools := e.toolMgr.GetOpenAITools()

// GetOpenAITools 自动为每个工具注入 execution_mode 参数
// 最终发送给 LLM 的 function 定义类似：
// {
//     "name": "get_weather",
//     "description": "查询指定城市的天气信息...",
//     "parameters": {
//         "type": "object",
//         "properties": {
//             "city": { "type": "string", "description": "城市名称" },
//             "province": { "type": "string", "description": "省份" },
//             "execution_mode": {
//                 "type": "string",
//                 "enum": ["sync", "concurrent", "async"],
//                 "default": "sync",
//                 "description": "执行模式..."
//             }
//         },
//         "required": ["city"]
//     }
// }
```

## 完整示例：天气工具

```go
// /server/internal/tools/weather.go
package tools

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/copcon/server/internal/domain/iface"
    "github.com/copcon/server/internal/tool"
)

type WeatherTool struct {
    apiKey string
    client *http.Client
}

func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{
        apiKey: apiKey,
        client: &http.Client{Timeout: 10 * time.Second},
    }
}

func (t *WeatherTool) Name() string {
    return "get_weather"
}

func (t *WeatherTool) Description() string {
    return "查询指定城市的天气信息，返回温度、天气状况和湿度"
}

func (t *WeatherTool) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{
                "type":        "string",
                "description": "城市名称，如 '北京'、'上海'",
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

    ctx := chatCtx.Context()

    req, err := http.NewRequestWithContext(ctx, "GET",
        fmt.Sprintf("https://api.weather.com/v1/current?city=%s&key=%s", city, t.apiKey), nil)
    if err != nil {
        return &tool.ToolResult{Success: false, Error: err.Error()}, nil
    }

    resp, err := t.client.Do(req)
    if err != nil {
        return &tool.ToolResult{Success: false, Error: fmt.Sprintf("请求失败: %v", err)}, nil
    }
    defer resp.Body.Close()

    var result struct {
        City    string  `json:"city"`
        Temp    float64 `json:"temperature"`
        Weather string  `json:"weather"`
        Humidity int    `json:"humidity"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return &tool.ToolResult{Success: false, Error: fmt.Sprintf("解析失败: %v", err)}, nil
    }

    return &tool.ToolResult{
        Success: true,
        Data: map[string]any{
            "city":        result.City,
            "temperature": result.Temp,
            "condition":   result.Weather,
            "humidity":    result.Humidity,
        },
    }, nil
}
```

## LLM 如何决定调用你的工具

当 LLM 收到用户消息后，系统提示词中包含的工具定义使其能够判断何时调用工具。以天气工具为例：

**用户消息**：
> 北京今天穿什么衣服合适？

**LLM 推理**：
1. 用户想知道北京天气 → 调用 `get_weather(city="北京")`
2. LLM 返回 function call 而不是文本回复
3. Agent Engine 执行工具，拿到天气数据
4. LLM 根据天气数据生成文本回复："北京今天晴，25°C，建议穿薄外套"

工具调用由 Agent Engine 自动编排，工具实现者无需关心 LLM 何时调用——只需实现 `Execute` 方法。

## 检查清单

新建工具后确认：

- [ ] 实现 `Tool` 接口全部四个方法
- [ ] `Name()` 返回 snake_case 的唯一名称
- [ ] `Description()` 准确描述工具的功能和输入输出
- [ ] `InputSchema()` 遵循 JSON Schema 格式，不包含 `execution_mode`
- [ ] `Execute()` 中参数校验完成后再执行业务逻辑
- [ ] 业务错误通过 `ToolResult.Error` 返回，不返回 Go error
- [ ] 使用 `chatCtx.Context()` 进行网络/数据库操作
- [ ] 在初始化代码中调用 `registry.Register()` 或 `toolMgr.Register()`
- [ ] 编写测试验证 Execute 行为