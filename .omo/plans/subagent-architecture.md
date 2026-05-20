# Subagent 调用 — 执行计划

## TL;DR

> **Quick Summary**: 实现 subagent 委托调用功能，主 agent 通过 `delegate_to` 工具将任务分派给其他 agent，子 agent 在独立子会话中执行并通过 SSE 实时输出，支持断线重连和多客户端连接。
>
> **Deliverables**:
> - ChatContext 升级为 ringbuf 多订阅者事件分发
> - AgentRegistry 从配置驱动改为工厂方法代码注册
> - `delegate_to` 统一委托工具 + 子会话管理
> - Incremental persist 增量持久化
> - 前端重连 + SubagentCard 子 agent 渲染
>
> **Estimated Effort**: Large
> **Parallel Execution**: YES - 8 waves
> **Critical Path**: P0 → P1 → P2 → P3 → P3.5 → P4 → P5

---

## Context

### Original Request
实现 subagent 调用，主 agent 可委托子 agent 执行子任务。方案已定稿于 `docs/subagent-architecture-design.md`。

### Interview Summary
**Key Discussions**:
- Agent 模型：Definition + Engine 分离，Hook 做差异化
- 注册方式：代码注册工厂方法，非 config.yaml 模板驱动
- 事件分发：引入 `github.com/golang-cz/ringbuf`，锁无关环形缓冲区
- 子 Agent 事件：独立 SSE 流，不转发到主 agent
- Subagent 调用：单一 `delegate_to` tool，`agent_id` 当作参数
- 上下文传递：返回 `{ sub_session_id, summary }`，主 agent 按需查询
- 嵌套深度：硬限制 3 层
- SSE 重连：首次和重连统一 handler，计算 `last_event_seq`
- AgentDefinition 新增 `Hooks []Hook` 字段
- 断线保证：ringbuf 撑短断线 + incremental persist 撑长断线 + React state 兜底

**Research Findings** (Metis):
- 7 个高影响风险：ringbuf 兼容性、`execution_mode` 参数碰撞、持久化模式变更、接口缺失、XRequest 阻塞、async 依赖、SessionAgentStore 并发
- P0 ringbuf spike 必须执行（降低存在性风险）
- P1 必须给 `ChatContextInterface` 增加 `Close()`/`Closed()` 方法
- P3 必须处理 `execution_mode` 自动注入碰撞
- P3.5 incremental persist 需要从 INSERT-only 改为 INSERT+UPDATE 模式
- P2.5 前端重连可与 P2 并行
- 建议 defer async mode 到 P5

---

## Work Objectives

### Core Objective
实现主 agent 通过 `delegate_to(agent_id, task)` 委托子 agent 执行子任务，子 agent 在独立子会话中运行，通过独立的 SSE 流实时输出，支持断线重连不丢消息。

### Concrete Deliverables
- `server/internal/domain/iface/chat.go` — ChatContextInterface 增加 Close/Closed/Depth
- `server/internal/chat_context/ringbuf.go` — ringbuf-based ChatContext 实现
- `server/internal/chat_context/store.go` — SessionAgentStore 并发安全实现
- `server/internal/agent/registry.go` — 工厂方法注册接口
- `server/internal/agent/definition.go` — AgentDefinition.Hooks 字段
- `server/internal/agent/engine.go` — hook 组合 + step limit + depth 检查
- `server/internal/tools/delegate.go` — delegate_to 工具实现
- `server/internal/session/model.go` — parent_session_id
- `server/internal/api/handlers.go` — 统一首次/重连 SSE handler
- `packages/ui/src/hooks/useAgentChat.ts` — 重连逻辑
- `packages/ui/src/components/SubagentCard/` — 子 agent 展示组件
- `packages/ui/src/api/agentClient.ts` — reconnect 方法

### Definition of Done
- [ ] `go test ./...` 全部通过（含新增测试）
- [ ] ringbuf spike 验证通过（ringbuf 在 Go 1.26 下正常编译和运行）
- [ ] 前端 `pnpm build` 无报错
- [ ] SSE 重连：断开 → 重连 → 零丢失（10 次测试）
- [ ] 3 层嵌套 subagent 正确执行并限制深度
- [ ] `delegate_to` 在 LLM tool list 中不包含重复的 execution_mode 参数

### Must Have
- ringbuf 引入并正确工作
- ChatContextInterface 增加 Close()/Closed()/Depth()/Subscribe()
- 统一首次/重连 SSE handler
- AgentRegistry 工厂方法注册
- AgentDefinition.Hooks 字段 + hook 组合逻辑
- delegate_to 工具（sync mode）
- 子会话创建 + parent_session_id
- incremental persist（INSERT+UPDATE 模式）
- 前端重连逻辑
- 3 层深度限制
- step limit（防止 agent 无限循环）

### Must NOT Have (Guardrails)
- 子 agent SSE 事件转发到主 agent 流
- agent_id 创建多个委托工具（只保留一个 delegate_to）
- config.yaml 定义 agent（逐步移除，P2 保留向后兼容）
- Agent 抽象为可执行接口（保持 Definition + Engine 分离）
- async delegate_to mode（defer 到 P5）
- 内存溢出：ringbuf 容量固定 1024，subscriber 上限 50
- execution_mode 参数碰撞：delegate_to 的 mode 参数和工具管理器注入的 execution_mode 不能同时出现

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed.

### Test Decision
- **Infrastructure exists**: YES (Go: `go test ./...`, Frontend: 无现有测试)
- **Automated tests**: TDD
- **Framework**: Go: `testing` + `testify`; Frontend: `vitest` (如需要)
- **TDD**: Each task follows RED (failing test) → GREEN (minimal impl) → REFACTOR

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.omo/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Go backend**: Bash (`go test ./... -v -run <TestName>`)
- **API/SSE**: Bash (`curl` for endpoints, SSE stream verification)
- **Frontend/UI**: Playwright (browser automation)

---

## Execution Strategy

### Parallel Execution Waves

> Target: 5-8 tasks per wave. Fewer than 3 per wave = under-splitting.

Wave 0 (Spike — 1 task):
├── Task 0: ringbuf validation spike [quick]

Wave 1 (ChatContext Upgrade — 5 tasks, MAX PARALLEL):
├── Task 1: ChatContextInterface — Close/Closed/Depth [quick]
├── Task 2: Ringbuf ChatContext impl [deep]
├── Task 3: SessionAgentStore [quick]
├── Task 4: Unified SSE handler [quick]
├── Task 5: Engine integration (step limit + Events() compat) [deep]

Wave 2 (AgentRegistry Factory — 4 tasks, MAX PARALLEL):
├── Task 6: DB migration — parent_session_id [quick]
├── Task 7: AgentDefinition.Hooks + hook compose [quick]
├── Task 8: Factory-based AgentRegistry [deep]
├── Task 9: Config deprecation + backward compat [quick]

Wave 2.5 (Frontend Reconnect — 3 tasks, runs parallel with Wave 2):
├── Task 10: XRequest reconnect spike [quick]
├── Task 11: AgentClient reconnect method [quick]
├── Task 12: useAgentChat reconnect logic [deep]

Wave 3 (delegate_to Tool — 5 tasks, MAX PARALLEL):
├── Task 13: delegate_to tool (execution_mode collision fix) [deep]
├── Task 14: Sub-session creation + parent_session_id [deep]
├── Task 15: Sub-agent spawning + depth check [deep]
├── Task 16: SSE handler — 204 for reconnect on finished [quick]
├── Task 17: delegate_to tool registration in engine [quick]

Wave 3.5 (Incremental Persist — 2 tasks):
├── Task 18: persistMessage INSERT+UPDATE redesign [deep]
├── Task 19: Incremental persist at step boundaries [deep]

Wave 4 (Frontend Subagent UI — 2 tasks):
├── Task 20: SubagentCard component [visual-engineering]
├── Task 21: Sub-session SSE connection hook [deep]

Wave FINAL (Polish — 4 tasks, MAX PARALLEL, after ALL above):
├── Task F1: read_sub_session tool [deep]
├── Task F2: Plan compliance audit [oracle]
├── Task F3: Code quality review [unspecified-high]
├── Task F4: Real QA + scope check [unspecified-high]

Total: 24 implementation tasks + 4 final verification
Max Concurrent: 5 (Wave 1 & Wave 3)
Parallel Speedup: ~60% vs sequential

### Critical Path
P0 → Task 2 → Task 5 → Task 8 → Task 13 → Task 15 → Task 18 → Task 20 → F1-F4

### Dependency Matrix

- **P0**: none → 1-5, Wave 1
- **1**: none → 2-5
- **2**: none → 5
- **3**: none → 4
- **4**: 3 → none
- **5**: 1,2 → 8,15
- **6**: none → 14
- **7**: none → 8
- **8**: 7 → 13
- **9**: none → none
- **10**: none → 11
- **11**: 10 → 12
- **12**: 11 → 20
- **13**: 8 → 14,17
- **14**: 6,13 → 15
- **15**: 5,14 → 18
- **16**: 4 → none
- **17**: 13 → none
- **18**: 15 → 19
- **19**: 18 → none
- **20**: 12 → 21
- **21**: 20 → none
- **F1-F4**: ALL above → user okay

### Agent Dispatch Summary

- **0**: **1** — P0 → `quick`
- **1**: **5** — T1-4 → `quick`, T5 → `deep`
- **2**: **4** — T6,7,9 → `quick`, T8 → `deep`
- **2.5**: **3** — T10-11 → `quick`, T12 → `deep`
- **3**: **5** — T16-17 → `quick`, T13-15 → `deep`
- **3.5**: **2** — T18-19 → `deep`
- **4**: **2** — T20 → `visual-engineering`, T21 → `deep`
- **FINAL**: **4** — F1 → `deep`, F2 → `oracle`, F3-4 → `unspecified-high`

---

## TODOs

- [x] 0. **Ringbuf Validation Spike**

  **What to do**:
  - Create a standalone Go test file that imports `github.com/golang-cz/ringbuf`
  - Verify ringbuf compiles and runs under Go 1.26
  - Test: single writer → 10 concurrent subscribers → each receives all events → no data races
  - Test: subscriber Seek(). Seek from position 50 in a 1024-sized buffer → verify replay
  - Test: subscriber lag → slow subscriber gets error → other subscribers unaffected
  - Run with `-race` flag to confirm no data races
  - Report: does ringbuf work? any bugs? should we proceed?

  **Must NOT do**:
  - Do NOT integrate ringbuf into ChatContext yet (that's Task 2)
  - Do NOT write a ringbuf wrapper/abstraction (just direct usage in test)

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`
  - **Reason**: Single-file spike, no production code

  **Parallelization**:
  - **Can Run In Parallel**: NO (blocks all Wave 1 tasks)
  - **Parallel Group**: Wave 0 (standalone)
  - **Blocks**: Tasks 1-5
  - **Blocked By**: None

  **References**:
  - `https://github.com/golang-cz/ringbuf` — library README and API docs
  - `https://pkg.go.dev/github.com/golang-cz/ringbuf` — Go package documentation
  - `docs/subagent-architecture-design.md` — §4, ringbuf design rationale
  - `go.mod` — current Go version (must be 1.26 compatible)

  **Acceptance Criteria**:
  - [ ] Test file created: `server/internal/chat_context/ringbuf_spike_test.go`
  - [ ] `go test ./internal/chat_context/... -run Spike -race -v` → PASS, no race detected
  - [ ] Seek replay test: subscriber that Seeks from position 50 gets events 50-100 correctly
  - [ ] Slow subscriber test: subscriber with MaxLag=10 gets ErrTooSlow, other subscriber continues

  **QA Scenarios**:

  ```
  Scenario: Basic fan-out correctness
    Tool: Bash (go test)
    Preconditions: Fresh ringbuf(1024)
    Steps:
      1. Create 10 subscribers, each in own goroutine
      2. Writer publishes 500 events
      3. Each subscriber reads its full stream
      4. Assert all 10 subscribers received exactly 500 events
      5. Assert no data races (-race flag)
    Expected Result: All subscribers receive identical event counts
    Failure Indicators: Mismatched counts, race detector warnings, deadlocks
    Evidence: .omo/evidence/task-0-fanout.txt

  Scenario: Seek replay
    Tool: Bash (go test)
    Preconditions: ringbuf(1024) with 100 events written
    Steps:
      1. Create subscriber with StartBehind=80
      2. Read events, verify count = 80
      3. Verify first event seq == 20
    Expected Result: Subscriber replays last 80 events correctly
    Evidence: .omo/evidence/task-0-seek.txt

  Scenario: Slow subscriber termination
    Tool: Bash (go test)
    Preconditions: ringbuf(64), MaxLag=10 for one subscriber
    Steps:
      1. Create normal subscriber and slow subscriber (MaxLag=10)
      2. Writer publishes 100 events rapidly
      3. Slow subscriber blocks, falls behind by >10
      4. Assert slow subscriber.Err() == ErrTooSlow
      5. Assert normal subscriber still functional
    Expected Result: Slow subscriber terminated, normal subscriber continues
    Evidence: .omo/evidence/task-0-slow.txt
  ```

  **Evidence to Capture**:
  - [ ] `go test` output with -race flag

  **Commit**: YES
  - Message: `test(chatctx): ringbuf validation spike`
  - Files: `server/internal/chat_context/ringbuf_spike_test.go`
  - Pre-commit: `go test ./internal/chat_context/... -run Spike -race -v`

- [x] 1. **ChatContextInterface — Add Close()/Closed()/Depth()/Subscribe()**

  **What to do**:
  - Add `Close()` method to `ChatContextInterface` (called when agent finishes)
  - Add `Closed() <-chan struct{}` method (signal channel for sync subagent completion)
  - Add `Depth() int` method (returns current nesting depth)
  - Add `Subscribe(fromSeq int64) (*Subscriber, bool)` method (ringbuf subscriber factory)
  - Implement on concrete `ChatContext` type (Task 2 fills in ringbuf details)
  - TDD: Write interface compliance test first

  **Must NOT do**:
  - Do NOT change existing `Events()` / `Emit()` / `Context()` / `SessionID()` / `AgentID()` signatures
  - Do NOT remove the chan-based `Events()` in this task (keep backward compat for Task 5)

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`
  - **Reason**: Interface extension, small scope, clear spec

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 2,3,4)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 5, 8, 15
  - **Blocked By**: Task 0

  **References**:
  - `server/internal/domain/iface/chat.go:10-16` — current ChatContextInterface
  - `server/internal/domain/iface/chat.go:18-55` — current ChatContext concrete type
  - `docs/subagent-architecture-design.md` — §4.3, Subscribe interface
  - `server/internal/agent/engine.go:386-461` — runAgentLoop (needs Depth check)

  **Acceptance Criteria**:
  - [ ] Test: `server/internal/domain/iface/chat_test.go` — `TestChatContextInterface` PASS
  - [ ] Test verifies Close() exists on interface, Closed() returns channel type
  - [ ] Test verifies Depth() returns int, Subscribe() returns (*Subscriber, bool)
  - [ ] `go test ./internal/... -v` → no regressions

  **QA Scenarios**:

  ```
  Scenario: Interface compliance
    Tool: Bash (go test)
    Preconditions: None
    Steps:
      1. Create *ChatContext
      2. Assign to ChatContextInterface variable
      3. Call Close(), verify no panic
      4. Call Closed(), verify returns <-chan struct{}
      5. Call Depth(), verify returns 0
    Expected Result: All methods accessible via interface
    Failure Indicators: Compile error, panic, wrong return type
    Evidence: .omo/evidence/task-1-interface.txt

  Scenario: Backward compatibility
    Tool: Bash (go test)
    Preconditions: Existing code that calls chatCtx.Events()
    Steps:
      1. Run `go test ./internal/api/... -v`
      2. Run `go test ./internal/agent/... -v`
      3. Verify existing tests still pass without modification
    Expected Result: All existing tests pass
    Failure Indicators: Test failures in api/ or agent/ packages
    Evidence: .omo/evidence/task-1-backward.txt
  ```

  **Evidence to Capture**:
  - [ ] Go test output showing all tests pass

  **Commit**: YES
  - Message: `feat(chatctx): add Close/Closed/Depth/Subscribe to ChatContextInterface`
  - Files: `server/internal/domain/iface/chat.go`
  - Pre-commit: `go test ./... -v`

- [x] 2. **Ringbuf ChatContext Implementation**

  **What to do**:
  - Replace internal `chan entity.Event` with `ringbuf.RingBuffer[entity.Event]`
  - Implement `Subscribe(fromSeq)` using ringbuf's Subscribe + SeekAfter
  - Implement `Close()` → ringbuf.Close(), broadcast to all subscribers
  - Implement `Closed()` → signal channel closed after ringbuf.Close()
  - Implement `Depth()` → return stored depth value (0 for root)
  - Convert `Events()` to create a backward-compat subscriber (single-consumer mode)
  - TDD: Write ringbuf integration test

  **Must NOT do**:
  - Do NOT change `Emit()` caller-side code (agent continues calling `chatCtx.Emit(event)`)
  - Do NOT remove `Events()` until Task 5 confirms all callers migrated

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`
  - **Reason**: Core infrastructure change, concurrency-sensitive, integration with third-party library

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 1,3,4)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 5
  - **Blocked By**: Task 0

  **References**:
  - `server/internal/domain/iface/chat.go` — ChatContext concrete type
  - `https://pkg.go.dev/github.com/golang-cz/ringbuf` — RingBuffer API: Subscribe, Seek, Write, Close
  - `docs/subagent-architecture-design.md` — §4, ChatContext upgrade design
  - `server/internal/agent/engine.go` — Emit call sites
  - `server/internal/api/handlers.go:339-342` — SSE reader (current Events() consumer)
  - `server/internal/agent/engine_tools.go` — Emit from async goroutines

  **Acceptance Criteria**:
  - [ ] Test: `server/internal/chat_context/ringbuf_test.go` — `TestRingbufChatContext` PASS
  - [ ] Test: single writer → 3 subscribers → each gets complete event stream
  - [ ] Test: subscriber Seek from position 50 gets events 50+
  - [ ] Test: Close() → all subscribers receive io.EOF on Iter()
  - [ ] Test: Closed() channel fires after Close()
  - [ ] `go test ./... -race` → no data races

  **QA Scenarios**:

  ```
  Scenario: Multi-subscriber correctness
    Tool: Bash (go test -race)
    Preconditions: ChatContext with ringbuf
    Steps:
      1. Create ChatContext(depth=0, cap=1024)
      2. Create 3 subscribers via Subscribe(0)
      3. Emit 100 events from writer goroutine
      4. Each subscriber reads all events
      5. Assert each received exactly 100 events
    Expected Result: 3 × 100 = 300 total received events
    Failure Indicators: Mismatched counts, race detector warnings
    Evidence: .omo/evidence/task-2-multi.txt

  Scenario: Emit from multiple goroutines (async tool safety)
    Tool: Bash (go test -race)
    Preconditions: ChatContext with ringbuf
    Steps:
      1. Create ChatContext + 1 subscriber
      2. Spawn 5 goroutines, each calls Emit() 50 times
      3. Join all goroutines
      4. Subscriber reads all events
      5. Assert subscriber received 250 events
    Expected Result: 250 events, no races
    Failure Indicators: Panic, race warnings, event count mismatch
    Evidence: .omo/evidence/task-2-multi-goroutine.txt

  Scenario: Seek boundary — events lost
    Tool: Bash (go test)
    Preconditions: 1024-cap ringbuf, 2000 events written
    Steps:
      1. Emit 2000 events (fills and overwrites buffer)
      2. Subscribe(fromSeq=0) — seq 0 already evicted
      3. Assert Subscribe returns ok=false
    Expected Result: Subscribe returns false
    Failure Indicators: Returns ok=true with empty events, panics
    Evidence: .omo/evidence/task-2-seek-lost.txt
  ```

  **Evidence to Capture**:
  - [ ] Go test output with -race flag

  **Commit**: YES
  - Message: `feat(chatctx): ringbuf-based event distribution`
  - Files: `server/internal/domain/iface/chat.go`, `server/internal/chat_context/ringbuf.go`
  - Pre-commit: `go test ./internal/... -race -v`

- [x] 3. **SessionAgentStore — Concurrency-Safe Implementation**

  **What to do**:
  - Create `SessionAgentStore` struct with `sync.RWMutex`-protected `map[string]*ChatContext`
  - `Put(sessionID, chatCtx)` — store active ChatContext
  - `Get(sessionID) → *ChatContext, bool` — lookup active ChatContext
  - `Remove(sessionID)` — called by ChatContext.Close()
  - `ChatContext.Close()` calls `SessionAgentStore.Remove()` automatically
  - TDD: Write concurrent access test

  **Must NOT do**:
  - Do NOT leak ChatContexts — ensure Remove is always called

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`
  - **Reason**: Simple map wrapper, well-defined interface, small code footprint

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 1,2,4)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 4
  - **Blocked By**: Task 0,1

  **References**:
  - `server/internal/tool/registry.go` — AsyncToolRegistry pattern (sync.Map + Register/Get)
  - `docs/subagent-architecture-design.md` — §5.3, SessionAgentStore design
  - `server/internal/domain/iface/chat.go` — ChatContext concrete type

  **Acceptance Criteria**:
  - [ ] Test: `server/internal/chat_context/store_test.go` — `TestSessionAgentStore` PASS
  - [ ] Test: Put → Get returns same ChatContext
  - [ ] Test: Remove → Get returns nil
  - [ ] Test: 10 concurrent goroutines Put/Get → no races
  - [ ] `go test ./... -race` → no data races

  **QA Scenarios**:

  ```
  Scenario: Basic Put/Get/Remove
    Tool: Bash (go test)
    Preconditions: Fresh SessionAgentStore
    Steps:
      1. Create ChatContext("session-A")
      2. store.Put("session-A", chatCtx)
      3. got, ok := store.Get("session-A")
      4. Assert ok == true, got == chatCtx
      5. store.Remove("session-A")
      6. got, ok = store.Get("session-A")
      7. Assert ok == false
    Expected Result: Correct Put/Get/Remove lifecycle
    Failure Indicators: Wrong ChatContext returned, Remove not working
    Evidence: .omo/evidence/task-3-basic.txt

  Scenario: Concurrent access
    Tool: Bash (go test -race)
    Preconditions: SessionAgentStore with 50 chat contexts
    Steps:
      1. Populate store with 50 entries
      2. Spawn 10 goroutines performing random Get/Put
      3. Spawn 5 goroutines performing Remove
      4. Run for 1 second under -race
    Expected Result: No races, no panics
    Failure Indicators: Race detector warnings
    Evidence: .omo/evidence/task-3-concurrent.txt
  ```

  **Evidence to Capture**:
  - [ ] Go test output with -race flag

  **Commit**: YES
  - Message: `feat(chatctx): SessionAgentStore with concurrency safety`
  - Files: `server/internal/chat_context/store.go`
  - Pre-commit: `go test ./internal/chat_context/... -race -v`

- [x] 4. **Unified SSE Handler — First-Connect + Reconnect**

  **What to do**:
  - Modify `POST /api/sessions/{id}/chat` handler to support:
    - `content` + no active agent → create ChatContext, start agent, SSE
    - `reconnect=true` + active agent → get ChatContext from SessionAgentStore, Subscribe
    - `reconnect=true` + no active agent → 204 No Content
    - `content` + active agent → 409 Conflict
  - Parse `last_event_seq` from request body for reconnect positioning
  - Subscribe logic: call `chatCtx.Subscribe(lastEventSeq+1)`
  - If Subscribe returns ok=false → write `{"type":"events_lost"}` event → close
  - TDD: Write handler test with test SSE client

  **Must NOT do**:
  - Do NOT change the SSE wire format (still `data: {json}\n\n`)
  - Do NOT create two code paths for first-connect and reconnect (single unified path)

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`
  - **Reason**: Handler modification, well-understood contract, small surface area

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 1,2,3)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 16
  - **Blocked By**: Task 0,3

  **References**:
  - `server/internal/api/handlers.go:308-350` — current Chat handler
  - `server/internal/api/routes.go` — route registration
  - `docs/subagent-architecture-design.md` — §5, SSE connection mechanism
  - `server/internal/domain/iface/chat.go` — Subscribe API

  **Acceptance Criteria**:
  - [ ] Test: `server/internal/api/handlers_test.go` — `TestSSEReconnect` PASS
  - [ ] Test: POST with content → 200 + SSE stream
  - [ ] Test: POST with reconnect=true, active agent → 200 + SSE stream from current point
  - [ ] Test: POST with reconnect=true, no active agent → 204
  - [ ] Test: POST with content, active agent → 409
  - [ ] Test: POST with reconnect + last_event_seq → events start from seq+1

  **QA Scenarios**:

  ```
  Scenario: Normal first-connect
    Tool: Bash (curl)
    Preconditions: Valid session exists, no active agent
    Steps:
      1. curl POST /api/sessions/{id}/chat -d '{"content":"hello"}'
      2. Read SSE stream for 2 seconds
      3. Assert response starts with "data:"
      4. Assert events contain step_create and part_create
    Expected Result: SSE stream with agent events
    Failure Indicators: Empty response, non-SSE format, 500 error
    Evidence: .omo/evidence/task-4-first-connect.txt

  Scenario: Reconnect to active agent
    Tool: Bash (curl)
    Preconditions: Agent running for session (from first-connect test)
    Steps:
      1. curl POST /api/sessions/{id}/chat -d '{"reconnect":true,"last_event_seq":5}'
      2. Read SSE stream
      3. Assert events continue from seq>=6
      4. Assert no duplicate events for seq<=5
    Expected Result: Events resume from reconnection point
    Failure Indicators: Events from seq<=5 appear, connection refused
    Evidence: .omo/evidence/task-4-reconnect.txt

  Scenario: Reconnect with no active agent
    Tool: Bash (curl)
    Preconditions: No active agent for session
    Steps:
      1. curl POST /api/sessions/{id}/chat -d '{"reconnect":true}'
      2. Assert HTTP status 204
      3. Assert empty response body
    Expected Result: 204 No Content
    Failure Indicators: 200 with empty SSE, 500 error
    Evidence: .omo/evidence/task-4-no-agent.txt

  Scenario: Events lost notification
    Tool: Bash (curl)
    Preconditions: Active agent, ringbuf fully overwritten
    Steps:
      1. Emit 2000+ events to fill and overwrite ringbuf(1024)
      2. curl POST /chat -d '{"reconnect":true,"last_event_seq":0}'
      3. Assert first SSE event is {"type":"events_lost"}
      4. Assert connection closes after
    Expected Result: events_lost event received, connection closed
    Failure Indicators: Garbage events, connection hang
    Evidence: .omo/evidence/task-4-events-lost.txt
  ```

  **Evidence to Capture**:
  - [ ] curl output for each scenario

  **Commit**: YES
  - Message: `feat(api): unified SSE handler with first-connect and reconnect`
  - Files: `server/internal/api/handlers.go`
  - Pre-commit: `go test ./internal/api/... -v`

- [x] 5. **Engine Integration — Step Limit + Ringbuf Migration + Depth Check**

  **What to do**:
  - Add `maxSteps` constant (=50) to agent loop; terminate with error event if exceeded
  - Add runtime depth check: `if chatCtx.Depth() >= 3 → return error`
  - Migrate SSE handler goroutine from `for event := range chatCtx.Events()` to `for event := range sub.Iter()` (ringbuf mode)
  - Keep `Events()` compat layer until this task confirms migration complete
  - TDD: Write step limit test, depth rejection test, ringbuf SSE integration test

  **Must NOT do**:
  - Do NOT change runAgentLoop core logic (context build, LLM stream, tool exec)
  - Do NOT break existing Emit() call sites in engine_tools.go

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`
  - **Reason**: Engine integration, concurrency-sensitive, migration coordination

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 1,2)
  - **Parallel Group**: Wave 1 (last task)
  - **Blocks**: Tasks 8, 15
  - **Blocked By**: Tasks 0, 1, 2

  **References**:
  - `server/internal/agent/engine.go:386-461` — runAgentLoop
  - `server/internal/agent/engine.go:125-133` — Chat entry point
  - `server/internal/agent/engine_tools.go` — Emit call sites from tool execution
  - `server/internal/api/handlers.go` — SSE reader (target for migration)
  - `docs/subagent-architecture-design.md` — §7.3, depth limit

  **Acceptance Criteria**:
  - [ ] Test: `server/internal/agent/engine_test.go` — `TestStepLimit` PASS
  - [ ] Test: `server/internal/agent/engine_test.go` — `TestDepthRejection` PASS
  - [ ] Test: step limit = 50 → engine stops at iteration 50 with error event
  - [ ] Test: depth = 3 → Chat() returns "max depth exceeded" error
  - [ ] `go test ./...` → no regressions

  **QA Scenarios**:

  ```
  Scenario: Step limit enforcement
    Tool: Bash (go test)
    Preconditions: Agent loop that always calls same tool (infinite loop)
    Steps:
      1. Create agent with single tool that returns "try again"
      2. Set maxSteps=10 for test
      3. Run agent; assert loop terminates at iteration 10
      4. Assert final event is error with "step limit exceeded"
    Expected Result: Agent terminates at step 10, error event emitted
    Failure Indicators: Agent hangs, no error event
    Evidence: .omo/evidence/task-5-step-limit.txt

  Scenario: Depth rejection
    Tool: Bash (go test)
    Preconditions: ChatContext with Depth=3
    Steps:
      1. Create ChatContext(depth=3)
      2. Call engine.Chat(chatCtx, "test")
      3. Assert error returned contains "max subagent depth"
      4. Assert no events emitted
    Expected Result: Immediate error, no agent execution
    Failure Indicators: Agent runs despite depth=3, no error
    Evidence: .omo/evidence/task-5-depth.txt

  Scenario: Ringbuf SSE integration
    Tool: Bash (curl)
    Preconditions: Engine migrated to ringbuf mode
    Steps:
      1. curl POST /chat with content
      2. Accept SSE stream
      3. Kill curl (simulate disconnect)
      4. Reconnect with reconnect=true
      5. Assert events continue without gaps
    Expected Result: Reconnect seamless in ringbuf mode
    Failure Indicators: Duplicate events, connection refused on reconnect
    Evidence: .omo/evidence/task-5-ringbuf-sse.txt
  ```

  **Evidence to Capture**:
  - [ ] Go test output
  - [ ] curl SSE output for reconnect test

  **Commit**: YES
  - Message: `feat(agent): step limit, depth check, ringbuf SSE integration`
  - Files: `server/internal/agent/engine.go`, `server/internal/api/handlers.go`
  - Pre-commit: `go test ./... -v`

- [x] 6. **DB Migration — parent_session_id Column**
- [x] 7. **AgentDefinition.Hooks + Hook Composition**
- [x] 10. **XRequest Reconnect Spike + AgentClient Reconnect**
- [x] 8. **Factory-Based AgentRegistry**
- [ ] 9. **Config Deprecation + Backward Compatibility**

  **What to do**:
  - Verify `@ant-design/x-sdk` XRequest can pass custom POST body parameters (`reconnect`, `last_event_seq`)
  - If XRequest supports extension: add `reconnect()` method to AgentClient
  - If XRequest does NOT support: switch to custom fetch-based SSE client for reconnect scenarios
  - Implement `AgentClient.reconnect(sessionId, lastEventSeq)` method
  - POST body: `{ reconnect: true, last_event_seq: N }`
  - TDD: Write test verifying correct POST body sent

  **Must NOT do**:
  - Do NOT rewrite AgentClient entirely (only add reconnect method)
  - Do NOT break existing `chat()` method

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`
  - **Reason**: Spike + single method addition

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 11-12 within Wave 2.5, AND with Wave 2)
  - **Parallel Group**: Wave 2.5
  - **Blocks**: Task 11
  - **Blocked By**: None (independent of backend changes)

  **References**:
  - `packages/ui/src/api/agentClient.ts` — current AgentClient
  - `@ant-design/x-sdk` — XRequest documentation
  - `packages/ui/src/hooks/useAgentChat.ts:44-61` — XRequest usage
  - `docs/subagent-architecture-design.md` — §5, reconnect parameters

  **Acceptance Criteria**:
  - [ ] Spike result: documented whether XRequest supports reconnect OR decision to replace
  - [ ] `AgentClient.reconnect()` sends correct POST body
  - [ ] Method type-safe (TypeScript strict)

  **QA Scenarios**:

  ```
  Scenario: Reconnect POST body
    Tool: Bash (pnpm test or manual verification)
    Steps:
      1. Call client.reconnect("sess-1", 42)
      2. Assert POST body = {reconnect:true, last_event_seq:42}
      3. Assert URL = /api/sessions/sess-1/chat
    Expected Result: Correct POST body and URL
    Evidence: .omo/evidence/task-10-reconnect-body.txt
  ```

  **Commit**: YES
  - Message: `feat(ui): AgentClient reconnect method`
  - Files: `packages/ui/src/api/agentClient.ts`
  - Pre-commit: `pnpm build`

- [ ] 11. **useAgentChat Reconnect Logic**

  **What to do**:
  - Add `isReconnecting` state to useAgentChat
  - On SSE error/close: set isReconnecting=true, preserve messages
  - Call `client.reconnect(sessionId, lastReceivedSeq)` from onError handler
  - On reconnect success: continue receiving events via CopConChatProvider
  - On reconnect failure (events_lost): GET /messages → merge with current state → subscribe fresh
  - Export `isReconnecting` for UI use
  - TDD: Write reconnect test (mock SSE, verify state transitions)

  **Must NOT do**:
  - Do NOT clear messages on disconnect
  - Do NOT change CopConChatProvider.transformMessage logic

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`
  - **Reason**: Complex state management, SSE lifecycle, integration with ant-design x-sdk

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on 10)
  - **Parallel Group**: Wave 2.5
  - **Blocks**: Task 12
  - **Blocked By**: Task 10

  **References**:
  - `packages/ui/src/hooks/useAgentChat.ts` — current hook
  - `packages/ui/src/providers/CopConChatProvider.ts` — message transformation
  - `packages/ui/src/api/agentClient.ts` — client methods
  - `packages/ui/src/api/types.ts` — CopConMessage type
  - `docs/subagent-architecture-design.md` — §6.3, reconnect flow

  **Acceptance Criteria**:
  - [ ] SSE disconnect → isReconnecting=true, messages preserved
  - [ ] Reconnect success → isReconnecting=false, events continue
  - [ ] Reconnect events_lost → GET /messages → merge → fresh subscribe
  - [ ] `pnpm build` → no errors

  **QA Scenarios**:

  ```
  Scenario: Disconnect and reconnect
    Tool: Playwright
    Preconditions: Active chat session with streaming response
    Steps:
      1. Send message, wait for streaming to start
      2. Kill SSE connection (via devtools network throttle → offline)
      3. Assert isReconnecting=true, bubble still shows partial text
      4. Restore network → trigger reconnect
      5. Assert streaming continues from where it left off
      6. Assert isReconnecting=false
    Expected Result: Seamless reconnect, no message loss
    Evidence: .omo/evidence/task-11-reconnect.png

  Scenario: Reconnect with events_lost
    Tool: Playwright
    Preconditions: Ringbuf fully overwritten
    Steps:
      1. Send many messages to fill ringbuf
      2. Disconnect for 60s, then reconnect
      3. Assert GET /messages call happens
      4. Assert UI shows merged state
    Expected Result: Falls back to GET /messages, UI consistent
    Evidence: .omo/evidence/task-11-events-lost.png
  ```

  **Evidence to Capture**:
  - [ ] Playwright screenshots for each scenario

  **Commit**: YES
  - Message: `feat(ui): useAgentChat reconnect logic`
  - Files: `packages/ui/src/hooks/useAgentChat.ts`
  - Pre-commit: `pnpm build`

- [ ] 12. **SubagentCard Component + Sub-session SSE Hook**

  **What to do**:
  - Create `SubagentCard` component with collapsible card UI
  - Props: `subSessionId`, `isRunning`
  - On mount: connect to sub-session SSE via `POST /chat {reconnect:true}`
  - Render sub-agent's Step/Part stream (reuse StepContent from demo App)
  - Show header with agent name + status indicator
  - Create `useSubagentSSE(sessionId)` custom hook for sub-session SSE connection
  - Hook manages: connect, disconnect, event accumulation, error handling
  - TDD: Write component test with mock SSE

  **Must NOT do**:
  - Do NOT embed sub-session SSE connection in main chat provider
  - Do NOT forward subagent events to main agent message list

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
  - **Skills**: `["frontend-ui-ux"]`
  - **Reason**: UI component with collapsible card, streaming content, status indicator

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on 11)
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: Task 11

  **References**:
  - `packages/demo/src/App.tsx:35-72` — StepContent rendering (reuse pattern)
  - `packages/ui/src/components/TodoItem/` — existing component structure
  - `packages/ui/src/providers/CopConChatProvider.ts` — SSE transformation reference
  - `docs/subagent-architecture-design.md` — §10.2, SubagentCard

  **Acceptance Criteria**:
  - [ ] SubagentCard renders collapsible card with subagent steps
  - [ ] Realtime streaming: part_create/update events update the card
  - [ ] Card collapses/expands on click
  - [ ] useSubagentSSE handles disconnect gracefully
  - [ ] `pnpm build` → no errors

  **QA Scenarios**:

  ```
  Scenario: Subagent card with streaming
    Tool: Playwright
    Preconditions: delegate_to tool called, sub_session_id returned
    Steps:
      1. Mock delegate_to tool result with sub_session_id
      2. SubagentCard mounts with sub_session_id
      3. Verify SSE connection established to sub-session
      4. Streaming events arrive: verify card updates in realtime
      5. Verify card shows reasoning, text, and tool-call sub-parts
    Expected Result: Real-time subagent output visible in card
    Evidence: .omo/evidence/task-12-streaming.png

  Scenario: Card collapse/expand
    Tool: Playwright
    Steps:
      1. SubagentCard renders expanded
      2. Click collapse button → card content hidden
      3. Click expand button → card content visible
    Expected Result: Smooth collapse/expand animation
    Evidence: .omo/evidence/task-12-collapse.png
  ```

  **Evidence to Capture**:
  - [ ] Playwright screenshots

  **Commit**: YES
  - Message: `feat(ui): SubagentCard component + useSubagentSSE hook`
  - Files: `packages/ui/src/components/SubagentCard/`, `packages/ui/src/hooks/useSubagentSSE.ts`
  - Pre-commit: `pnpm build`

- [ ] 13. **delegate_to Tool — Implementation**

  **What to do**:
  - Implement `DelegateToTool` in `server/internal/tools/delegate.go`, implements `tool.Tool`
  - Tool name: `delegate_to`, params: `agent_id` (required), `task` (required), `mode` (optional, default "sync"), `extra` (optional)
  - Suppress auto-injected `execution_mode` from tool manager: set a flag on the tool that GetOpenAITools() checks
  - Execute(): agent_id → GetFactory → factory.Create() → AgentDefinition → create sub-session → spawn sub-agent → return result
  - Sync mode: block until sub-agent ChatContext.Closed() → return {sub_session_id, summary, status:"completed"}
  - Handle missing agent_id: return ToolResult{Success:false, Error:"agent not found"}
  - Handle factory error: return ToolResult{Success:false, Error:"factory creation failed"}
  - TDD: Write test with mock factory and engine

  **Must NOT do**:
  - Do NOT implement async mode (defer to P5)
  - Do NOT embed sub-agent SSE events in parent stream
  - Do NOT let tool manager inject `execution_mode` into delegate_to's OpenAI schema

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`
  - **Reason**: Core business logic, tool implementation, integration with registry and engine

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 14,15,16,17)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 18
  - **Blocked By**: Task 8

  **References**:
  - `server/internal/tool/manager.go:31-36` — Tool interface
  - `server/internal/tools/` — existing tool implementations for pattern
  - `server/internal/agent/registry.go` — GetFactory API
  - `server/internal/agent/engine.go` — Chat() entry point for sub-agent
  - `server/internal/tool/manager.go:174-212` — GetOpenAITools (execution_mode injection)
  - `docs/subagent-architecture-design.md` — §7, delegate_to design

  **Acceptance Criteria**:
  - [ ] Test: `server/internal/tools/delegate_test.go` — `TestDelegateToTool` PASS
  - [ ] Test: delegate_to(sync) → returns correct sub_session_id + summary
  - [ ] Test: delegate_to(invalid agent_id) → ToolResult.Success=false
  - [ ] Test: delegate_to's OpenAI schema does NOT include execution_mode parameter
  - [ ] `go test ./internal/tools/... -v` → PASS

  **QA Scenarios**:

  ```
  Scenario: Sync delegation success
    Tool: Bash (go test)
    Preconditions: Mock factory returns simple agent, mock engine runs
    Steps:
      1. Execute delegate_to(agent_id="test-reviewer", task="review", mode="sync")
      2. Assert ToolResult.Success == true
      3. Assert output contains sub_session_id
      4. Assert output contains summary from sub-agent
    Expected Result: Successful delegation, result contains sub_session_id
    Evidence: .omo/evidence/task-13-sync.txt

  Scenario: Invalid agent_id
    Tool: Bash (go test)
    Steps:
      1. Execute delegate_to(agent_id="nonexistent", task="test")
      2. Assert ToolResult.Success == false
      3. Assert ToolResult.Error contains "not found"
    Expected Result: Graceful error
    Evidence: .omo/evidence/task-13-invalid-agent.txt

  Scenario: No execution_mode in schema
    Tool: Bash (go test)
    Steps:
      1. Call tool.InputSchema() on delegate_to
      2. Assert schema.properties does NOT contain "execution_mode"
      3. Assert schema.properties DOES contain "agent_id", "task", "mode"
    Expected Result: No execution_mode collision
    Evidence: .omo/evidence/task-13-schema.txt
  ```

  **Evidence to Capture**:
  - [ ] Go test output

  **Commit**: YES
  - Message: `feat(tools): delegate_to tool implementation`
  - Files: `server/internal/tools/delegate.go`
  - Pre-commit: `go test ./internal/tools/... -v`

- [ ] 14. **Sub-Session Creation with parent_session_id**

  **What to do**:
  - Update `SessionManager.Create` to accept `parentSessionID *uuid.UUID` parameter
  - In delegate_to.Execute: create sub-session with parent=current session
  - Sub-session title: auto-generated (e.g., "delegate_to_code-reviewer_2026-05-21")
  - Inject task as first user message in sub-session
  - TDD: Verify sub-session has correct parent_session_id, correct messages

  **Must NOT do**:
  - Do NOT change existing Create() call sites (parent uses nil default)
  - Do NOT create sub-sessions with mismatched agent_id

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`
  - **Reason**: Persistence integration, session lifece manager API change

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 13,15,16,17)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 15, 18
  - **Blocked By**: Task 6

  **References**:
  - `server/internal/session/manager.go` — SessionManager.Create
  - `server/internal/session/model.go:269-278` — Session struct with ParentSessionID
  - `server/internal/chat_context/manager.go:61-73` — AddMessage for task injection
  - `docs/subagent-architecture-design.md` — §7.2, sub-session creation

  **Acceptance Criteria**:
  - [ ] Test: sub-session.ParentSessionID == parent.ID
  - [ ] Test: sub-session has exactly 1 message (the task)
  - [ ] Test: existing Create() callers continue to work (parent=nil)
  - [ ] `go test ./internal/session/... -v` → PASS

  **QA Scenarios**:

  ```
  Scenario: Sub-session with parent and task message
    Tool: Bash (go test)
    Steps:
      1. Create parent session A
      2. Create sub-session B with parentSessionID=A.ID
      3. Inject task message "review main.go" (role=user)
      4. Fetch B from DB: assert B.parent_session_id == A.ID
      5. Fetch messages for B: assert len=1, content="review main.go"
    Expected Result: Correct parent + task injection
    Evidence: .omo/evidence/task-14-sub-session.txt
  ```

  **Commit**: YES
  - Message: `feat(session): sub-session creation with parent_session_id`
  - Files: `server/internal/session/manager.go`
  - Pre-commit: `go test ./internal/session/... -v`

- [ ] 15. **Sub-Agent Spawning + Depth Check**

  **What to do**:
  - In delegate_to.Execute: create subChatCtx with Depth=parent.Depth+1
  - Pass subChatCtx to engine.Chat() in goroutine (reuse existing async pattern)
  - Engine.Chat() entry: check depth >= 3 → return error immediately
  - Wait on subChatCtx.Closed() for sync mode
  - Collect sub-agent's last assistant message as summary
  - TDD: Write test verifying depth propagation, depth rejection, correct goroutine lifecycle

  **Must NOT do**:
  - Do NOT let sub-agent goroutine outlive parent (use context cancellation)
  - Do NOT share ChatContext between parent and sub-agent

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`
  - **Reason**: Goroutine management, depth tracking, engine integration

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 13,14,16,17)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 18
  - **Blocked By**: Tasks 5, 14

  **References**:
  - `server/internal/agent/engine.go:125-133` — Chat entry point
  - `server/internal/agent/engine_tools.go:152-373` — executeAsync goroutine pattern
  - `server/internal/domain/iface/chat.go` — Depth/Closed methods
  - `docs/subagent-architecture-design.md` — §7.3, depth limit

  **Acceptance Criteria**:
  - [ ] Test: subChatCtx.Depth == parent.Depth + 1
  - [ ] Test: depth=3 → engine.Chat rejects with error
  - [ ] Test: sub-agent goroutine starts and completes, Closed channel fires
  - [ ] `go test ./... -race` → no goroutine leaks

  **QA Scenarios**:

  ```
  Scenario: Depth propagation and rejection
    Tool: Bash (go test)
    Steps:
      1. Create ChatContext(depth=0), spawn sub-agent → subDepth=1
      2. Create ChatContext(depth=2), spawn sub-agent → subDepth=3
      3. In subDepth=3, spawn sub-sub-agent → assert error "max depth"
    Expected Result: Depth propagates, level 4 rejected
    Evidence: .omo/evidence/task-15-depth.txt

  Scenario: Sub-agent lifecycle
    Tool: Bash (go test)
    Steps:
      1. Spawn sub-agent goroutine
      2. Wait on subChatCtx.Closed()
      3. Assert Closed channel fires after agent finishes
      4. Assert subChatCtx removed from SessionAgentStore
    Expected Result: Clean lifecycle, no leaks
    Evidence: .omo/evidence/task-15-lifecycle.txt
  ```

  **Evidence to Capture**:
  - [ ] Go test output with -race

  **Commit**: YES
  - Message: `feat(agent): sub-agent spawning with depth check`
  - Files: `server/internal/tools/delegate.go`, `server/internal/agent/engine.go`
  - Pre-commit: `go test ./... -race -v`

- [ ] 16. **SSE Handler — 204 for Reconnect on Finished Agents**

  **What to do**:
  - When ChatContext.Close() is called → SessionAgentStore.Remove(sessionID)
  - SSE handler: if reconnect=true and Get(sessionID) returns nil → 204 No Content
  - Verify: 204 is returned after agent finishes (Closed fires)
  - TDD: Write handler test that verifies 204 after agent completion

  **Must NOT do**:
  - Do NOT return 204 while agent is still running
  - Do NOT remove ChatContext from store before Close()

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`
  - **Reason**: Small handler addition, clear spec

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 13,14,15,17)
  - **Parallel Group**: Wave 3
  - **Blocks**: None
  - **Blocked By**: Task 4

  **References**:
  - `server/internal/api/handlers.go` — SSE handler (modified in Task 4)
  - `server/internal/chat_context/store.go` — SessionAgentStore
  - `docs/subagent-architecture-design.md` — §5.4, ChatContext lifecycle

  **Acceptance Criteria**:
  - [ ] Test: agent running → reconnect returns 200 + SSE
  - [ ] Test: agent finished → reconnect returns 204
  - [ ] `go test ./internal/api/... -v` → PASS

  **QA Scenarios**:

  ```
  Scenario: 204 after agent completion
    Tool: Bash (curl)
    Steps:
      1. Start agent via POST /chat with content
      2. Wait for agent to complete (SSE stream ends)
      3. POST /chat with reconnect=true
      4. Assert HTTP 204
    Expected Result: Clean 204, no lingering connections
    Evidence: .omo/evidence/task-16-204.txt
  ```

  **Commit**: YES
  - Message: `feat(api): 204 on reconnect for finished agents`
  - Files: `server/internal/api/handlers.go`
  - Pre-commit: `go test ./internal/api/... -v`

- [ ] 17. **delegate_to Registration in Engine**

  **What to do**:
  - Register delegate_to tool in agent tool registry (during factory invocation or engine init)
  - Add delegate_to to agent's ToolManager in factory (when agent allows delegation)
  - Update main.go: register delegate_to tool instance during startup
  - TDD: Verify delegate_to appears in agent's tool list

  **Must NOT do**:
  - Do NOT hardcode delegate_to into engine core
  - Do NOT register delegate_to for agents that don't need it

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: `[]`
  - **Reason**: Wiring task, minimal logic

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 13,14,15,16)
  - **Parallel Group**: Wave 3
  - **Blocks**: None
  - **Blocked By**: Task 13

  **References**:
  - `server/cmd/server/main.go` — tool registration
  - `server/internal/agent/registry.go` — agent ToolManager
  - `server/internal/tool/manager.go` — Register method

  **Acceptance Criteria**:
  - [ ] `GET /api/agents` → code-assistant tools include "delegate_to"
  - [ ] Agent without subagents does NOT include delegate_to in tool list
  - [ ] `go test ./...` → no regressions

  **QA Scenarios**:

  ```
  Scenario: delegate_to in agent tool list
    Tool: Bash (curl)
    Steps:
      1. curl GET /api/agents
      2. Assert code-assistant.tools contains delegate_to
      3. Assert delegate_to.input_schema includes agent_id, task params
    Expected Result: delegate_to registered correctly
    Evidence: .omo/evidence/task-17-registration.txt
  ```

  **Commit**: YES
  - Message: `feat(tools): register delegate_to in engine`
  - Files: `server/cmd/server/main.go`, `server/internal/agent/registry.go`
  - Pre-commit: `go test ./... -v`

- [ ] 18. **persistMessage INSERT+UPDATE Redesign**

  **What to do**:
  - Current: every `persistMessage()` creates a new Message row (INSERT-only)
  - New: create Message row ONCE at first `persistMessage` call (when messageID is first generated)
  - Track message UUID in engine state between iterations
  - Subsequent calls UPDATE the same row's `Parts` JSONB column
  - Add `persistMessage` variant: `persistMessageUpsert(msgID, parts)` for incremental updates
  - TDD: Write test — verify single INSERT + multiple UPDATEs = 1 row

  **Must NOT do**:
  - Do NOT change `ContextManager.AddMessage` (still INSERT for final persist)
  - Do NOT lose tool-call data during incremental updates

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`
  - **Reason**: Persistence model redesign, DB interaction, message lifecycle

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on 15 for message UUID tracking)
  - **Parallel Group**: Wave 3.5
  - **Blocks**: Task 19
  - **Blocked By**: Task 15

  **References**:
  - `server/internal/agent/engine.go:524-553` — current persistMessage
  - `server/internal/chat_context/manager.go:61-73` — AddMessage (INSERT)
  - `server/internal/session/model.go:280-293` — Message struct with Parts JSONB
  - `docs/subagent-architecture-design.md` — §9, incremental persist

  **Acceptance Criteria**:
  - [ ] Test: 1st persistMessage → INSERT, 2nd persistMessage → UPDATE same row
  - [ ] Test: COUNT(*) for the streaming message = 1 after 5 incremental persists
  - [ ] Test: final persistMessage → Parts JSONB contains all accumulated data
  - [ ] `go test ./...` → no regressions on existing tests

  **QA Scenarios**:

  ```
  Scenario: Single row INSERT+UPDATE pattern
    Tool: Bash (go test)
    Steps:
      1. Start agent loop with messageID="msg-1"
      2. Call persistMessageUpsert("msg-1", [{type:"text",text:"Hel",state:"streaming"}])
      3. Call persistMessageUpsert("msg-1", [{type:"text",text:"Hello Wor",state:"streaming"}])
      4. Call persistMessageUpsert("msg-1", [{type:"text",text:"Hello World",state:"done"}])
      5. Query DB: assert COUNT=1 for messageID="msg-1"
      6. Query DB: assert latest Parts.text = "Hello World", state = "done"
    Expected Result: Single row, accumulated Parts
    Evidence: .omo/evidence/task-18-upsert.txt
  ```

  **Commit**: YES
  - Message: `feat(agent): persistMessage INSERT+UPDATE for incremental persist`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [ ] 19. **Incremental Persist at Step Boundaries**

  **What to do**:
  - After every ~10 text deltas → call persistMessageUpsert with current Parts
  - After part_update(state="done") → call persistMessageUpsert
  - After tool-call complete → call persistMessageUpsert
  - After step_create → call persistMessageUpsert with current Parts
  - TDD: Write test — verify DB has checkpoint after 10 deltas, after state=done, after tool-call

  **Must NOT do**:
  - Do NOT persist on every single delta (too many writes)
  - Do NOT change LLM streaming behavior

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`
  - **Reason**: Integration with streaming pipeline, delta counting, checkpoint logic

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on 18)
  - **Parallel Group**: Wave 3.5
  - **Blocks**: None
  - **Blocked By**: Task 18

  **References**:
  - `server/internal/agent/engine.go:195-380` — handleStreaming
  - `server/internal/agent/engine_tools.go` — tool execution
  - `docs/subagent-architecture-design.md` — §9, persist timing

  **Acceptance Criteria**:
  - [ ] Test: after 10 text deltas → DB has checkpoint with accumulated text
  - [ ] Test: after state=done → DB has final text for that part
  - [ ] Test: after tool-call complete → DB has tool output in Parts
  - [ ] `go test ./...` → no regressions

  **QA Scenarios**:

  ```
  Scenario: Checkpoint after 10 deltas
    Tool: Bash (go test)
    Steps:
      1. Simulate LLM stream: emit 15 text deltas ("Hello", " ", "World", ...)
      2. After 10th delta → assert persistMessageUpsert was called
      3. After 15th delta + state=done → assert persistMessageUpsert called again
      4. Verify DB Parts after all: text = full accumulated string
    Expected Result: Checkpoints at correct intervals
    Evidence: .omo/evidence/task-19-checkpoint.txt

  Scenario: Checkpoint after tool-call
    Tool: Bash (go test)
    Steps:
      1. Simulate tool-call execution
      2. Before execution: part_create(tool-call, pending) → no persist needed
      3. After execution: part_update(complete, output="...") → persistMessageUpsert called
      4. Verify DB Parts include tool-call with output
    Expected Result: Tool result persisted at completion
    Evidence: .omo/evidence/task-19-tool-persist.txt
  ```

  **Evidence to Capture**:
  - [ ] Go test output

  **Commit**: YES
  - Message: `feat(agent): incremental persist at step boundaries`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./... -v`

---

## Final Verification Wave

  **What to do**:
  - Implement `read_sub_session` tool that retrieves messages from a sub-session
  - Tool receives `sub_session_id` parameter
  - Returns UIMessage list for the sub-session (same format as GET /messages)
  - Register in tool registry
  - TDD: Write test first — verify tool returns correct messages, handles nonexistent sub-session

  **Must NOT do**:
  - Do NOT expose raw DB rows — return UIMessage format
  - Do NOT allow reading non-child sessions

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES (with F2-F4)
  - **Parallel Group**: Wave FINAL
  - **Blocks**: None
  - **Blocked By**: Task 21 (needs sub-session infrastructure)

  **References**:
  - `docs/subagent-architecture-design.md` — §7.4, read_sub_session design
  - `server/internal/tools/` — existing tool implementations for pattern
  - `server/internal/session/manager.go` — GetMessages for sub-session message retrieval
  - `server/internal/chat_context/manager.go` — BuildContext/GetHistory

  **Acceptance Criteria**:
  - [ ] Test file created: `server/internal/tools/subagent_test.go` — `TestReadSubSession` PASS
  - [ ] `go test ./internal/tools/... -run TestReadSubSession` → PASS
  - [ ] `go test ./...` → no regressions

  **QA Scenarios**:

  ```
  Scenario: Read existing sub-session messages
    Tool: Bash (curl + go test)
    Preconditions: Sub-session with 3 messages exists in DB
    Steps:
      1. Insert test sub-session with parent_session_id
      2. Insert 3 messages (user, assistant, assistant) into sub-session
      3. Call read_sub_session.Execute(sub_session_id)
      4. Assert result.Success == true
      5. Assert len(result.Data.messages) == 3
    Expected Result: 3 UIMessages returned with correct role/content
    Failure Indicators: Wrong count, missing messages, error on valid sub-session
    Evidence: .omo/evidence/task-F1-happy-path.txt

  Scenario: Nonexistent sub-session
    Tool: Bash (curl + go test)
    Preconditions: No session with given UUID exists
    Steps:
      1. Generate random UUID
      2. Call read_sub_session.Execute(random_uuid)
      3. Assert result.Success == false
      4. Assert error contains "not found"
    Expected Result: Error returned with clear message
    Evidence: .omo/evidence/task-F1-error-path.txt
  ```

  **Evidence to Capture**:
  - [ ] Go test output for both scenarios

  **Commit**: YES
  - Message: `feat(tools): add read_sub_session tool`
  - Files: `server/internal/tools/subagent.go`
  - Pre-commit: `go test ./internal/tools/... -v`

- [ ] F2. **Plan Compliance Audit** — `oracle`

  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, curl endpoint, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .omo/evidence/. Compare deliverables against plan.

  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F3. **Code Quality Review** — `unspecified-high`

  Run `go vet ./...` + `[pnpm build]`. Review all changed files for: `any`/`@ts-ignore`, empty catches, console.log in prod, commented-out code. Check AI slop: excessive comments, over-abstraction, generic names.

  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | VERDICT`

- [ ] F4. **Real QA + Scope Check** — `unspecified-high`

  Start from clean state. Execute EVERY QA scenario from EVERY task. Test cross-task integration (delegate_to → sub-session → SSE → frontend). Test edge cases: empty state, invalid agent_id, depth=3, step limit. Save to `.omo/evidence/final-qa/`.

  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

---

## Commit Strategy

- **1**: `feat(chatctx): add Close/Closed/Depth to ChatContextInterface` — iface/chat.go
- **2**: `feat(chatctx): ringbuf-based event distribution` — chat_context/ringbuf.go
- **3**: `feat(chatctx): SessionAgentStore` — chat_context/store.go
- **4**: `feat(api): unified SSE handler with reconnect` — api/handlers.go
- **5**: `feat(agent): step limit + ringbuf integration` — agent/engine.go
- **6**: `feat(db): add parent_session_id to sessions` — migration file
- **7**: `feat(agent): AgentDefinition.Hooks + hook composition` — agent/definition.go, agent/engine.go
- **8**: `feat(agent): factory-based AgentRegistry` — agent/registry.go
- **9**: `feat(config): deprecate config-based agents` — config/, main.go
- **10**: `feat(ui): AgentClient reconnect + XRequest spike` — api/agentClient.ts
- **11**: `feat(ui): useAgentChat reconnect logic` — hooks/useAgentChat.ts
- **13**: `feat(tools): delegate_to tool implementation` — tools/delegate.go
- **14**: `feat(session): sub-session creation` — session/manager.go
- **15**: `feat(agent): sub-agent spawning + depth check` — agent/engine.go
- **16**: `feat(api): 204 for reconnect on finished agents` — api/handlers.go
- **17**: `feat(tools): register delegate_to in engine` — agent/registry.go
- **18**: `feat(agent): persistMessage INSERT+UPDATE redesign` — agent/engine.go
- **19**: `feat(agent): incremental persist at step boundaries` — agent/engine.go
- **20**: `feat(ui): SubagentCard component` — components/SubagentCard/
- **21**: `feat(ui): sub-session SSE connection hook` — hooks/useSubagentSSE.ts

---

## Success Criteria

### Verification Commands
```bash
# Go backend
go test ./... -v                    # Expected: all pass
go vet ./...                        # Expected: no issues

# Frontend
pnpm build                          # Expected: no errors (after all frontend tasks)

# Integration (manual verification)
# Send message that triggers delegate_to → verify sub-session created
# Disconnect SSE mid-stream → reconnect → verify no message loss
# Trigger depth=3 nesting → verify 4th level rejected
# Trigger 50-step loop → verify step limit terminates with error
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass
- [ ] ringbuf works with Go 1.26
- [ ] execution_mode collision resolved in delegate_to schema
- [ ] Incremental persist uses INSERT+UPDATE pattern (not INSERT-only)
- [ ] SessionAgentStore is concurrency-safe
- [ ] ChatContextInterface has Close()/Closed()/Depth()
- [ ] Frontend reconnect works with XRequest
- [ ] Subagent depth = 3 rejects further nesting