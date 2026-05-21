# 人在回路 (HITL) — 架构设计方案

> 版本: v1.0 | 日期: 2026-05-21

## 目录

1. [概述](#1-概述)
2. [核心架构决策](#2-核心架构决策)
3. [交互模型](#3-交互模型)
4. [ChatContext 生命周期改造](#4-chatcontext-生命周期改造)
5. [HITL 内存阻塞机制](#5-hitl-内存阻塞机制)
6. [SSE + POST 交互流程](#6-sse--post-交互流程)
7. [SSE 连接退出机制](#7-sse-连接退出机制)
8. [Stop 能力重建](#8-stop-能力重建)
9. [工具侧设计](#9-工具侧设计)
10. [Engine 侧影响](#10-engine-侧影响)
11. [API 扩展](#11-api-扩展)
12. [前端改造](#12-前端改造)
13. [完整组件关系](#13-完整组件关系)
14. [实施阶段](#14-实施阶段)
15. [关键约束与边界](#15-关键约束与边界)

---

## 1. 概述

为 CopCon 增加「人在回路」(Human-in-the-Loop, HITL) 能力。部分工具在执行过程中可以暂停并请求人类输入，人类响应后工具继续执行。

### MVP 范围

两种交互类型：

| 交互类型 | 场景 | 人类返回 | LLM 看到 |
|---------|------|---------|---------|
| **工具审批** | 危险操作需确认 | approve / decline | 工具执行结果或拒绝错误 |
| **问答** | 工具需要人类补充信息 | 结构化输入 | 工具执行结果 |

### 不在 MVP 范围

- OAuth / URL Mode 鉴权流
- MCP Sampling（LLM-in-the-Loop）
- 持久化状态存储（Postgres/Redis snapshot）
- WebSocket 双向通道

### 设计原则

- **内存阻塞** — 工具通过 channel 阻塞等待人类输入，不做持久化序列化/反序列化
- **工具自治** — 工具通过 `chatCtx.RequestInput()` 自行决定何时、如何请求人类输入
- **LLM 无感知** — LLM 不知道工具暂停过，只看到正常的 ToolResult 或错误
- **SSE + POST 混合** — SSE 推送交互请求，POST 提交人类响应

---

## 2. 核心架构决策

### 决策 1：内存阻塞 vs 状态序列化

| | 内存阻塞 | 状态序列化 |
|---|---------|-----------|
| **机制** | 工具阻塞在 channel，agent goroutine 存活 | Agent 序列化状态到 DB，进程退出 |
| **恢复** | 往 channel 写入数据 | 从 DB 加载状态，重建 goroutine |
| **实现复杂度** | 极低（~50 行核心代码） | 高（序列化/反序列化/状态机） |
| **风险** | 进程重启丢失等待中的交互 | 无 |
| **与现有架构一致性** | 与 async tool 的 goroutine 模型一致 | 需要全新基础设施 |

**选择：内存阻塞。** 理由：

1. 系统已经是全内存模型（`SessionAgentStore`、`AsyncToolRegistry`、ringbuf 全在内存）
2. 工具阻塞在 channel 上和工具阻塞在 LLM API 调用上没有本质区别
3. MVP 阶段不需要跨进程持久化

### 决策 2：chatCtx.RequestInput vs ToolResult.Interrupt

| | chatCtx.RequestInput | ToolResult.Interrupt |
|---|---------|-----------|
| **调用方式** | 工具调 `chatCtx.RequestInput()`，阻塞直到人类响应 | 工具返回 `ToolResult{Interrupt: ...}`，Engine 做中断/恢复 |
| **工具侧心智模型** | 同步：调用返回就是有人类输入了 | 异步：需要区分首次执行和恢复执行 |
| **Engine 侧改动** | 无 | 需要改 handleToolCalls 识别中断信号 |
| **恢复机制** | 直接写 channel，工具自动继续 | 需要重新执行工具 + 传递 ResumeInput |

**选择：chatCtx.RequestInput。** 理由：

1. 工具侧零心智负担——调用返回就是有输入了，像读普通函数返回值
2. Engine 侧完全不改——`toolMgr.Execute` 内部阻塞对 Engine 透明
3. 不需要在 `ChatContextInterface` 上加 `ResumeInput()` 来区分首次/恢复执行

### 决策 3：ChatContext Lifecycle Context

当前 `NewChatContext` 使用 `c.Request.Context()`，agent 生命周期绑定在 HTTP 请求上。SSE 断开 → agent 被杀死。

**决策：将 ChatContext 的 context 与 HTTP 请求解耦，使用独立的 lifecycle context，只被 `chatCtx.Close()` cancel。**

理由：

1. HITL 场景下 agent 等待人类输入时 SSE 可能断开，agent 不应死亡
2. 修了现有 reconnection 的 bug——SSE 断开后 agent 马上被杀死，重连窗口极窄
3. 影响面极小——所有 `chatCtx.Context()` 的调用者语义上都应该跟 agent 生命周期走

---

## 3. 交互模型

### 3.1 两种交互类型

**审批型（Approval）**

```
Agent → 工具执行 → 发现需要人类确认
  → chatCtx.RequestInput({type: "approval", message: "确认删除？", summary: "删除文件: /tmp/xxx"})
  → 阻塞等待
  → 人类点 Approve → POST /resume → channel 写入响应
  → RequestInput 返回 {action: "approve"}
  → 工具继续执行，返回 ToolResult
```

**问答型（Question）**

```
Agent → 工具执行 → 需要人类补充信息
  → chatCtx.RequestInput({type: "question", message: "请选择", inputSchema: {...}})
  → 阻塞等待
  → 人类填写表单 → POST /resume → channel 写入响应
  → RequestInput 返回 {action: "submit", content: {selected_index: 2}}
  → 工具用人类输入继续执行，返回 ToolResult
```

### 3.2 LLM 隔离不变量

**LLM 永远不知道工具暂停过。** 对 LLM 而言，调用一个需要 HITL 的工具和调用一个耗时较长的工具没有区别——最终都返回正常的 ToolResult。

拒绝审批 = 工具返回 `ToolResult{Error: "user declined"}`，LLM 据此重新规划。

### 3.3 全有或全无原则

同步批次中的多个工具调用，如果其中一个需要 HITL 并阻塞，整个批次等待。这符合当前 `executeSync` 的顺序执行语义——一个工具阻塞，后续工具也等待。

---

## 4. ChatContext 生命周期改造

### 4.1 当前问题

```
HTTP request context = ChatContext.ctx = agent 所有操作的 context 根

退出链路（唯一的）：
客户端断开 → c.Request.Context() cancel → chatCtx.Context() cancel
  → provider.Stream() 被 cancel → agent goroutine 退出
  → defer chatCtx.Close() → ringbuf.Close() → SSE 循环退出
```

任何一环断了，整条链全断。reconnection 设计的「SSE 断开但 agent 还在跑」实际上极难发生。

### 4.2 改造方案

```go
// 改造前
func NewChatContext(ctx context.Context, sessionID, agentID string) *ChatContext {
    return &ChatContext{
        ctx:       ctx,                    // c.Request.Context()
        sessionID: sessionID,
        agentID:   agentID,
        rb:        ringbuf.New[entity.Event](1024),
        closed:    make(chan struct{}),
    }
}

// 改造后
func NewChatContext(_ context.Context, sessionID, agentID string) *ChatContext {
    return &ChatContext{
        ctx:       context.Background(),   // 独立 lifecycle context
        sessionID: sessionID,
        agentID:   agentID,
        rb:        ringbuf.New[entity.Event](1024),
        closed:    make(chan struct{}),
        lifecycleCancel: func() {},        // 初始为 no-op
    }
}
```

增加 lifecycle context 的 cancel 能力，只被 `Close()` 调用：

```go
type ChatContext struct {
    ctx             context.Context
    lifecycleCancel context.CancelFunc
    // ... 其余字段不变
}

func (c *ChatContext) Close() {
    c.lifecycleCancel()   // 取消 lifecycle context
    c.rb.Close()
    close(c.closed)
    if c.store != nil {
        c.store.Remove(c.sessionID)
    }
}
```

### 4.3 影响面分析

| 调用点 | 影响 | 是否需要改 |
|--------|------|-----------|
| `provider.Stream(chatCtx.Context(), ...)` | 客户端断开不再取消 LLM 调用 | 不改，LLM 调用本就不该因 SSE 断开而取消 |
| `e.contextMgr.BuildContext(chatCtx, ...)` | DB 查询不再随客户端断开取消 | 不改，DB 查询应该完成 |
| `e.contextMgr.AddMessage(chatCtx, ...)` | 同上 | 不改 |
| `e.sessionMgr.Get(chatCtx)` | 同上 | 不改 |
| `context.WithTimeout(chatCtx.Context(), ...)` (async tools) | 不再随客户端断开取消 | 不改，async 工具有自己的超时 |
| SSE 循环 | 需要新增退出条件 | **改**，见第 7 节 |
| 现有测试 | 使用 `context.Background()` | 不改 |

**唯一修改已有逻辑的地方是 SSE 循环的退出条件。**

---

## 5. HITL 内存阻塞机制

### 5.1 ChatContextInterface 扩展

```go
type ChatContextInterface interface {
    Context() context.Context
    SessionID() string
    AgentID() string
    Events() <-chan entity.Event
    Emit(event entity.Event)
    Close()
    Closed() <-chan struct{}
    Depth() int
    Subscribe(fromSeq int64) (*Subscriber, bool)

    // HITL: 请求人类输入并阻塞等待
    RequestInput(req InputRequest) (*InputResponse, error)

    // HITL: 由 resume handler 调用，将人类响应写入对应 channel
    ResolveInput(interruptID string, resp *InputResponse) error

    // HITL: 返回当前等待人类响应的中断列表
    PendingInputs() []InputRequest
}
```

### 5.2 HITL 类型定义

```go
// domain/iface/chat.go 新增

type InterruptType string

const (
    InterruptApproval InterruptType = "approval"
    InterruptQuestion InterruptType = "question"
)

type InputRequest struct {
    ID          string         `json:"id"`
    Type        InterruptType  `json:"type"`
    Message     string         `json:"message"`
    InputSchema map[string]any `json:"input_schema,omitempty"`
    Summary     string         `json:"summary,omitempty"`
    ToolName    string         `json:"tool_name"`
    ToolArgs    map[string]any `json:"tool_args,omitempty"`
}

type InputResponse struct {
    Action  string         `json:"action"`  // "approve" / "decline" / "submit" / "cancel"
    Content map[string]any `json:"content,omitempty"`
}
```

### 5.3 ChatContext 实现

```go
// ChatContext 新增字段
type ChatContext struct {
    // ... 现有字段 ...

    // HITL
    interruptMu    sync.Mutex
    interruptChans map[string]chan *InputResponse   // interruptID → response channel
    interruptReqs  map[string]*InputRequest          // interruptID → request (for PendingInputs)
}

// RequestInput 请求人类输入并阻塞等待
func (c *ChatContext) RequestInput(req InputRequest) (*InputResponse, error) {
    id := uuid.New().String()
    req.ID = id

    ch := make(chan *InputResponse, 1)

    c.interruptMu.Lock()
    c.interruptChans[id] = ch
    c.interruptReqs[id] = &req
    c.interruptMu.Unlock()

    // 确保 cleanup
    defer func() {
        c.interruptMu.Lock()
        delete(c.interruptChans, id)
        delete(c.interruptReqs, id)
        c.interruptMu.Unlock()
    }()

    // Emit 等待事件 — 通过 part_update 通知前端
    // (Engine 在 executeSync 中已经 emit 了 part_update(state: "running"))
    // 这里 emit 一个额外的 part_update 将 state 推进到 "waiting_for_input"
    // 注意：Emit 的调用由 Engine 侧完成，或由 RequestInput 内部完成
    // 详见第 6 节

    select {
    case resp := <-ch:
        return resp, nil
    case <-c.Closed():
        return nil, fmt.Errorf("session closed while waiting for input")
    }
}

// ResolveInput 由 resume handler 调用
func (c *ChatContext) ResolveInput(interruptID string, resp *InputResponse) error {
    c.interruptMu.Lock()
    ch, ok := c.interruptChans[interruptID]
    c.interruptMu.Unlock()

    if !ok {
        return ErrInterruptNotFound
    }

    ch <- resp
    return nil
}

// PendingInputs 返回待处理的中断列表
func (c *ChatContext) PendingInputs() []InputRequest {
    c.interruptMu.Lock()
    defer c.interruptMu.Unlock()

    result := make([]InputRequest, 0, len(c.interruptReqs))
    for _, req := range c.interruptReqs {
        result = append(result, *req)
    }
    return result
}
```

### 5.4 超时机制

`RequestInput` 的 select 同时监听 session 关闭信号。超时由外层控制（Engine 或工具自行 `context.WithTimeout`），不在 `RequestInput` 内部硬编码。

---

## 6. SSE + POST 交互流程

### 6.1 暂停阶段

```
Agent Loop 运行中
    │
    ↓
工具调用 chatCtx.RequestInput(req)
    │
    ├── 1. 生成 interrupt_id，注册 chan
    ├── 2. Emit part_update(state: "waiting_for_input", interrupt: {...})
    │       → SSE 推送到前端
    └── 3. <-chan 阻塞，等待人类响应
```

前端收到 `part_update` + `state: "waiting_for_input"` 后，根据 `interrupt.interruptType` 渲染审批或问答 UI。

**SSE 流此时仍然活着**，只是没有新事件推送。agent goroutine 阻塞在 channel 上，ringbuf 和 ChatContext 都正常存活。

### 6.2 恢复阶段

```
POST /api/sessions/:sessionId/resume
Body: { interrupt_id: "xxx", action: "approve", content: {...} }
    │
    ↓
Handler:
    ├── 1. sessionAgentStore.Get(sessionID) → chatCtx
    ├── 2. chatCtx.ResolveInput(interruptID, response)
    │       → channel 写入响应
    └── 3. 返回 200

同时（在 agent goroutine 中）：
    ←chan 收到响应，RequestInput 返回
    → 工具继续执行
    → Emit 后续事件（part_update: running → complete）
    → SSE 流自动推送
```

**不需要新建 SSE 连接。** 现有的 SSE 连接就能收到后续事件。

### 6.3 断线重连

如果用户在 HITL 等待期间刷新页面：

1. 前端调用 `getMessages` 获取历史消息
2. 消息中包含 `state: "waiting_for_input"` 的 ToolCallPart
3. 渲染时识别此状态，显示交互 UI
4. 用户操作后 POST /resume
5. 同时发起新的 SSE chat 请求（空 content，纯监听后续事件）

后续事件通过新 SSE 流推送。

---

## 7. SSE 连接退出机制

### 7.1 当前退出条件

SSE 循环唯一的退出条件是 `sub.Events` channel 关闭，即 ringbuf 关闭，即 agent 结束。

### 7.2 改造后

SSE 循环需要同时监听 HTTP request context 的取消，用于清理断开连接的 subscriber goroutine：

```go
// 改造前
for event := range sub.Events {
    data, _ := json.Marshal(event)
    fmt.Fprintf(c.Writer, "data: %s\n\n", data)
    flusher.Flush()
}

// 改造后
for {
    select {
    case event, ok := <-sub.Events:
        if !ok {
            return  // ringbuf 关闭 = agent 结束
        }
        data, _ := json.Marshal(event)
        _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data)
        if err != nil {
            return  // 写失败（客户端已断开）
        }
        flusher.Flush()
    case <-c.Request.Context().Done():
        return  // 客户端断开 → 只退出 SSE 推送，agent 继续跑
    }
}
```

**监听 HTTP context 不是为了控制 agent 生命周期，而是为了及时清理断开的 SSE 连接。** 防止 subscriber goroutine 在 channel 缓冲区满后永久阻塞。

| 场景 | SSE 循环 | Agent goroutine | Ringbuf |
|------|---------|----------------|---------|
| Agent 正常结束 | `sub.Events` 关闭 → 退出 | 正常返回 | `Close()` |
| 客户端意外断开 | HTTP context cancel → 退出 | **继续跑** | **不关** |
| 用户主动 Stop | HTTP context cancel → 退出 | `chatCtx.Close()` → 退出 | `Close()` |

---

## 8. Stop 能力重建

### 8.1 当前 Stop 路径

```
前端 abort 按钮 → AbortController.abort()
  → fetch 请求中断 → c.Request.Context() cancel
  → chatCtx.Context() cancel → agent 死
```

### 8.2 Lifecycle Context 下的问题

HTTP request context 和 agent lifecycle context 解绑后，abort fetch 只能断开 SSE 连接，不能杀死 agent。

### 8.3 解决方案

新增 `POST /api/sessions/:sessionId/stop` 端点，handler 调用 `chatCtx.Close()` 终止 lifecycle context。

前端 Stop 按钮行为改为：先调 stop endpoint，再 abort fetch。

```go
func (h *Handler) StopSession(c *gin.Context) {
    sessionID := c.Param("sessionId")
    chatCtx, found := h.sessionAgentStore.Get(sessionID)
    if !found {
        c.JSON(http.StatusNotFound, gin.H{"error": "no active agent for this session"})
        return
    }
    chatCtx.Close()
    c.Status(http.StatusNoContent)
}
```

---

## 9. 工具侧设计

### 9.1 审批型工具

```go
func (t *DeleteFileTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    path := args["path"].(string)

    // 请求审批，阻塞直到人类响应
    resp, err := chatCtx.RequestInput(iface.InputRequest{
        Type:     iface.InterruptApproval,
        Message:  "确认删除文件？此操作不可恢复",
        Summary:  fmt.Sprintf("删除文件: %s", path),
        ToolName: t.Name(),
        ToolArgs: args,
    })
    if err != nil {
        return nil, fmt.Errorf("approval failed: %w", err)
    }

    if resp.Action != "approve" {
        return &tool.ToolResult{Success: false, Error: "user declined"}, nil
    }

    // 执行实际删除
    os.Remove(path)
    return &tool.ToolResult{Success: true, Data: path}, nil
}
```

### 9.2 问答型工具

```go
func (t *SearchTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    results := search(args["query"].(string))

    if len(results) <= 5 {
        return &tool.ToolResult{Success: true, Data: results}, nil
    }

    // 结果太多，请求人类选择
    resp, err := chatCtx.RequestInput(iface.InputRequest{
        Type:        iface.InterruptQuestion,
        Message:     "搜索结果过多，请选择最相关的一个",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "selected_index": map[string]any{"type": "number"},
            },
            "required": []string{"selected_index"},
        },
        ToolName: t.Name(),
    })
    if err != nil {
        return nil, fmt.Errorf("input request failed: %w", err)
    }

    idx := int(resp.Content["selected_index"].(float64))
    return &tool.ToolResult{Success: true, Data: results[idx]}, nil
}
```

### 9.3 不需要 HITL 的工具

不需要任何改动。`chatCtx.RequestInput()` 只在工具主动调用时才触发，现有工具完全不受影响。

---

## 10. Engine 侧影响

### 10.1 不需要改的部分

| 组件 | 原因 |
|------|------|
| `Tool` 接口 | 签名不变 |
| `ToolResult` | 不加 Interrupt 字段 |
| `handleToolCalls` | `toolMgr.Execute` 内部阻塞对 Engine 透明 |
| `executeSync` | 工具阻塞 = 执行时间较长，Engine 正常等待 |
| `executeConcurrent` | 一个工具阻塞，该 goroutine 等待，不影响其他并发工具 |
| `executeAsync` | 不受影响 |

### 10.2 需要改的部分

`executeSync` 中 `toolMgr.Execute` 返回后，需要 emit `part_update(state: "waiting_for_input")` 的时机在 `RequestInput` 内部处理——因为阻塞发生在工具内部，Engine 无法在阻塞前后插入事件。

方案：`RequestInput` 内部 emit 事件。但当前 `Emit` 不支持传 `PartUpdateData` 的细节（stepIndex, partIndex 等）。需要 `RequestInput` 接受额外的定位参数，或由 Engine 在调用 `toolMgr.Execute` 前传入 emit 回调。

**推荐 MVP 方案：在 `executeSync` 中，调用 `toolMgr.Execute` 前将 emit 函数注入 ChatContext，`RequestInput` 内部调用。**

```go
// executeSync 中（伪代码，展示意图）
chatCtx.SetPartLocator(messageID, stepIndex, partIndex)  // 告知 RequestInput 当前 part 定位信息
result, err := toolMgr.Execute(chatCtx, tc.Name, args)
chatCtx.ClearPartLocator()
```

`RequestInput` 内部使用这个定位信息 emit 正确的 `part_update`。

### 10.3 并发工具调用的 HITL 处理

`executeConcurrent` 中如果一个工具调了 `RequestInput` 阻塞，该工具的 goroutine 等待，其他工具正常执行。WaitGroup 最终会等待所有 goroutine 完成。这符合「全有或全无」的语义——一个工具在等人类，整个批次等。

如果后续需要更细粒度的控制（如允许其他工具先返回结果），可以将需要 HITL 的工具从并发批次中拆出，单独同步执行。

---

## 11. API 扩展

### 11.1 Resume 端点

```
POST /api/sessions/:sessionId/resume
Content-Type: application/json

{
  "interrupt_id": "550e8400-e29b-41d4-a716-446655440000",
  "action": "approve",           // "approve" | "decline" | "submit" | "cancel"
  "content": {}                  // 问答型的人类输入，审批型可省略
}

→ 200 OK (空响应，后续事件通过 SSE 推送)
→ 404 Not Found (interrupt_id 不存在或已超时)
→ 409 Conflict (session 没有活跃的 agent)
```

### 11.2 Stop 端点

```
POST /api/sessions/:sessionId/stop

→ 204 No Content
→ 404 Not Found (session 没有活跃的 agent)
```

### 11.3 路由注册

```go
sessions.POST("/:sessionId/resume", handler.ResumeSession)
sessions.POST("/:sessionId/stop", handler.StopSession)
```

---

## 12. 前端改造

### 12.1 类型扩展

```ts
// types.ts

// 新增
export interface InterruptPayload {
  interruptId: string;
  interruptType: 'approval' | 'question';
  message: string;
  summary?: string;
  inputSchema?: Record<string, unknown>;
}

// ToolCallPart 扩展
export interface ToolCallPart {
  type: 'tool-call';
  toolCallId: string;
  toolName: string;
  args: string;
  output: string;
  error: string;
  state: 'pending' | 'running' | 'complete' | 'error' | 'waiting_for_input';  // 新增 state
  interrupt?: InterruptPayload;  // 新增
}
```

### 12.2 Provider 扩展

`CopConChatProvider.transformMessage` 的 `part_update` 分支中，`ToolCallPart` state 映射增加 `waiting_for_input`，并透传 `interrupt` payload。

改动约 5 行。

### 12.3 StepContent 渲染

在 `StepContent` 组件中，识别 `waiting_for_input` 状态的 `tool-call` part，渲染交互组件：

```tsx
case 'tool-call':
  if (part.state === 'waiting_for_input' && part.interrupt) {
    return (
      <HumanInteraction
        key={index}
        sessionId={sessionId}
        interrupt={part.interrupt}
      />
    );
  }
  toolCallParts.push(part);
  return null;
```

### 12.4 HumanInteraction 组件

```
┌──────────────────────────────────────────────┐
│  🔧 delete_file                              │
│  ────────────────────                        │
│                                              │
│  【审批型】                                  │
│  确认删除文件？此操作不可恢复                │
│  操作摘要: 删除文件: /tmp/important.log       │
│                                              │
│  [Approve]  [Decline]                        │
│                                              │
│  ────── 或 ──────                            │
│                                              │
│  【问答型】                                  │
│  搜索结果过多，请选择最相关的一个            │
│                                              │
│  selected_index: [  2  ▼]                    │
│                                              │
│  [Submit]  [Cancel]                          │
└──────────────────────────────────────────────┘
```

组件逻辑：

1. 根据 `interrupt.interruptType` 渲染审批按钮或表单
2. 用户点击后，调 `POST /api/sessions/:sessionId/resume`
3. 成功后无需手动更新本地 state——SSE 流会自动推送后续 `part_update` 事件，Provider 自动更新消息

### 12.5 AgentClient 扩展

```ts
async resume(
  sessionId: string,
  interruptId: string,
  action: 'approve' | 'decline' | 'submit' | 'cancel',
  content?: Record<string, unknown>,
): Promise<void> {
  const response = await fetch(
    `${this.baseUrl}/api/sessions/${sessionId}/resume`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ interrupt_id: interruptId, action, content }),
    },
  );
  if (!response.ok) {
    throw new Error(`Failed to resume: ${response.statusText}`);
  }
}

async stop(sessionId: string): Promise<void> {
  const response = await fetch(
    `${this.baseUrl}/api/sessions/${sessionId}/stop`,
    { method: 'POST' },
  );
  if (!response.ok) {
    throw new Error(`Failed to stop: ${response.statusText}`);
  }
}
```

### 12.6 前端改动清单

| 组件 | 改动 | 量 |
|------|------|---|
| `types.ts` | ToolCallPart 加 state + interrupt 字段 | ~10 行 |
| `CopConChatProvider.ts` | part_update 识别 `waiting_for_input` + 透传 interrupt | ~5 行 |
| `StepContent` (demo/App.tsx) | 识别 `waiting_for_input` 状态，渲染交互组件 | ~20 行 |
| `HumanInteraction` 组件 | 审批/问答两种 UI + 调 resume API | ~80 行 |
| `AgentClient` | 新增 `resume` + `stop` 方法 | ~30 行 |
| 前端 Stop 按钮 | 改为先调 stop endpoint，再 abort fetch | ~5 行 |

---

## 13. 完整组件关系

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Agent goroutine                              │
│                                                                      │
│  runAgentLoop() {                                                   │
│    for {                                                            │
│      handleStreaming()                                              │
│        → provider.Stream(lifecycleCtx)                             │
│        → Emit(part_create, part_update)  ──────────┐                │
│      handleToolCalls()                                              │
│        → executeSync()                                              │
│          → toolMgr.Execute(chatCtx, name, args)                    │
│            → tool.Execute()                                         │
│              → chatCtx.RequestInput(req)                            │
│                ├── 注册 chan                                        │
│                ├── Emit(part_update: waiting_for_input)  ──────────┐│
│                └── <-chan 阻塞  ←────────────────── chan ←──┐     ││
│              → (阻塞中，等待人类响应)                         │     ││
│          → (Engine 正常等待，对 Engine 透明)                  │     ││
│    }                                                          │     ││
│    chatCtx.Close()  ←── Stop endpoint 调用                    │     ││
│  }                                                             │     ││
│                                                                │     ││
│                                          ┌──────────┐         │     ││
│                                          │ ringbuf  │ ← Emit  │     ││
│                                          └────┬─────┘         │     ││
│                                               │                │     ││
└───────────────────────────────────────────────┼────────────────┼─────┘
                                                │                │
                    ┌───────────────────────────┼──────────────┐ │
                    │        SSE handler         │              │ │
                    │                            │              │ │
                    │  for {                     │              │ │
                    │    select {                │              │ │
                    │    case event := <-sub:    │              │ │
                    │      write SSE ────────────────── 前端     │ │
                    │    case <-httpCtx.Done():  │              │ │
                    │      return  (清理 subscriber)            │ │
                    │    }                        │              │ │
                    │  }                          │              │ │
                    └────────────────────────────┼──────────────┘ │
                                                 │                │
                    ┌────────────────────────────┼──────────────┐ │
                    │      Resume handler         │              │ │
                    │                             │              │ │
                    │  POST /sessions/:id/resume  │              │ │
                    │    → chatCtx.ResolveInput()─┼──────────────┘ │
                    │    → 200 OK                 │                │
                    └─────────────────────────────┼────────────────┘
                                                  │
                    ┌─────────────────────────────┼───────────────┐
                    │       Stop handler           │               │
                    │                              │               │
                    │  POST /sessions/:id/stop     │               │
                    │    → chatCtx.Close() ────────────────────────┘
                    │    → 204 No Content          │
                    └──────────────────────────────┘
```

---

## 14. 实施阶段

### Phase 1: Lifecycle Context 改造（前置依赖）

- [ ] ChatContext 使用独立 lifecycle context
- [ ] SSE 循环增加 HTTP context cancel 监听
- [ ] 新增 `POST /sessions/:id/stop` 端点
- [ ] 前端 Stop 按钮改造
- [ ] 验证 reconnection 在 SSE 断开后正常工作

### Phase 2: HITL 核心机制

- [ ] `InputRequest` / `InputResponse` / `InterruptType` 类型定义
- [ ] ChatContext 实现 `RequestInput` / `ResolveInput` / `PendingInputs`
- [ ] Part locator 机制（让 RequestInput 能 emit 正确的 part_update）
- [ ] `part_update` 事件扩展：`state: "waiting_for_input"` + interrupt payload
- [ ] `POST /sessions/:id/resume` 端点

### Phase 3: 前端 HITL 支持

- [ ] types.ts 扩展 ToolCallPart state + InterruptPayload
- [ ] CopConChatProvider 识别 `waiting_for_input` + 透传 interrupt
- [ ] HumanInteraction 组件（审批 + 问答两种模式）
- [ ] AgentClient 新增 `resume` 方法
- [ ] 刷新后从历史消息恢复 HITL UI

### Phase 4: 示例工具

- [ ] 实现一个审批型示例工具（如 delete_file）
- [ ] 实现一个问答型示例工具（如 search_with_selection）

---

## 15. 关键约束与边界

### 内存模型的局限

- 进程重启会丢失所有等待中的 HITL 交互。这是可接受的 MVP 权衡。
- 后续可通过持久化 snapshot（Postgres/Redis）升级为跨进程方案。

### 并发工具调用的 HITL

- 当前 `executeConcurrent` 中，一个工具阻塞在 `RequestInput` 上，该 goroutine 等待，其他工具正常完成。
- WaitGroup 会等待所有 goroutine，所以最终结果是一致的。
- 如果需要更细粒度的控制，后续可将需要 HITL 的工具拆出并发批次。

### 单客户端假设

- 当前设计假设同一 session 只有一个客户端。如果有多个客户端同时调 `resume`，channel 只会被第一个写入，后续调用返回 `ErrInterruptNotFound`。
- 多客户端场景需要额外的仲裁机制，不在 MVP 范围。

### ToolResult 的 LLM 上下文

- 审批通过：工具正常执行，返回正常 ToolResult → LLM 看到执行结果
- 审批拒绝：工具返回 `ToolResult{Success: false, Error: "user declined"}` → LLM 看到错误，据此重新规划
- 问答提交：工具用人类输入继续执行，返回正常 ToolResult → LLM 看到执行结果
- 取消/超时：工具返回错误 → LLM 看到错误

**LLM 上下文中永远不会出现中断请求本身、interrupt_id、或人类的原始输入凭证。**
