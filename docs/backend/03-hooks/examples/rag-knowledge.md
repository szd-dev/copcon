# 示例：RAG 知识注入

## 场景

你有一套向量数据库存储了项目文档、历史对话或领域知识。当用户提问时，你希望检索相关文档，将其作为附加上下文注入到 LLM 的消息列表中，帮助 LLM 给出更准确的回答。

这是典型的 RAG（Retrieval-Augmented Generation）模式。

## 为什么选择 AfterContextBuild

消息列表在 `AfterContextBuild` 时已经组装完成，包含系统提示词和历史对话。此时注入知识点笺合适：你既能看到用户最新消息（用作检索查询），又能在 LLM 调用前插入结果。

## 优先级考虑

知识注入属于上下文增强层，需要在业务逻辑 Hook 之前执行。设为 60，在默认 100 以下但在 20 之上，保证知识融入后在敏感词过滤等逻辑之前完成。

## 完整代码

```go
package rag

import (
    "fmt"
    "log/slog"

    "github.com/copcon/server/internal/domain/entity"
    "github.com/copcon/server/internal/hook"
)

// VectorDB 是向量数据库的抽象接口。
// 你可以替换为 Qdrant、Milvus、Weaviate 等任意实现。
type VectorDB interface {
    // Search 根据查询文本向量搜索最相关的文档。
    // query 是嵌入后的向量，topK 是返回数量。
    Search(query []float32, topK int) ([]Document, error)
}

// Document 是检索返回的文档。
type Document struct {
    Content string
    Score   float64
    Source  string
}

// Embedder 是将文本转换为向量的抽象接口。
type Embedder interface {
    // Embed 将文本转换为向量。
    Embed(text string) ([]float32, error)
}

// RAGKnowledgeHook 在 AfterContextBuild 时检索向量数据库，
// 将相关文档注入为 system 消息。
type RAGKnowledgeHook struct {
    db       VectorDB
    embedder Embedder
    topK     int
    logger   *slog.Logger
}

// NewRAGKnowledgeHook 创建一个 RAG 知识注入 Hook。
// db 可以为 nil —— 这种情况下 Hook 静默跳过所有操作（优雅降级）。
func NewRAGKnowledgeHook(db VectorDB, embedder Embedder, topK int) *RAGKnowledgeHook {
    if topK <= 0 {
        topK = 5
    }
    return &RAGKnowledgeHook{
        db:       db,
        embedder: embedder,
        topK:     topK,
        logger:    slog.Default(),
    }
}

// Name 返回 Hook 标识符。
func (h *RAGKnowledgeHook) Name() string {
    return "rag_knowledge_injection"
}

// Points 返回 AfterContextBuild，在消息列表构建完成时执行。
func (h *RAGKnowledgeHook) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.AfterContextBuild}
}

// Priority 返回 60。低于系统默认值 100，高于业务逻辑 20。
func (h *RAGKnowledgeHook) Priority() int {
    return 60
}

// Execute 执行检索和注入逻辑。
func (h *RAGKnowledgeHook) Execute(ctx *hook.HookContext) error {
    // 优雅降级：如果没有配置向量数据库，直接跳过
    if h.db == nil {
        return nil
    }

    if ctx.Messages == nil || len(*ctx.Messages) == 0 {
        return nil
    }

    // 找到最后一条用户消息作为检索查询
    query := h.findLastUserMessage(*ctx.Messages)
    if query == "" {
        return nil
    }

    // 将查询文本嵌入为向量
    vector, err := h.embedder.Embed(query)
    if err != nil {
        h.logger.Warn("failed to embed query",
            "session_id", ctx.SessionID,
            "error", err,
        )
        return nil // 嵌入失败不影响主流程
    }

    // 检索相关文档
    docs, err := h.db.Search(vector, h.topK)
    if err != nil {
        h.logger.Warn("vector search failed",
            "session_id", ctx.SessionID,
            "error", err,
        )
        return nil // 检索失败不影响主流程 —— 优雅降级
    }

    if len(docs) == 0 {
        return nil
    }

    // 将检索结果格式化为系统消息
    systemMsg := entity.MessageForLLM{
        Role:    "system",
        Content: h.formatContext(docs, query),
    }

    // 将知识文档放在消息列表最前面
    *ctx.Messages = append(
        []entity.MessageForLLM{systemMsg},
        *ctx.Messages...,
    )

    h.logger.Info("knowledge injected into context",
        "session_id", ctx.SessionID,
        "query_len", len(query),
        "docs_found", len(docs),
    )

    return nil
}

// findLastUserMessage 从消息列表中反向扫描，返回最后一条用户消息的内容。
func (h *RAGKnowledgeHook) findLastUserMessage(messages []entity.MessageForLLM) string {
    for i := len(messages) - 1; i >= 0; i-- {
        if messages[i].Role == "user" && messages[i].Content != "" {
            return messages[i].Content
        }
    }
    return ""
}

// formatContext 将检索到的文档格式化为系统提示词。
func (h *RAGKnowledgeHook) formatContext(docs []Document, query string) string {
    content := fmt.Sprintf(
        "以下是从知识库中检索到的与当前问题相关的参考文档：\n\n"+
        "用户问题：%s\n\n"+
        "参考文档：\n",
        query,
    )

    for i, doc := range docs {
        content += fmt.Sprintf(
            "\n--- 文档 %d (相关度: %.2f, 来源: %s) ---\n%s\n",
            i+1, doc.Score, doc.Source, doc.Content,
        )
    }

    content += "\n---\n请基于以上参考资料回答用户问题。" +
        "如果参考资料不足以回答问题，请根据你自身的知识回答，并说明信息来源。"

    return content
}
```

## 注册代码

```go
package main

import (
    "github.com/copcon/server/internal/hook"
    "your-project/rag"
    "your-project/vectordb"
)

func main() {
    runner := hook.NewHookRunner()

    // 初始化向量数据库（这里以 Qdrant 为例）
    db := vectordb.NewQdrantClient(
        vectordb.WithHost("localhost"),
        vectordb.WithPort(6333),
        vectordb.WithCollection("knowledge_base"),
    )

    // 初始化嵌入服务
    embedder := vectordb.NewOpenAIEmbedder(
        "your-api-key",
        "text-embedding-3-small",
    )

    // 创建 RAG Hook（topK=5，返回最相关的 5 篇文档）
    ragHook := rag.NewRAGKnowledgeHook(db, embedder, 5)

    runner.Register(ragHook)

    // 将 runner 传入引擎
    // engine := agent.NewEngine(agent.WithHookRunner(runner), ...)
}
```

## 执行流程说明

1. 用户发送消息："请解释 Go 语言的 goroutine 机制"
2. 引擎构建上下文 → 触发 `AfterContextBuild`
3. `RAGKnowledgeHook.Execute` 被调用
4. 找到最后一条用户消息："请解释 Go 语言的 goroutine 机制"
5. 调用 `embedder.Embed()` 将查询转为向量 `[0.12, 0.34, ...]`
6. 调用 `db.Search()` 检索最相关的 5 篇文档
7. 格式化文档为 system 消息并插入到消息列表最前面
8. LLM 收到的消息顺序是：`[知识文档, 系统提示词, 历史消息, 用户问题]`

## 优雅降级设计

这个 Hook 在多个环节实现了优雅降级：

| 环节 | 失败时行为 |
|------|-----------|
| 数据库未配置（`db == nil`） | 直接返回，不影响请求 |
| 嵌入服务失败 | 记录 Warn 日志，正常返回 |
| 向量检索失败 | 记录 Warn 日志，正常返回 |
| 检索结果为空 | 正常返回，不注入任何内容 |

**为什么这么设计？** LLM 调用不应该因为外围的知识检索失败而中断。即使拿不到参考文档，LLM 依然可以依靠自身知识回答问题。优雅降级保证了核心功能的可用性。

## 对比：与 MemoryPlugin 的差异

| 维度 | RAGKnowledgeHook | MemoryPlugin |
|------|-----------------|-------------|
| 数据来源 | 外部向量数据库 | 内部对话记忆 |
| 检索范围 | 项目文档、知识库 | 历史对话 |
| 注入时机 | 只读检索 | 检索 + 存储 |
| HookPoint | `AfterContextBuild` | `AfterContextBuild` + `OnMessagePersist` |

两者可以同时使用，互不冲突。MemoryPlugin 负责"记住对话"，RAGKnowledgeHook 负责"检索外部知识"。它们的 system message 会按照优先级依次插入消息列表。由于 RAGKnowledgeHook 的优先级（60）低于 MemoryPlugin（100），知识文档会在 MemoryPlugin 的状态消息之后插入，但这不影响功能。

## 扩展建议

- 支持多轮查询改写：先用 LLM 把用户问题改写为更适合检索的查询
- 添加缓存层：相同查询的结果缓存一段时间
- 按文档长度截断：长文档做摘要后再注入
- 使用 `ctx.ChatCtx.Context()` 控制嵌入和检索的超时