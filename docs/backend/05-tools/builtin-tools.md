# 内置工具参考

CopCon 内置以下工具，在 Engine 初始化时自动或手动注册。

## shell_executor — Shell 命令执行

**名称**：`shell_executor`

**用途**：在白名单范围内执行 shell 命令。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `command` | `string` | 是 | 要执行的 shell 命令 |

### 白名单

命令名必须在下表中才能执行：

```go
var allowedShellCommands = map[string]bool{
    "ls":     true,
    "cat":    true,
    "echo":   true,
    "pwd":    true,
    "date":   true,
    "whoami": true,
    "which":  true,
    "head":   true,
    "tail":   true,
    "wc":     true,
    "find":   true,
    "grep":   true,
}
```

### 超时

默认 10 秒。

### 返回结果

**成功**：
```json
{
    "output": "file1.go\nfile2.go\n",
    "exit_code": 0
}
```

**失败（命令不在白名单中）**：
```json
{
    "success": false,
    "error": "command 'rm' is not in the allowed list"
}
```

### Agent 提示词中如何使用

```
使用 shell_executor 查询文件列表: ls -la /data/
搜索 Go 文件中的函数定义: grep -rn "func.*Weather" /data/
查看文件前 50 行: head -n 50 /data/config.yaml
```

### LLM 调用示例

```json
{
    "name": "shell_executor",
    "arguments": {
        "command": "ls -la /data/copcon/server/internal/tools/"
    }
}
```

### 完整代码

```go
type ShellExecutor struct {
    timeout time.Duration
}

func NewShellExecutor() *ShellExecutor {
    return &ShellExecutor{timeout: 10 * time.Second}
}

func (t *ShellExecutor) Name() string        { return "shell_executor" }
func (t *ShellExecutor) Description() string { return "Execute allowed shell commands (whitelist enforced)" }

func (t *ShellExecutor) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "command": map[string]any{
                "type": "string",
                "description": "Shell command to execute",
            },
        },
        "required": []string{"command"},
    }
}

func (t *ShellExecutor) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    command, ok := args["command"].(string)
    if !ok {
        return &tool.ToolResult{Success: false, Error: "command is required"}, nil
    }

    parts := strings.Fields(command)
    if len(parts) == 0 {
        return &tool.ToolResult{Success: false, Error: "empty command"}, nil
    }

    cmdName := parts[0]
    if !allowedShellCommands[cmdName] {
        return &tool.ToolResult{
            Success: false,
            Error:   fmt.Sprintf("command '%s' is not in the allowed list", cmdName),
        }, nil
    }

    execCtx, cancel := context.WithTimeout(chatCtx.Context(), t.timeout)
    defer cancel()

    cmd := exec.CommandContext(execCtx, "sh", "-c", command)
    output, err := cmd.CombinedOutput()

    if err != nil {
        return &tool.ToolResult{
            Success: false,
            Data: map[string]any{
                "output":    string(output),
                "exit_code": 1,
                "error":     err.Error(),
            },
        }, nil
    }

    return &tool.ToolResult{
        Success: true,
        Data: map[string]any{
            "output":    string(output),
            "exit_code": 0,
        },
    }, nil
}
```

---

## code_executor — 代码执行

**名称**：`code_executor`

**用途**：在沙箱环境中执行 Python 或 JavaScript 代码。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `language` | `string` | 是 | 编程语言：`"python"` 或 `"javascript"` |
| `code` | `string` | 是 | 要执行的代码 |

### 超时

默认 30 秒。

### 返回结果

**成功（Python）**：
```json
{
    "stdout": "Hello, World!\n",
    "stderr": "",
    "exit_code": 0
}
```

**失败（语法错误）**：
```json
{
    "success": false,
    "data": {
        "stdout": "",
        "stderr": "  File \"<string>\", line 1\n    print(hello\n              ^\nSyntaxError: '(' was never closed\n",
        "exit_code": 1,
        "error": "exit status 1"
    }
}
```

### Agent 提示词中如何使用

```
使用 code_executor 运行 Python:
    language: python
    code: |
        import json
        data = {"a": 1, "b": 2}
        print(json.dumps(data, indent=2))

使用 code_executor 运行 JavaScript:
    language: javascript
    code: |
        const arr = [1, 2, 3, 4, 5];
        console.log(arr.reduce((a, b) => a + b, 0));
```

### LLM 调用示例

```json
{
    "name": "code_executor",
    "arguments": {
        "language": "python",
        "code": "print(sum(range(1, 101)))"
    }
}
```

### 完整代码

```go
type CodeExecutor struct {
    timeout time.Duration
}

func NewCodeExecutor() *CodeExecutor {
    return &CodeExecutor{timeout: 30 * time.Second}
}

func (t *CodeExecutor) Name() string        { return "code_executor" }
func (t *CodeExecutor) Description() string { return "Execute Python or JavaScript code in a sandboxed environment" }

func (t *CodeExecutor) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "language": map[string]any{
                "type": "string",
                "enum": []string{"python", "javascript"},
                "description": "Programming language",
            },
            "code": map[string]any{
                "type":        "string",
                "description": "Code to execute",
            },
        },
        "required": []string{"language", "code"},
    }
}

func (t *CodeExecutor) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    language, ok := args["language"].(string)
    if !ok {
        return &tool.ToolResult{Success: false, Error: "language is required"}, nil
    }

    code, ok := args["code"].(string)
    if !ok {
        return &tool.ToolResult{Success: false, Error: "code is required"}, nil
    }

    execCtx, cancel := context.WithTimeout(chatCtx.Context(), t.timeout)
    defer cancel()

    var cmd *exec.Cmd
    switch language {
    case "python":
        cmd = exec.CommandContext(execCtx, "python3", "-c", code)
    case "javascript":
        cmd = exec.CommandContext(execCtx, "node", "-e", code)
    default:
        return &tool.ToolResult{
            Success: false,
            Error:   fmt.Sprintf("unsupported language: %s", language),
        }, nil
    }

    output, err := cmd.CombinedOutput()
    if err != nil {
        return &tool.ToolResult{
            Success: false,
            Data: map[string]any{
                "stdout":    "",
                "stderr":    string(output),
                "exit_code": 1,
                "error":     err.Error(),
            },
        }, nil
    }

    return &tool.ToolResult{
        Success: true,
        Data: map[string]any{
            "stdout":    string(output),
            "stderr":    "",
            "exit_code": 0,
        },
    }, nil
}
```

---

## file_ops — 文件操作

**名称**：`file_ops`

**用途**：在工作目录内读写文件和列出目录内容。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `operation` | `string` | 是 | 操作类型：`"read"`、`"write"`、`"list"` |
| `path` | `string` | 是 | 文件或目录路径 |
| `content` | `string` | write 时必填 | 要写入的内容 |

### 限制

- 单文件最大 10 MB
- 写入时自动创建父目录（`os.MkdirAll`）
- 文件权限 0644

### 返回结果

**read 成功**：
```json
{
    "content": "package main\n\nimport \"fmt\"\n...",
    "size": 1234
}
```

**write 成功**：
```json
{
    "path": "/data/copcon/output.txt",
    "size": 567
}
```

**list 成功**：
```json
{
    "files": [
        {"name": "main.go", "isDir": false, "size": 1234},
        {"name": "internal", "isDir": true, "size": 4096}
    ],
    "count": 2
}
```

### Agent 提示词中如何使用

```
读取文件:
    file_ops: {operation: "read", path: "/data/copcon/server/internal/tool/manager.go"}

写入文件:
    file_ops: {operation: "write", path: "/data/copcon/output.csv", content: "name,age\nAlice,30\nBob,25"}

列出目录:
    file_ops: {operation: "list", path: "/data/copcon/server/internal/"}
```

### LLM 调用示例

```json
{
    "name": "file_ops",
    "arguments": {
        "operation": "read",
        "path": "/data/copcon/config.yaml"
    }
}
```

### 完整代码

```go
type FileOps struct {
    workDir string
}

func NewFileOps(workDir string) *FileOps {
    if workDir == "" {
        workDir, _ = os.Getwd()
    }
    return &FileOps{workDir: workDir}
}

func (t *FileOps) Name() string        { return "file_ops" }
func (t *FileOps) Description() string { return "Read and write files within the working directory" }

func (t *FileOps) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "operation": map[string]any{
                "type": "string",
                "enum": []string{"read", "write", "list"},
                "description": "File operation to perform",
            },
            "path": map[string]any{
                "type":        "string",
                "description": "File or directory path",
            },
            "content": map[string]any{
                "type":        "string",
                "description": "Content to write (for write operation)",
            },
        },
        "required": []string{"operation", "path"},
    }
}

func (t *FileOps) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    operation, ok := args["operation"].(string)
    if !ok {
        return &tool.ToolResult{Success: false, Error: "operation is required"}, nil
    }

    path, ok := args["path"].(string)
    if !ok {
        return &tool.ToolResult{Success: false, Error: "path is required"}, nil
    }

    switch operation {
    case "read":
        return t.readFile(path)
    case "write":
        content, ok := args["content"].(string)
        if !ok {
            return &tool.ToolResult{Success: false, Error: "content is required for write"}, nil
        }
        return t.writeFile(path, content)
    case "list":
        return t.listDir(path)
    default:
        return &tool.ToolResult{
            Success: false,
            Error:   fmt.Sprintf("unknown operation: %s", operation),
        }, nil
    }
}
```

---

## todolist — 任务管理

**名称**：`todolist`

**用途**：管理会话中的任务列表。Agent 用它来规划步骤、跟踪进度。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `action` | `string` | 是 | 操作：`create`、`start`、`complete`、`fail`、`list`、`replan` |
| `todo_id` | `string` | start/complete/fail 时必填 | 任务 ID |
| `content` | `string` | create 时必填 | 任务内容描述 |
| `validation` | `string` | 否 | 验证规则（create 和 fail 时使用） |
| `depends_on` | `[]string` | 否 | 依赖的其他任务 ID 列表 |
| `result` | `string` | complete 时必填 | 完成结果的描述 |
| `reason` | `string` | 否 | 失败原因（fail 时） |
| `todos` | `[]object` | replan 时必填 | 新任务列表 |

### 操作详解

#### create — 创建任务

```json
{
    "action": "create",
    "content": "读取用户配置文件",
    "validation": "能成功解析 JSON",
    "depends_on": []
}
```

**返回**：
```json
{
    "id": "a1b2c3d4-...",
    "content": "读取用户配置文件",
    "status": "pending",
    "message": "Todo created successfully"
}
```

#### start — 开始执行任务

```json
{
    "action": "start",
    "todo_id": "a1b2c3d4-..."
}
```

**返回**：
```json
{
    "id": "a1b2c3d4-...",
    "content": "读取用户配置文件",
    "status": "in_progress",
    "message": "Todo started successfully"
}
```

#### complete — 完成任务

```json
{
    "action": "complete",
    "todo_id": "a1b2c3d4-...",
    "result": "成功解析 config.yaml，获得 3 个配置项"
}
```

**返回**：
```json
{
    "id": "a1b2c3d4-...",
    "content": "读取用户配置文件",
    "status": "completed",
    "result": "成功解析 config.yaml，获得 3 个配置项",
    "message": "Todo completed successfully"
}
```

#### fail — 标记为失败

```json
{
    "action": "fail",
    "todo_id": "a1b2c3d4-...",
    "reason": "文件不存在"
}
```

**返回**：
```json
{
    "id": "a1b2c3d4-...",
    "content": "读取用户配置文件",
    "status": "failed",
    "retry_count": 1,
    "message": "Todo marked as failed"
}
```

`reason` 或 `validation` 字段记录失败原因。

#### list — 列出全部任务

```json
{
    "action": "list"
}
```

**返回**：
```json
{
    "todos": [
        {
            "id": "a1b2c3d4-...",
            "session_id": "sess-123",
            "content": "读取用户配置文件",
            "status": "completed",
            "created_at": "2026-05-17T10:00:00Z",
            "updated_at": "2026-05-17T10:01:00Z",
            "retry_count": 0,
            "result": "成功解析 config.yaml",
            "completed_at": "2026-05-17T10:01:00Z"
        },
        {
            "id": "b2c3d4e5-...",
            "content": "编写测试用例",
            "status": "in_progress",
            "validation": "所有测试通过"
        }
    ],
    "count": 2
}
```

#### replan — 重新规划

替换当前会话的全部任务列表：

```json
{
    "action": "replan",
    "todos": [
        {"content": "第一步：初始化项目", "validation": "项目目录已创建"},
        {"content": "第二步：编写核心逻辑"},
        {"content": "第三步：测试", "validation": "所有测试通过", "depends_on": ["第二步的ID"]}
    ]
}
```

删除所有旧任务后按顺序创建新任务。

### Task 状态机

```
pending ──start──→ in_progress
                      │
            ┌─────────┼─────────┐
            ↓                   ↓
        completed            failed
```

`failed` 状态可以重试（`retry_count` 递增），重新 start。

### Agent 提示词中的典型用法

Agent 在开始复杂任务时应先规划：

```
1. 调用 todolist(action="create") 创建任务列表
2. 按顺序执行每个任务：
   todolist(action="start", todo_id="...")
   执行实际工作...
   todolist(action="complete", todo_id="...", result="...")
3. 如果任务失败：
   todolist(action="fail", todo_id="...", reason="...")
4. 遇到新计划：
   todolist(action="replan", todos=[...])
```

### 依赖管理

`depends_on` 字段用于声明任务依赖关系：

```json
{
    "action": "create",
    "content": "测试 API 端点",
    "depends_on": ["step-1-id", "step-2-id"]
}
```

当 `step-1` 和 `step-2` 都完成后，`测试 API 端点` 才应该被 start。依赖关系由 Agent 自行管理，TodoManager 不强制校验。

---

## 异步管理工具

CopCon 额外内置了三个异步工具管理的工具，用于在 async 模式下查询和控制工具执行。

### get_tool_status — 查询异步工具状态

**名称**：`get_tool_status`

**参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `call_id` | `string` | 是 | 工具调用的唯一 ID |

**返回（运行中）**：
```json
{
    "call_id": "call-abc123",
    "tool_name": "code_executor",
    "status": "running",
    "start_time": "2026-05-17T10:05:00Z"
}
```

**返回（已完成）**：
```json
{
    "call_id": "call-abc123",
    "tool_name": "code_executor",
    "status": "completed",
    "start_time": "2026-05-17T10:05:00Z",
    "end_time": "2026-05-17T10:06:30Z",
    "duration": "1m30s",
    "result": {...}
}
```

**返回（失败）**：
```json
{
    "call_id": "call-abc123",
    "tool_name": "code_executor",
    "status": "failed",
    "start_time": "2026-05-17T10:05:00Z",
    "end_time": "2026-05-17T10:05:02Z",
    "duration": "2s",
    "error": "command not found: python3"
}
```

### get_tool_result — 获取异步工具结果

**名称**：`get_tool_result`

仅在工具状态为 `completed` 时可用。否则返回错误。

**LLM 调用示例**：
```json
{
    "name": "get_tool_result",
    "arguments": {
        "call_id": "call-abc123"
    }
}
```

### cancel_tool — 取消异步工具

**名称**：`cancel_tool`

只能取消 `running` 状态的工具。通过调用注册表中保存的 `CancelFunc` 取消执行。

**LLM 调用示例**：
```json
{
    "name": "cancel_tool",
    "arguments": {
        "call_id": "call-abc123"
    }
}
```

### list_async_tools — 列出会话中所有异步工具

**名称**：`list_async_tools`

**LLM 调用示例**：
```json
{
    "name": "list_async_tools",
    "arguments": {}
}
```

**返回**：
```json
{
    "tools": [
        {
            "call_id": "call-abc123",
            "tool_name": "code_executor",
            "status": "running",
            "start_time": "2026-05-17T10:05:00Z"
        },
        {
            "call_id": "call-def456",
            "tool_name": "code_executor",
            "status": "completed",
            "start_time": "2026-05-17T10:03:00Z",
            "duration_ms": 5000
        }
    ],
    "count": 2
}
```

---

## 工具注册总览

Engine 初始化时典型的注册顺序：

```go
func main() {
    // 1. 创建基础组件
    registry := tool.NewToolRegistry()
    asyncRegistry := tool.NewAsyncToolRegistry()

    // 2. 注册内置工具
    registry.Register(tools.NewFileOps(""))
    registry.Register(tools.NewCodeExecutor())
    registry.Register(tools.NewShellExecutor())

    // 3. 注册 todo 工具（需要 TodoManager）
    todoMgr := todo.NewTodoManager(db)
    registry.Register(tools.NewTodoTool(todoMgr))

    // 4. 注册异步管理工具
    registry.Register(tools.NewGetToolStatusTool(asyncRegistry))
    registry.Register(tools.NewGetToolResultTool(asyncRegistry))
    registry.Register(tools.NewCancelToolTool(asyncRegistry))
    registry.Register(tools.NewListAsyncToolsTool(asyncRegistry))

    // 5. 创建 ToolManager 并同步注册
    toolMgr := tool.NewToolManager()
    for _, info := range registry.List() {
        t, _ := registry.Get(info.Name)
        toolMgr.Register(t)
    }

    // 6. 创建 Engine
    engine := agent.NewAgentEngine(
        agent.WithToolManager(toolMgr),
        // ... 其他配置
    )
}
```