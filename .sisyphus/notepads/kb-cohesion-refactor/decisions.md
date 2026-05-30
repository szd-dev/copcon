# 决策记录

## Task 4: 移动 embedding-openai → knowledge-base/embedding

### 变更
- 将 `plugins/embedding-openai/` 下的 7 个 .go 文件全部移至 `plugins/knowledge-base/embedding/`
- 包名从 `package embedding` 改为 `package kbembedding`
- 旧目录 `plugins/embedding-openai/` 已删除

### 技术细节
- 文件列表: config.go, embedder.go, embedder_test.go, errors.go, factory.go, openai.go, openai_test.go
- 使用 `git mv` 确保 git 跟踪历史
- 新包导入路径: `github.com/copcon/plugins/knowledge-base/embedding`
- `embedder.go` 中的 `type Embedder = storage.Embedder` 类型别名未修改
- 编译验证通过，无错误
- 旧目录（`plugins/embedding-openai/`）已无其他文件（无 go.mod/go.sum），已删除
## Task 4: Move plugins/eval/ into plugins/knowledge-base/eval/

- 将 5 个 .go 文件从 `plugins/eval/` 移到 `plugins/knowledge-base/eval/`
- 将根目录 `eval/testdata/` 整体移到 `plugins/knowledge-base/eval/testdata/`
- 包名从 `package eval` 改为 `package kbeval`
- golden_test.go 中 3 处相对路径从 `filepath.Join("..", "..", "eval", "testdata", ...)` 改为 `filepath.Join("testdata", ...)`
- 旧目录 `plugins/eval/` 和 `eval/testdata/` 已清空
- `go test ./plugins/knowledge-base/eval/...` 通过（0.099s）

## 2026-05-30: sqlitevec 目录重命名

- **决策**: 将 `plugins/knowledge-base/sqlitevec/` 重命名为 `plugins/knowledge-base/store/sqlitevec/`
- **操作**: 5 个 .go 文件通过 `git mv` 移动到新路径
- **包名**: 保持 `package sqlitevec` 不变（不随目录路径变化）
- **引用更新**: `server/cmd/server/main.go` 中导入路径从 `.../knowledge-base/sqlitevec` 更新为 `.../knowledge-base/store/sqlitevec`
- **验证**: `go build` + `lsp_diagnostics` 均通过
- **旧目录**: 已删除

## Task 4: RAG 包迁移 (plugins/rag → plugins/knowledge-base/rag)

- 使用 `git mv` 移动 14 个 .go 文件和 3 个 testdata 文件
- 包名从 `package rag` 改为 `package kbrag`（避免与目录名 rag 冲突，kbrag = knowledge-base rag）
- 更新 server 端 2 个文件的 import：`"github.com/copcon/plugins/rag"` → `kbrag "github.com/copcon/plugins/knowledge-base/rag"`
- 修复 mock 实现以满足合并后的 `knowledgebase.KnowledgeStore` 接口（Task 1 将 PipelineStore 合并到 KnowledgeStore）
  - `mockPipelineStore` 添加了 CreateKB, DeleteKB, ListKBs, GetKB, DeleteDocument, GetDocument, ListDocuments, GetChunks, UpdateChunk, Search 方法
  - `inMemoryPipelineStore` 添加了 DeleteKB, ListKBs, GetKB, DeleteDocument, ListDocuments, UpdateChunk, Search 方法
- 删除了 `TestPipelineStoreInterface` 中对已不存在的 `PipelineStore` 类型的引用，改为 `knowledgebase.KnowledgeStore`
- `pipeline.go` 中的 import `knowledgebase "github.com/copcon/plugins/knowledge-base"` 保持不变（正确）

## Task 4: memory_persist_hook.go 迁移至 memory-file

**决策**: 将 `MemoryPersistHook` 从 knowledge-base 积入 memory-file，删除 `MemoryStorePersister` 接口（因 `MemoryStore` 已有 Store/Search 方法），通过 `memoryPersistHookCapabilityClosure` 注册为 HookCapability。

**关键变更**:
- `memory_persist_hook.go` 包名 `knowledgebase` → `memoryfile`
- `MemoryStorePersister` 接口删除，改用 `MemoryStore`
- `RegisterCapabilities` 签名增加 `emb storage.Embedder` 参数
- 新增 `memoryPersistHookCapabilityClosure` 实现 `HookCapability` 接口
- capability name 使用 `capabilities.HookMemoryPersist`（即 `"hooks.memory_persist"`）
- 测试代码同步迁移至 `memory_persist_hook_test.go`，mock 实现 `MemoryStore` 全接口
- knowledge-base 测试文件中移除 MemoryPersistHook 相关测试和 mock

**原因**: MemoryPersistHook 操作的是 MemoryStore（向量记忆），与 knowledge-base 的 KnowledgeStore（知识检索）职责不同，应归属 memory-file。

## Task 9: 更新 server/main.go 中 embedding 引用路径

- **变更**: `server/cmd/server/main.go` 中 import 从 `"github.com/copcon/plugins/embedding-openai"` 改为 `kbembedding "github.com/copcon/plugins/knowledge-base/embedding"`
- **引用更新**:
  - 第18行: import 路径 + 别名 `kbembedding`
  - 第53行: `embedding.NewFromConfig` → `kbembedding.NewFromConfig`
  - 第127行: 函数返回类型 `embedding.EmbeddingConfig` → `kbembedding.EmbeddingConfig`
  - 第129-130行: `embedding.EmbeddingConfig{` 和 `embedding.BackendType` → `kbembedding.EmbeddingConfig{` 和 `kbembedding.BackendType`
- **验证**: `go build ./server/...` 通过，`grep embedding-openai` 无残留
