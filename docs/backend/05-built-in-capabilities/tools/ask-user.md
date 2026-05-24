# Ask User Tool

**Tool name:** `ask_user`

**Capability:** `tools.ask_user`

**Depends on:** none

Asks the user a question and waits for their response. The agent can provide predefined options (multiple choice) or let the user type free text.

This is the second of CopCon's Human-in-the-Loop (HITL) tools. Use it when the agent needs information it can't infer from context alone.

## When to Use

Use this tool when the agent needs to:

- Clarify an ambiguous instruction
- Ask which option the user prefers
- Get a value the agent can't determine on its own (API keys, credentials, specific file paths, names)
- Confirm understanding before proceeding with a complex task

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `message` | string | Yes | The question to ask the user |
| `options` | object[] | No | Predefined choices. Each option has `label` (display text) and `value` (returned as the answer). When provided, the user selects from a list instead of typing free text. |

Option object:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `label` | string | Yes | Display text shown to the user |
| `value` | string | Yes | The value returned when selected |

The engine also injects an `execution_mode` parameter at runtime.

## Response

### User answers (free text)

```json
{
  "success": true,
  "data": {
    "answer": "Use the PostgreSQL connection string in .env"
  }
}
```

### User answers (multiple choice)

```json
{
  "success": true,
  "data": {
    "answer": "staging"
  }
}
```

The `data` field contains the `Content` map from the user's `InputResponse`. For free text, the key is typically `"answer"`. For multiple choice, it's the `value` from the selected option.

### User cancels

```json
{
  "success": false,
  "error": "user cancelled the input request"
}
```

## Example

### Free text question

```json
{
  "tool": "ask_user",
  "parameters": {
    "message": "What database should I connect to for this migration?"
  }
}
```

### Multiple choice question

```json
{
  "tool": "ask_user",
  "parameters": {
    "message": "Which environment should I deploy to?",
    "options": [
      {"label": "Staging (safe, for testing)", "value": "staging"},
      {"label": "Production (live, affects users)", "value": "production"},
      {"label": "Development (local, no impact)", "value": "development"}
    ]
  }
}
```

## How It Works Internally

When `options` are provided, the tool builds an `inputSchema` with an `enum` constraint and `optionLabels` map. This schema is sent to the frontend as part of the `InputRequest`, enabling the UI to render a dropdown or radio button list instead of a free text input.

The tool calls `chatCtx.RequestInput()` with an `InterruptQuestion` type. This:

1. Pauses the agent's execution loop
2. Sends the question to the frontend via SSE events
3. Waits for the user's response
4. Resumes execution with the answer

If no options are provided, the frontend renders a free text input field.

## Security Considerations

- Like `confirm_action`, this tool blocks execution until the user responds. No timeout on the user's side.
- Never use this tool to ask for secrets (passwords, API keys, tokens). The response is stored in the message history. Use environment variables or a secret store instead.
- The question and options are visible in the frontend, so avoid including sensitive context in the `message` parameter.

## Source

`core/capabilities/tools/hitl.go`