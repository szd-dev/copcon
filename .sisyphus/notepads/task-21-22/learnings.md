# Task 21-22: Provider Implementations

## Created Files

### Postgres Provider (`core/providers/postgres/`)
- `models.go` — GORM-annotated structs (Session, Message, Todo) + JSONB/UUIDArray custom types + AutoMigrate
- `convert.go` — Model ↔ storage domain conversion functions (sessionToStorage/From, messageToStorage/From, todoToStorage/From, plus ToolCall and Part conversions)
- `session.go` — SessionStore implementing storage.SessionStore
- `message.go` — MessageStore implementing storage.MessageStore (includes Upsert with ON CONFLICT)
- `todo.go` — TodoStore implementing storage.TodoStore
- `store.go` — Convenience Store aggregate with NewStore(db) constructor + compile-time interface checks + IsNotFound helper

### Qdrant Provider (`core/providers/qdrant/`)
- `memory.go` — MemoryStore implementing storage.MemoryStore (Store, Search, GetBySession, DeleteBySession)
- Uses storage.Memory.Timestamp (time.Time) instead of int64 from the server's Memory struct
- pointToMemory helper converts Qdrant points to storage.Memory

## Key Decisions
- GORM model structs are duplicated (not imported from server) to break the dependency cycle and keep core self-contained
- Named types with `Model` suffix (ToolCallModel, ToolCallsModel, FunctionCallModel) to avoid collision with storage.ToolCall etc.
- Compile-time interface checks consolidated in store.go
- Qdrant store converts int64 timestamps from payload to time.Time for storage.Memory
- Network was unavailable; go.sum populated from server's go.sum, indirect deps manually added to go.mod

## Build Verification
- `go build ./providers/postgres/...` ✓
- `go build ./providers/qdrant/...` ✓
- `go vet ./providers/postgres/... ./providers/qdrant/...` ✓
- LSP diagnostics: 0 errors in both packages
