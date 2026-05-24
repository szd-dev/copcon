# Todo Injection Hook

The todo injection hook appends the current session's todo list to the system prompt before each LLM call. This gives the agent visibility into its own task state without needing to explicitly query for it.

## Purpose

Agents that manage complex tasks need to know what's pending, what's in progress, and what's already done. Without this hook, the agent would have to call the `todolist` tool to check its task state, wasting a tool call on every turn just to stay oriented.

The todo injection hook solves this by automatically appending a formatted summary of the current todo list to the system prompt. The LLM sees its tasks as part of its instructions, so it can plan and prioritize without extra round trips.

## Hook Points

| Hook Point | What Happens |
|------------|-------------|
| `OnSystemPrompt` | Fetches all todos for the current session from `TodoStore` and appends them to the system prompt |

Priority: **50** (runs early, before higher-priority hooks, so the todo state is available to other hooks that might read the system prompt).

## How It Works

1. When the system prompt is being resolved, the hook fetches all todos for the current session using `TodoStore.List(ctx, sessionUUID)`.
2. It groups the todos by status: `pending`, `in_progress`, `completed`, `failed`, `blocked`.
3. It formats them into a single line appended to the system prompt:

```
Current todo list: [pending: task A, task B, in_progress: task C, completed: task D]
```

If the session has no todos, nothing is appended. The system prompt remains unchanged.

### ActiveForm

Each todo can have an `ActiveForm` field. When present, the hook uses the active form instead of the original content. This lets you write task descriptions that read naturally in the system prompt:

```
Content:    "Write documentation for the memory hook"
ActiveForm: "writing memory hook docs"
```

In the injected prompt: `in_progress: writing memory hook docs`

### Error Handling

If anything goes wrong (invalid session ID, store failure), the hook logs a warning and returns nil. It never blocks the agent turn.

```go
sessionUUID, err := uuid.Parse(ctx.SessionID)
if err != nil {
    h.logger.Warn("failed to parse session id for todo injection",
        "session_id", ctx.SessionID,
        "error", err,
    )
    return nil  // graceful degradation
}
```

## Configuration

### YAML

```yaml
hooks:
  - name: "todo_injection"
    enabled: true
```

The hook has no additional parameters. It reads from whatever `TodoStore` is provided by the `CapabilityDeps`.

### Go

```go
harness := core.NewHarness(core.HarnessConfig{
    Hooks: []HookSpec{
        {Name: "hooks.todo_injection", Enabled: true},
    },
})
```

### Dependencies

The hook requires a `TodoStore` implementation. CopCon provides a PostgreSQL-backed implementation. If `TodoStore` is nil in the `CapabilityDeps`, the hook is created with a nil store. It will fail to fetch todos and log a warning on each turn, but it won't crash.

```go
func (c *todoInjectionHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
    return NewTodoInjectionHook(deps.TodoStore), nil
}
```

## Todo Status Format

The injected text uses this format:

```
Current todo list: [status1: item1, item2, status2: item3, ...]
```

Only statuses with items are included. The statuses appear in this order:

1. `pending`
2. `in_progress`
3. `completed`
4. `failed`
5. `blocked`

### Example

A session with these todos:

| Content | Status |
|---------|--------|
| "Set up database schema" | `completed` |
| "Write API handlers" | `in_progress` |
| "Add integration tests" | `pending` |
| "Deploy to staging" | `pending` |

Produces this injected text:

```
Current todo list: [pending: Add integration tests, Deploy to staging, in_progress: Write API handlers, completed: Set up database schema]
```

This line is appended to the end of the system prompt with a double newline separator.

## Relationship to the Todo Tool

The `todolist` tool and the todo injection hook work together:

- **`todolist` tool**: Lets the LLM create, update, and delete todos during a conversation.
- **Todo injection hook**: Ensures the LLM always sees the current todo state, even if it hasn't recently called the tool.

The `todolist` tool capability declares a dependency on `hooks.todo_injection`:

```go
func (c *todoCapability) DependsOn() []string {
    return []string{"hooks.todo_injection"}
}
```

This means the capability system initializes the hook before the tool. When you enable the `todolist` tool, the todo injection hook is automatically included.

## Performance Impact

| Aspect | Impact | Notes |
|--------|--------|-------|
| Database query | One `List` call per LLM turn | Indexed by session ID; typically fast |
| Prompt size increase | 1 line appended | Minimal impact on token count |
| Concurrency | Safe. The hook holds no mutable state. | |

### Tips

- The hook runs at every LLM call, not just the first one. If the agent modifies todos during a turn (via the `todolist` tool), the updated state will be injected in the next turn.
- If you don't use the todo system at all, disable this hook to avoid the database query on every turn.
- For sessions with many todos (50+), consider whether the injected text is consuming too much of the context window. The hook doesn't truncate; it includes all todos.

## Example: Agent with Task Management

```yaml
agents:
  - name: "project-assistant"
    model: "gpt-4"
    system_prompt: |
      You are a project assistant. Break work into tasks,
      track progress, and mark tasks as complete.
    tools:
      - "todolist"
      - "code_executor"
      - "file_ops"
    hooks:
      - "logging"
      - "todo_injection"

hooks:
  - name: "todo_injection"
    enabled: true
```

With this setup, the agent can create tasks, and those tasks will automatically appear in its system prompt on every subsequent turn. No explicit "check my todos" step needed.
