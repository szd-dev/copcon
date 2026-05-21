# CopCon — 架构与设计思想

> 版本: 2026-05 | 覆盖: 后端引擎、前端组件库、交互协议、子系统设计

## 一、项目定位

CopCon 是一个完整的 **Agent 基建系统**——不是"又一个 ChatBot 框架"，而是一个为多模态 Agent 编排提供全链路基础设施的平台。

| 维度 | 定位 |
|------|------|
| 后端 | 通用 Agent 引擎：ReAct Loop、工具执行、会话/上下文/记忆管理 |
| 前端 | 可嵌入的 React 组件库（`@agent-infra/ui`），提供流式渲染、交互、子 Agent 可视化 |
| 协议 | 统一的 UI 层 SSE 事件协议（Step/Part 体系），前后端共享 Canonical 数据模型 |
| 编排 | 代码注册的多 Agent 工厂体系，支持 Subagent 委托和嵌套 |
| 交互 | 全内存 HITL（Human-in-the-Loop）机制，工具级人类审批与问答 |

---

## 二、核心设计哲学

### 2.1 引擎统一，行为外置

**一个 Engine，多种 Agent。** Agent 的差异通过三个可组合维度表达：

1. **AgentDefinition** — 模型、SystemPrompt、工具集、LLMProvider
2. **AgentFactory** — 任意复杂的工厂方法，运行时参数注入（Task、ParentContext、ModelOverride）
3. **Hook** — 声明式拦截引擎生命周期的特定节点

Agent 不需要不同的执行模式——它们都是 ReAct Loop 的实例。差异由 Hook 和 Definition 表达，不由引擎分支表达。

### 2.2 数据模型驱动

消息格式是架构契约，不是实现细节。前后端共享一个 Canonical 数据模型（`UIMessage → Steps → Parts`），SSE 事件协议和 REST API 产出完全相同形状的数据。这意味着：

- 流式路径和刷新路径后端无需维护两套序列化逻辑
- 前端对 SSE 事件和 GET /messages 响应做统一处理
- 字段命名统一为 camelCase，Go JSON tag 与 TypeScript 类型直接对应

### 2.3 内存优先，持久化兜底

核心运行状态（ChatContext、ringbuf 事件流、SessionAgentStore、HITL 阻塞）全部在内存中，数据库仅用于 checkpoint。三层数据保证：

```
React State (实时)  →  Ringbuf (缓冲窗口 1024)  →  PostgreSQL (增量 checkpoint)
```

### 2.4 协议精简

SSE 流只传输 **UI 层事件**（`step_create / part_create / part_update / message_done`），不传输 LLM 原始事件（`message / tool_call / tool_result`）。协议只关心"前端需要渲染什么"，不关心"LLM 如何产出这些内容"。

---

## 三、系统架构

```
                            ┌──────────────────────────┐
                            │        LiteLLM           │
                            │  (多模型统一代理网关)      │
                            │  kimi | gpt | claude |   │
                            │  gemini | deepseek       │
                            └───────────┬──────────────┘
                                        │ OpenAI-compatible API
                                        ▼
┌───────────────────────────────────────────────────────────────────┐
│                         Agent Engine                               │
│                                                                    │
│  ┌──────────────┐   ┌──────────────┐   ┌───────────────────────┐  │
│  │ AgentRegistry │   │ engineImpl    │   │   Hook Pipeline       │  │
│  │  Factory Map  │──▶│  runLoop()    │──▶│  Before/After hooks   │  │
│  │  id→Factory   │   │  compose()    │   │  10 hook points      │  │
│  └──────────────┘   └──────┬───────┘   └───────────────────────┘  │
│                             │                                       │
│         ┌───────────────────┼───────────────────┐                  │
│         ▼                   ▼                   ▼                  │
│  ┌─────────────┐   ┌──────────────┐   ┌──────────────┐            │
│  │ ContextMgr  │   │ ToolManager   │   │ LLMProvider   │            │
│  │ Build/Add/  │   │ Register/     │   │ Stream()     │            │
│  │ Upsert/Pers │   │ Execute       │   │ OpenAIClient │            │
│  └─────────────┘   └──────┬───────┘   └──────────────┘            │
│                            │                                       │
│         ┌──────────────────┼──────────────────┐                   │
│         ▼                  ▼                  ▼                   │
│  ┌────────────┐   ┌──────────────┐   ┌──────────────┐            │
│  │ code_exec  │   │ delegate_to  │   │ confirm/ask  │            │
│  │ file_ops   │   │ todolist     │   │ get_status   │            │
│  └────────────┘   └──────────────┘   └──────────────┘            │
│                                                                    │
│                         ┌─────────────┐                           │
│                         │  ChatContext │                           │
│                         │  ringbuf[1024]  │                          │
│                         │  HITL chans  │                          │
│                         └──────┬──────┘                           │
│                                │                                   │
└────────────────────────────────┼───────────────────────────────────┘
                                 │ SSE (text/event-stream)
                                 ▼
┌───────────────────────────────────────────────────────────────────┐
│                         Frontend (@agent-infra/ui)                 │
│                                                                    │
│  ┌────────────────┐  ┌─────────────────┐  ┌────────────────────┐  │
│  │ CopConChat      │  │ useAgentChat     │  │ useSubagentSSE     │  │
│  │ Provider        │  │ (useXChat适配)   │  │ (子Agent独立连接)   │  │
│  │ transformMessage│  │ reconnect        │  │ SubagentCard       │  │
│  └────────────────┘  └─────────────────┘  └────────────────────┘  │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  UI Components                               │              │  │
│  │  ┌───────────┐ ┌───────────┐ ┌───────────────────────┐      │  │
│  │  │HumanInter-│ │SubagentCard│ │ TodoList/TodoItem     │      │  │
│  │  │ action    │ │(独立SSE流) │ │ (Agent Todo可视化)    │      │  │
│  │  └───────────┘ └───────────┘ └───────────────────────┘      │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  ┌───────────────────────────────┐                                 │
│  │       AgentClient             │                                 │
│  │  send / reconnect / resume   │                                 │
│  │  stop / getMessages          │                                 │
│  └───────────────────────────────┘                                 │
└───────────────────────────────────────────────────────────────────┘
          │                    │                    │
          ▼                    ▼                    ▼
    ┌──────────┐       ┌──────────┐        ┌──────────┐
    │PostgreSQL│       │  Qdrant  │        │  Demo    │
    │Sessions  │       │ Vector   │        │  App     │
    │Messages  │       │ Memory   │        │ (Vite)   │
    └──────────┘       └──────────┘        └──────────┘
```

---

## 四、子系统详解

### 4.1 Agent Engine（核心引擎）

**职责**：执行 Agent 的主循环（ReAct Loop）

**关键设计决策**：

| 决策 | 选择 | 理由 |
|------|------|------|
| Agent 模型 | Definition + Engine 分离 | Subagent 不需要不同的执行模式，Hook 已覆盖所有定制需求 |
| 注册方式 | 代码注册（非 config.yaml） | 工厂方法可做任意复杂操作（DB查询、API调用、向量检索） |
| 消息格式 | Step/Part 体系 | 每次 LLM 迭代 = 一个 Step，迭代内内容 = 有序 Parts |
| 并发控制 | Semantic semaphore | 限制并发工具执行数量 |
| 增量持久化 | 每 10 个 text delta 做一次 upsert | 平衡 I/O 压力与数据安全性 |

**Agent Loop 流程**：

```
POST /chat(content="用户输入")
  → NewChatContext + SessionAgentStore.Put
  → go engine.Chat(chatCtx, content)
    → runAgent: compose(globalHooks, agentDef.Hooks)
    → for step in 0..50:
        → BeforeContextBuild hooks
        → BuildContext(chatCtx, userInput, maxTokens, systemPrompt)
        → AfterContextBuild hooks
        → handleStreaming → provider.Stream(lifecycleCtx)
          → Emit step_create / part_create / part_update
        → handleToolCalls → executeSync / executeConcurrent
          → Emit part_update(state: pending→running→complete/error)
        → persist checkpoint (每 10 delta 或 每次迭代结束)
    → Emit message_done
    → persist final message
  → chatCtx.Close() → ringbuf.Close() → SessionAgentStore.Remove()
```

**Hook 拦截点**：

| Hook Point | 触发时机 | 可变更内容 |
|------------|---------|-----------|
| `before_context_build` | 构建 LLM 上下文前 | SystemPrompt |
| `after_context_build` | 上下文构建后 | Messages for LLM |
| `on_system_prompt` | 解析 SystemPrompt 时 | Prompt 文本 |
| `before_llm_call` | LLM API 调用前 | 请求参数 |
| `after_llm_call` | LLM API 响应后 | 响应内容 |
| `before_tool_execute` | 工具执行前 | 工具参数 |
| `after_tool_execute` | 工具执行成功后 | 工具结果 |
| `on_tool_error` | 工具执行失败 | 提供回退结果 |
| `on_message_persist` | 消息持久化前 | 消息内容 |
| `on_session_resolve` | 会话解析时 | 会话 ID |

### 4.2 ChatContext（统一上下文对象）

**职责**：贯穿整个请求生命周期的唯一上下文，封装会话身份、事件流和交互状态

**核心设计**：

```
ChatContext = 生命周期上下文 + 事件环形缓冲区 + HITL 中断通道

┌────────────────────────────────────┐
│        ChatContext                  │
│                                     │
│  lifecycleCtx   独立生命周期      │  ← 与 HTTP Request 解耦
│    → agent 不受 SSE 断开影响      │
│                                     │
│  ringbuf[1024]  事件环形缓冲区    │  ← 多订阅者 + 各自游标
│    → Subscribe(fromSeq)           │     SSE1 / SSE2 / 重连
│                                     │
│  interruptChans HITL 阻塞通道     │  ← 内存阻塞，不序列化
│    → RequestInput / ResolveInput   │
│                                     │
│  atomic seq     全局事件序列号    │  ← 断线重连定位
└────────────────────────────────────┘
```

**关键设计决策**：

- **Lifecycle Context 解耦**：ChatContext 使用独立的 `context.Background()` 而不是 `http.Request.Context()`。这意味着客户端断开不会杀死 Agent goroutine，只退出 SSE 推送 goroutine。
- **Ringbuf 替代 Channel**：使用 `ringbuf.RingBuffer` 替代原生 `chan`，天然支持多订阅者（多个 SSE 连接同时读取同一个 Agent 的输出流），各订阅者拥有独立游标，支持 `Subscribe(fromSeq)` 从指定序列号恢复。
- **Emit 不变**：Agent 侧调用 `chatCtx.Emit(event)` 完全不变，内部从 `chan <- event` 改为 `ringbuf.Write(event)` + 原子递增序列号。

### 4.3 事件协议（Step/Part 体系）

**核心思想**：SSE 只传输 UI 层事件，不传输模型层事件。

**四种事件类型**：

```
step_create     → 开始一次新的 Agent Loop 迭代
part_create     → 首次产出 reasoning / text / tool-call 部件
part_update     → 追加内容 / 更新状态 / 产出 output
message_done    → 整个消息流式传输结束
```

**数据模型**：

```
UIMessage
├── id, role, metadata
└── steps: Step[]
    └── parts: Part[]
        ├── TextPart       { type, text, state }
        ├── ReasoningPart  { type, text, state }
        └── ToolCallPart   { type, toolCallId, toolName, args, output, error, state }
                            └── interrupt? (HITL)
```

**从旧协议迁移的原因**：

| 旧设计 | 新设计 | 解决的问题 |
|--------|--------|----------|
| 每个 LLM chunk 发射 2 个事件 | 1 个 part_update | 冗余、性能 |
| 全局 partIndex（迭代间重置） | stepIndex + partIndex（二级索引） | 多迭代 parts 冲突 |
| `tool_result(object)` 导致 JSON 解析 crash | part_update.output（后端序列化为 string） | 类型安全 |
| `done` 事件语义模糊 | `message_done` 明确标识 | 语义清晰 |

### 4.4 Agent Registry（Agent 注册体系）

**职责**：通过工厂方法注册 Agent，运行时按需创建

**设计原则**：

- **Factory 自由，Engine 统一**：每类 Agent 通过独立工厂构建，但共享同一套 Engine 执行逻辑
- **代码注册优于配置**：工厂方法可查询 DB、调用 API、检索向量、动态拼装 SystemPrompt——这些能力远超静态 YAML 模板
- **许可制委托**：只有标记 `allowDelegate=true` 的 Agent 才出现在 `ListDelegatable()` 中

```
AgentFactory(ctx, CreateParams{Task, ParentContext, ModelOverride, Extra})
    → AgentDefinition{ID, Name, Model, SystemPrompt, ToolManager, LLMProvider, Hooks}

AgentDefinition = 可组合的 Agent 配置块
Hooks           = 该 Agent 特有的行为定制
CreateParams    = 运行时参数注入（主 Agent 委托时为子 Agent 注入 Task）
```

### 4.5 Subagent 委托

**职责**：主 Agent 可以将子任务委托给其他 Agent，子 Agent 在独立会话中执行

**关键设计决策**：

| 决策 | 选择 | 理由 |
|------|------|------|
| 委托工具 | 单一 `delegate_to` + `agent_id` 参数 | LLM 自行选择目标，不按 Agent 拆分工具 |
| 子 Agent 会话 | 独立子会话（`parent_session_id`） | 上下文隔离，可审计 |
| 子 Agent 事件 | 独立 SSE 流，不转发到主流 | 主 Agent 流保持干净 |
| 嵌套深度 | 硬限制 3 层 | 覆盖主→子→孙场景，防止失控 |
| 结果返回 | `{ sub_session_id, summary }` | 主 Agent 看摘要决策，需详情时前端按需查询 |

**前端子 Agent 连接**：

```
主 Agent 流中收到:
  part_update(tool-call, output={ sub_session_id: "abc-123" })

前端:
  ① SubagentCard 组件自动发起:
     POST /api/sessions/abc-123/chat { reconnect: true }
  ② 收到子 Agent 的完整 Step/Part 事件流
  ③ 渲染为可折叠的子 Agent 卡片
  ④ 断开不影响子 Agent 继续执行
```

### 4.6 HITL（Human-in-the-Loop）

**职责**：工具执行过程中可暂停并请求人类输入，人类响应后继续执行

**核心约束**：

- **内存阻塞**：工具通过 `chatCtx.RequestInput()` 阻塞在 channel 上，不做持久化序列化
- **工具自治**：工具自行决定何时请求人类输入，Engine 完全无感知
- **LLM 隔离**：LLM 不知道工具暂停过——它只看到正常的 ToolResult 或错误
- **SSE + POST 混合**：SSE 推送 `waiting_for_input` 状态 → 前端渲染交互 UI → POST `/resume` 提交响应

```
Agent → 工具执行 → 发现需要人类确认
  → chatCtx.RequestInput({type: "approval", message: "确认删除？"})
  → Emit part_update(state: waiting_for_input, interrupt: {...})
  → 阻塞在 channel 等待
  → 人类点击 Approve → POST /resume → channel 写入响应
  → RequestInput 返回 {action: "approve"}
  → 工具继续执行 → 返回 ToolResult
```

**两种交互类型**：

| 类型 | 场景 | 人类返回 | LLM 看到 |
|------|------|---------|---------|
| 审批（Approval） | 危险操作需确认 | approve / decline | 工具结果 或 "user declined" |
| 问答（Question） | 需要补充信息 | 结构化表单输入 | 包含输入的完整工具结果 |

### 4.7 Hook 系统（可编程扩展）

**职责**：在 Engine 生命周期的 10 个关键节点插入自定义逻辑

**设计理念**：

- **声明式注册**：Hook 声明它关注哪些 Point，引擎在到达该 Point 时自动调用
- **优先级排序**：同一 Point 上的多个 Hook 按 Priority 排序执行
- **可变性**：HookContext 中的关键字段使用指针，Hook 可直接修改（如替换 SystemPrompt、修改工具参数、转换工具结果）
- **错误不中断**：Hook 返回 error 只记录日志，不中断 Pipeline。需要中断时应返回已知的 Sentinel Error

**当前 Built-in Hooks（Plugins）**：

| Plugin | Hook Point | 作用 |
|--------|-----------|------|
| `TodoInjectionHook` | `on_system_prompt` | 将当前 Todo 列表状态注入 SystemPrompt |
| `MemoryInjectionHook` | `before_context_build` | 从 Qdrant 检索相关记忆注入上下文 |
| `LoggingHook` | 多个 Point | 结构化日志记录 |
| `TracingHook` | 多个 Point | 分布式追踪 |

### 4.8 断线重连

**问题**：SSE 连接断开时如何不丢失数据？

**三层保证**：

```
React State (立即)  →  Ringbuf (缓冲)  →  PostgreSQL (持久化)
─────────────────      ───────────────     ─────────────────
页面不刷新，state 在    短断线内补发事件     checkpoint 重建

重连流程:
  POST /chat { reconnect: true, last_event_seq: N }
    → Subscribe(fromSeq=N+1) → 从断点继续
    → 如果 fromSeq 已被 evict → 返回 events_lost
      → GET /messages 拿 checkpoint → 合并 → 重新 Subscribe
```

**ChatContext 生命周期**：Agent goroutine 和 SSE goroutine 独立存活——SSE 断开只退出推送 goroutine，Agent 继续执行、ringbuf 继续缓冲。

### 4.9 内存系统

**Qdrant Vector Memory**：

| 记忆类型 | 用途 | 检索策略 |
|---------|------|---------|
| Conversation Memory | 当前会话的重要对话节点 | 同 Session，score_threshold=0.7 |
| Summary Memory | 长对话的摘要版本 | 周期性自动生成 |
| Important Memory | 显式标记的重要信息 | 可跨 Session，score_threshold=0.8 |

**嵌入模型**：text-embedding-ada-002（1536维），HNSW 索引，Payload 索引进 `session_id` + `memory_type`。

---

## 五、前端架构

### 5.1 组件体系

```
@agent-infra/ui (React 组件库)
│
├── CopConChatProvider    ← SSE 事件 → UIMessage 转换器
│   └── transformMessage()  识别 step_create / part_create / part_update / message_done
│
├── useAgentChat          ← 核心 Chat Hook
│   ├── send()            首次发送 / 断线重连
│   ├── loadMessages()    GET /messages 加载历史
│   ├── isReconnecting    断线状态暴露
│   └── adapter for useXChat  @ant-design/x 适配
│
├── useSubagentSSE        ← 子 Agent 独立 SSE 连接 Hook
│   └── SubagentCard 组件的状态管理
│
├── HumanInteraction      ← HITL 交互 UI 组件
│   ├── approval mode     审批/拒绝按钮
│   └── question mode     表单填写
│
├── SubagentCard           ← 子 Agent 可视化卡片
│   └── 独立 SSE 连接 + 可折叠展示
│
├── TodoList / TodoItem    ← Agent Todo 列表可视化
│
└── AgentClient            ← HTTP Client
    ├── send / reconnect / resume / stop
    └── getMessages / getSessions
```

### 5.2 useXChat 适配

CopCon 的消息模型（一条 UIMessage 包含多个 Steps）适配 `@ant-design/x` 的 `useXChat`：

- **1 次 Agent Loop 完整执行 = 1 条 MessageInfo**（符合 useXChat 的 1 request = 1 message 模型）
- 多个 Step 在一条消息气泡内展示，Step 之间用 Divider 分隔
- `transformMessage` 处理 SSE 事件增量更新当前消息的 Steps

---

## 六、API 总表

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/sessions` | 创建会话 |
| GET | `/api/sessions` | 会话列表 |
| GET | `/api/sessions/:id` | 获取会话详情 |
| DELETE | `/api/sessions/:id` | 删除会话 |
| GET | `/api/sessions/:id/messages` | 获取消息历史（UIMessage 格式） |
| POST | `/api/sessions/:id/chat` | 发起对话 / 断线重连（SSE 流式） |
| POST | `/api/sessions/:id/resume` | HITL 恢复（提交人类响应） |
| POST | `/api/sessions/:id/stop` | 强制停止 Agent 执行 |

**POST /chat 请求参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `content` | string (optional) | 有新内容时 → 启动 Agent |
| `agent_id` | string (optional) | 指定 Agent ID |
| `reconnect` | bool (optional) | 纯重连模式 |
| `last_event_seq` | int64 (optional) | 断线前最后一个事件序列号 |

---

## 七、基础设施

| 组件 | 用途 | 关键特性 |
|------|------|---------|
| **PostgreSQL 15** | 主数据库 | Sessions、Messages、Todos（JSONB Parts） |
| **Qdrant 1.17** | 向量数据库 | Agent Memory（1536维 HNSW） |
| **LiteLLM** | 多模型网关 | 统一 OpenAI-compatible API，支持 kimi/gpt/claude/gemini/deepseek |
| **Gin 1.12** | HTTP 框架 | SSE 流式响应 |
| **GORM 1.31** | ORM | DB 操作 + JSONB 支持 |
| **React 19 + TypeScript 5** | 前端框架 | Strict mode |
| **@ant-design/x 2.x** | AI UI 组件 | Bubble、Sender、ThoughtChain |
| **Vite 6** | 构建工具 | 前端 Demo App |
| **Storybook 8** | 组件文档 | 开发时 UI 调试 |

---

## 八、部署架构

```
docker compose up -d
  ├── postgres:15-alpine     (5432)
  ├── qdrant:v1.17.0         (6333, 6334)
  ├── litellm:main-stable    (4000)  ← 多模型网关
  └── copcon-server          (8080)  ← Go 后端
         ↑
    OPENAI_BASE_URL=http://litellm:4000/v1  ← 通过 LiteLLM 访问所有模型
```

LiteLLM 作为前端代理，屏蔽不同模型 API 的差异，`config/litellm-config.yaml` 统一注册所有可用模型。

---

## 九、设计原则总结

| 原则 | 含义 |
|------|------|
| **Canonical Data Model** | 前后端共享同一套数据结构，无转换层 |
| **UI-Only Events** | SSE 协议只包含 UI 需要的渲染信息，不泄漏 LLM 内部状态 |
| **Memory-First** | 核心运行状态在内存中，DB 是安全网不是执行器 |
| **Engine Singular, Behavior External** | 一个引擎执行所有 Agent，差异由 Hook + Definition 表达 |
| **Tool Autonomy** | 工具自决何时需要人类输入，Engine 无感知 |
| **Explicit Boundaries** | 子 Agent 独立会话、独立 SSE 流、独立上下文 |
| **Three-Layer Data Safety** | React State → Ringbuf → PostgreSQL，逐层降级不丢数据 |
| **LLM Isolation** | LLM 上下文中不出现 HITL 中断请求、子会话 ID、人类原始输入 |