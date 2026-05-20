# Agent Engine Refactor - runAgentLoop Restructuring

## TL;DR

> **Quick Summary**: Refactor the 222-line `runAgentLoop()` method in `server/internal/agent/engine.go` into structured, testable phases without changing external behavior.
> 
> **Deliverables**:
> - Extracted helper methods with single responsibilities
> - Tool list retrieval moved outside loop
> - Unified message persistence logic
> - Test infrastructure for safe refactoring
> 
> **Estimated Effort**: Short (2-4 hours with test infrastructure)
> **Parallel Execution**: NO - Sequential refactoring
> **Critical Path**: Test setup → Extract methods → Verify behavior

---

## Context

### Original Request
用户对 `agent/engine.go` 的 `runAgentLoop` 架构不满意，认为缺乏结构化，流水账式的代码不便于阅读和扩展。希望拆分成：agent定义获取、获取工具列表、进入循环、构建context、调用AI、执行工具调用、更新context。

### Interview Summary

**Key Discussions**:
- **问题确认**: 工具列表在循环内重复获取（Line 134）、Todo state 注入逻辑散乱、消息持久化重复、流式处理混杂
- **设计方案**: Phase 1 (Initialization) → Phase 2 (Loop) → Phase 3 (Tool Execution)
- **设计决策**: Todo注入移到 BuildContext 内部、日志保持内部、引入数据结构、保持简单循环、不抽象LLM Provider

**Research Findings**:
- **当前代码**: 222行 God Function，混杂多种职责
- **测试覆盖**: 现有测试不覆盖 runAgentLoop 内部逻辑（重大风险！）
- **紧耦合**: OpenAI SDK 紧耦合导致难以测试

### Metis Review

**Critical Findings** (MUST address):

1. **测试覆盖缺口** (CRITICAL): 现有测试不覆盖流式处理、工具调用累积、消息持久化逻辑
   - **Solution**: 创建 Mock OpenAI Stream 接口，添加单元测试

2. **Todo注入依赖** (HIGH): 移到 BuildContext() 会创建循环依赖（ContextManager → TodoManager）
   - **Solution**: 在 AgentEngine.buildLLMContext() 中处理 Todo，而不是移到 ContextManager

3. **消息持久化重复** (MEDIUM): Lines 258-266 vs 277-284 几乎相同
   - **Solution**: 统一到 persistMessage() helper

**Identified Gaps** (addressed):
- [Gap] 测试基础设施缺失 → Plan includes Mock OpenAI creation
- [Gap] 工具列表重复获取 → Move outside loop
- [Gap] 消息持久化重复 → Unified helper
- [Gap] Todo注入依赖问题 → Keep in AgentEngine, not ContextManager

---

## Work Objectives

### Core Objective
将 `runAgentLoop()` 从 222 行的 God Function 重构为结构化的多个阶段，提高可读性和可维护性，同时保持外部接口和行为不变。

### Concrete Deliverables
- `server/internal/agent/engine.go`:
  - 新增 helper 方法：prepareAgentLoop, buildLLMContext, createLLMRequest, handleStreaming, persistMessage, handleToolCalls
  - 重写 runAgentLoop 为简洁的循环结构
  - 引入数据结构：StreamResult, LLMRequest
- `server/internal/agent/engine_test.go`:
  - Mock OpenAI Stream 实现
  - 单元测试覆盖核心逻辑

### Definition of Done
- [ ] 所有现有测试通过
- [ ] 新增单元测试覆盖流式处理逻辑
- [ ] 每个提取的方法不超过 50 行
- [ ] 工具列表检索移到循环外
- [ ] 消息持久化逻辑统一
- [ ] 集成测试验证行为不变

### Must Have
- ✅ 提取独立方法，职责单一
- ✅ 工具列表移出循环
- ✅ 消息持久化统一
- ✅ 测试覆盖核心逻辑
- ✅ 保持外部接口不变
- ✅ 保持行为完全一致

### Must NOT Have (Guardrails)
- ❌ 不改变 `Chat()` 方法签名
- ❌ 不引入 LLMProvider 抽象
- ❌ 不改变事件发送顺序
- ❌ 不创建 ContextManager → TodoManager 依赖
- ❌ 不引入状态机
- ❌ 不添加新事件类型
- ❌ 不改变消息持久化时序

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES (Go test framework)
- **Automated tests**: YES (TDD approach - tests BEFORE refactor)
- **Framework**: `go test`
- **TDD**: YES - Write tests first, then refactor

### QA Policy
Every task MUST include agent-executed QA scenarios.

- **Backend**: Use `go test` - Run unit tests, integration tests, verify coverage
- **Code Quality**: Use `go vet`, `go fmt`, `golangci-lint` (if available)
- **Behavior Verification**: Compare event logs, message persistence before/after

---

## Execution Strategy

### Parallel Execution Waves

> Sequential refactoring - each step builds on the previous.

```
Wave 1 (Test Infrastructure - Day 1 Morning):
├── Task 1: Create Mock OpenAI Stream [deep]
├── Task 2: Add unit tests for streaming accumulation [deep]
└── Task 3: Add unit tests for tool call merging [deep]

Wave 2 (Method Extraction - Day 1 Afternoon):
├── Task 4: Extract prepareAgentLoop helper [quick]
├── Task 5: Move tool list retrieval outside loop [quick]
├── Task 6: Move todo package and update ContextManager [quick]
├── Task 7: Add StreamResult data structure [quick]
├── Task 8: Extract handleStreaming helper [deep]
├── Task 9: Extract persistMessage helper [quick]
└── Task 10: Extract handleToolCalls helper [quick]

Wave 3 (Finalization - Day 1 Evening):
├── Task 11: Rewrite runAgentLoop main loop [quick]
├── Task 12: Run all tests and verify [quick]
└── Task 13: Integration test with real OpenAI [quick]

Critical Path: T1 → T2/T3 → T4 → T5 → T6 → T7 → T8 → T9 → T10 → T11 → T12 → T13
```

### Dependency Matrix

- **1**: - 2, 3, 4
- **2, 3**: 1 - 8, 12
- **4**: 1 - 5, 6, 11
- **5**: 4 - 8, 11
- **6**: 4 - 7, 8
- **7**: 6 - 8
- **8**: 5, 7, 2, 3 - 9, 11
- **9**: 8 - 10, 11
- **10**: 9 - 11
- **11**: 4, 5, 6, 8, 9, 10 - 12
- **12**: 11, 2, 3 - 13
- **13**: 12

### Agent Dispatch Summary

- **Wave 1**: **3** - T1 → `deep`, T2 → `deep`, T3 → `deep`
- **Wave 2**: **7** - T4 → `quick`, T5 → `quick`, T6 → `quick`, T7 → `quick`, T8 → `deep`, T9 → `quick`, T10 → `quick`
- **Wave 3**: **3** - T11 → `quick`, T12 → `quick`, T13 → `quick`

---

## TODOs

- [x] 1. Create Mock OpenAI Stream Interface

  **What to do**:
  - Create `MockOpenAIStream` struct that simulates OpenAI streaming behavior
  - Implement methods to feed chunks (content, reasoning, tool calls)
  - Implement error injection capability
  - Add to `engine_test.go` or separate `mock_openai_test.go`

  **Must NOT do**:
  - Do NOT modify production code
  - Do NOT create new interfaces in production packages

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Requires understanding OpenAI SDK interfaces and mocking patterns
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Tasks 2, 3, 4
  - **Blocked By**: None

  **References**:
  - `server/internal/agent/engine.go:147-224` - Current streaming loop implementation to mock
  - `github.com/openai/openai-go/v3` - OpenAI SDK interfaces to mock

  **Acceptance Criteria**:
  - [ ] MockOpenAIStream struct created in test file
  - [ ] Can simulate content chunks
  - [ ] Can simulate reasoning chunks
  - [ ] Can simulate tool call deltas
  - [ ] Can inject stream errors

  **QA Scenarios**:
  ```
  Scenario: Mock stream returns content chunks
    Tool: Bash (go test)
    Preconditions: MockOpenAIStream implementation exists
    Steps:
      1. Create mock with content chunks ["Hello", " World"]
      2. Run test that consumes stream
      3. Verify accumulated content = "Hello World"
    Expected Result: Content correctly accumulated
    Evidence: .sisyphus/evidence/task-1-mock-content.txt

  Scenario: Mock stream returns tool call deltas
    Tool: Bash (go test)
    Preconditions: MockOpenAIStream implementation exists
    Steps:
      1. Create mock with tool call deltas (name="test", args chunks=["{", "\"key\"", ":", "\"value\"", "}"])
      2. Run test that consumes stream
      3. Verify tool call merged correctly
    Expected Result: Tool call arguments = `{"key":"value"}`
    Evidence: .sisyphus/evidence/task-1-mock-toolcall.txt
  ```

  **Commit**: YES
  - Message: `test: add MockOpenAIStream for testing runAgentLoop`
  - Files: `server/internal/agent/engine_test.go` or `server/internal/agent/mock_openai_test.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 2. Add Unit Tests for Streaming Accumulation

  **What to do**:
  - Create `TestRunAgentLoop_StreamingAccumulation` test
  - Test content delta accumulation
  - Test reasoning delta accumulation
  - Test tool call delta merging (multiple deltas for same tool call)
  - Use MockOpenAIStream from Task 1

  **Must NOT do**:
  - Do NOT modify production code
  - Do NOT test with real OpenAI API

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Requires understanding streaming accumulation logic in detail
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Task 3)
  - **Blocks**: Task 8, Task 12
  - **Blocked By**: Task 1

  **References**:
  - `server/internal/agent/engine.go:155-224` - Streaming loop logic to test
  - `server/internal/agent/engine.go:164-170` - Content delta handling
  - `server/internal/agent/engine.go:172-181` - Reasoning delta handling
  - `server/internal/agent/engine.go:183-206` - Tool call delta accumulation

  **Acceptance Criteria**:
  - [ ] Test file created: `engine_test.go`
  - [ ] Test `TestRunAgentLoop_StreamingAccumulation` passes
  - [ ] Coverage includes content, reasoning, and tool call accumulation

  **QA Scenarios**:
  ```
  Scenario: Content accumulation test passes
    Tool: Bash (go test)
    Preconditions: MockOpenAIStream and test exist
    Steps:
      1. Run: cd server && go test ./internal/agent/... -v -run TestRunAgentLoop_StreamingAccumulation
      2. Verify test passes
    Expected Result: PASS with content accumulation verified
    Evidence: .sisyphus/evidence/task-2-streaming-test.txt

  Scenario: Tool call merging test passes
    Tool: Bash (go test)
    Preconditions: MockOpenAIStream and test exist
    Steps:
      1. Run: cd server && go test ./internal/agent/... -v -run TestRunAgentLoop_StreamingAccumulation
      2. Verify tool call delta merging logic tested
    Expected Result: PASS with tool call merging verified
    Evidence: .sisyphus/evidence/task-2-toolcall-merge.txt
  ```

  **Commit**: YES
  - Message: `test: add unit tests for streaming accumulation`
  - Files: `server/internal/agent/engine_test.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 3. Add Unit Tests for Tool Call Merging

  **What to do**:
  - Create `TestRunAgentLoop_ToolCallMerging` test
  - Test multiple deltas for same tool call index
  - Test JustFinishedToolCall fallback path
  - Test tool call map to slice conversion (ordered by index)
  - Use MockOpenAIStream from Task 1

  **Must NOT do**:
  - Do NOT modify production code
  - Do NOT test with real OpenAI API

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Requires understanding complex tool call accumulation logic
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Task 2)
  - **Blocks**: Task 8, Task 12
  - **Blocked By**: Task 1

  **References**:
  - `server/internal/agent/engine.go:183-223` - Tool call delta accumulation and merging
  - `server/internal/agent/engine.go:208-223` - JustFinishedToolCall fallback
  - `server/internal/agent/engine.go:226-230` - Map to slice conversion

  **Acceptance Criteria**:
  - [ ] Test `TestRunAgentLoop_ToolCallMerging` passes
  - [ ] Coverage includes delta merging, fallback, and ordering

  **QA Scenarios**:
  ```
  Scenario: Tool call merging test passes
    Tool: Bash (go test)
    Preconditions: MockOpenAIStream and test exist
    Steps:
      1. Run: cd server && go test ./internal/agent/... -v -run TestRunAgentLoop_ToolCallMerging
      2. Verify test passes
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-3-toolcall-merge-test.txt
  ```

  **Commit**: YES
  - Message: `test: add unit tests for tool call merging`
  - Files: `server/internal/agent/engine_test.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 4. Extract prepareAgentLoop Helper

  **What to do**:
  - Extract lines 72-102 into `prepareAgentLoop(chatCtx, userInput) (*AgentDefinition, error)`
  - Include: session retrieval, agent resolution (3-layer fallback), user message persistence
  - Update `runAgentLoop` to call this helper
  - Add comments referencing original line numbers

  **Must NOT do**:
  - Do NOT change any behavior
  - Do NOT modify interfaces

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple extraction, no logic changes
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Tasks 5, 6, 11
  - **Blocked By**: Task 1

  **References**:
  - `server/internal/agent/engine.go:72-102` - Code to extract
  - `server/internal/agent/engine.go:77-89` - Agent resolution logic
  - `server/internal/agent/engine.go:97-102` - User message persistence

  **Acceptance Criteria**:
  - [ ] Method `prepareAgentLoop` created
  - [ ] Method body < 50 lines
  - [ ] `runAgentLoop` calls `prepareAgentLoop`
  - [ ] All tests pass

  **QA Scenarios**:
  ```
  Scenario: Extraction preserves behavior
    Tool: Bash (go test)
    Preconditions: prepareAgentLoop extracted
    Steps:
      1. Run: cd server && go test ./internal/agent/... -v
      2. Verify all tests pass
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-4-extract-prepare.txt
  ```

  **Commit**: YES
  - Message: `refactor(agent): extract prepareAgentLoop helper`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 5. Move Tool List Retrieval Outside Loop

  **What to do**:
  - Move line 134 (`tools := agentDef.ToolManager.GetOpenAITools()`) to before line 104 (loop start)
  - Store in variable accessible in loop
  - Remove duplicate retrieval inside loop
  - Add comment explaining why tools are static per agent

  **Must NOT do**:
  - Do NOT change tool execution behavior
  - Do NOT modify ToolManager interface

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple code movement, no logic changes
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Tasks 8, 11
  - **Blocked By**: Task 4

  **References**:
  - `server/internal/agent/engine.go:134` - Current tool list retrieval (inside loop)
  - `server/internal/agent/engine.go:104` - Loop start (move before this)

  **Acceptance Criteria**:
  - [ ] Tool list retrieval moved before loop
  - [ ] Only one call to `GetOpenAITools()` per agent loop
  - [ ] All tests pass

  **QA Scenarios**:
  ```
  Scenario: Tool list retrieved once
    Tool: Bash (grep)
    Preconditions: Tool list moved outside loop
    Steps:
      1. Run: grep -n "GetOpenAITools" server/internal/agent/engine.go
      2. Verify single occurrence
    Expected Result: Single occurrence before loop
    Evidence: .sisyphus/evidence/task-5-tool-list-location.txt
  ```

  **Commit**: YES
  - Message: `refactor(agent): move tool list retrieval outside loop`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 6. Move Todo Package to tools/todo and Modify ContextManager

  **What to do**:
  - Move `server/internal/todo/` directory to `server/internal/tools/todo/`
  - Update all import paths from `github.com/copcon/server/internal/todo` to `github.com/copcon/server/internal/tools/todo`
  - Update `ContextManager` constructor to accept `TodoManager` dependency
  - Modify `BuildContext()` to internally call `todoMgr.List()` and inject todo state
  - Move `formatTodoState()` function from `engine.go` to `chat_context/manager.go`
  - Update `main.go` dependency injection to pass `todoMgr` to `ContextManager`
  - Remove todo handling from `AgentEngine.runAgentLoop()` (lines 110-118)

  **Must NOT do**:
  - Do NOT create circular dependency (TodoManager should NOT depend on ContextManager)
  - Do NOT change BuildContext() signature (keep it simple)
  - Do NOT break existing TodoTool functionality

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Package reorganization and dependency injection change
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Tasks 7, 8
  - **Blocked By**: Task 4

  **References**:
  - `server/internal/todo/` - Current package location (move to tools/todo/)
  - `server/internal/tools/` - Target parent directory
  - `server/internal/chat_context/manager.go` - ContextManager implementation to modify
  - `server/internal/agent/engine.go:110-118` - Current todo injection logic (remove)
  - `server/internal/agent/engine.go:382-422` - `formatTodoState()` function (move)
  - `server/cmd/server/main.go` - Dependency injection setup to update
  - `server/internal/tools/todo_tool.go` - TodoTool that depends on TodoManager

  **Acceptance Criteria**:
  - [ ] `server/internal/tools/todo/` directory created
  - [ ] All imports updated to `internal/tools/todo`
  - [ ] `ContextManager` constructor accepts `TodoManager`
  - [ ] `BuildContext()` internally handles todo injection
  - [ ] `formatTodoState()` moved to `chat_context` package
  - [ ] `AgentEngine.runAgentLoop()` no longer has todo handling code
  - [ ] TodoTool still works correctly
  - [ ] All tests pass

  **QA Scenarios**:
  ```
  Scenario: Package structure reorganized
    Tool: Bash (ls)
    Preconditions: Package moved
    Steps:
      1. Run: ls server/internal/tools/todo/
      2. Verify manager.go exists
    Expected Result: File exists in new location
    Evidence: .sisyphus/evidence/task-6-package-structure.txt

  Scenario: Imports updated
    Tool: Bash (grep)
    Preconditions: Imports updated
    Steps:
      1. Run: grep -r "internal/todo" server/internal/ | grep -v "tools/todo"
      2. Verify no old import paths (excluding new tools/todo path)
    Expected Result: No occurrences of old import path
    Evidence: .sisyphus/evidence/task-6-imports.txt

  Scenario: Todo injection moved to ContextManager
    Tool: Bash (go test)
    Preconditions: ContextManager modified
    Steps:
      1. Run: cd server && go test ./internal/chat_context/... -v
      2. Verify context building includes todo state
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-6-context-todo.txt

  Scenario: TodoTool still works
    Tool: Bash (go test)
    Preconditions: Package reorganized
    Steps:
      1. Run: cd server && go test ./internal/tools/... -v
      2. Verify TodoTool tests pass
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-6-todotool.txt
  ```

  **Commit**: YES
  - Message: `refactor: move todo package to tools/todo and update ContextManager`
  - Files: `server/internal/tools/todo/manager.go`, `server/internal/chat_context/manager.go`, `server/internal/agent/engine.go`, `server/cmd/server/main.go`, and all files with updated imports
  - Pre-commit: `go test ./... -v`

- [x] 7. Add StreamResult Data Structure

  **What to do**:
  - Define `StreamResult` struct with fields: MessageID, Content, ReasoningContent, ToolCalls, Usage
  - Define `LLMRequest` struct with fields: Model, Messages, Tools, Params
  - Add comments explaining purpose
  - No behavior change yet - just data structure definitions

  **Must NOT do**:
  - Do NOT change any existing behavior
  - Do NOT export these types if not needed externally

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple data structure definition
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 8
  - **Blocked By**: Task 6

  **References**:
  - `server/internal/agent/engine.go:150-153` - Current local variables to encapsulate
  - `server/internal/agent/engine.go:30-35` - Existing `toolCallInfo` struct pattern

  **Acceptance Criteria**:
  - [ ] `StreamResult` struct defined
  - [ ] `LLMRequest` struct defined
  - [ ] Fields match current usage
  - [ ] All tests pass

  **QA Scenarios**:
  ```
  Scenario: Data structures compile
    Tool: Bash (go build)
    Preconditions: Structs defined
    Steps:
      1. Run: cd server && go build ./internal/agent/...
      2. Verify compilation succeeds
    Expected Result: Build succeeds
    Evidence: .sisyphus/evidence/task-7-data-structs.txt
  ```

  **Commit**: YES
  - Message: `refactor(agent): add StreamResult and LLMRequest data structures`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 8. Extract handleStreaming Helper

  **What to do**:
  - Extract lines 140-230 into `handleStreaming(chatCtx, client, request) (*StreamResult, error)`
  - Include: OpenAI stream creation, delta processing (content, reasoning, tool calls), error handling
  - Emit events inside this method
  - Return `StreamResult` struct
  - This is the most complex extraction - use tests from Tasks 2-3 to verify

  **Must NOT do**:
  - Do NOT change streaming behavior
  - Do NOT buffer content (emit as-you-go)
  - Do NOT change event emission order

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Complex streaming logic with delta accumulation and event emission
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Tasks 9, 11
  - **Blocked By**: Tasks 5, 7, 2, 3

  **References**:
  - `server/internal/agent/engine.go:140-230` - Code to extract (main streaming loop)
  - `server/internal/agent/engine.go:147-148` - Stream creation
  - `server/internal/agent/engine.go:155-224` - Delta processing loop
  - `server/internal/agent/engine.go:232-237` - Stream error handling
  - Test references: Tasks 2, 3

  **Acceptance Criteria**:
  - [ ] Method `handleStreaming` created
  - [ ] Returns `*StreamResult`
  - [ ] Events emitted in same order as before
  - [ ] All tests pass (including new unit tests)

  **QA Scenarios**:
  ```
  Scenario: Streaming behavior preserved
    Tool: Bash (go test)
    Preconditions: handleStreaming extracted
    Steps:
      1. Run: cd server && go test ./internal/agent/... -v
      2. Verify all tests pass including streaming tests
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-8-extract-streaming.txt

  Scenario: Event emission order unchanged
    Tool: Bash (go test)
    Preconditions: handleStreaming extracted
    Steps:
      1. Run streaming test that captures events
      2. Verify event order: EventMessage chunks → EventReasoning chunks → (EventToolCall if applicable)
    Expected Result: Same event order as before refactoring
    Evidence: .sisyphus/evidence/task-8-event-order.txt
  ```

  **Commit**: YES
  - Message: `refactor(agent): extract handleStreaming helper`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 9. Extract persistMessage Helper

  **What to do**:
  - Create `persistMessage(chatCtx, result, isFinal) error` helper
  - Unify lines 258-266 (with tool calls) and 277-284 (final message)
  - Handle both cases: with ToolCalls and without
  - If isFinal=true, set message ID; otherwise include ToolCalls

  **Must NOT do**:
  - Do NOT change message persistence timing
  - Do NOT change message structure

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Unifying duplicate code, straightforward
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Tasks 10, 11
  - **Blocked By**: Task 8

  **References**:
  - `server/internal/agent/engine.go:258-266` - Persistence with tool calls
  - `server/internal/agent/engine.go:277-284` - Persistence without tool calls (final)
  - `server/internal/agent/engine.go:359-372` - `convertToolCalls` helper (already exists)

  **Acceptance Criteria**:
  - [ ] Method `persistMessage` created
  - [ ] Duplicates removed from runAgentLoop
  - [ ] All tests pass

  **QA Scenarios**:
  ```
  Scenario: Message persistence unified
    Tool: Bash (go test)
    Preconditions: persistMessage extracted
    Steps:
      1. Run: cd server && go test ./internal/agent/... -v
      2. Verify message persistence tests pass
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-9-unified-persist.txt
  ```

  **Commit**: YES
  - Message: `refactor(agent): extract persistMessage helper`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 10. Extract handleToolCalls Helper

  **What to do**:
  - Create `handleToolCalls(chatCtx, toolMgr, result) (shouldContinue bool, error)`
  - Include: decision logic (tool calls present?), message persistence, tool execution, EventDone emission
  - Return true if should continue loop, false if should exit
  - Use persistMessage from Task 9

  **Must NOT do**:
  - Do NOT change tool execution order (sequential)
  - Do NOT change loop continuation logic

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Extracting decision and execution logic
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 11
  - **Blocked By**: Task 9

  **References**:
  - `server/internal/agent/engine.go:258-291` - Current decision and execution logic
  - `server/internal/agent/engine.go:268-272` - Tool execution loop
  - `server/internal/agent/engine.go:286-289` - EventDone emission
  - `server/internal/agent/engine.go:295-338` - `executeToolCall` helper (already exists)

  **Acceptance Criteria**:
  - [ ] Method `handleToolCalls` created
  - [ ] Returns bool indicating loop continuation
  - [ ] All tests pass

  **QA Scenarios**:
  ```
  Scenario: Tool call handling preserved
    Tool: Bash (go test)
    Preconditions: handleToolCalls extracted
    Steps:
      1. Run: cd server && go test ./internal/agent/... -v
      2. Verify tool execution tests pass
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-10-handle-toolcalls.txt
  ```

  **Commit**: YES
  - Message: `refactor(agent): extract handleToolCalls helper`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 11. Rewrite runAgentLoop Main Loop

  **What to do**:
  - Rewrite `runAgentLoop` to use all extracted helpers
  - Structure should be:
    ```go
    func (e *AgentEngine) runAgentLoop(chatCtx, userInput) error {
        // Phase 1: Initialization
        agentDef, err := e.prepareAgentLoop(chatCtx, userInput)
        if err != nil { return err }
        
        tools := agentDef.ToolManager.GetOpenAITools()  // Moved outside loop
        
        // Phase 2: Loop
        for {
            // ContextManager.BuildContext() now handles todo injection internally
            messages, err := e.contextMgr.BuildContext(chatCtx, "", 256000, agentDef.SystemPrompt)
            if err != nil { return err }
            
            openAIMessages := e.convertMessages(messages)
            
            result, err := e.handleStreaming(chatCtx, agentDef, openAIMessages, tools)
            if err != nil { return err }
            
            shouldContinue, err := e.handleToolCalls(chatCtx, agentDef.ToolManager, result)
            if err != nil { return err }
            
            if !shouldContinue { break }
        }
        return nil
    }
    ```
  - Remove all inline code that's now in helpers
  - Add clear comments for each phase

  **Must NOT do**:
  - Do NOT change any behavior
  - Do NOT change loop logic

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Assembling extracted methods into clean loop
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 12
  - **Blocked By**: Tasks 4, 5, 6, 8, 9, 10

  **References**:
  - All extracted methods from Tasks 4-10
  - Original `server/internal/agent/engine.go:71-293` - Original implementation

  **Acceptance Criteria**:
  - [ ] `runAgentLoop` rewritten with extracted helpers
  - [ ] Method body < 30 lines
  - [ ] Clear phase comments
  - [ ] All tests pass

  **QA Scenarios**:
  ```
  Scenario: Refactored loop works correctly
    Tool: Bash (go test)
    Preconditions: runAgentLoop rewritten
    Steps:
      1. Run: cd server && go test ./internal/agent/... -v
      2. Verify all tests pass
    Expected Result: PASS
    Evidence: .sisyphus/evidence/task-11-refactored-loop.txt

  Scenario: Method length reduced
    Tool: Bash (wc -l)
    Preconditions: runAgentLoop rewritten
    Steps:
      1. Count lines in new runAgentLoop method
      2. Verify < 30 lines
    Expected Result: Method body < 30 lines
    Evidence: .sisyphus/evidence/task-11-method-length.txt
  ```

  **Commit**: YES
  - Message: `refactor(agent): rewrite runAgentLoop with extracted helpers`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `go test ./internal/agent/... -v`

- [x] 12. Run All Tests and Verify

  **What to do**:
  - Run full test suite: `go test ./... -v`
  - Run linter: `go vet ./...`
  - Format code: `go fmt ./...`
  - Verify no regressions
  - Compare test results with pre-refactor baseline

  **Must NOT do**:
  - Do NOT skip any tests
  - Do NOT ignore failures

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Running tests and verification commands
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 13
  - **Blocked By**: Tasks 11, 2, 3

  **References**:
  - Test commands in AGENTS.md
  - All test files in `server/internal/agent/`

  **Acceptance Criteria**:
  - [ ] All tests pass
  - [ ] No `go vet` warnings
  - [ ] Code formatted with `go fmt`
  - [ ] No interface changes detected

  **QA Scenarios**:
  ```
  Scenario: All tests pass
    Tool: Bash (go test)
    Preconditions: Refactoring complete
    Steps:
      1. Run: cd server && go test ./... -v
      2. Verify all tests pass
    Expected Result: PASS (all tests)
    Evidence: .sisyphus/evidence/task-12-all-tests.txt

  Scenario: No vet warnings
    Tool: Bash (go vet)
    Preconditions: Refactoring complete
    Steps:
      1. Run: cd server && go vet ./...
      2. Verify no warnings
    Expected Result: No output (clean)
    Evidence: .sisyphus/evidence/task-12-vet.txt
  ```

  **Commit**: YES
  - Message: `test: verify all tests pass after refactoring`
  - Files: None (verification only)
  - Pre-commit: None

- [x] 13. Integration Test Verification

  **What to do**:
  - Run integration test with real OpenAI API (if available)
  - Compare event logs before/after refactoring
  - Verify message persistence produces same database records
  - Document any differences (should be none)

  **Must NOT do**:
  - Do NOT modify integration tests
  - Do NOT skip if OpenAI API unavailable (mark as skipped)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Running integration tests
  - **Skills**: []
  - **Skills Evaluated but Omitted**: None needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: None (final task)
  - **Blocked By**: Task 12

  **References**:
  - `server/internal/integration_test.go` (if exists)
  - OpenAI API key from environment

  **Acceptance Criteria**:
  - [ ] Integration test passes OR skipped (if API unavailable)
  - [ ] Event logs match baseline
  - [ ] Database records match baseline

  **QA Scenarios**:
  ```
  Scenario: Integration test passes
    Tool: Bash (go test)
    Preconditions: OPENAI_API_KEY set
    Steps:
      1. Run: cd server && go test ./... -v -run Integration
      2. Verify integration tests pass
    Expected Result: PASS OR SKIP (if no API key)
    Evidence: .sisyphus/evidence/task-13-integration.txt
  ```

  **Commit**: YES
  - Message: `test: integration test verification after refactoring`
  - Files: None (verification only)
  - Pre-commit: None

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

- [x] F1. **Plan Compliance Audit** — `oracle`
  Verify all "Must Have" items implemented, all "Must NOT Have" absent. Check evidence files.

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go test ./...`, `go vet ./...`, `go fmt ./...`. Review changed files for code smells.

- [x] F3. **Behavior Verification** — `unspecified-high`
  Compare integration test results before/after refactoring. Verify event logs identical.

- [x] F4. **Scope Fidelity Check** — `deep`
  Verify no scope creep: no interface changes, no new dependencies, no behavioral changes.

---

## Commit Strategy

Each task = one atomic commit. Revertible independently.

- **1**: `test: add MockOpenAIStream for testing runAgentLoop`
- **2-3**: `test: add unit tests for streaming and tool call logic`
- **4**: `refactor(agent): extract prepareAgentLoop helper`
- **5**: `refactor(agent): move tool list retrieval outside loop`
- **6**: `refactor: move todo package to tools/todo and update ContextManager`
- **7**: `refactor(agent): add StreamResult data structure`
- **8**: `refactor(agent): extract handleStreaming helper`
- **9**: `refactor(agent): extract persistMessage helper`
- **10**: `refactor(agent): extract handleToolCalls helper`
- **11**: `refactor(agent): rewrite runAgentLoop with extracted helpers`
- **12**: `test: verify all tests pass after refactoring`
- **13**: `test: integration test verification`
- **11**: `refactor(agent): rewrite runAgentLoop main loop`
- **12**: `test: verify all tests pass after refactoring`
- **13**: `test: integration test verification`

---

## Success Criteria

### Verification Commands

```bash
# All tests pass
cd server && go test ./internal/agent/... -v
# Expected: PASS

# No interface changes
git diff HEAD -- server/internal/domain/iface/
# Expected: No changes

# No event type changes
git diff HEAD -- server/internal/domain/entity/event.go
# Expected: No changes to EventType constants

# Method length verification
grep -n "^func (e \*AgentEngine)" server/internal/agent/engine.go
# Expected: No method body exceeds 50 lines

# Tool list retrieval verification
grep -n "GetOpenAITools" server/internal/agent/engine.go
# Expected: Single occurrence before loop
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass (existing + new)
- [ ] Integration test confirms behavior unchanged
- [ ] No interface changes
- [ ] Code formatted with `go fmt`
- [ ] No `go vet` warnings
