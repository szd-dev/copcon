
# Skill & MCP 管理平台 — Demo 应用管理能力

## TL;DR

> **Quick Summary**：在 demo 应用中新增 Skill 和 MCP 两套 CRUD 管理能力。后端新增 REST API（`/api/skills/*`、`/api/mcp/servers/*`），前端新增两个管理页面，支持查看、启用/禁用、增删操作。

> **Deliverables**：
> - Skill REST API（列表、详情、启用、禁用）
> - MCP REST API（列表、详情、新增、修改、启用、禁用、删除）
> - Skill 管理前端页面（Master-Detail 布局）
> - MCP 管理前端页面（Master-Detail 布局）
> - 前端类型定义 + API 客户端方法
> - 后端单元测试

> **Estimated Effort**：Medium
> **Parallel Execution**：YES - 5 waves
> **Critical Path**：ToolPool 改造 → Manager 实现 → API Handler → 路由注册 → 前端类型 → 前端页面 → 测试

---

## Context

### Original Request
在 demo 应用中增加 skill 和 mcp 的管理能力。这是两个独立部分：skill 要能查看所有的 skill 和内容，启用/禁用/移除；mcp 要能查看所有的 mcp 配置，并支持新增、启用、禁用、移除。

### Interview Summary
**Key Discussions**：
- 确认为经典 CRUD 管理页面，参考现有 KnowledgePage 的 Master-Detail 布局
- 后端用 Go + Gin，前端用 React 19 + Ant Design 6 + @ant-design/x
- 测试策略：实现后写测试（Tests after implementation）
- Skill 的"移除"改为"启用/禁用"（文件系统资源，移除不可逆无意义）

**Research Findings**：
- 现有测试基础设施：服务端 Go 测试成熟可用（mock 体系完善），demo 应用无测试基础设施
- Skill 通过 `SkillPlugin` 在启动时从文件系统发现，`skills` 字段私有无公共 getter
- MCP 通过 `MCPPlugin` 在启动时从 config.yaml 加载，配置和连接状态私有
- Handler 层当前无法访问 Plugin 状态，需通过 HandlerOption 注入

### Metis Review
**Identified Gaps** (addressed in plan):
| Gap | Resolution |
|-----|-----------|
| Handler 无法访问 Plugin 状态 | 定义 `SkillProvider`/`MCPProvider` 接口，通过 HandlerOption 注入 |
| SkillPlugin.skills 私有 | 添加公共 `Skills()` getter + 运行时启用状态 map |
| ToolPool 缺少 Unregister | 添加 `SetEnabled(name, bool)` 方法，Filter 禁用工具 |
| MCP env 泄露风险 | API 响应中**不返回** env 字段 |
| skill "remove" 不可逆 | 改为"启用/禁用"语义，禁用的 skill 仍出现在列表中 |
| 运行时变更不生效于活跃会话 | 文档说明：仅新会话反映变更（接受此限制） |
| config.yaml 写入非原子 | MCPManager 使用 tmp 文件 + rename 原子写入 |

---

## Work Objectives

### Core Objective
为 CopCon demo 应用增加 Skill 和 MCP 的运行时管理能力，提供完整的 CRUD 操作界面。

### Concrete Deliverables
- 后端：`server/internal/api/skills.go`、`server/internal/api/mcp.go`、`server/internal/manager/skill_manager.go`、`server/internal/manager/mcp_manager.go`
- 插件改动：`plugins/skill/plugin.go`（getter + 启用状态）、`plugins/mcp/plugin.go`（getter + 服务器管理）
- 核心改动：`core/plugin/pool.go`（SetEnabled 方法）
- 前端：`packages/demo/src/pages/SkillPage.tsx`、`packages/demo/src/pages/MCPPage.tsx`
- 前端库：`packages/chat-core/src/types.ts`（类型）、`packages/chat-core/src/agent-client.ts`（方法）
- 入口：`packages/demo/src/App.tsx`（新 Tab）
- 测试：`server/internal/api/skills_test.go`、`server/internal/api/mcp_test.go`

### Definition of Done
- [ ] `GET /api/skills` 返回所有发现的 skill（含启用状态）
- [ ] `POST /api/skills/:name/enable` 启用 skill
- [ ] `POST /api/skills/:name/disable` 禁用 skill
- [ ] `GET /api/skills/:name?include_content=true` 返回完整 skill 内容
- [ ] `GET /api/mcp/servers` 返回所有 MCP 服务器配置及连接状态
- [ ] `POST /api/mcp/servers` 新增 MCP 服务器并写入 config.yaml
- [ ] `DELETE /api/mcp/servers/:name` 移除 MCP 服务器
- [ ] `POST /api/mcp/servers/:name/enable` 启用 MCP 服务器
- [ ] `POST /api/mcp/servers/:name/disable` 禁用 MCP 服务器
- [ ] SkillPage 和 MCPPage 可从 demo 应用 Tab 访问
- [ ] 页面支持查看、启用/禁用操作，操作需有确认对话框
- [ ] 所有后端单元测试通过

### Must Have
- Skill 和 MCP 两套独立的 REST API
- 每个 API 端点返回标准 RESTful 状态码（200/201/204/404/503）
- 未配置插件时返回 503 + 有意义的错误消息
- MCP env 值不在 API 响应中暴露
- MCP 变更持久化到 config.yaml（原子写入）
- 所有删除/禁用操作需要用户确认（前端 Popconfirm）

### Must NOT Have (Guardrails)
- **NO** skill 创建/编辑/从文件系统删除 — skill 是文件系统资源，UI 仅管理运行时状态
- **NO** PUT/PATCH HTTP 方法 — 遵循现有代码库模式（POST 用于 action，DELETE 用于删除）
- **NO** MCP "test connection" 按钮 — 超出范围
- **NO** 批量操作（"全部启用"、"全部删除"）
- **NO** WebSocket/SSE 实时状态推送 — 轮询/刷新足够
- **NO** auth 中间件 — 现有路由无 auth
- **NO** MCP env 值在 API 响应中暴露 — 安全红线
- **NO** Skill "remove" 操作 — 改为 enable/disable
- **NO** 前端测试基础设施搭建 — demo 应用无测试框架，仅写后端测试

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** - 所有验证由 agent 执行。

### Test Decision
- **Infrastructure exists**：YES（后端 Go 测试成熟），NO（前端 demo 无测试）
- **Automated tests**：Tests-after（后端单元测试 + Agent QA 场景）
- **Framework**：Go standard testing + testify + httptest
- **Agent QA**：每个 task 包含 curl 验证场景，前端 task 包含 Playwright UI 验证场景

### QA Policy
每个实现任务必须包含 agent 可执行的 QA 场景。
- **后端 API**：使用 Bash (curl) — 发送请求，断言状态码 + JSON 字段
- **前端 UI**：使用 Playwright — 导航、交互、断言 DOM 元素
- 证据保存到 `.sisyphus/evidence/`

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately - Foundation):
├── Task 1:  ToolPool 添加 SetEnabled 方法 [quick]
├── Task 2:  SkillPlugin 添加公共 getter + 启用状态 [quick]
├── Task 3:  MCPPlugin 添加公共方法 [quick]
├── Task 4:  定义 SkillProvider + MCPProvider 接口 [quick]
├── Task 5:  实现 SkillManager [quick]
└── Task 6:  实现 MCPManager（含 config.yaml 原子写入）[unspecified-high]

Wave 2 (After Wave 1 - API Handlers, MAX PARALLEL):
├── Task 7:  Skills API handler（skills.go）[unspecified-high]
├── Task 8:  MCP API handler（mcp.go）[unspecified-high]
├── Task 9:  注册路由 + main.go 注入 [quick]
├── Task 10: chat-core 新增类型定义 [quick]
└── Task 11: chat-core AgentClient 新增 API 方法 [quick]

Wave 3 (After Wave 2 - Frontend Pages, MAX PARALLEL):
├── Task 12: SkillPage 组件 [visual-engineering]
├── Task 13: MCPPage 组件 [visual-engineering]
└── Task 14: App.tsx 添加 Tab [quick]

Wave 4 (After Wave 3 - Backend Tests, MAX PARALLEL):
├── Task 15: Skills API 单元测试 [unspecified-high]
└── Task 16: MCP API 单元测试 [unspecified-high]

Wave FINAL (After ALL tasks — 4 parallel reviews):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)
-> Present results -> Get explicit user okay

Critical Path: Task 1 → Task 5/6 → Task 7/8 → Task 9 → Task 12/13 → Task 14 → Task 15/16 → F1-F4
Parallel Speedup: ~60% faster than sequential
Max Concurrent: 6 (Waves 1 & 2)
```

### Dependency Matrix

| Task | Blocked By | Blocks |
|------|-----------|--------|
| 1 (ToolPool SetEnabled) | - | 5, 6 |
| 2 (SkillPlugin getter) | - | 5 |
| 3 (MCPPlugin getter) | - | 6 |
| 4 (Provider interfaces) | - | 5, 6, 7, 8 |
| 5 (SkillManager) | 1, 2, 4 | 7 |
| 6 (MCPManager) | 1, 3, 4 | 8 |
| 7 (Skills handler) | 4, 5 | 12 |
| 8 (MCP handler) | 4, 6 | 13 |
| 9 (Routes + main.go) | 7, 8 | 12, 13, 15, 16 |
| 10 (chat-core types) | - | 11, 12, 13 |
| 11 (AgentClient methods) | 10 | 12, 13 |
| 12 (SkillPage) | 7, 9, 10, 11 | 14 |
| 13 (MCPPage) | 8, 9, 10, 11 | 14 |
| 14 (App.tsx tabs) | 12, 13 | F1-F4 |
| 15 (Skills API test) | 7, 9 | - |
| 16 (MCP API test) | 8, 9 | - |

### Agent Dispatch Summary

| Wave | Count | Agents |
|------|-------|--------|
| 1 | 6 | 5×quick, 1×unspecified-high |
| 2 | 5 | 2×unspecified-high, 3×quick |
| 3 | 3 | 2×visual-engineering, 1×quick |
| 4 | 2 | 2×unspecified-high |
| FINAL | 4 | 1×oracle, 2×unspecified-high, 1×deep |

---

## TODOs

> 实现 + 验证 = 一个任务。绝不分开。每个任务必须有 QA 场景。

- [x] 1. ToolPool 添加 SetEnabled 方法

  **What to do**：
  - 在 `core/plugin/pool.go` 的 ToolPool 中添加 `enabled map[string]bool` 字段
  - 添加 `SetEnabled(name string, enabled bool)` 方法
  - 修改 `Select(names []string)` 方法：过滤掉 `enabled=false` 的工具
  - 初始化时所有工具默认 `enabled=true`
  - 添加 `IsEnabled(name string) bool` 方法供外部查询

  **Must NOT do**：
  - 不要添加 `Unregister` 方法（保持工具注册，仅过滤）
  - 不要修改 `Register` 方法签名

  **Recommended Agent Profile**：
  - **Category**：`quick`
    - Reason：单文件改动，逻辑简单，仅添加 map + 过滤逻辑
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 1（with Tasks 2, 3, 4）
  - **Blocks**：Tasks 5, 6
  - **Blocked By**：None

  **References**：
  - `core/plugin/pool.go` — ToolPool 当前实现，理解 Register 和 Select 逻辑

  **Acceptance Criteria**：
  - [ ] `SetEnabled("tool.name", false)` 后，`Select([]string{"tool.name"})` 不返回该工具
  - [ ] `SetEnabled("tool.name", true)` 后，`Select([]string{"tool.name"})` 恢复返回该工具
  - [ ] `IsEnabled("tool.name")` 返回正确的布尔值
  - [ ] 对所有已注册工具，默认 `IsEnabled()` 返回 true
  - [ ] 现有测试不受影响（`go test ./core/plugin/...` 通过）

  **QA Scenarios**：

  ```
  Scenario: 禁用工具后 Select 不返回该工具
    Tool: Bash (go test)
    Preconditions: ToolPool 中有注册的工具
    Steps:
      1. 编写测试：注册 toolA，调用 SetEnabled("toolA", false)
      2. 调用 Select([]string{"toolA"})
      3. 断言返回空列表
    Expected Result: len(result) == 0
    Failure Indicators: Select 返回了已禁用的工具
    Evidence: .sisyphus/evidence/task-1-set-enabled.txt

  Scenario: 重新启用后工具恢复可用
    Tool: Bash (go test)
    Preconditions: 工具被禁用
    Steps:
      1. SetEnabled("toolA", true)
      2. Select([]string{"toolA"})
      3. 断言 toolA 在结果中
    Expected Result: len(result) == 1, result[0].Name() == "toolA"
    Failure Indicators: 启用后工具仍不可用
    Evidence: .sisyphus/evidence/task-1-re-enable.txt
  ```

  **Evidence to Capture**：
  - [ ] task-1-set-enabled.txt — 禁用测试输出
  - [ ] task-1-re-enable.txt — 启用测试输出

  **Commit**：YES
  - Message：`feat(core): add SetEnabled/IsEnabled to ToolPool`
  - Files：`core/plugin/pool.go`
  - Pre-commit：`go test ./core/plugin/...`

- [x] 2. SkillPlugin 添加公共 getter + 运行时启用状态

  **What to do**：
  - 在 `plugins/skill/plugin.go` 的 SkillPlugin 中添加 `enabledSkills map[string]bool` 字段
  - 在 `Init()` 中初始化 `enabledSkills`，所有发现的 skill 默认 enabled=true
  - 添加公共方法 `Skills() []*skilltypes.Skill` 返回所有 skill（含禁用）
  - 添加 `SetSkillEnabled(name string, enabled bool)` 方法
  - 添加 `IsSkillEnabled(name string) bool` 方法
  - 修改 `Tools()` 和 `Hooks()` 方法：仅返回启用中的 skill 的 tool/hook

  **Must NOT do**：
  - 不要修改 `Discover()` 逻辑
  - 不要影响 skill 的发现和解析
  - 不要修改 `Skill` 类型定义

  **Recommended Agent Profile**：
  - **Category**：`quick`
    - Reason：单文件改动，添加字段和方法，逻辑简单
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 1（with Tasks 1, 3, 4）
  - **Blocks**：Task 5
  - **Blocked By**：None

  **References**：
  - `plugins/skill/plugin.go:34-39` — SkillPlugin 结构体，在此添加 enabledSkills
  - `plugins/skill/plugin.go:50-53` — Tools() 方法，需过滤禁用 skill
  - `plugins/skill/plugin.go:57-59` — Hooks() 方法，需过滤禁用 skill
  - `plugins/skill/plugin.go:62-75` — Init() 方法，在此初始化启用状态

  **Acceptance Criteria**：
  - [ ] `Skills()` 返回所有发现的 skill
  - [ ] `SetSkillEnabled("sample", false)` 后 `IsSkillEnabled("sample")` 返回 false
  - [ ] 禁用 skill 后，其 tool 不出现在 `Tools()` 返回值中
  - [ ] 禁用 skill 后，其 hook 不出现在 `Hooks()` 返回值中
  - [ ] `go build ./plugins/skill/...` 编译通过

  **QA Scenarios**：

  ```
  Scenario: 默认所有 skill 启用
    Tool: Bash (go test)
    Preconditions: SkillPlugin 已 Init
    Steps:
      1. 调用 plugin.Skills()
      2. 对每个 skill 调用 plugin.IsSkillEnabled(name)
      3. 断言全部返回 true
    Expected Result: 所有 skill 启用状态为 true
    Failure Indicators: 任何 skill 默认状态为 false
    Evidence: .sisyphus/evidence/task-2-default-enabled.txt

  Scenario: 禁用 skill 后 Tools() 不包含该 skill 的 tool
    Tool: Bash (go test)
    Preconditions: SkillPlugin 已 Init，有 skill 加载
    Steps:
      1. 记录 Tools() 返回值数量
      2. SetSkillEnabled("sample", false)
      3. 再次调用 Tools()
      4. 断言 Tools() 数量减少
    Expected Result: Tools() 包含的 tool 数量减少
    Failure Indicators: 禁用后 tool 仍存在
    Evidence: .sisyphus/evidence/task-2-disable-tool.txt
  ```

  **Evidence to Capture**：
  - [ ] task-2-default-enabled.txt
  - [ ] task-2-disable-tool.txt

  **Commit**：YES
  - Message：`feat(skill): add public getter and enable/disable state to SkillPlugin`
  - Files：`plugins/skill/plugin.go`
  - Pre-commit：`go build ./plugins/skill/...`

- [x] 3. MCPPlugin 添加公共方法

  **What to do**：
  - 在 `plugins/mcp/plugin.go` 的 mcpPlugin 中添加 `enabledServers map[string]bool` 和 `serverConfigs []MCPServerConfig`（可导出）
  - 在 `Init()` 中初始化 `enabledServers`，所有配置的 server 默认 enabled=true
  - 添加公共方法 `Servers() []MCPServerConfig` 返回所有 server 配置
  - 添加 `SetServerEnabled(name string, enabled bool)` 方法
  - 添加 `IsServerEnabled(name string) bool` 方法
  - 添加 `AddServer(cfg MCPServerConfig) error` 方法
  - 添加 `RemoveServer(name string) error` 方法
  - 修改 `discoverTools()` 仅连接启用中的 server
  - 添加 `RefreshTools()` 方法：重新从启用 server 发现工具（用于运行时增删 server）

  **Must NOT do**：
  - 不要修改 `MCPServerConfig` 类型定义
  - 不要修改 `ConnectionManager` 接口
  - 不要在 `AddServer` 中自动连接（连接由 MCPManager 层控制）

  **Recommended Agent Profile**：
  - **Category**：`quick`
    - Reason：单文件改动，添加字段和方法，逻辑清晰
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 1（with Tasks 1, 2, 4）
  - **Blocks**：Task 6
  - **Blocked By**：None

  **References**：
  - `plugins/mcp/plugin.go:13-18` — mcpPlugin 结构体
  - `plugins/mcp/plugin.go:63-102` — discoverTools() 方法
  - `plugins/mcp/plugin.go:106-108` — ConnectionManager() getter（已有此模式）
  - `plugins/mcp/config.go:20-41` — MCPServerConfig 类型

  **Acceptance Criteria**：
  - [ ] `Servers()` 返回所有配置的 server
  - [ ] `SetServerEnabled("name", false)` 后 `discoverTools()` 不包含该 server 的工具
  - [ ] `AddServer(cfg)` 后 `Servers()` 包含新 server
  - [ ] `RemoveServer("name")` 后 `Servers()` 不包含该 server
  - [ ] `RefreshTools()` 更新工具列表以反映启用状态变化
  - [ ] `go build ./plugins/mcp/...` 编译通过

  **QA Scenarios**：

  ```
  Scenario: 添加 server 后 Servers() 包含新配置
    Tool: Bash (go test)
    Preconditions: MCPPlugin 已 Init
    Steps:
      1. 记录 Servers() 数量
      2. AddServer(MCPServerConfig{Name: "test", Type: "stdio", Command: "echo"})
      3. 再次调用 Servers()
      4. 断言数量增加 1，新 server 在列表中
    Expected Result: Servers() 长度 +1，包含 Name="test" 的配置
    Failure Indicators: 新 server 未出现在列表中
    Evidence: .sisyphus/evidence/task-3-add-server.txt

  Scenario: 禁用 server 后 discoverTools 不包含该 server 的工具
    Tool: Bash (go test)
    Preconditions: 有启用的 server
    Steps:
      1. SetServerEnabled("name", false)
      2. 调用 discoverTools()
      3. 断言返回的工具不包含该 server 的
    Expected Result: 被禁用 server 的工具不出现在结果中
    Failure Indicators: 禁用后工具仍存在
    Evidence: .sisyphus/evidence/task-3-disable-server.txt
  ```

  **Evidence to Capture**：
  - [ ] task-3-add-server.txt
  - [ ] task-3-disable-server.txt

  **Commit**：YES
  - Message：`feat(mcp): add public methods and enable/disable state to MCPPlugin`
  - Files：`plugins/mcp/plugin.go`
  - Pre-commit：`go build ./plugins/mcp/...`

- [x] 4. 定义 SkillProvider + MCPProvider 接口

  **What to do**：
  - 新建 `server/internal/api/provider.go`
  - 定义 `SkillProvider` 接口：`ListSkills() []SkillInfo`、`GetSkill(name string) (*SkillInfo, error)`、`SetSkillEnabled(name string, enabled bool) error`
  - 定义 `MCPProvider` 接口：`ListServers() []MCPServerInfo`、`GetServer(name string) (*MCPServerInfo, error)`、`AddServer(cfg MCPServerConfig) error`、`RemoveServer(name string) error`、`SetServerEnabled(name string, enabled bool) error`
  - 定义 `SkillInfo` 和 `MCPServerInfo` 数据传输结构体（避免直接暴露插件类型）
  - 添加 `WithSkillProvider(sp SkillProvider) HandlerOption` 和 `WithMCPProvider(mp MCPProvider) HandlerOption`

  **Must NOT do**：
  - 不要在接口中暴露插件内部类型（如 `*skilltypes.Skill`）
  - 不要修改 Handler 结构体（HandlerOption 注入即可）

  **Recommended Agent Profile**：
  - **Category**：`quick`
    - Reason：新建接口文件，定义抽象，无复杂逻辑
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 1（with Tasks 1, 2, 3）
  - **Blocks**：Tasks 5, 6, 7, 8
  - **Blocked By**：None

  **References**：
  - `server/internal/api/handlers.go:56-70` — HandlerOption 模式（WithMemoryStore 等）
  - `server/internal/api/knowledge.go:345-393` — JSON 转换辅助函数模式

  **Acceptance Criteria**：
  - [ ] `SkillProvider` 和 `MCPProvider` 接口定义完整
  - [ ] `SkillInfo` 和 `MCPServerInfo` 结构体字段完整
  - [ ] `WithSkillProvider` 和 `WithMCPProvider` HandlerOption 函数可用
  - [ ] `go build ./server/...` 编译通过

  **QA Scenarios**：

  ```
  Scenario: 接口编译验证
    Tool: Bash (go build)
    Preconditions: 接口文件已创建
    Steps:
      1. go build ./server/internal/api/...
    Expected Result: 编译成功，无错误
    Failure Indicators: 编译失败
    Evidence: .sisyphus/evidence/task-4-build.txt
  ```

  **Evidence to Capture**：
  - [ ] task-4-build.txt

  **Commit**：YES
  - Message：`feat(api): define SkillProvider and MCPProvider interfaces`
  - Files：`server/internal/api/provider.go`
  - Pre-commit：`go build ./server/...`

- [x] 5. 实现 SkillManager

  **What to do**：
  - 新建 `server/internal/manager/skill_manager.go`
  - 实现 `SkillManager` 结构体，持有 `*skill.SkillPlugin` 引用
  - 实现 `SkillProvider` 接口的所有方法
  - `ListSkills()`：遍历 `plugin.Skills()`，返回 `[]SkillInfo`（含启用状态）
  - `GetSkill(name)`：返回单个 skill 详情（含 Instructions）
  - `SetSkillEnabled(name, enabled)`：调用 `plugin.SetSkillEnabled()` + `plugin.ToolPool.SetEnabled()`
  - 构造函数 `NewSkillManager(plugin *skill.SkillPlugin) *SkillManager`

  **Must NOT do**：
  - 不要修改 skill 文件系统内容
  - 不要修改 SkillPlugin 的发现逻辑

  **Recommended Agent Profile**：
  - **Category**：`quick`
    - Reason：简单适配器模式，桥接接口和插件实现
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 1（with Task 6）
  - **Blocks**：Task 7
  - **Blocked By**：Tasks 1, 2, 4

  **References**：
  - `server/internal/api/provider.go` — SkillProvider 接口定义
  - `plugins/skill/plugin.go` — SkillPlugin 公共方法
  - `core/capabilities/skill/types.go:4-19` — Skill 类型定义

  **Acceptance Criteria**：
  - [ ] `ListSkills()` 返回所有 skill 及其启用状态
  - [ ] `GetSkill("sample")` 返回完整 skill 信息（含 Instructions）
  - [ ] `GetSkill("nonexistent")` 返回 error
  - [ ] `SetSkillEnabled("sample", false)` 成功禁用
  - [ ] `go build ./server/...` 编译通过

  **QA Scenarios**：

  ```
  Scenario: ListSkills 返回所有 skill
    Tool: Bash (go test)
    Preconditions: SkillManager 已创建，有 skill 加载
    Steps:
      1. 调用 manager.ListSkills()
      2. 断言返回非空列表
      3. 断言每个 skill 有 name, description, enabled 字段
    Expected Result: 列表包含所有发现的 skill
    Failure Indicators: 列表为空或字段缺失
    Evidence: .sisyphus/evidence/task-5-list.txt

  Scenario: GetSkill 返回完整内容
    Tool: Bash (go test)
    Preconditions: SkillManager 已创建
    Steps:
      1. 调用 manager.GetSkill("sample")
      2. 断言 Instructions 非空
      3. 断言 Name == "sample"
    Expected Result: 返回完整 skill 详情
    Failure Indicators: Instructions 为空
    Evidence: .sisyphus/evidence/task-5-get.txt
  ```

  **Evidence to Capture**：
  - [ ] task-5-list.txt
  - [ ] task-5-get.txt

  **Commit**：YES
  - Message：`feat(manager): implement SkillManager`
  - Files：`server/internal/manager/skill_manager.go`
  - Pre-commit：`go build ./server/...`

- [x] 6. 实现 MCPManager（含 config.yaml 原子写入）

  **What to do**：
  - 新建 `server/internal/manager/mcp_manager.go`
  - 实现 `MCPManager` 结构体，持有 `*mcp.MCPPlugin` 引用 + `configPath string`
  - 实现 `MCPProvider` 接口的所有方法
  - `ListServers()`：遍历 `plugin.Servers()`，返回 `[]MCPServerInfo`（含连接状态，**不含 env 值**）
  - `GetServer(name)`：返回单个 server 详情（**不含 env 值**）
  - `AddServer(cfg)`：调用 `plugin.AddServer()` + 原子写入 config.yaml
  - `RemoveServer(name)`：调用 `plugin.RemoveServer()` + 原子写入 config.yaml
  - `SetServerEnabled(name, enabled)`：调用 `plugin.SetServerEnabled()` + 原子写入 config.yaml
  - 原子写入：先写 tmp 文件，再 `os.Rename`，确保并发安全
  - config.yaml 写入时保留原有结构（读取 → 修改 → 写回）

  **Must NOT do**：
  - 不要在 API 响应中暴露 env 值
  - 不要修改 config.yaml 的其他部分（仅 mcp.servers 部分）
  - 不要使用 `ioutil.WriteFile`（非原子）

  **Recommended Agent Profile**：
  - **Category**：`unspecified-high`
    - Reason：涉及文件原子写入和 YAML 结构保留，逻辑较复杂
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 1（with Task 5）
  - **Blocks**：Task 8
  - **Blocked By**：Tasks 1, 3, 4

  **References**：
  - `server/internal/api/provider.go` — MCPProvider 接口定义
  - `plugins/mcp/plugin.go` — MCPPlugin 公共方法
  - `plugins/mcp/config.go:20-41` — MCPServerConfig 类型
  - `server/config.yaml:48-60` — MCP 配置段格式
  - `server/internal/config/config.go:77-80` — MCPConfig 类型

  **Acceptance Criteria**：
  - [ ] `ListServers()` 返回所有 server，env 值为空（不暴露）
  - [ ] `AddServer(cfg)` 后 config.yaml 包含新 server
  - [ ] `RemoveServer("name")` 后 config.yaml 不包含该 server
  - [ ] 并发调用 AddServer/RemoveServer 不会损坏 config.yaml
  - [ ] config.yaml 非 mcp.servers 部分不受影响
  - [ ] `go build ./server/...` 编译通过

  **QA Scenarios**：

  ```
  Scenario: 添加 MCP server 持久化到 config.yaml
    Tool: Bash (go test)
    Preconditions: MCPManager 已创建，config.yaml 存在
    Steps:
      1. 调用 manager.AddServer(MCPServerConfig{Name: "test-fs", Type: "stdio", Command: "npx", Args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]})
      2. 读取 config.yaml
      3. 断言 mcp.servers 包含 Name="test-fs" 的条目
    Expected Result: config.yaml 中新增 server 配置
    Failure Indicators: config.yaml 未更新或格式损坏
    Evidence: .sisyphus/evidence/task-6-add-persist.txt

  Scenario: API 响应不暴露 env 值
    Tool: Bash (go test)
    Preconditions: 有包含 env 的 MCP server
    Steps:
      1. 调用 manager.ListServers()
      2. 断言返回的 server 中 env 为空/nil
    Expected Result: env 字段不出现在响应中
    Failure Indicators: env 值被暴露
    Evidence: .sisyphus/evidence/task-6-env-mask.txt
  ```

  **Evidence to Capture**：
  - [ ] task-6-add-persist.txt
  - [ ] task-6-env-mask.txt

  **Commit**：YES
  - Message：`feat(manager): implement MCPManager with atomic config.yaml writes`
  - Files：`server/internal/manager/mcp_manager.go`
  - Pre-commit：`go build ./server/...`

- [x] 7. Skills API handler（skills.go）

  **What to do**：
  - 新建 `server/internal/api/skills.go`
  - 在 `Handler` 中添加 `skillProvider SkillProvider` 字段
  - 实现 handler 方法：
    - `ListSkills(c *gin.Context)` — `GET /api/skills`，返回 `[]SkillInfo`（仅摘要，不含 Instructions）
    - `GetSkill(c *gin.Context)` — `GET /api/skills/:name`，支持 `?include_content=true` 返回完整 Instructions
    - `EnableSkill(c *gin.Context)` — `POST /api/skills/:name/enable`
    - `DisableSkill(c *gin.Context)` — `POST /api/skills/:name/disable`
  - 未配置 provider 时返回 503 `{"error": "skill plugin not configured"}`
  - 资源不存在时返回 404

  **Must NOT do**：
  - 不要使用 PUT/PATCH 方法
  - 不要在 list 响应中默认返回 Instructions（仅 `?include_content=true` 时）
  - 不要添加 skill 创建/编辑/删除端点

  **Recommended Agent Profile**：
  - **Category**：`unspecified-high`
    - Reason：新建 handler 文件，需实现多个路由和方法，参考现有 knowledge.go 模式
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 2（with Tasks 8, 10, 11）
  - **Blocks**：Task 9, 12
  - **Blocked By**：Tasks 4, 5

  **References**：
  - `server/internal/api/knowledge.go:25-28` — nil-check → 503 模式
  - `server/internal/api/knowledge.go:345-393` — JSON 转换辅助函数
  - `server/internal/api/handlers.go:56-70` — HandlerOption 注入模式
  - `server/internal/api/provider.go` — SkillInfo 结构体定义

  **Acceptance Criteria**：
  - [ ] `GET /api/skills` 返回 skill 列表（不含 Instructions）
  - [ ] `GET /api/skills/:name?include_content=true` 返回完整 skill 内容
  - [ ] `POST /api/skills/:name/enable` 返回 200
  - [ ] `POST /api/skills/:name/disable` 返回 200
  - [ ] 未配置 plugin 时返回 503 + "skill plugin not configured"
  - [ ] 不存在的 skill 返回 404
  - [ ] `go build ./server/...` 编译通过

  **QA Scenarios**：

  ```
  Scenario: 列表 API 返回 skill 摘要
    Tool: Bash (curl)
    Preconditions: server 运行中，skill plugin 已配置
    Steps:
      1. curl -s http://localhost:8088/api/skills | jq '.skills[0]'
      2. 断言有 name, description, enabled 字段
      3. 断言无 instructions 字段
    Expected Result: JSON 数组，每项含 name/description/enabled，无 instructions
    Failure Indicators: 包含 instructions 字段或缺少关键字段
    Evidence: .sisyphus/evidence/task-7-list.txt

  Scenario: 详情 API 含完整内容
    Tool: Bash (curl)
    Preconditions: 有 skill 存在
    Steps:
      1. curl -s "http://localhost:8088/api/skills/sample?include_content=true" | jq '.instructions'
      2. 断言 instructions 非空字符串
    Expected Result: instructions 字段包含完整 Markdown 内容
    Failure Indicators: instructions 为空
    Evidence: .sisyphus/evidence/task-7-detail.txt

  Scenario: 未配置 plugin 返回 503
    Tool: Bash (curl)
    Preconditions: server 运行但 skill plugin 未配置
    Steps:
      1. curl -s http://localhost:8088/api/skills | jq '.error'
    Expected Result: "skill plugin not configured"
    Failure Indicators: 返回 200 或空列表
    Evidence: .sisyphus/evidence/task-7-503.txt
  ```

  **Evidence to Capture**：
  - [ ] task-7-list.txt
  - [ ] task-7-detail.txt
  - [ ] task-7-503.txt

  **Commit**：YES
  - Message：`feat(api): add skills management endpoints`
  - Files：`server/internal/api/skills.go`, `server/internal/api/handlers.go`
  - Pre-commit：`go build ./server/...`

- [x] 8. MCP API handler（mcp.go）

  **What to do**：
  - 新建 `server/internal/api/mcp.go`
  - 在 `Handler` 中添加 `mcpProvider MCPProvider` 字段
  - 实现 handler 方法：
    - `ListMCPServers(c *gin.Context)` — `GET /api/mcp/servers`
    - `GetMCPServer(c *gin.Context)` — `GET /api/mcp/servers/:name`
    - `AddMCPServer(c *gin.Context)` — `POST /api/mcp/servers`，解析 JSON body
    - `RemoveMCPServer(c *gin.Context)` — `DELETE /api/mcp/servers/:name`
    - `EnableMCPServer(c *gin.Context)` — `POST /api/mcp/servers/:name/enable`
    - `DisableMCPServer(c *gin.Context)` — `POST /api/mcp/servers/:name/disable`
  - 未配置 provider 时返回 503
  - 资源不存在时返回 404
  - 无效请求体返回 400
  - 创建成功返回 201

  **Must NOT do**：
  - 不要在响应中暴露 MCP env 值
  - 不要使用 PUT/PATCH 方法
  - 不要在响应中返回原始 `MCPServerConfig` 类型（用 `MCPServerInfo`）

  **Recommended Agent Profile**：
  - **Category**：`unspecified-high`
    - Reason：新建 handler 文件，6 个端点，需处理多种 HTTP 方法和错误状态
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 2（with Tasks 7, 10, 11）
  - **Blocks**：Task 9, 13
  - **Blocked By**：Tasks 4, 6

  **References**：
  - `server/internal/api/knowledge.go:25-38` — 503 + 404 模式
  - `server/internal/api/knowledge.go:53-78` — POST 创建 + 201 响应
  - `server/internal/api/handlers.go:56-70` — HandlerOption 注入模式
  - `server/internal/api/provider.go` — MCPServerInfo 结构体定义

  **Acceptance Criteria**：
  - [ ] `GET /api/mcp/servers` 返回所有 server 列表
  - [ ] `POST /api/mcp/servers` 创建成功返回 201
  - [ ] `DELETE /api/mcp/servers/:name` 成功返回 204
  - [ ] `POST /api/mcp/servers/:name/enable` 返回 200
  - [ ] `POST /api/mcp/servers/:name/disable` 返回 200
  - [ ] 无效请求体返回 400 + error 消息
  - [ ] 不存在的 server 返回 404
  - [ ] 未配置 plugin 返回 503
  - [ ] `go build ./server/...` 编译通过

  **QA Scenarios**：

  ```
  Scenario: 创建 MCP server 成功
    Tool: Bash (curl)
    Preconditions: server 运行中，MCP plugin 已配置
    Steps:
      1. curl -s -X POST http://localhost:8088/api/mcp/servers \
         -H 'Content-Type: application/json' \
         -d '{"name":"test","type":"stdio","command":"echo","args":["hello"]}'
      2. 断言 HTTP 状态码 201
      3. 断言响应 JSON 包含 name="test"
    Expected Result: 201 Created，响应包含新 server 信息
    Failure Indicators: 状态码非 201 或响应无 name 字段
    Evidence: .sisyphus/evidence/task-8-create.txt

  Scenario: 删除 MCP server 成功
    Tool: Bash (curl)
    Preconditions: 有 MCP server 存在
    Steps:
      1. curl -s -o /dev/null -w "%{http_code}" -X DELETE http://localhost:8088/api/mcp/servers/test
      2. 断言返回 204
    Expected Result: 204 No Content
    Failure Indicators: 返回 404 或 500
    Evidence: .sisyphus/evidence/task-8-delete.txt

  Scenario: 无效请求体返回 400
    Tool: Bash (curl)
    Preconditions: server 运行中
    Steps:
      1. curl -s -X POST http://localhost:8088/api/mcp/servers \
         -H 'Content-Type: application/json' \
         -d '{"invalid":"data"}'
      2. 断言 HTTP 状态码 400
    Expected Result: 400 Bad Request
    Failure Indicators: 返回 201 或 500
    Evidence: .sisyphus/evidence/task-8-400.txt
  ```

  **Evidence to Capture**：
  - [ ] task-8-create.txt
  - [ ] task-8-delete.txt
  - [ ] task-8-400.txt

  **Commit**：YES
  - Message：`feat(api): add MCP server management endpoints`
  - Files：`server/internal/api/mcp.go`, `server/internal/api/handlers.go`
  - Pre-commit：`go build ./server/...`

- [x] 9. 注册路由 + main.go 注入

  **What to do**：
  - 在 `server/internal/api/handlers.go` 的 `SetupRoutes()` 中添加：
    - `skills := api.Group("/skills")`，注册 List/Get/Enable/Disable 路由
    - `mcpServers := api.Group("/mcp/servers")`，注册 List/Get/Add/Remove/Enable/Disable 路由
  - 在 `server/cmd/server/main.go` 中：
    - 在 `h.Register(skill.NewPlugin(...))` 之前保存 `skillPlugin` 引用
    - 在 `h.Register(mcp.NewPlugin(...))` 之前保存 `mcpPlugin` 引用
    - 创建 `SkillManager` 和 `MCPManager`
    - 通过 `api.WithSkillProvider()` 和 `api.WithMCPProvider()` 注入

  **Must NOT do**：
  - 不要修改 `SetupRoutes` 函数签名
  - 不要修改 `core.APIProvider` 接口
  - 不要修改现有路由

  **Recommended Agent Profile**：
  - **Category**：`quick`
    - Reason：路由注册和注入，逻辑简单，改动量小
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：NO
  - **Parallel Group**：Wave 2（after Tasks 7, 8）
  - **Blocks**：Tasks 12, 13, 15, 16
  - **Blocked By**：Tasks 7, 8

  **References**：
  - `server/internal/api/handlers.go:342-376` — SetupRoutes 现有路由注册模式
  - `server/cmd/server/main.go:102-112` — skill 和 mcp 插件注册位置
  - `server/cmd/server/main.go:116-124` — HandlerOption 注入位置

  **Acceptance Criteria**：
  - [ ] `/api/skills` 路由可访问
  - [ ] `/api/mcp/servers` 路由可访问
  - [ ] `go build ./server/...` 编译通过
  - [ ] 服务启动后 `curl http://localhost:8088/api/skills` 返回正常

  **QA Scenarios**：

  ```
  Scenario: 路由注册成功
    Tool: Bash (curl)
    Preconditions: 服务已编译
    Steps:
      1. 启动服务
      2. curl -s http://localhost:8088/api/skills
      3. 断言返回 JSON（非 404）
      4. curl -s http://localhost:8088/api/mcp/servers
      5. 断言返回 JSON（非 404）
    Expected Result: 两个端点均返回 JSON 响应
    Failure Indicators: 返回 404 Not Found
    Evidence: .sisyphus/evidence/task-9-routes.txt
  ```

  **Evidence to Capture**：
  - [ ] task-9-routes.txt

  **Commit**：YES
  - Message：`feat(api): register skills and MCP routes, inject managers`
  - Files：`server/internal/api/handlers.go`, `server/cmd/server/main.go`
  - Pre-commit：`go build ./server/...`

- [x] 10. chat-core 新增类型定义

  **What to do**：
  - 在 `packages/chat-core/src/types.ts` 中添加：
    - `SkillInfo` 接口：`{ name, description, enabled, source, metadata, allowed_tools }`
    - `SkillDetail` 接口：`SkillInfo + { instructions, resource_files }`
    - `MCPServerInfo` 接口：`{ name, type, command, args, url, enabled, status, allowed_tools }`
    - `MCPServerConfig` 接口：`{ name, type, command?, args?, url?, allowed_tools? }`（POST 请求体）
    - `MCPServerStatus` 类型：`'connected' | 'disconnected' | 'error'`
  - 在 `packages/chat-core/src/index.ts` 中导出新类型

  **Must NOT do**：
  - 不要修改现有类型定义
  - 不要添加服务器端独有的字段（如 env）

  **Recommended Agent Profile**：
  - **Category**：`quick`
    - Reason：纯类型定义，无逻辑，简单直接
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 2（with Tasks 7, 8, 11）
  - **Blocks**：Tasks 11, 12, 13
  - **Blocked By**：None

  **Acceptance Criteria**：
  - [ ] 新类型定义完整，字段与后端 API 响应匹配
  - [ ] `index.ts` 导出所有新类型
  - [ ] `npx tsc --noEmit` 在 `packages/chat-core` 下通过

  **QA Scenarios**：

  ```
  Scenario: 类型编译通过
    Tool: Bash (tsc)
    Preconditions: 类型已添加
    Steps:
      1. cd packages/chat-core && npx tsc --noEmit
    Expected Result: 无 TypeScript 错误
    Failure Indicators: 编译错误
    Evidence: .sisyphus/evidence/task-10-tsc.txt
  ```

  **Evidence to Capture**：
  - [ ] task-10-tsc.txt

  **Commit**：YES
  - Message：`feat(chat-core): add Skill and MCP type definitions`
  - Files：`packages/chat-core/src/types.ts`, `packages/chat-core/src/index.ts`
  - Pre-commit：`cd packages/chat-core && npx tsc --noEmit`

- [x] 11. chat-core AgentClient 新增 API 方法

  **What to do**：
  - 在 `packages/chat-core/src/agent-client.ts` 中添加：
    - `listSkills()` → `GET /api/skills`
    - `getSkill(name, includeContent?)` → `GET /api/skills/:name?include_content=true`
    - `enableSkill(name)` → `POST /api/skills/:name/enable`
    - `disableSkill(name)` → `POST /api/skills/:name/disable`
    - `listMCPServers()` → `GET /api/mcp/servers`
    - `getMCPServer(name)` → `GET /api/mcp/servers/:name`
    - `addMCPServer(config)` → `POST /api/mcp/servers`
    - `removeMCPServer(name)` → `DELETE /api/mcp/servers/:name`
    - `enableMCPServer(name)` → `POST /api/mcp/servers/:name/enable`
    - `disableMCPServer(name)` → `POST /api/mcp/servers/:name/disable`
  - 遵循现有方法模式：fetch + error throw + 类型标注

  **Must NOT do**：
  - 不要修改现有方法签名
  - 不要使用第三方 HTTP 库

  **Recommended Agent Profile**：
  - **Category**：`quick`
    - Reason：添加 10 个 fetch 方法，模式重复，机械操作
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 2（with Tasks 7, 8, 10）
  - **Blocks**：Tasks 12, 13
  - **Blocked By**：Task 10

  **References**：
  - `packages/chat-core/src/agent-client.ts:18-25` — getSessions 方法模式
  - `packages/chat-core/src/agent-client.ts:164-168` — listKnowledgeBases 方法模式
  - `packages/chat-core/src/types.ts` — SkillInfo, MCPServerInfo 等类型

  **Acceptance Criteria**：
  - [ ] 10 个方法全部实现，签名正确
  - [ ] 每个方法有正确的 HTTP 方法和路径
  - [ ] `npx tsc --noEmit` 在 `packages/chat-core` 下通过

  **QA Scenarios**：

  ```
  Scenario: API 方法类型正确
    Tool: Bash (tsc)
    Preconditions: 方法已添加
    Steps:
      1. cd packages/chat-core && npx tsc --noEmit
    Expected Result: 无 TypeScript 错误
    Failure Indicators: 编译错误
    Evidence: .sisyphus/evidence/task-11-tsc.txt
  ```

  **Evidence to Capture**：
  - [ ] task-11-tsc.txt

  **Commit**：YES
  - Message：`feat(chat-core): add Skill and MCP API client methods`
  - Files：`packages/chat-core/src/agent-client.ts`
  - Pre-commit：`cd packages/chat-core && npx tsc --noEmit`

- [x] 12. SkillPage 组件

  **What to do**：
  - 新建 `packages/demo/src/pages/SkillPage.tsx`
  - 参考 `KnowledgePage.tsx` 的 Master-Detail 布局：
    - 左侧 320px 列表：`Card` 列表，显示 skill 名称、描述、启用状态 badge
    - 右侧详情：显示完整 Instructions（`XMarkdown` 渲染）、metadata、allowed-tools、resource files
  - 启用/禁用操作：`Popconfirm` 确认后调用 `client.enableSkill/disableSkill`
  - 加载态：`Skeleton` 骨架屏
  - 空态：`Empty` 组件
  - 错误处理：`message.error` 提示
  - 使用 `useClient()` 获取 API 客户端

  **Must NOT do**：
  - 不要添加 skill 编辑/创建功能
  - 不要使用自定义 CSS（使用 Ant Design token）
  - 不要硬编码颜色值

  **Recommended Agent Profile**：
  - **Category**：`visual-engineering`
    - Reason：前端页面，Master-Detail 布局，Ant Design 组件，UI 交互
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 3（with Task 13）
  - **Blocks**：Task 14
  - **Blocked By**：Tasks 7, 9, 10, 11

  **References**：
  - `packages/demo/src/pages/KnowledgePage.tsx` — Master-Detail 布局参考
  - `packages/demo/src/components/kb/KBList.tsx` — 列表组件模式
  - `packages/demo/src/components/kb/KBDetail.tsx` — 详情组件模式
  - `packages/demo/src/context/ClientContext.tsx` — useClient() hook
  - `packages/chat-core/src/types.ts` — SkillInfo, SkillDetail 类型

  **Acceptance Criteria**：
  - [ ] 页面渲染左侧 skill 列表，右侧详情面板
  - [ ] 点击 skill 卡片，右侧显示完整 Instructions（Markdown 渲染）
  - [ ] 启用/禁用按钮可用，带 Popconfirm 确认
  - [ ] 禁用后 skill 显示 "Disabled" badge
  - [ ] 加载中和空状态正确显示
  - [ ] `npx tsc --noEmit` 在 `packages/demo` 下通过

  **QA Scenarios**：

  ```
  Scenario: SkillPage 显示 Master-Detail 布局
    Tool: Playwright
    Preconditions: 后端服务运行，有 skill 数据
    Steps:
      1. 导航到 Skills tab
      2. 验证左侧面板存在 .skill-list 区域
      3. 验证右侧面板存在 .skill-detail 区域
      4. 点击第一个 skill 卡片
      5. 验证右侧显示 Markdown 内容
      6. 截图保存
    Expected Result: 左侧列表 + 右侧详情，点击卡片后详情更新
    Failure Indicators: 布局不完整，点击无响应
    Evidence: .sisyphus/evidence/task-12-layout.png

  Scenario: 禁用 skill 操作
    Tool: Playwright
    Preconditions: SkillPage 已渲染，有启用的 skill
    Steps:
      1. 找到 "Disable" 按钮并点击
      2. 验证 Popconfirm 弹出
      3. 点击 "OK" 确认
      4. 验证 skill 卡片显示 "Disabled" badge
      5. 截图保存
    Expected Result: skill 状态变为 disabled，卡片显示禁用心 badge
    Failure Indicators: Popconfirm 未出现，状态未更新
    Evidence: .sisyphus/evidence/task-12-disable.png
  ```

  **Evidence to Capture**：
  - [ ] task-12-layout.png
  - [ ] task-12-disable.png

  **Commit**：YES
  - Message：`feat(demo): add SkillPage with Master-Detail layout`
  - Files：`packages/demo/src/pages/SkillPage.tsx`
  - Pre-commit：`cd packages/demo && npx tsc --noEmit`

- [x] 13. MCPPage 组件

  **What to do**：
  - 新建 `packages/demo/src/pages/MCPPage.tsx`
  - 参考 `KnowledgePage.tsx` 的 Master-Detail 布局：
    - 左侧 320px 列表：`Card` 列表，显示 server 名称、类型、连接状态 badge
    - 右侧详情：显示完整配置（type, command, args, url, allowed_tools）
    - 右上角 "Add Server" 按钮
  - 新增 MCP server：`Modal` 弹窗 + `Form`，字段：name, type (Select), command, args, url
  - 启用/禁用操作：`Popconfirm` 确认
  - 删除操作：`Popconfirm` 确认（红色危险按钮）
  - 连接状态显示：绿色 "Connected" / 灰色 "Disconnected" / 红色 "Error"
  - 加载态、空态、错误处理同 SkillPage
  - 使用 `useClient()` 获取 API 客户端

  **Must NOT do**：
  - 不要添加 "Test Connection" 按钮
  - 不要在表单中显示 env 字段
  - 不要使用自定义 CSS

  **Recommended Agent Profile**：
  - **Category**：`visual-engineering`
    - Reason：前端页面，表单 + 列表 + 详情布局，状态显示
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 3（with Task 12）
  - **Blocks**：Task 14
  - **Blocked By**：Tasks 8, 9, 10, 11

  **References**：
  - `packages/demo/src/pages/KnowledgePage.tsx` — Master-Detail 布局参考
  - `packages/demo/src/components/kb/CreateKBModal.tsx` — Modal + Form 模式
  - `packages/demo/src/pages/MemoryPage.tsx:122-131` — Tag 颜色映射模式
  - `packages/chat-core/src/types.ts` — MCPServerInfo 类型

  **Acceptance Criteria**：
  - [ ] 页面渲染左侧 server 列表，右侧详情面板
  - [ ] "Add Server" 按钮弹出 Modal 表单
  - [ ] 表单提交后列表刷新，显示新 server
  - [ ] 启用/禁用按钮可用，带 Popconfirm 确认
  - [ ] 删除按钮可用，带红色 Popconfirm 确认
  - [ ] 连接状态 badge 正确显示（connected/disconnected/error）
  - [ ] 加载中和空状态正确显示
  - [ ] `npx tsc --noEmit` 在 `packages/demo` 下通过

  **QA Scenarios**：

  ```
  Scenario: 新增 MCP server
    Tool: Playwright
    Preconditions: 后端服务运行
    Steps:
      1. 导航到 MCP tab
      2. 点击 "Add Server" 按钮
      3. 填写表单：name="test", type="stdio", command="echo", args="hello"
      4. 点击 "Create" 按钮
      5. 验证列表中出现新 server
      6. 截图保存
    Expected Result: 新增 server 出现在列表中
    Failure Indicators: 列表未刷新，表单提交失败
    Evidence: .sisyphus/evidence/task-13-add.png

  Scenario: 删除 MCP server
    Tool: Playwright
    Preconditions: 有 MCP server 存在
    Steps:
      1. 找到 server 的 "Delete" 按钮并点击
      2. 验证 Popconfirm 弹出
      3. 点击 "Delete" 确认
      4. 验证 server 从列表中消失
      5. 截图保存
    Expected Result: server 被移除
    Failure Indicators: Popconfirm 未出现，server 仍在列表中
    Evidence: .sisyphus/evidence/task-13-delete.png
  ```

  **Evidence to Capture**：
  - [ ] task-13-add.png
  - [ ] task-13-delete.png

  **Commit**：YES
  - Message：`feat(demo): add MCPPage with Master-Detail layout and CRUD`
  - Files：`packages/demo/src/pages/MCPPage.tsx`
  - Pre-commit：`cd packages/demo && npx tsc --noEmit`

- [x] 14. App.tsx 添加 Tab

  **What to do**：
  - 在 `packages/demo/src/App.tsx` 中：
    - 导入 `SkillPage` 和 `MCPPage`
    - 导入 `CodeOutlined` 和 `ApiOutlined` 图标（或类似图标）
    - 添加两个新 Tab：`skills` 和 `mcp`
    - 更新 `TabKey` 类型：`'chat' | 'knowledge' | 'memory' | 'skills' | 'mcp'`
    - `ErrorBoundary` 包裹新页面

  **Must NOT do**：
  - 不要修改现有 Tab 的逻辑
  - 不要修改 `ClientProvider` 或 `XProvider`

  **Recommended Agent Profile**：
  - **Category**：`quick`
    - Reason：单文件小改动，添加 Tab 配置
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：NO
  - **Parallel Group**：Wave 3（after Tasks 12, 13）
  - **Blocks**：F1-F4
  - **Blocked By**：Tasks 12, 13

  **References**：
  - `packages/demo/src/App.tsx:17-55` — 现有 Tab 配置模式
  - `packages/demo/src/pages/SkillPage.tsx` — SkillPage 组件
  - `packages/demo/src/pages/MCPPage.tsx` — MCPPage 组件

  **Acceptance Criteria**：
  - [ ] Skills Tab 可点击，渲染 SkillPage
  - [ ] MCP Tab 可点击，渲染 MCPPage
  - [ ] `npx tsc --noEmit` 在 `packages/demo` 下通过
  - [ ] `npm run build` 在 `packages/demo` 下通过

  **QA Scenarios**：

  ```
  Scenario: Skills 和 MCP Tab 可访问
    Tool: Playwright
    Preconditions: demo 应用运行
    Steps:
      1. 点击 "Skills" Tab
      2. 验证 SkillPage 渲染
      3. 截图
      4. 点击 "MCP" Tab
      5. 验证 MCPPage 渲染
      6. 截图
    Expected Result: 两个 Tab 可正常切换，页面内容正确渲染
    Failure Indicators: Tab 切换无响应，页面空白
    Evidence: .sisyphus/evidence/task-14-skills-tab.png, .sisyphus/evidence/task-14-mcp-tab.png
  ```

  **Evidence to Capture**：
  - [ ] task-14-skills-tab.png
  - [ ] task-14-mcp-tab.png

  **Commit**：YES
  - Message：`feat(demo): add Skills and MCP tabs to App`
  - Files：`packages/demo/src/App.tsx`
  - Pre-commit：`cd packages/demo && npx tsc --noEmit`

- [x] 15. Skills API 单元测试

  **What to do**：
  - 新建 `server/internal/api/skills_test.go`
  - 参考 `handlers_test.go` 的 mock 模式创建 mock SkillProvider
  - 覆盖场景：
    - `GET /api/skills` — 正常返回列表
    - `GET /api/skills` — 未配置 plugin 返回 503
    - `GET /api/skills/:name?include_content=true` — 返回完整内容
    - `GET /api/skills/:name` — 不存在的 skill 返回 404
    - `POST /api/skills/:name/enable` — 成功启用
    - `POST /api/skills/:name/disable` — 成功禁用
  - 使用 `httptest.NewRecorder` + `gin.CreateTestContext` 模式

  **Must NOT do**：
  - 不要依赖真实文件系统或数据库
  - 不要测试 SkillPlugin 内部逻辑（那是插件层的测试）

  **Recommended Agent Profile**：
  - **Category**：`unspecified-high`
    - Reason：新建测试文件，6+ 测试用例，需要 mock 和 httptest
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 4（with Task 16）
  - **Blocks**：None
  - **Blocked By**：Tasks 7, 9

  **References**：
  - `server/internal/api/handlers_test.go` — mock 体系参考（setupTestHandler, mock stores）
  - `server/internal/api/handlers_test.go:69-221` — 路由测试 httptest 模式
  - `server/internal/api/skills.go` — 被测 handler

  **Acceptance Criteria**：
  - [ ] 6+ 测试用例覆盖所有 API 端点
  - [ ] `go test ./server/internal/api/ -run TestSkills -v` 全部通过
  - [ ] 测试覆盖正常路径和错误路径

  **QA Scenarios**：

  ```
  Scenario: 所有测试通过
    Tool: Bash (go test)
    Preconditions: 测试文件已编写
    Steps:
      1. go test ./server/internal/api/ -run TestSkills -v
    Expected Result: 所有测试 PASS
    Failure Indicators: 任何测试 FAIL
    Evidence: .sisyphus/evidence/task-15-test.txt
  ```

  **Evidence to Capture**：
  - [ ] task-15-test.txt

  **Commit**：YES
  - Message：`test(api): add unit tests for skills endpoints`
  - Files：`server/internal/api/skills_test.go`
  - Pre-commit：`go test ./server/internal/api/ -run TestSkills`

- [x] 16. MCP API 单元测试

  **What to do**：
  - 新建 `server/internal/api/mcp_test.go`
  - 参考 `handlers_test.go` 的 mock 模式创建 mock MCPProvider
  - 覆盖场景：
    - `GET /api/mcp/servers` — 正常返回列表
    - `POST /api/mcp/servers` — 创建成功返回 201
    - `POST /api/mcp/servers` — 无效请求体返回 400
    - `DELETE /api/mcp/servers/:name` — 删除成功返回 204
    - `DELETE /api/mcp/servers/:name` — 不存在的 server 返回 404
    - `POST /api/mcp/servers/:name/enable` — 成功启用
    - `POST /api/mcp/servers/:name/disable` — 成功禁用
    - `GET /api/mcp/servers` — 未配置 plugin 返回 503
  - 使用 `httptest.NewRecorder` + `gin.CreateTestContext` 模式

  **Must NOT do**：
  - 不要依赖真实 MCP 连接
  - 不要测试 config.yaml 写入（那是 Manager 层的测试）

  **Recommended Agent Profile**：
  - **Category**：`unspecified-high`
    - Reason：新建测试文件，8+ 测试用例，覆盖 CRUD + 错误路径
  - **Skills**：`[]`

  **Parallelization**：
  - **Can Run In Parallel**：YES
  - **Parallel Group**：Wave 4（with Task 15）
  - **Blocks**：None
  - **Blocked By**：Tasks 8, 9

  **References**：
  - `server/internal/api/handlers_test.go` — mock 体系参考
  - `server/internal/api/handlers_test.go:69-221` — 路由测试 httptest 模式
  - `server/internal/api/mcp.go` — 被测 handler

  **Acceptance Criteria**：
  - [ ] 8+ 测试用例覆盖所有 API 端点
  - [ ] `go test ./server/internal/api/ -run TestMCP -v` 全部通过
  - [ ] 测试覆盖正常路径和错误路径（400/404/503）

  **QA Scenarios**：

  ```
  Scenario: 所有测试通过
    Tool: Bash (go test)
    Preconditions: 测试文件已编写
    Steps:
      1. go test ./server/internal/api/ -run TestMCP -v
    Expected Result: 所有测试 PASS
    Failure Indicators: 任何测试 FAIL
    Evidence: .sisyphus/evidence/task-16-test.txt
  ```

  **Evidence to Capture**：
  - [ ] task-16-test.txt

  **Commit**：YES
  - Message：`test(api): add unit tests for MCP endpoints`
  - Files：`server/internal/api/mcp_test.go`
  - Pre-commit：`go test ./server/internal/api/ -run TestMCP`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.
> **Do NOT auto-proceed after verification. Wait for user's explicit approval before marking work complete.**

- [x] F1. **Plan Compliance Audit** — `oracle`
  读取计划全文。对每个 "Must Have"：验证实现存在（读文件、curl 端点、运行命令）。对每个 "Must NOT Have"：搜索代码库中是否有禁止模式 — 如有则拒绝并给出 file:line。检查 `.sisyphus/evidence/` 中是否有证据文件。对比交付物与计划。
  Output：`Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  运行 `go test ./server/...` + `go build ./server/...` + `go vet ./server/...`。检查所有变更文件：空 catch、console.log、注释掉的代码、未使用的 import。检查 AI slop：过多注释、过度抽象、泛型命名（data/result/item/temp）。
  Output：`Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`（+ `playwright` skill if UI）
  从干净状态启动。执行所有任务中的 QA 场景 — 遵循确切步骤，捕获证据。测试跨任务集成（功能协同工作，而非孤立）。测试边界情况：空状态、无效输入、快速操作。保存到 `.sisyphus/evidence/final-qa/`。
  Output：`Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  对每个任务：阅读 "What to do"，阅读实际 diff（git log/diff）。验证 1:1 — 规范中的所有内容都已构建（无遗漏），规范之外的内容没有构建（无 creep）。检查 "Must NOT do" 合规性。检测跨任务污染：Task N 触碰 Task M 的文件。标记未计入的变更。
  Output：`Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

| Task | Message | Files |
|------|---------|-------|
| 1 | `feat(core): add SetEnabled/IsEnabled to ToolPool` | `core/plugin/pool.go` |
| 2 | `feat(skill): add public getter and enable/disable state to SkillPlugin` | `plugins/skill/plugin.go` |
| 3 | `feat(mcp): add public methods and enable/disable state to MCPPlugin` | `plugins/mcp/plugin.go` |
| 4 | `feat(api): define SkillProvider and MCPProvider interfaces` | `server/internal/api/provider.go` |
| 5 | `feat(manager): implement SkillManager` | `server/internal/manager/skill_manager.go` |
| 6 | `feat(manager): implement MCPManager with atomic config.yaml writes` | `server/internal/manager/mcp_manager.go` |
| 7 | `feat(api): add skills management endpoints` | `server/internal/api/skills.go`, `handlers.go` |
| 8 | `feat(api): add MCP server management endpoints` | `server/internal/api/mcp.go`, `handlers.go` |
| 9 | `feat(api): register skills and MCP routes, inject managers` | `handlers.go`, `server/cmd/server/main.go` |
| 10 | `feat(chat-core): add Skill and MCP type definitions` | `packages/chat-core/src/types.ts`, `index.ts` |
| 11 | `feat(chat-core): add Skill and MCP API client methods` | `packages/chat-core/src/agent-client.ts` |
| 12 | `feat(demo): add SkillPage with Master-Detail layout` | `packages/demo/src/pages/SkillPage.tsx` |
| 13 | `feat(demo): add MCPPage with Master-Detail layout and CRUD` | `packages/demo/src/pages/MCPPage.tsx` |
| 14 | `feat(demo): add Skills and MCP tabs to App` | `packages/demo/src/App.tsx` |
| 15 | `test(api): add unit tests for skills endpoints` | `server/internal/api/skills_test.go` |
| 16 | `test(api): add unit tests for MCP endpoints` | `server/internal/api/mcp_test.go` |

---

## Success Criteria

### Verification Commands
```bash
# 后端编译
go build ./server/...

# 后端测试
go test ./server/internal/api/ -run "TestSkills|TestMCP" -v

# 前端编译
cd packages/chat-core && npx tsc --noEmit
cd packages/demo && npx tsc --noEmit && npm run build

# API 冒烟测试
curl -s http://localhost:8088/api/skills | jq .
curl -s http://localhost:8088/api/mcp/servers | jq .
```

### Final Checklist
- [ ] 所有 16 个任务完成
- [ ] 所有 "Must Have" 交付物存在
- [ ] 所有 "Must NOT Have" 未出现
- [ ] 所有后端测试通过
- [ ] 前端编译通过
- [ ] Skill 和 MCP Tab 可从 demo 应用访问
- [ ] MCP env 不在 API 响应中暴露
- [ ] 禁用/删除操作有确认对话框
```
