# Capabilities 体系分析

## 速览

Capabilities 体系有 **5 层冗余抽象**，但其中只有 **ModuleCapability 是有价值的**。

```
┌─────────────────────────────────────────────────────────────────┐
│ 当前架构（冗余层用 🔴 标记）                                      │
│                                                                 │
│ main.go                                                        │
│   reg := capabilities.NewRegistry()                            │
│   hooks.RegisterAll(reg)           🔴 3个空struct包装的Hook     │
│   tools.RegisterAll(reg)           🔴 8个空struct包装的Tool     │
│   memoryfile.RegisterCapabilities(reg, ...)  ✅ ModuleCapability│
│   skill.RegisterCapabilities(reg, ...)       ✅ ModuleCapability│
│   mcp.RegisterCapabilities(reg, ...)         ✅ ModuleCapability│
│                                                                 │
│ Harness.Build()                                                 │
│   collectCapabilityNames()           🔴 合并builtIn+别名映射    │
│   ResolveDependencies()              🔴 拓扑排序（仅1处使用）    │
│   ExpandWildcards()                  🔴 main.go从未使用通配符    │
│   遍历capabilities:                                             │
│     ModuleCapability → NewHooks/NewTools  ✅ 有用               │
│     ToolCapability   → NewTool()          🔴 纯转发             │
│     HookCapability   → NewHook()          🔴 纯转发             │
│   capToToolName map                   🔴 中间映射层             │
│                                                                 │
│ makeAgentFactory                                              │
│   ExpandWildcards again              🔴 第二次展开             │
│   capToToolName[capName] → toolNames 🔴 通过中间映射查找        │
│   toolRegistry.Get(toolName) → toolMgr ✅ 实际选择工具          │
└─────────────────────────────────────────────────────────────────┘
```

## 冗余点详解

### 1. 🔴 ToolCapability/HookCapability 包装层（8 + 3 = 11 个空 struct）

每个内置工具都有一个无用的包装 struct：

```go
// core/capabilities/tools/code_executor.go
type codeExecutorCapability struct{}  // 空struct，零字段

func (c *codeExecutorCapability) Name() string     { return "tools.code_executor" }
func (c *codeExecutorCapability) Type() CapabilityType { return CapabilityTypeTool }
func (c *codeExecutorCapability) DependsOn() []string  { return nil }  // 从未使用
func (c *codeExecutorCapability) NewTool(deps CapabilityDeps) (tool.Tool, error) {
    return NewCodeExecutor(), nil  // 纯转发，deps 从未使用
}
```

11 个文件都是这个模式。`NewTool(deps)` 调用 `NewXxx()`，deps 参数从未被使用。相当于给每个 Tool 额外包了一层纸。

### 2. 🔴 双命名系统

每个工具有两个名字，中间有一个映射表：

```go
// core/capabilities/constants.go
ToolCodeExecutor  = "tools.code_executor"     // 内部注册名
AliasCodeExecutor = "code_executor"            // 用户配置名

// core/harness.go
toolNameToCap = map[string]string{
    "code_executor":  "tools.code_executor",   // 别名 → 内部名
    "shell_executor": "tools.shell_executor",
    "file_ops":       "tools.file_ops",
    "todolist":       "tools.todo",
}
```

同一个东西，两个名字，一个映射表。为什么不能统一用一个名字？

### 3. 🔴 DependsOn / 拓扑排序（几乎未使用）

11 个内置 capability 中，只有 `todo` 声明了依赖 `todolist`。其余全部返回 `nil`。拓扑排序的整个基础设施（Kahn 算法、collectTransitive 递归）为 1 个依赖关系服务。

### 4. 🔴 ExpandWildcards（main.go 从未使用）

`tools.*`、`modules.*`、`*` 通配符在 `main.go` 的 AgentSpec 配置中从未出现。所有工具列表都是显式指定。

### 5. ✅ ModuleCapability（唯一有价值的）

```go
// plugins/memory-file/capabilities_closure.go
type MemoryModule struct { store, llm, summaryLLM }

func (m *MemoryModule) NewHooks(deps CapabilityDeps) ([]hook.Hook, error) {
    return []hook.Hook{
        NewFileMemoryHook(m.store),
        NewMemoryRecallHook(m.store, m.llm),
        NewFactExtractionHook(m.store, m.llm, deps.MessageStore, ""),  // 使用 deps
        NewMemorySummaryHook(summarizer),
    }, nil
}
```

ModuleCapability 让一个注册点产出 4 个 hooks + 3 个 tools，且 `deps.MessageStore` 被实际使用。这是合理的抽象。

---

## 简化方案

### 目标状态

```
┌──────────────────────────────────────────────────┐
│ 简化后架构                                        │
│                                                  │
│ main.go                                         │
│   reg := capabilities.NewRegistry()             │
│   reg.RegisterModule(memoryfile.NewModule(...))  │
│   reg.RegisterModule(skill.NewModule(...))       │
│   reg.RegisterModule(mcp.NewModule(...))         │
│                                                  │
│   // 内置工具直接注册，无包装层                   │
│   reg.RegisterTool(NewCodeExecutor())            │
│   reg.RegisterTool(NewShellExecutor())           │
│   reg.RegisterHook(NewLoggingPlugin())           │
│   // ... 统一命名，无别名映射                     │
│                                                  │
│ Harness.Build()                                 │
│   for each module: NewHooks/NewTools             │
│   for each agent spec: 按名称从pool选工具        │
│   // 无拓扑排序，无通配符展开，无capToToolName    │
└──────────────────────────────────────────────────┘
```

### 要删除的

| 删除项 | 文件数 | 代码量 |
|--------|--------|--------|
| ToolCapability 接口 + 8 个实现 struct | 9 | ~150 行 |
| HookCapability 接口 + 3 个实现 struct | 4 | ~60 行 |
| CapabilityType 枚举 | 1 | 删除 use |
| DependsOn() + 拓扑排序 | 1 | ~80 行 |
| toolNameToCap 别名映射 | 1 | ~10 行 |
| capToToolName 中间映射 | 1 | 内联到 agent 创建 |
| ExpandWildcards | 1 | 或用更简单前缀匹配替代 |

### 要保留的

| 保留项 | 原因 |
|--------|------|
| ModuleCapability 接口 | 插件需要注册入口 |
| CapabilityDeps 依赖注入 | 插件需要 MessageStore 等 |
| ToolRegistry（全局）/ ToolManager（per-agent） | 合理的分层 |
| Registry.Register() | 模块注册入口，可简化 |
| ErrDependencyUnavailable | 优雅降级 |

### 影响范围

| 模块 | 影响 |
|------|------|
| `core/capabilities/tools/` | 8 个文件，删除 capability struct，保留 Tool 实现 |
| `core/capabilities/hooks/` | 3 个文件，删除 capability struct，保留 Hook 实现 |
| `core/capabilities/registry.go` | 简化接口，去掉 ToolCapability/HookCapability |
| `core/capabilities/constants.go` | 删除别名，统一命名 |
| `core/harness.go` | 简化 Build() 和 makeAgentFactory() |
| `core/harness_test.go` | 更新测试 |
| `server/cmd/server/main.go` | 调整注册方式 |
| `plugins/*` | 3 个 ModuleCapability 实现**不变** |