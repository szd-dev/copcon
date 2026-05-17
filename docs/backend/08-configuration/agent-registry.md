# Agent 注册表

`AgentRegistry` 是 CopCon 的多 Agent 管理核心。它从配置创建 Agent 定义，提供按 ID 查找和默认 Agent 回退的能力。每个 Agent 拥有独立的 LLM Provider、系统提示词和工具集。

**文件位置**: `server/internal/agent/registry.go`

## AgentDefinition 结构

```go
type AgentDefinition struct {
    ID           string
    Name         string
    Model        string
    SystemPrompt string
    ToolManager  tool.ToolManager
    LLMProvider  llm.LLMProvider
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `ID` | `string` | Agent 唯一标识，对应请求中的 `agent_id` |
| `Name` | `string` | 展示名称，用于 UI 和日志 |
| `Model` | `string` | 此 Agent 使用的 LLM 模型名 |
| `SystemPrompt` | `string` | 系统提示词，定义 Agent 的行为和角色 |
| `ToolManager` | `tool.ToolManager` | 此 Agent 的专用工具管理器，仅包含配置中指定的工具 |
| `LLMProvider` | `llm.LLMProvider` | OpenAI 兼容的 LLM 客户端适配器 |

注意：`ToolManager` 和 `LLMProvider` 是已初始化的运行时组件，不是原始配置值。每个 Agent 的 `ToolManager` 只注册了该 Agent 配置中指定的工具子集。

## AgentRegistry 接口

```go
type AgentRegistry interface {
    Get(id string) (AgentDefinition, error)
    List() []AgentInfo
    Default() (AgentDefinition, error)
}
```

| 方法 | 说明 | 错误 |
|------|------|------|
| `Get(id)` | 按 ID 查找 Agent | `ErrAgentNotFound` |
| `List()` | 列出所有 Agent 的摘要信息 | 无 |
| `Default()` | 返回默认 Agent | `ErrNoDefaultAgent` |

### 错误常量

```go
var (
    ErrAgentNotFound  = errors.New("agent not found")
    ErrNoDefaultAgent = errors.New("no default agent configured")
)
```

## NewAgentRegistry：创建注册表

```go
func NewAgentRegistry(cfg *config.Config, toolRegistry tool.ToolRegistry) (AgentRegistry, error)
```

创建过程为每个配置中的 Agent 执行以下步骤：

### 1. 验证工具存在性

```go
for _, toolName := range agentConfig.Tools {
    if _, err := toolRegistry.Get(toolName); err != nil {
        return nil, fmt.Errorf("agent %s: tool not found: %s", agentConfig.ID, toolName)
    }
}
```

所有工具名称必须在 `ToolRegistry` 中已注册。验证失败直接返回错误，不再继续。

### 2. 创建专用 ToolManager

```go
toolMgr := tool.NewToolManager()
for _, toolName := range agentConfig.Tools {
    t, _ := toolRegistry.Get(toolName)
    toolMgr.Register(t)
}
```

每个 Agent 拥有独立的 `ToolManager` 实例，只包含配置中指定的工具子集。这确保 Agent 不能调用未授权的工具。

### 3. 创建 LLM Provider

```go
opts := []option.RequestOption{
    option.WithAPIKey(cfg.OpenAI.APIKey),
}
baseURL := agentConfig.BaseURL
if baseURL == "" {
    baseURL = cfg.OpenAI.BaseURL
}
if baseURL != "" {
    opts = append(opts, option.WithBaseURL(baseURL))
}
client := openai.NewClient(opts...)
provider := llm.NewOpenAIAdapter(&client, agentConfig.Model)
```

每个 Agent 创建独立的 OpenAI client。如果 Agent 配置中指定了 `base_url`，则使用该地址覆盖全局配置。这使得不同 Agent 可以连接不同的 LLM 后端。

### 4. 组装 AgentDefinition

```go
agent := AgentDefinition{
    ID:           agentConfig.ID,
    Name:         agentConfig.Name,
    Model:        agentConfig.Model,
    SystemPrompt: agentConfig.SystemPrompt,
    ToolManager:  toolMgr,
    LLMProvider:  provider,
}
registry.agents[agentConfig.ID] = agent
```

所有注册的 Agent 存储在 `map[string]AgentDefinition` 中，以 ID 为键。

### 5. 存储默认 Agent

```go
registry.defaultAgent = cfg.DefaultAgentID
```

## Agent 解析链

引擎在 `Chat` 方法中按以下优先级解析 Agent：

```
chatCtx.AgentID()
    │
    ├─ 非空 → AgentRegistry.Get(agentID)
    │          └─ 找到 → 使用该 AgentDefinition
    │          └─ 未找到 → 返回错误
    │
    └─ 空 → 从 Session 获取 DefaultAgentID
             │
             ├─ 非空 → AgentRegistry.Get(session.DefaultAgentID)
             │
             └─ 空 → AgentRegistry.Default()
                      ├─ 存在 → 使用默认 Agent
                      └─ 不存在 → 返回 ErrNoDefaultAgent
```

此三级回退确保：
1. 请求可以显式指定 Agent
2. Session 可以绑定默认 Agent
3. 全局默认 Agent 作为最终回退

## AgentInfo 摘要

```go
type AgentInfo struct {
    ID    string
    Name  string
    Model string
}
```

`List()` 返回所有 Agent 的摘要信息，用于 UI 展示可用的 Agent 列表。不暴露 `ToolManager` 和 `LLMProvider` 等运行时组件。

## 并发安全

```go
type agentRegistry struct {
    mu           sync.RWMutex
    agents       map[string]AgentDefinition
    defaultAgent string
}
```

所有读操作（`Get`, `List`, `Default`）使用 `RLock`，支持并发读取。Agent 列表在初始化时通过 `NewAgentRegistry` 一次性构建，之后不再变更，因此无需写锁。

## 多 Agent 路由示例

### 配置定义

```yaml
default_agent_id: "code-assistant"

agents:
  - id: "code-assistant"
    name: "Code Assistant"
    model: "gpt-4o"
    system_prompt: "You are a coding expert..."
    tools: ["code_executor", "shell_executor", "file_ops", "todolist"]

  - id: "chat-assistant"
    name: "Chat Assistant"
    model: "gpt-4o-mini"
    system_prompt: "You are a friendly conversationalist..."
    tools: []
```

### 创建注册表

```go
cfg, _ := config.Load()

// ToolRegistry 包含所有可用的工具
toolRegistry := tool.NewToolRegistry()
toolRegistry.Register(code_executor.New())
toolRegistry.Register(shell_executor.New())
toolRegistry.Register(file_ops.New())
toolRegistry.Register(todolist.New())

// 基于配置创建 Agent 注册表
agentRegistry, err := agent.NewAgentRegistry(cfg, toolRegistry)
if err != nil {
    log.Fatal(err) // 工具缺失或配置错误
}
```

### 运行时路由

```go
// 场景 1: 请求明确指定 Agent
chatCtx := contextpkg.NewChatContext(ctx, sessionID, "code-assistant")

// 场景 2: 请求未指定，由 Session 决定
chatCtx := contextpkg.NewChatContext(ctx, sessionID, "")
// → 引擎从 Session 获取 DefaultAgentID
// → 如果 Session 也未设置，回退到 Default()

// 场景 3: 列出可用 Agent
agents := agentRegistry.List()
// → [{ID: "code-assistant", Name: "Code Assistant", Model: "gpt-4o"},
//     {ID: "chat-assistant", Name: "Chat Assistant", Model: "gpt-4o-mini"}]

// 场景 4: 查询特定 Agent 的配置
def, err := agentRegistry.Get("chat-assistant")
if err == agent.ErrAgentNotFound {
    // Agent 不存在
}
// def.ToolManager 只包含 chat-assistant 配置的工具（此例中为空）
// def.LLMProvider 使用 gpt-4o-mini 模型
// def.SystemPrompt = "You are a friendly conversationalist..."
```