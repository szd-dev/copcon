# Fix Message Handling and Todo Loop Issues

## TL;DR

> **Quick Summary**: Fix three critical bugs: (1) Add message ID and splitting logic for chat responses, (2) Return reasoning field in GetMessages API, (3) Prevent duplicate todo creation and enable auto-execution. TDD approach with backend Go changes and frontend TypeScript updates.

> **Deliverables**:
> - MessageID field in MessageData events with UUID format
> - Reasoning field returned in GetMessages API response
> - Todo state injection into agent context with duplicate detection
> - Frontend message splitting by MessageID

> **Estimated Effort**: Medium
> **Parallel Execution**: YES - 5 waves with parallel test writing
> **Critical Path**: Wave 1 (Tests) → Wave 2 (Backend) → Wave 3 (Frontend) → Wave 4 (Integration) → Wave 5 (Commits)

---

## Context

### Original Request
User reported three critical issues:
1. Chat responses lack message IDs and splitting - frontend cannot distinguish between messages
2. Reasoning (thinking process) shown during streaming but not returned in GetMessages API
3. Agent repeatedly creates todos without completing them in continuous tool calling scenarios

### Interview Summary
**Key Discussions**:
- **Issue 1 (MessageID)**: UUID format, sent with first content event, each agent loop iteration = one message
- **Issue 2 (Reasoning)**: Markdown format, already stored but not returned in API
- **Issue 3 (Todo)**: Create once then auto-execute, immediate Start after creation, Agent calls complete tool

**Research Findings**:
- **Issue 1**: MessageData lacks MessageID, generated too late (in done event), frontend uses temp IDs
- **Issue 2**: Reasoning field exists in DB and is stored, but GetMessages API doesn't return it - simple bug fix
- **Issue 3**: Agent loop never injects todo state, LLM has no visibility of existing todos, GetAvailableTodos orphaned

### Metis Review
**Identified Gaps** (addressed):
- **MessageID timing**: Resolved - send with first content event
- **Todo execution timing**: Resolved - immediate Start, subsequent Complete
- **Todo completion criteria**: Resolved - Agent explicitly calls complete tool
- **Frontend update mechanism**: Need to check if CopConChatProvider has updateMessage capability
- **Guardrails**: Must NOT refactor entire agent loop, add todo UI, or change streaming protocol

---

## Work Objectives

### Core Objective
Fix three specific bugs in message handling and todo management without changing system architecture or adding new features.

### Concrete Deliverables
- `server/internal/domain/entity/event.go`: Add MessageID to MessageData struct
- `server/internal/agent/engine.go`: Generate MessageID at loop start, inject todo state into context
- `server/internal/api/handlers.go`: Return reasoning field in GetMessages API
- `server/internal/todo/manager.go`: Add duplicate detection in Create method
- `packages/ui/src/providers/CopConChatProvider.ts`: Handle MessageID for message splitting
- Test files for all three issues following TDD approach

### Definition of Done
- [ ] All unit tests pass: `go test ./... -v` in server directory
- [ ] Frontend builds successfully: `pnpm build` in packages/ui
- [ ] Integration test covers all three fixes together
- [ ] Three atomic commits, one per issue, each passing all tests

### Must Have
- MessageID in every MessageData event
- Reasoning field in GetMessages API response
- Todo state injected into agent context before LLM call
- Duplicate todo detection prevents identical content creation
- Frontend can split messages by MessageID

### Must NOT Have (Guardrails)
- DO NOT refactor the entire agent loop architecture
- DO NOT add a todo UI/frontend component
- DO NOT change the event streaming protocol (only add MessageID field)
- DO NOT add new todo features (priority, due dates, etc.)
- DO NOT create a migration script for existing messages (assume empty DB)
- DO NOT add feature flags or toggle switches
- DO NOT change database schema beyond MessageID column addition to Message table
- DO NOT use `as any`, `@ts-ignore`, or suppress type errors

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES - testify/assert and testify/require in use
- **Automated tests**: YES (TDD) - write failing test first, then implement
- **Framework**: Go standard testing with testify
- **Pattern**: RED (failing test) → GREEN (minimal impl) → REFACTOR (if needed)

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Backend Tests**: Use `go test ./... -v` with specific test names
- **Frontend Tests**: Use `pnpm build` to verify compilation, manual demo app testing with Playwright
- **API Tests**: Use `curl` to verify HTTP responses contain expected fields
- **Integration Tests**: Use test database with setup/teardown via `t.Cleanup()`

---

## Execution Strategy

### Parallel Execution Waves

> TDD requires sequential test→implementation within each issue, but different issues can progress in parallel.

```
Wave 1 (Start Immediately - Write All Tests First):
├── Task 1: Issue 2 - Write reasoning API test [quick]
├── Task 2: Issue 1 - Write MessageID test [quick]
└── Task 3: Issue 3 - Write todo loop test [quick]

Wave 2 (After Wave 1 - Implement Backend Fixes):
├── Task 4: Issue 2 - Implement reasoning fix [quick]
├── Task 5: Issue 1 - Implement MessageID backend [quick]
└── Task 6: Issue 3 - Implement todo loop fix [unspecified-low]

Wave 3 (After Wave 2 - Frontend):
└── Task 7: Issue 1 - Frontend MessageID handling [visual-engineering]

Wave 4 (After Wave 3 - Integration):
└── Task 8: Integration test all issues [unspecified-low]

Wave 5 (After All Tests Pass - Commits):
├── Task 9: Commit Issue 2 fix [quick]
├── Task 10: Commit Issue 1 fix [quick]
└── Task 11: Commit Issue 3 fix [quick]

Wave FINAL (After ALL implementation tasks — 4 parallel reviews):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)
-> Present results -> Get explicit user okay

Critical Path: Task 2 → Task 5 → Task 7 → Task 8 → Task 10 → F1-F4 → user okay
Parallel Speedup: ~40% faster than sequential (tests written in parallel)
Max Concurrent: 3 (Wave 1 test writing)
```

### Dependency Matrix

- **1-3**: — — 4-6, 1
- **4**: 1 — 8, 9, 1
- **5**: 2 — 7, 10, 2
- **6**: 3 — 8, 11, 2
- **7**: 5 — 8, 1
- **8**: 4, 6, 7 — 9-11, 3
- **9**: 4, 8 — —
- **10**: 5, 8 — —
- **11**: 6, 8 — —
- **F1-F4**: 9-11 — —

### Agent Dispatch Summary

- **Wave 1**: **3** — T1 → `quick`, T2 → `quick`, T3 → `quick`
- **Wave 2**: **3** — T4 → `quick`, T5 → `quick`, T6 → `unspecified-low`
- **Wave 3**: **1** — T7 → `visual-engineering`
- **Wave 4**: **1** — T8 → `unspecified-low`
- **Wave 5**: **3** — T9 → `quick`, T10 → `quick`, T11 → `quick`
- **FINAL**: **4** — F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

> Implementation + Test = ONE Task. Never separate.
> EVERY task MUST have: Recommended Agent Profile + Parallelization info + QA Scenarios.

- [ ] 1. Issue 2 - Write reasoning API test

  **What to do**:
  - Create or add to `server/internal/api/handlers_test.go`
  - Write failing test `TestGetMessagesReasoning` that verifies GetMessages API returns reasoning field
  - Test should:
    1. Create a session and message with reasoning content
    2. Call GetMessages handler
    3. Assert response contains "reasoning" field with correct value
  - Use `testify/require` for assertions
  - Run test to confirm it FAILS for the right reason (field missing)

  **Must NOT do**:
  - DO NOT implement the fix yet (TDD: test first)
  - DO NOT change existing tests
  - DO NOT add unrelated test cases

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple test writing, single file, clear requirements
  - **Skills**: []
    - No special skills needed for test writing

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3)
  - **Blocks**: Task 4
  - **Blocked By**: None (can start immediately)

  **References**:
  - `server/internal/api/handlers.go:178-188` - Current GetMessages implementation (missing reasoning field)
  - `server/internal/session/model.go:101` - Message entity with Reasoning field
  - `server/internal/todo/manager_test.go` - Example test patterns with testify

  **Acceptance Criteria**:
  - [ ] Test file created or updated: server/internal/api/handlers_test.go
  - [ ] Test `TestGetMessagesReasoning` exists and compiles
  - [ ] `go test ./internal/api/... -run TestGetMessagesReasoning -v` → FAIL (expected)

  **QA Scenarios**:
  ```
  Scenario: Test fails as expected (TDD RED phase)
    Tool: Bash (go test)
    Preconditions: No implementation changes yet
    Steps:
      1. cd server
      2. go test ./internal/api/... -run TestGetMessagesReasoning -v
    Expected Result: Test FAILS with error about missing reasoning field
    Failure Indicators: Test passes (means test is wrong), compilation error
    Evidence: .sisyphus/evidence/task-1-test-fails.txt
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-1-test-fails.txt - Test failure output

  **Commit**: NO (part of Issue 2 commit later)

---

- [ ] 2. Issue 1 - Write MessageID test

  **What to do**:
  - Create or add to `server/internal/agent/engine_test.go`
  - Write failing test `TestMessageDataMessageID` that verifies:
    1. MessageData struct has MessageID field
    2. MessageID is generated at agent loop start (not at done event)
    3. MessageID is included in MessageData events during streaming
    4. MessageID is valid UUID format
    5. MessageID is consistent across all events for same message
  - Use `testify/require` and `regexp` for UUID validation
  - Run test to confirm it FAILS

  **Must NOT do**:
  - DO NOT implement MessageID generation yet
  - DO NOT change MessageData struct yet
  - DO NOT modify agent loop yet

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Test writing for well-defined requirements
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3)
  - **Blocks**: Task 5
  - **Blocked By**: None

  **References**:
  - `server/internal/domain/entity/event.go:24-26` - MessageData struct (currently no MessageID)
  - `server/internal/agent/engine.go:145-148` - Where MessageData events are emitted
  - `server/internal/agent/engine.go:255-268` - Where done event is emitted (current MessageID generation location)

  **Acceptance Criteria**:
  - [ ] Test file created or updated: server/internal/agent/engine_test.go
  - [ ] Test `TestMessageDataMessageID` exists
  - [ ] `go test ./internal/agent/... -run TestMessageDataMessageID -v` → FAIL

  **QA Scenarios**:
  ```
  Scenario: Test fails as expected
    Tool: Bash (go test)
    Steps:
      1. cd server
      2. go test ./internal/agent/... -run TestMessageDataMessageID -v
    Expected Result: Test FAILS (MessageID field doesn't exist yet)
    Evidence: .sisyphus/evidence/task-2-test-fails.txt
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-2-test-fails.txt

  **Commit**: NO

---

- [ ] 3. Issue 3 - Write todo loop test

  **What to do**:
  - Create or add to `server/internal/agent/engine_test.go`
  - Write failing test `TestTodoLoopFix` that verifies:
    1. Agent context includes existing todos before LLM call
    2. Duplicate todo detection prevents creating same content twice
    3. Todo count doesn't increase per loop iteration (no infinite creation)
    4. After todo creation, todo is automatically started (status = in_progress)
  - Use mock LLM responses to simulate tool calling scenarios
  - Run test to confirm it FAILS

  **Must NOT do**:
  - DO NOT implement todo state injection yet
  - DO NOT add duplicate detection yet
  - DO NOT modify agent loop yet

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Test writing for complex but well-defined behavior
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2)
  - **Blocks**: Task 6
  - **Blocked By**: None

  **References**:
  - `server/internal/agent/engine.go:99-272` - Agent loop (needs todo state injection)
  - `server/internal/todo/manager.go:56-90` - Create method (needs duplicate detection)
  - `server/internal/todo/manager.go:281-312` - GetAvailableTodos (orphaned method to use)
  - `server/internal/tools/todo_tool.go:121-157` - TodoTool handleCreate

  **Acceptance Criteria**:
  - [ ] Test file created or updated: server/internal/agent/engine_test.go
  - [ ] Test `TestTodoLoopFix` exists
  - [ ] `go test ./internal/agent/... -run TestTodoLoopFix -v` → FAIL

  **QA Scenarios**:
  ```
  Scenario: Test fails as expected
    Tool: Bash (go test)
    Steps:
      1. cd server
      2. go test ./internal/agent/... -run TestTodoLoopFix -v
    Expected Result: Test FAILS (todo state not injected yet)
    Evidence: .sisyphus/evidence/task-3-test-fails.txt
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-3-test-fails.txt

  **Commit**: NO

---

- [ ] 4. Issue 2 - Implement reasoning fix

  **What to do**:
  - Modify `server/internal/api/handlers.go` GetMessages handler (lines 178-188)
  - Add `"reasoning": msg.Reasoning` to the response map
  - This is a MINIMAL change - just add one field to the JSON response
  - Run test to confirm it PASSES
  - Run full test suite to ensure no regressions

  **Must NOT do**:
  - DO NOT change Message entity structure (field already exists)
  - DO NOT modify reasoning storage logic (already working)
  - DO NOT add validation or transformation of reasoning content

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Single-line fix, well-isolated change
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 5, 6)
  - **Blocks**: Task 8, Task 9
  - **Blocked By**: Task 1

  **References**:
  - `server/internal/api/handlers.go:178-188` - Where to add reasoning field
  - `server/internal/session/model.go:101` - Message.Reasoning field definition
  - Test from Task 1 - Will now pass

  **Acceptance Criteria**:
  - [ ] `go test ./internal/api/... -run TestGetMessagesReasoning -v` → PASS
  - [ ] `go test ./... -v` → All tests pass
  - [ ] Response includes reasoning field with correct value

  **QA Scenarios**:
  ```
  Scenario: Test passes after implementation (TDD GREEN phase)
    Tool: Bash (go test)
    Steps:
      1. cd server
      2. go test ./internal/api/... -run TestGetMessagesReasoning -v
    Expected Result: Test PASSES
    Evidence: .sisyphus/evidence/task-4-test-passes.txt

  Scenario: Full test suite passes
    Tool: Bash (go test)
    Steps:
      1. cd server
      2. go test ./... -v
    Expected Result: All tests PASS
    Evidence: .sisyphus/evidence/task-4-full-suite.txt

  Scenario: API returns reasoning field
    Tool: Bash (curl)
    Preconditions: Server running, session with reasoning exists
    Steps:
      1. curl http://localhost:8080/api/sessions/{sessionId}/messages
      2. Check response contains "reasoning" field
    Expected Result: JSON response includes reasoning field
    Evidence: .sisyphus/evidence/task-4-api-response.json
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-4-test-passes.txt
  - [ ] .sisyphus/evidence/task-4-full-suite.txt
  - [ ] .sisyphus/evidence/task-4-api-response.json

  **Commit**: NO (commit in Wave 5)

---

- [ ] 5. Issue 1 - Implement MessageID backend

  **What to do**:
  - **Step 1**: Add `MessageID string` field to `MessageData` struct in `server/internal/domain/entity/event.go`
  - **Step 2**: In `server/internal/agent/engine.go`, generate MessageID at loop start:
    - Before streaming begins (around line 100-120), generate: `messageID := uuid.New().String()`
    - Include messageID in every MessageData event emission (lines 145-148)
  - **Step 3**: Add `message_id` column to Message table:
    - Option A: GORM auto-migrate (add field to Message struct, run migration)
    - Option B: Manual migration SQL
  - **Step 4**: Run test to confirm it PASSES
  - **Step 5**: Run full test suite

  **Must NOT do**:
  - DO NOT create new event types (use existing MessageData)
  - DO NOT change done event logic (keep MessageID there too)
  - DO NOT modify frontend yet (separate task)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Struct field addition and simple generation logic
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 4, 6)
  - **Blocks**: Task 7, Task 10
  - **Blocked By**: Task 2

  **References**:
  - `server/internal/domain/entity/event.go:24-26` - Add MessageID to MessageData
  - `server/internal/agent/engine.go:100-120` - Where to generate MessageID at loop start
  - `server/internal/agent/engine.go:145-148` - Where to include MessageID in events
  - `server/internal/session/model.go` - Message entity (may need message_id column)
  - Test from Task 2 - Will now pass

  **Acceptance Criteria**:
  - [ ] `go test ./internal/agent/... -run TestMessageDataMessageID -v` → PASS
  - [ ] `go test ./... -v` → All tests pass
  - [ ] MessageData events contain valid UUID MessageID
  - [ ] MessageID consistent across all events for same message

  **QA Scenarios**:
  ```
  Scenario: Test passes after implementation
    Tool: Bash (go test)
    Steps:
      1. cd server
      2. go test ./internal/agent/... -run TestMessageDataMessageID -v
    Expected Result: Test PASSES
    Evidence: .sisyphus/evidence/task-5-test-passes.txt

  Scenario: SSE stream contains MessageID
    Tool: Bash (curl)
    Preconditions: Server running
    Steps:
      1. curl -N http://localhost:8080/api/sessions/{sessionId}/chat \
           -H "Content-Type: application/json" \
           -d '{"content":"test","agent_id":"default"}'
      2. Capture SSE stream
      3. Grep for MessageID in message events
    Expected Result: Each message event has "messageId":"UUID"
    Evidence: .sisyphus/evidence/task-5-sse-messageid.txt

  Scenario: MessageID is valid UUID format
    Tool: Bash (grep + regex)
    Steps:
      1. Extract messageId from captured SSE stream
      2. Validate UUID format with regex
    Expected Result: Matches UUID v4 pattern
    Evidence: .sisyphus/evidence/task-5-uuid-valid.txt
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-5-test-passes.txt
  - [ ] .sisyphus/evidence/task-5-sse-messageid.txt
  - [ ] .sisyphus/evidence/task-5-uuid-valid.txt

  **Commit**: NO

---

- [ ] 6. Issue 3 - Implement todo loop fix

  **What to do**:
  - **Step 1**: Inject todo state into agent context:
    - In `server/internal/agent/engine.go`, before BuildContext call (around line 99)
    - Fetch existing todos: `todos, err := tm.List(chatCtx)` (where tm is TodoManager)
    - Add todos to system prompt or context messages
    - Format: "Current todo list: [pending: Fix bug X, in_progress: List files, completed: ...]"
  - **Step 2**: Add duplicate detection in TodoManager.Create:
    - In `server/internal/todo/manager.go:56-90`, before creating new todo
    - Check if todo with same content already exists: `SELECT COUNT(*) FROM todos WHERE session_id = ? AND content = ? AND status != 'completed'`
    - If exists, return error or existing todo (choose based on test expectations)
  - **Step 3**: Auto-start todo after creation:
    - After todo creation in agent loop, call `tm.Start(chatCtx, todoID)` immediately
  - **Step 4**: Run test to confirm it PASSES
  - **Step 5**: Run full test suite

  **Must NOT do**:
  - DO NOT add todo UI/frontend component
  - DO NOT add new todo fields (priority, due dates)
  - DO NOT remove GetAvailableTodos method (may be useful)
  - DO NOT change todo status constants

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
    - Reason: Multiple small changes, requires understanding agent loop and todo manager
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 4, 5)
  - **Blocks**: Task 8, Task 11
  - **Blocked By**: Task 3

  **References**:
  - `server/internal/agent/engine.go:99` - Where to inject todo state
  - `server/internal/todo/manager.go:56-90` - Create method to add duplicate detection
  - `server/internal/todo/manager.go:281-312` - GetAvailableTodos (may use for context)
  - `server/internal/todo/manager.go:100-130` - Start method (for auto-start)
  - Test from Task 3 - Will now pass

  **Acceptance Criteria**:
  - [ ] `go test ./internal/agent/... -run TestTodoLoopFix -v` → PASS
  - [ ] `go test ./... -v` → All tests pass
  - [ ] Agent context contains todo list before LLM call
  - [ ] Duplicate todo creation returns error or existing todo
  - [ ] Todo count stays constant across loop iterations

  **QA Scenarios**:
  ```
  Scenario: Test passes after implementation
    Tool: Bash (go test)
    Steps:
      1. cd server
      2. go test ./internal/agent/... -run TestTodoLoopFix -v
    Expected Result: Test PASSES
    Evidence: .sisyphus/evidence/task-6-test-passes.txt

  Scenario: Duplicate todo detection works
    Tool: Bash (go test)
    Steps:
      1. Create integration test or use curl to create same todo twice
      2. Check database has only one todo
    Expected Result: Second creation prevented
    Evidence: .sisyphus/evidence/task-6-duplicate-prevention.txt

  Scenario: Todo auto-starts after creation
    Tool: Bash (database query)
    Steps:
      1. Trigger todo creation via agent
      2. Query database for todo status
    Expected Result: Status is 'in_progress', not 'pending'
    Evidence: .sisyphus/evidence/task-6-auto-start.txt
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-6-test-passes.txt
  - [ ] .sisyphus/evidence/task-6-duplicate-prevention.txt
  - [ ] .sisyphus/evidence/task-6-auto-start.txt

  **Commit**: NO

---

- [ ] 7. Issue 1 - Frontend MessageID handling

  **What to do**:
  - Modify `packages/ui/src/providers/CopConChatProvider.ts`
  - Update SSE event transformer to:
    1. Extract MessageID from MessageData events
    2. Map messages by MessageID instead of using temp IDs
    3. Group message chunks by MessageID for splitting
  - Update `useAgentChat.ts` if needed for message splitting logic
  - Test in demo app: `cd packages/demo && pnpm dev`
  - Verify messages split correctly by MessageID in UI

  **Must NOT do**:
  - DO NOT change backend MessageID format
  - DO NOT add message UI redesign
  - DO NOT change event streaming protocol
  - DO NOT use `as any` or `@ts-ignore`

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: Frontend TypeScript changes, UI behavior testing
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 3 (sequential)
  - **Blocks**: Task 8
  - **Blocked By**: Task 5

  **References**:
  - `packages/ui/src/providers/CopConChatProvider.ts:98` - Current temp ID generation
  - `packages/ui/src/providers/CopConChatProvider.ts:121-125` - Message accumulation logic
  - `packages/ui/src/api/types.ts` - Message interface with messageId field
  - Backend changes from Task 5 - MessageID now in SSE events

  **Acceptance Criteria**:
  - [ ] `pnpm build` in packages/ui → Success
  - [ ] Demo app runs without errors
  - [ ] Messages split by MessageID in UI (each ID = separate message bubble)

  **QA Scenarios**:
  ```
  Scenario: Frontend builds successfully
    Tool: Bash (pnpm)
    Steps:
      1. cd packages/ui
      2. pnpm build
    Expected Result: Build succeeds, no TypeScript errors
    Evidence: .sisyphus/evidence/task-7-build-success.txt

  Scenario: Demo app handles MessageID
    Tool: Playwright
    Preconditions: Server running, demo app running
    Steps:
      1. Navigate to http://localhost:5173
      2. Send chat message that triggers multi-part response
      3. Verify multiple message bubbles appear
      4. Each bubble corresponds to different MessageID
    Expected Result: Messages split correctly, no duplicate content
    Failure Indicators: All content in one bubble, or duplicate bubbles
    Evidence: .sisyphus/evidence/task-7-message-splitting.png
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-7-build-success.txt
  - [ ] .sisyphus/evidence/task-7-message-splitting.png

  **Commit**: NO

---

- [ ] 8. Integration test all issues

  **What to do**:
  - Create end-to-end integration test in `server/internal/integration_test.go`
  - Test scenario that covers all three fixes together:
    1. Start server with test database
    2. Send chat request with reasoning
    3. Verify MessageID present in SSE events
    4. Verify reasoning stored and returned in GetMessages
    5. Trigger todo creation, verify no duplicate and auto-start
    6. Run multiple agent iterations, verify todo count stable
  - Use `setupTestDB(t)` pattern with `t.Cleanup()`
  - Run test and verify it PASSES

  **Must NOT do**:
  - DO NOT create new test infrastructure (use existing patterns)
  - DO NOT mock unrelated components
  - DO NOT test features beyond the three issues

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
    - Reason: Integration test writing, requires understanding all three fixes
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 4 (sequential)
  - **Blocks**: Task 9, 10, 11
  - **Blocked By**: Task 4, Task 6, Task 7

  **References**:
  - `server/internal/todo/manager_test.go` - Test setup pattern
  - All previous tasks' implementations - The fixes to test
  - `server/internal/api/handlers.go` - For API testing
  - `server/internal/agent/engine.go` - For agent loop testing

  **Acceptance Criteria**:
  - [ ] `go test ./... -run TestIntegrationAllIssues -v` → PASS
  - [ ] All three fixes verified working together

  **QA Scenarios**:
  ```
  Scenario: Integration test passes
    Tool: Bash (go test)
    Steps:
      1. cd server
      2. go test ./... -run TestIntegrationAllIssues -v
    Expected Result: Test PASSES, all scenarios verified
    Evidence: .sisyphus/evidence/task-8-integration-passes.txt

  Scenario: End-to-end message flow works
    Tool: Bash (curl + database)
    Steps:
      1. Create session
      2. Send chat with reasoning content
      3. Verify SSE events have MessageID
      4. Query GetMessages, verify reasoning returned
      5. Verify message_id stored in database
    Expected Result: All steps succeed
    Evidence: .sisyphus/evidence/task-8-e2e-message.txt
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-8-integration-passes.txt
  - [ ] .sisyphus/evidence/task-8-e2e-message.txt

  **Commit**: NO

---

- [ ] 9. Commit Issue 2 fix

  **What to do**:
  - Create atomic commit for Issue 2 (reasoning API fix)
  - Commit message: `fix(api): return reasoning field in GetMessages response`
  - Files to include: `server/internal/api/handlers.go`, `server/internal/api/handlers_test.go`
  - Run `go test ./... -v` after commit to verify tests still pass

  **Must NOT do**:
  - DO NOT include other issues' changes in this commit
  - DO NOT commit if tests fail
  - DO NOT use generic commit message

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple git commit
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 5 (sequential)
  - **Blocks**: None
  - **Blocked By**: Task 4, Task 8

  **References**:
  - Changes from Task 4 - Implementation to commit
  - Test from Task 1 - Test to commit

  **Acceptance Criteria**:
  - [ ] `git log -1 --oneline` shows commit
  - [ ] `go test ./... -v` still passes after commit

  **QA Scenarios**:
  ```
  Scenario: Commit created successfully
    Tool: Bash (git)
    Steps:
      1. git add server/internal/api/handlers.go server/internal/api/handlers_test.go
      2. git commit -m "fix(api): return reasoning field in GetMessages response"
      3. git log -1 --oneline
    Expected Result: Commit appears in log
    Evidence: .sisyphus/evidence/task-9-commit.txt

  Scenario: Tests pass after commit
    Tool: Bash (go test)
    Steps:
      1. cd server
      2. go test ./... -v
    Expected Result: All tests PASS
    Evidence: .sisyphus/evidence/task-9-post-commit-tests.txt
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-9-commit.txt
  - [ ] .sisyphus/evidence/task-9-post-commit-tests.txt

  **Commit**: YES (this IS the commit task)
  - Message: `fix(api): return reasoning field in GetMessages response`
  - Files: `server/internal/api/handlers.go`, `server/internal/api/handlers_test.go`
  - Pre-commit: `go test ./... -v`

---

- [ ] 10. Commit Issue 1 fix

  **What to do**:
  - Create atomic commit for Issue 1 (MessageID fix)
  - Commit message: `fix(agent): add MessageID to chat message streaming`
  - Files to include:
    - `server/internal/domain/entity/event.go`
    - `server/internal/agent/engine.go`
    - `server/internal/agent/engine_test.go`
    - `packages/ui/src/providers/CopConChatProvider.ts`
    - `packages/ui/src/hooks/useAgentChat.ts` (if modified)
  - Run tests after commit

  **Must NOT do**:
  - DO NOT include Issue 2 or Issue 3 changes

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 5
  - **Blocks**: None
  - **Blocked By**: Task 5, Task 7, Task 8

  **References**:
  - Changes from Task 5 (backend)
  - Changes from Task 7 (frontend)
  - Test from Task 2

  **Acceptance Criteria**:
  - [ ] Commit created
  - [ ] Tests pass

  **QA Scenarios**:
  ```
  Scenario: Commit created
    Tool: Bash (git)
    Steps:
      1. git add [files listed above]
      2. git commit -m "fix(agent): add MessageID to chat message streaming"
    Expected Result: Commit created
    Evidence: .sisyphus/evidence/task-10-commit.txt
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-10-commit.txt

  **Commit**: YES
  - Message: `fix(agent): add MessageID to chat message streaming`
  - Files: Backend + frontend changes
  - Pre-commit: `go test ./... -v && cd packages/ui && pnpm build`

---

- [ ] 11. Commit Issue 3 fix

  **What to do**:
  - Create atomic commit for Issue 3 (todo loop fix)
  - Commit message: `fix(agent): prevent duplicate todo creation and inject todo state`
  - Files: `server/internal/agent/engine.go`, `server/internal/todo/manager.go`, test files
  - Run tests

  **Must NOT do**:
  - DO NOT mix with other issues

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 5
  - **Blocked By**: Task 6, Task 8

  **Acceptance Criteria**:
  - [ ] Commit created
  - [ ] Tests pass

  **QA Scenarios**:
  ```
  Scenario: Commit created
    Tool: Bash (git)
    Steps:
      1. git add server/internal/agent/engine.go server/internal/todo/manager.go [tests]
      2. git commit -m "fix(agent): prevent duplicate todo creation and inject todo state"
    Expected Result: Commit created
    Evidence: .sisyphus/evidence/task-11-commit.txt
  ```

  **Evidence to Capture**:
  - [ ] .sisyphus/evidence/task-11-commit.txt

  **Commit**: YES
  - Message: `fix(agent): prevent duplicate todo creation and inject todo state`
  - Pre-commit: `go test ./... -v`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, curl endpoint, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go test ./... -v` + `go vet ./...` + `go fmt ./...`. Review all changed files for: `as any`/`@ts-ignore`, empty catches, console.log in prod, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names (data/result/item/temp).
  Output: `Build [PASS/FAIL] | Lint [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [ ] F3. **Real Manual QA** — `unspecified-high`
  Start server and demo app. Test all three fixes: (1) Send chat, verify MessageID in events and message splitting in UI. (2) Load messages, verify reasoning displayed. (3) Trigger todo creation, verify no duplicate and auto-execution. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **Commit 1**: `fix(api): return reasoning field in GetMessages response` — server/internal/api/handlers.go, go test ./...
- **Commit 2**: `fix(agent): add MessageID to chat message streaming` — server/internal/domain/entity/event.go, server/internal/agent/engine.go, packages/ui/src/providers/CopConChatProvider.ts, go test ./...
- **Commit 3**: `fix(agent): prevent duplicate todo creation and inject todo state` — server/internal/agent/engine.go, server/internal/todo/manager.go, go test ./...

---

## Success Criteria

### Verification Commands
```bash
# Backend tests
cd server && go test ./... -v  # Expected: PASS

# Frontend build
cd packages/ui && pnpm build  # Expected: Build successful

# API verification - GetMessages returns reasoning
curl http://localhost:8080/api/sessions/{sessionId}/messages | jq '.[0].reasoning'
# Expected: string or null

# SSE verification - MessageID in events
curl -N http://localhost:8080/api/sessions/{sessionId}/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"test","agent_id":"default"}' \
  | grep -o '"messageId":"[^"]*"'
# Expected: UUID format

# Todo loop verification - no duplicates
# Run agent for multiple iterations, check todo count stays constant
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass
- [ ] Three atomic commits created
