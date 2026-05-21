# HITL (Human-in-the-Loop) 实现计划

## TL;DR

> **Quick Summary**: 为 CopCon Agent 实现"人在回路"能力——工具可在执行中通过 `chatCtx.RequestInput()` 阻塞等待人类输入（审批或问答），人类通过 POST /resume 提交响应后工具继续执行。同时将 ChatContext 生命周期与 HTTP 请求解耦，修复 Emit 并发安全。
> 
> **Deliverables**:
> - ChatContext 独立 lifecycle context + Emit mutex
> - RequestInput / ResolveInput / PendingInputs 方法
> - POST /sessions/:id/resume + POST /sessions/:id/stop 端点
> - PersistedPart 增加 Interrupt 字段
> - 前端 ToolCallPart 扩展 + HumanInteraction 组件
> - 示例 HITL 工具（审批型 + 问答型）
> 
> **Estimated Effort**: Medium
> **Parallel Execution**: YES - 4 waves
> **Critical Path**: Task 1 → Task 3 → Task 5 → Task 7 → Task 8 → Task 9 → Task 10

---

## Context

### Original Request
调研并实现"人在回路"功能，支持工具审批和问答两种交互方式。

### Interview Summary
**Key Discussions**:
- 行业调研：OpenAI requires_action、LangGraph interrupt、MCP elicitation、Coze InterruptEvent 等模式
- 简化为两种交互类型（审批 + 问答），使用 SSE+POST 模式
- 选择内存阻塞（chatCtx.RequestInput）而非状态序列化
- ChatContext lifecycle context 与 HTTP 请求解耦
- SSE 循环监听 HTTP context 仅用于 subscriber 清理，不影响 agent 生命周期
- Stop 按钮需要新端点，abort fetch 不再杀死 agent

**Research Findings**:
- 完整架构设计文档：docs/hitl-architecture-design.md
- Emit 存在并发安全 bug（executeConcurrent 中多 goroutine 竞争）
- PersistedPart 缺少 Interrupt 字段，影响刷新重连

### Metis Review
**Identified Gaps** (addressed):
- Emit 并发安全：加 mutex（修已有 bug + 支持 HITL）
- HITL 并发模式：MVP 限制为 sync-only 工具可触发 HITL
- Interrupt 持久化：PersistedPart 加 Interrupt JSONB 字段
- RequestInput 超时：由调用方控制，不硬编码

---

## Work Objectives

### Core Objective
实现 HITL 内存阻塞机制（RequestInput/ResolveInput），将 ChatContext 生命周期与 HTTP 解耦，修复 Emit 并发安全，并在前端支持交互 UI。

### Concrete Deliverables
- `server/internal/domain/iface/chat.go` — 新增 HITL 类型和接口方法
- `server/internal/domain/entity/event.go` — PartUpdateData 扩展
- `server/internal/session/model.go` — PersistedPart 加 Interrupt 字段
- `server/internal/api/handlers.go` — resume + stop 端点 + SSE 循环改造
- `server/internal/api/routes.go` — 新路由
- `packages/ui/src/api/types.ts` — ToolCallPart 扩展
- `packages/ui/src/providers/CopConChatProvider.ts` — 事件处理扩展
- `packages/ui/src/components/HumanInteraction/` — 交互组件
- `packages/ui/src/api/agentClient.ts` — resume + stop 方法

### Definition of Done
- [ ] 工具调 `chatCtx.RequestInput()` 后阻塞，POST /resume 后继续执行
- [ ] SSE 断开后 agent 不死，重连后正常推送后续事件
- [ ] 前端显示审批/问答 UI，提交后工具继续
- [ ] 刷新页面后从历史消息恢复 HITL UI
- [ ] Stop 按钮能终止 agent
- [ ] Emit 并发安全（go test -race 通过）

### Must Have
- chatCtx.RequestInput / ResolveInput 内存阻塞机制
- ChatContext 独立 lifecycle context
- Emit 并发安全（mutex）
- POST /resume + POST /stop 端点
- SSE 循环 HTTP context 监听
- PersistedPart Interrupt 字段
- 前端 HumanInteraction 组件
- 审批型 + 问答型示例工具

### Must NOT Have (Guardrails)
- 不做 OAuth/URL Mode 鉴权流
- 不做 MCP Sampling
- 不做 DB/Redis 持久化 snapshot
- 不做 WebSocket 双向通道
- 不在 ToolResult 上加 Interrupt 字段（用 chatCtx.RequestInput 替代）
- 不在 RequestInput 内部硬编码超时
- 不允许 HITL 在 async 模式工具中触发（MVP 限制）
- 不允许 LLM 上下文中出现 interrupt_id 或人类原始凭证

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed.

### Test Decision
- **Infrastructure exists**: YES (Go testify + vitest)
- **Automated tests**: YES (tests-after)
- **Framework**: Go: testify/assert + testify/require; TS: vitest

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.omo/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Backend API**: Bash (curl) — Send requests, assert status + response
- **Backend Unit**: Bash (go test) — Run unit tests
- **Frontend**: Playwright — Navigate, interact, assert DOM, screenshot

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Foundation — 3 tasks, no dependencies):
├── Task 1: ChatContext lifecycle context + Emit mutex [quick]
├── Task 2: HITL 类型定义 (InputRequest/InputResponse/InterruptType) [quick]
└── Task 3: PersistedPart + PartUpdateData 扩展 [quick]

Wave 2 (Core HITL mechanism — 2 tasks, depend on Wave 1):
├── Task 4: ChatContext RequestInput/ResolveInput 实现 (depends: 1, 2) [deep]
└── Task 5: SSE 循环改造 + Stop 端点 (depends: 1) [unspecified-high]

Wave 3 (API + Frontend — 3 tasks, depend on Wave 2):
├── Task 6: POST /resume 端点 (depends: 4, 5) [unspecified-high]
├── Task 7: 前端类型 + Provider 扩展 (depends: 3) [quick]
└── Task 8: 前端 HumanInteraction 组件 + AgentClient (depends: 7) [visual-engineering]

Wave 4 (Integration + Examples — 2 tasks, depend on Wave 3):
├── Task 9: 示例 HITL 工具 (depends: 4, 6) [unspecified-high]
└── Task 10: 端到端集成验证 (depends: 9, 8) [deep]

Wave FINAL (4 parallel reviews):
├── F1: Plan compliance audit (oracle)
├── F2: Code quality review (unspecified-high)
├── F3: Real manual QA (unspecified-high)
└── F4: Scope fidelity check (deep)

Critical Path: 1 → 4 → 6 → 9 → 10 → F1-F4
Parallel Speedup: ~50% faster than sequential
Max Concurrent: 3 (Waves 1 & 3)
```

### Dependency Matrix

| Task | Depends On | Blocks |
|------|-----------|--------|
| 1 | - | 4, 5 |
| 2 | - | 4 |
| 3 | - | 7 |
| 4 | 1, 2 | 6, 9 |
| 5 | 1 | 6 |
| 6 | 4, 5 | 9 |
| 7 | 3 | 8 |
| 8 | 7 | 10 |
| 9 | 4, 6 | 10 |
| 10 | 8, 9 | F1-F4 |

### Agent Dispatch Summary

- **Wave 1**: T1-T3 → `quick`
- **Wave 2**: T4 → `deep`, T5 → `unspecified-high`
- **Wave 3**: T6 → `unspecified-high`, T7 → `quick`, T8 → `visual-engineering`
- **Wave 4**: T9 → `unspecified-high`, T10 → `deep`
- **FINAL**: F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

- [x] 1. ChatContext lifecycle context + Emit mutex

  **What to do**:
  - 将 `NewChatContext` 的 `ctx` 参数改为 `context.Background()`，增加 `lifecycleCancel context.CancelFunc`
  - 在 `Close()` 中调用 `lifecycleCancel()` 取消 lifecycle context
  - 给 `Emit` 加 `sync.Mutex` 保护（修复 executeConcurrent 中多 goroutine 并发 Emit 的已有 bug）
  - 更新 `NewChatContext` 的所有调用方（handlers.go, tests）
  - 写单元测试验证 Emit 并发安全（`go test -race`）

  **Must NOT do**:
  - 不改变 `chatCtx.Context()` 的返回类型
  - 不修改 `toolMgr.Execute` 或 `handleToolCalls` 的逻辑

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3)
  - **Blocks**: Tasks 4, 5
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/domain/iface/chat.go:34-43` — ChatContext struct 定义，需加 emitMu 和 lifecycleCancel 字段
  - `server/internal/domain/iface/chat.go:74-77` — Emit 方法，需加 mutex
  - `server/internal/domain/iface/chat.go:81-87` — Close 方法，需加 lifecycleCancel 调用
  - `server/internal/domain/iface/chat.go:140-148` — NewChatContext，需改 ctx 初始化
  - `server/internal/api/handlers.go:398` — NewChatContext 调用方
  - `server/internal/agent/engine_tools.go:380-509` — executeConcurrent 中并发 Emit 的已有 bug 位置

  **Acceptance Criteria**:
  - [ ] `go test -race ./internal/domain/iface/...` 通过
  - [ ] `go vet ./internal/domain/iface/...` 通过
  - [ ] ChatContext.Context() 返回的 context 只被 Close() 取消

  **QA Scenarios**:
  ```
  Scenario: Emit 并发安全
    Tool: Bash
    Steps:
      1. go test -race -run TestConcurrentEmit ./internal/domain/iface/...
    Expected Result: PASS, no race detected
    Evidence: .omo/evidence/task-1-emit-race.txt

  Scenario: Lifecycle context 不随 HTTP 断开取消
    Tool: Bash
    Steps:
      1. go test -run TestLifecycleContext ./internal/domain/iface/...
    Expected Result: PASS, context 仅在 Close() 后取消
    Evidence: .omo/evidence/task-1-lifecycle-ctx.txt
  ```

  **Commit**: YES
  - Message: `refactor(chat): decouple lifecycle context from HTTP request + add Emit mutex`
  - Files: `server/internal/domain/iface/chat.go`, `server/internal/domain/iface/chat_test.go`
  - Pre-commit: `go test -race ./internal/domain/iface/...`

---

- [x] 2. HITL 类型定义

  **What to do**:
  - 在 `server/internal/domain/iface/chat.go` 中定义 `InterruptType`、`InputRequest`、`InputResponse` 类型
  - 在 `ChatContextInterface` 中声明 `RequestInput`、`ResolveInput`、`PendingInputs` 方法签名
  - 定义 `ErrInterruptNotFound` 错误

  **Must NOT do**:
  - 不实现方法（Task 4 做）
  - 不在 ToolResult 上加 Interrupt 字段

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3)
  - **Blocks**: Task 4
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/domain/iface/chat.go:12-22` — ChatContextInterface 接口定义位置
  - `docs/hitl-architecture-design.md:§5.2` — HITL 类型定义规范

  **Acceptance Criteria**:
  - [ ] `go build ./internal/domain/iface/...` 通过
  - [ ] InterruptType 有 "approval" 和 "question" 两个常量
  - [ ] InputRequest 包含 ID, Type, Message, InputSchema, Summary, ToolName, ToolArgs 字段
  - [ ] InputResponse 包含 Action, Content 字段

  **QA Scenarios**:
  ```
  Scenario: 类型编译通过
    Tool: Bash
    Steps:
      1. go build ./internal/domain/iface/...
    Expected Result: 编译成功
    Evidence: .omo/evidence/task-2-types-compile.txt
  ```

  **Commit**: YES
  - Message: `feat(hitl): add HITL type definitions and interface methods`
  - Files: `server/internal/domain/iface/chat.go`
  - Pre-commit: `go build ./internal/domain/iface/...`

---

- [x] 3. PersistedPart + PartUpdateData 扩展

  **What to do**:
  - 在 `server/internal/session/model.go` 的 `PersistedPart` 中增加 `Interrupt` JSONB 字段（类型为 `map[string]any` 或自定义 `InterruptPayload`）
  - 在 `server/internal/domain/entity/event.go` 的 `PartUpdateData` 中增加 `Interrupt` 字段
  - 在 `server/internal/domain/entity/event.go` 新增 `InterruptEventType` 常量（可选，或复用 PartUpdate）
  - 确保 DB migration 兼容（JSONB 字段，omitempty，无需 migration）

  **Must NOT do**:
  - 不改变 PersistedPart 的现有字段语义
  - 不做 DB migration（JSONB omitempty 自动兼容）

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2)
  - **Blocks**: Task 7
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/session/model.go` — PersistedPart 定义位置
  - `server/internal/domain/entity/event.go:106-116` — PartUpdateData 定义位置

  **Acceptance Criteria**:
  - [ ] `go build ./internal/session/... ./internal/domain/entity/...` 通过
  - [ ] PersistedPart 有 Interrupt 字段且 json tag 含 omitempty
  - [ ] PartUpdateData 有 Interrupt 字段

  **QA Scenarios**:
  ```
  Scenario: 扩展字段编译通过且向后兼容
    Tool: Bash
    Steps:
      1. go build ./internal/session/... ./internal/domain/entity/...
      2. go test ./internal/session/... ./internal/domain/entity/...
    Expected Result: 编译和测试通过
    Evidence: .omo/evidence/task-3-extend-compile.txt
  ```

  **Commit**: YES
  - Message: `feat(hitl): add Interrupt field to PersistedPart and PartUpdateData`
  - Files: `server/internal/session/model.go`, `server/internal/domain/entity/event.go`
  - Pre-commit: `go build ./internal/session/... ./internal/domain/entity/...`

---

- [x] 4. ChatContext RequestInput/ResolveInput 实现

  **What to do**:
  - 在 ChatContext struct 中增加 `interruptMu sync.Mutex`、`interruptChans map[string]chan *InputResponse`、`interruptReqs map[string]*InputRequest` 字段
  - 实现 `RequestInput`：生成 ID → 注册 chan → emit part_update(waiting_for_input) → 阻塞在 select{chan, Closed()}
  - 实现 `ResolveInput`：查找 chan → 写入响应
  - 实现 `PendingInputs`：遍历 interruptReqs 返回列表
  - 实现 Part locator 机制：`SetPartLocator(messageID, stepIndex, partIndex)` / `ClearPartLocator()`，让 RequestInput 能 emit 正确的 part_update
  - 写单元测试：RequestInput 阻塞 → ResolveInput 唤醒 → 返回正确响应
  - 写单元测试：session 关闭时 RequestInput 返回错误
  - 写单元测试：PendingInputs 返回正确列表

  **Must NOT do**:
  - 不在 RequestInput 内部硬编码超时
  - 不做 DB 持久化

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 1, 2)
  - **Parallel Group**: Wave 2 (with Task 5)
  - **Blocks**: Tasks 6, 9
  - **Blocked By**: Tasks 1, 2

  **References**:
  **Pattern References**:
  - `server/internal/domain/iface/chat.go` — ChatContext 实现，需加 HITL 字段和方法
  - `docs/hitl-architecture-design.md:§5.3` — RequestInput/ResolveInput 实现规范
  - `server/internal/agent/engine_tools.go:77-95` — executeSync 中 part locator 的上下文（messageID, stepIndex, partIndex 在此可用）

  **Acceptance Criteria**:
  - [ ] `go test -race ./internal/domain/iface/...` 通过
  - [ ] RequestInput 阻塞直到 ResolveInput 被调用
  - [ ] RequestInput 在 session Close 后返回错误
  - [ ] PendingInputs 返回当前等待的中断列表

  **QA Scenarios**:
  ```
  Scenario: RequestInput 阻塞并正常恢复
    Tool: Bash
    Steps:
      1. go test -run TestRequestInput_Resolve ./internal/domain/iface/... -v
    Expected Result: PASS, RequestInput 阻塞后由 ResolveInput 唤醒并返回正确响应
    Evidence: .omo/evidence/task-4-request-resolve.txt

  Scenario: RequestInput 在 session 关闭时返回错误
    Tool: Bash
    Steps:
      1. go test -run TestRequestInput_SessionClose ./internal/domain/iface/... -v
    Expected Result: PASS, RequestInput 返回 "session closed" 错误
    Evidence: .omo/evidence/task-4-request-close.txt
  ```

  **Commit**: YES
  - Message: `feat(hitl): implement RequestInput/ResolveInput/PendingInputs on ChatContext`
  - Files: `server/internal/domain/iface/chat.go`, `server/internal/domain/iface/chat_test.go`
  - Pre-commit: `go test -race ./internal/domain/iface/...`

---

- [x] 5. SSE 循环改造 + Stop 端点

  **What to do**:
  - 改造 `handlers.go` 中 Chat 方法的 SSE 循环：从 `for range sub.Events` 改为 `select { case event := <-sub.Events; case <-c.Request.Context().Done() }`
  - 新增 `StopSession` handler：从 `sessionAgentStore` 获取 ChatContext → 调 `chatCtx.Close()`
  - 在 `SetupRoutes` 中注册 `POST /:sessionId/stop`
  - 更新前端 Stop 按钮逻辑：先调 stop endpoint，再 abort fetch
  - 写测试：SSE 断开后 agent 继续运行
  - 写测试：Stop 端点终止 agent

  **Must NOT do**:
  - 不改变 agent goroutine 的启动逻辑
  - 不修改 ChatContext.Close() 的行为

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (depends only on Task 1)
  - **Parallel Group**: Wave 2 (with Task 4)
  - **Blocks**: Task 6
  - **Blocked By**: Task 1

  **References**:
  **Pattern References**:
  - `server/internal/api/handlers.go:422-426` — SSE 循环，需改为 select
  - `server/internal/api/handlers.go:54-61` — Handler struct，需加或复用 sessionAgentStore
  - `server/internal/api/handlers.go:444-463` — SetupRoutes，需加 stop 路由
  - `docs/hitl-architecture-design.md:§7` — SSE 退出机制规范
  - `docs/hitl-architecture-design.md:§8` — Stop 端点规范

  **Acceptance Criteria**:
  - [ ] SSE 循环在 HTTP context cancel 时退出
  - [ ] SSE 循环在 ringbuf 关闭时退出
  - [ ] POST /sessions/:id/stop 返回 204 并终止 agent
  - [ ] 不存在的 session 返回 404

  **QA Scenarios**:
  ```
  Scenario: Stop 端点终止活跃 agent
    Tool: Bash (curl)
    Steps:
      1. 创建 session
      2. POST /chat 启动 agent
      3. POST /stop
      4. 验证返回 204
    Expected Result: 204 No Content
    Evidence: .omo/evidence/task-5-stop-endpoint.txt

  Scenario: SSE 断开后 agent 不死
    Tool: Bash
    Steps:
      1. go test -run TestSSE_Disconnect_AgentSurvives ./internal/api/... -v
    Expected Result: PASS
    Evidence: .omo/evidence/task-5-sse-disconnect.txt
  ```

  **Commit**: YES
  - Message: `feat(hitl): SSE select loop + POST /stop endpoint`
  - Files: `server/internal/api/handlers.go`, `server/internal/api/routes.go`
  - Pre-commit: `go build ./internal/api/...`

---

- [x] 6. POST /resume 端点

  **What to do**:
  - 新增 `ResumeSession` handler：解析 request body（interrupt_id, action, content）→ 从 sessionAgentStore 获取 ChatContext → 调 `chatCtx.ResolveInput` → 返回 200
  - 处理错误：interrupt_id 不存在返回 404，session 无活跃 agent 返回 409
  - 在 `SetupRoutes` 中注册 `POST /:sessionId/resume`
  - 写测试：resume 正常唤醒、interrupt_id 不存在、session 无活跃 agent

  **Must NOT do**:
  - 不在 resume handler 中重启 agent goroutine（agent 本身就在阻塞等待）
  - 不做 resume 后的 SSE 流管理（现有 SSE 自动推送后续事件）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 4, 5)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 9
  - **Blocked By**: Tasks 4, 5

  **References**:
  **Pattern References**:
  - `server/internal/api/handlers.go:54-61` — Handler struct
  - `server/internal/api/handlers.go:444-463` — SetupRoutes
  - `docs/hitl-architecture-design.md:§11.1` — Resume 端点 API 规范

  **Acceptance Criteria**:
  - [ ] POST /resume 正确唤醒阻塞的 RequestInput
  - [ ] interrupt_id 不存在返回 404
  - [ ] session 无活跃 agent 返回 409
  - [ ] 审批型 action=approve/decline 正确传递
  - [ ] 问答型 action=submit + content 正确传递

  **QA Scenarios**:
  ```
  Scenario: Resume 正常唤醒
    Tool: Bash (curl)
    Steps:
      1. 创建 session + 启动 agent（触发 HITL 工具）
      2. 等待 waiting_for_input 事件
      3. POST /resume {interrupt_id, action: "approve"}
      4. 验证返回 200
    Expected Result: 200 OK
    Evidence: .omo/evidence/task-6-resume-ok.txt

  Scenario: Resume 不存在的 interrupt_id
    Tool: Bash (curl)
    Steps:
      1. POST /resume {interrupt_id: "nonexistent", action: "approve"}
    Expected Result: 404 Not Found
    Evidence: .omo/evidence/task-6-resume-404.txt
  ```

  **Commit**: YES
  - Message: `feat(hitl): add POST /sessions/:id/resume endpoint`
  - Files: `server/internal/api/handlers.go`, `server/internal/api/routes.go`
  - Pre-commit: `go build ./internal/api/...`

---

- [x] 7. 前端类型 + Provider 扩展

  **What to do**:
  - `types.ts`：ToolCallPart.state 增加 `'waiting_for_input'`，新增 `InterruptPayload` interface，ToolCallPart 增加 `interrupt?: InterruptPayload`
  - `CopConChatProvider.ts`：part_update 分支中 ToolCallPart state 映射增加 `waiting_for_input`，透传 `interrupt` payload
  - `agentClient.ts`：新增 `resume()` 和 `stop()` 方法

  **Must NOT do**:
  - 不改变现有 Part 类型的语义
  - 不在 Provider 中处理 HITL 交互逻辑（只透传数据）

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (depends only on Task 3)
  - **Parallel Group**: Wave 3 (with Tasks 6, 8)
  - **Blocks**: Task 8
  - **Blocked By**: Task 3

  **References**:
  **Pattern References**:
  - `packages/ui/src/api/types.ts:81-103` — ToolCallPart 定义，需加 state 和 interrupt
  - `packages/ui/src/providers/CopConChatProvider.ts:243-260` — part_update 中 ToolCallPart state 映射
  - `packages/ui/src/api/agentClient.ts` — AgentClient，需加 resume/stop 方法

  **Acceptance Criteria**:
  - [ ] TypeScript 编译通过（pnpm build）
  - [ ] ToolCallPart state 包含 'waiting_for_input'
  - [ ] InterruptPayload 包含 interruptId, interruptType, message, summary, inputSchema

  **QA Scenarios**:
  ```
  Scenario: 前端类型编译通过
    Tool: Bash
    Steps:
      1. cd packages/ui && pnpm build
    Expected Result: 构建成功
    Evidence: .omo/evidence/task-7-frontend-types.txt
  ```

  **Commit**: YES
  - Message: `feat(hitl): extend frontend types and provider for HITL`
  - Files: `packages/ui/src/api/types.ts`, `packages/ui/src/providers/CopConChatProvider.ts`, `packages/ui/src/api/agentClient.ts`
  - Pre-commit: `cd packages/ui && pnpm build`

---

- [x] 8. 前端 HumanInteraction 组件

  **What to do**:
  - 新建 `packages/ui/src/components/HumanInteraction/index.tsx`
  - 审批型 UI：显示 message + summary + Approve/Decline 按钮
  - 问答型 UI：显示 message + 根据 inputSchema 渲染表单 + Submit/Cancel 按钮
  - 点击按钮后调 `agentClient.resume(sessionId, interruptId, action, content)`
  - 在 `StepContent`（demo/App.tsx）中识别 `waiting_for_input` 状态，渲染 HumanInteraction
  - 更新 demo 的 Sender onCancel：先调 stop endpoint，再 abort fetch
  - 处理刷新恢复：从历史消息中识别 `waiting_for_input` 状态的 ToolCallPart

  **Must NOT do**:
  - 不在 HumanInteraction 中手动更新消息 state（SSE 自动推送后续事件）
  - 不做复杂的 JSON Schema 表单渲染引擎（MVP 只支持简单类型：string, number, boolean, enum）

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 7)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 10
  - **Blocked By**: Task 7

  **References**:
  **Pattern References**:
  - `packages/demo/src/App.tsx:35-72` — StepContent 组件，需加 waiting_for_input 分支
  - `packages/demo/src/App.tsx:367-378` — Sender onCancel，需改 Stop 逻辑
  - `packages/ui/src/api/agentClient.ts` — resume/stop 方法（Task 7 新增）
  - `docs/hitl-architecture-design.md:§12.4` — HumanInteraction 组件规范

  **Acceptance Criteria**:
  - [ ] 审批型：显示 Approve/Decline 按钮，点击后调 resume API
  - [ ] 问答型：显示表单，提交后调 resume API
  - [ ] 刷新后从历史消息恢复 HITL UI
  - [ ] demo 构建通过

  **QA Scenarios**:
  ```
  Scenario: 审批型 UI 渲染和交互
    Tool: Playwright
    Steps:
      1. 打开 demo 页面
      2. 发送触发审批工具的消息
      3. 等待 waiting_for_input 状态出现
      4. 验证 Approve/Decline 按钮可见
      5. 点击 Approve
      6. 验证工具继续执行
    Expected Result: 工具审批后继续执行
    Evidence: .omo/evidence/task-8-approval-ui.png

  Scenario: 问答型 UI 渲染和交互
    Tool: Playwright
    Steps:
      1. 打开 demo 页面
      2. 发送触发问答工具的消息
      3. 等待表单出现
      4. 填写表单
      5. 点击 Submit
      6. 验证工具用人类输入继续执行
    Expected Result: 工具用人类输入继续
    Evidence: .omo/evidence/task-8-question-ui.png
  ```

  **Commit**: YES
  - Message: `feat(hitl): add HumanInteraction component and integrate into StepContent`
  - Files: `packages/ui/src/components/HumanInteraction/index.tsx`, `packages/demo/src/App.tsx`
  - Pre-commit: `cd packages/ui && pnpm build && cd ../../packages/demo && pnpm build`

---

- [x] 9. 示例 HITL 工具

  **What to do**:
  - 实现审批型示例工具 `confirm_action`：接收 action 描述，调 RequestInput 请求审批，approve 后返回成功，decline 后返回拒绝
  - 实现问答型示例工具 `ask_user`：接收 question + inputSchema，调 RequestInput 请求输入，submit 后返回用户输入
  - 在 agent 注册中注册这两个工具
  - 在 engine_tools.go 的 executeSync 中设置 PartLocator（在 toolMgr.Execute 前后）

  **Must NOT do**:
  - 不修改现有工具
  - 不让 HITL 在 async 模式下触发

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 4, 6)
  - **Parallel Group**: Wave 4
  - **Blocks**: Task 10
  - **Blocked By**: Tasks 4, 6

  **References**:
  **Pattern References**:
  - `server/internal/tools/` — 现有工具实现目录
  - `server/internal/agent/engine_tools.go:77-147` — executeSync，需加 PartLocator 设置
  - `docs/hitl-architecture-design.md:§9` — 工具侧设计规范

  **Acceptance Criteria**:
  - [ ] confirm_action 工具调 RequestInput 后阻塞，approve 后返回成功
  - [ ] ask_user 工具调 RequestInput 后阻塞，submit 后返回用户输入
  - [ ] 两个工具注册到 agent 的 ToolManager
  - [ ] executeSync 中 PartLocator 正确设置

  **QA Scenarios**:
  ```
  Scenario: 审批型工具端到端
    Tool: Bash (curl)
    Steps:
      1. 创建 session
      2. POST /chat 发送 "请确认此操作"
      3. SSE 流中出现 waiting_for_input 事件
      4. POST /resume {action: "approve"}
      5. SSE 流中出现 part_update(state: "complete")
    Expected Result: 工具审批后完成
    Evidence: .omo/evidence/task-9-approval-e2e.txt

  Scenario: 问答型工具端到端
    Tool: Bash (curl)
    Steps:
      1. 创建 session
      2. POST /chat 发送 "请询问用户"
      3. SSE 流中出现 waiting_for_input 事件
      4. POST /resume {action: "submit", content: {answer: "42"}}
      5. SSE 流中出现 part_update(state: "complete")
    Expected Result: 工具用用户输入完成
    Evidence: .omo/evidence/task-9-question-e2e.txt
  ```

  **Commit**: YES
  - Message: `feat(hitl): add confirm_action and ask_user example HITL tools`
  - Files: `server/internal/tools/hitl_tools.go`, `server/internal/agent/engine_tools.go`
  - Pre-commit: `go build ./internal/tools/... ./internal/agent/...`

---

- [x] 10. 端到端集成验证

  **What to do**:
  - 验证完整 HITL 流程：agent 调 HITL 工具 → SSE 推送 waiting_for_input → 前端渲染交互 UI → 用户操作 → POST /resume → agent 继续 → SSE 推送后续事件
  - 验证 SSE 断线重连：HITL 等待期间断开 SSE → 重连 → 恢复交互 UI
  - 验证 Stop 按钮：HITL 等待期间点 Stop → agent 终止
  - 验证刷新恢复：HITL 等待期间刷新页面 → 从历史消息恢复交互 UI
  - 验证拒绝审批：decline → 工具返回错误 → LLM 重新规划
  - 验证 Emit 并发安全：go test -race 全量通过

  **Must NOT do**:
  - 不新增功能，只验证

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 8, 9)
  - **Parallel Group**: Wave 4
  - **Blocks**: F1-F4
  - **Blocked By**: Tasks 8, 9

  **References**:
  **Pattern References**:
  - `docs/hitl-architecture-design.md:§6` — SSE + POST 交互流程
  - `docs/hitl-architecture-design.md:§6.3` — 断线重连

  **Acceptance Criteria**:
  - [ ] 完整 HITL 流程通过
  - [ ] SSE 断线重连通过
  - [ ] Stop 按钮终止 agent
  - [ ] 刷新恢复 HITL UI
  - [ ] 拒绝审批后 LLM 重新规划
  - [ ] go test -race ./... 通过

  **QA Scenarios**:
  ```
  Scenario: 完整 HITL 审批流程
    Tool: Bash (curl)
    Steps:
      1. 创建 session
      2. POST /chat 触发审批工具
      3. 解析 SSE 事件，找到 waiting_for_input + interrupt_id
      4. POST /resume {interrupt_id, action: "approve"}
      5. 验证后续 SSE 事件包含 part_update(state: "complete")
    Expected Result: 完整流程闭环
    Evidence: .omo/evidence/task-10-full-flow.txt

  Scenario: 拒绝审批后 LLM 重新规划
    Tool: Bash (curl)
    Steps:
      1. 创建 session
      2. POST /chat 触发审批工具
      3. POST /resume {interrupt_id, action: "decline"}
      4. 验证 LLM 收到拒绝错误后生成新回复（不重试同一工具）
    Expected Result: LLM 重新规划
    Evidence: .omo/evidence/task-10-decline-replan.txt
  ```

  **Commit**: NO

---

## Final Verification Wave

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists. For each "Must NOT Have": search codebase for forbidden patterns. Check evidence files. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go test -race ./...` + `pnpm build`. Review all changed files for: `as any`/`@ts-ignore`, empty catches, console.log in prod, unused imports. Check AI slop.
  Output: `Build [PASS/FAIL] | Lint [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high` (+ `playwright` skill)
  Start from clean state. Execute EVERY QA scenario from EVERY task. Test cross-task integration. Test edge cases: decline, cancel, SSE disconnect, page refresh. Save to `.omo/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff. Verify 1:1. Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **Task 1**: `refactor(chat): decouple lifecycle context from HTTP request + add Emit mutex` — iface/chat.go, iface/chat_test.go
- **Task 2**: `feat(hitl): add HITL type definitions and interface methods` — iface/chat.go
- **Task 3**: `feat(hitl): add Interrupt field to PersistedPart and PartUpdateData` — session/model.go, entity/event.go
- **Task 4**: `feat(hitl): implement RequestInput/ResolveInput/PendingInputs on ChatContext` — iface/chat.go, iface/chat_test.go
- **Task 5**: `feat(hitl): SSE select loop + POST /stop endpoint` — api/handlers.go, api/routes.go
- **Task 6**: `feat(hitl): add POST /sessions/:id/resume endpoint` — api/handlers.go, api/routes.go
- **Task 7**: `feat(hitl): extend frontend types and provider for HITL` — types.ts, CopConChatProvider.ts, agentClient.ts
- **Task 8**: `feat(hitl): add HumanInteraction component and integrate into StepContent` — HumanInteraction/index.tsx, App.tsx
- **Task 9**: `feat(hitl): add confirm_action and ask_user example HITL tools` — tools/hitl_tools.go, engine_tools.go

---

## Success Criteria

### Verification Commands
```bash
go test -race ./...                    # Expected: PASS, no race conditions
go vet ./...                           # Expected: PASS
cd packages/ui && pnpm build           # Expected: build success
cd packages/demo && pnpm build         # Expected: build success
```

### Final Checklist
- [ ] chatCtx.RequestInput 阻塞 → POST /resume → 工具继续执行
- [ ] SSE 断开后 agent 不死，重连后正常推送
- [ ] 前端显示审批/问答 UI
- [ ] 刷新页面后恢复 HITL UI
- [ ] Stop 按钮终止 agent
- [ ] Emit 并发安全（go test -race 通过）
- [ ] LLM 不知道工具暂停过（上下文中无 interrupt_id）
- [ ] 拒绝审批后 LLM 重新规划
