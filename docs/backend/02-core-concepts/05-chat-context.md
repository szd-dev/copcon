# ChatContext

## 概述

`ChatContext` 是贯穿整个请求生命周期的统一上下文对象。定义在 `internal/domain/iface/chat.go`（接口）和 `internal/chat_context/`（具体实现）。它同时承担三个角色：

1. **身份载体** — 携带 `SessionID` 和 `AgentID`
2. **上下文传播** — 嵌入 `context.Context` 用于超时和取消
3. **事件总线** — 提供 `Events()` channel 和 `Emit()` 方法，实现引擎到 HTTP 层的流式通信

## 接口定义

```go
type ChatContextInterface interface {
    Context() context.Context
    SessionID() string
    AgentID() string
    Events() <-chan entity.Event
    Emit(event entity.Event)
}
```

| 方法 | 说明 |
|------|------|
| `Context()` | 返回标准 `context.Context`，用于超时控制、取消传播、和数据库操作 |
| `SessionID()` | 当前会话 ID |
| `AgentID()` | 当前 Agent ID，可能为空（此时使用会话默认或全局默认） |
| `Events()` | 返回只读事件 channel，HTTP Handler 通过它读取 SSE 事件 |
| `Emit(event)` | 发送事件到 channel。非阻塞（channel 满时记录警告后阻塞等待） |

## 构造

```go
func NewChatContext(ctx context.Context, sessionID, agentID string) *ChatContext {
    return &ChatContext{
        ctx:       ctx,
        sessionID: sessionID,
        agentID:   agentID,
        events:    make(chan entity.Event, 256),  // 256 缓冲
    }
}
```

- channel 缓冲区为 256 个事件
- `AgentID` 可以为空字符串，由引擎用回退逻辑解析

## Emit 实现细节

```go
func (c *ChatContext) Emit(event entity.Event) {
    select {
    case c.events <- event:
        // 发送成功
    default:
        // channel 接近满载：记录警告，然后阻塞等待
        slog.Warn("SSE event channel near capacity, blocking emit",
            "event_type", event.Type,
        )
        select {
        case c.events <- event:
        case <-c.ctx.Done():
            // context 已取消，丢弃事件
        }
    }
}
```

两阶段发送：
1. 先非阻塞尝试（channel 有空间时直接发送）
2. 满了则记录警告后阻塞等待，直到有空位或 context 取消

## Event 类型

所有事件定义在 `internal/domain/entity/event.go`。分为旧版（已废弃）和新版（Step/Part 级别）：

### 新版事件（当前使用）

```go
type EventType string

const (
    EventStepCreate  EventType = "step_create"   // 创建新的 Step
    EventPartCreate  EventType = "part_create"   // 创建新的 UI Part
    EventPartUpdate  EventType = "part_update"   // 更新已有 Part 的状态或内容
    EventMessageDone EventType = "message_done"  // 标记消息流结束
    EventError       EventType = "error"         // 错误事件
)
```

### StepCreateData

```go
type StepCreateData struct {
    MessageID string `json:"messageId"`
    StepIndex int    `json:"stepIndex"`
}
```

每轮 Agent Loop 迭代（第 2 轮及之后）会触发。第一轮不触发以节省事件量。

### PartCreateData

```go
type PartCreateData struct {
    MessageID  string `json:"messageId"`
    StepIndex  int    `json:"stepIndex"`
    PartIndex  int    `json:"partIndex"`
    PartType   string `json:"partType"`   // "text", "reasoning", "tool-call"
    State      string `json:"state,omitempty"` // "streaming", "pending"
    ToolCallID string `json:"toolCallId,omitempty"`
    ToolName   string `json:"toolName,omitempty"`
    Args       string `json:"args,omitempty"`
}
```

三种 PartType：
- `"text"` — 普通文本内容
- `"reasoning"` — 模型内部推理（思维链）
- `"tool-call"` — 工具调用

### PartUpdateData

```go
type PartUpdateData struct {
    MessageID string `json:"messageId"`
    StepIndex int    `json:"stepIndex"`
    PartIndex int    `json:"partIndex"`
    PartType  string `json:"partType"`
    TextDelta string `json:"textDelta,omitempty"` // 增量文本
    State     string `json:"state,omitempty"`     // "streaming", "done", "complete", "error"
    Output    string `json:"output,omitempty"`    // 工具输出
    Error     string `json:"error,omitempty"`     // 工具错误
}
```

`TextDelta` 和 `State`/`Output`/`Error` 是互斥的：
- 流式增量用 `TextDelta`
- 状态转换用 `State` + 可选的 `Output` 或 `Error`

## 完整事件流

一次典型的 Agent 交互的完整事件序列：

```
请求: POST /api/sessions/s-123/chat

Handler 层:
  1. NewChatContext(ctx, "s-123", "my-agent")
  2. go engine.Chat(chatCtx, "帮我查看文件")
  3. for event := range chatCtx.Events() { write SSE }

引擎内部（多轮迭代）:

┌─ Step 0 ───────────────────────────────────────────────────┐
│                                                              │
│  part_create  { partIndex:0, partType:"reasoning",          │
│                 state:"streaming" }                          │
│  part_update  { partIndex:0, textDelta:"我需要" }            │
│  part_update  { partIndex:0, textDelta:"查看..." }           │
│  part_update  { partIndex:0, state:"done" }                 │
│                                                              │
│  part_create  { partIndex:1, partType:"text",               │
│                 state:"streaming" }                          │
│  part_update  { partIndex:1, textDelta:"我来..." }           │
│  part_update  { partIndex:1, textDelta:"帮你..." }           │
│  part_update  { partIndex:1, state:"done" }                 │
│                                                              │
│  part_create  { partIndex:2, partType:"tool-call",          │
│                 toolName:"read_file", state:"pending" }      │
│  // 工具执行中...                                            │
│  part_update  { partIndex:2, state:"complete",              │
│                 output:"文件内容..." }                        │
│                                                              │
└──────────────────────────────────────────────────────────────┘

┌─ Step 1 ───────────────────────────────────────────────────┐
│                                                              │
│  step_create  { stepIndex:1 }                               │
│                                                              │
│  part_create  { stepIndex:1, partIndex:0,                   │
│                 partType:"reasoning", state:"streaming" }    │
│  part_update  { stepIndex:1, partIndex:0,                   │
│                 textDelta:"根据文件..." }                    │
│  part_update  { stepIndex:1, partIndex:0, state:"done" }   │
│                                                              │
│  part_create  { stepIndex:1, partIndex:1,                   │
│                 partType:"text", state:"streaming" }         │
│  part_update  { stepIndex:1, partIndex:1,                   │
│                 textDelta:"这个文件..." }                    │
│  part_update  { stepIndex:1, partIndex:1, state:"done" }   │
│                                                              │
└──────────────────────────────────────────────────────────────┘

  message_done { messageId:"msg-456" }

  (channel 关闭 → Handler 退出 SSE 循环 → 请求结束)
```

## ChatContext 在系统中的流转

```
                    ┌──────────────────┐
                    │   HTTP Handler   │
                    │  (创建 ChatCtx)  │
                    └────────┬─────────┘
                             │ ChatContext
                             ▼
              ┌──────────────────────────┐
              │    AgentEngine.Chat()    │
              │                          │
              │  chatCtx.SessionID() ──► 匹配会话
              │  chatCtx.AgentID()  ──► 匹配 Agent
              │  chatCtx.Context()  ──► DB 操作 / LLM 超时
              │  chatCtx.Emit()     ──► 推送事件
              │                          │
              │         │                │
              │         ▼                │
              │   ┌──────────────┐       │
              │   │ HookRunner   │       │
              │   │ (传入 chatCtx)│       │
              │   └──────┬───────┘       │
              │          │               │
              │          ▼               │
              │   ┌──────────────┐       │
              │   │ Hook.Execute │       │
              │   │ (ctx.ChatCtx)│       │
              │   └──────┬───────┘       │
              │          │               │
              │          ▼               │
              │   ┌──────────────┐       │
              │   │ Tool.Execute │       │
              │   │ (chatCtx)    │       │
              │   └──────────────┘       │
              │                          │
              │  chatCtx.Emit() ──► Events channel
              └──────────────────────────┘
                        │
                        ▼
              ┌──────────────────┐
              │   HTTP Handler   │
              │ (消费 events)    │
              │                  │
              │ for event :=     │
              │   range chatCtx. │
              │   Events() {     │
              │   write SSE      │
              │ }                │
              └──────────────────┘
```

## ChatContext 与 HookContext 的区别

这是两个容易混淆的概念：

| 特性 | ChatContext | HookContext |
|------|------------|-------------|
| 定义位置 | `domain/iface/chat.go` | `hook/hook.go` |
| 角色 | 请求级上下文 + 事件总线 | Hook 执行时的快照上下文 |
| 携带数据 | SessionID, AgentID, Context, Events | ChatCtx 引用 + Hook 特定字段 |
| 使用范围 | 整个请求生命周期 | 单个 Hook 的 `Execute()` 调用内 |
| 可变性 | 只读（除 Emit） | 指针字段可写（用于修改管道行为） |
| 创建者 | HTTP Handler | HookRunner.On() |
| 生命周期 | 请求开始到 SSE 流结束 | Hook 执行期间 |

关系：

```go
// HookContext 包含一个 ChatContext
type HookContext struct {
    ChatCtx   iface.ChatContextInterface  // ← 指向请求级的 ChatContext
    SessionID string
    AgentID   string
    // ... hook 特定字段 ...
}
```

Hook 中访问 ChatContext 的典型方式：

```go
func (h *MyHook) Execute(ctx *hook.HookContext) error {
    sessionID := ctx.ChatCtx.SessionID()
    agentID := ctx.ChatCtx.AgentID()

    // 通过 ChatContext 发送自定义事件
    ctx.ChatCtx.Emit(entity.Event{
        Type: entity.EventPartCreate,
        Data: entity.PartCreateData{...},
    })

    // 使用标准 context 做 DB 查询
    db.WithContext(ctx.ChatCtx.Context()).Find(&records)

    return nil
}
```

---

上一篇：[04-llm-provider.md](./04-llm-provider.md)