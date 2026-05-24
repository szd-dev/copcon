# 作为独立库使用

CopCon 的 `core/` 模块可以作为独立的 Go 库使用,让你在自己的项目中集成 AI Agent 能力。

## 安装

```bash
go get github.com/copcon/core
```

## 最小示例

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/copcon/core"
    "github.com/copcon/core/storage"
    "github.com/google/uuid"
)

func main() {
    // 使用内存存储
    store := storage.NewMemoryStore()
    
    // 创建 Harness
    cfg := &core.HarnessConfig{
        Agents: []core.AgentSpec{
            {
                Name:  "assistant",
                Model: "gpt-4",
                SystemPrompt: `你是一个有用的AI助手。`,
            },
        },
    }
    
    harness, err := core.NewHarnessWithStore(cfg, store)
    if err != nil {
        panic(err)
    }
    
    // 创建会话
    ctx := context.Background()
    sessionID := uuid.New().String()
    
    if err := store.CreateSession(ctx, sessionID); err != nil {
        panic(err)
    }
    
    // 创建对话上下文
    chatCtx := harness.NewChatContext(ctx, sessionID)
    
    // 发送消息
    req := &core.Request{
        Content: "你好",
    }
    
    if err := harness.Chat(chatCtx, req); err != nil {
        panic(err)
    }
    
    // 处理流式事件
    for event := range chatCtx.Events() {
        switch event.Type {
        case "message":
            fmt.Print(event.Data)
        case "tool_call":
            fmt.Printf("\n[工具调用: %s]\n", event.Data)
        case "done":
            println("\n[完成]")
        }
    }
}
```

## 存储实现

### 内存存储 (开发/测试)

```go
import "github.com/copcon/core/storage"

store := storage.NewMemoryStore()
```

### PostgreSQL 存储 (生产推荐)

```go
import "github.com/copcon/core/providers/postgres"

store := postgres.NewStore(postgres.Config{
    Host:     "localhost",
    Port:     5432,
    User:     "copcon",
    Password: "password",
    Database: "copcon",
    SSLMode:  "disable",
})

// 自动创建表
if err := store.Migrate(); err != nil {
    panic(err)
}
```

### MongoDB 存储

```go
import "github.com/copcon/core/providers/mongodb"

store := mongodb.NewStore(mongodb.Config{
    URI:      "mongodb://localhost:27017",
    Database: "copcon",
})
```

### SQLite 存储 (嵌入式场景)

```go
import "github.com/copcon/core/providers/sqlite"

store := sqlite.NewStore(sqlite.Config{
    Path: "./copcon.db",
})
```

## 自定义存储实现

如果内置存储不满足需求,可以实现 `Store` 接口:

```go
type Store interface {
    // Session 操作
    CreateSession(ctx context.Context, id string) error
    GetSession(ctx context.Context, id string) (*Session, error)
    ListSessions(ctx context.Context) ([]*Session, error)
    UpdateSession(ctx context.Context, session *Session) error
    DeleteSession(ctx context.Context, id string) error
    
    // Message 操作
    AppendMessage(ctx context.Context, sessionID string, msg *Message) error
    GetMessages(ctx context.Context, sessionID string) ([]*Message, error)
    
    // Tool 结果操作
    AppendToolResult(ctx context.Context, sessionID string, result *ToolResult) error
}
```

示例实现:

```go
type MyCustomStore struct {
    // 你的存储实现
}

// 实现所有接口方法...

// 使用
store := &MyCustomStore{}
harness, err := core.NewHarnessWithStore(cfg, store)
```

## 配置 LLM Provider

### OpenAI (默认)

```go
import "github.com/copcon/core/providers/openai"

provider := openai.NewProvider(openai.Config{
    APIKey:  os.Getenv("OPENAI_API_KEY"),
    BaseURL: "https://api.openai.com/v1", // 可选
})

harness, err := core.NewHarnessWithStoreAndProvider(cfg, store, provider)
```

### Azure OpenAI

```go
import "github.com/copcon/core/providers/openai"

provider := openai.NewProvider(openai.Config{
    APIKey:     os.Getenv("AZURE_API_KEY"),
    BaseURL:    "https://YOUR_RESOURCE.azure.com",
    APIVersion: "2024-02-15-preview",
})
```

### 其他 OpenAI 兼容 API

```go
provider := openai.NewProvider(openai.Config{
    APIKey:  os.Getenv("API_KEY"),
    BaseURL: "https://api.your-service.com/v1",
    Model:   "your-model",
})
```

## 集成到现有应用

### Gin 路由

```go
router := gin.Default()

router.GET("/chat/:sessionId", func(c *gin.Context) {
    sessionID := c.Param("sessionId")
    chatCtx := harness.NewChatContext(c.Request.Context(), sessionID)
    
    // 设置 SSE 头
    c.Writer.Header().Set("Content-Type", "text/event-stream")
    c.Writer.Header().Set("Cache-Control", "no-cache")
    c.Writer.Header().Set("Connection", "keep-alive")
    
    // 启动对话
    go func() {
        req := &core.Request{Content: "你好"}
        harness.Chat(chatCtx, req)
    }()
    
    // 流式响应
    for event := range chatCtx.Events() {
        c.SSEvent(string(event.Type), event.Data)
        c.Writer.Flush()
    }
})
```

### Echo 路由

```go
e := echo.New()

e.GET("/chat/:sessionId", func(c echo.Context) error {
    sessionID := c.Param("sessionId")
    chatCtx := harness.NewChatContext(c.Request().Context(), sessionID)
    
    c.Response().Header().Set("Content-Type", "text/event-stream")
    
    go func() {
        req := &core.Request{Content: "你好"}
        harness.Chat(chatCtx, req)
    }()
    
    for event := range chatCtx.Events() {
        fmt.Fprintf(c.Response(), "event: %s\ndata: %v\n\n", event.Type, event.Data)
        c.Response().Flush()
    }
    
    return nil
})
```

### 标准 HTTP Server

```go
http.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
    sessionID := r.URL.Query().Get("sessionId")
    ctx := r.Context()
    chatCtx := harness.NewChatContext(ctx, sessionID)
    
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
        return
    }
    
    go func() {
        req := &core.Request{Content: "你好"}
        harness.Chat(chatCtx, req)
    }()
    
    for event := range chatCtx.Events() {
        fmt.Fprintf(w, "event: %s\ndata: %v\n\n", event.Type, event.Data)
        flusher.Flush()
    }
})

http.ListenAndServe(":8080", nil)
```

## 高级用法

### 多 Agent 协作

```go
cfg := &core.HarnessConfig{
    Agents: []core.AgentSpec{
        {
            Name:  "planner",
            Model: "gpt-4",
            SystemPrompt: "你是项目规划师",
        },
        {
            Name:  "coder",
            Model: "gpt-4",
            SystemPrompt: "你是程序员",
        },
    },
}

// 切换到不同的 Agent
req := &core.Request{
    AgentName: "coder",
    Content: "请实现这个功能",
}
harness.Chat(chatCtx, req)
```

### 自定义工具链

```go
toolChain := []core.Tool{
    &core.CodeExecutorTool{},
    &core.FileOpsTool{},
    // 添加自定义工具
    &MyCustomTool{},
}

cfg := &core.HarnessConfig{
    Tools: toolChain,
    Agents: []core.AgentSpec{
        {
            Name:  "agent",
            Tools: []string{"code_executor", "file_ops", "my_custom_tool"},
        },
    },
}
```

详见 [自定义工具指南](../06-extending/custom-tool.md)

### 条件 Hook

```go
cfg.Hooks = append(cfg.Hooks, &ConditionalHook{
    Condition: func(req *core.Request) bool {
        return strings.Contains(req.Content, "敏感词")
    },
    Action: func(req *core.Request) {
        req.Content = "[内容已过滤]"
    },
})
```

详见 [自定义 Hook 指南](../06-extending/custom-hook.md)

## 测试

### 单元测试

```go
import "github.com/copcon/core/testing"

func TestMyAgent(t *testing.T) {
    harness := testing.NewTestHarness(t)
    
    ctx := context.Background()
    chatCtx := harness.NewChatContext(ctx, "test-session")
    
    req := &core.Request{Content: "测试"}
    if err := harness.Chat(chatCtx, req); err != nil {
        t.Fatal(err)
    }
    
    // 验证响应
    events := harness.CollectEvents(chatCtx)
    
    if len(events) == 0 {
        t.Error("expected events")
    }
}
```

### Integration 测试

```go
func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip()
    }
    
    // 使用真实存储
    store := postgres.NewTestStore(t)
    cfg := core.DefaultConfig()
    
    harness, err := core.NewHarnessWithStore(cfg, store)
    require.NoError(t, err)
    
    // 测试完整流程
    ctx := context.Background()
    chatCtx := harness.NewChatContext(ctx, "integration-test")
    
    req := &core.Request{Content: "完整测试"}
    err = harness.Chat(chatCtx, req)
    require.NoError(t, err)
    
    // 验证持久化
    messages, err := store.GetMessages(ctx, "integration-test")
    require.NoError(t, err)
    require.NotEmpty(t, messages)
}
```

## 示例项目

### 1. 简单聊天机器人

```go
// 完整的单文件聊天机器人
package main

import (
    "bufio"
    "context"
    "fmt"
    "os"
    
    "github.com/copcon/core"
    "github.com/copcon/core/storage"
    "github.com/google/uuid"
)

func main() {
    cfg := &core.HarnessConfig{
        Agents: []core.AgentSpec{
            {
                Name:         "chat",
                Model:        "gpt-4",
                ModelPrompt:  "你是友好的聊天助手",
            },
        },
    }
    
    store := storage.NewMemoryStore()
    harness, _ := core.NewHarnessWithStore(cfg, store)
    
    ctx := context.Background()
    sessionID := uuid.New().String()
    store.CreateSession(ctx, sessionID)
    chatCtx := harness.NewChatContext(ctx, sessionID)
    
    scanner := bufio.NewScanner(os.Stdin)
    fmt.Println("AI 助手 (Ctrl+C 退出):")
    
    for {
        fmt.Print("> ")
        if !scanner.Scan() {
            break
        }
        
        req := &core.Request{Content: scanner.Text()}
        go harness.Chat(chatCtx, req)
        
        for event := range chatCtx.Events() {
            if event.Type == "message" {
                fmt.Print(event.Data)
            }
        }
        fmt.Println()
    }
}
```

### 2. 代码审查助手

```go
cfg := &core.HarnessConfig{
    Agents: []core.AgentSpec{
        {
            Name:  "code-reviewer",
            Model: "gpt-4",
            SystemPrompt: `你是代码审查专家。
请审查以下代码并指出:
1. 潜在 bug
2. 性能问题
3. 安全漏洞
4. 代码风格建议

请用中文回答。`,
            Tools: []string{"file_ops"},
        },
    },
}
```

### 3. 数据分析助手

```go
cfg := &core.HarnessConfig{
    Agents: []core.AgentSpec{
        {
            Name:  "analyst",
            Model: "gpt-4",
            SystemPrompt: "你是数据分析师",
            Tools: []string{
                "code_executor",
                "file_ops",
                "database_query",
            },
        },
    },
}
```

## 最佳实践

### 1. 复用 Harness 实例

```go
// ❌ 错误: 每次请求都创建新实例
http.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
    harness, _ := core.NewHarness(cfg)
    // ...
})

// ✅ 正确: 复用实例
harness, _ := core.NewHarness(cfg)
http.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
    // 使用 harness
})
```

### 2. 正确处理上下文

```go
// ❌ 错误: 使用 background context
ctx := context.Background()
harness.Chat(chatCtx, req)

// ✅ 正确: 使用请求上下文
ctx := r.Context()
chatCtx := harness.NewChatContext(ctx, sessionID)
harness.Chat(chatCtx, req)
```

### 3. 错误恢复

```go
// 实现重试逻辑
for i := 0; i < 3; i++ {
    err := harness.Chat(chatCtx, req)
    if err == nil {
        break
    }
    
    if isRetryable(err) {
        time.Sleep(time.Second * time.Duration(i))
        continue
    }
    
    return err
}
```

### 4. 资源清理

```go
defer func() {
    if err := store.Close(); err != nil {
        log.Printf("store close error: %v", err)
    }
}()
```

## 下一步

- [自定义 Provider](custom-provider.md)
- [多 Agent 协作](multi-agent.md)
- [自定义工具](../06-extending/custom-tool.md)
- [自定义 Hook](../06-extending/custom-hook.md)
