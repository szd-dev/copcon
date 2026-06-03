# Memory 系统技术方案

> 版本: v1.0 | 日期: 2026-06-01

## 1. 概述

CopCon 的记忆系统为 Agent 提供跨会话的持久化知识管理能力。系统采用**文件驱动**架构——所有记忆以 Markdown 文件形式存储在文件系统中，通过两层索引（INDEX.md + FACTS.md）提供导航，通过 LLM 驱动的语义召回和自动事实提取实现主动记忆管理。

### 设计目标

| 目标 | 说明 |
|------|------|
| 人类可读 | 所有记忆为纯 Markdown 文件，可用编辑器直接查看和修改 |
| 可追溯 | 每条记忆关联到源会话和消息，支持正向/反向追溯 |
| 自动管理 | 后台自动提取事实、语义召回相关记忆、定期压缩摘要 |
| 无外部依赖 | 不依赖向量数据库、不依赖额外的 LLM client |

---

## 2. 核心概念

### 2.1 三个实体

```
┌─ Session (会话) ──────────────────────────────────────┐
│  用户与 Agent 的一次完整对话                             │
│  例如: session_id = "abc-123"                          │
│    msg-001: "我喜欢用 Go 写后端"                        │
│    msg-002: "Python 也可以，但 Go 更顺手"                │
│    msg-003: "项目用 PostgreSQL"                        │
└───────────────────────────────────────────────────────┘
         │                              │
         │ 自动提取 / 手动存储              │
         ▼                              ▼
┌─ Memory (记忆) ─────────────┐  ┌─ Memory (记忆) ─────────────┐
│ session_id: "abc-123"       │  │ session_id: "abc-123"       │
│ message_ids: [msg-001,002]  │  │ message_ids: [msg-003]      │
│ type: user                  │  │ type: project               │
│ content: "用户偏好 Go 语言"   │  │ content: "项目用 PG 15"     │
└─────────────────────────────┘  └─────────────────────────────┘
         │                              │
         │ 摘要触发                      │
         ▼                              ▼
┌─ Summary (摘要) ───────────────────────────────────────────┐
│ 对多条记忆的 LLM 驱动的压缩                                  │
│ 每个结论可追溯到源记忆 → 源会话 + 消息                        │
└─────────────────────────────────────────────────────────────┘
```

- **Fact（事实）** = 会话中的一条或多条消息，是记忆的"证据来源"
- **Memory（记忆）** = 从事实中提取的洞察，存储为 MD 文件
- **Summary（摘要）** = 多条记忆的压缩版本，放入热层供始终访问

### 2.2 两个正交维度

| 维度 | 含义 | 取值 |
|------|------|------|
| **type** (来源分类) | 这条记忆是关于什么类型的知识 | user / feedback / project / reference |
| **tier** (存储分层) | 这条记忆在哪个目录，决定注入时机 | system (热) / knowledge (温) / archive (冷) |

两者正交。一条 `type=feedback` 的记忆可以放在 `knowledge/`（温），被摘要后移到 `system/`（热）。

### 2.3 四类型封闭分类法

| type | 语义 | 示例 |
|------|------|------|
| **user** | 用户身份、偏好、知识背景 | "用户是后端工程师，偏好 Go 语言" |
| **feedback** | 对 Agent 行为的纠正或肯定 | "不要在响应末尾做总结" |
| **project** | 项目进展、技术决策、截止日期 | "2026-03-05 合并冻结" |
| **reference** | 外部系统的位置信息 | "Bug 跟踪在 Linear INGEST 项目" |

封闭分类（而非自由标签）迫使 Agent 做出明确语义分类，避免标签膨胀导致召回模糊。

**排除原则**：以下内容**不应**存储为记忆——
- 代码模式、架构、文件路径（可从代码直接获取）
- Git 历史（`git log` 是权威来源）
- 临时任务细节
- 已在 CLAUDE.md 或 system/ 中记录的内容

---

## 3. 目录结构

```
~/.copcon/memory/<agent_id>/
│
├── system/                          ← 热层：始终注入 SystemPrompt
│   ├── persona.md                   ← Agent 身份定义
│   ├── project.md                   ← 项目事实
│   ├── INDEX.md                     ← 记忆索引（自动生成）
│   ├── FACTS.md                     ← 事实索引（自动生成）
│   └── summary-2026-06-01.md        ← 摘要文件
│
├── knowledge/                       ← 温层：索引可见，按需召回
│   ├── language-preference.md
│   └── db-choice.md
│
└── archive/                         ← 冷层：索引可见，极少访问
    └── old-decision.md
```

### 3.1 冷热分层策略

| 层级 | 目录 | 注入时机 | 加载方式 |
|------|------|----------|----------|
| 🔥 热 | `system/` | 每次 OnSystemPrompt 全部读取 | 直接注入 SystemPrompt |
| 🌤 温 | `knowledge/` | INDEX.md 中可见，LLM 按需选择 | MemoryRecallHook 语义召回 |
| ❄️ 冷 | `archive/` | INDEX.md 中可见 | 同温层，但访问频率最低 |

默认写入 `knowledge/`（温层）。Agent 可通过 `memory_store` 的 `category` 参数指定。

---

## 4. 文件格式

### 4.1 记忆文件

每个 `.md` 文件 = YAML frontmatter + Markdown 正文：

```yaml
---
name: "语言偏好"
description: "用户偏好使用 Go 语言编写后端服务"
type: "user"
importance: 0.9
created_at: "2026-06-01T10:00:00Z"
updated_at: "2026-06-01T10:30:00Z"
session_id: "abc-123"
message_ids: ["msg-001", "msg-002"]
tags: ["language", "preference"]
---
用户偏好使用 Go 语言编写后端服务，认为 Go 比 Python 更顺手。
除非明确要求，否则不要建议使用其他语言。
```

**字段说明**：

| 字段 | 必填 | 说明 |
|------|------|------|
| name | 是 | 记忆名称，用于 INDEX.md 和召回 |
| description | 是 | 一行摘要，LLM 语义召回的主要依据 |
| type | 是 | user / feedback / project / reference |
| importance | 否 | 重要性 0-1，默认 0.5 |
| created_at | 是 | 创建时间 |
| updated_at | 是 | 最后更新时间 |
| session_id | 否 | 来源会话 ID |
| message_ids | 否 | 来源消息 ID 列表 |
| tags | 否 | 自由标签 |

### 4.2 INDEX.md

自动生成的记忆索引，扫描 `knowledge/` 和 `archive/` 目录：

```markdown
# Memory Index

- **language-preference** (`knowledge/language-preference.md`) — user [2026-06-01]
  > 用户偏好使用 Go 语言编写后端服务
- **db-choice** (`knowledge/db-choice.md`) — project [2026-06-01]
  > 项目使用 PostgreSQL 15
```

**生成规则**：
- 每次写/删记忆文件后全量重建
- 从 frontmatter 提取 `name`、`type`、`updated_at`、`description`
- 按路径排序
- 硬性限制：200 行 或 25KB，超出截断并追加警告

### 4.3 FACTS.md

自动生成的事实索引，扫描 `knowledge/` 和 `archive/` 目录：

```markdown
# Fact Index

## session: abc-123
| 消息 | 记忆 |
|------|------|
| msg-001 | [语言偏好](knowledge/language-preference.md) |
| msg-002 | [语言偏好](knowledge/language-preference.md) |
| msg-003 | [数据库选择](knowledge/db-choice.md) |

## session: def-456
| 消息 | 记忆 |
|------|------|
| msg-010 | [部署决策](knowledge/deploy-decision.md) |
```

**生成规则**：
- 从 frontmatter 提取 `session_id` 和 `message_ids`
- 按 session 分组，按 message 排序
- 提供**反向索引**：给定 session+message → 找到关联的记忆

### 4.4 摘要文件

LLM 生成的记忆压缩版本，存放在 `system/`：

```yaml
---
name: "Memory Summary"
description: "46 条记忆的结构化摘要"
type: "summary"
importance: 0.9
created_at: "2026-06-01T10:30:00Z"
updated_at: "2026-06-01T10:30:00Z"
source_files:
  - "knowledge/language-preference.md"
  - "knowledge/db-choice.md"
  - "knowledge/deploy-decision.md"
file_count: 46
token_used: 1842
---
# Memory Summary (2026-06-01)

## 技术偏好
- 用户偏好 Go 语言（来源: language-preference.md → session abc-123）
- 除非明确要求，不要建议其他语言

## 项目决策
- 使用 PostgreSQL 15（来源: db-choice.md → session abc-123）
- 部署使用 Docker Compose（来源: deploy-decision.md → session def-456）
```

---

## 5. Hook 体系

### 5.1 执行流程

```
用户发送消息
  │
  ▼ OnSystemPrompt (priority=90)
  MemorySummaryHook
  ├── 检查摘要触发条件（文件数 / 年龄 / 冷却时间）
  ├── 收集 knowledge/ + archive/ 文件
  ├── LLM 生成结构化摘要
  └── 写入 system/summary-{date}.md
  │
  ▼ OnSystemPrompt (priority=80)
  FileMemoryHook
  ├── 读取 system/*.md（含摘要、INDEX、FACTS）
  ├── 读取 INDEX.md
  └── 注入 SystemPrompt "## Agent Memory" 区块
  │
  ▼ AfterContextBuild (priority=70)
  MemoryRecallHook                          ★ 新增
  ├── 扫描 INDEX.md → 获取所有记忆条目
  ├── 构建提示 → LLM 选择 ≤5 个相关记忆
  └── 注入 *ctx.Messages 前缀
  │
  ▼ AfterContextBuild (priority=60)         已有
  KBRecallHook
  ├── 向量检索知识库
  └── 注入 *ctx.Messages 前缀
  │
  ▼ LLM 调用
  │
  ▼ OnMessagePersist
  FactExtractionHook                        ★ 新增
  ├── 互斥检查：Agent 已手动 store？→ 跳过
  ├── 异步 goroutine
  ├── LLM 从最近对话萃取事实
  └── 写入 knowledge/*.md + 更新 INDEX.md + FACTS.md
```

### 5.2 MemoryRecallHook — 语义召回

**触发**: AfterContextBuild, priority=70

**流程**:
1. 扫描 INDEX.md，获取所有记忆条目（name + type + description + path）
2. 构建提示给 LLM：
   ```
   你是记忆选择器。以下是可用记忆列表。用户的问题是：{last_user_message}
   选择与此问题相关的记忆（最多 5 个），不确定时跳过。
   已经活跃使用的工具文档跳过。

   [user] language-preference: 用户偏好Go语言，喜欢简洁回答
   [project] db-choice: 项目使用PostgreSQL 15
   [feedback] terse-reply: 不要在响应末尾做总结
   ...
   ```
3. LLM 返回选中的记忆路径列表
4. 读取对应文件内容，以系统消息形式注入 `*ctx.Messages` 前缀
5. 已注入的记忆本轮不再重复选择

**关键设计**：
- 复用 Harness 已有的 `llm.LLMProvider`，不引入额外 LLM client
- 不在 Agent 可见的 SystemPrompt 中注入（避免与 FileMemoryHook 重复），而是作为隐藏系统消息注入消息列表
- 每轮最多 5 个记忆，避免上下文膨胀

### 5.3 FactExtractionHook — 自动事实提取

**触发**: OnMessagePersist, async goroutine

**流程**:
1. **互斥检查**：Agent 本轮是否已手动调用 `memory_store`？是 → 跳过
2. 取最近几轮对话（当前 assistant 回复 + 前序用户消息）
3. 调用 LLM，提示：
   ```
   从以下对话中提取值得记住的事实。
   只提取无法从代码/项目中重新推导的信息。
   不为已在上下文或已有记忆中显而易见的事实创建条目。
   对每条事实标注类型：user / feedback / project / reference。
   为每条事实生成一个简洁的 description（一行，用于后续召回）。

   已有记忆：
   - language-preference: 用户偏好Go语言
   - db-choice: 项目使用PostgreSQL 15

   对话：
   User: 我希望回答简洁一点，不要总在末尾做总结
   Assistant: 好的，我会注意回答风格。

   输出 JSON:
   [{"content": "用户偏好简洁回答，不要在响应末尾做总结",
     "type": "feedback",
     "name": "terse-reply",
     "description": "用户偏好简洁回答风格",
     "importance": 0.8}]
   ```
4. 解析 LLM 返回的 JSON，对每条事实：
   - 写入 `knowledge/{name}.md`（含 session_id、message_ids）
   - 更新 INDEX.md
   - 更新 FACTS.md
5. 失败静默（日志警告，不阻塞对话）

**关键设计**：
- 异步 goroutine，不阻塞对话流水线
- 提示中包含已有记忆 → 避免重复提取
- 提示中包含提取原则（排除原则）→ 避免噪声记忆
- Agent 手动 store 优先 → Agent 的判断比自动提取更准确

### 5.4 MemorySummaryHook — 摘要生成

**触发**: OnSystemPrompt, priority=90

**触发条件**（任一满足）:
- `knowledge/` 文件数 > max_memories（默认 50）
- 最旧文件距今 > max_age_hours（默认 24h）
- 距上次摘要 > cooldown_minutes（默认 60min）

**流程**:
1. 检查触发条件
2. 收集 `knowledge/` + `archive/` 所有文件内容
3. 调用 LLM 生成结构化摘要，提示中要求保留事实来源
4. 写入 `system/summary-{date}.md`
5. 可选：将已摘要文件移入 `archive/`

**失败策略**: 静默跳过，不阻塞对话。

---

## 6. 配置模型

```yaml
# server/config.yaml
agents:
  - id: "code-assistant"
    name: "Code Assistant"
    model: "z-ai/glm-5"                # 对话模型
    memory:
      enabled: true
      base_path: "~/.copcon/memory"
      max_index_lines: 200
      max_index_bytes: 25600
      summarization:
        enabled: true
        model: "z-ai/glm-5-flash"       # 摘要专用模型（独立配置）
        api_key: ""                      # 继承 openai.api_key
        base_url: ""                     # 继承 openai.base_url
        max_tokens: 2000
        temperature: 0.3
        trigger:
          max_memories: 50
          max_age_hours: 24
          cooldown_minutes: 60
```

---

## 7. Agent 工具

### 7.1 memory_store

```json
{
  "name": "memory_store",
  "description": "存储一条记忆到持久化文件系统",
  "parameters": {
    "content":     { "type": "string",  "required": true,  "description": "记忆内容" },
    "type":        { "type": "string",  "required": false, "description": "user/feedback/project/reference" },
    "name":        { "type": "string",  "required": false, "description": "描述性名称（自动生成）" },
    "description": { "type": "string",  "required": false, "description": "一行摘要（用于召回）" },
    "importance":  { "type": "number",  "required": false, "description": "重要性 0-1" },
    "session_id":  { "type": "string",  "required": false, "description": "来源会话（默认当前会话）" },
    "message_ids": { "type": "array",   "required": false, "description": "来源消息ID列表" }
  }
}
```

**执行**：写入 MD 文件 → 更新 INDEX.md → 更新 FACTS.md。

### 7.2 memory_recall

```json
{
  "name": "memory_recall",
  "description": "语义搜索记忆",
  "parameters": {
    "query":    { "type": "string", "required": true,  "description": "搜索内容" },
    "type":     { "type": "string", "required": false, "description": "按类型过滤" },
    "limit":    { "type": "number", "required": false, "description": "最大结果数（默认5）" }
  }
}
```

**执行**：扫描 INDEX.md → LLM 选择相关文件 → 返回内容 + 来源信息。内部实现从关键词匹配升级为 LLM 语义选择。

### 7.3 memory_forget

```json
{
  "name": "memory_forget",
  "description": "删除一条记忆",
  "parameters": {
    "name": { "type": "string", "description": "记忆名称（在 knowledge/ 和 archive/ 中查找）" },
    "path": { "type": "string", "description": "精确路径" }
  }
}
```

**执行**：删除 MD 文件 → 更新 INDEX.md → 更新 FACTS.md。

---

## 8. 与业界对比

| 维度 | Claude Code | OpenClaw | CopCon |
|------|------------|----------|--------|
| 存储介质 | 纯 MD 文件 | MD + SQLite | 纯 MD 文件 |
| 索引 | MEMORY.md (200行/25KB) | FTS5 + sqlite-vec | INDEX.md (200行/25KB) + FACTS.md |
| 语义召回 | Sonnet LLM 选择 ≤5 个文件 | BM25 + 向量混合搜索 | MemoryRecallHook: LLM 选择 ≤5 个文件 |
| 自动提取 | extractMemories 后台 Agent | Dreaming 评分晋升 | FactExtractionHook 后台萃取 |
| 后台巩固 | AutoDream (24h+5会话) | Dreaming (三阶段) | MemorySummaryHook (阈值触发) |
| 事实溯源 | 文件路径+行号 | 文件路径+行号 | session_id + message_ids |
| 分类体系 | 4 type (user/feedback/project/reference) | — | 4 type + 3 tier |

---

### 9.1 能力注册模型

Memory 系统的所有能力通过 **单一注册 key** `modules.memory_file` 对外暴露——即已有的 `MemoryModule`。新增的 hooks 和 tools 直接在 `MemoryModule.NewHooks()` 和 `MemoryModule.NewTools()` 中追加，**不新增 capability 常量**，**不修改 `bundle.go`**。

```
RegisterCapabilities(reg, store, summarizer)
    │
    └── r.Register(&MemoryModule{
            store:      store,
            summarizer: summarizer,
        })
            │
            ├── NewHooks() → [
            │       FileMemoryHook,        // OnSystemPrompt(80): 注入 system/ + INDEX.md
            │       MemorySummaryHook,     // OnSystemPrompt(90): 摘要生成
            │       MemoryRecallHook,      // AfterContextBuild(70): LLM 语义召回
            │       FactExtractionHook,    // OnMessagePersist: 后台事实萃取
            │   ]
            │
            └── NewTools() → [
                    MemoryStoreTool,       // 已有，扩展字段
                    MemoryRecallTool,      // 已有，升级 LLM 选择
                    MemoryForgetTool,      // 已有，更新 FACTS.md
                ]
```

`MemoryBundleNames()` 保持不变：`[HookMemory, CapMemoryFile]`。`CapMemoryFile` 即 `modules.memory_file`，`Harness.Build()` 解析该 module 后自动注册其产出的所有 hooks 和 tools。

### 9.2 文件清单

### 9.2 文件清单

### 已有文件（需修改）

| 文件 | 修改内容 |
|------|----------|
| `plugins/memory-file/types/memory.go` | Memory 新增 SessionID, MessageIDs, Description；新增 SummaryResult |
| `plugins/memory-file/frontmatter.go` | Frontmatter 新增 SessionID, MessageIDs, Description, SourceFiles |
| `plugins/memory-file/memory_store_tool.go` | InputSchema 新增 type, description, session_id, message_ids |
| `plugins/memory-file/memory_recall_tool.go` | 内部从关键词扫描升级为 LLM 选择 |
| `plugins/memory-file/memory_forget_tool.go` | 执行后更新 FACTS.md |
| `plugins/memory-file/filememory.go` | Store/WriteFile 处理新字段 |
| `plugins/memory-file/index.go` | BuildIndex 格式扩展（含 description） |
| `plugins/memory-file/register.go` | RegisterCapabilities(reg, store, summarizer) |
| `plugins/memory-file/capabilities_closure.go` | MemoryModule.NewHooks() 追加 3 个 hook；NewTools() 不变 |
| `server/internal/config/config.go` | 新增 MemorySummarizationConfig |
| `server/cmd/server/main.go` | 创建摘要 LLM client，传入 RegisterCapabilities |

**不修改的文件**：
- `core/capabilities/constants.go` — 不新增常量
- `core/capabilities/bundle.go` — `MemoryBundleNames()` 保持不变

### 新增文件

| 文件 | 说明 |
|------|------|
| `plugins/memory-file/facts.go` | FACTS.md 管理：BuildFacts, ReadFacts, AddToFacts, RemoveFromFacts |
| `plugins/memory-file/recall_hook.go` | MemoryRecallHook: AfterContextBuild(70), LLM 语义召回 |
| `plugins/memory-file/extraction_hook.go` | FactExtractionHook: OnMessagePersist, async 事实萃取 |
| `plugins/memory-file/summarizer.go` | FileSummarizer: 摘要触发检查、LLM 调用、prompt 构建 |
| `plugins/memory-file/summary_hook.go` | MemorySummaryHook: OnSystemPrompt(90), 摘要生成 |
| `plugins/memory-file/facts_test.go` | 测试 |
| `plugins/memory-file/recall_hook_test.go` | 测试 |
| `plugins/memory-file/extraction_hook_test.go` | 测试 |
| `plugins/memory-file/summarizer_test.go` | 测试 |
| `plugins/memory-file/summary_hook_test.go` | 测试 |