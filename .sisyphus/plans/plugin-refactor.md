# 插件系统重构

## TL;DR

> **核心目标**：去掉 capabilities.Registry 中间层，改为 `h.Register(plugin)` 直连模式。插件是持有自身依赖的一等公民对象，Harness 直接提取其 Tools/Hooks 注入全局池。
> 
> **交付物**：
> - `core/plugin/` — Plugin 接口 + Pool + 内置插件
> - 4 个现有插件适配为新 Plugin 接口（memory-file, skill, knowledge-base, mcp）
> - 删除 ~600 行冗余代码（11 个 Capability struct、Registry、拓扑排序、通配符展开等）
> - Agent 配置支持 `namespace.*` 批量选中
> 
> **预计规模**：Large
> **并行执行**：YES — 5 波次，最大并行 4
> **关键路径**：Task 1 → Task 3 → Task 5 → Task 8 → Task 9
>
> **策略**：绞杀者模式（strangler fig）——新系统与旧系统并存，逐步迁移，最后删除旧代码。

---

## 上下文

### 原始需求
用户认为 capabilities 体系冗余且设计方向错误：
1. 插件应该自己注入能力到 core，而不是 core 通过 Capability 查询插件
2. 插件的 key（如 `modules.memory_file`）定义在 core 包里不合理——core 不应该知道有哪些插件
3. `agentSpecs()` 硬编码了每个插件的条件逻辑，每加一个插件都要改 main.go
4. Capability 两层命名（`tools.code_executor` + `code_executor`）让用户困惑

### 目标架构
```
plugin := memoryfile.NewPlugin(store, llm)     // 插件是一等公民
h := core.NewHarness(config)
h.Register(plugin)                               // 直接注入，无需 Registry
agent config: tools: ["memory.*"]                // 按命名空间批量选中
```

### Metis 审查
**已识别的关键问题**：
- delegate 工具需要 `Engine` + `AgentRegistry`，这两者在 Register 时还不存在，所以必须保留**两阶段注册**：Register 时只存 Plugin 引用，Build 时调用 `Init(deps)` 完成延迟初始化
- 三个文件的类型需要先迁移才能删除 capabilities 包：`skill/types.go`、`tools/helpers.go`、`tools/todo.go` 中的 TodoManager
- 绞杀者模式（strangler fig）：新旧系统并存，逐步迁移
- 两个已有 bug（ReadSubSessionTool 未注册、asyncCapability 只注册 1/4 工具）本次不修，单独 issue 跟踪

---

## 工作目标

### 核心目标
去掉 capabilities.Registry 中间层，改为 Plugin 直连 Harness 的注册模式。

### 具体交付物
- `core/plugin/plugin.go` — Plugin 接口定义
- `core/plugin/pool.go` — ToolPool + HookPool 全局注册池
- `core/plugin/builtin.go` — builtin.Plugin 归集所有内置工具/钩子
- `plugins/memory-file/plugin.go` — NewPlugin() 实现 Plugin 接口
- `plugins/knowledge-base/plugin.go` — NewPlugin() 实现 Plugin 接口
- `plugins/skill/plugin.go` — NewPlugin() 实现 Plugin 接口
- `plugins/mcp/` — 适配为 Plugin 接口
- `server/cmd/server/main.go` — 新的注册流程
- 删除 `core/capabilities/` 下的冗余代码
- 删除 `core/harness.go` 中的 Capability 相关逻辑

### 完成标准
- [ ] `cd core && go test ./plugin/... -v -race` → PASS
- [ ] `cd plugins && go test ./... -v -race` → PASS（所有插件测试通过）
- [ ] `cd server && go build ./...` → PASS
- [ ] 现有 harness_test.go 所有测试通过
- [ ] Agent 可通过 `memory.*` 批量选中插件工具
- [ ] 前端 Memory/Knowledge 管理页面不受影响

### 必须包含
- Plugin 接口：Name() + Tools() + Hooks() + Init(deps)
- ToolPool + HookPool 全局注册池
- builtin.Plugin 归集内置工具
- 4 个现有插件适配 Plugin 接口
- 绞杀者模式：新旧并存，逐步删除
- Agent 配置支持命名空间通配符 `namespace.*`

### 必须排除
- 修复 ReadSubSessionTool / asyncCapability 已有 bug
- 修改 tool.Tool / hook.Hook 接口
- 修改前端任何代码
- 修改前端 Memory/Knowledge 管理 API
- OAuth / Resources / Prompts / Sampling

---

## 验证策略

### 测试决策
- **基础设施存在**：YES
- **自动化测试**：TDD（新代码先写测试）
- **框架**：`go test -race`

### QA 策略
每个任务包含 Agent 可执行的 QA 场景。证据保存到 `.sisyphus/evidence/`。

---

## 执行策略

### 并行执行波次

```
Wave 1（立即开始 — 新系统基础，MAX PARALLEL）：
├── Task 1: Plugin 接口 + ToolPool/HookPool [deep]
├── Task 2: builtin.Plugin 归集内置工具/钩子 [deep]
└── Task 3: 迁移依赖类型（helpers/skill-types）到独立包 [quick]

Wave 2（Wave 1 完成后 — 适配现有插件，MAX PARALLEL）：
├── Task 4: memory-file 适配 Plugin 接口 [deep]
├── Task 5: knowledge-base 适配 Plugin 接口 [deep]
├── Task 6: skill 适配 Plugin 接口 [deep]
└── Task 7: mcp 适配 Plugin 接口 [quick]

Wave 3（Wave 2 完成后 — Harness 集成）：
├── Task 8: Harness.Register() + Harness.Build() 改造 [deep]
└── Task 9: main.go 新注册流程 + agentSpecs 简化 [deep]

Wave 4（Wave 3 完成后）：
└── Task 10: 删除旧 capabilities 代码 [quick]

Wave FINAL（所有任务完成后 — 4 个并行审查）：
├── Task F1: 计划合规审计（oracle）
├── Task F2: 代码质量审查（unspecified-high）
├── Task F3: 实际 QA 执行（unspecified-high）
└── Task F4: 范围保真度检查（deep）
```

**关键路径**：Task 1 → Task 4 → Task 8 → Task 10
**最大并发**：4（Wave 2）

---

## TODOs

- [x] 1. Plugin 接口 + ToolPool/HookPool 全局注册池

  **要做什么**：
  - 创建 `core/plugin/plugin.go`
  - 定义 `Plugin` 接口：
    ```go
    type Plugin interface {
        Name() string
        Tools() []tool.Tool
        Hooks() []hook.Hook
        Init(deps PluginDeps) error  // 两阶段：Register 时不调用，Build 时调用
    }
    ```
  - 定义 `PluginDeps` 结构体（从 CapabilityDeps 迁移，去掉不需要的）：
    ```go
    type PluginDeps struct {
        SessionStore        storage.SessionStore
        MessageStore        storage.MessageStore
        TodoStore           storage.TodoStore
        AgentRegistry       agent.AgentRegistry
        Engine              interface{}
        Logger              *slog.Logger
        AgentKnowledgeBases map[string][]string
    }
    ```
  - 创建 `core/plugin/pool.go`
  - 实现 `ToolPool`：`Register(t tool.Tool)`、`Get(name string)`、`Select(names []string) tool.ToolManager`
  - 实现 `HookPool`：`Register(h hook.Hook)`、`All() []hook.Hook`
  - 支持通配符匹配：`Select` 接受 `"namespace.*"` 格式，匹配前缀
  - **先写测试**：`pool_test.go` 测试 Select 的通配符匹配

  **禁止做**：
  - 不要删除任何现有代码（绞杀者模式先建新的）
  - 不要修改 tool.Tool 或 hook.Hook 接口

  **推荐 Agent Profile**：
  - **Category**: `deep` — 新接口设计 + 通配符匹配 + 池管理逻辑

  **并行化**：
  - **可并行执行**：YES（Wave 1，与 Task 2, 3 并行）
  - **阻塞**：Task 4, 5, 6, 7, 8
  - **被阻塞**：无

  **参考**：
  - `core/tool/manager.go` — ToolManager 接口
  - `core/hook/hook.go` — Hook 接口
  - `core/capabilities/registry.go:70-79` — CapabilityDeps 结构体（迁移基础）

  **验收标准**：
  - [ ] Plugin 接口编译通过
  - [ ] ToolPool.Select(["memory.*"]) 正确匹配 `memory.tool.xxx` 前缀
  - [ ] ToolPool.Select(["builtin.*", "memory.tool.memory_store"]) 混合匹配正确
  - [ ] pool_test.go 所有测试通过（至少 5 个场景）

  **QA 场景**：
  ```
  Scenario: ToolPool 通配符匹配正确
    Tool: Bash (go test)
    Steps:
      1. 注册 3 个 memory 工具（memory.tool.a, memory.tool.b, memory.tool.c）
      2. 注册 2 个 mcp 工具（mcp.tool.x, mcp.tool.y）
      3. pool.Select(["memory.*"]) 返回 3 个工具
      4. pool.Select(["mcp.*"]) 返回 2 个工具
    Expected Result: 返回正确数量和名称的工具
    Evidence: .sisyphus/evidence/task-1-pool.txt
  ```

  **提交**：YES — `feat(plugin): add Plugin interface and ToolPool/HookPool`

---

- [x] 2. builtin.Plugin 归集所有内置工具/钩子

  **要做什么**：
  - 创建 `core/plugin/builtin.go`
  - 实现 `builtin.Plugin`：
    - `Name()` → `"builtin"`
    - `Tools()` → 返回所有内置工具：`CodeExecutor`, `ShellExecutor`, `FileOps`, `TodoTool`, `ConfirmActionTool`, `AskUserTool`, `GetToolStatusTool`, `DelegateToTool`, `ReadSubSessionTool`
    - `Hooks()` → 返回所有内置钩子：`LoggingPlugin`, `TodoInjectionHook`, `TracingPlugin`
    - `Init(deps)` → 为需要延迟初始化的工具注入依赖（delegate 需要 Engine，todolist 需要 TodoStore）
  - 工具使用统一命名：`builtin.tool.code_executor`、`builtin.tool.shell_executor` 等
  - 钩子使用统一命名：`builtin.hook.logging`、`builtin.hook.todo_injection`、`builtin.hook.tracing`
  - **先写测试**：`builtin_test.go`

  **禁止做**：
  - 不要修改现有工具/钩子的实现代码
  - 不要删除 Capability wrapper struct（Task 10 处理）
  - 不要修复 asyncCapability 只注册 1/4 工具的 bug

  **推荐 Agent Profile**：
  - **Category**: `deep` — 需要理解 8 个内置工具 + 3 个内置钩子的构造方式

  **并行化**：
  - **可并行执行**：YES（Wave 1，与 Task 1, 3 并行）
  - **阻塞**：Task 8
  - **被阻塞**：无

  **参考**：
  - `core/capabilities/tools/register.go` — 现有注册顺序
  - `core/capabilities/hooks/register.go` — 现有钩子注册
  - `core/capabilities/tools/delegate.go` — DelegateToTool 需要 deps.Engine
  - `core/capabilities/tools/todo.go` — TodoTool 需要 deps.TodoStore
  - `core/harness.go:66-75` — builtInTools 和 builtInHooks 列表

  **验收标准**：
  - [ ] builtin.Plugin.Tools() 返回 9 个工具（含 delegate + read_sub_session）
  - [ ] builtin.Plugin.Hooks() 返回 3 个钩子
  - [ ] 所有工具名格式为 `builtin.tool.xxx`
  - [ ] 所有钩子名格式为 `builtin.hook.xxx`
  - [ ] delegate 工具的 Init(deps) 正确注入 Engine + AgentRegistry

  **QA 场景**：
  ```
  Scenario: Init 后 delegate 工具可正常使用
    Tool: Bash (go test)
    Steps:
      1. plugin := builtin.Plugin{}
      2. plugin.Tools() 返回 9 个工具（此时 delegate 的 engine 为 nil）
      3. plugin.Init(deps{Engine: mockEngine, ...})
      4. 再次获取 delegate 工具，验证 engine 不为 nil
    Expected Result: Init 前后 delegate 工具的 engine 字段变化正确
    Evidence: .sisyphus/evidence/task-2-builtin.txt
  ```

  **提交**：YES — `feat(plugin): add builtin.Plugin consolidating all built-in tools/hooks`

---

- [x] 3. 迁移依赖类型到独立包

  **要做什么**：
  - `core/capabilities/tools/helpers.go` 中的 `successResult`/`errorResult` → 移入 `core/tool/helpers.go`
  - `core/capabilities/skill/types.go` 中的 `Skill` 类型 → 保持原位置，或移入 `plugins/skill/`
  - 更新所有引用这些类型的 import 路径
  - `core/capabilities/tools/todo.go` 中的 `TodoManager` 接口 → 移入 `core/tool/todo.go`
  - 确保所有现有测试仍通过

  **禁止做**：
  - 不要修改任何类型的逻辑
  - 不要删除 capabilities 包中的原始文件（Task 10 处理）

  **推荐 Agent Profile**：
  - **Category**: `quick` — 纯文件移动 + import 更新

  **并行化**：
  - **可并行执行**：YES（Wave 1，与 Task 1, 2 并行）
  - **阻塞**：Task 4, 5, 6, 7（插件需要引用新路径）
  - **被阻塞**：无

  **参考**：
  - `core/capabilities/tools/helpers.go:1-20`
  - `core/capabilities/skill/types.go`
  - `core/capabilities/tools/todo.go:13-43` — TodoManager 接口

  **验收标准**：
  - [ ] `go build ./...` 在 core/ 和 plugins/ 下通过
  - [ ] 所有现有测试通过
  - [ ] 无循环依赖

  **QA 场景**：
  ```
  Scenario: 迁移后所有测试仍通过
    Tool: Bash
    Steps:
      1. cd core && go test ./... -count=1
      2. cd plugins && go test ./... -count=1
    Expected Result: 所有测试 PASS，exit code 0
    Evidence: .sisyphus/evidence/task-3-migration.txt
  ```

  **提交**：YES — `refactor: move helpers and types out of capabilities package`

---

- [x] 4. memory-file 适配 Plugin 接口

  **要做什么**：
  - 创建 `plugins/memory-file/plugin.go`
  - 实现 `Plugin` 接口：
    - `Name()` → `"memory"`
    - `Tools()` → 返回 `MemoryStoreTool`, `MemoryRecallTool`, `MemoryForgetTool`
    - `Hooks()` → 返回 `FileMemoryHook`, `MemoryRecallHook`, `FactExtractionHook`, `MemorySummaryHook`（条件）
    - `Init(deps)` → 为 FactExtractionHook 注入 `deps.MessageStore`
  - 工具命名：`memory.tool.memory_store`、`memory.tool.memory_recall`、`memory.tool.memory_forget`
  - 钩子命名：`memory.hook.file_memory`、`memory.hook.memory_recall`、`memory.hook.fact_extraction`、`memory.hook.memory_summary`
  - 暴露 `GetStore()` 方法供 API 层使用
  - 保留旧的 `RegisterCapabilities` 函数（绞杀者模式，标记为 Deprecated）
  - **先写测试**：`plugin_test.go`

  **禁止做**：
  - 不要删除旧的 `capabilities_closure.go`（Task 10 处理）
  - 不要修改现有工具/钩子的实现

  **推荐 Agent Profile**：
  - **Category**: `deep` — 需要理解 3 个工具 + 4 个钩子的构造方式

  **并行化**：
  - **可并行执行**：YES（Wave 2，与 Task 5, 6, 7 并行）
  - **阻塞**：Task 8
  - **被阻塞**：Task 1, 3

  **参考**：
  - `plugins/memory-file/capabilities_closure.go` — 现有 ModuleCapability 实现
  - `plugins/memory-file/memory_store_tool.go` — MemoryStoreTool 构造函数
  - `plugins/memory-file/extraction_hook.go` — FactExtractionHook 需要 MessageStore

  **验收标准**：
  - [ ] `plugin.Tools()` 返回 3 个工具，名称格式 `memory.tool.xxx`
  - [ ] `plugin.Hooks()` 返回 3-4 个钩子，名称格式 `memory.hook.xxx`
  - [ ] `plugin.GetStore()` 返回正确的 MemoryStore 实例
  - [ ] `plugin.Init(deps)` 正确注入 MessageStore 到 FactExtractionHook

  **QA 场景**：
  ```
  Scenario: 新旧注册方式产出相同的工具/钩子
    Tool: Bash (go test)
    Steps:
      1. 创建 mock store
      2. 用旧方式：memoryfile.RegisterCapabilities(reg, store, ...)
      3. 用新方式：plugin := memoryfile.NewPlugin(store, ...)
      4. 验证 plugin.Tools() 和 plugin.Hooks() 的数量与旧方式一致
    Expected Result: 新旧方式产出相同的工具/钩子数量和名称
    Evidence: .sisyphus/evidence/task-4-memory.txt
  ```

  **提交**：YES — `feat(memory-file): add Plugin interface implementation`

---

- [x] 5. knowledge-base 适配 Plugin 接口

  **要做什么**：
  - 创建 `plugins/knowledge-base/plugin.go`
  - 实现 `Plugin` 接口：
    - `Name()` → `"knowledge"`
    - `Tools()` → 返回空（knowledge-base 不产出工具）
    - `Hooks()` → 返回 `KBRecallHook`
    - `Init(deps)` → 注入 `deps.AgentKnowledgeBases`
  - 钩子命名：`knowledge.hook.kb_recall`
  - 暴露 `GetStore()` 和 `GetEmbedder()` 方法供 API 层使用
  - 保留旧的 `RegisterCapabilities` 函数（Deprecated）
  - **先写测试**：`plugin_test.go`

  **禁止做**：
  - 不要删除旧的 `capabilities_closure.go`
  - 不要修改 KBRecallHook 实现

  **推荐 Agent Profile**：
  - **Category**: `deep` — 需要理解 KBRecallHook 的构造方式

  **并行化**：
  - **可并行执行**：YES（Wave 2，与 Task 4, 6, 7 并行）
  - **阻塞**：Task 8
  - **被阻塞**：Task 1, 3

  **参考**：
  - `plugins/knowledge-base/capabilities_closure.go` — 现有 HookCapability 实现
  - `plugins/knowledge-base/kb_recall_hook.go` — KBRecallHook 构造函数

  **验收标准**：
  - [ ] `plugin.Tools()` 返回空
  - [ ] `plugin.Hooks()` 返回 1 个钩子，名称 `knowledge.hook.kb_recall`
  - [ ] `plugin.GetStore()` 返回正确的 KnowledgeStore 实例
  - [ ] `plugin.Init(deps)` 正确注入 AgentKnowledgeBases

  **QA 场景**：
  ```
  Scenario: 新旧注册方式产出相同的钩子
    Tool: Bash (go test)
    Steps:
      1. 创建 mock store + embedder
      2. 用旧方式：knowledgebase.RegisterCapabilities(reg, ks, emb)
      3. 用新方式：plugin := knowledgebase.NewPlugin(ks, emb)
      4. 验证 plugin.Hooks() 返回 1 个钩子，名称正确
    Expected Result: 新旧方式产出相同的钩子
    Evidence: .sisyphus/evidence/task-5-knowledge.txt
  ```

  **提交**：YES — `feat(knowledge-base): add Plugin interface implementation`

---

- [x] 6. skill 适配 Plugin 接口

  **要做什么**：
  - 创建 `plugins/skill/plugin.go`
  - 实现 `Plugin` 接口：
    - `Name()` → `"skill"`
    - `Tools()` → 返回 `SkillTool`
    - `Hooks()` → 返回 `SkillInfoHook`
    - `Init(deps)` → 注入 `deps.Logger`
  - 工具命名：`skill.tool.skill`
  - 钩子命名：`skill.hook.skill_info`
  - 暴露 `GetConfig()` 方法
  - 保留旧的 `RegisterCapabilities` 函数（Deprecated）
  - **先写测试**：`plugin_test.go`

  **禁止做**：
  - 不要删除旧的 `capability.go`
  - 不要修改 SkillTool/SkillInfoHook 实现

  **推荐 Agent Profile**：
  - **Category**: `deep` — 需要理解 SkillModule 的构造方式

  **并行化**：
  - **可并行执行**：YES（Wave 2，与 Task 4, 5, 7 并行）
  - **阻塞**：Task 8
  - **被阻塞**：Task 1, 3

  **参考**：
  - `plugins/skill/capability.go` — 现有 ModuleCapability 实现
  - `plugins/skill/tool.go` — SkillTool 构造函数
  - `plugins/skill/hook.go` — SkillInfoHook 构造函数

  **验收标准**：
  - [ ] `plugin.Tools()` 返回 1 个工具，名称 `skill.tool.skill`
  - [ ] `plugin.Hooks()` 返回 1 个钩子，名称 `skill.hook.skill_info`
  - [ ] `plugin.Init(deps)` 正确注入 Logger

  **QA 场景**：
  ```
  Scenario: 新旧注册方式产出相同的工具/钩子
    Tool: Bash (go test)
    Steps:
      1. 用旧方式：skill.RegisterCapabilities(reg, cfg)
      2. 用新方式：plugin := skill.NewPlugin(cfg)
      3. 验证 plugin.Tools() 和 plugin.Hooks() 与旧方式一致
    Expected Result: 新旧方式产出相同
    Evidence: .sisyphus/evidence/task-6-skill.txt
  ```

  **提交**：YES — `feat(skill): add Plugin interface implementation`

---

- [x] 7. mcp 适配 Plugin 接口

  **要做什么**：
  - 创建 `plugins/mcp/plugin.go`
  - 实现 `Plugin` 接口：
    - `Name()` → `"mcp"`
    - `Tools()` → 返回动态发现的 MCP 工具（MCPToolWrapper 列表）
    - `Hooks()` → 返回空
    - `Init(deps)` → 注入 `deps.Logger`
  - 工具命名保持现有格式：`mcp.tool.{server}__{tool}`
  - 保留旧的 `RegisterCapabilities` 函数（Deprecated）
  - **先写测试**：`plugin_test.go`

  **禁止做**：
  - 不要删除旧的 `capability.go`
  - 不要修改 MCPToolWrapper 实现

  **推荐 Agent Profile**：
  - **Category**: `quick` — MCP 已经接近目标模式，改动最小

  **并行化**：
  - **可并行执行**：YES（Wave 2，与 Task 4, 5, 6 并行）
  - **阻塞**：Task 8
  - **被阻塞**：Task 1

  **参考**：
  - `plugins/mcp/capability.go` — 现有 ModuleCapability 实现
  - `plugins/mcp/types.go` — MCPToolWrapper

  **验收标准**：
  - [ ] `plugin.Tools()` 返回 MCP 工具列表
  - [ ] `plugin.Hooks()` 返回空
  - [ ] `plugin.Init(deps)` 正确注入 Logger

  **QA 场景**：
  ```
  Scenario: 新旧注册方式产出相同的工具
    Tool: Bash (go test)
    Steps:
      1. 用旧方式：mcp.RegisterCapabilities(reg, configs)
      2. 用新方式：plugin := mcp.NewPlugin(configs)
      3. 验证 plugin.Tools() 返回正确数量的 MCP 工具
    Expected Result: 新旧方式产出相同的工具列表
    Evidence: .sisyphus/evidence/task-7-mcp.txt
  ```

  **提交**：YES — `feat(mcp): add Plugin interface implementation`

---

- [x] 8. Harness.Register() + Harness.Build() 改造

  **要做什么**：
  - 在 `core/harness.go` 中添加 `Harness.Register(p plugin.Plugin)` 方法
  - 改造 `Build()` 方法：
    1. 启动阶段：调用已注册 Plugin 的 `Tools()` + `Hooks()`，存入 ToolPool/HookPool
    2. 两阶段 Init：delegate 等需要延迟依赖的工具先以占位形式存在，Build 时调用 `plugin.Init(deps)` 完成注入
    3. 替换旧的 Capability 解析逻辑
  - 改造 `makeAgentFactory()`：
    1. 用 ToolPool.Select(spec.Tools) 替换旧的 ExpandWildcards + capToToolName 查找
    2. 支持 `namespace.*` 通配符
  - 新旧系统并存：如果 `Registry` 不为 nil，仍走旧逻辑；否则走新逻辑
  - **先写测试**：更新 `harness_test.go`

  **禁止做**：
  - 不要删除旧的 Capability 代码路径（Task 10 处理）
  - 不要修改 agent/engine.go

  **推荐 Agent Profile**：
  - **Category**: `deep` — 核心编排逻辑，需要理解完整的 Build 流程

  **并行化**：
  - **可并行执行**：NO（依赖 Wave 2 所有插件适配完成）
  - **并行组**：Wave 3（与 Task 9 并行）
  - **阻塞**：Task 10
  - **被阻塞**：Task 1, 2, 4, 5, 6, 7

  **参考**：
  - `core/harness.go:135-315` — 现有 Build() 完整流程
  - `core/harness.go:377-435` — 现有 makeAgentFactory()
  - `core/harness_test.go` — 现有测试场景

  **验收标准**：
  - [ ] `h.Register(builtin.Plugin{})` 不报错
  - [ ] `h.Register(memoryPlugin)` 不报错
  - [ ] `h.Build()` 后 agent 工具列表包含 `memory.tool.memory_store`
  - [ ] `AgentSpec.Tools = ["memory.*"]` 选中 memory 插件所有工具
  - [ ] 新旧 harness_test.go 所有测试通过

  **QA 场景**：
  ```
  Scenario: 注册 builtin + memory 插件后 agent 获得正确工具
    Tool: Bash (go test)
    Steps:
      1. h := core.NewHarness(config)
      2. h.Register(builtin.Plugin{})
      3. h.Register(memoryPlugin)
      4. config.Agents = [{Tools: ["builtin.*", "memory.*"]}]
      5. h.Build()
      6. 验证 agent 的 ToolManager 包含所有内置工具和 memory 工具
    Expected Result: agent 工具列表正确，无重复
    Evidence: .sisyphus/evidence/task-8-harness.txt

  Scenario: 新旧系统并存（Registry 不为 nil 时走旧逻辑）
    Tool: Bash (go test)
    Steps:
      1. 使用 HarnessConfig{Registry: oldRegistry} 创建 Harness
      2. 验证 Build() 走旧代码路径
      3. 验证旧测试仍然通过
    Expected Result: 旧的 harness_test.go 全部 PASS
    Evidence: .sisyphus/evidence/task-8-compat.txt
  ```

  **提交**：YES — `feat(harness): add Register(Plugin) and namespace-aware agent tool selection`

---

- [x] 9. main.go 新注册流程 + agentSpecs 简化

  **要做什么**：
  - 修改 `server/cmd/server/main.go`：
    1. 创建 Plugin 实例替代旧的 RegisterCapabilities 调用
    2. `h.Register(builtin.Plugin{})` 替代 `hooks.RegisterAll + tools.RegisterAll`
    3. `h.Register(memoryfile.NewPlugin(fmStore, llmAdapter, summaryLLM))` 替代旧调用
    4. API 层改用 `fmPlugin.GetStore()` 替代直接引用的 `fmStore`
  - 简化 `agentSpecs()`：删除所有硬编码的 if 块
    ```go
    // 旧：硬编码每个插件
    if fmStore != nil && cfg.Memory.Enabled {
        tools = append(tools, capabilities.CapMemoryFile)
    }
    // 新：config.yaml 中 agent 自己声明
    // agents: [{tools: ["builtin.*", "memory.*"]}]
    ```
  - 更新 `server/config.yaml` 示例
  - **先写/更新测试**

  **禁止做**：
  - 不要删除旧的 RegisterCapabilities 调用路径（Task 10 处理）
  - 不要修改前端代码
  - 不要修改 API 端点路径

  **推荐 Agent Profile**：
  - **Category**: `deep` — 涉及 main.go 重构和 config.yaml 变更

  **并行化**：
  - **可并行执行**：NO（依赖 Task 8）
  - **并行组**：Wave 3（与 Task 8 并行）
  - **阻塞**：Task 10
  - **被阻塞**：Task 1, 2, 8

  **参考**：
  - `server/cmd/server/main.go:75-117` — 当前注册流程
  - `server/cmd/server/main.go:140-166` — agentSpecs()
  - `server/config.yaml` — 当前配置

  **验收标准**：
  - [ ] `cd server && go build ./...` 通过
  - [ ] Agent 通过 config.yaml 的 `memory.*` 获取 memory 插件工具
  - [ ] API Handler 通过 `fmPlugin.GetStore()` 访问 MemoryStore
  - [ ] 前端 Memory/Knowledge 管理页面功能不变

  **QA 场景**：
  ```
  Scenario: 新注册流程后 agent 工具列表正确
    Tool: Bash (go test)
    Steps:
      1. 更新 config.yaml：agent tools 包含 "builtin.*" 和 "memory.*"
      2. 启动 server（go build 验证编译通过）
      3. 创建 agent 会话
      4. 验证 agent 工具列表中包含 builtin.tool.code_executor 和 memory.tool.memory_store
    Expected Result: 编译通过，工具列表正确
    Evidence: .sisyphus/evidence/task-9-main.txt
  ```

  **提交**：YES — `feat(server): use Plugin-based registration and simplify agentSpecs`

---

- [x] 10. 删除旧 capabilities 代码

  **要做什么**：
  - 确认所有引用已迁移到新系统
  - 删除以下文件：
    - `core/capabilities/registry.go`
    - `core/capabilities/constants.go`（保留内置工具常量如 ToolCodeExecutor 等，或移到 core/plugin/builtin.go）
    - `core/capabilities/tools/register.go` — 只删 Capability struct，保留 Tool 实现
    - `core/capabilities/hooks/register.go` — 只删 Capability struct，保留 Hook 实现
  - 从以下文件删除 Capability struct 定义：
    - `core/capabilities/tools/code_executor.go` — 删 `codeExecutorCapability` + `shellExecutorCapability`
    - `core/capabilities/tools/file_ops.go` — 删 `fileOpsCapability`
    - `core/capabilities/tools/todo.go` — 删 `todoCapability`
    - `core/capabilities/tools/async.go` — 删 `asyncCapability`
    - `core/capabilities/tools/delegate.go` — 删 `delegateCapability`
    - `core/capabilities/tools/hitl.go` — 删 `confirmActionCapability` + `askUserCapability`
    - `core/capabilities/hooks/logging.go` — 删 `loggingHookCapability`
    - `core/capabilities/hooks/todo_injection.go` — 删 `todoInjectionHookCapability`
    - `core/capabilities/hooks/tracing.go` — 删 `tracingHookCapability`
  - 删除各插件的旧 `capabilities_closure.go` 和 `RegisterCapabilities` 函数
  - 从 `core/harness.go` 删除旧代码路径（Registry 相关逻辑）
  - 运行 `go test ./...` 确保所有测试通过

  **禁止做**：
  - 不要删除任何 Tool/Hook 实现代码
  - 不要删除 `core/capabilities/tools/helpers.go`（已迁移到 core/tool/）

  **推荐 Agent Profile**：
  - **Category**: `quick` — 纯删除 + 清理 import

  **并行化**：
  - **可并行执行**：NO（依赖所有迁移完成）
  - **并行组**：Wave 4
  - **阻塞**：F1-F4
  - **被阻塞**：Task 8, 9

  **验收标准**：
  - [ ] 无 `core/capabilities/registry.go`
  - [ ] 无 `*Capability` struct 残留
  - [ ] `go build ./...` 通过
  - [ ] `go test ./... -race` 通过
  - [ ] 搜索 `"tools.code_executor"` 格式字符串无残留

  **QA 场景**：
  ```
  Scenario: 删除后所有测试仍通过
    Tool: Bash
    Steps:
      1. cd core && go test ./... -race -count=1
      2. cd plugins && go test ./... -race -count=1
      3. cd server && go build ./...
      4. grep -r "capabilities.CapMemoryFile" --include="*.go" | 应无输出
    Expected Result: 所有命令 exit 0，无旧引用残留
    Evidence: .sisyphus/evidence/task-10-cleanup.txt
  ```

  **提交**：YES — `refactor: remove deprecated capabilities.Registry and Capability wrappers`

---

## 最终验证波次

> 4 个审查 Agent 并行运行。所有必须 APPROVE。

- [x] F1. **计划合规审计** — `oracle`
  验证所有 "必须包含" 已实现、"必须排除" 未出现、10 个任务按 spec 完成。
  输出：`Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT`

- [x] F2. **代码质量审查** — `unspecified-high`
  运行 `go vet ./...` + `go build ./...` + `go test ./... -race`。检查 AI slop、遗留代码。
  输出：`Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | VERDICT`

- [x] F3. **实际 QA 执行** — `unspecified-high`
  执行每个任务的 QA 场景。测试新旧系统兼容性。检查前端不受影响。
  输出：`Scenarios [N/N pass] | Integration [N/N] | VERDICT`

- [x] F4. **范围保真度检查** — `deep`
  验证 1:1 对应，无蔓延，无交叉污染。
  输出：`Tasks [N/N compliant] | Contamination [CLEAN/N] | VERDICT`

---

## 提交策略

| Wave | 提交消息 |
|------|---------|
| 1 | `feat(plugin): add Plugin interface and ToolPool/HookPool` |
| 1 | `feat(plugin): add builtin.Plugin consolidating all built-in tools/hooks` |
| 1 | `refactor: move helpers and types out of capabilities package` |
| 2 | `feat(memory-file): add Plugin interface implementation` |
| 2 | `feat(knowledge-base): add Plugin interface implementation` |
| 2 | `feat(skill): add Plugin interface implementation` |
| 2 | `feat(mcp): add Plugin interface implementation` |
| 3 | `feat(harness): add Register(Plugin) and namespace-aware agent tool selection` |
| 3 | `feat(server): use Plugin-based registration and simplify agentSpecs` |
| 4 | `refactor: remove deprecated capabilities.Registry and Capability wrappers` |

---

## 成功标准

### 验证命令
```bash
cd core && go test ./... -v -race
cd plugins && go test ./... -v -race
cd server && go build ./...
```

### 最终检查清单
- [ ] 所有 "必须包含" 已实现
- [ ] 所有 "必须排除" 未出现
- [ ] 所有测试通过（`go test -race ./...`）
- [ ] 前端不受影响
- [ ] 无 `*Capability` struct 残留
- [ ] 无 capabilities 常量在 core 包中引用插件