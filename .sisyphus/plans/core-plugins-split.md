# 执行计划：core + plugins 架构分裂

## TL;DR

> **目标**：将当前的单体 core 分解为极简内核 + 独立可插拔的 plugins，每个 plugin 自包含，可被 server 显式组装。
> 
> **原则**：只做移动和切分，不动语义，不修架构问题。
> 
> **关键决策**：
> - Todo 保留在 core（内置工具）
> - Embedding 整个移到 plugins
> - Capability Registry 改为实例化（去全局状态）
> - Harness 不再自动创建 store，由 server 显式注入
>
> **Estimated Effort**: 15-20 小时（2-3 天）
> **Parallel Execution**: NO（严格串行）

---

## Context

### 背景

当前 core/ 包同时包含：
1. Agent 执行引擎（核心逻辑）
2. 存储实现（postgres、sqlite、sqlitevec、filememory）
3. 业务 capability（memory hooks、kb hooks）
4. 独立能力（rag、eval、embedding）
5. 全局状态（Capability Registry `builtins sync.Map`）

这导致 core 无法被独立复用——引入 core 就等于引入所有存储实现和业务概念。

### 目标架构

```
core/           ← 极简引擎 + 接口 + OpenAI adapter + 内置通用 capability
plugins/        ← 7 个独立 plugin，每个自包含
server/         ← Demo 应用，显式组装 core + plugins
```

### 关键决策

| 决策 | 结论 |
|------|------|
| go.mod 策略 | 共享仓库根 go.mod（所有包共用） |
| Capability Registry | 实例化，去全局状态 |
| OpenAI adapter | 保留在 core/llm/ |
| Embedding | **整个**移到 plugins/embedding-openai/ |
| Todo | 保留在 core（内置工具） |
| 迁移方式 | 一次性全部完成 |

---

## Work Objectives

### Core Objective

将 core/ 目录分解为"引擎内核"和"可插拔扩展"，建立清晰的模块边界，使 core 可被独立使用而不引入任何存储实现或业务概念。

### Concrete Deliverables

1. **core/** 精简为：引擎 + 接口 + OpenAI adapter + 内置 capability
2. **plugins/** 包含 7 个独立 plugin：
   - `storage-postgres`（Postgres StoreProvider）
   - `storage-sqlite`（SQLite StoreProvider）
   - `memory-file`（FileMemoryStore + Hook + 3 Tools + capability）
   - `knowledge-base`（KnowledgeStore 接口 + sqlitevec 实现 + Hook + capability）
   - `embedding-openai`（Embedder 接口 + OpenAI 实现 + 工厂）
   - `rag`（Parser + Chunker + Pipeline）
   - `eval`（Metrics + Golden Set + CI）
3. **StoreProvider** 简化为 `Sessions() + Messages() + Todos()` 三个方法
4. **Capability Registry** 改为实例化（无全局状态）
5. **Harness** 移除所有自动创建 store 的逻辑
6. **Server** 改为显式组装 plugin 实例
7. **构建通过**：`core/`、所有 `plugins/`、`server/`、`packages/demo/`

### Definition of Done

- [x] `cd core && go build ./...` 成功
- [x] `cd plugins/storage-postgres && go build ./...` 成功
- [x] `cd plugins/storage-sqlite && go build ./...` 成功
- [x] `cd plugins/memory-file && go build ./...` 成功
- [x] `cd plugins/knowledge-base && go build ./...` 成功
- [x] `cd plugins/embedding-openai && go build ./...` 成功
- [x] `cd plugins/rag && go build ./...` 成功
- [x] `cd plugins/eval && go build ./...` 成功
- [x] `cd server && go build ./...` 成功
- [x] `pnpm --filter @copcon/demo build` 成功
- [x] 所有新路径下的 `go test ./...` 通过

### Must Have

- ✅ core 不再 import 任何 plugins 下的包
- ✅ 每个 plugin 不依赖其他 plugin
- ✅ Capability Registry 无全局 sync.Map
- ✅ StoreProvider 接口只有 Sessions/Messages/Todos
- ✅ Harness 不再自动创建任何 store 实例

### Must NOT Have

- ❌ **不能**在 core/ 的 import 中出现 plugins/ 路径
- ❌ **不能**改变任何现有语义
- ❌ **不能**修改测试逻辑（仅修 import）
- ❌ **不能**删除或重命名任何 public 方法签名
- ❌ **不能**引入 plugin 间的交叉依赖

---

## Execution Strategy

### 严格串行执行

> 这是一次性分裂重构，严格串行执行。每个步骤完成并验证后才进入下一步。

```
Step 1: 创建目录结构
Step 2: 移动文件
Step 3: 修 import
Step 4: 重构 Capability Registry（全局 → 实例化）
Step 5: 精简 Harness（去掉自动创建 store 的逻辑）
Step 6: 精简 StoreProvider（只保留 Sessions/Messages/Todos）
Step 7: 改造 Server（显式组装 plugin 实例）
Step 8: 全局 import 修复 + 构建验证
Step 9: 测试验证 + bug 修复
Step 10: Commit
```

---

## TODOs

### Step 1: 创建 plugins 目录结构

- [x] 1. 创建 `plugins/` 顶层目录

  **What to do**：
  ```bash
  mkdir -p plugins/storage-postgres
  mkdir -p plugins/storage-sqlite
  mkdir -p plugins/memory-file
  mkdir -p plugins/knowledge-base/sqlitevec
  mkdir -p plugins/embedding-openai
  mkdir -p plugins/rag
  mkdir -p plugins/eval
  ```

  **Acceptance Criteria**：
  - 7 个 plugin 目录存在，每个为空
  - `plugins/` 目录结构正确

---

### Step 2: 移动文件

> 使用 `git mv` 移动文件，保留 git 历史。

#### 2a. 移动 postgres provider

- [x] 2a. 将 `core/providers/postgres/` 所有文件（含测试）移到 `plugins/storage-postgres/`

  **文件列表**：
  - `core/providers/postgres/*.go` → `plugins/storage-postgres/*.go`

  **What to do**：
  ```bash
  git mv core/providers/postgres/convert.go plugins/storage-postgres/
  git mv core/providers/postgres/doc.go plugins/storage-postgres/
  git mv core/providers/postgres/message.go plugins/storage-postgres/
  git mv core/providers/postgres/models.go plugins/storage-postgres/
  git mv core/providers/postgres/session.go plugins/storage-postgres/
  git mv core/providers/postgres/store.go plugins/storage-postgres/
  git mv core/providers/postgres/todo.go plugins/storage-postgres/
  ```
  - After move: update `package postgres` → keep as `package postgres`
  - Remove `Knowledge()` method from store.go (no longer in StoreProvider)

#### 2b. 移动 sqlite provider

- [x] 2b. 将 `core/providers/sqlite/` 所有文件（含测试）移到 `plugins/storage-sqlite/`

  **What to do**：
  ```bash
  git mv core/providers/sqlite/*.go plugins/storage-sqlite/
  ```
  - After move: keep `package sqlite`
  - Remove `Knowledge()` method from store.go

#### 2c. 移动 filememory 和 memory hooks/tools

- [x] 2c. 将 Memory 相关所有文件移到 `plugins/memory-file/`

  **文件列表**：
  ```
  core/providers/filememory/*.go         → plugins/memory-file/
  core/storage/memory.go                 → plugins/memory-file/store_interface.go
  core/capabilities/hooks/file_memory.go             → plugins/memory-file/
  core/capabilities/hooks/file_memory_capability.go  → plugins/memory-file/
  core/capabilities/hooks/file_memory_test.go        → plugins/memory-file/
  core/capabilities/hooks/memory.go                  → plugins/memory-file/
  core/capabilities/hooks/memory_types.go            → plugins/memory-file/
  core/capabilities/tools/memory_store.go            → plugins/memory-file/
  core/capabilities/tools/memory_store_capability.go → plugins/memory-file/
  core/capabilities/tools/memory_recall.go           → plugins/memory-file/
  core/capabilities/tools/memory_recall_capability.go → plugins/memory-file/
  core/capabilities/tools/memory_forget.go           → plugins/memory-file/
  core/capabilities/tools/memory_forget_capability.go → plugins/memory-file/
  core/capabilities/tools/memory_test.go             → plugins/memory-file/
  ```
  - After move: keep package names as `filememory`, `hooks`, `tools`
  - OR unify into a single `package memoryfile`
  **Decision**: Move physical files, fix imports, keep package names separate for now.

#### 2d. 移动 knowledge base

- [x] 2d. 将 knowledge 相关所有文件移到 `plugins/knowledge-base/`

  **文件列表**：
  ```
  core/storage/knowledge.go             → plugins/knowledge-base/store_interface.go
  core/providers/sqlitevec/*.go         → plugins/knowledge-base/sqlitevec/
  core/capabilities/hooks/kb_recall.go                 → plugins/knowledge-base/
  core/capabilities/hooks/kb_recall_capability.go      → plugins/knowledge-base/
  core/capabilities/hooks/memory_persist.go            → plugins/knowledge-base/
  core/capabilities/hooks/memory_persist_capability.go → plugins/knowledge-base/
  core/capabilities/hooks/knowledge_integration_test.go → plugins/knowledge-base/
  ```
  - After move: keep `package sqlitevec` for sqlitevec files
  - hooks files: keep as `package hooks` or decide to unify

#### 2e. 移动 embedding

- [x] 2e. 将 `core/providers/embedding/` 所有文件移到 `plugins/embedding-openai/`

  **文件列表**：
  ```
  core/providers/embedding/*.go     → plugins/embedding-openai/
  ```
  - After move: keep as `package embedding`

#### 2f. 移动 rag

- [x] 2f. 将 `core/rag/` 所有文件移到 `plugins/rag/`

  **文件列表**：
  ```
  core/rag/*.go     → plugins/rag/
  ```
  - After move: keep as `package rag`

#### 2g. 移动 eval

- [x] 2g. 将 `core/eval/` 所有文件移到 `plugins/eval/`

  **文件列表**：
  ```
  core/eval/*.go     → plugins/eval/
  ```
  - After move: keep as `package eval`
  - 同时移动 `eval/testdata/` 目录

#### 2h. 清理 core/providers/

- [x] 2h. 删除空的 `core/providers/` 目录

  `core/providers/` 应该整体删除（所有子目录已移走）。

---

### Step 3: 修复 import（所有移动过的文件）

> 这一步分 3 个子步骤（按依赖顺序）。

#### 3a. 修复 plugins 内部的 import

- [x] 3a. 修复所有 plugins 内部文件的 import 路径

  核心替换：
  ```
  "github.com/copcon/core/providers/postgres"       → "github.com/copcon/plugins/storage-postgres"
  "github.com/copcon/core/providers/sqlite"         → "github.com/copcon/plugins/storage-sqlite"
  "github.com/copcon/core/providers/sqlitevec"      → "github.com/copcon/plugins/knowledge-base/sqlitevec"
  "github.com/copcon/core/providers/filememory"     → "github.com/copcon/plugins/memory-file"
  "github.com/copcon/core/providers/embedding"      → "github.com/copcon/plugins/embedding-openai"
  "github.com/copcon/core/rag"                      → "github.com/copcon/plugins/rag"
  "github.com/copcon/core/eval"                     → "github.com/copcon/plugins/eval"
  "github.com/copcon/core/storage"                  → (keep - core package)
  "github.com/copcon/core/capabilities"             → (keep - core package)
  ```

  Also fix `core/storage/memory.go` → `plugins/memory-file/store_interface.go`:
  - Package becomes `package memoryfile` or stays in a sub-package
  - All files that import `github.com/copcon/core/storage` for `MemoryStore` → import `github.com/copcon/plugins/memory-file`

  And `core/storage/knowledge.go` → `plugins/knowledge-base/store_interface.go`:
  - Same pattern

#### 3b. 修复 core 内部的 import

- [x] 3b. 修复 core/ 中引用已移走包的 import

  **受影响的 core 文件**（非测试）：
  - `core/harness.go` — import filememory, sqlitevec, embedding
  - `core/capabilities/hooks/logging.go` — may import memory types
  - `core/capabilities/hooks/tracing.go` — may import memory types
  - `core/capabilities/registry.go` — has `MemoryStore`, `KnowledgeStore`, `FileMemoryStore`, `Embedder` in `CapabilityDeps`

  **受影响的测试文件**：
  - `core/harness_test.go`
  - `core/harness_integration_test.go`

  For `CapabilityDeps`, these fields should change from typed `interface{}` or typed references to `interface{}` (to avoid circular imports):
  ```go
  type CapabilityDeps struct {
      // ...keep SessionStore, MessageStore, TodoStore, Logger
      FileMemoryStore     interface{} // was: interface{} — stays
      KnowledgeStore      interface{} // was: interface{} — stays  
      Embedder            interface{} // was: interface{} — stays
      MemoryStore         interface{} // was: storage.MemoryStore → interface{}
      AgentKnowledgeBases map[string][]string   // stays
      AgentRegistry       agent.AgentRegistry   // stays
      Engine              interface{}           // stays
  }
  ```

#### 3c. 修复 server 的 import

- [x] 3c. 修复 server/ 中引用 core 已移走包的 import

  **server 文件**：
  - `server/cmd/server/main.go` — import postgres/sqlite store
  - `server/internal/api/handlers.go` — import embedding, rag
  - `server/internal/api/knowledge.go` — import core storage types
  - `server/internal/api/knowledge_options.go` — import embedding, rag
  - `server/internal/store/` — import postgres/sqlite

  替换规则同 3a。

---

### Step 4: 重构 Capability Registry（全局 → 实例化）

- [x] 4. 将 `core/capabilities/registry.go` 的全局 `sync.Map` 改为实例化 Registry

  **What to do**：

  **4a.** 重写 `registry.go`：
  ```go
  type Registry struct {
      builtins map[string]Capability
      mu       sync.RWMutex
  }

  func NewRegistry() *Registry {
      return &Registry{builtins: make(map[string]Capability)}
  }

  func (r *Registry) Register(c Capability) error { ... }
  func (r *Registry) Get(name string) (Capability, bool) { ... }
  func (r *Registry) ListByType(t CapabilityType) []Capability { ... }
  func (r *Registry) ExpandWildcards(names []string) []string { ... }
  func (r *Registry) ResolveDependencies(names []string) ([]Capability, error) { ... }
  ```

  **4b.** 每个 plugin 添加显式注册函数：
  ```go
  // plugins/memory-file/register.go
  package memoryfile
  func RegisterCapabilities(r *capabilities.Registry) error { ... }

  // plugins/knowledge-base/register.go  
  package knowledgebase
  func RegisterCapabilities(r *capabilities.Registry) error { ... }
  ```

  **4c.** 更新 `core/harness.go`：
  - `HarnessConfig` 新增 `Registry *capabilities.Registry`
  - `Build()` 使用配置的 Registry 而不是隐式获取

  **4d.** 更新 `server/main.go`：
  ```go
  registry := capabilities.NewRegistry()
  memoryfile.RegisterCapabilities(registry)
  knowledgebase.RegisterCapabilities(registry)
  // ...
  h := core.NewHarness(core.HarnessConfig{
      Registry: registry,
      // ...
  })
  ```

  **Acceptance Criteria**：
  - `core/capabilities/registry.go` 中没有 `var builtins sync.Map`
  - 所有 plugin 导出了 `RegisterCapabilities(*capabilities.Registry) error`
  - `HarnessConfig` 有 `Registry` 字段

---

### Step 5: 精简 Harness（去掉自动创建 store）

- [x] 5. 移除 `core/harness.go` 中所有自动创建 store/logic 的代码

  **What to do**：删除以下代码块：

  **5a.** 删除 `initStores()` 中的 FileMemory 自动创建：
  ```go
  // 删除这段：
  if anyMemoryEnabled && h.config.Store.FileMemory == nil {
      fm, err := filememory.NewFileMemoryStore(...)
      h.config.Store.FileMemory = fm
  }
  ```

  **5b.** 删除 `initStores()` 中的 KnowledgeStore 自动创建：
  ```go
  // 删除这段：
  if h.config.Store.KnowledgeStore == nil {
      ks, err := sqlitevec.NewKnowledgeStoreFromDSN(":memory:")
      h.config.Store.KnowledgeStore = ks
  }
  ```

  **5c.** 删除 `initStores()` 中的 Embedder 自动创建：
  ```go
  // 删除这段：
  if h.config.Store.Embedder == nil && h.config.EmbeddingConfig.Backend != "" {
      emb, err := embedding.NewFromConfig(...)
      h.config.Store.Embedder = emb
  }
  ```

  **5d.** 删除未使用的 import：
  - `"github.com/copcon/plugins/memory-file"` (原 filememory)
  - `"github.com/copcon/plugins/knowledge-base/sqlitevec"`
  - `"github.com/copcon/plugins/embedding-openai"`

  **5e.** 清理未使用的变量：
  - `anyMemoryEnabled`, `anyKBReferenced`, `agentKBs`

  **5f.** 移除 `Harness.EmbeddingConfig` 字段

  **Acceptance Criteria**：
  - harpess.go 中只有 `go build` 验证
  - StoreConfig 的 `FileMemory`、`KnowledgeStore`、`Embedder` 字段保留，但只由 server 显式设置

---

### Step 6: 精简 StoreProvider

- [x] 6. 将 `StoreProvider` 简化为三个方法

  **What to do**：
  ```go
  // core/storage/provider.go
  type StoreProvider interface {
      Sessions() SessionStore
      Messages() MessageStore
      Todos()    TodoStore
  }
  ```
  - 移除 `Knowledge() KnowledgeStore`
  - 移除对应的 import (`core/storage/knowledge.go` 不存在了)

  **impacted files**：
  - `plugins/storage-postgres/store.go` — 删除 `Knowledge()` 方法
  - `plugins/storage-sqlite/store.go` — 删除 `Knowledge()` 方法
  - `core/harness.go` 中的 `quickStoreProvider` — 删除 `Knowledge()` 方法
  - `core/harness_test.go` 中的 `testStoreProvider` — 删除 `Knowledge()` 方法
  - `server/internal/api/knowledge_test.go` 中的 `kbTestStoreProvider` — 可能需要调整

  **Acceptance Criteria**：
  - `core/storage/provider.go` 只包含 3 个方法
  - 所有 StoreProvider 实现者编译通过

---

### Step 7: 改造 Server API

- [x] 7. 改造 `server/internal/api/handlers.go` 和 `server/cmd/server/main.go`

  **What to do**：

  **7a.** Handler struct 改变化：
  ```go
  type Handler struct {
      config         *config.Config
      sessionStore   storage.SessionStore
      messageStore   storage.MessageStore
      todoStore      storage.TodoStore
      knowledgeStore storage.KnowledgeStore     // ← 直接持有
      fileMemoryStore *FileMemoryStore           // ← 直接持有
      embedder       embedding.Embedder         // ← 直接持有
      ragPipeline    *rag.Pipeline              // ← 直接持有
      // ...
  }
  ```
  - `knowledgeStore` 不再从 `h.Store().Knowledge()` 获取，改为直接注入
  - 添加 `WithKnowledgeStore`, `WithFileMemory`, `WithEmbedder`, `WithRAGPipeline` options

  **7b.** Server main.go 改为显式组装：
  ```go
  // 创建 store
  storeProvider := stor.CreateStoreProvider(cfg.Database)
  
  // 创建 fileMemoryStore
  var fmStore *memoryfile.FileMemoryStore
  for _, a := range cfg.Agents {
      if a.Memory.Enabled {
          fmStore, _ = memoryfile.NewFileMemoryStore(...)
          break
      }
  }
  
  // 创建 knowledgeStore
  var ks storage.KnowledgeStore
  if cfg.KnowledgeBaseEnabled {
      ks, _ = sqlitevec.NewKnowledgeStoreFromDSN(cfg.KnowledgeBaseConfig.StoragePath)
  }
  
  // 创建 embedder
  var emb embedding.Embedder
  if cfg.KnowledgeBaseEnabled {
      emb, _ = embedding.NewFromConfig(...)
  }
  
  // 创建 harness
  h := core.NewHarness(core.HarnessConfig{
      Registry: registry,
      Store: core.StoreConfig{
          Provider:       storeProvider,
          Memory:         nil,
          FileMemory:     fmStore,       // 显式注入
          KnowledgeStore: ks,            // 显式注入
          Embedder:       emb,           // 显式注入
      },
      LLM:    llm.NewOpenAIAdapter(&cl, cfg.OpenAI.Model),
      Logger: log,
      Agents: agentSpecs(cfg),
  })
  
  // 创建 handler
  var apiOpts []api.HandlerOption
  if fmStore != nil { apiOpts = append(apiOpts, api.WithMemoryStore(fmStore)) }
  if ks != nil { apiOpts = append(apiOpts, api.WithKnowledgeStore(ks)) }
  if emb != nil { apiOpts = append(apiOpts, api.WithEmbedder(emb)) }
  if ks != nil && emb != nil {
      pipeline := rag.NewPipeline(...)
      apiOpts = append(apiOpts, api.WithRAGPipeline(pipeline))
  }
  
  api.SetupRoutes(r, cfg, h, apiOpts...)
  ```

  **Acceptance Criteria**：
  - `server` build 成功
  - API handler 直接持有 store instance，不通过 harness 间接获取

---

### Step 8: 全局 import 修复 + 构建验证

- [x] 8. 运行全量构建，修复所有编译错误

  **What to do**：
  ```bash
  cd /data/copcon && go build ./core/... 2>&1        # 修复所有错误
  cd /data/copcon && go build ./plugins/... 2>&1     # 修复所有错误
  cd /data/copcon && go build ./server/... 2>&1      # 修复所有错误
  cd /data/copcon && pnpm --filter @copcon/demo build 2>&1
  ```
  重复修复直到所有构建通过。

  **Acceptance Criteria**：
  - 四条构建命令全部 0 error
  - `grep -rn "core/providers/" plugins/` 返回空
  - `grep -rn "core/rag" core/` 返回空
  - `grep -rn "core/eval" core/` 返回空

---

### Step 9: 测试验证 + bug 修复

- [x] 9. 运行全量测试，修复所有失败

  **What to do**：
  ```bash
  cd /data/copcon && go test -count=1 ./core/... 2>&1
  cd /data/copcon && go test -count=1 ./plugins/... 2>&1
  cd /data/copcon && go test -count=1 ./server/internal/... 2>&1
  cd /data/copcon && pnpm --filter @copcon/chat-core test 2>&1
  ```
  逐 packge 修复测试失败。

  **Acceptance Criteria**：
  - All `go test ./core/...` 通过
  - All `go test ./plugins/...` 通过
  - All `go test ./server/internal/...` 通过
  - `pnpm --filter @copcon/chat-core test` 54/54 通过

---

### Step 10: 最终提交

- [x] 10. 一个原子 commit

  **Commit message**：
  ```
  refactor: split core into minimal engine + pluggable capability plugins
  
  - core/ reduced to engine + interfaces + built-in capabilities
  - plugins/ contains 7 independent plugins: 
    storage-postgres, storage-sqlite, memory-file, knowledge-base,
    embedding-openai, rag, eval
  - Capability Registry changed from global state to instance-based
  - StoreProvider interface simplified to Sessions() + Messages() + Todos()
  - Harness no longer auto-creates FileMemory/KnowledgeStore/Embedder
  - Server explicitly assembles plugin instances
  
  All existing functionality preserved — no semantic changes.
  ```

---

## Commit Strategy

- **1 个原子 commit**：包含所有文件移动、import 修复、架构重构
- 在 `step 10` 提交前验证构建 + 测试全部通过

---

## Success Criteria

### Build Verification

```bash
cd core && go build ./...
go build ./plugins/storage-postgres/...
go build ./plugins/storage-sqlite/...
go build ./plugins/memory-file/...
go build ./plugins/knowledge-base/...
go build ./plugins/embedding-openai/...
go build ./plugins/rag/...
go build ./plugins/eval/...
cd server && go build ./...
pnpm --filter @copcon/demo build
```

### Final Checklist

- [x] core/ 的 import 中不包含任何 plugins/ 路径
- [x] Capability Registry 无全局 sync.Map
- [x] StoreProvider 只有 3 个方法
- [x] Harness 不自动创建 store
- [x] Server 显式组装 plugin 实例
- [x] 所有构建通过
- [x] 所有测试通过
