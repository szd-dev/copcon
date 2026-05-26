# 记忆与知识库系统设计

## 概述

本文档描述 CopCon 的两层记忆与知识库系统。系统通过 **能力包 (Capability Bundle)** 模式为 Agent 提供持久化记忆能力——开启记忆只需一个配置开关,即可一次性注册完整的 hooks、tools 和存储后端。

**第一层: MD 文件记忆** — 结构化的 markdown 文件,定义"Agent 对自己知道什么"。始终注入上下文,人类可读,可版本控制。

**第二层: 向量知识库 (RAG)** — 基于用户管理的文档集合进行语义检索。解决"Agent 能访问哪些具体知识"的问题。

两层均为可选,可按 Agent 独立配置,并在 demo 应用中完整呈现以实现端到端能力演示。

---

## 架构: 当前状态

### 能力系统 (Capability System)

CopCon 的能力系统 (`core/capabilities/`) 支持四种类型:`tool`、`hook`、`skill`、`memory`。能力通过 `init()` 函数注册,可传递性解析依赖关系,并支持通配符展开 (`tools.*`、`hooks.*`、`memory.*`)。

```
harness.go :: Build()
     │
     ├── collectCapabilityNames()
     │     = agents[].tools + builtInTools + builtInHooks
     │
     ├── capabilities.ResolveDependencies()
     │     → 拓扑排序
     │
     ├── 对每个已解析的能力:
     │     ├── type=tool  → ToolCapability.NewTool(deps) → toolRegistry.Register()
     │     └── type=hook  → HookCapability.NewHook(deps) → hookRunner.Register()
     │         特例: "hooks.memory" && MemoryStore==nil → 跳过
     │
     └── AgentFactory 构建 AgentDefinition,带过滤后的 ToolManager
```

### Hook 系统 (`core/hook/`)

10 个生命周期挂钩点。Hook 实现接收 `HookContext`,其中包含指针字段 (`*SystemPrompt`、`*Messages`),允许就地修改引擎的执行管线。

| HookPoint | 触发时机 | 可修改字段 |
|---|---|---|
| `OnSystemPrompt` | 系统提示词解析时 | `*ctx.SystemPrompt` |
| `BeforeContextBuild` | 上下文组装前 | `*ctx.SystemPrompt` |
| `AfterContextBuild` | 上下文组装后、发送 LLM 前 | `*ctx.Messages` |
| `BeforeLLMCall` | 调用 API 前 | `*ctx.Messages` |
| `AfterLLMCall` | 收到 API 响应后 | `*ctx.Messages` |
| `OnMessagePersist` | 消息持久化时 | `*ctx.Messages` |
| `BeforeToolExecute` | 工具调用前 | `*ctx.ToolArgs` |
| `AfterToolExecute` | 工具完成后 | `*ctx.ToolResult` |
| `OnToolError` | 工具执行失败 | `*ctx.ToolResult` |
| `OnSessionResolve` | 会话 ID 解析时 | — |

优先级排序: 数字越大越先执行。默认值 = 100。

### 现有记忆实现

当前 `hooks/memory.go` 为占位实现:
- `AfterContextBuild`: 字节编码嵌入 (非真实语义向量) → `MemoryStore.Search()`
- `OnMessagePersist`: Goroutine 原样存储助手消息
- 无 Agent 主动管理记忆的工具
- 无 MD 文件记忆层
- `StoreProvider` 不包含 `Memory()` — 通过 `StoreConfig.Memory` 单独传入

### 前端技术栈

- **`packages/ui/`**: React 组件库 (Ant Design + TypeScript)。导出: `HumanInteraction`、`TodoList`、`SubagentCard`、`useAgentChat`、`AgentClient`
- **`packages/demo/`**: Vite + React 19 + Ant Design X。三栏聊天布局 (会话侧边栏 | Bubble.List | Todo 侧边栏)
- 无路由库 — 单页应用,使用组件级状态
- 通过 `AgentClient` 类封装 `fetch()` 调用 API

---

## 核心设计原则: 记忆 = 能力包 (Capability Bundle)

记忆不是工具,而是 **能力包**。在 Agent 上启用记忆,会一次性注册完整的组件集合:

```
memory 能力包
├── Hook: FileMemoryHook       → OnSystemPrompt (priority=80)
├── Hook: KBRecallHook         → AfterContextBuild (priority=60)
├── Hook: MemoryPersistHook    → OnMessagePersist (priority=40)
├── Tool: memory_store         → Agent 主动记住事实
├── Tool: memory_recall        → Agent 主动回忆记忆
├── Tool: memory_forget        → Agent 主动遗忘过时记忆
└── Storage: MemoryStore       → 底层持久化
```

### 配置模式 (Configuration Schema)

```yaml
# server/config.yaml
agents:
  - id: assistant
    name: "AI 助手"
    model: gpt-4o
    system_prompt: "你是一个乐于助人的助手。"
    tools: [code_executor, file_ops]
    memory:
      enabled: true
      file_memory: true              # 启用 MD 文件记忆
      vector_memory: true            # 启用向量知识库检索
      knowledge_bases:               # 关联的知识库
        - company_docs
        - product_manuals

knowledge_bases:
  - id: company_docs
    name: "公司文档"
    backend: qdrant
    collection: kb_company_docs
    chunk_size: 800
    chunk_overlap: 100
  - id: product_manuals
    name: "产品手册"
    backend: qdrant
    collection: kb_product_manuals
    chunk_size: 512
    chunk_overlap: 50
```

### Harness 展开逻辑

在 `collectCapabilityNames()` 中,当 `agent.Memory.Enabled == true` 时:

```go
func (h *Harness) collectCapabilityNames() []string {
    // ... 现有逻辑 ...

    for _, spec := range h.config.Agents {
        if spec.Memory.Enabled {
            for _, name := range MemoryBundleNames() {
                add(name)
            }
        }
        // ... 其余现有逻辑 ...
    }
}
```

`MemoryBundleNames()` 返回:

```go
func MemoryBundleNames() []string {
    return []string{
        "hooks.file_memory",
        "hooks.kb_recall",
        "hooks.memory_persist",
        "tools.memory_store",
        "tools.memory_recall",
        "tools.memory_forget",
    }
}
```

现有的守卫逻辑 (`"hooks.memory" && MemoryStore == nil → 跳过`) 被替换为每个 Hook 的独立守卫,检查各自的依赖项。

---

## 第一层: MD 文件记忆

### 目录结构

```
~/.copcon/memory/<agent_id>/
├── INDEX.md              # 始终加载,最大 200 行 / 25KB
├── system/               # 始终注入系统提示词
│   ├── persona.md        # Agent 身份、行为准则
│   └── project.md        # 当前项目事实
├── knowledge/            # 按需加载 (在 INDEX 中可见,相关时加载)
│   ├── decisions.md
│   ├── preferences.md
│   └── pitfalls.md
└── archive/              # 历史记录,极少访问
    └── 2026-05-notes.md
```

### 文件格式

每个 MD 文件使用 YAML frontmatter:

```yaml
---
name: "用户偏好"
description: "用户喜欢简洁的回答,避免代码注释"
type: feedback              # user | feedback | project | reference
created: 2026-05-26
importance: 0.8
---

用户喜欢简洁的回答,不要尾部总结。
除非明确要求,否则不要添加代码注释。
```

### INDEX.md 协议

INDEX.md 是 **索引,而非数据存储**。仅包含单行指针:

```markdown
- [用户偏好](knowledge/preferences.md) — 简洁回答,无注释
- [阿尔法项目](knowledge/decisions.md) — 使用 Go 1.26, PostgreSQL 15
- [测试反馈](knowledge/pitfalls.md) — 集成测试必须使用真实数据库
```

**硬性限制**: 200 行 或 25KB,以先到者为准。Harness 在超限时截断并发出警告。

### FileMemoryHook

```go
type FileMemoryHook struct {
    basePath string           // ~/.copcon/memory/<agent_id>/
    logger   *slog.Logger
}

func (h *FileMemoryHook) Name() string     { return "file_memory" }
func (h *FileMemoryHook) Points() []HookPoint { return []HookPoint{hook.OnSystemPrompt} }
func (h *FileMemoryHook) Priority() int    { return 80 }

func (h *FileMemoryHook) Execute(ctx *HookContext) error {
    // 1. 读取 system/ 目录下所有文件
    // 2. 读取 INDEX.md (截断到限制范围)
    // 3. 追加到 *ctx.SystemPrompt:
    //    "## Agent Memory\n\n### System Context\n<文件内容>\n\n### Memory Index\n<INDEX.md>"
    return nil
}
```

### Agent 操作 MD 文件记忆的工具

`memory_store` 工具在两层都启用时,同时写入 MD 文件和向量存储:

```go
// memory_store 输入 schema
{
  "type": "object",
  "properties": {
    "content": {"type": "string", "description": "要记住的事实"},
    "category": {"type": "string", "enum": ["user", "feedback", "project", "reference"]},
    "name": {"type": "string", "description": "文件的简短描述性名称"},
    "importance": {"type": "number", "minimum": 0, "maximum": 1}
  },
  "required": ["content", "category"]
}
```

执行时:
1. 创建/更新 `<category>/<name>.md`,带 YAML frontmatter
2. 在 INDEX.md 中添加新的指针行
3. 若 vector_memory 启用,嵌入并存入 Qdrant

---

## 第二层: 向量知识库 (RAG)

### 存储接口

```go
// core/storage/knowledge.go

type Document struct {
    ID        string
    KBID      string
    Filename  string
    Source    string              // "upload", "api", "sync"
    Status    DocumentStatus      // pending | parsing | ready | error
    ChunkCount int
    TokenCount int
    CreatedAt time.Time
    UpdatedAt time.Time
    Metadata  map[string]any
}

type DocumentStatus string

const (
    DocStatusPending DocumentStatus = "pending"
    DocStatusParsing DocumentStatus = "parsing"
    DocStatusReady   DocumentStatus = "ready"
    DocStatusError   DocumentStatus = "error"
)

type Chunk struct {
    ID           string
    DocumentID   string
    KBID         string
    Content      string
    Context      string           // 上下文检索前缀
    Index        int              // 在文档中的位置
    TokenCount   int
    Metadata     map[string]any
    Score        float32          // 检索时填充分数
}

type KnowledgeStore interface {
    // 知识库管理
    CreateKB(ctx context.Context, kb *KnowledgeBase) error
    DeleteKB(ctx context.Context, kbID string) error
    ListKBs(ctx context.Context) ([]*KnowledgeBase, error)

    // 文档管理
    IngestDocument(ctx context.Context, kbID string, doc *Document, content []byte) error
    ListDocuments(ctx context.Context, kbID string) ([]*Document, error)
    DeleteDocument(ctx context.Context, kbID string, docID string) error
    GetDocument(ctx context.Context, kbID string, docID string) (*Document, error)

    // 分块管理
    GetChunks(ctx context.Context, docID string) ([]*Chunk, error)
    UpdateChunk(ctx context.Context, chunk *Chunk) error

    // 检索
    Search(ctx context.Context, kbIDs []string, query []float32, opts SearchOptions) ([]*Chunk, error)
}

type SearchOptions struct {
    TopK               int
    SimilarityThreshold float32
    Hybrid              bool     // 稠密 + 稀疏 RRF 融合
    Filters            map[string]any
}

// 嵌入器接口
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
}
```

### StoreProvider 增强

```go
// core/storage/provider.go

type StoreProvider interface {
    Sessions()  SessionStore
    Messages()  MessageStore
    Todos()     TodoStore
    Memory()    MemoryStore       // 新增
}
```

### 增强后的 MemoryStore

```go
// core/storage/memory.go — 增强版

type MemoryType string

const (
    MemoryTypeEpisodic   MemoryType = "episodic"    // 对话轮次
    MemoryTypeSemantic   MemoryType = "semantic"    // 事实、知识
    MemoryTypeProcedural MemoryType = "procedural"  // 习得的模式
)

type Memory struct {
    ID         string
    Content    string
    SessionID  string
    Role       string
    Timestamp  time.Time
    MemoryType MemoryType
    Metadata   map[string]any
    Score      float32

    // 时间字段 (受 Graphiti 启发)
    ValidAt    *time.Time      // 事实在现实中变为真的时间
    InvalidAt  *time.Time      // 事实在现实中不再为真的时间
    Importance float64         // 复合评分权重
}

type MemoryStore interface {
    Store(ctx context.Context, memory *Memory) error
    Search(ctx context.Context, query []float32, limit int) ([]*Memory, error)
    GetBySession(ctx context.Context, sessionID string, limit int) ([]*Memory, error)
    DeleteBySession(ctx context.Context, sessionID string) error
    List(ctx context.Context, filter MemoryFilter) ([]*Memory, error)
    Delete(ctx context.Context, id string) error
}
```

### Qdrant 集合设计

```
Collection: kb_<kb_id>
├── 命名向量 (Named Vectors):
│   ├── "dense" (1024维, COSINE)    — 语义搜索
│   └── "sparse" (稀疏)             — 关键词匹配
│
├── Payload 字段 (在入库前建索引):
│   ├── document_id: keyword
│   ├── chunk_index: integer
│   ├── kb_id: keyword
│   ├── created_at: integer (unix 时间戳)
│   ├── doc_type: keyword
│   └── chunk_context: text
│
└── 通过 Query API 进行混合检索:
    prefetch: [dense(top-100), sparse(top-100)]
    fusion: RRF (倒数排名融合)
    limit: top_k
```

### 入库管线 (Ingestion Pipeline)

```
文档上传
    │
    ▼
[解析器] → 从 PDF/MD/TXT/HTML 中提取文本
    │
    ▼
[递归分块器] → chunk_size=800, overlap=100, markdown 感知
    │
    ▼
[嵌入器] → 稠密向量 (1024维) + 稀疏向量
    │
    ▼
[Qdrant Upsert] → 批量写入,稳定 ID 基于 doc_id + chunk_index
```

### 检索管线 (KBRecallHook)

```
最后一条用户消息
    │
    ▼
[嵌入器] → 查询向量
    │
    ▼
[Qdrant 混合检索] → 稠密 + 稀疏 RRF 融合 → top-K
    │
    ▼
[格式化结果] → 以系统消息注入 *ctx.Messages:
    "## 检索知识
     - [文档: refund_v2.pdf §3] (分数: 0.92) 退款政策规定...
     - [文档: faq/refund.md] (分数: 0.78) 退货必须包含..."
```

### Agent 操作知识库的工具

```go
// memory_recall: 跨知识库的语义搜索
{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "检索内容"},
    "limit": {"type": "integer", "default": 5, "maximum": 20}
  },
  "required": ["query"]
}

// memory_forget: 移除过时记忆
{
  "type": "object",
  "properties": {
    "id": {"type": "string", "description": "要遗忘的记忆 ID"},
    "query": {"type": "string", "description": "搜索要遗忘的记忆"}
  }
}
```

---

## 服务端 API

### 新增端点

```
POST   /api/kb                    # 创建知识库
GET    /api/kb                    # 列出知识库
DELETE /api/kb/:kbId              # 删除知识库

GET    /api/kb/:kbId/docs         # 列出知识库中的文档
POST   /api/kb/:kbId/docs         # 上传文档 (multipart/form-data)
GET    /api/kb/:kbId/docs/:docId  # 获取文档详情 (含分块)
DELETE /api/kb/:kbId/docs/:docId  # 删除文档

POST   /api/kb/:kbId/search       # 检索测试
       body: {"query": "...", "top_k": 5}
       response: {"chunks": [{"content": "...", "score": 0.92, "doc": "..."}]}

GET    /api/sessions/:sessionId/memories     # 列出会话的 Agent 记忆
DELETE /api/sessions/:sessionId/memories/:id # 删除特定记忆
```

### Handler 增强

```go
type Handler struct {
    config         *config.Config
    sessionStore   storage.SessionStore
    messageStore   storage.MessageStore
    todoStore      storage.TodoStore
    memoryStore    storage.MemoryStore       // 新增
    knowledgeStore storage.KnowledgeStore    // 新增
    agent          agent.AgentEngine
    agentRegistry  agent.AgentRegistry
    chatStore      chat.ActiveSessions
}
```

---

## Hooks 数据流

### 单条消息的完整生命周期

```
用户: "我们的退款政策是什么?"
  │
  ▼ HookPoint: OnSystemPrompt (priority=80)
  FileMemoryHook 读取 system/persona.md + INDEX.md
  → 追加到 *ctx.SystemPrompt:
    "## Agent Memory
     ### System Context
     你正在处理客户支持项目。
     ### Memory Index
     - [退款规则](knowledge/refund.md) — 更新的 Q2 政策
     - [用户偏好](knowledge/preferences.md) — 简洁回答"

  │
  ▼ HookPoint: AfterContextBuild (priority=60)
  KBRecallHook: embed("我们的退款政策是什么?")
  → 在已配置的 knowledge_bases 中执行 Qdrant 混合检索
  → 在 *ctx.Messages 前插入系统消息:
    "## 检索知识
     - [refund_v2.pdf §3] (0.92) 7天内可全额退款...
     - [faq/refund.md] (0.78) 退货需凭原始收据..."

  │
  ▼ LLM 调用 (使用富化后的上下文)
  Agent 基于检索到的知识生成可靠的回答

  │
  ▼ HookPoint: OnMessagePersist (priority=40)
  MemoryPersistHook (在 goroutine 中):
  → 从助手回复中提取关键事实
  → 嵌入并存入 Qdrant 记忆集合
  → 酌情更新 MD 文件记忆 (若足够重要)
```

---

## Demo 应用

### 布局重构

顶层通过 Ant Design `Tabs` 组件导航:

```
┌──────────────────────────────────────────────────────┐
│  CopCon Demo            [ 聊天 ]  [ 知识库 ]         │
├──────────────────────────────────────────────────────┤
│                                                      │
│  聊天 Tab: 现有三栏布局 (不变)                        │
│    会话列表 | 聊天区 | Todo + 记忆 侧边栏             │
│                                                      │
│  知识库 Tab: 新的管理 UI                              │
│    知识库列表 | 知识库详情 (文档 + 检索测试)           │
│                                                      │
└──────────────────────────────────────────────────────┘
```

### 聊天 Tab 增强

在右侧侧边栏 (与 Todos 并列) 增加可折叠的 **记忆面板**:

```
┌── 右侧边栏 ─────────────┐
│ 任务                      │
│ ├─ [x] 回答用户            │
│ └─ [ ] 跟进                │
│                           │
│ 记忆             [▾]      │
│ ├─ 📁 persona.md          │
│ ├─ 📁 project.md          │
│ ├─ 💡 用户喜欢...          │
│ └─ 💡 项目使用 Go...       │
└───────────────────────────┘
```

### 知识库 Tab

使用 Ant Design 组件的双栏布局:

```
┌── 知识库列表 ─┐  ┌── 知识库详情 ────────────────────────┐
│ 📁 company    │  │                                       │
│ 📁 product    │  │  文档                                 │
│ 📁 faq        │  │  ┌─────────────────────────────────┐  │
│               │  │  │ refund_v2.pdf   ✅ 142 个分块    │  │
│ [+ 新建知识库]│  │  │ api_docs.md     ✅ 89 个分块     │  │
│               │  │  │ draft.txt       🔄 解析中...     │  │
│               │  │  └─────────────────────────────────┘  │
│               │  │                                       │
│               │  │  [上传文档]  [测试检索]                │
│               │  │                                       │
│               │  │  检索测试                              │
│               │  │  ┌───────────────────────────────┐    │
│               │  │  │ 查询: [退款政策          🔍]  │    │
│               │  │  │ 结果:                         │    │
│               │  │  │ ├ refund_v2.pdf §3  (0.92)    │    │
│               │  │  │ └ faq/refund.md     (0.78)    │    │
│               │  │  └───────────────────────────────┘    │
└───────────────┘  └───────────────────────────────────────┘
```

### 新增组件

```
packages/ui/src/
├── components/
│   ├── KnowledgeBase/
│   │   ├── KBList.tsx              # 知识库卡片列表
│   │   ├── KBDetail.tsx            # 文档表格 + 状态
│   │   ├── KBUpload.tsx            # 拖拽式文档上传
│   │   ├── KBRetrievalTest.tsx     # 查询输入框 + 结果面板
│   │   └── ChunkViewer.tsx         # 分块预览 (点击文档 → 查看分块)
│   └── MemoryPanel/
│       └── MemoryPanel.tsx         # 聊天侧边栏的记忆文件列表
├── api/
│   ├── agentClient.ts              # + listKB(), uploadDoc(), testRetrieval()
│   └── types.ts                    # + KnowledgeBase, Document, Chunk, Memory 类型
└── index.ts                        # + 导出新组件

packages/demo/src/
├── App.tsx                         # 重构: Tabs 包装
├── pages/
│   ├── ChatPage.tsx                # 从现有 App.tsx 抽取
│   └── KnowledgePage.tsx           # 知识库管理页
└── App.css                         # Tab 布局样式
```

### AgentClient API 扩展

```typescript
// AgentClient 新增方法
class AgentClient {
  // ... 现有方法 ...

  // 知识库
  listKnowledgeBases(): Promise<{ kbs: KnowledgeBase[] }>;
  createKnowledgeBase(name: string): Promise<KnowledgeBase>;
  deleteKnowledgeBase(kbId: string): Promise<void>;

  listDocuments(kbId: string): Promise<{ documents: Document[] }>;
  uploadDocument(kbId: string, file: File): Promise<Document>;
  deleteDocument(kbId: string, docId: string): Promise<void>;
  getDocumentChunks(kbId: string, docId: string): Promise<{ chunks: Chunk[] }>;

  testRetrieval(kbId: string, query: string, topK?: number): Promise<{ chunks: Chunk[] }>;

  // 记忆
  getSessionMemories(sessionId: string): Promise<{ memories: Memory[] }>;
  deleteSessionMemory(sessionId: string, memoryId: string): Promise<void>;
}
```

---

## 嵌入策略

### 主要方案: OpenAI Embedding (阶段 0-2)

复用现有 `LLMProvider`,使用 `text-embedding-3-small`:
- 1536 维, $0.02/百万 tokens
- 零额外基础设施
- 开发与初期部署的质量足够

### 升级路径: BGE-M3 Sidecar (阶段 3+)

用于生产环境自托管:
- 以 Python sidecar 形式部署 BGE-M3 (ONNX Runtime + gRPC/HTTP)
- 一个模型同时产出 1024维稠密向量 + 稀疏向量 + ColBERT
- 添加到 `docker-compose.yml`
- 通过配置切换: `embedding.backend: openai | bge_m3`

```yaml
# config.yaml
embedding:
  backend: openai           # 或 bge_m3
  openai:
    model: text-embedding-3-small
  bge_m3:
    endpoint: http://localhost:8080/embed
```

---

## 评估策略

### 第一级: Go 原生检索指标 (每个 PR)

```go
// core/eval/retrieval.go
type RetrievalTestCase struct {
    Query          string
    RelevantDocIDs []string
}

type RetrievalResult struct {
    RecallAtK    map[int]float64    // K → 召回率
    PrecisionAtK map[int]float64   // K → 精确率
    MRR          float64
    NDCGAtK      map[int]float64
    HitRateAtK   map[int]float64
}

func EvaluateRetrieval(
    testCases []RetrievalTestCase,
    retriever func(query string, k int) []string,
    ks []int,
) RetrievalResult
```

- 零依赖,零 LLM 调用
- 黄金测试集: `eval/testdata/golden_set.jsonl` (50-100 条,版本控制)
- 质量门禁: Recall@5 ≥ 0.80, MRR ≥ 0.75

### 第二级: 检索测试面板 (持续)

Demo 的知识库检索测试面板同时作为轻量级评估工具:
- 运营者测试查询并查看带分数的结果
- 标记结果相关/不相关 → 反馈回流到黄金测试集
- 这就是"评估 UI" — 无需独立平台

### 第三级: LLM 作为裁判 (每日夜间,延期)

阶段 3+,将 Ragas 容器化为 Python sidecar:
- 忠实度 (目标 ≥0.90) — 陈述是否由检索到的上下文支持?
- 答案相关性 (目标 ≥0.80) — 回答是否针对了查询?
- 每晚针对 400+ 测试用例运行

---

## 实施阶段

### 阶段 0: 基础 (3 天)

| 变更 | 文件 | 说明 |
|---|---|---|
| 增强 MemoryStore | `core/storage/memory.go` | 添加 `List`、`Delete(id)`、`Update`,时间字段 |
| KnowledgeStore 接口 | `core/storage/knowledge.go` | 文档级 CRUD + 语义搜索 |
| StoreProvider.Memory() | `core/storage/provider.go` | 添加 `Memory() MemoryStore` |
| Embedder 接口 | `core/providers/embedding/embedder.go` | `Embed(text) → []float32` |
| OpenAI embedding 实现 | `core/providers/embedding/openai.go` | 复用现有 LLM provider |
| AgentSpec.Memory | `core/agent/config.go` | 添加 `Memory MemoryConfig` |
| 配置结构体 | `server/internal/config/config.go` | `MemoryConfig`, `KnowledgeBaseConfig` |

### 阶段 1: 记忆 Hooks (1 周)

| 变更 | 文件 | Hook |
|---|---|---|
| FileMemoryHook | `core/capabilities/hooks/file_memory.go` | `OnSystemPrompt` — 注入 MD 文件 |
| KBRecallHook | `core/capabilities/hooks/kb_recall.go` | `AfterContextBuild` — 向量检索 |
| MemoryPersistHook | `core/capabilities/hooks/memory_persist.go` | `OnMessagePersist` — 异步存储 |
| 能力包定义 | `core/capabilities/bundle.go` | `MemoryBundleNames()` |
| Harness 展开逻辑 | `core/harness.go` | Memory 配置 → 能力名称 |

### 阶段 2: 记忆 Tools (3 天)

| 变更 | 文件 | Tool |
|---|---|---|
| memory_store | `core/capabilities/tools/memory_store.go` | Agent 存储一个事实 |
| memory_recall | `core/capabilities/tools/memory_recall.go` | Agent 检索记忆 |
| memory_forget | `core/capabilities/tools/memory_forget.go` | Agent 遗忘过时记忆 |

### 阶段 3: 知识库后端 (1 周)

| 变更 | 文件 | 说明 |
|---|---|---|
| KB handler | `server/internal/api/knowledge.go` | 知识库 CRUD、上传、检索测试 |
| 路由注册 | `server/internal/api/handlers.go` | `/api/kb/*` 路由组 |
| Qdrant KB 实现 | `core/providers/qdrant/knowledge.go` | Qdrant 的 KnowledgeStore |
| 替换占位嵌入 | `core/providers/qdrant/store.go` | 真正的 Embedder 替换字节编码 |

### 阶段 4: Demo UI (1.5 周)

| 变更 | 文件 | 说明 |
|---|---|---|
| Tab 导航 | `demo/src/App.tsx` | 顶层 Tabs: 聊天 / 知识库 |
| ChatPage 抽取 | `demo/src/pages/ChatPage.tsx` | 抽取现有聊天逻辑 |
| KnowledgePage | `demo/src/pages/KnowledgePage.tsx` | 知识库管理页 |
| KBList 组件 | `ui/src/components/KnowledgeBase/KBList.tsx` | 知识库卡片列表 |
| KBDetail 组件 | `ui/src/components/KnowledgeBase/KBDetail.tsx` | 文档表格 + 状态 |
| KBUpload 组件 | `ui/src/components/KnowledgeBase/KBUpload.tsx` | 拖拽上传 |
| KBRetrievalTest | `ui/src/components/KnowledgeBase/KBRetrievalTest.tsx` | 搜索测试面板 |
| ChunkViewer | `ui/src/components/KnowledgeBase/ChunkViewer.tsx` | 分块预览 |
| MemoryPanel | `ui/src/components/MemoryPanel/MemoryPanel.tsx` | 侧边栏记忆列表 |
| AgentClient 扩展 | `ui/src/api/agentClient.ts` | 知识库 + 记忆 API 方法 |
| Types 扩展 | `ui/src/api/types.ts` | 知识库/记忆类型定义 |

### 阶段 5: 评估 (3 天)

| 变更 | 文件 | 说明 |
|---|---|---|
| 检索评估 | `core/eval/retrieval.go` | Recall@K, Precision@K, MRR, nDCG |
| 黄金测试集 | `eval/testdata/golden_set.jsonl` | 初始 50 条 |
| CI 集成 | `.github/workflows/eval.yml` | PR 门禁 |

### 并行执行

```
阶段 0 (基础)
    │
    ├── 阶段 1 (hooks) ─── 阶段 2 (tools)
    │
    └── 阶段 3 (知识库后端)
              │
              └── 阶段 4 (UI)
                        │
                        └── 阶段 5 (评估,可与阶段 4 重叠)
```

阶段 1 和阶段 3 可在阶段 0 完成后并行执行。

### 预估时间线

| 场景 | 预估时长 |
|---|---|
| 单人开发 | ~5-6 周 |
| 双人开发 (后端 + 前端并行) | ~3-4 周 |

---

## 附录: 行业参考

### MD 文件记忆模式

| 系统 | 模式 | 关键洞见 |
|---|---|---|
| **Letta (MemGPT) MemFS** | Git 支持的 MD 文件,`system/` 始终加载 + 按需加载 | Agent 通过 bash 工具自管理记忆;子代理使用 git worktree |
| **Claude Code MEMORY.md** | 索引文件 (200 行上限) + 主题文件 | 索引是索引,非数据存储;两步写入协议 |
| **OpenAI Codex** | 两阶段管线:每个线程抽取 → 全局合并 | 最可靠但最复杂;SQLite 管理状态 |
| **Cursor .cursor/rules/** | YAML frontmatter + 通配符范围激活 | 静态规则,无动态自编辑 |

### RAG 架构 (2026 生产栈)

| 组件 | 最佳实践 |
|---|---|
| **分块** | 递归字符分块,800 tokens,100 overlap,markdown 感知 |
| **嵌入** | BGE-M3 (自托管) 或 OpenAI text-embedding-3-small (API) |
| **检索** | 混合: 稠密 + 稀疏,通过 RRF 融合 |
| **重排** | Cross-encoder 重排 top-50 → top-5 |
| **上下文检索** | 每个分块前置 50-100 token 上下文 (减少 35-67% 检索失败) |

### 评估方法

| 工具 | 用途 | 集成方式 |
|---|---|---|
| **Go 原生指标** | Recall@K, MRR, nDCG — 零成本,确定性 | 每个 PR 跑 `go test` |
| **Ragas** (Python sidecar) | 忠实度、答案相关性 | 每日夜间 CI |
| **Langfuse** (自托管) | 生产可观测性,通过 OTel | 持续监控 |
| **检索测试面板** | 轻量级人工评估 | Demo UI,反馈 → 黄金测试集 |
