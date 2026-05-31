# Learnings - VectorStore Architecture

## Codebase Structure
- **Go workspace**: `go.work` with 3 modules: core, plugins, server
- **plugins/go.mod**: Single module `github.com/copcon/plugins` for ALL plugins
- **Module paths**: `github.com/copcon/plugins/knowledge-base`, `github.com/copcon/plugins/knowledge-base/types`, etc.

## Existing Code
- `KnowledgeStore` interface already in `store_interface.go` (31 lines)
- `KnowledgeStore` (concrete struct) in `sqlitevec/knowledge.go` has `db *gorm.DB`, `sqlDB *sql.DB`, `dimension int`
- `dialector.go` imports `modernc.org/sqlite` + `modernc.org/sqlite/vec` - the DRIVER CONFLICT source
- `vector.go` has `toBlob`/`fromBlob` helpers (29 lines)
- `schema.go` has GORM models (kbModel, docModel, chunkModel) + `initVectorTable` + `chunkIDToRowID`
- `chunkModel` has `Vector []byte` blob column - vector stored in chunks table (useful for brute-force)
- `cosineSimilarity` helper in `knowledge.go:381-398`
- `jsonb` type defined in `schema.go:14-35`

## Server Integration
- `server/cmd/server/main.go:137-151`: `createKnowledgeStore()` uses `NewKnowledgeStoreFromDSN(dsn)`
- NO config flag for brute-force vs sqlite-vec - need to add `VectorBackend` to `KnowledgeConfig`
- Server uses `glebarez/sqlite` for its main DB, knowledge uses `modernc.org/sqlite` via dialector (THE CONFLICT)
- `BuildKnowledgeOptions` in `knowledge_options.go` receives `knowledgebase.KnowledgeStore` interface

## Types Package
- `types/knowledge.go`: `KnowledgeBase`, `Document`, `Chunk`, `SearchOptions` - all pure value types
- `Chunk` has: ID, DocumentID, KBID, Content, Context, Index, TokenCount, Metadata, Score
- `SearchOptions` has: TopK, SimilarityThreshold, Filters

## Key Patterns
- Option pattern: `WithDimension(d int) Option` 
- Compile-time interface check: `var _ knowledgebase.KnowledgeStore = (*KnowledgeStore)(nil)`
- Domain conversion: `toDomain()` methods on GORM models
- RAG pipeline calls `knowledgebase.KnowledgeStore` interface methods

## Gotchas
- `openDialector` is used by tests (`:memory:`) - MUST KEEP or refactor tests
- `dialector.go` has custom GORM dialector implementation (257 lines) - tests depend on `openDialector` for in-memory SQLite
- After removing dialector.go, tests need to use `glebarez/sqlite` instead of modernc
