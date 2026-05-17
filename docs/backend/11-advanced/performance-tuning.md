# 性能调优

## 概述

CopCon 的性能涉及多个层面：Agent 并发控制、工具执行模式、数据库查询、向量检索、LLM 流式处理。本章逐一分析调优策略。

## 1. 并发控制

### WithConcurrency

Agent Engine 通过 `WithConcurrency(n)` 控制工具并发执行的上限：

```go
agentEngine := agent.NewAgentEngine(
    agentRegistry, sessionMgr, contextMgr, asyncRegistry,
    agent.WithConcurrency(10),  // 最多 10 个工具同时执行
)
```

**默认值：5**

内部实现使用 `golang.org/x/sync/semaphore.Weighted`：

```go
// engine.go
type engineImpl struct {
    concurrency    int
    concurrencySem *semaphore.Weighted
}
```

### 并发数选择

| 并发数 | 适用场景 | 考量 |
|--------|---------|------|
| 1 | 需要严格顺序执行的工具链 | 每次只执行一个工具，无竞态问题 |
| 3-5 | 一般 Web 应用 | 默认配置，适合大多数场景 |
| 10-20 | 大量 I/O 密集型工具（Shell、API 调用） | CPU 可能成为瓶颈 |
| 50+ | 批量处理、高并发场景 | 需要充足的 CPU 和内存 |

**调优建议：**

- 工具以 I/O 为主（Shell 执行、文件读写、API 调用）→ 适当提高并发数
- 工具以 CPU 为主（代码执行、复杂计算）→ 并发数不超过 CPU 核心数
- 通过 `AfterLLMCall` Hook 监控并发工具的排队时间，判断是否需要调整

### 全局并发控制

除了引擎内部的工具并发控制，还可通过 Gin 中间件控制整体请求并发：

```go
var globalSem = semaphore.NewWeighted(100) // 最多 100 个并发请求

func GlobalConcurrencyMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        if !globalSem.TryAcquire(1) {
            c.JSON(503, gin.H{"error": "server busy"})
            c.Abort()
            return
        }
        defer globalSem.Release(1)
        c.Next()
    }
}
```

## 2. 工具执行模式选择

CopCon 为每个工具提供三种执行模式（`execution_mode` 参数）：

| 模式 | 说明 | 适用场景 |
|------|------|---------|
| `sync` | 同步执行，Agent 等待工具完成后继续 | 快速工具（< 1s），结果直接影响后续推理 |
| `concurrent` | 并发执行，与其他工具并行 | 独立的工具调用，无需互相等待 |
| `async` | 异步执行，后台运行，Agent 继续推理 | 长时间工具（> 5s），结果在后续循环中获取 |

### 执行模式示例

在 `part_create` 事件中，工具调用的 `args` 字段包含执行模式：

```json
{
  "type": "part_create",
  "data": {
    "partType": "tool-call",
    "toolName": "code_executor",
    "args": "{\"language\":\"python\",\"code\":\"...\",\"execution_mode\":\"async\"}"
  }
}
```

### 选择策略

```
工具执行时间
    │
    ├── < 1s ──→ sync       （简单 shell 命令、文件读取）
    │
    ├── 1-5s ──→ concurrent （中等代码执行、API 查询）
    │
    └── > 5s ──→ async      （大数据处理、长时间训练）
```

**注意：** 当前执行模式的选择由 LLM 根据工具描述自主决定。通过调整工具的 InputSchema 描述可以引导 LLM 选择合适的模式。

## 3. 内存优化

### Qdrant 批处理

向量检索时，批次大小直接影响内存和延迟：

```go
// Qdrant 客户端批量 upsert
const (
    DefaultBatchSize  = 100   // 默认批次大小
    MaxBatchSize      = 500   // 最大批次（避免单次请求过大）
    RecommBatchSize   = 200   // 推荐值
)

func (m *MemoryManager) Upsert(points []Point) error {
    for i := 0; i < len(points); i += RecommBatchSize {
        end := i + RecommBatchSize
        if end > len(points) {
            end = len(points)
        }
        batch := points[i:end]
        // upsert batch...
    }
}
```

**调优建议：**

| 向量数 | 建议批次 | 说明 |
|--------|---------|------|
| < 1000 | 全部一次提交 | 无需分批 |
| 1000 - 10000 | 200/批 | 平衡延迟和吞吐 |
| > 10000 | 500/批 | 最大化吞吐 |

### 向量维度

CopCon 默认使用 OpenAI `text-embedding-3-small`（1536 维）或 `text-embedding-3-large`（3072 维）。

维度与 Qdrant 内存消耗的关系：

```
单条向量内存 ≈ 维度 × 4 bytes × 1.3 (索引开销)

1536 维: 约 8KB / 条
3072 维: 约 16KB / 条
```

如果记忆数据量不大（< 10万条），使用 1536 维即可。

### 上下文消息缓存

`ContextManager.BuildContext` 从数据库加载历史消息。设置合理的 Token 上限可以避免加载过多消息：

```go
// 256000 tokens — 当前硬编码值
messages, err := e.contextMgr.BuildContext(chatCtx, "", 256000, systemPrompt)
```

建议：
- 短对话应用：32000-64000 tokens
- 长对话应用：128000-256000 tokens
- 通过 config.yaml 暴露 maxTokens 配置项

## 4. LLM 流式处理

### 流式缓冲

Go channel 的缓冲区大小影响流式输出的平滑度：

```go
// chat_context.go
func NewChatContext(ctx context.Context, sessionID, agentID string) *ChatContext {
    return &ChatContext{
        ctx:       ctx,
        sessionID: sessionID,
        agentID:   agentID,
        events:    make(chan entity.Event, 64), // 缓冲区大小
    }
}
```

**建议：**
- 默认 64 已足够
- 高吞吐场景提高到 256
- 缓冲区太大会增加延迟（批量写入）
- 缓冲区太小会导致 LLM 流式输出阻塞

### Chunk 处理

`handleStreaming` 中，每个 chunk 都会触发一次 `part_update` 事件：

```go
for chunk := range ch {
    if chunk.Content != "" {
        chatCtx.Emit(entity.Event{...}) // 每 chunk 一次
    }
}
```

如果 LLM Provider 返回的 chunk 太小（如每个词一个 chunk），会导致大量小事件。可以通过合并小 chunk 优化：

```go
const minChunkSize = 10 // 至少 10 个字符才发送

type chunkBuffer struct {
    buf     strings.Builder
    lastFlush time.Time
}

func (b *chunkBuffer) Add(text string) (shouldFlush bool) {
    b.buf.WriteString(text)
    return b.buf.Len() >= minChunkSize || time.Since(b.lastFlush) > 50*time.Millisecond
}
```

这可以显著减少网络 I/O 和前端渲染压力。

## 5. pprof 性能分析

### 启用 pprof

```go
import _ "net/http/pprof"

func main() {
    // 启动 pprof HTTP 服务器（生产慎用，仅调试时）
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
    // ...
}
```

### 常用分析命令

```bash
# CPU Profile（30 秒采样）
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 内存 Profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine 分析
go tool pprof http://localhost:6060/debug/pprof/goroutine

# 互斥锁争用
go tool pprof http://localhost:6060/debug/pprof/mutex

# Web UI（火焰图）
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile?seconds=30
```

### 性能基准测试

```bash
cd server

# 运行标准 benchmark
go test ./... -bench=. -benchmem

# 针对性 benchmark
go test ./internal/agent/... -bench=Engine -benchtime=10s -cpuprofile=cpu.prof
```

### 常见性能瓶颈检查

| 检查项 | pprof 命令 | 关注点 |
|--------|-----------|--------|
| 高 CPU | `profile` | LLM 流式处理、JSON 序列化 |
| 内存泄漏 | `heap` | ChatContext 事件通道未关闭 |
| Goroutine 泄漏 | `goroutine` | Agent 循环 goroutine 未退出 |
| 锁竞争 | `mutex` | ToolRegistry、AgentRegistry 的 RWMutex |
| 分配热点 | `allocs` | 频密的 JSON 编解码 |

## 6. 数据库优化

### PostgreSQL 索引

GORM AutoMigrate 不会自动创建性能和查询索引。手动添加：

```sql
-- session_id 索引（消息查询）—— 已有，GORM auto-index
-- 额外建议：
CREATE INDEX idx_messages_created_at ON messages(session_id, created_at);
CREATE INDEX idx_sessions_updated_at ON sessions(updated_at DESC);
CREATE INDEX idx_todos_session_id ON todos(session_id);
```

### 连接池配置

参考 [生产环境部署清单](../10-deployment/production-checklist.md) 第 1 节。

### 读写分离

GORM 支持多数据库源：

```go
dbResolver := dbresolver.Register(dbresolver.Config{
    Sources:  []gorm.Dialector{postgres.Open(sourceDSN)},  // 写
    Replicas: []gorm.Dialector{postgres.Open(replicaDSN)}, // 读
    Policy:   dbresolver.RandomPolicy{},
})

db.Use(dbResolver)
```

> ⚠ 注意：Agent 循环中的读写有严格顺序（先写消息再读取上下文），读写分离可能导致读到过期数据。

## 7. 综合调优清单

| 参数 | 默认值 | 建议范围 | 调优方向 |
|------|--------|---------|---------|
| `concurrency` | 5 | 1-20 | I/O 密集场景提高 |
| SSE event channel buffer | 64 | 64-256 | 高吞吐场景提高 |
| Qdrant batch size | 100 | 100-500 | 数据量大则提高 |
| Token limit (context) | 256000 | 32000-256000 | 按场景设置 |
| DB MaxOpenConns | 25 | 10-50 | 按并发量调整 |
| DB MaxIdleConns | 10 | 5-20 | 保持合理空闲连接 |
| Gin server read timeout | 30s | 10-60s | 平衡安全和长连接 |

## 8. 压力测试

使用 `wrk` 或 `vegeta` 进行压力测试：

```bash
# 创建 Session（非流式）
echo "POST http://localhost:8088/api/sessions" | vegeta attack -rate=100 -duration=30s | vegeta report

# Chat 请求（流式，需自定义测试工具）
# SSE 长连接不适合用通用 HTTP 压测工具
# 建议编写自定义脚本模拟并发 Chat 请求
```

**监控指标：**

- P50 / P95 / P99 响应时间
- 并发连接数
- 内存使用量（RSS）
- Goroutine 数量
- DB 连接数

## 9. 负载均衡（多实例）

CopCon 的无状态设计使其易于横向扩展：

```
                  ┌─────────────────┐
                  │   负载均衡器     │
                  │  (Nginx/ALB)    │
                  └──────┬──────────┘
                         │
          ┌──────────────┼──────────────┐
          ▼              ▼              ▼
   ┌──────────┐   ┌──────────┐   ┌──────────┐
   │ Server 1 │   │ Server 2 │   │ Server 3 │
   └────┬─────┘   └────┬─────┘   └────┬─────┘
        └──────────────┼──────────────┘
                       ▼
              ┌─────────────────┐
              │  PostgreSQL     │
              │  Qdrant         │
              └─────────────────┘
```

**限制：**
- 单个 Session 的 Agent 循环绑定到一台实例（goroutine 本地执行）
- 负载均衡器需做 Session Affinity（sticky session），将同一 Session 路由到同一实例
- 或使用 Redis pub/sub 在不同实例间同步 SSE 事件（较复杂，当前版本未实现）