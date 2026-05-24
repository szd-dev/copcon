# 多 Agent 协作

CopCon 支持定义多个 Agent,每个 Agent 有不同的能力和角色。本指南介绍如何配置和使用多 Agent 系统。

## 基本概念

### Agent 定义

每个 Agent 由以下属性定义:

```go
type AgentSpec struct {
    Name         string           // 唯一名称
    Model        string           // LLM 模型
    SystemPrompt string           // 系统提示词
    Tools        []string         // 可用工具列表
    Hooks        []string         // Hook 列表
    MaxTokens    int              // 最大输出 token 数
    Temperature  float64          // 温度参数 (0.0-2.0)
}
```

### 切换 Agent

在对话过程中可以通过指定 Agent 名称来切换:

```go
req := &core.Request{
    AgentName: "coder",    // 切换到 coder agent
    Content: "请实现这个功能",
}
harness.Chat(chatCtx, req)
```

## 配置示例

### 基础多 Agent

```go
cfg := &core.HarnessConfig{
    Agents: []core.AgentSpec{
        {
            Name:  "assistant",
            Model: "gpt-4",
            SystemPrompt: `你是一个通用的AI助手。
你可以帮助用户回答问题、提供建议、解释概念。
请用通俗易懂的语言回答。`,
            Tools: []string{"search"},
            Hooks: []string{"logging"},
        },
        {
            Name:  "coder",
            Model: "gpt-4",
            SystemPrompt: `你是一个专业的程序员。
你擅长编写、审查和优化代码。
你熟悉多种编程语言和最佳实践。`,
            Tools: []string{"code_executor", "file_ops"},
            Hooks: []string{"logging", "tracing"},
        },
        {
            Name:  "analyst",
            Model: "gpt-4",
            SystemPrompt: `你是一个数据分析师。
你擅长数据处理、统计分析和可视化。
你熟悉 Python、SQL 和数据分析工具。`,
            Tools: []string{"database_query", "code_executor", "search"},
            Hooks: []string{"logging"},
        },
    },
}

harness, err := core.NewHarnessWithStore(cfg, store)
```

### 层级 Agent

```go
cfg := &core.HarnessConfig{
    Agents: []core.AgentSpec{
        {
            Name:  "manager",
            Model: "gpt-4",
            SystemPrompt: `你是一个项目经理。
你的职责是:
1. 理解用户需求
2. 将任务分解为子任务
3. 分配给合适的专家 Agent
4. 整合各方结果

可用的专家:
- coder: 负责代码实现
- reviewer: 负责代码审查
- tester: 负责测试`,
            Tools: []string{"todo", "delegate_to_agent"},
            Hooks: []string{"logging", "todo_injection"},
        },
        {
            Name:  "coder",
            Model: "gpt-4",
            SystemPrompt: `你是一个高级程序员,负责实现代码功能。
请严格按照要求实现功能,不要假设额外需求。`,
            Tools: []string{"code_executor", "file_ops"},
            Hooks: []string{"logging"],
        },
        {
            Name:  "reviewer",
            Model: "gpt-4",
            SystemPrompt: `你是一个代码审查专家,负责审查代码质量。
请指出:
- 潜在的 bug
- 性能问题
- 安全漏洞
- 代码风格问题`,
            Tools: []string{"file_ops"],
            Hooks: []string{"logging"},
        },
    },
}
```

## 使用场景

### 场景 1: 按需切换

用户可以根据需要手动切换 Agent:

```go
// 使用 assistant
req := &core.Request{
    AgentName: "assistant",
    Content: "解释什么是 API?",
}
harness.Chat(chatCtx, req)

// 切换到 coder
req = &core.Request{
    AgentName: "coder",
    Content: "请实现一个 REST API 的示例",
}
harness.Chat(chatCtx, req)

// 切换到 analyst
req = &core.Request{
    AgentName: "analyst",
    Content: "分析这个 API 的调用日志",
}
harness.Chat(chatCtx, req)
```

### 场景 2: 自动路由

通过 Hook 实现智能路由:

```go
type RouterHook struct{}

func (h *RouterHook) Name() string { return "router" }

func (h *RouterHook) HookPoints() []core.HookPoint {
    return []core.HookPoint{core.BeforeLLMCall}
}

func (h *RouterHook) Execute(ctx context.Context, conv *core.Conversation) error {
    // 分析用户输入,决定使用哪个 Agent
    userInput := conv.LastUserMessage()
    
    agentName := "assistant"
    
    if strings.Contains(userInput, "代码") || strings.Contains(userInput, "实现") {
        agentName = "coder"
    } else if strings.Contains(userInput, "分析") || strings.Contains(userInput, "统计") {
        agentName = "analyst"
    }
    
    // 设置当前 Agent
    conv.Context["current_agent"] = agentName
    
    return nil
}

// 注册 Hook
cfg.Hooks = append(cfg.Hooks, "router")
```

### 场景 3: 任务委派

使用 `delegate_to_agent` 工具实现 Agent 之间的委派:

```go
// Manager Agent 可以委派任务给其他 Agent
managerRequest := &core.Request{
    AgentName: "manager",
    Content: `用户要求实现一个用户注册功能。
请:
1. 将这个任务分解为子任务 (使用 todo 工具)
2. 委派给合适的专家 (使用 delegate_to_agent 工具)
3. 整合结果`,
}

harness.Chat(chatCtx, managerRequest)

// Manager 内部会:
// 1. 创建 Todo: "设计 API"、"实现代码"、"编写测试"
// 2. 调用 delegate_to_agent(coder, "实现用户注册 API")
// 3. 调用 delegate_to_agent(reviewer, "审查代码")
// 4. 汇总结果返回
```

## 高级用法

### 共享状态

多个 Agent 可以共享对话状态:

```go
// 在会话级别设置上下文
store.UpdateSessionMetadata(ctx, sessionID, map[string]interface{}{
    "project": "user-management",
    "tech_stack": "Go + PostgreSQL",
})

// 所有 Agent 都可以访问这个上下文
for _, agent := range []string{"assistant", "coder", "analyst"} {
    req := &core.Request{
        AgentName: agent,
        Content: "请根据项目上下文回答问题",
    }
    harness.Chat(chatCtx, req)
}
```

### Agent 专用工具

为不同 Agent 配置专用工具:

```go
cfg := &core.HarnessConfig{
    Agents: []core.AgentSpec{
        {
            Name:  "researcher",
            Model: "gpt-4",
            Tools: []string{
                "search",
                "web_browse",
                "api_call",
                "database_query",
            },
        },
        {
            Name:  "writer",
            Model: "gpt-4",
            Tools: []string{
                "file_ops",
                "format_document",
                "translate",
            },
        },
        {
            Name:  "executor",
            Model: "gpt-4",
            Tools: []string{
                "code_executor",
                "shell_executor",
                "api_call",
            },
        },
    },
}
```

### 工具链协调

Agent 之间通过工具调用协调:

```go
// 定义工作流
workflow := []struct {
    Agent   string
    Task    string
}{
    {"researcher", "研究最新的技术趋势"},
    {"researcher", "收集数据并形成报告"},
    {"analyst", "分析数据,提取洞察"},
    {"writer", "撰写最终报告"},
}

for _, step := range workflow {
    req := &core.Request{
        AgentName: step.Agent,
        Content: step.Task,
    }
    
    if err := harness.Chat(chatCtx, req); err != nil {
        log.Printf("step failed: %v", err)
        break
    }
}
```

## 监控与调试

### 跟踪 Agent 执行

```go
// 启用详细日志
cfg.Log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

// 使用 tracing Hook
cfg.Agents[0].Hooks = append(cfg.Agents[0].Hooks, "tracing")
```

### 查看 Agent 历史

```go
// 获取 Agent 的对话历史
messages, err := store.GetMessages(ctx, sessionID)
if err != nil {
    return err
}

// 按 Agent 分组
agentMessages := make(map[string][]*storage.Message)
for _, msg := range messages {
    agentMessages[msg.AgentName] = append(agentMessages[msg.AgentName], msg)
}

// 分析每个 Agent 的使用情况
for agent, msgs := range agentMessages {
    fmt.Printf("Agent: %s, Messages: %d\n", agent, len(msgs))
}
```

## 最佳实践

### 1. 明确角色分工

```go
// ✅ 清晰的角色定义
{
    Name: "security-expert",
    SystemPrompt: `你是一个安全专家,专注于:
1. 识别安全漏洞
2. 提供安全建议
3. 审查安全最佳实践

不要处理非安全相关的问题。`,
    Tools: ["code_executor", "vulnerability_scanner"],
}

// ❌ 模糊的角色定义
{
    Name: "helper",
    SystemPrompt: "你可以帮助用户处理任何问题",
    Tools: ["code_executor", "file_ops", "search", "database_query"],
}
```

### 2. 限制权限

```go
// 为每个 Agent 只分配必要的工具
restrictedAgent := core.AgentSpec{
    Name: "readonly-analyst",
    SystemPrompt: "你只能查看和分析数据,不能修改。",
    Tools: []string{
        "database_query",  // 允许读取
        "search",          // 允许搜索
        // 不包含 code_executor 或 file_ops (写入工具)
    },
}
```

### 3. 设置输出限制

```go
verboseAgent := core.AgentSpec{
    Name: "researcher",
    SystemPrompt: "你是一个研究员",
    MaxTokens: 4000,      // 限制输出长度
    Temperature: 0.7,     // 平衡创意和准确性
}
```

### 4. 错误隔离

```go
// 每个 Agent 独立错误处理
for _, agentName := range []string{"coder", "reviewer", "tester"} {
    req := &core.Request{
        AgentName: agentName,
        Content: "请执行任务",
    }
    
    if err := harness.Chat(chatCtx, req); err != nil {
        log.Printf("Agent %s failed: %v", agentName, err)
        // 继续执行其他 Agent
        continue
    }
}
```

## 示例: 完整的多 Agent 工作流

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/copcon/core"
    "github.com/copcon/core/storage"
)

func main() {
    // 配置多 Agent
    cfg := &core.HarnessConfig{
        Agents: []core.AgentSpec{
            {
                Name:  "planner",
                Model: "gpt-4",
                SystemPrompt: `你是项目规划师。
你的任务是:
1. 理解用户需求
2. 制定实施计划
3. 分配任务给专家
4. 监控进度`,
                Tools: []string{"todo", "search"],
                Hooks: []string{"logging", "todo_injection"},
            },
            {
                Name:  "architect",
                Model: "gpt-4",
                SystemPrompt: `你是系统架构师。
你负责:
- 设计系统架构
- 选择技术方案
- 制定技术规范`,
                Tools: []string{"file_ops", "search"],
                Hooks: []string{"logging"},
            },
            {
                Name:  "developer",
                Model: "gpt-4",
                SystemPrompt: `你是开发工程师。
你负责:
- 实现功能代码
- 编写单元测试
- 修复 bug`,
                Tools: []string{"code_executor", "file_ops"],
                Hooks: []string{"logging", "tracing"],
            },
            {
                Name:  "reviewer",
                Model: "gpt-4",
                SystemPrompt: `你是代码审查专家。
你负责:
- 审查代码质量
- 提出改进建议
- 确保最佳实践`,
                Tools: []string{"file_ops"],
                Hooks: []string{"logging"},
            },
        },
    }
    
    // 初始化
    store := storage.NewMemoryStore()
    harness, err := core.NewHarnessWithStore(cfg, store)
    if err != nil {
        panic(err)
    }
    
    ctx := context.Background()
    sessionID := "project-123"
    
    // 创建会话
    store.CreateSession(ctx, sessionID)
    
    chatCtx := harness.NewChatContext(ctx, sessionID)
    
    // 工作流
    steps := []struct {
        Agent   string
        Task    string
    }{
        {
            Agent: "planner",
            Task: `用户需求: 开发一个待办事项 API
请制定实施计划并分配任务。`,
        },
        {
            Agent: "architect",
            Task: `请为待办事项 API 设计架构:
- 选择框架和数据库
- 设计 API endpoints
- 定义数据模型`,
        },
        {
            Agent: "developer",
            Task: `请实现待办事项 API:
- 创建项目结构
- 实现 CRUD endpoints
- 编写单元测试`,
        },
        {
            Agent: "reviewer",
            Task: `请审查开发的代码:
- 代码质量
- 性能
- 安全性
- 测试覆盖率`,
        },
    }
    
    for _, step := range steps {
        fmt.Printf("\n=== Agent: %s ===\n", step.Agent)
        
        req := &core.Request{
            Content: step.Task,
            AgentName: step.Agent,
        }
        
        if err := harness.Chat(chatCtx, req); err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }
        
        // 读取响应
        for event := range chatCtx.Events() {
            if event.Type == "message" {
                fmt.Print(event.Data)
            }
        }
        fmt.Println()
    }
    
    // 获取最终状态
    todos, _ := store.GetTodos(ctx, sessionID)
    fmt.Printf("\n=== 完成的任务 (%d) ===\n", len(todos))
    for _, todo := range todos {
        fmt.Printf("- [%s] %s\n", todo.Status, todo.Content)
    }
}
```

## 常见问题

### Q: 如何在 Agent 之间共享变量?

A: 使用 Session 级别的 Metadata:
```go
store.UpdateSessionMetadata(ctx, sessionID, map[string]interface{}{
    "shared_var": "value",
})
```

### Q: Agent A 可以调用 Agent B 吗?

A: 使用 `delegate_to_agent` 工具,或在 Hook 中实现 Agent 调用。

### Q: 如何处理 Agent 之间的循环依赖?

A: 明确定义执行顺序,避免循环。使用状态机或工作流引擎控制。

## 下一步

- [配置指南](configuration.md)
- [自定义工具](../06-extending/custom-tool.md)
- [自定义 Hook](../06-extending/custom-hook.md)
