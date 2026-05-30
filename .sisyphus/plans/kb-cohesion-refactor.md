# 知识库模块内聚重组 + sqlite-vec 接入

## TL;DR

> **快速摘要**：将 `plugins/` 下零散的知识库相关包（embedding-openai、rag、eval）内聚到 `knowledge-base/` 模块内，将记忆持久化能力归位到 `memory-file/`，同步迁移 SQLite 驱动从 glebarez/sqlite 到 modernc/sqlite + sqlite/vec 扩展实现原生向量索引。
>
> **产出物**：
> - `plugins/knowledge-base/embedding/` — 向量化子模块
> - `plugins/knowledge-base/rag/` — 文档摄入管线
> - `plugins/knowledge-base/eval/` — 检索评估
> - `plugins/knowledge-base/store/sqlitevec/` — sqlite-vec 驱动的向量存储
> - `plugins/memory-file/` — 含 memory_persist_hook 的完整记忆模块
>
> **预估工作量**：Large（~40 个文件变更）
> **并行执行**：YES — 5 个 Wave
> **关键路径**：目录重组 → 接口统一 → 驱动迁移 → import 更新 → 全量验证

---

## Context

### 原始需求
用户发现 plugins/ 中功能模块内聚不够，`embedding-openai`、`rag`、`eval` 都是知识库的能力而非独立功能级包。同时要求接入 sqlite-vec 扩展实现真正的向量索引，替代当前应用层暴力检索。

### 讨论决策
1. **sqlite-vec 接入**：选择「迁移到 modernc/sqlite + modernc/sqlite/vec」
2. **PipelineStore 统一**：合并到 KnowledgeStore 接口
3. **eval/testdata 归属**：随包移动到 knowledge-base/eval/testdata/
4. **memory_persist_hook**：移入 memory-file，删除 MemoryStoreDeps
5. **拆分两阶段**：目录重构先行，sqlite-vec 接入随后，中间有全量验证门

### 研究发现
- **依赖方向**：server → plugins → core（单向，无循环）
- **跨插件引用**：仅 `sqlitevec/ → knowledge-base/`（子包引用父接口，重组后保持合法）
- **server 侧引用点**：3 个文件（main.go, handlers.go, knowledge_options.go）

### Metis 审查
- **关键发现**：glebarez/sqlite 与 sqlite-vec 不兼容，必须迁移到 modernc/sqlite 驱动
- **已解决**：确认迁移方案、接口统一策略、testdata 移动方案

---

## Work Objectives

### 核心目标
将知识库相关代码内聚到 `plugins/knowledge-base/` 下，将记忆持久化归入 `plugins/memory-file/`，同时用 sqlite-vec 扩展替代暴力余弦检索。

### 交付物
- `plugins/knowledge-base/embedding/`（移入并更新包名）
- `plugins/knowledge-base/rag/`（移入并更新包名）
- `plugins/knowledge-base/eval/`（移入含 testdata）
- `plugins/knowledge-base/store/sqlitevec/`（重命名 + modernc/sqlite + sqlite/vec 集成）
- `plugins/memory-file/memory_persist_hook.go`（移入并适配包名）
- `server/internal/api/*.go`（import 路径更新）

### 验收标准
- [ ] `go build ./plugins/...` 零错误
- [ ] `go test ./plugins/...` 全部通过
- [ ] `go build ./server/...` 零错误
- [ ] `go test ./server/...` 全部通过
- [ ] `go vet ./...` 无新增警告

### Must Have
- 所有移入的包通过编译和测试
- sqlite-vec 的 `vec_f32` 虚拟表正常工作
- KnowledgeStore 的 Search() 返回结果与旧实现一致（允许微小浮点差异）

### Must NOT Have
- 改变已公开接口的语义行为
- 引入 CGO 编译依赖
- 遗留未引用的导入（goimports 验证）
- 在数据库迁移中丢失已有数据（提供迁移脚本）

---

## Verification Strategy

### 测试决策
- **测试框架**：Go testing + testify（当前项目已有）
- **自动化测试**：Tests-after（每个变更后立即运行现有测试）
- **Agent QA**：每个 task 包含可执行验证场景

### QA 策略
每个 task 的验证方式：
- **编译**：`go build ./plugins/...` 和 `go build ./server/...`
- **测试**：`go test ./plugins/...` 和 `go test ./server/...`
- **向量检索**：Bash 执行 go test，对比 Search() 结果

---

## Execution Strategy

### 并行执行 Waves

```
Wave 1（开始 — 接口层变更，全部并行）：
├── Task 1: 合并 PipelineStore 到 KnowledgeStore [quick]
├── Task 2: 清理死代码 + 删除 MemoryStoreDeps [quick]
└── Task 3: 创建 knowledge-base 子目录结构 [quick]

Wave 2（Wave 1 之后 — 目录重组，全部并行）：
├── Task 4: 移入 embedding-openai → knowledge-base/embedding [quick]
├── Task 5: 移入 rag → knowledge-base/rag [deep]
├── Task 6: 移入 eval → knowledge-base/eval [quick]
├── Task 7: 移入 memory_persist_hook → memory-file [deep]
└── Task 8: 重命名 sqlitevec → store/sqlitevec [quick]

Wave 3（Wave 2 之后 — server 侧引用更新 + 验证）：
├── Task 9: 更新 server import 路径 [quick]
├── Task 10: 全量编译验证 plugins [quick]
└── Task 11: 全量编译 + 测试验证 server [quick]

Wave 4（Wave 3 之后 — sqlite-vec 驱动迁移）：
├── Task 12: 迁移 glebarez/sqlite → modernc/sqlite 驱动 [deep]
├── Task 13: 集成 sqlite/vec 扩展，改造 Search() [deep]
└── Task 14: 使现有测试通过 [deep]

Wave FINAL（所有 task 之后 — 并行审查）：
├── Task F1: 计划合规审计（oracle）
├── Task F2: 代码质量审查（unspecified-high）
├── Task F3: 真实手动 QA（unspecified-high）
└── Task F4: 范围一致性检查（deep）

关键路径：Task 1 → Task 5 → Task 9 → Task 12 → Task 13 → Task 14 → F1-F4
最大并行度：Wave 2（5 个并行 task）
```

---

## TODOs

> Implementation + Test = ONE Task. Never separate.
> EVERY task MUST have: Recommended Agent Profile + Parallelization info + QA Scenarios.

- [x] 1. 合并 `rag.PipelineStore` 到 `knowledgebase.KnowledgeStore`

  **What to do**：
  - 在 `plugins/knowledge-base/store_interface.go` 的 `KnowledgeStore` 接口中新增两个方法：
    ```go
    StoreChunks(ctx context.Context, kbID string, docID string, chunks []*storage.Chunk, vectors [][]float32) error
    UpdateDocumentStatus(ctx context.Context, kbID string, docID string, status storage.DocumentStatus) error
    ```
  - 删除 `plugins/rag/pipeline.go` 中的 `PipelineStore` 接口定义
  - 将 `plugins/rag/pipeline.go` 中 `Pipeline` 的 `store` 字段类型从 `PipelineStore` 改为 `knowledgebase.KnowledgeStore`
  - 更新 `pipeline.go` 中添加 `knowledgebase` 的 import
  - 删除 `server/internal/api/knowledge_options.go:19` 的类型断言 `ks.(rag.PipelineStore)`，直接传 `ks`

  **Must NOT do**：
  - 改变任何方法的函数签名或行为
  - 修改 `Pipeline` 的核心逻辑（parse→chunk→embed→store）

  **Recommended Agent Profile**：
  - **Category**：`quick` — Reason：接口合并是纯机械操作
  - **Skills**：`[]`

  **Parallelization**：
  - **Parallel Group**：Wave 1（与 Task 2, 3 并行）
  - **Blocks**：Task 5
  - **Blocked By**：None

  **References**：
  - `plugins/knowledge-base/store_interface.go` — KnowledgeStore 当前接口定义
  - `plugins/rag/pipeline.go:16-20` — PipelineStore 接口定义（待删除）
  - `plugins/rag/pipeline.go:23-28` — Pipeline 结构体
  - `server/internal/api/knowledge_options.go:19` — 类型断言（待删除）

  **Acceptance Criteria**：
  - [ ] `KnowledgeStore` 接口包含 13 个方法（原 11 + 新增 2）
  - [ ] `rag.PipelineStore` 接口已删除
  - [ ] `Pipeline.store` 类型为 `knowledgebase.KnowledgeStore`
  - [ ] `knowledge_options.go` 中无 `rag.PipelineStore` 或类型断言

  **QA Scenarios**：
  ```
  Scenario: plugins 编译验证
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go build ./...
    Expected Result: 退出码 0，零错误
    Evidence: .sisyphus/evidence/task-1-build.txt

  Scenario: server 编译验证
    Tool: Bash
    Steps:
      1. cd /data/copcon/server && go build ./...
    Expected Result: 退出码 0，零错误
    Evidence: .sisyphus/evidence/task-1-server-build.txt
  ```

  **Commit**：YES
  - Message：`refactor: merge PipelineStore into KnowledgeStore interface`
  - Files：`plugins/knowledge-base/store_interface.go`, `plugins/rag/pipeline.go`, `server/internal/api/knowledge_options.go`

- [x] 2. 清理死代码 + 删除 MemoryStoreDeps

  **What to do**：
  - 删除 `plugins/knowledge-base/kb_recall_capability.go`（NewHook 返回 nil, nil）
  - 删除 `plugins/knowledge-base/memory_persist_capability.go`（NewHook 返回 nil, nil）
  - 从 `plugins/knowledge-base/register.go` 中删除 `MemoryStoreDeps` 接口定义（第 12-15 行）
  - 从 `RegisterCapabilities` 函数签名中删除 `ms MemoryStoreDeps` 参数
  - 删除 `RegisterCapabilities` 中对 `memoryPersistHookCapabilityClosure` 的注册
  - 删除 `capabilities_closure.go` 中 `memoryPersistHookCapabilityClosure` 结构体
  - 删除 `memory_persist_hook.go:263` 的 `FormatKBResultsStub()` 函数

  **Must NOT do**：
  - 删除 `memory_persist_hook.go` 文件本身（由 Task 7 移入 memory-file）

  **Recommended Agent Profile**：
  - **Category**：`quick` — Reason：纯删除操作

  **Parallelization**：
  - **Parallel Group**：Wave 1（与 Task 1, 3 并行）
  - **Blocks**：Task 7
  - **Blocked By**：None

  **References**：
  - `plugins/knowledge-base/kb_recall_capability.go:14` — stub
  - `plugins/knowledge-base/memory_persist_capability.go:14` — stub
  - `plugins/knowledge-base/register.go:12-15` — MemoryStoreDeps
  - `plugins/knowledge-base/capabilities_closure.go:22-33` — memoryPersistHookCapabilityClosure

  **Acceptance Criteria**：
  - [ ] `kb_recall_capability.go` 和 `memory_persist_capability.go` 已删除
  - [ ] `MemoryStoreDeps` 已从 register.go 删除
  - [ ] `RegisterCapabilities` 签名变为 `(r *capabilities.Registry, ks KnowledgeStore, emb storage.Embedder)`
  - [ ] `memoryPersistHookCapabilityClosure` 已从 capabilities_closure.go 删除
  - [ ] `FormatKBResultsStub()` 已删除

  **QA Scenarios**：
  ```
  Scenario: 编译验证
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go build ./knowledge-base/...
    Expected Result: 退出码 0，零错误
    Evidence: .sisyphus/evidence/task-2-build.txt

  Scenario: go vet 检查
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go vet ./knowledge-base/...
    Expected Result: 零警告
    Evidence: .sisyphus/evidence/task-2-vet.txt
  ```

  **Commit**：YES
  - Message：`chore: remove dead code (stub capabilities, MemoryStoreDeps, FormatKBResultsStub)`
  - Files：`plugins/knowledge-base/kb_recall_capability.go`, `plugins/knowledge-base/memory_persist_capability.go`, `plugins/knowledge-base/register.go`, `plugins/knowledge-base/capabilities_closure.go`, `plugins/knowledge-base/memory_persist_hook.go`

- [x] 3. 创建 knowledge-base 子目录结构

  **What to do**：
  - 创建 `plugins/knowledge-base/embedding/`
  - 创建 `plugins/knowledge-base/rag/` 和 `plugins/knowledge-base/rag/testdata/`
  - 创建 `plugins/knowledge-base/eval/` 和 `plugins/knowledge-base/eval/testdata/`
  - 创建 `plugins/knowledge-base/store/`

  **Must NOT do**：
  - 移动或修改任何 .go 文件

  **Recommended Agent Profile**：
  - **Category**：`quick` — Reason：纯目录创建

  **Parallelization**：
  - **Parallel Group**：Wave 1（与 Task 1, 2 并行）
  - **Blocks**：Task 4, 5, 6, 8
  - **Blocked By**：None

  **Acceptance Criteria**：
  - [ ] 4 个子目录全部存在

  **QA Scenarios**：
  ```
  Scenario: 验证目录结构
    Tool: Bash
    Steps:
      1. ls -d plugins/knowledge-base/{embedding,rag,eval,store}
    Expected Result: 4 个路径列出
    Evidence: .sisyphus/evidence/task-3-dirs.txt
  ```

  **Commit**：YES
  - Message：`chore: create knowledge-base submodule directories`
  - Files：4 个空目录

- [x] 4. 移入 embedding-openai → knowledge-base/embedding

  **What to do**：
  - 将 `plugins/embedding-openai/` 下所有文件移入 `plugins/knowledge-base/embedding/`
  - 修改 `package embedding` 为 `package kbembedding`
  - 由于 `embedder.go` 中 `type Embedder = storage.Embedder` 是类型别名，保持不变
  - 更新所有 embedder 文件中的内部引用（同一包内无需 import 变更）

  **Must NOT do**：
  - 改变 embedding 包的公开 API

  **Recommended Agent Profile**：
  - **Category**：`quick` — Reason：纯文件移动 + 包名重命名

  **Parallelization**：
  - **Parallel Group**：Wave 2（与 Task 5, 6, 7, 8 并行）
  - **Blocks**：Task 9
  - **Blocked By**：Task 3

  **References**：
  - `plugins/embedding-openai/` — 源目录，7 个 .go 文件

  **Acceptance Criteria**：
  - [ ] `plugins/embedding-openai/` 目录已清空
  - [ ] `plugins/knowledge-base/embedding/` 包含所有 embedder 文件
  - [ ] 包名为 `package kbembedding`
  - [ ] `go build ./plugins/knowledge-base/embedding/...` 通过

  **QA Scenarios**：
  ```
  Scenario: 编译验证
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go build ./knowledge-base/embedding/...
    Expected Result: 退出码 0
    Evidence: .sisyphus/evidence/task-4-build.txt
  ```

  **Commit**：YES
  - Message：`refactor: move embedding-openai into knowledge-base/embedding`
  - Files：`plugins/knowledge-base/embedding/*`, `plugins/embedding-openai/`（删除）

- [x] 5. 移入 rag → knowledge-base/rag

  **What to do**：
  - 将 `plugins/rag/` 下所有 .go 文件移入 `plugins/knowledge-base/rag/`
  - 保留 `plugins/rag/testdata/` 文件，移到 `plugins/knowledge-base/rag/testdata/`
  - 修改 `package rag` 为 `package kbrag`
  - 更新 pipeline.go 中的 import：`knowledgebase "github.com/copcon/plugins/knowledge-base"`（因为 Pipeline.store 类型变为 knowledgebase.KnowledgeStore）
  - 将 chunker_test.go 和 parser_test.go 的测试包名也改为 `package kbrag`

  **Must NOT do**：
  - 改变 Parser、Chunker、Pipeline 的功能逻辑

  **Recommended Agent Profile**：
  - **Category**：`deep` — Reason：涉及多文件 import 变更和包名适配
  - **Skills**：`[]`

  **Parallelization**：
  - **Parallel Group**：Wave 2（与 Task 4, 6, 7, 8 并行）
  - **Blocks**：Task 9
  - **Blocked By**：Task 1, 3

  **References**：
  - `plugins/rag/` — 源目录，12 个 .go 文件 + testdata/
  - `plugins/rag/pipeline.go:16` — PipelineStore（已在 Task 1 删除）
  - `plugins/knowledge-base/store_interface.go` — KnowledgeStore 接口

  **Acceptance Criteria**：
  - [ ] `plugins/rag/` 目录已清空
  - [ ] `plugins/knowledge-base/rag/` 包含所有 rag 文件
  - [ ] 包名为 `package kbrag`
  - [ ] `go build ./plugins/knowledge-base/rag/...` 通过

  **QA Scenarios**：
  ```
  Scenario: 编译验证
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go build ./knowledge-base/rag/...
    Expected Result: 退出码 0
    Evidence: .sisyphus/evidence/task-5-build.txt

  Scenario: rag 测试运行
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go test ./knowledge-base/rag/...
    Expected Result: PASS，无失败
    Evidence: .sisyphus/evidence/task-5-test.txt
  ```

  **Commit**：YES
  - Message：`refactor: move rag into knowledge-base/rag`
  - Files：`plugins/knowledge-base/rag/*`, `plugins/rag/`（删除）

- [x] 6. 移入 eval → knowledge-base/eval

  **What to do**：
  - 将 `plugins/eval/` 下所有 .go 文件移入 `plugins/knowledge-base/eval/`
  - 将项目根目录 `/data/copcon/eval/testdata/` 移入 `plugins/knowledge-base/eval/testdata/`
  - 修改 `package eval` 为 `package kbeval`
  - 更新 `golden_test.go` 中的相对路径：
    `filepath.Join("..", "..", "eval", "testdata", …)` → `filepath.Join("testdata", …)`
  - 更新 `retrieval_test.go` 的相对路径引用（如有）

  **Must NOT do**：
  - 改变 eval 的公开 API
  - 改变 golden 测试的断言逻辑

  **Recommended Agent Profile**：
  - **Category**：`quick` — Reason：纯文件移动 + testdata 路径更新

  **Parallelization**：
  - **Parallel Group**：Wave 2（与 Task 4, 5, 7, 8 并行）
  - **Blocks**：Task 9
  - **Blocked By**：Task 3

  **References**：
  - `plugins/eval/` — 源目录，5 个 .go 文件
  - `eval/testdata/` — 项目根目录测试数据
  - `plugins/eval/golden_test.go:150-151` — 相对路径需更新

  **Acceptance Criteria**：
  - [ ] `plugins/eval/` 目录已清空
  - [ ] `plugins/knowledge-base/eval/` 包含所有 eval 文件 + testdata/
  - [ ] 包名为 `package kbeval`
  - [ ] `go test ./plugins/knowledge-base/eval/...` 通过

  **QA Scenarios**：
  ```
  Scenario: eval 测试运行
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go test ./knowledge-base/eval/...
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-6-test.txt
  ```

  **Commit**：YES
  - Message：`refactor: move eval into knowledge-base/eval with testdata`
  - Files：`plugins/knowledge-base/eval/*`, `plugins/eval/`（删除）, `eval/testdata/`（删除）

- [x] 7. 移入 memory_persist_hook → memory-file

  **What to do**：
  - 复制 `memory_persist_hook.go` 到 `plugins/memory-file/`
  - 修改包名：`package knowledgebase` → `package memoryfile`
  - 删除 `cosineSim()` 和 `toFloat32Slice()` 两个私有函数（检查 memory-file 是否有等价实现，若无则保留但不重复定义）
  - 将 `MemoryStorePersister` 接口与 memory-file 已有的 `MemoryStore` 接口对比：`MemoryStore` 已有 Store/Search 方法，不需重新定义接口
  - 更新 `NewMemoryPersistHook` 的参数类型：`MemoryStorePersister` → `MemoryStore`
  - 在 `plugins/memory-file/register.go` 中新增 `memoryPersistHookCapabilityClosure` 结构和注册
  - `RegisterCapabilities` 签名增加 `emb storage.Embedder` 参数
  - 从 `plugins/knowledge-base/` 删除 `memory_persist_hook.go`

  **Must NOT do**：
  - 改变 MemoryPersistHook 的行为逻辑
  - 重复定义 memory-file 已有的接口

  **Recommended Agent Profile**：
  - **Category**：`deep` — Reason：涉及接口适配和跨包整合
  - **Skills**：`[]`

  **Parallelization**：
  - **Parallel Group**：Wave 2（与 Task 4, 5, 6, 8 并行）
  - **Blocks**：Task 9
  - **Blocked By**：Task 2, 3

  **References**：
  - `plugins/knowledge-base/memory_persist_hook.go` — 源文件
  - `plugins/memory-file/store_interface.go:10-18` — MemoryStore 接口
  - `plugins/memory-file/register.go:5-9` — RegisterCapabilities 模式

  **Acceptance Criteria**：
  - [ ] `memory_persist_hook.go` 在 `plugins/memory-file/` 下，包名 `memoryfile`
  - [ ] `memory_persist_hook.go` 不在 `plugins/knowledge-base/`
  - [ ] `NewMemoryPersistHook` 接受 `MemoryStore` 而非 `MemoryStorePersister`
  - [ ] `memoryfile.RegisterCapabilities` 签名包含 `emb storage.Embedder`

  **QA Scenarios**：
  ```
  Scenario: memory-file 编译验证
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go build ./memory-file/...
    Expected Result: 退出码 0
    Evidence: .sisyphus/evidence/task-7-build.txt

  Scenario: memory-file 测试运行
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go test ./memory-file/...
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-7-test.txt
  ```

  **Commit**：YES
  - Message：`refactor: move memory_persist_hook from knowledge-base to memory-file`
  - Files：`plugins/memory-file/memory_persist_hook.go`, `plugins/memory-file/register.go`, `plugins/knowledge-base/memory_persist_hook.go`（删除）

- [x] 8. 重命名 sqlitevec → store/sqlitevec

  **What to do**：
  - 将 `plugins/knowledge-base/sqlitevec/` 下所有文件移入 `plugins/knowledge-base/store/sqlitevec/`
  - 包名保持不变：`package sqlitevec`（包名不随路径变化）
  - 更新 `knowledge.go` 中 `knowledge-base` 的 import 路径（因为相对路径变了）：`"github.com/copcon/plugins/knowledge-base"` 保持不变（这是同一 Go module 内部的包引用）

  **Must NOT do**：
  - 改变包名（保持 `package sqlitevec`）
  - 修改向量搜索逻辑

  **Recommended Agent Profile**：
  - **Category**：`quick` — Reason：纯目录移动

  **Parallelization**：
  - **Parallel Group**：Wave 2（与 Task 4, 5, 6, 7 并行）
  - **Blocks**：Task 9
  - **Blocked By**：Task 2, 3

  **References**：
  - `plugins/knowledge-base/sqlitevec/` — 源目录，5 个 .go 文件

  **Acceptance Criteria**：
  - [ ] `plugins/knowledge-base/sqlitevec/` 已清空
  - [ ] `plugins/knowledge-base/store/sqlitevec/` 包含所有文件
  - [ ] `go build ./plugins/knowledge-base/store/sqlitevec/...` 通过

  **QA Scenarios**：
  ```
  Scenario: 编译验证
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go build ./knowledge-base/store/sqlitevec/...
    Expected Result: 退出码 0
    Evidence: .sisyphus/evidence/task-8-build.txt
  ```

  **Commit**：YES
  - Message：`refactor: rename sqlitevec to store/sqlitevec`
  - Files：`plugins/knowledge-base/store/sqlitevec/*`, `plugins/knowledge-base/sqlitevec/`（删除）

- [x] 9. 更新 server 侧 import 路径

  **What to do**：
  - `server/cmd/server/main.go`：
    - 删除 `"github.com/copcon/plugins/embedding-openai"`，新增 `kbembedding "github.com/copcon/plugins/knowledge-base/embedding"`
    - 修改 `"github.com/copcon/plugins/knowledge-base/sqlitevec"` → `"github.com/copcon/plugins/knowledge-base/store/sqlitevec"`
    - 更新 `knowledgebase.RegisterCapabilities(reg, ks, emb, fmStore)` → `knowledgebase.RegisterCapabilities(reg, ks, emb)`（删除 fmStore 参数）
    - 增加 `memoryfile.RegisterCapabilities(reg, fmStore, emb)`（增加 emb 参数）
    - 更新 `resolveEmbeddingConfig` 返回类型：`embedding.EmbeddingConfig` → `kbembedding.EmbeddingConfig`
    - 更新 `embedding.NewFromConfig` → `kbembedding.NewFromConfig`
    - 更新 `embedding.BackendType` → `kbembedding.BackendType`
  - `server/internal/api/handlers.go`：
    - 删除 `"github.com/copcon/plugins/rag"`
    - 新增 `kbrag "github.com/copcon/plugins/knowledge-base/rag"`
    - 更新 `*rag.Pipeline` → `*kbrag.Pipeline`
  - `server/internal/api/knowledge_options.go`：
    - 删除 `"github.com/copcon/plugins/rag"`
    - 新增 `kbrag "github.com/copcon/plugins/knowledge-base/rag"`
    - 更新 `rag.NewPipeline` → `kbrag.NewPipeline`
    - 更新 `rag.NewDefaultParser` → `kbrag.NewDefaultParser`
    - 删除 `ks.(rag.PipelineStore)` 及相关逻辑

  **Must NOT do**：
  - 改变任何业务逻辑

  **Recommended Agent Profile**：
  - **Category**：`quick` — Reason：纯 import 路径替换

  **Parallelization**：
  - **Can Run In Parallel**：NO
  - **Parallel Group**：Wave 3（在 Wave 2 全部完成后）
  - **Blocks**：Task 10, 11
  - **Blocked By**：Task 4, 5, 6, 7, 8

  **References**：
  - `server/cmd/server/main.go:18-21` — plugins import
  - `server/cmd/server/main.go:67` — RegisterCapabilities 调用
  - `server/internal/api/handlers.go:16-19` — plugins import
  - `server/internal/api/knowledge_options.go:5-6` — plugins import

  **Acceptance Criteria**：
  - [ ] `go build ./server/...` 通过
  - [ ] 无引用旧 import 路径

  **QA Scenarios**：
  ```
  Scenario: server 编译验证
    Tool: Bash
    Steps:
      1. cd /data/copcon/server && go build ./...
    Expected Result: 退出码 0
    Evidence: .sisyphus/evidence/task-9-build.txt
  ```

  **Commit**：YES
  - Message：`refactor: update server imports for knowledge-base module restructure`
  - Files：`server/cmd/server/main.go`, `server/internal/api/handlers.go`, `server/internal/api/knowledge_options.go`

- [x] 10. 全量编译 + 测试验证 plugins

  **What to do**：
  - 运行 `go build ./plugins/...`
  - 运行 `go test ./plugins/...`
  - 检查无编译错误、无测试失败

  **Recommended Agent Profile**：
  - **Category**：`quick` — Reason：编译/测试验证

  **Parallelization**：
  - **Can Run In Parallel**：YES（与 Task 11 可并行）
  - **Parallel Group**：Wave 3
  - **Blocks**：Task 12
  - **Blocked By**：Task 9

  **QA Scenarios**：
  ```
  Scenario: 全量编译
    Tool: Bash (timeout=180000)
    Steps:
      1. cd /data/copcon/plugins && go build ./...
    Expected Result: 退出码 0
    Evidence: .sisyphus/evidence/task-10-build.txt

  Scenario: 全量测试
    Tool: Bash (timeout=300000)
    Steps:
      1. cd /data/copcon/plugins && go test ./...
    Expected Result: 全部 PASS
    Evidence: .sisyphus/evidence/task-10-test.txt
  ```

  **Commit**：NO（验证 step）

- [x] 11. 全量编译 + 测试验证 server

  **What to do**：
  - 运行 `go build ./server/...`
  - 运行 `go test ./server/...`
  - 检查无编译错误、无测试失败

  **Recommended Agent Profile**：
  - **Category**：`quick` — Reason：编译/测试验证

  **Parallelization**：
  - **Can Run In Parallel**：YES（与 Task 10 可并行）
  - **Parallel Group**：Wave 3
  - **Blocks**：Task 12
  - **Blocked By**：Task 9

  **QA Scenarios**：
  ```
  Scenario: 全量编译
    Tool: Bash (timeout=180000)
    Steps:
      1. cd /data/copcon/server && go build ./...
    Expected Result: 退出码 0
    Evidence: .sisyphus/evidence/task-11-build.txt

  Scenario: 全量测试
    Tool: Bash (timeout=300000)
    Steps:
      1. cd /data/copcon/server && go test ./...
    Expected Result: 全部 PASS
    Evidence: .sisyphus/evidence/task-11-test.txt
  ```

  **Commit**：NO（验证 step）

- [x] 12. 迁移 glebarez/sqlite → modernc/sqlite 驱动

  **What to do**：
  - 在 `plugins/knowledge-base/store/sqlitevec/` 中：
    - 替换 import：`"github.com/glebarez/sqlite"` → `"modernc.org/sqlite"`
    - 注意：`modernc.org/sqlite` 不是 GORM 驱动，需要**创建一个轻量 GORM Dialector 适配层**
    - 参考 [modernc.org/sqlite GORM 集成模式](https://pkg.go.dev/modernc.org/sqlite)：
      ```go
      import (
          "database/sql"
          "modernc.org/sqlite"
      )
      // 注册 sqlite 驱动
      func init() { sql.Register("sqlite", &sqlite.Driver{}) }
      ```
    - 使用 `gorm.io/gorm` 的 `gorm.Open` 配合标准 `database/sql` 连接：
      ```go
      sqlDB, _ := sql.Open("sqlite", dsn)
      db, _ := gorm.Open(&gorm.Config{ConnPool: sqlDB})
      ```
  - 同样的迁移在测试文件中执行
  - `plugins/go.mod`：删除 `github.com/glebarez/sqlite`，添加 `modernc.org/sqlite`
  - `server/cmd/server/main.go` 中的 `createKnowledgeStore` 函数也需要更新（它调用 `sqlitevec.NewKnowledgeStoreFromDSN`，不需要直接引用 glebarez/sqlite）

  **Must NOT do**：
  - 改变已有数据库 schema（表结构不变）
  - 丢失现有的 `knowledge.db` 兼容性

  **Recommended Agent Profile**：
  - **Category**：`deep` — Reason：驱动替换涉及 GORM 适配，需要谨慎
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：NO（依赖 Wave 3 验证结果）
  - **Parallel Group**：Wave 4
  - **Blocks**：Task 13
  - **Blocked By**：Task 10, 11

  **References**：
  - `plugins/knowledge-base/store/sqlitevec/knowledge.go:10` — glebarez/sqlite import
  - `plugins/knowledge-base/store/sqlitevec/knowledge.go:35-40` — DSN 打开方式
  - `plugins/knowledge-base/store/sqlitevec/knowledge_test.go:8` — 测试中 glebarez/sqlite import

  **Acceptance Criteria**：
  - [ ] `go build ./plugins/knowledge-base/store/sqlitevec/...` 通过
  - [ ] `go test ./plugins/knowledge-base/store/sqlitevec/...` 全部通过
  - [ ] `go.mod` 中无 `github.com/glebarez/sqlite` 依赖

  **QA Scenarios**：
  ```
  Scenario: sqlitevec 测试通过
    Tool: Bash (timeout=120000)
    Steps:
      1. cd /data/copcon/plugins && go test -v ./knowledge-base/store/sqlitevec/...
    Expected Result: PASS，全部测试通过
    Evidence: .sisyphus/evidence/task-12-test.txt

  Scenario: 确认无 glebarez 依赖
    Tool: Bash
    Steps:
      1. grep -r "glebarez/sqlite" plugins/
    Expected Result: 无任何匹配
    Evidence: .sisyphus/evidence/task-12-grep.txt
  ```

  **Commit**：YES
  - Message：`refactor: replace glebarez/sqlite with modernc.org/sqlite in knowledge store`
  - Files：`plugins/knowledge-base/store/sqlitevec/knowledge.go`, `plugins/knowledge-base/store/sqlitevec/knowledge_test.go`, `plugins/knowledge-base/store/sqlitevec/integration_test.go`, `plugins/go.mod`, `plugins/go.sum`

- [x] 13. 集成 sqlite/vec 扩展，改造 Search()

  **What to do**：
  - 在 `plugins/go.mod` 中添加：
    - `modernc.org/sqlite`（已在 Task 12 添加）
    - 注意：`modernc.org/sqlite` v1.45+ 内置了 `vec0` 虚拟表支持
  - 在 schema.go 中增加 `initVectorTable()`：
    ```go
    func (s *KnowledgeStore) initVectorTable(ctx context.Context) error {
        dim := s.dimensions
        if dim == 0 {
            dim = 1536 // text-embedding-3-small default
        }
        _, err := s.sqlDB.ExecContext(ctx,
            fmt.Sprintf("CREATE VIRTUAL TABLE IF NOT EXISTS chunks_vec USING vec0(embedding float[%d])", dim))
        return err
    }
    ```
  - 改造 StoreChunks：插入 chunk 后同时写入 chunks_vec：
    ```go
    result := tx.Create(m)
    tx.Exec("INSERT INTO chunks_vec(rowid, embedding) VALUES (?, vec_f32(?))", result.RowsAffected, toBlob(vector))
    ```
  - 改造 Search()：使用 vec0 虚拟表的 KNN 查询替代应用层遍历：
    ```go
    func (s *KnowledgeStore) Search(ctx context.Context, kbIDs []string, query []float32, opts storage.SearchOptions) ([]*storage.Chunk, error) {
        rows, err := s.sqlDB.QueryContext(ctx, `
            SELECT c.id, c.document_id, c.kb_id, c.content, c.context,
                   c.chunk_index, c.token_count, c.metadata,
                   vec_distance_cosine(v.embedding, vec_f32(?)) as score
            FROM chunks c
            JOIN chunks_vec v ON v.rowid = c.vec_rowid
            WHERE c.kb_id IN (`+placeholders(len(kbIDs))+`)
            ORDER BY score ASC
            LIMIT ?`, args...)
        // parse rows...
    }
    ```
  - KnowledgeStore 结构体增加 `sqlDB *sql.DB` 字段用于原始 SQL 操作

  **Must NOT do**：
  - 删除 `cosineSimilarity()` 函数（保留作为 fallback）
  - 改变 Search 的返回类型或语义

  **Recommended Agent Profile**：
  - **Category**：`deep` — Reason：向量索引集成 + SQL 改写
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：NO
  - **Parallel Group**：Wave 4
  - **Blocks**：Task 14
  - **Blocked By**：Task 12

  **References**：
  - `plugins/knowledge-base/store/sqlitevec/knowledge.go:196-247` — 当前 Search 实现
  - `plugins/knowledge-base/store/sqlitevec/schema.go` — GORM 模型定义
  - `plugins/knowledge-base/store/sqlitevec/vector.go` — toBlob/fromBlob 工具

  **Acceptance Criteria**：
  - [ ] `initVectorTable()` 成功创建 chunks_vec 虚拟表
  - [ ] StoreChunks 同时写入 chunks 和 chunks_vec
  - [ ] Search() 使用 SQL 层 vec_distance_cosine 查询
  - [ ] `go test ./plugins/knowledge-base/store/sqlitevec/...` Search 相关测试通过

  **QA Scenarios**：
  ```
  Scenario: vec0 表创建
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go test -run TestNewKnowledgeStore ./knowledge-base/store/sqlitevec/... -v
    Expected Result: PASS，无错误
    Evidence: .sisyphus/evidence/task-13-create-test.txt

  Scenario: 向量搜索测试
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go test -run TestIntegrationVectorSearchAccuracy ./knowledge-base/store/sqlitevec/... -v
    Expected Result: PASS，Search 返回正确的 top-1 结果
    Failure Indicators: 搜索结果排序与旧实现不一致
    Evidence: .sisyphus/evidence/task-13-search-test.txt
  ```

  **Commit**：YES
  - Message：`feat: integrate sqlite/vec extension for native vector search`
  - Files：`plugins/knowledge-base/store/sqlitevec/knowledge.go`, `plugins/knowledge-base/store/sqlitevec/schema.go`, `plugins/go.mod`

- [x] 14. 使全部现有测试通过

  **What to do**：
  - 运行 `go test ./plugins/...` 验证所有测试通过
  - 运行 `go vet ./plugins/...` 检查无误
  - 如果有因 sqlite-vec 改动导致的测试失败，修复之
  - 确认 `plugins/knowledge-base/knowledge_integration_test.go` 仍可通过

  **Recommended Agent Profile**：
  - **Category**：`deep` — Reason：可能需要调试测试失败
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：NO
  - **Parallel Group**：Wave 4（最后一步）
  - **Blocks**：F1-F4
  - **Blocked By**：Task 13

  **QA Scenarios**：
  ```
  Scenario: 全量测试
    Tool: Bash (timeout=300000)
    Steps:
      1. cd /data/copcon/plugins && go test ./...
    Expected Result: 全部 PASS
    Evidence: .sisyphus/evidence/task-14-plugins-test.txt

  Scenario: 全量 vet
    Tool: Bash
    Steps:
      1. cd /data/copcon/plugins && go vet ./...
    Expected Result: 零输出（无警告）
    Evidence: .sisyphus/evidence/task-14-vet.txt

  Scenario: server 全量验证
    Tool: Bash (timeout=300000)
    Steps:
      1. cd /data/copcon/server && go build ./... && go test ./... && go vet ./...
    Expected Result: 全部通过
    Evidence: .sisyphus/evidence/task-14-server.txt
  ```

  **Commit**：YES
  - Message：`test: verify all tests pass after sqlite-vec integration`
  - Files：任何需要修复的测试文件

---

## Final Verification Wave

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, check import path, run build). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go build ./...` for plugins and server. Review all changed files for: unused imports, package name mismatches, stale references to old import paths. Check AI slop: excessive comments, over-abstraction, dead code remnants.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Run `go test ./plugins/...` and `go test ./server/...`. Verify:
  - All knowledge-base/store/sqlitevec tests pass (including vector search)
  - All knowledge-base/rag tests pass (chunker, parser)
  - All knowledge-base/eval tests pass (golden test)
  - memory-file tests pass (including memory_persist if applicable)
  Save to `.sisyphus/evidence/final-qa/`.
  Output: `Tests [N/N pass] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff. Verify 1:1 — everything in spec was built, nothing beyond spec was built. Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

| Task | 消息 | 关键文件 |
|------|------|----------|
| 1 | `refactor: merge PipelineStore into KnowledgeStore interface` | `store_interface.go`, `pipeline.go`, `knowledge_options.go` |
| 2 | `chore: remove dead code (stub capabilities, MemoryStoreDeps, FormatKBResultsStub)` | `register.go`, `capabilities_closure.go` |
| 3 | `chore: create knowledge-base submodule directories` | 4 个空目录 |
| 4 | `refactor: move embedding-openai into knowledge-base/embedding` | `knowledge-base/embedding/*` |
| 5 | `refactor: move rag into knowledge-base/rag` | `knowledge-base/rag/*` |
| 6 | `refactor: move eval into knowledge-base/eval with testdata` | `knowledge-base/eval/*` |
| 7 | `refactor: move memory_persist_hook from knowledge-base to memory-file` | `memory-file/memory_persist_hook.go` |
| 8 | `refactor: rename sqlitevec to store/sqlitevec` | `knowledge-base/store/sqlitevec/*` |
| 9 | `refactor: update server imports for knowledge-base module restructure` | `main.go`, `handlers.go`, `knowledge_options.go` |
| 12 | `refactor: replace glebarez/sqlite with modernc.org/sqlite in knowledge store` | `go.mod`, `knowledge.go` |
| 13 | `feat: integrate sqlite/vec extension for native vector search` | `knowledge.go`, `schema.go` |
| 14 | `test: verify all tests pass after sqlite-vec integration` | 任何需要修复的测试 |

---

## Success Criteria

### 验证命令
```bash
# plugins 全量编译
cd plugins && go build ./...           # Expected: 零错误

# plugins 全量测试
cd plugins && go test ./...            # Expected: 全部 PASS

# server 全量编译
cd server && go build ./...            # Expected: 零错误

# server 全量测试
cd server && go test ./...             # Expected: 全部 PASS

# 全量 vet
go vet ./plugins/... && go vet ./server/...  # Expected: 零输出
```

### 最终检查清单
- [ ] 知识库全部能力在 `plugins/knowledge-base/` 下内聚
- [ ] `plugins/embedding-openai/`, `plugins/rag/`, `plugins/eval/` 已清空
- [ ] `plugins/knowledge-base/store/sqlitevec/` 使用 modernc.org/sqlite + vec
- [ ] `plugins/memory-file/` 包含 memory_persist_hook
- [ ] server 正确引用新路径
- [ ] 无遗留 dead code
- [ ] 全部测试通过