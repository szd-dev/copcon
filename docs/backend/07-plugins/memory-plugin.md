# 记忆插件

`MemoryPlugin` 是 CopCon 的长期记忆系统插件。它在每次请求中完成两个关键操作：将相关历史记忆注入上下文窗口（检索），并将新产生的 assistant 回复存入向量数据库（存储）。

**文件位置**: `server/internal/plugins/memory/memory_plugin.go`

## 核心设计

```go
type MemoryPlugin struct {
    memoryMgr memory.MemoryManager
    logger    *slog.Logger
}

func NewMemoryPlugin(memoryMgr memory.MemoryManager) *MemoryPlugin {
    return &MemoryPlugin{
        memoryMgr: memoryMgr,
        logger:    slog.Default(),
    }
}
```

### Hook 元数据

| 属性 | 值 | 说明 |
|------|-----|------|
| `Name()` | `"memory_plugin"` | 标识符 |
| `Points()` | `[AfterContextBuild, OnMessagePersist]` | 两个 hook 点 |
| `Priority()` | `100` | 默认优先级 |

## AfterContextBuild：检索并注入记忆

在上下文构建完成后、发送给 LLM 之前，MemoryPlugin 从向量数据库中检索与当前对话相关的历史记忆，并将其注入消息序列。

### 执行流程

#### 1. 查找最后一条用户消息

```go
lastUserContent := p.findLastUserMessage(*ctx.Messages)
```

从消息序列末尾向前遍历，找到第一条 `role="user"` 且有内容的消息，提取其文本作为搜索查询。

#### 2. 生成查询向量

```go
query := encodeTextToVector(lastUserContent)
```

将用户消息文本转换为 `[]float32` 向量。当前实现是一个简单的占位编码（逐字节归一化到 [0, 1]），生产环境应替换为真正的 Embedding 模型（如 OpenAI text-embedding-3-small）。

#### 3. 执行向量搜索

```go
results, err := p.memoryMgr.Search(ctx.ChatCtx, query, 5)
```

调用 `MemoryManager.Search` 在 Qdrant 中搜索 Top-5 相关记忆。搜索通过 `session_id` 过滤，确保只检索当前会话的历史内容。

#### 4. 注入记忆到消息序列

```go
systemMsg := entity.MessageForLLM{
    Role:    "system",
    Content: formatSearchResults(results),
}
*ctx.Messages = append([]entity.MessageForLLM{systemMsg}, *ctx.Messages...)
```

将检索结果格式化为 `role="system"` 的消息，插入到消息序列的最前面（在所有其他消息之前）。

检索结果格式：

```
Relevant context from previous conversations:
- [记忆内容 1]
- [记忆内容 2]
- ...
```

## OnMessagePersist：异步存储消息

在消息持久化 hook 点，MemoryPlugin 将 assistant 的新回复异步存入向量数据库。

### 执行流程

#### 1. 查找最新 assistant 消息

从消息序列末尾向前遍历，找到第一条 `role="assistant"` 且有 `Content` 的消息。

#### 2. 异步存储

```go
go func() {
    err := mgr.Store(chatCtx, &memory.Memory{
        Content:    content,
        SessionID:  sessionID,
        Role:       "assistant",
        MemoryType: "conversation",
    })
    if err != nil {
        logger.Warn("memory store failed",
            "session_id", sessionID,
            "error", err,
        )
    }
}()
```

存储操作在独立的 goroutine 中异步执行，不阻塞 Agent 循环。所有变量在闭包内捕获以确保并发安全。存储失败仅记录 Warning 日志。

### 存储的 Memory 结构

```go
type Memory struct {
    ID         string         `json:"id"`
    Content    string         `json:"content"`
    SessionID  string         `json:"session_id"`
    Role       string         `json:"role"`
    Timestamp  int64          `json:"timestamp"`
    MemoryType string         `json:"memory_type"`
    Metadata   map[string]any `json:"metadata"`
    Score      float32        `json:"score,omitempty"`
}
```

## Nil Client 处理（优雅降级）

当 `memoryMgr` 为 nil 时，所有 hook 操作直接返回 nil，变为零开销的 no-op：

```go
func (p *MemoryPlugin) Execute(ctx *hook.HookContext) error {
    if p.memoryMgr == nil {
        return nil
    }
    // ...
}
```

这意味着未配置 Qdrant 的部署场景下 MemoryPlugin 不会产生任何错误。

## Qdrant 生产环境配置

### 依赖服务

Qdrant 通过 Docker Compose 启动：

```yaml
qdrant:
  image: qdrant/qdrant:v1.17.0
  ports:
    - "6333:6333"
    - "6334:6334"
```

### 初始化

使用初始化脚本创建 Collection：

```bash
bash scripts/init-qdrant.sh
```

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `QDRANT_HOST` | `localhost` | Qdrant 服务地址 |
| `QDRANT_PORT` | `6333` | Qdrant HTTP 端口 |

### 注册示例

```go
// 创建 Qdrant 客户端
qdrantClient := qdrant.NewClient(&qdrant.Config{
    Host: os.Getenv("QDRANT_HOST"),
    Port: 6333,
})

// 创建 MemoryManager
memoryMgr := memory.NewMemoryManager(qdrantClient, "agent_memories")

// 创建 MemoryPlugin
memoryPlugin := memorypkg.NewMemoryPlugin(memoryMgr)

// 注册到 HookRunner
runner.Register(memoryPlugin)
```

若 `memoryMgr` 为 nil（例如 Qdrant 不可用时），插件自动降级为 no-op。

### 向量编码说明

当前 `encodeTextToVector` 是一个占位实现，仅用于功能验证。生产部署时，需要在 `encodeTextToVector` 中集成真正的 Embedding 服务（如调用 OpenAI Embeddings API），或者将查询向量生成逻辑移到 `MemoryManager` 内部。嵌入质量直接影响检索精度。