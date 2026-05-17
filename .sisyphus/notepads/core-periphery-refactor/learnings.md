# Task 1: NewTestEngine + memoryMgr removal — Learnings

## Completed: 2026-05-17

### What was done
- Created `server/internal/agent/engine_test_helper_test.go` with:
  - `NewTestEngine(opts ...TestEngineOption)` — constructs AgentEngine with mock defaults
  - `NewTestEngineWithRegistry(asyncRegistry, opts ...TestEngineOption)` — variant with injected async registry
  - Option functions: `WithTestRegistry`, `WithTestSessionMgr`, `WithTestContextMgr`, `WithTestAsyncRegistry`
- Removed `memoryMgr` field from `AgentEngine` struct (engine.go)
- Removed `memoryMgr` parameter from `NewAgentEngine()` constructor
- Removed `"github.com/copcon/server/internal/memory"` import from engine.go (zombie)
- Updated all 6 `NewAgentEngine(memoryMgr)` calls + `memoryMgr := &mockMemoryManager{}` + `assert.NotNil(engine.memoryMgr)` in engine_test.go
- Replaced struct literals in engine_execution_test.go with `NewTestEngine()` / `NewTestEngineWithRegistry()`
- Removed `memoryMgr` from integration_test.go struct and constructor
- Removed `memoryMgr` creation + memory import from cmd/server/main.go
- Cleaned up unused `"golang.org/x/sync/semaphore"` import from engine_execution_test.go

### Key lesson: test helper FILE NAME matters for Go
- `engine_test_helper.go` (non-test file) cannot reference symbols defined in `_test.go` files
- Must be named `engine_test_helper_test.go` to access mock constructors like `newMockAgentRegistry()`
- This is standard Go: `_test.go` files compile only during `go test`, not `go build`

### Key lesson: option pattern for test helpers
- Using `TestEngineOption func(*AgentEngine)` is clean and extensible
- Test files that need custom fields (like `orderedMgr` for contextMgr) can use `NewTestEngine(WithTestContextMgr(orderedMgr))`

---

# Task 2: AgentEngine interface extraction — Learnings

## Completed: 2026-05-17

### What was done
- Defined `AgentEngine` interface in engine.go with single `Chat()` method
- Renamed `AgentEngine` struct → `engineImpl` (unexported)
- Added compile-time check `var _ AgentEngine = (*engineImpl)(nil)`
- Updated `NewAgentEngine()` return type from `*AgentEngine` → `AgentEngine`
- Changed all method receivers in `engine.go` and `engine_tools.go` from `(e *AgentEngine)` → `(e *engineImpl)`
- Updated `engine_test_helper_test.go`: `TestEngineOption func(*engineImpl)`, `NewTestEngine()` returns `*engineImpl`
- Updated `engine_execution_test.go`: `createTestEngine()` returns `*engineImpl`
- Updated `integration_test.go`: harness field `*AgentEngine` → `*engineImpl`, type assertion `engine.(*engineImpl)`
- Updated `api/handlers.go`: Handler field, NewHandler param, SetupRoutes param all `agent.AgentEngine` (interface)
- Updated `engine_test.go`: `TestAgentEngineStateless` type-asserts `engine.(*engineImpl)` for field access
- Fixed pre-existing compilation errors in test files (missing `*slog.Logger` arg to `chat_context.NewContextManager`)

### Key lessons
- When renaming a struct to unexported, ALL files in the same package that reference the type name need updating
- LSP diagnostics can be stale — always verify with actual `go build` / `go test`
- Test compilation errors in ANY test function block ALL tests from running, even filtered ones
- Interface extraction means test files that access struct fields directly need type assertions
- Files to check: engine.go, engine_tools.go, engine_test_helper_test.go, engine_test.go, engine_execution_test.go, integration_test.go, api/handlers.go, cmd/server/main.go

### Verification results
- `go build ./...` — exit 0
- `go vet ./internal/agent/... ./internal/api/...` — clean
- `go test ./internal/agent/... -run "TestAgentEngineStateless|TestResultOrdering|TestSyncExecution"` — all 3 pass