# AgentEngine 引擎

## 概述

`AgentEngine` 是 CopCon 后端的核心组件，定义在 `internal/agent/engine.go`。它负责编排 Agent 的"思考-行动"循环：接收用户输入，反复调用 LLM 并执行工具，直到 LLM 输出纯文本回复。整个过程中通过 Hook 系统暴露拦截点，通过 ChatContext 推送流式事件到前端。

## 接口定义

```go
// AgentEngine 定义 Agent 引擎的公共接口。
type AgentEngine interface {
    Chat(chatCtx iface.ChatContextInterface, userInput string) error
}
```

只有一个方法：`Chat`。

- `chatCtx` 携带会话标识、Agent 标识和事件通道
- `userInput` 是用户输入文本。可以为空字符串（用于异步工具完成后触发新一轮 Agent Loop）
- 返回的 `error` 仅表示启动层面的失败；业务错误通过 `chatCtx.Emit(Event{Type: EventError, ...})` 发送

## 实现结构

```go
type engineImpl struct {
    logger         *slog.Logger
    agentRegistry  AgentRegistry          // Agent 定义注册表
    sessionMgr     session.SessionManager  // 会话管理器
    contextMgr     chat_context.ContextManager // 上下文管理器
    llmProvider    llm.LLMProvider         // LLM provider（可覆盖 Agent 定义中的）
    concurrency    int                     // 最大并发工具执行数
    concurrencySem *semaphore.Weighted     // 信号量
    asyncRegistry  *tool.AsyncToolRegistry // 异步工具注册表
    hookRunner     hook.HookRunner         // 可选的 Hook 执行器
}
```

## 构造函数

```go
func NewAgentEngine(
    agentRegistry AgentRegistry,
    sessionMgr    session.SessionManager,
    contextMgr    chat_context.ContextManager,
    asyncRegistry *tool.AsyncToolRegistry,
    opts ...EngineOption,
) AgentEngine
```

必需参数：
- `agentRegistry` — Agent 定义注册表
- `sessionMgr` — 会话管理器
- `contextMgr` — 上下文管理器
- `asyncRegistry` — 异步工具注册表

默认值：
- `hookRunner`: `hook.NewEmptyRunner()`（无 Hook 的空 Runner）
- `concurrency`: 5
- `logger`: `slog.New(slog.NewTextHandler(os.Stderr, nil))`

## EngineOption 选项

### WithHookRunner

```go
// WithHookRunner 设置 Hook 执行器。
// 为 nil 时（默认），不执行任何 Hook。
func WithHookRunner(runner hook.HookRunner) EngineOption
```

使用示例：

```go
runner := hook.NewHookRunner()
runner.Register(myMemoryHook)
runner.Register(myLoggingHook)

engine := agent.NewAgentEngine(
    registry, sessionMgr, contextMgr, asyncReg,
    agent.WithHookRunner(runner),
)
```

### WithLLMProvider

```go
// WithLLMProvider 设置 LLM 提供者。
// 为 nil 时（默认），使用 Agent 定义中配置的 Provider。
func WithLLMProvider(p llm.LLMProvider) EngineOption
```

使用示例：

```go
// 全局替换 LLM 后端
engine := agent.NewAgentEngine(
    registry, sessionMgr, contextMgr, asyncReg,
    agent.WithLLMProvider(myCustomProvider),
)
```

### WithConcurrency

```go
// WithConcurrency 设置最大并发工具执行数。
// n 必须大于 0，否则 panic。
func WithConcurrency(n int) EngineOption
```

使用示例：

```go
engine := agent.NewAgentEngine(
    registry, sessionMgr, contextMgr, asyncReg,
    agent.WithConcurrency(10), // 允许 10 个工具同时执行
)
```

### WithLogger

```go
// WithLogger 设置结构化日志记录器。
func WithLogger(logger *slog.Logger) EngineOption
```

使用示例：

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
engine := agent.NewAgentEngine(
    registry, sessionMgr, contextMgr, asyncReg,
    agent.WithLogger(logger),
)
```

## 生命周期

一次 `Chat()` 调用经历以下阶段：

```
Chat(chatCtx, userInput)
    │
    ├── 1. prepareAgentLoop()   初始化准备
    │       ├── 加载会话
    │       ├── 解析 Agent（3层回退）
    │       └── 持久化用户消息
    │
    ├── 2. runAgentLoop()       主循环
    │       │
    │       ├── 2.1 构建上下文 + Hook 拦截
    │       │       ├── OnSystemPrompt
    │       │       ├── BeforeContextBuild
    │       │       ├── BuildContext()
    │       │       ├── AfterContextBuild
    │       │       └── BeforeLLMCall
    │       │
    │       ├── 2.2 handleStreaming()  流式调用 LLM
    │       │       ├── LLMProvider.Stream(params)
    │       │       ├── 实时 part_create / part_update
    │       │       └── 返回 StreamResult
    │       │
    │       ├── 2.3 AfterLLMCall Hook
    │       │
    │       ├── 2.4 handleToolCalls()  工具调用
    │       │       ├── BeforeToolExecute / AfterToolExecute / OnToolError
    │       │       ├── ToolManager.Execute()
    │       │       └── 持久化 tool 消息
    │       │
    │       ├── 2.5 persistMessage()   持久化助手消息
    │       │       └── OnMessagePersist Hook
    │       │
    │       └── 2.6 判断是否继续
    │               ├── 有 tool_calls → stepIndex++, goto 2.1
    │               └── 无 tool_calls → Emit message_done, return
    │
    └── 3. 错误处理
            └── chatCtx.Emit(EventError)
```

### Stage 1: prepareAgentLoop

```go
func (e *engineImpl) prepareAgentLoop(
    chatCtx iface.ChatContextInterface,
    userInput string,
) (*AgentDefinition, error)
```

初始化阶段做三件事：

1. **加载会话** — `e.sessionMgr.Get(chatCtx)` 获取当前会话
2. **解析 Agent** — 3 层回退策略：

```
chatCtx.AgentID()          ← 请求中指定
    ↓ 为空时
sess.DefaultAgentID        ← 会话默认 Agent
    ↓ 为空时
registry.Default()         ← 全局默认 Agent
    ↓ 为空时
返回 error: "no agent specified and no default agent"
```

3. **持久化用户消息** — 如果 `userInput` 不为空，写入 `contextMgr.AddMessage()`

### Stage 2: runAgentLoop

多轮迭代循环，结构如下：

```go
func (e *engineImpl) runAgentLoop(
    chatCtx iface.ChatContextInterface,
    userInput string,
) error {
    agentDef, err := e.prepareAgentLoop(chatCtx, userInput)
    // ...

    tools := agentDef.ToolManager.GetOpenAITools()
    messageID := uuid.New().String()

    isFirstIteration := true
    stepIndex := 0

    for {
        // 非首轮：发送 step_create 事件
        if !isFirstIteration {
            chatCtx.Emit(entity.Event{
                Type: entity.EventStepCreate,
                Data: entity.StepCreateData{
                    MessageID: messageID,
                    StepIndex: stepIndex,
                },
            })
        }
        isFirstIteration = false

        // Hook 系列 + Context 构建 + LLM 调用 + 工具执行
        // ...

        if !shouldContinue {
            return nil  // LLM 返回纯文本，结束循环
        }
        stepIndex++
    }
}
```

关键点：
- 每个 `messageID` 贯穿整个 Agent Loop，标识一次完整的 `Chat()` 调用
- 第一轮不发送 `step_create`，从第二轮开始才发送
- `stepIndex` 从 0 开始，每轮工具调用后递增
- 每次 SSEResult 的 Parts 中携带 `StepIndex`，前端据此将 Part 归入对应的 Step

### Stage 3: handleStreaming

```go
func (e *engineImpl) handleStreaming(
    chatCtx iface.ChatContextInterface,
    agentDef *AgentDefinition,
    llmMessages []llm.Message,
    tools []llm.ToolDef,
    messageID string,
    stepIndex int,
) (*StreamResult, error)
```

流式调用 LLM 并实时推送事件：

1. 选择 Provider：优先 `e.llmProvider`，否则用 `agentDef.LLMProvider`
2. 调用 `provider.Stream(ctx, params)`
3. 遍历 chunk channel：
   - `chunk.Content != ""` → 发送 `part_create`（首次） + `part_update`（每次）
   - `chunk.ReasoningContent != ""` → 同上，partType 为 "reasoning"
   - `chunk.ToolCalls` → 增量累积，按 index 合并
   - `chunk.Usage != nil` → 记录 token 用量
4. Stream 结束后，发送最终 state 更新（state: "done"）
5. 检查 error channel，若有错则返回 error

### Stage 4: handleToolCalls

详见 `engine_tools.go`，核心逻辑：

```go
func (e *engineImpl) handleToolCalls(
    chatCtx iface.ChatContextInterface,
    toolMgr tool.ToolManager,
    result *StreamResult,
) (shouldContinue bool, err error)
```

- 如果没有 tool_calls → `shouldContinue = false`，结束循环
- 并发执行所有 tool calls（受 `concurrencySem` 信号量限制）
- 每个工具执行前后触发 `BeforeToolExecute` / `AfterToolExecute` / `OnToolError` Hook
- 将 tool 消息持久化到 `contextMgr`
- 返回 `shouldContinue = true`，下一轮迭代继续

### Stage 5: persistMessage

```go
func (e *engineImpl) persistMessage(
    chatCtx iface.ChatContextInterface,
    result *StreamResult,
    isFinal bool,
) error
```

将助手消息持久化到数据库：
- 组装 `PersistedParts`（reasoning + text + tool-call 的 UI 结构）
- 如果是最终消息（不再有工具调用），设置 `msg.ID`
- 调用 `contextMgr.AddMessage()`
- 触发 `OnMessagePersist` Hook

## 消息 ID 与 Step 的关系

一次完整的 Agent Loop 对应**一个** `messageID`，其中包含**多个** Step：

```
messageID: "abc-123"
    ├── Step 0: LLM 第1次调用 → 文本回复 + tool_calls
    │       ├── part 0 (reasoning): "我需要读取文件..."
    │       ├── part 1 (text):      "我来帮你查看..."
    │       ├── part 2 (tool-call):  read_file
    │       └── part 3 (tool-call):  list_dir
    │
    ├── Step 1: LLM 第2次调用（收到 tool 结果）→ 纯文本
    │       ├── part 0 (reasoning): "根据文件内容..."
    │       └── part 1 (text):      "这个文件包含了..."
    │
    └── message_done
```

前端收到的 SSE 事件序列：

```
step_create  { messageId: "abc-123", stepIndex: 0 }   ← 非首轮
part_create  { messageId: "...", stepIndex: 0, partIndex: 0, partType: "reasoning" }
part_update  { messageId: "...", stepIndex: 0, partIndex: 0, textDelta: "我" }
part_update  { messageId: "...", stepIndex: 0, partIndex: 0, textDelta: "需要" }
part_update  { messageId: "...", stepIndex: 0, partIndex: 0, state: "done" }
part_create  { messageId: "...", stepIndex: 0, partIndex: 1, partType: "text" }
part_update  { messageId: "...", stepIndex: 0, partIndex: 1, textDelta: "我来帮..." }
part_update  { messageId: "...", stepIndex: 0, partIndex: 1, state: "done" }
part_create  { messageId: "...", stepIndex: 0, partIndex: 2, partType: "tool-call" }
part_create  { messageId: "...", stepIndex: 0, partIndex: 3, partType: "tool-call" }
// ... tool 执行 ...
part_update  { stepIndex: 0, partIndex: 2, state: "complete", output: "..." }
part_update  { stepIndex: 0, partIndex: 3, state: "complete", output: "..." }

step_create  { messageId: "abc-123", stepIndex: 1 }   ← 第二轮
// ... part_create / part_update for step 1 ...

message_done { messageId: "abc-123" }
```

---

下一篇：[03-hook-system.md](./03-hook-system.md)