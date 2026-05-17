# Hello World：第一个 Agent 程序

这篇指南带你从头写一个可运行的 Agent 程序。不需要数据库，不需要 API Key，所有依赖都用 mock 实现。

完成之后你会理解：

- Agent Engine 的创建和依赖注入
- 如何写一个 Hook 拦截引擎事件
- LLM Provider 的作用
- SSE 事件的消费方式

## 完整代码

把下面的代码保存到 `server/cmd/hello/main.go`：

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/copcon/server/internal/agent"
	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/hook"
	"github.com/copcon/server/internal/llm"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
)

// ============================================================
// 第一步：写一个简单的 LLM Provider（回显 Hello World）
// ============================================================

// EchoProvider 是一个自定义 LLM Provider。
// 它不调任何真实 API，直接模拟流式返回一段文本。
type EchoProvider struct{}

func (p *EchoProvider) Stream(ctx context.Context, params llm.StreamParams) (<-chan llm.StreamChunk, <-chan error) {
	ch := make(chan llm.StreamChunk)
	errc := make(chan error)

	go func() {
		defer close(ch)
		defer close(errc)

		// 模拟返回 "你好，我是 CopCon Agent！"
		text := "你好，我是 CopCon Agent！"
		for _, r := range text {
			select {
			case ch <- llm.StreamChunk{Content: string(r)}:
			case <-ctx.Done():
				return
			}
			time.Sleep(30 * time.Millisecond) // 模拟网络延迟
		}
	}()

	return ch, errc
}

// ============================================================
// 第二步：写一个 EchoHook（在每个生命周期节点打印日志）
// ============================================================

// EchoHook 是一个自定义 Hook。
// 它在引擎生命周期的关键节点执行，打印一条日志。
type EchoHook struct{}

func (h *EchoHook) Name() string {
	return "echo-hook"
}

// Points 返回这个 Hook 要拦截的生命周期节点
func (h *EchoHook) Points() []hook.HookPoint {
	return []hook.HookPoint{
		hook.BeforeContextBuild,
		hook.AfterContextBuild,
		hook.BeforeLLMCall,
		hook.AfterLLMCall,
	}
}

func (h *EchoHook) Priority() int {
	return 100 // 默认优先级
}

func (h *EchoHook) Execute(ctx *hook.HookContext) error {
	fmt.Printf("[%s] Hook 触发: session=%s, agent=%s\n",
		ctx.CurrentPoint, ctx.SessionID, ctx.AgentID)
	return nil
}

// ============================================================
// 第三步：写 mock 依赖（SessionManager、ContextManager）
// ============================================================

// mockSessionManager 是 SessionManager 的简单实现。
// 内存存储，不需要数据库。
type mockSessionManager struct {
	sessions map[string]*session.Session
}

func newMockSessionManager() *mockSessionManager {
	return &mockSessionManager{sessions: make(map[string]*session.Session)}
}

func (m *mockSessionManager) Create(chatCtx iface.ChatContextInterface, title, defaultAgentID string) (*session.Session, error) {
	s := &session.Session{
		ID:             uuid.New(),
		Title:          title,
		DefaultAgentID: defaultAgentID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       make(map[string]any),
	}
	m.sessions[s.ID.String()] = s
	return s, nil
}

func (m *mockSessionManager) Get(chatCtx iface.ChatContextInterface) (*session.Session, error) {
	s, ok := m.sessions[chatCtx.SessionID()]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	return s, nil
}

func (m *mockSessionManager) List(chatCtx iface.ChatContextInterface, limit, offset int) ([]*session.Session, int64, error) {
	return nil, 0, nil
}

func (m *mockSessionManager) Delete(chatCtx iface.ChatContextInterface) error {
	delete(m.sessions, chatCtx.SessionID())
	return nil
}

func (m *mockSessionManager) UpdateTitle(chatCtx iface.ChatContextInterface, title string) error {
	return nil
}

func (m *mockSessionManager) UpdateMetadata(chatCtx iface.ChatContextInterface, metadata map[string]any) error {
	return nil
}

func (m *mockSessionManager) AddAsyncCompletionPending(chatCtx iface.ChatContextInterface, event map[string]any) error {
	return nil
}

func (m *mockSessionManager) GetMessageCount(chatCtx iface.ChatContextInterface) (int64, error) {
	return 0, nil
}

func (m *mockSessionManager) GetDB() interface{} { return nil }

// mockContextManager 是 ContextManager 的简单实现。
type mockContextManager struct{}

func (m *mockContextManager) GetHistory(chatCtx iface.ChatContextInterface, limit int) ([]session.Message, error) {
	return nil, nil
}

func (m *mockContextManager) AddMessage(chatCtx iface.ChatContextInterface, msg *session.Message) error {
	return nil
}

func (m *mockContextManager) BuildContext(chatCtx iface.ChatContextInterface, userInput string, maxTokens int, systemPrompt string) ([]entity.MessageForLLM, error) {
	return nil, nil
}

func (m *mockContextManager) DeleteBySession(chatCtx iface.ChatContextInterface) error {
	return nil
}

// mockAgentRegistry 创建一个只包含一个 Echo Agent 的 Registry。
func newMockAgentRegistry() agent.AgentRegistry {
	// 这里简化处理：直接用测试辅助函数创建
	return &mockRegistry{provider: &EchoProvider{}}
}

type mockRegistry struct {
	provider llm.LLMProvider
}

func (r *mockRegistry) Get(id string) (agent.AgentDefinition, error) {
	return agent.AgentDefinition{
		ID:           "echo-agent",
		Name:         "Echo Agent",
		Model:        "echo-v1",
		SystemPrompt: "你是一个回显助手。",
		ToolManager:  tool.NewToolManager(),
		LLMProvider:  r.provider,
	}, nil
}

func (r *mockRegistry) List() []agent.AgentInfo {
	return []agent.AgentInfo{{ID: "echo-agent", Name: "Echo Agent", Model: "echo-v1"}}
}

func (r *mockRegistry) Default() (agent.AgentDefinition, error) {
	return r.Get("echo-agent")
}

// ============================================================
// 第四步：组装并运行
// ============================================================

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// 1. 创建依赖
	sessionMgr := newMockSessionManager()
	ctxMgr := new(mockContextManager)
	agentRegistry := newMockAgentRegistry()
	asyncRegistry := tool.NewAsyncToolRegistry()

	// 2. 创建 Hook Runner 并注册 EchoHook
	hookRunner := hook.NewHookRunner()
	hookRunner.Register(&EchoHook{})

	// 3. 创建 Agent Engine（依赖注入）
	engine := agent.NewAgentEngine(
		agentRegistry,
		sessionMgr,
		ctxMgr,
		asyncRegistry,
		agent.WithLogger(logger),
		agent.WithHookRunner(hookRunner),
	)

	// 4. 先创建一个会话
	createCtx := iface.NewChatContext(context.Background(), "", "echo-agent")
	sess, err := sessionMgr.Create(createCtx, "Hello Session", "echo-agent")
	if err != nil {
		panic(err)
	}
	createCtx.Close()

	// 5. 创建 ChatContext 并调用 Chat
	chatCtx := iface.NewChatContext(context.Background(), sess.ID.String(), "echo-agent")

	var wg sync.WaitGroup
	wg.Add(1)

	// 启动 Consumer：读取 SSE 事件
	go func() {
		defer wg.Done()
		for event := range chatCtx.Events() {
			data, _ := json.MarshalIndent(event, "  ", "  ")
			fmt.Printf("SSE 事件:\n%s\n\n", string(data))
		}
	}()

	// 6. 发送消息
	fmt.Println("=== 开始 Chat ===")
	if err := engine.Chat(chatCtx, "你好"); err != nil {
		fmt.Printf("Chat 错误: %v\n", err)
	}

	// 等待 Consumer 处理完所有事件
	wg.Wait()
	fmt.Println("=== Chat 结束 ===")
}
```

## 运行

```bash
cd server
go run ./cmd/hello
```

## 预期输出

```
=== 开始 Chat ===
[before_context_build] Hook 触发: session=xxx-xxx, agent=echo-agent
[after_context_build] Hook 触发: session=xxx-xxx, agent=echo-agent
[before_llm_call] Hook 触发: session=xxx-xxx, agent=echo-agent

SSE 事件:
{
  "type": "step_create",
  "data": {
    "messageId": "xxx",
    "stepIndex": 0
  }
}

SSE 事件:
{
  "type": "part_create",
  "data": {
    "messageId": "xxx",
    "stepIndex": 0,
    "partIndex": 0,
    "partType": "text",
    "state": "streaming"
  }
}

SSE 事件:
{
  "type": "part_update",
  "data": {
    "messageId": "xxx",
    "stepIndex": 0,
    "partIndex": 0,
    "partType": "text",
    "textDelta": "你"
  }
}

... (每个字一个 part_update 事件) ...

[after_llm_call] Hook 触发: session=xxx-xxx, agent=echo-agent

SSE 事件:
{
  "type": "part_update",
  "data": {
    "messageId": "xxx",
    "stepIndex": 0,
    "partIndex": 0,
    "partType": "text",
    "state": "done"
  }
}

SSE 事件:
{
  "type": "message_done",
  "data": {
    "messageId": "xxx"
  }
}

=== Chat 结束 ===
```

## 代码解读

### LLM Provider

`LLMProvider` 接口只有一个方法：

```go
Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error)
```

你把消息发过去，它返回两个 channel：一个流式吐 `StreamChunk`，一个在出错时吐 error。CopCon 引擎已经完全解耦了 LLM 调用，你只需要实现这个接口就能接入任何模型，不管是 OpenAI、Claude 还是本地模型。

### Hook 系统

Hook 是 CopCon 的"插件机制"。每个 Hook 实现三个方法：

- `Name()`：一个人类可读的名字，用于日志
- `Points()`：返回要拦截的生命周期节点列表
- `Execute(ctx *HookContext)`：在节点到达时被调用

可用的生命周期节点（全部定义在 `hook.HookPoint`）：

| 节点 | 触发时机 |
|---|---|
| `OnSessionResolve` | 会话解析时 |
| `OnSystemPrompt` | 系统提示词解析时 |
| `BeforeContextBuild` | 组装上下文之前 |
| `AfterContextBuild` | 组装上下文之后 |
| `BeforeLLMCall` | LLM 请求发出前 |
| `AfterLLMCall` | LLM 响应收到后 |
| `BeforeToolExecute` | 工具执行前 |
| `AfterToolExecute` | 工具执行后 |
| `OnToolError` | 工具执行出错时 |
| `OnMessagePersist` | 消息持久化时 |

`HookContext` 中携带了各节点相关的上下文信息（SessionID、AgentID、SystemPrompt、Messages 等），Hook 可以通过修改指针字段来影响下游行为。

### SSE 事件流

所有事件都定义在 `entity.Event` 中，主要类型：

| 事件类型 | 含义 |
|---|---|
| `step_create` | Agent 循环开始新一轮（工具调用后会再开 step） |
| `part_create` | 创建新的内容片段 |
| `part_update` | 更新片段内容（流式增量文本、状态变更） |
| `message_done` | 消息处理完成 |

一个典型的流式响应流程：

```
step_create → part_create(text, streaming)
           → part_update × N (流式文本)
           → part_update(state: done)
           → message_done
```

### 依赖注入

`agent.NewAgentEngine` 接收必需的依赖和可变配置选项：

```go
engine := agent.NewAgentEngine(
    registry,           // Agent 注册表
    sessionMgr,         // 会话管理
    ctxMgr,             // 上下文管理
    asyncRegistry,      // 异步工具追踪
    agent.WithLogger(logger),
    agent.WithHookRunner(hookRunner),
    agent.WithConcurrency(10),
    agent.WithLLMProvider(customProvider),
)
```

这种方式让你可以在生产环境用真实的 PostgreSQL + Qdrant 依赖，在测试中用 mock 实现，不需要改任何核心代码。

## 下一步

- [快速开始](quickstart.md) — 用真实的 LLM API 跑完整服务
- [安装指南](installation.md) — 配置真实数据库和模型