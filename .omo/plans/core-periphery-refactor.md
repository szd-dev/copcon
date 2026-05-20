# Core-Periphery Architecture Refactoring

## TL;DR

> **Quick Summary**: Refactor AgentEngine from a monolithic concrete struct into a core-periphery architecture — core engine handles LLM calls + tool dispatch + event emission + hook triggering; all extensions (memory, logging, tracing, todo injection) become ordered-priority hooks.
> 
> **Deliverables**:
> - `AgentEngine` interface + `engineImpl` (backward-compatible)
> - `hook` package: HookPoint enum, Hook interface, HookRunner
> - `llm` package: LLMProvider interface + OpenAI adapter
> - `plugins/`: MemoryPlugin, LoggingPlugin, TracingPlugin
> - `context_builder` split from persistence
> - slog replaces all 46 `log.Printf`/`log.Fatalf` calls
> 
> **Estimated Effort**: Large
> **Parallel Execution**: YES — 3 waves
> **Critical Path**: Task 1 → Task 8 → Task 9 → Task 13 → Task 14 → Task 15 → Task 16 → Task 17 → Task 20 → F1-F4

---

## Context

### Original Request
用户要求将 AgentEngine 从单体混凝土 struct 重构为核心-边缘分层架构。核心引擎专注于 LLM调用/工具调度/事件发射/钩子触发，所有扩展能力（memory、logging、tracing、knowledge base、context editing）通过 Hook 接入。

### Interview Summary
**Key Discussions**:
- [架构方案]: 用户确认了核心-边缘分层设计（AgentEngine interface + LLMProvider + Hook 系统 + 插件）
- [实施范围]: 全部 3 个 Phase（基础设施 + 钩子系统 + LLM抽象 + 插件）
- [测试策略]: TDD (RED → GREEN → REFACTOR)
- [Scope]: Go backend only，no frontend，no DB migrations，no Qdrant activation in Phase 1

**Research Findings**:
- `memoryMgr`: 0 次生产调用，nil client，纯僵尸代码 — 需从 5+ 调用点删除
- `log.Printf`: 实际 46 处（29 Printf + 17 Fatalf），而非初始估算的 21 处
- `concurrencySem`: 硬编码 `semaphore.NewWeighted(5)`，同时被 concurrent 和 async 模式共享
- `createTestEngine()`: 使用 struct literal 构造 AgentEngine — 提取 interface 前必须先加 test constructor
- `asyncRegistry`: 被 engine + session + 4 个 tools 共享 — 必须作为独立 service
- AgentEngine 无 interface，Handler 直接引用 `*agent.AgentEngine`

### Metis Review
**Identified Gaps** (addressed):
- [log.Printf 数量低估]: 46 处而非 21 处 — slog 迁移任务量翻倍
- [Test struct literal 会炸]: `createTestEngine()` 使用 struct literal — Task 1 必须先创建 `NewTestEngine()` helper
- [asyncRegistry 共享依赖]: 3 个组件共享 — Task 1 将其从 engine 字段提取为独立 service 注入
- [concurrencySem 双模式共享]: concurrent + async 共用同一个 semaphore — 保持默认值 5，可配置
- [Gin 日志未考虑]: `gin.Default()` 使用 `log.Printf` — 暂不迁移 Gin 日志，仅迁移 app 代码
- [Hook 错误语义未定义]: BeforeExecute 错误 → 跳过工具执行？AfterExecute 错误 → log and continue — Task 9 设计时明确

**Guardrails Applied**:
- MUST: 所有新 interface 遵循 `(chatCtx iface.ChatContextInterface, ...)` 参数模式
- MUST: Hook 执行顺序 = sort by priority desc, then registration timestamp
- MUST: asyncRegistry 作为独立 service 注入，不嵌入 engineImpl
- MUST: slog 使用 `slog.NewTextHandler(os.Stderr, nil)` 保持 stderr 输出
- MUST NOT: 修改 `gin.Default()` 日志
- MUST NOT: 修改 `messages` 表 schema
- MUST NOT: 添加超过 4 个 hook（Todo、Memory、Logging、Tracing）

---

## Work Objectives

### Core Objective
将 AgentEngine 从单体混凝土 struct 重构为核心-边缘分层架构，核心只做 LLM调用/工具调度/事件发射/钩子触发，所有扩展通过 Hook 接入，零回归。

### Concrete Deliverables
- `server/internal/agent/engine.go`: AgentEngine interface + engineImpl
- `server/internal/agent/engine_test_helper.go`: NewTestEngine() constructor
- `server/internal/hook/hook.go`: HookPoint, Hook, HookContext, HookRunner
- `server/internal/llm/provider.go`: LLMProvider interface
- `server/internal/llm/openai_adapter.go`: OpenAI adapter
- `server/internal/context_builder/`: split from chat_context
- `server/internal/plugins/memory/`: MemoryPlugin
- `server/internal/plugins/logging/`: LoggingPlugin
- `server/internal/plugins/tracing/`: TracingPlugin

### Definition of Done
- [ ] `go test ./internal/agent/... -v -count=1` — identical pass/fail as pre-refactor baseline
- [ ] `go test ./internal/... -v -count=1` — zero regressions across all packages
- [ ] `grep -r "log\.Printf" server/internal/ --include="*.go" | grep -v "_test.go"` — zero matches in production code
- [ ] `grep -r "memoryMgr" server/internal/agent/ --include="*.go"` — zero matches
- [ ] Hook system tests pass: no hooks, 1 hook, 3 hooks, erroring hook, panicking hook
- [ ] LLMProvider swap test: mock provider produces identical behavior as direct OpenAI client

### Must Have
- AgentEngine interface with Chat(chatCtx, userInput) error method
- engineImpl preserves all existing behavior (streaming, tool dispatch, step/part events)
- HookRunner with deterministic ordering (priority desc + registration timestamp)
- LLMProvider interface decoupling OpenAI from engine
- slog replacing all 46 log.Printf/Fatalf in production code
- memoryMgr completely removed from AgentEngine (5+ call sites)
- concurrencySem configurable via EngineOption, default=5
- Todo injection moved from BuildContext to hook, byte-identical output

### Must NOT Have (Guardrails)
- NO frontend changes
- NO DB migrations (no ALTER TABLE, no new columns)
- NO Qdrant connectivity in Phase 1
- NO changes to `gin.Default()` logging
- NO changes to `session.SessionManager`
- NO new tool registration patterns
- NO hooks beyond Todo, Memory, Logging, Tracing (4 total)
- NO `as any` or `@ts-ignore` (Go: no `interface{}` when typed alternative exists)
- NO breaking changes to ChatContextInterface

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES (Go tests with testify/assert, test DB via setupTestDB(t))
- **Automated tests**: TDD
- **Framework**: go test with testify
- **If TDD**: Each task follows RED (failing test) → GREEN (minimal impl) → REFACTOR

### QA Policy
Every task MUST include agent-executed QA scenarios (see TODO template below).
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.log`.

- **Go tests**: Use Bash (`go test ./internal/... -v -run TestXxx`) — assert PASS, check output
- **Log verification**: Use Bash (`grep -r "log\.Printf"`) — assert zero matches
- **Build verification**: Use Bash (`go build ./cmd/server`) — assert exit 0

---

## Execution Strategy

### Parallel Execution Waves

> Maximize throughput by grouping independent tasks into parallel waves.
> Each wave completes before the next begins.
> Target: 5-8 tasks per wave.

```
Wave 1 (Start Immediately — test infrastructure + interface extraction):
├── Task 1: NewTestEngine() constructor + remove memoryMgr [quick]
├── Task 2: Extract AgentEngine interface [quick]
├── Task 3: Make concurrencySem configurable [quick]
├── Task 4: Replace log.Printf → slog in agent/ [quick]
├── Task 5: Replace log.Printf → slog in chat_context/ [quick]
├── Task 6: Replace log.Printf → slog in api/ + domain/ [quick]
└── Task 7: Replace log.Printf → slog in cmd/server + cmd/init-db [quick]

Wave 2 (After Wave 1 — hook system + context refactor):
├── Task 8: Create hook package (HookPoint, Hook, HookContext, HookRunner) [deep]
├── Task 9: Define hook error semantics + implement HookRunner [deep]
├── Task 10: Split ContextBuilder from chat_context (pure building, no persistence) [deep]
├── Task 11: Move Todo injection to TodoInjectionHook [deep]
├── Task 12: Add BeforeToolExecute / AfterToolExecute hooks to engine_tools.go [deep]
└── Task 13: Wire hooks into engineImpl (HookRunner integration) [deep]

Wave 3 (After Wave 2 — LLMProvider + plugins):
├── Task 14: Create llm package (LLMProvider interface + StreamParams/StreamChunk) [deep]
├── Task 15: Implement OpenAI adapter for LLMProvider [deep]
├── Task 16: Refactor handleStreaming to use LLMProvider [deep]
├── Task 17: Implement MemoryPlugin [deep]
├── Task 18: Implement LoggingPlugin [deep]
├── Task 19: Implement TracingPlugin [deep]
└── Task 20: Wire plugins into main.go startup [quick]

Wave FINAL (After ALL tasks — 4 parallel reviews, then user okay):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)
-> Present results -> Get explicit user okay

Critical Path: Task 1 → Task 8 → Task 9 → Task 13 → Task 14 → Task 15 → Task 16 → Task 17 → Task 20 → F1-F4 → user okay
Parallel Speedup: ~60% faster than sequential
Max Concurrent: 7 (Wave 1)
```

### Dependency Matrix

- **1**: - - 2, 3, 8 (test constructor needed before all)
- **2**: 1 - 8 (interface needed before hook wiring)
- **3**: 1 - 8, 12 (configurable semaphore before tool hooks)
- **4-7**: - - - (slog migration independent of other Wave 1 tasks)
- **8**: 1 - 10, 11, 12, 13 (hook package needed by all hook consumers)
- **9**: 8 - 13 (error semantics before wiring)
- **10**: 8 - 11, 14 (ContextBuilder needed by TodoHook + LLMProvider integration)
- **11**: 8, 10 - - (TodoHook depends on hook package + ContextBuilder)
- **12**: 3, 8 - 13 (tool hooks depend on configurable semaphore + hook package)
- **13**: 8, 9, 10, 11, 12 - 14 (engine wiring depends on all hook components)
- **14**: 13 - 15, 16 (LLMProvider interface after engine wiring complete)
- **15**: 14 - 16 (OpenAI adapter after interface)
- **16**: 14, 15 - 17, 18, 19 (handleStreaming refactor after provider ready)
- **17**: 16 - - (MemoryPlugin independent of other plugins)
- **18**: 16 - - (LoggingPlugin independent of other plugins)
- **19**: 16 - - (TracingPlugin independent of other plugins)
- **20**: 17, 18, 19 - - (main.go wiring after all plugins)

### Agent Dispatch Summary

- **Wave 1**: **7** — T1-T3 → `quick`, T4-T7 → `quick`
- **Wave 2**: **6** — T8-T13 → `deep`
- **Wave 3**: **7** — T14-T16 → `deep`, T17-T19 → `deep`, T20 → `quick`
- **FINAL**: **4** — F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

- [x] 1. **Create NewTestEngine() constructor + delete memoryMgr zombie**

  **What to do**:
  - RED: Write test `TestNewTestEngine` — verify constructor creates engine with all required fields, verify engine.Chat() works with mock dependencies
  - GREEN: Add `NewTestEngine(opts ...TestEngineOption)` to new file `server/internal/agent/engine_test_helper.go`
  - GREEN: Replace all struct-literal constructions in `createTestEngine()` (engine_execution_test.go:154-163) and `TestAgentEngineStateless` (engine_test.go:407-432) with `NewTestEngine()`
  - GREEN: Remove `memoryMgr` field from AgentEngine struct (engine.go:56-63)
  - GREEN: Remove `memoryMgr` parameter from `NewAgentEngine` (engine.go:66-78) — 5 params → 4 params
  - GREEN: Remove `memoryMgr` argument from `main.go:79` call to `NewAgentEngine`
  - GREEN: Remove `memory.NewMemoryManager(nil, "agent_memory")` from `main.go:44`
  - GREEN: Remove `assert.NotNil(t, engine.memoryMgr)` from engine_test.go:431
  - GREEN: Remove `memoryMgr` from integration_test.go mock constructions
  - REFACTOR: Verify zero `memoryMgr` references remain: `grep -r "memoryMgr" server/internal/agent/ --include="*.go"`

  **Must NOT do**:
  - Do NOT delete `server/internal/memory/` package (may be used later by MemoryPlugin in Phase 3)
  - Do NOT change any method signatures other than `NewAgentEngine`
  - Do NOT change test behavior — only construction method

  **Recommended Agent Profile**:
  > Quick task — single-concern, mechanical refactoring, 1-3 files touched
  - **Category**: `quick`
    - Reason: Mechanical field removal + test helper creation, no logic changes
  - **Skills**: [`git-master`]
    - `git-master`: Atomic commit of field removal changes

  **Parallelization**:
  - **Can Run In Parallel**: NO (blocks all other tasks)
  - **Parallel Group**: Wave 1, Sequential (first task)
  - **Blocks**: Tasks 2, 3, 8 (test constructor needed before interface extraction and hook system)
  - **Blocked By**: None (can start immediately)

  **References**:
  - `server/internal/agent/engine.go:56-63` — AgentEngine struct fields to modify
  - `server/internal/agent/engine.go:66-78` — NewAgentEngine constructor to modify
  - `server/internal/agent/engine_execution_test.go:154-163` — createTestEngine struct literal to replace
  - `server/internal/agent/engine_test.go:407-432` — TestAgentEngineStateless struct literal to replace
  - `server/cmd/server/main.go:44,79` — memoryMgr creation and NewAgentEngine call to modify
  - `server/internal/agent/integration_test.go` — mock AgentEngine constructions to update
  - PATTERN: `server/internal/session/manager.go` — NewSessionManager pattern for constructor style
  - PATTERN: `server/internal/testutil/chat_context.go` — NewMockChatContext pattern for test helpers

  **Acceptance Criteria**:

  **If TDD (tests enabled):**
  - [ ] Test file: `server/internal/agent/engine_test_helper_test.go` created
  - [ ] `go test ./internal/agent/... -run TestNewTestEngine -v` → PASS
  - [ ] `go test ./internal/agent/... -v -count=1` → identical pass/fail as pre-refactor

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: NewTestEngine creates valid engine with all dependencies
    Tool: Bash (go test)
    Preconditions: NewTestEngine() function exists in engine_test_helper.go
    Steps:
      1. cd server && go test ./internal/agent/... -run TestNewTestEngine -v -count=1
      2. Assert output contains "PASS"
      3. Assert output contains "TestNewTestEngine"
    Expected Result: Test passes, engine created with all required fields
    Failure Indicators: "FAIL", "panic", "nil pointer dereference"
    Evidence: .sisyphus/evidence/task-1-new-test-engine.log

  Scenario: Zero memoryMgr references in agent package
    Tool: Bash (grep)
    Preconditions: memoryMgr field removed from AgentEngine struct
    Steps:
      1. grep -r "memoryMgr" server/internal/agent/ --include="*.go"
      2. Assert output is empty (zero matches)
    Expected Result: Zero matches
    Failure Indicators: Any output lines (residual references)
    Evidence: .sisyphus/evidence/task-1-no-memorymgr.log

  Scenario: Full agent test suite passes after field removal
    Tool: Bash (go test)
    Preconditions: Tasks 1 complete, all struct literals updated
    Steps:
      1. cd server && go test ./internal/agent/... -v -count=1 2>&1 | tee /tmp/agent-test-output.log
      2. Assert last line contains "ok" and not "FAIL"
      3. Compare with pre-refactor baseline: diff <(grep "PASS\|FAIL" /tmp/pre-refactor-baseline.log) <(grep "PASS\|FAIL" /tmp/agent-test-output.log)
    Expected Result: All tests pass, identical pass/fail count as baseline
    Failure Indicators: Any "FAIL" in output, different pass/fail counts from baseline
    Evidence: .sisyphus/evidence/task-1-agent-tests.log
  ```

  **Evidence to Capture**:
  - [ ] task-1-new-test-engine.log
  - [ ] task-1-no-memorymgr.log
  - [ ] task-1-agent-tests.log

  **Commit**: YES (standalone)
  - Message: `refactor(agent): add NewTestEngine constructor, remove memoryMgr zombie field`
  - Files: `server/internal/agent/engine.go`, `server/internal/agent/engine_test_helper.go`, `server/internal/agent/engine_test.go`, `server/internal/agent/engine_execution_test.go`, `server/internal/agent/integration_test.go`, `server/cmd/server/main.go`
  - Pre-commit: `cd server && go test ./internal/agent/... -count=1`

- [x] 2. **Extract AgentEngine interface**

  **What to do**:
  - RED: Write test `TestAgentEngineInterface` — verify engineImpl satisfies AgentEngine interface at compile time (`var _ AgentEngine = (*engineImpl)(nil)`)
  - RED: Write test `TestAgentEngineInterfaceMethod` — verify Chat(chatCtx, userInput) is the only exported method on the interface
  - GREEN: Define `AgentEngine` interface in `engine.go`: `type AgentEngine interface { Chat(chatCtx iface.ChatContextInterface, userInput string) error }`
  - GREEN: Rename current `AgentEngine` struct to `engineImpl` (unexported)
  - GREEN: Update `NewAgentEngine` to return `AgentEngine` interface: `func NewAgentEngine(...) AgentEngine { return &engineImpl{...} }`
  - GREEN: Update `Handler` struct in `api/handlers.go:25`: `agent AgentEngine` (was `*agent.AgentEngine`)
  - GREEN: Update `SetupRoutes` signature: `agentEngine agent.AgentEngine` (was `*agent.AgentEngine`)
  - GREEN: Verify compile-time: `go build ./...`
  - REFACTOR: Run `go test ./internal/... -v -count=1` — must pass identically

  **Must NOT do**:
  - Do NOT add methods to the interface beyond `Chat`
  - Do NOT export `engineImpl` — it must remain unexported
  - Do NOT change any method bodies — only type/interface declarations

  **Recommended Agent Profile**:
  > Quick task — interface extraction is a mechanical rename + type change
  - **Category**: `quick`
    - Reason: Mechanical type extraction, no logic changes
  - **Skills**: [`git-master`]
    - `git-master`: Atomic commit of interface extraction

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 1, after Task 1
  - **Blocks**: Task 8 (hook wiring needs interface)
  - **Blocked By**: Task 1 (needs NewTestEngine)

  **References**:
  - `server/internal/agent/engine.go:56-63` — current AgentEngine struct to rename
  - `server/internal/agent/engine.go:66-78` — NewAgentEngine to update return type
  - `server/internal/api/handlers.go:25` — Handler struct field to change type
  - `server/internal/api/handlers.go:361` — SetupRoutes signature to update
  - PATTERN: `server/internal/session/manager.go:19-28` — SessionManager interface pattern
  - PATTERN: `server/internal/domain/iface/chat.go:10-16` — ChatContextInterface pattern (exported interface, private impl)

  **Acceptance Criteria**:

  **If TDD (tests enabled):**
  - [ ] `var _ AgentEngine = (*engineImpl)(nil)` — compile-time check
  - [ ] `go test ./internal/agent/... -run TestAgentEngineInterface -v` → PASS
  - [ ] `go build ./...` → exit 0

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: AgentEngine interface compiles and engineImpl satisfies it
    Tool: Bash (go build)
    Preconditions: AgentEngine interface defined, engineImpl renamed
    Steps:
      1. cd server && go build ./...
      2. Assert exit code 0
    Expected Result: Build succeeds without errors
    Failure Indicators: Exit code non-zero, "does not implement" errors
    Evidence: .sisyphus/evidence/task-2-build.log

  Scenario: Handler uses AgentEngine interface correctly
    Tool: Bash (go vet)
    Preconditions: Handler struct updated to use AgentEngine interface
    Steps:
      1. cd server && go vet ./internal/api/...
      2. Assert exit code 0, no errors
    Expected Result: No vet errors
    Failure Indicators: "cannot use" type mismatch errors
    Evidence: .sisyphus/evidence/task-2-vet.log

  Scenario: Full test suite passes with interface extraction
    Tool: Bash (go test)
    Preconditions: All interface extractions complete
    Steps:
      1. cd server && go test ./internal/... -v -count=1 2>&1 | tail -20
      2. Assert output contains "ok" for each package
      3. Assert no "FAIL" in output
    Expected Result: All tests pass
    Failure Indicators: Any "FAIL" in output
    Evidence: .sisyphus/evidence/task-2-tests.log
  ```

  **Evidence to Capture**:
  - [ ] task-2-build.log
  - [ ] task-2-vet.log
  - [ ] task-2-tests.log

  **Commit**: YES (standalone)
  - Message: `refactor(agent): extract AgentEngine interface, rename impl to engineImpl`
  - Files: `server/internal/agent/engine.go`, `server/internal/api/handlers.go`
  - Pre-commit: `cd server && go build ./... && go test ./internal/agent/... -count=1`

- [x] 3. **Make concurrencySem configurable via EngineOption**

  **What to do**:
  - RED: Write test `TestConcurrencyConfigurable` — verify default concurrency is 5, verify WithConcurrency(3) sets limit to 3, verify WithConcurrency(0) panics or errors
  - GREEN: Define `EngineOption` type: `type EngineOption func(*engineImpl)`
  - GREEN: Define `WithConcurrency(n int) EngineOption` — validates n > 0, sets `engineImpl.concurrency`
  - GREEN: Change `NewAgentEngine` to variadic: `func NewAgentEngine(registry, sessionMgr, contextMgr, asyncRegistry, opts ...EngineOption) AgentEngine`
  - GREEN: Change `concurrencySem` from hardcoded `semaphore.NewWeighted(5)` to `semaphore.NewWeighted(int64(e.concurrency))` where `e.concurrency` defaults to 5
  - GREEN: Add `concurrency int` field to `engineImpl` struct
  - GREEN: Update `main.go:79` — no opts needed (uses default 5)
  - GREEN: Update `NewTestEngine` to accept opts
  - REFACTOR: Run `TestConcurrencyLimit` — must still pass with default 5

  **Must NOT do**:
  - Do NOT separate semaphores for concurrent vs async (keep shared)
  - Do NOT change default value from 5
  - Do NOT add config file support for this (constructor option only)

  **Recommended Agent Profile**:
  > Quick task — functional options pattern, 2-3 files
  - **Category**: `quick`
    - Reason: Functional options pattern is mechanical, limited scope
  - **Skills**: []
    - No special skills needed for functional options pattern

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 4-7)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 12 (tool hooks need configurable semaphore)
  - **Blocked By**: Task 1 (needs NewTestEngine)

  **References**:
  - `server/internal/agent/engine.go:77` — hardcoded `semaphore.NewWeighted(5)` to replace
  - `server/internal/agent/engine.go:56-63` — struct to add `concurrency int` field
  - `server/internal/agent/engine.go:66-78` — constructor to make variadic
  - `server/internal/agent/engine_execution_test.go:358-416` — TestConcurrencyLimit to verify
  - PATTERN: Go functional options pattern — `type Option func(*T)`

  **Acceptance Criteria**:

  **If TDD (tests enabled):**
  - [ ] `go test ./internal/agent/... -run TestConcurrencyConfigurable -v` → PASS
  - [ ] `go test ./internal/agent/... -run TestConcurrencyLimit -v` → PASS (default 5)

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: Default concurrency is 5
    Tool: Bash (go test)
    Preconditions: WithConcurrency option implemented
    Steps:
      1. cd server && go test ./internal/agent/... -run TestConcurrencyConfigurable -v -count=1
      2. Assert output contains "PASS: TestConcurrencyConfigurable"
      3. Assert TestConcurrencyLimit still passes
    Expected Result: Default = 5, test with 8 tools passes
    Failure Indicators: Test failure, concurrency limit not applied
    Evidence: .sisyphus/evidence/task-3-concurrency.log

  Scenario: WithConcurrency(3) limits to 3 concurrent executions
    Tool: Bash (go test)
    Preconditions: Test creates engine with WithConcurrency(3)
    Steps:
      1. cd server && go test ./internal/agent/... -run TestConcurrencyCustomLimit -v -count=1
      2. Assert output contains "PASS"
    Expected Result: Only 3 tools execute concurrently
    Failure Indicators: More than 3 concurrent executions observed
    Evidence: .sisyphus/evidence/task-3-custom-limit.log
  ```

  **Evidence to Capture**:
  - [ ] task-3-concurrency.log
  - [ ] task-3-custom-limit.log

  **Commit**: YES
  - Message: `feat(agent): make concurrency limit configurable via WithConcurrency option`
  - Files: `server/internal/agent/engine.go`, `server/internal/agent/engine_test_helper.go`
  - Pre-commit: `cd server && go test ./internal/agent/... -count=1`

- [x] 4. **Replace log.Printf → slog in agent/ package**

  **What to do**:
  - RED: Write test `TestSlogOutput` — verify engine writes to provided `*slog.Logger`, verify log format includes "msg", "session_id" keys
  - GREEN: Add `logger *slog.Logger` field to `engineImpl` struct
  - GREEN: Add `WithLogger(logger *slog.Logger) EngineOption`
  - GREEN: Replace all `log.Printf(...)` in `engine.go` (14 calls) with `e.logger.Info(...)` / `e.logger.Warn(...)` / `e.logger.Error(...)` with structured key=value pairs
  - GREEN: Replace all `log.Printf(...)` in `engine_tools.go` (4 calls) with structured slog calls
  - GREEN: Add `session_id` and `agent_id` keys to all log calls via `slog.String("session_id", chatCtx.SessionID())`
  - GREEN: Default logger in `NewAgentEngine`: `slog.New(slog.NewTextHandler(os.Stderr, nil))`
  - REFACTOR: `grep -r "log\.Printf" server/internal/agent/ --include="*.go" | grep -v "_test.go"` — must be zero

  **Must NOT do**:
  - Do NOT change log message semantics — same information, structured format
  - Do NOT add new log calls — only convert existing 18 calls
  - Do NOT change Gin logging

  **Recommended Agent Profile**:
  > Quick task — mechanical find-and-replace with structured logging
  - **Category**: `quick`
    - Reason: Mechanical log call replacement, 18 calls across 2 files
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 3, 5, 6, 7 — different packages)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 18 (LoggingPlugin builds on slog)
  - **Blocked By**: Task 1 (needs engineImpl struct to add logger field)

  **References**:
  - `server/internal/agent/engine.go` — 14 log.Printf calls to convert
  - `server/internal/agent/engine_tools.go` — 4 log.Printf calls to convert
  - PATTERN: Go slog package — `slog.Info("msg", "key", value)` structured logging

  **Acceptance Criteria**:

  **If TDD (tests enabled):**
  - [ ] `go test ./internal/agent/... -run TestSlogOutput -v` → PASS
  - [ ] `grep -r "log\.Printf" server/internal/agent/ --include="*.go" | grep -v "_test.go"` → zero matches

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: Zero log.Printf in agent production code
    Tool: Bash (grep)
    Preconditions: All log.Printf replaced with slog
    Steps:
      1. grep -r "log\.Printf" server/internal/agent/ --include="*.go" | grep -v "_test.go"
      2. Assert output is empty
    Expected Result: Zero matches (zero log.Printf in production code)
    Failure Indicators: Any output lines
    Evidence: .sisyphus/evidence/task-4-no-printf.log

  Scenario: slog output is valid structured text
    Tool: Bash (go test)
    Preconditions: TestSlogOutput test exists
    Steps:
      1. cd server && go test ./internal/agent/... -run TestSlogOutput -v -count=1
      2. Assert output contains "PASS"
    Expected Result: Log output contains structured key=value pairs
    Failure Indicators: Test failure, unstructured output
    Evidence: .sisyphus/evidence/task-4-slog-output.log
  ```

  **Evidence to Capture**:
  - [ ] task-4-no-printf.log
  - [ ] task-4-slog-output.log

  **Commit**: YES
  - Message: `refactor(agent): replace log.Printf with slog structured logging`
  - Files: `server/internal/agent/engine.go`, `server/internal/agent/engine_tools.go`
  - Pre-commit: `cd server && go test ./internal/agent/... -count=1`

- [x] 5. **Replace log.Printf → slog in chat_context/ package**

  **What to do**:
  - RED: Write test `TestContextManagerSlog` — verify context manager writes to provided logger
  - GREEN: Add `logger *slog.Logger` to `contextManager` struct
  - GREEN: Update `NewContextManager(db, todoMgr)` to `NewContextManager(db, todoMgr, logger *slog.Logger)`
  - GREEN: Replace `log.Printf("Warning: ...")` in `BuildContext` todo fetch error (manager.go:95) with `m.logger.Warn(...)`
  - GREEN: Replace any other `log.Printf` calls in chat_context package
  - GREEN: Update `main.go` to pass logger: `chat_context.NewContextManager(db, todoMgr, logger)`
  - REFACTOR: `grep -r "log\.Printf" server/internal/chat_context/ --include="*.go" | grep -v "_test.go"` → zero

  **Must NOT do**:
  - Do NOT change the ContextManager interface — only the constructor and implementation
  - Do NOT change log message semantics

  **Recommended Agent Profile**:
  > Quick task — 1-2 log calls in single package
  - **Category**: `quick`
    - Reason: Small scope, single package, few log calls
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 3, 4, 6, 7)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 10 (ContextBuilder split needs slog)
  - **Blocked By**: Task 1 (test infrastructure)

  **References**:
  - `server/internal/chat_context/manager.go:95` — log.Printf to convert
  - `server/cmd/server/main.go` — NewContextManager call to update

  **Acceptance Criteria**:
  - [ ] `go test ./internal/chat_context/... -run TestContextManagerSlog -v` → PASS
  - [ ] `grep -r "log\.Printf" server/internal/chat_context/ --include="*.go" | grep -v "_test.go"` → zero

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: Zero log.Printf in chat_context production code
    Tool: Bash (grep)
    Steps:
      1. grep -r "log\.Printf" server/internal/chat_context/ --include="*.go" | grep -v "_test.go"
      2. Assert output is empty
    Expected Result: Zero matches
    Evidence: .sisyphus/evidence/task-5-no-printf.log
  ```

  **Evidence to Capture**:
  - [ ] task-5-no-printf.log

  **Commit**: YES (grouped with Tasks 6, 7)
  - Message: `refactor: replace log.Printf with slog in chat_context, api, domain packages`

- [x] 6. **Replace log.Printf → slog in api/ and domain/ packages**

  **What to do**:
  - GREEN: Replace `log.Printf("Error in agent chat: %v", err)` in `handlers.go` Chat handler with `slog.Error(...)`
  - GREEN: Replace `log.Printf("WARNING: SSE event channel near capacity...")` in `domain/iface/chat.go:36` with `slog.Warn(...)`
  - GREEN: Replace `log.Printf("WARNING: SSE event channel near capacity...")` in `testutil/chat_context.go:37` with `slog.Warn(...)`
  - GREEN: Inject logger into Handler struct: `logger *slog.Logger`
  - REFACTOR: `grep -r "log\.Printf" server/internal/api/ server/internal/domain/ server/internal/testutil/ --include="*.go" | grep -v "_test.go"` → zero

  **Must NOT do**:
  - Do NOT change Gin logging
  - Do NOT change SSE event emission logic

  **Recommended Agent Profile**:
  > Quick task — 3 log calls across 3 files
  - **Category**: `quick`
    - Reason: Small scope, few files
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 3, 4, 5, 7)
  - **Parallel Group**: Wave 1
  - **Blocks**: None directly
  - **Blocked By**: Task 1

  **References**:
  - `server/internal/api/handlers.go` — Chat handler log.Printf
  - `server/internal/domain/iface/chat.go:36` — SSE channel capacity warning
  - `server/internal/testutil/chat_context.go:37` — testutil channel capacity warning

  **Acceptance Criteria**:
  - [ ] `grep -r "log\.Printf" server/internal/api/ server/internal/domain/ server/internal/testutil/ --include="*.go" | grep -v "_test.go"` → zero

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: Zero log.Printf in api/domain/testutil production code
    Tool: Bash (grep)
    Steps:
      1. grep -r "log\.Printf" server/internal/api/ server/internal/domain/ server/internal/testutil/ --include="*.go" | grep -v "_test.go"
      2. Assert output is empty
    Expected Result: Zero matches
    Evidence: .sisyphus/evidence/task-6-no-printf.log
  ```

  **Evidence to Capture**:
  - [ ] task-6-no-printf.log

  **Commit**: YES (grouped with Tasks 5, 7)

- [x] 7. **Replace log.Printf/Fatalf → slog in cmd/server and cmd/init-db**

  **What to do**:
  - GREEN: Replace 7 `log.Printf`/`log.Fatalf` in `cmd/server/main.go` with `slog.Info(...)` / `slog.Error(...)` + `os.Exit(1)`
  - GREEN: Replace 4 `log.Fatalf` in `cmd/init-db/main.go` with `slog.Error(...)` + `os.Exit(1)`
  - GREEN: Create root logger in main: `logger := slog.New(slog.NewTextHandler(os.Stderr, nil))`
  - GREEN: Pass logger to all constructors that now require it
  - REFACTOR: `grep -r "log\.Printf\|log\.Fatalf" server/cmd/ --include="*.go"` → zero

  **Must NOT do**:
  - Do NOT change startup behavior — errors still exit, just via `os.Exit(1)` instead of `log.Fatalf`
  - Do NOT change config loading logic

  **Recommended Agent Profile**:
  > Quick task — 11 log calls in 2 files
  - **Category**: `quick`
    - Reason: Mechanical replacement in startup code
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 3, 4, 5, 6)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 20 (main.go wiring)
  - **Blocked By**: Task 1

  **References**:
  - `server/cmd/server/main.go` — 7 log.Printf/Fatalf calls
  - `server/cmd/init-db/main.go` — 4 log.Fatalf calls

  **Acceptance Criteria**:
  - [ ] `grep -r "log\.Printf\|log\.Fatalf" server/cmd/ --include="*.go"` → zero
  - [ ] `cd server && go build ./cmd/server` → exit 0
  - [ ] `cd server && go build ./cmd/init-db` → exit 0

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: Zero log.Printf/Fatalf in cmd/ packages
    Tool: Bash (grep)
    Steps:
      1. grep -r "log\.Printf\|log\.Fatalf" server/cmd/ --include="*.go"
      2. Assert output is empty
    Expected Result: Zero matches
    Evidence: .sisyphus/evidence/task-7-no-printf.log

  Scenario: Both binaries build successfully
    Tool: Bash (go build)
    Steps:
      1. cd server && go build ./cmd/server && go build ./cmd/init-db
      2. Assert exit code 0
    Expected Result: Build succeeds
    Evidence: .sisyphus/evidence/task-7-build.log
  ```

  **Evidence to Capture**:
  - [ ] task-7-no-printf.log
  - [ ] task-7-build.log

  **Commit**: YES (grouped with Tasks 5, 6)
  - Message: `refactor: replace log.Printf/Fatalf with slog across all production code`
  - Files: `server/internal/chat_context/manager.go`, `server/internal/api/handlers.go`, `server/internal/domain/iface/chat.go`, `server/internal/testutil/chat_context.go`, `server/cmd/server/main.go`, `server/cmd/init-db/main.go`
  - Pre-commit: `cd server && go build ./... && go test ./internal/... -count=1`

- [x] 8. **Create hook package (HookPoint, Hook, HookContext)**

  **What to do**:
  - RED: Write test `TestHookInterface` — verify Hook interface compiles, verify HookContext carries all required fields
  - RED: Write test `TestHookPointEnum` — verify all HookPoint constants are unique strings
  - GREEN: Create `server/internal/hook/hook.go` with:
    - `HookPoint` type and all 9 constants (BeforeContextBuild, AfterContextBuild, OnSystemPrompt, OnMessagePersist, BeforeToolExecute, AfterToolExecute, OnToolError, BeforeLLMCall, AfterLLMCall, OnSessionResolve)
    - `HookContext` struct with: ChatCtx, SessionID, AgentID, SystemPrompt (*string), Messages (*[]MessageForLLM), ToolName, ToolArgs, ToolResult, Logger, CurrentPoint
    - `Hook` interface: `Name() string`, `Points() []HookPoint`, `Priority() int`, `Execute(ctx *HookContext) error`
  - REFACTOR: Verify package compiles: `go build ./internal/hook/...`

  **Must NOT do**:
  - Do NOT implement HookRunner in this task — that's Task 9
  - Do NOT add more than 10 HookPoint constants
  - Do NOT add hooks beyond the interface definition

  **Recommended Agent Profile**:
  > Deep task — new package design, interface definition
  - **Category**: `deep`
    - Reason: New package design with interface contracts, requires careful API design
  - **Skills**: []
    - No special skills — pure Go interface design

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 2, first task
  - **Blocks**: Tasks 9, 10, 11, 12, 13
  - **Blocked By**: Task 1 (test infrastructure)

  **References**:
  - PATTERN: `server/internal/domain/iface/chat.go` — ChatContextInterface pattern for interface definition
  - PATTERN: `server/internal/tool/manager.go:11-23` — Tool interface pattern

  **Acceptance Criteria**:
  - [ ] `go build ./internal/hook/...` → exit 0
  - [ ] `go test ./internal/hook/... -run TestHookInterface -v` → PASS
  - [ ] `go test ./internal/hook/... -run TestHookPointEnum -v` → PASS

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: Hook package compiles and tests pass
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/hook/... -v -count=1
      2. Assert output contains "PASS" for all tests
      3. Assert no compilation errors
    Expected Result: All hook tests pass
    Evidence: .sisyphus/evidence/task-8-hook-tests.log

  Scenario: HookPoint constants are all unique
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/hook/... -run TestHookPointEnum -v -count=1
      2. Assert output contains "PASS"
    Expected Result: No duplicate HookPoint values
    Evidence: .sisyphus/evidence/task-8-hookpoint-enum.log
  ```

  **Evidence to Capture**:
  - [ ] task-8-hook-tests.log
  - [ ] task-8-hookpoint-enum.log

  **Commit**: YES (standalone)
  - Message: `feat(hook): create hook package with HookPoint, Hook, HookContext interfaces`
  - Files: `server/internal/hook/hook.go`
  - Pre-commit: `cd server && go build ./internal/hook/... && go test ./internal/hook/... -count=1`

- [x] 9. **Define hook error semantics + implement HookRunner**

  **What to do**:
  - RED: Write test `TestHookRunnerNoHooks` — verify Run with empty hook list doesn't panic
  - RED: Write test `TestHookRunnerSingleHook` — verify single hook executes, receives correct HookContext
  - RED: Write test `TestHookRunnerMultipleHooks` — verify 3 hooks execute in priority order (highest first)
  - RED: Write test `TestHookRunnerErrorHook` — verify hook returning error doesn't crash runner, error is logged
  - RED: Write test `TestHookRunnerPanicHook` — verify panicking hook is recovered, doesn't crash runner
  - RED: Write test `TestHookRunnerContextCancelled` — verify hooks are skipped when context is cancelled
  - RED: Write test `TestHookRunnerDeterministicOrder` — verify same priority hooks execute in registration order
  - GREEN: Implement `HookRunner` interface: `Register(hook Hook)`, `Run(point HookPoint, ctx *HookContext)`
  - GREEN: Implement `hookRunner` struct with `[]Hook` sorted by priority desc + registration timestamp
  - GREEN: `Run()` checks `ctx.ChatCtx.Context().Err()` before executing any hook → cancelled → skip all
  - GREEN: `Run()` wraps each `hook.Execute()` in `defer recover()` → panic → log + continue
  - GREEN: `Run()` logs hook errors but does NOT abort the chain
  - GREEN: Implement `Register()` with thread-safe append (sync.Mutex or sorted insert)
  - REFACTOR: Verify all 6 tests pass

  **Must NOT do**:
  - Do NOT abort hook chain on single hook error — log and continue
  - Do NOT allow hooks to cancel tool execution (BeforeExecute error ≠ cancel)
  - Do NOT use non-deterministic ordering

  **Recommended Agent Profile**:
  > Deep task — concurrency-safe runner with error semantics, 6 test scenarios
  - **Category**: `deep`
    - Reason: Concurrency + error handling + recovery + ordering — multiple concerns
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 10, 12)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 13 (engine wiring needs runner)
  - **Blocked By**: Task 8 (hook interface)

  **References**:
  - `server/internal/hook/hook.go` — Hook interface from Task 8
  - PATTERN: `server/internal/tool/registry.go` — AsyncToolRegistry for concurrent-safe patterns
  - PATTERN: Go `net/http` middleware chain — ordered execution, error handling

  **Acceptance Criteria**:
  - [ ] All 6 TDD tests pass
  - [ ] `go test ./internal/hook/... -v -count=1 -race` → PASS (no data races)

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: HookRunner handles all edge cases
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/hook/... -v -count=1 -race
      2. Assert all 6+ tests PASS
      3. Assert no "DATA RACE" warnings
    Expected Result: All tests pass, race-free
    Evidence: .sisyphus/evidence/task-9-hookrunner.log

  Scenario: HookRunner skips hooks on cancelled context
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/hook/... -run TestHookRunnerContextCancelled -v -count=1
      2. Assert output contains "PASS"
    Expected Result: Hooks skipped when context cancelled
    Evidence: .sisyphus/evidence/task-9-cancelled.log

  Scenario: Panicking hook doesn't crash runner
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/hook/... -run TestHookRunnerPanicHook -v -count=1
      2. Assert output contains "PASS", no "panic" in test output
    Expected Result: Panic recovered, runner continues
    Evidence: .sisyphus/evidence/task-9-panic.log
  ```

  **Evidence to Capture**:
  - [ ] task-9-hookrunner.log
  - [ ] task-9-cancelled.log
  - [ ] task-9-panic.log

  **Commit**: YES (standalone)
  - Message: `feat(hook): implement HookRunner with deterministic ordering and error recovery`
  - Files: `server/internal/hook/runner.go`
  - Pre-commit: `cd server && go test ./internal/hook/... -count=1 -race`

- [x] 10. **Split ContextBuilder from chat_context (pure building, no persistence)**

  **What to do**:
  - RED: Write test `TestContextBuilderBuild` — verify BuildContext produces identical `[]MessageForLLM` as current chat_context.BuildContext for same inputs
  - RED: Write test `TestContextBuilderNoPersistence` — verify ContextBuilder has NO `*gorm.DB` field, NO persistence methods
  - GREEN: Create `server/internal/context_builder/builder.go` with `ContextBuilder` interface:
    - `Build(ctx, messages []entity.UIMessage, systemPrompt string, userInput string) ([]MessageForLLM, error)`
  - GREEN: Implement `contextBuilder` struct — pure function, no DB, no side effects
  - GREEN: Move message conversion logic (UIMessage → MessageForLLM) from `chat_context/manager.go` to `context_builder/builder.go`
  - GREEN: Move `convertDBMessagesToUI`, `synthesizeUIMessage`, `convertModelToolCalls`, `groupPartsByStep` from chat_context to context_builder (as helpers)
  - GREEN: `chat_context.BuildContext` becomes thin wrapper: fetch history → call `contextBuilder.Build()` → return
  - REFACTOR: Verify `go test ./internal/chat_context/... -v -count=1` passes identically

  **Must NOT do**:
  - Do NOT change the ContextManager interface
  - Do NOT change BuildContext output format
  - Do NOT move persistence methods (AddMessage, GetHistory) out of chat_context

  **Recommended Agent Profile**:
  > Deep task — package split with behavior preservation
  - **Category**: `deep`
    - Reason: Package extraction with zero-regression requirement
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 9, 12)
  - **Parallel Group**: Wave 2
  - **Blocks**: Tasks 11, 14
  - **Blocked By**: Task 8 (hook interface), Task 5 (slog in chat_context)

  **References**:
  - `server/internal/chat_context/manager.go:84-149` — BuildContext to extract
  - `server/internal/chat_context/manager.go:149-340` — helper functions to move
  - `server/internal/domain/entity/convert.go` — UIMessage → ModelMessage conversion (already extracted)

  **Acceptance Criteria**:
  - [ ] `go test ./internal/context_builder/... -v -count=1` → PASS
  - [ ] `go test ./internal/chat_context/... -v -count=1` → PASS (identical to pre-split)
  - [ ] ContextBuilder struct has NO `*gorm.DB` field

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: ContextBuilder produces identical output as BuildContext
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/context_builder/... -v -count=1
      2. Assert all tests PASS
      3. cd server && go test ./internal/chat_context/... -v -count=1
      4. Assert all tests PASS (identical pass/fail as pre-split)
    Expected Result: Behavior preserved, tests pass
    Evidence: .sisyphus/evidence/task-10-builder-tests.log

  Scenario: ContextBuilder has no DB dependency
    Tool: Bash (grep)
    Steps:
      1. grep -r "gorm\|sql\.DB\|database" server/internal/context_builder/ --include="*.go"
      2. Assert output is empty
    Expected Result: No database imports in context_builder
    Evidence: .sisyphus/evidence/task-10-no-db.log
  ```

  **Evidence to Capture**:
  - [ ] task-10-builder-tests.log
  - [ ] task-10-no-db.log

  **Commit**: YES (standalone)
  - Message: `refactor: split ContextBuilder (pure) from chat_context (persistence)`
  - Files: `server/internal/context_builder/builder.go`, `server/internal/chat_context/manager.go`
  - Pre-commit: `cd server && go test ./internal/... -count=1`

- [x] 11. **Move Todo injection to TodoInjectionHook**

  **What to do**:
  - RED: Write test `TestTodoInjectionHook` — verify hook produces byte-identical system prompt append as current `BuildContext` todo injection (lines 91-99 of manager.go)
  - RED: Write test `TestTodoInjectionHookNoTodos` — verify hook is no-op when todo list is empty
  - RED: Write test `TestTodoInjectionHookError` — verify hook logs warning and continues when todoMgr.List fails
  - GREEN: Create `server/internal/plugins/todo_hook.go` with `TodoInjectionHook` struct implementing `hook.Hook`
  - GREEN: `TodoInjectionHook.Points()` returns `[hook.HookOnSystemPrompt]`
  - GREEN: `TodoInjectionHook.Priority()` returns 50 (runs before memory injection at 100)
  - GREEN: `TodoInjectionHook.Execute()` calls `todoMgr.List()`, formats via `formatTodoState`, appends to `ctx.SystemPrompt`
  - GREEN: Move `formatTodoState` from `chat_context/manager.go:334-374` to shared location (e.g., `chat_context/format.go` or `plugins/todo_format.go`)
  - GREEN: Remove todo injection code from `chat_context/manager.go` BuildContext (lines 91-99)
  - GREEN: Remove `todoMgr` field and constructor param from `contextManager` (no longer needed)
  - GREEN: Register `TodoInjectionHook` in `main.go` via `hookRunner.Register(NewTodoInjectionHook(todoMgr))`
  - REFACTOR: Verify `go test ./internal/... -v -count=1` passes identically

  **Must NOT do**:
  - Do NOT change `formatTodoState` output format — byte-identical
  - Do NOT change the system prompt append position (after system prompt, before user message)
  - Do NOT change error handling behavior (log warn, continue without todos)

  **Recommended Agent Profile**:
  > Deep task — behavior extraction with byte-identical output requirement
  - **Category**: `deep`
    - Reason: Behavior preservation is critical, requires careful comparison testing
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 12, 13)
  - **Parallel Group**: Wave 2
  - **Blocks**: None directly
  - **Blocked By**: Tasks 8, 10 (hook interface + ContextBuilder)

  **References**:
  - `server/internal/chat_context/manager.go:91-99` — todo injection to extract
  - `server/internal/chat_context/manager.go:334-374` — formatTodoState to move
  - PATTERN: `server/internal/plugins/todo_hook.go` — new plugin pattern

  **Acceptance Criteria**:
  - [ ] `go test ./internal/plugins/... -run TestTodoInjectionHook -v` → PASS (all 3 scenarios)
  - [ ] `go test ./internal/chat_context/... -v -count=1` → PASS (no todo injection code remains)
  - [ ] Byte-identical system prompt output for same todo list input

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: TodoInjectionHook produces identical output as old BuildContext
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/plugins/... -run TestTodoInjectionHook -v -count=1
      2. Assert output contains "PASS" for byte-identical comparison
    Expected Result: Identical system prompt append
    Evidence: .sisyphus/evidence/task-11-todo-hook.log

  Scenario: Empty todo list is no-op
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/plugins/... -run TestTodoInjectionHookNoTodos -v -count=1
      2. Assert system prompt unchanged
    Expected Result: No modification when todos empty
    Evidence: .sisyphus/evidence/task-11-no-todos.log
  ```

  **Evidence to Capture**:
  - [ ] task-11-todo-hook.log
  - [ ] task-11-no-todos.log

  **Commit**: YES (standalone)
  - Message: `refactor: extract Todo injection from BuildContext to TodoInjectionHook`
  - Files: `server/internal/plugins/todo_hook.go`, `server/internal/chat_context/manager.go`, `server/cmd/server/main.go`
  - Pre-commit: `cd server && go test ./internal/... -count=1`

- [x] 12. **Add BeforeToolExecute / AfterToolExecute hooks to engine_tools.go**

  **What to do**:
  - RED: Write test `TestToolHooksBeforeExecute` — verify BeforeToolExecute hook receives ToolName + ToolArgs, verify hook can modify ToolArgs
  - RED: Write test `TestToolHooksAfterExecute` — verify AfterToolExecute hook receives ToolResult
  - RED: Write test `TestToolHooksOnError` — verify OnToolError hook fires on tool execution failure
  - RED: Write test `TestToolHooksContextCancelled` — verify hooks are skipped when context cancelled
  - GREEN: Add `hookRunner hook.HookRunner` field to `engineImpl`
  - GREEN: Add `WithHookRunner(runner hook.HookRunner) EngineOption`
  - GREEN: In `executeSync` (engine_tools.go:73): call `hookRunner.Run(BeforeToolExecute, ctx)` before `toolMgr.Execute()`
  - GREEN: In `executeSync`: call `hookRunner.Run(AfterToolExecute, ctx)` after successful execute; `hookRunner.Run(OnToolError, ctx)` on error
  - GREEN: Same pattern in `executeConcurrent` (engine_tools.go:341) and `executeAsync` (engine_tools.go:137)
  - REFACTOR: Verify all tool execution tests pass

  **Must NOT do**:
  - Do NOT allow BeforeToolExecute to cancel execution (hook error ≠ cancel)
  - Do NOT change tool execution logic — only add hook calls around existing code
  - Do NOT add hooks to async tool goroutine completion (already has event emission)

  **Recommended Agent Profile**:
  > Deep task — 3 execution modes × hook injection, 4 test scenarios
  - **Category**: `deep`
    - Reason: Must inject hooks into 3 different execution paths correctly
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 9, 10, 11)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 13 (engine wiring)
  - **Blocked By**: Tasks 3, 8 (configurable semaphore + hook interface)

  **References**:
  - `server/internal/agent/engine_tools.go:73-140` — executeSync to modify
  - `server/internal/agent/engine_tools.go:341-468` — executeConcurrent to modify
  - `server/internal/agent/engine_tools.go:137-238` — executeAsync to modify

  **Acceptance Criteria**:
  - [ ] `go test ./internal/agent/... -run TestToolHooks -v` → PASS (all 4 scenarios)
  - [ ] `go test ./internal/agent/... -v -count=1` → PASS (no regressions)

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: Tool hooks fire in correct order
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/agent/... -run "TestToolHooks" -v -count=1
      2. Assert all 4 test scenarios PASS
      3. Assert BeforeToolExecute fires before Execute, AfterToolExecute fires after
    Expected Result: Hooks fire in correct lifecycle order
    Evidence: .sisyphus/evidence/task-12-tool-hooks.log

  Scenario: Tool hooks skipped on cancelled context
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/agent/... -run TestToolHooksContextCancelled -v -count=1
      2. Assert hooks are NOT executed when context cancelled
    Expected Result: Hooks skipped on cancelled context
    Evidence: .sisyphus/evidence/task-12-cancelled.log
  ```

  **Evidence to Capture**:
  - [ ] task-12-tool-hooks.log
  - [ ] task-12-cancelled.log

  **Commit**: YES
  - Message: `feat(agent): add BeforeToolExecute/AfterToolExecute/OnToolError hooks`
  - Files: `server/internal/agent/engine.go`, `server/internal/agent/engine_tools.go`
  - Pre-commit: `cd server && go test ./internal/agent/... -count=1`

- [x] 13. **Wire hooks into engineImpl (HookRunner integration + context hooks)**

  **What to do**:
  - RED: Write test `TestContextHooksBeforeBuild` — verify BeforeContextBuild hook fires in runAgentLoop before BuildContext
  - RED: Write test `TestContextHooksAfterBuild` — verify AfterContextBuild hook can modify messages array
  - RED: Write test `TestContextHooksOnSessionResolve` — verify OnSessionResolve hook fires in prepareAgentLoop
  - RED: Write test `TestMessagePersistHook` — verify OnMessagePersist hook fires after persistMessage
  - GREEN: In `runAgentLoop` (engine.go): call `hookRunner.Run(BeforeContextBuild, ctx)` before `contextMgr.BuildContext()`
  - GREEN: In `runAgentLoop`: call `hookRunner.Run(AfterContextBuild, ctx)` after BuildContext, passing mutable `*ctx.Messages`
  - GREEN: In `prepareAgentLoop` (engine.go): call `hookRunner.Run(OnSessionResolve, ctx)` after session resolution
  - GREEN: In `persistMessage` (engine.go): call `hookRunner.Run(OnMessagePersist, ctx)` after AddMessage
  - GREEN: In `runAgentLoop`: call `hookRunner.Run(BeforeLLMCall, ctx)` before handleStreaming; `hookRunner.Run(AfterLLMCall, ctx)` after
  - GREEN: HookRunner must be injected into NewAgentEngine (already added in Task 12)
  - REFACTOR: Verify full test suite passes with hooks wired

  **Must NOT do**:
  - Do NOT change the agent loop logic — only add hook calls at defined points
  - Do NOT skip the loop if a hook errors — log and continue

  **Recommended Agent Profile**:
  > Deep task — 4 hook points in engine loop, 4 test scenarios
  - **Category**: `deep`
    - Reason: Hook injection at multiple lifecycle points, must not break loop flow
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 11)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 14 (LLMProvider needs wired hooks)
  - **Blocked By**: Tasks 8, 9, 12 (hook interface + runner + tool hooks)

  **References**:
  - `server/internal/agent/engine.go:96-135` — prepareAgentLoop to add OnSessionResolve
  - `server/internal/agent/engine.go:364-420` — runAgentLoop to add context/LLM hooks
  - `server/internal/agent/engine.go:463-492` — persistMessage to add OnMessagePersist

  **Acceptance Criteria**:
  - [ ] `go test ./internal/agent/... -run TestContextHooks -v` → PASS (all 4 scenarios)
  - [ ] `go test ./internal/agent/... -v -count=1` → PASS (no regressions)

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: All 4 context hook points fire during agent loop
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/agent/... -run "TestContextHooks" -v -count=1
      2. Assert all 4 test scenarios PASS
      3. Assert hooks fire at correct lifecycle points
    Expected Result: Hooks fire in correct order during loop
    Evidence: .sisyphus/evidence/task-13-context-hooks.log

  Scenario: Full agent test suite passes with hooks wired
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/agent/... -v -count=1 2>&1 | tail -5
      2. Assert output contains "ok" and no "FAIL"
    Expected Result: No regressions
    Evidence: .sisyphus/evidence/task-13-agent-tests.log
  ```

  **Evidence to Capture**:
  - [ ] task-13-context-hooks.log
  - [ ] task-13-agent-tests.log

  **Commit**: YES
  - Message: `feat(agent): wire HookRunner into agent loop for context/LLM/session hooks`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `cd server && go test ./internal/agent/... -count=1`

- [x] 14. **Create llm package (LLMProvider interface + StreamParams/StreamChunk)**

  **What to do**:
  - RED: Write test `TestLLMProviderInterface` — verify interface compiles, verify StreamParams carries all required fields
  - GREEN: Create `server/internal/llm/provider.go` with:
    - `StreamChunk` struct: Content, ReasoningContent, ToolCalls, Usage, FinishReason
    - `StreamParams` struct: Model, Messages, Tools, Temperature, MaxTokens
    - `LLMProvider` interface: `Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error)`
    - `Message` struct: Role, Content, ToolCalls, ToolCallID, Name
    - `ToolDef` struct: Name, Description, Parameters (json.RawMessage)
  - REFACTOR: Verify package compiles

  **Must NOT do**:
  - Do NOT implement the OpenAI adapter in this task — that's Task 15
  - Do NOT change any existing code to use LLMProvider yet

  **Recommended Agent Profile**:
  > Deep task — new package with API contract design
  - **Category**: `deep`
    - Reason: API design for LLM provider abstraction
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 3, first task
  - **Blocks**: Tasks 15, 16
  - **Blocked By**: Task 13 (engine wiring complete)

  **References**:
  - `server/internal/agent/engine.go:28-45` — current StreamResult for chunk shape reference
  - `server/internal/agent/engine.go:141-288` — handleStreaming for stream behavior reference
  - PATTERN: `server/internal/tool/manager.go` — Tool interface for abstraction pattern

  **Acceptance Criteria**:
  - [ ] `go build ./internal/llm/...` → exit 0
  - [ ] `go test ./internal/llm/... -run TestLLMProviderInterface -v` → PASS

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: LLMProvider interface compiles and tests pass
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/llm/... -v -count=1
      2. Assert output contains "PASS"
    Expected Result: Interface compiles, tests pass
    Evidence: .sisyphus/evidence/task-14-llm-interface.log
  ```

  **Evidence to Capture**:
  - [ ] task-14-llm-interface.log

  **Commit**: YES (standalone)
  - Message: `feat(llm): define LLMProvider interface and stream types`
  - Files: `server/internal/llm/provider.go`
  - Pre-commit: `cd server && go build ./internal/llm/...`

- [x] 15. **Implement OpenAI adapter for LLMProvider**

  **What to do**:
  - RED: Write test `TestOpenAIAdapterStream` — verify adapter calls OpenAI API with correct params, verify chunks flow through channel
  - RED: Write test `TestOpenAIAdapterError` — verify adapter propagates API errors through error channel
  - RED: Write test `TestOpenAIAdapterToolCalls` — verify tool call chunks are correctly parsed from OpenAI response
  - GREEN: Create `server/internal/llm/openai_adapter.go` with `OpenAIAdapter` struct implementing `LLMProvider`
  - GREEN: `OpenAIAdapter` wraps `*openai.Client`
  - GREEN: `Stream()` converts `StreamParams` to `openai.ChatCompletionNewParams`, uses `client.Chat.Completions.NewStreaming()`
  - GREEN: `Stream()` converts OpenAI chunks to `StreamChunk` in a goroutine, sends to channel
  - GREEN: `Stream()` handles errors via error channel, closes both channels on completion
  - REFACTOR: Verify adapter tests pass

  **Must NOT do**:
  - Do NOT change any existing code to use the adapter yet — only create it
  - Do NOT handle anything beyond basic streaming + tool calls in this task

  **Recommended Agent Profile**:
  > Deep task — adapter implementation wrapping OpenAI SDK
  - **Category**: `deep`
    - Reason: Adapter pattern with goroutine channel management
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 3, after Task 14
  - **Blocks**: Task 16 (handleStreaming refactor)
  - **Blocked By**: Task 14 (LLMProvider interface)

  **References**:
  - `server/internal/llm/provider.go` — LLMProvider interface from Task 14
  - `server/internal/agent/engine.go:141-288` — current handleStreaming for OpenAI client usage patterns
  - PATTERN: `server/internal/agent/registry.go` — AgentDefinition.OpenAIClient usage

  **Acceptance Criteria**:
  - [ ] `go test ./internal/llm/... -run TestOpenAIAdapter -v` → PASS (all 3 scenarios)
  - [ ] `go test ./internal/llm/... -v -count=1 -race` → PASS (no goroutine leaks)

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: OpenAI adapter streams chunks correctly
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/llm/... -run TestOpenAIAdapter -v -count=1
      2. Assert all 3 test scenarios PASS
      3. Assert chunks flow through channel with correct types
    Expected Result: Adapter correctly wraps OpenAI streaming
    Evidence: .sisyphus/evidence/task-15-adapter.log
  ```

  **Evidence to Capture**:
  - [ ] task-15-adapter.log

  **Commit**: YES (standalone)
  - Message: `feat(llm): implement OpenAI adapter for LLMProvider interface`
  - Files: `server/internal/llm/openai_adapter.go`
  - Pre-commit: `cd server && go test ./internal/llm/... -count=1 -race`

- [x] 16. **Refactor handleStreaming to use LLMProvider**

  **What to do**:
  - RED: Write test `TestHandleStreamingWithProvider` — verify engine streams via LLMProvider with same behavior as direct OpenAI client
  - RED: Write test `TestHandleStreamingProviderError` — verify engine handles provider errors correctly
  - GREEN: Add `llmProvider llm.LLMProvider` field to `engineImpl`
  - GREEN: Add `WithLLMProvider(p llm.LLMProvider) EngineOption`
  - GREEN: Add `openaiAdapter *llm.OpenAIAdapter` to `AgentDefinition` (replaces `OpenAIClient openai.Client`)
  - GREEN: Refactor `handleStreaming` to call `e.llmProvider.Stream(ctx, params)` instead of direct OpenAI API calls
  - GREEN: Convert `agentDef.OpenAIClient` usage to `agentDef.LLMProvider` in agent registry and loop
  - GREEN: Map `StreamChunk` fields back to current `StreamResult` (Content, ReasoningContent, ToolCalls, Usage)
  - GREEN: Existing part_create/part_update event emission must continue unchanged
  - REFACTOR: Verify all existing streaming tests pass with provider

  **Must NOT do**:
  - Do NOT change SSE event emission — part_create/part_update must work identically
  - Do NOT change tool call accumulation logic
  - Do NOT remove OpenAI adapter — it remains as the default provider

  **Recommended Agent Profile**:
  > Deep task — core streaming refactor with behavior preservation
  - **Category**: `deep`
    - Reason: Critical path refactor, must preserve exact streaming behavior
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 3, after Tasks 14, 15
  - **Blocks**: Tasks 17, 18, 19 (plugins need provider-using engine)
  - **Blocked By**: Tasks 14, 15 (LLMProvider + adapter)

  **References**:
  - `server/internal/agent/engine.go:141-288` — handleStreaming to refactor
  - `server/internal/agent/registry.go` — AgentDefinition to update
  - `server/internal/llm/provider.go` — LLMProvider interface
  - `server/internal/llm/openai_adapter.go` — OpenAIAdapter

  **Acceptance Criteria**:
  - [ ] `go test ./internal/agent/... -run TestHandleStreamingWithProvider -v` → PASS
  - [ ] `go test ./internal/agent/... -v -count=1` → PASS (identical pass/fail as pre-refactor)
  - [ ] SSE events (part_create, part_update) unchanged

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: Streaming via LLMProvider produces identical events
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/agent/... -run TestHandleStreamingWithProvider -v -count=1
      2. Assert output contains "PASS"
      3. Assert part_create/part_update events match expected sequence
    Expected Result: Identical streaming behavior via provider
    Evidence: .sisyphus/evidence/task-16-provider-stream.log

  Scenario: Full agent test suite passes with LLMProvider
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/agent/... -v -count=1 2>&1 | tail -10
      2. Assert no "FAIL" in output
    Expected Result: All tests pass
    Evidence: .sisyphus/evidence/task-16-agent-tests.log
  ```

  **Evidence to Capture**:
  - [ ] task-16-provider-stream.log
  - [ ] task-16-agent-tests.log

  **Commit**: YES (standalone)
  - Message: `refactor(agent): use LLMProvider interface instead of direct OpenAI client`
  - Files: `server/internal/agent/engine.go`, `server/internal/agent/registry.go`, `server/cmd/server/main.go`
  - Pre-commit: `cd server && go test ./internal/agent/... -count=1`

- [x] 17. **Implement MemoryPlugin (AfterContextBuild + OnMessagePersist)**

  **What to do**:
  - RED: Write test `TestMemoryPluginSearch` — verify hook calls `memoryMgr.Search()` on AfterContextBuild, injects results as system message
  - RED: Write test `TestMemoryPluginStore` — verify hook calls `memoryMgr.Store()` on OnMessagePersist for assistant messages
  - RED: Write test `TestMemoryPluginNoMemoryMgr` — verify hook is no-op when memoryMgr is nil (graceful degradation)
  - GREEN: Create `server/internal/plugins/memory/memory_plugin.go` with `MemoryPlugin` struct implementing `hook.Hook`
  - GREEN: `MemoryPlugin.Points()` returns `[HookAfterContextBuild, HookOnMessagePersist]`
  - GREEN: `MemoryPlugin.Priority()` returns 100 (runs after TodoInjectionHook at 50)
  - GREEN: `AfterContextBuild`: call `memoryMgr.Search()`, format results as system message, prepend to `*ctx.Messages`
  - GREEN: `OnMessagePersist`: call `memoryMgr.Store()` for assistant messages with substantial content
  - GREEN: All memory operations wrapped in error handling — log warn, never abort
  - REFACTOR: Verify plugin tests pass

  **Must NOT do**:
  - Do NOT activate Qdrant — use MemoryManager interface with mock in tests
  - Do NOT block the agent loop on memory operations (async store)
  - Do NOT store every message — only assistant messages with content

  **Recommended Agent Profile**:
  > Deep task — plugin implementation with graceful degradation
  - **Category**: `deep`
    - Reason: Plugin with async operations, graceful nil handling, priority ordering
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 18, 19)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 20 (main.go wiring)
  - **Blocked By**: Task 16 (engine with LLMProvider + hooks)

  **References**:
  - `server/internal/memory/manager.go` — MemoryManager interface
  - `server/internal/hook/hook.go` — Hook interface
  - PATTERN: `server/internal/plugins/todo_hook.go` — plugin pattern from Task 11

  **Acceptance Criteria**:
  - [ ] `go test ./internal/plugins/memory/... -run TestMemoryPlugin -v` → PASS (all 3 scenarios)
  - [ ] Plugin is no-op when memoryMgr is nil (no panic)

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: MemoryPlugin injects search results into context
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/plugins/memory/... -run TestMemoryPluginSearch -v -count=1
      2. Assert output contains "PASS"
      3. Assert messages array has memory context prepended
    Expected Result: Memory results injected as system message
    Evidence: .sisyphus/evidence/task-17-search.log

  Scenario: MemoryPlugin gracefully handles nil memoryMgr
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/plugins/memory/... -run TestMemoryPluginNoMemoryMgr -v -count=1
      2. Assert no panic, no error
    Expected Result: No-op, no crash
    Evidence: .sisyphus/evidence/task-17-nil-mgr.log
  ```

  **Evidence to Capture**:
  - [ ] task-17-search.log
  - [ ] task-17-nil-mgr.log

  **Commit**: YES (standalone)
  - Message: `feat(plugins): implement MemoryPlugin for context injection and message storage`
  - Files: `server/internal/plugins/memory/memory_plugin.go`
  - Pre-commit: `cd server && go test ./internal/plugins/memory/... -count=1`

- [x] 18. **Implement LoggingPlugin (BeforeLLMCall + AfterLLMCall + BeforeToolExecute + AfterToolExecute)**

  **What to do**:
  - RED: Write test `TestLoggingPluginLLM` — verify hook logs structured info on BeforeLLMCall (message count) and AfterLLMCall (tokens, duration)
  - RED: Write test `TestLoggingPluginTool` — verify hook logs tool name + args on BeforeToolExecute, result on AfterToolExecute
  - RED: Write test `TestLoggingPluginFormat` — verify log output is valid JSON (if using JSON handler) or structured text
  - GREEN: Create `server/internal/plugins/logging/logging_plugin.go` with `LoggingPlugin` struct implementing `hook.Hook`
  - GREEN: `LoggingPlugin.Points()` returns `[HookBeforeLLMCall, HookAfterLLMCall, HookBeforeToolExecute, HookAfterToolExecute]`
  - GREEN: `LoggingPlugin.Priority()` returns 200 (runs after functional hooks)
  - GREEN: Each hook point logs structured key=value pairs: session_id, agent_id, message_count, tool_name, duration_ms, token_count
  - GREEN: Use `ctx.Logger` (already injected via slog migration in Tasks 4-7)
  - REFACTOR: Verify log output is parseable

  **Must NOT do**:
  - Do NOT log message content (privacy) — only metadata
  - Do NOT add new log levels beyond Info/Warn/Error

  **Recommended Agent Profile**:
  > Deep task — structured logging plugin
  - **Category**: `deep`
    - Reason: Structured logging with privacy considerations
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 17, 19)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 20
  - **Blocked By**: Task 16

  **References**:
  - `server/internal/hook/hook.go` — Hook interface
  - `server/internal/agent/engine.go` — current logLLMRequest/logLLMResponse for log content reference

  **Acceptance Criteria**:
  - [ ] `go test ./internal/plugins/logging/... -run TestLoggingPlugin -v` → PASS (all 3 scenarios)
  - [ ] Log output contains session_id, agent_id keys

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: LoggingPlugin produces structured log output
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/plugins/logging/... -run TestLoggingPlugin -v -count=1
      2. Assert all 3 scenarios PASS
      3. Assert log output contains structured key=value pairs
    Expected Result: Structured metadata logged at each hook point
    Evidence: .sisyphus/evidence/task-18-logging.log
  ```

  **Evidence to Capture**:
  - [ ] task-18-logging.log

  **Commit**: YES (standalone)
  - Message: `feat(plugins): implement LoggingPlugin for structured LLM/tool observability`
  - Files: `server/internal/plugins/logging/logging_plugin.go`
  - Pre-commit: `cd server && go test ./internal/plugins/logging/... -count=1`

- [x] 19. **Implement TracingPlugin (spans for agent loop + tool execution)**

  **What to do**:
  - RED: Write test `TestTracingPluginSpans` — verify hook creates span on BeforeLLMCall, closes on AfterLLMCall
  - RED: Write test `TestTracingPluginToolSpans` — verify hook creates child span for each tool execution
  - RED: Write test `TestTracingPluginNoTracer` — verify hook is no-op when tracer is nil
  - GREEN: Create `server/internal/plugins/tracing/tracing_plugin.go` with `TracingPlugin` struct implementing `hook.Hook`
  - GREEN: `TracingPlugin.Points()` returns `[HookBeforeLLMCall, HookAfterLLMCall, HookBeforeToolExecute, HookAfterToolExecute, HookOnToolError]`
  - GREEN: `TracingPlugin.Priority()` returns 200
  - GREEN: Define `Tracer` interface: `StartSpan(name string) Span`, `Span` interface: `End()`, `SetAttribute(key, value string)`, `SetError(err error)`
  - GREEN: BeforeLLMCall → start "agent.llm_call" span; AfterLLMCall → set attributes (tokens, duration) → end span
  - GREEN: BeforeToolExecute → start "agent.tool.$name" span; AfterToolExecute/OnToolError → end span
  - GREEN: All spans carry session_id, agent_id, step_index attributes
  - REFACTOR: Verify plugin is no-op when tracer is nil

  **Must NOT do**:
  - Do NOT import OpenTelemetry — define our own Tracer/Span interface
  - Do NOT add tracing overhead when tracer is nil

  **Recommended Agent Profile**:
  > Deep task — tracing abstraction with span lifecycle
  - **Category**: `deep`
    - Reason: Span lifecycle management across hook points
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 17, 18)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 20
  - **Blocked By**: Task 16

  **References**:
  - `server/internal/hook/hook.go` — Hook interface
  - PATTERN: OpenTelemetry Go SDK span API — StartSpan → SetAttribute → End

  **Acceptance Criteria**:
  - [ ] `go test ./internal/plugins/tracing/... -run TestTracingPlugin -v` → PASS (all 3 scenarios)
  - [ ] Plugin is no-op when tracer is nil

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: TracingPlugin creates and closes spans correctly
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/plugins/tracing/... -run TestTracingPlugin -v -count=1
      2. Assert all 3 scenarios PASS
      3. Assert spans have correct attributes (session_id, agent_id)
    Expected Result: Spans created and closed at correct lifecycle points
    Evidence: .sisyphus/evidence/task-19-tracing.log
  ```

  **Evidence to Capture**:
  - [ ] task-19-tracing.log

  **Commit**: YES (standalone)
  - Message: `feat(plugins): implement TracingPlugin for agent loop and tool span tracing`
  - Files: `server/internal/plugins/tracing/tracing_plugin.go`
  - Pre-commit: `cd server && go test ./internal/plugins/tracing/... -count=1`

- [x] 20. **Wire plugins into main.go startup**

  **What to do**:
  - GREEN: In `main.go`: create `HookRunner`, register all 4 plugins in priority order:
    1. `TodoInjectionHook` (priority 50)
    2. `MemoryPlugin` (priority 100)
    3. `LoggingPlugin` (priority 200)
    4. `TracingPlugin` (priority 200)
  - GREEN: Create `LLMProvider` (OpenAIAdapter) from existing OpenAI client
  - GREEN: Pass `HookRunner` + `LLMProvider` to `NewAgentEngine` via options
  - GREEN: Pass logger to all constructors: `NewAgentEngine`, `NewContextManager`, `NewHandler`
  - GREEN: Verify `go build ./cmd/server` succeeds
  - REFACTOR: Run `go test ./internal/... -v -count=1` — must pass

  **Must NOT do**:
  - Do NOT change startup order
  - Do NOT change config loading
  - Do NOT remove any existing initialization

  **Recommended Agent Profile**:
  > Quick task — wiring in main.go
  - **Category**: `quick`
    - Reason: Mechanical wiring of already-built components
  - **Skills**: []
    - No special skills needed

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 3, last task
  - **Blocks**: F1-F4 (final verification)
  - **Blocked By**: Tasks 17, 18, 19 (all plugins)

  **References**:
  - `server/cmd/server/main.go` — current startup wiring
  - `server/internal/agent/engine.go` — NewAgentEngine signature
  - `server/internal/plugins/todo_hook.go` — TodoInjectionHook constructor
  - `server/internal/plugins/memory/memory_plugin.go` — MemoryPlugin constructor
  - `server/internal/plugins/logging/logging_plugin.go` — LoggingPlugin constructor
  - `server/internal/plugins/tracing/tracing_plugin.go` — TracingPlugin constructor

  **Acceptance Criteria**:
  - [ ] `cd server && go build ./cmd/server` → exit 0
  - [ ] `cd server && go test ./internal/... -v -count=1` → PASS

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: Server builds and all tests pass with plugins wired
    Tool: Bash (go build + go test)
    Steps:
      1. cd server && go build ./cmd/server
      2. Assert exit code 0
      3. cd server && go test ./internal/... -v -count=1 2>&1 | tail -30
      4. Assert no "FAIL" in output
    Expected Result: Build succeeds, all tests pass
    Evidence: .sisyphus/evidence/task-20-build-test.log

  Scenario: Startup with no plugins doesn't crash (empty HookRunner)
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/agent/... -run TestEngineNoPlugins -v -count=1
      2. Assert output contains "PASS"
    Expected Result: Engine works without any plugins registered
    Evidence: .sisyphus/evidence/task-20-no-plugins.log
  ```

  **Evidence to Capture**:
  - [ ] task-20-build-test.log
  - [ ] task-20-no-plugins.log

  **Commit**: YES (standalone)
  - Message: `feat: wire HookRunner, LLMProvider, and all plugins into main.go startup`
  - Files: `server/cmd/server/main.go`
  - Pre-commit: `cd server && go build ./cmd/server && go test ./internal/... -count=1`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.
>
> **Do NOT auto-proceed after verification. Wait for user's explicit approval before marking work complete.**

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, grep, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go build ./cmd/server`. Review all changed files for: `interface{}` (use typed alternatives), empty catch/error ignores, `log.Printf` in production code, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Run full test suite: `go test ./internal/... -v -count=1`. Verify identical pass/fail as pre-refactor. Verify slog output: `go run ./cmd/server 2>&1 | head -20`. Verify no log.Printf in production code. Verify hook system with edge cases (context cancellation, concurrent registration). Save to `.sisyphus/evidence/final-qa/`.
  Output: `Tests [N/N pass] | slog [OK/FAIL] | log.Printf [CLEAN/N found] | Hooks [N scenarios pass] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **Wave 1**: 1 commit per task or grouped by concern
- **Wave 2**: Hook package + ContextBuilder + TodoHook = 1 commit; Tool hooks + wiring = 1 commit
- **Wave 3**: LLMProvider + adapter + refactor = 1 commit; 3 plugins + wiring = 1 commit
- Pre-commit: `go test ./internal/... -count=1` must pass

---

## Success Criteria

### Verification Commands
```bash
# Pre-refactor baseline (save for comparison)
go test ./internal/agent/... -v -count=1 > /tmp/pre-refactor-baseline.log

# Post-refactor verification
go test ./internal/... -v -count=1          # Expected: all pass, identical to baseline
grep -r "log\.Printf" server/internal/ --include="*.go" | grep -v "_test.go"  # Expected: zero matches
grep -r "memoryMgr" server/internal/agent/ --include="*.go"                    # Expected: zero matches
go build ./cmd/server                                                           # Expected: exit 0
go vet ./...                                                                     # Expected: exit 0
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass
- [ ] Zero log.Printf in production code
- [ ] Zero memoryMgr references in agent/
- [ ] Hook system tests cover: no hooks, 1 hook, 3 hooks, erroring hook, panicking hook
- [ ] LLMProvider mock produces identical streaming behavior
- [ ] TodoInjectionHook produces byte-identical output as BuildContext todo injection