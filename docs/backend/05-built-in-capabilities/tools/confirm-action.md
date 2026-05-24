# Confirm Action Tool

**Tool name:** `confirm_action`

**Capability:** `tools.confirm_action`

**Depends on:** none

Asks the user to approve or decline a proposed action. The agent sends a message describing what it plans to do, and the execution pauses until the user responds.

This is one of CopCon's Human-in-the-Loop (HITL) tools. Use it before executing any action that could be destructive, irreversible, or that the user should have visibility into.

## When to Use

Use this tool before:

- Deleting files or data
- Running commands that modify the system
- Deploying to production
- Making financial transactions
- Any operation where a wrong move can't be easily undone

The LLM should call this tool on its own when it recognizes that a proposed action needs human approval. You can also configure hooks or prompts that encourage the agent to seek confirmation for specific categories of actions.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `message` | string | Yes | The question to display to the user. Should clearly describe the action and its consequences. |
| `summary` | string | No | A short summary of the action. Useful for UI displays that need a concise title alongside the detailed message. |

The engine also injects an `execution_mode` parameter at runtime.

## Response

### User approves

```json
{
  "success": true,
  "data": "action approved"
}
```

### User declines

```json
{
  "success": false,
  "error": "user declined the action"
}
```

If the user declines, the agent receives the error and should not proceed with the action. The agent should acknowledge the refusal and offer alternatives.

## Example

```json
{
  "tool": "confirm_action",
  "parameters": {
    "message": "I'm about to delete the staging database and recreate it with the new schema. This will erase all data in the staging environment. Should I proceed?",
    "summary": "Delete and recreate staging database"
  }
}
```

## How It Works Internally

The tool calls `chatCtx.RequestInput()` with an `InterruptApproval` type. This creates an interrupt that:

1. Pauses the agent's execution loop
2. Sends the approval request to the frontend via SSE events
3. Waits for the user's response (approve or decline)
4. Resumes execution with the user's decision

The `InputRequest` structure includes the tool name and arguments, so the frontend can display context about what the agent is trying to do.

## Security Considerations

- The tool blocks the agent's execution until the user responds. There is no timeout on the user's side. If the user never responds, the agent stays paused.
- The approval request includes the full tool arguments (`ToolArgs`), giving the user complete visibility into what the agent plans to do.
- There is no way for the agent to bypass this check. If the tool returns "user declined", the agent must respect it.

## Source

`core/capabilities/tools/hitl.go`