# File Ops Tool

**Tool name:** `file_ops`

**Capability:** `tools.file_ops`

Reads, writes, and lists files within the agent's working directory. All operations are scoped to the working directory. Paths outside it, or in system directories, are rejected.

## When to Use

Use this tool when the agent needs to:

- Read a file's contents (`read` operation)
- Write or create a file (`write` operation)
- List the contents of a directory (`list` operation)

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `operation` | string | Yes | One of: `read`, `write`, `list` |
| `path` | string | Yes | File or directory path. Must be within the working directory. |
| `content` | string | No | Content to write. Required when `operation` is `write`. |

The engine also injects an `execution_mode` parameter at runtime.

## Operations

### `read`

Reads a file's contents. Returns the content as a string and the file size in bytes.

Files larger than 10 MB are rejected.

```json
{
  "success": true,
  "data": {
    "content": "package main\n\nfunc main() {\n  fmt.Println(\"hello\")\n}\n",
    "size": 52
  }
}
```

### `write`

Writes content to a file. Creates parent directories if they don't exist. Returns the file path and the number of bytes written.

Content larger than 10 MB is rejected.

```json
{
  "success": true,
  "data": {
    "path": "/workspace/output.txt",
    "size": 128
  }
}
```

If the file already exists, it is overwritten.

### `list`

Lists entries in a directory. Returns each entry's name, whether it's a directory, and its size.

```json
{
  "success": true,
  "data": {
    "files": [
      {"name": "main.go", "isDir": false, "size": 245},
      {"name": "config", "isDir": true, "size": 4096},
      {"name": "README.md", "isDir": false, "size": 1024}
    ],
    "count": 3
  }
}
```

## Example

### Read a file

```json
{
  "tool": "file_ops",
  "parameters": {
    "operation": "read",
    "path": "/workspace/src/main.go"
  }
}
```

### Write a file

```json
{
  "tool": "file_ops",
  "parameters": {
    "operation": "write",
    "path": "/workspace/output/result.txt",
    "content": "Task completed successfully.\nNo errors found."
  }
}
```

### List a directory

```json
{
  "tool": "file_ops",
  "parameters": {
    "operation": "list",
    "path": "/workspace/src"
  }
}
```

## Path Restrictions

The tool enforces two layers of path protection:

1. **Working directory scope**: The resolved absolute path must start with the working directory's absolute path. If the tool's `workDir` is `/workspace`, then only paths under `/workspace` are allowed.

2. **Forbidden system paths**: The following prefixes are always blocked, regardless of the working directory:

   | Forbidden path | Reason |
   |----------------|--------|
   | `/etc` | System configuration |
   | `/root` | Root user home |
   | `/home` | User home directories |
   | `/var` | System data and logs |
   | `/usr` | System programs and libraries |
   | `/bin` | System binaries |
   | `/sbin` | System administration binaries |
   | `/lib` | System libraries |

If a path fails either check, the tool returns an error.

## Size Limits

| Limit | Value | Applies to |
|-------|-------|-----------|
| Max file size (read) | 10 MB | `MaxFileSize` constant |
| Max content size (write) | 10 MB | `MaxFileSize` constant |

## Security Considerations

- The path check uses `filepath.Abs()` to resolve relative paths and symlinks before comparing. This prevents `../../etc/passwd` style attacks.
- Files are written with permissions `0644` (owner read/write, everyone else read-only). Directories are created with `0755`.
- There is no file locking. Concurrent writes to the same file may overwrite each other.
- The working directory defaults to the process's current directory if not explicitly configured.

## Configuration

| Option | Default | Description |
|--------|---------|-------------|
| `workDir` | Current working directory | Set via `NewFileOps(workDir)`. The capability registration uses `""`, which falls back to `os.Getwd()`. |
| `MaxFileSize` | 10 MB | Constant in `file_ops.go`. Change in source if needed. |

## Source

`core/capabilities/tools/file_ops.go`