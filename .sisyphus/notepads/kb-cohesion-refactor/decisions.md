# Decisions: Type Migration from core/storage to plugin types packages

## 2026-05-31: Phase 1 — Move plugin-specific types out of core/storage

### What was moved
| Source | Destination | Types |
|--------|-------------|-------|
| `core/storage/embedder.go` | `plugins/knowledge-base/types/embedder.go` | `Embedder` interface |
| `core/storage/knowledge_types.go` | `plugins/knowledge-base/types/knowledge.go` | `KnowledgeBase`, `Document`, `Chunk`, `DocumentStatus`, `SearchOptions` + constants |
| `core/storage/memory.go` | `plugins/memory-file/types/memory.go` | `Memory`, `MemoryFilter`, `MemoryType` + constants |

### Import alias conventions
- `kbtypes "github.com/copcon/plugins/knowledge-base/types"` — knowledge-base types
- `memtypes "github.com/copcon/plugins/memory-file/types"` — memory-file types

### Key decisions
1. **Package name `types`** — chosen to be idiomatic Go for a pure-type sub-package
2. **Alias naming** — `kbtypes`/`memtypes` to avoid collision with `storage` and keep references short
3. **memory_persist_hook.go** — this file in memory-file uses both `Embedder` (kbtypes) and `Memory` types (memtypes), so it imports both
4. **server/internal/api/handlers.go** — keeps `"github.com/copcon/core/storage"` for `SessionStore`/`MessageStore`/`TodoStore`/`Session`/`Message`, adds `kbtypes` for `Embedder`
5. **render.go, handlers_test.go, factory.go** — unchanged; they only use `storage.Part`, `storage.Message`, `storage.Session`, etc. which remain in core/storage

### Files NOT changed (core/*, storage-postgres/*, storage-sqlite/*)
These files only reference types that remain in `core/storage` (Session, Message, Todo, StoreProvider, etc.)
