# Architecture Refactoring: Monolith → AgentHarness

## TL;DR

> **Quick Summary**: 将 CopCon Go 后端从单一模块 `github.com/copcon/server` 重构为核心库 `github.com/copcon/core`（AgentHarness）+ 参考应用 `github.com/copcon/server`，使外部用户可通过 `go get github.com/copcon/core` 直接使用内置能力的 Agent 引擎。
> 
> **Deliverables**:
> - `core/` — 独立 Go module，零基础设施依赖（Gin/GORM），内置工具/Hook/Skill 能力
> - `core/harness.go` — NewAgent() + NewHarness() 入口
> - `core/capabilities/` — 可插拔能力系统（工具 + Hook 按 init() 自注册）
> - `core/storage/` — SessionStore/MessageStore/TodoStore/MemoryStore 接口
> - `core/providers/postgres/` — GORM 存储实现
> - `core/providers/qdrant/` — Qdrant 存储实现
> - `server/` — 精简参考应用（main.go ≤ 60 行）
> - `go.work` — 多模块工作区
> 
> **Estimated Effort**: Large
> **Parallel Execution**: YES - 6 waves
> **Critical Path**: Phase 0 → Phase 1 → Phase 2 → Phase 3+4 → Phase 5 → Phase 6 → F1-F4

---

## Context

### Original Request
用户发现当前系统架构不符合最初目的：核心能力和应用建设耦合在一起。期望：
1. 核心能力 + 外围扩展的架构
2. 其它用户可以直接引用仓库开始使用
3. 定位为 AgentHarness（非 AgentSDK）— 内置开箱即用能力

### Interview Summary
**Key Discussions**:
- 工具、Hook、Skill 应该是核心库的一等公民，不是外部样例实现
- 必须支持 AgentFactory 工厂方法（不只是静态配置）— delegate_to 动态创建子 Agent
- 静态 AgentSpec 应自动生成 AgentFactory，用户也可通过 AgentFactorySpec 完全控制
- 最简场景一行创建 Engine，也要支持自定义扩展和多 Agent 互调
- AgentEngine 保持具体类型（不额外定义 Delegator 接口）
- OpenAI adapter 放在 core 内置（接受依赖膨胀代价）

**Research Findings**:
- etcd/go-workspace + libs/ + services/ 模式最适合 CopCon
- GOWORK=off 在 CI 中验证每个模块可独立构建
- OpenTelemetry 的 `internal/` 作用域规则：子模块不得导入父模块的 `internal/`
- Terraform 是反面案例（纯 internal，不对外暴露）

### Metis Review
**Identified Gaps** (addressed):
- config.yaml 包含真实 API key → 加入 Phase 0 安全修复
- session.Message 泄露范围覆盖 8 个包（不只是 handlers.go）→ 扩大 Phase 1 范围
- tools/delegate.go → agent.AgentEngine 具体类型依赖 → 用户选择保持具体类型（同 module 内 OK）
- TodoManager 既是工具又是 Hook 依赖的服务 → 加入 TodoStore 到 storage 接口
- testutil 跨模块共享问题 → 公开化 core/testutil/
- Phase 1 时间估算偏乐观 → 调整为 5-7 天
- backfillParts 旧序列化格式兼容 → Phase 1 加入兼容性保障
- ringbuf 依赖验证 → 加入 Phase 1 预检查

---

## Work Objectives

### Core Objective
将 CopCon 后端从单一 Go module 拆分为 importable 核心库 + 参考应用，使外部用户 `go get github.com/copcon/core` 即可获得完整 AgentHarness。

### Concrete Deliverables
- `core/go.mod` — module github.com/copcon/core，零 Gin/GORM 依赖
- `core/harness.go` — NewAgent() + NewHarness() + Harness.Build()
- `core/capabilities/` — 内置工具(7个) + Hook(4个)，init() 自注册
- `core/storage/` — SessionStore/MessageStore/TodoStore/MemoryStore 接口
- `core/providers/postgres/` — GORM 实现
- `core/providers/qdrant/` — Qdrant 实现
- `server/cmd/server/main.go` — ≤ 60 行
- `go.work` — use ./core ./server
- CI pipeline — GOWORK=off 验证独立构建

### Definition of Done
- [ ] `cd core && GOWORK=off go build ./...` 编译通过
- [ ] `cd core && GOWORK=off go test ./...` 全部通过
- [ ] `cd server && GOWORK=off go build ./...` 编译通过（require core）
- [ ] `grep -r "gorm\." server/internal/api/ --include="*.go"` 零结果
- [ ] `grep -r "github.com/copcon/server" core/ --include="*.go"` 零结果
- [ ] HTTP API 行为等价（SSE 事件类型 + 响应格式不变）

### Must Have
- core/ 可独立 `go get`
- 内置能力开箱即用（按名开启）
- AgentFactory 支持（静态 + 工厂双轨）
- 多 Agent 互调（delegate_to）
- 存储接口抽象（用户注入实现）
- 向后兼容 HTTP API

### Must NOT Have (Guardrails)
- Phase 1-2 期间不新增工具/Hook/API 端点
- Phase 1-2 期间不改 SSE 事件格式
- core/ 不得依赖 server/
- core/ 不得直接导入 gin/gorm/qdrant client（providers/ 除外）
- 不在 Phase 3 实现 skills/ 和 capabilities/memory/ — 仅留 stub
- 不添加新的 LLM provider（只保留 OpenAIAdapter）
- HarnessConfig 不做 YAML 加载 / 环境变量映射 — 纯 Go struct

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES
- **Automated tests**: Tests-after（Phase 0 补充基线测试，Phase 1-6 每步后验证）
- **Framework**: go test + testify（现有）
- **If TDD**: N/A

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Backend API**: Use Bash (curl) — Send requests, assert status + response fields
- **Go modules**: Use Bash (go build/test/vet) — Compile, test, lint
- **Import verification**: Use Bash (grep) — Verify no forbidden imports
- **File structure**: Use Bash (ls/find) — Verify directory structure

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 0 (Start Immediately — baseline + security):
├── Task 1: API baseline test capture [quick]
├── Task 2: Security fix — remove API key from config.yaml [quick]
└── Task 3: Integration test — server startup + CRUD + SSE [unspecified-high]

Wave 1 (After Wave 0 — decouple in-place, MOSTLY SEQUENTIAL):
├── Task 4: Create storage interfaces + pure value types [deep]
├── Task 5: Implement SessionStore + MessageStore in existing packages [unspecified-high]
├── Task 6: Remove GetDB() from SessionManager + rewrite handlers [unspecified-high]
├── Task 7: Move backfillParts/groupPartsByStep to entity [quick]
├── Task 8: Replace GetOpenAITools() → GetToolDefs() [unspecified-high]
├── Task 9: Abstract AsyncToolRegistry → AsyncToolTracker interface [unspecified-high]
├── Task 10: Split ChatContext — iface stays, impl moves to chatcontext/ [deep]
├── Task 11: Decouple agent/registry from config.Config [unspecified-high]
├── Task 12: Add TodoStore interface + decouple TodoManager [unspecified-high]
├── Task 13: Verify ringbuf dependency + validate GORM conversion POC [quick]
└── Task 14: Phase 1 full regression — API contract test [deep]

Wave 2 (After Wave 1 — extract core/ module):
├── Task 15: Create core/ directory + go.mod + skeleton [quick]
├── Task 16: Migrate entity/ + iface/ + chatcontext/ to core/ [unspecified-high]
├── Task 17: Migrate tool/ + llm/ + hook/ + context_builder/ to core/ [unspecified-high]
├── Task 18: Migrate agent/ + storage/ to core/ [unspecified-high]
├── Task 19: Migrate tools/ → core/capabilities/tools/ [unspecified-high]
├── Task 20: Migrate plugins/ → core/capabilities/hooks/ [unspecified-high]
├── Task 21: Migrate GORM models → core/providers/postgres/ [unspecified-high]
├── Task 22: Migrate Qdrant → core/providers/qdrant/ [unspecified-high]
├── Task 23: Create go.work + update all import paths [deep]
└── Task 24: Verify standalone builds — GOWORK=off [deep]

Wave 3 (After Wave 2 — capability system + harness, MAX PARALLEL):
├── Task 25: Create capability registry + Capability interface [deep]
├── Task 26: Add init() self-registration to all tools [unspecified-high]
├── Task 27: Add init() self-registration to all hooks [unspecified-high]
├── Task 28: Implement dependency resolution + wildcard expansion [deep]
├── Task 29: Implement Harness — NewAgent() + NewHarness() + Build() [deep]
├── Task 30: Implement AgentSpec → AgentFactory auto-conversion [unspecified-high]
├── Task 31: Implement custom tools/hooks merge + delegate_to late registration [deep]
├── Task 32: Create core/testutil/ as public package [quick]

Wave 4 (After Wave 3 — rewrite server + CI):
├── Task 33: Rewrite server/main.go using Harness [unspecified-high]
├── Task 34: Rewrite config → HarnessConfig mapping [unspecified-high]
├── Task 35: Rewrite api/ handlers — use Harness.Engine/Registry + storage interfaces [deep]
├── Task 36: CI pipeline — GOWORK=off per module [quick]
└── Task 37: Final integration test — full API contract equivalence [deep]

Wave FINAL (After ALL tasks — 4 parallel reviews):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)
→ Present results → Get explicit user okay

Critical Path: T1 → T4 → T6 → T14 → T15 → T23 → T24 → T29 → T33 → T37 → F1-F4 → user okay
Max Concurrent: 3 (Wave 0), 1 (Wave 1 — sequential for safety), 5 (Wave 2), 5 (Wave 3), 3 (Wave 4)
```

### Dependency Matrix

| Task | Depends On | Blocks | Wave |
|------|-----------|--------|------|
| 1 | - | 3, 14, 37 | 0 |
| 2 | - | - | 0 |
| 3 | 1 | 14 | 0 |
| 4 | - | 5, 6, 8, 12 | 1 |
| 5 | 4 | 6 | 1 |
| 6 | 4, 5 | 14 | 1 |
| 7 | - | 14 | 1 |
| 8 | 4 | 14 | 1 |
| 9 | - | 14 | 1 |
| 10 | - | 14, 16 | 1 |
| 11 | - | 14, 18 | 1 |
| 12 | 4 | 14 | 1 |
| 13 | - | 14 | 1 |
| 14 | 1, 3, 4-13 | 15 | 1 |
| 15 | 14 | 16-22 | 2 |
| 16 | 15, 10 | 23 | 2 |
| 17 | 15 | 23 | 2 |
| 18 | 15, 11 | 23 | 2 |
| 19 | 15 | 23, 26 | 2 |
| 20 | 15 | 23, 27 | 2 |
| 21 | 15, 4 | 23 | 2 |
| 22 | 15, 4 | 23 | 2 |
| 23 | 15-22 | 24 | 2 |
| 24 | 23 | 25 | 2 |
| 25 | 24 | 26-28 | 3 |
| 26 | 19, 25 | 29 | 3 |
| 27 | 20, 25 | 29 | 3 |
| 28 | 25 | 29 | 3 |
| 29 | 25-28 | 30, 31 | 3 |
| 30 | 29 | 33 | 3 |
| 31 | 29 | 33 | 3 |
| 32 | 24 | - | 3 |
| 33 | 29 | 37 | 4 |
| 34 | 33 | 37 | 4 |
| 35 | 33 | 37 | 4 |
| 36 | 24 | - | 4 |
| 37 | 33-35, 1 | F1-F4 | 4 |

### Agent Dispatch Summary

- **Wave 0**: 3 — T1 → `quick`, T2 → `quick`, T3 → `unspecified-high`
- **Wave 1**: 11 — T4 → `deep`, T5-T6 → `unspecified-high`, T7 → `quick`, T8-T9 → `unspecified-high`, T10 → `deep`, T11-T12 → `unspecified-high`, T13 → `quick`, T14 → `deep`
- **Wave 2**: 10 — T15 → `quick`, T16-T22 → `unspecified-high`, T23-T24 → `deep`
- **Wave 3**: 8 — T25, T28-T29, T31 → `deep`, T26-T27, T30 → `unspecified-high`, T32 → `quick`
- **Wave 4**: 5 — T33-T34 → `unspecified-high`, T35, T37 → `deep`, T36 → `quick`
- **FINAL**: 4 — F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

- [x] 1. Capture API baseline test data

  **What to do**:
  - Start the server via `docker compose up -d && cd server && go run ./cmd/server`
  - For each endpoint, capture exact request/response pairs:
    - `POST /api/sessions` → 201 + {id, title, default_agent_id, created_at, updated_at, message_count}
    - `GET /api/sessions` → 200 + {sessions: [...], total: N}
    - `GET /api/sessions/{id}` → 200 + {id, title, ...}
    - `DELETE /api/sessions/{id}` → 204
    - `GET /api/sessions/{id}/messages` → 200 + {messages: [...]}
    - `POST /api/sessions/{id}/chat` → SSE stream with event types: step_create, part_create, part_update, message_done
  - Save captured responses as JSON files in `.sisyphus/evidence/task-1-baseline/`
  - Record the exact SSE event sequence for a chat request (event types + data field shapes)

  **Must NOT do**:
  - Do not modify any code
  - Do not add new test infrastructure

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Pure capture task, no code changes
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 0 (with Tasks 2, 3)
  - **Blocks**: Tasks 3, 14, 37
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/api/handlers.go:502-523` — SetupRoutes shows all endpoint paths and HTTP methods
  - `server/internal/api/handlers.go:74-118` — CreateSession handler shows exact response format
  - `server/internal/api/handlers.go:344-439` — Chat handler shows SSE event format

  **External References**:
  - OpenAPI spec: `api/openapi.yaml` — Full API contract

  **Acceptance Criteria**:
  - [ ] `.sisyphus/evidence/task-1-baseline/create-session.json` exists with 201 response body
  - [ ] `.sisyphus/evidence/task-1-baseline/list-sessions.json` exists with 200 response body
  - [ ] `.sisyphus/evidence/task-1-baseline/get-messages.json` exists with 200 response body
  - [ ] `.sisyphus/evidence/task-1-baseline/chat-sse-events.txt` exists with captured SSE event sequence

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Baseline capture — create session
    Tool: Bash (curl)
    Preconditions: Server running on localhost:8088
    Steps:
      1. curl -sf -X POST http://localhost:8088/api/sessions -H 'Content-Type: application/json' -d '{"title":"baseline-test","default_agent_id":"code-assistant"}' -o .sisyphus/evidence/task-1-baseline/create-session.json -w "%{http_code}"
      2. Verify HTTP 201
      3. Parse JSON, verify fields: id, title, default_agent_id, created_at, message_count
    Expected Result: 201 status, JSON body with all expected fields
    Failure Indicators: Non-201 status, missing fields in response
    Evidence: .sisyphus/evidence/task-1-baseline/create-session.json

  Scenario: Baseline capture — SSE chat stream
    Tool: Bash (curl)
    Preconditions: Session created from previous scenario
    Steps:
      1. Extract session ID from create-session.json
      2. curl -N -sf -X POST http://localhost:8088/api/sessions/{id}/chat -H 'Content-Type: application/json' -d '{"content":"hello","agent_id":"code-assistant"}' -o .sisyphus/evidence/task-1-baseline/chat-sse-events.txt
      3. Verify file contains lines starting with "data:" containing JSON with "type" field
    Expected Result: SSE stream with events containing type field
    Failure Indicators: Empty file, no "data:" lines, missing "type" field
    Evidence: .sisyphus/evidence/task-1-baseline/chat-sse-events.txt
  ```

  **Commit**: YES
  - Message: `test(server): capture API baseline for refactoring contract`
  - Files: `.sisyphus/evidence/task-1-baseline/*.json`, `.sisyphus/evidence/task-1-baseline/*.txt`

- [x] 2. Security fix — remove API key from config.yaml

  **What to do**:
  - Replace the real API key in `server/config.yaml` with placeholder `your-api-key-here`
  - Ensure `server/config.yaml` is in `.gitignore`
  - Verify `server/config.yaml.template` has the placeholder (not a real key)
  - Run `git log --all --oneline -- server/config.yaml` to check if key was ever committed — if yes, note that key rotation is needed

  **Must NOT do**:
  - Do not change any Go code
  - Do not modify config loading logic

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple file edit, no code changes
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 0 (with Tasks 1, 3)
  - **Blocks**: None
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/config.yaml:14` — Contains `api_key: "fk3542824333._AqVHr-B6b7njVE-YwPLevrh7T6RnFVp0d7ee39f"`
  - `server/config.yaml.template` — Should already have placeholder
  - `.gitignore` — Need to add config.yaml if not present

  **Acceptance Criteria**:
  - [ ] `server/config.yaml` contains `api_key: "your-api-key-here"` (or similar placeholder)
  - [ ] `server/config.yaml` is listed in `.gitignore`
  - [ ] `grep -r "fk3542824333" server/config.yaml` returns nothing

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Verify API key removed from config
    Tool: Bash (grep)
    Preconditions: File edit completed
    Steps:
      1. grep "fk3542824333" server/config.yaml
      2. Verify exit code is 1 (not found)
      3. grep "api_key" server/config.yaml
      4. Verify line contains placeholder, not real key
    Expected Result: No real API key in config.yaml
    Failure Indicators: Real API key still present
    Evidence: .sisyphus/evidence/task-2-security-fix.txt

  Scenario: Verify config.yaml in .gitignore
    Tool: Bash (grep)
    Preconditions: .gitignore updated
    Steps:
      1. grep "config.yaml" .gitignore
      2. Verify match found
    Expected Result: config.yaml listed in .gitignore
    Failure Indicators: Not listed — accidental commit risk
    Evidence: .sisyphus/evidence/task-2-gitignore-check.txt
  ```

  **Commit**: YES
  - Message: `fix(security): remove API key from config.yaml, add to .gitignore`
  - Files: `server/config.yaml`, `.gitignore`

- [x] 3. Add integration tests — server startup + CRUD + SSE

  **What to do**:
  - Create `server/internal/integration_test.go` (or extend existing)
  - Test: server starts and responds to `/health`
  - Test: `POST /api/sessions` creates a session, returns 201
  - Test: `GET /api/sessions` lists sessions, returns 200
  - Test: `GET /api/sessions/{id}/messages` returns messages, returns 200
  - Test: `DELETE /api/sessions/{id}` deletes, returns 204
  - Tests should use `setupTestDB(t)` pattern (existing) + httptest recorder
  - If full SSE testing is too complex for unit tests, at minimum test that Chat endpoint returns correct headers (Content-Type: text/event-stream)

  **Must NOT do**:
  - Do not modify production code
  - Do not change existing test patterns
  - Do not add external test dependencies

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Requires understanding existing test patterns + HTTP testing setup
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (but depends on Task 1 for reference data)
  - **Parallel Group**: Wave 0 (with Tasks 1, 2)
  - **Blocks**: Task 14
  - **Blocked By**: Task 1 (needs baseline data for assertions)

  **References**:
  **Pattern References**:
  - `server/internal/session/manager_test.go` — `setupTestDB(t)` pattern for GORM test DB
  - `server/internal/api/handlers_test.go` — Existing handler test patterns with `testify/assert`
  - `server/internal/integration_test.go` — Existing integration test file

  **API/Type References**:
  - `server/internal/api/handlers.go:502-523` — SetupRoutes for endpoint registration
  - `server/internal/api/handlers.go:63` — NewHandler constructor

  **Acceptance Criteria**:
  - [ ] `go test ./internal/... -run TestIntegration -v` passes
  - [ ] At least 5 test cases: health, create session, list sessions, get messages, delete session
  - [ ] Each test uses httptest.NewRecorder() for response capture

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Integration tests pass
    Tool: Bash (go test)
    Preconditions: PostgreSQL running (docker compose up -d)
    Steps:
      1. cd server && go test ./internal/... -run TestIntegration -v -count=1
      2. Verify exit code 0
      3. Verify output contains PASS for each test case
    Expected Result: All integration tests pass
    Failure Indicators: Any test FAIL, exit code non-zero
    Evidence: .sisyphus/evidence/task-3-integration-test-output.txt

  Scenario: Tests cover all CRUD endpoints
    Tool: Bash (grep)
    Preconditions: Tests written
    Steps:
      1. grep -c "func Test" server/internal/integration_test.go
      2. Verify count >= 5
    Expected Result: At least 5 test functions
    Failure Indicators: Fewer than 5 tests
    Evidence: .sisyphus/evidence/task-3-test-count.txt
  ```

  **Commit**: YES
  - Message: `test(server): add integration tests for CRUD + SSE endpoints`
  - Files: `server/internal/integration_test.go`
  - Pre-commit: `cd server && go test ./internal/... -count=1`

- [x] 4. Create storage interfaces + pure value types

  **What to do**:
  - Create `server/internal/storage/` package
  - Define `Session` struct (pure Go, no GORM annotations) with fields: ID, Title, DefaultAgentID, ParentSessionID, Metadata, CreatedAt, UpdatedAt
  - Define `Message` struct (pure Go) with fields: ID, SessionID, Role, Content, Reasoning, ToolCalls, Parts, Model, TokenCount, DurationMs, CreatedAt
  - Define `ToolCall`, `FunctionCall`, `Part` structs
  - Define `Todo` struct (pure Go) with fields: ID, SessionID, Content, Status, Priority, CreatedAt, UpdatedAt
  - Define `Memory` struct (pure Go) with fields: ID, Content, SessionID, Role, Timestamp, MemoryType, Metadata, Score
  - Define `SessionStore` interface: Create, Get, List, Delete, UpdateTitle, UpdateMetadata, GetMessageCount
  - Define `MessageStore` interface: List, Add, Update, Upsert, DeleteBySession
  - Define `TodoStore` interface: Create, Get, List, UpdateStatus, DeleteBySession
  - Define `MemoryStore` interface: Store, Search, GetBySession, DeleteBySession
  - Write conversion helpers: `sessionToDomain()` / `sessionToModel()` as proof-of-concept

  **Must NOT do**:
  - Do not modify existing code yet
  - Do not remove existing interfaces

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Requires careful interface design to match existing usage patterns across 8 packages
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 7, 9, 10, 11, 13)
  - **Parallel Group**: Wave 1
  - **Blocks**: Tasks 5, 6, 8, 12
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/session/manager.go:28-38` — Current SessionManager interface (methods to replicate)
  - `server/internal/session/model.go` — Current GORM Session/Message models (field definitions)
  - `server/internal/chat_context/manager.go:22-29` — Current ContextManager interface
  - `server/internal/tools/todo/manager.go` — Current TodoManager interface
  - `server/internal/memory/manager.go:18-23` — Current MemoryManager interface
  - `server/internal/domain/iface/chat.go:15-30` — Interface definition style

  **Acceptance Criteria**:
  - [ ] `server/internal/storage/session.go` exists with Session struct + SessionStore interface
  - [ ] `server/internal/storage/message.go` exists with Message struct + MessageStore interface
  - [ ] `server/internal/storage/todo.go` exists with Todo struct + TodoStore interface
  - [ ] `server/internal/storage/memory.go` exists with Memory struct + MemoryStore interface
  - [ ] `cd server && go build ./internal/storage/...` compiles
  - [ ] No `gorm.io` imports in any storage/ file

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Storage interfaces compile without GORM
    Tool: Bash (go build + grep)
    Preconditions: storage/ package created
    Steps:
      1. cd server && go build ./internal/storage/...
      2. Verify exit code 0
      3. grep -r "gorm" server/internal/storage/ --include="*.go"
      4. Verify zero matches
    Expected Result: Compiles cleanly, no GORM dependency
    Failure Indicators: Compilation error, GORM import found
    Evidence: .sisyphus/evidence/task-4-storage-interfaces.txt

  Scenario: GORM conversion POC works
    Tool: Bash (go test)
    Preconditions: Conversion helpers written
    Steps:
      1. cd server && go test ./internal/storage/... -v -count=1
      2. Verify conversion round-trip: model→domain→model produces identical result
    Expected Result: All conversion tests pass
    Failure Indicators: Data loss in round-trip conversion
    Evidence: .sisyphus/evidence/task-4-conversion-poc.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): add storage interfaces and pure value types`
  - Files: `server/internal/storage/*.go`
  - Pre-commit: `cd server && go build ./internal/storage/...`

- [x] 5. Implement SessionStore + MessageStore in existing packages

  **What to do**:
  - In `session/manager.go`, add methods that implement `storage.SessionStore` interface
  - In `chat_context/manager.go`, add methods that implement `storage.MessageStore` interface
  - In `tools/todo/manager.go`, add methods that implement `storage.TodoStore` interface
  - Add compile-time interface checks: `var _ storage.SessionStore = (*sessionManager)(nil)`
  - Keep existing methods working — this is additive, not replacement

  **Must NOT do**:
  - Do not remove existing SessionManager/ContextManager interfaces
  - Do not break existing callers

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Requires modifying existing code while preserving backward compatibility
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 4)
  - **Parallel Group**: Wave 1 (after Task 4)
  - **Blocks**: Task 6
  - **Blocked By**: Task 4

  **References**:
  **Pattern References**:
  - `server/internal/session/manager.go:40-46` — sessionManager struct with *gorm.DB
  - `server/internal/chat_context/manager.go:31-35` — contextManager struct with *gorm.DB
  - `server/internal/tools/todo/manager.go` — todoManager struct

  **Acceptance Criteria**:
  - [ ] `sessionManager` implements `storage.SessionStore` (compile-time check passes)
  - [ ] `contextManager` implements `storage.MessageStore` (compile-time check passes)
  - [ ] `todoManager` implements `storage.TodoStore` (compile-time check passes)
  - [ ] `go test ./... -count=1` still passes (no regressions)

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Existing tests still pass after adding interface implementations
    Tool: Bash (go test)
    Preconditions: Interface implementations added
    Steps:
      1. cd server && go test ./... -count=1
      2. Verify exit code 0
    Expected Result: All tests pass, zero regressions
    Failure Indicators: Any test failure
    Evidence: .sisyphus/evidence/task-5-test-results.txt

  Scenario: Compile-time interface checks pass
    Tool: Bash (go build)
    Preconditions: var _ checks added
    Steps:
      1. cd server && go build ./internal/session/... ./internal/chat_context/... ./internal/tools/todo/...
      2. Verify exit code 0
    Expected Result: All interface implementations verified at compile time
    Failure Indicators: Missing method, wrong signature
    Evidence: .sisyphus/evidence/task-5-interface-checks.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): implement storage interfaces in existing managers`
  - Files: `server/internal/session/manager.go`, `server/internal/chat_context/manager.go`, `server/internal/tools/todo/manager.go`
  - Pre-commit: `cd server && go test ./... -count=1`

- [x] 6. Remove GetDB() from SessionManager + rewrite handlers

  **What to do**:
  - Remove `GetDB() *gorm.DB` from `SessionManager` interface
  - Move the GORM query in `handlers.go:204-211` (GetMessages) into `MessageStore.List()`
  - Rewrite `handlers.go` `GetMessages()` to use `h.messageStore.List()` instead of raw GORM
  - Remove `GetDB() *gorm.DB` from `TodoManager` interface
  - Update `Handler` struct to hold `MessageStore` instead of (or in addition to) `SessionManager` for message queries
  - Update `SetupRoutes` signature to accept `MessageStore`
  - Update `main.go` to pass `MessageStore` to `SetupRoutes`

  **Must NOT do**:
  - Do not change HTTP response format
  - Do not add new API endpoints

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Breaking API change in internal interfaces, needs careful update of all callers
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 4, 5)
  - **Parallel Group**: Wave 1 (after Task 5)
  - **Blocks**: Task 14
  - **Blocked By**: Tasks 4, 5

  **References**:
  **Pattern References**:
  - `server/internal/api/handlers.go:204-211` — The raw GORM query to eliminate
  - `server/internal/api/handlers.go:54-61` — Handler struct fields
  - `server/internal/api/handlers.go:502-523` — SetupRoutes signature
  - `server/cmd/server/main.go:217` — SetupRoutes call in main

  **Acceptance Criteria**:
  - [ ] `grep "GetDB()" server/internal/session/manager.go` returns nothing (method removed from interface)
  - [ ] `grep "GetDB()" server/internal/api/handlers.go` returns nothing
  - [ ] `grep "gorm\." server/internal/api/handlers.go` returns nothing (no GORM in API layer)
  - [ ] `go test ./... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: GetMessages endpoint still works after removing GetDB
    Tool: Bash (curl)
    Preconditions: Server running, session with messages exists
    Steps:
      1. curl -sf http://localhost:8088/api/sessions/{id}/messages
      2. Verify HTTP 200
      3. Verify JSON body has "messages" array with correct structure
    Expected Result: Same response format as baseline
    Failure Indicators: HTTP error, missing fields, different structure
    Evidence: .sisyphus/evidence/task-6-getmessages.txt

  Scenario: No GORM imports in API handlers
    Tool: Bash (grep)
    Preconditions: Code refactored
    Steps:
      1. grep -r "gorm\." server/internal/api/ --include="*.go"
      2. Verify zero matches
    Expected Result: API layer is GORM-free
    Failure Indicators: GORM reference found
    Evidence: .sisyphus/evidence/task-6-no-gorm.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): remove GetDB() leak, use MessageStore in handlers`
  - Files: `server/internal/api/handlers.go`, `server/internal/session/manager.go`, `server/internal/tools/todo/manager.go`, `server/cmd/server/main.go`
  - Pre-commit: `cd server && go test ./... -count=1`

- [x] 7. Move backfillParts/groupPartsByStep to entity package

  **What to do**:
  - Move `groupPartsByStep()` from `api/handlers.go:259-289` to `domain/entity/convert.go` (or new `domain/entity/render.go`)
  - Move `backfillParts()` from `api/handlers.go:293-342` to same location
  - These functions operate on `session.PersistedParts` — after storage abstraction they'll operate on `storage.Part` (or `entity.UIPart`)
  - For now, move as-is (still using `session.PersistedParts`) — the type change happens in Task 6
  - Update `handlers.go` to import from `entity` instead of defining locally

  **Must NOT do**:
  - Do not change function behavior
  - Do not change the output format

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Pure code move, minimal logic change
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 14
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/api/handlers.go:259-289` — groupPartsByStep function
  - `server/internal/api/handlers.go:293-342` — backfillParts function
  - `server/internal/domain/entity/convert.go` — Existing conversion functions (pattern to follow)

  **Acceptance Criteria**:
  - [ ] `groupPartsByStep` no longer defined in `api/handlers.go`
  - [ ] `backfillParts` no longer defined in `api/handlers.go`
  - [ ] Both functions accessible from `domain/entity/` package
  - [ ] `go test ./... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: GetMessages still returns correct step structure
    Tool: Bash (curl)
    Preconditions: Server running, session with multi-step messages exists
    Steps:
      1. curl -sf http://localhost:8088/api/sessions/{id}/messages
      2. Verify response contains "steps" array with correct structure
    Expected Result: Same step grouping as before the move
    Failure Indicators: Missing steps, wrong grouping, empty response
    Evidence: .sisyphus/evidence/task-7-step-structure.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): move backfillParts/groupPartsByStep to entity package`
  - Files: `server/internal/domain/entity/render.go`, `server/internal/api/handlers.go`

- [x] 8. Replace GetOpenAITools() → GetToolDefs()

  **What to do**:
  - Add `GetToolDefs() []llm.ToolDef` method to `ToolManager` interface
  - Implement `GetToolDefs()` in `toolManager` — convert internal tool definitions to `llm.ToolDef` (already have the data)
  - Replace all callers of `GetOpenAITools()` with `GetToolDefs()`
  - Move OpenAI-specific conversion (`convertToLLMTools` in `agent/engine.go:592-609`) into `llm/openai_adapter.go`
  - Remove `GetOpenAITools()` from `ToolManager` interface
  - Remove `openai` import from `tool/manager.go`

  **Must NOT do**:
  - Do not change tool execution behavior
  - Do not modify Tool interface

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Interface change affects multiple packages, need to update all callers
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 7, 9, 10, 11, 13)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 14
  - **Blocked By**: Task 4 (needs llm.ToolDef type)

  **References**:
  **Pattern References**:
  - `server/internal/tool/manager.go:52` — `GetOpenAITools() []openai.ChatCompletionToolUnionParam` (the method to replace)
  - `server/internal/tool/manager.go:182-223` — Current implementation using OpenAI SDK types
  - `server/internal/agent/engine.go:592-609` — `convertToLLMTools()` that does OpenAI conversion
  - `server/internal/llm/provider.go:107-118` — `ToolDef` struct (target type)

  **Acceptance Criteria**:
  - [ ] `grep "GetOpenAITools" server/internal/tool/manager.go` returns nothing
  - [ ] `grep "openai\." server/internal/tool/manager.go` returns nothing (no OpenAI SDK in tool package)
  - [ ] `grep "GetToolDefs" server/internal/tool/manager.go` returns match
  - [ ] `go test ./... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Tool definitions still work for LLM calls
    Tool: Bash (curl)
    Preconditions: Server running
    Steps:
      1. Send a chat message that triggers a tool call
      2. Verify tool_call event appears in SSE stream
    Expected Result: Tool calls work identically
    Failure Indicators: No tool_call events, LLM errors about tool format
    Evidence: .sisyphus/evidence/task-8-tool-defs.txt

  Scenario: No OpenAI SDK types in tool package
    Tool: Bash (grep)
    Preconditions: Code refactored
    Steps:
      1. grep -r "openai\." server/internal/tool/ --include="*.go"
      2. Verify zero matches (excluding test files)
    Expected Result: tool package is OpenAI-SDK-free
    Failure Indicators: OpenAI SDK import found
    Evidence: .sisyphus/evidence/task-8-no-openai.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): replace GetOpenAITools() with provider-agnostic GetToolDefs()`
  - Files: `server/internal/tool/manager.go`, `server/internal/agent/engine.go`, `server/internal/llm/openai_adapter.go`

- [x] 9. Abstract AsyncToolRegistry → AsyncToolTracker interface

  **What to do**:
  - Define `AsyncToolTracker` interface in `tool/registry.go` with methods: Register, Unregister, Complete, Fail, GetStatus, Cancel, CancelSession, ListBySession
  - Add compile-time check: `var _ AsyncToolTracker = (*AsyncToolRegistry)(nil)`
  - Change `NewAgentEngine()` signature: accept `AsyncToolTracker` instead of `*AsyncToolRegistry`
  - Change `NewSessionManager()` signature: accept `AsyncToolTracker` instead of `*AsyncToolRegistry`
  - Update `main.go` to pass `*AsyncToolRegistry` as `AsyncToolTracker`
  - Document thread-safety contract on `AsyncToolTracker` interface (must be goroutine-safe)

  **Must NOT do**:
  - Do not change AsyncToolRegistry internal implementation
  - Do not change async tool execution behavior

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Interface change propagates to engine + session + main
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 7, 8, 10, 11, 13)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 14
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/tool/registry.go:34-37` — AsyncToolRegistry struct
  - `server/internal/tool/registry.go:40-133` — All methods to include in interface
  - `server/internal/agent/engine.go:118` — NewAgentEngine takes `*tool.AsyncToolRegistry`
  - `server/internal/session/manager.go:45` — NewSessionManager takes `*tool.AsyncToolRegistry`

  **Acceptance Criteria**:
  - [ ] `AsyncToolTracker` interface defined in `tool/registry.go`
  - [ ] `NewAgentEngine` accepts `AsyncToolTracker` (not `*AsyncToolRegistry`)
  - [ ] `NewSessionManager` accepts `AsyncToolTracker` (not `*AsyncToolRegistry`)
  - [ ] `go test ./... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Async tools still work
    Tool: Bash (curl)
    Preconditions: Server running
    Steps:
      1. Send a chat message that triggers async tool execution
      2. Verify async_tool_started + async_tool_complete events in SSE stream
    Expected Result: Async tool lifecycle works identically
    Failure Indicators: Missing async events, tool execution errors
    Evidence: .sisyphus/evidence/task-9-async-tools.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): abstract AsyncToolRegistry into AsyncToolTracker interface`
  - Files: `server/internal/tool/registry.go`, `server/internal/agent/engine.go`, `server/internal/session/manager.go`, `server/cmd/server/main.go`

- [x] 10. Split ChatContext — iface stays, impl moves to chatcontext/

  **What to do**:
  - Keep in `domain/iface/chat.go`: `ChatContextInterface`, `Storer`, `Subscriber`, `InputRequest`, `InputResponse`, `InterruptType`, `ErrInterruptNotFound`
  - Move to new `chatcontext/` package: `ChatContext` struct, `NewChatContext()`, `WithDepth()`, all private methods (Emit, Close, Subscribe, RequestInput, ResolveInput, etc.)
  - Remove `ringbuf` import from `domain/iface/chat.go`
  - Update all callers of `iface.NewChatContext()` to import from `chatcontext` package instead
  - Add compile-time check in chatcontext/: `var _ iface.ChatContextInterface = (*ChatContext)(nil)`
  - Also move `chat_context/store.go` `SessionAgentStore` — update to use `iface.ChatContextInterface` (not `*iface.ChatContext`)

  **Must NOT do**:
  - Do not change ChatContextInterface methods
  - Do not change SSE event emission behavior

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Touches many files (30+ callers of NewChatContext), needs careful import path updates
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 7, 8, 9, 11, 13)
  - **Parallel Group**: Wave 1
  - **Blocks**: Tasks 14, 16
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/domain/iface/chat.go:48-64` — ChatContext struct (to move)
  - `server/internal/domain/iface/chat.go:194-206` — NewChatContext() (to move)
  - `server/internal/api/handlers.go:100,107,131,399,529,560` — Callers of iface.NewChatContext()
  - `server/internal/chat_context/store.go` — SessionAgentStore

  **Acceptance Criteria**:
  - [ ] `domain/iface/chat.go` has no `ringbuf` import
  - [ ] `domain/iface/chat.go` has no `ChatContext` struct definition
  - [ ] `chatcontext/` package has `ChatContext` struct + `NewChatContext()`
  - [ ] `go test ./... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Chat still works after ChatContext split
    Tool: Bash (curl)
    Preconditions: Server running
    Steps:
      1. curl -N -X POST http://localhost:8088/api/sessions/{id}/chat -H 'Content-Type: application/json' -d '{"content":"test after split"}'
      2. Verify SSE stream with step_create, part_create, part_update, message_done events
    Expected Result: Chat behavior identical to baseline
    Failure Indicators: No SSE events, connection errors
    Evidence: .sisyphus/evidence/task-10-chatcontext-split.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): split ChatContext — interface in iface/, impl in chatcontext/`
  - Files: `server/internal/domain/iface/chat.go`, `server/internal/chatcontext/chat_context.go`, `server/internal/api/handlers.go`, `server/cmd/server/main.go`

- [x] 11. Decouple agent/registry from config.Config

  **What to do**:
  - Change `NewAgentRegistry()` to NOT take `*config.Config`
  - Instead, accept: `defaultAgentID string` + use `RegisterFactory()` for all agent registration
  - Remove `config` import from `agent/registry.go`
  - Remove the config-based agent factory creation loop in `NewAgentRegistry()` (lines 84-159)
  - Move that logic to `main.go` — it already exists there as the `RegisterFactory` calls (lines 122-193)
  - Simplify `NewAgentRegistry()` to just create an empty registry with a default agent ID

  **Must NOT do**:
  - Do not change AgentRegistry interface
  - Do not change AgentFactory signature

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Changes registry constructor + main.go wiring
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 7, 8, 9, 10, 13)
  - **Parallel Group**: Wave 1
  - **Blocks**: Tasks 14, 18
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/agent/registry.go:84-160` — NewAgentRegistry with config dependency
  - `server/cmd/server/main.go:114-193` — RegisterFactory calls (already doing the work)
  - `server/internal/config/config.go:9-16` — Config struct with Agents field

  **Acceptance Criteria**:
  - [ ] `grep "config" server/internal/agent/registry.go` returns nothing (no config import)
  - [ ] `NewAgentRegistry` signature does not include `*config.Config`
  - [ ] `go test ./... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Agent registration still works
    Tool: Bash (curl)
    Preconditions: Server running
    Steps:
      1. curl -sf http://localhost:8088/api/agents
      2. Verify response lists code-assistant and chat-assistant
    Expected Result: Both agents registered and listed
    Failure Indicators: Empty agents list, missing agents
    Evidence: .sisyphus/evidence/task-11-agent-registry.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): decouple agent/registry from config.Config`
  - Files: `server/internal/agent/registry.go`, `server/cmd/server/main.go`

- [x] 12. Add TodoStore interface + decouple TodoManager

  **What to do**:
  - Add `TodoStore` interface to `storage/todo.go` (if not already in Task 4)
  - Implement `TodoStore` in `tools/todo/manager.go` using existing GORM code
  - Update `plugins/todo_hook.go` to depend on `TodoStore` interface instead of concrete `TodoManager`
  - Remove `GetDB() *gorm.DB` from `TodoManager` interface
  - Ensure `TodoTool` gets `TodoStore` through `CapabilityDeps` (prep for Phase 3)

  **Must NOT do**:
  - Do not change Todo business logic
  - Do not change Todo API response format

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Cross-cutting: storage interface + tool + hook all need update
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 4)
  - **Parallel Group**: Wave 1 (after Task 4)
  - **Blocks**: Task 14
  - **Blocked By**: Task 4

  **References**:
  **Pattern References**:
  - `server/internal/tools/todo/manager.go` — TodoManager with *gorm.DB
  - `server/internal/plugins/todo_hook.go` — TodoInjectionHook depending on TodoManager
  - `server/internal/tools/todo_tool.go` — TodoTool depending on TodoManager

  **Acceptance Criteria**:
  - [ ] `TodoStore` interface defined in `storage/`
  - [ ] `plugins/todo_hook.go` depends on `TodoStore` (not `TodoManager`)
  - [ ] `GetDB()` removed from `TodoManager` interface
  - [ ] `go test ./... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Todo CRUD still works
    Tool: Bash (curl)
    Preconditions: Server running, session exists
    Steps:
      1. curl -sf http://localhost:8088/api/sessions/{id}/todos
      2. Verify HTTP 200 with todo list
    Expected Result: Todo functionality unchanged
    Failure Indicators: HTTP error, missing todos
    Evidence: .sisyphus/evidence/task-12-todo-store.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): add TodoStore interface, decouple TodoManager from GORM`
  - Files: `server/internal/storage/todo.go`, `server/internal/tools/todo/manager.go`, `server/internal/plugins/todo_hook.go`

- [x] 13. Verify ringbuf dependency + validate GORM conversion POC

  **What to do**:
  - Check `github.com/golang-cz/ringbuf` maintenance: go doc, GitHub last commit date, open issues
  - Verify ringbuf transitive dependency footprint: `cd server && go mod graph | grep ringbuf`
  - Write a POC test that creates a `storage.Message` from a `session.Message` (GORM model) and back — verify zero data loss
  - Test with real data shapes: messages with Parts, messages with ToolCalls, messages with PersistedParts
  - If conversion POC reveals issues, document them for Phase 2 mitigation

  **Must NOT do**:
  - Do not change any production code
  - Do not replace ringbuf

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Verification task, no code changes
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 14
  - **Blocked By**: None

  **References**:
  **Pattern References**:
  - `server/internal/session/model.go` — All GORM model definitions
  - `server/internal/storage/` — New pure value types (from Task 4)

  **Acceptance Criteria**:
  - [ ] ringbuf maintenance status documented (last commit, open issues)
  - [ ] GORM conversion POC test passes for all message shapes
  - [ ] Any conversion issues documented

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: GORM conversion round-trip preserves data
    Tool: Bash (go test)
    Preconditions: POC test written
    Steps:
      1. cd server && go test ./internal/storage/... -v -run TestConversion
      2. Verify all test cases pass
    Expected Result: Zero data loss in GORM ↔ storage conversion
    Failure Indicators: Any test failure = data shape mismatch
    Evidence: .sisyphus/evidence/task-13-conversion-poc.txt
  ```

  **Commit**: YES
  - Message: `test(server): validate GORM conversion POC and ringbuf dependency`
  - Files: `server/internal/storage/conversion_test.go`

- [x] 14. Phase 1 full regression — API contract test

  **What to do**:
  - Run all Phase 0 baseline tests again
  - Compare HTTP responses against captured baseline data
  - Verify SSE event sequence matches baseline
  - Run full `go test ./... -count=1`
  - Run `go vet ./...`
  - Document any deviations from baseline

  **Must NOT do**:
  - Do not fix any bugs found — document them for separate fix

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Comprehensive regression analysis, needs to compare against baseline
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on ALL Phase 1 tasks)
  - **Parallel Group**: Wave 1 (final gate)
  - **Blocks**: Task 15 (Phase 2 cannot start until Phase 1 is verified)
  - **Blocked By**: Tasks 1, 3, 4-13

  **References**:
  **Pattern References**:
  - `.sisyphus/evidence/task-1-baseline/` — Captured baseline data

  **Acceptance Criteria**:
  - [ ] `go test ./... -count=1` passes
  - [ ] `go vet ./...` passes
  - [ ] All CRUD endpoints return same response format as baseline
  - [ ] SSE event types match baseline

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Full API contract equivalence
    Tool: Bash (curl + diff)
    Preconditions: All Phase 1 tasks complete, server running
    Steps:
      1. Re-run all baseline capture commands from Task 1
      2. Compare each response against .sisyphus/evidence/task-1-baseline/ files
      3. Verify same HTTP status codes, same JSON field structure
    Expected Result: API behavior identical to baseline (excluding timestamps/UUIDs)
    Failure Indicators: Different status codes, missing fields, changed structure
    Evidence: .sisyphus/evidence/task-14-regression-report.txt
  ```

  **Commit**: NO (verification only)

- [x] 15. Create core/ directory + go.mod + skeleton

  **What to do**:
  - Create `core/` directory at repo root
  - Create `core/go.mod` with `module github.com/copcon/core`
  - Set Go version to 1.26 (matching server)
  - Add required dependencies: openai-go, uuid, ringbuf, gorm (for providers), qdrant (for providers), testify, x/sync, yaml
  - Create placeholder directories: `core/entity/`, `core/iface/`, `core/tool/`, `core/llm/`, `core/hook/`, `core/agent/`, `core/context_builder/`, `core/storage/`, `core/capabilities/`, `core/chatcontext/`, `core/providers/`, `core/testutil/`
  - Create `go.work` at repo root: `use ./core ./server`

  **Must NOT do**:
  - Do not move any code yet
  - Do not modify server/

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Directory + file creation, no logic
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 14 — Phase 1 must be complete)
  - **Parallel Group**: Wave 2 (first task)
  - **Blocks**: Tasks 16-22
  - **Blocked By**: Task 14

  **References**:
  **Pattern References**:
  - `server/go.mod` — Current module definition (for dependency versions)

  **Acceptance Criteria**:
  - [ ] `core/go.mod` exists with `module github.com/copcon/core`
  - [ ] `go.work` exists with `use ./core ./server`
  - [ ] `cd core && go build ./...` compiles (empty packages OK)

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: core module compiles
    Tool: Bash (go build)
    Preconditions: core/ directory created
    Steps:
      1. cd core && go build ./...
      2. Verify exit code 0
    Expected Result: Empty module compiles
    Failure Indicators: go.mod errors, missing directories
    Evidence: .sisyphus/evidence/task-15-core-skeleton.txt
  ```

  **Commit**: YES
  - Message: `refactor(arch): create core/ module skeleton + go.work`
  - Files: `core/go.mod`, `go.work`

- [x] 16. Migrate entity/ + iface/ + chatcontext/ to core/

  **What to do**:
  - Copy `server/internal/domain/entity/*.go` → `core/entity/`
  - Copy ChatContextInterface + DTOs from `server/internal/domain/iface/chat.go` → `core/iface/chat.go`
  - Copy ChatContext impl from `server/internal/chatcontext/` → `core/chatcontext/`
  - Update import paths: `github.com/copcon/server/internal/domain/entity` → `github.com/copcon/core/entity`
  - Update import paths: `github.com/copcon/server/internal/domain/iface` → `github.com/copcon/core/iface`
  - Update all files that import these packages (use ast_grep_replace for bulk update)
  - Delete originals from server/ after verifying all imports updated
  - `go test ./...` in both modules

  **Must NOT do**:
  - Do not change any logic
  - Do not modify public API

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Many files to move + import path updates across both modules
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 17, 18, 19, 20, 21, 22)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 23
  - **Blocked By**: Tasks 10, 15

  **References**:
  **Pattern References**:
  - `server/internal/domain/entity/` — Source files
  - `server/internal/domain/iface/chat.go` — Source (interface only, after Task 10 split)
  - `server/internal/chatcontext/` — Source (ChatContext impl, after Task 10 split)

  **Acceptance Criteria**:
  - [ ] `core/entity/event.go` exists and compiles
  - [ ] `core/iface/chat.go` exists with ChatContextInterface only
  - [ ] `core/chatcontext/chat_context.go` exists with ChatContext impl
  - [ ] `grep "server/internal/domain" core/ --include="*.go" -r` returns nothing
  - [ ] `go test ./core/... ./server/... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: core entity/iface/chatcontext compile independently
    Tool: Bash (go build)
    Preconditions: Files migrated
    Steps:
      1. cd core && go build ./entity/... ./iface/... ./chatcontext/...
      2. Verify exit code 0
    Expected Result: All migrated packages compile in core
    Failure Indicators: Import path errors, missing dependencies
    Evidence: .sisyphus/evidence/task-16-core-migration.txt
  ```

  **Commit**: YES
  - Message: `refactor(arch): migrate entity/iface/chatcontext to core/`
  - Files: `core/entity/`, `core/iface/`, `core/chatcontext/`, `server/` (import updates)

- [x] 17. Migrate tool/ + llm/ + hook/ + context_builder/ to core/

  **What to do**:
  - Copy `server/internal/tool/*.go` → `core/tool/` (excluding test files that depend on server internals)
  - Copy `server/internal/llm/*.go` → `core/llm/`
  - Copy `server/internal/hook/*.go` → `core/hook/`
  - Copy `server/internal/context_builder/*.go` → `core/context_builder/`
  - Update all import paths
  - Move/adapt test files
  - Delete originals from server/

  **Must NOT do**:
  - Do not change any logic

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 16, 18, 19, 20, 21, 22)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 23
  - **Blocked By**: Task 15

  **Acceptance Criteria**:
  - [ ] `core/tool/`, `core/llm/`, `core/hook/`, `core/context_builder/` all compile
  - [ ] `go test ./core/... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Migrated packages compile in core
    Tool: Bash (go build)
    Preconditions: Files migrated
    Steps:
      1. cd core && go build ./tool/... ./llm/... ./hook/... ./context_builder/...
      2. Verify exit code 0
    Expected Result: All packages compile
    Failure Indicators: Import errors, missing deps
    Evidence: .sisyphus/evidence/task-17-core-packages.txt
  ```

  **Commit**: YES
  - Message: `refactor(arch): migrate tool/llm/hook/context_builder to core/`
  - Files: `core/tool/`, `core/llm/`, `core/hook/`, `core/context_builder/`

- [x] 18. Migrate agent/ + storage/ to core/

  **What to do**:
  - Copy `server/internal/agent/*.go` → `core/agent/`
  - Copy `server/internal/storage/*.go` → `core/storage/`
  - Update agent/ to use `storage.SessionStore`, `storage.MessageStore` instead of `session.SessionManager`, `chat_context.ContextManager`
  - Update import paths
  - Move/adapt test files
  - Delete originals from server/

  **Must NOT do**:
  - Do not change agent behavior

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 16, 17, 19, 20, 21, 22)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 23
  - **Blocked By**: Tasks 11, 15

  **Acceptance Criteria**:
  - [ ] `core/agent/` compiles with storage interfaces
  - [ ] `core/storage/` compiles with no GORM imports
  - [ ] `go test ./core/agent/... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Agent engine works with storage interfaces
    Tool: Bash (go test)
    Preconditions: Agent migrated to core
    Steps:
      1. cd core && go test ./agent/... -v -count=1
      2. Verify all tests pass
    Expected Result: Agent tests pass in core module
    Failure Indicators: Test failures, missing mocks
    Evidence: .sisyphus/evidence/task-18-agent-migration.txt
  ```

  **Commit**: YES
  - Message: `refactor(arch): migrate agent/ + storage/ to core/`
  - Files: `core/agent/`, `core/storage/`

- [x] 19. Migrate tools/ → core/capabilities/tools/

  **What to do**:
  - Copy `server/internal/tools/code_executor.go` → `core/capabilities/tools/code_executor.go`
  - Copy `server/internal/tools/shell_executor.go` → `core/capabilities/tools/shell_executor.go`
  - Copy `server/internal/tools/file_ops.go` → `core/capabilities/tools/file_ops.go`
  - Copy `server/internal/tools/todo_tool.go` → `core/capabilities/tools/todo.go`
  - Copy `server/internal/tools/delegate.go` → `core/capabilities/tools/delegate.go`
  - Copy `server/internal/tools/async_tools.go` → `core/capabilities/tools/async.go`
  - Copy `server/internal/tools/hitl_tools.go` → `core/capabilities/tools/hitl.go`
  - Update import paths in each file
  - Do NOT add init() registration yet (that's Task 26)

  **Must NOT do**:
  - Do not add init() functions yet
  - Do not change tool behavior

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 16-18, 20-22)
  - **Parallel Group**: Wave 2
  - **Blocks**: Tasks 23, 26
  - **Blocked By**: Task 15

  **Acceptance Criteria**:
  - [ ] All 7 tool files exist in `core/capabilities/tools/`
  - [ ] `cd core && go build ./capabilities/tools/...` compiles

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Tool files compile in core
    Tool: Bash (go build)
    Preconditions: Files migrated
    Steps:
      1. cd core && go build ./capabilities/tools/...
      2. Verify exit code 0
    Expected Result: All tools compile in core
    Failure Indicators: Import errors
    Evidence: .sisyphus/evidence/task-19-tools-migration.txt
  ```

  **Commit**: YES
  - Message: `refactor(arch): migrate tool implementations to core/capabilities/tools/`
  - Files: `core/capabilities/tools/`

- [x] 20. Migrate plugins/ → core/capabilities/hooks/

  **What to do**:
  - Copy `server/internal/plugins/todo_hook.go` + `server/internal/plugins/todo_format.go` → `core/capabilities/hooks/todo_injection.go`
  - Copy `server/internal/plugins/logging/logging_plugin.go` → `core/capabilities/hooks/logging.go`
  - Copy `server/internal/plugins/memory/memory_plugin.go` → `core/capabilities/hooks/memory.go`
  - Copy `server/internal/plugins/tracing/tracing_plugin.go` → `core/capabilities/hooks/tracing.go`
  - Update import paths
  - Do NOT add init() registration yet (that's Task 27)

  **Must NOT do**:
  - Do not add init() functions yet
  - Do not change hook behavior

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 16-19, 21, 22)
  - **Parallel Group**: Wave 2
  - **Blocks**: Tasks 23, 27
  - **Blocked By**: Task 15

  **Acceptance Criteria**:
  - [ ] All 4 hook files exist in `core/capabilities/hooks/`
  - [ ] `cd core && go build ./capabilities/hooks/...` compiles

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Hook files compile in core
    Tool: Bash (go build)
    Preconditions: Files migrated
    Steps:
      1. cd core && go build ./capabilities/hooks/...
      2. Verify exit code 0
    Expected Result: All hooks compile in core
    Failure Indicators: Import errors
    Evidence: .sisyphus/evidence/task-20-hooks-migration.txt
  ```

  **Commit**: YES
  - Message: `refactor(arch): migrate hook implementations to core/capabilities/hooks/`
  - Files: `core/capabilities/hooks/`

- [x] 21. Migrate GORM models → core/providers/postgres/

  **What to do**:
  - Create `core/providers/postgres/go.mod` (or keep as subdirectory of core)
  - Create `core/providers/postgres/models.go` with GORM-annotated structs (Session, Message, Todo)
  - Create `core/providers/postgres/session.go` implementing `storage.SessionStore` using GORM
  - Create `core/providers/postgres/message.go` implementing `storage.MessageStore` using GORM
  - Create `core/providers/postgres/todo.go` implementing `storage.TodoStore` using GORM
  - Add conversion functions: model ↔ domain
  - Add `AutoMigrate()` method
  - Create `core/providers/postgres/store.go` with `NewStore(db *gorm.DB) *Store` convenience constructor

  **Must NOT do**:
  - Do not change GORM model definitions (preserve existing field names, tags, indexes)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 16-20, 22)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 23
  - **Blocked By**: Tasks 4, 15

  **Acceptance Criteria**:
  - [ ] `core/providers/postgres/session.go` implements `storage.SessionStore`
  - [ ] `core/providers/postgres/message.go` implements `storage.MessageStore`
  - [ ] `core/providers/postgres/todo.go` implements `storage.TodoStore`
  - [ ] `cd core && go build ./providers/postgres/...` compiles

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Postgres provider compiles and implements interfaces
    Tool: Bash (go build + go test)
    Preconditions: Provider code written
    Steps:
      1. cd core && go build ./providers/postgres/...
      2. Verify compile-time interface checks pass
    Expected Result: All storage interfaces implemented
    Failure Indicators: Missing methods, wrong signatures
    Evidence: .sisyphus/evidence/task-21-postgres-provider.txt
  ```

  **Commit**: YES
  - Message: `refactor(arch): create postgres provider with GORM storage implementations`
  - Files: `core/providers/postgres/`

- [x] 22. Migrate Qdrant → core/providers/qdrant/

  **What to do**:
  - Create `core/providers/qdrant/memory.go` implementing `storage.MemoryStore`
  - Move logic from `server/internal/memory/manager.go`
  - Add conversion: `storage.Memory` ↔ internal Qdrant point representation

  **Must NOT do**:
  - Do not change Qdrant query behavior

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 16-21)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 23
  - **Blocked By**: Tasks 4, 15

  **Acceptance Criteria**:
  - [ ] `core/providers/qdrant/memory.go` implements `storage.MemoryStore`
  - [ ] `cd core && go build ./providers/qdrant/...` compiles

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Qdrant provider compiles
    Tool: Bash (go build)
    Preconditions: Provider code written
    Steps:
      1. cd core && go build ./providers/qdrant/...
      2. Verify exit code 0
    Expected Result: Qdrant provider compiles
    Failure Indicators: Import errors
    Evidence: .sisyphus/evidence/task-22-qdrant-provider.txt
  ```

  **Commit**: YES
  - Message: `refactor(arch): create qdrant provider with MemoryStore implementation`
  - Files: `core/providers/qdrant/`

- [x] 23. Create go.work + update all import paths

  **What to do**:
  - Ensure `go.work` exists at repo root with `use ./core ./server`
  - Use `ast_grep_replace` (dry-run first) to find all remaining `github.com/copcon/server/internal/` imports in server/
  - Update server/ import paths to point to `github.com/copcon/core/` for migrated packages
  - Update server/ imports for packages that stayed (api/, config/)
  - Run `go mod tidy` in both core/ and server/
  - Verify both modules compile in workspace mode

  **Must NOT do**:
  - Do not change any logic

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Bulk import path changes across many files, needs careful verification
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on ALL Wave 2 migration tasks)
  - **Parallel Group**: Wave 2 (final gate)
  - **Blocks**: Task 24
  - **Blocked By**: Tasks 15-22

  **References**:
  **Pattern References**:
  - `server/go.mod` — Current module path
  - `core/go.mod` — New module path

  **Acceptance Criteria**:
  - [ ] `go work sync` runs without errors
  - [ ] `go test ./core/... ./server/... -count=1` passes in workspace mode
  - [ ] No remaining `github.com/copcon/server/internal/domain` imports in server/

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Both modules compile in workspace mode
    Tool: Bash (go build)
    Preconditions: Import paths updated
    Steps:
      1. go build ./core/... ./server/...
      2. Verify exit code 0
    Expected Result: Full workspace compiles
    Failure Indicators: Import resolution errors
    Evidence: .sisyphus/evidence/task-23-workspace-build.txt
  ```

  **Commit**: YES
  - Message: `refactor(arch): update all import paths for core/ module split`
  - Files: `go.work`, `server/**/*.go`, `core/**/*.go`

- [x] 24. Verify standalone builds — GOWORK=off

  **What to do**:
  - Run `cd core && GOWORK=off go build ./...` — must compile without server/
  - Run `cd core && GOWORK=off go test ./... -count=1` — must pass without server/
  - Run `cd core && GOWORK=off go vet ./...` — must pass
  - Run `cd server && GOWORK=off go build ./...` — must compile (requires core as dependency)
  - Run `cd server && GOWORK=off go test ./... -count=1` — must pass
  - Verify `core/go.mod` does NOT contain `require github.com/copcon/server`
  - Run `go mod tidy` in both modules

  **Must NOT do**:
  - Do not change any code

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Comprehensive verification, may need to fix issues found
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 23)
  - **Parallel Group**: Wave 2 (verification)
  - **Blocks**: Task 25
  - **Blocked By**: Task 23

  **Acceptance Criteria**:
  - [ ] `cd core && GOWORK=off go build ./...` exit 0
  - [ ] `cd core && GOWORK=off go test ./... -count=1` all pass
  - [ ] `cd server && GOWORK=off go build ./...` exit 0
  - [ ] `grep "copcon/server" core/go.mod` returns nothing

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: core builds standalone
    Tool: Bash (go build)
    Preconditions: Module split complete
    Steps:
      1. cd core && GOWORK=off go build ./...
      2. Verify exit code 0
      3. cd core && GOWORK=off go test ./... -count=1
      4. Verify all tests pass
    Expected Result: core is independently buildable and testable
    Failure Indicators: Build errors, test failures
    Evidence: .sisyphus/evidence/task-24-standalone-verification.txt

  Scenario: core has no server dependency
    Tool: Bash (grep)
    Preconditions: go.mod tidy
    Steps:
      1. grep "copcon/server" core/go.mod
      2. Verify exit code 1 (not found)
    Expected Result: core/go.mod is free of server references
    Failure Indicators: Server dependency found
    Evidence: .sisyphus/evidence/task-24-no-server-dep.txt
  ```

  **Commit**: NO (verification only)

- [x] 25. Create capability registry + Capability interface

  **What to do**:
  - Create `core/capabilities/registry.go`
  - Define `CapabilityType` enum: "tool", "hook", "skill", "memory"
  - Define `Capability` interface: Name(), Type(), DependsOn()
  - Define `ToolCapability` interface: Capability + NewTool(deps CapabilityDeps) tool.Tool
  - Define `HookCapability` interface: Capability + NewHook(deps CapabilityDeps) hook.Hook
  - Define `CapabilityDeps` struct: SessionStore, MessageStore, TodoStore, MemoryStore, AgentRegistry, Engine, Logger
  - Define global `builtins` map + `Register()`, `Get()`, `ListByType()` functions
  - Define `ResolveDependencies(names []string) ([]Capability, error)` with topological sort
  - Define `ExpandWildcards(names []string) []string` for "tools.*" → all tools
  - Write tests for dependency resolution and wildcard expansion

  **Must NOT do**:
  - Do not implement skills/ or capabilities/memory/ — stubs only
  - Do not register any capabilities yet (that's Tasks 26-27)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Core design work — interface definitions + dependency resolution algorithm
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 24)
  - **Parallel Group**: Wave 3 (first task)
  - **Blocks**: Tasks 26-28
  - **Blocked By**: Task 24

  **References**:
  **Pattern References**:
  - `docs/architecture-refactoring-plan.md` Section 5.4 — Capability registration mechanism design
  - `server/internal/hook/runner.go:50-55` — HookRunner registration pattern (similar)

  **Acceptance Criteria**:
  - [ ] `core/capabilities/registry.go` compiles
  - [ ] `ResolveDependencies()` handles: no deps, single dep, chain deps, wildcard expansion
  - [ ] `cd core && go test ./capabilities/... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Dependency resolution works
    Tool: Bash (go test)
    Preconditions: Registry implemented
    Steps:
      1. cd core && go test ./capabilities/... -v -run TestResolve -count=1
      2. Verify all test cases pass
    Expected Result: Topological sort + wildcard expansion work correctly
    Failure Indicators: Circular dep not detected, wrong order
    Evidence: .sisyphus/evidence/task-25-capability-registry.txt
  ```

  **Commit**: YES
  - Message: `feat(core): add capability registry with dependency resolution`
  - Files: `core/capabilities/registry.go`, `core/capabilities/registry_test.go`

- [x] 26. Add init() self-registration to all tools

  **What to do**:
  - Add `init()` + `ToolCapability` implementation to each tool file in `core/capabilities/tools/`:
    - code_executor.go, shell_executor.go, file_ops.go, todo.go, delegate.go, async.go, hitl.go
  - Each `init()` calls `registry.Register(&xxxCapability{})`
  - Each capability's `NewTool()` creates and returns the tool instance
  - Set `DependsOn()` appropriately (e.g., todo → hooks.todo_injection, delegate → none but needs Engine at runtime)

  **Must NOT do**:
  - Do not change tool execution logic
  - Do not add new tools

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 27, 28, 32)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 29
  - **Blocked By**: Tasks 19, 25

  **Acceptance Criteria**:
  - [ ] All 7 tool files have `init()` functions
  - [ ] `registry.Get("tools.code_executor")` returns a valid ToolCapability
  - [ ] `cd core && go test ./capabilities/tools/... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: All tools self-register
    Tool: Bash (go test)
    Preconditions: init() functions added
    Steps:
      1. cd core && go test ./capabilities/... -v -run TestRegistration -count=1
      2. Verify 7 tool capabilities registered
    Expected Result: All tools discoverable by name
    Failure Indicators: Missing registrations, name mismatches
    Evidence: .sisyphus/evidence/task-26-tool-registration.txt
  ```

  **Commit**: YES
  - Message: `feat(core): add init() self-registration to all tool capabilities`
  - Files: `core/capabilities/tools/*.go`

- [x] 27. Add init() self-registration to all hooks

  **What to do**:
  - Add `init()` + `HookCapability` implementation to each hook file in `core/capabilities/hooks/`:
    - todo_injection.go, memory.go, logging.go, tracing.go
  - Each `init()` calls `registry.Register(&xxxHookCapability{})`
  - Each capability's `NewHook()` creates and returns the hook instance

  **Must NOT do**:
  - Do not change hook execution logic
  - Do not add new hooks

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 26, 28, 32)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 29
  - **Blocked By**: Tasks 20, 25

  **Acceptance Criteria**:
  - [ ] All 4 hook files have `init()` functions
  - [ ] `registry.Get("hooks.logging")` returns a valid HookCapability
  - [ ] `cd core && go test ./capabilities/hooks/... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: All hooks self-register
    Tool: Bash (go test)
    Preconditions: init() functions added
    Steps:
      1. cd core && go test ./capabilities/... -v -run TestHookRegistration -count=1
      2. Verify 4 hook capabilities registered
    Expected Result: All hooks discoverable by name
    Failure Indicators: Missing registrations
    Evidence: .sisyphus/evidence/task-27-hook-registration.txt
  ```

  **Commit**: YES
  - Message: `feat(core): add init() self-registration to all hook capabilities`
  - Files: `core/capabilities/hooks/*.go`

- [x] 28. Implement dependency resolution + wildcard expansion

  **What to do**:
  - Implement `ExpandWildcards(names []string) []string`:
    - "tools.*" → all registered tool capabilities
    - "hooks.*" → all registered hook capabilities
    - "*" → all registered capabilities
  - Implement `ResolveDependencies(names []string) ([]Capability, error)`:
    - Expand wildcards first
    - Build dependency graph from `DependsOn()` returns
    - Topological sort (Kahn's algorithm)
    - Detect circular dependencies → return error
    - Return capabilities in dependency order
  - Write comprehensive tests: wildcards, deps, circular detection, empty input

  **Must NOT do**:
  - Do not implement skill/memory capabilities

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Algorithm implementation with edge cases
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 26, 27, 32)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 29
  - **Blocked By**: Task 25

  **Acceptance Criteria**:
  - [ ] `ExpandWildcards("tools.*")` returns all 7 tool names
  - [ ] `ResolveDependencies(["tools.todo"])` includes "hooks.todo_injection" (auto-added dep)
  - [ ] Circular dependency returns error
  - [ ] `cd core && go test ./capabilities/... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Wildcard expansion and dependency resolution
    Tool: Bash (go test)
    Preconditions: Implementation complete
    Steps:
      1. cd core && go test ./capabilities/... -v -run TestResolve -count=1
      2. Verify all test cases pass
    Expected Result: Correct expansion + topological ordering
    Failure Indicators: Missing deps, wrong order, circular not detected
    Evidence: .sisyphus/evidence/task-28-dependency-resolution.txt
  ```

  **Commit**: YES
  - Message: `feat(core): implement wildcard expansion + dependency resolution for capabilities`
  - Files: `core/capabilities/registry.go`, `core/capabilities/registry_test.go`

- [x] 29. Implement Harness — NewAgent() + NewHarness() + Build()

  **What to do**:
  - Create `core/harness.go`
  - Implement `AgentQuickConfig` + `NewAgent()`:
    - Creates a HarnessConfig with sensible defaults
    - Calls NewHarness().Build() internally
    - Returns Engine directly
  - Implement `HarnessConfig` + `StoreConfig` + `AgentSpec` + `AgentFactorySpec`
  - Implement `NewHarness()` — creates Harness struct with config
  - Implement `Harness.Build()` — the full construction sequence:
    1. Initialize storage from StoreConfig
    2. Resolve capabilities (expand wildcards + dependency sort)
    3. Create global ToolRegistry + register built-in + custom tools
    4. Create global HookRunner + register built-in + custom hooks
    5. Create AgentRegistry + register agents (AgentSpec → factory, AgentFactorySpec → direct)
    6. Create AgentEngine with all dependencies
    7. Register cross-agent tools (delegate_to, read_sub_session) — needs Engine reference
    8. Store engine + registry references
  - Implement `Harness.Engine()` + `Harness.Registry()`

  **Must NOT do**:
  - Do not add YAML/env config loading to HarnessConfig
  - Do not implement skill capabilities

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Central design piece, complex wiring logic
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 25-28)
  - **Parallel Group**: Wave 3
  - **Blocks**: Tasks 30, 31
  - **Blocked By**: Tasks 25, 26, 27, 28

  **References**:
  **Pattern References**:
  - `docs/architecture-refactoring-plan.md` Section 5.1-5.5 — Full Harness design
  - `server/cmd/server/main.go:34-224` — Current wiring logic (the 200+ lines Harness replaces)

  **Acceptance Criteria**:
  - [ ] `core/harness.go` compiles
  - [ ] `NewAgent(AgentQuickConfig{...})` returns a working AgentEngine
  - [ ] `NewHarness(HarnessConfig{...}).Build()` returns a working Harness
  - [ ] `cd core && go test ./... -run TestHarness -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: NewAgent creates working engine
    Tool: Bash (go test)
    Preconditions: Harness implemented
    Steps:
      1. cd core && go test ./... -v -run TestNewAgent -count=1
      2. Verify engine.Chat() works with mock LLM + mock store
    Expected Result: Single-agent harness builds and chats
    Failure Indicators: Build error, Chat panic
    Evidence: .sisyphus/evidence/task-29-new-agent.txt

  Scenario: NewHarness with multi-agent builds
    Tool: Bash (go test)
    Preconditions: Harness implemented
    Steps:
      1. cd core && go test ./... -v -run TestNewHarness -count=1
      2. Verify multiple agents registered, delegate_to available
    Expected Result: Multi-agent harness builds with delegation
    Failure Indicators: Agent not found, delegate_to not registered
    Evidence: .sisyphus/evidence/task-29-new-harness.txt
  ```

  **Commit**: YES
  - Message: `feat(core): implement AgentHarness — NewAgent + NewHarness + Build`
  - Files: `core/harness.go`, `core/harness_test.go`

- [x] 30. Implement AgentSpec → AgentFactory auto-conversion

  **What to do**:
  - Implement `buildDefaultFactory(spec AgentSpec)` method on Harness
  - Factory function:
    - Resolves tool names from capability registry + custom tools
    - Resolves hook names from capability registry + custom hooks
    - Injects Task/ParentContext into system prompt (matching current delegate_to behavior)
    - Handles ModelOverride from CreateParams
    - Returns AgentDefinition with all dependencies wired
  - Write tests: static spec → factory → AgentDefinition with correct tools/hooks/prompt

  **Must NOT do**:
  - Do not change existing AgentFactory behavior

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 31)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 33
  - **Blocked By**: Task 29

  **References**:
  **Pattern References**:
  - `docs/architecture-refactoring-plan.md` Section 5.3 — AgentSpec → AgentFactory conversion logic
  - `server/cmd/server/main.go:122-193` — Current factory registration (the logic to replicate)

  **Acceptance Criteria**:
  - [ ] `buildDefaultFactory` creates a valid `AgentFactory`
  - [ ] Factory with `CreateParams{Task: "deploy"}` produces system prompt with "Current Task: deploy"
  - [ ] `cd core && go test ./... -run TestBuildDefaultFactory -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: AgentSpec auto-factory creates correct AgentDefinition
    Tool: Bash (go test)
    Preconditions: Implementation complete
    Steps:
      1. cd core && go test ./... -v -run TestBuildDefaultFactory -count=1
      2. Verify tool resolution, hook resolution, prompt injection
    Expected Result: Factory produces AgentDefinition matching spec
    Failure Indicators: Wrong tools, missing hooks, no task injection
    Evidence: .sisyphus/evidence/task-30-auto-factory.txt
  ```

  **Commit**: YES
  - Message: `feat(core): implement AgentSpec → AgentFactory auto-conversion`
  - Files: `core/harness.go`, `core/harness_test.go`

- [x] 31. Implement custom tools/hooks merge + delegate_to late registration

  **What to do**:
  - In `Harness.Build()`, implement custom tool merge logic:
    - Custom tools registered alongside built-in tools
    - Same name: custom overrides built-in
  - Implement custom hook merge logic (same pattern)
  - Implement delegate_to late registration:
    - After Engine creation, create DelegateToTool with Engine reference
    - Register in ToolRegistry
    - Re-build ToolManager for agents that include "delegate_to" in their tool list
  - Implement read_sub_session late registration (same pattern)

  **Must NOT do**:
  - Do not change delegate_to behavior
  - Do not add new cross-agent tools

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Circular dependency resolution + merge logic with edge cases
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 30)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 33
  - **Blocked By**: Task 29

  **References**:
  **Pattern References**:
  - `server/cmd/server/main.go:197-209` — Current late registration of delegate_to + read_sub_session
  - `docs/architecture-refactoring-plan.md` Section 5.5 — Build step 7

  **Acceptance Criteria**:
  - [ ] Custom tool with same name as built-in overrides it
  - [ ] delegate_to tool registered after Engine creation
  - [ ] Multi-agent with delegate_to works in test
  - [ ] `cd core && go test ./... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Custom tool overrides built-in
    Tool: Bash (go test)
    Preconditions: Merge logic implemented
    Steps:
      1. cd core && go test ./... -v -run TestCustomToolMerge -count=1
      2. Verify custom tool replaces built-in with same name
    Expected Result: Custom tool takes precedence
    Failure Indicators: Built-in tool still used
    Evidence: .sisyphus/evidence/task-31-custom-merge.txt

  Scenario: delegate_to works after late registration
    Tool: Bash (go test)
    Preconditions: Late registration implemented
    Steps:
      1. cd core && go test ./... -v -run TestDelegateLateReg -count=1
      2. Verify agent can delegate to another agent
    Expected Result: Multi-agent delegation works
    Failure Indicators: delegate_to not found, delegation fails
    Evidence: .sisyphus/evidence/task-31-delegate-late.txt
  ```

  **Commit**: YES
  - Message: `feat(core): implement custom tool/hook merge + delegate_to late registration`
  - Files: `core/harness.go`, `core/harness_test.go`

- [x] 32. Create core/testutil/ as public package

  **What to do**:
  - Create `core/testutil/` (NOT under internal/ — must be importable by server/ tests)
  - Move `MockChatContext` and test helpers from `server/internal/testutil/` to `core/testutil/`
  - Update server/ test files to import from `github.com/copcon/core/testutil`
  - Add any new mock types needed: MockSessionStore, MockMessageStore, MockLLMProvider (if not already in llm/)

  **Must NOT do**:
  - Do not put testutil under `core/internal/` (would break server/ test imports)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: File move + import updates
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 26-28, 30-31)
  - **Parallel Group**: Wave 3
  - **Blocks**: None
  - **Blocked By**: Task 24

  **Acceptance Criteria**:
  - [ ] `core/testutil/` is a public package (not under internal/)
  - [ ] `server/` test files can import `github.com/copcon/core/testutil`
  - [ ] `cd core && go test ./testutil/... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: testutil importable from server
    Tool: Bash (go build)
    Preconditions: testutil moved to core
    Steps:
      1. cd server && go build ./internal/api/... (uses testutil in tests)
      2. Verify exit code 0
    Expected Result: Server tests can use core test utilities
    Failure Indicators: Import resolution failure
    Evidence: .sisyphus/evidence/task-32-testutil.txt
  ```

  **Commit**: YES
  - Message: `refactor(core): create public testutil package for cross-module test sharing`
  - Files: `core/testutil/`, `server/` (import updates)

- [x] 33. Rewrite server/main.go using Harness

  **What to do**:
  - Replace the 200+ line manual wiring with `core.NewHarness(cfg).Build()`
  - Build HarnessConfig from config.yaml values
  - Target: ≤ 60 lines in main.go
  - Keep: config loading, GORM DB init, Gin engine setup, route registration
  - Remove: all manual tool registration, hook registration, agent factory creation, engine construction

  **Must NOT do**:
  - Do not change HTTP endpoints
  - Do not remove config.yaml support

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 29-31)
  - **Parallel Group**: Wave 4
  - **Blocks**: Task 37
  - **Blocked By**: Tasks 29, 30, 31

  **References**:
  **Pattern References**:
  - `server/cmd/server/main.go` — Current 228-line main (to replace)
  - `docs/architecture-refactoring-plan.md` Appendix 7.2 — Target main.go example

  **Acceptance Criteria**:
  - [ ] `wc -l server/cmd/server/main.go` ≤ 60
  - [ ] Server starts and responds to `/health`
  - [ ] `go test ./server/... -count=1` passes

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Server starts with Harness
    Tool: Bash (go run + curl)
    Preconditions: main.go rewritten
    Steps:
      1. cd server && go run ./cmd/server &
      2. sleep 2
      3. curl -sf http://localhost:8088/health
      4. Verify HTTP 200
    Expected Result: Server starts successfully
    Failure Indicators: Startup error, health check fails
    Evidence: .sisyphus/evidence/task-33-harness-main.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): rewrite main.go using AgentHarness`
  - Files: `server/cmd/server/main.go`

- [x] 34. Rewrite config → HarnessConfig mapping

  **What to do**:
  - Create `server/internal/wiring/config.go` that maps `config.Config` → `core.HarnessConfig`
  - Map: cfg.OpenAI → llm.NewOpenAIAdapter
  - Map: cfg.Database → postgres.NewStore(db)
  - Map: cfg.Agents → []core.AgentSpec
  - Map: cfg.DefaultAgentID → HarnessConfig.DefaultAgent
  - Keep config.yaml format unchanged for backward compatibility

  **Must NOT do**:
  - Do not add YAML loading to HarnessConfig
  - Do not change config.yaml format

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 35, 36)
  - **Parallel Group**: Wave 4
  - **Blocks**: Task 37
  - **Blocked By**: Task 33

  **Acceptance Criteria**:
  - [ ] `server/internal/wiring/config.go` compiles
  - [ ] Mapping produces valid HarnessConfig

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Config mapping produces valid HarnessConfig
    Tool: Bash (go test)
    Preconditions: Mapping implemented
    Steps:
      1. cd server && go test ./internal/wiring/... -v -count=1
      2. Verify HarnessConfig has correct agent specs, LLM, store
    Expected Result: Config correctly mapped to Harness
    Failure Indicators: Missing fields, wrong mapping
    Evidence: .sisyphus/evidence/task-34-config-mapping.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): add config → HarnessConfig mapping`
  - Files: `server/internal/wiring/config.go`

- [x] 35. Rewrite api/ handlers — use Harness.Engine/Registry + storage interfaces

  **What to do**:
  - Update `Handler` struct to hold `agent.AgentEngine` + `agent.AgentRegistry` + `storage.MessageStore` (no SessionManager, no TodoManager)
  - Update `SetupRoutes` to accept Engine + Registry + MessageStore
  - Rewrite `GetMessages` to use `MessageStore.List()` (should already be done in Task 6)
  - Remove any remaining GORM imports from handlers
  - Remove `*config.Config` from Handler — get DefaultAgentID from Registry instead

  **Must NOT do**:
  - Do not change HTTP response format
  - Do not add new endpoints

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Handler rewrite needs to maintain exact API compatibility
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Tasks 34, 36)
  - **Parallel Group**: Wave 4
  - **Blocks**: Task 37
  - **Blocked By**: Task 33

  **Acceptance Criteria**:
  - [ ] `grep "gorm\." server/internal/api/ --include="*.go" -r` returns nothing
  - [ ] `grep "config\." server/internal/api/handlers.go` returns nothing (no config dependency)
  - [ ] All CRUD endpoints work identically

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: All API endpoints work with new handler structure
    Tool: Bash (curl)
    Preconditions: Server running
    Steps:
      1. POST /api/sessions → verify 201
      2. GET /api/sessions → verify 200
      3. GET /api/sessions/{id}/messages → verify 200
      4. DELETE /api/sessions/{id} → verify 204
      5. POST /api/sessions/{id}/chat → verify SSE stream
    Expected Result: All endpoints return same format as baseline
    Failure Indicators: Different status, missing fields
    Evidence: .sisyphus/evidence/task-35-handler-rewrite.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): rewrite API handlers to use Harness + storage interfaces`
  - Files: `server/internal/api/handlers.go`

- [x] 36. CI pipeline — GOWORK=off per module

  **What to do**:
  - Add CI step: `cd core && GOWORK=off go build ./... && GOWORK=off go test ./... -count=1 && GOWORK=off go vet ./...`
  - Add CI step: `cd server && GOWORK=off go build ./... && GOWORK=off go test ./... -count=1`
  - Add CI step: verify `core/go.mod` has no `github.com/copcon/server` dependency
  - Add CI step: `go work sync` in repo root

  **Must NOT do**:
  - Do not change existing CI steps

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: CI configuration, no production code changes
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: Task 24

  **Acceptance Criteria**:
  - [ ] CI runs `GOWORK=off` for both modules
  - [ ] CI verifies no cross-module dependency violations

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: CI GOWORK=off validation works
    Tool: Bash (go build)
    Preconditions: CI steps added
    Steps:
      1. cd core && GOWORK=off go build ./...
      2. cd server && GOWORK=off go build ./...
      3. Verify both exit 0
    Expected Result: Both modules build independently
    Failure Indicators: Build failure in either module
    Evidence: .sisyphus/evidence/task-36-ci-verification.txt
  ```

  **Commit**: YES
  - Message: `chore(ci): add GOWORK=off validation for core and server modules`
  - Files: `.github/workflows/` (or equivalent CI config)

- [x] 37. Final integration test — full API contract equivalence

  **What to do**:
  - Re-run all baseline capture commands from Task 1
  - Compare against `.sisyphus/evidence/task-1-baseline/`
  - Verify SSE event sequence matches
  - Test multi-agent delegation: create session with architect agent, send message, verify delegate_to works
  - Test async tool execution
  - Test SSE reconnection
  - Run `go test ./core/... ./server/... -count=1`
  - Run `go vet ./core/... ./server/...`
  - Document any deviations

  **Must NOT do**:
  - Do not fix bugs — document for separate fix

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Comprehensive end-to-end verification
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 33-35)
  - **Parallel Group**: Wave 4 (final gate)
  - **Blocks**: F1-F4
  - **Blocked By**: Tasks 1, 33, 34, 35

  **Acceptance Criteria**:
  - [ ] All CRUD endpoints return same format as baseline
  - [ ] SSE event types match baseline
  - [ ] Multi-agent delegation works
  - [ ] `go test ./core/... ./server/... -count=1` all pass
  - [ ] `wc -l server/cmd/server/main.go` ≤ 60

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Full API contract equivalence with baseline
    Tool: Bash (curl + diff)
    Preconditions: All refactoring complete
    Steps:
      1. Re-run baseline capture from Task 1
      2. Compare each response against baseline
      3. Verify same HTTP status + JSON structure
    Expected Result: API behavior identical to pre-refactoring baseline
    Failure Indicators: Different responses, missing features
    Evidence: .sisyphus/evidence/task-37-contract-equivalence.txt

  Scenario: Multi-agent delegation works end-to-end
    Tool: Bash (curl)
    Preconditions: Server running with architect + coder agents
    Steps:
      1. Create session with architect agent
      2. Send message that triggers delegate_to
      3. Verify SSE stream shows delegation events
    Expected Result: Agent delegation works through Harness
    Failure Indicators: delegate_to not found, delegation fails
    Evidence: .sisyphus/evidence/task-37-delegation-e2e.txt
  ```

  **Commit**: NO (verification only) (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [x] F1. **Plan Compliance Audit** — `oracle` ✅ APPROVED (Must Have [5/6] | Must NOT Have [4/5])
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, curl endpoint, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high` ✅ APPROVED (Build PASS | Vet PASS core+server)
  Run `go vet ./...` + `go test ./...` in both core/ and server/. Review all changed files for: `as any`/type suppression, empty catches, console.log in prod, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high` ✅ APPROVED
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test cross-task integration: multi-agent delegation, async tool completion, SSE reconnection. Test edge cases: empty session, invalid input, rapid actions. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep` ✅ APPROVED
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **Wave 0**: `fix(security): remove API key from config.yaml, add .gitignore entry` + `test(server): add baseline integration tests`
- **Wave 1**: `refactor(server): decouple storage layer — add storage interfaces` → one commit per task
- **Wave 2**: `refactor(arch): extract core/ module from server/`
- **Wave 3**: `feat(core): add capability registry + harness build system`
- **Wave 4**: `refactor(server): rewrite main.go using Harness`
- **Final**: `chore(ci): add GOWORK=off validation + tag core/v0.1.0`

---

## Success Criteria

### Verification Commands
```bash
cd /data/copcon/core && GOWORK=off go build ./...          # Expected: exit 0
cd /data/copcon/core && GOWORK=off go test ./... -count=1  # Expected: all pass
cd /data/copcon/server && GOWORK=off go build ./...        # Expected: exit 0
cd /data/copcon/server && GOWORK=off go test ./... -count=1 # Expected: all pass
grep -r "gorm\." /data/copcon/server/internal/api/ --include="*.go"  # Expected: zero matches
grep -r "github.com/copcon/server" /data/copcon/core/ --include="*.go" # Expected: zero matches
wc -l /data/copcon/server/cmd/server/main.go               # Expected: ≤ 60
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass in both modules
- [ ] core/ builds standalone (GOWORK=off)
- [ ] HTTP API behavior unchanged (contract test)
