# Hooks Overview

Hooks are event-driven extension points that run at specific stages of an agent's lifecycle. They let you inject custom logic without modifying the engine itself: logging every LLM call, persisting memories, injecting context into system prompts, tracing tool execution, and more.

Tools let agents *do* things. Hooks let *you* observe and influence how agents work.

## Hook Catalog

| Hook | Capability Name | Priority | Hook Points | Purpose |
|------|----------------|----------|-------------|---------|
| [Memory](memory.md) | `hooks.memory` | 100 | `AfterContextBuild`, `OnMessagePersist` | Retrieve and store conversational memory |
| [Logging](logging.md) | `hooks.logging` | 200 | `BeforeLLMCall`, `AfterLLMCall`, `BeforeToolExecute`, `AfterToolExecute` | Structured logging of agent activity |
| [Tracing](tracing.md) | `hooks.tracing` | 200 | `BeforeLLMCall`, `AfterLLMCall`, `BeforeToolExecute`, `AfterToolExecute`, `OnToolError` | Distributed tracing with span lifecycle |
| [Todo Injection](todo.md) | `hooks.todo_injection` | 50 | `OnSystemPrompt` | Inject current todo state into the system prompt |

## How Hooks Work

Every hook implements the `hook.Hook` interface:

```go
type Hook interface {
    Name() string
    Points() []HookPoint
    Priority() int
    Execute(ctx *HookContext) error
}
```

- **Name** returns a human-readable identifier, used in logs and debugging.
- **Points** declares which lifecycle events this hook responds to. A single hook can listen to multiple points.
- **Priority** determines execution order. Higher numbers run first. Default is 100.
- **Execute** is the callback. It receives a `HookContext` with fields populated according to the current hook point.

### HookContext

`HookContext` carries all the data available at a given hook point. Not every field is populated for every point; what you get depends on where the hook fires.

```go
type HookContext struct {
    ChatCtx      iface.ChatContextInterface  // always populated
    SessionID    string                      // always populated
    AgentID      string                      // always populated
    SystemPrompt *string                     // OnSystemPrompt, BeforeContextBuild
    Messages     *[]entity.MessageForLLM     // AfterContextBuild, BeforeLLMCall, AfterLLMCall
    ToolName     string                      // BeforeToolExecute, AfterToolExecute, OnToolError
    ToolArgs     map[string]any              // BeforeToolExecute, AfterToolExecute, OnToolError
    ToolResult   *tool.ToolResult            // AfterToolExecute, OnToolError
    Logger       *slog.Logger                // always populated
    CurrentPoint HookPoint                   // always populated
}
```

Pointer fields (`*string`, `*[]MessageForLLM`, `*tool.ToolResult`) are mutable. A hook at `OnSystemPrompt` can rewrite the prompt. A hook at `AfterContextBuild` can prepend messages. This is how hooks influence downstream behavior.

## Lifecycle Events

The engine dispatches hooks at these points, in the order they occur during a single agent turn:

```
Request arrives
       |
       v
  OnSessionResolve        session ID lookup or creation
       |
       v
  OnSystemPrompt          system prompt resolution
       |
       v
  BeforeContextBuild      before the context window is assembled
       |
       v
  AfterContextBuild       after messages are assembled, before LLM call
       |
       v
  BeforeLLMCall           just before the LLM API request
       |
       v
  [LLM Response]
       |
       v
  AfterLLMCall            after the LLM response is received
       |
       v
  BeforeToolExecute       before a tool invocation       (if tool call)
       |
       v
  [Tool Execution]
       |
       v
  AfterToolExecute        after tool completes           (if tool call)
  OnToolError             on tool failure                (if error)
       |
       v
  OnMessagePersist        before a message is persisted
       |
       v
  Response sent
```

Here is the full list of `HookPoint` constants:

| Hook Point | Constant | When It Fires | Mutable Fields |
|------------|----------|---------------|----------------|
| `before_context_build` | `BeforeContextBuild` | Before the context window is assembled | `SystemPrompt` |
| `after_context_build` | `AfterContextBuild` | After messages are assembled, before LLM | `Messages` |
| `on_system_prompt` | `OnSystemPrompt` | During system prompt resolution | `SystemPrompt` |
| `on_message_persist` | `OnMessagePersist` | Before a message is written to storage | `Messages` |
| `before_tool_execute` | `BeforeToolExecute` | Just before a tool runs | `ToolArgs` |
| `after_tool_execute` | `AfterToolExecute` | After a tool completes | `ToolResult` |
| `on_tool_error` | `OnToolError` | When a tool execution fails | `ToolResult` |
| `before_llm_call` | `BeforeLLMCall` | Before the LLM API request | `Messages` |
| `after_llm_call` | `AfterLLMCall` | After the LLM API response | `Messages` |
| `on_session_resolve` | `OnSessionResolve` | During session ID resolution | none |

## Execution Order

When multiple hooks register for the same hook point, the `HookRunner` sorts them by priority in descending order (higher numbers run first). Ties are broken by registration order (earlier registrations win).

For example, with these hooks registered:

| Hook | Priority |
|------|----------|
| Logging | 200 |
| Tracing | 200 |
| Memory | 100 |
| Todo Injection | 50 |

At `BeforeLLMCall`, execution order is:

1. **Logging** (priority 200, registered first)
2. **Tracing** (priority 200, registered second)
3. Memory and Todo Injection don't fire here (they registered for different points)

At `OnSystemPrompt`, execution order is:

1. **Todo Injection** (priority 50, the only hook at this point)

## Error Handling

Hooks are fault-tolerant. If a hook returns an error or panics, the engine logs it and continues executing the remaining hooks. A failing hook never aborts the pipeline.

```go
// From the HookRunner:
if err := h.Execute(ctx); err != nil {
    slog.Warn("hook returned error",
        "hook", h.Name(),
        "error", err,
        "point", ctx.CurrentPoint,
    )
}
```

If the underlying `context.Context` is cancelled before `Run` is called, the entire chain is skipped.

## Capability Registration

Hooks 通过 Capability 系统注册。内置 Hook 在 `core/capabilities/hooks/register.go` 中通过 `RegisterAll()` 函数统一注册：

```go
// core/capabilities/hooks/register.go
func RegisterAll(r *capabilities.Registry) {
    r.Register(&loggingHookCapability{})
    r.Register(&tracingHookCapability{})
    // ...
}
```

Harness 在 `Build()` 中自动调用 `hooks.RegisterAll(registry)`，无需手动引入。

Hook 能力实现 `HookCapability` 接口：

```go
type HookCapability interface {
    Capability
    NewHook(deps CapabilityDeps) (hook.Hook, error)
}
```

`CapabilityDeps` 携带运行依赖：

```go
type CapabilityDeps struct {
    SessionStore        storage.SessionStore
    MessageStore        storage.MessageStore
    TodoStore           storage.TodoStore
    AgentRegistry       agent.AgentRegistry
    Engine              interface{}
    Logger              *slog.Logger
    AgentKnowledgeBases map[string][]string
}
```

Hook 如果 `NewHook` 返回 `ErrDependencyUnavailable`，Harness 会优雅跳过而不报错。

## Enabling and Disabling Hooks

Hooks are enabled or disabled through the Harness configuration. In YAML:

```yaml
# Enable specific hooks
hooks:
  - name: "logging"
    enabled: true
  - name: "tracing"
    enabled: true
  - name: "memory"
    enabled: false    # disabled
```

In Go:

```go
harness := core.NewHarness(core.HarnessConfig{
    Hooks: []HookSpec{
        {Name: "hooks.logging", Enabled: true},
        {Name: "hooks.tracing", Enabled: true},
        {Name: "hooks.memory",  Enabled: false},
    },
})
```

You can also use wildcards to enable all hooks of a type:

```yaml
hooks:
  - name: "hooks.*"    # enables all registered hooks
```

### Conditional Behavior

Some hooks adapt to the absence of their required dependencies rather than failing:

- **Memory hook**: If `MemoryStore` is nil, the hook is created with a nil manager and becomes a no-op. No Qdrant? No problem. The hook silently skips.
- **Todo injection hook**: If `TodoStore` is nil, the hook won't be able to fetch todos, but it won't crash.
- **Tracing hook**: If the `Tracer` is nil, the hook skips all span operations.

## Performance Considerations

Hooks run synchronously within the agent's request pipeline. A slow hook slows the entire turn. Keep these guidelines in mind:

1. **Be fast.** Hooks should do minimal work. Heavy operations (vector search, network calls) should run asynchronously when possible. The memory hook, for instance, stores memories in a goroutine at `OnMessagePersist`.
2. **Don't block.** If you need to call an external service, consider using a timeout or running the work in a background goroutine.
3. **Fail gracefully.** Return errors for logging, but don't expect the pipeline to stop. If your hook's work is critical, log at error level and handle the consequences downstream.

## Next Steps

- [Memory Hook](memory.md): Long-term conversational memory
- [Logging Hook](logging.md): Structured activity logging
- [Tracing Hook](tracing.md): Distributed tracing for LLM and tool calls
- [Todo Injection Hook](todo.md): Automatic todo state in system prompts
