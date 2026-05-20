# GetSessionUpdates API 重新设计

## TL;DR

> **Quick Summary**: 重新设计事件轮询 API，解决当前实现的竞态条件和缺乏队列维护问题，引入有序事件队列、原子消费机制和类型扩展注册。
>
> **Deliverables**:
> - EventQueue 模块（有序队列、序号机制）
> - SessionManager 原子消费方法
> - GetSessionUpdates API 改造
> - 类型注册机制
> - 单元测试（含并发测试）
>
> **Estimated Effort**: Medium
> **Parallel Execution**: YES - 3 waves
> **Critical Path**: T1 → T2 → T3 → T4 → T5

---

## Context

### Original Request
用户指出 GetSessionUpdates 接口设计不足以支持最初的设计意图（事件轮询），缺失：
1. 事件队列的维护
2. 事件的实体定义
3. 类型扩展等设计

### Interview Summary
**Key Discussions**:
- 使用场景：断线重连恢复（SSE 中断后轮询获取异步任务完成通知）
- 事件范围：仅 `AsyncCompletionPending` 类型
- 存储位置：继续使用 `Session.Metadata` JSONB 字段
- 消费模式：返回即删除（GET 后自动清理已返回事件）
- 类型扩展：需要可扩展的类型注册机制
- 队列容量：不限制
- 响应格式：保持 `{has_updates: bool, events: []}`

**Research Findings**:
- 当前实现存在 **竞态条件**：read-modify-write 无事务保护
- ChatContext.events channel 是内存 channel，事件不持久化
- Session.Metadata 是 PostgreSQL JSONB 类型
- 无现成的游标分页模式

### Metis Review
**Identified Critical Issues**:
- **竞态条件**：多客户端同时轮询会导致事件丢失/重复
- **无序号机制**：字符串 ID 字典序比较不可靠
- **无并发保护**：SessionManager 缺乏行级锁

**Guardrails Applied**:
- 必须使用 PostgreSQL 事务 + `SELECT FOR UPDATE` 保证原子性
- 必须使用单调递增序号保证顺序
- 必须在同一事务中读取和删除事件

---

## Work Objectives

### Core Objective
实现可靠的事件轮询机制，支持：
1. 断线重连后获取错过的异步任务完成通知
2. 多客户端并发轮询不丢失/重复事件
3. 可扩展的事件类型注册

### Concrete Deliverables
- `internal/session/eventqueue.go` - EventQueue 类型定义
- `internal/session/manager.go` - 新增原子消费方法
- `internal/api/handlers.go` - GetSessionUpdates 改造
- `internal/domain/entity/event.go` - 事件类型注册机制
- 测试文件

### Definition of Done
- [ ] `go test ./internal/session/... -v` 全部通过
- [ ] `go test ./internal/api/... -v` 全部通过
- [ ] 并发测试验证无事件丢失/重复
- [ ] API 响应格式保持 `{has_updates: bool, events: []}`

### Must Have
- PostgreSQL 事务保护的原子读删操作
- 单调递增序号（int64）保证 FIFO 顺序
- 类型注册机制（支持扩展）
- 并发安全（SELECT FOR UPDATE）

### Must NOT Have (Guardrails)
- 修改 SSE streaming (Chat handler)
- 持久化其他事件类型（仅 AsyncCompletionPending）
- 创建新的数据库表
- 添加事件 TTL/过期机制
- 添加优先队列
- 添加 webhook 推送
- 添加 metrics/监控
- 添加事件重放机制
- 改变前端 API 响应格式

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed.

### Test Decision
- **Infrastructure exists**: YES (go test + testify)
- **Automated tests**: Tests-after
- **Framework**: go test + testify/assert

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Backend**: Use Bash (go test) - Run tests, verify PASS, check coverage
- **API**: Use Bash (curl) - Send requests, assert status + response fields

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Foundation - types and registry):
├── Task 1: EventQueue 类型定义 [quick]
├── Task 2: 事件类型注册机制 [quick]
└── Task 3: EventQueue 单元测试 [quick]

Wave 2 (SessionManager atomic methods):
├── Task 4: AddEvent 方法（带序号） [quick]
├── Task 5: GetAndConsumeEvents 方法（原子操作） [deep]
└── Task 6: SessionManager 测试（含并发） [unspecified-high]

Wave 3 (API and integration):
├── Task 7: GetSessionUpdates 改造 [quick]
├── Task 8: Handler 测试 [unspecified-high]
└── Task 9: 集成验证 [unspecified-high]

Wave FINAL (Review):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Concurrency stress test (unspecified-high)
└── Task F4: Scope fidelity check (deep)
```

### Dependency Matrix

- **1-3**: - - 4-6
- **4**: 1, 2 - 5, 6
- **5**: 1, 4 - 6, 7
- **6**: 4, 5 - 8
- **7**: 5 - 8
- **8**: 6, 7 - 9
- **9**: 8 - F1-F4

### Agent Dispatch Summary

- **Wave 1**: 3 tasks → T1-T3 `quick`
- **Wave 2**: 3 tasks → T4 `quick`, T5 `deep`, T6 `unspecified-high`
- **Wave 3**: 3 tasks → T7 `quick`, T8-T9 `unspecified-high`
- **FINAL**: 4 tasks → F1 `oracle`, F2-F3 `unspecified-high`, F4 `deep`

---

## TODOs

- [ ] 1. **EventQueue 类型定义**

  **What to do**:
  - 在 `internal/session/eventqueue.go` 定义 `QueuedEvent` 结构体，包含序号、类型、时间戳、数据
  - 定义 `EventQueue` 结构体，封装 `[]QueuedEvent`
  - 实现基本的 Add、GetAfter、RemoveBefore 方法（非原子版本）
  - 序号使用 int64 单调递增

  **Must NOT do**:
  - 不要在此文件实现数据库操作（放在 SessionManager）
  - 不要添加 TTL/过期逻辑

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 类型定义是纯结构设计，无复杂逻辑
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 2)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 4, 5
  - **Blocked By**: None

  **References**:
  - `server/internal/session/model.go:13-36` - JSONB 类型定义模式
  - `server/internal/domain/entity/event.go:23-26` - Event 结构参考
  - `server/internal/domain/entity/event.go:90-95` - AsyncCompletionPendingData 当前结构

  **Acceptance Criteria**:
  - [ ] `QueuedEvent` 结构体包含：Seq(int64), Type(string), Timestamp(time.Time), Data(any)
  - [ ] `EventQueue` 结构体包含：Events([]QueuedEvent), nextSeq(int64)
  - [ ] Add() 方法返回新事件的序号
  - [ ] GetAfter(seq) 返回序号大于 seq 的所有事件
  - [ ] RemoveBefore(seq) 删除序号小于等于 seq 的所有事件

  **QA Scenarios**:
  ```
  Scenario: EventQueue Add and Get
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/session/... -v -run TestEventQueue_AddAndGet
    Expected Result: PASS, events returned in FIFO order
    Evidence: .sisyphus/evidence/task-1-add-get.log
  ```

  **Commit**: NO (groups with Task 2, 3)

---

- [ ] 2. **事件类型注册机制**

  **What to do**:
  - 在 `internal/domain/entity/event.go` 添加 `EventRegistry` 接口
  - 实现简单的类型注册：通过 map 存储类型名到验证函数
  - 注册 `AsyncCompletionPending` 类型
  - 提供类型验证方法 `ValidateEventType(typeName string) bool`

  **Must NOT do**:
  - 不要实现复杂的 schema 验证（保持简单）
  - 不要引入外部依赖

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单的类型注册实现
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 1, 3)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 4
  - **Blocked By**: None

  **References**:
  - `server/internal/domain/entity/event.go:4-20` - 现有 EventType 常量定义
  - `server/internal/domain/entity/event.go:90-95` - AsyncCompletionPendingData

  **Acceptance Criteria**:
  - [ ] `EventRegistry` 结构体定义
  - [ ] `RegisterEventType(name, validator)` 方法
  - [ ] `ValidateEventType(name)` 方法
  - [ ] 默认注册 `async_completion_pending` 类型
  - [ ] 验证函数检查 Data 是否为 AsyncCompletionPendingData 类型

  **QA Scenarios**:
  ```
  Scenario: Event Registry Validation
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/domain/entity/... -v -run TestEventRegistry
    Expected Result: PASS, registered type validated, unknown type rejected
    Evidence: .sisyphus/evidence/task-2-registry.log
  ```

  **Commit**: NO (groups with Task 1, 3)

---

- [ ] 3. **EventQueue 单元测试**

  **What to do**:
  - 在 `internal/session/eventqueue_test.go` 编写测试
  - 测试用例：空队列、添加事件、序号递增、GetAfter 边界、RemoveBefore 边界
  - 使用 `testify/assert` 断言

  **Must NOT do**:
  - 不要测试数据库操作（留在 Task 6）
  - 不要跳过边界条件测试

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准单元测试编写
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 1, 2)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 6
  - **Blocked By**: Task 1

  **References**:
  - `server/internal/session/manager_test.go` - 现有测试模式参考
  - `server/internal/tool/manager_test.go` - testify 使用模式

  **Acceptance Criteria**:
  - [ ] TestEventQueue_Empty
  - [ ] TestEventQueue_Add_IncrementsSequence
  - [ ] TestEventQueue_GetAfter_NoMatch
  - [ ] TestEventQueue_GetAfter_PartialMatch
  - [ ] TestEventQueue_GetAfter_AllMatch
  - [ ] TestEventQueue_RemoveBefore_Partial
  - [ ] TestEventQueue_RemoveBefore_All

  **QA Scenarios**:
  ```
  Scenario: EventQueue Tests Pass
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/session/... -v -run TestEventQueue
    Expected Result: 7 tests PASS
    Evidence: .sisyphus/evidence/task-3-tests.log
  ```

  **Commit**: YES
  - Message: `feat(session): add event queue types with sequence numbers`
  - Files: `internal/session/eventqueue.go`, `internal/session/eventqueue_test.go`
  - Pre-commit: `go test ./internal/session/... -v -run TestEventQueue`

---

- [ ] 4. **AddEvent 方法（带序号）**

  **What to do**:
  - 在 `internal/session/manager.go` 添加 `AddAsyncEvent(chatCtx, eventType string, data any) error`
  - 读取 Session.Metadata，反序列化为 EventQueue
  - 调用 EventQueue.Add() 添加事件
  - 序列化回 Metadata，更新数据库
  - **注意**：此版本不处理并发（Task 5 解决）

  **Must NOT do**:
  - 不要创建新表
  - 不要添加 TTL 逻辑
  - 不要修改现有 Create/Get/Delete 方法签名

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单的 CRUD 操作扩展
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 1, 2)
  - **Parallel Group**: Sequential (Wave 2 first)
  - **Blocks**: Task 5, 6
  - **Blocked By**: Task 1, 2

  **References**:
  - `server/internal/session/manager.go` - 现有 SessionManager 实现
  - `server/internal/session/manager.go:132-153` - 现有 AddAsyncCompletionPending 实现（需要重构）
  - `server/internal/session/model.go:91` - Metadata JSONB 字段

  **Acceptance Criteria**:
  - [ ] `AddAsyncEvent(chatCtx, eventType, data)` 方法签名
  - [ ] 正确序列化/反序列化 EventQueue
  - [ ] 序号正确递增
  - [ ] 返回错误时 session 不变

  **QA Scenarios**:
  ```
  Scenario: Add Event to Queue
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/session/... -v -run TestSessionManager_AddAsyncEvent
    Expected Result: PASS, event persisted to metadata
    Evidence: .sisyphus/evidence/task-4-add-event.log
  ```

  **Commit**: NO (groups with Task 5, 6)

---

- [ ] 5. **GetAndConsumeEvents 方法（原子操作）**

  **What to do**:
  - 在 `internal/session/manager.go` 添加 `GetAndConsumeEvents(chatCtx, sinceSeq int64) ([]QueuedEvent, error)`
  - **关键**：使用 PostgreSQL 事务 + `SELECT FOR UPDATE` 保证原子性
  - 模式：`db.Transaction(func(tx) { SELECT ... FOR UPDATE; UPDATE ... })`
  - 返回序号 > sinceSeq 的所有事件，同时删除这些事件
  - 如果 sinceSeq 为 0，返回并删除所有事件

  **Must NOT do**:
  - 不要使用 read-modify-write 无锁模式（会导致竞态）
  - 不要返回未删除的事件（违反消费即删除原则）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 需要深入理解 GORM 事务和 PostgreSQL 行级锁
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 1, 4)
  - **Parallel Group**: Sequential (Wave 2 second)
  - **Blocks**: Task 6, 7
  - **Blocked By**: Task 1, 4

  **References**:
  - `server/internal/session/manager.go` - 现有 SessionManager
  - GORM 事务文档：`https://gorm.io/docs/transactions.html`
  - PostgreSQL SELECT FOR UPDATE：行级锁模式

  **Acceptance Criteria**:
  - [ ] 使用 `db.Transaction()` 包装整个操作
  - [ ] 使用 `SELECT ... FOR UPDATE` 锁定行
  - [ ] 原子性地读取和删除事件
  - [ ] sinceSeq=0 返回所有事件
  - [ ] 返回的事件按序号升序排列

  **QA Scenarios**:
  ```
  Scenario: Atomic Consume Events
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/session/... -v -run TestSessionManager_GetAndConsumeEvents
    Expected Result: PASS, events returned and deleted atomically
    Evidence: .sisyphus/evidence/task-5-consume.log

  Scenario: Concurrent Pollers No Duplication
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/session/... -v -run TestSessionManager_ConcurrentPollers
    Expected Result: PASS, each event delivered exactly once
    Evidence: .sisyphus/evidence/task-5-concurrent.log
  ```

  **Commit**: NO (groups with Task 4, 6)

---

- [ ] 6. **SessionManager 测试（含并发）**

  **What to do**:
  - 在 `internal/session/manager_test.go` 添加测试
  - 测试用例：AddEvent、GetAndConsumeEvents 空队列、GetAndConsumeEvents 有事件、
  - **关键**：并发测试 - 多个 goroutine 同时调用 GetAndConsumeEvents，验证无丢失/重复
  - 使用 `sync.WaitGroup` 协调并发

  **Must NOT do**:
  - 不要跳过并发测试（这是验证原子性的关键）
  - 不要使用 mock 数据库（使用真实 PostgreSQL）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 并发测试需要仔细设计，验证竞态条件
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 4, 5)
  - **Parallel Group**: Sequential (Wave 2 last)
  - **Blocks**: Task 8
  - **Blocked By**: Task 4, 5, 3

  **References**:
  - `server/internal/session/manager_test.go` - 现有测试模式
  - `server/internal/testutil/chat_context.go` - MockChatContext

  **Acceptance Criteria**:
  - [ ] TestSessionManager_AddAsyncEvent
  - [ ] TestSessionManager_GetAndConsumeEvents_Empty
  - [ ] TestSessionManager_GetAndConsumeEvents_All
  - [ ] TestSessionManager_GetAndConsumeEvents_WithCursor
  - [ ] TestSessionManager_ConcurrentPollers_NoDuplication (10 goroutines)
  - [ ] TestSessionManager_ConcurrentPollers_NoLoss (验证总事件数)

  **QA Scenarios**:
  ```
  Scenario: All SessionManager Tests Pass
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/session/... -v -run TestSessionManager
    Expected Result: 6 tests PASS, no race detected
    Evidence: .sisyphus/evidence/task-6-manager-tests.log
  ```

  **Commit**: YES
  - Message: `feat(session): add atomic GetAndConsumeEvents method`
  - Files: `internal/session/manager.go`, `internal/session/manager_test.go`
  - Pre-commit: `go test ./internal/session/... -v`

---

- [ ] 7. **GetSessionUpdates 改造**

  **What to do**:
  - 修改 `internal/api/handlers.go` 的 `GetSessionUpdates` 方法
  - 解析 `since` 参数（转换为 int64 序号）
  - 调用 `sessionMgr.GetAndConsumeEvents(chatCtx, sinceSeq)`
  - 保持响应格式：`{has_updates: bool, events: []}`
  - 处理无效 cursor 格式（返回 400）
  - 处理 session 不存在（返回 404）

  **Must NOT do**:
  - 不要改变响应格式结构
  - 不要添加分页参数（超出当前需求）
  - 不要修改 Chat handler

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单的 API handler 修改
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 5)
  - **Parallel Group**: Sequential (Wave 3 first)
  - **Blocks**: Task 8
  - **Blocked By**: Task 5

  **References**:
  - `server/internal/api/handlers.go:270-300` - 当前 GetSessionUpdates 实现
  - `server/internal/api/handlers.go:120-139` - GetSession 错误处理模式

  **Acceptance Criteria**:
  - [ ] `since` 参数解析为 int64（默认 0）
  - [ ] 调用 GetAndConsumeEvents
  - [ ] 响应格式：`{has_updates: bool, events: []}`
  - [ ] 无效 cursor 返回 400 `{error: "invalid cursor format"}`
  - [ ] session 不存在返回 404 `{error: "session not found"}`

  **QA Scenarios**:
  ```
  Scenario: Get Updates API No Cursor
    Tool: Bash (curl)
    Steps:
      1. Create session: curl -X POST http://localhost:8080/api/sessions
      2. Add event via test or internal call
      3. curl http://localhost:8080/api/sessions/{id}/updates
    Expected Result: 200, {has_updates: true, events: [...]}
    Evidence: .sisyphus/evidence/task-7-api-nocursor.log

  Scenario: Get Updates API Invalid Cursor
    Tool: Bash (curl)
    Steps:
      1. curl http://localhost:8080/api/sessions/{id}/updates?since=invalid
    Expected Result: 400, {error: "invalid cursor format"}
    Evidence: .sisyphus/evidence/task-7-api-invalid.log
  ```

  **Commit**: NO (groups with Task 8, 9)

---

- [ ] 8. **Handler 测试**

  **What to do**:
  - 在 `internal/api/handlers_test.go` 添加测试
  - 测试用例：空队列、有事件、带 cursor、无效 cursor、session 不存在
  - 验证响应格式和事件消费

  **Must NOT do**:
  - 不要跳过错误场景测试
  - 不要使用硬编码 session ID

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: API 测试需要完整覆盖各种场景
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 6, 7)
  - **Parallel Group**: Sequential (Wave 3 second)
  - **Blocks**: Task 9
  - **Blocked By**: Task 6, 7

  **References**:
  - `server/internal/session/manager_test.go` - 测试模式参考
  - `server/internal/api/handlers.go:270-300` - Handler 实现

  **Acceptance Criteria**:
  - [ ] TestGetSessionUpdates_EmptyQueue
  - [ ] TestGetSessionUpdates_HasEvents
  - [ ] TestGetSessionUpdates_WithCursor
  - [ ] TestGetSessionUpdates_InvalidCursor
  - [ ] TestGetSessionUpdates_SessionNotFound
  - [ ] TestGetSessionUpdates_EventsConsumedAfterRead

  **QA Scenarios**:
  ```
  Scenario: All Handler Tests Pass
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/api/... -v -run TestGetSessionUpdates
    Expected Result: 6 tests PASS
    Evidence: .sisyphus/evidence/task-8-handler-tests.log
  ```

  **Commit**: NO (groups with Task 7, 9)

---

- [ ] 9. **集成验证**

  **What to do**:
  - 运行完整测试套件验证所有功能
  - 验证异步工具完成事件正确写入队列
  - 端到端测试：添加事件 → 轮询获取 → 验证删除

  **Must NOT do**:
  - 不要跳过集成测试
  - 不要忽略测试失败

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 集成验证需要检查各模块协同工作
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 8)
  - **Parallel Group**: Sequential (Wave 3 last)
  - **Blocks**: F1-F4
  - **Blocked By**: Task 8

  **References**:
  - `server/internal/integration_test.go` - 现有集成测试模式

  **Acceptance Criteria**:
  - [ ] `go test ./... -v` 全部通过
  - [ ] 异步工具完成事件写入队列验证
  - [ ] 端到端轮询验证

  **QA Scenarios**:
  ```
  Scenario: Full Test Suite Pass
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./... -v
    Expected Result: All tests PASS, no failures
    Evidence: .sisyphus/evidence/task-9-full-suite.log

  Scenario: Integration Test Async Event
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/... -v -run TestIntegration_AsyncEventPolling
    Expected Result: PASS, async event persisted and retrievable
    Evidence: .sisyphus/evidence/task-9-integration.log
  ```

  **Commit**: YES
  - Message: `feat(api): update GetSessionUpdates with atomic consume`
  - Files: `internal/api/handlers.go`, `internal/api/handlers_test.go`
  - Pre-commit: `go test ./... -v`

---

## Final Verification Wave

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Verify all "Must Have" implemented, all "Must NOT Have" absent.

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...`, `go test ./...`, check for race patterns.

- [ ] F3. **Concurrency Stress Test** — `unspecified-high`
  Run concurrent poller tests with 10+ goroutines, verify no loss/duplication.

- [ ] F4. **Scope Fidelity Check** — `deep`
  Verify no scope creep, API contract preserved.

---

## Commit Strategy

> **Note**: Commits 4 and 5 are merged into existing tasks (Task 6 includes concurrency tests).

- **Commit 1**: `feat(session): add event queue types with sequence numbers`
- **Commit 2**: `feat(session): add atomic GetAndConsumeEvents method`
- **Commit 3**: `feat(api): update GetSessionUpdates with atomic consume`

---

## Success Criteria

### Verification Commands
```bash
cd server && go test ./internal/session/... -v -run TestEventQueue
cd server && go test ./internal/api/... -v -run TestGetSessionUpdates
cd server && go test ./... -v
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass
- [ ] Concurrency tests verify atomicity
- [ ] API response format matches `{has_updates: bool, events: []}`