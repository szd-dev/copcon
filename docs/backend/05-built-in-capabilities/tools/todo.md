# Todo Tool

**Tool name:** `todolist`

**Capability:** `tools.todo`

**Depends on:** `hooks.todo_injection`

Manages todo items within a session. Agents use this to plan work, track progress, and coordinate multi-step tasks.

Todos are persisted in the database via `storage.TodoStore`, so they survive across conversation turns.

## When to Use

Use this tool when the agent needs to:

- Plan a multi-step task before starting work
- Track which steps are done, in progress, or failed
- Replan when priorities or requirements change mid-task
- Report progress back to the user

## Todo Lifecycle

A todo item goes through these states:

| Status | Meaning |
|--------|---------|
| `pending` | Created but not started yet |
| `in_progress` | Currently being worked on |
| `completed` | Finished successfully, with a result |
| `failed` | Could not complete, with a reason |
| `blocked` | Waiting on dependencies (set automatically) |

## Actions

### `create`

Creates a new todo item in the current session.

**Required:** `content`

**Optional:** `validation`, `depends_on`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | Yes | Description of the task |
| `validation` | string | No | How to verify this todo is truly done (e.g. "tests pass", "file exists at /output/result.csv") |
| `depends_on` | string[] | No | IDs of todos that must be completed before this one can start |

```json
{
  "tool": "todolist",
  "parameters": {
    "action": "create",
    "content": "Write unit tests for the handler",
    "validation": "go test ./handler/... passes with no failures"
  }
}
```

Response:

```json
{
  "success": true,
  "data": {
    "response": "{\"id\":\"a1b2c3d4\",\"session_id\":\"s5e6f7g8\",\"content\":\"Write unit tests for the handler\",\"status\":\"pending\",\"validation\":\"go test ./handler/... passes with no failures\",\"created_at\":\"2025-01-15T10:30:00Z\",\"updated_at\":\"2025-01-15T10:30:00Z\",\"retry_count\":0}"
  }
}
```

### `start`

Marks a todo as in progress.

**Required:** `todo_id`

```json
{
  "tool": "todolist",
  "parameters": {
    "action": "start",
    "todo_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
  }
}
```

### `complete`

Marks a todo as completed and records the result.

**Required:** `todo_id`, `result`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `todo_id` | string | Yes | ID of the todo to complete |
| `result` | string | Yes | What was accomplished or the output produced |

```json
{
  "tool": "todolist",
  "parameters": {
    "action": "complete",
    "todo_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "result": "12 tests written, all passing"
  }
}
```

### `fail`

Marks a todo as failed and records the reason.

**Required:** `todo_id`

**Optional:** `reason` or `validation` (either can serve as the failure explanation)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `todo_id` | string | Yes | ID of the todo to mark as failed |
| `reason` | string | No | Why it failed |
| `validation` | string | No | Alternative field for the failure reason. If `reason` is empty, `validation` is used as fallback. |

```json
{
  "tool": "todolist",
  "parameters": {
    "action": "fail",
    "todo_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "reason": "Database connection refused, integration tests require PostgreSQL"
  }
}
```

### `list`

Returns all todos for the current session.

No additional parameters needed.

```json
{
  "tool": "todolist",
  "parameters": {
    "action": "list"
  }
}
```

Response:

```json
{
  "success": true,
  "data": {
    "response": "{\"todos\":[{\"id\":\"...\",\"content\":\"Write tests\",\"status\":\"completed\",\"result\":\"12 tests passing\"},{\"id\":\"...\",\"content\":\"Deploy to staging\",\"status\":\"in_progress\"}],\"count\":2}"
  }
}
```

### `replan`

Deletes all existing todos for the session and creates a fresh set. Use this when the plan needs a significant overhaul.

**Required:** `todos` (array of todo objects)

Each todo object in the array:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content` | string | Yes | Description of the task |
| `validation` | string | No | Validation rules |
| `depends_on` | string[] | No | IDs of prerequisite todos |

```json
{
  "tool": "todolist",
  "parameters": {
    "action": "replan",
    "todos": [
      {"content": "Investigate the bug", "validation": "Root cause identified and documented"},
      {"content": "Write a fix", "validation": "Fix passes unit tests", "depends_on": ["<id-from-first-todo>"]},
      {"content": "Deploy the fix", "depends_on": ["<id-from-second-todo>"]}
    ]
  }
}
```

Note: `depends_on` IDs in a replan refer to IDs that will be created as part of the replan itself. Since the IDs are generated at creation time, you typically can't reference them in the same call. Use `replan` for independent tasks, or create todos one by one with `create` and reference the returned IDs.

## Parameters Summary

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | Yes | One of: `create`, `start`, `complete`, `fail`, `list`, `replan` |
| `todo_id` | string | Conditional | Required for `start`, `complete`, `fail` |
| `content` | string | Conditional | Required for `create` |
| `validation` | string | No | Validation rules (for `create`) or failure reason (for `fail`, if `reason` is empty) |
| `depends_on` | string[] | No | IDs of prerequisite todos (for `create`) |
| `result` | string | Conditional | Required for `complete` |
| `reason` | string | No | Failure reason (for `fail`) |
| `todos` | object[] | Conditional | Required for `replan` |

## Todo Data Structure

Each todo item contains:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `session_id` | string (UUID) | Session this todo belongs to |
| `content` | string | Task description |
| `active_form` | string | Present-tense form (e.g. "Writing tests") |
| `status` | string | Current status (`pending`, `in_progress`, `completed`, `blocked`, `failed`) |
| `validation` | string | How to verify completion |
| `result` | string | What was accomplished (set on completion) |
| `depends_on` | string[] | IDs of prerequisite todos |
| `retry_count` | int | How many times this todo has been retried |
| `created_at` | timestamp | When the todo was created |
| `updated_at` | timestamp | When the todo was last modified |
| `completed_at` | timestamp | When the todo was completed (null if not completed) |

## Dependency on `hooks.todo_injection`

The `todolist` tool depends on the `hooks.todo_injection` hook capability. This hook injects the current todo status into the agent's context at each turn, so the agent always knows what tasks are pending, in progress, or completed.

If `hooks.todo_injection` is not registered, the `todolist` tool won't be available.

## Security Considerations

- Todos are scoped to a session. An agent can only manage todos in its own session.
- IDs are UUIDs, making them hard to guess or forge.
- The `replan` action deletes all existing todos. This is irreversible.

## Source

`core/capabilities/tools/todo.go`, `core/capabilities/tools/todo_types.go`, `core/storage/todo.go`