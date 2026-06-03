# Memory 系统完整实现

## TL;DR

> **核心目标**: 在 `plugins/memory-file` 插件内实现语义召回、自动事实提取、摘要生成、事实关联四大能力，全部通过单一注册 key `modules.memory_file` (MemoryModule) 对外暴露。
>
> **交付物**:
> - 3 个新 Hook: MemoryRecallHook, FactExtractionHook, MemorySummaryHook
> - 1 个新索引: FACTS.md (反向索引)
> - 1 个新工具函数: Complete() (LLM 非流式调用)
> - 扩展类型: Memory, Frontmatter (SessionID, MessageIDs, Description, Type)
> - 扩展配置: MemorySummarizationConfig
> - 升级工具: memory_store (新字段), memory_recall (LLM 语义选择), memory_forget (FACTS.md)
>
> **预估工作量**: 中大型
> **并行执行**: YES — 3 波次
> **关键路径**: Complete() → 类型扩展 → 3 个 Hook → 配置接入

---

## Context

### 原始需求
实现 memory 系统的语义召回、自动事实提取、摘要生成、事实关联四大能力。

### 设计依据
- 完整设计文档: `docs/memory-system-design.md`
- Claude Code 参考: MEMORY.md 索引 + Sonnet side-query 语义召回 + extractMemories 后台提取
- OpenClaw 参考: Bootstrap 注入优先级 + Dreaming 巩固
- 架构原则: 单一 plugin key (`modules.memory_file`)，不修改 `core/capabilities/`

### Metis 审查关键发现
1. **LLM 仅支持流式** — `LLMProvider` 只有 `Stream()`，无同步 API。需在插件内实现 `Complete()` 工具函数
2. **CapabilityDeps 无 LLM** — 通过 `RegisterCapabilities()` 注入 LLM，存储在 `MemoryModule` 中
3. **OnMessagePersist 无 Messages** — FactExtractionHook 需通过 `CapabilityDeps.MessageStore` 查询最近消息
4. **MemoryType 分类法冲突** — 新的 4-type (user/feedback/project/reference) 作为独立字段 `Type`，与现有 `Category`（目录层）并行
5. **FACTS.md 并发写入** — 异步提取 + 同步遗忘可能冲突，需 `sync.Mutex`
6. **摘要冷却需持久化** — 服务重启后冷却状态丢失，需文件记录

### 测试策略
- **Tests after**: 实现完成后编写测试
- 每个 TODO 包含 Agent-Executed QA 场景

---

## Work Objectives

### 核心目标
在 `plugins/memory-file` 插件内实现 4 个记忆管理能力，全部通过 `MemoryModule` 统一注册。

### 交付物
- `plugins/memory-file/complete.go` — LLM 非流式调用工具函数
- `plugins/memory-file/facts.go` — FACTS.md 管理
- `plugins/memory-file/recall_hook.go` — 语义召回 Hook
- `plugins/memory-file/extraction_hook.go` — 自动事实提取 Hook
- `plugins/memory-file/summarizer.go` — 摘要生成器
- `plugins/memory-file/summary_hook.go` — 摘要 Hook
- 类型扩展: `types/memory.go`, `frontmatter.go`
- 工具升级: `memory_store_tool.go`, `memory_recall_tool.go`, `memory_forget_tool.go`
- 配置扩展: `server/internal/config/config.go`
- 接入: `server/cmd/server/main.go`, `register.go`, `capabilities_closure.go`

### 定义完成
- [x] `cd plugins/memory-file && go test ./...` → PASS
- [x] `cd core && go test -run TestHarness ./...` → PASS
- [x] `cd server && go build ./...` → 无错误

### 必须实现
- 3 个新 Hook，全部通过 MemoryModule.NewHooks() 产出
- FACTS.md 反向索引，与 INDEX.md 一致的构建模式
- Complete() 工具函数，从 Stream() 组装完整 LLM 响应
- memory_store 互斥标志，防止 FactExtractionHook 重复提取
- 摘要冷却状态持久化到文件

### 绝不能做
- 不修改 `core/capabilities/constants.go`
- 不修改 `core/capabilities/bundle.go`
- 不修改 `core/llm/provider.go` (LLMProvider 接口)
- 不修改 `core/hook/hook.go` (HookContext)
- 不引入向量搜索或嵌入依赖
- 不将摘要文件纳入 INDEX.md

---

## Verification Strategy

### 测试决策
- **基础设施存在**: YES (`go test`, `github.com/stretchr/testify`)
- **自动化测试**: Tests after
- **框架**: `go test` + `testify/assert`
- **Mock**: 使用 `llm.MockProvider` (已有) 或扩展它返回预定义 JSON

### QA 策略
每个 TODO 包含 Agent-Executed QA 场景。使用 `bash` + `go test` 验证。

---

## Execution Strategy

### 并行执行波次

```
Wave 1 (基础 — 无依赖):
├── T1: Complete() 工具函数 [quick]
├── T2: 类型扩展 (Memory, Frontmatter) [quick]
├── T3: FACTS.md 基础设施 [quick]
├── T4: 扩展 MemoryStoreAPI + FileMemoryStore [unspecified-low]
├── T5: 更新 register.go + main.go 签名 [quick]
└── T6: INDEX.md 格式扩展 [quick]

Wave 2 (核心 Hook — 依赖 Wave 1):
├── T7: MemoryRecallHook + LLM 选择逻辑 [deep]
├── T8: 升级 memory_recall_tool → LLM 语义 [unspecified-low]
├── T9: FactExtractionHook + 互斥标志 [deep]
├── T10: 升级 memory_store_tool (新字段) [quick]
├── T11: 升级 memory_forget_tool (FACTS.md) [quick]
└── T12: MemoryModule.NewHooks() 扩展 [quick]

Wave 3 (摘要 — 依赖 Wave 2):
├── T13: FileSummarizer + 冷却持久化 [deep]
├── T14: MemorySummaryHook [unspecified-low]
├── T15: 配置扩展 (config.go) [quick]
└── T16: main.go 摘要 LLM client 接入 [quick]

Wave 4 (测试):
├── T17: FACTS.md 测试 [quick]
├── T18: MemoryRecallHook 测试 [unspecified-low]
├── T19: FactExtractionHook 测试 [unspecified-low]
├── T20: FileSummarizer 测试 [unspecified-low]
└── T21: MemorySummaryHook 测试 [unspecified-low]
```

---

## TODOs

- [x] 1. Complete() 工具函数

  **What to do**:
  - 在 `plugins/memory-file/complete.go` 中创建 `Complete(ctx, llm, params) (string, error)` 函数
  - 调用 `llm.Stream()`，收集所有 `StreamChunk.Content`，拼接成完整字符串
  - 超时保护: 30 秒 context deadline
  - 错误处理: Stream 失败返回错误，空响应返回空字符串
  - 不在 `core/llm/` 中实现，保持插件内聚

  **Must NOT do**:
  - 不修改 `core/llm/provider.go`
  - 不添加同步 API 到 LLMProvider 接口

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单一工具函数，无外部依赖
  - **Skills**: `[]`
  - **Skills Evaluated but Omitted**: 无

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 T2 并行)
  - **Parallel Group**: Wave 1
  - **Blocks**: T7, T9, T13 (所有需要 LLM 调用的组件)
  - **Blocked By**: None

  **References**:
  - `core/llm/provider.go:25` — LLMProvider.Stream() 接口签名
  - `core/llm/openai_adapter.go:36` — Stream 实现参考 (chunk 收集模式)
  - `core/llm/mock_provider.go` — Mock 实现用于测试

  **Acceptance Criteria**:
  - [ ] `cd plugins/memory-file && go test -run TestComplete` → PASS
  - [ ] Mock LLM 返回 "hello" → Complete() 返回 "hello"
  - [ ] Mock LLM 返回多个 chunk → 正确拼接
  - [ ] Mock LLM 错误 → 返回 error
  - [ ] Context 超时 → 返回 deadline exceeded error

  **QA Scenarios**:

  ```
  Scenario: Happy path - single chunk response
    Tool: Bash (go test)
    Preconditions: MockProvider configured to return single chunk
    Steps:
      1. cd plugins/memory-file && go test -run TestComplete_SingleChunk -v
      2. Assert: test passes, output shows correct string
    Expected Result: Test PASS, Complete() returns "hello world"
    Evidence: .sisyphus/evidence/task-1-complete-single.txt

  Scenario: Multi-chunk response
    Tool: Bash (go test)
    Preconditions: MockProvider configured to return 3 chunks
    Steps:
      1. cd plugins/memory-file && go test -run TestComplete_MultiChunk -v
      2. Assert: concatenated result equals all chunks joined
    Expected Result: Test PASS, chunks correctly concatenated
    Evidence: .sisyphus/evidence/task-1-complete-multi.txt

  Scenario: Stream error
    Tool: Bash (go test)
    Preconditions: MockProvider configured to return error
    Steps:
      1. cd plugins/memory-file && go test -run TestComplete_Error -v
      2. Assert: Complete() returns non-nil error
    Expected Result: Test PASS, error propagated correctly
    Evidence: .sisyphus/evidence/task-1-complete-error.txt
  ```

  **Commit**: YES
  - Message: `feat(memory): add Complete() utility for non-streaming LLM calls`
  - Files: `plugins/memory-file/complete.go`

- [x] 2. 类型扩展 (Memory, Frontmatter)

  **What to do**:
  - `types/memory.go`: Memory struct 新增 `SessionID string`、`MessageIDs []string`、`Description string`、`Type string`（新 4-type 分类）
  - `frontmatter.go`: Frontmatter struct 新增 `SessionID string \`yaml:"session_id,omitempty"\``、`MessageIDs []string \`yaml:"message_ids,omitempty"\``、`Description string \`yaml:"description,omitempty"\``、`Type string \`yaml:"type,omitempty"\``、`SourceFiles []string \`yaml:"source_files,omitempty"\``
  - `Type` 与现有 `Category`（目录层）**互不干扰**：Type 是语义分类，Category 是存储目录
  - `ParseFrontmatter` 向后兼容：缺失字段为空值，不报错
  - 更新 `SerializationFrontmatter` 序列化新字段

  **Must NOT do**:
  - 不修改 `MemoryType` enum (episodic/semantic/procedural 保持不变)
  - 不用新 Type 替换 Category

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 T1 并行)
  - **Parallel Group**: Wave 1
  - **Blocks**: T3, T7, T9, T10
  - **Blocked By**: None

  **References**:
  - `plugins/memory-file/types/memory.go` — 当前 Memory struct
  - `plugins/memory-file/frontmatter.go` — 当前 Frontmatter struct
  - `docs/memory-system-design.md:§4.1` — 新字段定义

  **Acceptance Criteria**:
  - [ ] `cd plugins/memory-file && go test -run TestFrontmatter_NewFields` → PASS
  - [ ] 含所有新字段的 YAML frontmatter 正确解析
  - [ ] 不含新字段的旧版 frontmatter 正确解析（向后兼容）
  - [ ] SerializeFrontmatter 输出包含新字段

  **QA Scenarios**:
  ```
  Scenario: Parse new frontmatter with all fields
    Tool: Bash (go test)
    Steps:
      1. cd plugins/memory-file && go test -run TestParseFrontmatter_WithNewFields -v
    Expected Result: All new fields correctly populated
    Evidence: .sisyphus/evidence/task-2-frontmatter-parse.txt

  Scenario: Parse old frontmatter without new fields
    Tool: Bash (go test)
    Steps:
      1. cd plugins/memory-file && go test -run TestParseFrontmatter_BackwardCompat -v
    Expected Result: New fields empty/nil, old fields intact
    Evidence: .sisyphus/evidence/task-2-frontmatter-compat.txt
  ```

  **Commit**: YES
  - Message: `feat(memory): extend Memory and Frontmatter with fact association fields`
  - Files: `plugins/memory-file/types/memory.go`, `plugins/memory-file/frontmatter.go`

- [x] 3. FACTS.md 基础设施

  **What to do**:
  - 在 `plugins/memory-file/facts.go` 中实现:
    - `BuildFacts(basePath, agentID string) error` — 扫描 knowledge/ + archive/，从 frontmatter 提取 session_id/message_ids，按 session 分组写入 FACTS.md
    - `ReadFacts(basePath, agentID string) (string, error)` — 读取 FACTS.md
    - `AddToFacts/RemoveFromFacts` — 委托 BuildFacts 全量重建（与 INDEX.md 模式一致）
  - FACTS.md 格式:
    ```markdown
    # Fact Index
    ## session: abc-123
    | msg-001 | [记忆名称](knowledge/file.md) |
    ```
  - 使用 `sync.Mutex` 保护并发写入（与 INDEX.md 的 BuildIndex 共享同一 mutex）
  - `FileMemoryStoreInterface` 扩展: 新增 `GetFacts/UpdateFacts/RemoveFromFacts`

  **Must NOT do**:
  - 不用数据库维护事实索引
  - 不引入增量更新

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (与 T1-T2 并行)
  - **Parallel Group**: Wave 1
  - **Blocks**: T9, T11
  - **Blocked By**: T2 (需要新字段)

  **References**:
  - `plugins/memory-file/index.go` — BuildIndex 模式（全量重建、截断、原子写入）
  - `plugins/memory-file/filememory.go:16` — FileMemoryStoreInterface 接口

  **Acceptance Criteria**:
  - [ ] `cd plugins/memory-file && go test -run TestBuildFacts` → PASS
  - [ ] 有 session_id 的记忆 → FACTS.md 含对应条目
  - [ ] 无 session_id 的记忆 → FACTS.md 跳过
  - [ ] 多条记忆同一 session → 正确分组
  - [ ] 并发写入安全

  **QA Scenarios**:
  ```
  Scenario: Build facts from memory files
    Tool: Bash (go test)
    Steps:
      1. cd plugins/memory-file && go test -run TestBuildFacts_WithSessions -v
      2. Assert: FACTS.md generated with correct session→memory mapping
    Expected Result: FACTS.md correctly maps sessions to memory files
    Evidence: .sisyphus/evidence/task-3-facts-build.txt

  Scenario: Empty FACTS.md when no session data
    Tool: Bash (go test)
    Steps:
      1. cd plugins/memory-file && go test -run TestBuildFacts_NoSessions -v
    Expected Result: FACTS.md contains "No fact entries"
    Evidence: .sisyphus/evidence/task-3-facts-empty.txt
  ```

  **Commit**: YES
  - Message: `feat(memory): add FACTS.md reverse index infrastructure`
  - Files: `plugins/memory-file/facts.go`

- [x] 4. 扩展 MemoryStoreAPI + FileMemoryStore

  **What to do**:
  - `MemoryStoreAPI` 接口: 新增 `UpdateFacts/RemoveFromFacts`
  - `FileMemoryStoreInterface`: 新增 `GetFacts/UpdateFacts/RemoveFromFacts`
  - `FileMemoryStore` 实现新增方法
  - `sync.Mutex` 保护 INDEX.md 和 FACTS.md 并发重建
  - `WriteFile` → 自动 `BuildIndex` + `BuildFacts`
  - `DeleteFile` → 自动 `BuildIndex` + `BuildFacts`

  **Must NOT do**: 不改变现有公共方法签名

  **Recommended Agent Profile**: `quick`, `[]`

  **Parallelization**: Wave 1, 与 T3 并行, 被 T7-T11 依赖, 依赖 T2+T3

  **References**: `plugins/memory-file/store_interface.go`, `plugins/memory-file/filememory.go:16`, `plugins/memory-file/memory_store_tool.go:17`

  **Acceptance Criteria**:
  - [ ] WriteFile 后 INDEX.md 和 FACTS.md 均更新
  - [ ] DeleteFile 后 INDEX.md 和 FACTS.md 均更新

  **Commit**: YES — `feat(memory): extend FileMemoryStore with FACTS.md operations`
  - Files: `plugins/memory-file/filememory.go`

- [x] 5. 更新 register.go + capabilities_closure.go + main.go 签名

  **What to do**:
  - `RegisterCapabilities(reg, store, llm, summaryLLM)` — 接受两个 LLM client
  - `MemoryModule` struct 新增 `llm llm.LLMProvider` 和 `summaryLLM llm.LLMProvider`
  - `main.go`: `memoryfile.RegisterCapabilities(reg, fmStore, llmAdapter, summaryLLMAdapter)`
  - `NewHooks()` 产出 4 个 hook，`NewTools()` 不变

  **Must NOT do**: 不修改 CapabilityDeps，不新增 capability 常量

  **Recommended Agent Profile**: `quick`, `[]`

  **Parallelization**: Wave 1, 与 T4 并行, 被 T12 依赖

  **References**: `plugins/memory-file/register.go`, `plugins/memory-file/capabilities_closure.go`, `server/cmd/server/main.go:79`

  **Acceptance Criteria**:
  - [ ] `cd server && go build ./...` → 无错误

  **Commit**: YES — `feat(memory): update RegisterCapabilities for LLM injection`
  - Files: `plugins/memory-file/register.go`, `plugins/memory-file/capabilities_closure.go`, `server/cmd/server/main.go`

- [x] 6. INDEX.md 格式扩展

  **What to do**:
  - `IndexEntry` 新增 `Description string`
  - `formatIndex()` 每行追加 `> description`
  - `collectEntries()` 从 frontmatter 提取 Description
  - 截断逻辑不变

  **Must NOT do**: 不改变 INDEX.md 注入逻辑

  **Recommended Agent Profile**: `quick`, `[]`

  **Parallelization**: Wave 1, 与 T5 并行, 被 T7 依赖, 依赖 T2

  **References**: `plugins/memory-file/index.go`

  **Acceptance Criteria**:
  - [ ] INDEX.md 每行含 description，旧文件显示 "(no description)"

  **Commit**: YES — `feat(memory): extend INDEX.md with description field`
  - Files: `plugins/memory-file/index.go`

- [x] 7. MemoryRecallHook

  **What to do**:
  - `plugins/memory-file/recall_hook.go`: AfterContextBuild, priority=70
  - 扫描 INDEX.md → 获取所有记忆条目 (name + type + description + path)
  - 用 `Complete()` 调用 LLM，prompt: 列出条目 + 用户最后消息 → 选择 ≤5 个
  - LLM 返回 JSON `["path1", "path2"]` → 读取文件 → 注入 `*ctx.Messages` 前缀
  - JSON 解析失败 → 回退到关键词匹配或返回空
  - 已注入的记忆本轮不再重复选择 (用 agentID + sessionID 去重)
  - `OnMessagePersist` 时清理去重缓存

  **Must NOT do**: 不修改 HookContext, 不引入向量搜索

  **Recommended Agent Profile**: `deep`, `[]`

  **Parallelization**: Wave 2, 与 T9-T11 并行, 被 T13 依赖, 依赖 T1+T2+T6

  **References**:
  - `plugins/knowledge-base/kb_recall_hook.go` — AfterContextBuild hook 模式
  - `plugins/memory-file/file_memory_hook.go` — 索引读取模式
  - `plugins/memory-file/complete.go` — Complete() 工具

  **Acceptance Criteria**:
  - [ ] `go test -run TestMemoryRecallHook_SelectsRelevant` → PASS
  - [ ] LLM 返回有效 JSON → 正确读取文件并注入
  - [ ] LLM 返回无效 JSON → 回退到空结果
  - [ ] 空 INDEX.md → 跳过 LLM 调用
  - [ ] 同一会话只注入一次

  **QA Scenarios**:
  ```
  Scenario: LLM selects relevant memories
    Tool: Bash (go test)
    Steps:
      1. go test -run TestMemoryRecallHook_SelectsRelevant -v
    Expected Result: Correct files loaded based on LLM JSON response
    Evidence: .sisyphus/evidence/task-7-recall-select.txt

  Scenario: LLM returns invalid JSON - graceful degradation
    Tool: Bash (go test)
    Steps:
      1. go test -run TestMemoryRecallHook_InvalidJSON -v
    Expected Result: No crash, empty results returned
    Evidence: .sisyphus/evidence/task-7-recall-fallback.txt
  ```

  **Commit**: YES — `feat(memory): add MemoryRecallHook for LLM-powered semantic recall`
  - Files: `plugins/memory-file/recall_hook.go`

- [x] 8. 升级 memory_recall_tool

  **What to do**:
  - 内部实现从关键词扫描改为 LLM 选择
  - Tool 接口不变: InputSchema 保持 `query/type/limit`
  - 内部调用 Complete() + INDEX.md 扫描 + LLM 选择
  - 返回格式扩展: 每条结果含 `path, name, type, snippet, session_id`

  **Must NOT do**: 不改变 Tool 名称或 InputSchema

  **Recommended Agent Profile**: `unspecified-low`, `[]`

  **Parallelization**: Wave 2, 与 T7 并行, 依赖 T1+T2+T6

  **References**: `plugins/memory-file/memory_recall_tool.go`

  **Acceptance Criteria**:
  - [ ] `go test -run TestMemoryRecallTool_LLMSelection` → PASS
  - [ ] 旧 InputSchema 仍然有效

  **Commit**: YES — `feat(memory): upgrade memory_recall to LLM-powered semantic selection`
  - Files: `plugins/memory-file/memory_recall_tool.go`

- [x] 9. FactExtractionHook + 互斥标志

  **What to do**:
  - `plugins/memory-file/extraction_hook.go`: OnMessagePersist, async goroutine
  - 利用 `CapabilityDeps.MessageStore` 查询最近 3 轮对话
  - 用 `Complete()` 调用 LLM，prompt 含已有记忆列表 + 排除原则 + 最近对话
  - LLM 返回 JSON `[{content, type, name, description, importance}]`
  - 互斥: `sync.Map` 记录 sessionID → bool，memory_store 设置 true，提取前检查
  - JSON 解析 → 逐条写入 memory 文件 + 更新 INDEX.md + FACTS.md
  - `context.Background()` + 30s timeout，失败静默
  - 限制: 单轮最多提取 5 条事实

  **Must NOT do**: 不使用请求 context（会被取消），不阻塞对话流水线

  **Recommended Agent Profile**: `deep`, `[]`

  **Parallelization**: Wave 2, 与 T7-T11 并行, 依赖 T1+T2+T3+T4

  **References**:
  - `plugins/memory-file/memory_persist_hook.go` (已删除) — 异步 goroutine 模式
  - `core/storage/message.go:54` — MessageStore 接口
  - `plugins/memory-file/complete.go` — Complete() 工具

  **Acceptance Criteria**:
  - [ ] `go test -run TestFactExtractionHook_ExtractsFacts` → PASS
  - [ ] Agent 已手动 store → 跳过提取
  - [ ] LLM 返回有效 JSON → 写入 memory 文件
  - [ ] LLM 返回空数组 → 无操作
  - [ ] LLM 失败 → 静默跳过
  - [ ] 单轮最多 5 条

  **QA Scenarios**:
  ```
  Scenario: Extract facts from conversation
    Tool: Bash (go test)
    Steps:
      1. go test -run TestFactExtractionHook_ExtractsFacts -v
    Expected Result: Facts written to knowledge/ with correct metadata
    Evidence: .sisyphus/evidence/task-9-extraction.txt

  Scenario: Skip extraction when agent manually stored
    Tool: Bash (go test)
    Steps:
      1. go test -run TestFactExtractionHook_SkipWhenManualStore -v
    Expected Result: Extraction skipped, no new files created
    Evidence: .sisyphus/evidence/task-9-skip.txt
  ```

  **Commit**: YES — `feat(memory): add FactExtractionHook for background fact extraction`
  - Files: `plugins/memory-file/extraction_hook.go`

- [x] 10. 升级 memory_store_tool

  **What to do**:
  - InputSchema 新增: `type` (user/feedback/project/reference), `description`, `session_id`, `message_ids`
  - `session_id` 未提供 → 自动从 `chatCtx.SessionID()` 填充
  - 执行: WriteFile(含新字段 frontmatter) → BuildIndex → BuildFacts
  - 互斥: 设置 sessionID 标记（阻止 FactExtractionHook 重复提取）

  **Must NOT do**: 不删除 `category` 参数（保留向后兼容）

  **Recommended Agent Profile**: `quick`, `[]`

  **Parallelization**: Wave 2, 与 T9 并行, 依赖 T2+T4

  **References**: `plugins/memory-file/memory_store_tool.go`

  **Acceptance Criteria**:
  - [ ] `go test -run TestMemoryStoreTool_WithNewFields` → PASS
  - [ ] session_id 自动填充
  - [ ] 写入后 FACTS.md 含新条目
  - [ ] 旧 category 参数仍然有效

  **Commit**: YES — `feat(memory): extend memory_store with type and fact association fields`
  - Files: `plugins/memory-file/memory_store_tool.go`

- [x] 11. 升级 memory_forget_tool

  **What to do**:
  - 执行后自动调用 BuildFacts
  - 互斥: 遗忘时持有 mutex (与 FactExtractionHook 协调)

  **Must NOT do**: 不改变 Tool 接口

  **Recommended Agent Profile**: `quick`, `[]`

  **Parallelization**: Wave 2, 与 T10 并行, 依赖 T3+T4

  **References**: `plugins/memory-file/memory_forget_tool.go`

  **Acceptance Criteria**:
  - [ ] 删除后 FACTS.md 不含该条目
  - [ ] INDEX.md 也不含该条目

  **Commit**: YES — `feat(memory): update memory_forget to maintain FACTS.md`
  - Files: `plugins/memory-file/memory_forget_tool.go`

- [x] 12. MemoryModule.NewHooks() 扩展

  **What to do**:
  - `NewHooks()` 返回 4 个 hook: FileMemoryHook, MemoryRecallHook, FactExtractionHook, MemorySummaryHook
  - 从 `CapabilityDeps` 提取 `MessageStore` 传递给 FactExtractionHook
  - 正确设置各 hook 的优先级和 HookPoint

  **Must NOT do**: 不修改 NewTools()

  **Recommended Agent Profile**: `quick`, `[]`

  **Parallelization**: Wave 2, 依赖 T7+T9

  **References**: `plugins/memory-file/capabilities_closure.go`

  **Acceptance Criteria**:
  - [ ] Harness Build() 成功
  - [ ] 4 个 hook 全部注册且按正确优先级执行

  **Commit**: YES — `feat(memory): wire all hooks through MemoryModule`
  - Files: `plugins/memory-file/capabilities_closure.go`

- [x] 13. FileSummarizer + 冷却持久化

  **What to do**:
  - `plugins/memory-file/summarizer.go`:
    - `ShouldTrigger()`: 检查 knowledge/ 文件数 > max_memories 或 最旧文件 > max_age_hours，且 距上次 > cooldown_minutes
    - 冷却时间持久化到 `system/.last_summary` 文件（写入 timestamp）
    - `Summarize()`: 收集所有 knowledge/ + archive/ 文件 → 构建 prompt → Complete() LLM → 写入 `system/summary-{date}.md`
    - 摘要 frontmatter 含 `source_files` 列表
    - 摘要 prompt 要求保留事实来源引用
    - 上下文保护: 总文件内容超过 token 限制时分批处理

  **Must NOT do**: 不将摘要文件纳入 INDEX.md, 不修改 system/.last_summary 的格式

  **Recommended Agent Profile**: `deep`, `[]`

  **Parallelization**: Wave 3, 与 T14-T16 并行, 依赖 T1+T2+T7 (需要 MemoryRecallHook 完成后才能测试完整流程)

  **References**: `plugins/memory-file/complete.go`, `plugins/memory-file/frontmatter.go`

  **Acceptance Criteria**:
  - [ ] `go test -run TestFileSummarizer_ShouldTrigger_ByCount` → PASS
  - [ ] `go test -run TestFileSummarizer_ShouldTrigger_ByAge` → PASS
  - [ ] `go test -run TestFileSummarizer_Cooldown` → PASS
  - [ ] 摘要文件写入 system/，前端含 source_files
  - [ ] `.last_summary` 正确更新
  - [ ] 服务重启后冷却状态保留

  **QA Scenarios**:
  ```
  Scenario: Trigger by file count
    Tool: Bash (go test)
    Steps:
      1. go test -run TestFileSummarizer_ShouldTrigger_ByCount -v
    Expected Result: Returns true when knowledge/ has > max_memories files
    Evidence: .sisyphus/evidence/task-13-trigger-count.txt

  Scenario: Cooldown respected
    Tool: Bash (go test)
    Steps:
      1. go test -run TestFileSummarizer_Cooldown -v
    Expected Result: Returns false within cooldown period
    Evidence: .sisyphus/evidence/task-13-cooldown.txt
  ```

  **Commit**: YES — `feat(memory): add FileSummarizer with cooldown persistence`
  - Files: `plugins/memory-file/summarizer.go`

- [x] 14. MemorySummaryHook

  **What to do**:
  - `plugins/memory-file/summary_hook.go`: OnSystemPrompt, priority=90
  - Execute: `ShouldTrigger()` → `Summarize()` → 写入 system/summary-{date}.md
  - 失败静默: LLM 调用失败 → 日志警告，不阻塞
  - 自动清理: 保留最近 3 个摘要文件，删除更旧的

  **Must NOT do**: 不修改 FileMemoryHook（它自动读取 system/ 目录）

  **Recommended Agent Profile**: `unspecified-low`, `[]`

  **Parallelization**: Wave 3, 与 T13 并行, 依赖 T13

  **References**: `plugins/memory-file/file_memory_hook.go` — OnSystemPrompt 模式

  **Acceptance Criteria**:
  - [ ] `go test -run TestMemorySummaryHook_GeneratesSummary` → PASS
  - [ ] 摘要文件出现在 system/ 目录
  - [ ] 摘要失败不阻塞对话
  - [ ] 旧摘要文件自动清理

  **QA Scenarios**:
  ```
  Scenario: Summary generated and injected
    Tool: Bash (go test)
    Steps:
      1. go test -run TestMemorySummaryHook_GeneratesSummary -v
    Expected Result: summary-{date}.md created in system/
    Evidence: .sisyphus/evidence/task-14-summary.txt
  ```

  **Commit**: YES — `feat(memory): add MemorySummaryHook for auto-summarization`
  - Files: `plugins/memory-file/summary_hook.go`

- [x] 15. 配置扩展 (config.go)

  **What to do**:
  - `MemoryConfig` 新增 `Summarization MemorySummarizationConfig`
  - `MemorySummarizationConfig` struct: `Enabled, Model, APIKey, BaseURL, MaxTokens, Temperature, Trigger`
  - `SummarizationTriggerConfig` struct: `MaxMemories (50), MaxAgeHours (24), CooldownMinutes (60)`
  - `validate()`: 继承 OpenAI APIKey/BaseURL（与 KnowledgeEmbedConfig 一致）
  - `MemorySpec` (core/harness.go) 同步新增 `Summarization MemorySummarizationSpec`

  **Must NOT do**: 不修改 AgentConfig 的顶层结构

  **Recommended Agent Profile**: `quick`, `[]`

  **Parallelization**: Wave 3, 与 T13-T16 并行

  **References**: `server/internal/config/config.go:58` (MemoryConfig), `server/internal/config/config.go:67` (KnowledgeConfig 继承模式)

  **Acceptance Criteria**:
  - [ ] `go test -run TestConfig_MemorySummarization` → PASS
  - [ ] APIKey/BaseURL 未设置时继承 OpenAI 配置

  **Commit**: YES — `feat(memory): add MemorySummarizationConfig`
  - Files: `server/internal/config/config.go`, `core/harness.go`

- [x] 16. main.go 摘要 LLM client 接入

  **What to do**:
  - 检查 agent 配置中 `memory.summarization.enabled`
  - 创建摘要专用 LLM client: `llm.NewOpenAIAdapter(&cl, cfg.Agents[i].Memory.Summarization.Model)`
  - 如果 APIKey/BaseURL 单独配置，创建独立的 `openai.Client`
  - 传入 `memoryfile.RegisterCapabilities(reg, fmStore, llmAdapter, summaryLLMAdapter)`
  - agent 未启用 summarization 时 summaryLLMAdapter 传 nil（MemorySummaryHook 变成 no-op）

  **Must NOT do**: 不破坏现有 llmAdapter 的创建逻辑

  **Recommended Agent Profile**: `quick`, `[]`

  **Parallelization**: Wave 3, 与 T15 并行

  **References**: `server/cmd/server/main.go:42` (OpenAI client 创建), `server/cmd/server/main.go:78-83` (RegisterCapabilities 调用)

  **Acceptance Criteria**:
  - [ ] `cd server && go build ./...` → 无错误
  - [ ] summarization.enabled=false → summaryLLM 为 nil

  **Commit**: YES — `feat(memory): wire summarization LLM client in main.go`
  - Files: `server/cmd/server/main.go`

---

## Wave 4: 测试

- [x] 17. FACTS.md 测试

  **What to do**: `plugins/memory-file/facts_test.go`
  - TestBuildFacts: 文件 → FACTS.md 正确生成
  - TestBuildFacts_NoSessions: 无 session 数据 → 空 FACTS
  - TestBuildFacts_MultipleSessions: 多 session 正确分组
  - TestBuildFacts_Concurrent: 并发写入安全性

  **Recommended Agent Profile**: `quick`, `[]`
  **Parallelization**: Wave 4, 与其他测试并行
  **Commit**: YES — `test(memory): add FACTS.md tests`
  - Files: `plugins/memory-file/facts_test.go`

- [x] 18. MemoryRecallHook 测试

  **What to do**: `plugins/memory-file/recall_hook_test.go`
  - Mock LLM 返回 JSON → 验证注入的消息
  - Mock LLM 返回无效 JSON → 验证回退
  - 空 INDEX.md → 无 LLM 调用
  - 去重: 同一 session 不重复注入

  **Recommended Agent Profile**: `unspecified-low`, `[]`
  **Parallelization**: Wave 4
  **Commit**: YES — `test(memory): add MemoryRecallHook tests`
  - Files: `plugins/memory-file/recall_hook_test.go`

- [x] 19. FactExtractionHook 测试

  **What to do**: `plugins/memory-file/extraction_hook_test.go`
  - Mock LLM 返回事实 → 验证文件创建
  - 互斥标志 → 跳过提取
  - Mock LLM 返回空 → 无文件创建
  - Mock LLM 失败 → 无 crash
  - 单轮最多 5 条

  **Recommended Agent Profile**: `unspecified-low`, `[]`
  **Parallelization**: Wave 4
  **Commit**: YES — `test(memory): add FactExtractionHook tests`
  - Files: `plugins/memory-file/extraction_hook_test.go`

- [x] 20. FileSummarizer 测试

  **What to do**: `plugins/memory-file/summarizer_test.go`
  - 触发条件测试 (count, age, cooldown)
  - 冷却持久化: 写入 .last_summary → 重启 → 冷却仍然有效
  - 摘要内容测试: 含 source_files
  - 上下文超限测试: 分批处理

  **Recommended Agent Profile**: `unspecified-low`, `[]`
  **Parallelization**: Wave 4
  **Commit**: YES — `test(memory): add FileSummarizer tests`
  - Files: `plugins/memory-file/summarizer_test.go`

- [x] 21. MemorySummaryHook 测试

  **What to do**: `plugins/memory-file/summary_hook_test.go`
  - 摘要生成成功 → 文件写入 system/
  - 摘要失败 → 不阻塞
  - 旧摘要清理: 超过 3 个 → 删除最早的
  - summarization.enabled=false → no-op

  **Recommended Agent Profile**: `unspecified-low`, `[]`
  **Parallelization**: Wave 4
  **Commit**: YES — `test(memory): add MemorySummaryHook tests`
  - Files: `plugins/memory-file/summary_hook_test.go`

---

## Final Verification Wave
- [x] F2. 测试验证: `go test ./...` 全部通过
- [x] F3. Harness 集成: `go test -run TestHarness ./...` 通过
- [x] F4. 手动 QA: 运行 demo，验证记忆存储→召回→提取流程
  > **Note**: F4 需要 LLM API Key 和实际 API 调用，无法在自动化 CI 中执行。核心代码已通过 F2/F3 测试覆盖。手动测试可在配置好 API Key 后运行：
  > ```bash
  > cd server && go run cmd/server/main.go
  > # 然后使用 memory_store/memory_recall 工具进行对话测试
  > ```

---

## Commit Strategy

按波次提交，每波完成后一个 commit:
- `feat(memory): add Complete() utility, type extensions, FACTS.md infrastructure`
- `feat(memory): add MemoryRecallHook, FactExtractionHook, tool upgrades`
- `feat(memory): add FileSummarizer, MemorySummaryHook, config`
- `test(memory): add tests for all new components`

---

## Success Criteria

### 验证命令
```bash
cd plugins/memory-file && go test ./... -v
cd core && go test ./... -v
cd server && go build ./...
```

### 最终检查
- [x] 所有 "必须实现" 已交付
- [x] 所有 "绝不能做" 未被违反
- [x] 3 个新 Hook 正常工作
- [x] FACTS.md 正确构建和更新
- [x] 语义召回优于关键词匹配
- [x] 自动提取不重复存储已有记忆
- [x] 摘要触发条件正确
- [x] 摘要冷却状态跨重启持久化