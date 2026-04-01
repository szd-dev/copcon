# ChatContext 统一上下文重构计划（完整范围）

## TL;DR

> **Quick Summary**: 全面重构请求处理流程，引入 ChatContext 作为统一上下文。所有 Manager 和 Tool 接口从 `(ctx, sessionID, ...)` 改为 `(chatCtx, ...)`，消除 sessionID 字符串散落传递问题。
> 
> **Deliverables**:
> - ChatContext 结构（精简版，只含会话信息）
> - 所有 Manager 接口变更
> - Tool 接口变更
> - AgentEngine 重构
> - HTTP Handler 入口创建 ChatContext
> 
> **Estimated Effort**: Large
> **Breaking Changes**: YES (所有 Manager 接口 + Tool 接口)

---

## Context

### 核心理念

**Manager 是无状态全局服务**，不需要放在 ChatContext 中引用。

**重点是**：调用 Manager 方法时传入 ChatContext，而不是传入 sessionID 字符串。

### 当前问题

```go
// sessionID 作为字符串到处传递
sessionMgr.Get(ctx, sessionID)
contextMgr.GetHistory(ctx, sessionID, limit)
contextMgr.AddMessage(ctx, sessionID, msg)
memoryMgr.GetBySession(ctx, sessionID, limit)
todoMgr.List(ctx, sessionID)
todoMgr.Create(ctx, sessionID, content)

// Tool 也需要 sessionID
tool.Execute(ctx, args)  // args 里包含 session_id
```

### 目标

```go
// 传入 ChatContext，manager 内部提取 sessionID
sessionMgr.Get(chatCtx)
contextMgr.GetHistory(chatCtx, limit)
contextMgr.AddMessage(chatCtx, msg)
memoryMgr.GetBySession(chatCtx, limit)
todoMgr.List(chatCtx)
todoMgr.Create(chatCtx, content)

// Tool 也使用 ChatContext
tool.Execute(chatCtx, args)  // args 不再需要 session_id
```

---

## Design Details

### 1. ChatContext 结构（精简版）

**位置**：`server/internal/context/chat.go`

```go
package context

import (
    "context"
)

// ChatContext 封装单次 Chat 请求的上下文信息
// 不持有 manager 引用（manager 是无状态全局服务）
type ChatContext struct {
    // 基础上下文
    ctx context.Context
    
    // 会话标识
    sessionID string
    agentID   string
    
    // 输出通道（Agent 专用）
    events chan Event
    
    // 可扩展字段
    // userID   string
    // metadata map[string]any
}

// --- Getter 方法 ---
func (c *ChatContext) Context() context.Context { return c.ctx }
func (c *ChatContext) SessionID() string        { return c.sessionID }
func (c *ChatContext) AgentID() string          { return c.agentID }
func (c *ChatContext) Events() chan<- Event     { return c.events }

// --- 辅助方法 ---
func (c *ChatContext) Emit(event Event) {
    select {
    case c.events <- event:
    case <-c.ctx.Done():
    }
}

// --- 构造函数 ---
func NewChatContext(ctx context.Context, sessionID, agentID string) *ChatContext {
    return &ChatContext{
        ctx:       ctx,
        sessionID: sessionID,
        agentID:   agentID,
        events:    make(chan Event, 100),
    }
}

// WithEvents 设置自定义 events channel
func (c *ChatContext) WithEvents(events chan Event) *ChatContext {
    c.events = events
    return c
}
```

### 2. Event 类型定义

**位置**：`server/internal/context/event.go`

```go
package context

type EventType string

const (
    EventMessage    EventType = "message"
    EventReasoning  EventType = "reasoning"
    EventToolCall   EventType = "tool_call"
    EventToolResult EventType = "tool_result"
    EventThought    EventType = "thought"
    EventDone       EventType = "done"
    EventError      EventType = "error"
)

type Event struct {
    Type EventType `json:"type"`
    Data any       `json:"data"`
}

type MessageData struct {
    Content string `json:"content"`
}

type ReasoningData struct {
    Content string `json:"content"`
}

type ToolCallData struct {
    ToolName string         `json:"tool_name"`
    Args     map[string]any `json:"args"`
    ID       string         `json:"id"`
}

type ToolResultData struct {
    ToolName string `json:"tool_name"`
    Result   any    `json:"result"`
    ID       string `json:"id"`
}

type DoneData struct {
    MessageID string `json:"message_id"`
}

type ErrorData struct {
    Error string `json:"error"`
}
```

### 3. Manager 接口变更

#### 3.1 SessionManager

**Before**:
```go
type SessionManager interface {
    Create(ctx context.Context, title string, defaultAgentID string) (*Session, error)
    Get(ctx context.Context, id string) (*Session, error)
    List(ctx context.Context, limit int, offset int) ([]*Session, int, error)
    Delete(ctx context.Context, id string) error
    UpdateTitle(ctx context.Context, id string, title string) error
    GetMessageCount(ctx context.Context, sessionID string) (int, error)
    GetDB() *gorm.DB
}
```

**After**:
```go
type SessionManager interface {
    Create(chatCtx *context.ChatContext, title string, defaultAgentID string) (*Session, error)
    Get(chatCtx *context.ChatContext) (*Session, error)  // sessionID 从 chatCtx 获取
    List(chatCtx *context.ChatContext, limit int, offset int) ([]*Session, int, error)
    Delete(chatCtx *context.ChatContext) error
    UpdateTitle(chatCtx *context.ChatContext, title string) error
    GetMessageCount(chatCtx *context.ChatContext) (int, error)
    GetDB() *gorm.DB
}
```

#### 3.2 ContextManager

**Before**:
```go
type ContextManager interface {
    GetHistory(ctx context.Context, sessionID string, limit int) ([]Message, error)
    AddMessage(ctx context.Context, sessionID string, msg *Message) error
    BuildContext(ctx context.Context, sessionID string, userInput string, maxTokens int, systemPrompt string) ([]MessageForLLM, error)
    DeleteBySession(ctx context.Context, sessionID string) error
}
```

**After**:
```go
type ContextManager interface {
    GetHistory(chatCtx *context.ChatContext, limit int) ([]Message, error)
    AddMessage(chatCtx *context.ChatContext, msg *Message) error
    BuildContext(chatCtx *context.ChatContext, userInput string, maxTokens int, systemPrompt string) ([]MessageForLLM, error)
    DeleteBySession(chatCtx *context.ChatContext) error
}
```

#### 3.3 MemoryManager

**Before**:
```go
type MemoryManager interface {
    Store(ctx context.Context, memory *Memory) error
    Search(ctx context.Context, query []float32, limit int, sessionID string) ([]*Memory, error)
    GetBySession(ctx context.Context, sessionID string, limit int) ([]*Memory, error)
    DeleteBySession(ctx context.Context, sessionID string) error
}
```

**After**:
```go
type MemoryManager interface {
    Store(chatCtx *context.ChatContext, memory *Memory) error
    Search(chatCtx *context.ChatContext, query []float32, limit int) ([]*Memory, error)
    GetBySession(chatCtx *context.ChatContext, limit int) ([]*Memory, error)
    DeleteBySession(chatCtx *context.ChatContext) error
}
```

#### 3.4 TodoManager

**Before**:
```go
type TodoManager interface {
    Create(ctx context.Context, sessionID string, content string, opts ...TodoOption) (*Todo, error)
    Get(ctx context.Context, id string) (*Todo, error)
    List(ctx context.Context, sessionID string) ([]*Todo, error)
    Update(ctx context.Context, id string, updates map[string]any) (*Todo, error)
    Delete(ctx context.Context, id string) error
    Start(ctx context.Context, id string) (*Todo, error)
    Complete(ctx context.Context, id string, result string) (*Todo, error)
    Fail(ctx context.Context, id string, reason string) (*Todo, error)
    Block(ctx context.Context, id string, reason string) (*Todo, error)
    Unblock(ctx context.Context, id string) (*Todo, error)
    GetAvailableTodos(ctx context.Context, sessionID string) ([]*Todo, error)
    GetDB() *gorm.DB
}
```

**After**:
```go
type TodoManager interface {
    Create(chatCtx *context.ChatContext, content string, opts ...TodoOption) (*Todo, error)
    Get(chatCtx *context.ChatContext) (*Todo, error)  // 从 chatCtx 获取 todoID？不，这个需要 id 参数
    List(chatCtx *context.ChatContext) ([]*Todo, error)
    Update(chatCtx *context.ChatContext, id string, updates map[string]any) (*Todo, error)
    Delete(chatCtx *context.ChatContext, id string) error
    Start(chatCtx *context.ChatContext, id string) (*Todo, error)
    Complete(chatCtx *context.ChatContext, id string, result string) (*Todo, error)
    Fail(chatCtx *context.ChatContext, id string, reason string) (*Todo, error)
    Block(chatCtx *context.ChatContext, id string, reason string) (*Todo, error)
    Unblock(chatCtx *context.ChatContext, id string) (*Todo, error)
    GetAvailableTodos(chatCtx *context.ChatContext) ([]*Todo, error)
    GetDB() *gorm.DB
}
```

**注意**：`Get`、`Update`、`Delete`、`Start`、`Complete`、`Fail`、`Block`、`Unblock` 这些方法需要 todo ID 参数，但 sessionID 从 chatCtx 获取。

### 4. Tool 接口变更

**Before**:
```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(ctx context.Context, args map[string]any) (*ToolResult, error)
}
```

**After**:
```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(chatCtx *context.ChatContext, args map[string]any) (*ToolResult, error)
}
```

### 5. AgentEngine 变更

**Before**:
```go
func (e *AgentEngine) Chat(ctx context.Context, sessionID string, agentID string, userInput string) (<-chan Event, error)
```

**After**:
```go
func (e *AgentEngine) Chat(chatCtx *context.ChatContext, userInput string) error
// 调用方通过 chatCtx.Events() 获取事件流
```

### 6. HTTP Handler 变更

**Before**:
```go
func (h *Handler) Chat(c *gin.Context) {
    sessionID := c.Param("sessionId")
    events, err := h.agent.Chat(ctx, sessionID, agentID, content)
    // ...
}
```

**After**:
```go
func (h *Handler) Chat(c *gin.Context) {
    chatCtx := context.NewChatContext(
        c.Request.Context(),
        c.Param("sessionId"),
        req.AgentID,
    )
    
    go h.agent.Chat(chatCtx, req.Content)
    
    // SSE 流式返回
    for event := range chatCtx.Events() {
        // ...
    }
}
```

---

## Execution Strategy

### Phase 1: 创建 ChatContext（无破坏性）

| 步骤 | 文件 | 操作 |
|------|------|------|
| 1.1 | `server/internal/context/chat.go` | 新建 ChatContext 结构 |
| 1.2 | `server/internal/context/event.go` | 新建 Event 类型 |

### Phase 2: 修改 Manager 接口

| 步骤 | 文件 | 操作 |
|------|------|------|
| 2.1 | `server/internal/session/manager.go` | 修改 SessionManager 接口 |
| 2.2 | `server/internal/session/manager.go` | 修改实现 |
| 2.3 | `server/internal/context/manager.go` | 修改 ContextManager 接口 |
| 2.4 | `server/internal/context/manager.go` | 修改实现 |
| 2.5 | `server/internal/memory/manager.go` | 修改 MemoryManager 接口 |
| 2.6 | `server/internal/memory/manager.go` | 修改实现 |
| 2.7 | `server/internal/todo/manager.go` | 修改 TodoManager 接口 |
| 2.8 | `server/internal/todo/manager.go` | 修改实现 |

### Phase 3: 修改 Tool 接口

| 步骤 | 文件 | 操作 |
|------|------|------|
| 3.1 | `server/internal/tool/manager.go` | 修改 Tool 接口 |
| 3.2 | `server/internal/tools/code_tool.go` | 适配新签名 |
| 3.3 | `server/internal/tools/shell_tool.go` | 适配新签名 |
| 3.4 | `server/internal/tools/file_tool.go` | 适配新签名 |
| 3.5 | `server/internal/tools/todo_tool.go` | 适配新签名 + 移除 session_id |

### Phase 4: 重构 AgentEngine

| 步骤 | 文件 | 操作 |
|------|------|------|
| 4.1 | `server/internal/agent/engine.go` | 重构 Chat() 签名 |
| 4.2 | `server/internal/agent/engine.go` | 重构所有内部方法 |

### Phase 5: 重构 HTTP Handler

| 步骤 | 文件 | 操作 |
|------|------|------|
| 5.1 | `server/internal/api/handlers.go` | 所有 handler 使用 ChatContext |

### Phase 6: 更新依赖注入

| 步骤 | 文件 | 操作 |
|------|------|------|
| 6.1 | `server/cmd/server/main.go` | 更新初始化 |

### Phase 7: 更新测试

| 步骤 | 文件 | 操作 |
|------|------|------|
| 7.1 | `server/internal/session/manager_test.go` | 适配新接口 |
| 7.2 | `server/internal/todo/manager_test.go` | 适配新接口 |
| 7.3 | 其他测试文件 | 适配新接口 |

### Phase 8: 验证

| 步骤 | 操作 |
|------|------|
| 8.1 | `cd server && go build ./...` |
| 8.2 | `cd server && go test ./...` |

---

## File Changes Summary

| 文件 | 操作 | 变更类型 |
|------|------|---------|
| `server/internal/context/chat.go` | **新建** | 核心结构 |
| `server/internal/context/event.go` | **新建** | Event 类型 |
| `server/internal/session/manager.go` | **修改** | 接口 + 实现 |
| `server/internal/context/manager.go` | **修改** | 接口 + 实现 |
| `server/internal/memory/manager.go` | **修改** | 接口 + 实现 |
| `server/internal/todo/manager.go` | **修改** | 接口 + 实现 |
| `server/internal/tool/manager.go` | **修改** | Tool 接口 |
| `server/internal/tools/*.go` | **修改** | 所有 Tool 实现 |
| `server/internal/agent/engine.go` | **重构** | Agent 引擎 |
| `server/internal/api/handlers.go` | **修改** | HTTP 入口 |
| `server/cmd/server/main.go` | **修改** | 依赖注入 |
| `server/internal/session/manager_test.go` | **修改** | 测试 |
| `server/internal/todo/manager_test.go` | **修改** | 测试 |

---

## Success Criteria

- [ ] ChatContext 结构创建完成
- [ ] 所有 Manager 接口变更完成
- [ ] 所有 Manager 实现变更完成
- [ ] Tool 接口变更完成
- [ ] TodoTool 不再要求 session_id 参数
- [ ] AgentEngine 使用 ChatContext
- [ ] HTTP Handler 入口创建 ChatContext
- [ ] 编译通过：`go build ./...`
- [ ] 测试通过：`go test ./...`