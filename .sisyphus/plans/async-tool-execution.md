# Async Tool Execution Implementation Plan

## TL;DR

> **Quick Summary**: Implement parallel tool execution with bounded concurrency (5 workers), in-memory state tracking, and cancellation support. Keep Tool interface synchronous - parallelization happens in engine layer only.
> 
> **Deliverables**:
> - ToolExecutionTracker (in-memory state management)
> - Parallel dispatch logic in AgentEngine
> - Status query API (GET /tools/:callId/status)
> - Cancellation API (DELETE /tools/:callId/cancel)
> - Lifecycle events (EventToolStart, EventToolComplete, EventToolFailed)
> - Concurrency control via semaphore
> 
> **Estimated Effort**: Medium
> **Parallel Execution**: YES - 3 waves
> **Critical Path**: Tracker → Engine → API → Tests

---

## Context

### Original Request
实现工具调用的异步执行，支持并行执行和长时间运行任务，提供状态查询和完成通知。

### Interview Summary
**Key Discussions**:
- **工具范围**: 所有工具支持异步
- **使用场景**: 并行执行 + 长时间运行任务
- **状态反馈**: 可查询状态（GET API）+ 完成时通知（SSE event）
- **架构模式**: 进程内异步（goroutine + semaphore）
- **错误策略**: 继续执行（一个工具失败不影响其他）
- **进度粒度**: 仅生命周期事件（start → complete/failed）
- **并发限制**: 全局限制 5 个并行工具
- **取消能力**: 支持取消单个工具
- **长任务处理**: 工具内部通过 Context 超时处理
- **测试策略**: Tests-after

**Research Findings**:
- **Current Architecture**: engine.go:436 顺序执行工具，Tool 接口同步，ChatContext.Emit 非阻塞
- **Industry Patterns**: OpenAI (requires_action), LangChain (AsyncBackgroundExecutor), CrewAI (akickoff)
- **Go Patterns**: Semaphore (chan struct{}), Context timeout layers, sync.Map for tracking
- **Validated**: Tool call ID 来自 OpenAI，无需生成新 ID；ChatContext 线程安全

### Metis Review
**Identified Gaps** (addressed):
- **Gap 1**: Result persistence strategy - Persist immediately to avoid data loss on crash
- **Gap 2**: Agent loop blocking behavior - Stream intermediate results but wait for all before next LLM call
- **Gap 3**: Tool interface signature - Keep synchronous, parallelization in engine only
- **Gap 4**: State tracking - Use sync.Map in-memory, no DB persistence

**Guardrails Applied**:
- MUST use golang.org/x/sync/semaphore for concurrency
- MUST NOT change Tool interface signature
- MUST persist results immediately using existing contextMgr.AddMessage pattern
- MUST use OpenAI's tool call ID (tc.ID) - no new ID generation
- MUST NOT persist tool execution state to database
- MUST handle panics in tool goroutines gracefully
- MUST NOT allow goroutine leaks on session termination

---

## Work Objectives

### Core Objective
Enable parallel execution of tool calls with bounded concurrency, in-memory state tracking, and per-tool cancellation support, while maintaining backward compatibility with synchronous Tool interface.

### Concrete Deliverables
- `server/internal/tool/tracker.go` - ToolExecutionTracker with sync.Map
- `server/internal/agent/engine.go` - Parallel dispatch logic with semaphore
- `server/internal/api/handlers.go` - Status and cancellation endpoints
- `server/internal/api/routes.go` - New route registrations
- `server/internal/domain/entity/event.go` - Lifecycle event types
- `server/internal/tool/tracker_test.go` - Tracker unit tests
- `server/internal/agent/engine_parallel_test.go` - Parallel execution tests

### Definition of Done
- [ ] All tools can execute in parallel (up to 5 concurrent)
- [ ] Status query API returns correct state
- [ ] Cancellation API stops tool execution
- [ ] One tool failure doesn't affect other tools
- [ ] Results are persisted immediately and correctly ordered
- [ ] No goroutine leaks after execution/cancellation
- [ ] All tests pass with `go test -race`

### Must Have
- Parallel tool execution with bounded concurrency (5 workers)
- In-memory ToolExecutionTracker for state management
- Status query API endpoint
- Cancellation API endpoint
- Lifecycle events (start, complete, failed)
- Error isolation (one failure doesn't stop others)
- Result ordering by tool call ID
- Panic recovery in tool goroutines

### Must NOT Have (Guardrails)
- Changes to Tool interface signature
- Database persistence of tool execution state
- New ID generation for tool calls (use OpenAI's tc.ID)
- External dependencies beyond golang.org/x/sync
- Blocking agent loop unnecessarily
- Goroutine leaks on session termination
- Changes to existing tool implementations

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES (go test with testify)
- **Automated tests**: Tests-after
- **Framework**: go test
- **Coverage**: Unit tests for tracker, integration tests for engine, API tests for endpoints

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
├── Task 1: Create ToolExecutionTracker struct [quick]
├── Task 2: Add lifecycle event types [quick]
└── Task 3: Add semaphore to AgentEngine [quick]

Wave 2 (After Wave 1 - core logic):
├── Task 4: Implement parallel dispatch in engine [unspecified-high]
├── Task 5: Implement result collection and ordering [quick]
└── Task 6: Add panic recovery wrapper [quick]

Wave 3 (After Wave 2 - API + integration):
├── Task 7: Add status query API endpoint [quick]
├── Task 8: Add cancellation API endpoint [quick]
└── Task 9: Register new routes [quick]

Wave 4 (After Wave 3 - tests):
├── Task 10: Write tracker unit tests [quick]
├── Task 11: Write parallel execution tests [unspecified-high]
└── Task 12: Write API endpoint tests [quick]

Wave FINAL (After ALL tasks — 4 parallel reviews, then user okay):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)
-> Present results -> Get explicit user okay

Critical Path: Task 1 → Task 4 → Task 7 → Task 10 → F1-F4 → user okay
Parallel Speedup: ~60% faster than sequential
Max Concurrent: 3 (Waves 1 & 3)
```

### Dependency Matrix

- **1-3**: - - 4-6, 1
- **4**: 1, 3 - 5, 6, 2
- **5**: 4 - 7-9, 3
- **6**: 4 - 7-9, 3
- **7-9**: 4, 5, 6 - 10-12, 4
- **10**: 1 - 11, 12, 5
- **11**: 4, 5, 6 - 12, 6
- **12**: 7, 8, 9 - 13, 7

### Agent Dispatch Summary

- **1**: **3** - T1 → `quick`, T2 → `quick`, T3 → `quick`
- **2**: **3** - T4 → `unspecified-high`, T5 → `quick`, T6 → `quick`
- **3**: **3** - T7 → `quick`, T8 → `quick`, T9 → `quick`
- **4**: **3** - T10 → `quick`, T11 → `unspecified-high`, T12 → `quick`
- **FINAL**: **4** - F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

> Implementation + Test = ONE Task. Never separate.
> EVERY task MUST have: Recommended Agent Profile + Parallelization info + QA Scenarios.

- [ ] 1. **Create ToolExecutionTracker struct**

  **What to do**:
  - Create `server/internal/tool/tracker.go`
  - Define `ToolExecutionTracker` struct with `sync.Map` for concurrent-safe state tracking
  - Define `RunningTool` struct: `CallID`, `StartTime`, `CancelFunc`, `Status` ("running"|"completed"|"failed"|"cancelled")
  - Implement methods: `Register(callID, cancelFunc)`, `Unregister(callID)`, `Cancel(callID)`, `GetStatus(callID)`
  - Follow pattern from `server/internal/tool/manager.go:47-56` (sync.RWMutex usage)
  
  **Must NOT do**:
  - Persist state to database (in-memory only)
  - Generate new IDs (use provided callID)
  - Add complex status fields beyond lifecycle states

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Well-defined struct creation following existing patterns
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3)
  - **Blocks**: Task 4 (needs tracker for parallel dispatch)
  - **Blocked By**: None (can start immediately)

  **References**:
  - `server/internal/tool/manager.go:47-56` - Sync mutex pattern for concurrent access
  - `server/internal/domain/entity/event.go:34-48` - ToolCallData structure for call ID reference
  - `golang.org/x/sync/semaphore` documentation - Semaphore pattern

  **Acceptance Criteria**:
  - [ ] tracker.go created with ToolExecutionTracker struct
  - [ ] sync.Map used for concurrent-safe storage
  - [ ] Register stores cancel function and start time
  - [ ] Cancel calls stored cancel function and removes from map
  - [ ] GetStatus returns correct state or error for unknown call ID

  **QA Scenarios**:
  ```
  Scenario: Tracker concurrent access
    Tool: Bash (go test)
    Preconditions: tracker.go implemented
    Steps:
      1. Run: go test ./internal/tool/tracker_test.go -race -v
      2. Check output for race condition detection
    Expected Result: No race conditions detected, all tests pass
    Failure Indicators: "DATA RACE" in output, test failures
    Evidence: .sisyphus/evidence/task-1-tracker-race.txt
  
  Scenario: Tracker cancellation
    Tool: Bash (go test)
    Preconditions: tracker.go with Cancel method
    Steps:
      1. Run: go test ./internal/tool/tracker_test.go -run TestTrackerCancel -v
      2. Verify cancel function is called
    Expected Result: Test passes, cancel function invoked
    Failure Indicators: Test failure, cancel not called
    Evidence: .sisyphus/evidence/task-1-tracker-cancel.txt
  ```

  **Evidence to Capture**:
  - [ ] tracker_race.txt - Race detector output
  - [ ] tracker_cancel.txt - Cancel test output

  **Commit**: YES (groups with Task 2, 3)
  - Message: `feat(tool): add ToolExecutionTracker for async state management`
  - Files: `server/internal/tool/tracker.go`
  - Pre-commit: `go test ./internal/tool/...`

- [ ] 2. **Add lifecycle event types**

  **What to do**:
  - Extend `server/internal/domain/entity/event.go`
  - Add `EventToolStart` event type (if not using existing EventToolCall)
  - Ensure `EventToolComplete` and `EventToolFailed` are defined
  - Update `ToolCallData` to include `StartTime` field (if needed)
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
  - **Parallel Group**: Wave 1 (with Tasks 1, 3)
  - **Blocks**: Task 4 (needs events for emission)
  - **Blocked By**: None (can start immediately)

  **References**:
  - `server/internal/domain/entity/event.go:8-21` - Existing event type definitions
  - `server/internal/domain/entity/event.go:34-48` - ToolCallData and ToolResultData structure

  **Acceptance Criteria**:
  - [ ] EventToolStart defined (or reuse EventToolCall)
  - [ ] EventToolComplete already exists (verify)
  - [ ] EventToolFailed defined if not present
  - [ ] Event emission follows existing pattern

  **QA Scenarios**:
  ```
  Scenario: Event type constants
    Tool: Bash (grep + go build)
    Preconditions: event.go updated
    Steps:
      1. grep "EventToolStart" server/internal/domain/entity/event.go
      2. grep "EventToolFailed" server/internal/domain/entity/event.go
      3. Run: go build ./internal/domain/entity/...
    Expected Result: All event types found, build succeeds
    Failure Indicators: grep returns empty, build errors
    Evidence: .sisyphus/evidence/task-2-events.txt
  ```

  **Commit**: YES (groups with Task 1, 3)
  - Message: `feat(tool): add ToolExecutionTracker for async state management`
  - Files: `server/internal/domain/entity/event.go`
  - Pre-commit: `go build ./...`

- [ ] 3. **Add semaphore to AgentEngine**

  **What to do**:
  - Modify `server/internal/agent/engine.go`
  - Add import for `golang.org/x/sync/semaphore`
  - Add `semaphore *semaphore.Weighted` field to `AgentEngine` struct
  - Initialize semaphore with limit 5 in `NewAgentEngine()` or appropriate constructor
  - Follow existing struct modification pattern
  
  **Must NOT do**:
  - Change AgentEngine initialization logic beyond semaphore addition
  - Implement parallel execution logic yet (Task 4)
  - Add per-tool semaphore (global only)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple struct field addition
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2)
  - **Blocks**: Task 4 (needs semaphore for dispatch)
  - **Blocked By**: None (can start immediately)

  **References**:
  - `server/internal/agent/engine.go:60-80` - AgentEngine struct definition
  - `golang.org/x/sync/semaphore` documentation - Semaphore usage
  - Metis directive: "Use golang.org/x/sync/semaphore for global concurrency limit (5)"

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

  **Commit**: YES (groups with Task 1, 2)
  - Message: `feat(tool): add ToolExecutionTracker for async state management`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go build ./...`

- [ ] 4. **Implement parallel dispatch in engine**

  **What to do**:
  - Modify `server/internal/agent/engine.go` handleToolCalls method (lines 426-455)
  - Replace sequential `for _, tc := range result.ToolCalls` loop with parallel dispatch
  - Use `errgroup.WithContext` or manual goroutine + WaitGroup pattern
  - For each tool call:
    1. Acquire semaphore slot (blocking with context check)
    2. Register in tracker (callID, cancelFunc)
    3. Emit EventToolStart
    4. Launch goroutine: execute tool, persist result, emit EventToolComplete/EventToolFailed
    5. Unregister from tracker on completion
  - Use `sync.Map` or mutex-protected array to collect results
  - Order results by tool call ID before returning
  - Follow gollem pattern: `agent.go:1064-1113` (semaphore + WaitGroup)
  
  **Must NOT do**:
  - Change Tool interface signature (keep synchronous Execute)
  - Implement retry logic (out of scope)
  - Block agent loop longer than necessary
  - Allow goroutine leaks

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Complex concurrent logic requiring careful implementation
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Tasks 5-9
  - **Blocked By**: Tasks 1, 2, 3 (tracker, events, semaphore)

  **References**:
  - `server/internal/agent/engine.go:426-455` - Current sequential execution
  - `server/internal/agent/engine.go:315-358` - executeToolCall method (to parallelize)
  - `server/internal/agent/engine.go:348-357` - Result persistence pattern
  - Gollem `agent.go:1064-1113` - Semaphore + WaitGroup pattern with context check
  - Metis directive: "Use errgroup-based parallel execution, collect results, order by tool call ID"

  **Acceptance Criteria**:
  - [ ] Parallel dispatch implemented with semaphore control
  - [ ] Tools execute concurrently (verified by overlapping timestamps)
  - [ ] Results collected and ordered by tool call ID
  - [ ] Results persisted immediately using contextMgr.AddMessage
  - [ ] Tracker registration/unregistration correct
  - [ ] Events emitted correctly (start, complete, failed)
  - [ ] No goroutine leaks

  **QA Scenarios**:
  ```
  Scenario: Parallel execution verification
    Tool: Bash (go test)
    Preconditions: Parallel dispatch implemented
    Steps:
      1. Run: go test ./internal/agent/engine_parallel_test.go -run TestParallelToolExecution -v
      2. Check timestamps overlap in test output
    Expected Result: Test passes, timestamps show concurrent execution
    Failure Indicators: Sequential timestamps, test failure
    Evidence: .sisyphus/evidence/task-4-parallel.txt
  
  Scenario: Concurrency limit enforcement
    Tool: Bash (go test)
    Preconditions: Semaphore limit 5
    Steps:
      1. Run: go test ./internal/agent/engine_parallel_test.go -run TestConcurrencyLimit -v
      2. Verify max 5 tools execute simultaneously
    Expected Result: Test passes, max 5 concurrent tools observed
    Failure Indicators: More than 5 concurrent executions
    Evidence: .sisyphus/evidence/task-4-concurrency.txt
  
  Scenario: Error isolation
    Tool: Bash (go test)
    Preconditions: Error handling implemented
    Steps:
      1. Run: go test ./internal/agent/engine_parallel_test.go -run TestErrorIsolation -v
      2. Verify one failure doesn't stop others
    Expected Result: Test passes, all tools complete despite one failure
    Failure Indicators: Early termination, incomplete results
    Evidence: .sisyphus/evidence/task-4-error-isolation.txt
  
  Scenario: Goroutine leak detection
    Tool: Bash (go test)
    Preconditions: Cleanup implemented
    Steps:
      1. Run: go test ./internal/agent/engine_parallel_test.go -run TestGoroutineLeak -v
      2. Check final goroutine count matches initial
    Expected Result: No goroutine leaks detected
    Failure Indicators: Increased goroutine count after test
    Evidence: .sisyphus/evidence/task-4-goroutine-leak.txt
  ```

  **Commit**: YES
  - Message: `feat(agent): implement parallel tool execution with semaphore`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [ ] 5. **Implement result collection and ordering**

  **What to do**:
  - Part of Task 4, but focused on result aggregation
  - Use mutex-protected slice or `sync.Map` to collect results
  - After all goroutines complete, sort results by tool call ID
  - Persist results in order using existing `contextMgr.AddMessage` pattern
  - Ensure result ordering matches OpenAI's expectation
  
  **Must NOT do**:
  - Persist results to new table (use existing session.Message)
  - Batch persistence (persist immediately per Metis directive)
  - Lose result ordering information

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Follows existing persistence pattern, well-defined ordering logic
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Task 4, 6)
  - **Blocks**: Tasks 7-9
  - **Blocked By**: Task 4

  **References**:
  - `server/internal/agent/engine.go:348-357` - Result persistence pattern
  - `server/internal/domain/entity/event.go:34-48` - ToolResultData.ID field
  - Metis directive: "Persist immediately to avoid data loss, use existing pattern"

  **Acceptance Criteria**:
  - [ ] Results collected safely from concurrent goroutines
  - [ ] Results ordered by tool call ID before persistence
  - [ ] Each result persisted immediately after completion
  - [ ] Persistence uses contextMgr.AddMessage pattern

  **QA Scenarios**:
  ```
  Scenario: Result ordering
    Tool: Bash (go test)
    Preconditions: Result collection implemented
    Steps:
      1. Run: go test ./internal/agent/engine_parallel_test.go -run TestResultOrdering -v
      2. Verify result order matches tool call ID order
    Expected Result: Test passes, results in correct order
    Failure Indicators: Wrong ordering, test failure
    Evidence: .sisyphus/evidence/task-5-ordering.txt
  ```

  **Commit**: YES (groups with Task 4)
  - Message: `feat(agent): implement parallel tool execution with semaphore`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [ ] 6. **Add panic recovery wrapper**

  **What to do**:
  - Create `safeExecuteTool` function in `server/internal/agent/engine.go`
  - Wrap tool execution with `defer func() { if r := recover() {...} }`
  - On panic: emit EventToolFailed with panic message, unregister from tracker
  - Follow gollem pattern: `agent.go:1516-1525` (safeCallHandler)
  
  **Must NOT do**:
  - Crash the server on tool panic
  - Lose panic information (include in error message)
  - Skip tracker cleanup on panic

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple defer pattern following existing gollem implementation
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 4, 5)
  - **Blocks**: Tasks 7-9
  - **Blocked By**: Task 4

  **References**:
  - Gollem `agent.go:1516-1525` - safeCallHandler with panic recovery
  - `server/internal/domain/entity/event.go` - EventToolFailed type

  **Acceptance Criteria**:
  - [ ] safeExecuteTool wrapper function created
  - [ ] Panic caught and converted to error
  - [ ] EventToolFailed emitted with panic message
  - [ ] Tracker cleanup happens on panic
  - [ ] Server doesn't crash on tool panic

  **QA Scenarios**:
  ```
  Scenario: Panic recovery
    Tool: Bash (go test)
    Preconditions: safeExecuteTool implemented
    Steps:
      1. Run: go test ./internal/agent/engine_parallel_test.go -run TestPanicRecovery -v
      2. Verify panic tool doesn't crash server
    Expected Result: Test passes, panic converted to failed event
    Failure Indicators: Server crash, missing failure event
    Evidence: .sisyphus/evidence/task-6-panic.txt
  ```

  **Commit**: YES (groups with Task 4)
  - Message: `feat(agent): implement parallel tool execution with semaphore`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/...`

- [ ] 7. **Add status query API endpoint**

  **What to do**:
  - Modify `server/internal/api/handlers.go`
  - Add `GetToolStatus(c *gin.Context)` handler
  - Extract `callId` from path parameter
  - Query ToolExecutionTracker for status
  - Return JSON: `{ "callId": "...", "status": "running"|"completed"|"failed"|"cancelled", "startTime": "...", "duration": "..." }`
  - Return 404 if call ID not found
  - Follow existing handler pattern from `handlers.go:196-232`
  
  **Must NOT do**:
  - Query database for tool status (in-memory only)
  - Expose internal tracker implementation details
  - Add authentication (assume session-level auth already applied)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple GET endpoint following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 8, 9)
  - **Blocks**: Task 12
  - **Blocked By**: Tasks 4, 5, 6 (tracker populated by engine)

  **References**:
  - `server/internal/api/handlers.go:196-232` - Existing handler pattern
  - `server/internal/tool/tracker.go` - GetStatus method

  **Acceptance Criteria**:
  - [ ] GetToolStatus handler created
  - [ ] Returns correct status for running/completed/failed/cancelled tools
  - [ ] Returns 404 for unknown call ID
  - [ ] JSON response includes callId, status, startTime, duration

  **QA Scenarios**:
  ```
  Scenario: Status query success
    Tool: Bash (curl)
    Preconditions: Tool executing or completed
    Steps:
      1. curl -X GET http://localhost:8080/api/tools/{callId}/status
      2. Check response has status field
    Expected Result: 200 OK with status JSON
    Failure Indicators: 404, missing fields, wrong format
    Evidence: .sisyphus/evidence/task-7-status-success.json
  
  Scenario: Status query not found
    Tool: Bash (curl)
    Preconditions: Invalid call ID
    Steps:
      1. curl -X GET http://localhost:8080/api/tools/invalid-id/status
    Expected Result: 404 Not Found
    Failure Indicators: 200 OK with empty status
    Evidence: .sisyphus/evidence/task-7-status-notfound.txt
  ```

  **Commit**: YES (groups with Task 8, 9)
  - Message: `feat(api): add tool status and cancellation endpoints`
  - Files: `server/internal/api/handlers.go`
  - Pre-commit: `go test ./internal/api/...`

- [ ] 8. **Add cancellation API endpoint**

  **What to do**:
  - Modify `server/internal/api/handlers.go`
  - Add `CancelTool(c *gin.Context)` handler
  - Extract `callId` from path parameter
  - Call `tracker.Cancel(callId)`
  - Return JSON: `{ "callId": "...", "cancelled": true|false }`
  - Return 404 if call ID not found or already completed
  - Follow existing handler pattern
  
  **Must NOT do**:
  - Force kill tool process (use context cancellation)
  - Cancel tools that are already completed
  - Add confirmation prompts (API is programmatic)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple DELETE endpoint following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 7, 9)
  - **Blocks**: Task 12
  - **Blocked By**: Tasks 4, 5, 6 (tracker with cancel capability)

  **References**:
  - `server/internal/api/handlers.go:196-232` - Existing handler pattern
  - `server/internal/tool/tracker.go` - Cancel method

  **Acceptance Criteria**:
  - [ ] CancelTool handler created
  - [ ] Calls tracker.Cancel(callId)
  - [ ] Returns 200 with cancelled: true if successful
  - [ ] Returns 404 if call ID not found or already completed
  - [ ] Tool execution actually stops (context cancellation propagates)

  **QA Scenarios**:
  ```
  Scenario: Cancel running tool
    Tool: Bash (curl)
    Preconditions: Tool is currently executing
    Steps:
      1. curl -X DELETE http://localhost:8080/api/tools/{callId}/cancel
      2. Query status to verify cancelled
    Expected Result: 200 OK with cancelled: true, status shows "cancelled"
    Failure Indicators: cancelled: false, status still "running"
    Evidence: .sisyphus/evidence/task-8-cancel-success.json
  
  Scenario: Cancel completed tool
    Tool: Bash (curl)
    Preconditions: Tool already completed
    Steps:
      1. curl -X DELETE http://localhost:8080/api/tools/{completed-callId}/cancel
    Expected Result: 404 Not Found or 400 Bad Request
    Failure Indicators: 200 OK with cancelled: true
    Evidence: .sisyphus/evidence/task-8-cancel-completed.txt
  ```

  **Commit**: YES (groups with Task 7, 9)
  - Message: `feat(api): add tool status and cancellation endpoints`
  - Files: `server/internal/api/handlers.go`
  - Pre-commit: `go test ./internal/api/...`

- [ ] 9. **Register new routes**

  **What to do**:
  - Modify `server/internal/api/routes.go`
  - Add routes:
    - `GET /api/tools/:callId/status` → handlers.GetToolStatus
    - `DELETE /api/tools/:callId/cancel` → handlers.CancelTool
  - Follow existing route registration pattern
  - Ensure routes are under session authentication if applicable
  
  **Must NOT do**:
  - Create new route group (use existing)
  - Add routes outside /api namespace
  - Modify existing route order

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple route registration
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 7, 8)
  - **Blocks**: Task 12
  - **Blocked By**: Tasks 7, 8 (handlers exist)

  **References**:
  - `server/internal/api/routes.go` - Existing route registration pattern

  **Acceptance Criteria**:
  - [ ] Both routes registered in routes.go
  - [ ] Routes accessible via curl
  - [ ] Routes follow existing pattern and naming

  **QA Scenarios**:
  ```
  Scenario: Route accessibility
    Tool: Bash (curl)
    Preconditions: Server running
    Steps:
      1. curl -X GET http://localhost:8080/api/tools/test-id/status
      2. curl -X DELETE http://localhost:8080/api/tools/test-id/cancel
    Expected Result: Routes respond (404 for missing ID is OK)
    Failure Indicators: 404 for route itself, "route not found"
    Evidence: .sisyphus/evidence/task-9-routes.txt
  ```

  **Commit**: YES (groups with Task 7, 8)
  - Message: `feat(api): add tool status and cancellation endpoints`
  - Files: `server/internal/api/routes.go`
  - Pre-commit: `go test ./internal/api/...`

- [ ] 10. **Write tracker unit tests**

  **What to do**:
  - Create `server/internal/tool/tracker_test.go`
  - Test cases:
    - `TestTrackerRegister`: Verify registration stores cancel func and start time
    - `TestTrackerCancel`: Verify cancel calls stored function, returns correct result
    - `TestTrackerGetStatus`: Verify status query for all states
    - `TestTrackerConcurrentAccess`: Verify thread safety with multiple goroutines
  - Use `testify/assert` and `testify/require`
  - Run with `go test -race` to verify concurrent safety
  
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
  - **Parallel Group**: Wave 4 (with Tasks 11, 12)
  - **Blocks**: None
  - **Blocked By**: Task 1 (tracker exists)

  **References**:
  - `server/internal/session/manager_test.go` - Test pattern with testify
  - `server/internal/tool/manager.go` - Tracker implementation to test

  **Acceptance Criteria**:
  - [ ] tracker_test.go created
  - [ ] All test cases pass
  - [ ] No race conditions detected with -race flag
  - [ ] Code coverage > 80% for tracker.go

  **QA Scenarios**:
  ```
  Scenario: All tracker tests pass
    Tool: Bash (go test)
    Preconditions: tracker_test.go created
    Steps:
      1. Run: go test ./internal/tool/tracker_test.go -v -race -cover
    Expected Result: All tests pass, no races, coverage > 80%
    Failure Indicators: Test failures, race detected, low coverage
    Evidence: .sisyphus/evidence/task-10-tracker-tests.txt
  ```

  **Commit**: YES
  - Message: `test: add comprehensive tests for async tool execution`
  - Files: `server/internal/tool/tracker_test.go`
  - Pre-commit: `go test ./internal/tool/... -race`

- [ ] 11. **Write parallel execution tests**

  **What to do**:
  - Create `server/internal/agent/engine_parallel_test.go`
  - Test cases:
    - `TestParallelToolExecution`: Verify tools execute concurrently (check timestamp overlap)
    - `TestConcurrencyLimit`: Verify max 5 tools run simultaneously
    - `TestToolCancellation`: Verify cancel API stops tool
    - `TestErrorIsolation`: Verify one failure doesn't affect others
    - `TestResultOrdering`: Verify results ordered by call ID
    - `TestPanicRecovery`: Verify panicked tool doesn't crash server
    - `TestGoroutineLeak`: Verify no goroutine leaks after execution
  - Use `testify/assert` and mock tools with controllable duration
  - Use `setupTestDB(t)` pattern for database setup
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
  - **Parallel Group**: Wave 4 (with Tasks 10, 12)
  - **Blocks**: None
  - **Blocked By**: Tasks 4, 5, 6 (parallel execution implemented)

  **References**:
  - `server/internal/agent/engine_test.go` - Existing engine test pattern
  - `server/internal/session/manager_test.go` - setupTestDB pattern
  - Gollem `agent_test.go` - Parallel execution test examples

  **Acceptance Criteria**:
  - [ ] engine_parallel_test.go created
  - [ ] All test cases pass
  - [ ] No race conditions detected
  - [ ] No goroutine leaks detected
  - [ ] Code coverage > 70% for new parallel logic

  **QA Scenarios**:
  ```
  Scenario: All parallel tests pass
    Tool: Bash (go test)
    Preconditions: engine_parallel_test.go created
    Steps:
      1. Run: go test ./internal/agent/engine_parallel_test.go -v -race -cover
    Expected Result: All tests pass, no races, coverage > 70%
    Failure Indicators: Test failures, race detected, goroutine leaks
    Evidence: .sisyphus/evidence/task-11-parallel-tests.txt
  ```

  **Commit**: YES
  - Message: `test: add comprehensive tests for async tool execution`
  - Files: `server/internal/agent/engine_parallel_test.go`
  - Pre-commit: `go test ./internal/agent/... -race`

- [ ] 12. **Write API endpoint tests**

  **What to do**:
  - Add tests to `server/internal/api/handlers_test.go` (or create if needed)
  - Test cases:
    - `TestGetToolStatus`: Verify status query for running/completed/not-found
    - `TestCancelTool`: Verify cancellation for running/completed/not-found
  - Use `httptest.NewRecorder` for HTTP testing
  - Mock ToolExecutionTracker for unit testing
  
  **Must NOT do**:
  - Use real server (unit tests only)
  - Skip HTTP status code verification
  - Test without mocking tracker

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple HTTP endpoint tests following existing pattern
  - **Skills**: []
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with Tasks 10, 11)
  - **Blocks**: None
  - **Blocked By**: Tasks 7, 8, 9 (endpoints exist)

  **References**:
  - `server/internal/api/handlers_test.go` - Existing handler test pattern (if exists)
  - Gin documentation for httptest usage

  **Acceptance Criteria**:
  - [ ] API tests created
  - [ ] All test cases pass
  - [ ] HTTP status codes verified
  - [ ] Response JSON structure verified

  **QA Scenarios**:
  ```
  Scenario: All API tests pass
    Tool: Bash (go test)
    Preconditions: API tests created
    Steps:
      1. Run: go test ./internal/api/handlers_test.go -v -cover
    Expected Result: All tests pass, coverage > 80%
    Failure Indicators: Test failures, wrong status codes
    Evidence: .sisyphus/evidence/task-12-api-tests.txt
  ```

  **Commit**: YES
  - Message: `test: add comprehensive tests for async tool execution`
  - Files: `server/internal/api/handlers_test.go`
  - Pre-commit: `go test ./internal/api/...`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, curl endpoint, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go test -race ./...`. Review all changed files for: `panic` without recovery, goroutine leaks, missing context propagation, unused imports, race conditions. Check AI slop: excessive comments, over-abstraction, generic names.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Race [CLEAN/N issues] | VERDICT`

- [ ] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test cross-task integration. Test edge cases: concurrent limit, cancellation, error isolation. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **1**: `feat(tool): add ToolExecutionTracker for async state management` - server/internal/tool/tracker.go, go test ./internal/tool/...
- **2**: `feat(agent): implement parallel tool execution with semaphore` - server/internal/agent/engine.go, go test ./internal/agent/...
- **3**: `feat(api): add tool status and cancellation endpoints` - server/internal/api/handlers.go, server/internal/api/routes.go, go test ./internal/api/...
- **4**: `test: add comprehensive tests for async tool execution` - server/internal/tool/tracker_test.go, server/internal/agent/engine_parallel_test.go, go test -race ./...

---

## Success Criteria

### Verification Commands
```bash
# Unit tests
go test ./internal/tool/... -v -race
go test ./internal/agent/... -v -race

# Integration test
go test ./internal/api/... -v

# All tests with coverage
go test ./... -cover -race

# Manual verification
curl http://localhost:8080/api/tools/{callId}/status
curl -X DELETE http://localhost:8080/api/tools/{callId}/cancel
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass with race detector
- [ ] No goroutine leaks detected
- [ ] API endpoints functional
- [ ] Parallel execution verified
- [ ] Error isolation verified
- [ ] Cancellation works correctly
