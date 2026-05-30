# Agent 工具开发指南

## 概述

CopCon Agent 框架通过 Capability 系统提供可扩展的工具和钩子机制。本文档说明如何开发自定义 Agent 工具，并将其注册到系统中。

## 工具接口

每个工具必须实现 `tool.Tool` 接口：

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage  // JSON Schema 格式
    Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}
```

## 开发步骤

### 第一步：定义工具

```go
type weatherTool struct{}

func (t *weatherTool) Name() string {
    return "get_weather"
}

func (t *weatherTool) Description() string {
    return "获取指定城市的天气信息"
}

func (t *weatherTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "city": {"type": "string", "description": "城市名称"},
            "days": {"type": "integer", "description": "预报天数", "default": 1}
        },
        "required": ["city"]
    }`)
}
```

### 第二步：实现执行逻辑

```go
func (t *weatherTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
    var params struct {
        City string `json:"city"`
        Days int    `json:"days"`
    }
    if err := json.Unmarshal(input, &params); err != nil {
        return nil, fmt.Errorf("invalid parameters: %w", err)
    }

    // 调用天气 API
    weather, err := fetchWeather(ctx, params.City, params.Days)
    if err != nil {
        return nil, fmt.Errorf("weather fetch failed: %w", err)
    }

    result, _ := json.Marshal(weather)
    return result, nil
}
```

### 第三步：注册工具

工具通过 Capability 自动注册。创建一个 capability 结构体：

```go
type weatherCapability struct{}

func (c *weatherCapability) NewTool(config CapabilityConfig) (tool.Tool, error) {
    return &weatherTool{}, nil
}

func init() {
    capabilities.Register("weather", &weatherCapability{})
}
```

### 第四步：启用工具

在 Harness 配置中添加工具：

```go
config := core.HarnessConfig{
    CustomCapabilities: []string{"weather"},
}
h := core.NewHarness(config)
```

## 内置工具参考

CopCon 内置以下工具（位于 `core/capabilities/tools/`）：

| 工具 | 说明 | 注册名 |
|------|------|--------|
| ask_user | 向用户提问并等待回复 | ask_user |
| code_executor | 执行代码片段 | code_executor |
| shell_executor | 执行 shell 命令 | shell_executor |
| todo | 任务管理 | todo |
| knowledge_search | 知识库检索 | knowledge_search |

这些工具通过 `init()` 自动注册，Harness 导入后自动可用。

## 钩子系统

钩子在特定生命周期事件触发，用于插入通用逻辑：

- **logging**：记录所有工具调用和模型请求
- **tracing**：分布式追踪（OpenTelemetry）
- **memory**：长期记忆存储和检索
- **todo_injection**：自动注入任务管理工具

钩子注册方式与工具类似，通过 Capability 接口和 `init()` 函数。

## 测试

```go
func TestWeatherTool(t *testing.T) {
    tool := &weatherTool{}
    input := json.RawMessage(`{"city": "上海"}`)
    
    result, err := tool.Execute(context.Background(), input)
    assert.NoError(t, err)
    assert.Contains(t, string(result), "temperature")
}
```

测试时建议使用 mock 替换外部 API 调用，确保测试可重复执行。