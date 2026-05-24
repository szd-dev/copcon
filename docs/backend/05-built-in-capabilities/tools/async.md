# Async Tools

CopCon provides four tools for managing asynchronous tool executions. When a tool runs with `execution_mode: "async"`, it operates in the background. These async tools let the agent check on, retrieve results from, cancel, and list background tasks.

The async tools share an `AsyncToolRegistry` that tracks every background execution's state.

## Async Tool States

| Status | Meaning |
|--------|---------|
| `running` | The tool is executing in the background |
| `completed` | The tool finished successfully, result is available |
| `failed` | The tool encountered an error |
| `cancelled` | The tool was cancelled by the agent or user |

---

## get_tool_status

**Tool name:** `get_tool_status`

**Capability:** `tools.async`

Checks the current status of an async tool execution. Returns status, timestamps, and any available result or error.

### When to Use

Poll this tool to check whether a background task has finished. It's the primary way to monitor async progress.

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `call_id` | string | Yes | The unique identifier of the tool call. This is the ID assigned when the async execution started. |

### Response

#### Still running

```json
{
  "success": true,
  "data": {
    "response": "{\"call_id\":\"abc123\",\"tool_name\":\"code_executor\",\"status\":\"running\",\"start_time\":\"2025-01-15T10:30:00Z\"}"
  }
}
```

#### Completed

```json
{
  "success": true,
  "data": {
    "response": "{\"call_id\":\"abc123\",\"tool_name\":\"code_executor\",\"status\":\"completed\",\"start_time\":\"2025-01-15T10:30:00Z\",\"end_time\":\"2025-01-15T10:30:45Z\",\"duration\":\"45s\",\"result\":{\"stdout\":\"Hello\\n\",\"exit_code\":0}}"
  }
}
```

#### Failed

```json
{
  "success": true,
  "data": {
    "response": "{\"call_id\":\"abc123\",\"tool_name\":\"code_executor\",\"status\":\"failed\",\"start_time\":\"2025-01-15T10:30:00Z\",\"end_time\":\"2025-01-15T10:30:05Z\",\"duration\":\"5s\",\"error\":\"execution timed out\"}"
  }
}
```

#### Not found

```json
{
  "success": false,
  "error": "tool call not found: abc123"
}
```

### Example

```json
{
  "tool": "get_tool_status",
  "parameters": {
    "call_id": "abc123-def456-ghi789"
  }
}
```

---

## get_tool_result

**Tool name:** `get_tool_result`

**Capability:** `tools.async`

Retrieves the result of a completed async tool execution. Only works when the status is `completed`. If the tool is still running or has failed, this returns an error.

### When to Use

Call this after `get_tool_status` confirms the task is done. It extracts just the result data, without the timing metadata.

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `call_id` | string | Yes | The unique identifier of the completed tool call |

### Response

#### Success

```json
{
  "success": true,
  "data": {
    "stdout": "Hello, World!\n",
    "stderr": "",
    "exit_code": 0
  }
}
```

#### Not completed yet

```json
{
  "success": false,
  "error": "tool is not completed (status: running)"
}
```

#### Not found

```json
{
  "success": false,
  "error": "tool not found: abc123"
}
```

### Example

```json
{
  "tool": "get_tool_result",
  "parameters": {
    "call_id": "abc123-def456-ghi789"
  }
}
```

---

## cancel_tool

**Tool name:** `cancel_tool`

**Capability:** `tools.async`

Cancels a running async tool execution. The tool's cancel function is invoked immediately, and the status is set to `cancelled`. The result is not recoverable.

### When to Use

Use this when:

- A background task is taking too long
- The agent decides the task isn't needed anymore
- The user requests cancellation through a HITL interrupt

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `call_id` | string | Yes | The unique identifier of the running tool call |

### Response

#### Cancelled successfully

```json
{
  "success": true,
  "data": {
    "response": "{\"call_id\":\"abc123\",\"cancelled\":true}"
  }
}
```

#### Not running or not found

```json
{
  "success": false,
  "error": "could not cancel tool (not running or not found): abc123"
}
```

You can only cancel tools with status `running` that have a `CancelFunc`. Completed, failed, or already cancelled tools can't be cancelled.

### Example

```json
{
  "tool": "cancel_tool",
  "parameters": {
    "call_id": "abc123-def456-ghi789"
  }
}
```

---

## list_async_tools

**Tool name:** `list_async_tools`

**Capability:** `tools.async`

Lists all async tool executions for the current session. Useful for getting an overview of background task progress.

### When to Use

Use this when the agent needs to see all its background tasks at once, rather than checking each one individually.

### Parameters

No parameters required. The tool automatically uses the current session ID to filter results.

### Response

```json
{
  "success": true,
  "data": {
    "response": "{\"tools\":[{\"call_id\":\"abc123\",\"tool_name\":\"code_executor\",\"status\":\"completed\",\"start_time\":\"2025-01-15T10:30:00Z\",\"duration_ms\":45000},{\"call_id\":\"def456\",\"tool_name\":\"shell_executor\",\"status\":\"running\",\"start_time\":\"2025-01-15T10:31:00Z\"}],\"count\":2}"
  }
}
```

### Example

```json
{
  "tool": "list_async_tools",
  "parameters": {}
}
```

---

## Async Workflow

A typical async workflow looks like this:

1. **Start**: The agent calls a tool with `execution_mode: "async"`. The engine starts the tool in a background goroutine and returns a `call_id` to the agent.

2. **Monitor**: The agent calls `get_tool_status` periodically to check progress, or `list_async_tools` to see all tasks at once.

3. **Retrieve**: When `get_tool_status` shows `completed`, the agent calls `get_tool_result` to get the output.

4. **Cancel** (optional): If the task is no longer needed, the agent calls `cancel_tool`.

## Security Considerations

- `call_id` values are unique per execution. There's no way to access another session's async tools through `get_tool_status` or `get_tool_result` (they don't check session ownership), but `list_async_tools` filters by session ID.
- `cancel_tool` invokes the context's `CancelFunc`, which cancels the underlying `context.Context`. This is a hard cancel: the tool process receives a cancellation signal and must handle it.
- `CancelSession` is available at the registry level for cleaning up all async tools when a session ends. This isn't exposed as a tool, but the engine calls it when closing a session.

## Source

`core/capabilities/tools/async.go`, `core/tool/registry.go`