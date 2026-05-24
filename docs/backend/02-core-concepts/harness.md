# Harness 配置指南

Harness 是 CopCon 的核心配置入口,通过 `HarnessConfig` 声明所有组件。

## 核心概念

Harness 负责:
- **初始化**: 加载配置、连接存储、注册能力
- **编排**: 协调 Agent、Tools、Hooks 的执行流程
- **生命周期**: 管理会话创建、对话、清理

## 配置结构

```go
type HarnessConfig struct {
    Store     StoreConfig    // 存储配置
    Agents    []AgentSpec    // Agent 规格列表
    Tools     []ToolSpec     // 工具规格列表
    Hooks     []HookSpec     // Hook 规格列表
    Log       *slog.Logger   // 日志实例(可选)
}
```

## StoreConfig 详解

```go
type StoreConfig struct {
    Provider string              // 存储提供者类型
    Settings map[string]any      // 提供者特定配置
}
```

### 支持的 Provider

| Provider | 说明 | 示例 |
|----------|------|------|
| `postgres` | PostgreSQL 存储 | `"provider": "postgres"` |
| `qdrant` | Qdrant 向量存储 | `"provider": "qdrant"` |
| `memory` | 内存存储 (开发用) | `"provider": "memory"` |

### PostgreSQL 配置示例

```yaml
store:
  provider: postgres
  settings:
    host: localhost
    port: 5432
    user: copcon
    password: secret
    database: copcon
    sslmode: disable
```

### Qdrant 配置示例

```yaml
store:
  provider: qdrant
  settings:
    host: localhost
    port: 6333
    collection: copcon_memories
    vector_size: 384
```

## AgentSpec 详解

```go
type AgentSpec struct {
    Name        string   // Agent 名称 (唯一)
    Model       string   // LLM 模型名称
    SystemPrompt string  // 系统提示词
    Tools       []string // 工具名称列表
    Hooks       []string // Hook 名称列表
}
```

### 配置示例

```yaml
agents:
  - name: assistant
    model: gpt-4
    system_prompt: |
      你是一个有用的AI助手。
      你可以帮助用户回答问题、编写代码、分析数据。
    tools:
      - code_executor
      - file_ops
      - search
    hooks:
      - logging
      - memory
```

## 初始化流程

### 1. 创建 Harness 实例

```go
harness, err := core.NewHarness(cfg)
if err != nil {
    log.Fatal(err)
}
```

### 2. 自动迁移数据库

```go
if err := harness.AutoMigrate(); err != nil {
    log.Fatal(err)
}
```

### 3. 创建会话

```go
sessionID := uuid.New().String()
if err := harness.Store().SessionStore().Create(ctx, &storage.Session{
    ID: sessionID,
    AgentName: "assistant",
}); err != nil {
    log.Fatal(err)
}
```

### 4. 创建对话上下文

```go
chatCtx := harness.NewChatContext(ctx, sessionID)
```

### 5. 发送消息

```go
if err := harness.Chat(chatCtx, req); err != nil {
    log.Fatal(err)
}
```

### 6. 处理流式事件

```go
for event := range chatCtx.Events() {
    // 处理事件
    switch event.Type {
    case "message":
        fmt.Print(event.Data)
    case "tool_call":
        fmt.Printf("\n[Calling tool: %s]\n", event.Data)
    case "tool_result":
        fmt.Printf("\n[Tool result: %s]\n", event.Data)
    }
}
```

## 完整示例

```go
package main

import (
    "context"
    "log"
    
    "github.com/copcon/core"
    "github.com/google/uuid"
)

func main() {
    // 配置
    cfg := &core.HarnessConfig{
        Store: core.StoreConfig{
            Provider: "postgres",
            Settings: map[string]any{
                "host":     "localhost",
                "port":     5432,
                "user":     "copcon",
                "password": "secret",
                "database": "copcon",
            },
        },
        Agents: []core.AgentSpec{
            {
                Name:  "assistant",
                Model: "gpt-4",
                SystemPrompt: `你是一个有用的AI助手。
你可以帮助用户回答问题、编写代码、分析数据。`,
                Tools: []string{"code_executor", "file_ops"},
                Hooks: []string{"logging", "memory"},
            },
        },
    }
    
    // 初始化
    harness, err := core.NewHarness(cfg)
    if err != nil {
        log.Fatal(err)
    }
    
    // 迁移数据库
    if err := harness.AutoMigrate(); err != nil {
        log.Fatal(err)
    }
    
    // 创建会话
    ctx := context.Background()
    sessionID := uuid.New().String()
    if err := harness.Store().SessionStore().Create(ctx, &storage.Session{
        ID:        sessionID,
        AgentName: "assistant",
    }); err != nil {
        log.Fatal(err)
    }
    
    // 创建对话上下文
    chatCtx := harness.NewChatContext(ctx, sessionID)
    
    // 发送消息
    req := &core.Request{
        Content: "你好,请帮我写一个Python程序",
    }
    if err := harness.Chat(chatCtx, req); err != nil {
        log.Fatal(err)
    }
    
    // 处理流式事件
    for event := range chatCtx.Events() {
        switch event.Type {
        case "message":
            fmt.Print(event.Data)
        case "tool_call":
            fmt.Printf("\n[调用工具: %s]\n", event.Data)
        case "tool_result":
            fmt.Printf("\n[工具结果: %s]\n", event.Data)
        }
    }
}
```

## 最佳实践

### 1. 配置外部化

```go
cfg, err := core.LoadConfigFromFile("config.yaml")
if err != nil {
    log.Fatal(err)
}
harness, err := core.NewHarness(cfg)
```

### 2. 错误处理

```go
harness, err := core.NewHarness(cfg)
if err != nil {
    // 初始化失败
    switch err.(type) {
    case *core.ConfigError:
        log.Printf("配置错误: %v", err)
    case *core.StorageError:
        log.Printf("存储连接失败: %v", err)
    default:
        log.Printf("未知错误: %v", err)
    }
    return
}
```

### 3. 资源清理

```go
defer harness.Close()
```

### 4. 并发安全

```go
// 多个协程可以同时使用同一个 harness
go func() {
    harness.Chat(chatCtx1, req1)
}()

go func() {
    harness.Chat(chatCtx2, req2)
}()
```

## 常见问题

### Q: 如何动态修改配置?

A: Harness 支持配置热重载。修改配置后调用 `harness.Reload(newCfg)` 即可。

### Q: 如何支持多个 Agent?

A: 在 `Agents` 配置中定义多个 Agent,通过 `agent_name` 参数选择使用哪个 Agent。

### Q: 如何禁用某个工具或 Hook?

A: 从 `AgentSpec.Tools` 或 `AgentSpec.Hooks` 列表中移除即可。

### Q: 如何添加自定义工具?

A: 
1. 实现 Tool 接口
2. 在 `init()` 中自动注册
3. 在 `AgentSpec.Tools` 中引用

详见 [自定义工具指南](../06-extending/custom-tool.md)

## 下一步

- [能力系统](capabilities.md)
- [SSD 流式传输](streaming.md)
- [核心库独立使用](../03-core-library/as-library.md)
