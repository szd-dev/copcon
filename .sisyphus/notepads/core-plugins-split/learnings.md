# Learnings: core-plugins-split

## Workspace Structure
- go.work at root manages core/ and server/ modules
- core/ has its own go.mod: `module github.com/copcon/core`
- server/ has its own go.mod: `module github.com/copcon/server` with `replace github.com/copcon/core => ../core`
- plugins/ will need its own go.mod + go.work entry

## Current Core Dependencies (go.mod)
- github.com/glebarez/sqlite - used by sqlite provider
- github.com/golang-cz/ringbuf
- github.com/google/uuid
- github.com/openai/openai-go/v3 - used by llm + embedding
- github.com/qdrant/go-client - was used by qdrant (removed in recent commit)
- github.com/stretchr/testify - tests
- golang.org/x/sync
- gorm.io/gorm - used by postgres + sqlite providers

## Files to Move (Exact Inventory)
### postgres (7 files)
store.go, session.go, message.go, todo.go, models.go, convert.go, doc.go

### sqlite (7 files)
store.go, session.go, message.go, todo.go, models.go, convert.go, sqlite_test.go

### filememory (6 files)
filememory.go, index.go, frontmatter.go, dir.go, path_validator.go, filememory_test.go

### sqlitevec (5 files)
knowledge.go, vector.go, schema.go, knowledge_test.go, integration_test.go

### embedding (7 files)
config.go, embedder.go, factory.go, openai.go, errors.go, embedder_test.go, openai_test.go

### rag (14 files + testdata dir)
parser.go, default_parser.go, markdown.go, html.go, pdf.go, text.go,
chunker.go, markdown_chunker.go, recursive_chunker.go, pipeline.go,
parser_test.go, chunker_test.go, pipeline_test.go, integration_test.go, testdata/

### eval (5 files)
metrics.go, retrieval.go, reporter.go, retrieval_test.go, golden_test.go

### Memory-related (14 files from 3 dirs)
- core/storage/memory.go → plugins/memory-file/store_interface.go
- core/providers/filememory/*.go (6 files) → plugins/memory-file/
- core/capabilities/hooks/file_memory.go → plugins/memory-file/
- core/capabilities/hooks/file_memory_capability.go → plugins/memory-file/
- core/capabilities/hooks/file_memory_test.go → plugins/memory-file/
- core/capabilities/hooks/memory.go → plugins/memory-file/
- core/capabilities/hooks/memory_types.go → plugins/memory-file/
- core/capabilities/tools/memory_store.go → plugins/memory-file/
- core/capabilities/tools/memory_store_capability.go → plugins/memory-file/
- core/capabilities/tools/memory_recall.go → plugins/memory-file/
- core/capabilities/tools/memory_recall_capability.go → plugins/memory-file/
- core/capabilities/tools/memory_forget.go → plugins/memory-file/
- core/capabilities/tools/memory_forget_capability.go → plugins/memory-file/
- core/capabilities/tools/memory_test.go → plugins/memory-file/

### Knowledge-related (8 files from 3 dirs)
- core/storage/knowledge.go → plugins/knowledge-base/store_interface.go
- core/providers/sqlitevec/*.go (5 files) → plugins/knowledge-base/sqlitevec/
- core/capabilities/hooks/kb_recall.go → plugins/knowledge-base/
- core/capabilities/hooks/kb_recall_capability.go → plugins/knowledge-base/
- core/capabilities/hooks/memory_persist.go → plugins/knowledge-base/
- core/capabilities/hooks/memory_persist_capability.go → plugins/knowledge-base/
- core/capabilities/hooks/knowledge_integration_test.go → plugins/knowledge-base/

## Package Structure in Capabilities
- hooks/ files are `package hooks` with `init()` registering to global sync.Map
- tools/ files are `package tools` with `init()` registering to global sync.Map
- After move to plugins, package names need unification or sub-packages

## Key Interfaces
- StoreProvider (core/storage/provider.go): Sessions(), Messages(), Todos(), Knowledge()
- MemoryStore (core/storage/memory.go): Store, Search, GetBySession, etc.
- KnowledgeStore (core/storage/knowledge.go): CreateKB, DeleteKB, etc.

## CapabilityDeps (registry.go)
Has fields for ALL stores including MemoryStore, FileMemoryStore, KnowledgeStore, Embedder
These are typed as interface{} to avoid circular imports

## Server Dependencies on Moving Code
- server/internal/store/factory.go: imports core/providers/sqlite, core/providers/postgres
- server/internal/api/handlers.go: imports core/providers/embedding, core/rag
- server/internal/api/knowledge_options.go: imports core/providers/embedding, core/rag
- server/cmd/server/main.go: imports core (HarnessConfig, StoreConfig)

## Step 1+2 Completion Notes
- All 70+ files moved via `git mv` preserving history
- core/providers/ fully removed (including doc.go via `git rm`)
- core/rag/ fully removed
- core/eval/ fully removed
- core/storage/ still has: doc.go, message.go, provider.go, session.go, todo.go (staying)
- core/capabilities/hooks/ still has: doc.go, logging.go, todo_injection.go, tracing.go (staying)
- core/capabilities/tools/ still has: doc.go, hitl.go, todo.go, todo_types.go, async.go, code_executor.go, file_ops.go, delegate.go, helpers.go (staying)
- go.work is in .gitignore (local workspace file) - updated to include ./plugins
- plugins/go.mod created with `module github.com/copcon/plugins` and replace directive for core
- Package declarations fixed: memory-file → `package memoryfile`, knowledge-base root → `package knowledgebase`
- sqlitevec subdirectory keeps `package sqlitevec`
- All other packages keep their original names (postgres, sqlite, embedding, rag, eval)
