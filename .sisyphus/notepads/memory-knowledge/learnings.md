# Learnings — memory-knowledge plan

## 2026-05-29 Session Start

### Codebase Structure
- `core/storage/memory.go` — Memory struct (ID, Content, SessionID, Role, Timestamp, MemoryType, Metadata, Score) + MemoryStore interface (Store, Search, GetBySession, DeleteBySession)
- `core/storage/provider.go` — StoreProvider interface: Sessions(), Messages(), Todos() — no Memory() or Knowledge() yet
- `core/capabilities/registry.go` — CapabilityDeps: SessionStore, MessageStore, TodoStore, MemoryStore, AgentRegistry, Engine, Logger. Uses sync.Map for global builtins registry.
- `core/harness.go` — StoreConfig{Provider, Memory}; AgentSpec{ID, Name, Model, SystemPrompt, Tools, AllowDelegate}; builtInHooks=[todo_injection, memory, logging, tracing]; builtInTools=[confirm_action, ask_user, todo, async]; collectCapabilityNames uses seen map for dedup
- `core/providers/` — existing providers: qdrant/, postgres/
- `core/capabilities/hooks/` and `core/capabilities/tools/` — auto-registered via init() with blank imports in harness.go

### Key Design Decisions
- Memory and KnowledgeBase are two separate capability bundles, independently enabled
- KnowledgeStore is pluggable via RegisterKnowledgeStoreProvider — sqlite-vec this phase
- MemoryStore.Knowledge() returns nil when not configured (same as Todos())
- File memory uses MD files with YAML frontmatter + INDEX.md (200 line/25KB limit)
- MemoryPersistHook uses keyword extraction only, NO LLM calls
- Embedder interface abstracts text embedding, OpenAI implementation reuses existing LLMProvider

### Module Boundaries
- core/ MUST NOT import server/
- New storage interfaces go in core/storage/
- New providers go in core/providers/
- Capability registration uses init() + global registry

## 2026-05-30 W1.1 — Enhanced MemoryStore interface

### Changes Made
- Added `MemoryType` type (`string` alias) with constants: `MemoryTypeEpisodic="episodic"`, `MemoryTypeSemantic="semantic"`, `MemoryTypeProcedural="procedural"`, `MemoryTypeConversation=MemoryTypeEpisodic` (backward-compat alias)
- Added `MemoryFilter` struct: `SessionID`, `MemoryType []MemoryType`, `Limit`, `Offset`, `Since`, `Until` — zero values = no filter
- Extended `Memory` struct with 3 new fields at end: `ValidAt *time.Time`, `InvalidAt *time.Time`, `Importance float64`
- Extended `MemoryStore` interface with 4 new methods: `List`, `Get`, `Update`, `Delete`
- Existing 4 methods unchanged (Store/Search/GetBySession/DeleteBySession)
- Added stub implementations to Qdrant provider (`core/providers/qdrant/memory.go`) returning `errors.New("not yet implemented: ...")` so compilation passes
- Verified: `go build ./core/...` and `go vet ./core/storage/...` pass clean

## 2026-05-30 W1.3 — Embedder interface package

### Changes Made
- Created `core/providers/embedding/` package with 4 files:
  - `embedder.go` — Embedder interface with 4 methods: `Embed(ctx, text) ([]float32, error)`, `EmbedBatch(ctx, texts) ([][]float32, error)`, `Dimensions() int`, `Name() string`
  - `config.go` — `BackendType` (string alias) with constants `BackendOpenAI="openai"`, `BackendBGEM3="bge_m3"` (reserved); `EmbeddingConfig` struct with `yaml` tags
  - `errors.go` — 3 sentinel errors: `ErrUnsupportedBackend`, `ErrEmptyText`, `ErrDimensionMismatch`
  - `embedder_test.go` — `mockEmbedder` struct with `var _ Embedder = (*mockEmbedder)(nil)` compile-time check + 5 test cases
- Package doc follows same pattern as `core/llm/provider.go`
- BGE-M3 backend is NOT implemented — only config field exists, marked as reserved
- Verified: `go build ./providers/embedding/...` + `go test ./providers/embedding/... -v` both pass

## 2026-05-30 W1.2: KnowledgeStore Interface

### Files Created
- `core/storage/knowledge.go` — KnowledgeBase, Document, DocumentStatus, Chunk, SearchOptions types + KnowledgeStore interface (11 methods)
- `core/storage/knowledge_test.go` — mockKnowledgeStore implementing all 11 methods + signature verification tests

### Files Modified
- `core/storage/provider.go` — added `Knowledge() KnowledgeStore` to StoreProvider interface
- `core/providers/postgres/store.go` — added `Knowledge()` returning nil
- `core/providers/sqlite/store.go` — added `Knowledge()` returning nil
- `core/harness.go` — added `Knowledge()` returning nil on quickStoreProvider
- `core/harness_test.go` — added `Knowledge()` returning nil on testStoreProvider
- `server/internal/api/handlers_test.go` — added `Knowledge()` returning nil on testStoreProvider

### Key Decisions
- Knowledge() returns nil on all existing providers (backward compatible, same pattern as Todos())
- DocumentStatus is a string type with 4 constants (pending/parsing/ready/error), matching TodoStatus pattern
- Search takes kbIDs []string for cross-KB search capability
- IngestDocument takes raw []byte content — parsing is implementation concern
- Chunk.Context field reserved for Contextual Retrieval technique
- No uuid.UUID used for KB/Document/Chunk IDs — using string for flexibility (supports both UUID and non-UUID backends)
- All StoreProvider implementations across core/ and server/ updated in one pass

## 2026-05-30 W1.5 — Configuration extension for Memory & KnowledgeBase

### Changes Made
- Added `MemoryConfig` struct (Enabled, BasePath, SystemDir, IndexFile, MaxIndexLines, MaxIndexBytes) to `server/internal/config/config.go`
- Added `EmbeddingConfig` struct (Backend, OpenAIModel, BGEM3Endpoint) to `server/internal/config/config.go` — server-level type, separate from `core/providers/embedding.EmbeddingConfig`
- Added `KnowledgeBaseConfig` struct (ID, Name, Backend, SQLitePath, ChunkSize, ChunkOverlap, Embedding) to `server/internal/config/config.go`
- Added `Memory MemoryConfig` and `KnowledgeBases []string` fields to `AgentConfig`
- Added `KnowledgeBases []KnowledgeBaseConfig` field to `Config`
- Extended `validate()` with 4 new checks:
  1. Duplicate KB IDs rejected
  2. Agent KB references validated against known KB IDs
  3. Embedding backend must be "openai" or empty (empty = not referenced by agent = valid)
  4. Memory.BasePath must be absolute or start with ~/ when non-empty
- Updated `config.yaml.template` with commented example sections for memory and knowledge_bases
- Added 8 new test cases covering all new validation rules
- Verified: `go build ./internal/config/...` + `go test ./internal/config/... -v -count=1` — 17 tests all pass

### Key Decisions
- EmbeddingConfig at server level is intentionally separate from core/providers/embedding.EmbeddingConfig to avoid circular deps
- Empty embedding backend is valid (KB exists but isn't referenced by any agent) — validation only fires when agent references a KB
- KnowledgeBase ref validation (len(kbIDSet) > 0 || len(c.Agents) > 0) prevents false positives on empty configs

## 2026-05-30 W1.6 — AgentSpec dual Bundle support

### Changes Made
- Added `MemorySpec` struct to `core/harness.go` with fields: Enabled, BasePath, SystemDir, IndexFile, MaxIndexLines, MaxIndexBytes
- Added `Memory MemorySpec` and `KnowledgeBases []string` fields to `AgentSpec` struct
- Created `core/capabilities/bundle.go` with two exported functions:
  - `MemoryBundleNames()` returns `["hooks.file_memory", "tools.memory_store", "tools.memory_recall", "tools.memory_forget"]`
  - `KnowledgeBaseBundleNames()` returns `["hooks.kb_recall", "hooks.memory_persist"]`
- Created `core/capabilities/bundle_test.go` with 5 tests:
  - TestMemoryBundleNamesNonEmpty, TestKnowledgeBaseBundleNamesNonEmpty
  - TestBundlesAreDisjoint (verifies no overlap)
  - TestMemoryBundleNamesCount (expects 4), TestKnowledgeBaseBundleNamesCount (expects 2)
- Verified: `go build ./core/...`, `go test ./core/capabilities/...`, `go test ./core/` all pass

### Key Decisions
- MemorySpec is a standalone struct in core/harness.go (NOT importing server config types) to maintain module boundary
- bundle.go avoids any imports beyond `package capabilities` — just returns string literals
- Bundle name strings must match capability names that W2.4/W3.7 will register
- bundle.go intentionally does NOT import core/providers/filememory — it's just name definitions

## 2026-05-30 W1.4 — OpenAI Embedder implementation

### Files Created
- `core/providers/embedding/openai.go` — `openAIEmbedder` struct implementing Embedder via direct HTTP calls to OpenAI's `/v1/embeddings` endpoint
- `core/providers/embedding/factory.go` — `NewFromConfig(cfg EmbeddingConfig, llm llm.LLMProvider) (Embedder, error)` factory dispatching by BackendType
- `core/providers/embedding/openai_test.go` — 18 test cases covering all methods and error conditions

### Files Modified
- `core/providers/embedding/config.go` — added `BaseURL string` and `APIKey string` fields to `EmbeddingConfig` (needed for HTTP calls, LLMProvider interface doesn't expose them)

### Key Design Decisions
- `NewOpenAIEmbedder(llm, baseURL, apiKey, model)` accepts baseURL/apiKey explicitly since `llm.LLMProvider` only exposes `Stream()` — no way to extract config from it
- `LLMProvider` stored as struct field for dependency marker consistency, but embedder makes direct HTTP requests (not through the provider)
- Default baseURL: `https://api.openai.com/v1/` when empty string provided
- Ada-002 does NOT support the `dimensions` parameter — handled with model-specific logic
- Error response parsing: reads response body directly, attempts structured error extraction for OpenAI-style `{"error":{"message":"..."}}` responses
- EmbedBatch sends all texts in a single HTTP request (OpenAI API supports batch natively), rather than N individual calls

### Test Coverage (18 tests)
- Model validation: valid models (3 variants), invalid model, empty model, wrong casing
- Embed: single text, batch (3 texts)
- Error cases: empty text, empty slice, empty string in slice, HTTP 500, timeout, dimension mismatch, invalid JSON
- Ada-002 dimension exclusion verification
- Factory: OpenAI dispatch, BGE-M3 unsupported, unknown backend
- URL normalization: trailing slash, no trailing slash, empty default
- Compile-time interface check

### Gotchas Fixed
- HTTP 500 error parsing required a nested struct wrapper `{"error": {...}}` — flat unmarshal of `openAIErrorBody` from the full body returns empty Message
- Test handlers that decode request body then delegate to another handler cause EOF on the second decode — build response inline instead

## 2026-05-30 W1.7 — KnowledgeStore Provider registry

### Files Created
- `core/storage/knowledge_registry.go` — `KnowledgeStoreFactory` type, `RegisterKnowledgeStoreProvider`/`LookupKnowledgeStoreProvider` functions
- `core/storage/knowledge_registry_test.go` — 4 test cases

### Key Decisions
- `map[string]KnowledgeStoreFactory` + `sync.Mutex` (not `sync.Map`) — simpler and sufficient for this use case
- `RegisterKnowledgeStoreProvider` panics on duplicate (programmer error = fail fast), consistent with AGENTS.md guidance
- No `UnregisterKnowledgeStoreProvider` — registration is immutable
- Error message on lookup failure includes list of registered backends for UX
- `providerNames()` is unexported helper for error messages — intentionally unsorted (just a debug aid)

### Test Coverage (4 tests, all pass under -race)
1. `TestRegisterAndLookup` — register + lookup happy path
2. `TestLookupUnknown` — lookup nonexistent returns error
3. `TestDuplicateRegisterPanics` — duplicate registration panics (defer/recover)
4. `TestConcurrentAccess` — 10 goroutines register + 10 lookup concurrently

## 2026-05-30 W2.1-W2.5 — Complete Memory Capability subsystem

### W2.1: File-based Memory Store
- Created `core/providers/filememory/` with 6 files: filememory.go, frontmatter.go, index.go, path_validator.go, dir.go, filememory_test.go
- `FileMemoryStore` implements both `storage.MemoryStore` (8 methods) and `FileMemoryStoreInterface` (7 file-level methods)
- Store method: writes MD file with YAML frontmatter under knowledge/ or archive/ depending on `categoryFromType(memoryType)`, preserving original `memory_type` in frontmatter metadata
- Search returns error (vector search not supported for file memory — keyword recall is the alternative)
- Path validation: rejects `..`, absolute paths, dotfiles, unclean components
- Frontmatter uses `gopkg.in/yaml.v3` for serialization — roundtrip tested
- INDEX.md generation with line/byte truncation limits
- Directory permissions: 0o700 dirs, 0o600 files
- Key gotcha: `MemoryTypeConversation` is alias for `MemoryTypeEpisodic` — duplicate case in switch causes compile error; fixed by removing the duplicate case
- Key gotcha: Writing to nil metadata map panics — always initialize `fm.Metadata = make(map[string]string)` before writing to it, regardless of whether memory.Metadata is nil
- MemoryStore Store method uses `memory.ID` as filename (no Role prefix) — consistent for Update/Delete

### W2.2: FileMemoryHook
- Created `core/capabilities/hooks/file_memory.go` 
- Implements hook.Hook: Name="file_memory", Points=[OnSystemPrompt], Priority=80
- Uses `FileMemoryStoreReader` interface (local, just BasePath()) to avoid circular import of filememory package from hooks package
- Injects system/ MD files + INDEX.md + Memory Protocol into system prompt
- Skips injection when no agent directory exists or no files present
- INDEX.md is NOT injected as a system file (excluded from System Context section, only appears in Memory Index section)

### W2.3: Memory Tools
- Created 3 files: `memory_store.go`, `memory_recall.go`, `memory_forget.go` in `core/capabilities/tools/`
- `MemoryStoreAPI` interface defined in tools package (not importing filememory directly) — avoids circular dependency
- Tools use `iface.ChatContextInterface` for Execute method (matches tool.Tool interface)
- memory_store: writes MD file with frontmatter + updates INDEX via WriteFile (which calls BuildIndex)
- memory_recall: keyword matching (case-insensitive) across knowledge/ + archive/ directories
- memory_forget: deletes file by path or name search + regenerates INDEX

### W2.4: Memory Capability Registration
- Created 4 capability files: `file_memory_capability.go`, `memory_store_capability.go`, `memory_recall_capability.go`, `memory_forget_capability.go`
- Each has `init()` that registers via `capabilities.Register()`
- Extended `CapabilityDeps` with `FileMemoryStore interface{}` field — avoids circular import of filememory package from capabilities package
- Type assertion happens in each capability's NewHook/NewTool method: store is type-asserted to the appropriate local interface
- Capability names match `MemoryBundleNames()`: "hooks.file_memory", "tools.memory_store", "tools.memory_recall", "tools.memory_forget"
- Tool capabilities depend on "hooks.file_memory" for dependency ordering

### W2.5: Comprehensive Tests
- filememory_test.go: 38 tests covering Store, GetBySession, Update, Delete, WriteFile/ReadFile/DeleteFile, ListFiles, GetIndex, List with filter, Search error, frontmatter roundtrip, path security, INDEX truncation, permissions, integration cycle
- file_memory_test.go: 9 tests covering hook Name/Points/Priority, injection of system files, injection of INDEX, skip when no files, skip when nil prompt, skip when empty agentID, INDEX not in system files
- memory_test.go: 12 tests covering all 3 tools' Name, Execute, error cases, and full store→recall→forget integration cycle

### Architectural Decision: Circular Import Avoidance
- capabilities package CANNOT import core/providers/filememory (filememory imports core/storage which is imported by capabilities)
- Solution: Use `interface{}` in CapabilityDeps, type-assert in each capability's NewHook/NewTool
- Tools package defines `MemoryStoreAPI` locally instead of importing FileMemoryStoreInterface
- Hooks package defines `FileMemoryStoreReader` locally (just needs BasePath())
