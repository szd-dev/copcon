# Built-in Tools Overview

CopCon ships with a set of built-in tools that give agents the ability to run code, execute commands, manage files, track tasks, interact with humans, delegate work, and run operations asynchronously.

All tools are auto-registered by the Harness. You don't need to manually import them in your application code. The capability system handles registration via `init()` functions when the `tools` package is imported.

## Tool Catalog

| Tool | Name | Category | Purpose |
|------|------|----------|---------|
| [Code Executor](code-executor.md) | `code_executor` | Execution | Run Python or JavaScript code in a sandboxed environment |
| [Shell Executor](shell-executor.md) | `shell_executor` | Execution | Run whitelisted shell commands |
| [File Ops](file-ops.md) | `file_ops` | I/O | Read, write, and list files within the working directory |
| [Todo](todo.md) | `todolist` | Task Management | Create, track, and manage todo items per session |
| [Confirm Action](confirm-action.md) | `confirm_action` | HITL | Ask the user to approve or decline a proposed action |
| [Ask User](ask-user.md) | `ask_user` | HITL | Ask the user a question and wait for their response |
| [Delegate](delegate.md) | `delegate_to` | Delegation | Hand off a task to another agent |
| [Delegate (Read)](delegate.md#read_sub_session) | `read_sub_session` | Delegation | Read messages from a sub-session |
| [Get Tool Status](async.md#get_tool_status) | `get_tool_status` | Async | Check the status of an async tool execution |
| [Get Tool Result](async.md#get_tool_result) | `get_tool_result` | Async | Fetch the result of a completed async tool |
| [Cancel Tool](async.md#cancel_tool) | `cancel_tool` | Async | Cancel a running async tool execution |
| [List Async Tools](async.md#list_async_tools) | `list_async_tools` | Async | List all async tool executions for the current session |

## How Tools Work

Every tool implements the `tool.Tool` interface:

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*ToolResult, error)
}
```

When the LLM decides to call a tool, the engine passes the arguments to `Execute`. The return value is a `ToolResult`:

```go
type ToolResult struct {
    Success bool   `json:"success"`
    Data    any    `json:"data,omitempty"`
    Error   string `json:"error,omitempty"`
}
```

## Execution Modes

Every tool (except delegation tools) supports an `execution_mode` parameter that the engine injects automatically:

| Mode | Behavior |
|------|----------|
| `sync` | Default. The agent waits for the tool to finish before continuing. |
| `concurrent` | Multiple tool calls run in parallel within the same turn. |
| `async` | The tool runs in the background. Use the async tools (`get_tool_status`, `get_tool_result`) to check on it later. |

This parameter is added to every tool's input schema at runtime. You don't define it in your tool's `InputSchema()` method.

Delegation tools (like `delegate_to`) are excluded from this injection because they define their own `mode` parameter.

## Capability Registration

Tools are registered as capabilities. Each tool has a corresponding capability struct in `core/capabilities/tools/`:

```go
// Auto-registered via init()
func init() {
    capabilities.Register(&codeExecutorCapability{})
}
```

The Harness imports the tools package with a blank import, which triggers all `init()` functions:

```go
import _ "github.com/copcon/core/capabilities/tools"
```

Some tools have dependencies on other capabilities. For example, the `todolist` tool depends on `hooks.todo_injection`:

```go
func (c *todoCapability) DependsOn() []string {
    return []string{"hooks.todo_injection"}
}
```

The capability system resolves these dependencies before creating tool instances.

## Adding Custom Tools

You can register your own tools alongside the built-in ones. See the [Capabilities](../README.md) documentation for details on creating custom capabilities.
