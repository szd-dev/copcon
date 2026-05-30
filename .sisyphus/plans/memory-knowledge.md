# 记忆与知识库系统实施计划 (Memory & Knowledge Base)

## TL;DR

> **Quick Summary**: 为 CopCon AI Agent 框架实现双层记忆系统——MD 文件记忆(`memory` bundle)与向量知识库(`knowledge_base` bundle)。两个能力完全解耦,通过配置独立启用。知识库后端采用 **sqlite-vec(纯 Go)** 作为默认实现,保留 `KnowledgeStore` 可插拔接口以供未来扩展 Qdrant/pgvector。Demo 应用通过 Ant Design 6 提供生产级可用 UI。
>
> **Deliverables**:
> - `core/storage/knowledge.go` + `core/storage/knowledge_registry.go` — 可插拔知识库存储接口与注册机制
> - `core/storage/memory.go` 增强 + `StoreProvider.Memory()` / `Knowledge()` — 存储接口统一入口
> - `core/providers/embedding/` — OpenAI Embedder (复用现有 LLM provider)
> - `core/providers/filememory/` — MD 文件记忆实现(system/knowledge/archive 三层 + INDEX.md)
> - `core/providers/sqlitevec/` — sqlite-vec KnowledgeStore(纯 Go, zero external deps)
> - `core/rag/` — 解析器(pdf/md/txt/html)+ 分块器(Recursive/Markdown)+ 入库管线
> - `core/capabilities/hooks/file_memory.go` — `OnSystemPrompt` MD 注入
> - `core/capabilities/hooks/kb_recall.go` — `AfterContextBuild` 向量检索
> - `core/capabilities/hooks/memory_persist.go` — `OnMessagePersist` 异步事实提取(纯关键词,无 LLM)
> - `core/capabilities/tools/memory_{store,recall,forget}.go` — Agent 主动存/忆/忘 3 个 Tool
> - `server/internal/api/knowledge.go` + 记忆端点 — REST API (KB 管理 + 检索测试 + 会话记忆)
> - `@copcon/chat-core` types + AgentClient 扩展 — 前端对接 10+ 新方法
> - `@copcon/demo` 重构 — Tabs 布局 + KB 列表/详情/上传/检索/分块预览 + 记忆管理页
> - `core/eval/retrieval.go` + golden set — 检索指标(Recall@K, MRR, nDCG)+ CI 门禁
>
> **Estimated Effort**: 33-35 天关键路径(单人);AI 并行开发约 8.5 天 wall-clock + review/iteration 至 10-12 天
> **Parallel Execution**: YES — 5 个 Wave,最大并行度 3
> **Critical Path**: W1 → W3 (sqlite-vec KnowledgeStore) → W4 → W5 → W6 (Demo UI)

---

## Context

### Original Request

> "现在我想在这个项目中实现一个记忆和知识库能力,这包括两层:基于 md 文件的记忆和索引,以及基于向量存储的知识库。前者决定了:我知道什么,后者决定了:具体的知识存储。"

### Interview Summary

**Key Discussions**:
- 记忆是**能力(Capability Bundle)**而非工具——开启记忆 = 一次性注册 hooks + tools + storage;不能以 tool 形式暴露
- 必须支持**多种配置方式**动态启用 agent/tools/记忆,类似配置驱动而非 runtime plugin 接口
- UI 体系已重构为 `packages/{chat-core, chat-react, headless-hooks, demo}` 四层架构,新 UI 必须基于此体系(不再使用旧 `packages/ui/`)
- Demo 应用需要达到**生产可用级别**(错误/加载/空态完整、表单校验、主题一致、可访问性),**不做响应式**(响应式推迟)
- 拆分为**两个独立能力**: `memory` (MD 文件) 与 `knowledge_base` (RAG),可独立启用
- 知识库后端本期**仅做 sqlite-vec**(零外部依赖),**不做 Qdrant/pgvector**,但保留可插拔接口供未来扩展
- **MemoryPersistHook 采用简单关键词提取**(正则 + 停用词),不调用 LLM——超出本次范围,未来可单独迭代
- **Contextual Retrieval 推迟**(避免 LLM 调用)
- UI 可访问性保留,响应式推迟

**Research Findings**:
- Letta MemFS / Claude Code MEMORY.md / OpenAI Codex 采用 Index + On-Demand 模式,INDEX.md 200行/25KB 是事实标准
- 2026 RAG 生产栈:混合检索(dense+sparse RRF)+ Contextual Retrieval(推迟)+ Cross-Encoder Reranking
- BGE-M3 是自托管首选,但目前 OpenAI Embedding 复用现有 provider 足矣,推迟 BGE-M3
- GraphRAG / Graphiti 适合关系推理,本期不做,仅保留接口
- sqlite-vec + ncruces/go-sqlite3 提供纯 Go 向量存储,符合"最低依赖"原则

### Metis Review

**Identified Gaps (addressed)**:
- 早期方案把 memory 当 tool,现改为 capability bundle
- 早期方案 UI 在旧 `packages/ui/`,现对齐新架构 `chat-core` + `chat-react` + `headless-hooks` + `demo`
- 早期方案硬编码 Qdrant,现改为可插拔 `KnowledgeStore` 接口 + Provider 注册机制
- MemoryPersistHook 早期可能引入 LLM,现确认为纯关键词提取
- 增加 W1.7 KnowledgeStore registry 工作项,实现配置驱动 backend 切换
- Bundle 拆分为两个独立(Memory + KnowledgeBase),避免强耦合

---

## Work Objectives

### Core Objective

为 CopCon 实现**配置驱动的双层记忆+知识库系统**,让 Agent 具备跨会话的持久记忆能力与基于外部文档的语义检索能力,同时提供生产级可用的 Demo UI 进行端到端能力演示。

### Concrete Deliverables

1. **MD 文件记忆能力包**(`memory` bundle): FileMemoryHook + 3 个工具 + 文件存储实现
2. **向量知识库能力包**(`knowledge_base` bundle): KBRecallHook + MemoryPersistHook + sqlite-vec 实现 + RAG 管线
3. **可插拔 `KnowledgeStore` 接口**: 仿照 `StoreProvider` 的 Provider 模式,通过 `RegisterKnowledgeStoreProvider()` 注册实现,本期 sqlite-vec,未来可插 Qdrant/pgvector
4. **Server API**: KB 管理 + 检索测试 + 会话记忆;RESTful + 完整错误处理
5. **`@copcon/chat-core` 扩展**: 类型定义 + AgentClient 10+ 新方法,与后端对齐
6. **Demo UI**(生产级): Tabs 布局重构 + KB 列表/详情/上传/检索测试/分块预览 + 记忆管理页;Ant Design 6 + 可访问性
7. **评估框架**: Go-native 检索指标(Recall@K, MRR, nDCG)+ 黄金测试集 + CI 门禁

### Definition of Done

- [ ] `cd core && go build ./...` 成功
- [ ] `cd core && go test ./...` 全部通过(包含 filememory/rag/sqlitevec/eval 测试)
- [ ] `cd server && go build ./...` 成功
- [ ] `cd server && go test ./internal/...` 全部通过(包含 knowledge API 测试)
- [ ] `pnpm --filter @copcon/* build` 成功(chat-core/chat-react/headless-hooks/demo)
- [ ] `pnpm --filter @copcon/chat-core test` 全部通过
- [ ] Demo UI 可启动 + 完整走通: 创建 KB → 上传文档 → 检索测试 → 创建会话 → 聊天中记忆/知识库生效 → 查看记忆;全程可访问
- [ ] `go test ./core/eval/...` 在 CI 中通过质量门禁(Recall@5 ≥ 0.80, MRR ≥ 0.75)

### Must Have

- ✅ MD 文件记忆完整可用: `system/` 始终注入 + INDEX.md(200行/25KB 硬限) + 3 个工具
- ✅ sqlite-vec KnowledgeStore 完整可用 + `KnowledgeStore` 可插拔接口(为未来 backend 留路)
- ✅ KB/文档管理 REST API 完整(CRUD + multipart 上传 + 检索测试)
- ✅ Parser 支持 pdf/md/txt/html 四种文档类型
- ✅ 入库管线: Parse → Chunk → Embed → Store(本期无 Contextual Augmentation)
- ✅ 三个 hook 生效: FileMemoryHook / KBRecallHook / MemoryPersistHook(纯关键词提取,无 LLM)
- ✅ Demo UI Tabs 布局(Chat / KB / Memory)+ Chat 侧边栏嵌入精简版记忆面板
- ✅ `@copcon/chat-core` types 与 Go 后端严格对齐,Ant Design 6 主题一致
- ✅ 所有新组件通过可访问性校验(ARIA、键盘导航、屏幕阅读器)
- ✅ 评估框架在 CI 中运行,失败阻断合并

### Must NOT Have (Guardrails)

- ❌ **不能**把 3 个 memory tool 作为独立 ToolCapability 散列注册——必须通过 `MemoryBundleNames()` 整体启用
- ❌ **不能**在新代码中硬编码 Qdrant 依赖,所有向量存储必须通过 `KnowledgeStore` 接口
- ❌ **不能**在 Demo UI 中引用已废弃的 `@copcon/ui` 包或旧的 `AgentClient`
- ❌ **不能**在新代码中引入多 LLM 管理模块(Contextual Retrieval/MemoryPersistHook 都不能依赖 LLM)
- ❌ **不能**破坏现有的 `StoreProvider` 兼容性(新增方法不能使现有实现编译失败)
- ❌ **不能**在 memory hook 中直接调用 Qdrant——必须通过 `KnowledgeStore`/`MemoryStore` 接口
- ❌ **不能在 UI 中做响应式**(xs/sm/md/lg 等断点适配推迟)
- ❌ **不能**在 core/ 模块中 import server/ 任何代码(模块边界)
- ❌ **不能**让 `MemoryStore` 在 `StoreProvider` 中强制非 nil——保留 `Memory() MemoryStore` 可选返回 nil 的语义(与现有 Todos 一致)
- ❌ **不做 GraphRAG / 时间图谱 / 多 LLM 管理**——这些超出本次范围,只留接口不给实现
- ❌ **不做 BGE-M3 sidecar 部署**——本期用 OpenAI Embedding,延迟 BGE-M3 至后续任务

---

## Verification Strategy

### Test Decision

- ** Infrastructure exists**: YES (`cd core && go test ./...` + `pnpm --filter @copcon/chat-core test`)
- ** Automated tests**: Tests-after(先实现后补测试,因为 AI 开发模式下 task 内嵌 verification)
- **Framework**:
  - Go: `testify/assert` + `testify/require`(与项目现有保持一致)`
  - TypeScript: `vitest`(已配置于 `packages/chat-core/vitest.config.ts`)
- **TDD 策略**: 不强求 TDD,但每个任务必须附带单元测试,QA Scenarios 验证实际行为

### QA Policy

**ZERO HUMAN INTERVENTION** — 所有验证由 agent 自动执行。禁止"用户手动测试"类的验收标准。

每个任务必须包含:
- ✅ Go 单元测试(`*_test.go` 文件,`testify` 断言)
- ✅ TypeScript 单元测试(`*.test.ts`,Vitest,仅 chat-core 包需要)
- ✅ Agent 执行 QA Scenarios 至少 1 个 happy path + 1 个 failure/edge case
- ✅ 证据保存到 `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`

QA 工具选择:
- **Go 后端**: `bash` (执行 `go test`,读取输出);集成测试用 testcontainers(SQLite in-memory)
- **API 端点**: `curl` 或 Go `net/http/httptest` 客户端断言
- **前端 UI**: `Playwright`(使用 `playwright` skill)或 `dev-browser`(使用 `dev-browser` skill);断言 DOM 状态 / ARIA 属性 / 截图
- **构建验证**: `bash` 运行 `pnpm build`/`go build` 确认无编译错误
- **可访问性**: Playwright 的 `@axe-core` 或 `pa11y` 扫描

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (基础层, 1-2 agents, ~1d):
├── W1.1 MemoryStore 增强                    [deep]
├── W1.2 KnowledgeStore 可插拔接口          [deep]
├── W1.3 Embedder 接口                       [quick]
├── W1.4 OpenAI Embedder 实现               [quick]
├── W1.5 Configuration 扩展                 [quick]
├── W1.6 AgentSpec 双 Bundle 支持           [quick]
└── W1.7 KnowledgeStore 注册机制            [deep]

Wave 2 (记忆+知识库双轨并行, 2 agents, ~3d):
├── 记忆 Agent (W2 全部, ~5.5d):
│   ├── W2.1 文件记忆存储实现                 [deep]
│   ├── W2.2 FileMemoryHook                   [quick]
│   ├── W2.3 Memory Tools (store/recall/forget) [unspecified-high]
│   ├── W2.4 Memory Capability 注册           [quick]
│   └── W2.5 记忆单元测试                     [unspecified-high]
└── 知识库 Agent (W3 全部, ~10d):
    ├── W3.1 sqlite-vec KnowledgeStore 实现   [deep]
    ├── W3.2 文档解析器                       [unspecified-high]
    ├── W3.3 分块器                           [quick]
    ├── W3.4 入库管线                         [deep]
    ├── W3.5 KBRecallHook                     [quick]
    ├── W3.6 MemoryPersistHook (关键词提取)   [quick]
    ├── W3.7 KnowledgeBase Capability 注册    [quick]
    └── W3.8 知识库单元测试                    [unspecified-high]

Wave 3 (集成+评估并行, 2 agents, ~1d):
├── W4.1 Harness 双 Bundle 展开             [unspecified-high]
├── W4.2 StoreConfig + backend 切换         [quick]
├── W4.3 细粒度 skip 逻辑                   [quick]
├── W4.4 Harness 集成测试                   [unspecified-high]
├── W7.1 检索评估框架                        [quick]
├── W7.2 黄金测试集                          [quick]
└── W7.3 CI 集成                             [quick]

Wave 4 (Server API + chat-core 并行, 2 agents, ~1.5d):
├── 后端 API Agent (W5.1-W5.4):
│   ├── W5.1 知识库管理 API                   [deep]
│   ├── W5.2 检索测试+记忆管理 API          [quick]
│   ├── W5.3 API 路由注册 + DI              [quick]
│   └── W5.4 API 集成测试                     [unspecified-high]
└── chat-core Agent (W5.5-W5.7):
    ├── W5.5 types.ts 扩展                    [quick]
    ├── W5.6 AgentClient 方法扩展            [unspecified-high]
    └── W5.7 chat-core 单元测试               [unspecified-high]

Wave 5 (Demo UI 并行, 3 agents, ~2d):
├── UI Agent 1 (布局 + KB 页面, ~2d):
│   ├── W6.1 Demo 布局重构                   [visual-engineering]
│   ├── W6.2 知识库列表页                     [visual-engineering]
│   └── W6.3 知识库详情页                     [visual-engineering]
├── UI Agent 2 (上传+检索+分块, ~2d):
│   ├── W6.4 文档上传组件                     [visual-engineering]
│   ├── W6.5 分块预览                         [visual-engineering]
│   └── W6.6 检索测试                         [visual-engineering]
└── UI Agent 3 (记忆 UI + polish + a11y, ~2d):
    ├── W6.7 记忆管理页                       [visual-engineering]
    ├── W6.8 UI polish                        [visual-engineering]
    └── W6.9 可访问性                         [visual-engineering]

Wave FINAL (4 个审查 agent 并行):
├── Task F1: 计划合规审查 (oracle)
├── Task F2: 代码质量审查 (unspecified-high)
├── Task F3: 真实手工 QA (unspecified-high + playwright)
└── Task F4: 范围保真检查 (deep)
→ 呈现结果 → 获取用户明确 okay → 完成

Critical Path: W1 → W3 (sqlite-vec) → W4 → W5 → W6 → FINAL → user okay
Parallel Speedup: ~73% faster than sequential (33.5d → ~8.5d wall-clock)
Max Concurrent: 3 (Wave 5)
```

### Dependency Matrix (43 个工作项)

| ID | Depends On | Blocks | Wave |
|---|---|---|---|
| W1.1 | none | W2.1, W2.3, W3.5, W3.6 | 1 |
| W1.2 | none | W3.1, W4.2, W5.1 | 1 |
| W1.3 | none | W1.4, W3.4, W3.6 | 1 |
| W1.4 | W1.3 | W3.4, W3.6 | 1 |
| W1.5 | none | W2.1, W3.4, W4.2 | 1 |
| W1.6 | none | W4.1 | 1 |
| W1.7 | W1.2 | W3.1, W4.2 | 1 |
| W2.1 | W1.1, W1.5 | W2.2, W2.3, W2.5 | 2A |
| W2.2 | W2.1 | W2.4, W2.5 | 2A |
| W2.3 | W1.1, W2.1 | W2.4, W2.5 | 2A |
| W2.4 | W2.2, W2.3 | W4.1, W4.4 | 2A |
| W2.5 | W2.1, W2.2, W2.3 | F2 | 2A |
| W3.1 | W1.2, W1.7 | W3.4, W3.5, W3.6, W3.8 | 2B |
| W3.2 | none | W3.4 | 2B |
| W3.3 | none | W3.4 | 2B |
| W3.4 | W1.4, W3.1, W3.2, W3.3 | W3.6, W3.8 | 2B |
| W3.5 | W1.1, W3.1 | W3.7, W3.8, W4.1 | 2B |
| W3.6 | W1.1, W3.1 | W3.7, W3.8 | 2B |
| W3.7 | W3.5, W3.6 | W4.1, W4.4 | 2B |
| W3.8 | W3.1, W3.4, W3.5, W3.6 | F2 | 2B |
| W4.1 | W1.6, W2.4, W3.7 | W4.2, W4.4 | 3 |
| W4.2 | W1.5, W1.7, W4.1 | W4.4, W5.3 | 3 |
| W4.3 | W4.1 | W4.4 | 3 |
| W4.4 | W4.1, W4.2, W4.3 | F1, F2, F4 | 3 |
| W5.1 | W1.2, W3.1, W4.2 | W5.2, W5.3, W5.4, W5.6 | 4A |
| W5.2 | W5.1, W2.1 | W5.3, W5.4, W5.6 | 4A |
| W5.3 | W4.2, W5.1, W5.2 | W5.4, W6.1 | 4A |
| W5.4 | W5.1, W5.2, W5.3 | F2, F3 | 4A |
| W5.5 | W1.2, W5.1 (类型对齐) | W5.6, W6.x | 4B |
| W5.6 | W5.5 | W6.x, F2 | 4B |
| W5.7 | W5.5, W5.6 | F2, F3 | 4B |
| W6.1 | W5.1, W5.2, W5.3, W5.6 | W6.2, W6.3, W6.4, W6.7 | 5A |
| W6.2 | W6.1 | W6.3, F3 | 5A |
| W6.3 | W6.1, W6.2 | F3 | 5A |
| W6.4 | W6.1 | F3 | 5B |
| W6.5 | W6.3 | F3 | 5B |
| W6.6 | W6.1, W6.3 | F3 | 5B |
| W6.7 | W6.1 | F3 | 5C |
| W6.8 | W6.2, W6.3, W6.4, W6.5, W6.6, W6.7 | F2, F3 | 5C |
| W6.9 | W6.8 | F3 | 5C |
| W7.1 | W3.4 | W7.2, W7.3 | 3 (与 W4 并行) |
| W7.2 | W7.1 | W7.3 | 3 |
| W7.3 | W7.1, W7.2 | F1 | 3 |

### Agent Dispatch Summary

| Wave | Total Tasks | Agent Profile Dispatch |
|---|---|---|
| **Wave 1** | 7 | W1.1 → `deep`; W1.2 → `deep`; W1.7 → `deep`; W1.3-W1.6 → `quick` (4 个任务) |
| **Wave 2A** | 5 | W2.x 全部 → 单个 `deep` agent (memory subsystem) |
| **Wave 2B** | 8 | W3.x 全部 → 单个 `deep` agent (knowledge subsystem, sqlite-vec 重点) |
| **Wave 3** | 7 | W4.x → 单个 `unspecified-high` agent; W7.x → 单个 `quick` agent |
| **Wave 4A** | 4 | W5.1-W5.4 → 单个 `deep` agent (server API) |
| **Wave 4B** | 3 | W5.5-W5.7 → 单个 `unspecified-high` agent (chat-core) |
| **Wave 5A** | 3 | W6.1-W6.3 → `visual-engineering` (布局重构+KB 列表/详情) |
| **Wave 5B** | 3 | W6.4-W6.6 → `visual-engineering` (上传+分块+检索) |
| **Wave 5C** | 3 | W6.7-W6.9 → `visual-engineering` (记忆 UI + polish + a11y) |
| **FINAL** | 4 | F1 → `oracle`; F2 → `unspecified-high`; F3 → `unspecified-high` + `playwright`; F4 → `deep` |

---

## TODOs

<!--
  说明: 每个任务遵循统一模板,包含:
  - What to do / Must NOT do
  - Recommended Agent Profile (category + skills + 理由)
  - Parallelization info
  - References (精确文件:行数 + 为什么重要)
  - Acceptance Criteria (agent 可执行的测试命令)
  - QA Scenarios (精确步骤 + 断言 + 证据路径;每个任务至少 1 happy + 1 failure)
  - Commit strategy (YES/NO + 信息 + 预提交命令)
-->

### Wave 1 — W1: 基础层 (基础接口、配置、Embedder)

- [x] W1.1 增强 MemoryStore 接口

  **What to do**:
  - 在 `core/storage/memory.go` 中:
    - 给 `Memory` struct 新增 `ValidAt *time.Time`、`InvalidAt *time.Time`、`Importance float64` 字段 (受 Graphiti 启发)
    - 调整 `MemoryType` 常量命名: `MemoryTypeEpisodic = "episodic"` (对话轮次)、`MemoryTypeSemantic = "semantic"` (事实/知识)、`MemoryTypeProcedural = "procedural"` (模式),保留 `"conversation"` 作为 `MemoryTypeEpisodic` 的别名以保证向后兼容
    - 新增 `MemoryFilter` struct 用于 List: `SessionID string`, `MemoryType []MemoryType`, `Limit int`, `Offset int`, `Since time.Time`, `Until time.Time`
  - 扩展 `MemoryStore` interface 新增 3 个方法:
    ```go
    List(ctx context.Context, filter MemoryFilter) ([]*Memory, error)
    Get(ctx context.Context, id string) (*Memory, error)
    Update(ctx context.Context, memory *Memory) error
    Delete(ctx context.Context, id string) error
    ```
  - 保持现有 4 个方法 (`Store`/`Search`/`GetBySession`/`DeleteBySession`) 签名不变,保证现有 Qdrant 实现编译通过
  - 新增 `MemoryFilter` 相关的单元测试桩,在 W1.5 之后由 filememory 和 sqlitevec 实现

  **Must NOT do**:
  - 不要修改 `Memory` struct 的字段顺序或重命名现有字段 (会破坏 Qdrant provider 和现有测试)
  - 不要在 core/ 中 import server/
  - 不要在此任务中实现任何 provider (Qdrant 现有实现保留原状,新方法由 W2.1/W3.1 实现)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
    - Reason: 纯接口扩展工作,无复杂逻辑,只需精确理解现有代码
  - **Skills**: `[]` (空)
    - 不需要特定 skill,标准 Go 接口设计

  **Parallelization**:
  - **Can Run In Parallel**: YES (独立任务,与 W1.2-W1.7 可并行)
  - **Parallel Group**: Wave 1 (与 W1.2, W1.3, W1.4, W1.5, W1.6, W1.7 同 wave)
  - **Blocks**: W2.1, W2.3, W2.5, W3.5, W3.6
  - **Blocked By**: None (可立即启动)

  **References**:
  - `core/storage/memory.go:1-26` — 当前 `Memory` struct 和 `MemoryStore` 接口的完整定义,必须保持 `Store/Search/GetBySession/DeleteBySession` 字段/方法签名不变;新增字段加在末尾
  - `core/providers/qdrant/` — 现有的 Qdrant MemoryStore 实现,扩展接口后必须仍能编译通过 (本任务不负责新增方法的实现,由 W3.x 在 sqlitevec 实现)
  - `core/capabilities/hooks/memory.go:14-178` — 现有 memory hook 使用 `MemoryStore.Store` 和 `MemoryStore.Search`,本任务新增的方法不会被其使用,但签名变更会导致 hook 编译失败
  - **为何这样设计**: 字段加在末尾避免打破现有 provider 的 struct 初始化;新方法 `List/Get/Update/Delete` 是标准 CRUD,与 `SessionStore`/`MessageStore` 风格对齐

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./storage/...` — PASS, 0 errors
  - [ ] `cd core && go build ./...` — PASS (包括 Qdrant provider 仍能编译)
  - [ ] `cd core && go test ./storage/...` — PASS (如果存在测试)
  - [ ] `cd core && go vet ./storage/...` — PASS, 0 warnings
  - [ ] 新增类型 `MemoryFilter`、`MemoryType` 枚举、`StoreProvider.Memory()` 返回签名正确

  **QA Scenarios**:

  ```
  Scenario: 接口扩展后 Qdrant provider 不破坏编译
    Tool: bash
    Preconditions: 工作目录在 /data/copcon
    Steps:
      1. git status 确认无未提交改动
      2. 执行 cd core && go build ./providers/qdrant/... 2>&1
      3. 执行 cd core && go build ./... 2>&1
    Expected Result: 两条命令都返回 0 退出码,无编译错误输出;Memory struct 包含新增的 ValidAt/InvalidAt/Importance 字段 (通过 go doc github.com/copcon/core/storage Memory 检查)
    Failure Indicators: 任一命令返回非零退出码;输出含 "undefined" 或 "cannot use" 等编译错误
    Evidence: .sisyphus/evidence/task-W1.1-compile-check.txt
  ```

  ```
  Scenario: MemoryStore 接口新增方法签名正确 (类型检查)
    Tool: bash
    Preconditions: 新增方法后
    Steps:
      1. 创建临时 Go 文件 /tmp/test_memory.go 包含:
         ```go
         package main
         import "github.com/copcon/core/storage"
         import "context"
         func test(s storage.MemoryStore) {
           _ = s.List
           _ = s.Get
           _ = s.Update
           _ = s.Delete
           _, _ = s.List(context.Background(), storage.MemoryFilter{Limit: 5})
         }
         func main() {}
         ```
      2. 执行 cd core && go build /tmp/test_memory.go
    Expected Result: go build 返回 0,无任何错误
    Failure Indicators: "not found in interface" 或 "undefined: MemoryFilter"
    Evidence: .sisyphus/evidence/task-W1.1-interface-signature.txt
  ```

  **Commit**: YES (groups with none, atomic)
  - Message: `feat(storage): enhance MemoryStore with list/get/update/delete + temporal fields`
  - Files: `core/storage/memory.go`
  - Pre-commit: `cd core && go build ./... && go vet ./...`

- [x] W1.2 新增 KnowledgeStore 可插拔接口

  **What to do**:
  - 新建 `core/storage/knowledge.go` 定义完整接口类型:
    - `KnowledgeBase` struct: `ID string`, `Name string`, `Backend string` (如 `"sqlite-vec"`), `Config map[string]any`, `CreatedAt`, `UpdatedAt`, `Metadata map[string]any`
    - `Document` struct: `ID`, `KBID`, `Filename`, `Source` ("upload"/"api"/"sync"), `Status DocumentStatus`, `ChunkCount int`, `TokenCount int`, `CreatedAt`, `UpdatedAt`, `Metadata`
    - `DocumentStatus` type + 4 个常量: `DocStatusPending`/`DocStatusParsing`/`DocStatusReady`/`DocStatusError`
    - `Chunk` struct: `ID`, `DocumentID`, `KBID`, `Content`, `Context string` (为 Contextual Retrieval 预留字段,本期不使用), `Index int`, `TokenCount int`, `Metadata`, `Score float32` (检索时填充)
    - `SearchOptions` struct: `TopK int`, `SimilarityThreshold float32`, `Filters map[string]any`
    - `KnowledgeStore` interface 包含:
      - `CreateKB(ctx, *KnowledgeBase) error`
      - `DeleteKB(ctx, string kbID) error`
      - `ListKBs(ctx) ([]*KnowledgeBase, error)`
      - `GetKB(ctx, string kbID) (*KnowledgeBase, error)`
      - `IngestDocument(ctx, kbID, *Document, content []byte) error` (content 是未解析的原始字节)
      - `ListDocuments(ctx, kbID) ([]*Document, error)`
      - `DeleteDocument(ctx, kbID, docID) error`
      - `GetDocument(ctx, kbID, docID) (*Document, error)`
      - `GetChunks(ctx, docID) ([]*Chunk, error)`
      - `UpdateChunk(ctx, *Chunk) error`
      - `Search(ctx, []string kbIDs, query []float32, SearchOptions) ([]*Chunk, error)`
  - 修改 `core/storage/provider.go` 让 `StoreProvider` interface 新增 `Knowledge() KnowledgeStore` 方法;返回 nil 表示未配置 (与 `Todos()` 一致,允许 `StoreProvider` 不启用知识库)
  - 新增单元测试 `core/storage/knowledge_test.go` 用 mock 实现验证接口方法存在

  **Must NOT do**:
  - 不要在接口中硬编码 Qdrant 或 sqlite-vec 特有的参数 (所有实现特定细节放到 provider 内部)
  - 不要修改现有 `Sessions()`/`Messages()`/`Todos()` 3 个方法的签名
  - 不要让 `Knowledge()` 强制非 nil——必须接受返回 nil (保持现有 postgres StoreProvider 编译通过)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 核心接口设计,影响后续所有 provider 实现,需要仔细思考边界和扩展性
  - **Skills**: `[]` (空)
    - 不需要特定 skill

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (与 W1.1-W1.7 同 wave)
  - **Blocks**: W3.1 (sqlite-vec 实现)、W4.2 (StoreConfig enhancement)、W5.1 (API handler)
  - **Blocked By**: None

  **References**:
  - `core/storage/provider.go:1-10` — 当前 `StoreProvider` 的 3 个方法签名,本任务在此基础上增加 `Knowledge()`;参考 `Todos()` 返回 nil 的模式
  - `core/storage/memory.go` — `MemoryStore` 接口风格一致性参考
  - `core/storage/session.go`, `core/storage/message.go` — 参考现有 Store interface 的方法设计 (List 用 limit/offset,Create 接收指针,返回 error)
  - **为何这样设计**: `KBID` 作为独立字段 (而非嵌套 `KnowledgeBase`) 方便检索时只查 chunks 不加载 KB;`Context` 字段预留给 Contextual Retrieval (本期不使用但接口不需再改);`DocumentStatus` 用 string type enum 与 `TodoStatus` 模式一致

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./storage/...` — PASS
  - [ ] `cd core && go vet ./storage/...` — PASS
  - [ ] `cd core && go test ./storage/...` — PASS (包括新增 mock 测试)
  - [ ] 现有 postgres/qdrant providers 编译通过 (`cd core && go build ./providers/...`)
  - [ ] `Knowledge()` 方法返回 nil 不会触发任何 panic (写一个测试验证)

  **QA Scenarios**:

  ```
  Scenario: 现有 StoreProvider 实现仍可编译 (向后兼容)
    Tool: bash
    Preconditions: 工作目录 /data/copcon
    Steps:
      1. 执行 cd core && go build ./providers/postgres/... 2>&1
      2. 执行 cd core && go build ./providers/qdrant/... 2>&1
      3. 执行 cd server && go build ./... 2>&1
    Expected Result: 所有 3 条命令返回 0;如果 postgres/qdrant provider 未实现 Knowledge() 方法,go build 报错说明需要更新 (此时由对应 provider 在 W3 任务处理),本任务不要求 provider 立即实现该方法
    Note: 为了让现有 providers 不报错,可以在 `store/provider.go` 中提供空接口的默认实现 OR 让 `Knowledge()` 的返回类型改为 `KnowledgeStore` interface,允许返回 nil (推荐后者)
    Failure Indicators: "does not implement StoreProvider (missing Knowledge method)"
    Evidence: .sisyphus/evidence/task-W1.2-backward-compat.txt
  ```

  ```
  Scenario: KnowledgeStore 接口方法可静态验证
    Tool: bash
    Steps:
      1. 创建临时文件 /tmp/test_ks.go 实现所有 KnowledgeStore 方法的桩
      2. 编译通过
      3. 用 go vet 检查
    Expected Result: 0 errors,所有方法签名与 core/storage/knowledge.go 一致
    Evidence: .sisyphus/evidence/task-W1.2-interface-signature.txt
  ```

  **Commit**: YES
  - Message: `feat(storage): add pluggable KnowledgeStore interface with Document/Chunk types`
  - Files: `core/storage/knowledge.go`, `core/storage/provider.go`, `core/storage/knowledge_test.go`
  - Pre-commit: `cd core && go build ./... && go vet ./... && go test ./storage/...`

- [x] W1.3 定义 Embedder 接口

  **What to do**:
  - 新建 `core/providers/embedding/embedder.go`:
    ```go
    package embedding

    import "context"

    // Embedder 抽象文本嵌入能力。
    // 实现可以对接 OpenAI、BGE-M3 sidecar、本地 ONNX 模型等。
    type Embedder interface {
      // Embed 返回文本的稠密向量 (维度由实现决定)
      Embed(ctx context.Context, text string) ([]float32, error)
      // EmbedBatch 批量嵌入;实现应复用底层请求以提高吞吐
      EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
      // Dimensions 返回向量维度 (配置或运行时查询,用于 Qdrant/sqlite-vec 初始化)
      Dimensions() int
      // Name 返回嵌入器名称 (用于日志和配置校验,如 "openai:text-embedding-3-small")
      Name() string
    }
    ```
  - 新建 `core/providers/embedding/config.go` 定义 `EmbeddingConfig` struct:
    ```go
    type BackendType string
    const (
      BackendOpenAI BackendType = "openai"
      BackendBGEM3  BackendType = "bge_m3" // 预留,本期不实现
    )
    type EmbeddingConfig struct {
      Backend      BackendType `yaml:"backend"`
      OpenAIModel  string      `yaml:"openai_model,omitempty"` // 默认 "text-embedding-3-small"
      BGEM3Endpoint string    `yaml:"bge_m3_endpoint,omitempty"` // 预留
    }
    ```
  - 新增 `core/providers/embedding/errors.go` 定义常见错误: `ErrUnsupportedBackend`, `ErrEmptyText`, `ErrDimensionMismatch`
  - 单元测试: `embedder_test.go` 验证接口类型完整性

  **Must NOT do**:
  - 不要在此任务中实现 BGE-M3 后端 (仅预留 config 字段)
  - 不要把 OpenAI API client 实例化在此任务 (W1.4 负责)
  - 不要在 Embedder 接口中引入多 LLM 管理模块

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 纯接口定义,无逻辑
  - **Skills**: `[]` (空)

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Blocks**: W1.4 (OpenAI 实现)、W3.4 (入库管线)、W3.5 (KBRecallHook)
  - **Blocked By**: None

  **References**:
  - `core/llm/provider.go` (或类似路径,先 grep 查找 LLMProvider 接口定义) — 参考现有 LLM provider 的接口设计风格
  - `core/capabilities/hooks/memory.go:132-139` — 现有 `encodeTextToVector` 函数是 byte-encoding 占位符,W1.4 后用 Embedder 替代
  - **为何这样设计**: EmbedBatch 显式提供批量接口而非让调用方循环调用,允许实现做请求合并;Dimensions() 让上游 sqlite-vec 集合初始化时查询维度,避免硬编码

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./providers/embedding/...` — PASS
  - [ ] `cd core && go test ./providers/embedding/...` — PASS (接口验证测试)
  - [ ] 接口可被外部包引用: `import "github.com/copcon/core/providers/embedding"`

  **QA Scenarios**:

  ```
  Scenario: Embedder 接口可被 mock 实现
    Tool: bash
    Steps:
      1. 创建 /tmp/mock_embedder.go 实现 Embedder 4 个方法
      2. 编译通过 + 运行一个简单的测试 main 调用
    Expected Result: mock 实现可编译可执行
    Evidence: .sisyphus/evidence/task-W1.3-interface-mock.txt
  ```

  **Commit**: YES
  - Message: `feat(embedding): define Embedder interface with backend abstraction`
  - Files: `core/providers/embedding/embedder.go`, `core/providers/embedding/config.go`, `core/providers/embedding/errors.go`, `core/providers/embedding/embedder_test.go`
  - Pre-commit: `cd core && go build ./providers/embedding/... && go test ./providers/embedding/...`

- [x] W1.4 实现 OpenAI Embedder

  **What to do**:
  - 新建 `core/providers/embedding/openai.go`:
    - 定义 `openAIEmbedder` struct: `llm llm.LLMProvider` (复用项目现有 `core/llm.LLMProvider`), `model string`, `dimensions int`
    - 实现 `Embedder` 接口 4 个方法:
      - `Embed`: 构造 `/v1/embeddings` 请求,调用 llm provider 的 HTTP 接口 (如果有直接的 API) 或直接发起 HTTP 调用 (参考 llm provider 的实现方式)
      - `EmbedBatch`: 一次请求多个文本 (OpenAI API 支持 batch),如果 provider 不支持 batch 则循环 fallback
      - `Dimensions`: 返回配置值,默认 1536 (text-embedding-3-small)
      - `Name`: 返回 `"openai:" + model`
    - 新增构造函数 `NewOpenAIEmbedder(llm llm.LLMProvider, model string) (Embedder, error)`:
      - 校验 model ∈ {`text-embedding-3-small`, `text-embedding-3-large`, `text-embedding-ada-002`}
      - 根据 model 设置默认 dimensions
  - 编写单元测试 `openai_test.go`:
    - 用 `net/http/httptest` 模拟 OpenAI API
    - 测试 happy path: 单文本嵌入、批量嵌入
    - 测试错误: HTTP 500、超时、非法 JSON 响应
    - 测试维度校验: 响应向量维度必须等于声明的 dimensions
  - 更新 `core/providers/embedding/` 包,新增便捷构造函数 `NewFromConfig(cfg EmbeddingConfig, llm llm.LLMProvider) (Embedder, error)`:
    - Backend=openai → return NewOpenAIEmbedder(llm, cfg.OpenAIModel)
    - Backend=bge_m3 → return ErrUnsupportedBackend (本期不实现)

  **Must NOT do**:
  - 不要直接引入 OpenAI Go SDK (`github.com/sashabaranov/go-openai`)——复用项目现有 LLM provider 抽象,避免双份 API 管理
  - 不要在 Embedder 实现中加入 Contextual Retrieval 逻辑 (这属于 W3.4 入库管线的范畴,本期不做)
  - 不要在测试中实际调用 OpenAI API (用 httptest mock)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 主要是 HTTP 客户端封装,有明确的测试模式
  - **Skills**: `[]` (空)

  **Parallelization**:
  - **Can Run In Parallel**: YES (依赖 W1.3 完成接口定义)
  - **Blocks**: W3.4 (入库管线)、W3.5 (KBRecallHook)、W3.6 (MemoryPersistHook)
  - **Blocked By**: W1.3

  **References**:
  - `core/llm/provider.go` — 复用现有 LLMProvider 的 HTTP 请求能力;如果 LLMProvider 不提供 embedding 端点,则需要直接使用 `llm.Config` 中的 `BaseURL` + `APIKey` 构造请求
  - `core/capabilities/hooks/memory.go:132-139` — 现有 `encodeTextToVector` 字节编码占位符必须被替换,W3.5/W3.6 的 hook 会调用此 embedder
  - `core/providers/qdrant/memory.go` (grep 查找现有 vector 处理逻辑) — 了解现有 Qdrant provider 如何处理 float32 向量,以确保 OpenAIEmbedder 返回类型与之兼容
  - **为何这样设计**: 复用 LLMProvider 可避免引入多 LLM 管理 (用户明确要求不做);批量 API 显著降低 token 成本和延迟 (1 次 HTTP vs N 次)

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./providers/embedding/...` — PASS
  - [ ] `cd core && go test ./providers/embedding/...` — PASS (单文本/批量/错误 case 都覆盖)
  - [ ] httptest mock 测试返回的向量维度等于 `text-embedding-3-small` 的默认 1536
  - [ ] `NewFromConfig` 在 Backend=bge_m3 时返回 ErrUnsupportedBackend

  **QA Scenarios**:

  ```
  Scenario: OpenAI Embedder happy path 单文本嵌入
    Tool: bash
    Steps:
      1. 执行 cd core && go test -run TestOpenAIEmbedder_Single ./providers/embedding/... -v
      2. 测试内部:启动 httptest.Server 模拟 /v1/embeddings (返回固定 1536-dim 向量)
      3. 调用 Embed("hello world"),断言返回 []float32 长度 = 1536
    Expected Result: PASS;测试输出含 "--- PASS: TestOpenAIEmbedder_Single"
    Evidence: .sisyphus/evidence/task-W1.4-single-embed.txt
  ```

  ```
  Scenario: OpenAI Embedder batch API 复用请求
    Tool: bash
    Steps:
      1. 测试内部:httptest.Server 计数请求次数
      2. 调用 EmbedBatch(["a", "b", "c"])
      3. 断言 server 只收到 1 次请求 (batch 复用) 或 fallback 时收到 3 次 (需在文档中说明策略)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W1.4-batch-embed.txt
  ```

  ```
  Scenario: OpenAI Embedder 处理 HTTP 500 错误
    Tool: bash
    Steps:
      1. httptest.Server 返回 500
      2. 调用 Embed() 接收 error
      3. 断言 error 不为 nil 且包含 "500" 或类似信息
    Expected Result: PASS,无 panic
    Evidence: .sisyphus/evidence/task-W1.4-error-handling.txt
  ```

  **Commit**: YES
  - Message: `feat(embedding): implement OpenAI embedder reusing existing LLMProvider`
  - Files: `core/providers/embedding/openai.go`, `core/providers/embedding/openai_test.go`, `core/providers/embedding/factory.go` (NewFromConfig)
  - Pre-commit: `cd core && go test ./providers/embedding/...`

- [x] W1.5 配置扩展 (MemoryConfig + KnowledgeBaseConfig + EmbeddingConfig)

  **What to do**:
  - 在 `server/internal/config/config.go` 新增:
    ```go
    // AgentConfig 新增字段
    type AgentConfig struct {
      // ... 现有 ID/Name/Model/SystemPrompt/Tools/BaseURL ...
      Memory         MemoryConfig   `yaml:"memory,omitempty"`
      KnowledgeBases []string       `yaml:"knowledge_bases,omitempty"` // 引用 knowledge_bases 段中定义的 id
    }

    // 记忆 (MD 文件) 配置
    type MemoryConfig struct {
      Enabled    bool   `yaml:"enabled"`
      BasePath   string `yaml:"base_path,omitempty"` // 默认 ~/.copcon/memory/<agent_id>/
      SystemDir  string `yaml:"system_dir,omitempty"` // 默认 "system"
      IndexFile  string `yaml:"index_file,omitempty"` // 默认 "INDEX.md"
      MaxIndexLines int `yaml:"max_index_lines,omitempty"` // 默认 200
      MaxIndexBytes int `yaml:"max_index_bytes,omitempty"` // 默认 25000
    }

    // 知识库 (RAG) 配置,顶层定义,Agent 通过 id 引用
    type KnowledgeBaseConfig struct {
      ID            string         `yaml:"id"`            // 唯一标识,被 Agent 引用
      Name          string         `yaml:"name"`
      Backend       string         `yaml:"backend"`       // 本期只支持 "sqlite-vec"
      SQLitePath    string         `yaml:"sqlite_path,omitempty"` // sqlite-vec 文件路径
      ChunkSize     int            `yaml:"chunk_size,omitempty"` // 默认 800
      ChunkOverlap  int            `yaml:"chunk_overlap,omitempty"` // 默认 100
      Embedding     EmbeddingConfig `yaml:"embedding"`
    }

    // 嵌入配置
    type EmbeddingConfig struct {
      Backend       string `yaml:"backend"` // "openai" (本期唯一支持)
      OpenAIModel   string `yaml:"openai_model,omitempty"` // 默认 "text-embedding-3-small"
      BGEM3Endpoint string `yaml:"bge_m3_endpoint,omitempty"` // 预留
    }
    ```
  - 在 root `Config` struct 新增: `KnowledgeBases []KnowledgeBaseConfig `yaml:"knowledge_bases,omitempty"``
  - 在 `validate()` 中新增检查:
    - `KnowledgeBases` 中 id 不能重复
    - `Agent.KnowledgeBases` 中引用的 id 必须在顶层 `KnowledgeBases` 中存在
    - 当 `Agent.KnowledgeBases` 非空时,对应的 `KnowledgeBaseConfig.Embedding.Backend` 必须是已支持的 (本期只有 "openai")
    - `Agent.Memory.BasePath` 如不为空,必须是绝对路径或 `~/` 前缀
  - 更新 `server/config.yaml.template`:添加示例 memory 段 + knowledge_bases 段 (注释说明用法)

  **Must NOT do**:
  - 不要在 config 验证阶段创建任何文件系统目录 (这是 W2.1 filememory 的责任)
  - 不要修改现有 `ServerConfig` / `DatabaseConfig` / `OpenAIConfig` / `QdrantConfig` 字段 (即使 Qdrant 不再强制,保留向后兼容)
  - 不要把 `EmbeddingConfig` 重复放在 `AgentConfig` 中——embedding 配置绑定在 KnowledgeBase 上,Agent 只引用 KB id

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 主要是 YAML struct 定义 + 简单校验
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Blocks**: W2.1 (filememory 用 BasePath)、W3.1 (sqlite-vec 用 SQLitePath)、W3.4 (入库用 ChunkSize/Embedding)、W4.2 (StoreConfig)
  - **Blocked By**: None

  **References**:
  - `server/internal/config/config.go:11-18` — 现有 `Config` struct,`KnowledgeBases` 加在 `Qdrant` 之后
  - `server/internal/config/config.go:20-27` — 现有 `AgentConfig`,新增 `Memory` + `KnowledgeBases` 字段
  - `server/internal/config/config.go:49-52` — 现有 `QdrantConfig` 参考,`KnowledgeBaseConfig` 风格对齐
  - `server/internal/config/config.go:82-105` — 现有 `validate()` 方法,在此追加新校验规则
  - `server/config.yaml.template` — 现有模板文件,需同步更新
  - **为何这样设计**: KB 顶层定义 + Agent 引用 id 的模式,允许一个 KB 被多个 Agent 共享;embedding 绑定 KB 而非 Agent,因为不同 KB 可用不同 embedding (虽然本期都只用 openai)

  **Acceptance Criteria**:
  - [ ] `cd server && go build ./internal/config/...` — PASS
  - [ ] `cd server && go test ./internal/config/...` — PASS (如已存在测试)
  - [ ] 新增单元测试验证:
    - duplicate KB id 报错
    - Agent 引用不存在的 KB id 报错
    - 不支持的 embedding.backend 报错
  - [ ] `server/config.yaml.template` 包含 memory + knowledge_bases 示例段

  **QA Scenarios**:

  ```
  Scenario: 重复 KB id 校验失败
    Tool: bash
    Steps:
      1. 创建临时 yaml 含 2 个同名 id 的 knowledge_bases 条目
      2. 调用 config.Load() + validate()
      3. 断言返回错误含 "duplicate knowledge base id"
    Expected Result: validate 返回非 nil error
    Evidence: .sisyphus/evidence/task-W1.5-duplicate-id.txt
  ```

  ```
  Scenario: Agent 引用不存在的 KB id 校验失败
    Tool: bash
    Steps:
      1. yaml 中定义 KB id="kb1",Agent 引用 "kb2"
      2. validate 返回错误
    Expected Result: 错误信息指向出错的 Agent 和 KB id
    Evidence: .sisyphus/evidence/task-W1.5-missing-kb-ref.txt
  ```

  ```
  Scenario: 合法配置通过校验
    Tool: bash
    Steps:
      1. 写入 server/config.yaml.template 中的所有字段
      2. 调用 config.Load()
      3. 验证返回 nil error,所有字段按预期解码
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W1.5-valid-config.txt
  ```

  **Commit**: YES
  - Message: `feat(config): add MemoryConfig, KnowledgeBaseConfig, EmbeddingConfig structs`
  - Files: `server/internal/config/config.go`, `server/internal/config/config_test.go` (新增或扩展), `server/config.yaml.template`
  - Pre-commit: `cd server && go test ./internal/config/...`

- [x] W1.6 AgentSpec 双 Bundle + MemoryBundleNames / KnowledgeBaseBundleNames

  **What to do**:
  - 在 `core/agent.go` (或类似文件,先 grep 查找 AgentSpec 定义) 中为 `AgentSpec` struct 新增:
    ```go
    Memory         MemorySpec   // 与 server/internal/config/config.go 的 MemoryConfig 对应,但 core/ 内部使用
    KnowledgeBases []string      // 引用已配置的知识库 id
    ```
  - 新建 `core/capabilities/bundle.go` 定义两个函数:
    ```go
    package capabilities

    // MemoryBundleNames 返回记忆能力包包含的所有 capability name
    // 包含:FileMemoryHook + 3 个 memory tools (store/recall/forget) + 现有 memory hook
    func MemoryBundleNames() []string {
      return []string{
        "hooks.file_memory",
        "hooks.memory",        // 现有的 vector memory hook (W4 中可能重构/合并到 KB recall)
        "tools.memory_store",
        "tools.memory_recall",
        "tools.memory_forget",
      }
    }

    // KnowledgeBaseBundleNames 返回知识库能力包包含的所有 capability name
    func KnowledgeBaseBundleNames() []string {
      return []string{
        "hooks.kb_recall",
        "hooks.memory_persist",
        // 注意:知识库不暴露 tool,只通过 hook 自动工作
      }
    }
    ```
  - 新增单元测试 `core/capabilities/bundle_test.go`:
    - 验证两个函数返回的 slice 非空
    - 验证两个 slice 元素**不重叠** (集合无交集)
    - 验证所有返回的 capability name 在 `ListByType` 或 ExpandWildcards 中可解析 (W2/W3 实现后补,本任务先定义函数)

  **Must NOT do**:
  - 不要在 core/capabilities/bundle.go 中引用 server/internal/config/config.go (模块边界违规)
  - 不要让 MemoryBundleNames 返回任何 hooks.kb_recall (这两个 bundle 必须解耦)
  - 不要修改现有 `harness.go` 的 `collectCapabilityNames()`——那是 W4.1 的工作

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 主要是 struct 字段追加 + 2 个简单函数定义
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Blocks**: W4.1 (Harness 双 bundle 展开用 MemoryBundleNames + KnowledgeBaseBundleNames)
  - **Blocked By**: None

  **References**:
  - `core/harness.go:82-99` — 现有 `AgentSpec` 和相关 struct 定义位置;在此处新增字段
  - `core/harness.go:299-327` — 现有 `collectCapabilityNames()`,了解其结构以便 W4.1 修改 (本任务不动)
  - `core/capabilities/registry.go:20-23` — 现有 4 种 CapabilityType (tool/hook/skill/memory)
  - `core/harness.go:65-66` — 现有 `builtInHooks = []string{"hooks.todo_injection", "hooks.memory", "hooks.logging", "hooks.tracing"}`,了解当前 hooks.memory 已列入;新的 file_memory 是新的 capability
  - **为何这样设计**: 把 bundle 定义放在 `core/capabilities/bundle.go` 而不是散列在 `harness.go`,便于后续维护和测试;hooks.memory 保留在 bundle 中 (W3.1 的 sqlite-vec 实现会重写 hooks/memory.go 的占位实现)

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./...` — PASS
  - [ ] `cd core && go build ./capabilities/...` — PASS
  - [ ] `cd core && go test ./capabilities/...` — PASS (新增 bundle_test.go)
  - [ ] AgentSpec 新增字段可被外部包引用
  - [ ] MemoryBundleNames 与 KnowledgeBaseBundleNames 无交集 (单元测试)

  **QA Scenarios**:

  ```
  Scenario: 两个 bundle 无交集
    Tool: bash
    Steps:
      1. 执行 cd core && go test -run TestBundleDisjoint ./capabilities/... -v
      2. 测试内部取两个 slice,放入 set,断言无重复
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W1.6-bundle-disjoint.txt
  ```

  ```
  Scenario: AgentSpec 新字段可被设置和读取
    Tool: bash
    Steps:
      1. 在临时 main.go 中构造 AgentSpec,设置 Memory (enabled=true) 和 KnowledgeBases=[]{ "docs" }
      2. 编译执行
    Expected Result: 编译执行成功,字段读取与设置一致
    Evidence: .sisyphus/evidence/task-W1.6-spec-fields.txt
  ```

  **Commit**: YES
  - Message: `feat(agent): add Memory + KnowledgeBases fields to AgentSpec, define bundle names`
  - Files: `core/agent.go`, `core/capabilities/bundle.go`, `core/capabilities/bundle_test.go`
  - Pre-commit: `cd core && go build ./... && go test ./capabilities/...`

- [x] W1.7 KnowledgeStore Provider 注册机制

  **What to do**:
  - 新建 `core/storage/knowledge_registry.go` (包级别 registry,非 agent 级):
    ```go
    package storage

    import (
      "fmt"
      "sync"
    )

    // KnowledgeStoreFactory 由 provider (如 sqlite-vec) 在 init() 中注册。
    // config 是从 YAML 解析出的 *KnowledgeBaseConfig 的 map 形态,具体字段由各 provider 自行解析。
    type KnowledgeStoreFactory func(config map[string]any) (KnowledgeStore, error)

    var (
      knowledgeStoreMu       sync.Mutex
      knowledgeStoreFactories = map[string]KnowledgeStoreFactory{}
    )

    // RegisterKnowledgeStoreProvider 应在 init() 中调用。
    // name 示例:"sqlite-vec"、"qdrant"、"pgvector"
    func RegisterKnowledgeStoreProvider(name string, factory KnowledgeStoreFactory) {
      knowledgeStoreMu.Lock()
      defer knowledgeStoreMu.Unlock()
      if _, exists := knowledgeStoreFactories[name]; exists {
        panic(fmt.Sprintf("storage: duplicate KnowledgeStore provider: %s", name))
      }
      knowledgeStoreFactories[name] = factory
    }

    // LookupKnowledgeStoreProvider 由 Harness.Build() 调用,根据 config 中的 backend 查找 provider。
    func LookupKnowledgeStoreProvider(name string) (KnowledgeStoreFactory, error) {
      knowledgeStoreMu.Lock()
      defer knowledgeStoreMu.Unlock()
      f, ok := knowledgeStoreFactories[name]
      if !ok {
        return nil, fmt.Errorf("unknown KnowledgeStore backend: %s (registered: %v)", name, providerNames())
      }
      return f, nil
    }

    func providerNames() []string {
      var names []string
      for n := range knowledgeStoreFactories {
        names = append(names, n)
      }
      return names
    }
    ```
  - 新增单元测试 `core/storage/knowledge_registry_test.go`:
    - happy path: Register + Lookup 成功
    - Lookup 未知 backend 返回 error
    - 重复 Register panic (用 `recover()` 捕获断言)
    - 并发 Register/Lookup 不产生 race (用 -race flag 跑)
  - **注意**: 本任务不实现 sqlite-vec provider (那是 W3.1),只搭建 registration mechanism

  **Must NOT do**:
  - 不要让 Register 返回 error 而非 panic——重复注册是程序错误应 fail fast (参考 AGENTS.md "Fail-fast errors")
  - 不要在 core/storage 包内 import 任何 providers 子包 (避免循环依赖;providers 反向 import storage)
  - 不要提供 `UnregisterKnowledgeStoreProvider` (注册应是不可变的)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 模式明确的 sync.Map/Map registry + 单元测试
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (依赖 W1.2 完成 KnowledgeStore 接口,但可与 W1.2 同 wave 跑,只要 W1.2 先于 W1.7 完成即可)
  - **Blocks**: W3.1 (sqlite-vec 实现会调 RegisterKnowledgeStoreProvider)、W4.2 (Harness 用 Lookup)
  - **Blocked By**: W1.2 (KnowledgeStore 接口存在)

  **References**:
  - `core/storage/` 目录 — 现有 storage 包结构
  - `core/capabilities/registry.go:66-98` — 参考现有 `Register(c Capability)` 的全局 sync.Map 模式;本任务用更简单的 map + mutex 就够
  - `core/harness.go:152-155` — 现有 `initStores()` 模式,本任务的 LookupKnowledgeStoreProvider 会在此处被调用 (W4.2 修改)
  - **为何这样设计**: init() 注册 + Lookup 查找是 Go 项目的常见 plugin 模式 (参考 image/png 等标准库),无需引入 plugin package;map + mutex 比 sync.Map 更易读且够用

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./storage/...` — PASS
  - [ ] `cd core && go test ./storage/...` — PASS (包括 race detector)
  - [ ] `cd core && go vet ./storage/...` — PASS
  - [ ] `go test -race ./storage/...` 不报 race condition

  **QA Scenarios**:

  ```
  Scenario: Register + Lookup happy path
    Tool: bash
    Steps:
      1. 单元测试中注册 "test-backend"
      2. Lookup "test-backend" 返回 factory,无 error
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W1.7-happy-path.txt
  ```

  ```
  Scenario: 重复 Register panic
    Tool: bash
    Steps:
      1. 单元测试中注册 "dup" + defer recover
      2. 再次注册 "dup"
      3. 断言 recovered value 包含 "duplicate"
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W1.7-duplicate-panic.txt
  ```

  ```
  Scenario: 并发读写安全 (race detector)
    Tool: bash
    Steps:
      1. 启动 10 个 goroutine 同时 Register 不同 backend
      2. 启动 10 个 goroutine 同时 Lookup
      3. go test -race
    Expected Result: 无 race condition
    Evidence: .sisyphus/evidence/task-W1.7-race-safety.txt
  ```

  **Commit**: YES
  - Message: `feat(storage): add RegisterKnowledgeStoreProvider registry for backend dispatch`
  - Files: `core/storage/knowledge_registry.go`, `core/storage/knowledge_registry_test.go`
  - Pre-commit: `cd core && go test -race ./storage/...`

### Wave 2A — W2: 记忆能力 (MD 文件,单 agent 串行)

- [x] W2.1 文件记忆存储实现 (filememory provider)

  **What to do**:
  - 新建目录 `core/providers/filememory/`,包含 5 个文件:
    - `filememory.go` — 主入口,实现 `storage.MemoryStore` + 文件级 CRUD 扩展接口 `FileMemoryStore`:
      ```go
      type FileMemoryStore interface {
        storage.MemoryStore  // 继承基础 CRUD
        // 文件级操作 (agent 工具使用)
        ReadFile(ctx context.Context, agentID, relPath string) ([]byte, error)
        WriteFile(ctx context.Context, agentID, relPath string, content []byte, metadata FileMetadata) error
        DeleteFile(ctx context.Context, agentID, relPath string) error
        ListFiles(ctx context.Context, agentID, relPath string) ([]FileEntry, error)
        // INDEX.md 操作
        GetIndex(ctx context.Context, agentID string) ([]byte, error)
        UpdateIndex(ctx context.Context, agentID string, entry IndexEntry) error
        RemoveFromIndex(ctx context.Context, agentID, relPath string) error
      }
      ```
    - `frontmatter.go` — YAML frontmatter 解析/写入:
      ```go
      type FileMetadata struct {
        Name        string    `yaml:"name"`
        Description string    `yaml:"description"`
        Type        string    `yaml:"type"` // user | feedback | project | reference
        Created     time.Time `yaml:"created"`
        Importance  float64   `yaml:"importance,omitempty"`
      }
      // ParseFrontmatter(data []byte) (FileMetadata, []byte content, error)
      // SerializeFrontmatter(meta FileMetadata, content []byte) []byte
      ```
    - `index.go` — INDEX.md 维护:
      - `BuildIndex(agentID) ([]byte, error)` — 扫描 knowledge/ + archive/ 目录,生成 MD 格式的索引 (每行 `- [Name](relpath) — Description`)
      - 严格遵守 `max_index_lines` + `max_index_bytes` 硬限,超限时截断并附加 warning 注释
      - INDEX.md 的每行不超过 150 字符 (超过的描述用 `...` 截断)
    - `path_validator.go` — 路径安全校验:
      - 拒绝 `..` 组件
      - 拒绝绝对路径
      - 拒绝 `.dot` 隐藏文件 (如 `.env`)
      - 拒绝符号链接 (或至少拒绝指向 base_path 外的链接)
      - 在 Resolve 后再次校验仍在 base_path 内 (防 symlink escape)
    - `dir.go` — 目录管理:
      - `EnsureAgentDirs(agentID)` — 创建 `system/` `knowledge/` `archive/` 子目录 (如不存在)
      - 目录权限 0o700,文件权限 0o600 (与 Anthropic memory protocol 对齐)
    - `filememory_test.go` — 综合单元测试 (覆盖正常路径 + 路径遍历攻击 + INDEX 截断)
  - 构造函数 `NewFileMemoryStore(basePath string, cfg MemoryConfig) (FileMemoryStore, error)`:
    - 规范化 basePath (处理 `~/` 前缀)
    - 读取 cfg 中的 max_index_lines/max_index_bytes 默认值 200/25000
    - 创建根目录 (如不存在)

  **Must NOT do**:
  - 不要在此任务中实现任何 Hook 或 Tool (后续任务 W2.2/W2.3)
  - 不要在 filememory.go 中引用 `hook` 或 `tool` 包 (模块边界)
  - 不要使用 `go-git` 做版本控制 (Letta MemFS 用 git,但 CopCon 不需要——增加依赖)
  - 不要实现任何向量嵌入逻辑 (MD 文件记忆不依赖向量)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 实现 file-based 存储涉及路径安全、YAML 解析、目录权限、INDEX 截断等边界,需要仔细
  - **Skills**: `[]` (空)

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W3 知识库 agent 并行,但与 W2.2-W2.5 串行)
  - **Parallel Group**: Wave 2A (依赖 W1.1 MemoryStore,与 Wave 2B 知识库 agent 并行)
  - **Blocks**: W2.2 (hook 用 filememory.FileMemoryStore)、W2.3 (tools 用)、W2.4 (capability)、W2.5 (test)
  - **Blocked By**: W1.1

  **References**:
  - `core/storage/memory.go` (W1.1 后版本) — 必须实现的 `storage.MemoryStore` 接口 (8 个方法: Store/Search/GetBySession/DeleteBySession/List/Get/Update/Delete)
  - `core/storage/provider.go` (W1.2 后版本) — 注意: `StoreProvider.Memory()` 返回 `storage.MemoryStore`,所以 filememory 必须既是 MemoryStore (CRUD 接口) 又提供文件级扩展 (FileMemoryStore),后者由 Tool 层直接依赖
  - `server/internal/config/config.go:MemoryConfig` (W1.5 后版本) — 构造函数接收此配置
  - **外部参考**: Letta MemFS 的 system/ vs non-system/ 分层加载模式 (`https://docs.letta.com/letta-code/memfs` 或通过 librarian 查询)
  - **外部参考**: Claude Code MEMORY.md 的 200 行/25KB 限制 (Anthropic 文档)
  - **为何这样设计**: FileMetadata 单独作为 struct,方便 JSON 序列化 (用于 API) 和 YAML 序列化 (frontmatter);路径安全 4 重检查是 Anthropic memory tool 的标准防护;`max_index_lines/bytes` 配置化而非硬编码

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./providers/filememory/...` — PASS
  - [ ] `cd core && go test ./providers/filememory/...` — PASS (全部单元测试)
  - [ ] 实现满足 `storage.MemoryStore` 全部 8 个方法 (编译期检查: `var _ storage.MemoryStore = (*FileMemoryStore)(nil)`)
  - [ ] 单元测试覆盖: 创建/读取/删除 MD 文件;YAML frontmatter 解析;INDEX.md 生成;200 行截断;`..` 拒绝;符号链接拒绝
  - [ ] 所有文件权限 0o600,目录权限 0o700

  **QA Scenarios**:

  ```
  Scenario: Agent 目录结构按需创建
    Tool: bash
    Steps:
      1. 准备空临时目录 /tmp/fm-test
      2. 调用 EnsureAgentDirs("/tmp/fm-test/agent-1")
      3. 断言 system/ knowledge/ archive/ 三个子目录存在
      4. 断言目录权限为 0o700 (ls -ld)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.1-dir-creation.txt
  ```

  ```
  Scenario: 路径遍历攻击被拒绝
    Tool: bash
    Steps:
      1. 调用 WriteFile 传入 relPath "../../../etc/passwd"
      2. 调用 ReadFile 传入 relPath "/etc/passwd"
      3. 调用 WriteFile 传入 relPath ".env"
      4. 三个调用全部返回 error
    Expected Result: error 信息包含 "invalid path" 或类似
    Evidence: .sisyphus/evidence/task-W2.1-path-validation.txt
  ```

  ```
  Scenario: INDEX.md 超过 200 行被截断
    Tool: bash
    Steps:
      1. 在 knowledge/ 下创建 250 个 MD 文件,每个有 description
      2. 调用 BuildIndex
      3. 断言返回内容不超过 200 行且不超过 25KB
      4. 断言末尾有 "truncated" warning 注释
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.1-index-truncation.txt
  ```

  ```
  Scenario: YAML frontmatter 双向序列化
    Tool: bash
    Steps:
      1. SerializeFrontmatter(meta, "content") 生成 []byte
      2. ParseFrontmatter(同上 []byte) 解析回 meta + content
      3. 断言 meta 字段完全一致;content == "content"
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.1-frontmatter-roundtrip.txt
  ```

  **Commit**: YES
  - Message: `feat(filememory): implement file-based MemoryStore with YAML frontmatter + INDEX.md`
  - Files: `core/providers/filememory/*.go` (6 个文件 + test)
  - Pre-commit: `cd core && go test ./providers/filememory/...`

- [x] W2.2 FileMemoryHook (OnSystemPrompt 注入 MD 文件)

  **What to do**:
  - 新建 `core/capabilities/hooks/file_memory.go`:
    ```go
    type FileMemoryHook struct {
      fileStore filememory.FileMemoryStore
      logger    *slog.Logger
    }

    func NewFileMemoryHook(fileStore filememory.FileMemoryStore) *FileMemoryHook {...}

    func (h *FileMemoryHook) Name() string     { return "file_memory" }
    func (h *FileMemoryHook) Points() []hook.HookPoint { return []hook.HookPoint{hook.OnSystemPrompt} }
    func (h *FileMemoryHook) Priority() int    { return 80 }  // 高于现有 hooks.memory (默认 100) 先注入

    func (h *FileMemoryHook) Execute(ctx *hook.HookContext) error {
      // 1. 检查 ctx.AgentID,如果没有则 log + 返回 nil
      // 2. 读取 system/ 目录下所有 *.md 文件
      // 3. 读取 INDEX.md (filememory 已经处理了截断)
      // 4. 组装注入文本:
      //    "## Agent Memory\n\n" +
      //    "### System Context\n" +  // system/ 文件内容,每个文件一个 ### 子段
      //    "--- file: persona.md ---\n" + fileContent + "\n" + ...
      //    "### Memory Index\n" +  // INDEX.md 内容
      //    indexContent + "\n"
      //    "### Memory Protocol\n" +
      //    "You have a persistent file-based memory system. Use memory_store/memory_recall/memory_forget tools to manage it."
      // 5. 修改 *ctx.SystemPrompt := *ctx.SystemPrompt + "\n\n" + 注入文本
      return nil
    }
    ```
  - 错误处理: 任一读取失败 (路径不存在/权限问题) 仅 `logger.Warn` + continue,不返回 error (保持 hook 失败不阻断 pipeline 的现有行为)
  - 空值处理: 如果 filememory 的 system/ 目录为空且 INDEX.md 不存在,跳过注入 (不添加无效的 "## Agent Memory" 段)

  **Must NOT do**:
  - 不要在此 hook 中读取 knowledge/ 目录 (on-demand 加载由 memory_recall tool 处理)
  - 不要让 hook 返回 error 阻断 LLM 调用 (即使文件读取失败也只是 log warning)
  - 不要修改现有 `hooks/memory.go` (保留现状,W3.5 的 KBRecallHook 会处理 vector 检索)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单一 hook 实现,有明确参考 (todo_injection.go)
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: 与 W2.3 (tools) 并行可能冲突 (都修改 hook/tool 文件);建议串行
  - **Blocks**: W2.4 (capability 注册)、W2.5 (test)
  - **Blocked By**: W2.1 (FileMemoryStore 存在)

  **References**:
  - `core/capabilities/hooks/todo_injection.go:14-122` — **核心参考**! 这是最接近的参考实现: 同样 `OnSystemPrompt` + `Priority()` + 修改 `*ctx.SystemPrompt`;照这个模式做
  - `core/hook/hook.go:72-97` — HookContext 字段完整定义,`SystemPrompt *string` 是关键
  - `core/capabilities/registry.go:48-51` — HookCapability 接口,要求 NewHook(deps) (hook.Hook, error)
  - `core/providers/filememory/filememory.go` (W2.1 实现) — 调用 `ListFiles(ctx, agentID, "system/")` 和 `GetIndex(ctx, agentID)`
  - **为何这样设计**: Priority=80 让它跑在其他 hook 之前 (数字越大越先跑,根据 hook/runner.go);注入文本包含 "Memory Protocol" 段让 agent 知道如何使用工具

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./capabilities/hooks/...` — PASS
  - [ ] FileMemoryHook 实现 `hook.Hook` 接口 (编译期检查)
  - [ ] hook 在 system/ 为空时**不**注入任何内容
  - [ ] hook 在 INDEX.md 有内容时正确注入
  - [ ] 读取失败时仅 log warning,不返回 error

  **QA Scenarios**:

  ```
  Scenario: FileMemoryHook 注入 system prompt 包含完整内容
    Tool: bash
    Steps:
      1. 准备临时 filememory 存储,在 system/ 下放 persona.md (内容 "You are X"),INDEX.md 放 2 行
      2. 准备 mock HookContext (SystemPrompt = "base prompt")
      3. 调用 hook.Execute(ctx)
      4. 断言 *ctx.SystemPrompt 包含 "base prompt" + "## Agent Memory" + "You are X" + "- [name]"
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.2-inject-full.txt
  ```

  ```
  Scenario: FileMemoryHook 跳过空目录
    Tool: bash
    Steps:
      1. 准备空 filememory 存储 (无 system/ 文件,无 INDEX.md)
      2. HookContext.SystemPrompt = "base"
      3. 调用 Execute
      4. 断言 *ctx.SystemPrompt == "base" (未改变)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.2-skip-empty.txt
  ```

  **Commit**: YES
  - Message: `feat(hooks): add FileMemoryHook for OnSystemPrompt MD injection`
  - Files: `core/capabilities/hooks/file_memory.go`
  - Pre-commit: `cd core && go build ./capabilities/hooks/...`

- [x] W2.3 Memory Tools (store/recall/forget)

  **What to do**:
  - 新建 3 个文件,每个文件 1 个 Tool,都实现 `tool.Tool` 接口:
    - `core/capabilities/tools/memory_store.go` — `memory_store` tool:
      - InputSchema: `{content: string, category: enum[user|feedback|project|reference], name?: string, importance?: number 0-1}`
      - Execute:
        1. 根据 `name` 生成 relPath: `knowledge/<name>.md` (如未提供 name,用 `feedback_<timestamp>` 之类自动生成)
        2. 构造 FileMetadata
        3. 调用 `fileStore.WriteFile` + `fileStore.UpdateIndex`
        4. 返回 `ToolResult{Success: true, Data: "Stored to knowledge/<name>.md"}`
      - 错误: 文件名冲突时返回 `ToolResult{Error: "file already exists: knowledge/x.md"}`
    - `core/capabilities/tools/memory_recall.go` — `memory_recall` tool:
      - InputSchema: `{query: string, category?: string, limit?: int (默认 5)}`
      - Execute:
        1. 调用 `fileStore.ListFiles(ctx, agentID, "knowledge/")` + `fileStore.ListFiles(ctx, agentID, "archive/")`
        2. 读取每个 MD 文件,ParseFrontmatter 拿 description + 正文
        3. **关键词匹配**: 在 description + content 中查找 query (大小写不敏感子串匹配,不用 embedding)
        4. 按匹配次数排序,返回 top `limit` 个结果
        5. 格式化为: `Found N memories:\n\n- [name](path): description\n  > content excerpt...`
      - 空结果: "No memories matched the query." 不是错误,而是 Success + 空数据
    - `core/capabilities/tools/memory_forget.go` — `memory_forget` tool:
      - InputSchema: `{name?: string, path?: string}` (二选一)
      - Execute:
        1. 解析 path (或构造 `knowledge/<name>.md`)
        2. 调用 `fileStore.DeleteFile`
        3. 调用 `fileStore.RemoveFromIndex`
        4. 返回成功;如果文件不存在返回错误
  - 每个 Tool 的依赖注入:`New<Tool>(fileStore filememory.FileMemoryStore)` 构造函数

  **Must NOT do**:
  - 不要使用 embedding/向量检索 (memory_recall 仅用关键词匹配,避免 LLM/embedding 依赖)
  - 不要让 memory_store 覆盖现有文件 (冲突就报错,让 LLM 重试或选择新 name)
  - 不要在这 3 个 tool 中调用 vector KnowledgeStore (这是独立的 RAG 能力,W3.x 负责)
  - 不要在 `memory_store` 中强制 category (允许空,默认为 "user")

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 3 个工具的交互逻辑需要仔细设计,涉及错误处理、输入校验、输出格式;但无复杂算法
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: 与 W2.2 串行更安全 (都操作 hooks/ 与 tools/ 目录,但实际不冲突);本任务 3 个文件串行即可
  - **Blocks**: W2.4 (capability 注册)、W2.5 (test)
  - **Blocked By**: W2.1 (FileMemoryStore 接口) + W1.3 (Embedder 接口不需要,本任务不用)

  **References**:
  - `core/tool/manager.go:30-35` — Tool 接口: `Name()/Description()/InputSchema()/Execute()`
  - `core/capabilities/tools/todo.go` (grep 查找具体位置) — **核心参考**! 现有 todo tool 是最接近的参考实现: InputSchema 定义、参数解析、ToolResult 构造模式
  - `core/capabilities/tools/helpers.go` — 可能包含公共工具 (如参数验证),值得 grep 看
  - `core/providers/filememory/filememory.go` (W2.1 实现) — FileMemoryStore 的文件级方法 (WriteFile/ListFiles/DeleteFile/UpdateIndex 等)
  - **外部参考**: Anthropic Memory Tool Protocol (`BetaMemoryTool20250818` 的 CRUD namespace 设计)
  - **为何这样设计**: memory_recall 用关键词匹配而非 embedding,符合用户"不依赖 LLM"要求;memory_store 不覆盖防止 LLM 误操作;memory_forget 必须同时删文件和移除索引,避免 INDEX.md 悬挂引用

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./capabilities/tools/...` — PASS
  - [ ] 3 个 Tool 都实现 `tool.Tool` 接口 (编译期检查)
  - [ ] memory_store InputSchema 包含 required=content
  - [ ] memory_recall 无匹配时返回 Success + 空数据 (不是 Error)
  - [ ] memory_forget 删除不存在的文件返回 Error (明确错误信息)

  **QA Scenarios**:

  ```
  Scenario: memory_store happy path
    Tool: bash
    Steps:
      1. 准备空 filememory 存储
      2. 调用 memory_store.Execute(args={content: "hello", category: "feedback", name: "foo"})
      3. 断言 knowledge/foo.md 文件存在
      4. 断言 INDEX.md 包含 "- [feedback](knowledge/foo.md)"
      5. 断言 ToolResult.Success == true
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.3-store-happy.txt
  ```

  ```
  Scenario: memory_store 文件名冲突报错
    Tool: bash
    Steps:
      1. 先调用 memory_store 创建 knowledge/foo.md
      2. 再次调用 memory_store 创建同名 knowledge/foo.md,参数 name="foo"
      3. 第二次调用返回 ToolResult.Success=false + ToolResult.Error 含 "already exists"
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.3-store-conflict.txt
  ```

  ```
  Scenario: memory_recall 大小写不敏感匹配
    Tool: bash
    Steps:
      1. 存储一个 content = "User loves PYTHON programming"
      2. 调用 memory_recall.Execute(args={query: "python"})
      3. 断言返回结果包含该记忆
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.3-recall-case-insensitive.txt
  ```

  ```
  Scenario: memory_forget 同时删除文件和 INDEX 条目
    Tool: bash
    Steps:
      1. 存储 foo.md + 更新 INDEX
      2. 调用 memory_forget.Execute(args={name: "foo"})
      3. 断言 knowledge/foo.md 被移除
      4. 断言 INDEX.md 不再包含 foo
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.3-forget-cleanup.txt
  ```

  **Commit**: YES
  - Message: `feat(tools): implement memory_store/recall/forget tools for MD file ops`
  - Files: `core/capabilities/tools/memory_store.go`, `core/capabilities/tools/memory_recall.go`, `core/capabilities/tools/memory_forget.go`
  - Pre-commit: `cd core && go build ./capabilities/tools/...`

- [x] W2.4 Memory Capability 注册 (bundle + init)

  **What to do**:
  - 新建 `core/capabilities/hooks/file_memory_capability.go`:
    ```go
    type fileMemoryHookCapability struct{}

    func (c *fileMemoryHookCapability) Name() string                         { return "hooks.file_memory" }
    func (c *fileMemoryHookCapability) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeHook }
    func (c *fileMemoryHookCapability) DependsOn() []string                  { return nil }
    func (c *fileMemoryHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
      // 检查 deps.FileMemoryStore (需要扩展 CapabilityDeps,见下)
      if deps.FileMemoryStore == nil {
        return nil, fmt.Errorf("hooks.file_memory: FileMemoryStore dependency not available")
      }
      return NewFileMemoryHook(deps.FileMemoryStore), nil
    }

    func init() {
      capabilities.Register(&fileMemoryHookCapability{})
    }
    ```
  - 类似地,新建 `core/capabilities/tools/memory_store_capability.go` / `memory_recall_capability.go` / `memory_forget_capability.go`,每个文件一个 Capability:
    - Name: `tools.memory_store` / `tools.memory_recall` / `tools.memory_forget`
    - Type: `CapabilityTypeTool`
    - NewTool(deps) 返回对应 Tool 实例
  - **重要**: 扩展 `core/capabilities/registry.go:CapabilityDeps` struct 增加 `FileMemoryStore filememory.FileMemoryStore` 字段 (需要同时 import `core/providers/filememory`)
  - **重要**: 在 `core/harness.go:Build()` 中,当 `spec.Memory.Enabled == true` 时,实例化 `FileMemoryStore` 并传入 `CapabilityDeps.FileMemoryStore`;本任务只需准备 hook/tool 注册的代码,harness 集成在 W4.2 完成
  - 在 `core/capabilities/bundle.go` 中 `MemoryBundleNames()` 返回 4 个 capability name (之前定义的是 5 个,现在确认是 4 个:`hooks.file_memory`、`tools.memory_store`、`tools.memory_recall`、`tools.memory_forget`;不包含 `hooks.memory`)

  **Must NOT do**:
  - 不要修改 `core/harness.go` (那是 W4.1/W4.2 的工作)
  - 不要在此任务中实例化 FileMemoryStore (Harness.Build 在 W4.2 会做)
  - 不要让 capability 直接依赖 `server/internal/config/` (模块边界)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 引用 todo_injection_capability 模式,简单重复
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: 与 W2.2/W2.3 串行 (都改 capabilities/ 目录)
  - **Blocks**: W4.1 (Harness bundle 展开), W2.5 (test 用 capability name 验证)
  - **Blocked By**: W2.1, W2.2, W2.3

  **References**:
  - `core/capabilities/hooks/todo_injection.go:111-122` — **核心参考**! todo_injection capability 注册 + init 的样板代码
  - `core/capabilities/registry.go:52-63` — CapabilityDeps struct,本任务扩展它
  - `core/capabilities/bundle.go` (W1.6 实现) — MemoryBundleNames() 函数,本任务可能需要调整返回值
  - `core/harness.go:160-211` — Harness.Build() 如何处理 capability + HookCapability (参考,W4.2 会修改此段)
  - **为何这样设计**: 每个 tool 一个 capability 文件,避免单文件过大;capability.Name() 与 tools 名一致 (`tools.memory_store`),让 Harness 通过 name 查找

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./capabilities/...` — PASS
  - [ ] 4 个 capability 都通过 `init()` 注册到全局 builtins
  - [ ] `capabilities.ListByType(CapabilityTypeHook)` 包含 `hooks.file_memory`
  - [ ] `capabilities.ListByType(CapabilityTypeTool)` 包含 `tools.memory_store`, `tools.memory_recall`, `tools.memory_forget`
  - [ ] `CapabilityDeps` 包含 `FileMemoryStore` 字段

  **QA Scenarios**:

  ```
  Scenario: init() 自动注册 4 个 memory capability
    Tool: bash
    Steps:
      1. 在 main 或测试中 import `core/capabilities/hooks` + `core/capabilities/tools` (用 `_`)
      2. 调用 capabilities.Get("hooks.file_memory")
      3. 调用 capabilities.Get("tools.memory_store")
      4. 调用 capabilities.Get("tools.memory_recall")
      5. 调用 capabilities.Get("tools.memory_forget")
      6. 全部 ok=true
    Expected Result: 4 个 capability 都可查到
    Evidence: .sisyphus/evidence/task-W2.4-init-register.txt
  ```

  ```
  Scenario: MemoryBundleNames 返回正确的 4 个
    Tool: bash
    Steps:
      1. 调用 capabilities.MemoryBundleNames()
      2. 断言返回 ["hooks.file_memory", "tools.memory_store", "tools.memory_recall", "tools.memory_forget"] (或任意顺序)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.4-bundle-names.txt
  ```

  **Commit**: YES
  - Message: `feat(capabilities): register memory capability bundle with init()`
  - Files: `core/capabilities/hooks/file_memory_capability.go`, `core/capabilities/tools/memory_store_capability.go`, `core/capabilities/tools/memory_recall_capability.go`, `core/capabilities/tools/memory_forget_capability.go`, `core/capabilities/registry.go` (扩展), `core/capabilities/bundle.go`
  - Pre-commit: `cd core && go test ./capabilities/...`

- [x] W2.5 记忆能力单元测试 (综合)

  **What to do**:
  - 为 W2.1-W2.4 的所有组件编写**集成式**单元测试:
    - `core/providers/filememory/integration_test.go` — 端到端模拟 agent 使用记忆:
      1. 创建 FileMemoryStore (指向临时目录)
      2. 构造 FileMemoryHook + 注入 mock system prompt
      3. 调用 memory_store tool (用 mock ChatContext)
      4. 再次构造 hook,断言 system prompt 包含新记忆
      5. 调用 memory_recall 工具,验证能找回
      6. 调用 memory_forget,验证被删除且 hook 不再注入
    - `core/capabilities/hooks/file_memory_test.go` — hook 的 mock test:
      - 空目录不注入
      - 单文件正确注入
      - 读取失败 log 但不报错
    - `core/capabilities/tools/memory_tools_test.go` — 3 个 tool 的单元测试:
      - memory_store happy + conflict + invalid category
      - memory_recall happy + 空匹配 + case insensitive
      - memory_forget happy + not found
  - 所有测试使用临时目录 (`t.TempDir()`),不污染实际文件系统
  - 测试覆盖边界:
    - INDEX.md 200 行截断
    - YAML frontmatter 损坏 (手动写入非法 YAML,读取时优雅降级)
    - 路径遍历攻击 (多个用例: ..、.dot、symlink、绝对路径)
    - 大文件性能: 写 100 个 MD 文件,验证读取时间 < 1s

  **Must NOT do**:
  - 不要在测试中使用外部网络 (mock 所有)
  - 不要在集成 test 中引入真实 Qdrant/PG (memory 完全本地)
  - 不要修改 W2.1-W2.4 的实现代码来"通过测试" (如果测试失败意味着实现有问题,应修复实现)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 综合集成测试需要覆盖多个交互场景,需要思考完整用例矩阵
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (W2.1-W2.4 必须先完成才能写测试)
  - **Blocks**: FINAL F2 (代码质量审查验证测试覆盖)
  - **Blocked By**: W2.1, W2.2, W2.3, W2.4

  **References**:
  - `core/providers/filememory/` (W2.1 所有文件) — 测试对象
  - `core/capabilities/hooks/file_memory.go` (W2.2) — 测试对象
  - `core/capabilities/tools/memory_*.go` (W2.3) — 测试对象
  - `core/harness_test.go` — 参考现有集成测试风格 (构造 Harness + 验证行为)
  - **为何这样设计**: 集成测试验证记忆系统的端到端工作 (模拟 agent 真实使用),单元测试验证组件隔离;两者都要

  **Acceptance Criteria**:
  - [ ] `cd core && go test -v ./providers/filememory/...` — PASS,覆盖 ≥ 5 个端到端场景
  - [ ] `cd core && go test -v ./capabilities/hooks/...` — PASS,含 file_memory 测试 (至少 5 case)
  - [ ] `cd core && go test -v ./capabilities/tools/...` — PASS,3 个 tool 全覆盖 (至少 10 case)
  - [ ] `cd core && go test -cover ./providers/filememory/...` — 覆盖率 ≥ 85%
  - [ ] `cd core && go test -race ./providers/filememory/... ./capabilities/hooks/... ./capabilities/tools/...` — 无 race

  **QA Scenarios**:

  ```
  Scenario: 端到端记忆循环 (store + recall + forget)
    Tool: bash
    Steps:
      1. 执行 cd core && go test -run TestMemoryEndToEnd ./providers/filememory/... -v
      2. 测试内部:创建 → hook 注入 → recall → forget → hook 不再注入
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W2.5-end-to-end.txt
  ```

  ```
  Scenario: 全部 memory 测试 PASS
    Tool: bash
    Steps:
      1. 执行 cd core && go test ./providers/filememory/... ./capabilities/hooks/ -run "FileMemory|MemoryTool" -count=1
      2. 输出 PASS
      3. 执行 cd core && go test -cover ./providers/filememory/...
      4. coverage >= 85%
    Expected Result: all PASS,coverage OK
    Evidence: .sisyphus/evidence/task-W2.5-coverage.txt
  ```

  ```
  Scenario: 路径安全测试集
    Tool: bash
    Steps:
      1. 执行 cd core && go test -run TestPathValidation ./providers/filememory/... -v
      2. 测试内部:.., .env, 绝对路径, symlink escape (每个子 case 一个 subtest)
    Expected Result: PASS (所有攻击都被拒绝)
    Evidence: .sisyphus/evidence/task-W2.5-path-security.txt
  ```

  **Commit**: YES
  - Message: `test(memory): comprehensive filememory + hooks + tools unit tests`
  - Files: `core/providers/filememory/integration_test.go`, `core/capabilities/hooks/file_memory_test.go`, `core/capabilities/tools/memory_tools_test.go`
  - Pre-commit: `cd core && go test ./providers/filememory/... ./capabilities/hooks/... ./capabilities/tools/...`



### Wave 2B — W3: 知识库能力 (RAG,单 agent 串行,与 Wave 2A 并行)

- [x] W3.1 sqlite-vec KnowledgeStore 实现 (默认 + 唯一 backend)

  **What to do**:
  - 新建目录 `core/providers/sqlitevec/`:
    - `knowledge.go` — 实现 `storage.KnowledgeStore` 接口的全部 11 个方法 (W1.2 定义):
      - `CreateKB/DeleteKB/ListKBs/GetKB` — 管理 knowledge_bases 表
      - `IngestDocument` — 接收原始字节 (PDF/MD/TXT/HTML 解析不在本任务,由 W3.4 管线传入已解析文本,本任务只存储)
        - 但 IngestDocument 签名接收 `[]byte content`,所以内部需要调用 parser (W3.2 提供);本任务可先用 stub parser,后续集成
      - `ListDocuments/DeleteDocument/GetDocument` — documents 表 CRUD
      - `GetChunks/UpdateChunk` — chunks 表 CRUD
      - `Search(kbIDs []string, query []float32, opts SearchOptions) ([]*Chunk, error)`:
        - 构造 SQL: `SELECT * FROM vec_chunks WHERE kb_id IN (...) AND vec MATCH ? ORDER BY distance LIMIT ?`
        - 使用 sqlite-vec 的 `vec_distance_cosine` 函数做相似度排序
        - 应用 `opts.TopK` + `opts.SimilarityThreshold` (距离 > 1-threshold 时过滤)
        - 应用 `opts.Filters` (map[string]any,映射到 chunks.metadata 的 JSON 查询)
    - `schema.go` — SQL schema 定义:
      ```sql
      -- 知识库元信息
      CREATE TABLE IF NOT EXISTS knowledge_bases (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        backend TEXT NOT NULL DEFAULT 'sqlite-vec',
        config_json TEXT,
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL,
        metadata_json TEXT
      );

      -- 文档元信息
      CREATE TABLE IF NOT EXISTS documents (
        id TEXT PRIMARY KEY,
        kb_id TEXT NOT NULL,
        filename TEXT NOT NULL,
        source TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'pending',
        chunk_count INTEGER DEFAULT 0,
        token_count INTEGER DEFAULT 0,
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL,
        metadata_json TEXT,
        FOREIGN KEY (kb_id) REFERENCES knowledge_bases(id) ON DELETE CASCADE
      );
      CREATE INDEX IF NOT EXISTS idx_documents_kb_id ON documents(kb_id);

      -- chunks 文本元信息
      CREATE TABLE IF NOT EXISTS chunks (
        id TEXT PRIMARY KEY,
        document_id TEXT NOT NULL,
        kb_id TEXT NOT NULL,
        content TEXT NOT NULL,
        context TEXT DEFAULT '',  -- 为 Contextual Retrieval 预留
        chunk_index INTEGER NOT NULL,
        token_count INTEGER DEFAULT 0,
        metadata_json TEXT,
        FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
        FOREIGN KEY (kb_id) REFERENCES knowledge_bases(id) ON DELETE CASCADE
      );
      CREATE INDEX IF NOT EXISTS idx_chunks_document ON chunks(document_id);
      CREATE INDEX IF NOT EXISTS idx_chunks_kb ON chunks(kb_id);
      ```
      - sqlite-vec 虚拟表:`CREATE VIRTUAL TABLE IF NOT EXISTS vec_chunks USING vec0(id TEXT PRIMARY KEY, vec FLOAT[<N>])` (维度 N 取决于 Embedder.Dimensions())
    - `vector.go` — float32 切片 ↔ sqlite-vec blob 转换:
      - `vecToBlob(v []float32) []byte` (小端 float32 拼接)
      - `blobToVec(b []byte, dims int) []float32`
      - 参考 https://alexgarcia.xyz/sqlite-vec/go.html 示例
    - `init.go` — provider 自注册:
      ```go
      func init() {
        storage.RegisterKnowledgeStoreProvider("sqlite-vec", func(config map[string]any) (storage.KnowledgeStore, error) {
          sqlitePath, _ := config["sqlite_path"].(string)
          return NewSQLiteVecStore(sqlitePath, config)
        })
      }
      ```
    - `knowledge_test.go` — 单元测试 (in-memory sqlite-vec):
      - KB CRUD
      - Document CRUD + status lifecycle
      - Chunk CRUD + 向量相似度查询 (写入 10 个 chunk 已知向量,查询新向量,验证 top-3 正确)
      - Cascade delete (删 KB → documents/chunks 也删)
    - `README.md` — 简要说明用法 (参考 sqlite-fallback.md 文档风格)
  - 新增依赖:`github.com/ncruces/go-sqlite3` (纯 Go,无 CGO) + sqlite-vec 扩展 (需 grep 确认 ncruces 是否原生支持 sqlite-vec,或需使用 `sqlite3_vec` 子包)
  - **注意**: 如果 ncruces/go-sqlite3 不直接支持 sqlite-vec,可能需要使用 alternative 或自行编译;此任务的 deep agent 需要先调研可行性

  **Must NOT do**:
  - 不要在 sqlite-vec 实现中引用任何 Qdrant 代码 (即使 Qdrant provider 已存在)
  - 不要把 `vector.go` 的转换函数硬编码 1024 维度 (用 Embedder.Dimensions() 查询)
  - 不要在本任务中实现文档解析或分块 (W3.2/W3.3 的工作;本任务的 IngestDocument 可先用 stub)
  - 不要忘记用 `db.Close()` 在测试中正确清理

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: sqlite-vec 是新依赖,需要先调研可行性;schema 设计影响所有后续任务;向量转换正确性至关重要
  - **Skills**: `[]` (空,但 agent 需要主动 grep librarian 查 sqlite-vec + ncruces/go-sqlite3 文档)

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W2 记忆 agent 完全并行,但与 W3.2-W3.8 串行)
  - **Parallel Group**: Wave 2B
  - **Blocks**: W3.2 (parser 测试可独立进行), W3.4 (入库管线用 IngestDocument), W3.5 (KBRecallHook 用 Search), W3.8 (test)
  - **Blocked By**: W1.2 (KnowledgeStore 接口)、W1.7 (RegisterKnowledgeStoreProvider)

  **References**:
  - `core/storage/knowledge.go` (W1.2 实现) — 必须实现的全部 11 个方法
  - `core/storage/knowledge_registry.go` (W1.7 实现) — init() 注册调用 `RegisterKnowledgeStoreProvider("sqlite-vec", ...)`
  - **外部参考**: https://alexgarcia.xyz/sqlite-vec/go.html — sqlite-vec Go 使用教程(必读)
  - **外部参考**: https://github.com/ncruces/go-sqlite3 — 纯 Go sqlite3 实现,README 含扩展加载示例
  - **外部参考**: `core/providers/qdrant/memory.go` — 参考现有 Qdrant MemoryStore 实现风格 (注意本任务实现的是 KnowledgeStore,方法集不同)
  - `core/storage/provider.go` — 理解 StoreProvider.Knowledge() 的预期返回 (nil OK)
  - **为何这样设计**: in-memory sqlite 作为测试模式避免污染文件系统;schema 把 metadata 用 JSON 列存储,灵活且不需要 migration 每个新字段;chunks.metadata_json + filters map 让 SearchOptions.Filters 实现灵活 (后续可加索引)

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./providers/sqlitevec/...` — PASS,无 CGO 警告
  - [ ] `cd core && go test ./providers/sqlitevec/...` — PASS 所有单元测试
  - [ ] `go mod tidy` 拉入 `github.com/ncruces/go-sqlite3` 依赖
  - [ ] 实现可被 `var _ storage.KnowledgeStore = (*SQLiteVecStore)(nil)` 静态断言
  - [ ] Vector 相似度查询测试: 写入 10 个已知向量,查询已知最近邻,返回正确 top-3 (距离 < 阈值)
  - [ ] Cascade delete: 删除 KB 后,相关 documents/chunks 也被删除 (测试断言 SELECT COUNT(*) FROM chunks WHERE kb_id=...)

  **QA Scenarios**:

  ```
  Scenario: KB 生命周期 + 文档 + chunks 端到端
    Tool: bash
    Steps:
      1. 在 in-memory sqlite 中创建 KB
      2. IngestDocument (传入 stub 文本)
      3. 验证 documents 表有 1 行, chunks 表有 N 行 (N 由 chunker 决定,本任务用 stub chunker)
      4. Search 查询返回 chunks
      5. DeleteDocument → chunks 被级联删除
      6. DeleteKB → documents 也被级联删除
    Expected Result: 6 步全 PASS
    Evidence: .sisyphus/evidence/task-W3.1-lifecycle.txt
  ```

  ```
  Scenario: Vector 相似度正确性
    Tool: bash
    Steps:
      1. 写入 10 个向量,每个向量的第 i 位 = 1 (其余 0)
      2. 查询向量: 第 0 位 = 1 (其余 0) + 轻微扰动
      3. Search TopK=3
      4. 断言 top-1 是 i=0 的 chunk,且距离 < 0.05
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.1-vector-similarity.txt
  ```

  ```
  Scenario: Filters 应用
    Tool: bash
    Steps:
      1. 写入 chunks,chunks.metadata_json 包含 {"doc_type": "pdf"} 和 {"doc_type": "md"}
      2. Search 传入 Filters = {"doc_type": "pdf"},TopK=100
      3. 断言所有返回 chunks 的 metadata.doc_type == "pdf"
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.1-filters.txt
  ```

  **Commit**: YES
  - Message: `feat(sqlitevec): implement KnowledgeStore with ncruces/go-sqlite3, pure Go, no CGO`
  - Files: `core/providers/sqlitevec/*.go`, `core/providers/sqlitevec/README.md`, `go.mod` (新增依赖)
  - Pre-commit: `cd core && go test -v ./providers/sqlitevec/...`

- [x] W3.2 文档解析器 (PDF/MD/TXT/HTML)

  **What to do**:
  - 新建目录 `core/rag/`:
    - `parser.go` — 主入口 + 接口:
      ```go
      package rag

      type Parser interface {
        Parse(ctx context.Context, content []byte, mimetype string) (string, error)
      }

      // Dispatcher 根据 mimetype 路由到具体 parser
      type DefaultParser struct {
        pdf      Parser
        markdown Parser
        text     Parser
        html     Parser
      }

      func NewDefaultParser() *DefaultParser {
        return &DefaultParser{
          pdf:      &PDFParser{},
          markdown: &MarkdownParser{},
          text:     &TextParser{},
          html:     &HTMLParser{},
        }
      }

      func (p *DefaultParser) Parse(ctx context.Context, content []byte, mimetype string) (string, error) {
        switch {
        case strings.HasPrefix(mimetype, "application/pdf"):
          return p.pdf.Parse(ctx, content, mimetype)
        case mimetype == "text/markdown" || mimetype == "text/x-markdown":
          return p.markdown.Parse(ctx, content, mimetype)
        case strings.HasPrefix(mimetype, "text/html"):
          return p.html.Parse(ctx, content, mimetype)
        case strings.HasPrefix(mimetype, "text/"):
          return p.text.Parse(ctx, content, mimetype)
        default:
          return "", fmt.Errorf("unsupported mimetype: %s", mimetype)
        }
      }
      ```
    - `pdf.go` — PDF 解析:
      - 使用 `github.com/pdfcpu/pdfcpu` 或 `github.com/dslipak/pdf` (轻量级,无 CGO)
      - 提取每页文本,拼接为单个 string,页之间加 `\n\n---\n\n` 分隔
      - 错误处理: 解密 PDF/损坏 PDF → 返回清晰 error
      - 注意: 扫描版 PDF 无法提取文本,返回 warning (不报错)
    - `markdown.go` — Markdown 解析:
      - 直接作为纯文本返回 (保留所有格式符号,后续 chunker 会按 `#` 分割)
      - 处理 UTF-8 BOM、Windows CRLF
    - `text.go` — 纯文本解析: 直接返回字符串,处理 BOM/CRLF
    - `html.go` — HTML 解析:
      - 使用 `github.com/PuerkitoBio/goquery` (Go 原生 HTML 解析)
      - 移除 `<script>`、`<style>`、`<nav>`、`<footer>` 等噪音标签
      - 保留文本内容,段落之间加 `\n\n`
      - 处理 `<h1>` 到 `<h6>` 时前缀 `#` 符号 (便于后续 chunker 识别结构)
    - `parser_test.go` — 单元测试:
      - PDF: 用 fixture PDF 文件 (放在 `testdata/`) 验证提取
      - MD: BOM 处理、CRLF 处理
      - HTML: 噪音标签移除、标题前缀
      - TXT: BOM 处理
      - 不支持的 mimetype: 返回 error

  **Must NOT do**:
  - 不要依赖 CGO (所有库必须纯 Go)
  - 不要在 parser 层做 chunking (那是 W3.3 的工作)
  - 不要实现 OCR (扫描版 PDF 直接返回 warning,不阻塞)
  - 不要实现 Word (`.docx`) 或 EPUB 解析 (不在本期范围)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 4 种格式的解析需要考虑各种边界(损坏文件、加密 PDF、非 UTF-8 编码)
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: 与 W3.3 (chunker) 并行可行 (都独立),但与 W3.1 (KnowledgeStore) 无直接依赖
  - **Blocks**: W3.4 (入库管线调用), W3.8 (test 用 fixture)
  - **Blocked By**: None (可立即启动,只要 Rag 包已建)

  **References**:
  - `core/rag/parser.go` (新建) — 接口定义
  - **外部库**: `github.com/dslipak/pdf` — 纯 Go PDF 文本提取(轻量,适合简单场景);备选 `github.com/pdfcpu/pdfcpu` (更强大)
  - **外部库**: `github.com/PuerkitoBio/goquery` — Go HTML 解析标准库
  - `encoding/xml` 或 `golang.org/x/net/html` — HTML 解析底层
  - **为何这样设计**: Parser 接口隔离,允许未来添加 Word/EPUB parser 而不改管线代码;DefaultParser 的 mimetype 路由让上层只需传 mimetype 即可,不关心具体 parser

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./rag/...` — PASS
  - [ ] `cd core && go test ./rag/...` — PASS 所有测试
  - [ ] `go.mod` 新增依赖 (`dslipak/pdf` 或 `pdfcpu`、`goquery`)
  - [ ] PDF 解析: 至少 1 个 fixture PDF 可正确提取 ≥ 1 段文本
  - [ ] HTML 解析: `<script>` 和 `<style>` 内容被移除
  - [ ] 所有 parser 处理 UTF-8 BOM 正确 (无乱码)

  **QA Scenarios**:

  ```
  Scenario: PDF 文本提取
    Tool: bash
    Steps:
      1. 准备 testdata/sample.pdf (简单 1 页,含 "Hello World" 文本)
      2. 调用 PDFParser.Parse 读取该文件
      3. 断言返回字符串包含 "Hello World"
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.2-pdf-parse.txt
  ```

  ```
  Scenario: HTML 噪音标签移除
    Tool: bash
    Steps:
      1. 构造 HTML: <html><body><script>evil</script><h1>Title</h1><p>Content</p></body></html>
      2. 调用 HTMLParser.Parse
      3. 断言输出不含 "evil",包含 "Title" 和 "Content",且 "Title" 前有 "\n# " 或类似标记
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.2-html-noise.txt
  ```

  ```
  Scenario: 不支持的 mimetype 返回错误
    Tool: bash
    Steps:
      1. 调用 DefaultParser.Parse(..., "application/zip")
      2. 断言返回 error 含 "unsupported mimetype"
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.2-unsupported-mimetype.txt
  ```

  **Commit**: YES
  - Message: `feat(rag): add document parser with PDF/MD/TXT/HTML support`
  - Files: `core/rag/parser.go`, `core/rag/pdf.go`, `core/rag/markdown.go`, `core/rag/text.go`, `core/rag/html.go`, `core/rag/parser_test.go`, `core/rag/testdata/*` (fixtures)
  - Pre-commit: `cd core && go test ./rag/...`

- [x] W3.3 分块器 (RecursiveChunker + MarkdownAwareChunker)

  **What to do**:
  - 新建 `core/rag/chunker.go`:
    ```go
    package rag

    type Chunker interface {
      Chunk(text string, opts ChunkOptions) ([]ChunkResult, error)
    }

    type ChunkOptions struct {
      ChunkSize    int    // 默认 800 (字符数,非 token 数,简化处理)
      Overlap      int    // 默认 100
      Strategy     string // "recursive" | "markdown"
    }

    type ChunkResult struct {
      Content  string
      Index    int       // 在原文件中的位置 (0-indexed)
      Metadata map[string]any  // 可选,如 markdown 的 header path
    }

    // RecursiveChunker 按段落 → 句子 → 字符 的优先级递归分割
    type RecursiveChunker struct{}

    // MarkdownAwareChunker 按 # 标题分割,每个 chunk 带上层级 header 前缀
    type MarkdownAwareChunker struct{}
    ```
  - **RecursiveChunker 算法**:
    1. 按 `\n\n` (段落) 切分
    2. 如果单个段落长度 > chunk_size,按句子切分 (`.`, `!`, `?` + 空格)
    3. 如果单个句子仍 > chunk_size,按字符硬切 (每 chunk_size 一段,保留 overlap 字符交叠)
    4. 合并相邻段落,直至接近 chunk_size 但不超过
    5. 输出时应用 overlap: 前后 chunk 共享 overlap 字符
  - **MarkdownAwareChunker 算法**:
    1. 按 `#`, `##`, `###` 等标题分行
    2. 每个 section (从某个 `##` 开始到下一个 `##` 之前) 作为一个 chunk
    3. 如果 section 仍 > chunk_size,fallback 到 RecursiveChunker
    4. 每个 chunk 在开头插入 header path,如 `# 主标题 / ## 子标题\n\n` (便于检索时了解位置)
  - 单元测试 `chunker_test.go`:
    - 短文本(< chunk_size)直接返回 1 chunk
    - 长文本按 chunk_size + overlap 切分,断言每个 chunk 长度合理
    - Markdown 多级标题正确解析
    - overlap 测试: 第 i 个 chunk 末尾 == 第 i+1 个 chunk 开头 (前 overlap 字符)
    - 边界: 空字符串、纯空格、全是 `\n` 的字符串

  **Must NOT do**:
  - 不要把 ChunkSize 作为 token 数处理 (简化为字符数;真实 token 数由 Embedder 决定)
  - 不要依赖 embedding 来判断 chunk 边界 (用户明确不做 LLM/embedding 相关)
  - 不要在 chunker 中写入数据库 (那是 W3.4 管线的工作)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 算法明确,有清晰的边界条件测试
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: 与 W3.2 并行 (都独立)
  - **Blocks**: W3.4 (管线使用)、W3.8
  - **Blocked By**: None

  **References**:
  - https://docs.llamaindex.ai/en/stable/module_guides/loading/node_parsers/modules/ — LlamaIndex 的 SentenceSplitter/MarkdownSplitter 参考 (外部调研)
  - `core/rag/parser.go` (W3.2 实现) — chunker 接收 parser 的文本输出
  - **为何这样设计**: 两种 chunker 通过 Chunker interface 解耦,管线可按 mimetype 选择 (MD 文件用 MarkdownAware,其他用 Recursive);header path 注入让 chunk 在检索结果中携带结构信息 (用户可读性)

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./rag/...` — PASS
  - [ ] `cd core && go test ./rag/...` — PASS
  - [ ] RecursiveChunker 处理 5000 字符的文本,默认 chunk_size=800 overlap=100,输出 6-8 个 chunks 且每个 chunk 长度 ∈ [700, 850]
  - [ ] MarkdownAwareChunker 处理 10 section 的 MD 文档,输出 10 个 chunk,每个 chunk 开头含 header path

  **QA Scenarios**:

  ```
  Scenario: RecursiveChunker 标准切分
    Tool: bash
    Steps:
      1. 准备 2000 字符文本(20 段,每段 100 字符)
      2. 调用 RecursiveChunker.Chunk(text, {ChunkSize: 500, Overlap: 50})
      3. 断言输出 ≈ 4 chunks (2000/500),每个含 overlap
      4. 断言 chunk 拼接起来能覆盖全文 (去重 overlap 后)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.3-recursive-chunk.txt
  ```

  ```
  Scenario: MarkdownAwareChunker 多级标题
    Tool: bash
    Steps:
      1. 准备 MD 文本: # H1 \n 内容 \n ## H2a \n 内容 \n ## H2b \n 内容
      2. 调用 MarkdownAwareChunker
      3. 断言输出 3 chunks,每个 chunk 开头包含 "# H1" 或 "# H1 / ## H2x" 的 header path
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.3-markdown-chunk.txt
  ```

  ```
  Scenario: 空字符串处理
    Tool: bash
    Steps:
      1. 调用 Recursive().Chunk("", ...)
      2. 断言返回 []ChunkResult{} (空 slice,不是 nil) + error = nil
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.3-empty.txt
  ```

  **Commit**: YES
  - Message: `feat(rag): add RecursiveChunker + MarkdownAwareChunker with configurable size/overlap`
  - Files: `core/rag/chunker.go`, `core/rag/chunker_test.go`
  - Pre-commit: `cd core && go test ./rag/...`

- [x] W3.4 入库管线 (Ingestion Pipeline)

  **What to do**:
  - 新建 `core/rag/pipeline.go`:
    ```go
    package rag

    type Pipeline struct {
      parser   Parser
      chunker  Chunker  // 默认 NewRecursiveChunker,可按 mimetype 切换
      embedder embedding.Embedder
      store    storage.KnowledgeStore
    }

    type PipelineConfig struct {
      ChunkSize       int    // 默认 800
      ChunkOverlap    int    // 默认 100
      UseMarkdownAware bool   // 当 mimetype 含 markdown 时启用 MarkdownAware
    }

    func NewPipeline(parser Parser, chunker Chunker, embedder embedding.Embedder, store storage.KnowledgeStore) *Pipeline {...}

    type ProgressFunc func(current, total int, stage string)  // 可选进度回调

    // 主入口:解析 + 分块 + 嵌入 + 存储
    func (p *Pipeline) Ingest(ctx context.Context, kbID string, doc *storage.Document, content []byte, progress ProgressFunc) error {
      // Stage 1: Parse
      if progress != nil { progress(0, 4, "parsing") }
      text, err := p.parser.Parse(ctx, content, doc.Metadata["mimetype"].(string))
      if err != nil {
        // 更新 document.status = error,写入 store (让 UI 可见)
        doc.Status = storage.DocStatusError
        _ = p.markDocError(ctx, kbID, doc, err)
        return err
      }

      // Stage 2: Chunk (MD 用 MarkdownAware,其他用 Recursive)
      if progress != nil { progress(1, 4, "chunking") }
      opts := ChunkOptions{ChunkSize: 800, Overlap: 100, Strategy: "recursive"}
      chunks, err := p.chunker.Chunk(text, opts)
      if err != nil { return err }

      // Stage 3: Embed (批量)
      if progress != nil { progress(2, 4, "embedding") }
      texts := make([]string, len(chunks))
      for i, c := range chunks { texts[i] = c.Content }
      vectors, err := p.embedder.EmbedBatch(ctx, texts)
      if err != nil { return err }

      // Stage 4: Store (批量写入)
      if progress != nil { progress(3, 4, "storing") }
      for i, c := range chunks {
        chunk := &storage.Chunk{
          ID:         fmt.Sprintf("%s-%d", doc.ID, c.Index),
          DocumentID: doc.ID,
          KBID:       kbID,
          Content:    c.Content,
          Index:      c.Index,
          TokenCount: len(c.Content),  // 简化 token 估算
          Metadata:   c.Metadata,
          // Vector:    vectors[i],  -- 注意:存储接口如何传 vector 取决于 store 实现
        }
        // 调用 store 写入 (可能需要扩展 storage.Chunk 加 Vector 字段,或直接调用 store 的私有方法)
        // 这里需要与 W1.2 KnowledgeStore 接口设计协调
      }

      // 更新 document 统计信息
      doc.ChunkCount = len(chunks)
      doc.TokenCount = estimateTokens(text)
      doc.Status = storage.DocStatusReady
      // p.store.UpdateDocument(...)
      return nil
    }
    ```
  - **重要设计决策**: `storage.KnowledgeStore.IngestDocument(ctx, kbID, doc, content)` 接口的签名是接收原始字节 `content`,意味着 **store 自己负责** 调用 parser/chunker/embedder,还是 **管线** 在外层处理后只传解析结果?
    - **推荐方案**: 让 `IngestDocument` 接收原始字节,内部实现由 sqlite-vec store 调用管线 (通过依赖注入)。本任务定义管线,管线依赖 parser/chunker/embedder;sqlite-vec store 把这三个组件作为构造参数接收,在 `IngestDocument` 内调用管线
    - 这意味着 **W3.1 (sqlite-vec 实现)** 和 **W3.4 (管线)** 之间存在循环依赖;需要在 W3.1 中预留管线钩子,W3.4 完成实现
    - 实际操作: W3.1 的 `IngestDocument` 先实现为 stub (直接存原始字节 + 标记 pending),W3.4 完成后把管线集成到 W3.1 (通过 setter 或构造参数)
  - 异步处理: `Ingest` 函数本身同步执行;如需异步,suite 上层用 goroutine 包装;提供 progress 回调让上层可展示进度
  - Chunk 去重: 基于 content hash (sha256),避免重复文档重复入库
  - 错误重试: 嵌入 API 失败时指数退避 (3 次重试,初始 500ms)
  - 单元测试 `pipeline_test.go`:
    - Mock Parser、Mock Chunker、Mock Embedder、Mock KnowledgeStore
    - 验证每个 stage 被调用正确的次数
    - 验证 EmbedBatch 失败时,文档状态被标记为 error
    - 验证进度回调被调用 4 次 (4 个 stage)

  **Must NOT do**:
  - 不要在本任务中实现 Contextual Augmentation (用户明确推迟)
  - 不要实现真正的 LLM 调用 (用 mock 或 stub)
  - 不要在管线中直接访问 sqlite 数据库 (必须通过 KnowledgeStore 接口)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 设计决策复杂 (谁负责调用 parser/chunker);错误处理需要周到;进度回调 API 设计需要仔细
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖 W3.1 + W3.2 + W3.3 全部完成)
  - **Blocks**: W3.5 (KBRecallHook 用管线检索)、W3.8 (测试)
  - **Blocked By**: W3.1, W3.2, W3.3

  **References**:
  - `core/providers/sqlitevec/knowledge.go` (W3.1) — sqlite-vec store 的 IngestDocument 实现 (本任务的管线会被它调用)
  - `core/rag/parser.go` (W3.2) — Parser 接口
  - `core/rag/chunker.go` (W3.3) — Chunker 接口
  - `core/providers/embedding/embedder.go` (W1.3) — Embedder 接口
  - **为何这样设计**: 管线作为独立组件,既被 sqlite-vec store 的 IngestDocument 调用 (用户通过 API 上传),也可被其他入口直接调用 (如未来的批量重导入);progress 回调让 UI 可显示实时进度

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./rag/...` — PASS
  - [ ] `cd core && go test ./rag/...` — PASS (包含管线 mock 测试)
  - [ ] Mock Embedder 失败时,管线更新 document.status = error + 返回 error
  - [ ] 进度回调在 4 个 stage 各被调用一次
  - [ ] 集成 sqlite-vec store (用 in-memory DB) 端到端测试: 传入 PDF fixture → 解析 → 分块 → 嵌入 → 存储 → Search 可查出

  **QA Scenarios**:

  ```
  Scenario: 完整入库管线端到端
    Tool: bash
    Steps:
      1. 用 in-memory sqlite-vec store + mock embedder (返回固定 1536-dim 向量)
      2. 调用 Pipeline.Ingest(ctx, "kb1", doc, fixtureText, nil)
      3. 验证 store 中 docs 表有 1 行,chunks 表有 N 行
      4. 调用 store.Search(["kb1"], sameVector, TopK=N)
      5. 断言返回所有 chunks
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.4-pipeline-e2e.txt
  ```

  ```
  Scenario: Embedding API 失败时优雅降级
    Tool: bash
    Steps:
      1. 用 fail-once mock embedder (第一次调用返回 error)
      2. 调用 Pipeline.Ingest
      3. 断言管线重试 (embedder 被调用 ≥ 2 次) 或最终报错并标记 doc status = error
      4. 断言 doc 在 store 中的状态为 error
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.4-retry.txt
  ```

  ```
  Scenario: 进度回调被触发 4 次
    Tool: bash
    Steps:
      1. 注册 progress func,内部记录调用次数和 stage
      2. 调用 Pipeline.Ingest
      3. 断言 progress 被调用 4 次 (parse/chunk/embed/store 各一次)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.4-progress.txt
  ```

  **Commit**: YES
  - Message: `feat(rag): implement ingestion pipeline — Parse → Chunk → Embed → Store with async + progress`
  - Files: `core/rag/pipeline.go`, `core/rag/pipeline_test.go`
  - Pre-commit: `cd core && go test ./rag/...`

- [x] W3.5 KBRecallHook (AfterContextBuild 向量检索)

  **What to do**:
  - 新建 `core/capabilities/hooks/kb_recall.go`:
    ```go
    type KBRecallHook struct {
      knowledgeStore storage.KnowledgeStore
      embedder       embedding.Embedder
      agentKBs       map[string][]string  // agentID → []kbID (从 Agent.KnowledgeBases 取)
      logger         *slog.Logger
      defaultTopK    int  // 默认 5
    }

    func NewKBRecallHook(ks storage.KnowledgeStore, emb embedding.Embedder, agentKBs map[string][]string) *KBRecallHook {...}

    func (h *KBRecallHook) Name() string { return "kb_recall" }
    func (h *KBRecallHook) Points() []hook.HookPoint { return []hook.HookPoint{hook.AfterContextBuild} }
    func (h *KBRecallHook) Priority() int { return 60 }  // 在 FileMemoryHook (80) 之后跑

    func (h *KBRecallHook) Execute(ctx *hook.HookContext) error {
      // 1. 检查 ctx.AgentID 在 h.agentKBs 中是否配置了 KB,如果不是则直接 return nil (agent 未启用 KB)
      kbIDs := h.agentKBs[ctx.AgentID]
      if len(kbIDs) == 0 { return nil }

      // 2. 从 *ctx.Messages 找到最后一条 user message
      lastUserMsg := findLastUserMessage(*ctx.Messages)
      if lastUserMsg == "" { return nil }

      // 3. 用 embedder 嵌入 query
      queryVec, err := h.embedder.Embed(ctx.ChatCtx.Context(), lastUserMsg)
      if err != nil {
        h.logger.Warn("KBRecallHook: embed failed", "error", err)
        return nil  // 降级:不注入,继续 LLM 调用
      }

      // 4. 跨 kbIDs 联合检索
      chunks, err := h.knowledgeStore.Search(ctx.ChatCtx.Context(), kbIDs, queryVec, storage.SearchOptions{
        TopK: h.defaultTopK,
      })
      if err != nil {
        h.logger.Warn("KBRecallHook: search failed", "error", err)
        return nil
      }
      if len(chunks) == 0 { return nil }

      // 5. 格式化为 system message 注入到 messages 列表头部 (或 system prompt 后)
      formatted := formatRetrievalResults(chunks)
      retrievedMsg := entity.MessageForLLM{
        Role:    "system",
        Content: formatted,
      }
      // 在 messages 头部插入 (现有 hooks.memory 的做法)
      *ctx.Messages = append([]entity.MessageForLLM{retrievedMsg}, *ctx.Messages...)
      return nil
    }

    func formatRetrievalResults(chunks []*storage.Chunk) string {
      out := "## Retrieved Knowledge\n\n"
      for i, c := range chunks {
        out += fmt.Sprintf("%d. [doc: %s, chunk %d] (score: %.2f)\n%s\n\n",
          i+1, c.DocumentID[:8], c.Index, c.Score, truncate(c.Content, 400))
      }
      out += "---\nUse the above retrieved knowledge to answer the user's question accurately. If the knowledge is not relevant, say so."
      return out
    }
    ```
  - findLastUserMessage 和 truncate 是 helper 函数 (可以放到 `helpers.go` 或本文件内)
  - Priority=60 让 KB recall 跑在 FileMemoryHook (80) 之后 (数字越大越先),所以注入顺序: FileMemory 先 (system prompt 追加),KBRecall 后 (messages 前插)
  - agentKBs map 由 Harness.Build() 在 W4.2 根据 config 构造后传入

  **Must NOT do**:
  - 不要让 hook 失败阻断 LLM 调用 (任何 error 都 log + continue)
  - 不要在 hook 中调用 LLM (不调 embedding 之外的任何 LLM)
  - 不要把检索结果塞进 system prompt (注入到 messages 更合适,因为 messages 是 LLM 的 conversation context)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单一 hook,参考 FileMemoryHook 模式即可
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W3.6 MemoryPersistHook 独立)
  - **Blocks**: W3.7 (capability 注册)、W3.8 (test)
  - **Blocked By**: W3.1 (KnowledgeStore 存在)、W3.4 (管线完成让 store 有数据),W1.4 (embedder)

  **References**:
  - `core/capabilities/hooks/file_memory.go` (W2.2) — 参考同 wave 的另一个 hook
  - `core/capabilities/hooks/memory.go:52-86` — **现有 vector memory hook**! 它也是 AfterContextBuild + 改 *ctx.Messages;本任务的 KBRecallHook 与它**共存**,不是替代 (现有 memory hook 处理"记忆"存储的对话历史,KBRecallHook 处理"知识库"的外部文档)
  - `core/storage/knowledge.go` (W1.2) — KnowledgeStore.Search 接口
  - `core/hook/hook.go:99-100` — HookContext.Messages 字段
  - `core/entity/message_for_llm.go` (grep 查找) — MessageForLLM 类型
  - **为何这样设计**: 注入 messages 而非 system prompt,因为 system prompt 已被 FileMemoryHook 占用且可能较长;messages 作为额外上下文更清晰分离

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./capabilities/hooks/...` — PASS
  - [ ] KBRecallHook 实现 `hook.Hook` 接口 (编译期检查)
  - [ ] Agent 未配置 KB 时,Execute 直接 return nil (不调用 embedder)
  - [ ] Agent 配置 KB 后,Execute 触发 embed + search + 注入 messages
  - [ ] 任一 step 失败 (embed/search),log warning 但不阻断

  **QA Scenarios**:

  ```
  Scenario: agent 启用 KB 时检索并注入
    Tool: bash
    Steps:
      1. 准备 mock KnowledgeStore (Search 返回 2 chunks) + mock Embedder
      2. 准备 agentKBs = {"agent1": ["kb1"]}
      3. 准备 HookContext{AgentID: "agent1", Messages: [user msg "hello"]}
      4. 调用 Execute
      5. 断言 *ctx.Messages[0].Role == "system" 且 Content 含 "Retrieved Knowledge" + 2 条 chunks
      6. 断言 *ctx.Messages[1] 是原始 user msg
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.5-recall-inject.txt
  ```

  ```
  Scenario: agent 未配置 KB 时跳过
    Tool: bash
    Steps:
      1. agentKBs = {} (空 map)
      2. HookContext{AgentID: "agent2", Messages: [user msg]}
      3. 调用 Execute
      4. 断言 *ctx.Messages 未变 (仍只有 user msg)
      5. 断言 embedder 未被调用
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.5-skip-no-kb.txt
  ```

  ```
  Scenario: embedding 失败时优雅降级
    Tool: bash
    Steps:
      1. mock Embedder 返回 error
      2. 调用 Execute
      3. 断言返回 nil error (不阻断 hook chain)
      4. 断言 *ctx.Messages 未变
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.5-embed-fail.txt
  ```

  **Commit**: YES
  - Message: `feat(hooks): add KBRecallHook for AfterContextBuild vector retrieval across KBs`
  - Files: `core/capabilities/hooks/kb_recall.go`
  - Pre-commit: `cd core && go build ./capabilities/hooks/...`

- [x] W3.6 MemoryPersistHook (异步 + 关键词提取,无 LLM)

  **What to do**:
  - 新建 `core/capabilities/hooks/memory_persist.go`:
    ```go
    type MemoryPersistHook struct {
      memoryStore     storage.MemoryStore  // 用于存储提取的事实
      knowledgeStore  storage.KnowledgeStore  // 用于检索相似度 (避免重复提取)
      embedder        embedding.Embedder
      logger          *slog.Logger
      keywordExtractor *KeywordExtractor
    }

    type KeywordExtractor struct {
      stopWords map[string]bool  // 常见停用词
      minWordLen int  // 最短词长 (默认 3)
      maxKeywords int  // 每条消息最多提取几个关键词 (默认 5)
    }

    func NewKeywordExtractor() *KeywordExtractor {
      // 中英文混合停用词表 (常见 200 个)
      return &KeywordExtractor{
        stopWords: defaultStopWords(),
        minWordLen: 3,
        maxKeywords: 5,
      }
    }

    // Extract 返回 top-N 关键词 (按出现频率)
    func (e *KeywordExtractor) Extract(text string) []string { ... }

    func NewMemoryPersistHook(ms storage.MemoryStore, ks storage.KnowledgeStore, emb embedding.Embedder) *MemoryPersistHook {...}

    func (h *MemoryPersistHook) Name() string { return "memory_persist" }
    func (h *MemoryPersistHook) Points() []hook.HookPoint { return []hook.HookPoint{hook.OnMessagePersist} }
    func (h *MemoryPersistHook) Priority() int { return 40 }  // 低于 KBRecallHook (60)

    func (h *MemoryPersistHook) Execute(ctx *hook.HookContext) error {
      // 异步处理,不阻塞主流程
      msgs := *ctx.Messages
      chatCtx := ctx.ChatCtx
      sessionID := ctx.SessionID

      go func() {
        // 找到最后一条 assistant message
        lastAssistant := findLastAssistantMessage(msgs)
        if lastAssistant == "" { return }

        // 关键词提取
        keywords := h.keywordExtractor.Extract(lastAssistant)
        if len(keywords) == 0 { return }

        // 用 keywords 拼装 "事实陈述": "Keywords: kw1, kw2. Content: ..."
        fact := fmt.Sprintf("Keywords: %s\nContent: %s", strings.Join(keywords, ", "), truncate(lastAssistant, 500))

        // 嵌入以支持未来相似度检索 (也用于本次重复检测)
        vec, err := h.embedder.Embed(chatCtx.Context(), fact)
        if err != nil {
          h.logger.Warn("memory_persist: embed failed", "error", err)
          return
        }

        // 简单去重: 检索 Top-3,如果相似度 > 0.95 就跳过 (避免同一事实重复存储)
        existing, _ := h.memoryStore.Search(chatCtx.Context(), vec, 3)
        for _, m := range existing {
          if m.Score > 0.95 {
            h.logger.Debug("memory_persist: duplicate detected, skip", "existing_id", m.ID)
            return
          }
        }

        // 存储到 memoryStore
        err = h.memoryStore.Store(chatCtx.Context(), &storage.Memory{
          Content:    fact,
          SessionID:  sessionID,
          Role:       "assistant",
          MemoryType: storage.MemoryTypeSemantic,  // 事实陈述是语义记忆
          Metadata: map[string]any{
            "keywords": keywords,
            "source":   "memory_persist_hook",
          },
          Importance: 0.5,  // 默认中等重要度
        })
        if err != nil {
          h.logger.Warn("memory_persist: store failed", "error", err)
        }
      }()

      return nil  // 主流程立即返回
    }
    ```
  - `defaultStopWords()` 函数返回中英文混合停用词表 (常见 100-200 个): "我", "的", "是", "在", "和", "the", "and", "is", "of" 等
  - keywordExtractor 用词频统计 + TF-IDF 简化版 (仅单文档 TF)

  **Must NOT do**:
  - **绝对不要调用 LLM API** — 这是用户明确拒绝的;只用关键词统计
  - 不要在 Execute 主路径做任何网络调用 (所有 IO 在 goroutine 内)
  - 不要存储完整的 assistant message (可能很大,浪费空间);只存 truncated + keywords
  - 不要让 hook 返回 error (异步处理无法同步报错)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单一 hook + 简单关键词算法
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: 与 W3.5 并行 (独立)
  - **Blocks**: W3.7 (capability 注册)、W3.8 (test)
  - **Blocked By**: W1.1 (MemoryStore 增强: List/Get/Update 不需要,但 Store 需要)、W1.4 (Embedder)

  **References**:
  - `core/capabilities/hooks/memory.go:88-121` — **现有 memory hook 的 OnMessagePersist**! 本任务的 MemoryPersistHook 与其**共存但不同用途**:现有 memory hook 直接存 conversation memory,本任务提取事实存 semantic memory;两者都会写入 MemoryStore
  - `core/storage/memory.go` (W1.1) — MemoryStore.Store 方法,Memory 类型字段
  - **为何这样设计**: 关键词提取虽不精准但成本低、不需要 LLM;相似度去重避免重复存储相同对话内容;goroutine 异步避免拖慢主对话流程

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./capabilities/hooks/...` — PASS
  - [ ] MemoryPersistHook 实现 `hook.Hook` 接口
  - [ ] KeywordExtractor 对 "Copcon 是一个 AI Agent 框架" 提取出 ["copcon", "agent", "框架"] 之类的关键词 (具体看停用词表)
  - [ ] Execute 主路径不阻塞 (测量 0 ms 内返回)
  - [ ] goroutine 内存储失败不 panic (用 recover)
  - [ ] 相似度 > 0.95 时不重复存储

  **QA Scenarios**:

  ```
  Scenario: 关键词提取正确性
    Tool: bash
    Steps:
      1. 调用 KeywordExtractor.Extract("PostgreSQL 是一个开源关系数据库,支持 ACID 事务")
      2. 断言结果包含 ["postgresql", "开源", "关系数据库", "acid", "事务"] 中的 ≥ 3 个
      3. 断言不含 "是", "一个" 等停用词
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.6-keywords.txt
  ```

  ```
  Scenario: 去重机制防止重复存储
    Tool: bash
    Steps:
      1. 准备 mock MemoryStore,Search 返回 1 个 Score=0.98 的现有 memory
      2. 注入同样的 fact
      3. 等待 goroutine 完成 (用 channel 或 time.Sleep)
      4. 断言 memoryStore.Store 未被调用
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.6-dedup.txt
  ```

  ```
  Scenario: Execute 主路径不阻塞
    Tool: bash
    Steps:
      1. 用慢 mock MemoryStore (Store 时 sleep 2s)
      2. 调用 Execute 并计时
      3. 断言 Execute 在 50ms 内返回 (goroutine 异步)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.6-async.txt
  ```

  **Commit**: YES
  - Message: `feat(hooks): add MemoryPersistHook with keyword-based fact extraction (no LLM)`
  - Files: `core/capabilities/hooks/memory_persist.go` (含 KeywordExtractor)
  - Pre-commit: `cd core && go build ./capabilities/hooks/...`

- [x] W3.7 KnowledgeBase Capability 注册 + Bundle

  **What to do**:
  - 新建 `core/capabilities/hooks/kb_recall_capability.go`:
    ```go
    type kbRecallCapability struct{}
    func (c *kbRecallCapability) Name() string                         { return "hooks.kb_recall" }
    func (c *kbRecallCapability) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeHook }
    func (c *kbRecallCapability) DependsOn() []string                  { return nil }
    func (c *kbRecallCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
      if deps.KnowledgeStore == nil || deps.Embedder == nil {
        return nil, fmt.Errorf("hooks.kb_recall: KnowledgeStore or Embedder not available")
      }
      // agentKBs 由 Harness 在 W4.2 Build 时传入 (通过扩展 CapabilityDeps)
      return NewKBRecallHook(deps.KnowledgeStore, deps.Embedder, deps.AgentKnowledgeBases), nil
    }
    func init() { capabilities.Register(&kbRecallCapability{}) }
    ```
  - 类似新建 `core/capabilities/hooks/memory_persist_capability.go` for `hooks.memory_persist`
  - **重要**: 扩展 `core/capabilities/registry.go:CapabilityDeps` struct 新增:
    - `KnowledgeStore storage.KnowledgeStore`
    - `Embedder embedding.Embedder`
    - `AgentKnowledgeBases map[string][]string` (agentID → []kbID)
  - 更新 `core/capabilities/bundle.go:KnowledgeBaseBundleNames()` 返回 `["hooks.kb_recall", "hooks.memory_persist"]`

  **Must NOT do**:
  - 不要修改 core/harness.go (W4.1/W4.2 的工作)
  - 不要在此任务中实例化 KnowledgeStore (Harness Build 在 W4.2 做)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 类似 W2.4 的 capability 注册模式
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (依赖 W3.5/W3.6 完成)
  - **Blocks**: W4.1 (Harness 用 KnowledgeBaseBundleNames)、W3.8 (test 用 capability name 验证)
  - **Blocked By**: W3.5, W3.6

  **References**:
  - `core/capabilities/hooks/todo_injection.go:111-122` — capability 注册样板代码
  - `core/capabilities/hooks/file_memory_capability.go` (W2.4 实现) — 同 wave 的另一个 capability 参考
  - `core/capabilities/registry.go` (W2.4 已扩展 FileMemoryStore) — 本任务扩展 KnowledgeStore + Embedder + AgentKnowledgeBases
  - `core/capabilities/bundle.go` (W1.6) — KnowledgeBaseBundleNames() 函数

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./capabilities/...` — PASS
  - [ ] 2 个 capability 注册成功 (`hooks.kb_recall`, `hooks.memory_persist`)
  - [ ] KnowledgeBaseBundleNames() 返回正确的 2 个
  - [ ] CapabilityDeps 包含 KnowledgeStore + Embedder + AgentKnowledgeBases 字段

  **QA Scenarios**:

  ```
  Scenario: KB bundle capability 注册完整
    Tool: bash
    Steps:
      1. 调用 capabilities.ListByType(CapabilityTypeHook),过滤 name 含 "kb_recall" 或 "memory_persist"
      2. 断言包含 2 个
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.7-register.txt
  ```

  **Commit**: YES
  - Message: `feat(capabilities): register knowledge_base capability bundle`
  - Files: `core/capabilities/hooks/kb_recall_capability.go`, `core/capabilities/hooks/memory_persist_capability.go`, `core/capabilities/registry.go` (扩展), `core/capabilities/bundle.go`
  - Pre-commit: `cd core && go test ./capabilities/...`

- [x] W3.8 知识库集成单元测试

  **What to do**:
  - 综合单元测试验证知识库子系统端到端工作:
    - `core/providers/sqlitevec/integration_test.go` — sqlite-vec provider + in-memory DB 集成测试:
      - KB 创建 + 列表 + 删除
      - Document 生命周期: 创建 (status pending) → 入库 → status ready
      - Chunk 向量写入 + 检索 (相似度正确性)
      - Cascade delete (KB 删除 → docs/chunks 同步删除)
    - `core/rag/integration_test.go` — 管线集成:
      - Parser → Chunker → Embedder → Store 完整流程 (用 testdata PDF)
      - Mock Embedder (返回固定向量)
      - 真实 sqlite-vec in-memory store
      - 验证: 写入后 Search 可查出
    - `core/capabilities/hooks/kb_recall_test.go` — KBRecallHook 单元测试 (已在 W3.5 写,这里扩展覆盖更多 case)
    - `core/capabilities/hooks/memory_persist_test.go` — MemoryPersistHook 单元测试 (扩展)
  - **Testcontainers 模式** (如 sqlite-vec 支持): 用 in-memory sqlite 而不是真实 DB (简化)
  - 性能测试: 入库 100 个 chunks 的时间 ≤ 2s

  **Must NOT do**:
  - 不要启动真实 Qdrant 容器 (本任务只测 sqlite-vec)
  - 不要调用真实 LLM API (mock embedder)
  - 不要修改 W3.1-W3.7 的实现 (如测试失败意味着实现有 bug,应修 bug 而非改测试)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 测试设计需要全面,但无新代码 (仅测试)
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (W3.1-W3.7 必须先完成)
  - **Blocks**: FINAL F2 (代码质量审查验证测试覆盖)
  - **Blocked By**: W3.1, W3.2, W3.3, W3.4, W3.5, W3.6, W3.7

  **References**:
  - 所有 W3 任务的实现文件 (测试对象)
  - `core/harness_test.go` — 参考现有集成测试写法
  - `core/testutil/` (如存在) — 可能有 mock helpers
  - **为何这样设计**: 集成测试验证子系统端到端;单元测试验证组件隔离;性能测试捕获明显回归

  **Acceptance Criteria**:
  - [ ] `cd core && go test -v ./providers/sqlitevec/... ./rag/... ./capabilities/hooks/ -run "KB|Memory|RAG"` — ALL PASS
  - [ ] `cd core && go test -cover ./providers/sqlitevec/... ./rag/...` — 覆盖率 ≥ 80%
  - [ ] `cd core && go test -race ./providers/sqlitevec/... ./rag/... ./capabilities/...` — 无 race condition
  - [ ] 集成测试: 写入 100 chunks 时间 ≤ 2s
  - [ ] 端到端集成测试: 上传 PDF → pipeline 处理 → search 查出,全 PASS

  **QA Scenarios**:

  ```
  Scenario: sqlite-vec 端到端 (KB + Doc + Chunk + Search)
    Tool: bash
    Steps:
      1. 执行 cd core && go test -v -run TestSQLiteVecE2E ./providers/sqlitevec/...
      2. 测试内部:创建 KB → ingest 文档 → search → 验证 top-3
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.8-sqlitevec-e2e.txt
  ```

  ```
  Scenario: RAG 管线集成 (parse + chunk + embed + store)
    Tool: bash
    Steps:
      1. 执行 cd core && go test -v -run TestRAGPipeline ./rag/...
      2. 用 testdata PDF + mock Embedder + in-memory sqlite-vec
      3. 验证 KB 中 chunks 数 > 0,Search 返回 top-N
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W3.8-pipeline-integration.txt
  ```

  ```
  Scenario: race detector 全过
    Tool: bash
    Steps:
      1. cd core && go test -race ./providers/sqlitevec/... ./rag/... ./capabilities/hooks/... ./capabilities/tools/...
    Expected Result: 0 race 警告
    Evidence: .sisyphus/evidence/task-W3.8-race.txt
  ```

  **Commit**: YES
  - Message: `test(knowledge): integration tests for sqlite-vec, pipeline, hooks end-to-end`
  - Files: `core/providers/sqlitevec/integration_test.go`, `core/rag/integration_test.go`, `core/capabilities/hooks/kb_recall_test.go` (扩展), `core/capabilities/hooks/memory_persist_test.go` (扩展)
  - Pre-commit: `cd core && go test -race ./providers/sqlitevec/... ./rag/... ./capabilities/...`



### Wave 3 — W4: Harness 集成 + W7 评估(并行)

- [x] W4.1 Harness 双 Bundle 展开 (memory + knowledge_base)

  **What to do**:
  - 重写 `core/harness.go:collectCapabilityNames()` 函数:
    ```go
    func (h *Harness) collectCapabilityNames() []string {
      seen := make(map[string]bool)
      var names []string

      add := func(n string) {
        if !seen[n] {
          seen[n] = true
          names = append(names, n)
        }
      }

      for _, spec := range h.config.Agents {
        // 保留现有: 每个 agent 都注入 builtInTools + builtInHooks
        for _, t := range builtInTools { add(t) }
        for _, hk := range builtInHooks { add(hk) }

        // 新增: 如果 agent 启用 memory (MD 文件),展开 MemoryBundleNames
        if spec.Memory.Enabled {
          for _, n := range capabilities.MemoryBundleNames() {
            add(n)
          }
        }

        // 新增: 如果 agent 配置了 knowledge_bases (非空),展开 KnowledgeBaseBundleNames
        if len(spec.KnowledgeBases) > 0 {
          for _, n := range capabilities.KnowledgeBaseBundleNames() {
            add(n)
          }
        }

        // 保留现有: spec.Tools 自定义 tool 列表
        for _, t := range spec.Tools {
          if capName, ok := toolNameToCap[t]; ok {
            add(capName)
          } else {
            add(t)
          }
        }
      }

      return names
    }
    ```
  - **注意**: `builtInHooks` 当前包含 `"hooks.memory"`,而 MemoryBundleNames() 也包含它;`seen` map 保证不重复注册
  - **向后兼容**: 如果 `spec.Memory.Enabled == false` 且 `spec.KnowledgeBases == nil`,则收集结果与原逻辑完全等价

  **Must NOT do**:
  - 不要修改 builtInTools / builtInHooks 的现有内容 (避免破坏向后兼容)
  - 不要在此任务中实现 FileMemoryStore / KnowledgeStore 的实例化 (W4.2 的工作)
  - 不要让 collectCapabilityNames 返回 nil (即使是空配置也应返回 builtInTools + builtInHooks)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 修改核心 Harness 函数,需要小心保持向后兼容
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W7 并行,与 W4.2 串行更安全)
  - **Blocks**: W4.2 (需要展开结果来构造 CapabilityDeps)、W4.3 (skip 逻辑)、W4.4 (test)
  - **Blocked By**: W1.6 (MemoryBundleNames 存在)、W2.4 (memory capabilities 注册)、W3.7 (KB capabilities 注册)

  **References**:
  - `core/harness.go:299-327` — 现有 `collectCapabilityNames()` 实现,本任务基于此扩展
  - `core/capabilities/bundle.go` (W1.6) — MemoryBundleNames() 和 KnowledgeBaseBundleNames() 函数
  - **为何这样设计**: 通过 spec 字段开关控制 bundle 展开,让用户在 yaml 中配置决定启用哪个 bundle;seen map 去重避免重复注册同一 capability

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./...` — PASS
  - [ ] 测试 4 种配置组合都产生正确的 capability names 集合:
    - 两关闭: 仅 builtInTools + builtInHooks + spec.Tools
    - 仅 memory.Memory.Enabled = true: + MemoryBundleNames (4 个)
    - 仅 KnowledgeBases 非空: + KnowledgeBaseBundleNames (2 个)
    - 两者都开: + 6 个去重后的 capability
  - [ ] 向后兼容: 现有配置 (无 memory/kb) 行为不变

  **QA Scenarios**:

  ```
  Scenario: 双 bundle 展开 + 去重
    Tool: bash
    Steps:
      1. 构造 HarnessConfig{Agents: [{ID: "a1", Memory: {Enabled: true}, KnowledgeBases: ["kb1"]}, {ID: "a2"}]}
      2. 调用 h.collectCapabilityNames()
      3. 断言结果包含 builtInHooks + builtInTools + MemoryBundleNames + KnowledgeBaseBundleNames + 自定义 Tools
      4. 断言结果无重复
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W4.1-dual-bundle.txt
  ```

  **Commit**: YES
  - Message: `feat(harness): expand collectCapabilityNames for dual bundle (memory + knowledge_base)`
  - Files: `core/harness.go`
  - Pre-commit: `cd core && go build ./...`

- [x] W4.2 StoreConfig + KnowledgeStore backend 实例化 + CapabilityDeps 注入

  **What to do**:
  - 在 `core/harness.go:StoreConfig` struct 新增:
    ```go
    type StoreConfig struct {
      Provider       storage.StoreProvider
      Memory         storage.MemoryStore  // 现有
      FileMemory     filememory.FileMemoryStore  // 新增:用于 MD 文件记忆
      KnowledgeStore storage.KnowledgeStore  // 新增:用于 RAG 知识库
      Embedder       embedding.Embedder  // 新增
    }
    ```
    - 注意: 也可以不直接在 StoreConfig 中嵌入这些字段,而是让 StoreProvider 接口扩展 Memory()/Knowledge() 方法 (W1.2 已扩展 Knowledge);本任务决定用哪种
    - **推荐**: 保持 StoreConfig 字段独立,因为 FileMemory/MemoryStore/KnowledgeStore 可能来自不同 provider 组合;不强求它们共享同一个 StoreProvider 实例
  - 在 `core/harness.go:Build()` 方法中,在 `capDeps := ...` 之前,添加:
    ```go
    // 如果配置了任何 agent 启用 memory,实例化 FileMemoryStore
    var fileMemoryStore filememory.FileMemoryStore
    for _, spec := range h.config.Agents {
      if spec.Memory.Enabled {
        // 用 spec.Memory.BasePath 或默认值
        basePath := spec.Memory.BasePath
        if basePath == "" {
          homeDir, _ := os.UserHomeDir()
          basePath = filepath.Join(homeDir, ".copcon", "memory")
        }
        var err error
        fileMemoryStore, err = filememory.NewFileMemoryStore(basePath, spec.Memory)
        if err != nil {
          return fmt.Errorf("init filememory store: %w", err)
        }
        break  // 多个 agent 共享一个 filememory store (按 agent_id 分子目录)
      }
    }

    // 如果配置了任何 agent 带 knowledge_bases,实例化 KnowledgeStore + Embedder
    var knowledgeStore storage.KnowledgeStore
    var embedder embedding.Embedder
    for _, spec := range h.config.Agents {
      if len(spec.KnowledgeBases) > 0 {
        // 从配置读取第一个 KB 的 backend + path (多 KB 共享 sqlite 文件,不同 collection)
        // 或者更简单: 用统一的 sqlite 文件 ~/.copcon/knowledge.db 存所有 KB
        // 本任务采用简化方案: 统一 sqlite 文件
        kbPath := "~/.copcon/knowledge.db"  // 或从 config 读
        kbPath, _ = expandPath(kbPath)
        // 通过 registry 创建
        factory, err := storage.LookupKnowledgeStoreProvider("sqlite-vec")  // 本期硬编码,后续按 config 切换
        if err != nil {
          return fmt.Errorf("lookup knowledgestore provider: %w", err)
        }
        knowledgeStore, err = factory(map[string]any{"sqlite_path": kbPath})
        if err != nil {
          return fmt.Errorf("create knowledgestore: %w", err)
        }

        // 创建 embedder (OpenAI 复用现有 LLM provider)
        embedder, err = embedding.NewFromConfig(h.config.EmbeddingConfig, h.config.LLM)
        if err != nil {
          return fmt.Errorf("create embedder: %w", err)
        }
        break
      }
    }

    // 构造 agentID → []kbID map (传给 KBRecallHook)
    agentKBs := make(map[string][]string)
    for _, spec := range h.config.Agents {
      if len(spec.KnowledgeBases) > 0 {
        agentKBs[spec.ID] = spec.KnowledgeBases
      }
    }
    ```
  - 更新 `capDeps := capabilities.CapabilityDeps{...}` 构造:
    - 新增字段: `FileMemoryStore: fileMemoryStore`, `KnowledgeStore: knowledgeStore`, `Embedder: embedder`, `AgentKnowledgeBases: agentKBs`
  - 需要扩展 `HarnessConfig`:
    ```go
    type HarnessConfig struct {
      // ... 现有字段 ...
      EmbeddingConfig embedding.EmbeddingConfig  // 新增,来自 server config
      KnowledgeBaseConfigs []KnowledgeBaseConfigMeta  // 新增,传递 KB configs 给 store (可选)
    }
    ```
    - 注意: 不要在 core/harness.go 直接 import server/internal/config (模块边界),定义 meta struct 或使用 map[string]any

  **Must NOT do**:
  - 不要在 Build() 中 panic 失败 (用 return error 让调用方处理)
  - 不要让 fileMemoryStore/knowledgeStore/embedder 强制非 nil (如果未配置则保持 nil,hook 中处理 nil)
  - 不要在 core/harness.go import `server/internal/config` (保持模块边界)
  - 不要删除现有 `StoreConfig.Memory storage.MemoryStore` 字段 (向后兼容,虽然本任务会引入 FileMemoryStore,但旧的 MemoryStore 字段保留给现有 Qdrant provider 用)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 修改 Build() 核心路径;多 provider 实例化;错误处理必须周到
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖 W4.1 完成)
  - **Blocks**: W4.3 (skip 逻辑)、W4.4 (test)、W5.3 (API handler DI)
  - **Blocked By**: W4.1, W1.7 (knowledge registry)、W2.1 (filememory.NewFileMemoryStore)、W3.1 (sqlite-vec 实现)

  **References**:
  - `core/harness.go:140-268` — 现有 Build() 函数,本任务修改
  - `core/harness.go:76-79` — 现有 StoreConfig struct,本任务扩展
  - `core/harness.go:100-107` — 现有 HarnessConfig struct,本任务扩展
  - `core/harness.go:161-172` — 现有 capDeps 构造,本任务扩展
  - `core/providers/filememory/filememory.go` (W2.1) — NewFileMemoryStore 构造函数签名
  - `core/storage/knowledge_registry.go` (W1.7) — LookupKnowledgeStoreProvider 函数
  - `core/providers/embedding/factory.go` (W1.4) — NewFromConfig 函数
  - **为何这样设计**: 多 provider 实例 (filememory vs sqlite-vec vs qdrant 现有) 通过 StoreConfig 字段独立管理,不强求统一;简化方案: 本期所有 agent 共享一个 sqlite 文件,通过 kb_id 区分数据

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./...` — PASS
  - [ ] `cd core && go test ./...` — PASS (现有 harness_test.go 不受影响)
  - [ ] StoreConfig 扩展后,现有 HarnessConfig 使用方式 (仅 Provider + Memory) 仍能编译
  - [ ] Build() 在 Memory.Enabled=false 且 KnowledgeBases=nil 时不创建 filememory/knowledge store (验证 lazy init)
  - [ ] Build() 在任一 provider 创建失败时 return error (不 panic)

  **QA Scenarios**:

  ```
  Scenario: 仅 memory.Enabled 时 FileMemoryStore 被创建
    Tool: bash
    Steps:
      1. 构造 HarnessConfig{Agents: [{Memory: {Enabled: true, BasePath: "/tmp/mem"}}]}
      2. 调用 Build() 成功
      3. 验证 capDeps.FileMemoryStore != nil 且 capDeps.KnowledgeStore == nil, capDeps.Embedder == nil
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W4.2-memory-only.txt
  ```

  ```
  Scenario: 仅 knowledge_bases 时 KnowledgeStore + Embedder 被创建
    Tool: bash
    Steps:
      1. HarnessConfig{Agents: [{KnowledgeBases: ["kb1"]}], EmbeddingConfig: {...}, LLM: mock}
      2. Build() 成功
      3. 验证 capDeps.KnowledgeStore != nil, capDeps.Embedder != nil
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W4.2-kb-only.txt
  ```

  ```
  Scenario: 两者都关闭时 Build() 不创建额外 store
    Tool: bash
    Steps:
      1. HarnessConfig{Agents: [{}]} (无 memory/kb 配置)
      2. Build() 成功
      3. capDeps.FileMemoryStore == nil && capDeps.KnowledgeStore == nil
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W4.2-none.txt
  ```

  **Commit**: YES
  - Message: `feat(harness): add KnowledgeStore + backend dispatch in StoreConfig + Build()`
  - Files: `core/harness.go`
  - Pre-commit: `cd core && go test ./...`

- [x] W4.3 细粒度 skip 逻辑 (per-hook 依赖检查)

  **What to do**:
  - 重写 `core/harness.go:Build()` 中 hook 注册逻辑 (当前 196-209 行):
    ```go
    // 旧逻辑 (硬编码 hooks.memory + MemoryStore==nil skip):
    if cap.Name() == "hooks.memory" && h.config.Store.Memory == nil {
      logger.Info("harness: skipping memory hook (MemoryStore not configured)")
      continue
    }

    // 新逻辑: 每个 hook 在 NewHook 内部检查自己的依赖,返回特定错误
    hc, ok := cap.(capabilities.HookCapability)
    if !ok {
      return fmt.Errorf(...)
    }
    hk, err := hc.NewHook(capDeps)
    if err != nil {
      // 区分"依赖未配置"和真正错误
      if isDependencyUnavailableError(err) {
        logger.Info("harness: skipping hook (dependency unavailable)", "capability", cap.Name(), "reason", err.Error())
        continue
      }
      return fmt.Errorf("create hook from capability %q: %w", cap.Name(), err)
    }
    hookRunner.Register(hk)
    ```
  - 定义 sentinel error:
    ```go
    var ErrDependencyUnavailable = errors.New("dependency unavailable")
    ```
    或者用字符串前缀判断
  - 修改所有 Hook NewHook 实现,在依赖缺失时返回 ErrDependencyUnavailable (或包装):
    - `hooks.file_memory.NewHook`: deps.FileMemoryStore == nil → return wrapped ErrDependencyUnavailable
    - `hooks.memory.NewHook` (现有 vector memory hook): deps.MemoryStore == nil → return ErrDependencyUnavailable
    - `hooks.kb_recall.NewHook`: deps.KnowledgeStore == nil || deps.Embedder == nil → return wrapped ErrDependencyUnavailable
    - `hooks.memory_persist.NewHook`: 同上
    - `hooks.todo_injection.NewHook`: 现有逻辑 (deps.TodoStore == nil 时如何处理,需检查)
  - `isDependencyUnavailableError` helper: 用 errors.Is 判断

  **Must NOT do**:
  - 不要删除现有 `"hooks.memory" && MemoryStore==nil` 的特殊处理立即 (而是让 hooks.memory.NewHook 自己处理,然后删除 harness 中的特殊处理)
  - 不要让 hook 返回普通 error 被当成依赖缺失 (需要明确 sentinel error 或 error type)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 模式统一改写,无复杂新逻辑
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖 W4.2 完成)
  - **Blocks**: W4.4 (test)
  - **Blocked By**: W4.2, 各 hook capability 文件 (W2.4, W3.7)

  **References**:
  - `core/harness.go:195-211` — 现有 hook 注册逻辑,本任务修改
  - `core/capabilities/hooks/todo_injection.go:120-122` — 现有 todo_injection_hookCapability.NewHook 实现,需检查 TodoStore==nil 时如何处理 (如果是 nil 直接 panic 则需修改)
  - `core/capabilities/hooks/file_memory_capability.go` (W2.4) — NewHook 实现
  - `core/capabilities/hooks/kb_recall_capability.go` (W3.7) — NewHook 实现
  - **为何这样设计**: 细粒度 skip 让 harness.Build 不需要硬编码每个 hook 的依赖,降低新增 hook 时改动 harness 的成本;sentinel error 区分"未配置"(应该 skip)和真实 bug(应该 fail fast)

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./...` — PASS
  - [ ] 删除 harness.go 中旧的 `"hooks.memory" && MemoryStore==nil` 硬编码分支后,现有 Qdrant MemoryStore 未配置时仍能 skip hooks.memory (通过 hook 自己返回 ErrDependencyUnavailable)
  - [ ] hook 返回普通 error (非 ErrDependencyUnavailable) 时,Build() 返回 error (不被忽略)
  - [ ] 4 个 hook 全部在依赖缺失时跳过,不报错

  **QA Scenarios**:

  ```
  Scenario: 依赖缺失时 hook 被跳过 (log info 无 error)
    Tool: bash
    Steps:
      1. HarnessConfig{Agents: [{Memory: {Enabled: true}}]} (启用 memory 但 FileMemoryStore 创建失败,假设 basePath 不可写)
      2. Build 时 capDeps.FileMemoryStore == nil (模拟)
      3. 验证 logger.Info 包含 "skipping hook" + "hooks.file_memory"
      4. 验证 Build 最终仍成功 (跳过 hook 但不失败)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W4.3-skip-unavailable.txt
  ```

  ```
  Scenario: 真实错误不被忽略
    Tool: bash
    Steps:
      1. 让某个 hook capability 的 NewHook 返回普通错误 (非 ErrDependencyUnavailable)
      2. Build() 应该返回 error 而非 log + continue
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W4.3-real-error.txt
  ```

  **Commit**: YES
  - Message: `feat(hooks): fine-grained skip logic per hook based on dependency readiness`
  - Files: `core/harness.go`, `core/capabilities/hooks/*.go` (修改 NewHook 错误返回)
  - Pre-commit: `cd core && go test ./...`

- [x] W4.4 Harness 集成测试 (4 种配置组合端到端)

  **What to do**:
  - 新建或扩展 `core/harness_integration_test.go`:
    - 4 个测试用例:
      1. **none**: `Agents: [{ID: "a1"}]` (无 memory/kb 配置) — 验证 Build 成功,仅注册 builtInHooks + builtInTools
      2. **memory only**: `Agents: [{ID: "a1", Memory: {Enabled: true, BasePath: t.TempDir()}}]` — 验证 FileMemoryHook + 3 memory tools 注册
      3. **kb only**: `Agents: [{ID: "a1", KnowledgeBases: ["kb1"]}], EmbeddingConfig: {...}, LLM: mock` — 验证 KBRecallHook + MemoryPersistHook 注册
      4. **both**: `Agents: [{ID: "a1", Memory: {Enabled: true}, KnowledgeBases: ["kb1"]}]` — 验证所有 capabilities 都注册,且无重复
    - 每个测试用 `t.TempDir()` 作为 filememory basePath 避免污染
    - 验证注册:
      - 遍历 `hookRunner.Register` 的 hook (通过 mock runner 或反射)
      - 遍历 `toolRegistry.List()` 验证包含预期的 tool
    - 测试 hook chain 执行:
      - 构造 mock ChatContext + mock Messages
      - 调用 hookRunner.Run(OnSystemPrompt, ctx)
      - 断言 *ctx.SystemPrompt 包含/不包含 memory 注入文本
  - 复用现有 `core/harness_test.go` 的测试结构 (mock LLM、mock store 等)

  **Must NOT do**:
  - 不要在测试中实际调用 LLM API (用 mock)
  - 不要修改 W4.1-W4.3 的实现 (如测试失败意味实现有 bug,应修 bug)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 集成测试设计需要 4 个用例 + 多种验证方式
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (W4.1-W4.3 必须先完成)
  - **Blocks**: FINAL F1 (计划合规审查)、F2 (代码质量)、F4 (范围保真)
  - **Blocked By**: W4.1, W4.2, W4.3

  **References**:
  - `core/harness_test.go` — 现有 harness 测试结构 (参考 mock 创建 + Build 调用方式)
  - `core/harness.go` (W4.1-W4.3 修改后版本) — 测试对象
  - **为何这样设计**: 4 种组合是 memory/kb 正交性的完整覆盖;hook chain 验证确保注入逻辑正确连接

  **Acceptance Criteria**:
  - [ ] `cd core && go test -v -run TestHarness.* ./...` — PASS 4 个组合测试
  - [ ] `cd core && go test -race -run TestHarness.* ./...` — 无 race
  - [ ] 每个测试用例验证: capabilities 数量正确、hook chain 行为正确、无 panic
  - [ ] 集成测试覆盖:
    - builtInHooks 仍注册 (todo_injection, logging, tracing 等)
    - 新增 hooks (file_memory, kb_recall, memory_persist) 按配置注册
    - 新增 tools (memory_store/recall/forget) 按配置注册

  **QA Scenarios**:

  ```
  Scenario: 全部 4 种配置组合 PASS
    Tool: bash
    Steps:
      1. cd core && go test -v -run "TestHarnessIntegration" ./... 2>&1
      2. 输出中 --- PASS 出现 4 次 (TestHarnessIntegration_None/MemoryOnly/KBOnly/Both)
    Expected Result: 4 PASS
    Evidence: .sisyphus/evidence/task-W4.4-four-combinations.txt
  ```

  ```
  Scenario: hook chain 实际注入验证
    Tool: bash
    Steps:
      1. TestHarnessIntegration_MemoryOnly 中: 触发 OnSystemPrompt hook chain
      2. 断言 system prompt 含 "Agent Memory" (来自 FileMemoryHook)
      3. TestHarnessIntegration_Both 中: 触发 AfterContextBuild hook chain
      4. 断言 messages 头部含 system message (来自 KBRecallHook)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W4.4-hook-chain.txt
  ```

  ```
  Scenario: 配置错误 fail fast
    Tool: bash
    Steps:
      1. HarnessConfig{Agents: [{Memory: {Enabled: true, BasePath: "/non/existent/read/only/path"}}]}
      2. Build() 返回 error (不 panic,返回具体原因)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W4.4-fail-fast.txt
  ```

  **Commit**: YES
  - Message: `test(harness): end-to-end integration for 4 config combinations (none/memory/KB/both)`
  - Files: `core/harness_integration_test.go` (新建或扩展)
  - Pre-commit: `cd core && go test -race -run TestHarness ./...`



### Wave 4 — W5: Server API + chat-core 扩展(两 agent 并行: 后端 API + chat-core)

- [x] W5.1 知识库管理 API (REST CRUD + 文档上传)

  **What to do**:
  - 新建 `server/internal/api/knowledge.go`:
    - `POST /api/kb` — 创建知识库
      - Request: `{"name": "Company Docs", "backend": "sqlite-vec", "chunk_size": 800, "chunk_overlap": 100}`
      - 实现: 调用 `knowledgeStore.CreateKB(ctx, &KnowledgeBase{ID: uuid.New(), Name, Backend, Config, CreatedAt, UpdatedAt})`
      - Response 201: `{"id": "kb_...", "name": "...", ...}`
      - 错误: name 重复返回 409, invalid backend 返回 400
    - `GET /api/kb` — 列出所有知识库
      - Response 200: `{"kbs": [...]}`
    - `GET /api/kb/:kbId` — 获取单个知识库详情 + 统计信息 (文档数/chunks 数)
    - `DELETE /api/kb/:kbId` — 删除知识库(级联删除文档 + chunks)
      - Response 204, 不存在返回 404
    - `POST /api/kb/:kbId/docs` — 上传文档 (multipart/form-data)
      - 接收文件 (单文件或多文件),读取 bytes
      - 创建 Document 记录 (status=pending)
      - **异步启动入库管线**(goroutine 调用 `rag.Pipeline.Ingest`,完成后更新 document.status=ready 或 error)
      - 立即返回 Response 202: `{"documents": [{id, filename, status: "pending"}]}`
      - 文件类型校验: 仅允许 pdf/md/txt/html,其他返回 415
      - 文件大小限制 (如 10MB,可配置)
    - `GET /api/kb/:kbId/docs` — 列出知识库中所有文档(含 status/chunk_count)
      - 支持 query 参数:`?status=ready&limit=20&offset=0`
    - `GET /api/kb/:kbId/docs/:docId` — 获取单个文档详情(含 chunks 列表,如 query `?include_chunks=true`)
    - `DELETE /api/kb/:kbId/docs/:docId` — 删除文档 + chunks
      - Response 204
  - 所有方法接收 `*gin.Context`,使用现有 Handler struct 字段
  - **错误处理规范**:
    - 400: 请求参数错误
    - 404: KB/document 不存在
    - 409: 唯一字段冲突 (如 name)
    - 415: 不支持的文件类型
    - 500: 数据库错误 (log + 返回通用 error)
  - 单元测试: 使用 `httptest.NewRecorder()` + `gin.CreateTestContext`

  **Must NOT do**:
  - 不要在同步 HTTP handler 中执行入库管线 (必须异步,立即返回 pending)
  - 不要硬编码文件类型判断 (用 mimetype map 集中管理)
  - 不要在知识库 API 中引用 agent config (kb 是全局的,不绑定 agent)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: REST API 设计、multipart 上传、异步处理、错误码规范需要仔细
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W5.5-W5.7 chat-core 并行,与 W5.2 串行更安全避免 handler.go 冲突)
  - **Blocks**: W5.2 (检索测试 API 用同样 handler struct)、W5.3 (路由注册)、W5.4 (test)
  - **Blocked By**: W1.2 (KnowledgeStore 接口)、W3.1 (sqlite-vec 实现)、W3.4 (管线)、W4.2 (capDeps)

  **References**:
  - `server/internal/api/handlers.go:19-40` — 现有 Handler struct,本任务新增 knowledgeStore 字段
  - `server/internal/api/handlers.go:311-332` — 现有 `SetupRoutes` 函数,本任务新增路由
  - `server/internal/api/handlers.go:41-89` — 现有 CreateSession 实现,参考 JSON 请求解析 + JSON 响应格式
  - `github.com/gin-gonic/gin` — Gin 框架文档:c.Param / c.ShouldBindJSON / c.File 等
  - **为何这样设计**: 异步入库立即返回,避免客户端等待长上传 + 处理;Document 状态机 (pending/parsing/ready/error) 让 UI 可显示实时进度

  **Acceptance Criteria**:
  - [ ] `cd server && go build ./internal/api/...` — PASS
  - [ ] `cd server && go test ./internal/api/...` — PASS (新增单元测试)
  - [ ] 单元测试覆盖:
    - POST /api/kb happy path
    - POST /api/kb 名称重复 → 409
    - GET /api/kb 列表排序 (按 created_at desc)
    - DELETE /api/kb 级联删除 (验证 docs/chunks 被清空)
    - POST /api/kb/:id/docs 上传 PDF → 202 + doc status=pending
    - POST /api/kb/:id/docs 上传 .zip → 415
  - [ ] 异步入库:上传后立即 GET /api/kb/:id/docs/:docId → status=pending,等 2s 再 GET → status=ready

  **QA Scenarios**:

  ```
  Scenario: KB CRUD 全流程
    Tool: bash (curl 或 httptest)
    Steps:
      1. POST /api/kb → 201,记 kbId
      2. GET /api/kb → 列表含该 kb
      3. DELETE /api/kb/:kbId → 204
      4. GET /api/kb/:kbId → 404
    Expected Result: 4 步全 PASS
    Evidence: .sisyphus/evidence/task-W5.1-kb-crud.txt
  ```

  ```
  Scenario: 文档上传 + 异步入库
    Tool: bash
    Steps:
      1. POST /api/kb/:id/docs 带 form-data file=sample.md
      2. 立即响应 202 + documents[0].status="pending"
      3. GET /api/kb/:id/docs/:docId (等待 1-3s 轮询)
      4. 最终 status="ready" + chunk_count > 0
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.1-doc-upload.txt
  ```

  ```
  Scenario: 文件类型校验
    Tool: bash
    Steps:
      1. POST .zip → 415
      2. POST .exe → 415
      3. POST .pdf → 202 (happy)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.1-mimetype.txt
  ```

  **Commit**: YES
  - Message: `feat(api): knowledge base management REST endpoints (KB CRUD + doc CRUD + multipart upload)`
  - Files: `server/internal/api/knowledge.go`, `server/internal/api/knowledge_test.go`
  - Pre-commit: `cd server && go test ./internal/api/...`

- [x] W5.2 检索测试 + 会话记忆管理 API

  **What to do**:
  - 在 `server/internal/api/knowledge.go` 中新增:
    - `POST /api/kb/:kbId/search` — 检索测试
      - Request: `{"query": "退款政策", "top_k": 5}` (top_k 可选,默认 5)
      - 实现: 调用 embedder.Embed(query) → knowledgeStore.Search([kbId], vec, {TopK})
      - Response 200: `{"chunks": [{"id": "...", "content": "...", "score": 0.92, "document_id": "...", "chunk_index": 3, "metadata": {...}}, ...]}`
      - KB 不存在 → 404
  - 新建 `server/internal/api/memory.go`:
    - `GET /api/sessions/:sessionId/memories` — 列出会话的 agent 记忆
      - 实现: `memoryStore.GetBySession(ctx, sessionID, limit)` (limit 默认 50)
      - Response 200: `{"memories": [{"id": "...", "content": "...", "role": "...", "memory_type": "...", "timestamp": "..."}]}`
      - 会话不存在 → 404
    - `DELETE /api/sessions/:sessionId/memories/:memoryId` — 删除单条记忆
      - 实现: `memoryStore.Delete(ctx, memoryId)` + 验证 memory.sessionID == sessionID (防越权)
      - Response 204, memoryId 不存在 → 404
  - 测试: 同 W5.1 风格的 httptest

  **Must NOT do**:
  - 不要在 DELETE memory API 中忽略 sessionID 校验 (允许跨会话删除是安全漏洞)
  - 不要让检索测试失败返回 500 (embedder 错误返回 503 + 可读 error message)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 2 个简单 API
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖 W5.1 Handler 有 knowledgeStore)
  - **Blocks**: W5.3 (路由注册)、W5.4 (test)
  - **Blocked By**: W5.1, W2.1 (filememory 提供 GetBySession/Delete)

  **References**:
  - `server/internal/api/handlers.go:152-167` — 参考 DeleteSession 实现 (UUID parse + 404 处理)
  - `core/storage/memory.go` (W1.1 实现) — MemoryStore.GetBySession/Delete
  - **为何这样设计**: 检索测试 API 让 UI 可独立验证 RAG 效果;会话记忆 API 让前端显示 + 管理 agent 已积累的记忆

  **Acceptance Criteria**:
  - [ ] `cd server && go build ./internal/api/...` — PASS
  - [ ] 单元测试: 3 个新 API 各 ≥ 2 个 case (happy + 404)
  - [ ] DELETE memory 越权校验 (memory 不属于 session 时拒绝)

  **QA Scenarios**:

  ```
  Scenario: 检索测试返回 ranked chunks
    Tool: bash
    Steps:
      1. 先上传文档 (异步,等 ready)
      2. POST /api/kb/:id/search {"query": "relevant keyword"}
      3. 响应 chunks 数组,按 score desc 排序
      4. 每个 chunk 含 content + score + document_id
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.2-search.txt
  ```

  ```
  Scenario: DELETE memory 越权拒绝
    Tool: bash
    Steps:
      1. 创建 2 个 session: s1, s2
      2. s1 下存储一个 memory m1
      3. DELETE /api/sessions/s2/memories/m1
      4. 响应 404 (假装不存在) 或 403 (跨会话)
    Expected Result: m1 未被删除
    Evidence: .sisyphus/evidence/task-W5.2-cross-session-deny.txt
  ```

  **Commit**: YES
  - Message: `feat(api): retrieval test + session memory endpoints`
  - Files: `server/internal/api/knowledge.go` (追加 search), `server/internal/api/memory.go` (新建), `server/internal/api/memory_test.go`
  - Pre-commit: `cd server && go test ./internal/api/...`

- [x] W5.3 API 路由注册 + Handler 依赖注入

  **What to do**:
  - 修改 `server/internal/api/handlers.go:`
    - Handler struct 新增字段:
      ```go
      type Handler struct {
        // ... 现有
        knowledgeStore storage.KnowledgeStore  // 新增,可为 nil
        memoryStore    storage.MemoryStore     // 新增,可为 nil
        embedder       embedding.Embedder      // 新增,可为 nil
        ragPipeline    *rag.Pipeline           // 新增,可为 nil (用于异步入库)
      }
      ```
    - `NewHandler` 函数签名扩展接收新字段 (通过 `core.APIProvider` 或额外参数):
      ```go
      func NewHandler(cfg *config.Config, h core.APIProvider, opts ...HandlerOption) *Handler {
        // 使用 functional options 模式扩展,避免破坏现有调用
      }
      ```
    - 或更简单: 修改 `core.APIProvider` interface 暴露 `KnowledgeStore()` / `Embedder()` / `RAGPipeline()` 方法 (W4.2 已经在 Harness 中实例化)
  - 修改 `SetupRoutes(r *gin.Engine, cfg, h)`:
    ```go
    func SetupRoutes(r *gin.Engine, cfg *config.Config, h core.APIProvider) {
      handler := NewHandler(cfg, h)

      api := r.Group("/api")
      {
        // ... 现有路由
        api.GET("/agents", handler.ListAgents)

        sessions := api.Group("/sessions")
        { /* 现有 */ }

        // 新增: KB 路由 (只在 knowledgeStore 配置时注册,减少无效路由)
        if handler.knowledgeStore != nil {
          kb := api.Group("/kb")
          {
            kb.POST("", handler.CreateKB)
            kb.GET("", handler.ListKBs)
            kb.GET("/:kbId", handler.GetKB)
            kb.DELETE("/:kbId", handler.DeleteKB)

            docs := kb.Group("/:kbId/docs")
            {
              docs.GET("", handler.ListDocuments)
              docs.POST("", handler.UploadDocument)  // multipart
              docs.GET("/:docId", handler.GetDocument)
              docs.DELETE("/:docId", handler.DeleteDocument)
            }

            kb.POST("/:kbId/search", handler.SearchKB)
          }
        }

        // 新增: 会话记忆路由
        sessions.GET("/:sessionId/memories", handler.GetSessionMemories)
        sessions.DELETE("/:sessionId/memories/:memoryId", handler.DeleteSessionMemory)
      }
    }
    ```
  - 修改 `server/cmd/server/main.go` (或相关入口),在构造 Handler 时传入新字段 (从 Harness 中取)

  **Must NOT do**:
  - 不要删除现有路由 (向后兼容)
  - 不要在 SetupRoutes 中创建新的 Handler 实例如果 Handler 已存在 (保持 NewHandler 入口单一)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 路由注册 + DI 模式调整
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖 W5.1 + W5.2 + W4.2)
  - **Blocks**: W5.4 (集成测试用完整路由)、W6.1 (UI 调用这些端点)
  - **Blocked By**: W5.1, W5.2, W4.2

  **References**:
  - `server/internal/api/handlers.go:19-39` — 现有 Handler struct
  - `server/internal/api/handlers.go:29-39` — 现有 NewHandler 签名
  - `server/internal/api/handlers.go:311-332` — 现有 SetupRoutes,本任务扩展
  - `core/api.go` (grep 查找 APIProvider 定义) — 可能需要扩展 interface
  - `server/cmd/server/main.go` (grep 查找入口) — 注入新字段的地方
  - **为何这样设计**: 路由按资源嵌套 (/kb/:id/docs/:docId) 符合 RESTful 规范;conditional 注册 (knowledgeStore != nil 才挂载 /api/kb) 避免未配置 KB 时暴露无效端点

  **Acceptance Criteria**:
  - [ ] `cd server && go build ./...` — PASS (包括 cmd/server/main.go)
  - [ ] 所有新端点可通过 `gin.Engine.Routes()` 列出
  - [ ] 现有端点 (/api/sessions 等) 路由行为不变
  - [ ] Handler.knowledgeStore == nil 时不注册 /api/kb 路由 (GET /api/kb → 404)

  **QA Scenarios**:

  ```
  Scenario: 路由注册验证
    Tool: bash
    Steps:
      1. 启动 server (mock config + 启用 KB)
      2. 访问 http://localhost:PORT/api/kb → 200 (空列表)
      3. 访问 http://localhost:PORT/api/kb/new-kb-id → 404 (未创建)
      4. POST /api/kb → 201 + GET 验证
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.3-routes.txt
  ```

  ```
  Scenario: KB 未配置时 /api/kb 不可达
    Tool: bash
    Steps:
      1. 配置 config.yaml 不含 knowledge_bases
      2. 启动 server
      3. GET /api/kb → 404 (route not found,不是 500)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.3-no-kb-config.txt
  ```

  **Commit**: YES
  - Message: `feat(api): mount /api/kb/* routes + handler dependency injection`
  - Files: `server/internal/api/handlers.go`, `server/cmd/server/main.go` (或入口), `core/api.go` (如扩展 APIProvider)
  - Pre-commit: `cd server && go build ./...`

- [x] W5.4 API 集成测试 (httptest 全覆盖)

  **What to do**:
  - 扩展 `server/internal/api/knowledge_test.go` + `server/internal/api/memory_test.go`:
    - 设置: 创建临时 sqlite-vec + filememory,构造完整 Handler + gin.Engine,注册路由
    - 测试用例:
      - KB CRUD 全流程 (POST/GET/DELETE)
      - 文档上传 + 异步入库等待 ready
      - KB 检索测试 (依赖已入库数据)
      - 会话记忆 list + delete (含越权校验)
      - 错误分支: 重复 name、不存在 kbId、不支持 file 类型、跨 session 删 memory
      - 集成场景: 上传文档 → 等待 ready → chat → 验证 agent 基于 KB 内容回答 → 查看会话记忆
    - 使用 testutil (如存在) 简化 fixture 创建
  - 测试覆盖率目标: 新增 API 行覆盖 ≥ 80%
  - **性能相关测试**: 上传 1MB PDF,验证响应时间 < 500ms (因为异步入库)

  **Must NOT do**:
  - 不要在测试中启动真实服务器 (用 httptest.NewServer 或 mock)
  - 不要让测试依赖外部网络/服务
  - 不要修改 W5.1-W5.3 的实现 (如测试失败意味实现有 bug,应修 bug)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 测试覆盖矩阵大,需要仔细设计用例
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (W5.1-W5.3 必须先完成)
  - **Blocks**: FINAL F2 (代码质量审查)、F3 (manual QA 依赖这些端点)
  - **Blocked By**: W5.1, W5.2, W5.3

  **References**:
  - `server/internal/api/handlers_test.go` — 现有 API 测试模式 (参考 httptest setup)
  - `server/internal/testutil/` (如有) — 测试工具包
  - `core/providers/sqlitevec/` (W3.1) — in-memory sqlite 实例化用于测试
  - **为何这样设计**: 集成测试用完整栈 (router + handler + store) 验证真实行为,不仅仅是 handler 单独测试

  **Acceptance Criteria**:
  - [ ] `cd server && go test -v ./internal/api/...` — PASS,所有新 API 覆盖
  - [ ] `cd server && go test -cover ./internal/api/...` — 覆盖率 ≥ 80% (仅新增 API 部分)
  - [ ] 端到端集成测试:
    - 创建 KB → 上传 PDF → 等待 ready → 检索测试返回 chunks → agent chat 基于 KB 回答
    - List session memories 能看到 agent 积累的记忆 → delete 后不再显示
  - [ ] `cd server && go test -race ./internal/api/...` — 无 race (特别是异步入库 goroutine)

  **QA Scenarios**:

  ```
  Scenario: 端到端 KB 集成测试
    Tool: bash
    Steps:
      1. 启动 server (httptest.NewServer)
      2. POST /api/kb 创建 kb1
      3. POST /api/kb/kb1/docs 上传 testdata/sample.md
      4. 轮询 GET /api/kb/kb1/docs/:id 直到 status=ready
      5. POST /api/kb/kb1/search {"query": "sample"} → chunks 非空
      6. DELETE /api/kb/kb1 → 404 on GET
    Expected Result: 6 步全 PASS
    Evidence: .sisyphus/evidence/task-W5.4-kb-e2e.txt
  ```

  ```
  Scenario: 异步入库并发
    Tool: bash
    Steps:
      1. 并发 POST 5 个不同文档 (各 100KB)
      2. 等待所有 ready
      3. GET /api/kb/kb1/docs → 5 个 docs 全 ready
      4. 无 deadlock 无 race
    Expected Result: PASS,-race 无警告
    Evidence: .sisyphus/evidence/task-W5.4-concurrent-upload.txt
  ```

  **Commit**: YES
  - Message: `test(api): comprehensive httptest coverage for knowledge + memory endpoints`
  - Files: `server/internal/api/knowledge_test.go` (扩展), `server/internal/api/memory_test.go`
  - Pre-commit: `cd server && go test -race ./internal/api/...`

- [x] W5.5 `@copcon/chat-core` 类型扩展 (TypeScript)

  **What to do**:
  - 修改 `packages/chat-core/src/types.ts`:
    - 新增 type definitions (与 Go 后端严格对齐):
      ```typescript
      // 知识库
      export interface KnowledgeBase {
        id: string;
        name: string;
        backend: string; // "sqlite-vec"
        chunk_size?: number;
        chunk_overlap?: number;
        created_at: string;  // ISO datetime
        updated_at: string;
        metadata?: Record<string, any>;
        // 统计字段 (list/get API 返回时填充)
        document_count?: number;
        chunk_count?: number;
        token_count?: number;
      }

      // 文档状态
      export type DocumentStatus = "pending" | "parsing" | "ready" | "error";

      // 文档
      export interface Document {
        id: string;
        kb_id: string;
        filename: string;
        source: "upload" | "api" | "sync";
        status: DocumentStatus;
        chunk_count: number;
        token_count: number;
        created_at: string;
        updated_at: string;
        metadata?: Record<string, any>;
        error_message?: string; // 当 status=error 时填充
      }

      // Chunk
      export interface Chunk {
        id: string;
        document_id: string;
        kb_id: string;
        content: string;
        context?: string;  // 预留,本期不填充
        chunk_index: number;
        token_count: number;
        metadata?: Record<string, any>;
        score?: number;  // 检索时填充
      }

      // Retrieval Search Result (API /api/kb/:id/search)
      export interface SearchResult {
        chunks: Chunk[];
        query?: string;  // 原始 query
      }

      // Memory (来自会话,与 Go storage.Memory 对齐)
      export type MemoryType = "episodic" | "semantic" | "procedural" | "conversation";
      export interface Memory {
        id: string;
        content: string;
        session_id: string;
        role: string;
        timestamp: string;
        memory_type: MemoryType;
        metadata?: Record<string, any>;
        score?: number;
        importance?: number;
      }
      ```
    - snake_case 与 Go 后端 JSON 字段对齐 (不用 camelCase,保持传输兼容)
  - 新建单元测试 `packages/chat-core/src/types.test.ts`:
    - 验证 type 字段可被正确赋值 (类型检查)
    - 验证 snake_case 字段名与 Go 后端一致

  **Must NOT do**:
  - 不要使用 camelCase (除非 Go 后端 JSON 也用 camelCase)
  - 不要把 optional 字段省略 (用 `?` 标记 optional)
  - 不要在 types.ts 中写实现代码 (纯类型定义)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 纯类型定义,无逻辑
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W5.1-W5.4 后端 API 并行,因为只需类型定义,不需实现)
  - **Blocks**: W5.6 (AgentClient 用这些类型)、W6.x (UI 用)
  - **Blocked By**: W1.2 (后端的 KnowledgeBase 类型已定义,需要对齐)

  **References**:
  - `packages/chat-core/src/types.ts` — 现有类型定义,本任务扩展
  - `core/storage/knowledge.go` (W1.2) — 后端 KnowledgeBase/Document/Chunk struct,字段一一对应
  - `core/storage/memory.go` (W1.1) — 后端 Memory struct,对应前端 Memory type
  - `server/internal/api/knowledge.go` (W5.1) — API 返回的 JSON 字段
  - **为何这样设计**: snake_case 与 Go encoding/json 默认行为一致 (除非显式 `json:"..."` 覆盖,后端默认用 struct 字段的原名字)

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/chat-core build` — PASS, 0 type errors
  - [ ] `pnpm --filter @copcon/chat-core test` — PASS
  - [ ] types.ts 新增类型被正确 export (在 index.ts 中)
  - [ ] snake_case 字段与 Go 后端的 JSON tag 完全一致 (人工 review)

  **QA Scenarios**:

  ```
  Scenario: TypeScript 类型可被正确实例化
    Tool: bash
    Steps:
      1. 在 types.test.ts 中写: const kb: KnowledgeBase = { id: "...", name: "...", ... }
      2. compile 通过 + 测试 PASS
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.5-types.txt
  ```

  **Commit**: YES
  - Message: `feat(chat-core): add KnowledgeBase/Document/Chunk/Memory/SearchResult types`
  - Files: `packages/chat-core/src/types.ts`, `packages/chat-core/src/types.test.ts` (新建), `packages/chat-core/src/index.ts` (export)
  - Pre-commit: `pnpm --filter @copcon/chat-core build && pnpm --filter @copcon/chat-core test`

- [x] W5.6 `AgentClient` 方法扩展 (10+ 新方法)

  **What to do**:
  - 修改 `packages/chat-core/src/agent-client.ts`,在 class AgentClient 上新增方法:
    ```typescript
    // === 知识库管理 ===

    async listKnowledgeBases(): Promise<{ kbs: KnowledgeBase[] }> {
      const res = await fetch(`${this.baseUrl}/api/kb`);
      if (!res.ok) throw new Error(`listKBs failed: ${res.status}`);
      return res.json();
    }

    async createKnowledgeBase(name: string, opts?: { chunk_size?: number, chunk_overlap?: number }): Promise<KnowledgeBase> {
      const res = await fetch(`${this.baseUrl}/api/kb`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ name, ...opts }),
      });
      if (!res.ok) throw new Error(`createKB failed: ${res.status}`);
      return res.json();
    }

    async deleteKnowledgeBase(kbId: string): Promise<void> {
      const res = await fetch(`${this.baseUrl}/api/kb/${kbId}`, { method: 'DELETE' });
      if (!res.ok) throw new Error(`deleteKB failed: ${res.status}`);
    }

    async listDocuments(kbId: string, opts?: { status?: string, limit?: number, offset?: number }): Promise<{ documents: Document[] }> {
      const params = new URLSearchParams();
      if (opts?.status) params.set('status', opts.status);
      if (opts?.limit) params.set('limit', String(opts.limit));
      if (opts?.offset) params.set('offset', String(opts.offset));
      const res = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs?${params}`);
      if (!res.ok) throw new Error(`listDocs failed: ${res.status}`);
      return res.json();
    }

    async uploadDocument(kbId: string, file: File): Promise<Document> {
      const formData = new FormData();
      formData.append('file', file);
      const res = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs`, {
        method: 'POST',
        body: formData,  // 不要手动设置 Content-Type,浏览器会自动加 boundary
      });
      if (!res.ok) throw new Error(`uploadDoc failed: ${res.status}`);
      const data = await res.json();
      return data.documents[0];
    }

    async deleteDocument(kbId: string, docId: string): Promise<void> {
      const res = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs/${docId}`, { method: 'DELETE' });
      if (!res.ok) throw new Error(`deleteDoc failed: ${res.status}`);
    }

    async getDocumentChunks(kbId: string, docId: string): Promise<{ chunks: Chunk[] }> {
      const res = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs/${docId}?include_chunks=true`);
      if (!res.ok) throw new Error(`getDoc failed: ${res.status}`);
      const data = await res.json();
      return { chunks: data.chunks || [] };
    }

    async testRetrieval(kbId: string, query: string, topK?: number): Promise<SearchResult> {
      const res = await fetch(`${this.baseUrl}/api/kb/${kbId}/search`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ query, top_k: topK ?? 5 }),
      });
      if (!res.ok) throw new Error(`testRetrieval failed: ${res.status}`);
      return res.json();
    }

    // === Memory 管理 ===

    async getSessionMemories(sessionId: string, limit?: number): Promise<{ memories: Memory[] }> {
      const params = limit ? `?limit=${limit}` : '';
      const res = await fetch(`${this.baseUrl}/api/sessions/${sessionId}/memories${params}`);
      if (!res.ok) throw new Error(`getSessionMemories failed: ${res.status}`);
      return res.json();
    }

    async deleteSessionMemory(sessionId: string, memoryId: string): Promise<void> {
      const res = await fetch(`${this.baseUrl}/api/sessions/${sessionId}/memories/${memoryId}`, { method: 'DELETE' });
      if (!res.ok) throw new Error(`deleteSessionMemory failed: ${res.status}`);
    }
    ```
  - 在 `packages/chat-core/src/index.ts` 导出新增方法:
    - 方法已通过 class 自动导出 (import { AgentClient } from './agent-client')
    - 类型已在 W5.5 导出

  **Must NOT do**:
  - 不要手动设置 multipart 的 Content-Type header (浏览器会自动加 boundary)
  - 不要把所有错误都抛 Error (考虑返回 `{error: "..."}` 让调用方处理)
  - 不要在 AgentClient 中实现 retry 逻辑 (让上层处理)
  - 不要添加新的第三方依赖 (fetch 是 standard)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 10+ 方法,需要仔细处理 multipart、error、pagination
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: 与 W5.5 串行 (依赖 types);与 W5.1-W5.4 完全并行 (前端不依赖后端代码)
  - **Blocks**: W6.x (UI 调用这些方法)
  - **Blocked By**: W5.5

  **References**:
  - `packages/chat-core/src/agent-client.ts` — 现有 AgentClient 实现 (参考 fetch 调用、错误处理模式)
  - `packages/chat-core/src/types.ts` (W5.5) — 类型定义
  - `server/internal/api/knowledge.go` (W5.1) — 后端 API 字段 (snake_case)
  - **为何这样设计**: 与后端字段完全对齐 (snake_case),避免字段转换;multipage 处理在 listDocuments (通过 limit/offset)

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/chat-core build` — PASS
  - [ ] `pnpm --filter @copcon/chat-core test` — PASS (新增方法 mock fetch 测试)
  - [ ] Mock fetch 测试覆盖:
    - listKnowledgeBases happy + 500 error
    - uploadDocument multipart 不手动设置 Content-Type
    - deleteDocument 204 处理
    - testRetrieval query + top_k 参数
  - [ ] 所有新方法正确 export 自 `@copcon/chat-core`

  **QA Scenarios**:

  ```
  Scenario: uploadDocument 不调用 setRequestHeader('Content-Type')
    Tool: bash (vitest)
    Steps:
      1. 用 vitest mock global fetch
      2. 调用 uploadDocument
      3. 断言 mock fetch 收到的 headers 不含 'Content-Type' (或值为 null)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.6-no-manual-ct.txt
  ```

  ```
  Scenario: 所有新方法可被外部 import 调用
    Tool: bash
    Steps:
      1. 在测试中 import { AgentClient } from '@copcon/chat-core'
      2. 实例化 client,调用 10+ 个新方法
      3. Mock fetch,断言所有调用成功
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.6-all-methods.txt
  ```

  **Commit**: YES
  - Message: `feat(chat-core): extend AgentClient with 10 KB + memory API methods`
  - Files: `packages/chat-core/src/agent-client.ts`
  - Pre-commit: `pnpm --filter @copcon/chat-core build && pnpm --filter @copcon/chat-core test`

- [x] W5.7 `chat-core` 单元测试 (vitest 全覆盖)

  **What to do**:
  - 扩展或新建 `packages/chat-core/src/agent-client.test.ts`:
    - 为 W5.5 + W5.6 的每个新方法编写单元测试:
      - Mock fetch (用 vitest 的 `vi.fn()` 拦截 `global.fetch`)
      - 验证请求 URL、method、headers、body 正确
      - 模拟各种响应: 200/201/204/400/404/500
    - 测试覆盖点:
      - listKnowledgeBases: 空数组、多 KB、分页
      - createKnowledgeBase: happy + 409 (name 重复)
      - deleteKnowledgeBase: 204 (成功) + 404 (不存在)
      - listDocuments: 带 status/limit/offset 参数
      - uploadDocument: FormData 构造正确
      - deleteDocument: 204 + 404
      - getDocumentChunks: 含/不含 chunks
      - testRetrieval: 默认 top_k + 自定义 top_k
      - getSessionMemories: 默认 limit + 自定义 limit
      - deleteSessionMemory: 204 + 404
    - 错误处理: 非 200 状态码抛出含 HTTP code 的 Error
  - 运行:`pnpm --filter @copcon/chat-core test`

  **Must NOT do**:
  - 不要在测试中实际发起网络请求 (全部 mock)
  - 不要为 types.ts 写测试 (类型编译正确即可)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 大量测试用例 (每个方法 ≥ 2 case)
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖 W5.6 完成)
  - **Blocks**: FINAL F2 (代码质量)
  - **Blocked By**: W5.5, W5.6

  **References**:
  - `packages/chat-core/vitest.config.ts` — 现有 vitest 配置
  - `packages/chat-core/src/message-reducer.test.ts`、`packages/chat-core/src/sse-parser.test.ts`、`packages/chat-core/src/utils.test.ts` — 现有测试风格参考
  - `packages/chat-core/src/agent-client.ts` (W5.6 实现) — 测试对象
  - **为何这样设计**: 单元测试覆盖所有新方法 + 错误路径;mock fetch 让测试快且稳定

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/chat-core test` — PASS,覆盖 ≥ 15 个测试用例
  - [ ] 每个新方法至少 2 case (happy + 1 error)
  - [ ] 测试运行时间 < 5s
  - [ ] 测试报告含覆盖率信息 (如 vitest 配置了)

  **QA Scenarios**:

  ```
  Scenario: 全测试套件 PASS
    Tool: bash
    Steps:
      1. pnpm --filter @copcon/chat-core test
      2. 输出含 "✓ X tests passed"
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.7-all-tests.txt
  ```

  ```
  Scenario: Mock fetch 验证请求构造
    Tool: bash (vitest)
    Steps:
      1. 在 testRetrieval 测试中,断言 fetch 收到 {method: 'POST', body: '{"query":"...","top_k":5}'}
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W5.7-request-shape.txt
  ```

  **Commit**: YES
  - Message: `test(chat-core): vitest unit tests for all new AgentClient methods with mocked fetch`
  - Files: `packages/chat-core/src/agent-client.test.ts`
  - Pre-commit: `pnpm --filter @copcon/chat-core test`



### Wave 5 — W6: Demo UI 生产可用 (3 agents 并行)

- [x] W6.1 Demo 布局重构 (Tabs: 聊天/知识库/记忆)

  **What to do**:
  - 重构 `packages/demo/src/App.tsx`:
    - 把现有聊天相关 JSX (sessions/conversations/bubbles/sender/todo sidebar) 抽取到 `packages/demo/src/pages/ChatPage.tsx`
    - `App.tsx` 变成顶层 Tabs 容器:
      ```tsx
      import { Tabs } from 'antd';
      import { MessageOutlined, BookOutlined, MemoryOutlined } from '@ant-design/icons';

      const App: React.FC = () => {
        const [activeTab, setActiveTab] = useState('chat');
        return (
          <XProvider>
            <Tabs
              activeKey={activeTab}
              onChange={setActiveTab}
              items={[
                { key: 'chat', label: <><MessageOutlined /> 聊天</>, children: <ChatPage /> },
                { key: 'kb', label: <><BookOutlined /> 知识库</>, children: <KnowledgePage /> },
                { key: 'memory', label: <><MemoryOutlined /> 记忆</>, children: <MemoryPage /> },
              ]}
              tabBarStyle={{ padding: '0 16px', marginBottom: 0 }}
            />
          </XProvider>
        );
      };
      ```
    - **保留**所有现有聊天功能 (ChatPage 完全等价于原 App.tsx 主功能)
  - 新建 `packages/demo/src/pages/ChatPage.tsx`:
    - 移入原 App.tsx 的全部逻辑
    - 接收 `client: AgentClient` 作为 props (原文件是顶层 const)
    - 导出为默认 component
  - 新建 `packages/demo/src/pages/KnowledgePage.tsx` — 占位骨架 (后续 W6.2-W6.6 填充):
    ```tsx
    const KnowledgePage: React.FC = () => {
      return <Flex>知识库页(待实现)</Flex>;
    };
    ```
  - 新建 `packages/demo/src/pages/MemoryPage.tsx` — 占位骨架 (W6.7 填充)
  - 更新 `packages/demo/src/App.css`:
    - Tabs 全屏高度
    - 每个 tab 的 content 用 `height: calc(100vh - tabbar height)` 填满
  - **可访问性**: 每个 Tab 用 `aria-label`,支持键盘切换 (方向键 + Enter)

  **Must NOT do**:
  - 不要丢失任何现有聊天功能 (TodoList、StepContent、ThinkingBlock、ToolCallCard 等必须仍然工作)
  - 不要修改 `@copcon/chat-core` 或 `@copcon/chat-react` 的代码 (它们不应感知布局重构)
  - 不要做响应式 (用户明确拒绝)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: 布局重构,需要视觉 + 工程平衡;保证现有功能不回归
  - **Skills**: `['/frontend-ui-ux']` (如可用,帮助 UI 决策)

  **Parallelization**:
  - **Can Run In Parallel**: YES (作为 UI wave 的入口任务,启动后立即并行 W6.2-W6.9 子任务)
  - **Blocks**: W6.2-W6.7 (各页面开发依赖 Tabs 容器存在)
  - **Blocked By**: W5.1-W5.3 (后端路由注册完成), W5.5-W5.6 (types + AgentClient 方法)

  **References**:
  - `packages/demo/src/App.tsx` (现有 ~450 行) — 完整重构对象
  - `@ant-design/x` Tabs 组件文档 + `antd Tabs` 文档 (v6 API)
  - `@ant-design/icons` MessageOutlined, BookOutlined (可能存在的图标,否则用自定义图标)
  - `packages/demo/src/App.css` — 现有样式表
  - **为何这样设计**: Tabs 是单页应用切换视图的最简单方式;ChatPage 抽取保持现有功能完整;KnowledgePage/MemoryPage 占位让 W6.2-W6.7 子任务可并行填充

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/demo build` — PASS
  - [ ] `pnpm --filter @copcon/demo dev` 启动成功,浏览器看到 3 个 tabs (聊天/知识库/记忆)
  - [ ] 聊天 tab: 所有现有功能工作 (sessions, bubbles, sender, TodoList, SubagentCard, HumanInteraction, StepContent)
  - [ ] 知识库/记忆 tab: 显示占位文字 (待 W6.2/W6.7 填充)
  - [ ] Tab 切换无 re-render 整个聊天 (保持 ChatPage 状态)
  - [ ] 键盘 Tab + Arrow 可切换 tabs (可访问性)

  **QA Scenarios**:

  ```
  Scenario: 3 tabs 渲染 + 切换
    Tool: Playwright
    Steps:
      1. 启动 demo app
      2. 打开浏览器,访问 http://localhost:5173
      3. 截屏 .sisyphus/evidence/task-W6.1-tab-chat.png
      4. 点击 "知识库" tab
      5. 截屏 .sisyphus/evidence/task-W6.1-tab-kb.png
      6. 点击 "记忆" tab
      7. 截屏 .sisyphus/evidence/task-W6.1-tab-memory.png
    Expected Result: 3 个截屏显示不同内容;聊天 tab 含 Bubble.List;KB/Memory tab 含占位文字
    Evidence: .sisyphus/evidence/task-W6.1-*.png
  ```

  ```
  Scenario: 聊天 tab 功能完整
    Tool: Playwright
    Steps:
      1. 切换到 chat tab
      2. 点击 "New" 按钮 → 创建新会话
      3. 在 sender 输入 "hello" 提交
      4. 观察 Bubble.List 中出现用户消息 + AI 回复
      5. 右侧 TodoList 仍然可见(可能空)
      6. 截屏 .sisyphus/evidence/task-W6.1-chat-e2e.png
    Expected Result: PASS,无报错
    Evidence: .sisyphus/evidence/task-W6.1-chat-e2e.png
  ```

  **Commit**: YES
  - Message: `refactor(demo): top-level Tabs (Chat/KB/Memory) + extract ChatPage`
  - Files: `packages/demo/src/App.tsx`, `packages/demo/src/pages/ChatPage.tsx` (新建), `packages/demo/src/pages/KnowledgePage.tsx` (占位), `packages/demo/src/pages/MemoryPage.tsx` (占位), `packages/demo/src/App.css`
  - Pre-commit: `pnpm --filter @copcon/demo build`

- [x] W6.2 知识库列表页 (KBList)

  **What to do**:
  - 新建 `packages/demo/src/components/kb/KBList.tsx`:
    - Props: `kbs: KnowledgeBase[]`, `loading: boolean`, `onCreate: () => void`, `onSelect: (kbId) => void`, `onDelete: (kbId) => void`
    - 渲染:
      - 顶部: "知识库" 标题 + [+ 新建] 按钮 (Ant Design Button)
      - 主体: Ant Design `Row` + `Col` 网格 + Card 列表
      - 每个 Card:
        - 标题: KB name
        - content: `${document_count} 文档 · ${chunk_count} 分块 · ${token_count} tokens`
        - 标签: badge 显示 backend (如 "sqlite-vec")
        - 创建时间: `new Date(created_at).toLocaleString()`
        - 操作: Dropdown 菜单 (详情 / 删除)
      - 空状态: Ant Design `Empty` + 引导文案 "暂无知识库,点击右上角 '新建'"
      - Loading 状态: `Spin` 全局 loading
  - 完善 `packages/demo/src/pages/KnowledgePage.tsx`:
    - 用 `useEffect` 调用 `client.listKnowledgeBases()` 加载
    - 用 antd `Modal.confirm` 做删除确认
    - 用 antd `Modal` + Form 做创建表单 (name input, backend select 默认 sqlite-vec)
    - 状态管理: useState<kbs>, useState<loading>, useState<selected>
    - 选中 KB 后,显示右侧 `KBDetail` 组件 (W6.3)
    - 整体布局: 左右分栏 (左侧 300px KBList,右侧 KBDetail)
  - 新建 CreateKBModal 子组件 (form 校验: name 必填、不重复)

  **Must NOT do**:
  - 不要直接在 KBList 组件内 fetch (通过 props.onXxx 回调或 context 拿 client)
  - 不要让删除按钮不确认 (误操作风险)
  - 不要实现响应式 (固定 300px 左侧栏)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: 标准 Card 列表 + Modal,主要是 antd 组件组合
  - **Skills**: `['/frontend-ui-ux']`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W6.4/W6.7 并行)
  - **Blocks**: W6.3 (详情依赖列表的 onSelect)
  - **Blocked By**: W6.1

  **References**:
  - `packages/demo/src/App.tsx` — 参考现有 Conversations 列表模式 + antd 使用风格
  - `https://ant.design/components/card-cn` — Ant Design Card 文档
  - `https://ant.design/components/modal-cn` — Modal 对话框 (确认 + 表单)
  - `https://ant.design/components/empty-cn` — Empty 空状态
  - `packages/chat-core/src/agent-client.ts` (W5.6) — listKnowledgeBases / createKnowledgeBase / deleteKnowledgeBase
  - `packages/chat-core/src/types.ts` (W5.5) — KnowledgeBase type
  - **为何这样设计**: Card 网格比 Table 更视觉化,适合知识库这种有 "身份" 的资源;Dropdown 操作隐藏低频动作 (删除);空状态引导创建

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/demo build` — PASS
  - [ ] KBList 渲染 0/1/多 KB 三种状态视觉正确 (截屏对比)
  - [ ] [+ 新建] 弹 Modal,表单校验 name 必填
  - [ ] 删除确认 Modal 阻止误操作
  - [ ] 选中 KB 触发 props.onSelect(kbId)

  **QA Scenarios**:

  ```
  Scenario: 空状态引导
    Tool: Playwright
    Steps:
      1. 清空所有 KB
      2. 切换到 KB tab
      3. 截屏 .sisyphus/evidence/task-W6.2-empty.png
      4. 断言显示 "暂无知识库" 文案 + 新建按钮可见
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.2-empty.png
  ```

  ```
  Scenario: KB 卡片渲染 + 选择
    Tool: Playwright
    Steps:
      1. 创建 3 个 KB (company, product, faq)
      2. 切换到 KB tab
      3. 截屏 .sisyphus/evidence/task-W6.2-cards.png
      4. 点击其中一个 KB
      5. 验证右侧 KBDetail 显示对应 KB (即使为空也显示)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.2-cards.png
  ```

  ```
  Scenario: 删除确认 Modal
    Tool: Playwright
    Steps:
      1. Hover KB 卡片,点击 Dropdown "删除"
      2. 弹出确认 Modal
      3. 截屏 .sisyphus/evidence/task-W6.2-confirm.png
      4. 点"取消" → KB 仍在
      5. 点"确认" → KB 消失
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.2-confirm.png
  ```

  **Commit**: YES
  - Message: `feat(demo): KBList page with Card grid, create modal, delete confirm`
  - Files: `packages/demo/src/components/kb/KBList.tsx`, `packages/demo/src/components/kb/CreateKBModal.tsx`, `packages/demo/src/pages/KnowledgePage.tsx`
  - Pre-commit: `pnpm --filter @copcon/demo build`

- [x] W6.3 知识库详情页 (KBDetail)

  **What to do**:
  - 新建 `packages/demo/src/components/kb/KBDetail.tsx`:
    - Props: `kb: KnowledgeBase`, `onUpload: () => void`, `onTestRetrieval: () => void`, `onSelectDoc: (docId) => void`, `onDeleteDoc: (docId) => void`
    - 顶部: KB name + 整体统计 (`文档: X · 分块: Y · Tokens: Z`)
    - 中间: Ant Design `Table` 列:
      - 文件名 (`doc.filename`)
      - 来源 (badge: upload/api/sync)
      - 状态 (Tag 颜色: pending=processing, parsing=default, ready=success, error=error)
      - Chunk 数 / Token 数
      - 创建时间
      - 操作 (查看 chunks / 下载 / 删除)
    - Table 支持:
      - 排序 (按 filename / status / created_at)
      - 筛选 (status filter: 全部/pending/parsing/ready/error)
      - 分页 (每页 20)
    - 顶部工具栏: [上传文档] + [检索测试] 按钮
    - Loading 状态: Table 内置 loading=true
    - 空状态: "暂无文档,点击 '上传文档' 开始"
  - 新建 StatusBadge 子组件 (Tag 颜色映射)
  - 新建 DocumentStats 子组件 (顶部统计卡片)

  **Must NOT do**:
  - 不要让状态颜色硬编码 (用 antd token 的 colorSuccess/error 等)
  - 不要实现复杂的 Table row selection + bulk actions (本期不做)
  - 不要支持响应式 (Table 横向滚动即可)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: antd Table 是主力组件,需熟悉其配置
  - **Skills**: `['/frontend-ui-ux']`

  **Parallelization**:
  - **Can Run In Parallel**: 与 W6.4 (上传组件) 串行更安全 (共享 toolbar);与 W6.5/6.6 并行可行
  - **Blocks**: W6.5 (ChunkViewer 从本组件触发)、W6.6 (检索测试从本组件触发)
  - **Blocked By**: W6.2

  **References**:
  - `packages/demo/src/App.tsx` (现有 Bubble.List 列表风格) — 参考状态颜色 + 标签使用
  - `https://ant.design/components/table-cn` — antd Table 文档 (columns, filters, pagination)
  - `https://ant.design/components/tag-cn` — Tag 组件 (状态颜色)
  - `https://ant.design/components/statistic-cn` — Statistic 组件 (可选用于顶部统计)
  - `packages/chat-core/src/types.ts` (W5.5) — Document/DocumentStatus types
  - **为何这样设计**: Table 是展示结构化数据的最佳组件;Tag 颜色让状态一目了然 (绿/黄/红/蓝);操作列把低频动作 (chunk viewer/delete) 集中

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/demo build` — PASS
  - [ ] Table 渲染 0/多文档正确 (空状态 + 多行)
  - [ ] status Tag 颜色 4 种正确映射 (pending→processing, parsing→default+spin, ready→success, error→error)
  - [ ] Table 支持 status 筛选
  - [ ] Table 支持点击列排序
  - [ ] "查看 chunks" 按钮触发 props.onSelectDoc(docId)

  **QA Scenarios**:

  ```
  Scenario: 多文档 Table 渲染
    Tool: Playwright
    Steps:
      1. 上传 5 个文档 (PDF/MD/TXT/HTML + 一个失败的)
      2. 切到 KB tab,选择对应 KB
      3. 截屏 .sisyphus/evidence/task-W6.3-table.png
      4. 验证 5 行,状态颜色各异
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.3-table.png
  ```

  ```
  Scenario: status 筛选 + 排序
    Tool: Playwright
    Steps:
      1. Table 含 5 行 (2 ready, 1 error, 1 pending, 1 parsing)
      2. 点 status 列 filter,选 ready
      3. 验证只剩 2 行
      4. 点 created_at 列排序,验证顺序变化
      5. 截屏 .sisyphus/evidence/task-W6.3-filter.png
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.3-filter.png
  ```

  **Commit**: YES
  - Message: `feat(demo): KBDetail page with Table + status badges + stats`
  - Files: `packages/demo/src/components/kb/KBDetail.tsx`, `packages/demo/src/components/kb/StatusBadge.tsx`, `packages/demo/src/components/kb/DocumentStats.tsx`
  - Pre-commit: `pnpm --filter @copcon/demo build`

- [x] W6.4 文档上传组件 (KBUpload,drag-drop + 多文件)

  **What to do**:
  - 新建 `packages/demo/src/components/kb/KBUpload.tsx`:
    - 用 antd `Upload.Dragger` 组件
    - Props: `kbId: string`, `onSuccess: () => void` (上传成功后刷新文档列表), `onError: (msg) => void`
    - 功能:
      - 拖拽文件到组件上传 + 点击选择文件
      - 多文件批量上传 (并行上传,不阻塞 UI)
      - 文件类型限制: `accept=".pdf,.md,.txt,.html"` + MIME type check
      - 文件大小限制: `beforeUpload` 校验 ≤ 10MB,超过弹 message.error
      - 上传进度条 (antd Upload 内置 `percent`)
      - 上传成功后调用 props.onSuccess 触发父组件刷新列表
      - 上传失败时 message.error 显示原因
    - 使用 AgentClient.uploadDocument (FormData 自动处理)
  - 集成到 KBDetail (W6.3) 顶部工具栏的 [上传文档] 按钮:
    - 点按钮弹 Modal,Modal 内嵌 KBUpload
    - 上传完成后关闭 Modal + 刷新

  **Must NOT do**:
  - 不要手动设置 `Content-Type: multipart/form-data` (浏览器自动加 boundary,参考 W5.6)
  - 不要阻塞 UI (每个文件异步上传,UI 立即响应)
  - 不要让上传失败导致整个批次中断 (继续上传其他文件)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: Upload.Dragger + 异步控制
  - **Skills**: `['/frontend-ui-ux']`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W6.5/6.6/6.7 并行)
  - **Blocks**: F3 (manual QA)
  - **Blocked By**: W6.1, W5.6 (uploadDocument method)

  **References**:
  - `https://ant.design/components/upload-cn` — antd Upload 组件文档 (Dragger/multiple/beforeUpload/onChange)
  - `packages/chat-core/src/agent-client.ts` (W5.6) — uploadDocument 方法
  - **为何这样设计**: Dragger 提供最佳上传 UX;并发上传提升批量效率;beforeUpload 在客户端校验减少服务端压力

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/demo build` — PASS
  - [ ] 拖拽 PDF 上传 → 进度条 → 成功 → 文档列表刷新
  - [ ] 上传 .zip → beforeUpload 弹 message.error "不支持的格式"
  - [ ] 上传 15MB 文件 → 弹 message.error "文件超过 10MB 限制"
  - [ ] 批量上传 5 个文件,各文件独立进度独立成功/失败

  **QA Scenarios**:

  ```
  Scenario: 拖拽上传 PDF + 进度条
    Tool: Playwright
    Steps:
      1. 打开 KB detail 页
      2. 点击 "上传文档" 打开 Modal
      3. Playwright 用 setInputFiles 模拟拖拽 1 个 PDF
      4. 截屏 .sisyphus/evidence/task-W6.4-progress.png 显示进度
      5. 等待完成,截屏 .sisyphus/evidence/task-W6.4-done.png
      6. 关闭 Modal,验证 Table 刷新含新文档
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.4-*.png
  ```

  ```
  Scenario: 不支持格式拒绝
    Tool: Playwright
    Steps:
      1. setInputFiles .zip
      2. message.error "不支持的格式" 出现
      3. 截屏 .sisyphus/evidence/task-W6.4-reject.png
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.4-reject.png
  ```

  ```
  Scenario: 并发批量上传
    Tool: Playwright
    Steps:
      1. setInputFiles 5 个不同大小 MD 文件
      2. 5 个进度条独立显示
      3. 各文件独立成功,Table 出现 5 行
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.4-batch.png
  ```

  **Commit**: YES
  - Message: `feat(demo): KBUpload with DragDrop + multi-file + progress + error feedback`
  - Files: `packages/demo/src/components/kb/KBUpload.tsx`
  - Pre-commit: `pnpm --filter @copcon/demo build`

- [x] W6.5 分块预览 (ChunkViewer Drawer)

  **What to do**:
  - 新建 `packages/demo/src/components/kb/ChunkViewer.tsx`:
    - 用 antd `Drawer` 组件 (从右侧滑出,宽度 600-800px)
    - Props: `open: boolean`, `docId: string`, `onClose: () => void`, `kbId: string`
    - 功能:
      - 调用 `client.getDocumentChunks(kbId, docId)` 加载 chunks
      - Drawer title: "分块预览:<filename>"
      - 顶部: 搜索 Input (实时过滤,匹配 content)
      - 主体: List of cards,每个 card:
        - Header: `Chunk #<index>` + `Token 数: X`
        - Body: 文档 content (Markdown 渲染,高亮匹配关键词)
        - Footer: metadata 展示 (如有,用 Tag 显示)
      - 空搜索: "无匹配内容"
      - Loading: Skeleton 占位
    - 高亮实现: 用 `<mark>` HTML 标签包裹匹配词 (或 react-highlight-words 库,如果允许引入)
  - 集成到 KBDetail: Table 操作列 "查看 chunks" 按钮触发 Drawer

  **Must NOT do**:
  - 不要渲染所有 chunks 在初始 load 时 (文档可能很大,hundreds of chunks);用 antd List + 无限滚动,或简单分页
  - 不要让 Markdown 渲染支持任意 HTML (security);用安全的 markdown 库

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: Drawer + List + 高亮渲染
  - **Skills**: `['/frontend-ui-ux']`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W6.4/6.6/6.7 并行)
  - **Blocks**: F3 (manual QA)
  - **Blocked By**: W6.3 (触发入口)

  **References**:
  - `https://ant.design/components/drawer-cn` — Drawer 文档
  - `https://ant.design/components/list-cn` — List 文档 (itemLayout/loadMore 等)
  - `packages/demo/src/components/StreamMarkdown.tsx` — 现有 Markdown 渲染组件,复用
  - `packages/chat-core/src/agent-client.ts` (W5.6) — getDocumentChunks
  - **为何这样设计**: Drawer 不阻断主页面操作;关键词高亮让用户看到为什么 chunk 在搜索结果中

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/demo build` — PASS
  - [ ] 点击 Table "查看 chunks" 打开 Drawer
  - [ ] Drawer 列出该文档的所有 chunks,卡片样式清晰
  - [ ] 输入 "关键词" 实时过滤,匹配的 chunk 内高亮关键词
  - [ ] Loading 状态 (Skeleton) + 空状态 ("无匹配内容") 都正确

  **QA Scenarios**:

  ```
  Scenario: 分块预览 + 高亮搜索
    Tool: Playwright
    Steps:
      1. KBDetail 选中一个有 10 个 chunks 的文档
      2. 点击 "查看 chunks"
      3. Drawer 打开,显示 10 chunk cards
      4. 截屏 .sisyphus/evidence/task-W6.5-drawer.png
      5. 在搜索框输入 "PostgreSQL"
      6. 截屏 .sisyphus/evidence/task-W6.5-highlight.png,验证匹配 chunk 内 "PostgreSQL" 高亮
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.5-*.png
  ```

  **Commit**: YES
  - Message: `feat(demo): ChunkViewer Drawer with search + highlight`
  - Files: `packages/demo/src/components/kb/ChunkViewer.tsx`
  - Pre-commit: `pnpm --filter @copcon/demo build`

- [x] W6.6 检索测试组件 (KBRetrievalTest)

  **What to do**:
  - 新建 `packages/demo/src/components/kb/KBRetrievalTest.tsx`:
    - Props: `kbId: string`, `kbName: string` (显示在 header)
    - 功能:
      - 顶部: Input (query) + Search 按钮 + top_k Select (5/10/20)
      - 主体: 检索结果 List + Score 卡片:
        - 每个结果:
          - 头部: [Chunk #<idx>, doc: <filename>] + Score badge (颜色分级)
          - 内容: chunk.content (truncate 后 + "... 展开全文" 按钮)
          - 元信息: document 创建时间 / metadata
      - 空结果: Empty + "无匹配内容,尝试不同 query"
      - Loading: Spin
    - 分数分级颜色:
      - ≥ 0.8 → 绿色 (strong match)
      - 0.6 - 0.8 → 黄色 (moderate)
      - < 0.6 → 红色 (weak)
    - 集成到 KBDetail: [检索测试] 按钮弹 Modal,Modal 嵌入 KBRetrievalTest 组件
  - 使用 antd `message.success/error` 反馈异常

  **Must NOT do**:
  - 不要让 query 为空就搜索 (校验必填)
  - 不要显示 raw vector (对用户无意义)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: 输入 + 结果展示,标准模式
  - **Skills**: `['/frontend-ui-ux']`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W6.4/6.5/6.7 并行)
  - **Blocks**: F3 (manual QA)
  - **Blocked By**: W6.3 (触发入口)

  **References**:
  - `packages/chat-core/src/agent-client.ts` (W5.6) — testRetrieval
  - `packages/chat-core/src/types.ts` (W5.5) — SearchResult, Chunk
  - `https://ant.design/components/badge-cn` — Badge 组件 (score 显示)
  - `https://ant.design/components/input-cn` — Input + Search 形态
  - `https://ant.design/components/select-cn` — Select top_k
  - **为何这样设计**: Score 颜色分级让用户快速识别相关性强弱;Modal 内嵌让检索测试作为独立任务区

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/demo build` — PASS
  - [ ] 输入 query,点 "搜索" → 返回 chunks,按 score desc 排序
  - [ ] Score badge 颜色分级正确 (绿/黄/红)
  - [ ] 无结果时显示 Empty 引导
  - [ ] 点 "展开全文" 可看完整 chunk content

  **QA Scenarios**:

  ```
  Scenario: 检索测试返回 ranked chunks
    Tool: Playwright
    Steps:
      1. KB 已导入数据
      2. 打开检索测试 Modal
      3. 输入 "退款政策",选 top_k=5
      4. 点 "搜索"
      5. 截屏 .sisyphus/evidence/task-W6.6-results.png
      6. 验证 5 个 chunk cards,score 由高到低,颜色分级
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.6-results.png
  ```

  ```
  Scenario: 无匹配结果
    Tool: Playwright
    Steps:
      1. 输入完全无关 query "xyzabc123456789"
      2. 验证显示 Empty + 引导文案
      3. 截屏 .sisyphus/evidence/task-W6.6-empty.png
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.6-empty.png
  ```

  **Commit**: YES
  - Message: `feat(demo): KBRetrievalTest query + results with tiered score colors`
  - Files: `packages/demo/src/components/kb/KBRetrievalTest.tsx`
  - Pre-commit: `pnpm --filter @copcon/demo build`

- [x] W6.7 记忆管理页 (MemoryPage + MemoryPanel)

  **What to do**:
  - 完善 `packages/demo/src/pages/MemoryPage.tsx`:
    - 用 antd `Tabs` 二次嵌套 (每个 session 一个 sub-tab) 或 `Collapse` 分组显示
    - 主体: Ant Design `List` + 会话分组:
      - 每个会话组:
        - Header: 会话标题 (可点击切换到 chat tab)
        - 主体: Memory 列表 (Card)
          - 每个 memory card: content (Markdown 渲染),metadata tags (memory_type, 关键词), importance 进度条, timestamp, delete 按钮
        - 删除确认 + 删除后刷新列表
    - 顶部: 会话选择 Select (列出所有 session,选一个看其记忆) 或 Tabs
    - Loading + Empty 状态
  - 新建 `packages/demo/src/components/memory/MemoryPanel.tsx`:
    - 嵌入 ChatPage 右侧侧边栏 (与 TodoList 并列,可折叠)
    - 显示当前 session 的最近 N 条记忆 (默认 5,可配置)
    - 简化版: 仅显示 content 摘要 + metadata tag,不可删除 (删除必须到 Memory 页)
    - "查看全部记忆" 链接跳转到 Memory 页
  - 复用 `packages/demo/src/components/StreamMarkdown.tsx` 渲染记忆 content (如含 markdown)

  **Must NOT do**:
  - 不要让 MemoryPanel 在 chat 中每次消息更新时 refetch 记忆 (性能);只在 session 切换或定时 (如 30s) 刷新
  - 不要渲染过长的 memory content (truncate 200 chars + "... 展开")

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: List + Collapse + 跨 tab 链接
  - **Skills**: `['/frontend-ui-ux']`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W6.2-W6.6 完全并行)
  - **Blocks**: F3 (manual QA)
  - **Blocked By**: W6.1, W5.6

  **References**:
  - `packages/demo/src/App.tsx` — 现有 TodoList sidebar 模式,MemoryPanel 与之并列
  - `packages/chat-core/src/agent-client.ts` (W5.6) — getSessionMemories / deleteSessionMemory
  - `packages/chat-core/src/types.ts` (W5.5) — Memory type
  - `https://ant.design/components/collapse-cn` — Collapse 组件 (可选用于会话分组)
  - `https://ant.design/components/list-cn` — List 组件
  - `https://ant.design/components/progress-cn` — Progress 组件 (importance 可视化)
  - **为何这样设计**: MemoryPage 是完整管理界面 (含删除);MemoryPanel 是轻量的快速查看 (嵌入聊天侧边栏);两者复用大部分组件逻辑

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/demo build` — PASS
  - [ ] Memory Page: 选择 session → 显示该 session 的所有记忆 (按 timestamp desc)
  - [ ] Memory Page: 删除单条记忆,列表刷新,被删除的记忆消失
  - [ ] Memory Panel: 嵌入 ChatPage 右侧边栏,显示当前 session 最近 5 条记忆
  - [ ] Memory Panel: 切换 session 时自动刷新
  - [ ] "查看全部记忆" 链接跳转到 Memory Page

  **QA Scenarios**:

  ```
  Scenario: Memory Page 完整功能
    Tool: Playwright
    Steps:
      1. 先 chat 一会让 agent 积累记忆
      2. 切到 "记忆" tab
      3. 选择刚 chat 的 session
      4. 截屏 .sisyphus/evidence/task-W6.7-page.png
      5. 删除某条记忆
      6. 验证消失
      7. 截屏 .sisyphus/evidence/task-W6.7-after-delete.png
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.7-*.png
  ```

  ```
  Scenario: Chat 页面 MemoryPanel sidebar
    Tool: Playwright
    Steps:
      1. Chat tab,选中 session
      2. 观察右侧边栏,应看到 TodoList + MemoryPanel (两个可折叠部分)
      3. 截屏 .sisyphus/evidence/task-W6.7-panel.png
      4. 发送新消息让 agent 产生新记忆
      5. 等待 30s 或切换 session 回来
      6. MemoryPanel 显示新记忆
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.7-panel.png
  ```

  **Commit**: YES
  - Message: `feat(demo): MemoryPage + MemoryPanel with MD render + inline sidebar version`
  - Files: `packages/demo/src/pages/MemoryPage.tsx`, `packages/demo/src/components/memory/MemoryPanel.tsx`
  - Pre-commit: `pnpm --filter @copcon/demo build`

- [x] W6.8 UI polish (统一 Skeleton/Empty/ErrorBoundary/notifications + 主题 token)

  **What to do**:
  - 为所有新组件添加统一错误/加载/空状态处理:
    - **Skeleton**: 所有首次加载场景显示 Ant Design `Skeleton` (避免 layout shift)
      - KBList loading: Card 占位 3 个
      - KBDetail Table loading: Table 占位 5 行
      - ChunkViewer: List 占位 10 项
    - **Empty**: 所有空场景显示 antd `Empty` + 引导文案
      - 无 KB: "暂无知识库,点击右上角 '新建'"
      - 无文档: "暂无文档,点击 '上传文档' 开始"
      - 无记忆: "此会话尚未积累记忆,继续聊天试试吧"
      - 无检索结果: "无匹配内容,尝试调整查询或上传相关文档"
    - **ErrorBoundary**: 在 `packages/demo/src/pages/` 各组件外包裹 React ErrorBoundary
      - 捕获子组件崩溃
      - 显示友好 fallback: "某处出错了,请刷新页面或联系支持"
      - 可选: 实现自定义错误上报 (console.error)
    - **Notifications**: 用 antd `message` 全局通知:
      - 上传成功: `message.success('文件上传成功')` (或 "已加入队列,正在解析")
      - 上传失败: `message.error('上传失败: <原因>')`
      - 删除成功: `message.success('已删除')`
      - 网络错误: `message.error('网络错误: <原因>')`
      - 创建成功: `message.success('知识库创建成功')`
  - 主题一致性:
    - 所有颜色用 `token` 而非硬编码 (如 `token.colorSuccess` 而非 `#52c41a`)
    - 所有间距用 `token.padding`, `token.margin`
    - 用 `theme.useToken()` 在组件内获取
  - 代码清理:
    - 移除开发期 console.log
    - 删除未使用的 imports
    - 提取常量 (如 ACCEPTED_TYPES = ['.pdf', '.md', '.txt', '.html'])

  **Must NOT do**:
  - 不要引入新的大型依赖 (如第三方 ErrorBoundary lib);用 React 内置 ErrorBoundary 简单实现
  - 不要修改现有 StreamMarkdown / TodoList / 等组件 (除非 bug fix)
  - 不要硬编码颜色/间距 (用 token)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: UI polish 工作,需要审美 + 代码整理能力
  - **Skills**: `['/frontend-ui-ux']`, `['/ai-slop-remover']` (清理 slop)

  **Parallelization**:
  - **Can Run In Parallel**: NO (W6.2-W6.7 必须先完成)
  - **Blocks**: W6.9 (a11y 在 polish 后做)
  - **Blocked By**: W6.2, W6.3, W6.4, W6.5, W6.6, W6.7

  **References**:
  - `https://ant.design/components/skeleton-cn` — Skeleton
  - `https://ant.design/components/empty-cn` — Empty
  - `https://ant.design/components/message-cn` — message
  - `https://ant.design/docs/react/customize-theme-cn` — 主题 token 系统
  - `packages/demo/src/App.tsx` — 现有 style 参考
  - **为何这样设计**: 统一状态处理减少代码重复 + 提升 UX;ErrorBoundary 捕获 bug 不至于白屏

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/demo build` — PASS
  - [ ] `pnpm --filter @copcon/demo build` 无 "unused import" warnings
  - [ ] 各页面首次加载显示 Skeleton (非白屏)
  - [ ] 各页面空状态显示 Empty + 引导文案
  - [ ] 各操作 (上传/删除/创建) 成功失败都有 message 反馈
  - [ ] 子组件崩溃时 ErrorBoundary fallback 显示,不是整页白屏
  - [ ] 所有颜色/间距通过 token 引用 (grep `#` 和 `px` 字面量 ≤ 10 个,仅用于图标/micro layout)

  **QA Scenarios**:

  ```
  Scenario: 首次加载显示 Skeleton
    Tool: Playwright (throttle network 为 Slow 3G)
    Steps:
      1. 启动 dev,切换到 KB tab,首次加载
      2. 在数据返回前截屏
      3. 验证显示 Card skeleton 占位
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.8-skeleton.png
  ```

  ```
  Scenario: 空状态 Empty + 引导文案
    Tool: Playwright
    Steps:
      1. KB tab 无任何 KB + Memory tab 无任何 session
      2. 截屏验证 Empty 组件 + 引导文案
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.8-empty.png
  ```

  ```
  Scenario: ErrorBoundary 捕获崩溃
    Tool: Playwright
    Steps:
      1. 在某个组件中故意 throw Error (临时)
      2. 整页不崩溃,ErrorBoundary fallback 显示
      3. 截屏 .sisyphus/evidence/task-W6.8-errorboundary.png
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.8-errorboundary.png
  ```

  ```
  Scenario: 主题 token 一致性
    Tool: bash
    Steps:
      1. grep -r "#[0-9a-fA-F]\{3,8\}" packages/demo/src/ | wc -l (硬编码颜色)
      2. 结果应 ≤ 10 (仅用于极小范围,如图标颜色微调)
    Expected Result: ≤ 10
    Evidence: .sisyphus/evidence/task-W6.8-token-grep.txt
  ```

  **Commit**: YES
  - Message: `feat(demo): unified UI polish — Skeleton/Empty/ErrorBoundary/notifications with token`
  - Files: `packages/demo/src/components/kb/*.tsx` (修改), `packages/demo/src/components/memory/*.tsx` (修改), `packages/demo/src/pages/*.tsx` (修改), `packages/demo/src/ErrorBoundary.tsx` (新建)
  - Pre-commit: `pnpm --filter @copcon/demo build`

- [x] W6.9 可访问性 (ARIA + 键盘导航 + screen-reader friendly)

  **What to do**:
  - 为所有新组件添加可访问性属性:
    - **ARIA 标签**:
      - 所有按钮: `aria-label` (如 "上传文档", "删除知识库")
      - Input/Textarea: 关联 `<label>` 或 `aria-label`
      - 表格 (`Table`): antd Table 自动处理 ARIA,但需验证 columns.title 非空
      - 状态 badge 用 `aria-label` 或 `role="status"` 让屏幕阅读器读出状态
      - Modal / Drawer: antd 已内置 ARIA,确认 `title` 属性必填
    - **键盘导航**:
      - 所有按钮可 Tab 聚焦 + Enter/Space 触发
      - Tab 顺序逻辑: 顶部 toolbar → 列表 → 操作列 (避免乱序)
      - Modal / Drawer 打开时焦点自动移入 (antd 默认),关闭时回到触发按钮
      - 顶部 Tabs 用方向键 + Enter 切换 (antd 默认支持)
    - **屏幕阅读器**:
      - 状态变化用 `aria-live="polite"` 区 (如"上传完成"、"检索返回 5 个结果")
      - 图标按钮 (仅 icon) 必须有 `aria-label` (如 MenuFoldOutlined + "折叠侧边栏")
      - 图表/装饰性图标加 `aria-hidden="true"`
    - **颜色对比度**:
      - 文本与背景对比度 ≥ 4.5:1 (WCAG AA)
      - 可用 axe-core 检查
  - **axe-core 扫描**:
    - 安装 `@axe-core/playwright` (devDependency)
    - 在 Playwright 测试中运行:`await new AxeBuilder({page}).analyze()`
    - 期望: 0 critical violations,0 serious violations (minor 可接受)
  - 编写可访问性测试:
    - 用 Playwright `page.keyboard.press('Tab')` 验证焦点顺序
    - 用 `page.locator('button[aria-label="上传文档"]')` 验证 ARIA 属性

  **Must NOT do**:
  - 不要让所有按钮都加 `tabindex="-1"` (让按钮从 Tab 顺序消失是反模式)
  - 不要仅用 color 传达信息 (如仅用颜色区分状态);加图标或文字标签
  - 不要用 `outline: none` 移除焦点轮廓 (用 `:focus-visible` 优化外观但保留键盘可见性)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: a11y 工作,需要熟悉 ARIA 规范 + Playwright axe-core
  - **Skills**: `['/playwright']`, `['/frontend-ui-ux']`

  **Parallelization**:
  - **Can Run In Parallel**: NO (W6.8 必须先完成, polish 后再做 a11y)
  - **Blocks**: FINAL F3 (manual QA 验证 a11y)
  - **Blocked By**: W6.2-W6.8

  **References**:
  - WAI-ARIA Authoring Practices Guide: https://www.w3.org/WAI/ARIA/apg/
  - `https://ant.design/docs/react/accessibility-cn` — antd a11y 文档 (如有)
  - `https://github.com/dequelabs/axe-core-npm` — axe-core playwright integration
  - **为何这样设计**: ARIA 让屏幕阅读器可访问;keyboard nav 让 motor impairment 用户可用;axe-core 自动扫描捕获大部分问题

  **Acceptance Criteria**:
  - [ ] `pnpm --filter @copcon/demo build` — PASS
  - [ ] axe-core 扫描: 0 critical + 0 serious violations
  - [ ] Playwright `page.keyboard.press('Tab')` 可遍历所有输入框/按钮 (无 "keyboard trap")
  - [ ] Modal/Drawer 打开时焦点进入,关闭时焦点回到触发器
  - [ ] 所有 icon-only 按钮有 aria-label
  - [ ] 所有状态变化有 aria-live 区

  **QA Scenarios**:

  ```
  Scenario: axe-core 无 critical/serious violations
    Tool: Playwright + axe-core
    Steps:
      1. 启动 demo
      2. Playwright 遍历所有 tab
      3. 在每个 tab 运行 AxeBuilder.analyze()
      4. 断言 results.violations.filter(v => v.impact === 'critical' || v.impact === 'serious').length === 0
      5. 输出详细 report 到 .sisyphus/evidence/task-W6.9-axe.txt
    Expected Result: 0 critical + 0 serious
    Evidence: .sisyphus/evidence/task-W6.9-axe.txt
  ```

  ```
  Scenario: Tab 键遍历完整
    Tool: Playwright
    Steps:
      1. 在 Chat tab 上按 Tab 键 20 次
      2. 每次 focus 元素截屏
      3. 验证: sender input → send button → todo panel → memory panel (合理顺序)
    Expected Result: 焦点顺序逻辑
    Evidence: .sisyphus/evidence/task-W6.9-tab-order.gif
  ```

  ```
  Scenario: Modal 焦点陷阱
    Tool: Playwright
    Steps:
      1. 触发 "上传文档" Modal
      2. Modal 内按 Tab 键
      3. 焦点循环在 Modal 内(不会跑到背景)
      4. ESC 关闭 Modal,焦点回到触发按钮
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.9-modal-focus.txt
  ```

  ```
  Scenario: screen reader 状态播报
    Tool: bash (或 Playwright 模拟)
    Steps:
      1. aria-live 区域在"上传完成"时输出文本节点
      2. 验证 aria-live 属性值
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W6.9-aria-live.txt
  ```

  **Commit**: YES
  - Message: `feat(demo): accessibility — ARIA labels + keyboard navigation + screen-reader friendly`
  - Files: `packages/demo/src/components/kb/*.tsx` (修改), `packages/demo/src/components/memory/*.tsx` (修改), `packages/demo/tests/a11y.test.ts` (新建)
  - Pre-commit: `pnpm --filter @copcon/demo build`



### Wave 3+ — W7: 评估体系(可与 W4 并行启动)

- [x] W7.1 检索评估框架 (Go-native 指标)

  **What to do**:
  - 新建目录 `core/eval/`:
    - `retrieval.go` — 主文件,定义所有检索指标:
      ```go
      package eval

      // 检索测试用例 (query + 已知相关 doc ids 集合)
      type RetrievalTestCase struct {
        Query          string
        RelevantDocIDs []string  // ground truth
      }

      // 评测报告
      type RetrievalResult struct {
        RecallAtK    map[int]float64   // K → recall (e.g. {3: 0.78, 5: 0.85, 10: 0.92})
        PrecisionAtK map[int]float64   // K → precision
        MRR          float64            // Mean Reciprocal Rank
        NDCGAtK      map[int]float64   // K → nDCG
        HitRateAtK   map[int]float64   // K → % 至少命中 1 个相关 doc

        // 详细 (per query)
        QueryBreakdown []QueryScore     // 可选,用于诊断失败 query
      }

      type QueryScore struct {
        Query       string
        RecallAt5   float64
        MRR         float64
        FirstHitRank int  // -1 表示未命中
      }

      // Retriever 接口 (被测对象,通常是 KnowledgeStore.Search 的封装)
      type Retriever func(query string, k int) []string  // 返回 doc ids

      // 主评测函数
      func EvaluateRetrieval(
        testCases []RetrievalTestCase,
        retriever Retriever,
        ks []int,  // 评测不同 K 值 (如 [3, 5, 10])
      ) RetrievalResult { ... }
      ```
    - `metrics.go` — 内部指标计算:
      - `recallAtK(retrieved, relevant []string, k int) float64`
      - `precisionAtK(retrieved, relevant []string, k int) float64`
      - `mrr(retrieved, relevant []string) float64` — 第一个相关 doc 的 rank 的倒数
      - `ndcgAtK(retrieved, relevant []string, k int) float64` — binary relevance
      - `hitRateAtK(results []QueryScore, k int) float64` — 有 hit 的 query 占比
    - `reporter.go` — 结果格式化 (终端 + JSON):
      - `PrintSummary(result RetrievalResult)` — 控制台输出表格式 summary
      - `WriteJSON(result RetrievalResult, path string)` — 输出 JSON 供 CI 分析
    - `retrieval_test.go` — 单元测试:
      - Mock testCases + Mock retriever
      - 验证: 完美检索 → Recall=1, Precision=1, MRR=1, NDCG=1
      - 验证: 完全无检索 → 所有指标 = 0
      - 验证: 部分检索 → 手工计算期望值,断言一致

  **Must NOT do**:
  - 不要在 `core/eval/` 中依赖任何外部评测框架 (Ragas 等);Go-native 实现
  - 不要让评测函数调用 LLM (仅数学计算)
  - 不要硬编码指标阈值 (让 CI 工作流或调用方配置阈值)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 纯数学计算 + 单元测试
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W4 并行,与 W1.2 之后的任何任务独立)
  - **Blocks**: W7.2 (golden set 用此框架)、W7.3 (CI 用)
  - **Blocked By**: W3.4 (入库管线完成,才能产生 retriever,但评测函数本身不依赖)

  **References**:
  - Information Retrieval 教材 (标准指标定义,如 Croft 的 "Search Engines" 课本)
  - `core/storage/knowledge.go` (W1.2) — KnowledgeStore.Search 接口;retriever 函数包装此方法
  - **为何这样设计**: Go-native 实现无依赖,可在 CI 中快速运行;ks 参数可同时评测多个 K 值;QueryBreakdown 让诊断失败原因更简单

  **Acceptance Criteria**:
  - [ ] `cd core && go build ./eval/...` — PASS
  - [ ] `cd core && go test ./eval/...` — PASS (单元测试覆盖 5 指标)
  - [ ] 验证: 完美检索 → Recall/Precision/MRR/NDCG 全部 = 1.0
  - [ ] 验证: 完全无相关结果 → 全部 = 0.0
  - [ ] 验证: 手工计算的中等 case → 指标值正确

  **QA Scenarios**:

  ```
  Scenario: 完美检索 (所有 top-K 都是相关)
    Tool: bash
    Steps:
      1. 执行 cd core && go test -run TestEvaluateRetrieval_Perfect ./eval/... -v
      2. 测试用例: query → 应该检索 [d1, d2],retriever 返回 [d1, d2] (按顺序)
      3. 断言 Recall@5 = 1.0, Precision@5 = 2/5 = 0.4, MRR = 1.0, NDCG@5 = 1.0, HitRate@5 = 1.0
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W7.1-perfect.txt
  ```

  ```
  Scenario: 完全无匹配
    Tool: bash
    Steps:
      1. retriever 返回 [] (空)
      2. 断言所有指标 = 0
      3. MRR = 0 (no hit)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W7.1-empty.txt
  ```

  ```
  Scenario: JSON 报告输出
    Tool: bash
    Steps:
      1. 调用 WriteJSON(result, "/tmp/eval.json")
      2. cat /tmp/eval.json,验证格式正确
      3. 验证包含 RecallAtK/PrecisionAtK/MRR/NDCGAtK/HitRateAtK 字段
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W7.1-json.txt
  ```

  **Commit**: YES
  - Message: `feat(eval): Go-native retrieval metrics — Recall@K, Precision@K, MRR, nDCG, HitRate`
  - Files: `core/eval/retrieval.go`, `core/eval/metrics.go`, `core/eval/reporter.go`, `core/eval/retrieval_test.go`
  - Pre-commit: `cd core && go test ./eval/...`

- [x] W7.2 黄金测试集 (golden_set.jsonl,50 条)

  **What to do**:
  - 新建目录 `eval/testdata/`:
  - 新建 `eval/testdata/golden_set.jsonl` — 50 条测试用例,比例:
    - **60% 高频查询**(30 条): 来自真实场景的常见问题 (模拟客服/内部文档查询)
      - 示例:`{"query": "退款政策是什么?", "relevant_docs": ["refund_policy_v2.md"]}`
      - 示例:`{"query": "API 认证怎么配置?", "relevant_docs": ["api_auth.md", "security_guide.md"]}`
    - **20% 长尾查询**(10 条): 低频、模糊、歧义查询
      - 示例:`{"query": "公司上次融资是什么时候?", "relevant_docs": ["investor_relations_q2.md"]}`
    - **10% 对抗性查询**(5 条): 应被拒绝或知识库无答案
      - 示例:`{"query": "明天的天气怎么样?", "relevant_docs": []}` (KB 不含天气信息)
      - 示例:`{"query": "PostgreSQL vs MongoDB 哪个更好?", "relevant_docs": []}` (KB 不应主观判断)
    - **10% 时间漂移查询**(5 条): 文档近期变更过的 (用于验证 chunk 更新后检索仍能命中)
      - 示例:`{"query": "最新版本号是什么?", "relevant_docs": ["release_notes.md"]}`
  - 新建 `eval/testdata/fixtures/` — 测试用文档 (PDF/MD/TXT/HTML 各若干):
    - `refund_policy_v2.md`
    - `api_auth.md`
    - `security_guide.md`
    - `investor_relations_q2.md`
    - `release_notes.md`
    - ... (共 ~20 个文档,覆盖常见主题)
  - 新建 `eval/testdata/README.md` — 说明测试集构成、如何添加新用例、比例原则

  **Must NOT do**:
  - 不要使用真实客户文档 (用虚构数据)
  - 不要让 relevant_docs 过于集中 (应分散到多个文档)
  - 不要只放简单查询;必须包含模糊/对抗性/时间漂移等复杂 case

  **Recommended Agent Profile**:
  - **Category**: `quick` (但需要内容创作能力,可考虑 `writing` 类 agent)
    - Reason: 主要是内容创作 + 数据编排
  - **Skills**: `['writing']` (如可用) 或空

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 W7.1, W4, W5, W6 都并行)
  - **Blocks**: W7.3 (CI 调用此测试集)
  - **Blocked By**: None (可立即启动)

  **References**:
  - WBS W7.2 节 (原始计划提及的 60/20/10/10 分布)
  - **外部参考**: Anthropic Contextual Retrieval 论文的评测集构成 (如可查)
  - **为何这样设计**: 4 类查询分布覆盖真实场景 (高频) 和边缘场景 (长尾/对抗/漂移);让评测能捕获各类失败模式

  **Acceptance Criteria**:
  - [ ] `eval/testdata/golden_set.jsonl` 含 50 行
  - [ ] 50 行按 60/20/10/10 分布 (人工 review)
  - [ ] 所有 referenced documents 在 fixtures/ 中存在
  - [ ] JSONL 格式正确 (每行一个 JSON object,无语法错误)
  - [ ] 文档质量良好 (无拼写错误,内容合理)

  **QA Scenarios**:

  ```
  Scenario: golden_set.jsonl 格式校验
    Tool: bash
    Steps:
      1. cat eval/testdata/golden_set.jsonl | while read line; do jq -e . <<< "$line"; done
      2. 所有 50 行解析成功 (jq 返回 0)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W7.2-jsonl-valid.txt
  ```

  ```
  Scenario: referenced doc 存在性校验
    Tool: bash
    Steps:
      1. jq 解析每行 .relevant_docs,展开为列表
      2. 对每个 doc 检查 eval/testdata/fixtures/<doc> 存在
      3. 全部存在 → PASS
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W7.2-docs-exist.txt
  ```

  ```
  Scenario: 分布比例校验
    Tool: bash (脚本)
    Steps:
      1. 编写脚本统计 4 类查询数量
      2. 输出: 高频=30, 长尾=10, 对抗=5, 漂移=5
      3. 允许 ±5% 偏差
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W7.2-distribution.txt
  ```

  **Commit**: YES
  - Message: `eval: bootstrap golden_set.jsonl with 50 test cases (60/20/10/10 distribution)`
  - Files: `eval/testdata/golden_set.jsonl`, `eval/testdata/fixtures/*` (多个文档), `eval/testdata/README.md`
  - Pre-commit: 手动 review (无自动化脚本)

- [x] W7.3 CI 集成 (eval workflow + 质量门禁)

  **What to do**:
  - 新建 `.github/workflows/eval.yml`:
    ```yaml
    name: RAG Evaluation

    on:
      push:
        branches: [main, feat/v2]
      pull_request:
        branches: [main]

    jobs:
      retrieval-eval:
        runs-on: ubuntu-latest
        steps:
          - uses: actions/checkout@v4

          - uses: actions/setup-go@v5
            with:
              go-version: '1.26'

          - name: Build core
            run: cd core && go build ./...

          - name: Run retrieval evaluation
            run: |
              cd core
              go test -v -run TestRetrievalEval ./eval/... -golden=../eval/testdata/golden_set.jsonl
            env:
              # 如需环境变量 (OpenAI API key 等)
              OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}

          - name: Quality Gate
            run: |
              # 从测试结果中提取关键指标 (或调用 reporter 写 JSON)
              # 验证 Recall@5 >= 0.80, MRR >= 0.75
              cd core
              go test -v -run TestRetrievalEval_QualityGate ./eval/...
    ```
  - 在 `core/eval/` 中新增测试 `core/eval/golden_test.go` (或 `retrieval_test.go` 中):
    ```go
    func TestRetrievalEval_Golden(t *testing.T) {
      goldenPath := flag.String("golden", "../../../eval/testdata/golden_set.jsonl", "path to golden set")
      flag.Parse()

      // 1. 加载测试集
      cases := loadGoldenSet(*goldenPath)

      // 2. 构造 retriever (包装 KnowledgeStore.Search)
      // 注意: retriever 需要 sqlite-vec in-memory DB + 已入库 fixture documents
      // 这需要在测试 setup 阶段完成入库
      setup := setupTestKB(t)  // fixture docs 入库
      retriever := func(query string, k int) []string {
        vec, _ := setup.embedder.Embed(context.Background(), query)
        chunks, _ := setup.store.Search(context.Background(), setup.kbIDs, vec, storage.SearchOptions{TopK: k})
        return uniqueDocIDs(chunks)
      }

      // 3. 评测
      result := eval.EvaluateRetrieval(cases, retriever, []int{3, 5, 10})

      // 4. 输出报告 + 断言阈值
      eval.PrintSummary(result)
      eval.WriteJSON(result, "/tmp/retrieval_eval.json")

      // 质量门禁
      if result.RecallAtK[5] < 0.80 {
        t.Errorf("Recall@5 = %.2f < 0.80 threshold", result.RecallAtK[5])
      }
      if result.MRR < 0.75 {
        t.Errorf("MRR = %.2f < 0.75 threshold", result.MRR)
      }
    }
    ```
  - 添加 `eval/testdata/fixtures/` 入库逻辑 (测试 setup)
  - 可选:在 `README.md` 顶层说明如何本地运行 eval

  **Must NOT do**:
  - 不要在 CI 中调用外部 LLM API (除非 mock);golden test 使用 in-memory sqlite-vec + OpenAI Embedding (CI 中用 API key from secrets)
  - 不要让 CI 失败时隐藏错误原因 (日志详细输出)
  - 不要硬编码阈值 (可通过 env var 或 flag 配置)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 主要是 GitHub Actions 配置 + Go 测试
  - **Skills**: `['/git-master']` (如可用,帮助 Git workflow 设计)

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖 W7.1 评测框架 + W7.2 测试集)
  - **Blocks**: FINAL F1 (plan compliance 验证 CI 配置正确)
  - **Blocked By**: W7.1, W7.2

  **References**:
  - `.github/workflows/` (现有 workflow,如 `go.yml` 或类似) — 参考现有 CI 配置风格
  - `core/eval/retrieval.go` (W7.1) — 评测函数
  - `eval/testdata/golden_set.jsonl` (W7.2) — 测试集
  - `.github/actions/setup-go` 文档 — Go 环境配置
  - **为何这样设计**: GitHub Actions 内置支持 Go 环境;threshold 通过 Go test 断言而非脚本比较 (更易调试)

  **Acceptance Criteria**:
  - [ ] `.github/workflows/eval.yml` 文件有效 (YAML 语法正确)
  - [ ] `cd core && go test -run TestRetrievalEval_Golden ./eval/... -golden=...` — PASS (使用 in-memory sqlite-vec)
  - [ ] CI job 在 mock PR 触发时通过,显示 Recall@5 + MRR 指标
  - [ ] 模拟 Recall 低于 0.80 时,CI 失败 (用故意低质量的 retriever 测试)

  **QA Scenarios**:

  ```
  Scenario: 本地 eval 测试命令
    Tool: bash
    Steps:
      1. cd core && go test -v -run TestRetrievalEval_Golden ./eval/... -golden=../eval/testdata/golden_set.jsonl
      2. 输出报告,含 Recall@5 + MRR
      3. 测试 PASS (假设当前实现满足阈值)
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W7.3-local.txt
  ```

  ```
  Scenario: 阈值失败时 CI 阻断
    Tool: bash
    Steps:
      1. 临时把阈值改为 0.99 (故意失败)
      2. 重跑测试
      3. 测试失败,输出 "Recall@5 = 0.85 < 0.99 threshold"
      4. 恢复阈值
    Expected Result: 测试按预期失败
    Evidence: .sisyphus/evidence/task-W7.3-quality-gate.txt
  ```

  ```
  Scenario: GitHub Actions YAML 校验
    Tool: bash
    Steps:
      1. yamllint .github/workflows/eval.yml (或 go test 中)
      2. YAML 无语法错误
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-W7.3-yaml-valid.txt
  ```

  **Commit**: YES
  - Message: `ci: eval workflow with Recall@5>=0.80 + MRR>=0.75 quality gates`
  - Files: `.github/workflows/eval.yml`, `core/eval/golden_test.go` (新建), `README.md` (顶层,可选说明)
  - Pre-commit: `cd core && go test ./eval/...` + YAML lint



---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 个审查 agent 并行运行。ALL 必须 APPROVE。汇总结果给用户,获取明确 "okay" 才能完成。
> **未获得用户明确 okay 前,不得自动完成工作。**

- [x] F1. **计划合规审查** — `oracle`

  完整阅读本计划。逐项检查:
  - 每个 "Must Have" 是否在代码中实现(读文件 / 跑命令 / curl 端点)
  - 每个 "Must NOT Have" 是否在代码库中搜索禁止模式 (命中即拒绝,附 `file:line`)
  - `.sisyphus/evidence/` 是否所有 QA 场景证据文件齐备
  - 计划的 deliverables 与实际代码是否一一对照

  **输出格式**:
  ```
  Must Have: [N/7]
  Must NOT Have: [N/11] (违反项列表,每项附 file:line)
  Tasks: [43/43 completed]
  VERDICT: APPROVE | REJECT (附具体原因)
  ```

- [x] F2. **代码质量审查** — `unspecified-high`

  依次运行:
  1. `cd core && go vet ./...` + `go build ./...` + `go test ./...`
  2. `cd server && go build ./...` + `go test ./internal/...`
  3. `pnpm --filter @copcon/* build`
  4. `pnpm --filter @copcon/chat-core test`
  5. `git log --oneline main..HEAD | wc -l` (查看 commit 数量)

  审查所有改动文件:
  - 检查 Go 代码:`as any` / `@ts-ignore` 类型断言滥用 / 空 catch / 生产代码含 console.log / 注释掉的代码 / 未 import 的包
  - 检查 AI slop: `data` / `result` / `item` / `temp` 等泛化命名;过度注释;过度抽象
  - 检查模块边界: `core/` 不得 import `server/`

  **输出格式**:
  ```
  Build: [PASS|FAIL]
  Lint: [PASS|FAIL] (vet/tsc)
  Tests: [N pass / M fail]
  Files: [N clean / M with issues] (列问题 file:line)
  AI Slop: [N instances]
  VERDICT: APPROVE | REJECT
  ```

- [x] F3. **真实手工 QA** — `unspecified-high` + `playwright` skill

  从 **clean state** 启动 (新容器/新数据库/新 Qdrant 实例):
  1. 启动所有服务: `docker compose up -d` + `cd server && go run cmd/server/main.go` + `cd packages/demo && pnpm dev`
  2. 浏览器打开 demo 应用,Playwright 接管
  3. 按顺序执行:
     - W2 QA: 创建会话 → 聊天 → 验证 agent 调 memory_store 写入 MD 文件 → 关闭会话 → 新建会话 → 验证 INDEX.md 注入 prompt
     - W3 QA: 创建 KB → 上传 1 份 PDF 和 1 份 MD → 看到文档解析完成 → 点击文档查看 chunks → 在检索测试面板查询 → 看到带分数的结果
     - W6 QA: 切换到 Memory Tab → 查看会话记忆 → 删除单条记忆 → 切换回 Chat → 验证 UI 可访问性 (键盘 Tab 遍历所有按钮 / 输入框)
  4. 测试跨任务集成: 在 Chat 中提一个关于上传文档的问题 → agent 应能基于 KB 内容回答
  5. 测试边界情况: 上传 10MB 大文件 / 0 字节文件 / 不支持的格式 / 检索已删除的 KB / INDEX.md 超过 200 行时的截断行为

  所有证据保存到 `.sisyphus/evidence/final-qa/`

  **输出格式**:
  ```
  Scenarios: [N/N passed]
  Integration: [N/N]
  Edge Cases: [N tested / M passed]
  Accessibility: [PASS|FAIL] (axe-core 扫描)
  VERDICT: APPROVE | REJECT
  ```

- [x] F4. **范围保真检查** — `deep`

  对每个任务 (W1.1 - W7.3,共 43 个):
  - 读 "What to do" 和实际 git diff (按 commit)
  - 验证 1:1 对应关系: spec 中要求的都做了 (no missing) + nothing beyond spec (no creep)
  - 检查 "Must NOT do" 项是否违反
  - 检测跨任务污染: W2.x 是否动到了 W3.x 的文件 (不该动)
  - Flag 所有未被 spec 解释的改动

  **输出格式**:
  ```
  Tasks: [N/43 compliant]
  Contamination: [CLEAN | N issues with file:line]
  Unaccounted Changes: [CLEAN | N files with git hashes]
  VERDICT: APPROVE | REJECT
  ```

### Review Loop Protocol

```
While ANY F1-F4 REJECT:
  1. 分析拒绝原因
  2. 修复具体问题(不要过度改,只修复被标记的)
  3. 只重跑失败的那个审查 agent(F1/F2/F3/F4)
  4. 重新汇总全部结果,再次请求用户 okay
  → 不要自动跳过用户审批
```

---

## Commit Strategy

> 每个任务一个或多个**原子 commit**。commit message 遵循 `type(scope): desc` 约定。
> 每个任务完成时立即 commit,不要攒到 wave 末尾。
> 所有 commit 前必须跑对应验证命令,失败则不允许 commit。

| Task | Commit Message | Files to Include | Pre-commit Check |
|---|---|---|---|
| W1.1 | `feat(storage): enhance MemoryStore with list/get/update/delete + temporal fields` | `core/storage/memory.go`, 新测试文件 | `cd core && go test ./storage/...` |
| W1.2 | `feat(storage): add pluggable KnowledgeStore interface with Document/Chunk types` | `core/storage/knowledge.go`, `core/storage/provider.go` | `cd core && go build ./storage/...` |
| W1.3 | `feat(embedding): define Embedder interface with backend abstraction` | `core/providers/embedding/embedder.go` | `cd core && go build ./providers/...` |
| W1.4 | `feat(embedding): implement OpenAI embedder reusing existing LLMProvider` | `core/providers/embedding/openai.go`, `*_test.go` | `cd core && go test ./providers/embedding/...` |
| W1.5 | `feat(config): add MemoryConfig, KnowledgeBaseConfig, EmbeddingConfig structs` | `server/internal/config/config.go`, `server/config.yaml.template` | `cd server && go build ./internal/config/...` |
| W1.6 | `feat(agent): add Memory + KnowledgeBases fields to AgentSpec, define bundle names` | `core/agent.go`, `core/capabilities/bundle.go` | `cd core && go build ./...` |
| W1.7 | `feat(storage): add RegisterKnowledgeStoreProvider registry for backend dispatch` | `core/storage/knowledge_registry.go` | `cd core && go test ./storage/...` |
| W2.1 | `feat(filememory): implement file-based MemoryStore with YAML frontmatter + INDEX.md` | `core/providers/filememory/*.go`, `*_test.go` | `cd core && go test ./providers/filememory/...` |
| W2.2 | `feat(hooks): add FileMemoryHook for OnSystemPrompt MD injection` | `core/capabilities/hooks/file_memory.go`, `*_test.go` | `cd core && go test ./capabilities/hooks/...` |
| W2.3 | `feat(tools): implement memory_store/recall/forget tools for MD file ops` | `core/capabilities/tools/memory_{store,recall,forget}.go`, `*_test.go` | `cd core && go test ./capabilities/tools/...` |
| W2.4 | `feat(capabilities): register memory capability bundle with init()` | `core/capabilities/hooks/memory.go`, `core/capabilities/tools/memory_*.go` | `cd core && go build ./capabilities/...` |
| W2.5 | `test(memory): comprehensive filememory + hooks + tools unit tests` | `core/providers/filememory/*_test.go`, `core/capabilities/hooks/file_memory_test.go`, `core/capabilities/tools/memory_*_test.go` | `cd core && go test ./providers/filememory/... ./capabilities/...` |
| W3.1 | `feat(sqlitevec): implement KnowledgeStore with ncruces/go-sqlite3, pure Go, no CGO` | `core/providers/sqlitevec/*.go`, `*_test.go` | `cd core && go test ./providers/sqlitevec/...` |
| W3.2 | `feat(rag): add document parser with PDF/MD/TXT/HTML support` | `core/rag/parser.go`, `core/rag/pdf.go`, `core/rag/markdown.go`, `core/rag/text.go`, `core/rag/html.go`, `*_test.go` | `cd core && go test ./rag/...` |
| W3.3 | `feat(rag): add RecursiveChunker + MarkdownAwareChunker with configurable size/overlap` | `core/rag/chunker.go`, `*_test.go` | `cd core && go test ./rag/...` |
| W3.4 | `feat(rag): implement ingestion pipeline — Parse → Chunk → Embed → Store with async + progress` | `core/rag/pipeline.go`, `*_test.go` | `cd core && go test ./rag/...` |
| W3.5 | `feat(hooks): add KBRecallHook for AfterContextBuild vector retrieval across KBs` | `core/capabilities/hooks/kb_recall.go`, `*_test.go` | `cd core && go test ./capabilities/hooks/...` |
| W3.6 | `feat(hooks): add MemoryPersistHook with keyword-based fact extraction (no LLM)` | `core/capabilities/hooks/memory_persist.go`, `*_test.go` | `cd core && go test ./capabilities/hooks/...` |
| W3.7 | `feat(capabilities): register knowledge_base capability bundle` | `core/capabilities/hooks/kb_*.go`, `core/capabilities/bundle.go` | `cd core && go build ./capabilities/...` |
| W3.8 | `test(knowledge): testcontainers integration for sqlite-vec + pipeline end-to-end` | `core/providers/sqlitevec/*_test.go`, `core/rag/*_test.go` | `cd core && go test -tags=integration ./providers/sqlitevec/... ./rag/...` |
| W4.1 | `feat(harness): expand collectCapabilityNames for dual bundle (memory + knowledge_base)` | `core/harness.go` | `cd core && go build ./...` |
| W4.2 | `feat(harness): add KnowledgeStore + backend dispatch in StoreConfig + Build()` | `core/harness.go`, `core/agent.go` | `cd core && go build ./...` |
| W4.3 | `feat(hooks): fine-grained skip logic per hook based on dependency readiness` | `core/capabilities/hooks/{file_memory,kb_recall,memory_persist}.go` | `cd core && go build ./capabilities/hooks/...` |
| W4.4 | `test(harness): end-to-end integration for 4 config combinations (none/memory/KB/both)` | `core/harness_integration_test.go` (or extend `core/harness_test.go`) | `cd core && go test -run Integration ./...` |
| W5.1 | `feat(api): knowledge base management REST endpoints (KB CRUD + doc CRUD + multipart upload)` | `server/internal/api/knowledge.go`, `*_test.go` | `cd server && go test ./internal/api/...` |
| W5.2 | `feat(api): retrieval test + session memory endpoints` | `server/internal/api/knowledge.go`, `server/internal/api/memory.go`, `*_test.go` | `cd server && go test ./internal/api/...` |
| W5.3 | `feat(api): mount /api/kb/* routes + handler dependency injection` | `server/internal/api/handlers.go` | `cd server && go build ./internal/api/...` |
| W5.4 | `test(api): comprehensive httptest coverage for knowledge + memory endpoints` | `server/internal/api/knowledge_test.go`, `server/internal/api/memory_test.go` | `cd server && go test ./internal/api/...` |
| W5.5 | `feat(chat-core): add KnowledgeBase/Document/Chunk/Memory/SearchResult types` | `packages/chat-core/src/types.ts` | `pnpm --filter @copcon/chat-core build` |
| W5.6 | `feat(chat-core): extend AgentClient with 10 KB + memory API methods` | `packages/chat-core/src/agent-client.ts` | `pnpm --filter @copcon/chat-core build` |
| W5.7 | `test(chat-core): vitest unit tests for all new AgentClient methods with mocked fetch` | `packages/chat-core/src/*.test.ts` | `pnpm --filter @copcon/chat-core test` |
| W6.1 | `refactor(demo): top-level Tabs (Chat/KB/Memory) + extract ChatPage` | `packages/demo/src/App.tsx`, `packages/demo/src/pages/ChatPage.tsx`, `packages/demo/src/App.css` | `pnpm --filter @copcon/demo build` |
| W6.2 | `feat(demo): KBList page with Card grid, create modal, delete confirm` | `packages/demo/src/pages/KnowledgePage.tsx`, `packages/demo/src/components/kb/KBList.tsx` | `pnpm --filter @copcon/demo build` |
| W6.3 | `feat(demo): KBDetail page with Table + status badges + stats` | `packages/demo/src/components/kb/KBDetail.tsx` | `pnpm --filter @copcon/demo build` |
| W6.4 | `feat(demo): KBUpload with DragDrop + multi-file + progress + error feedback` | `packages/demo/src/components/kb/KBUpload.tsx` | `pnpm --filter @copcon/demo build` |
| W6.5 | `feat(demo): ChunkViewer Drawer with search + highlight` | `packages/demo/src/components/kb/ChunkViewer.tsx` | `pnpm --filter @copcon/demo build` |
| W6.6 | `feat(demo): KBRetrievalTest query + results with tiered score colors` | `packages/demo/src/components/kb/KBRetrievalTest.tsx` | `pnpm --filter @copcon/demo build` |
| W6.7 | `feat(demo): MemoryPage + MemoryPanel with MD render + inline sidebar version` | `packages/demo/src/pages/MemoryPage.tsx`, `packages/demo/src/components/memory/MemoryPanel.tsx` | `pnpm --filter @copcon/demo build` |
| W6.8 | `feat(demo): unified UI polish — Skeleton/Empty/ErrorBoundary/notifications with token` | `packages/demo/src/components/kb/*.tsx`, `packages/demo/src/components/memory/*.tsx` | `pnpm --filter @copcon/demo build` |
| W6.9 | `feat(demo): accessibility — ARIA labels + keyboard navigation + screen-reader friendly` | all new demo components | `pnpm --filter @copcon/demo build` + `playwright axe-core scan` |
| W7.1 | `feat(eval): Go-native retrieval metrics — Recall@K, Precision@K, MRR, nDCG, HitRate` | `core/eval/retrieval.go`, `*_test.go` | `cd core && go test ./eval/...` |
| W7.2 | `eval: bootstrap golden_set.jsonl with 50 test cases (60/20/10/10 distribution)` | `eval/testdata/golden_set.jsonl` | (manual curation, no automated check) |
| W7.3 | `ci: eval workflow with Recall@5>=0.80 + MRR>=0.75 quality gates` | `.github/workflows/eval.yml` | (trigger workflow manually, verify gate) |

---

## Success Criteria

### Verification Commands

```bash
# Go 后端编译 + 测试
cd core && go build ./...                                          # Expected: 0 errors
cd core && go test ./...                                           # Expected: ALL PASS
cd core && go test -tags=integration ./...                         # Expected: ALL PASS (sqlite-vec in-memory)
cd core && go test ./eval/...                                      # Expected: Recall@5 >= 0.80, MRR >= 0.75

cd server && go build ./...                                        # Expected: 0 errors
cd server && go test ./internal/...                                # Expected: ALL PASS

# TypeScript 前端编译 + 测试
pnpm --filter @copcon/chat-core build                              # Expected: 0 errors
pnpm --filter @copcon/chat-core test                               # Expected: ALL PASS
pnpm --filter @copcon/chat-react build                             # Expected: 0 errors
pnpm --filter @copcon/headless-hooks build                         # Expected: 0 errors
pnpm --filter @copcon/demo build                                   # Expected: 0 errors

# Demo 应用启动 + 集成测试
pnpm --filter @copcon/demo dev
# Expected: 访问 http://localhost:5173/ 看到三 Tab (聊天/知识库/记忆)
# Expected: 创建 KB → 上传 MD → 在 Chat 中查询 → agent 基于 KB 内容回答
# Expected: Chat 中的 memory_tool 调用 → 下次会话 INDEX.md 中出现新记忆
```

### Final Checklist

- [ ] 所有 7 个 "Must Have" 项全部在代码中落实
- [ ] 所有 11 个 "Must NOT Have" 项均无对应代码模式(grep 验证)
- [ ] 43 个任务对应的单元测试 + QA Scenarios 全部 PASS,`.sisyphus/evidence/` 齐备
- [ ] `StoreProvider.Memory()` 返回 nil 不会导致编译错误或运行时 panic(与 `Todos()` 行为一致)
- [ ] `MemoryBundleNames()` 和 `KnowledgeBaseBundleNames()` 是两个互不重叠的 capability 集合
- [ ] `KnowledgeStore` 接口 + provider 注册机制存在,本期 sqlite-vec 实现,未来可插 Qdrant/pgvector 无需修改 core/
- [ ] INDEX.md 200 行 / 25KB 硬限被 filememory 严格截断
- [ ] MemoryPersistHook 完全不调用 LLM(代码审查确认)
- [ ] Demo UI 可访问性: 键盘 Tab 可遍历所有输入框和按钮;屏幕阅读器能读出页面结构
- [ ] Commit 历史:每个任务至少 1 个 commit,message 符合 `type(scope): desc`;失败任务的 commit 不应出现在 main
- [ ] 模块边界: `core/` 没有任何代码 import `server/`
- [ ] F1-F4 全部 APPROVE + 用户明确 "okay"
