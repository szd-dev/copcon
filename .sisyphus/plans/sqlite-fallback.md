# SQLite Adaptive Fallback — Provider + Auto-Detection

## TL;DR

> **Quick Summary**: Add `core/providers/sqlite/` as an independent storage provider mirroring the postgres pattern, plus auto-detection logic in server startup: PostgreSQL configured → use PG; otherwise → auto-SQLite.
>
> **Deliverables**:
> - New `core/providers/sqlite/` package (7 files: models, store, convert, session, message, todo, tests)
> - Factory function `server/internal/store/factory.go` with auto-detection
> - Extended `DatabaseConfig` with `Type` and `SQLitePath` fields
> - Separate `config.yaml.sqlite.template` for SQLite-only users
> - Updated `init-db/main.go` with SQLite no-op path
> - TDD tests with in-memory SQLite backend
>
> **Estimated Effort**: Medium
> **Parallel Execution**: YES — 4 waves
> **Critical Path**: Task 1 → Task 3 → Task 8 → Task 12 → Task 15 → F1-F4

---

## Context

### Original Request
> 在 server 中，目前强依赖外部的 pg 数据库，不够自适应。如果用户没有指定 store，就自己初始化一个 sqlite 数据库。

### Interview Summary
**Key Discussions**:
- 方案选型：独立 `core/providers/sqlite/` 包 vs 改造现有 postgres provider → **选择独立包**（零风险、职责清晰、可插拔原则一致）
- 自动降级逻辑：`host` 字段为空时自动选 SQLite，显式 `type: sqlite/postgres` 也可强制指定
- SQLite 驱动：`github.com/glebarez/sqlite`（纯 Go，无 CGO，兼容 `CGO_ENABLED=0`）
- 默认路径：`data/copcon.db`
- 集成测试保持 PG only（test helpers 直接使用 `pgstore.Session{}` 类型）
- `init-db` SQLite 路径：no-op + 仅执行 `AutoMigrate()`

**Research Findings**:
- `pgstore.NewStore(db *gorm.DB)` 已经是驱动无关的 — 只依赖 `*gorm.DB` 接口
- PostgreSQL provider 没有任何测试文件 — SQLite provider 将成为第一个有测试的 provider
- SQLite PRAGMA 配置必须在 factory 中设置（`busy_timeout`, `WAL`, `foreign_keys`, `synchronous`）
- `SetMaxOpenConns(1)` 对 SQLite 并发安全至关重要
- `UUIDArray` 需在 SQLite 中序列化为 JSON TEXT（而非 PG 的 `{uuid1,uuid2}` 格式）
- `convert.go`、`session.go`、`message.go`、`todo.go`、`store.go` 可几乎逐字复制自 postgres provider

### Metis Review
**Identified Gaps** (addressed):
- PRAGMA 配置缺失 → 在 factory 中硬编码 DSN pragma 参数
- `SetMaxOpenConns(1)` 缺失 → factory 中自动设置
- `UUIDArray` PG 格式不兼容 → SQLite 版 `UUIDArray` 改为 JSON 序列化
- `init-db` SQL raw SQL 不兼容 → 添加驱动检测，SQLite 走 AutoMigrate 路径
- `.gitignore` 缺失 `*.db` 和 `data/` → 补全
- `foreign_keys` 默认关闭 → factory DSN 中 `_pragma=foreign_keys(1)`

---

## Work Objectives

### Core Objective
实现一个可插拔的 SQLite storage provider，并在 server 启动时自动检测：有 PG 配置 → 用 PG，无 PG 配置 → 自动降级到 SQLite。

### Concrete Deliverables
- `core/providers/sqlite/models.go` — SQLite 安全的 GORM 模型
- `core/providers/sqlite/store.go` — `StoreProvider` 实现
- `core/providers/sqlite/session.go` — `SessionStore` 实现
- `core/providers/sqlite/message.go` — `MessageStore` 实现
- `core/providers/sqlite/todo.go` — `TodoStore` 实现
- `core/providers/sqlite/convert.go` — 模型↔存储类型转换
- `core/providers/sqlite/sqlite_test.go` — TDD 测试（RED→GREEN）
- `server/internal/store/factory.go` — 工厂函数 + PRAGMA 配置
- `server/internal/store/factory_test.go` — 工厂测试
- `server/internal/config/config.go` — 新增 `Type`、`SQLitePath` 字段
- `server/config.yaml.sqlite.template` — SQLite 配置模板
- Updated `server/cmd/server/main.go`、`server/cmd/init-db/main.go`、`.gitignore`、相关 docs

### Definition of Done
- [x] `cd core && go build ./...` 编译通过
- [x] `cd core && go test ./providers/sqlite/... -count=1` 全部 PASS
- [x] `cd server && go build ./...` 编译通过
- [x] `cd server && go test ./internal/store/... -count=1` 全部 PASS
- [x] PG 集成测试不变：`cd server && go test ./internal/... -run "Integration" -v` PASS
- [x] SQLite 配置启动 server → `/health` 返回 200 → 创建 session → 列出 sessions
- [x] SQLite 配置运行 `init-db` → "AutoMigrate complete" → exit 0
- [x] 无 `host` 字段的配置 → 自动创建 `data/copcon.db`

### Must Have
- SQLite provider 实现完整的 `storage.StoreProvider` 接口
- 编译时接口检查：`var _ storage.StoreProvider = (*Store)(nil)`
- 工厂函数自动设置 WAL、busy_timeout、foreign_keys、synchronous PRAGMAs
- 工厂函数自动设置 `SetMaxOpenConns(1)`
- 模糊配置（同时有 host 和 sqlite_path）→ 报错
- `data/` 目录自动创建

### Must NOT Have (Guardrails)
- **DO NOT** 修改 `core/providers/postgres/` 任何文件 — 零风险
- **DO NOT** 修改 `core/storage/` 任何接口 — 契约不变
- **DO NOT** 修改 `server/internal/integration_test.go` — PG 集成测试保持原样
- **DO NOT** 抽取共享模型 — SQLite 有自己完整的 `models.go`
- **DO NOT** 添加 env var overloading — config 文件是唯一配置源
- **DO NOT** 添加 `:memory:` 模式、迁移工具、SQLite 特定查询优化
- **DO NOT** 移除 docker-compose 中已有的数据库 env vars（即使目前未使用）

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES (core/ + server/internal/ 使用 testify)
- **Automated tests**: TDD
- **Framework**: testify (assert + require)
- **Pattern**: RED (failing tests) → GREEN (minimal implementation) → REFACTOR

### QA Policy
Every task MUST include agent-executed QA scenarios (see TODO template below).
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Unit tests**: `go test` — assert/require, in-memory SQLite backend
- **API/Backend**: Bash (curl) — Send requests, assert status + response fields
- **Build verification**: `go build` — Compile check for core + server

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — foundation + RED tests):
├── Task 1: Add SQLite driver dependency [quick]
├── Task 2: Create SQLite-adapted models.go [deep]
├── Task 3: Write RED tests for SQLite provider [deep]
├── Task 4: Extend DatabaseConfig [quick]
├── Task 5: Create config.yaml.sqlite.template [quick]
└── Task 6: Update .gitignore [quick]

Wave 2 (After Wave 1 — GREEN implementation, MAX PARALLEL):
├── Task 7: Implement store.go + convert.go (depends: 2) [deep]
├── Task 8: Implement session.go (depends: 2) [quick]
├── Task 9: Implement message.go (depends: 2) [quick]
├── Task 10: Implement todo.go (depends: 2) [quick]
├── Task 11: Verify tests pass → GREEN + builder check (depends: 3, 7-10) [quick]
└── Task 12: Implement factory function (depends: 4, 7) [deep]

Wave 3 (After Wave 2 — wiring + docs, MAX PARALLEL):
├── Task 13: Write factory tests (depends: 12) [quick]
├── Task 14: Update main.go (depends: 12) [quick]
├── Task 15: Update init-db/main.go (depends: 4) [quick]
└── Task 16: Update documentation (depends: 5) [writing]

Wave 4 (After Wave 3 — final verification):
├── Task 17: Full build verification [quick]
├── Task 18: Integration verification (PG tests still pass) [quick]
└── Task 19: Smoke test with SQLite config [quick]

Final Verification Wave (After ALL tasks — 4 parallel reviews):
├── Task F1: Plan Compliance Audit [oracle]
├── Task F2: Code Quality Review [unspecified-high]
├── Task F3: Real Manual QA [unspecified-high]
└── Task F4: Scope Fidelity Check [deep]

Critical Path: Task 1 → Task 2 → Task 7 → Task 12 → Task 14 → Task 17 → F1-F4
Parallel Speedup: ~55% faster than sequential
Max Concurrent: 6 (Waves 1 & 2)
```

### Dependency Matrix

- **1**: — — 2, 3, 4, 5, 6 — —
- **2**: — — 7, 8, 9, 10 — —
- **3**: — — 11 — —
- **4**: — — 12, 15 — —
- **5**: — — 16 — —
- **6**: — — — — —
- **7, 8, 9, 10**: 2 — — —
- **11**: 3, 7, 8, 9, 10 — — —
- **12**: 4, 7 — 13, 14 — —
- **13**: 12 — — — —
- **14**: 12 — 17, 18 — —
- **15**: 4 — 18 — —
- **16**: 5 — — — —
- **17**: 14 — — — —
- **18**: 11, 14, 15 — — —
- **19**: 17 — — — —

### Agent Dispatch Summary

- **1**: **6** — T1 → `quick`, T2 → `deep`, T3 → `deep`, T4 → `quick`, T5 → `quick`, T6 → `quick`
- **2**: **6** — T7 → `deep`, T8-T10 → `quick`, T11 → `quick`, T12 → `deep`
- **3**: **4** — T13-T14 → `quick`, T15 → `quick`, T16 → `writing`
- **4**: **3** — T17-T18 → `quick`, T19 → `quick`
- **FINAL**: **4** — F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

> Implementation + Test = ONE Task. Never separate.
> EVERY task MUST have: Recommended Agent Profile + Parallelization info + QA Scenarios.
> **A task WITHOUT QA Scenarios is INCOMPLETE. No exceptions.**

- [x] 1. Add SQLite driver dependency

  **What to do**:
  - Add `github.com/glebarez/sqlite` to `server/go.mod` via `go get github.com/glebarez/sqlite@latest`
  - Run `cd server && go mod tidy` to update go.sum
  - Verify: `grep glebarez/sqlite server/go.mod` returns the dependency line
  - **Do NOT add to `core/go.mod`** — core/providers/sqlite/ only uses `*gorm.DB`, not the dialector

  **Must NOT do**:
  - Do not add `gorm.io/driver/sqlite` — glebarez/sqlite is a pure-Go replacement
  - Do not add CGO-enabled alternatives like `mattn/go-sqlite3`

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Single go get command, minimal risk
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2-6)
  - **Blocks**: None directly; enables all SQLite-related tasks
  - **Blocked By**: None

  **Acceptance Criteria**:
  - [x] `grep "github.com/glebarez/sqlite" server/go.mod` returns a versioned dependency
  - [x] `cd server && go mod tidy` exits 0

  **QA Scenarios**:
  ```
  Scenario: Dependency added and resolves
    Tool: Bash
    Steps:
      1. cd server && go get github.com/glebarez/sqlite@latest
      2. go mod tidy
      3. grep "github.com/glebarez/sqlite" go.mod
    Expected Result: grep returns line like "github.com/glebarez/sqlite v1.x.x"
    Failure Indicators: grep empty, go get fails
    Evidence: .sisyphus/evidence/task-1-dependency.txt
  ```

  **Commit**: YES (groups with Task 2)
  - Message: `feat(server): add glebarez/sqlite driver`
  - Files: `server/go.mod`, `server/go.sum`

- [x] 2. Create SQLite-adapted models.go

  **What to do**:
  - Create `core/providers/sqlite/models.go`
  - Define GORM models for `Session`, `Message`, `Todo` with SQLite-safe tags
  - Key adaptations from PG models:
    - `ID` tag: `gorm:"type:char(36);primaryKey"` (no `default:gen_random_uuid()`)
    - `JSONB` fields → `gorm:"serializer:json"` (not `type:jsonb`)
    - `UUIDArray` → rewritten: `Value()` → `json.Marshal`, `Scan()` → `json.Unmarshal`, `GormDataType()` → `"text"`
    - Keep `BeforeCreate` hooks (they already generate UUIDs in Go code)
  - Add `AutoMigrate(db *gorm.DB) error` function identical to PG version
  - Namespace error variables unexported (lowercase) to avoid collision with PG provider

  **Must NOT do**:
  - Do NOT copy-paste PG-specific tags (`type:jsonb`, `type:uuid`, `default:gen_random_uuid()`)
  - Do NOT create shared model base — SQLite gets its own complete file
  - Do NOT add foreign key tags (GORM handles relationships without DB constraints for SQLite)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Complex type adaptation (UUID, JSON serialization, array handling)
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3-6)
  - **Blocks**: Tasks 7-10 (all implementation files depend on models)
  - **Blocked By**: None

  **References**:
  - `core/providers/postgres/models.go` — Struct fields, relationships, BeforeCreate hooks, TableName methods
  - `core/storage/session.go:Session` — Value type field parity
  - `core/storage/message.go:Message` — Value type field parity
  - `core/storage/todo.go:Todo` — Value type field parity
  - `https://gorm.io/docs/serializer.html` — serializer:json usage
  - `https://github.com/glebarez/sqlite` — DSN pragma format reference

  **Acceptance Criteria**:
  - [x] `cd core && go build ./providers/sqlite/...` compiles
  - [x] `AutoMigrate` function compiles and links correctly

  **QA Scenarios**:
  ```
  Scenario: Models compile with SQLite-safe tags
    Tool: Bash
    Steps:
      1. cd core && go build ./providers/sqlite/...
    Expected Result: Exit 0, no compilation errors
    Failure Indicators: "undefined", "cannot use", "type mismatch"
    Evidence: .sisyphus/evidence/task-2-build.txt
  ```

  **Commit**: YES (with Task 1)
  - Message: `feat(core): add SQLite-adapted GORM models`
  - Files: `core/providers/sqlite/models.go`

- [x] 3. Write RED tests for SQLite provider

  **What to do**:
  - Create `core/providers/sqlite/sqlite_test.go`
  - Write failing tests (RED phase of TDD) for ALL store methods:
    - **SessionStore**: Create, Get, List, Delete, UpdateTitle, UpdateMetadata, GetMessageCount, AppendMetadata
    - **MessageStore**: List, Add, Update, Upsert, DeleteBySession
    - **TodoStore**: Create, Get, List, UpdateStatus, DeleteBySession
  - Use in-memory SQLite: `gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})`
  - Follow testify pattern: `require` for setup, `assert` for assertions
  - Add compile-time interface checks:
    ```go
    var _ storage.StoreProvider = (*Store)(nil)
    var _ storage.SessionStore = (*SessionStore)(nil)
    var _ storage.MessageStore = (*MessageStore)(nil)
    var _ storage.TodoStore = (*TodoStore)(nil)
    ```

  **Must NOT do**:
  - Do NOT write tests against PG models
  - Do NOT expect tests to pass (RED phase — will fail until Wave 2 implementation)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 20+ test cases across 3 interfaces, TDD pattern, testify usage
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1-2, 4-6)
  - **Blocks**: Task 11 (GREEN verification)
  - **Blocked By**: Task 2 (models.go must exist for test types to reference)

  **References**:
  - `core/providers/postgres/session.go` — Store method behavior reference
  - `core/providers/postgres/message.go` — Message store behavior
  - `core/providers/postgres/todo.go` — Todo store behavior
  - `core/storage/session.go:SessionStore` — Interface contract
  - `core/storage/message.go:MessageStore` — Interface contract
  - `core/storage/todo.go:TodoStore` — Interface contract
  - `core/agent/engine_test.go` — testify pattern reference
  - `server/internal/api/handlers_test.go` — Test setup/teardown pattern

  **Acceptance Criteria** (RED phase):
  - [x] Build fails — test references undefined types (`*Store`, `*SessionStore`, etc.)

  **QA Scenarios**:
  ```
  Scenario: RED tests fail as expected (no implementation)
    Tool: Bash
    Steps:
      1. cd core && go build ./providers/sqlite/... 2>&1
    Expected Result: Build fails — undefined type errors
    Failure Indicators: Build succeeds (tests not comprehensive) or no test file found
    Evidence: .sisyphus/evidence/task-3-red-build-fail.txt
  ```

  **Commit**: YES (groups with Tasks 4-6)
  - Message: `test(core): add RED tests for SQLite provider (TDD)`
  - Files: `core/providers/sqlite/sqlite_test.go`

- [x] 4. Extend DatabaseConfig

  **What to do**:
  - Add two new fields to `DatabaseConfig` in `server/internal/config/config.go`:
    ```go
    Type       string `yaml:"type"`        // "postgres" | "sqlite" | "" (auto-detect)
    SQLitePath string `yaml:"sqlite_path"` // SQLite file path, default "data/copcon.db"
    ```
  - Add `HasPostgresConfig() bool` method: returns `d.Host != ""`
  - Add config validation in `Config.validate()`: if `Host != "" && SQLitePath != "" && Type == ""` → error
  - Ensure backward compatibility: existing fields unchanged

  **Must NOT do**:
  - Do NOT add env var overloading
  - Do NOT change DSN() method
  - Do NOT remove or rename existing fields

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 2 new fields + 1 validation rule in a small file
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1-3, 5, 6)
  - **Blocks**: Tasks 12 (factory), 15 (init-db)
  - **Blocked By**: None

  **References**:
  - `server/internal/config/config.go:33-39` — Current DatabaseConfig struct
  - `server/internal/config/config.go:80-94` — Existing validate() method
  - `server/internal/config/config_test.go` — Existing tests (do NOT break)

  **Acceptance Criteria**:
  - [x] `cd server && go build ./...` compiles
  - [x] `cd server && go test ./internal/config/... -count=1` passes (no regressions)
  - [x] Ambiguous config triggers validation error

  **QA Scenarios**:
  ```
  Scenario: Ambiguous config raises error
    Tool: Bash
    Steps:
      1. cd server && go test ./internal/config/... -run "AmbiguousDatabase" -v -count=1
    Expected Result: Test passes, error contains "ambiguous database config"
    Evidence: .sisyphus/evidence/task-4-ambiguous-test.txt

  Scenario: Existing config tests still pass
    Tool: Bash
    Steps:
      1. cd server && go test ./internal/config/... -count=1
    Expected Result: ALL existing tests PASS
    Failure Indicators: Any existing test fails
    Evidence: .sisyphus/evidence/task-4-backward-compat.txt
  ```

  **Commit**: YES (groups with Tasks 3, 5, 6)
  - Message: `feat(server): extend DatabaseConfig with SQLite support`
  - Files: `server/internal/config/config.go`

- [x] 5. Create config.yaml.sqlite.template

  **What to do**:
  - Create `server/config.yaml.sqlite.template`
  - Full server config template optimized for SQLite
  - Database section:
    ```yaml
    database:
      type: sqlite
      sqlite_path: "data/copcon.db"
      # host/password etc — NOT needed for SQLite
    ```
  - Include relevant openai, qdrant, agents sections from existing template

  **Must NOT do**:
  - Do NOT modify existing `server/config.yaml.template` (PG users stay unaffected)
  - Do NOT include PG-specific database fields (host, port, user, password, dbname)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Create a config template file following existing pattern
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1-4, 6)
  - **Blocks**: Task 16 (docs)
  - **Blocked By**: None

  **References**:
  - `server/config.yaml.template` — Copy structure, replace database section

  **Acceptance Criteria**:
  - [x] File `server/config.yaml.sqlite.template` exists
  - [x] Valid YAML: `yq eval . server/config.yaml.sqlite.template` exits 0

  **QA Scenarios**:
  ```
  Scenario: Template is valid YAML
    Tool: Bash
    Steps:
      1. cat server/config.yaml.sqlite.template | python3 -c "import yaml,sys; yaml.safe_load(sys.stdin)"
    Expected Result: No error, YAML parses successfully
    Evidence: .sisyphus/evidence/task-5-template-parse.txt
  ```

  **Commit**: YES (groups with Tasks 3, 4, 6)
  - Message: `feat(server): add SQLite config template`
  - Files: `server/config.yaml.sqlite.template`

- [x] 6. Update .gitignore

  **What to do**:
  - Add `*.db` to root `.gitignore` — prevents accidental commit of SQLite database files
  - Add `data/` to root `.gitignore` — prevents committing default SQLite data directory

  **Must NOT do**:
  - Do NOT add overly broad patterns (e.g., `*.sqlite` alone is fine)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 2-line addition to existing gitignore
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1-5)
  - **Blocks**: None
  - **Blocked By**: None

  **References**:
  - `.gitignore` (root) — Existing entries to maintain

  **Acceptance Criteria**:
  - [x] `*.db` entry exists in `.gitignore`
  - [x] `data/` entry exists in `.gitignore`

  **QA Scenarios**:
  ```
  Scenario: SQLite artifacts are ignored
    Tool: Bash
    Steps:
      1. mkdir -p data && touch data/copcon.db
      2. git status --short data/
    Expected Result: No output (directory/files are ignored)
    Failure Indicators: git status shows untracked files in data/
    Evidence: .sisyphus/evidence/task-6-gitignore.txt
  ```
  > Clean up: `rm -rf data/` after verification

  **Commit**: YES (groups with Tasks 3-5)
  - Message: `chore: ignore SQLite database artifacts`
  - Files: `.gitignore`

---

## Wave 2

- [x] 7. Implement store.go + convert.go

  **What to do**:
  - Create `core/providers/sqlite/store.go`
  - Mirror `core/providers/postgres/store.go` structure exactly:
    ```go
    type Store struct {
        SessionStore *SessionStore
        MessageStore *MessageStore
        TodoStore    *TodoStore
    }
    func NewStore(db *gorm.DB) *Store {
        AutoMigrate(db)
        return &Store{...}
    }
    func (s *Store) Sessions() storage.SessionStore { ... }
    func (s *Store) Messages() storage.MessageStore { ... }
    func (s *Store) Todos() storage.TodoStore { ... }
    ```
  - Create `core/providers/sqlite/convert.go`
  - Copy `core/providers/postgres/convert.go` verbatim, change only package name to `sqlite`
  - All conversion functions map between SQLite GORM models and `storage.*` value types

  **Must NOT do**:
  - Do NOT change convert function signatures — same as PG provider

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Store wiring with AutoMigrate, interface compliance
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 8-12)
  - **Blocks**: Task 11 (GREEN), Task 12 (factory)
  - **Blocked By**: Task 2 (models.go)

  **References**:
  - `core/providers/postgres/store.go` — Exact structure to mirror
  - `core/providers/postgres/convert.go` — Verbatim copy target
  - `core/storage/provider.go:StoreProvider` — Interface to implement

  **Acceptance Criteria**:
  - [x] `core/providers/sqlite/store.go` compiles
  - [x] `core/providers/sqlite/convert.go` compiles
  - [x] `Store` implements `storage.StoreProvider` (compile-time check via tests)

  **QA Scenarios**:
  ```
  Scenario: Store and convert files compile
    Tool: Bash
    Steps:
      1. cd core && go build ./providers/sqlite/...
    Expected Result: Exit 0, no errors
    Failure Indicators: Compilation errors
    Evidence: .sisyphus/evidence/task-7-build.txt
  ```

  **Commit**: YES (with Tasks 8-10)
  - Message: `feat(core): implement SQLite store and converters`
  - Files: `core/providers/sqlite/store.go`, `core/providers/sqlite/convert.go`

- [x] 8. Implement session.go

  **What to do**:
  - Create `core/providers/sqlite/session.go`
  - Copy `core/providers/postgres/session.go` structure, change only:
    - Package name to `sqlite`
    - Error variable namespace (unexported, lowercase — won't collide)
    - GORM model references (`sqlite.Session` instead of `postgres.Session`)
  - All CRUD methods use exact same GORM operations as PG provider

  **Must NOT do**:
  - Do NOT change query logic — use same GORM patterns as PG provider

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Nearly verbatim copy with package name change
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7, 9-12)
  - **Blocks**: Task 11 (GREEN)
  - **Blocked By**: Task 2 (models.go)

  **References**:
  - `core/providers/postgres/session.go` — Complete implementation to mirror
  - `core/storage/session.go:SessionStore` — Interface contract

  **Acceptance Criteria**:
  - [x] `core/providers/sqlite/session.go` compiles
  - [x] `SessionStore` implements `storage.SessionStore` (compile-time check via tests)

  **QA Scenarios**:
  ```
  Scenario: Session store compiles
    Tool: Bash
    Steps:
      1. cd core && go build ./providers/sqlite/...
    Expected Result: Exit 0
    Evidence: .sisyphus/evidence/task-8-build.txt
  ```

  **Commit**: YES (with Tasks 7, 9, 10)
  - Message: `feat(core): implement SQLite session store`
  - Files: `core/providers/sqlite/session.go`

- [x] 9. Implement message.go

  **What to do**:
  - Create `core/providers/sqlite/message.go`
  - Mirror `core/providers/postgres/message.go` — package name + model references only
  - Methods: List, Add, Update, Upsert, DeleteBySession

  **Must NOT do**:
  - Do NOT change query logic

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Nearly verbatim copy
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7, 8, 10-12)
  - **Blocks**: Task 11 (GREEN)
  - **Blocked By**: Task 2 (models.go)

  **References**:
  - `core/providers/postgres/message.go` — Complete implementation
  - `core/storage/message.go:MessageStore` — Interface contract

  **Acceptance Criteria**:
  - [x] `core/providers/sqlite/message.go` compiles

  **QA Scenarios**:
  ```
  Scenario: Message store compiles
    Tool: Bash
    Steps:
      1. cd core && go build ./providers/sqlite/...
    Expected Result: Exit 0
    Evidence: .sisyphus/evidence/task-9-build.txt
  ```

  **Commit**: YES (with Tasks 7, 8, 10)
  - Message: `feat(core): implement SQLite message store`
  - Files: `core/providers/sqlite/message.go`

- [x] 10. Implement todo.go

  **What to do**:
  - Create `core/providers/sqlite/todo.go`
  - Mirror `core/providers/postgres/todo.go` — package name + model references only
  - Methods: Create, Get, List, UpdateStatus, DeleteBySession

  **Must NOT do**:
  - Do NOT change query logic

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Nearly verbatim copy
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-9, 11, 12)
  - **Blocks**: Task 11 (GREEN)
  - **Blocked By**: Task 2 (models.go)

  **References**:
  - `core/providers/postgres/todo.go` — Complete implementation
  - `core/storage/todo.go:TodoStore` — Interface contract

  **Acceptance Criteria**:
  - [x] `core/providers/sqlite/todo.go` compiles

  **QA Scenarios**:
  ```
  Scenario: Todo store compiles
    Tool: Bash
    Steps:
      1. cd core && go build ./providers/sqlite/...
    Expected Result: Exit 0
    Evidence: .sisyphus/evidence/task-10-build.txt
  ```

  **Commit**: YES (with Tasks 7-9)
  - Message: `feat(core): implement SQLite todo store`
  - Files: `core/providers/sqlite/todo.go`

- [x] 11. Verify tests pass → GREEN + build check

  **What to do**:
  - Run `cd core && go test ./providers/sqlite/... -v -count=1`
  - All RED tests from Task 3 should now pass (GREEN phase)
  - Fix any failing tests — iteration loop until all pass
  - Run `cd core && go build ./...` to verify no import cycles or cross-package issues
  - Run `cd server && go build ./...` to verify server still compiles with new core package

  **Must NOT do**:
  - Do NOT skip any test — all 20+ tests must pass
  - Do NOT modify test assertions to "make them pass" — fix implementations instead

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Test execution + targeted fixes if needed
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (depends on Tasks 7-10 completing)
  - **Blocks**: Tasks 17 (full build), 18 (integration)
  - **Blocked By**: Tasks 3 (tests), 7-10 (implementation)

  **References**:
  - `core/providers/sqlite/sqlite_test.go` — Test file to run
  - `AGENTS.md` — `cd core && go test ./... -count=1` command

  **Acceptance Criteria**:
  - [x] `cd core && go test ./providers/sqlite/... -v -count=1` → ALL PASS
  - [x] `cd core && go build ./...` → Exit 0
  - [x] `cd server && go build ./...` → Exit 0

  **QA Scenarios**:
  ```
  Scenario: All SQLite provider tests pass (GREEN)
    Tool: Bash
    Steps:
      1. cd core && go test ./providers/sqlite/... -v -count=1
    Expected Result: ALL tests PASS, no failures or panics
    Failure Indicators: Any FAIL, panic, or test timeout
    Evidence: .sisyphus/evidence/task-11-green-tests.txt

  Scenario: Full build check — no import cycles
    Tool: Bash
    Steps:
      1. cd core && go build ./...
      2. cd server && go build ./...
    Expected Result: Both exit 0
    Failure Indicators: "import cycle", "undefined", "cannot use"
    Evidence: .sisyphus/evidence/task-11-full-build.txt
  ```

  **Commit**: NO (verification only, no code changes expected; if fixes needed, amend previous commits)

- [x] 12. Implement factory function

  **What to do**:
  - Create `server/internal/store/factory.go`
  - Implement `CreateStoreProvider(cfg config.DatabaseConfig) (storage.StoreProvider, error)`:
    - Detect backend: `cfg.Type == "sqlite" || (cfg.Type == "" && !cfg.HasPostgresConfig())` → SQLite
    - SQLite path: `cfg.SQLitePath` with fallback `"data/copcon.db"`
    - `os.MkdirAll(filepath.Dir(path), 0755)` before `gorm.Open()`
    - SQLite DSN: `fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)`
    - Open DB: `gorm.Open(sqlite.Open(dsn), &gorm.Config{})`
    - Set connection limits:
      ```go
      sqlDB, _ := db.DB()
      sqlDB.SetMaxOpenConns(1)
      sqlDB.SetMaxIdleConns(1)
      ```
    - Return `sqlitestore.NewStore(db)`
    - PG branch: `gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{})` → `pgstore.NewStore(db)`
    - Log backend choice: `slog.Info("using SQLite", "path", path)` or `slog.Info("using PostgreSQL", "host", cfg.Host)`

  **Must NOT do**:
  - Do NOT skip PRAGMA configuration — every SQLite DSN must include all 4 pragmas
  - Do NOT skip `SetMaxOpenConns(1)` — critical for SQLite concurrency
  - Do NOT auto-create parent directories for PG path (only SQLite)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Multi-path factory logic with PRAGMA config, connection pool settings, error handling
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-11)
  - **Blocks**: Tasks 13 (factory tests), 14 (main.go)
  - **Blocked By**: Tasks 4 (config fields), 7 (Store type)

  **References**:
  - `server/cmd/server/main.go:24-26` — Current gorm.Open pattern to replace
  - `server/internal/config/config.go:33-39` — Config struct fields
  - `core/providers/postgres/store.go:17` — `NewStore(db *gorm.DB) *Store` constructor
  - `core/providers/sqlite/store.go` — SQLite NewStore (created in Task 7)
  - `https://github.com/glebarez/sqlite` — DSN pragma format

  **Acceptance Criteria**:
  - [x] `cd server && go build ./...` compiles
  - [x] Factory returns `sqlite.NewStore(db)` when host is empty
  - [x] Factory returns `postgres.NewStore(db)` when host is set
  - [x] SQLite path auto-creates parent directory

  **QA Scenarios**:
  ```
  Scenario: Factory compiles
    Tool: Bash
    Steps:
      1. cd server && go build ./...
    Expected Result: Exit 0
    Evidence: .sisyphus/evidence/task-12-build.txt
  ```

  **Commit**: YES
  - Message: `feat(server): add database store factory with SQLite support`
  - Files: `server/internal/store/factory.go`

---

## Wave 3

- [x] 13. Write factory tests

  **What to do**:
  - Create `server/internal/store/factory_test.go`
  - Test cases:
    - `DatabaseConfig{Host: ""}` → returns SQLite store, file `data/copcon.db` created
    - `DatabaseConfig{Host: "localhost", Port: 5432, ...}` → returns PG store (skip if PG unavailable)
    - `DatabaseConfig{Type: "sqlite"}` → returns SQLite store regardless of host
    - `DatabaseConfig{Type: "postgres", Host: ""}` → returns error (missing required config)
    - `DatabaseConfig{Host: "localhost", SQLitePath: "/tmp/x.db"}` → returns error (ambiguous)
    - `DatabaseConfig{SQLitePath: "/tmp/test_copcon.db"}` → creates file at custom path
  - Use testify: `require` for setup, `assert` for assertions
  - PG tests should `t.Skip()` if PostgreSQL unavailable (following integration test pattern)

  **Must NOT do**:
  - Do NOT skip PG tests silently — use `t.Skipf("PostgreSQL not available: %v", err)`
  - Do NOT leave test database files — cleanup with `os.Remove()` in test cleanup

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 6 test cases following clear factory behavior spec
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 14-16)
  - **Blocks**: None
  - **Blocked By**: Task 12 (factory implementation)

  **References**:
  - `server/internal/config/config_test.go` — Test pattern: temp files, testify
  - `server/internal/integration_test.go` — Skip-if-unavailable pattern

  **Acceptance Criteria**:
  - [x] `cd server && go test ./internal/store/... -v -count=1` → PASS
  - [x] All 6 scenarios covered

  **QA Scenarios**:
  ```
  Scenario: Factory tests pass
    Tool: Bash
    Steps:
      1. cd server && go test ./internal/store/... -v -count=1
    Expected Result: ALL tests PASS (PG tests skipped if unavailable)
    Failure Indicators: Any FAIL
    Evidence: .sisyphus/evidence/task-13-factory-tests.txt
  ```

  **Commit**: YES (with Task 16)
  - Message: `test(server): add database factory tests`
  - Files: `server/internal/store/factory_test.go`

- [x] 14. Update main.go

  **What to do**:
  - In `server/cmd/server/main.go`, replace:
    ```go
    // OLD (lines ~23-26):
    db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
    chk(log, err)
    pg := pgstore.NewStore(db)
    ```
    With:
    ```go
    // NEW:
    storeProvider, err := stor.Factory.CreateStoreProvider(cfg.Database)
    chk(log, err)
    ```
  - Update `StoreConfig{Provider: ...}` to use `storeProvider` instead of `pg`
  - Remove unused imports (`gorm.io/gorm`, `gorm.io/driver/postgres`, `pgstore`) if no longer needed
  - Add import for `server/internal/store` as `stor` (or similar)

  **Must NOT do**:
  - Do NOT change any other part of main.go (LLM adapter, Harness config, agent specs, routes)
  - Do NOT remove `chk()` calls — keep existing error handling pattern

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Replace 3 lines with 2, adjust imports
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 13, 15, 16)
  - **Blocks**: Tasks 17 (full build), 18 (integration)
  - **Blocked By**: Task 12 (factory)

  **References**:
  - `server/cmd/server/main.go:22-34` — Current startup code
  - `server/internal/store/factory.go` — Factory function to call

  **Acceptance Criteria**:
  - [x] `cd server && go build ./...` compiles
  - [x] No unused imports remain

  **QA Scenarios**:
  ```
  Scenario: main.go compiles with factory
    Tool: Bash
    Steps:
      1. cd server && go build ./cmd/server/...
    Expected Result: Exit 0
    Evidence: .sisyphus/evidence/task-14-main-build.txt
  ```

  **Commit**: YES
  - Message: `refactor(server): use store factory in main.go`
  - Files: `server/cmd/server/main.go`

- [x] 15. Update init-db/main.go

  **What to do**:
  - In `server/cmd/init-db/main.go`, detect database type from config
  - PG path: keep existing behavior (`CREATE DATABASE`, run migrations, triggers)
  - SQLite path:
    - Print: `"SQLite: database auto-migrated at <path>"`
    - Call `AutoMigrate(db)` only — no raw SQL
    - Exit 0
  - Load config via `config.Load()` (currently uses hardcoded DSN — add config awareness)

  **Must NOT do**:
  - Do NOT run raw PG SQL on SQLite backend
  - Do NOT fail if config has no PG fields

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Branch on config type, skip PG SQL for SQLite
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 13, 14, 16)
  - **Blocks**: Task 18 (integration verification)
  - **Blocked By**: Task 4 (config fields)

  **References**:
  - `server/cmd/init-db/main.go` — Current init-db logic
  - `server/internal/config/config.go:52-78` — Config.Load() pattern
  - `core/providers/sqlite/models.go:AutoMigrate()` — SQLite AutoMigrate to call

  **Acceptance Criteria**:
  - [x] `cd server && go build ./cmd/init-db/...` compiles
  - [x] SQLite config → prints "AutoMigrate complete" → exit 0

  **QA Scenarios**:
  ```
  Scenario: init-db with SQLite config is no-op
    Tool: Bash
    Steps:
      1. echo 'database: {type: sqlite, sqlite_path: /tmp/test_init.db}' > /tmp/sqlite_config.yaml
      2. cd server && CONFIG_PATH=/tmp/sqlite_config.yaml go run cmd/init-db/main.go
    Expected Result: Prints "SQLite: database auto-migrated" or similar, exits 0
    Failure Indicators: Error, panic, raw SQL failure
    Evidence: .sisyphus/evidence/task-15-initdb-sqlite.txt
  ```
  > Clean up: `rm -f /tmp/test_init.db /tmp/sqlite_config.yaml`

  **Commit**: YES
  - Message: `feat(server): add SQLite support to init-db`
  - Files: `server/cmd/init-db/main.go`

- [x] 16. Update documentation

  **What to do**:
  - Update `docs/backend/04-server-app/database.md`:
    - Change "SQLite is not supported" → document SQLite support
    - Add configuration examples for both PG and SQLite
    - Document auto-detection behavior
    - Document PRAGMA defaults
  - Update `docs/ARCHITECTURE.md`:
    - Add SQLite to infrastructure table
  - Update `docs/backend/07-deployment/docker.md`:
    - Note that SQLite is available without external dependencies
    - Document `config.yaml.sqlite.template`

  **Must NOT do**:
  - Do NOT rewrite entire documentation — minimal targeted updates only
  - Do NOT remove PG documentation

  **Recommended Agent Profile**:
  - **Category**: `writing`
    - Reason: Technical documentation updates
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 13-15)
  - **Blocks**: None
  - **Blocked By**: Task 5 (template created)

  **References**:
  - `docs/backend/04-server-app/database.md` — File to update
  - `docs/ARCHITECTURE.md:447-457` — Infrastructure table
  - `server/config.yaml.sqlite.template` — Reference for docs

  **Acceptance Criteria**:
  - [x] "SQLite is not supported" text replaced with SQLite documentation
  - [x] Architecture infrastructure table includes SQLite

  **QA Scenarios**:
  ```
  Scenario: Docs no longer claim SQLite is unsupported
    Tool: Bash
    Steps:
      1. grep -i "sqlite.*not supported\|unsupported.*sqlite" docs/backend/04-server-app/database.md
    Expected Result: No matches (old claim removed)
    Evidence: .sisyphus/evidence/task-16-docs-no-unsupported.txt
  ```

  **Commit**: YES (with Task 13)
  - Message: `docs: document SQLite database support`
  - Files: `docs/backend/04-server-app/database.md`, `docs/ARCHITECTURE.md`, `docs/backend/07-deployment/docker.md`

---

## Wave 4

- [x] 17. Full build verification

  **What to do**:
  - Run `cd core && go build ./...` — verify core builds clean
  - Run `cd server && go build ./...` — verify server builds clean
  - Run `cd server && go build ./cmd/init-db/...` — verify init-db builds
  - Check for any import cycles: `cd core && go vet ./...`
  - Check for any unused dependencies: `cd server && go mod tidy`

  **Must NOT do**:
  - Do NOT skip vet or tidy — these catch subtle issues

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Build verification commands
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (final verification step)
  - **Parallel Group**: Wave 4 (after Wave 3 complete)
  - **Blocks**: None
  - **Blocked By**: Tasks 14 (main.go), 15 (init-db)

  **Acceptance Criteria**:
  - [x] All `go build` commands exit 0
  - [x] `go vet` passes
  - [x] `go mod tidy` passes

  **QA Scenarios**:
  ```
  Scenario: Full build passes
    Tool: Bash
    Steps:
      1. cd core && go build ./... && go vet ./...
      2. cd ../server && go build ./... && go build ./cmd/init-db/... && go vet ./...
      3. go mod tidy
    Expected Result: All exit 0, no errors
    Evidence: .sisyphus/evidence/task-17-full-build.txt
  ```

  **Commit**: NO (verification only)

- [x] 18. Integration verification — PG tests still pass

  **What to do**:
  - Start PostgreSQL if available: `docker compose up -d postgres`
  - Run `cd server && go test ./internal/... -run "Integration" -v -count=1`
  - Verify all existing integration tests pass (no regressions from config changes)
  - If PG unavailable, skip gracefully: `t.Skip("PostgreSQL not available")`

  **Must NOT do**:
  - Do NOT modify integration tests
  - Do NOT fail the plan if PG unavailable — skip instead

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Run existing test suite
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with Tasks 17, 19)
  - **Blocks**: None
  - **Blocked By**: Tasks 11 (core tests pass), 14 (main.go), 15 (init-db)

  **Acceptance Criteria**:
  - [x] All integration tests PASS (or all skipped if PG unavailable)

  **QA Scenarios**:
  ```
  Scenario: PG integration tests pass
    Tool: Bash
    Steps:
      1. docker compose up -d postgres && sleep 3
      2. cd server && go test ./internal/... -run "Integration" -v -count=1
    Expected Result: PASS (or SKIP if PG unavailable)
    Failure Indicators: Any FAIL
    Evidence: .sisyphus/evidence/task-18-pg-integration.txt
  ```

  **Commit**: NO (verification only)

- [x] 19. Smoke test with SQLite config

  **What to do**:
  - Create test config from SQLite template
  - Start server: `CONFIG_PATH=config.yaml.sqlite go run cmd/server/main.go &`
  - Wait for startup, then:
    - `curl -s http://localhost:PORT/health` → status "ok"
    - `curl -s -X POST .../api/sessions -d '{...}'` → 201 with UUID
    - `curl -s .../api/sessions` → returns list with count ≥ 1
  - Verify `data/copcon.db` file exists and is non-zero
  - Stop server, cleanup test artifacts

  **Must NOT do**:
  - Do NOT keep server running — kill after verification
  - Do NOT commit test database files

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Smoke test with curl commands
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (requires server startup)
  - **Parallel Group**: Wave 4 (last task)
  - **Blocks**: None
  - **Blocked By**: Task 17 (full build passes)

  **Acceptance Criteria**:
  - [x] `/health` returns 200 with `{"status":"ok"}`
  - [x] POST `/api/sessions` returns 201 with valid UUID
  - [x] GET `/api/sessions` returns list with count ≥ 1
  - [x] `data/copcon.db` exists and is non-zero size

  **QA Scenarios**:
  ```
  Scenario: SQLite server roundtrip
    Tool: Bash
    Steps:
      1. cp server/config.yaml.sqlite.template /tmp/smoke_config.yaml
      2. echo "  port: '8099'" >> /tmp/smoke_config.yaml
      3. cd server && CONFIG_PATH=/tmp/smoke_config.yaml go run cmd/server/main.go &
      4. sleep 5
      5. curl -s http://localhost:8099/health
      6. curl -s -X POST http://localhost:8099/api/sessions -H "Content-Type: application/json" -d '{"title":"Smoke Test","default_agent_id":"chat-assistant"}'
      7. curl -s http://localhost:8099/api/sessions
      8. ls -la data/copcon.db
      9. kill %1
    Expected Result: Health=ok, Create=201 with UUID, List shows sessions, DB file exists > 0 bytes
    Failure Indicators: Any non-2xx response, DB file missing or 0 bytes
    Evidence: .sisyphus/evidence/task-19-smoke-test.txt
  ```
  > Clean up: `kill %1; rm -rf data/ /tmp/smoke_config.yaml`

  **Commit**: NO (verification only)


## Commit Strategy

- **Wave 1**: 2 commits
  - `feat(server): add glebarez/sqlite driver` + `feat(core): add SQLite-adapted GORM models` → Tasks 1-2
  - `test(core): add RED tests for SQLite provider (TDD)` + `feat(server): extend DatabaseConfig with SQLite support` + `feat(server): add SQLite config template` + `chore: ignore SQLite database artifacts` → Tasks 3-6
- **Wave 2**: 1 commit
  - `feat(core): implement SQLite provider` → Tasks 7-10
  - `feat(server): add database store factory with SQLite support` → Task 12
- **Wave 3**: 3 commits
  - `test(server): add database factory tests` + `docs: document SQLite database support` → Tasks 13, 16
  - `refactor(server): use store factory in main.go` → Task 14
  - `feat(server): add SQLite support to init-db` → Task 15
- **No commits for verification tasks** (11, 17, 18, 19) unless fixes are needed

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in `.sisyphus/evidence/`. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `cd core && go vet ./...` + `cd server && go vet ./...`. Review all changed files for: empty catches, unused imports, comparing errors with `==` instead of `errors.Is`. Check AI slop: excessive comments, over-abstraction, generic variable names. Verify no `as any` equivalent Go patterns.
  Output: `Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test cross-task integration: SQLite provider → factory → main.go → health endpoint → session CRUD. Test edge cases: empty config, ambiguous config, custom SQLite path, init-db SQLite path. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Success Criteria

### Verification Commands
```bash
# Provider tests
cd core && go test ./providers/sqlite/... -v -count=1     # ALL PASS

# Factory tests
cd server && go test ./internal/store/... -v -count=1      # ALL PASS

# Full build
cd core && go build ./...                                   # Exit 0
cd server && go build ./... && go build ./cmd/init-db/...   # Exit 0

# PG integration (no regressions)
cd server && go test ./internal/... -run "Integration" -v   # ALL PASS

# Smoke test (SQLite)
cp server/config.yaml.sqlite.template /tmp/smoke.yaml
cd server && CONFIG_PATH=/tmp/smoke.yaml go run cmd/server/main.go &
curl http://localhost:PORT/health                           # {"status":"ok"}
```

### Final Checklist
- [x] All "Must Have" present (19 tasks complete)
- [x] All "Must NOT Have" absent (zero PG provider modifications, zero storage interface changes)
- [x] All provider tests pass (20+ test cases)
- [x] All factory tests pass (6 scenarios)
- [x] PG integration tests unchanged
- [x] SQLite server roundtrip works
- [x] init-db SQLite path works
- [x] `data/` and `*.db` in .gitignore