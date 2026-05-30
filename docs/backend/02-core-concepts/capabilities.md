# 能力系统 (Capabilities)

CopCon 采用基于 **Registry + 接口** 的能力插件架构。每个可插拔功能模块（文件记忆、知识库等）通过实现 `Capability` 接口并注册到 `Registry` 来接入系统。

## 三层能力模型

```
                        Capability
                            │
          ┌─────────────────┼─────────────────┐
          ▼                 ▼                  ▼
   ToolCapability     HookCapability     ModuleCapability
    (单一 Tool)        (单一 Hook)       (多个 Tool + Hook)
```

| 接口 | 产出 | 使用场景 |
|------|------|---------|
| `ToolCapability` | 1 个 `tool.Tool` | 单个工具（如 `code_executor`） |
| `HookCapability` | 1 个 `hook.Hook` | 单个钩子（如 `logging`） |
| `ModuleCapability` | N 个 Tool + M 个 Hook | 功能模块（如 `memory_file`） |

## Capability 接口

```go
// core/capabilities/registry.go

type Capability interface {
    Name() string          // 唯一标识，如 "tools.code_executor"
    Type() CapabilityType  // tool / hook / module / skill / memory
    DependsOn() []string   // 依赖的其他 Capability 名称
}

type ToolCapability interface {
    Capability
    NewTool(deps CapabilityDeps) (tool.Tool, error)
}

type HookCapability interface {
    Capability
    NewHook(deps CapabilityDeps) (hook.Hook, error)
}

type ModuleCapability interface {
    Capability
    NewHooks(deps CapabilityDeps) ([]hook.Hook, error)
    NewTools(deps CapabilityDeps) ([]tool.Tool, error)
}
```

### CapabilityDeps — 运行时依赖注入

`NewTool` / `NewHook` / `NewHooks` / `NewTools` 接收 `CapabilityDeps`，框架在实例化时注入：

```go
type CapabilityDeps struct {
    SessionStore        storage.SessionStore
    MessageStore        storage.MessageStore
    TodoStore           storage.TodoStore
    AgentRegistry       agent.AgentRegistry
    Engine              interface{}  // AgentEngine
    Logger              *slog.Logger
    AgentKnowledgeBases map[string][]string
}
```

## 注册方式

插件通过显式注册函数接入（非 `init()` 自动注册）：

```go
// plugins/my-plugin/register.go
package myplugin

import "github.com/copcon/core/capabilities"

func RegisterCapabilities(r *capabilities.Registry, store *MyStore) {
    r.Register(&MyModuleCapability{store: store})
}
```

调用方在构建 Harness 前显式调用：

```go
registry := capabilities.NewRegistry()
myplugin.RegisterCapabilities(registry, myStore)
cfg := HarnessConfig{Registry: registry, ...}
```

## ModuleCapability 示例：File Memory

`memory-file` 插件用一个 `ModuleCapability` 产出 1 个 Hook + 3 个 Tool：

```go
// plugins/memory-file/capabilities_closure.go

type MemoryModule struct {
    store *FileMemoryStore
}

func (m *MemoryModule) Name() string                      { return "modules.memory_file" }
func (m *MemoryModule) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeModule }
func (m *MemoryModule) DependsOn() []string               { return nil }

func (m *MemoryModule) NewHooks(deps capabilities.CapabilityDeps) ([]hook.Hook, error) {
    return []hook.Hook{NewFileMemoryHook(m.store)}, nil
}

func (m *MemoryModule) NewTools(deps capabilities.CapabilityDeps) ([]tool.Tool, error) {
    return []tool.Tool{
        NewMemoryStoreTool(m.store),
        NewMemoryRecallTool(m.store),
        NewMemoryForgetTool(m.store),
    }, nil
}
```

注册只需一行：

```go
// plugins/memory-file/register.go
func RegisterCapabilities(r *capabilities.Registry, store *FileMemoryStore) {
    r.Register(&MemoryModule{store: store})
}
```

## 单个 Tool/Hook 示例

对于只需产出单个 Tool 的场景，用 `ToolCapability`：

```go
type codeExecutorCapability struct{}

func (c *codeExecutorCapability) Name() string                    { return "tools.code_executor" }
func (c *codeExecutorCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *codeExecutorCapability) DependsOn() []string             { return nil }
func (c *codeExecutorCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
    return NewCodeExecutorTool(), nil
}
```

## Capability Bundle

多个能力的逻辑组合通过 Bundle 函数声明，在 `Harness.Build()` 中自动展开：

```go
// core/capabilities/bundle.go

func MemoryBundleNames() []string {
    return []string{
        "hooks.memory",        // 向量记忆 Hook
        "modules.memory_file", // 文件记忆模块（含 1 Hook + 3 Tool）
    }
}
```

配置 Memory 时只需一个开关：

```go
spec := AgentSpec{Memory: MemorySpec{Enabled: true}}
// Build() 自动注入 MemoryBundleNames() 中的所有能力
```

## 内置能力一览

### Tools

| 名称 | 标识 |
|------|------|
| 确认操作 | `tools.confirm_action` |
| 询问用户 | `tools.ask_user` |
| 任务管理 | `tools.todo` |
| 异步任务 | `tools.async` |
| 代码执行 | `tools.code_executor` |
| Shell 命令 | `tools.shell_executor` |
| 文件操作 | `tools.file_ops` |
| Agent 委托 | `tools.delegate` |

### Hooks

| 名称 | 标识 |
|------|------|
| 任务注入 | `hooks.todo_injection` |
| 日志记录 | `hooks.logging` |
| 链路追踪 | `hooks.tracing` |
| 向量记忆 | `hooks.memory` |
| 知识库召回 | `hooks.kb_recall` |
| 记忆持久化 | `hooks.memory_persist` |

### Modules

| 名称 | 标识 | 产出 |
|------|------|------|
| 文件记忆 | `modules.memory_file` | 1 Hook (`file_memory`) + 3 Tools (`memory_store`, `memory_recall`, `memory_forget`) |

## Build 流程

`Harness.Build()` 按以下顺序处理能力：

```
1. Registry.ResolveDependencies(names)
    ├── 展开通配符 (tools.*, hooks.*, modules.*, *)
    ├── 递归收集传递依赖 (DependsOn)
    └── 拓扑排序

2. 遍历 resolved，按类型实例化：
    ├── ModuleCapability → NewHooks() + NewTools()
    ├── ToolCapability   → NewTool()
    └── HookCapability   → NewHook()

3. 产物注册到全局 ToolRegistry / HookRunner
```

## 下一步

- [自定义 Tool 完整指南](../06-extending/custom-tool.md)
- [自定义 Hook 完整指南](../06-extending/custom-hook.md)
- [Harness 配置详解](harness.md)
- [内置 Hook 文档](../05-built-in-capabilities/hooks/overview.md)