# CopCon 架构重构方案：从单体到 AgentHarness

> 状态：Draft
> 日期：2026-05-23

## 目录

- [1. 背景与目标](#1-背景与目标)
- [2. 现状诊断](#2-现状诊断)
- [3. 目标架构](#3-目标架构)
- [4. 目录结构](#4-目录结构)
- [5. 核心设计](#5-核心设计)
- [6. 迁移路线图](#6-迁移路线图)
- [7. 附录](#7-附录)

---

## 1. 背景与目标

### 1.1 问题

当前系统架构不符合最初设计意图：

1. **核心能力与应用建设耦合在一起** — 后端所有代码在同一个 Go Module（`github.com/copcon/server`），Agent 引擎、会话管理、工具执行等核心逻辑与 Gin HTTP、GORM、YAML 配置等基础设施交织
2. **外部用户无法独立使用核心库** — 因为 `go get github.com/copcon/server` 会连带导入 Gin、GORM、OpenAI SDK 等所有依赖
3. **核心库容易污染** — 服务层代码可以绕过抽象直接操作数据库（如 `sessionMgr.GetDB()`），这种模式一旦成为习惯就会侵蚀核心边界

### 1.2 目标

| 编号 | 目标 | 成功标准 |
|------|------|---------|
| G1 | 核心能力可独立导入 | `go get github.com/copcon/core` 不引入 Gin/GORM 等基础设施依赖 |
| G2 | 开箱即用 | 最简场景下一行代码创建可用 Engine，不需要手动 wiring |
| G3 | 可扩展 | 用户可注入自定义工具、Hook、Agent 工厂 |
| G4 | 多 Agent 协作 | 支持注册多个 Agent，支持 Agent 间委派（delegate_to） |
| G5 | 前端不变 | `packages/ui` 和 `packages/demo` 保持现有结构 |

### 1.3 定位：AgentHarness（非 AgentSDK）

```
AgentSDK 模式：
  "给你接口，工具你自己实现"
  → 用户要写 CodeExecutor, ShellExecutor, TodoTool...

AgentHarness 模式（我们的定位）：
  "能力都内置了，你选择开哪些就行"
  → 用户只需：配置 LLM + 存储 → 选择 Capability → 直接使用
```

核心区别：工具、Hook、Skill 等能力是 `core` 的一等公民，不是外部样例实现。

---

## 2. 现状诊断

### 2.1 设计良好的部分

| 方面 | 现状 | 评价 |
|------|------|------|
| 前端拆分 | `packages/ui` vs `packages/demo` | ✅ 已是正确模式 |
| 领域类型 | `domain/entity/` 纯值类型，零外部依赖 | ✅ 干净 |
| LLM 抽象 | `llm/LLMProvider` 接口，与 OpenAI SDK 解耦 | ✅ 正确 |
| Hook 系统 | `hook/Hook` + `hook/HookRunner`，已是核心+扩展模式 | ✅ 设计正确 |
| ChatContext | `iface.ChatContextInterface` 接口设计合理 | ✅ 好的抽象层 |
| AgentFactory | 工厂模式支持 delegate_to 时动态注入 Task | ✅ 关键设计，必须保留 |

### 2.2 六大关键耦合点

| 严重度 | 耦合点 | 位置 | 影响 |
|--------|--------|------|------|
| 🔴 CRITICAL | `GetDB() *gorm.DB` 泄露在 SessionManager/TodoManager 接口上 | `session/manager.go`, `tools/todo/` | 所有消费者传递依赖 GORM |
| 🔴 CRITICAL | `session.Message`（GORM 模型）贯穿全栈 | `api/handlers.go`, `agent/engine.go`, `chat_context/manager.go` | 存储格式与业务逻辑耦合 |
| 🔴 CRITICAL | `GetOpenAITools()` 返回 `openai.ChatCompletionToolUnionParam` | `tool/manager.go` ToolManager 接口 | 接口绑死 OpenAI SDK |
| 🔴 CRITICAL | `*tool.AsyncToolRegistry` 作为具体类型传递 | `agent/engine.go`, `session/manager.go`, `main.go` | 未抽象为接口 |
| 🟡 MODERATE | `config.Config` 传入 `agent.NewAgentRegistry()` | `agent/registry.go` | 核心包依赖应用配置类型 |
| 🟡 MODERATE | `ChatContext` 具体实现在 `domain/iface/` 包内 | `domain/iface/chat.go` | 接口包引入 ringbuf 依赖 |

### 2.3 现状依赖全景

```
cmd/server/main.go (228 行单一组合根)
│
├─ gorm.Open(postgres.Open(dsn)) → *gorm.DB              ← 存储泄露到入口
├─ session.NewSessionManager(db, asyncRegistry)            ← 依赖 GORM
├─ chat_context.NewContextManager(db, builder, logger)     ← 依赖 GORM
├─ agent.NewAgentRegistry(cfg, toolRegistry)               ← 依赖 config.Config
│   └─ 内部创建 openai.NewClient()                         ← OpenAI SDK 泄露到核心
├─ agent.NewAgentEngine(registry, sessionMgr, ...)         ← 接收 5 个参数
├─ 注册 10 个具体 tool 实现                                ← 必须在这里写死
├─ tools.NewDelegateToTool(registry, sessionMgr, ...)      ← 循环依赖需延迟注册
└─ api.SetupRoutes(r, cfg, sessionMgr, todoMgr, ...)      ← 传递 5 个依赖
```

### 2.4 API 层直接操作数据库的例子

```go
// api/handlers.go:205 — HTTP 层绕过抽象直接写 GORM 查询
db := h.sessionMgr.GetDB()
db.WithContext(c.Request.Context()).
    Where("session_id = ?", sessUUID).
    Order("created_at ASC").
    Limit(limit).
    Find(&messages)
```

这种模式在重构后必须消除：API 层只通过接口操作，不直接接触存储层。

---

## 3. 目标架构

### 3.1 整体架构

```
                         ┌─────────────────────────────┐
                         │         用户应用              │
                         │  (Gin / Fiber / gRPC / CLI)  │
                         └─────────────┬───────────────┘
                                       │ import
                         ┌─────────────▼───────────────┐
                         │      core.AgentHarness       │
                         │                              │
                         │  用户提供: LLM + Store        │
                         │  用户选择: 哪些 Capabilities  │
                         │  库提供:   全部内置能力       │
                         └─────────────┬───────────────┘
                                       │
              ┌────────────────────────┼────────────────────────┐
              ▼                        ▼                        ▼
    ┌──────────────────┐   ┌──────────────────┐   ┌──────────────────────┐
    │ capabilities/    │   │ agent/           │   │ storage/ (接口)      │
    │ tools/ hooks/    │   │ engine.go        │   │ + providers/ (实现)  │
    │ skills/ memory/  │   │ registry.go      │   │                      │
    │                  │   │                  │   │ ← 用户注入的实现      │
    │ ← 库内置,按需开启 │   │ ← 核心引擎       │   │                      │
    └──────────────────┘   └──────────────────┘   └──────────────────────┘
```

### 3.2 模块依赖方向（严格单向）

```
server ──→ core

capabilities ──→ core/tool, core/hook, core/entity, core/iface

providers ──→ core/storage

core 内部：
  agent/ ──→ tool/, hook/, llm/, storage/, entity/, iface/
  tool/  ──→ entity/, iface/
  hook/  ──→ entity/, iface/, tool/
  llm/   ──→ (无内部依赖)
  storage/ ──→ entity/

禁止：
  core/ ──✕──→ server/
  core/ ──✕──→ gin, gorm, qdrant, openai SDK
  capabilities/ ──✕──→ server/
```

### 3.3 三条核心原则

1. **core/ 零基础设施依赖** — 不导入 Gin、GORM、Qdrant client、OpenAI SDK
2. **capabilities/ 是 core/ 的一等公民** — 内置能力，不是样例实现
3. **storage/ 是接口，providers/ 是实现** — 用户注入存储实现，core 不知道数据存在哪里

---

## 4. 目录结构

```
copcon/
├── go.work                              # use ./core ./server
│
├── core/                                # github.com/copcon/core
│   ├── go.mod                           # module github.com/copcon/core
│   │
│   ├── harness.go                       # NewAgent() + NewHarness() + Harness.Build()
│   ├── harness_test.go
│   │
│   ├── entity/                          # 纯值类型（从 domain/entity/ 迁入）
│   │   ├── event.go                     # Event, EventType, *Data 类型
│   │   ├── ui_message.go               # UIMessage, UIPart, UIStep
│   │   ├── model_message.go            # ModelMessage, ModelToolCall
│   │   ├── message_for_llm.go          # MessageForLLM
│   │   └── convert.go                  # ConvertToModelMessages()
│   │
│   ├── iface/                           # 核心接口（从 domain/iface/ 迁入，仅接口+DTO）
│   │   └── chat.go                      # ChatContextInterface, InputRequest, InputResponse
│   │
│   ├── tool/                            # 工具系统接口
│   │   ├── tool.go                      # Tool, ToolResult, DelegationTool
│   │   ├── manager.go                   # ToolManager 接口（不含 GetOpenAITools）
│   │   └── registry.go                  # ToolRegistry, AsyncToolTracker 接口
│   │
│   ├── llm/                             # LLM 抽象层
│   │   ├── provider.go                  # LLMProvider 接口 + StreamParams/Chunk/Message/ToolDef
│   │   └── openai_adapter.go           # 内置 OpenAI 适配器（core 可选依赖 openai SDK）
│   │
│   ├── hook/                            # Hook 系统
│   │   ├── hook.go                      # Hook, HookPoint, HookContext, HookExtra
│   │   └── runner.go                    # HookRunner 接口 + 实现
│   │
│   ├── agent/                           # Agent 引擎
│   │   ├── engine.go                    # AgentEngine 接口 + engineImpl
│   │   ├── engine_tools.go             # 工具执行（sync/concurrent/async）
│   │   ├── registry.go                 # AgentRegistry 接口 + 实现
│   │   ├── definition.go              # AgentDefinition, AgentInfo, AgentFactory, CreateParams
│   │   └── options.go                 # EngineOption
│   │
│   ├── context_builder/                # 上下文构建
│   │   └── builder.go                  # ContextBuilder 接口 + 实现
│   │
│   ├── storage/                        # 存储抽象接口
│   │   ├── session.go                  # Session struct + SessionStore 接口
│   │   ├── message.go                  # Message struct + MessageStore 接口
│   │   └── memory.go                   # Memory struct + MemoryStore 接口
│   │
│   ├── capabilities/                   # 内置可插拔能力
│   │   ├── registry.go                # builtins map + Capability 接口 + 注册逻辑
│   │   ├── tools/
│   │   │   ├── code_executor.go
│   │   │   ├── shell_executor.go
│   │   │   ├── file_ops.go
│   │   │   ├── todo.go
│   │   │   ├── delegate.go
│   │   │   ├── async.go              # get_tool_status, get_tool_result, cancel_tool, list_async
│   │   │   └── hitl.go              # ask_user, confirm_action
│   │   ├── hooks/
│   │   │   ├── todo_injection.go
│   │   │   ├── memory.go
│   │   │   ├── logging.go
│   │   │   └── tracing.go
│   │   ├── skills/                    # (future) Skill 支持
│   │   └── memory/                    # (future) 向量记忆能力
│   │
│   ├── chatcontext/                    # ChatContext 具体实现
│   │   └── chat_context.go           # ChatContext struct + NewChatContext()
│   │
│   └── providers/                      # 存储适配器实现
│       ├── postgres/
│       │   ├── go.mod                 # module github.com/copcon/providers/postgres
│       │   ├── session.go            # GORM SessionStore 实现
│       │   ├── message.go            # GORM MessageStore 实现
│       │   ├── todo.go               # GORM TodoStore 实现
│       │   └── models.go             # GORM 模型定义（Session, Message, Todo）
│       └── qdrant/
│           ├── go.mod                 # module github.com/copcon/providers/qdrant
│           └── memory.go             # Qdrant MemoryStore 实现
│
├── server/                            # github.com/copcon/server (参考应用)
│   ├── go.mod                         # require github.com/copcon/core
│   ├── cmd/
│   │   └── server/main.go            # 精简的组合根（~40 行）
│   ├── internal/
│   │   ├── api/                       # Gin HTTP handlers + SSE
│   │   ├── config/                    # 应用配置 → 映射到 HarnessConfig
│   │   ├── middleware/                # Gin 中间件
│   │   └── wiring/                    # 配置到 HarnessConfig 的转换逻辑
│   ├── config.yaml
│   ├── Dockerfile
│   └── migrations/
│
├── packages/                          # (不变)
│   ├── ui/                            # React 组件库
│   └── demo/                          # Vite Demo 应用
│
├── api/                               # OpenAPI spec (不变)
├── scripts/                           # 运维脚本 (不变)
├── docker-compose.yaml                # (不变)
└── go.work
```

### 关于 providers/ 模块位置

`providers/postgres/` 和 `providers/qdrant/` 的归属有两种选择：

| 方案 | 路径 | 优点 | 缺点 |
|------|------|------|------|
| A（推荐） | `core/providers/` | 用户一个 `go get` 拿到全部；版本同步发布 | core 的 go.mod 会间接依赖 gorm/qdrant |
| B | 仓库根目录独立模块 | core 真正零重依赖 | 用户需额外 `go get`；多模块版本管理复杂 |

**推荐方案 A**：将 providers 作为 core 的子目录，但在 core 的 go.mod 中使用 `// indirect` 标记。用户不使用 postgres provider 时，GORM 不会被编译进来（Go 只编译被引用的包）。同时提供 `core/providers/` 作为可选导入路径。

如果后续发现 provider 依赖造成问题，可以随时拆分为独立 module（Go module 拆分是容易的，合并才难）。

---

## 5. 核心设计

### 5.1 AgentHarness 入口

```go
// core/harness.go

package core

// ═══════════════════════════════════════════════
// 最简入口：一行创建可用 Engine
// ═══════════════════════════════════════════════

type AgentQuickConfig struct {
    // 必选
    LLM   llm.LLMProvider
    Store StoreConfig

    // 可选（有合理默认值）
    Name         string   // default: "default"
    Model        string   // default: empty（使用 LLM 的默认 model）
    SystemPrompt string   // default: "You are a helpful AI assistant..."
    Capabilities []string // default: 内置全部工具 + todo_injection + logging
}

// NewAgent 创建一个开箱即用的单 Agent Engine
func NewAgent(cfg AgentQuickConfig) (agent.AgentEngine, error) { ... }


// ═══════════════════════════════════════════════
// 完整入口：多 Agent + 自定义扩展
// ═══════════════════════════════════════════════

type HarnessConfig struct {
    // === 必选：基础设施 ===
    LLM   llm.LLMProvider
    Store StoreConfig

    // === Agent 注册（两种方式可混用）===
    Agents         []AgentSpec         // 静态定义 → 自动生成 Factory
    AgentFactories []AgentFactorySpec  // 自定义工厂 → 完全控制创建逻辑

    DefaultAgent string // 不填 = 取 Agents[0] 或 AgentFactories[0]

    // === 内置能力（按名开启）===
    Capabilities []string

    // === 用户扩展（注入自定义工具/Hook）===
    CustomTools []tool.Tool
    CustomHooks []hook.Hook

    // === 运行时参数 ===
    Concurrency int  // default: 5
    MaxSteps    int  // default: 50
}

type StoreConfig struct {
    Session storage.SessionStore
    Message storage.MessageStore
    Memory  storage.MemoryStore // nil = 不使用记忆功能
}

type Harness struct {
    engine   agent.AgentEngine
    registry agent.AgentRegistry
    // ... 内部状态
}

func NewHarness(cfg HarnessConfig) *Harness { ... }
func (h *Harness) Build() error             { ... }
func (h *Harness) Engine() agent.AgentEngine        { ... }
func (h *Harness) Registry() agent.AgentRegistry    { ... }
```

### 5.2 AgentSpec vs AgentFactorySpec

```go
// ═══════════════════════════════════════════════
// 静态定义：覆盖 90% 场景
// Harness.Build() 内部自动生成 AgentFactory
// ═══════════════════════════════════════════════

type AgentSpec struct {
    ID            string
    Name          string
    Model         string
    SystemPrompt  string
    Tools         []string // 工具名列表：["code_executor", "shell_executor", "自定义工具名"]
    Hooks         []string // Hook 名列表（agent 级别）
    AllowDelegate bool     // 是否允许其他 Agent 委派给自己
}

// ═══════════════════════════════════════════════
// 工厂定义：需要动态行为的场景
// 与现有 agent.AgentFactory 签名一致
// ═══════════════════════════════════════════════

type AgentFactorySpec struct {
    ID            string
    Name          string
    Model         string
    AllowDelegate bool
    Factory       agent.AgentFactory
}

// agent.AgentFactory — 保持现有签名不变
type AgentFactory func(ctx context.Context, params agent.CreateParams) (agent.AgentDefinition, error)

// agent.CreateParams — 保持现有字段不变
type CreateParams struct {
    Task          string         // delegate_to 时注入的任务描述
    ParentContext string         // 父 agent 的 session context
    ModelOverride string         // 动态覆盖 model
    Extra         map[string]any // 未来扩展
}
```

### 5.3 AgentSpec → AgentFactory 自动转换

`Harness.Build()` 内部将静态 `AgentSpec` 转为标准 `AgentFactory`：

```go
func (h *Harness) buildDefaultFactory(spec AgentSpec) agent.AgentFactory {
    return func(ctx context.Context, params agent.CreateParams) (agent.AgentDefinition, error) {
        // 1. 按 spec.Tools 从 capability registry 取工具
        toolMgr := tool.NewToolManager()
        for _, toolName := range spec.Tools {
            t, err := h.capRegistry.NewTool(toolName)
            if err != nil {
                return agent.AgentDefinition{}, fmt.Errorf("agent %s: %w", spec.ID, err)
            }
            if err := toolMgr.Register(t); err != nil {
                return agent.AgentDefinition{}, fmt.Errorf("agent %s: %w", spec.ID, err)
            }
        }

        // 2. 自定义工具也注册进去（按名称匹配）
        for _, ct := range h.customTools {
            for _, name := range spec.Tools {
                if ct.Name() == name {
                    toolMgr.Register(ct)
                }
            }
        }

        // 3. 按 spec.Hooks 取 hook
        var agentHooks []hook.Hook
        for _, hookName := range spec.Hooks {
            hk, err := h.capRegistry.NewHook(hookName)
            if err != nil {
                return agent.AgentDefinition{}, fmt.Errorf("agent %s: %w", spec.ID, err)
            }
            agentHooks = append(agentHooks, hk)
        }

        // 4. 动态注入 Task（保持现有 delegate_to 行为）
        systemPrompt := spec.SystemPrompt
        if params.Task != "" {
            systemPrompt = systemPrompt + "\n\nCurrent Task: " + params.Task
        }
        if params.ParentContext != "" {
            systemPrompt = systemPrompt + "\n\nParent Context: " + params.ParentContext
        }

        // 5. 动态覆盖 Model
        model := spec.Model
        if params.ModelOverride != "" {
            model = params.ModelOverride
        }

        return agent.AgentDefinition{
            ID:           spec.ID,
            Name:         spec.Name,
            Model:        model,
            SystemPrompt: systemPrompt,
            ToolManager:  toolMgr,
            LLMProvider:  h.cfg.LLM,
            Hooks:        agentHooks,
        }, nil
    }
}
```

### 5.4 Capability 注册机制

```go
// core/capabilities/registry.go

type CapabilityType string

const (
    CapabilityTool   CapabilityType = "tool"
    CapabilityHook   CapabilityType = "hook"
    CapabilitySkill  CapabilityType = "skill"   // future
    CapabilityMemory CapabilityType = "memory"  // future
)

// Capability 描述一个可插拔能力的元数据
type Capability interface {
    Name()    string         // e.g. "tools.code_executor"
    Type()    CapabilityType
    DependsOn() []string     // e.g. tools.todo → hooks.todo_injection
}

// ToolCapability 扩展 Capability，提供工具实例创建
type ToolCapability interface {
    Capability
    NewTool(deps CapabilityDeps) tool.Tool
}

// HookCapability 扩展 Capability，提供 Hook 实例创建
type HookCapability interface {
    Capability
    NewHook(deps CapabilityDeps) hook.Hook
}

// CapabilityDeps 提供能力创建时可能需要的外部依赖
type CapabilityDeps struct {
    SessionStore  storage.SessionStore
    MessageStore  storage.MessageStore
    MemoryStore   storage.MemoryStore
    AgentRegistry agent.AgentRegistry
    Engine        agent.AgentEngine  // delegate_to 需要
    Logger        *slog.Logger
}

// 全局注册表
var builtins = map[string]Capability{}

func Register(c Capability) {
    builtins[c.Name()] = c
}

// Get 按名获取能力
func Get(name string) (Capability, bool) {
    c, ok := builtins[name]
    return c, ok
}

// ListByType 列出指定类型的所有能力
func ListByType(t CapabilityType) []Capability { ... }

// ResolveDependencies 解析依赖，返回拓扑排序的能力列表
func ResolveDependencies(names []string) ([]Capability, error) { ... }
```

每个能力实现文件自注册：

```go
// core/capabilities/tools/code_executor.go

func init() {
    registry.Register(&codeExecutorCapability{})
}

type codeExecutorCapability struct{}

func (c *codeExecutorCapability) Name() string            { return "tools.code_executor" }
func (c *codeExecutorCapability) Type() CapabilityType    { return registry.CapabilityTool }
func (c *codeExecutorCapability) DependsOn() []string     { return nil }
func (c *codeExecutorCapability) NewTool(deps registry.CapabilityDeps) tool.Tool {
    return &CodeExecutor{}
}
```

```go
// core/capabilities/tools/todo.go

func init() {
    registry.Register(&todoCapability{})
}

type todoCapability struct{}

func (c *todoCapability) Name() string            { return "tools.todo" }
func (c *todoCapability) Type() CapabilityType    { return registry.CapabilityTool }
func (c *todoCapability) DependsOn() []string     { return []string{"hooks.todo_injection"} }
func (c *todoCapability) NewTool(deps registry.CapabilityDeps) tool.Tool {
    // todo tool 需要 TodoStore，从 deps.SessionStore 获取或创建
    return NewTodoTool(deps.TodoStore)
}
```

```go
// core/capabilities/tools/delegate.go

func init() {
    registry.Register(&delegateCapability{})
}

type delegateCapability struct{}

func (c *delegateCapability) Name() string            { return "tools.delegate" }
func (c *delegateCapability) Type() CapabilityType    { return registry.CapabilityTool }
func (c *delegateCapability) DependsOn() []string     { return nil } // 但运行时需要 Engine
func (c *delegateCapability) NewTool(deps registry.CapabilityDeps) tool.Tool {
    return NewDelegateToTool(deps.AgentRegistry, deps.SessionStore, deps.MessageStore, deps.Engine)
}
```

### 5.5 Harness.Build() 流程

```
输入: HarnessConfig

1. 初始化存储
   ├─ sessionStore = cfg.Store.Session
   ├─ messageStore = cfg.Store.Message
   └─ memoryStore  = cfg.Store.Memory

2. 解析 Capabilities 列表
   ├─ 展开通配符: "tools.*" → 所有 tools.* 条目
   ├─ 展开通配符: "hooks.*" → 所有 hooks.* 条目
   └─ 依赖解析: "tools.todo" → 自动追加 "hooks.todo_injection"
   └─ 拓扑排序: 按依赖关系确定注册顺序

3. 创建 ToolRegistry（全局共享）
   ├─ 按拓扑序从 CapabilityDeps.NewTool() 创建工具实例
   ├─ 注册用户自定义 CustomTools
   └─ 自定义工具与内置同名时，自定义覆盖内置

4. 创建 HookRunner（全局）
   ├─ 按拓扑序从 CapabilityDeps.NewHook() 创建 Hook 实例
   ├─ 注册用户自定义 CustomHooks
   └─ 同名覆盖逻辑同上

5. 创建 AgentRegistry
   ├─ 为每个 AgentSpec → buildDefaultFactory() 生成 AgentFactory
   ├─ 为每个 AgentFactorySpec → 直接使用用户提供的 Factory
   ├─ RegisterFactory() 注册到 AgentRegistry
   └─ 设置 DefaultAgent

6. 创建 AgentEngine
   ├─ NewAgentEngine(registry, sessionStore, messageStore, asyncTracker, opts...)
   └─ opts: WithHookRunner, WithConcurrency, WithLogger, WithMaxSteps

7. 注册跨 Agent 工具（delegate_to, read_sub_session）
   ├─ 这些工具需要 Engine 引用，所以在 Engine 创建后注册
   └─ 重新创建含 delegate_to 的 ToolRegistry 并刷新 Agent 工具集

8. 存储引擎和注册表引用
   └─ h.engine = engine; h.registry = registry

输出: Harness 就绪，可调用 .Engine() 和 .Registry()
```

### 5.6 存储接口设计

```go
// core/storage/session.go

type Session struct {
    ID             string
    Title          string
    DefaultAgentID string
    ParentSessionID *string
    Metadata       map[string]any
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type SessionStore interface {
    Create(ctx context.Context, session *Session) error
    Get(ctx context.Context, id string) (*Session, error)
    List(ctx context.Context, limit, offset int) ([]*Session, int64, error)
    Delete(ctx context.Context, id string) error
    UpdateTitle(ctx context.Context, id, title string) error
    UpdateMetadata(ctx context.Context, id string, metadata map[string]any) error
    GetMessageCount(ctx context.Context, sessionID string) (int64, error)
}
```

```go
// core/storage/message.go

type Message struct {
    ID        string
    SessionID string
    Role      string
    Content   string
    Reasoning string
    ToolCalls []ToolCall
    Parts     []Part
    Model     string
    TokenCount int
    DurationMs int64
    CreatedAt time.Time
}

type ToolCall struct {
    ID       string
    Type     string
    Function FunctionCall
}

type FunctionCall struct {
    Name      string
    Arguments string
}

type Part struct {
    Type       string `json:"type"`        // "text", "reasoning", "tool-call"
    Text       string `json:"text,omitempty"`
    State      string `json:"state,omitempty"`
    StepIndex  int    `json:"stepIndex"`
    ToolCallID string `json:"toolCallId,omitempty"`
    ToolName   string `json:"toolName,omitempty"`
    Args       string `json:"args,omitempty"`
    Output     string `json:"output,omitempty"`
    Error      string `json:"error,omitempty"`
    Interrupt  map[string]any `json:"interrupt,omitempty"`
}

type MessageStore interface {
    List(ctx context.Context, sessionID string, limit int) ([]*Message, error)
    Add(ctx context.Context, msg *Message) error
    Update(ctx context.Context, msg *Message) error
    Upsert(ctx context.Context, msg *Message) error
    DeleteBySession(ctx context.Context, sessionID string) error
}
```

```go
// core/storage/memory.go

type Memory struct {
    ID         string
    Content    string
    SessionID  string
    Role       string
    Timestamp  int64
    MemoryType string
    Metadata   map[string]any
    Score      float32
}

type MemoryStore interface {
    Store(ctx context.Context, memory *Memory) error
    Search(ctx context.Context, query []float32, limit int) ([]*Memory, error)
    GetBySession(ctx context.Context, sessionID string, limit int) ([]*Memory, error)
    DeleteBySession(ctx context.Context, sessionID string) error
}
```

**关键设计决策**：`storage/` 中的类型是纯 Go struct，不含任何 ORM 注解。`providers/postgres/` 中有对应的 GORM 模型和双向转换函数。

```go
// core/providers/postgres/models.go

// GORM 模型（不导出，仅在 provider 内部使用）
type sessionModel struct {
    ID             uuid.UUID       `gorm:"primaryKey"`
    Title          string
    DefaultAgentID string
    ParentSessionID *uuid.UUID
    Metadata       datatypes.JSONMap
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

// 转换函数
func sessionToDomain(m *sessionModel) *storage.Session { ... }
func sessionToModel(s *storage.Session) *sessionModel { ... }
```

### 5.7 ChatContext 拆分

现有 `domain/iface/chat.go` 同时包含接口和实现。拆分为：

```go
// core/iface/chat.go — 仅接口 + DTO
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
    RequestInput(req InputRequest) (*InputResponse, error)
    ResolveInput(interruptID string, resp *InputResponse) error
    PendingInputs() []InputRequest
    SetPartLocator(messageID string, stepIndex, partIndex int)
    ClearPartLocator()
}

type Subscriber struct { Events <-chan entity.Event }
type InputRequest struct { ... }
type InputResponse struct { ... }
type InterruptType string
type Storer interface { Remove(sessionID string) }
```

```go
// core/chatcontext/chat_context.go — 具体实现
type ChatContext struct {
    // 内部字段，导入 ringbuf
    ...
}

func NewChatContext(ctx context.Context, sessionID, agentID string) *ChatContext { ... }

// 编译期接口校验
var _ iface.ChatContextInterface = (*ChatContext)(nil)
```

### 5.8 ToolManager 接口修正

```go
// 现有（耦合 OpenAI SDK）:
type ToolManager interface {
    // ...
    GetOpenAITools() []openai.ChatCompletionToolUnionParam  // ❌
}

// 修正后（使用 provider-agnostic 类型）:
type ToolManager interface {
    Register(tool Tool) error
    Unregister(name string) error
    Get(name string) (Tool, error)
    List() []ToolInfo
    Execute(chatCtx iface.ChatContextInterface, name string, args map[string]any) (*ToolResult, error)
    GetToolDefs() []llm.ToolDef  // ✅ 返回通用类型
}
```

OpenAI SDK 的转换移至 `llm/openai_adapter.go`：

```go
// core/llm/openai_adapter.go
func (a *OpenAIAdapter) Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error) {
    // params.Tools 已经是 []ToolDef（通用类型）
    // 在这里转换为 openai.ChatCompletionToolUnionParam
    openaiTools := convertToOpenAITools(params.Tools)
    // ...
}
```

### 5.9 AsyncToolTracker 接口

```go
// 现有（具体类型直接传递）:
func NewAgentEngine(registry, sessionMgr, contextMgr, *tool.AsyncToolRegistry, ...)

// 修正后（接口抽象）:
type AsyncToolTracker interface {
    Register(sessionID, callID, toolName string, cancelFunc context.CancelFunc)
    Unregister(callID string)
    Complete(callID string, result any)
    Fail(callID string, errMsg string)
    GetStatus(callID string) (*AsyncToolState, error)
    Cancel(callID string) bool
    CancelSession(sessionID string) int
    ListBySession(sessionID string) []*AsyncToolState
}

func NewAgentEngine(registry, sessionStore, messageStore, asyncTracker AsyncToolTracker, ...)
```

现有 `tool.AsyncToolRegistry` 直接实现此接口，无需改动内部逻辑。

### 5.10 三种典型使用场景

**场景一：最简单 — 单 Agent 全默认**

```go
import "github.com/copcon/core"

engine, _ := core.NewAgent(core.AgentQuickConfig{
    LLM:   llm.NewOpenAIAdapter(client, "gpt-4o"),
    Store: core.StoreConfig{
        Session: postgres.NewSessionStore(db),
        Message: postgres.NewMessageStore(db),
    },
})
engine.Chat(chatCtx, "帮我写代码")
```

**场景二：单 Agent + 自定义工具**

```go
engine, _ := core.NewAgent(core.AgentQuickConfig{
    LLM:   llm.NewOpenAIAdapter(client, "gpt-4o"),
    Store: core.StoreConfig{
        Session: postgres.NewSessionStore(db),
        Message: postgres.NewMessageStore(db),
    },
    Capabilities: []string{
        "tools.code_executor",
        "tools.todo",
        "hooks.memory",
    },
    CustomTools: []tool.Tool{
        myK8sDeployTool,
    },
    SystemPrompt: "你是一个 SRE 助手...",
})
```

**场景三：多 Agent 互调**

```go
harness := core.NewHarness(core.HarnessConfig{
    LLM:   llm.NewOpenAIAdapter(client, "gpt-4o"),
    Store: core.StoreConfig{
        Session: postgres.NewSessionStore(db),
        Message: postgres.NewMessageStore(db),
        Memory:  qdrant.NewMemoryStore(qdrantClient, "copcon"),
    },
    Agents: []core.AgentSpec{
        {
            ID:            "architect",
            Name:          "Architect",
            SystemPrompt:  "你是架构师，负责分析需求并委派给编码 Agent...",
            Tools:         []string{"file_ops", "delegate_to", "read_sub_session"},
            AllowDelegate: true,
        },
        {
            ID:           "coder",
            Name:         "Coder",
            SystemPrompt: "你是编码专家，负责实际编写和运行代码...",
            Tools:        []string{"code_executor", "shell_executor", "file_ops", "todo"},
        },
    },
    DefaultAgent: "architect",
    Capabilities: []string{"hooks.logging", "hooks.memory"},
})
harness.Build()

// architect 收到请求 → 分析 → delegate_to("coder") → coder 写代码 → architect 总结
harness.Engine().Chat(chatCtx, "设计并实现一个 Redis 连接池")
```

**场景四：自定义 AgentFactory（动态行为）**

```go
harness := core.NewHarness(core.HarnessConfig{
    LLM:   myLLM,
    Store: myStore,
    AgentFactories: []core.AgentFactorySpec{
        {
            ID:   "dynamic-coder",
            Factory: func(ctx context.Context, params agent.CreateParams) (agent.AgentDefinition, error) {
                // 根据 Task 内容动态选择工具集
                tools := []string{"code_executor", "shell_executor"}
                if strings.Contains(params.Task, "deploy") {
                    tools = append(tools, "k8s_deploy") // 自定义工具
                }
                // ... 构建 AgentDefinition
            },
        },
    },
})
```

---

## 6. 迁移路线图

### Phase 0：前置准备（不改文件，只加测试）

**目标**：确保重构不破坏行为

| 步骤 | 内容 | 验收标准 |
|------|------|---------|
| 0.1 | 补充 `cmd/server/main.go` 完整启动流程的集成测试 | 能启动+创建会话+发送消息+收到SSE |
| 0.2 | 对 `api/handlers.go` 关键路径添加 HTTP 集成测试 | Chat, GetMessages, CreateSession 覆盖 |
| 0.3 | 记录当前 `go test ./...` 基线 | 所有测试通过 |

### Phase 1：解耦修复（在单一 module 内，保持向后兼容）

**目标**：消除所有 🔴 级耦合，不改变文件位置

| 步骤 | 内容 | 影响范围 | 破坏性 |
|------|------|---------|--------|
| 1.1 | 创建 `storage/` 包：定义 SessionStore, MessageStore, MemoryStore 接口 + 纯值类型 | 新文件 | 无 |
| 1.2 | 在 `session/` 中实现 SessionStore 接口 | 新方法，保留旧接口 | 无 |
| 1.3 | 在 `chat_context/` 中实现 MessageStore 接口 | 新方法，保留旧接口 | 无 |
| 1.4 | 移除 `SessionManager.GetDB()` — handlers.go 中的 GORM 查询改用 MessageStore | 修改 handlers.go | **是** |
| 1.5 | 将 `backfillParts()` / `groupPartsByStep()` 移至 `entity/convert.go` | 修改 handlers.go | **是** |
| 1.6 | 替换 `GetOpenAITools()` → `GetToolDefs() []llm.ToolDef` | 修改 ToolManager 接口 | **是** |
| 1.7 | 将 `AsyncToolRegistry` 抽象为 `AsyncToolTracker` 接口 | 修改 engine, session 签名 | **是** |
| 1.8 | 拆分 `domain/iface/chat.go`：接口留 iface/，实现移至 chatcontext/ | 修改所有 ChatContext 构造调用 | **是** |
| 1.9 | `agent/registry.go` 不再依赖 `config.Config` → 接收工厂注册 | 修改 NewAgentRegistry 签名 | **是** |
| 1.10 | 全部测试通过 | `go test ./...` | — |

**Phase 1 完成标志**：`internal/api/` 只依赖接口和领域类型，不直接接触 GORM/OpenAI SDK。

### Phase 2：提取 core/ 模块

**目标**：创建独立的 importable 核心库

| 步骤 | 内容 |
|------|------|
| 2.1 | 创建 `core/` 目录 + `core/go.mod` |
| 2.2 | 迁移 `domain/entity/` → `core/entity/` |
| 2.3 | 迁移 `domain/iface/`（仅接口+DTO） → `core/iface/` |
| 2.4 | 迁移 ChatContext 实现 → `core/chatcontext/` |
| 2.5 | 迁移 `tool/`（接口+实现） → `core/tool/` |
| 2.6 | 迁移 `llm/`（接口+类型+adapter） → `core/llm/` |
| 2.7 | 迁移 `hook/` → `core/hook/` |
| 2.8 | 迁移 `context_builder/` → `core/context_builder/` |
| 2.9 | 迁移 `agent/` → `core/agent/`（使用 storage 接口） |
| 2.10 | 迁移 `storage/` 接口 → `core/storage/` |
| 2.11 | 迁移 `session/model.go` GORM 模型 → `core/providers/postgres/models.go` |
| 2.12 | 迁移 `memory/manager.go` Qdrant 实现 → `core/providers/qdrant/memory.go` |
| 2.13 | 更新所有 import 路径 `github.com/copcon/server/internal/...` → `github.com/copcon/core/...` |
| 2.14 | 创建 `go.work`：`use ./core ./server` |
| 2.15 | 全部测试通过 |

### Phase 3：能力注册系统

| 步骤 | 内容 |
|------|------|
| 3.1 | 创建 `core/capabilities/registry.go` — Capability 接口 + 全局注册表 |
| 3.2 | 迁移所有工具实现 → `core/capabilities/tools/`，每个文件添加 init() 自注册 |
| 3.3 | 迁移所有 Hook 实现 → `core/capabilities/hooks/`，每个文件添加 init() 自注册 |
| 3.4 | 实现 CapabilityDeps 注入 |
| 3.5 | 实现依赖解析和拓扑排序 |
| 3.6 | 全部测试通过 |

### Phase 4：Harness 构建

| 步骤 | 内容 |
|------|------|
| 4.1 | 实现 `core/harness.go` — NewAgent() + NewHarness() |
| 4.2 | 实现 Harness.Build() — 完整构建流程 |
| 4.3 | 实现 AgentSpec → AgentFactory 自动转换 |
| 4.4 | 实现通配符展开（`tools.*`, `hooks.*`） |
| 4.5 | 实现自定义工具/Hook 的合并逻辑 |
| 4.6 | 处理 delegate_to 的循环依赖（Engine 创建后注册） |
| 4.7 | 全部测试通过 |

### Phase 5：重写 server

| 步骤 | 内容 |
|------|------|
| 5.1 | 重写 `cmd/server/main.go`：使用 NewHarness() 替代手动 wiring |
| 5.2 | 迁移 `internal/config/` → 映射到 HarnessConfig |
| 5.3 | `internal/api/` 改为使用 Harness.Engine() + Harness.Registry() |
| 5.4 | 删除 `server/internal/` 中已迁移到 core/ 的代码 |
| 5.5 | 全部测试通过 |

### Phase 6：CI + 发布

| 步骤 | 内容 |
|------|------|
| 6.1 | CI 加入 `GOWORK=off go test ./...` per module（验证独立构建） |
| 6.2 | CI 加入 `go vet ./...` per module |
| 6.3 | 编写 `core/` 使用文档 |
| 6.4 | Tag `core/v0.1.0` |
| 6.5 | 更新 README 和 AGENTS.md |

### 迁移时间线估算

| Phase | 复杂度 | 预估 |
|-------|--------|------|
| Phase 0 | 低 | 0.5 天 |
| Phase 1 | 高（破坏性改动多） | 3-5 天 |
| Phase 2 | 中（主要是文件移动+import替换） | 2-3 天 |
| Phase 3 | 中 | 2-3 天 |
| Phase 4 | 高（新逻辑） | 3-5 天 |
| Phase 5 | 低（main.go 大幅精简） | 1-2 天 |
| Phase 6 | 低 | 1 天 |
| **合计** | | **13-20 天** |

---

## 7. 附录

### 7.1 参考项目

| 项目 | 模式 | 适用场景 |
|------|------|---------|
| etcd | 多子模块按能力拆分 | 需要外部用户 `go get` 子模块 |
| Kubernetes | Monorepo + staging + publishing-bot | 超大项目（100k+ LOC），多团队 |
| OpenTelemetry | 按稳定性拆分模块 | 稳定 vs 实验性 API 并存 |
| Terraform | 纯 internal，不对外暴露 | CLI 应用，不作为库使用 |
| go-edge | libs/ + services/ + go.work | **最适合 CopCon** |

### 7.2 重构后的 server/main.go 示例

```go
package main

import (
    "log/slog"
    "os"

    "github.com/gin-gonic/gin"
    "github.com/openai/openai-go/v3"
    "github.com/openai/openai-go/v3/option"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"

    "github.com/copcon/core"
    "github.com/copcon/core/llm"
    "github.com/copcon/core/providers/postgres"
    "github.com/copcon/server/internal/api"
    "github.com/copcon/server/internal/config"
)

func main() {
    logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

    cfg, err := config.Load()
    if err != nil {
        logger.Error("Failed to load config", "error", err)
        os.Exit(1)
    }

    db, err := gorm.Open(postgres.Open(buildDSN(cfg.Database)), &gorm.Config{})
    if err != nil {
        logger.Error("Failed to connect database", "error", err)
        os.Exit(1)
    }

    // 自动迁移
    pgStore := pgstore.New(db)
    if err := pgStore.AutoMigrate(); err != nil {
        logger.Error("Failed to migrate database", "error", err)
        os.Exit(1)
    }

    // 构建 Harness — 替代原来的 200+ 行手动 wiring
    harness := core.NewHarness(core.HarnessConfig{
        LLM: llm.NewOpenAIAdapter(
            openai.NewClient(
                option.WithAPIKey(cfg.OpenAI.APIKey),
                option.WithBaseURL(cfg.OpenAI.BaseURL),
            ),
            cfg.OpenAI.Model,
        ),
        Store: core.StoreConfig{
            Session: pgStore.SessionStore(),
            Message: pgStore.MessageStore(),
        },
        Agents: []core.AgentSpec{
            {
                ID:            "code-assistant",
                Name:          "Code Assistant",
                Model:         cfg.OpenAI.Model,
                SystemPrompt:  "You are a helpful coding assistant...",
                Tools:         []string{"code_executor", "shell_executor", "file_ops", "todo"},
                AllowDelegate: true,
            },
            {
                ID:           "chat-assistant",
                Name:         "Chat Assistant",
                Model:        cfg.OpenAI.Model,
                SystemPrompt: "You are a friendly chat assistant...",
                Tools:        []string{},
            },
        },
        DefaultAgent: cfg.DefaultAgentID,
        Capabilities: []string{"hooks.logging"},
    })

    if err := harness.Build(); err != nil {
        logger.Error("Failed to build harness", "error", err)
        os.Exit(1)
    }

    r := gin.Default()
    r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
    api.SetupRoutes(r, harness.Engine(), harness.Registry())

    logger.Info("Server starting", "port", cfg.Server.Port)
    if err := r.Run(":" + cfg.Server.Port); err != nil {
        logger.Error("Failed to start server", "error", err)
        os.Exit(1)
    }
}
```

### 7.3 第三方用户使用 core 的示例

```go
// 第三方项目 — 使用 core 构建自己的 Agent 应用
package main

import (
    "github.com/copcon/core"
    "github.com/copcon/core/llm"
    "github.com/copcon/core/providers/postgres"
)

func main() {
    // 只需 3 样东西：LLM、存储、配置
    engine, _ := core.NewAgent(core.AgentQuickConfig{
        LLM:   llm.NewOpenAIAdapter(myClient, "gpt-4o"),
        Store: core.StoreConfig{
            Session: postgres.NewSessionStore(myDB),
            Message: postgres.NewMessageStore(myDB),
        },
    })

    // 直接使用 — 不需要 Gin、不需要 config.yaml
    chatCtx := core.NewChatContext(ctx, sessionID, "")
    engine.Chat(chatCtx, "Hello!")
}
```

### 7.4 术语表

| 术语 | 含义 |
|------|------|
| AgentHarness | CopCon 的定位 — 内置完整能力的 Agent 框架，非底层 SDK |
| Capability | 可插拔能力单元：工具、Hook、Skill、记忆等 |
| AgentSpec | 静态 Agent 定义，Harness 自动转换为 AgentFactory |
| AgentFactorySpec | 自定义 Agent 工厂定义，用户完全控制创建逻辑 |
| AgentFactory | 工厂函数签名，保持现有 `func(ctx, CreateParams) → (AgentDefinition, error)` |
| Storage 接口 | SessionStore/MessageStore/MemoryStore — 用户实现，core 消费 |
| Provider | 存储适配器实现：postgres, qdrant 等 |
| HarnessConfig | 完整配置，包含 LLM + Store + Agents + Capabilities + 扩展 |
