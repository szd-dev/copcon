# AGENTS.md

## Build Commands

```bash
# Core module (standalone)
cd core && go build ./...
cd core && go test ./...

# Server module (depends on core)
cd server && go build ./...
cd server && go test ./...

# Both (from workspace root)
go build ./core/... ./server/...

# DO NOT run from root without ./core/... or ./server/... — root is not a Go module
```

## Test Patterns

```bash
# Integration tests require PostgreSQL
cd server && go test -run "Integration" -v

# Unit tests
cd core && go test ./... -count=1
cd server && go test ./internal/... -count=1

# Specific test
cd core && go test -run "TestNewHarness" -v
```

## Code Style

### Go
- camelCase for private, PascalCase for exported
- Wrap errors: `fmt.Errorf("context: %w", err)`
- Always pass `context.Context` as first parameter
- GORM: use `db.WithContext(ctx)` on every query
- Tests: use `testify/assert` and `testify/require`

### Critical: No Type Suppression
- NEVER use `as any`, `@ts-ignore`, `@ts-expect-error` in TypeScript
- NEVER use type assertions to bypass Go's type system unless absolutely necessary

## Architecture Constraints

### Module Boundaries
```
core/          ← reusable library, NO server imports
server/        ← thin app, imports core/
```

**VIOLATION EXAMPLE (DO NOT):**
```go
// In core/harness.go
import "github.com/copcon/server/internal/config"  // WRONG: core depends on server
```

**CORRECT:**
```go
// In core/ — only use core/* imports
import "github.com/copcon/core/storage"

// In server/ — can import core
import "github.com/copcon/core"
```

### Storage Abstraction
```
core/storage/           ← pure interfaces (SessionStore, MessageStore, etc.)
core/providers/         ← implementations (postgres, qdrant)
server/internal/        ← use core.Provider, do NOT implement storage
```

**DO NOT** create new storage implementations in server/

### Capability System
Built-in tools/hooks live in `core/capabilities/`:
```
core/capabilities/tools/   ← ask_user, code_executor, shell_executor, todo, etc.
core/capabilities/hooks/   ← logging, tracing, memory, todo_injection
```

These auto-register via `init()`. The harness imports them with blank imports:
```go
// In core/harness.go
import (
    _ "github.com/copcon/core/capabilities/hooks"
    _ "github.com/copcon/core/capabilities/tools"
)
```

**DO NOT** manually import these in server/ — they are built-in

### ChatContext Flow
```
handler → chatCtx → engine.Chat(chatCtx, input) → events stream → SSE
```

- Engine runs in goroutine, emits via `chatCtx.Emit()`
- Handler streams via `core/chat.HandleChat()` (framework-agnostic)
- DO NOT create new context types — use `core/chatcontext`

## Common Mistakes

### 1. Trying to build from workspace root
```bash
# WRONG
go build ./...          # fails: root is not a module

# CORRECT
go build ./core/... ./server/...
```

### 2. Adding new storage methods to Harness
```go
// WRONG — Harness is APIProvider, not a service layer
type HarnessConfig struct {
    AddCustomStore func() storage.StoreProvider  // DON'T
}

// CORRECT — User provides their own Provider
type StoreConfig struct {
    Provider storage.StoreProvider  // user implements this
}
```

### 3. Creating noop implementations
```go
// WRONG — silently masking errors
if provider == nil {
    provider = &noopProvider{}  // DON'T
}

// CORRECT — fail fast
if provider == nil {
    return nil, fmt.Errorf("StoreConfig.Provider is required")
}
```

Exception: `memory` is optional — skip registration if `MemoryStore` is nil

### 4. Splitting capabilities incorrectly
```go
// WRONG — one capability returning wrong tool
type hitlCapability struct{}

func (c *hitlCapability) NewTool(...) (tool.Tool, error) {
    return NewConfirmActionTool(), nil  // always returns confirm_action
}

// CORRECT — separate capabilities for each tool
type confirmActionCapability struct{}
type askUserCapability struct{}
// Each NewTool returns its own tool
```

### 5. Importing tools/hooks in server/
```go
// WRONG — these are built-in
import "github.com/copcon/core/capabilities/tools"  // DON'T

// CORRECT — just use Harness
import "github.com/copcon/core"
h := core.NewHarness(cfg)  // capabilities auto-registered
```

## Git Conventions

- Commit messages: `type(scope): description`
  - `feat(core): add AgentFactorySpec`
  - `fix(server): correct session delete`
  - `refactor(core): remove noop types`
- Branch naming: `feat/feature-name`, `fix/issue-name`
- Current branch: `feat/v2`

## Environment

PostgreSQL required for integration tests:
```bash
docker compose up -d postgres
# DATABASE_HOST=localhost, DATABASE_PORT=5432, DATABASE_USER=admin
# DATABASE_PASSWORD=changeme, DATABASE_DBNAME=copcon
```

Qdrant optional (memory features):
```bash
# If MemoryStore is nil, hooks.memory skips registration
# No error, just silently disabled
```
