# Delegate Tool

**Tool name:** `delegate_to`

**Capability:** `tools.delegate`

Hands off a task to another agent. The parent agent specifies which agent to call and what task to give it. The sub-agent runs in a separate session, completes the task, and returns a summary.

There is also a companion tool, `read_sub_session`, for reading the full message history of a sub-session.

## When to Use

Use `delegate_to` when:

- The current agent lacks expertise for a specific task (e.g., a general agent delegating code review to a specialized code-review agent)
- A task can be handled independently without close coordination
- You want to parallelize work across multiple agents

## delegate_to

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `agent_id` | string | Yes | ID of the target agent. Must be registered in the `AgentRegistry`. |
| `task` | string | Yes | Description of the task for the sub-agent. This becomes the sub-agent's initial user message. |
| `mode` | string | No | Currently only `sync` is supported. The parent agent blocks until the sub-agent finishes. |
| `extra` | object | No | Additional parameters to pass to the sub-agent. |

Note: `delegate_to` is a `DelegationTool`, so the engine does not inject the `execution_mode` parameter. The tool defines its own `mode` field instead.

### Response

```json
{
  "success": true,
  "data": {
    "sub_session_id": "550e8400-e29b-41d4-a716-446655440000",
    "summary": "I reviewed the pull request and found 3 issues: a missing error check on line 42, an unhandled nil pointer in the handler, and a potential SQL injection in the query builder.",
    "status": "completed"
  }
}
```

The `summary` is extracted from the last assistant message in the sub-session. If no assistant message exists, it defaults to "Task completed".

### Error: Agent not found

```json
{
  "success": false,
  "error": "agent not found: code_reviewer"
}
```

### Example

```json
{
  "tool": "delegate_to",
  "parameters": {
    "agent_id": "code_reviewer",
    "task": "Review the changes in pull request #42. Check for security issues, code quality, and test coverage."
  }
}
```

## read_sub_session

**Tool name:** `read_sub_session`

Reads the full message history from a sub-session created by `delegate_to`. Use this when you need more detail than the summary provided by `delegate_to`.

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `sub_session_id` | string | Yes | ID of the sub-session to read. Must be a child of the current session. |

### Response

```json
{
  "success": true,
  "data": {
    "messages": [
      {
        "id": "...",
        "session_id": "...",
        "role": "user",
        "content": "Review the changes...",
        "steps": [],
        "metadata": {
          "created_at": "2025-01-15T10:30:00Z",
          "model": "gpt-4",
          "token_count": 150,
          "duration_ms": 3200
        }
      },
      {
        "id": "...",
        "session_id": "...",
        "role": "assistant",
        "content": "I found 3 issues...",
        "steps": [
          {
            "parts": [
              {"type": "tool-call", "tool_name": "file_ops", "args": {"operation": "read", "path": "/src/handler.go"}},
              {"type": "tool-output", "output": "package handler..."}
            ]
          }
        ]
      }
    ]
  }
}
```

### Security

The tool verifies that the sub-session's `ParentSessionID` matches the current session ID. This prevents agents from reading sessions they didn't create.

### Example

```json
{
  "tool": "read_sub_session",
  "parameters": {
    "sub_session_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

## How Delegation Works

1. The parent agent calls `delegate_to` with an `agent_id` and `task`.
2. The tool looks up the agent factory from the `AgentRegistry`.
3. A new sub-session is created with `ParentSessionID` set to the parent's session.
4. The task is added as a user message in the sub-session.
5. The agent engine runs the sub-agent in a goroutine: `engine.Chat(subChatCtx, task)`.
6. The tool waits for the sub-session to close (`subChatCtx.Closed()`).
7. The last assistant message from the sub-session is extracted as the summary.
8. The summary is returned to the parent agent.

The sub-agent's `ChatContext` has a `Depth` one greater than the parent's. This tracks delegation depth and prevents infinite recursion if a sub-agent tries to delegate further.

## Security Considerations

- The `agent_id` must exist in the `AgentRegistry`. Unknown agents return an error.
- Sub-sessions are linked to their parent via `ParentSessionID`. Only the parent session can read the sub-session.
- Delegation depth is tracked. Configure a maximum depth in the engine to prevent runaway delegation chains.
- The sub-agent runs with its own `ChatContext`, so HITL interrupts in the sub-agent go to the user independently of the parent agent's flow.
- The sub-session's messages are persisted in the database and can be reviewed later.

## Configuration

The `delegate_to` tool requires these dependencies at capability creation time:

- `AgentRegistry`: maps agent IDs to factory functions
- `SessionStore`: creates and retrieves sessions
- `MessageStore`: persists messages
- `Engine` (typed as `agent.AgentEngine`): runs the sub-agent

These are provided via `CapabilityDeps` when the capability's `NewTool` method is called.

## Source

`core/capabilities/tools/delegate.go`