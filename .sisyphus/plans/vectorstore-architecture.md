# VectorStore 架构重构

## TL;DR

> **快速摘要**：将 KnowledgeStore 中向量搜索逻辑抽出为 `VectorStore` 接口，sqlite-vec 和 brute-force 作为两个实现。KnowledgeStore 通过依赖注入获得 VectorStore，不再 import 任何 SQLite 驱动。初始化时自动校验每个 KB 的元数据与向量数据一致性，不一致的 KB 标记 `available=false`。
>
> **产出物**：
> - `plugins/knowledge-base/vector.go` — VectorStore 接口 + Chunk 值类型
> - `plugins/knowledge-base/store/bruteforce/` — 应用层余弦相似度实现（零依赖）
> - `plugins/knowledge-base/store/sqlitevec/` — 重构为 VectorStore 实现 + KnowledgeStore
>
> **预估工作量**：Medium
> **并行执行**：YES — 2 个 Wave

---

## Context

### 原始需求
当前 KnowledgeStore 将元数据 CRUD 和向量搜索耦合在一个类型中，且 internally imports modernc.org/sqlite（与 glebarez 冲突）。需要：
1. 解耦为 VectorStore 接口，支持可替换实现
2. 插件不 import 任何 SQLite 驱动——连接由外部注入
3. 架构可扩展（未来支持 qdrant 等）
4. 初始化时自动校验数据一致性

### 讨论决策
1. **VectorStore 接口**：含 Store/Search/Delete 能力 + Verify/Bckend 自描述
2. **两个实现**：brute-force（零依赖）、sqlite-vec（需要 vec 扩展的 *sql.DB）
3. **一致性校验**：KnowledgeStore 构造时自动检查，逐 KB 标记 available
4. **连接注入**：通过 `gorm.Open(sqlite.Open(&sqlite.Config{DriverName: "sqlite3", ...}))` 注入，插件不 import 驱动

---

## Work Objectives

### 核心目标
提取 VectorStore 接口，双向实现验证架构，消除驱动冲突。

### Must Have
- VectorStore 接口定义并验证（通过两种实现互换通过测试）
- KnowledgeStore 不 import 任何 SQLite 驱动
- 两个实现均可通过测试
- 一致性校验自动运行，逐 KB 标记

### Must NOT Have
- 改变 KnowledgeStore 接口的公开语义
- 在插件包内 import 具体 SQLite 驱动
- 遗留 dialector.go

---

## Execution Strategy

```
Wave 1（并行 — 接口 + 实现）：
├── Task 1: 定义 VectorStore 接口 + Chunk 值类型 [quick]
├── Task 2: 实现 brute-force VectorStore [deep]
└── Task 3: 重构 sqlitevec 为 VectorStore 实现 [deep]

Wave 2（Wave 1 之后 — 集成 + 验证）：
├── Task 4: KnowledgeStore 使用 VectorStore 注入模式 [deep]
├── Task 5: 初始化一致性校验逻辑 [quick]
├── Task 6: 更新 server/main.go 组装注入链 [quick]
└── Task 7: 全量编译 + 测试验证 [quick]
```

---

## TODOs

- [x] 1. 定义 VectorStore 接口 + Chunk 值类型

  **What to do**：
  - 创建 `plugins/knowledge-base/vector.go`
  - 定义 `VectorChunk` 值类型（不依赖 GORM）：
    ```go
    type VectorChunk struct {
        ID, DocumentID, KBID string
        Content string
        Index   int
        Score   float32
    }
    ```
  - 定义 `VectorStore` 接口：
    ```go
    type VectorStore interface {
        Store(ctx context.Context, kbID, docID string, chunks []VectorChunk, vectors [][]float32) error
        Search(ctx context.Context, kbIDs []string, query []float32, opts SearchOptions) ([]SearchResult, error)
        DeleteByKB(ctx context.Context, kbID string) error
        DeleteByDocument(ctx context.Context, kbID, docID string) error
        Backend() string
        Verify(ctx context.Context) (map[string]int, error) // kbID -> actual chunk count
    }
    type SearchResult struct { ChunkID string; KBID string; Score float32 }
    ```
  - 包名 `package knowledgebase`

  **Must NOT do**：
  - 不包含任何 SQLite 驱动 import

- [x] 2. 实现 brute-force VectorStore

  **What to do**：
  - 创建 `plugins/knowledge-base/store/bruteforce/bfstore.go`
  - 构造函数：`func New(db *gorm.DB) *BruteForceVectorStore`
  - Store：no-op（chunks 表和 vector blob 列由 KnowledgeStore 的 GORM 写入）
  - Search：通过 gorm.DB 加载相关 chunks，fromBlob 解码向量，cosineSimilarity 计算排序
  - DeleteByKB/DeleteByDocument：no-op（GORM cascade 删除）
  - Verify：`SELECT kb_id, COUNT(*) FROM chunks WHERE vector IS NOT NULL GROUP BY kb_id`
  - Backend：`"brute-force"`
  - 包名 `package bruteforce`
  - 从 `sqlitevec/` 复制 `toBlob/fromBlob/cosineSimilarity` 三个函数（或放在 shared util）

  **Must NOT do**：
  - 不 import 任何 SQLite 驱动

- [x] 3. 重构 sqlitevec 为 VectorStore 实现

  **What to do**：
  - 在 `plugins/knowledge-base/store/sqlitevec/` 新增 `vecstore.go`
  - 类型 `SQLiteVecStore struct { sqlDB *sql.DB; dim int }`
  - 构造函数 `func New(sqlDB *sql.DB, dim int) *SQLiteVecStore`
  - 方法：Store（INSERT chunks_vec）、Search（vec0 KNN + vec_distance_cosine）、DeleteByKB/DeleteByDocument、Verify、Backend
  - 从 knowledge.go 中移除 vec 相关代码（initVectorTable、chunkIDToRowID、Search 中的 vec0 SQL、StoreChunks 中的 chunks_vec 写入）
  - 保留 schema.go 中的模型定义（kbModel/docModel/chunkModel 等，KnowledgeStore 仍需要）
  - **删除 dialector.go**

  **Must NOT do**：
  - 不改变 KnowledgeStore 的元数据 CRUD 逻辑

- [x] 4. KnowledgeStore 使用 VectorStore 注入

  **What to do**：
  - 修改 `sqlitevec/knowledge.go`：
    - KnowledgeStore 结构体增加 `vec VectorStore` 字段
    - 删除 `sqlDB *sql.DB` 和 `dimension int` 字段
    - 删除 `Option`/`WithDimension`
    - `NewKnowledgeStore(db *gorm.DB, vec VectorStore) (*KnowledgeStore, error)`
    - `Search()` 委托给 `s.vec.Search()`
    - `StoreChunks()` 中：GORM 写入 chunks 表后，调用 `s.vec.Store()`
    - `DeleteKB()`/`DeleteDocument()` 中：GORM cascade 后，调用 `s.vec.DeleteByKB()/DeleteByDocument()`
    - 删除 `NewKnowledgeStoreFromDSN`
  - 包内 `var _ VectorStore = (*SQLiteVecStore)(nil)` 编译期验证

  **Must NOT do**：
  - 不改变 KnowledgeStore 接口的公开方法签名

- [x] 5. 初始化一致性校验

  **What to do**：
  - 在 `NewKnowledgeStore` 中，auto-migrate 后执行：
    1. 调用 `vec.Verify(ctx)` 获取实际 chunk 统计
    2. `ListKBs(ctx)` 获取所有 KB
    3. 对每个 KB，对比元数据 `doc.ChunkCount` 与实际 `verify[kbID]`
    4. 不一致的 KB：`kb.Config["available"] = false; kb.Config["unavailable_reason"] = "..."` 并 update
  - 保存 KnowledgeBase 时在 Config 中写 `available=true`

  **Must NOT do**：
  - 不一致不阻塞启动（标记即可）

- [x] 6. 更新 server/main.go 组装注入链

  **What to do**：
  - `createKnowledgeStore` 函数改为：
    ```go
    gormDB, _ := gorm.Open(glebarezSqlite.Open(dsn), &gorm.Config{})
    // 根据配置选 VectorStore
    var vec knowledgebase.VectorStore
    if cfg.UseVec {
        sqlDB, _ := gormDB.DB()
        vec = sqlitevec.New(sqlDB, 1536)
    } else {
        vec = bruteforce.New(gormDB)
    }
    ks, _ = sqlitevec.NewKnowledgeStore(gormDB, vec)
    ```
  - 删除 `sqlitevec.NewKnowledgeStoreFromDSN` 的引用

  **Must NOT do**：
  - 不改变 main.go 的启动流程结构

- [x] 7. 全量编译 + 测试验证

  **What to do**：
  - `go build ./plugins/...` `go build ./server/...`
  - `go test ./plugins/...` `go test ./server/...`
  - `go vet ./plugins/...` `go vet ./server/...`
  - 配置一个测试运行两种 VectorStore 实现

  **Commit**：NO（验证步骤）

---

## Final Verification Wave

- [x] F1. 编译 + 测试全部通过（所有 plugins + server）
- [x] F2. 两种 VectorStore 实现可互换且测试通过
- [x] F3. 一致性校验逻辑验证（构造不一致状态，确认标记）

---

## Success Criteria

```bash
go build ./plugins/...      # 零错误
go build ./server/...       # 零错误
go test ./plugins/...       # 全部 PASS
go test ./server/...        # 全部 PASS
go vet ./plugins/...        # 零输出
go vet ./server/...         # 零输出
```

- [x] bruto-force 和 sqlite-vec 两种实现均可通过测试
- [x] KnowledgeStore 代码中零 SQLite 驱动 import
- [x] 初始化时自动校验并标记不一致的 KB