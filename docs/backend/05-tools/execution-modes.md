# 工具执行模式

CopCon 的 Agent Engine 支持三种工具执行模式。LLM 在调用每个工具时可以通过 `execution_mode` 参数选择执行方式。此参数由 `ToolManager` 自动注入到每个工具的 Schema 中。

## 模式概览

| 模式 | 常量 | 阻塞主循环 | 并发执行 | 适用场景 |
|------|------|-----------|---------|---------|
| `sync` | `ExecutionModeSync` | 是 | 否 | 后续依赖此结果、快速完成的工具 |
| `concurrent` | `ExecutionModeConcurrent` | 是 | 是 | 多个独立工具可以并行执行 |
| `async` | `ExecutionModeAsync` | 否 | 独立 | 长时间运行的后台任务 |

## 模式一：sync（同步执行）

**默认模式**。工具在当前 goroutine 中顺序执行，完成后 Agent 继续对话循环。

```go
func (e *engineImpl) executeSync(
    chatCtx iface.ChatContextInterface,
    toolMgr tool.ToolManager,
    tc toolCallInfo,
    args map[string]any,
    messageID string,
    stepIndex int,
    partIndices map[string]int,
    toolResults map[string]*ToolCallResult,
) error {
    // 1. 发送 running 状态事件
    chatCtx.Emit(entity.Event{
        Type: entity.EventPartUpdate,
        Data: entity.PartUpdateData{
            MessageID: messageID,
            PartIndex: partIndices[tc.ID],
            State:     "running",
        },
    })

    // 2. 触发前置 Hook
    e.hookRunner.On(hook.BeforeToolExecute, chatCtx, e.logger,
        hook.HookExtra{ToolName: &tc.Name, ToolArgs: args})

    // 3. 执行工具（阻塞）
    result, err := toolMgr.Execute(chatCtx, tc.Name, args)

    // 4. 触发后置 Hook
    if err != nil {
        e.hookRunner.On(hook.OnToolError, ...)
    } else {
        e.hookRunner.On(hook.AfterToolExecute, ...)
    }

    // 5. 发送 complete/error 状态事件
    // 6. 持久化工具结果消息
}
```

**优点**：
- 执行顺序可预测
- 错误处理简单
- Hook 完整触发（before + after）

**适用场景**：
- 工具结果直接影响下一步对话逻辑
- 工具之间需要顺序执行（如先读文件再写文件）
- 快速完成的操作

**LLM 调用方式**：

```json
{
    "name": "file_ops",
    "arguments": {
        "operation": "read",
        "path": "/data/config.json",
        "execution_mode": "sync"
    }
}
```

## 模式二：concurrent（并发执行）

多个标记为 `concurrent` 的工具在同一轮调用中通过 goroutine 池并行执行，用信号量控制并发上限。

```go
func (e *engineImpl) executeConcurrent(
    chatCtx iface.ChatContextInterface,
    toolMgr tool.ToolManager,
    toolCalls []parsedToolCall,
    messageID string,
    stepIndex int,
    partIndices map[string]int,
    toolResults map[string]*ToolCallResult,
) error {
    var (
        results = make([]toolExecutionResult, len(toolCalls))
        mu      sync.Mutex
        wg      sync.WaitGroup
    )

    for i, p := range toolCalls {
        wg.Add(1)
        go func(idx int, p parsedToolCall) {
            defer wg.Done()

            // 获取信号量（最多 5 个并发）
            if err := e.concurrencySem.Acquire(chatCtx.Context(), 1); err != nil {
                // 无法获取信号量，记录错误
                return
            }
            defer e.concurrencySem.Release(1)

            // 触发 Hook
            e.hookRunner.On(hook.BeforeToolExecute, ...)

            // 执行工具
            execResult, execErr := toolMgr.Execute(chatCtx, p.tc.Name, p.args)

            // 记录结果
            mu.Lock()
            results[idx] = toolExecutionResult{...}
            mu.Unlock()

            // 触发 Hook
            e.hookRunner.On(hook.AfterToolExecute, ...)

            // 发送完成事件
        }(i, p)
    }

    wg.Wait()

    // 按 tool call ID 排序结果并持久化
    sort.Slice(results, func(i, j int) bool {
        return results[i].tc.ID < results[j].tc.ID
    })
    // ...
}
```

**关键特性**：

1. **信号量限流**：通过 `*semaphore.Weighted`（默认权重 5）限制同时执行的工具数量
2. **互不干扰**：单个工具失败不会影响其他工具的执行
3. **结果排序**：完成后按 tool call ID 排序，保证持久化顺序一致
4. **全部等待**：主循环等待所有并发工具完成后才进入下一轮对话

**优点**：
- 多个独立工具可同时执行，减少总等待时间
- 信号量保护，不会无限创建 goroutine
- 单个失败不影响整体

**适用场景**：
- LLM 同时查询多个独立数据源（如查天气 + 查新闻）
- 批量文件操作
- 多个子任务互不依赖

**LLM 调用方式**：

```json
{
    "name": "get_weather",
    "arguments": {
        "city": "北京",
        "execution_mode": "concurrent"
    }
}
```

并发数可通过 Engine 配置修改：

```go
engine := agent.NewAgentEngine(
    agent.WithConcurrency(10), // 最多 10 个并发工具
    // ...
)
```

## 模式三：async（异步执行）

工具在独立 goroutine 中执行，Agent 主循环**不等待**结果。异步工具完成后通过事件通知前端。

```go
func (e *engineImpl) executeAsync(
    chatCtx iface.ChatContextInterface,
    toolMgr tool.ToolManager,
    tc toolCallInfo,
    args map[string]any,
    messageID string,
    stepIndex int,
    partIndices map[string]int,
) error {
    sessionID := chatCtx.SessionID()

    // 创建独立 context（5 分钟超时）
    ctx, cancel := context.WithTimeout(chatCtx.Context(), 5*time.Minute)

    // 注册到异步注册表
    e.asyncRegistry.Register(sessionID, tc.ID, tc.Name, cancel)

    // 发送 started 事件（同步部分）
    chatCtx.Emit(entity.Event{
        Type: entity.EventAsyncToolStarted,
        Data: entity.AsyncToolStartedData{
            CallID: tc.ID, ToolName: tc.Name, SessionID: sessionID,
        },
    })

    // 触发前置 Hook（仅同步部分）
    e.hookRunner.On(hook.BeforeToolExecute, ...)

    go func() {
        defer e.asyncRegistry.Unregister(tc.ID)
        defer cancel()

        // 获取信号量
        if err := e.concurrencySem.Acquire(ctx, 1); err != nil {
            e.asyncRegistry.Fail(tc.ID, err.Error())
            // 发送失败事件
            return
        }
        defer e.concurrencySem.Release(1)

        // Panic 保护
        defer func() {
            if r := recover(); r != nil {
                // 发送失败事件 + 记录错误
            }
        }()

        // 执行工具
        result, err := toolMgr.Execute(chatCtx, tc.Name, args)

        if err != nil {
            // AsyncToolFailed 事件 + 持久化
            e.asyncRegistry.Fail(tc.ID, err.Error())
        } else {
            // AsyncToolComplete 事件 + 持久化
            e.asyncRegistry.Complete(tc.ID, result)
        }
    }()

    return nil
}
```

**关键特性**：

1. **独立 context**：使用 `context.WithTimeout`（默认 5 分钟），与主 context 解耦
2. **异步注册表**：`AsyncToolRegistry` 跟踪所有异步工具的状态
3. **可取消**：注册表中保存 `CancelFunc`，外部可通过 `cancel_tool` 工具取消执行
4. **Panic 保护**：完整的 recover 机制，panic 不会导致 goroutine 泄露
5. **信号量共享**：与 concurrent 模式共用同一个信号量（默认 5）
6. **事件通知**：开始、完成、失败均通过 Event 通道推送

**优点**：
- 主循环不阻塞，可以继续处理新消息
- 适合长时间运行的任务（代码编译、数据处理等）
- 支持取消和状态查询

**适用场景**：
- 代码编译/测试
- 长时间数据处理
- 无需立即返回结果的后台任务

**LLM 调用方式**：

```json
{
    "name": "code_executor",
    "arguments": {
        "language": "python",
        "code": "import time; time.sleep(60); print('done')",
        "execution_mode": "async"
    }
}
```

## 模式选择与分发

`handleToolCalls` 负责按模式分类和分发工具调用：

```go
func (e *engineImpl) handleToolCalls(...) (bool, error) {
    var (
        syncToolCalls       []toolCallInfo
        concurrentToolCalls []parsedToolCall
        asyncToolCalls      []toolCallInfo
    )

    for _, tc := range result.ToolCalls {
        args := parseArgs(tc.Arguments)
        mode, args := parseExecutionMode(args)

        switch mode {
        case ExecutionModeSync:
            syncToolCalls = append(syncToolCalls, tc)
        case ExecutionModeConcurrent:
            concurrentToolCalls = append(concurrentToolCalls,
                parsedToolCall{tc: tc, args: args})
        case ExecutionModeAsync:
            asyncToolCalls = append(asyncToolCalls, tc)
        }
    }

    // 1. 同步工具顺序执行
    for _, tc := range syncToolCalls {
        e.executeSync(chatCtx, toolMgr, tc, args, ...)
    }

    // 2. 并发工具并行执行，等待全部完成
    if len(concurrentToolCalls) > 0 {
        e.executeConcurrent(chatCtx, toolMgr, concurrentToolCalls, ...)
    }

    // 3. 异步工具启动后不等待
    for _, tc := range asyncToolCalls {
        e.executeAsync(chatCtx, toolMgr, tc, args, ...)
    }

    return true, nil
}
```

执行原则：
- 先 sync，再 concurrent，最后 async
- concurrent 全部完成后才进入主循环下一轮
- async 启动后立即返回，结果通过事件异步通知

## execution_mode 参数提取

Agent Engine 在 `parseExecutionMode` 中从工具参数中提取模式：

```go
func parseExecutionMode(args map[string]any) (ExecutionMode, map[string]any) {
    mode := ExecutionModeSync
    if val, ok := args["execution_mode"]; ok {
        if str, ok := val.(string); ok {
            switch str {
            case "sync", "concurrent", "async":
                mode = ExecutionMode(str)
            }
        }
        delete(args, "execution_mode")
    }
    return mode, args
}
```

提取后 `execution_mode` 从 args 中删除，不会传给工具的 `Execute` 方法。未指定时默认 `sync`。

## 异步工具管理工具

CopCon 提供了三个配套工具用于管理异步任务：

- `get_tool_status`：查询异步工具执行状态
- `get_tool_result`：获取已完成工具的结果
- `cancel_tool`：取消正在执行的工具
- `list_async_tools`：列出当前会话所有异步工具

详见 [内置工具参考](./builtin-tools.md)。

## 使用建议

```
同步执行 (sync):
    ↓
    工具1 (等待)
    ↓
    LLM 继续思考
    ↓
    工具2 (等待)

并发执行 (concurrent):
    ↓
    ┌─ 工具1 ─┐
    ├─ 工具2 ─┤  (并行)
    ├─ 工具3 ─┤
    └─ 等待全部完成 ─┘
    ↓
    LLM 继续思考

异步执行 (async):
    ↓
    工具1 启动...
    ↓ (不等待)
    LLM 继续思考
    ...
    ↓ (5分钟后)
    工具1 完成 → 事件通知
```

选择建议：
- 快速操作（< 2 秒）：sync
- 多个独立查询：concurrent
- 长时间任务（> 10 秒）：async
- 结果直接影响下一轮对话：sync（不要用 async）