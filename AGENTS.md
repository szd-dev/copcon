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

## ChatContext Pattern

### Overview

ChatContext is the unified context object that flows through the entire request lifecycle: HTTP Handler → Agent Engine → Manager → Tool. It encapsulates session identity, context, and event streaming.

### Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Request Flow                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   HTTP Handler              Agent Engine               Manager/Tool         │
│   ┌─────────────┐          ┌─────────────┐           ┌─────────────┐       │
│   │ Create      │          │ Use         │           │ Extract     │       │
│   │ ChatContext │ ───────→ │ chatCtx     │ ────────→ │ SessionID() │       │
│   │             │          │             │           │ Context()   │       │
│   └─────────────┘          └─────────────┘           └─────────────┘       │
│          │                        │                     │                   │
│          │                        ↓                     │                   │
│          │                 ┌─────────────┐              │                   │
│          │                 │ Emit Events │              │                   │
│          │                 │ via chatCtx │              │                   │
│          │                 └─────────────┘              │                   │
│          │                        │                     │                   │
│          │                        │                     │                   │
│          └────────────────────────┼─────────────────────┘                   │
│                                   ↓                                         │
│                          SSE Stream to Client                               │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Package Structure

```
server/internal/domain/
├── entity/
│   └── event.go         # Event, EventType, MessageData, ToolCallData, etc.
└── iface/
    └── chat.go          # ChatContextInterface
```

### ChatContextInterface

```go
// Defined in: server/internal/domain/iface/chat.go
type ChatContextInterface interface {
    Context() context.Context        // Standard context
    SessionID() string               // Current session ID
    AgentID() string                 // Current agent ID
    Events() <-chan entity.Event     // Event stream (receive-only)
    Emit(event entity.Event)         // Send event to stream
}
```

### ChatContext Implementation

```go
// Defined in: server/internal/context/chat.go
type ChatContext struct {
    ctx       context.Context
    sessionID string
    agentID   string
    events    chan entity.Event
}

func NewChatContext(ctx context.Context, sessionID, agentID string) *ChatContext
```

### Usage in Managers

All Manager interfaces use `ChatContextInterface` as the first parameter:

```go
// SessionManager
type SessionManager interface {
    Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string) (*Session, error)
    Get(chatCtx iface.ChatContextInterface) (*Session, error)
    // ...
}

// TodoManager
type TodoManager interface {
    Create(chatCtx iface.ChatContextInterface, content string, opts ...TodoOption) (*Todo, error)
    List(chatCtx iface.ChatContextInterface) ([]*Todo, error)
    // ...
}
```

### Usage in Tools

Tools receive `ChatContextInterface` to access session info:

```go
type Tool interface {
    Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*ToolResult, error)
}

// In tool implementation:
func (t *TodoTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*ToolResult, error) {
    sessionID := chatCtx.SessionID()
    ctx := chatCtx.Context()
    // ...
}
```

### Usage in HTTP Handler

```go
func (h *Handler) Chat(c *gin.Context) {
    chatCtx := contextpkg.NewChatContext(
        c.Request.Context(),
        c.Param("sessionId"),
        req.AgentID,
    )
    
    go h.agent.Chat(chatCtx, req.Content)
    
    // Stream events to client
    for event := range chatCtx.Events() {
        data, _ := json.Marshal(event)
        fmt.Fprintf(c.Writer, "data: %s\n\n", data)
        c.Writer.(http.Flusher).Flush()
    }
}
```

### Why This Pattern?

| Problem | Solution |
|---------|----------|
| `sessionID` passed as string everywhere | Encapsulated in `ChatContext` |
| Tools cannot access session info | `chatCtx.SessionID()` available |
| Duplicate interface definitions | Centralized in `domain/iface` |
| Event types scattered | Centralized in `domain/entity` |
| Import cycles between packages | Interface defined in `domain/iface`, implementation in `context` |

### Adding a New Manager

When creating a new manager:

1. Define interface in `internal/<package>/manager.go` using `iface.ChatContextInterface`
2. Extract sessionID via `chatCtx.SessionID()`
3. Use `chatCtx.Context()` for database operations
4. DO NOT define local `ChatContextInterface` — import from `domain/iface`

```go
package mypackage

import "github.com/copcon/server/internal/domain/iface"

type MyManager interface {
    DoSomething(chatCtx iface.ChatContextInterface, arg string) error
}
```

---

## Architecture Notes (Original)

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
| `internal/domain` | Shared domain types: entities (`entity.Event`) and interfaces (`iface.ChatContextInterface`) |
| `internal/agent` | Core agent loop, streaming, tool orchestration |
| `internal/session` | Session CRUD, message storage (GORM) |
| `internal/context` | Context window management for LLM |
| `internal/memory` | Vector memory via Qdrant |
| `internal/todo` | Todo list management for agents |
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
