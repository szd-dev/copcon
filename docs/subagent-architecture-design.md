# Subagent 调用 — 架构设计方案

> 版本: v1.0 | 日期: 2026-05-21

## 目录

1. [概述](#1-概述)
2. [核心架构决策](#2-核心架构决策)
3. [Agent 注册体系](#3-agent-注册体系)
4. [ChatContext 事件分发](#4-chatcontext-事件分发)
5. [SSE 连接机制](#5-sse-连接机制)
6. [断线重连保证](#6-断线重连保证)
7. [Subagent 工具设计](#7-subagent-工具设计)
8. [子 Agent 事件隔离](#8-子-agent-事件隔离)
9. [Incremental Persist](#9-incremental-persist)
10. [前端改造](#10-前端改造)
11. [完整组件关系](#11-完整组件关系)
12. [实施阶段](#12-实施阶段)
13. [关键依赖与约束](#13-关键依赖与约束)

---

## 1. 概述

为 CopCon 增加 subagent 调用能力。主 agent 可以将子任务委托给其他 agent（如 code-reviewer、sre-agent），子 agent 在独立会话中执行，通过 SSE 事件流实时输出，结果返回给主 agent。

### 设计原则

- **Factory 自由，Engine 统一，Hook 做差异** — 每个 agent 通过独立的工厂方法构建，但共享同一套引擎执行逻辑，差异通过 per-agent Hook 表达
- **确保正确性** — 事件分发引入 ringbuf 库，避免自实现并发细节的正确性风险
- **复用现有基础设施** — 不改 Engine 核心循环、不改 Step/Part 事件协议、不改 Tool 接口

---

## 2. 核心架构决策

| 决策 | 选择 | 理由 |
|------|------|------|
| Agent 模型 | Definition + Engine 分离 | Subagent 不需要不同的执行模式（都是 ReAct loop）。Hook 已覆盖所有定制需求。所有 agent 共享同一套经过测试的引擎 |
| 注册方式 | 代码注册（非 config.yaml） | 工厂方法可做任意复杂操作（DB 查询、API 调用、向量检索），远超模板能力 |
| 事件分发 | `github.com/golang-cz/ringbuf` | 锁无关环形缓冲区，内建 Seek 重连，慢读者自动终止。用库的正确性替代自实现的调试成本 |
| Subagent 调用 | 单一 `delegate_to` tool，`agent_id` 作为参数 | 不按 agent 创建多个 tool，LLM 自行选择目标 agent |
| 子 Agent 执行 | 独立子会话 (`parent_session_id`) | 上下文隔离，可审计，子内部推理不污染主 agent 上下文 |
| 子 Agent 事件 | 独立 SSE 流，不转发到主 agent | 主 agent 流保持干净，子 agent 事件按需连接 |
| 嵌套深度 | 硬限制 3 层 | 覆盖主→子→孙场景，防止失控 |
| 上下文传递 | 返回 `{ sub_session_id, summary }` | 主 agent 看摘要做决策，需详情时查询子会话 |

---

## 3. Agent 注册体系

### 3.1 AgentDefinition 结构

```
AgentDefinition {
    ID, Name, Model           // 标识与模型
    SystemPrompt              // 系统提示词
    ToolManager               // 工具集
    LLMProvider               // LLM 客户端
    Hooks []Hook              // ← Agent 级别的行为定制
}
```

Hook 直接挂载在 `AgentDefinition` 上，不再需要额外的包装类型。引擎执行时组合 `引擎全局 hooks + agentDef.Hooks` 作为完整 hook 链。

### 3.2 注册方式

```go
// main.go 或 init 阶段
registry := agent.NewRegistry()

registry.RegisterFactory(
    id:    "code-assistant",
    name:  "Code Assistant",
    model: "z-ai/glm-5",
    factory: func(ctx context.Context, params agent.CreateParams) (agent.AgentDefinition, error) {
        // 工厂内部可做任意操作:
        // - 从 Qdrant 检索相关上下文
        // - 调用外部 API 获取信息
        // - 查询 git blame 获取文件作者
        // - 动态拼接 system prompt
        return agent.AgentDefinition{
            ID:           "code-assistant",
            Name:         "Code Assistant",
            Model:        params.ModelOverride,
            SystemPrompt: assembledPrompt,
            ToolManager:  toolMgr,
            LLMProvider:  llmProvider,
            Hooks: []hook.Hook{
                // 该 agent 特有的行为
                injectContextHook,
                retryOnNetworkErrorHook,
            },
        }, nil
    },
)
```

### 3.3 工厂方法参数

```go
type CreateParams struct {
    Task          string         // 子 agent 的任务描述
    ParentContext string         // 父会话上下文摘要
    ModelOverride string         // 可选模型覆盖
    Extra         map[string]any // 额外灵活参数
}
```

### 3.4 引擎 hook 组合

引擎在执行入口处：

```
runAgentLoop(agentDef, chatCtx, userInput):
    effectiveHooks = compose(engine.globalHooks, agentDef.Hooks)
    // 后续全程使用 effectiveHooks
```

---

## 4. ChatContext 事件分发

### 4.1 引入 ringbuf

当前：`ChatContext` 内部是一个 `chan Event`（单消费者）。

改造后：`ChatContext` 内部是 `ringbuf.RingBuffer[entity.Event]`（多消费者 + 各自独立游标）。

```
Agent goroutine              SSE Handler 1         SSE Handler 2
     │ chatCtx.Emit(event)       │                    │
     ▼                           ▼                    ▼
┌─────────────────────────────────────────────────────────┐
│  ChatContext                                             │
│                                                          │
│  ringbuf.RingBuffer[entity.Event]                        │
│  ┌──────────────────────────────────────────────────┐   │
│  │ seq=41  seq=42  seq=43  seq=44  seq=45  seq=46  │   │
│  └──────────────────────────────────────────────────┘   │
│                                                          │
│  Subscribe(fromSeq) → (sub, ok)                          │
│  ───────────────────────────────                         │
│  sub.SeekAfter(fn) → found    ← 断线重连定位             │
│  sub.Iter()       → <-chan     ← 阻塞读取新事件           │
│  sub.Err()        → error      ← 慢读者被终止             │
│                                                          │
│  Close() → 关闭 ringbuf，所有 subscriber 终止             │
└─────────────────────────────────────────────────────────┘
```

### 4.2 Emit 不变

```go
// agent 侧调用完全不变
chatCtx.Emit(entity.Event{
    Type: entity.EventPartUpdate,
    Data: entity.PartUpdateData{...},
})
```

内部 `Emit` 调用 `ringbuf.Write(event)` 代替 `ch <- event`。

### 4.3 封装接口

```go
func (c *ChatContext) Subscribe(lastSeq int64) (*ringbuf.Subscriber[entity.Event], bool) {
    sub := c.rb.Subscribe(ctx, &ringbuf.SubscribeOpts{
        StartBehind: c.rb.Size(),
    })
    found := sub.SeekAfter(func(e entity.Event) int {
        if e.Seq == lastSeq { return 0 }
        if e.Seq < lastSeq { return 1 }
        return -1
    })
    return sub, found
}
```

`found=false` 意味着 `lastSeq` 已被 evict，前端需走 `GET /messages` 补齐。

---

## 5. SSE 连接机制

### 5.1 端点设计

复用 `POST /api/sessions/{id}/chat`，增加两个可选参数：

```
POST /api/sessions/{id}/chat

Request:
  content        string   (optional)  新消息内容。有值 → 启动 agent
  reconnect      bool     (optional)  仅重连模式
  last_event_seq int64    (optional)  断线前收到的最后一个 seq

Response:
  200  → SSE 流（首次或重连成功）
  204  → reconnect=true 但无活跃 agent（空响应）
  409  → 有活跃 agent 且 reconnect=false（前端应改用 reconnect）
```

### 5.2 Handler 统一逻辑

```
POST /chat handler:

  chatCtx = SessionAgentStore.Get(sessionID)

  // 情况 1: 有 content → 启动新 agent
  if content != "":
      if chatCtx != nil → 409 (agent already running)
      chatCtx = NewChatContext(...)
      SessionAgentStore.Put(sessionID, chatCtx)
      go engine.Chat(chatCtx, content)

  // 情况 2: reconnect → 接已有流
  if reconnect:
      if chatCtx == nil → 204 (no active agent)
      // 连接已有 chatCtx

  // 情况 3: 无效请求
  if chatCtx == nil → 400 (no content, not reconnect)

  // 统一: Subscribe + SSE 写入
  sub, ok = chatCtx.Subscribe(lastEventSeq+1)
  if !ok:
      写入 {"type": "events_lost"} → 关闭连接
      return

  for event := range sub.Iter():
      写入 SSE
      c.Writer.Flush()
```

### 5.3 SessionAgentStore

```
全局单例，sessionID → ChatContext 映射

  Put(sessionID, chatCtx)   // agent 启动时注册
  Get(sessionID) → chatCtx  // 重连时查找
  Remove(sessionID)         // agent 完成时移除 (chatCtx.Close 内调用)
```

### 5.4 ChatContext 生命周期

```
POST /chat (content="hello")
  → NewChatContext → SessionAgentStore.Put
  → go engine.Chat() → Emit events → ringbuf
  → SSE goroutine → Subscribe → 读取 ringbuf → 写入 HTTP

agent 完成:
  → persistMessage()
  → chatCtx.Close()
    → ringbuf.Close()     → 所有 subscriber 收到 io.EOF
    → SessionAgentStore.Remove()

SSE disconnect (前端主动关闭):
  → chatCtx.Unsubscribe(sub)
  → agent 继续运行（ringbuf 不受影响）
  → 前端可重连
```

---

## 6. 断线重连保证

### 6.1 数据三态

```
已持久化 (DB)              流中 (ringbuf)              未产出
══════════════              ═════════════              ══════
GET /messages 可见          Subscribe 可 replay        LLM 还没生成
state = "done"              state = "streaming"        不存在
```

### 6.2 重连场景覆盖

| 场景 | 前端状态 | 恢复方式 | 丢失 |
|------|---------|---------|------|
| 短断线（< ringbuf 容量） | React state 保留 | Subscribe(seq) → Seek 定位 → 从断点继续 | 零 |
| 中断线（ringbuf 部分 evict） | React state 保留 | Subscribe → ok=false → GET /messages 拿 checkpoint → Subscribe(current) | 最多 ~10 delta（见 §9） |
| 长断线（agent 已结束） | React state 保留 | Subscribe → ok=false → GET /messages 拿完整消息 | 零 |
| 页面刷新 | State 丢失 | GET /messages 拿 checkpoint + Subscribe(current) | 同上 |

### 6.3 前端重连流程

```
SSE onError / onClose:

  ① 标记 isReconnecting = true
  ② 保留当前 messages 不修改
  ③ POST /chat { reconnect: true, last_event_seq: N }

  重连成功:
    ④ CopConChatProvider 继续在原 message 上追加
    ⑤ isReconnecting → false

  重连失败 (events_lost):
    ⑥ GET /messages → 获取最新 checkpoint
    ⑦ 合并: 如果当前 streaming message 的 text 比 checkpoint 长 → 保留当前
    ⑧ POST /chat { reconnect: true, last_event_seq: 0 }
    ⑨ 继续收 part_update 直到 message_done
```

---

## 7. Subagent 工具设计

### 7.1 delegate_to 工具

统一的委托工具，不按 agent 拆分：

```
Tool name: delegate_to

Parameters:
  agent_id   string   (required) 目标 agent ID
  task       string   (required) 子 agent 任务描述
  mode       string   (optional) "sync" | "async", 默认 "sync"
  extra      object   (optional) 传给工厂方法的额外参数
```

### 7.2 Execute 流程

```
delegate_to.Execute(chatCtx, args):

  1. 获取工厂
     factory = agentRegistry.GetFactory(args.agent_id)

  2. 构建 AgentDefinition
     agentDef = factory.Create(ctx, CreateParams{
         Task:          args.task,
         ParentContext: buildParentSummary(chatCtx),
         Extra:         args.extra,
     })
     → AgentDefinition { ..., Hooks: [...] }

  3. 创建子会话
     subSession = sessionMgr.Create(parent_session_id=chatCtx.SessionID())

  4. 创建子 ChatContext + 注册
     subChatCtx = NewChatContext(ctx, subSession.ID, args.agent_id)
     SessionAgentStore.Put(subSession.ID, subChatCtx)

  5. 注入 task 作为子会话首条消息
     contextMgr.AddMessage(subChatCtx, &Message{Role: "user", Content: args.task})

  6. 启动子 Agent
     go engine.Chat(subChatCtx, args.task)

  7. 返回
     if mode == "sync":
         等待 subChatCtx.Closed()
         收集结果 → { sub_session_id, summary, status: "completed" }
     if mode == "async":
         立即返回 → { sub_session_id, summary: "", status: "started" }
```

### 7.3 嵌套深度限制

```
engine.Chat() 入口检查:
  if chatCtx.Depth >= 3 → return error("max subagent depth exceeded")

会话树:
  session-A          (depth=0)
    ├─ session-sub1  (depth=1, parent=session-A)
    └─ session-sub2  (depth=1, parent=session-A)
         └─ session-sub2-1 (depth=2, parent=session-sub2)
```

### 7.4 主 Agent 获取子 Agent 结果

- sync 模式：tool result 的 output 直接包含 summary
- async 模式：tool result 的 output 包含 `sub_session_id`，后续通过两条路径获取结果：
  1. 前端轮询 `GET /updates` → 发现 async 完成 → 重新 POST /chat 让主 agent 处理
  2. 主 agent 调用 `read_sub_session` tool 按需读取详情

---

## 8. 子 Agent 事件隔离

### 8.1 原则

子 Agent 的 SSE 事件**永不转发到主 Agent 的 SSE 流**。主 Agent 的 tool-call output 只包含 `{ sub_session_id, summary }`。

### 8.2 前端连接子 Agent

```
主 agent 流中收到:
  part_update(tool-call, state=complete, output={ sub_session_id: "abc-123", ... })

前端:
  ① 在侧边栏/弹出层中:
     POST /api/sessions/abc-123/chat { reconnect: true }
  ② 收到子 agent 的完整 Step/Part 事件流
  ③ 渲染为可折叠的 SubagentCard
  ④ 断开不影响子 agent 继续执行
```

### 8.3 前端依赖

主 agent 流中不再包含 `subagent_start` / `subagent_end` 事件类型。主 agent 只看到普通 tool-call lifecycle。子 agent 的连接和渲染由前端根据 `sub_session_id` 主动发起。

---

## 9. Incremental Persist

### 9.1 时机

| 事件 | Action |
|------|--------|
| `part_create` | 不 persist |
| `part_update` text delta (每 ~10 个) | UPSERT message.parts (当前状态) |
| `part_update` state="done" | UPSERT message.parts |
| `part_update` tool-call complete | UPSERT message.parts |
| `message_done` | 最终 persist，标记 message 完成 |

### 9.2 保证

`GET /messages` 所见数据永远落后最多 ~10 个 text delta。配合 ringbuf（1024 槽位）撑住短断线，DB checkpoint 撑住长断线，前端 React state 撑住页面未刷新，三层保证不丢消息。

---

## 10. 前端改造

### 10.1 useAgentChat 增加重连

```
useAgentChat 当前:
  - 加载历史: GET /messages
  - 发送消息: POST /chat → SSE 流 → CopConChatProvider.transformMessage
  - 断线: 停止

useAgentChat 改造后:
  - 增加 onSSEError / onSSEClose 回调
  - 断线时:
    ① isReconnecting = true, 保留 messages
    ② POST /chat { reconnect: true, last_event_seq }
    ③ 成功: 继续 transform + isReconnecting = false
    ④ 失败: GET /messages → merge → POST /chat reconnect
  - 导出 isReconnecting 供 UI 使用
```

### 10.2 子 Agent 前端组件

```
SubagentCard:
  - 根据 sub_session_id 发起 SSE 连接
  - 渲染子 agent 的 Step/Part 流
  - 可折叠/展开
  - 独立生命周期（断开不影响子 agent）
```

### 10.3 类型扩展

```
ToolCallPart 扩展:
  subSessionId?: string    // delegate_to 产出的子会话 ID
  subSummary?:   string    // sync 模式下的摘要
  subStatus?:    string    // "started" | "completed" | "failed"
```

---

## 11. 完整组件关系

```
                          main.go
                             │
            注册 agent 工厂 ──┼── 注册 plugins (全局 hooks)
                             │
                    ┌────────┴────────┐
                    │  AgentRegistry   │
                    │  map[id]factory  │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
              ▼              ▼              ▼
        code-assistant  code-reviewer   sre-agent
        factory_A()     factory_B()     factory_C()
              │              │              │
              ▼              ▼              ▼
         AgentDef       AgentDef       AgentDef
         {Hooks}        {Hooks}        {Hooks}
              │              │              │
              └──────────────┼──────────────┘
                             │
                    ┌────────┴────────┐
                    │   engineImpl     │
                    │   (一个进程一个)   │
                    │                  │
                    │ runAgentLoop():  │
                    │  composeHooks()  │
                    │  BuildContext()  │
                    │  handleStream()  │
                    │  handleTools()   │
                    │   └─ delegate_to │
                    │       .Execute() │
                    │       → factory  │
                    │       → sub sess │
                    │       → sub eng  │
                    └────────┬────────┘
                             │
                    ┌────────┴────────┐
                    │ SessionAgent     │
                    │ Store            │
                    │ sessID→ChatCtx   │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
              ▼              ▼              ▼
         ChatCtx-A      ChatCtx-sub1   ChatCtx-sub2
         ringbuf         ringbuf        ringbuf
              │              │              │
         ┌────┴────┐   ┌────┴────┐   ┌────┴────┐
         ▼         ▼   ▼         ▼   ▼         ▼
       SSE 1    SSE 2  SSE 3  SSE 4  SSE 5   SSE 6
       主会话    主会话   子1     子1    子2     子2
       (首次)   (重连)  (首次)  (重连)  (首次)  (重连)
```

---

## 12. 实施阶段

| 阶段 | 目标 | 关键改动 |
|------|------|---------|
| **P1** | ringbuf 集成 + ChatContext 升级 | `ringbuf` 依赖引入, `ChatContext` 内部 chan → ringbuf, `SessionAgentStore` 新增, SSE handler 统一首次/重连 |
| **P2** | AgentRegistry 工厂化 | `RegisterFactory()` 接口, `AgentDefinition.Hooks`, `config.yaml` 移除 agents 段, `main.go` 显式注册 |
| **P3** | delegate_to + 子会话 | `tools/delegate.go` 新增, 子会话创建, `sessions.parent_session_id`, 深度限制 |
| **P4** | incremental persist + 前端重连 | 引擎中增量 persist, `useAgentChat` 重连逻辑, `AgentClient.reconnect()` |
| **P5** | 前端子会话连接 + SubagentCard | 侧边栏 SSE 连接, 折叠组件, async 模式轮询 |
| **P6** | 深度限制完善 + 边界处理 | `read_sub_session` tool, error 处理, 完整体验 |

---

## 13. 关键依赖与约束

### 依赖

| 组件 | 来源 | 用途 |
|------|------|------|
| `github.com/golang-cz/ringbuf` | 第三方 | 事件分发环形缓冲区 |
| 现有 engine + hook 体系 | 内部 | 不改，直接复用 |
| 现有 Step/Part 事件协议 | 内部 | 不改 |
| 现有 Tool 接口 | 内部 | delegate_to 实现 `tool.Tool` |

### 约束

| 约束 | 值 |
|------|-----|
| 最大嵌套深度 | 3 层 |
| ringbuf 容量 | 1024 槽位 |
| Incremental persist 频率 | 每 ~10 个 text delta |
| 每个 subscriber goroutine | 1 个（SSE 连接量级可忽略） |
| SSE 重连参数 | `reconnect` + `last_event_seq` |

### 不做的事

- config.yaml 定义 agent（改为代码注册）
- config.yaml 模板驱动 prompt（改为工厂方法自由实现）
- 子 agent 事件转发到主 agent SSE 流（改为独立连接）
- 按 agent 创建多个 delegate_to 工具（改为统一工具 + agent_id 参数）
- Agent 抽象为可执行接口（保持 Definition + Engine 分离）