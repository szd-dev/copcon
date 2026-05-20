# Async Tool Execution Implementation Plan

## TL;DR

> **Quick Summary**: Implement three execution modes (sync/concurrent/async) with model-controlled parameters, unified tool description injection, async lifecycle management for SSE-online and SSE-offline scenarios, and specialized tools for async task control.
> 
> **Deliverables**:
> - AsyncToolRegistry (in-memory state tracking)
> - Execution mode dispatcher (sync/concurrent/async)
> - Tool description injection (execution_mode parameter)
> - Specialized tools (get_tool_status, get_tool_result, cancel_tool, list_async_tools)
> - Async lifecycle handling (SSE-online + SSE-offline)
> - Frontend polling API (GET /updates)
> - prepareAgentLoop optimization (empty input handling)
> - Event system extension (EventAsyncToolStarted/Complete/Failed)
> - Concurrency control (Semaphore, max 5)
> 
> **Estimated Effort**: Large
> **Parallel Execution**: YES - 5 waves
> **Critical Path**: Registry → Engine → Tools → API → Tests

---

## Context

### Original Request
实现工具调用的异步执行，支持并行执行和长时间运行任务，由模型自主决策执行模式，支持 SSE 断开后的结果处理。

### Interview Summary
**Key Discussions**:
- **执行模式**: sync / concurrent / async，由模型通过参数指定
- **工具描述**: 统一注入 execution_mode 参数，所有工具支持三种模式
- **异步结果持久化**: 持久化到 session.Message 表
- **SSE 在线通知**: EventAsyncToolComplete 事件推送
- **SSE 断开通知**: 前端轮询 + 空输入会话触发
- **专用工具**: get_tool_status, get_tool_result, cancel_tool, list_async_tools
- **生命周期**: Session 绑定 + 5 分钟超时 + 手动取消
- **代码优化**: 复用 prepareAgentLoop，通过判断处理空输入

**Research Findings**:
- **Current Architecture**: engine.go:436 顺序执行，Tool 接口同步
- **Industry Patterns**: OpenAI (requires_action), LangChain (AsyncBackgroundExecutor)
- **Go Patterns**: Semaphore, Context timeout layers, sync.Map for tracking
- **Validated**: Tool call ID 来自 OpenAI，ChatContext 线程安全

### Metis Review
**Identified Gaps** (addressed):
- **Gap 1**: 模型如何知道执行模式 → 统一注入 execution_mode 参数到工具描述
- **Gap 2**: SSE 断开后如何更新 → 前端轮询 + 空输入会话机制
- **Gap 3**: 代码重复问题 → 复用 prepareAgentLoop，通过判断处理空输入

**Guardrails Applied**:
- MUST inject execution_mode parameter in GetOpenAITools
- MUST support empty input in prepareAgentLoop
- MUST persist results immediately to session.Message
- MUST NOT create separate ChatWithoutUserInput method
- MUST use OpenAI's tool call ID (tc.ID) - no new ID generation
- MUST handle panics in async tool goroutines
- MUST NOT allow goroutine leaks

---

## Work Objectives

### Core Objective
Implement three execution modes (sync/concurrent/async) with model-controlled parameters, unified tool description injection, async lifecycle management for both SSE-online and SSE-offline scenarios, while maintaining backward compatibility and minimal code duplication.

### Concrete Deliverables
- `server/internal/tool/registry.go` - AsyncToolRegistry with sync.Map
- `server/internal/agent/engine.go` - Execution mode dispatcher
- `server/internal/tool/manager.go` - Tool description injection
- `server/internal/tools/async_tools.go` - Specialized tools implementation
- `server/internal/api/handlers.go` - Frontend polling endpoint
- `server/internal/domain/entity/event.go` - Async event types
- `server/internal/session/manager.go` - Session cleanup integration
- Test files for all components

### Definition of Done
- [ ] All tools support three execution modes (sync/concurrent/async)
- [ ] Tool descriptions include execution_mode parameter
- [ ] Concurrent execution respects semaphore limit (max 5)
- [ ] Async tools execute in background without blocking main loop
- [ ] Async results persisted to session.Message
- [ ] SSE-online: EventAsyncToolComplete pushed to client
- [ ] SSE-offline: Frontend polling discovers pending events
- [ ] Empty input triggers new session round without duplication
- [ ] Specialized tools work correctly (status/result/cancel/list)
- [ ] Session cleanup cancels all async tasks
- [ ] No goroutine leaks after execution/cancellation
- [ ] All tests pass with `go test -race`

### Must Have
- Three execution modes (sync/concurrent/async) with model control
- Unified execution_mode parameter in tool descriptions
- AsyncToolRegistry for in-memory state tracking
- Specialized tools for async task control
- Async lifecycle handling (SSE-online + SSE-offline)
- Frontend polling API for pending events
- prepareAgentLoop empty input support
- Session binding and cleanup
- 5-minute timeout for async tasks
- Panic recovery in async goroutines

### Must NOT Have (Guardrails)
- Separate ChatWithoutUserInput method (use prepareAgentLoop)
- External HTTP API for status/cancel (use tools instead)
- Database persistence of async task state
- New ID generation for tool calls (use OpenAI's tc.ID)
- External dependencies beyond golang.org/x/sync
- Changes to existing Tool interface signature
- Changes to existing tool implementations
- Goroutine leaks on session termination

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES (go test with testify)
- **Automated tests**: Tests-after
- **Framework**: go test
- **Coverage**: Unit tests for registry, integration tests for engine, API tests for endpoints

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Backend/Go**: Use Bash (go test) - Run tests, check coverage, verify race conditions
- **API**: Use Bash (curl) - Send requests, assert status + response fields

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately - foundation):
├── Task 1: Create AsyncToolRegistry struct [quick]
├── Task 2: Add async event types [quick]
├── Task 3: Add semaphore to AgentEngine [quick]
└── Task 4: Extend prepareAgentLoop for empty input [quick]

Wave 2 (After Wave 1 - tool description):
├── Task 5: Inject execution_mode parameter [quick]
└── Task 6: Parse execution_mode in engine [quick]

Wave 3 (After Wave 2 - execution logic):
├── Task 7: Implement sync execution [quick]
├── Task 8: Implement concurrent execution [unspecified-high]
├── Task 9: Implement async execution [unspecified-high]
└── Task 10: Add panic recovery wrapper [quick]

Wave 4 (After Wave 3 - specialized tools + API):
├── Task 11: Implement get_tool_status tool [quick]
├── Task 12: Implement get_tool_result tool [quick]
├── Task 13: Implement cancel_tool tool [quick]
├── Task 14: Implement list_async_tools tool [quick]
└── Task 15: Add frontend polling API [quick]

Wave 5 (After Wave 4 - integration + cleanup):
├── Task 16: Integrate Session cleanup [quick]
├── Task 17: Add async completion persistence [quick]
└── Task 18: Add timeout handling [quick]

Wave 6 (After Wave 5 - tests):
├── Task 19: Write registry unit tests [quick]
├── Task 20: Write execution mode tests [unspecified-high]
├── Task 21: Write specialized tools tests [quick]
└── Task 22: Write integration tests [unspecified-high]

Wave FINAL (After ALL tasks — 4 parallel reviews, then user okay):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)
-> Present results -> Get explicit user okay

Critical Path: Task 1 → Task 5 → Task 7-9 → Task 11-14 → Task 16 → Task 19-22 → F1-F4 → user okay
Parallel Speedup: ~65% faster than sequential
Max Concurrent: 4 (Waves 1, 3, 4)
```

### Dependency Matrix

- **1-4**: - - 5-10, 1
- **5**: 1 - 6, 2
- **6**: 5 - 7-10, 3
- **7**: 3, 6 - 11-14, 4
- **8**: 1, 3, 6 - 11-14, 5
- **9**: 1, 3, 6 - 11-14, 5
- **10**: 3 - 7-9, 6
- **11-14**: 1, 7-10 - 15, 7
- **15**: 9, 17 - 19-22, 8
- **16**: 1 - 19-22, 9
- **17**: 9 - 15, 10
- **18**: 1, 9 - 19-22, 11
- **19**: 1, 16, 18 - 20-22, 12
- **20**: 7-10, 16 - 21, 22, 13
- **21**: 11-14 - 22, 14
- **22**: 11-14, 16, 19-21 - 15, 15

### Agent Dispatch Summary

- **1**: **4** - T1 → `quick`, T2 → `quick`, T3 → `quick`, T4 → `quick`
- **2**: **2** - T5 → `quick`, T6 → `quick`
- **3**: **4** - T7 → `quick`, T8 → `unspecified-high`, T9 → `unspecified-high`, T10 → `quick`
- **4**: **5** - T11 → `quick`, T12 → `quick`, T13 → `quick`, T14 → `quick`, T15 → `quick`
- **5**: **3** - T16 → `quick`, T17 → `quick`, T18 → `quick`
- **6**: **4** - T19 → `quick`, T20 → `unspecified-high`, T21 → `quick`, T22 → `unspecified-high`
- **FINAL**: **4** - F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

> Implementation + Test = ONE Task. Never separate.
> EVERY task MUST have: Recommended Agent Profile + Parallelization info + QA Scenarios.

- [x] 1. **Create AsyncToolRegistry struct**

  **What to do**:
  - Create `server/internal/tool/registry.go`
  - Define `AsyncToolState` struct: CallID, ToolName, Status, StartTime, EndTime, Result, Error, SessionID, CancelFunc
  - Define `AsyncToolRegistry` struct with `sync.Map` for concurrent-safe state tracking
  - Implement methods: `Register(sessionID, callID, cancelFunc)`, `Unregister(callID)`, `Complete(callID, result)`, `Fail(callID, error)`, `GetStatus(callID)`, `Cancel(callID)`, `CancelSession(sessionID)`
  - Follow pattern from `server/internal/tool/manager.go:47-56` (sync.RWMutex usage)
  
  **Must NOT do**:
  - Persist state to database (in-memory only)
  - Generate new IDs (use provided callID from OpenAI)
  - Add complex status fields beyond lifecycle states

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Well-defined struct creation following existing patterns
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3, 4)
  - **Blocks**: Tasks 8, 9, 11-14, 16, 18, 19
  - **Blocked By**: None (can start immediately)

  **References**:
  - `server/internal/tool/manager.go:47-56` - Sync mutex pattern for concurrent access
  - `server/internal/domain/entity/event.go:34-48` - ToolCallData structure for call ID reference
  - `golang.org/x/sync/semaphore` documentation - Semaphore pattern

  **Acceptance Criteria**:
  - [ ] registry.go created with AsyncToolRegistry struct
  - [ ] sync.Map used for concurrent-safe storage
  - [ ] Register stores cancel function, session ID, and start time
  - [ ] Complete persists result and updates status
  - [ ] Fail records error and updates status
  - [ ] Cancel calls stored cancel function and removes from map
  - [ ] CancelSession cancels all tools for a session
  - [ ] GetStatus returns correct state or error for unknown call ID

  **QA Scenarios**:
  ```
  Scenario: Registry concurrent access
    Tool: Bash (go test)
    Preconditions: registry.go implemented
    Steps:
      1. Run: go test ./internal/tool/registry_test.go -race -v
      2. Check output for race condition detection
    Expected Result: No race conditions detected, all tests pass
    Failure Indicators: "DATA RACE" in output, test failures
    Evidence: .sisyphus/evidence/task-1-registry-race.txt
  
  Scenario: Registry session cleanup
    Tool: Bash (go test)
    Preconditions: CancelSession method implemented
    Steps:
      1. Run: go test ./internal/tool/registry_test.go -run TestCancelSession -v
      2. Verify all tools for session are cancelled
    Expected Result: Test passes, all tools cancelled
    Failure Indicators: Test failure, tools not cancelled
    Evidence: .sisyphus/evidence/task-1-registry-session-cleanup.txt
  ```

  **Evidence to Capture**:
  - [ ] registry_race.txt - Race detector output
  - [ ] registry_session_cleanup.txt - Session cleanup test output

  **Commit**: YES (groups with Task 2, 3, 4)
  - Message: `feat(tool): add AsyncToolRegistry for async state management`
  - Files: `server/internal/tool/registry.go`
  - Pre-commit: `go test ./internal/tool/... -race`

- [x] 2. **Add async event types**

  **What to do**:
  - Extend `server/internal/domain/entity/event.go`
  - Add `EventAsyncToolStarted` event type
  - Add `EventAsyncToolComplete` event type
  - Add `EventAsyncToolFailed` event type
  - Add `AsyncCompletionPending` event type (for frontend polling)
  - Follow existing event pattern from `event.go:8-21`
  
  **Must NOT do**:
  - Create complex event structures beyond lifecycle
  - Add progress percentage fields (out of scope)
  - Change existing event type constants

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple event type addition following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3, 4)
  - **Blocks**: Task 9
  - **Blocked By**: None (can start immediately)

  **References**:
  - `server/internal/domain/entity/event.go:8-21` - Existing event type definitions
  - `server/internal/domain/entity/event.go:34-48` - ToolCallData and ToolResultData structure

  **Acceptance Criteria**:
  - [ ] EventAsyncToolStarted defined
  - [ ] EventAsyncToolComplete defined
  - [ ] EventAsyncToolFailed defined
  - [ ] EventAsyncCompletionPending defined
  - [ ] Event emission follows existing pattern

  **QA Scenarios**:
  ```
  Scenario: Event type constants
    Tool: Bash (grep + go build)
    Preconditions: event.go updated
    Steps:
      1. grep "EventAsyncToolStarted" server/internal/domain/entity/event.go
      2. grep "EventAsyncToolComplete" server/internal/domain/entity/event.go
      3. grep "EventAsyncToolFailed" server/internal/domain/entity/event.go
      4. Run: go build ./internal/domain/entity/...
    Expected Result: All event types found, build succeeds
    Failure Indicators: grep returns empty, build errors
    Evidence: .sisyphus/evidence/task-2-events.txt
  ```

  **Commit**: YES (groups with Task 1, 3, 4)
  - Message: `feat(tool): add AsyncToolRegistry for async state management`
  - Files: `server/internal/domain/entity/event.go`
  - Pre-commit: `go build ./...`

- [x] 3. **Add semaphore to AgentEngine**

  **What to do**:
  - Modify `server/internal/agent/engine.go`
  - Add import for `golang.org/x/sync/semaphore`
  - Add `semaphore *semaphore.Weighted` field to `AgentEngine` struct
  - Initialize semaphore with limit 5 in `NewAgentEngine()` or appropriate constructor
  - Follow existing struct modification pattern
  
  **Must NOT do**:
  - Change AgentEngine initialization logic beyond semaphore addition
  - Implement execution logic yet (Task 7-9)
  - Add per-tool semaphore (global only)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple struct field addition
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 4)
  - **Blocks**: Tasks 7, 8, 9
  - **Blocked By**: None (can start immediately)

  **References**:
  - `server/internal/agent/engine.go:60-80` - AgentEngine struct definition
  - `golang.org/x/sync/semaphore` documentation - Semaphore usage

  **Acceptance Criteria**:
  - [ ] semaphore import added
  - [ ] semaphore field added to AgentEngine struct
  - [ ] Semaphore initialized with limit 5
  - [ ] Build succeeds without errors

  **QA Scenarios**:
  ```
  Scenario: Semaphore initialization
    Tool: Bash (go build)
    Preconditions: engine.go modified
    Steps:
      1. grep "semaphore.NewWeighted(5)" server/internal/agent/engine.go
      2. Run: go build ./internal/agent/...
    Expected Result: Pattern found, build succeeds
    Failure Indicators: grep empty, build errors
    Evidence: .sisyphus/evidence/task-3-semaphore.txt
  ```

  **Commit**: YES (groups with Task 1, 2, 4)
  - Message: `feat(tool): add AsyncToolRegistry for async state management`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go build ./...`

- [x] 4. **Extend prepareAgentLoop for empty input**

  **What to do**:
  - Modify `server/internal/agent/engine.go` `prepareAgentLoop` method
  - Add conditional check: if `userContent != ""`, persist user message
  - If `userContent == ""`, skip user message persistence and continue
  - Add comment explaining the async completion trigger scenario
  - Keep all subsequent logic unchanged
  
  **Must NOT do**:
  - Create a separate method for empty input
  - Change any logic after the user message persistence
  - Add validation to reject empty input (frontend handles this)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple conditional addition with clear pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 3)
  - **Blocks**: Tasks 7, 9
  - **Blocked By**: None (can start immediately)

  **References**:
  - `server/internal/agent/engine.go` - prepareAgentLoop method location
  - `server/internal/context/manager.go` - AddMessage pattern

  **Acceptance Criteria**:
  - [ ] Conditional check for empty userContent added
  - [ ] User message persistence skipped when empty
  - [ ] Subsequent logic unchanged
  - [ ] Comment added explaining async completion scenario
  - [ ] Build succeeds

  **QA Scenarios**:
  ```
  Scenario: Empty input handling
    Tool: Bash (go test)
    Preconditions: prepareAgentLoop modified
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestPrepareAgentLoop -v
      2. Verify empty input doesn't persist user message
    Expected Result: Test passes, no user message persisted for empty input
    Failure Indicators: Test failure, user message persisted
    Evidence: .sisyphus/evidence/task-4-empty-input.txt
  ```

  **Commit**: YES (groups with Task 1, 2, 3)
  - Message: `feat(tool): add AsyncToolRegistry for async state management`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [x] 5. **Inject execution_mode parameter**

  **What to do**:
  - Modify `server/internal/tool/manager.go` `GetOpenAITools()` method
  - For each tool's InputSchema, inject `execution_mode` parameter
  - Parameter definition: `{type: "string", enum: ["sync", "concurrent", "async"], default: "sync", description: "..."}`
  - Add to properties map, not required array (uses default)
  - Ensure all tools get the parameter uniformly
  
  **Must NOT do**:
  - Add parameter to individual tool definitions (inject at framework level)
  - Make parameter required (use default value)
  - Modify existing tool InputSchema definitions

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple schema manipulation following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Task 6)
  - **Blocks**: Task 6
  - **Blocked By**: Task 1

  **References**:
  - `server/internal/tool/manager.go` - GetOpenAITools method
  - OpenAI Tool Definition format documentation

  **Acceptance Criteria**:
  - [ ] execution_mode parameter injected for all tools
  - [ ] Parameter has correct enum values and default
  - [ ] Description clearly explains three modes
  - [ ] Parameter not in required array
  - [ ] OpenAI tool definitions generated correctly

  **QA Scenarios**:
  ```
  Scenario: Tool description verification
    Tool: Bash (go test)
    Preconditions: GetOpenAITools modified
    Steps:
      1. Run: go test ./internal/tool/manager_test.go -run TestGetOpenAITools -v
      2. Verify all tools have execution_mode parameter
    Expected Result: Test passes, all tools have parameter
    Failure Indicators: Missing parameter, wrong format
    Evidence: .sisyphus/evidence/task-5-tool-description.txt
  
  Scenario: Parameter format verification
    Tool: Bash (go run)
    Preconditions: Tool definitions generated
    Steps:
      1. Create test script to print tool definitions as JSON
      2. Run and inspect output
    Expected Result: execution_mode in all tools with correct format
    Failure Indicators: Missing parameter, wrong enum values
    Evidence: .sisyphus/evidence/task-5-tool-definitions.json
  ```

  **Commit**: YES (groups with Task 6)
  - Message: `feat(agent): add execution mode support (sync/concurrent/async)`
  - Files: `server/internal/tool/manager.go`
  - Pre-commit: `go test ./internal/tool/...`

- [x] 6. **Parse execution_mode in engine**

  **What to do**:
  - Modify `server/internal/agent/engine.go` `executeToolCall` method (or create new dispatcher)
  - Extract `execution_mode` from tool arguments JSON
  - Default to "sync" if not present
  - Remove `execution_mode` from args map before passing to tool
  - Dispatch to appropriate execution method based on mode
  
  **Must NOT do**:
  - Pass execution_mode to tool Execute method (remove before calling)
  - Fail if execution_mode missing (use default)
  - Validate execution_mode values (trust model output)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple JSON parsing and dispatch logic
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Task 5)
  - **Blocks**: Tasks 7, 8, 9, 10
  - **Blocked By**: Task 5

  **References**:
  - `server/internal/agent/engine.go:315-358` - executeToolCall method
  - `encoding/json` Unmarshal documentation

  **Acceptance Criteria**:
  - [ ] execution_mode extracted from arguments
  - [ ] Default value "sync" used when missing
  - [ ] execution_mode removed from args before tool call
  - [ ] Dispatch logic routes to correct method (sync/concurrent/async)
  - [ ] Build succeeds

  **QA Scenarios**:
  ```
  Scenario: Mode parsing
    Tool: Bash (go test)
    Preconditions: Parsing logic implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestParseExecutionMode -v
      2. Test with all three modes and missing mode
    Expected Result: Correct mode extracted, default used when missing
    Failure Indicators: Wrong mode, crash on missing mode
    Evidence: .sisyphus/evidence/task-6-mode-parsing.txt
  
  Scenario: Parameter removal
    Tool: Bash (go test)
    Preconditions: Parameter removal implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestExecutionModeRemoved -v
      2. Verify execution_mode not in args passed to tool
    Expected Result: Test passes, parameter removed
    Failure Indicators: Parameter still in args
    Evidence: .sisyphus/evidence/task-6-param-removal.txt
  ```

  **Commit**: YES (groups with Task 5)
  - Message: `feat(agent): add execution mode support (sync/concurrent/async)`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [x] 7. **Implement sync execution**

  **What to do**:
  - Add `executeSync` method to AgentEngine (or keep existing behavior as default)
  - This is the baseline: sequential tool execution, main loop waits for completion
  - Emit EventToolCall (start), EventToolResult (complete)
  - Persist result to session.Message using contextMgr.AddMessage
  - Follow existing `executeToolCall` pattern from `engine.go:315-358`
  
  **Must NOT do**:
  - Change existing behavior for tools without execution_mode (backward compatibility)
  - Add concurrency control (this is sequential)
  - Implement parallel or async logic here

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: This is existing behavior, just formalize as a method
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 8, 9, 10)
  - **Blocks**: Tasks 11-14
  - **Blocked By**: Tasks 3, 6

  **References**:
  - `server/internal/agent/engine.go:315-358` - Current executeToolCall implementation
  - `server/internal/agent/engine.go:348-357` - Result persistence pattern

  **Acceptance Criteria**:
  - [ ] executeSync method created (or existing logic preserved)
  - [ ] Tool executes sequentially
  - [ ] Main loop waits for completion
  - [ ] Result persisted to session.Message
  - [ ] Events emitted correctly
  - [ ] Backward compatible with existing behavior

  **QA Scenarios**:
  ```
  Scenario: Sync execution
    Tool: Bash (go test)
    Preconditions: executeSync implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestSyncExecution -v
      2. Verify sequential execution and result persistence
    Expected Result: Test passes, tool executes sequentially
    Failure Indicators: Parallel execution, missing persistence
    Evidence: .sisyphus/evidence/task-7-sync-execution.txt
  ```

  **Commit**: YES (groups with Tasks 8, 9, 10)
  - Message: `feat(agent): implement execution mode dispatcher (sync/concurrent/async)`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [x] 8. **Implement concurrent execution**

  **What to do**:
  - Add `executeConcurrent` method to AgentEngine
  - Receive multiple tool calls to execute concurrently
  - Use semaphore to limit concurrency (max 5)
  - Use errgroup or WaitGroup for coordination
  - Collect results with mutex protection
  - Wait for all to complete before returning
  - Order results by tool call ID
  - Persist results in order to session.Message
  - Follow gollem pattern: `agent.go:1064-1113`
  
  **Must NOT do**:
  - Fail entire batch on single error (continue execution)
  - Return before all tools complete
  - Allow more than 5 concurrent executions
  - Lose result ordering

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Complex concurrent logic requiring careful implementation
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 7, 9, 10)
  - **Blocks**: Tasks 11-14
  - **Blocked By**: Tasks 1, 3, 6

  **References**:
  - `server/internal/agent/engine.go:426-455` - Current sequential execution to replace
  - Gollem `agent.go:1064-1113` - Semaphore + WaitGroup pattern
  - `golang.org/x/sync/errgroup` - Error group pattern

  **Acceptance Criteria**:
  - [ ] executeConcurrent method created
  - [ ] Semaphore limits to max 5 concurrent tools
  - [ ] All tools execute concurrently (verified by overlapping timestamps)
  - [ ] One failure doesn't stop others
  - [ ] Results collected with mutex protection
  - [ ] Results ordered by tool call ID
  - [ ] All results persisted to session.Message
  - [ ] Events emitted for each tool (start, complete, failed)

  **QA Scenarios**:
  ```
  Scenario: Concurrent execution
    Tool: Bash (go test)
    Preconditions: executeConcurrent implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestConcurrentExecution -v
      2. Verify timestamps overlap, results ordered correctly
    Expected Result: Test passes, tools execute concurrently
    Failure Indicators: Sequential timestamps, wrong ordering
    Evidence: .sisyphus/evidence/task-8-concurrent-execution.txt
  
  Scenario: Concurrency limit
    Tool: Bash (go test)
    Preconditions: Semaphore limit 5
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestConcurrencyLimit -v
      2. Submit 10 tools, verify max 5 concurrent
    Expected Result: Max 5 concurrent tools observed
    Failure Indicators: More than 5 concurrent
    Evidence: .sisyphus/evidence/task-8-concurrency-limit.txt
  
  Scenario: Error isolation
    Tool: Bash (go test)
    Preconditions: Error handling implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestErrorIsolation -v
      2. One tool fails, verify others complete
    Expected Result: All tools complete despite one failure
    Failure Indicators: Early termination
    Evidence: .sisyphus/evidence/task-8-error-isolation.txt
  ```

  **Commit**: YES (groups with Tasks 7, 9, 10)
  - Message: `feat(agent): implement execution mode dispatcher (sync/concurrent/async)`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [x] 9. **Implement async execution**

  **What to do**:
  - Add `executeAsync` method to AgentEngine
  - Register tool in AsyncToolRegistry with SessionID, CallID, CancelFunc
  - Create context with cancel and 5-minute timeout
  - Launch goroutine to execute tool in background
  - Emit EventAsyncToolStarted immediately
  - **Main loop returns immediately** (does NOT wait)
  - In goroutine:
    - Execute tool with context
    - On completion: Persist result to session.Message, update registry, emit EventAsyncToolComplete
    - On error: Persist error to session.Message, update registry, emit EventAsyncToolFailed
    - If SSE disconnected: Record async_completion_pending event in session.metadata
  - Use panic recovery wrapper (Task 10)
  
  **Must NOT do**:
  - Block main loop waiting for tool completion
  - Forget to unregister from registry on completion
  - Allow goroutine leaks (use defer cleanup)
  - Skip result persistence

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Complex async lifecycle with SSE-online and SSE-offline scenarios
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 7, 8, 10)
  - **Blocks**: Tasks 11-14, 15, 17
  - **Blocked By**: Tasks 1, 3, 6

  **References**:
  - `server/internal/agent/engine.go:315-358` - Tool execution pattern
  - `server/internal/tool/registry.go` - AsyncToolRegistry
  - `server/internal/domain/entity/event.go` - Async event types
  - Draft document async lifecycle scenarios A and B

  **Acceptance Criteria**:
  - [ ] executeAsync method created
  - [ ] Tool registered in AsyncToolRegistry
  - [ ] Goroutine launched, main loop returns immediately
  - [ ] EventAsyncToolStarted emitted
  - [ ] On completion: result persisted, registry updated, EventAsyncToolComplete emitted
  - [ ] On error: error persisted, registry updated, EventAsyncToolFailed emitted
  - [ ] On SSE disconnect: async_completion_pending recorded
  - [ ] No goroutine leaks (verified by test)
  - [ ] 5-minute timeout enforced

  **QA Scenarios**:
  ```
  Scenario: Async execution - main loop continues
    Tool: Bash (go test)
    Preconditions: executeAsync implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestAsyncMainLoopContinues -v
      2. Verify main loop returns before tool completes
    Expected Result: Main loop returns immediately
    Failure Indicators: Main loop waits for tool
    Evidence: .sisyphus/evidence/task-9-async-main-loop.txt
  
  Scenario: Async completion - SSE online
    Tool: Bash (go test)
    Preconditions: Completion logic implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestAsyncCompletionSSEOnline -v
      2. Verify event emitted, result persisted
    Expected Result: EventAsyncToolComplete emitted, result in session.Message
    Failure Indicators: Missing event or persistence
    Evidence: .sisyphus/evidence/task-9-async-completion-online.txt
  
  Scenario: Async completion - SSE offline
    Tool: Bash (go test)
    Preconditions: Pending event recording implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestAsyncCompletionSSEOffline -v
      2. Verify pending event recorded in session.metadata
    Expected Result: async_completion_pending recorded
    Failure Indicators: Missing pending event
    Evidence: .sisyphus/evidence/task-9-async-completion-offline.txt
  
  Scenario: Goroutine leak prevention
    Tool: Bash (go test)
    Preconditions: Cleanup implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestAsyncGoroutineLeak -v
      2. Verify no goroutine leaks after completion
    Expected Result: No goroutine leaks detected
    Failure Indicators: Increased goroutine count
    Evidence: .sisyphus/evidence/task-9-async-goroutine-leak.txt
  ```

  **Commit**: YES (groups with Tasks 7, 8, 10)
  - Message: `feat(agent): implement execution mode dispatcher (sync/concurrent/async)`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [x] 10. **Add panic recovery wrapper**

  **What to do**:
  - Create `safeExecuteTool` function in `server/internal/agent/engine.go`
  - Wrap tool execution with `defer func() { if r := recover() {...} }`
  - On panic: emit EventAsyncToolFailed with panic message, unregister from registry
  - Log panic details for debugging
  - Return error result to caller
  - Follow gollem pattern: `agent.go:1516-1525`
  
  **Must NOT do**:
  - Crash the server on tool panic
  - Lose panic information (include stack trace in logs)
  - Skip registry cleanup on panic

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple defer pattern following existing gollem implementation
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 7, 8, 9)
  - **Blocks**: None
  - **Blocked By**: Task 3

  **References**:
  - Gollem `agent.go:1516-1525` - safeCallHandler with panic recovery
  - `server/internal/domain/entity/event.go` - EventAsyncToolFailed type

  **Acceptance Criteria**:
  - [ ] safeExecuteTool wrapper function created
  - [ ] Panic caught and converted to error
  - [ ] EventAsyncToolFailed emitted with panic message
  - [ ] Registry cleanup happens on panic
  - [ ] Server doesn't crash on tool panic
  - [ ] Stack trace logged for debugging

  **QA Scenarios**:
  ```
  Scenario: Panic recovery
    Tool: Bash (go test)
    Preconditions: safeExecuteTool implemented
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestPanicRecovery -v
      2. Verify panicked tool doesn't crash server
    Expected Result: Test passes, panic converted to failed event
    Failure Indicators: Server crash, missing failure event
    Evidence: .sisyphus/evidence/task-10-panic-recovery.txt
  ```

  **Commit**: YES (groups with Tasks 7, 8, 9)
  - Message: `feat(agent): implement execution mode dispatcher (sync/concurrent/async)`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [x] 11. **Implement get_tool_status tool**

  **What to do**:
  - Create `server/internal/tools/async_tools.go`
  - Define `GetToolStatusTool` struct implementing Tool interface
  - Name: "get_tool_status"
  - Description: "查询异步工具的执行状态。用于检查后台任务是否完成。"
  - InputSchema: `{"call_id": {"type": "string", "description": "工具调用的唯一标识符"}}`
  - Execute: Query AsyncToolRegistry.GetStatus(call_id), return status JSON
  - Register in tool manager
  
  **Must NOT do**:
  - Query database for status (use in-memory registry)
  - Fail if tool not found (return error status)
  - Include sensitive information (only status, start_time, duration)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple tool implementation following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with Tasks 12, 13, 14, 15)
  - **Blocks**: Tasks 19, 20, 21
  - **Blocked By**: Tasks 1, 7, 8, 9, 10

  **References**:
  - `server/internal/tools/todo_tool.go` - Example tool implementation
  - `server/internal/tool/registry.go` - AsyncToolRegistry to query

  **Acceptance Criteria**:
  - [ ] GetToolStatusTool created
  - [ ] Tool registered in manager
  - [ ] Returns correct status for running/completed/failed/cancelled tools
  - [ ] Returns error for unknown call ID
  - [ ] Response includes: call_id, tool_name, status, start_time, duration

  **QA Scenarios**:
  ```
  Scenario: Get status - running tool
    Tool: Bash (go test)
    Preconditions: Tool implemented, async tool running
    Steps:
      1. Run: go test ./internal/tools/async_tools_test.go -run TestGetToolStatusRunning -v
      2. Verify status is "running"
    Expected Result: Test passes, status is "running"
    Failure Indicators: Wrong status, missing fields
    Evidence: .sisyphus/evidence/task-11-get-status-running.txt
  
  Scenario: Get status - completed tool
    Tool: Bash (go test)
    Preconditions: Tool implemented, async tool completed
    Steps:
      1. Run: go test ./internal/tools/async_tools_test.go -run TestGetToolStatusCompleted -v
      2. Verify status is "completed", duration present
    Expected Result: Test passes, status is "completed"
    Failure Indicators: Wrong status, missing duration
    Evidence: .sisyphus/evidence/task-11-get-status-completed.txt
  ```

  **Commit**: YES (groups with Tasks 12, 13, 14)
  - Message: `feat(tools): add specialized async control tools`
  - Files: `server/internal/tools/async_tools.go`
  - Pre-commit: `go test ./internal/tools/...`

- [x] 12. **Implement get_tool_result tool**

  **What to do**:
  - Add to `server/internal/tools/async_tools.go`
  - Define `GetToolResultTool` struct implementing Tool interface
  - Name: "get_tool_result"
  - Description: "获取已完成异步工具的执行结果。仅在工具状态为 completed 时可用。"
  - InputSchema: `{"call_id": {"type": "string", "description": "工具调用的唯一标识符"}}`
  - Execute: Query AsyncToolRegistry.GetStatus, if completed return result, else return error
  - Register in tool manager
  
  **Must NOT do**:
  - Return result for running tools (only completed)
  - Return result for failed tools (return error instead)
  - Include internal implementation details

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple tool implementation following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with Tasks 11, 13, 14, 15)
  - **Blocks**: Tasks 21
  - **Blocked By**: Tasks 1, 7, 8, 9, 10

  **References**:
  - `server/internal/tools/todo_tool.go` - Example tool implementation
  - `server/internal/tool/registry.go` - AsyncToolRegistry to query

  **Acceptance Criteria**:
  - [ ] GetToolResultTool created
  - [ ] Tool registered in manager
  - [ ] Returns result for completed tools
  - [ ] Returns error for running/failed/cancelled tools
  - [ ] Returns error for unknown call ID
  - [ ] Response includes: call_id, status, result (or error)

  **QA Scenarios**:
  ```
  Scenario: Get result - completed tool
    Tool: Bash (go test)
    Preconditions: Tool implemented, async tool completed
    Steps:
      1. Run: go test ./internal/tools/async_tools_test.go -run TestGetToolResultCompleted -v
      2. Verify result is returned
    Expected Result: Test passes, result returned
    Failure Indicators: No result, error returned
    Evidence: .sisyphus/evidence/task-12-get-result-completed.txt
  
  Scenario: Get result - running tool
    Tool: Bash (go test)
    Preconditions: Tool implemented, async tool running
    Steps:
      1. Run: go test ./internal/tools/async_tools_test.go -run TestGetToolResultRunning -v
      2. Verify error returned (tool still running)
    Expected Result: Test passes, error returned
    Failure Indicators: Result returned for running tool
    Evidence: .sisyphus/evidence/task-12-get-result-running.txt
  ```

  **Commit**: YES (groups with Tasks 11, 13, 14)
  - Message: `feat(tools): add specialized async control tools`
  - Files: `server/internal/tools/async_tools.go`
  - Pre-commit: `go test ./internal/tools/...`

- [x] 13. **Implement cancel_tool tool**

  **What to do**:
  - Add to `server/internal/tools/async_tools.go`
  - Define `CancelToolTool` struct implementing Tool interface
  - Name: "cancel_tool"
  - Description: "取消正在执行的异步工具。工具会立即停止，结果不可恢复。"
  - InputSchema: `{"call_id": {"type": "string", "description": "工具调用的唯一标识符"}}`
  - Execute: Call AsyncToolRegistry.Cancel(call_id), return success/failure
  - Register in tool manager
  
  **Must NOT do**:
  - Cancel completed tools (return error)
  - Force kill processes (use context cancellation)
  - Allow cancelling tools from other sessions

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple tool implementation following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with Tasks 11, 12, 14, 15)
  - **Blocks**: Tasks 21
  - **Blocked By**: Tasks 1, 7, 8, 9, 10

  **References**:
  - `server/internal/tools/todo_tool.go` - Example tool implementation
  - `server/internal/tool/registry.go` - AsyncToolRegistry.Cancel method

  **Acceptance Criteria**:
  - [ ] CancelToolTool created
  - [ ] Tool registered in manager
  - [ ] Cancels running tools successfully
  - [ ] Returns error for completed/unknown tools
  - [ ] Context cancellation propagates to tool execution
  - [ ] Response includes: call_id, cancelled (true/false)

  **QA Scenarios**:
  ```
  Scenario: Cancel running tool
    Tool: Bash (go test)
    Preconditions: Tool implemented, async tool running
    Steps:
      1. Run: go test ./internal/tools/async_tools_test.go -run TestCancelToolRunning -v
      2. Verify tool cancelled, status updated
    Expected Result: Test passes, tool cancelled
    Failure Indicators: Tool not cancelled, wrong status
    Evidence: .sisyphus/evidence/task-13-cancel-running.txt
  
  Scenario: Cancel completed tool
    Tool: Bash (go test)
    Preconditions: Tool implemented, async tool completed
    Steps:
      1. Run: go test ./internal/tools/async_tools_test.go -run TestCancelToolCompleted -v
      2. Verify error returned (cannot cancel completed tool)
    Expected Result: Test passes, error returned
    Failure Indicators: Success returned for completed tool
    Evidence: .sisyphus/evidence/task-13-cancel-completed.txt
  ```

  **Commit**: YES (groups with Tasks 11, 12, 14)
  - Message: `feat(tools): add specialized async control tools`
  - Files: `server/internal/tools/async_tools.go`
  - Pre-commit: `go test ./internal/tools/...`

- [x] 14. **Implement list_async_tools tool**

  **What to do**:
  - Add to `server/internal/tools/async_tools.go`
  - Define `ListAsyncToolsTool` struct implementing Tool interface
  - Name: "list_async_tools"
  - Description: "列出当前会话中所有异步工具的状态。用于查看后台任务进度。"
  - InputSchema: `{}` (no parameters, uses session ID from chatCtx)
  - Execute: Query AsyncToolRegistry for all tools in current session, return list
  - Register in tool manager
  
  **Must NOT do**:
  - List tools from other sessions (only current session)
  - Include sensitive information (only status summary)
  - Fail if no async tools (return empty list)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple tool implementation following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with Tasks 11, 12, 13, 15)
  - **Blocks**: Tasks 21, 22
  - **Blocked By**: Tasks 1, 7, 8, 9, 10

  **References**:
  - `server/internal/tools/todo_tool.go` - Example tool implementation
  - `server/internal/tool/registry.go` - AsyncToolRegistry to query

  **Acceptance Criteria**:
  - [ ] ListAsyncToolsTool created
  - [ ] Tool registered in manager
  - [ ] Returns list of async tools for current session
  - [ ] Returns empty list if no async tools
  - [ ] Each item includes: call_id, tool_name, status, start_time
  - [ ] Does not include tools from other sessions

  **QA Scenarios**:
  ```
  Scenario: List async tools - multiple tools
    Tool: Bash (go test)
    Preconditions: Tool implemented, multiple async tools running
    Steps:
      1. Run: go test ./internal/tools/async_tools_test.go -run TestListAsyncTools -v
      2. Verify all tools listed with correct info
    Expected Result: Test passes, all tools listed
    Failure Indicators: Missing tools, wrong info
    Evidence: .sisyphus/evidence/task-14-list-async-tools.txt
  
  Scenario: List async tools - empty
    Tool: Bash (go test)
    Preconditions: Tool implemented, no async tools
    Steps:
      1. Run: go test ./internal/tools/async_tools_test.go -run TestListAsyncToolsEmpty -v
      2. Verify empty list returned
    Expected Result: Test passes, empty list
    Failure Indicators: Error returned, non-empty list
    Evidence: .sisyphus/evidence/task-14-list-async-tools-empty.txt
  ```

  **Commit**: YES (groups with Tasks 11, 12, 13)
  - Message: `feat(tools): add specialized async control tools`
  - Files: `server/internal/tools/async_tools.go`
  - Pre-commit: `go test ./internal/tools/...`

- [x] 15. **Add frontend polling API**

  **What to do**:
  - Modify `server/internal/api/handlers.go`
  - Add `GetSessionUpdates(c *gin.Context)` handler
  - Extract sessionID from path, since (last_event_id) from query
  - Query session.metadata for async_completion_pending events since last_event_id
  - Return JSON: `{has_updates, events: [{type, tool_call_id, tool_name, completed_at, status}]}`
  - Register route: `GET /api/sessions/:id/updates` in routes.go
  
  **Must NOT do**:
  - Return events from other sessions
  - Include sensitive information (only summary)
  - Fail if no updates (return has_updates: false)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple GET endpoint following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with Tasks 11, 12, 13, 14)
  - **Blocks**: Tasks 19, 22
  - **Blocked By**: Tasks 9, 17

  **References**:
  - `server/internal/api/handlers.go:196-232` - Existing handler pattern
  - `server/internal/session/manager.go` - Session metadata access

  **Acceptance Criteria**:
  - [ ] GetSessionUpdates handler created
  - [ ] Route registered in routes.go
  - [ ] Returns pending async completion events
  - [ ] Filters by since parameter
  - [ ] Returns has_updates: false if no updates
  - [ ] Only returns events for current session

  **QA Scenarios**:
  ```
  Scenario: Polling - pending events
    Tool: Bash (curl)
    Preconditions: Async tool completed, SSE disconnected
    Steps:
      1. curl -X GET "http://localhost:8080/api/sessions/{id}/updates?since={last_id}"
      2. Verify pending event returned
    Expected Result: 200 OK with pending event
    Failure Indicators: No event, wrong format
    Evidence: .sisyphus/evidence/task-15-polling-pending.json
  
  Scenario: Polling - no updates
    Tool: Bash (curl)
    Preconditions: No pending events
    Steps:
      1. curl -X GET "http://localhost:8080/api/sessions/{id}/updates"
      2. Verify has_updates: false
    Expected Result: 200 OK with has_updates: false
    Failure Indicators: Error returned
    Evidence: .sisyphus/evidence/task-15-polling-no-updates.json
  ```

  **Commit**: YES
  - Message: `feat(api): add frontend polling endpoint for async events`
  - Files: `server/internal/api/handlers.go`, `server/internal/api/routes.go`
  - Pre-commit: `go test ./internal/api/...`

- [x] 16. **Integrate Session cleanup**

  **What to do**:
  - Modify `server/internal/session/manager.go` Delete method
  - Before deleting session, call AsyncToolRegistry.CancelSession(sessionID)
  - This cancels all async tools associated with the session
  - Ensure cleanup happens even on error
  - Add logging for cancelled tools
  
  **Must NOT do**:
  - Block session deletion waiting for tools to finish (cancel immediately)
  - Skip cleanup on error (use defer)
  - Delete session before cancelling tools

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple integration point following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5 (with Tasks 17, 18)
  - **Blocks**: Tasks 19, 20, 22
  - **Blocked By**: Task 1

  **References**:
  - `server/internal/session/manager.go` - Session Delete method
  - `server/internal/tool/registry.go` - CancelSession method

  **Acceptance Criteria**:
  - [ ] Session Delete calls CancelSession
  - [ ] All async tools cancelled when session deleted
  - [ ] Cleanup happens even on error
  - [ ] Logging added for cancelled tools
  - [ ] No goroutine leaks after session deletion

  **QA Scenarios**:
  ```
  Scenario: Session cleanup
    Tool: Bash (go test)
    Preconditions: Async tools running
    Steps:
      1. Run: go test ./internal/session/manager_test.go -run TestSessionCleanup -v
      2. Delete session, verify tools cancelled
    Expected Result: Test passes, all tools cancelled
    Failure Indicators: Tools still running after deletion
    Evidence: .sisyphus/evidence/task-16-session-cleanup.txt
  ```

  **Commit**: YES (groups with Tasks 17, 18)
  - Message: `feat(session): integrate async task cleanup and timeout handling`
  - Files: `server/internal/session/manager.go`
  - Pre-commit: `go test ./internal/session/...`

- [x] 17. **Add async completion persistence**

  **What to do**:
  - Modify async tool completion logic in engine.go (from Task 9)
  - When async tool completes:
    - Persist result to session.Message (role="tool", tool_call_id="...", content=result_json)
    - If SSE disconnected: Record async_completion_pending in session.metadata
  - Use existing contextMgr.AddMessage pattern
  - Ensure persistence happens before event emission
  
  **Must NOT do**:
  - Skip persistence (results must be durable)
  - Persist before tool completes (only on completion)
  - Block tool completion on persistence failure (log error and continue)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Follows existing persistence pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5 (with Tasks 16, 18)
  - **Blocks**: Task 15
  - **Blocked By**: Task 9

  **References**:
  - `server/internal/agent/engine.go:348-357` - Result persistence pattern
  - `server/internal/context/manager.go` - AddMessage method
  - `server/internal/session/manager.go` - Metadata update

  **Acceptance Criteria**:
  - [ ] Result persisted to session.Message on completion
  - [ ] async_completion_pending recorded if SSE disconnected
  - [ ] Persistence happens before event emission
  - [ ] Error logged if persistence fails (doesn't block)

  **QA Scenarios**:
  ```
  Scenario: Result persistence
    Tool: Bash (go test)
    Preconditions: Async tool completed
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestAsyncResultPersistence -v
      2. Verify result in session.Message
    Expected Result: Test passes, result persisted
    Failure Indicators: Missing result in session.Message
    Evidence: .sisyphus/evidence/task-17-result-persistence.txt
  ```

  **Commit**: YES (groups with Tasks 16, 18)
  - Message: `feat(session): integrate async task cleanup and timeout handling`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [x] 18. **Add timeout handling**

  **What to do**:
  - Modify async tool execution in engine.go (from Task 9)
  - Use context.WithTimeout(sessionCtx, 5*time.Minute) when launching goroutine
  - On timeout:
    - Persist error to session.Message (error="timeout after 5 minutes")
    - Update registry status to "failed"
    - Emit EventAsyncToolFailed
    - Record async_completion_pending if SSE disconnected
  - Make timeout configurable (via config or environment variable)
  
  **Must NOT do**:
  - Hardcode timeout without config option
  - Cancel tool without cleanup
  - Skip error persistence on timeout

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Follows existing timeout pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5 (with Tasks 16, 17)
  - **Blocks**: Tasks 19, 20, 22
  - **Blocked By**: Tasks 1, 9

  **References**:
  - `server/internal/tools/code_executor.go:65-66` - Existing timeout pattern
  - `context.WithTimeout` documentation

  **Acceptance Criteria**:
  - [ ] 5-minute timeout applied to async tools
  - [ ] On timeout: error persisted, registry updated, event emitted
  - [ ] async_completion_pending recorded if SSE disconnected
  - [ ] Timeout configurable via config
  - [ ] Context cancellation propagates to tool

  **QA Scenarios**:
  ```
  Scenario: Timeout handling
    Tool: Bash (go test)
    Preconditions: Async tool running > 5 minutes
    Steps:
      1. Run: go test ./internal/agent/engine_test.go -run TestAsyncTimeout -v
      2. Verify timeout error persisted
    Expected Result: Test passes, timeout error in session.Message
    Failure Indicators: No timeout, missing error
    Evidence: .sisyphus/evidence/task-18-timeout-handling.txt
  ```

  **Commit**: YES (groups with Tasks 16, 17)
  - Message: `feat(session): integrate async task cleanup and timeout handling`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [x] 19. **Write registry unit tests**

  **What to do**:
  - Create `server/internal/tool/registry_test.go`
  - Test cases:
    - `TestRegistryRegister`: Verify registration stores all fields
    - `TestRegistryComplete`: Verify completion updates status and result
    - `TestRegistryFail`: Verify failure updates status and error
    - `TestRegistryCancel`: Verify cancel calls function and removes entry
    - `TestRegistryCancelSession`: Verify all session tools cancelled
    - `TestRegistryConcurrentAccess`: Verify thread safety with multiple goroutines
  - Use `testify/assert` and `testify/require`
  - Run with `go test -race`
  
  **Must NOT do**:
  - Create integration tests (unit tests only)
  - Test with real database (mock dependencies)
  - Skip race detection

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Well-defined unit tests following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 6 (with Tasks 20, 21, 22)
  - **Blocks**: None
  - **Blocked By**: Tasks 1, 16, 18

  **References**:
  - `server/internal/session/manager_test.go` - Test pattern with testify
  - `server/internal/tool/registry.go` - Registry implementation to test

  **Acceptance Criteria**:
  - [ ] registry_test.go created
  - [ ] All test cases pass
  - [ ] No race conditions detected with -race flag
  - [ ] Code coverage > 80% for registry.go

  **QA Scenarios**:
  ```
  Scenario: All registry tests pass
    Tool: Bash (go test)
    Preconditions: registry_test.go created
    Steps:
      1. Run: go test ./internal/tool/registry_test.go -v -race -cover
    Expected Result: All tests pass, no races, coverage > 80%
    Failure Indicators: Test failures, race detected, low coverage
    Evidence: .sisyphus/evidence/task-19-registry-tests.txt
  ```

  **Commit**: YES (groups with Tasks 20, 21, 22)
  - Message: `test: add comprehensive tests for async execution`
  - Files: `server/internal/tool/registry_test.go`
  - Pre-commit: `go test ./internal/tool/... -race`

- [x] 20. **Write execution mode tests**

  **What to do**:
  - Create `server/internal/agent/engine_execution_test.go`
  - Test cases:
    - `TestSyncExecution`: Verify sequential execution and result persistence
    - `TestConcurrentExecution`: Verify concurrent execution with overlapping timestamps
    - `TestConcurrencyLimit`: Verify max 5 concurrent tools
    - `TestAsyncExecution`: Verify main loop returns immediately
    - `TestAsyncCompletionSSEOnline`: Verify event emission and persistence
    - `TestAsyncCompletionSSEOffline`: Verify pending event recording
    - `TestErrorIsolation`: Verify one failure doesn't affect others
    - `TestResultOrdering`: Verify results ordered by call ID
    - `TestPanicRecovery`: Verify panicked tool doesn't crash server
    - `TestGoroutineLeak`: Verify no goroutine leaks after execution
  - Use `testify/assert` and mock tools with controllable duration
  - Use `setupTestDB(t)` pattern
  - Run with `go test -race`
  
  **Must NOT do**:
  - Skip race detection
  - Use production tools (create mock tools)
  - Skip goroutine leak detection

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Complex concurrent test scenarios requiring careful design
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 6 (with Tasks 19, 21, 22)
  - **Blocks**: None
  - **Blocked By**: Tasks 7, 8, 9, 10, 16

  **References**:
  - `server/internal/agent/engine_test.go` - Existing engine test pattern
  - `server/internal/session/manager_test.go` - setupTestDB pattern
  - Gollem `agent_test.go` - Parallel execution test examples

  **Acceptance Criteria**:
  - [ ] engine_execution_test.go created
  - [ ] All test cases pass
  - [ ] No race conditions detected
  - [ ] No goroutine leaks detected
  - [ ] Code coverage > 70% for new execution logic

  **QA Scenarios**:
  ```
  Scenario: All execution tests pass
    Tool: Bash (go test)
    Preconditions: engine_execution_test.go created
    Steps:
      1. Run: go test ./internal/agent/engine_execution_test.go -v -race -cover
    Expected Result: All tests pass, no races, coverage > 70%
    Failure Indicators: Test failures, race detected, goroutine leaks
    Evidence: .sisyphus/evidence/task-20-execution-tests.txt
  ```

  **Commit**: YES (groups with Tasks 19, 21, 22)
  - Message: `test: add comprehensive tests for async execution`
  - Files: `server/internal/agent/engine_execution_test.go`
  - Pre-commit: `go test ./internal/agent/... -race`

- [x] 21. **Write specialized tools tests**

  **What to do**:
  - Create `server/internal/tools/async_tools_test.go`
  - Test cases:
    - `TestGetToolStatusRunning`: Verify status query for running tool
    - `TestGetToolStatusCompleted`: Verify status query for completed tool
    - `TestGetToolResultCompleted`: Verify result retrieval for completed tool
    - `TestGetToolResultRunning`: Verify error for running tool
    - `TestCancelToolRunning`: Verify cancellation works
    - `TestCancelToolCompleted`: Verify error for completed tool
    - `TestListAsyncTools`: Verify listing multiple tools
    - `TestListAsyncToolsEmpty`: Verify empty list
  - Use `testify/assert` and mock registry
  - Run with `go test -race`
  
  **Must NOT do**:
  - Use real async tools (mock registry)
  - Skip error scenarios
  - Skip race detection

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Well-defined unit tests following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 6 (with Tasks 19, 20, 22)
  - **Blocks**: None
  - **Blocked By**: Tasks 11, 12, 13, 14

  **References**:
  - `server/internal/tools/todo_tool_test.go` - Example tool test pattern
  - `server/internal/tools/async_tools.go` - Tools to test

  **Acceptance Criteria**:
  - [ ] async_tools_test.go created
  - [ ] All test cases pass
  - [ ] No race conditions detected
  - [ ] Code coverage > 80% for async_tools.go

  **QA Scenarios**:
  ```
  Scenario: All specialized tools tests pass
    Tool: Bash (go test)
    Preconditions: async_tools_test.go created
    Steps:
      1. Run: go test ./internal/tools/async_tools_test.go -v -race -cover
    Expected Result: All tests pass, no races, coverage > 80%
    Failure Indicators: Test failures, race detected
    Evidence: .sisyphus/evidence/task-21-specialized-tools-tests.txt
  ```

  **Commit**: YES (groups with Tasks 19, 20, 22)
  - Message: `test: add comprehensive tests for async execution`
  - Files: `server/internal/tools/async_tools_test.go`
  - Pre-commit: `go test ./internal/tools/... -race`

- [x] 22. **Write integration tests**

  **What to do**:
  - Create `server/internal/agent/integration_test.go`
  - Test cases:
    - `TestFullAsyncLifecycle`: Complete flow from async call to completion to result retrieval
    - `TestSSEDisconnectScenario`: Async tool completes after SSE disconnect, frontend polls, triggers new session
    - `TestMultipleAsyncTools`: Multiple async tools with different completion times
    - `TestSessionCleanupIntegration`: Verify session deletion cancels all async tools
    - `TestTimeoutIntegration`: Verify timeout triggers correct cleanup and notification
  - Use real database (setupTestDB)
  - Use mock tools with configurable duration
  - Test complete request/response flow
  - Run with `go test -race`
  
  **Must NOT do**:
  - Use mocks for database (integration test)
  - Skip SSE disconnect scenario
  - Skip timeout scenario

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Complex integration scenarios testing multiple components
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 6 (with Tasks 19, 20, 21)
  - **Blocks**: None
  - **Blocked By**: Tasks 11, 12, 13, 14, 16, 19, 20, 21

  **References**:
  - `server/internal/agent/engine_test.go` - Existing integration test pattern
  - `server/internal/session/manager_test.go` - setupTestDB pattern
  - Draft document async lifecycle scenarios

  **Acceptance Criteria**:
  - [ ] integration_test.go created
  - [ ] All test cases pass
  - [ ] No race conditions detected
  - [ ] SSE disconnect scenario tested
  - [ ] Timeout scenario tested
  - [ ] Full lifecycle tested

  **QA Scenarios**:
  ```
  Scenario: All integration tests pass
    Tool: Bash (go test)
    Preconditions: integration_test.go created
    Steps:
      1. Run: go test ./internal/agent/integration_test.go -v -race -cover
    Expected Result: All tests pass, no races
    Failure Indicators: Test failures, race detected
    Evidence: .sisyphus/evidence/task-22-integration-tests.txt
  ```

  **Commit**: YES (groups with Tasks 19, 20, 21)
  - Message: `test: add comprehensive tests for async execution`
  - Files: `server/internal/agent/integration_test.go`
  - Pre-commit: `go test ./internal/agent/... -race`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, curl endpoint, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go test -race ./...`. Review all changed files for: `panic` without recovery, goroutine leaks, missing context propagation, unused imports, race conditions. Check AI slop: excessive comments, over-abstraction, generic names.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Race [CLEAN/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test cross-task integration. Test edge cases: concurrent limit, async timeout, SSE disconnect, empty input. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **1**: `feat(tool): add AsyncToolRegistry for async state management` - server/internal/tool/registry.go, go test ./internal/tool/...
- **2**: `feat(agent): add execution mode support (sync/concurrent/async)` - server/internal/agent/engine.go, server/internal/tool/manager.go, go test ./internal/agent/...
- **3**: `feat(tools): add specialized async control tools` - server/internal/tools/async_tools.go, go test ./internal/tools/...
- **4**: `feat(api): add frontend polling endpoint for async events` - server/internal/api/handlers.go, server/internal/api/routes.go, go test ./internal/api/...
- **5**: `feat(session): integrate async task cleanup` - server/internal/session/manager.go, go test ./internal/session/...
- **6**: `test: add comprehensive tests for async execution` - all test files, go test -race ./...

---

## Success Criteria

### Verification Commands
```bash
# Unit tests
go test ./internal/tool/... -v -race
go test ./internal/agent/... -v -race
go test ./internal/tools/... -v -race

# Integration test
go test ./internal/api/... -v

# All tests with coverage
go test ./... -cover -race

# Manual verification
curl http://localhost:8080/api/sessions/{id}/updates
curl -X POST http://localhost:8080/api/sessions/{id}/chat -d '{"content":""}'
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass with race detector
- [ ] No goroutine leaks detected
- [ ] Tool descriptions include execution_mode
- [ ] All three execution modes work correctly
- [ ] Async lifecycle handling complete
- [ ] Frontend polling API functional
- [ ] Empty input handling correct
- [ ] Specialized tools functional
