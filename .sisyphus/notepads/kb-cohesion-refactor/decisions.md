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

## 迁移 glebarez/sqlite → modernc.org/sqlite (sqlitevec 包)

**日期**: 2026-05-30

**决策**: 不使用 `gorm.io/driver/sqlite` 包（它默认引入 `mattn/go-sqlite3` CGo 依赖），而是在 `sqlitevec/` 包内实现自定义 GORM Dialector (`dialector.go`)。

**理由**:
- `gorm.io/driver/sqlite` 的 `sqlite.go` 中 import `_ "github.com/mattn/go-sqlite3"`，导致 CGo 编译依赖
- 任务要求"不引入 CGO 编译依赖"
- 自定义 Dialector 使用 `modernc.org/sqlite` 注册的 "sqlite" 驱动名（`database/sql.Open("sqlite", dsn)`），完全 CGo-free
- Dialector 实现从 `gorm.io/driver/sqlite` 的核心逻辑复制，但移除了 CGo import，使用默认 migrator

**影响**:
- `storage-sqlite` 包仍使用 `glebarez/sqlite`，go.mod 中保留该依赖（不在本任务范围）
- `modernc.org/sqlite` 从 indirect 升级为 direct 依赖
- `gorm.io/driver/sqlite` 和 `mattn/go-sqlite3` 从 go.mod 中移除
- `CGO_ENABLED=0 go build` 编译通过

## Task 13: sqlite-vec 集成决策

### 关键决策1：使用 modernc.org/sqlite 内置 vec 扩展
- `modernc.org/sqlite` v1.47.0+ (2026-03-17) 内置了 sqlite-vec 的 CGo-free 端口
- 只需空白导入 `_ "modernc.org/sqlite/vec"` 即可自动注册 vec0 虚拟表
- 从 v1.23.1 升级到 v1.51.0，无需切换驱动
- 替代方案 `asg017/sqlite-vec-go-bindings/ncruces` 需要 ncruces/go-sqlite3 WASM 驱动，与现有 modernc.org/sqlite 不兼容

### 关键决策2：vec0 虚拟表使用 L2 距离做 KNN 排序
- vec0 的 KNN `WHERE embedding MATCH ?` 使用 L2（欧几里得）距离排序
- `vec_distance_cosine()` 是独立的 SQL 标量函数，可计算 cosine 距离
- 解决方案：KNN 查询 SELECT 中同时使用 `vec_distance_cosine(embedding, ?)` 获取准确 cosine 距离
- KNN 用 L2 排序筛选候选，cosine 距离用于精确评分和阈值过滤

### 关键决策3：chunk_id → rowid 映射使用 FNV-1a 哈希
- vec0 的 rowid 必须是整数，而 chunkModel.ID 是 string UUID
- 使用 `hash/fnv.New64a()` 将 UUID 映射为 int64，碰撞概率极低
- 映射是确定性的，可从 chunk ID 反推 rowid 用于删除操作

### 关键决策4：vec0 表增加 metadata 列 chunk_id 和 kb_id
- `chunk_id TEXT`：用于 KNN 结果与 chunks 表的 JOIN
- `kb_id TEXT`：用于 KNN 查询中 `WHERE kb_id = ?` 过滤
- metadata 列支持在 KNN 查询的 WHERE 子句中使用等值约束

### 关键决策5：dimension 作为可配置参数（WithDimension 选项）
- vec0 虚拟表在 CREATE TABLE 时固定维度
- 默认 1536（text-embedding-3-small），通过 `WithDimension(d)` 选项可覆盖
- 测试使用不同维度（1-3维），每个测试创建独立 store
- 新增 `opts ...Option` 参数向后兼容，不破坏现有调用
