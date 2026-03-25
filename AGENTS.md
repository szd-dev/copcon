# AGENTS.md — CopCon Agent Infrastructure

## Project Overview

Monorepo containing a Go backend (Agent Engine) and React component library. Uses pnpm workspaces for frontend packages.

**Tech Stack:**
- Backend: Go 1.26, Gin, GORM, go-openai, Qdrant client
- Frontend: React 19, TypeScript 5, Vite 6, @ant-design/x 2.x
- Infra: PostgreSQL 15, Qdrant 1.17, Docker Compose

---

## Build, Test, Lint Commands

### Backend (Go)

```bash
cd server

# Run all tests (requires PostgreSQL)
go test ./... -v

# Run single package tests
go test ./internal/session/... -v
go test ./internal/tool/... -v
go test ./internal/tools/... -v

# Build
go build ./cmd/server

# Run dev server
go run ./cmd/server

# Lint (standard go vet)
go vet ./...

# Format
go fmt ./...
```

### Frontend (packages/ui)

```bash
cd packages/ui

pnpm install       # Install dependencies
pnpm dev           # Dev server (port 5173, remote accessible)
pnpm build         # Build library
pnpm storybook     # Storybook docs (port 6006, remote accessible)
```

### Frontend (packages/demo)

```bash
cd packages/demo
pnpm install
pnpm dev           # Dev server
```

### Docker Compose

```bash
docker compose up -d           # Start all services
docker compose down            # Stop all services
docker compose logs -f server  # Follow server logs
```

---

## Code Style Guidelines

### Go

- **Naming**: camelCase for private, PascalCase for exported. Abbreviations stay uppercase (e.g. `ID`, `HTTP`, `API`)
- **Imports**: Standard lib → blank line → external → blank line → internal. Use aliases for clarity (`openai "github.com/sashabaranov/go-openai"`)
- **Error Handling**: Wrap errors with `fmt.Errorf("action: %w", err)`. Define sentinel errors as package-level vars (`var ErrNotFound = errors.New("...")`)
- **Types**: Prefer interfaces for dependencies. Export interfaces, keep implementations private (`type SessionManager interface` vs `type sessionManager struct`)
- **GORM**: Use `db.WithContext(ctx)` on every query. Check `result.RowsAffected` for mutations
- **Tests**: Use `testify/assert` and `testify/require`. Setup via `setupTestDB(t)` pattern with `t.Cleanup()` for teardown
- **File Structure**: One primary type per file. Tests alongside source (`manager_test.go` next to `manager.go`)

### TypeScript / React

- **Strict Mode**: tsconfig has `"strict": true`. Never use `as any` or `@ts-ignore`
- **Imports**: React first → third-party → local. Use named exports, not default exports
- **Components**: Named function components with explicit `React.FC<Props>` typing. Props interface exported above component
- **Hooks**: Custom hooks in `src/hooks/`. Return object with named fields. Use `useCallback` for event handlers
- **Types**: Define in same file or `types.ts`. Export types separately from runtime code
- **Styling**: CSS-in-JS or classes, no inline styles for complex layouts

### General

- **No type suppression**: Never use `as any`, `@ts-ignore`, `@ts-expect-error`
- **No empty catch blocks**: Always handle or propagate errors
- **One responsibility per function**: If a function does two things, split it
- **Context propagation**: Always pass `context.Context` as first arg in Go

---

## Architecture Notes

### Backend Flow

```
HTTP Request → Gin Handler → AgentEngine → OpenAI API (streaming)
                                    ↓
                              Tool Execution → MCP Tools (Code/Shell/File)
                                    ↓
                              Session/Context Manager → PostgreSQL + Qdrant
```

### Key Packages

| Package | Responsibility |
|---------|---------------|
| `internal/agent` | Core agent loop, streaming, tool orchestration |
| `internal/session` | Session CRUD, message storage (GORM) |
| `internal/context` | Context window management for LLM |
| `internal/memory` | Vector memory via Qdrant |
| `internal/tool` | Tool registry and execution |
| `internal/tools` | Concrete tool implementations |

### Frontend Packages

| Package | Responsibility |
|---------|---------------|
| `packages/ui` | Reusable React component library (@copcon/ui) |
| `packages/demo` | Vite demo app consuming the UI library |

---

## Common Patterns

### Adding a New Go Endpoint

1. Define handler in `internal/api/handlers.go`
2. Register route in `internal/api/routes.go`
3. Add business logic to appropriate manager in `internal/*/`
4. Write tests with `setupTestDB(t)` pattern

### Adding a New React Component

1. Create `src/components/ComponentName/index.tsx` with props interface
2. Export from `src/index.ts`
3. Add Storybook story in `index.stories.tsx`

### Adding a New Tool

1. Implement in `internal/tools/`
2. Register in `internal/tool/manager.go`
3. Add to OpenAI tool definitions via `GetOpenAITools()`

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| OPENAI_API_KEY | Yes | - | OpenAI API key |
| DATABASE_HOST | No | localhost | PostgreSQL host |
| DATABASE_PORT | No | 5432 | PostgreSQL port |
| DATABASE_USER | No | admin | DB user |
| DATABASE_PASSWORD | No | changeme | DB password |
| DATABASE_DBNAME | No | agent_infra | DB name |
| QDRANT_HOST | No | localhost | Qdrant host |
| QDRANT_PORT | No | 6333 | Qdrant port |
| CONFIG_PATH | No | config.yaml | Path to config file |

---

## Gotchas

- Go 1.26 required — older versions will fail to compile
- Tests require PostgreSQL running (uses real DB, not mocks)
- Frontend `peerDependencies` include `antd` and `@ant-design/x` — install in consuming app
- Storybook and dev server configured for remote access (binds to 0.0.0.0)
- Qdrant collection must be initialized before memory features work (`scripts/init-qdrant.sh`)
