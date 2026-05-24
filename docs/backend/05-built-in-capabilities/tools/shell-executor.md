# Shell Executor Tool

**Tool name:** `shell_executor`

**Capability:** `tools.shell_executor`

Runs shell commands that are on a hardcoded whitelist. Any command not in the allowed list is rejected immediately.

## When to Use

Use this tool when the agent needs to:

- Inspect the filesystem (`ls`, `find`, `cat`, `head`, `tail`, `wc`)
- Search file contents (`grep`)
- Check environment details (`pwd`, `whoami`, `which`, `date`, `echo`)

This is a read-only inspection tool. It can't modify files, install packages, or run arbitrary commands.

## Allowed Commands

The whitelist is hardcoded in `code_executor.go`:

| Command | Typical use |
|---------|-------------|
| `ls` | List directory contents |
| `cat` | Print file contents |
| `echo` | Print text or variables |
| `pwd` | Print working directory |
| `date` | Print current date/time |
| `whoami` | Print current user |
| `which` | Locate a command's path |
| `head` | Print first lines of a file |
| `tail` | Print last lines of a file |
| `wc` | Count lines, words, bytes |
| `find` | Search for files by name or pattern |
| `grep` | Search file contents for patterns |

Any command not on this list returns an error.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | Yes | The shell command to execute. Only whitelisted base commands are accepted. |

The engine also injects an `execution_mode` parameter (`sync`, `concurrent`, `async`) at runtime.

## Response

### Success

```json
{
  "success": true,
  "data": {
    "output": "main.go\nconfig.yaml\nREADME.md\n",
    "exit_code": 0
  }
}
```

### Command not allowed

```json
{
  "success": false,
  "error": "command 'rm' is not in the allowed list"
}
```

### Command failed

```json
{
  "success": false,
  "data": {
    "output": "No such file or directory\n",
    "exit_code": 1,
    "error": "exit status 1"
  }
}
```

## Example

```json
{
  "tool": "shell_executor",
  "parameters": {
    "command": "ls -la /workspace"
  }
}
```

```json
{
  "tool": "shell_executor",
  "parameters": {
    "command": "grep -r \"func main\" /workspace"
  }
}
```

## Timeout

Shell commands have a 10-second timeout. Long-running commands are killed automatically.

## Security Considerations

- The whitelist prevents destructive commands (`rm`, `chmod`, `sudo`, `curl`, `wget`, etc.).
- Commands run via `sh -c`, so you can pass flags and arguments (e.g. `ls -la`, `grep -r`).
- The base command (first word) must be on the whitelist. Arguments are not checked separately, so `cat /etc/passwd` would pass the whitelist check but might expose sensitive data if the path is accessible.
- The 10-second timeout is shorter than the code executor's, reflecting the expected quick nature of these commands.

## Configuration

| Option | Default | Description |
|--------|---------|-------------|
| Timeout | 10s | Hardcoded in `NewShellExecutor()`. Change in source if needed. |
| Allowed commands | Fixed list | `allowedShellCommands` map in `code_executor.go`. Modify the map to add or remove commands. |

## Source

`core/capabilities/tools/code_executor.go` (the shell executor is defined in the same file)