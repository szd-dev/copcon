# Decisions

## Wave 1: Merge `rag.PipelineStore` into `knowledgebase.KnowledgeStore`

- **Decision**: 将 `rag.PipelineStore` 接口的 `StoreChunks` 和 `UpdateDocumentStatus` 方法合并到 `knowledgebase.KnowledgeStore` 接口中，然后删除 `rag.PipelineStore` 接口定义。
- **Rationale**: 消除两个 store 接口的重复定义问题。`KnowledgeStore` 原本已有 `IngestDocument`（PipelineStore 的另一个方法），加上新增的两个方法后，PipelineStore 的三个方法全部存在于 KnowledgeStore 中。
- **Changes**:
  1. `plugins/knowledge-base/store_interface.go`: 新增 `StoreChunks` 和 `UpdateDocumentStatus` 方法（KnowledgeStore 从 11 方法增加到 13 方法）
  2. `plugins/rag/pipeline.go`: 删除 `PipelineStore` 接口（16-20行），`Pipeline.store` 字段类型改为 `knowledgebase.KnowledgeStore`，构造器参数同步更新，添加 `knowledgebase` import
  3. `server/internal/api/knowledge_options.go`: 移除 `ks.(rag.PipelineStore)` 类型断言，直接传递 `ks` 给 `rag.NewPipeline`
- **Verification**: `go build ./plugins/...` 和 `go build ./server/...` 均通过，无 LSP 错误
## [2026-05-30 22:~] Task 4: 清理死代码 + 删除 MemoryStoreDeps
- Deleted files: `kb_recall_capability.go`, `memory_persist_capability.go` — stub capabilities returning nil,nil
- Deleted: `MemoryStoreDeps` interface from `register.go`
- Changed: `RegisterCapabilities` signature removed `ms MemoryStoreDeps` param → `(r *capabilities.Registry, ks KnowledgeStore, emb storage.Embedder)`
- Deleted: `memoryPersistHookCapabilityClosure` from `capabilities_closure.go`
- Deleted: `FormatKBResultsStub()` from `memory_persist_hook.go`
- Removed unused imports: `"context"` from `register.go`, `"fmt"` from `memory_persist_hook.go`
- Updated caller: `server/cmd/server/main.go:67` — removed `fmStore` arg
- Status: ✅ 完成 (go build + go vet passed)
