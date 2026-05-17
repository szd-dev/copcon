# 核心-外设架构 (Core-Periphery Architecture)

## 概述

CopCon 后端采用"核心-外设"架构。核心引擎负责 Agent 循环、LLM 调用和工具调度，这些逻辑**永不改变**。所有可扩展行为（记忆、日志、追踪、自定义插件）通过 Hook 系统注入，作为外设附着在核心管道上。

新能力的添加方式是编写 Hook 并注册，而不是修改核心引擎代码。

## 架构示意图

```
┌─────────────────────────────────────────────────────────────────────┐
│                           HTTP 入口层                                │
│  POST /api/sessions/:id/chat  --->  POST /api/sessions  --->  ...   │
└───────────────────────────────┬─────────────────────────────────────┘
                                │ ChatContext
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                       ┌──────────────┐                               │
│                       │   Handler    │                               │
│                       └──────┬───────┘                               │
│                              │ chatCtx, userInput                    │
│                              ▼                                       │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                      CORE（引擎核心）                           │  │
│  │                                                                │  │
│  │   AgentEngine.Chat(chatCtx, userInput)                         │  │
│  │         │                                                      │  │
│  │         ▼                                                      │  │
│  │   ┌──────────────────────────────────────────┐                │  │
│  │   │           Agent Loop（主循环）             │                │  │
│  │   │                                           │                │  │
│  │   │   prepareAgentLoop()                      │                │  │
│  │   │       │                                   │                │  │
│  │   │       ▼                                   │                │  │
│  │   │   ┌──────┐    ┌──────┐    ┌───────────┐  │                │  │
│  │   │   │ Hook │───>│ LLM  │───>│  Tools    │  │                │  │
│  │   │   │Runner│    │Call  │    │Dispatcher │  │                │  │
│  │   │   └──────┘    └──────┘    └───────────┘  │                │  │
│  │   │       │           │             │         │                │  │
│  │   │       ▼           ▼             ▼         │                │  │
│  │   │   事件流     Streaming    工具执行         │                │  │
│  │   │       │           │             │         │                │  │
│  │   │       └───────────┴─────┬───────┘         │                │  │
│  │   │                         ▼                 │                │  │
│  │   │              persistMessage()             │                │  │
│  │   │                         │                 │                │  │
│  │   │              是否有 tool_calls？           │                │  │
│  │   │              │ yes         │ no           │                │  │
│  │   │              ▼             ▼              │                │  │
│  │   │          下一轮迭代    message_done        │                │  │
│  │   └──────────────────────────────────────────┘                │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                              │                                       │
│                     SSE Stream → Client                              │
└─────────────────────────────────────────────────────────────────────┘

                                  ▲
                                  │ Hook 注入点
                                  │
┌─────────────────────────────────┼─────────────────────────────────┐
│                           PERIPHERY（外设层）                       │
│                                                                     │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │
│   │ Memory   │  │ Logging  │  │ Tracing  │  │  Todo Manager    │  │
│   │ Manager  │  │  Hook    │  │  Hook    │  │  (via Hook)      │  │
│   └──────────┘  └──────────┘  └──────────┘  └──────────────────┘  │
│                                                                     │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │
│   │ Rate     │  │ Audit    │  │ Custom   │  │  Custom Plugin   │  │
│   │ Limit    │  │  Log     │  │ Filter   │  │  Plugin B        │  │
│   └──────────┘  └──────────┘  └──────────┘  └──────────────────┘  │
│                                                                     │
│   任何业务需要 ──→ 写一个 Hook ──→ 注册到 HookRunner ──→ 生效       │
└─────────────────────────────────────────────────────────────────────┘
```

## 请求完整流程

一次 `/chat` 请求从入站到 SSE 输出，经过以下路径：

```
HTTP POST (JSON body)
    │
    ▼
Handler 层:
    1. 解析请求，提取 content 和 agentID
    2. 创建 ChatContext（携带 sessionID、agentID、events channel）
    3. 启动 goroutine 调用 AgentEngine.Chat(chatCtx, content)
    4. 进入 SSE 循环: for event := range chatCtx.Events() { write SSE }

AgentEngine.Chat():
    │
    ├─ prepareAgentLoop()
    │   ├─ sessionMgr.Get(chatCtx)           // 加载会话
    │   ├─ Hook: OnSessionResolve            // 钩子：会话解析后
    │   ├─ 3层 AgentID 回退:                 // agentID → defaultAgentID → registry.Default()
    │   ├─ agentRegistry.Get(agentID)        // 获取 Agent 定义
    │   └─ contextMgr.AddMessage(user msg)   // 持久化用户消息
    │       │
    │       ▼
    ├─ Agent Loop (多轮迭代):
    │   │
    │   ├─ Emit step_create（第2轮起）
    │   │
    │   ├─ Hook: OnSystemPrompt              // 钩子：解析系统提示
    │   ├─ Hook: BeforeContextBuild          // 钩子：构建上下文前
    │   ├─ contextMgr.BuildContext()         // 组装消息窗口
    │   ├─ Hook: AfterContextBuild           // 钩子：上下文构建后
    │   ├─ Hook: BeforeLLMCall               // 钩子：LLM 调用前
    │   │
    │   ├─ handleStreaming()
    │   │   ├─ LLMProvider.Stream(params)    // 流式调用 LLM
    │   │   └─ 实时 Emit: part_create / part_update
    │   │
    │   ├─ Hook: AfterLLMCall                // 钩子：LLM 调用后
    │   │
    │   ├─ handleToolCalls()
    │   │   ├─ Hook: BeforeToolExecute       // 钩子：工具执行前
    │   │   ├─ ToolManager.Execute()         // 执行工具
    │   │   ├─ Hook: AfterToolExecute        // 钩子：工具执行后
    │   │   ├─ Hook: OnToolError（失败时）    // 钩子：工具错误
    │   │   └─ persistToolResults → contextMgr.AddMessage（tool role）
    │   │
    │   ├─ persistMessage()
    │   │   ├─ contextMgr.AddMessage(assistant msg)  // 持久化助手消息
    │   │   └─ Hook: OnMessagePersist        // 钩子：消息持久化后
    │   │
    │   ├─ 有 tool_calls 且有结果 → stepIndex++, 回到 Loop 顶部
    │   └─ 无 tool_calls → Emit message_done, return
    │
    └─ 错误时: Emit error 事件
```

## 核心组件

| 组件 | 文件 | 职责 |
|------|------|------|
| `AgentEngine` | `internal/agent/engine.go` | 主循环编排，协调 LLM / Hook / Tool |
| `AgentRegistry` | `internal/agent/registry.go` | 管理已注册的 Agent 定义 |
| `AgentDefinition` | `internal/agent/types.go` | Agent 配置（模型、提示词、工具集） |
| `HookRunner` | `internal/hook/runner.go` | Hook 注册和执行引擎 |
| `LLMProvider` | `internal/llm/provider.go` | LLM 后端抽象接口 |
| `ToolManager` | `internal/tool/manager.go` | 工具注册和执行 |
| `ContextManager` | `internal/chat_context/` | 上下文窗口管理和消息持久化 |
| `SessionManager` | `internal/session/` | 会话 CRUD |

## 外设示例

| 外设 | 实现方式 | Hook 点 |
|------|---------|---------|
| Memory（向量记忆） | 注册 Memory Hook | `AfterContextBuild`, `OnMessagePersist` |
| 请求日志 | 注册 Logging Hook | `BeforeLLMCall`, `AfterLLMCall` |
| 链路追踪 | 注册 Tracing Hook | 多个 Hook 点 |
| 速率限制 | 注册 RateLimit Hook | `BeforeLLMCall` |
| Todo 管理 | 注册 Todo Hook | `OnSystemPrompt`, `AfterLLMCall` |
| 自定义插件 | 实现 `hook.Hook` 接口 | 任意 Hook 点 |

## 包结构概览

```
server/internal/
├── agent/             # 核心引擎：AgentEngine, AgentRegistry, AgentDefinition
│   ├── engine.go      #   Chat(), runAgentLoop(), handleStreaming(), persistMessage()
│   ├── engine_tools.go#   handleToolCalls(), executeTools()
│   └── registry.go    #   AgentRegistry 实现
│
├── domain/            # 共享领域类型
│   ├── entity/        #   Event, StepCreateData, PartCreateData 等
│   └── iface/         #   ChatContextInterface, AgentInterface 等
│
├── hook/              # Hook 系统（外设机制）
│   ├── hook.go        #   HookPoint 常量, Hook 接口, HookContext
│   └── runner.go      #   HookRunner 实现：Register(), Run(), On()
│
├── llm/               # LLM Provider 抽象
│   └── provider.go    #   LLMProvider 接口, StreamParams, StreamChunk, Message
│
├── chat_context/      # 上下文管理
│   └── manager.go     #   ContextManager: BuildContext(), AddMessage()
│
├── session/           # 会话管理
├── memory/            # 向量记忆（Qdrant）
├── tool/              # 工具注册和执行
├── tools/             # 具体工具实现
├── plugins/           # 内置插件（外设）
└── api/               # HTTP 处理器和路由
```

## 关键设计原则

**1. 引擎不变，外设可变**

AgentEngine 的核心循环不会因业务需求而修改。需要记忆、日志、追踪等功能时，编写 Hook 并注册，而不是修改 `engine.go`。

**2. Hook 优先于配置**

比配置系统更灵活。Hook 可以在运行时修改上下文消息、拦截工具调用、转换 LLM 输出。配置文件只能做静态参数。

**3. 接口隔离**

核心组件之间通过接口通信：
- `AgentEngine` → `iface.ChatContextInterface`（不是具体类型）
- `AgentEngine` → `llm.LLMProvider`（可以替换为任何 LLM 后端）
- `AgentEngine` → `hook.HookRunner`（可以不注入，则无 Hook 执行）

**4. 事件驱动流式输出**

引擎不直接写 HTTP 响应。所有输出通过 `ChatContext.Emit()` 发送事件，由 Handler 层的 SSE 循环读取并推送给客户端。这样引擎不依赖 HTTP 传输协议。

**5. 错误不传播，只记录**

Hook 执行的错误不会中止管道。错误的 Hook 被记录后跳过，后续 Hook 继续执行。同样的，单个工具失败也不会阻止 Agent Loop 继续。

---

下一篇：[02-agent-engine.md](./02-agent-engine.md)