# Code Executor Tool

**Tool name:** `code_executor`

**Capability:** `tools.code_executor`

Executes Python or JavaScript code snippets in a sandboxed environment. The agent sends a code string and a language identifier, and receives the output (stdout, stderr, exit code).

## When to Use

Use this tool when the agent needs to:

- Compute something that's easier to express as code than as text
- Validate data by running a quick script
- Generate output by running a transformation or calculation
- Prototype logic before integrating it into a larger workflow

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `language` | string | Yes | Programming language. Accepted values: `python`, `javascript` |
| `code` | string | Yes | The code to execute |

The engine also injects an `execution_mode` parameter (`sync`, `concurrent`, `async`) at runtime. See the [overview](overview.md#execution-modes) for details.

## Response

### Success

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

### Failure

```json
{
  "success": false,
  "data": {
    "stdout": "",
    "stderr": "Traceback (most recent call last):\n  ...\nNameError: name 'x' is not defined\n",
    "exit_code": 1,
    "error": "exit status 1"
  }
}
```

### Unsupported language

```json
{
  "success": false,
  "error": "unsupported language: ruby"
}
```

## Example

### Python

```json
{
  "tool": "code_executor",
  "parameters": {
    "language": "python",
    "code": "import json\nresult = {'status': 'ok', 'count': 42}\nprint(json.dumps(result))"
  }
}
```

### JavaScript

```json
{
  "tool": "code_executor",
  "parameters": {
    "language": "javascript",
    "code": "const fib = n => n <= 1 ? n : fib(n-1) + fib(n-2);\nconsole.log(fib(10));"
  }
}
```

## Timeout

Code execution has a hard timeout of 30 seconds. If the code runs longer than that, the process is killed and the tool returns an error.

## Security Considerations

- Code runs directly on the host via `python3 -c` or `node -e`. It is not Docker-isolated by default.
- The code has the same filesystem and network access as the CopCon process.
- There is no sandboxing beyond the timeout. If you need stronger isolation, deploy CopCon in a containerized environment.
- The 30-second timeout prevents runaway processes, but it won't stop code that does damage quickly.

## Configuration

| Option | Default | Description |
|--------|---------|-------------|
| Timeout | 30s | `CodeExecutionTimeout` constant in `code_executor.go`. Change this value in the source if you need a different limit. |

## Source

`core/capabilities/tools/code_executor.go`