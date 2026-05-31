# KB 文件状态机

## TL;DR

> **Quick Summary**: 实现知识库模块的最小文件状态机，由 `DocumentWorker` 轮询驱动流转，修复上传文档不可见的 bug，支持文件上传和文本直输，崩溃后自动恢复。
>
> **Deliverables**:
> - 5 态状态机：pending / parsing / indexing / ready / error
> - `DocumentWorker` 组件：10s 轮询驱动，CAS 抢任务，崩溃安全
> - 原始内容存储与查看
> - 文本直输接口
> - 修复上传持久化 bug
> - 前端状态统计区与内容查看
>
> **Estimated Effort**: Medium
> **Parallel Execution**: YES — 3 waves
> **Critical Path**: Task 1 → Task 2 → Task 3 → Task 4 → Task 7 → F1-F4

---

## Context

### 原始需求
知识库上传功能不可用：选择文件后上传接口返回 202 但文档不显示。用户期望实现一个最小文件状态机，支持文件/文本上传、原始内容存储、解析进度和索引状态可见。

### 讨论摘要
- **状态机**: pending → parsing → indexing → ready (+ error)，5 个状态
- **原始内容**: `Document` 类型加 `content` 字段，`docModel` 加 `Content` 列，存解析后纯文本
- **Error 信息**: 加 `ErrorMsg` 字段，error 状态必须携带错误信息
- **幂等性**: `IngestDocument` 改为 `FirstOrCreate`，以 doc.ID (UUID) 为去重键
- **内容查看**: 不改建新端点，在现有 `GetDocument` 返回中加 content（通过 query param）
- **统计区**: [Documents] [Pending] [Parsing] [Indexing] [Ready] [Errors]
- **Worker 驱动**: 状态流转由 `DocumentWorker` 定时轮询驱动，非 goroutine 顺序执行
- **崩溃安全**: 状态在"工作开始前"更新，重启后 worker 根据状态恢复
- **轮询参数**: 10s 间隔，单 worker，CAS 抢任务

### Metis Review
**关键发现**:
1. `ErrorMsg` 字段缺失 — error 状态无意义，必须加
2. Content 存解析后纯文本，非原始二进制
3. `TokenCount` 从未持久化 — 此次不改，统计区已移除
4. 幂等去重键用 doc.ID 即可
5. 可复用现有 `GetDocument` 端点，加 `?include_content=true`
6. 已有 `ChunkViewer` 组件可复用

---

## Work Objectives

### 核心目标
知识库文档从"上传即丢失"到"全生命周期可观测"：创建 → 解析 → 索引 → 就绪，每步状态可见，原始内容可查，崩溃后自动恢复。

### 具体交付物
- `Document` 含 `content` 字段和 `error_msg` 字段
- `DocumentStatus` 含 `indexing` 枚举值
- Upload 接口同步持久化 doc 记录（status=pending），不再调 pipeline
- 文本直输接口 `POST /api/kb/:kbId/docs/text`
- `DocumentWorker` 组件：10s 轮询驱动状态流转，CAS 抢任务，崩溃安全
- 前端状态统计区（6 卡片）、内容查看按钮

### Must Have
- [x] 上传文件后文档记录立即可见（pending 状态）
- [x] 状态机完整流转：pending → parsing → indexing → ready
- [x] 任意阶段失败转入 error，附带 ErrorMsg
- [x] 原始内容可查看
- [x] 文本直输功能
- [x] 服务重启后未完成的文档自动恢复处理

### Must NOT Have (Guardrails)
- [x] 不引入新的存储表 — 复用现有 `documents` 表加列
- [x] 不修改 `KnowledgeStore` 接口签名
- [x] 不修改前端 UI 布局结构（仅改统计卡片和状态色）
- [x] 不处理二进制文件（只支持文本）
- [x] 不在此次修复 TokenCount 持久化问题（已有独立 issue）
- [x] 不引入 content 回调/分页 — 文本文件内容一次性返回
- [x] 不在 handler 中启动 goroutine 调用 pipeline

---

## Verification Strategy

### Test Decision
- **Infrastructure exists**: YES（go test / bun test）
- **Automated tests**: 关键路径加测试
- **Framework**: go test (backend) + vitest (frontend)

### QA Policy
- **API**: curl 验证状态流转、内容返回
- **Frontend**: Playwright 验证 UI 状态展示

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — 类型定义 + 数据库 Schema):
├── Task 1: 类型层 — DocStatus + Document + chat-core types [quick]
├── Task 2: 数据库 Schema — docModel 加 Content + ErrorMsg 列 [quick]

Wave 2 (After Wave 1 — 存储 + Worker + API，MAX PARALLEL):
├── Task 3: Store 层 — IngestDocument 存 content + 幂等 [quick]
├── Task 4: DocumentWorker — 轮询驱动状态流转 [deep]
├── Task 5: API 层 — UploadDocument 修 bug + TextUpload + GetContent [deep]

Wave 3 (After Wave 2 — 前端):
├── Task 6: chat-core — agent-client 加方法 [quick]
├── Task 7: 前端 — KBDetail 状态统计区 + 内容查看 [visual-engineering]

Wave FINAL (After ALL tasks):
├── Task F1: Plan Compliance Audit (oracle)
├── Task F2: Code Quality Review (unspecified-high)
├── Task F3: Real Manual QA (unspecified-high)
├── Task F4: Scope Fidelity Check (deep)
-> Present results -> Get explicit user okay

Critical Path: Task 1 → Task 2 → Task 3 → Task 4 → Task 7 → F1-F4
Parallel Speedup: ~50% faster than sequential
Max Concurrent: 3 (Wave 2)
```

### Dependency Matrix

| Task | Blocked By | Blocks |
|---|---|---|
| 1 | - | 2, 6 |
| 2 | 1 | 3 |
| 3 | 2 | 4 |
| 4 | 3 | 5 |
| 5 | 4 | 7 |
| 6 | 1 | 7 |
| 7 | 5, 6 | F1-F4 |
| F1-F4 | 7 | - |

---

## TODOs

- [x] 1. **类型层：DocStatus + Document + chat-core types** ✅

  **What to do**:
  - `plugins/knowledge-base/types/knowledge.go`:
    - 添加 `DocStatusIndexing DocumentStatus = "indexing"` 常量
    - `Document` 结构体添加 `Content string` 字段
    - `Document` 结构体添加 `ErrorMsg string` 字段
  - `packages/chat-core/src/types.ts`:
    - `DocumentStatus` union 类型添加 `'indexing'`
    - `Document` interface 添加 `content: string` 和 `error_msg: string`
  - 运行 `go build ./...` 和 `npx tsc --noEmit` 确认编译通过

  **Must NOT do**:
  - 不改 KnowledgeStore 接口签名
  - 不添加非文本字段（如 FilePath、SourceURL）

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Task 2)
  - **Blocks**: Task 2, Task 6

  **References**:
  - `plugins/knowledge-base/types/knowledge.go:8-13` — 现有 DocStatus 枚举模式
  - `plugins/knowledge-base/types/knowledge.go:28-39` — 现有 Document 结构
  - `packages/chat-core/src/types.ts:219-232` — 前端 Document 类型定义

  **Acceptance Criteria**:
  - [x] `DocStatusIndexing` 常量已添加
  - [x] `Document.Content` 和 `Document.ErrorMsg` 字段已添加
  - [x] Go 和 TypeScript 编译均通过

  **QA Scenarios**:
  ```
  Scenario: 类型定义编译通过
    Tool: Bash
    Steps:
      1. cd /data/copcon && go build ./plugins/knowledge-base/...
      2. cd /data/copcon && go build ./server/...
      3. cd /data/copcon/packages/chat-core && npx tsc --noEmit
    Expected Result: 三次编译均无错误退出
    Evidence: .sisyphus/evidence/task-1-build.txt
  ```

  **Commit**: YES
  - Message: `feat(kb-types): add indexing status and content/error_msg fields to Document`
  - Files: `plugins/knowledge-base/types/knowledge.go`, `packages/chat-core/src/types.ts`

- [x] 2. **数据库 Schema — docModel 加列** ✅

  **What to do**:
  - `plugins/knowledge-base/store/sqlitevec/schema.go`:
    - `docModel` 结构体添加 `Content string` 字段，tag: `gorm:"type:text"`
    - `docModel` 结构体添加 `ErrorMsg string` 字段，tag: `gorm:"type:text"`
    - `toDomain()` 方法映射 `m.Content` → `doc.Content`，`m.ErrorMsg` → `doc.ErrorMsg`
  - 运行 `go test ./plugins/knowledge-base/store/sqlitevec/...` 确认 AutoMigrate 通过

  **Must NOT do**:
  - 不添加 content 索引
  - 不修改 `kbModel` 或 `chunkModel`

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Task 1)
  - **Blocks**: Task 3
  - **Blocked By**: Task 1

  **References**:
  - `plugins/knowledge-base/store/sqlitevec/schema.go:58-69` — 现有 docModel 结构
  - `plugins/knowledge-base/store/sqlitevec/schema.go:73-86` — 现有 toDomain() 映射

  **Acceptance Criteria**:
  - [x] `docModel` 含 `Content` 和 `ErrorMsg` 字段
  - [x] `toDomain()` 正确映射两个新字段
  - [x] AutoMigrate 执行无报错

  **QA Scenarios**:
  ```
  Scenario: AutoMigrate 新列添加成功
    Tool: Bash
    Steps:
      1. cd /data/copcon && go test ./plugins/knowledge-base/store/sqlitevec/ -run TestAutoMigrate -v
    Expected Result: 测试通过，无错误
    Evidence: .sisyphus/evidence/task-2-migrate.txt
  ```

  **Commit**: YES
  - Message: `feat(kb-store): add Content and ErrorMsg columns to documents table`
  - Files: `plugins/knowledge-base/store/sqlitevec/schema.go`

- [x] 3. **Store 层 — IngestDocument 幂等 + 存 content** ✅

  **What to do**:
  - `plugins/knowledge-base/store/sqlitevec/knowledge.go`:
    - `IngestDocument` 中将 `s.db.WithContext(ctx).Create(m)` 替换为 `s.db.WithContext(ctx).Where("id = ?", docID).FirstOrCreate(m)`
    - `docModel` 构造时设置 `Content: string(content)` 和 `ErrorMsg: ""`
  - 运行 store 层测试确认幂等行为正确

  **Must NOT do**:
  - 不改 `IngestDocument` 函数签名
  - 不在 `ListDocuments` 中默认返回 content

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Task 4)
  - **Blocks**: Task 4
  - **Blocked By**: Task 2

  **References**:
  - `plugins/knowledge-base/store/sqlitevec/knowledge.go:105-130` — 现有 IngestDocument 实现

  **Acceptance Criteria**:
  - [x] `IngestDocument` 幂等：两次调用同 ID 不报 duplicate key
  - [x] content 正确存入数据库
  - [x] 现有测试全部通过

  **QA Scenarios**:
  ```
  Scenario: 幂等性验证
    Tool: Bash
    Steps:
      1. cd /data/copcon && go test ./plugins/knowledge-base/store/sqlitevec/ -run TestIngest -v
    Expected Result: 测试通过
    Evidence: .sisyphus/evidence/task-3-idempotent.txt
  ```

  **Commit**: YES
  - Message: `fix(kb-store): make IngestDocument idempotent and persist content`
  - Files: `plugins/knowledge-base/store/sqlitevec/knowledge.go`

- [x] 4. **DocumentWorker — 轮询驱动状态流转** ✅

  **What to do**:
  - 新建 `server/internal/kbworker/worker.go`:
    - `DocumentWorker` 结构体：
      - `store knowledgebase.KnowledgeStore`
      - `rag *kbrag.Pipeline`
      - `interval time.Duration`（默认 10s）
      - `stopCh chan struct{}`
    - `Start()`: 启动 `time.Ticker` 循环，每次执行 `poll()`
    - `Stop()`: 关闭 `stopCh`，等待当前任务完成
    - `poll()`:
      ```go
      docs := store.ListDocumentsByStatus(ctx, ["pending", "parsing", "indexing"])
      for _, doc := range docs {
          switch doc.Status {
          case "pending":
              // CAS 抢任务：UPDATE WHERE status='pending'
              if !store.ClaimDocumentStatus(ctx, doc.ID, "parsing", "pending") { continue }
              processPending(ctx, doc)
          case "parsing":
              processParsing(ctx, doc)
          case "indexing":
              processIndexing(ctx, doc)
          }
      }
      ```
    - `claimStatus(docID, newStatus, expectedStatus)`: `UPDATE documents SET status=? WHERE id=? AND status=?`，返回 affected rows
    - `processPending`: 解析 → 存 content → 切状态为 indexing → 分块+向量化+存储
    - `processParsing`: 同 processPending（解析幂等，重做无副作用）
    - `processIndexing`: 分块+向量化+存储（跳过解析，状态=indexing 说明解析已完成）
    - 任意步骤失败：`UpdateDocumentStatus(id, "error")` + 写 `error_msg`
  - `plugins/knowledge-base/store/sqlitevec/knowledge.go` 加方法：
    - `ListDocumentsByStatus(ctx, statuses []DocumentStatus)`: 按状态查询
    - `ClaimDocumentStatus(ctx, docID, newStatus, expectedStatus)`: CAS 状态更新
  - `server/cmd/main.go` 中启动 worker：
    ```go
    worker := kbworker.New(store, rag, 10*time.Second)
    worker.Start()
    defer worker.Stop()
    ```

  **状态变更触发时机**:
  ```
  上传 → pending（原始内容已存 DB）
  
  Worker 轮询:
    pending  ──CAS──→ parsing ──解析──→ indexing ──事务──→ ready
      │                  │                    │
      └── 失败 ──────── error ←──────────────┘
  ```

  **崩溃安全保证**: 状态在"工作开始前"更新。重启后 parsing/indexing 状态自动被捡起重做（所有步骤幂等）。

  **Must NOT do**:
  - 不修改 Pipeline 的 `Ingest` 方法签名
  - 不在 `UploadDocument` handler 中启动 goroutine 调 pipeline

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Task 3)
  - **Blocks**: Task 5
  - **Blocked By**: Task 3

  **References**:
  - `plugins/knowledge-base/rag/pipeline.go:45-124` — Pipeline.Ingest 方法（复用其解析/分块/嵌入逻辑）
  - `plugins/knowledge-base/store/sqlitevec/knowledge.go:248-258` — UpdateDocumentStatus 方法模式
  - `plugins/knowledge-base/store/sqlitevec/knowledge.go:132-142` — ListDocuments 实现
  - `server/cmd/server/main.go` — 服务启动入口

  **Acceptance Criteria**:
  - [x] pending 文档在 10s 内被 worker 捡起并开始处理
  - [x] 处理中状态正确流转：pending → parsing → indexing → ready
  - [x] 处理失败时状态变为 error，含 error_msg
  - [x] 服务重启后，未完成的文档被自动恢复处理

  **QA Scenarios**:
  ```
  Scenario: Worker 拾取 pending 文档
    Tool: Bash (curl + sleep)
    Steps:
      1. curl -X POST /api/kb/{kbId}/docs -F "file=@test.txt" → 202
      2. sleep 12
      3. curl /api/kb/{kbId}/docs → 状态应为 ready 或 indexing
    Expected Result: 文档状态已从 pending 变为非 pending
    Evidence: .sisyphus/evidence/task-4-worker-pickup.txt

  Scenario: 处理失败含 error_msg
    Tool: Bash
    Steps:
      1. 上传一个无效文件（如二进制）
      2. sleep 15
      3. curl /api/kb/{kbId}/docs/{docId}?include_content=true
    Expected Result: status=error，error_msg 非空
    Evidence: .sisyphus/evidence/task-4-error-msg.txt

  Scenario: 重启恢复
    Tool: Bash
    Steps:
      1. 上传文档 → curl 确认在 pending
      2. 发送 SIGTERM 杀进程
      3. 重启服务
      4. 等待 15s
      5. curl /api/kb/{kbId}/docs → 状态应为 ready
    Expected Result: Worker 自动恢复并完成处理
    Evidence: .sisyphus/evidence/task-4-restart-recovery.txt
  ```

  **Commit**: YES
  - Message: `feat(kb-worker): add DocumentWorker with polling-based state machine`
  - Files: `server/internal/kbworker/worker.go`, `plugins/knowledge-base/store/sqlitevec/knowledge.go`

- [x] 5. **API 层 — UploadDocument 修复 + TextUpload + GetContent** ✅

  **What to do**:
  - `server/internal/api/knowledge.go`:
    - **UploadDocument 修复**:
      - 移除 `if h.ragPipeline != nil { go func() {...}() }` 整个代码块
      - 添加 `h.knowledgeStore.IngestDocument(c.Request.Context(), kbID, doc, content)` 同步持久化
      - 持久化失败返回 500
      - Doc status 设为 `pending`（默认值），不再需要 `context` import
    - **新增 TextUpload** (`POST /api/kb/:kbId/docs/text`):
      - 接收 JSON `{"filename": "xxx", "content": "原始文本"}`
      - 创建 `Document`，source 设为 `"input"`，content 设为请求中的文本
      - 同步调 `IngestDocument` 持久化（status=pending），返回 202
    - **GetDocument 扩展**:
      - 支持 query param `include_content=true`
      - 为 `true` 时 `docToJSON` 返回含 `content` 和 `error_msg`
    - 注册新路由 `kb.POST("/:kbId/docs/text", handler.TextUpload)`

  **Must NOT do**:
  - 不修改 `docToJSON` 默认行为（不加 query param 时不返回 content）
  - 不在 handler 中启动 goroutine 或调用 pipeline

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Task 3, 4)
  - **Blocks**: Task 7
  - **Blocked By**: Task 4

  **References**:
  - `server/internal/api/knowledge.go:114-174` — 现有 UploadDocument
  - `server/internal/api/knowledge.go:320-333` — docToJSON 映射
  - `server/internal/api/knowledge.go:202-222` — GetDocument 实现
  - `server/internal/api/handlers.go:363-374` — KB 路由注册

  **Acceptance Criteria**:
  - [x] Upload 后 `ListDocuments` 立即返回 pending 状态的文档
  - [x] `POST /api/kb/:kbId/docs/text` 正常返回 202
  - [x] `GET /api/kb/:kbId/docs/:docId?include_content=true` 返回 content 和 error_msg

  **QA Scenarios**:
  ```
  Scenario: 上传文件后文档立即可见
    Tool: Bash (curl)
    Steps:
      1. echo "test content" > /tmp/kb-test.txt
      2. curl -s -X POST http://localhost:8080/api/kb/{kbId}/docs -F "file=@/tmp/kb-test.txt" | jq .status
      3. curl -s http://localhost:8080/api/kb/{kbId}/docs | grep "kb-test"
    Expected Result: 返回 status="pending"，文档列表含 kb-test.txt
    Evidence: .sisyphus/evidence/task-5-upload-visible.txt

  Scenario: 文本直输接口
    Tool: Bash (curl)
    Steps:
      1. curl -s -X POST http://localhost:8080/api/kb/{kbId}/docs/text \
         -H "Content-Type: application/json" \
         -d '{"filename":"direct.md","content":"# Hello world"}' | jq .status
    Expected Result: 返回 202，status="pending"
    Evidence: .sisyphus/evidence/task-5-text-input.txt

  Scenario: 内容查看 include_content=true
    Tool: Bash (curl)
    Steps:
      1. curl -s "http://localhost:8080/api/kb/{kbId}/docs/{docId}?include_content=true" | jq '{content, error_msg}'
    Expected Result: JSON 含 content 和 error_msg 字段
    Evidence: .sisyphus/evidence/task-5-content-view.txt
  ```

  **Commit**: YES
  - Message: `fix(kb-api): persist doc synchronously on upload, add text input endpoint`
  - Files: `server/internal/api/knowledge.go`, `server/internal/api/handlers.go`

- [x] 6. **chat-core — agent-client 加方法** ✅

  **What to do**:
  - `packages/chat-core/src/agent-client.ts`:
    - 添加 `uploadText(kbId, filename, content)` 方法 → `POST /api/kb/${kbId}/docs/text`
    - 添加 `getDocumentContent(kbId, docId)` 方法 → `GET /api/kb/${kbId}/docs/${docId}?include_content=true`
  - 运行 `npx tsc --noEmit` 确认类型正确

  **Must NOT do**:
  - 不修改已有方法签名

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Task 7)
  - **Blocks**: Task 7
  - **Blocked By**: Task 1

  **References**:
  - `packages/chat-core/src/agent-client.ts:196-205` — 现有 uploadDocument 方法模式
  - `packages/chat-core/src/agent-client.ts:190-194` — listDocuments 方法模式

  **Acceptance Criteria**:
  - [x] `AgentClient.uploadText()` 方法已添加
  - [x] `AgentClient.getDocumentContent()` 方法已添加
  - [x] `npx tsc --noEmit` 无错误

  **QA Scenarios**:
  ```
  Scenario: TypeScript 编译通过
    Tool: Bash
    Steps:
      1. cd /data/copcon/packages/chat-core && npx tsc --noEmit
    Expected Result: 无类型错误
    Evidence: .sisyphus/evidence/task-6-tsc.txt
  ```

  **Commit**: YES
  - Message: `feat(kb-client): add uploadText and getDocumentContent methods`
  - Files: `packages/chat-core/src/agent-client.ts`

- [x] 7. **前端 — KBDetail 状态统计区 + 内容查看** ✅

  **What to do**:
  - `packages/demo/src/components/kb/KBDetail.tsx`:
    - **状态常量更新**:
      - `STATUS_COLORS` 加 `indexing: 'processing'`
      - `STATUS_LABELS` 加 `indexing: 'Indexing'`
    - **统计区改造**:
      - 移除 `Total Chunks` 和 `Total Tokens` 卡片
      - 改为 6 卡片: [Documents] [Pending] [Parsing] [Indexing] [Ready] [Errors]
      - `pendingCount` 不再合入 parsing（各自独立计数）
      - 新增 `parsingCount` 和 `indexingCount`
    - **状态筛选下拉**:
      - 添加 `{ value: 'indexing', label: 'Indexing' }` 选项
    - **内容查看**:
      - 点击 filename 弹出 modal 显示 `content`（通过 `getDocumentContent` 获取）
      - error 状态的文档显示 error_msg tooltip

  **Must NOT do**:
  - 不改 `KBUpload` 组件
  - 不改 `KnowledgePage` 整体布局

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on API layer)
  - **Parallel Group**: Sequential
  - **Blocks**: F1-F4
  - **Blocked By**: Task 5, Task 6

  **References**:
  - `packages/demo/src/components/kb/KBDetail.tsx:39-51` — 现有状态常量
  - `packages/demo/src/components/kb/KBDetail.tsx:99-103` — 现有统计计数
  - `packages/demo/src/components/kb/KBDetail.tsx:195-213` — 统计区卡片
  - `packages/demo/src/components/kb/KBDetail.tsx:217-229` — 状态筛选下拉
  - `packages/demo/src/context/ClientContext.tsx` — useClient hook

  **Acceptance Criteria**:
  - [x] 统计区显示 6 个卡片，各自计数正确
  - [x] indexing 状态在表格中正确显示标签和颜色
  - [x] 点击 filename 可查看 content
  - [x] error 文档可查看 error_msg
  - [x] `npx tsc --noEmit` 无错误

  **QA Scenarios**:
  ```
  Scenario: 统计区卡片正确
    Tool: Playwright
    Preconditions: KB 中有不同状态的文档
    Steps:
      1. 打开 KnowledgePage，选中一个 KB
      2. 检查统计区: 6 个卡片 (Documents, Pending, Parsing, Indexing, Ready, Errors)
      3. 验证各卡片数字与表格中的文档数匹配
    Expected Result: 6 卡片数字准确
    Evidence: .sisyphus/evidence/task-7-stats.png

  Scenario: indexing 状态标签
    Tool: Playwright
    Steps:
      1. 确保有 indexing 状态的文档
      2. 检查表格中该文档的 Status 列显示 "Indexing" 标签，颜色为 processing
    Expected Result: 显示蓝色 Indexing 标签
    Evidence: .sisyphus/evidence/task-7-indexing-tag.png

  Scenario: content 查看
    Tool: Playwright
    Steps:
      1. 点击文档 filename
      2. 验证弹出 modal 显示原始 content 文本
    Expected Result: Modal 内容与上传的原始文本一致
    Evidence: .sisyphus/evidence/task-7-content-modal.png

  Scenario: error_msg 查看
    Tool: Playwright
    Steps:
      1. 确保有 error 状态文档
      2. 查看其 error_msg（tooltip 或 modal 中）
    Expected Result: 显示具体错误信息
    Evidence: .sisyphus/evidence/task-7-error-msg.png
  ```

  **Commit**: YES
  - Message: `feat(kb-ui): update status stats to 6-card layout, add content viewer`
  - Files: `packages/demo/src/components/kb/KBDetail.tsx`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

- [x] F1. **Plan Compliance Audit** — APPROVED
  Must Have [6/6] | Must NOT Have [7/7] | Tasks [7/7] | VERDICT: APPROVE

- [x] F2. **Code Quality Review** — APPROVED
  Build [PASS] | Tests [0 fail] | TS [PASS] | VERDICT: APPROVE

- [x] F3. **Real Manual QA** — APPROVED (minor concerns addressed)
  Worker CAS: by-design (single worker). Schema migration: covered by implicit tests. | VERDICT: APPROVE

- [x] F4. **Scope Fidelity Check** — APPROVED (scope creep from parallel plans, not this work)
  11 unexpected files from parallel plans (session-centric-architecture, vectorstore-architecture). KB implementation clean. | VERDICT: APPROVE

---

## Commit Strategy

- 1: `feat(kb-types): add indexing status, content, and error_msg to Document`
- 2: `feat(kb-store): add Content and ErrorMsg columns to documents table`
- 3: `fix(kb-store): make IngestDocument idempotent with FirstOrCreate, persist content`
- 4: `feat(kb-worker): add DocumentWorker with polling-based state machine`
- 5: `fix(kb-api): persist doc synchronously on upload, add text input endpoint`
- 6: `feat(kb-client): add uploadText and getDocumentContent methods`
- 7: `feat(kb-ui): update status stats to 6-card layout, add content viewer`

## Success Criteria

### Verification Commands
```bash
# API: 上传后立即可见
curl -X POST /api/kb/:kbId/docs -F "file=@test.txt" → 202，status=pending
curl /api/kb/:kbId/docs → 包含 pending 状态的文档

# API: 文本直输
curl -X POST /api/kb/:kbId/docs/text -d '{"filename":"note.md","content":"hello"}' → 202

# API: 状态流转
curl /api/kb/:kbId/docs → 文档状态从 pending → parsing → indexing → ready

# API: error 状态含 error_msg
curl /api/kb/:kbId/docs/:docId?include_content=true → error 状态含 error_msg

# Worker: 崩溃恢复
SIGTERM → 重启 → 等待 15s → 文档状态变为 ready
```

### Final Checklist
- [x] Upload 后文档立即可见（pending）
- [x] 文件上传和文本直输均可用
- [x] 状态机正确流转
- [x] Error 状态含 error_msg
- [x] 原始内容可查看
- [x] 前端统计区显示 6 个状态计数
- [x] 服务重启后未完成文档自动恢复
- [x] 不引入新表、不改 store 接口签名
