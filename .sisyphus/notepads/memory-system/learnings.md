# Learnings - Memory System Implementation

## Codebase Conventions

### Package Structure
- Plugin package: `plugins/memory-file/` (package name: `memoryfile`)
- Core packages: `core/llm/`, `core/hook/`, `core/capabilities/`, `core/storage/`
- Server: `server/cmd/server/main.go`, `server/internal/config/config.go`

### Patterns
- Frontmatter: YAML between `---` delimiters, serialized with `yaml.NewEncoder`
- INDEX.md: Full rebuild pattern (`BuildIndex` → `collectEntries` → `formatIndex`)
- Hook system: `Hook` interface with `Name()`, `Points()`, `Priority()`, `Execute(*HookContext)` 
- Module capability: `MemoryModule` implements `ModuleCapability` with `NewHooks()` + `NewTools()`
- File permissions: `0o600` for files, `0o700` for directories
- Dir structure: `{basePath}/{agentID}/{system,knowledge,archive}/`

### Important Interfaces
- `LLMProvider.Stream(ctx, StreamParams) (<-chan StreamChunk, <-chan error)` — stream-only API
- `FileMemoryStoreInterface`: extends `MemoryStore` with file-level ops
- `MemoryStoreAPI`: used by memory tools (store/recall/forget)
- `CapabilityDeps`: has `SessionStore`, `MessageStore`, `Logger`, `AgentRegistry`
- `HookContext`: has `ChatCtx`, `SessionID`, `AgentID`, `SystemPrompt`, `Messages`, `CurrentPoint`
- `MessageStore.List(ctx, sessionID, limit)` — for querying recent messages

### Go Module Path
- `github.com/copcon/core/...`
- `github.com/copcon/plugins/memory-file` (package: memoryfile)
- `github.com/copcon/server/...`

## [TIMESTAMP 2026-06-01T16:08] Session Start
- Plan: memory-system
- Tasks: 21 implementation + 3 final verification
- 4 waves: Base → Hooks → Summary → Tests
