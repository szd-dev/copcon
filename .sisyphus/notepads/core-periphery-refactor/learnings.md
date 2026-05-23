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
---

# Task 16: Migrate entity/ + iface/ + chatcontext/ to core/ — Learnings

## Completed: 2026-05-23

### What was done
- Copied 6 .go files from `server/internal/domain/entity/` to `core/entity/` (event.go, ui_message.go, message_for_llm.go, model_message.go, convert.go, convert_test.go)
- Copied `server/internal/domain/iface/chat.go` to `core/iface/chat.go`
- Copied `server/internal/chatcontext/chat_context.go` and `chat_context_test.go` to `core/chatcontext/`
- Updated import paths in all 3 core/ packages:
  - `github.com/copcon/server/internal/domain/entity` → `github.com/copcon/core/entity`
  - `github.com/copcon/server/internal/domain/iface` → `github.com/copcon/core/iface`
- Updated import paths in all 29+ server/ files referencing the moved packages
- Added `github.com/copcon/core v0.0.0` to server's go.mod require block
- Added `replace github.com/copcon/core => ../core` directive to server's go.mod
- Deleted original files from server/ and removed empty directories

### Key learnings
- `sed -i` with `find -exec` works for bulk import path replacement but can miss some files — always verify with grep afterward
- Some files had import paths that didn't get replaced by sed (likely due to ordering or line-end issues) — manual `edit` tool was needed for 2 files
- `GOPROXY=off go mod tidy` and `GOPROXY=off go build` work for offline verification when network is unavailable
- Core module's go.mod already had the right dependencies (ringbuf, uuid, testify, openai-go) from Task 15 skeleton
- Entity package is self-contained (no server-internal imports), making it the cleanest migration
- Iface imports only entity (plus stdlib), also clean
- Chatcontext imports entity, iface, ringbuf, uuid — all already in core's go.mod

### Verification results
- `core && go build ./entity/... ./iface/... ./chatcontext/...` — SUCCESS
- `server && go build ./...` — SUCCESS  
- `core && go test ./entity/... ./chatcontext/...` — all PASS
- LSP diagnostics: 0 errors, 0 warnings (only pre-existing hints)
