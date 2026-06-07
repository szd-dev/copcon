# Plugin Refactor - Learnings

## Module Structure
- `github.com/copcon/core` → core 库 (go.mod in /data/copcon/core/)
- `github.com/copcon/plugins` → 插件库 (go.mod in /data/copcon/plugins/, replace ../core)
- `github.com/copcon/server` → 服务器应用 (go.mod in /data/copcon/server/, replace ../core, ../plugins)
- Go workspace: go.work at root

## Key Interfaces
- `tool.Tool`: Name() string, Description() string, InputSchema() map[string]any, Execute(ChatContextInterface, map[string]any) (*ToolResult, error)
- `tool.ToolManager`: Register/Unregister/Get/List/Execute/GetToolDefs
- `tool.ToolRegistry`: Register/Get/List (simpler subset)
- `hook.Hook`: Name() string, Points() []HookPoint, Priority() int, Execute(ctx *HookContext) error
- `hook.HookRunner`: Register(hook.Hook) + ExecuteAt(HookPoint, HookContext)

## Current Naming
- Capabilities: `tools.code_executor`, `hooks.logging`, `modules.memory_file`
- Tool Aliases: `code_executor`, `shell_executor`, `file_ops`, `todolist`
- Wildcards: `tools.*`, `hooks.*`, `skills.*`, `memory.*`, `modules.*`, `*`

## Target Naming
- builtin tools: `builtin.tool.code_executor`, `builtin.tool.shell_executor`, etc.
- builtin hooks: `builtin.hook.logging`, `builtin.hook.todo_injection`, `builtin.hook.tracing`
- memory tools: `memory.tool.memory_store`, `memory.tool.memory_recall`, `memory.tool.memory_forget`
- memory hooks: `memory.hook.file_memory`, `memory.hook.memory_recall`, `memory.hook.fact_extraction`, `memory.hook.memory_summary`
- knowledge hooks: `knowledge.hook.kb_recall`
- skill: `skill.tool.skill`, `skill.hook.skill_info`
- mcp: `mcp.tool.{server}__{tool}`

## Dependency Injection
- CapabilityDeps has: SessionStore, MessageStore, TodoStore, AgentRegistry, Engine(interface{}), Logger, AgentKnowledgeBases
- Engine typed as interface{} to avoid circular imports
- Two-phase: Register stores plugin, Build calls Init(deps)

## Build Flow
1. initStores()
2. Register all capabilities to Registry
3. Resolve capabilities (expand wildcards + dependency sort)
4. Create ToolRegistry + register tools
5. Create HookRunner + register hooks
6. Create AgentRegistry + register factories
7. Create Engine
8. Register cross-agent tools (delegate, read_sub_session)

## Task 1: Plugin Interface + ToolPool/HookPool (completed)

- `tool.Tool.Execute` 的第一个参数是 `iface.ChatContextInterface`，不是 `interface{}`。测试 stub 必须引用 `github.com/copcon/core/iface`
- ToolPool.Select 通配符匹配逻辑：`strings.HasSuffix(pattern, ".*")` + `strings.HasPrefix(name, prefix)` 实现 namespace 前缀匹配
- `"memory.*"` 去掉 `.*` 后变成 `"memory."`（含点），`HasPrefix` 匹配时自然包含了分隔符
- Pool 使用 `sync.RWMutex` 保证线程安全，Select 中整体持读锁遍历 map
- HookPool.All() 返回 slice 的 copy（防止外部修改内部状态）
- PluginDeps.Engine 用 `interface{}` 避免与 agent.Engine 的循环引用，与 CapabilityDeps 一致
- 新包 `core/plugin` 与旧 `core/capabilities` 共存，绞杀者模式
- 测试并发注册时名字必须唯一（不能用 `rune('a'+idx%26)` 只产生26个）
