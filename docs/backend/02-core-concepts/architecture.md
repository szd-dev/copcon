# 架构概览

CopCon 采用分层架构,将核心能力与业务应用分离,确保高可复用性和灵活性。

## 分层结构

```
┌─────────────────────────────────────────┐
│         应用层 (server/)                │
│  ┌──────────────┐  ┌──────────────────┐ │
│  │ REST API     │  │ 配置管理         │ │
│  │ (Gin)        │  │ (config.yaml)    │ │
│  └──────────────┘  └──────────────────┘ │
└─────────────────────────────────────────┘
                  │
                  │ 依赖
                  ▼
┌─────────────────────────────────────────┐
│         核心层 (core/)                  │
│                                         │
│  ┌───────────────────────────────────┐  │
│  │  Harness (统一配置入口)           │  │
│  │  - StoreConfig                    │  │
│  │  - AgentSpec                      │  │
│  │  - AutoMigrate                    │  │
│  └───────────────────────────────────┘  │
│                                         │
│  ┌───────────────────────────────────┐  │
│  │  Agent 引擎                       │  │
│  │  • 对话循环                       │  │
│  │  • 流式响应                       │  │
│  │  • 上下文管理                     │  │
│  │  • 工具调用调度                   │  │
│  └───────────────────────────────────┘  │
│                                         │
│  ┌───────────────────────────────────┐  │
│  │  能力系统 (Capabilities)          │  │
│  │  ┌──────────┐  ┌──────────────┐  │  │
│  │  │ Tools    │  │ Hooks        │  │  │
│  │  │ 8 个内置 │  │ 4 个内置     │  │  │
│  │  └──────────┘  └──────────────┘  │  │
│  └───────────────────────────────────┘  │
│                                         │
│  ┌───────────────────────────────────┐  │
│  │  存储抽象 (Storage)               │  │
│  │  • SessionStore                   │  │
│  │  • MessageStore                   │  │
│  │  • TodoStore                      │  │
│  │  • MemoryStore (可选)             │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

## 核心组件

### Harness

Harness 是整个系统的配置入口,通过 `HarnessConfig` 声明所有组件:

```go
type HarnessConfig struct {
    Store     StoreConfig    // 存储配置
    Agents    []AgentSpec    // Agent 规格
    Tools     []ToolSpec     // 工具规格
    Hooks     []HookSpec     // Hook 规格
    Log       *slog.Logger   // 日志实例(可选)
}
```

**关键特性:**
- 所有组件通过配置注入,无全局状态
- 支持热重载:配置变更后重新构建 Harness
- AutoMigrate:自动创建数据库表结构

### Agent 引擎

Agent 引擎负责:
1. **对话循环**: 接收用户消息 → 调用 LLM → 处理工具调用 → 返回响应
2. **流式响应**: 实时输出 LLM 生成内容
3. **上下文管理**: 维护对话历史,支持滑动窗口
4. **工具调度**: 解析 LLM 工具调用请求,执行并返回结果

**执行流程:**
```
用户消息 
  → 构建上下文(历史 + 当前消息)
  → LLM 生成响应
  → 检测工具调用
  → 执行工具
  → 将结果反馈给 LLM
  → 继续生成(循环直到完成)
  → 返回最终响应
```

### 能力系统 (Capabilities)

能力系统统一管理 Tools 和 Hooks,采用**自动注册**机制:

#### Tools (工具)

工具让 Agent 能够执行特定操作:

```go
// 接口定义
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, input ToolInput) (ToolOutput, error)
}
```

**内置工具 (8 个):**
- `code_executor`: 代码执行 (Python/JavaScript/Shell)
- `shell_executor`: Shell 命令执行
- `file_ops`: 文件读写操作
- `todo`: 任务管理
- `search`: 文档搜索
- `web_browse`: 网页浏览
- `api_call`: HTTP API 调用
- `database_query`: 数据库查询

**注册方式:**
```go
init() {
    capabilities.RegisterTool(&CodeExecutorTool{})
}
```

#### Hooks (钩子)

Hooks 在对话生命周期的特定阶段执行:

```go
type Hook interface {
    Name() string
    OnRequest(ctx context.Context, req *Request) error
    OnResponse(ctx context.Context, resp *Response) error
    OnToolCall(ctx context.Context, tool ToolCall) error
    OnComplete(ctx context.Context, conv *Conversation) error
}
```

**内置 Hooks (4 个):**
- `logging`: 请求/响应日志
- `tracing`: 链路追踪
- `memory`: 长期记忆管理
- `todo_injection`: 自动注入任务上下文

**注册方式:**
```go
init() {
    capabilities.RegisterHook(&LoggingHook{})
}
```

**生命周期阶段:**
```
Request → Pre-Process Hooks → LLM Call → Tool Execution → Post-Process Hooks → Response
```

### 存储抽象

存储层定义了清晰的接口,具体实现由 Provider 提供:

```go
// 存储接口
type SessionStore interface {
    Create(ctx context.Context, session *Session) error
    Get(ctx context.Context, id string) (*Session, error)
    Update(ctx context.Context, session *Session) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filter SessionFilter) ([]*Session, error)
}

type MessageStore interface {
    Add(ctx context.Context, msg *Message) error
    GetHistory(ctx context.Context, sessionId string) ([]*Message, error)
    DeleteBySession(ctx context.Context, sessionId string) error
}

type TodoStore interface {
    Create(ctx context.Context, todo *Todo) error
    Get(ctx context.Context, id string) (*Todo, error)
    Update(ctx context.Context, todo *Todo) error
    List(ctx context.Context, sessionId string) ([]*Todo, error)
}

type MemoryStore interface {
    Add(ctx context.Context, mem *Memory) error
    Search(ctx context.Context, query string, topK int) ([]*Memory, error)
}
```

**实现方式:**
- `core/providers/postgres`: PostgreSQL 实现
- `core/providers/qdrant`: Qdrant 向量存储实现

## 设计原则

### 1. 接口抽象

所有组件通过接口交互,便于:
- 单元测试 (Mock 实现)
- 组件替换
- 扩展新功能

### 2. 配置驱动

通过 `HarnessConfig` 声明所有组件:
- 显式依赖注入
- 无全局状态
- 支持多实例

### 3. 自动注册

Tools 和 Hooks 通过 `init()` 自动注册:
- 减少手动配置
- 避免遗漏
- 插件化架构

### 4. 关注点分离

```
core/   → 通用能力,无业务逻辑
server/ → 具体应用,业务配置
```

## 数据流

### 标准对话流程

```
1. 用户发送消息
   ↓
2. server 接收请求 (REST API)
   ↓
3. Harness 创建 Conversation
   ↓
4. Pre-Process Hooks 执行 (logging, memory search)
   ↓
5. Agent Engine 构建上下文
   ↓
6. 调用 LLM 生成响应 (流式)
   ↓
7. LLM 返回工具调用请求?
   ├─ 是 → 执行工具 → 反馈结果 → 继续生成 (循环)
   └─ 否 → 完成生成
   ↓
8. Post-Process Hooks 执行 (logging, memory save)
   ↓
9. 保存 Conversation 到 SessionStore
   ↓
10. 返回响应给用户
```

### 流式响应机制

```go
// server 端 (Gin)
func (h *Handler) Chat(c *gin.Context) {
    // 开启 SSE 连接
    c.Writer.Header().Set("Content-Type", "text/event-stream")
    
    // 创建 ChatContext
    ctx := h.harness.NewChatContext(c.Request.Context())
    
    // 开始对话 (异步)
    go h.harness.Chat(ctx, req)
    
    // 流式输出
    for event := range ctx.Events() {
        c.SSEvent("message", event.Data)
        c.Writer.Flush()
    }
}
```

```go
// core 端 (Agent Engine)
func (e *Engine) Chat(ctx *ChatContext, req *Request) {
    for {
        // 调用 LLM
        stream := e.llm.Generate(ctx, req.Context)
        
        for chunk := range stream {
            ctx.Emit(Event{Type: "message", Data: chunk})
            
            if chunk.IsToolCall {
                // 执行工具
                result := e.executeTool(chunk.ToolCall)
                ctx.Emit(Event{Type: "tool_result", Data: result})
            }
            
            if chunk.IsEnd {
                break
            }
        }
        
        if !ctx.HasPendingToolCalls() {
            break
        }
    }
}
```

## 扩展模式

### 添加新工具

1. 实现 Tool 接口
2. 在 `core/tools/` 包中添加 `init()` 注册
3. 在 AgentSpec 中引用工具

详见 [扩展指南 - 自定义工具](../06-extending/custom-tool.md)

### 添加新 Hook

1. 实现 Hook 接口
2. 在 `core/hooks/` 包中添加 `init()` 注册
3. 在 AgentSpec 中引用 Hook

详见 [扩展指南 - 自定义 Hook](../06-extending/custom-hook.md)

### 添加新存储 Provider

1. 实现 Store 接口
2. 在 `core/providers/` 包中实现
3. 在 HarnessConfig.Store 中配置

详见 [核心库 - 自定义 Provider](../03-core-library/custom-provider.md)

## 部署架构

### 单体部署 (推荐中小规模)

```
┌─────────────────────────────────┐
│         Load Balancer           │
└─────────────────────────────────┘
                  │
        ┌─────────┼─────────┐
        ▼         ▼         ▼
    ┌───────┐ ┌───────┐ ┌───────┐
    │Server │ │Server │ │Server │
    │   1   │ │   2   │ │   3   │
    └───────┘ └───────┘ └───────┘
        │         │         │
        └─────────┼─────────┘
                  │
        ┌─────────┴─────────┐
        ▼                   ▼
   ┌──────────┐      ┌──────────┐
   │PostgreSQL│      │  Qdrant  │
   │ (主从)   │      │          │
   └──────────┘      └──────────┘
```

### 微服务部署 (大规模场景)

```
┌──────────────┐     ┌──────────────┐
│ API Gateway  │────▶│  Auth Service│
└──────────────┘     └──────────────┘
       │
       ├─────────────┐
       ▼             ▼
┌─────────────┐ ┌─────────────┐
│ CopCon Core │ │ CopCon Core │
│   (Agent)   │ │   (Agent)   │
└─────────────┘ └─────────────┘
       │             │
       ▼             ▼
┌─────────────────────────────┐
│  Message Queue (Kafka)      │
└─────────────────────────────┘
             │
   ┌─────────┼─────────┐
   ▼         ▼         ▼
┌──────┐ ┌──────┐ ┌──────┐
│ Post │ │Qdrant│ │Redis │
│ gres │ │      │ │Cache │
└──────┘ └──────┘ └──────┘
```

## 性能特性

| 特性 | 指标 | 说明 |
|------|------|------|
| 并发连接 | 10,000+ | 单个 server 实例 |
| 响应延迟 | <100ms | 首次 token 时间 |
| 吞吐量 | 1000 req/s | LLM 请求 |
| 内存占用 | ~200MB | 单实例基础占用 |

## 安全模型

### 认证

- JWT Token 验证
- API Key 支持
- OAuth 2.0 (可选)

### 授权

- 会话级别权限控制
- 工具调用权限
- 数据访问权限

### 数据安全

- 传输加密 (TLS)
- 存储加密 (数据库级别)
- 敏感信息脱敏

## 下一步

- [Harness 配置](harness.md)
- [能力系统](capabilities.md)
- [SSD 流式传输](streaming.md)
