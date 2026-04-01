# Session-Centric Architecture Refactoring

## TL;DR

> **Quick Summary**: 重构后端架构，从"以Agent为中心"转向"以Session为中心"。Agent从全局单例变为可配置的可插拔组件，Session在创建时绑定默认Agent，Chat接口支持指定Agent。
> 
> **Deliverables**:
> - 配置文件支持多Agent定义（模型、提示词、Tool列表）
> - Session模型新增default_agent_id字段
> - ToolManager重构为Registry+Manager双层架构
> - AgentEngine重构为无状态执行引擎
> - API支持指定Agent的Chat请求
> - 新增GET /api/agents端点
> 
> **Estimated Effort**: Large
> **Parallel Execution**: YES - 3 waves
> **Critical Path**: Config → Tool → Agent → API → Integration

---

## Context

### Original Request
用户希望将后端架构从"以Agent为中心"重构为"以Session为中心"，让Agent成为可插拔的共用组件。当前AgentEngine是全局单例，所有Session共享同一个Agent配置，不支持多Agent切换。

### Interview Summary
**Key Discussions**:
- Agent存储位置：配置文件(yaml)，不支持动态添加
- Tool隔离策略：每个Agent可配置不同的Tool集
- Agent切换处理：保留历史消息，新Agent继承上下文
- 默认Agent机制：配置文件定义default_agent_id
- Agent列表API：需要GET /api/agents端点
- 测试策略：TDD

**Current Architecture Issues**:
- AgentEngine是全局单例(main.go:54)
- Session模型无Agent绑定(session/model.go:85-92)
- Chat接口无Agent选择机制(api/handlers.go:156-188)
- 配置只有单一OpenAI配置(config/config.go:8-13)

### Metis Review
**Identified Gaps** (to be addressed in plan generation):
- Tool配置中引用的Tool名称需要验证存在性
- Agent ID唯一性校验
- Session绑定Agent不存在时的错误处理
- 向后兼容：现有Session的默认Agent处理

---

## Work Objectives

### Core Objective
实现Session-Centric架构：Session绑定Agent，Agent配置化，Tool按Agent隔离。

### Concrete Deliverables
- `config.yaml` 支持agents列表配置
- `config/config.go` 新增AgentConfig结构体
- `session/model.go` Session新增DefaultAgentID字段
- `tool/manager.go` 拆分为ToolRegistry + ToolManager
- `agent/engine.go` 重构为无状态引擎 + AgentRegistry
- `api/handlers.go` Chat/CreateSession支持Agent参数
- `GET /api/agents` 新端点

### Definition of Done
- [ ] 所有单元测试通过 (`go test ./... -v`)
- [ ] 服务启动成功，Agent列表可查询
- [ ] 创建Session时可指定Agent
- [ ] Chat时可指定Agent或使用Session默认Agent

### Must Have
- Agent配置文件支持（id, name, model, system_prompt, tools）
- Session绑定DefaultAgentID
- Tool按Agent隔离
- Chat接口支持agent_id参数
- GET /api/agents端点

### Must NOT Have (Guardrails)
- 不实现Agent动态添加/删除（配置文件定义，启动加载）
- 不实现热更新（修改配置需重启）
- 不修改Message表结构
- 不删除现有Tool实现
- 不破坏现有API兼容性（新增可选参数）

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed.

### Test Decision
- **Infrastructure exists**: YES (go test with testify)
- **Automated tests**: TDD (RED → GREEN → REFACTOR for each task)
- **Framework**: go test + testify/assert + testify/require
- **TDD Flow**: Each task follows RED (failing test) → GREEN (minimal impl) → REFACTOR

### QA Policy
Every task includes Agent-Executed QA Scenarios with evidence capture.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Backend API**: Use Bash (curl) — Send requests, assert status + response fields
- **Go Module**: Use Bash (go test) — Run tests, assert pass/fail
- **Database**: Use Bash (psql/docker exec) — Query tables, assert schema

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — Config + Schema + Types):
├── Task 1: AgentConfig结构体 + 配置加载 [quick]
├── Task 2: Session模型新增DefaultAgentID [quick]
├── Task 3: ToolRegistry接口定义 [quick]
├── Task 4: AgentRegistry接口定义 [quick]
├── Task 5: 数据库迁移脚本 [quick]
└── Task 6: 配置文件示例更新 [quick]

Wave 2 (After Wave 1 — Core Implementation):
├── Task 7: ToolRegistry实现 [unspecified-high]
├── Task 8: ToolManager重构（按Agent创建） [unspecified-high]
├── Task 9: AgentRegistry实现 [unspecified-high]
├── Task 10: AgentDefinition构建 [unspecified-high]
├── Task 11: AgentEngine重构（无状态） [deep]
├── Task 12: SessionManager扩展（指定Agent） [quick]

Wave 3 (After Wave 2 — API + Integration):
├── Task 13: CreateSession API修改 [quick]
├── Task 14: Chat API修改 [quick]
├── Task 15: ListAgents API新增 [quick]
├── Task 16: main.go初始化重构 [unspecified-high]
├── Task 17: 集成测试 [deep]

Wave FINAL (After ALL tasks — Verification):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)
-> Present results -> Get explicit user okay

Critical Path: Task 1 → Task 7 → Task 9 → Task 11 → Task 16 → Task 17 → F1-F4
Parallel Speedup: ~50% faster than sequential
Max Concurrent: 6 (Wave 1)
```

### Dependency Matrix

| Task | Depends On | Blocks |
|------|-----------|--------|
| 1 | — | 7, 9, 10, 16 |
| 2 | — | 12, 13 |
| 3 | — | 7 |
| 4 | — | 9 |
| 5 | 2 | 16 |
| 6 | 1 | 16 |
| 7 | 1, 3 | 8, 11, 16 |
| 8 | 7 | 11, 16 |
| 9 | 1, 4 | 10, 11, 16 |
| 10 | 1, 9 | 11, 16 |
| 11 | 8, 9, 10 | 16, 17 |
| 12 | 2 | 13 |
| 13 | 2, 12 | 17 |
| 14 | 11 | 17 |
| 15 | 9 | 17 |
| 16 | 5, 6, 7, 8, 9, 10, 11 | 17 |
| 17 | 13, 14, 15, 16 | F1-F4 |

### Agent Dispatch Summary

- **Wave 1**: 6 tasks → T1-T6 `quick`
- **Wave 2**: 6 tasks → T7-T10 `unspecified-high`, T11 `deep`, T12 `quick`
- **Wave 3**: 5 tasks → T13-T15 `quick`, T16 `unspecified-high`, T17 `deep`
- **FINAL**: 4 tasks → F1 `oracle`, F2-F3 `unspecified-high`, F4 `deep`

---

## TODOs

- [x] 1. **AgentConfig结构体 + 配置加载** — `quick`

  **What to do**:
  - 在 `config/config.go` 新增 `AgentConfig` 结构体（ID, Name, Model, SystemPrompt, Tools, BaseURL）
  - 在 `Config` 结构体新增 `Agents []AgentConfig` 和 `DefaultAgentID string`
  - 修改 `Load()` 函数支持加载新配置
  - 添加 `GetAgent(id string)` 方法获取指定Agent配置
  - 添加验证：Agent ID唯一性、DefaultAgentID存在性

  **TDD - Test First**:
  ```go
  // config/config_test.go
  func TestLoadConfigWithAgents(t *testing.T) {
      // RED: 测试加载包含agents的配置
  }
  func TestGetAgent(t *testing.T) {
      // RED: 测试获取指定Agent
  }
  func TestValidateAgentIDs(t *testing.T) {
      // RED: 测试ID唯一性验证
  }
  ```

  **Must NOT do**:
  - 不修改现有 OpenAIConfig 结构
  - 不删除现有配置加载逻辑

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 结构体定义和配置解析，单一职责
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2-6)
  - **Blocks**: Tasks 7, 9, 10, 16
  - **Blocked By**: None

  **References**:
  - `server/internal/config/config.go:8-36` - 当前Config结构体定义
  - `server/config.yaml` - 当前配置文件格式

  **Acceptance Criteria**:
  - [ ] AgentConfig结构体定义完成
  - [ ] Config.Agents和Config.DefaultAgentID字段添加
  - [ ] Load()支持加载agents配置
  - [ ] GetAgent()方法实现
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Load config with multiple agents
    Tool: Bash (go test)
    Steps:
      1. Create test config.yaml with 2 agents
      2. Run `go test ./internal/config/... -v -run TestLoadConfigWithAgents`
    Expected Result: Test passes, agents loaded correctly
    Evidence: .sisyphus/evidence/task-01-config-load.txt

  Scenario: Validate duplicate agent IDs
    Tool: Bash (go test)
    Steps:
      1. Create test config.yaml with duplicate agent IDs
      2. Run `go test ./internal/config/... -v -run TestValidateAgentIDs`
    Expected Result: Test fails with duplicate ID error
    Evidence: .sisyphus/evidence/task-01-validate-error.txt
  ```

  **Commit**: NO (groups with Wave 1)

- [x] 2. **Session模型新增DefaultAgentID** — `quick`

  **What to do**:
  - 在 `session/model.go` Session结构体新增 `DefaultAgentID string` 字段
  - 添加 gorm tag: `gorm:"size:64"`
  - 更新 `BeforeCreate` hook 确保字段有效

  **TDD - Test First**:
  ```go
  // session/model_test.go
  func TestSessionWithDefaultAgentID(t *testing.T) {
      // RED: 测试Session包含DefaultAgentID字段
  }
  ```

  **Must NOT do**:
  - 不修改Message结构体
  - 不删除现有字段

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Tasks 12, 13
  - **Blocked By**: None

  **References**:
  - `server/internal/session/model.go:85-92` - 当前Session结构体

  **Acceptance Criteria**:
  - [ ] Session.DefaultAgentID字段添加
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Create session with agent ID
    Tool: Bash (go test)
    Steps:
      1. Run `go test ./internal/session/... -v -run TestSessionWithDefaultAgentID`
    Expected Result: Test passes
    Evidence: .sisyphus/evidence/task-02-session-model.txt
  ```

  **Commit**: NO (groups with Wave 1)

- [x] 3. **ToolRegistry接口定义** — `quick`

  **What to do**:
  - 在 `tool/manager.go` 定义 `ToolRegistry` 接口
  - 接口方法：`Register(tool Tool)`, `Get(name string)`, `List() []ToolInfo`
  - 定义 `NewToolRegistry()` 构造函数

  **TDD - Test First**:
  ```go
  // tool/registry_test.go
  func TestToolRegistry(t *testing.T) {
      // RED: 测试ToolRegistry基本操作
  }
  ```

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 7
  - **Blocked By**: None

  **References**:
  - `server/internal/tool/manager.go:37-44` - 当前ToolManager接口

  **Acceptance Criteria**:
  - [ ] ToolRegistry接口定义
  - [ ] NewToolRegistry()实现
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: ToolRegistry operations
    Tool: Bash (go test)
    Steps:
      1. Run `go test ./internal/tool/... -v -run TestToolRegistry`
    Expected Result: Test passes
    Evidence: .sisyphus/evidence/task-03-registry.txt
  ```

  **Commit**: NO (groups with Wave 1)

- [x] 4. **AgentRegistry接口定义** — `quick`

  **What to do**:
  - 在 `agent/registry.go` 定义 `AgentRegistry` 接口
  - 定义 `AgentDefinition` 结构体（ID, Name, Model, SystemPrompt, ToolManager, OpenAIClient）
  - 接口方法：`Get(id string)`, `List() []AgentInfo`, `Default()`

  **TDD - Test First**:
  ```go
  // agent/registry_test.go
  func TestAgentRegistry(t *testing.T) {
      // RED: 测试AgentRegistry基本操作
  }
  ```

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 9
  - **Blocked By**: None

  **References**:
  - `server/internal/agent/engine.go:80-87` - 当前AgentEngine结构体

  **Acceptance Criteria**:
  - [ ] AgentRegistry接口定义
  - [ ] AgentDefinition结构体定义
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: AgentRegistry operations
    Tool: Bash (go test)
    Steps:
      1. Run `go test ./internal/agent/... -v -run TestAgentRegistry`
    Expected Result: Test passes
    Evidence: .sisyphus/evidence/task-04-agent-registry.txt
  ```

  **Commit**: NO (groups with Wave 1)

---

- [x] 5. **数据库迁移脚本** — `quick`

  **What to do**:
  - 更新 `session/schema.sql` 添加 `default_agent_id VARCHAR(64)` 列
  - 添加迁移SQL：`ALTER TABLE sessions ADD COLUMN default_agent_id VARCHAR(64)`
  - 考虑现有数据：设置默认值或允许NULL

  **Must NOT do**:
  - 不删除现有列
  - 不修改messages表

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 16
  - **Blocked By**: Task 2

  **References**:
  - `server/internal/session/schema.sql:1-8` - 当前sessions表定义

  **Acceptance Criteria**:
  - [ ] schema.sql更新
  - [ ] 迁移SQL编写

  **QA Scenarios**:
  ```
  Scenario: Apply migration
    Tool: Bash (psql)
    Steps:
      1. Connect to test database
      2. Run migration SQL
      3. Verify column exists: `SELECT column_name FROM information_schema.columns WHERE table_name='sessions' AND column_name='default_agent_id'`
    Expected Result: Column exists
    Evidence: .sisyphus/evidence/task-05-migration.txt
  ```

  **Commit**: NO (groups with Wave 1)

- [x] 6. **配置文件示例更新** — `quick`

  **What to do**:
  - 更新 `config.yaml` 添加示例agents配置
  - 添加 `default_agent_id` 字段
  - 添加至少2个agent示例（不同模型、不同tools）

  **Example**:
  ```yaml
  default_agent_id: "code-assistant"
  
  agents:
    - id: "code-assistant"
      name: "Code Assistant"
      model: "gpt-4o"
      system_prompt: "You are a helpful coding assistant..."
      tools: ["code_executor", "shell_executor", "file_ops"]
    - id: "chat-assistant"
      name: "Chat Assistant"
      model: "gpt-4o-mini"
      system_prompt: "You are a friendly chat assistant..."
      tools: []
  ```

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 16
  - **Blocked By**: Task 1

  **References**:
  - `server/config.yaml` - 当前配置文件

  **Acceptance Criteria**:
  - [ ] config.yaml更新
  - [ ] 至少2个agent示例
  - [ ] default_agent_id设置

  **QA Scenarios**:
  ```
  Scenario: Load updated config
    Tool: Bash (go run)
    Steps:
      1. Run `go run ./cmd/server` briefly to test config loading
      2. Check logs for successful agent loading
    Expected Result: Server starts, agents loaded
    Evidence: .sisyphus/evidence/task-06-config-example.txt
  ```

  **Commit**: YES
  - Message: `refactor(config): add AgentConfig and session agent binding schema`
  - Files: config/config.go, session/model.go, session/schema.sql, config.yaml

---

- [x] 7. **ToolRegistry实现** — `unspecified-high`

  **What to do**:
  - 实现 `toolRegistry` 结构体（全局Tool注册表）
  - 实现 `Register()`, `Get()`, `List()` 方法
  - 保持线程安全（sync.RWMutex）
  - 从现有 `toolManager` 提取公共逻辑

  **TDD - Test First**:
  ```go
  // tool/registry_test.go
  func TestToolRegistryRegister(t *testing.T) { /* RED */ }
  func TestToolRegistryGet(t *testing.T) { /* RED */ }
  func TestToolRegistryList(t *testing.T) { /* RED */ }
  func TestToolRegistryConcurrent(t *testing.T) { /* RED - 并发安全测试 */ }
  ```

  **Must NOT do**:
  - 不删除现有ToolManager（暂时共存）
  - 不修改Tool接口

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 需要重构现有代码，保持兼容性
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 1, 3)
  - **Parallel Group**: Wave 2
  - **Blocks**: Tasks 8, 11, 16
  - **Blocked By**: Tasks 1, 3

  **References**:
  - `server/internal/tool/manager.go:46-55` - 当前toolManager实现

  **Acceptance Criteria**:
  - [ ] toolRegistry结构体实现
  - [ ] 所有接口方法实现
  - [ ] 线程安全
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Concurrent tool registration
    Tool: Bash (go test)
    Steps:
      1. Run `go test ./internal/tool/... -v -run TestToolRegistryConcurrent -race`
    Expected Result: No race conditions detected
    Evidence: .sisyphus/evidence/task-07-registry-concurrent.txt
  ```

  **Commit**: NO (groups with Wave 2)

- [x] 8. **ToolManager重构（按Agent创建）** — `unspecified-high`

  **What to do**:
  - 重构 `ToolManager` 为按Agent创建的实例
  - 新增 `NewToolManager(registry ToolRegistry, toolNames []string)` 构造函数
  - `GetOpenAITools()` 只返回配置的Tool子集
  - `Execute()` 验证Tool在配置列表中

  **TDD - Test First**:
  ```go
  // tool/manager_test.go - 更新现有测试
  func TestToolManagerWithSubset(t *testing.T) {
      // RED: 测试ToolManager只使用配置的Tool子集
  }
  func TestToolManagerExecuteRestricted(t *testing.T) {
      // RED: 测试未配置的Tool执行失败
  }
  ```

  **Must NOT do**:
  - 不修改Tool接口
  - 不删除现有测试用例

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 7)
  - **Parallel Group**: Wave 2
  - **Blocks**: Tasks 11, 16
  - **Blocked By**: Task 7

  **References**:
  - `server/internal/tool/manager.go:119-133` - 当前GetOpenAITools实现

  **Acceptance Criteria**:
  - [ ] ToolManager按Agent创建
  - [ ] Tool子集过滤
  - [ ] 执行权限验证
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Execute restricted tool
    Tool: Bash (go test)
    Steps:
      1. Create ToolManager with subset ["code_executor"]
      2. Attempt to execute "shell_executor"
    Expected Result: Error - tool not allowed
    Evidence: .sisyphus/evidence/task-08-tool-restricted.txt
  ```

  **Commit**: NO (groups with Wave 2)

---

- [x] 9. **AgentRegistry实现** — `unspecified-high`

  **What to do**:
  - 实现 `agentRegistry` 结构体
  - 从配置加载Agent定义
  - 为每个Agent创建ToolManager和OpenAIClient
  - 实现 `Get()`, `List()`, `Default()` 方法
  - 验证Agent配置的Tool名称存在

  **TDD - Test First**:
  ```go
  // agent/registry_test.go
  func TestAgentRegistryLoad(t *testing.T) { /* RED */ }
  func TestAgentRegistryGet(t *testing.T) { /* RED */ }
  func TestAgentRegistryDefault(t *testing.T) { /* RED */ }
  func TestAgentRegistryValidateTools(t *testing.T) { /* RED - Tool名称验证 */ }
  ```

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 1, 4)
  - **Parallel Group**: Wave 2
  - **Blocks**: Tasks 10, 11, 16
  - **Blocked By**: Tasks 1, 4

  **References**:
  - `server/internal/agent/engine.go:89-111` - 当前NewAgentEngine实现

  **Acceptance Criteria**:
  - [ ] agentRegistry实现
  - [ ] Agent定义加载
  - [ ] Tool名称验证
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Load agents with invalid tool
    Tool: Bash (go test)
    Steps:
      1. Create config with agent referencing non-existent tool
      2. Attempt to create AgentRegistry
    Expected Result: Error - tool not found
    Evidence: .sisyphus/evidence/task-09-invalid-tool.txt
  ```

  **Commit**: NO (groups with Wave 2)

- [x] 10. **AgentDefinition构建** — `unspecified-high`

  **What to do**:
  - 实现 `buildAgentDefinition(cfg AgentConfig, registry ToolRegistry)` 函数
  - 创建Agent专属的ToolManager
  - 创建OpenAIClient（考虑BaseURL覆盖）
  - 组装AgentDefinition结构体

  **TDD - Test First**:
  ```go
  // agent/definition_test.go
  func TestBuildAgentDefinition(t *testing.T) { /* RED */ }
  func TestAgentDefinitionWithCustomBaseURL(t *testing.T) { /* RED */ }
  ```

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 1, 9)
  - **Parallel Group**: Wave 2
  - **Blocks**: Tasks 11, 16
  - **Blocked By**: Tasks 1, 9

  **References**:
  - `server/internal/agent/engine.go:96-110` - 当前OpenAI客户端创建逻辑

  **Acceptance Criteria**:
  - [ ] buildAgentDefinition函数实现
  - [ ] ToolManager创建
  - [ ] OpenAIClient创建
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Build agent with custom base URL
    Tool: Bash (go test)
    Steps:
      1. Create AgentConfig with custom base_url
      2. Call buildAgentDefinition
      3. Verify OpenAIClient uses custom base_url
    Expected Result: Custom base_url applied
    Evidence: .sisyphus/evidence/task-10-custom-url.txt
  ```

  **Commit**: NO (groups with Wave 2)

---

- [x] 11. **AgentEngine重构（无状态）** — `deep`

  **What to do**:
  - 重构 `AgentEngine` 为无状态执行引擎
  - 移除 `config`, `openaiClient` 字段（从AgentDefinition获取）
  - 新增 `agentRegistry AgentRegistry` 字段
  - 修改 `Chat(ctx, sessionID, agentID, userInput)` 方法签名
  - 从AgentRegistry获取AgentDefinition执行
  - 添加SystemPrompt处理（在context构建时注入）

  **TDD - Test First**:
  ```go
  // agent/engine_test.go
  func TestAgentEngineChatWithAgent(t *testing.T) { /* RED */ }
  func TestAgentEngineChatWithDefaultAgent(t *testing.T) { /* RED */ }
  func TestAgentEngineSystemPrompt(t *testing.T) { /* RED */ }
  ```

  **Must NOT do**:
  - 不修改Event结构体
  - 不修改流式响应逻辑

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 核心逻辑重构，需要深入理解现有实现
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 8, 9, 10)
  - **Parallel Group**: Wave 2
  - **Blocks**: Tasks 16, 17
  - **Blocked By**: Tasks 8, 9, 10

  **References**:
  - `server/internal/agent/engine.go:113-128` - 当前Chat方法
  - `server/internal/agent/engine.go:130-316` - runAgentLoop核心逻辑
  - `server/internal/agent/engine.go:163-169` - OpenAI调用参数构建

  **Acceptance Criteria**:
  - [ ] AgentEngine无状态化
  - [ ] Chat方法支持agentID参数
  - [ ] SystemPrompt注入
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Chat with specific agent
    Tool: Bash (go test)
    Steps:
      1. Create session with default agent A
      2. Chat with agent B specified
      3. Verify response uses agent B's model
    Expected Result: Agent B used for chat
    Evidence: .sisyphus/evidence/task-11-chat-agent.txt

  Scenario: System prompt injection
    Tool: Bash (go test)
    Steps:
      1. Chat with agent having custom system_prompt
      2. Verify system message in context
    Expected Result: System prompt injected correctly
    Evidence: .sisyphus/evidence/task-11-system-prompt.txt
  ```

  **Commit**: NO (groups with Wave 2)

- [x] 12. **SessionManager扩展（指定Agent）** — `quick`

  **What to do**:
  - 修改 `SessionManager.Create()` 接口签名：`Create(ctx, title, defaultAgentID string)`
  - 更新实现
  - 更新测试

  **TDD - Test First**:
  ```go
  // session/manager_test.go
  func TestCreateSessionWithAgent(t *testing.T) { /* RED */ }
  ```

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 2)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 13
  - **Blocked By**: Task 2

  **References**:
  - `server/internal/session/manager.go:16-17` - 当前Create接口
  - `server/internal/session/manager.go:34-48` - 当前Create实现

  **Acceptance Criteria**:
  - [ ] Create接口签名更新
  - [ ] DefaultAgentID存储
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Create session with agent
    Tool: Bash (go test)
    Steps:
      1. Run `go test ./internal/session/... -v -run TestCreateSessionWithAgent`
    Expected Result: Test passes, agent ID stored
    Evidence: .sisyphus/evidence/task-12-session-agent.txt
  ```

  **Commit**: YES
  - Message: `refactor(tool,agent): implement ToolRegistry and AgentRegistry with per-agent isolation`
  - Files: tool/manager.go, agent/registry.go, agent/engine.go, session/manager.go

---

- [x] 13. **CreateSession API修改** — `quick`

  **What to do**:
  - 修改 `CreateSession` handler
  - 请求体新增可选 `default_agent_id` 字段
  - 不传则使用配置的默认Agent
  - 返回值包含 `default_agent_id`

  **API变更**:
  ```go
  // Request
  {
      "title": "New Chat",
      "default_agent_id": "code-assistant"  // 可选
  }
  
  // Response
  {
      "id": "...",
      "title": "New Chat",
      "default_agent_id": "code-assistant",
      ...
  }
  ```

  **TDD - Test First**:
  ```go
  // api/handlers_test.go
  func TestCreateSessionWithAgent(t *testing.T) { /* RED */ }
  func TestCreateSessionWithDefaultAgent(t *testing.T) { /* RED */ }
  ```

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 2, 12)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 17
  - **Blocked By**: Tasks 2, 12

  **References**:
  - `server/internal/api/handlers.go:30-46` - 当前CreateSession实现

  **Acceptance Criteria**:
  - [ ] 请求体支持default_agent_id
  - [ ] 默认Agent处理
  - [ ] 响应包含default_agent_id
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Create session without agent
    Tool: Bash (curl)
    Steps:
      1. POST /api/sessions with empty body
      2. Verify default_agent_id set to config default
    Expected Result: Session created with default agent
    Evidence: .sisyphus/evidence/task-13-create-default.txt
  ```

  **Commit**: NO (groups with Wave 3)

- [x] 14. **Chat API修改** — `quick`

  **What to do**:
  - 修改 `Chat` handler
  - 请求体新增可选 `agent_id` 字段
  - 不传则使用Session的default_agent_id
  - 传递agent_id给AgentEngine.Chat()

  **API变更**:
  ```go
  // Request
  {
      "content": "Hello",
      "agent_id": "chat-assistant"  // 可选
  }
  ```

  **TDD - Test First**:
  ```go
  // api/handlers_test.go
  func TestChatWithAgent(t *testing.T) { /* RED */ }
  func TestChatWithSessionDefaultAgent(t *testing.T) { /* RED */ }
  ```

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 11)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 17
  - **Blocked By**: Task 11

  **References**:
  - `server/internal/api/handlers.go:156-188` - 当前Chat实现

  **Acceptance Criteria**:
  - [ ] 请求体支持agent_id
  - [ ] Session默认Agent处理
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: Chat with specific agent
    Tool: Bash (curl)
    Steps:
      1. Create session with agent A
      2. POST /api/sessions/:id/chat with agent_id: B
      3. Verify response uses agent B
    Expected Result: Agent B used
    Evidence: .sisyphus/evidence/task-14-chat-specific.txt
  ```

  **Commit**: NO (groups with Wave 3)

---

- [x] 15. **ListAgents API新增** — `quick`

  **What to do**:
  - 新增 `ListAgents` handler
  - 返回所有可用Agent列表
  - 路由：`GET /api/agents`

  **API响应**:
  ```go
  // GET /api/agents
  {
      "agents": [
          {
              "id": "code-assistant",
              "name": "Code Assistant",
              "model": "gpt-4o"
          },
          ...
      ]
  }
  ```

  **TDD - Test First**:
  ```go
  // api/handlers_test.go
  func TestListAgents(t *testing.T) { /* RED */ }
  ```

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 9)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 17
  - **Blocked By**: Task 9

  **References**:
  - `server/internal/api/handlers.go:190-205` - SetupRoutes函数

  **Acceptance Criteria**:
  - [ ] ListAgents handler实现
  - [ ] GET /api/agents路由注册
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: List agents
    Tool: Bash (curl)
    Steps:
      1. GET /api/agents
      2. Verify response contains all configured agents
    Expected Result: Agent list returned
    Evidence: .sisyphus/evidence/task-15-list-agents.txt
  ```

  **Commit**: NO (groups with Wave 3)

- [x] 16. **main.go初始化重构** — `unspecified-high`

  **What to do**:
  - 重构初始化流程
  - 创建ToolRegistry并注册所有Tool
  - 创建AgentRegistry从配置加载Agent
  - 创建无状态AgentEngine
  - 更新Handler初始化

  **初始化顺序**:
  ```
  1. Load config
  2. Connect database
  3. AutoMigrate (新增字段)
  4. Create ToolRegistry
  5. Register all tools to ToolRegistry
  6. Create AgentRegistry (from config, ToolRegistry)
  7. Create SessionManager, ContextManager, MemoryManager
  8. Create AgentEngine (with AgentRegistry)
  9. Setup API routes
  ```

  **Must NOT do**:
  - 不删除现有组件初始化
  - 不修改健康检查端点

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖所有Wave 1和Wave 2任务)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 17
  - **Blocked By**: Tasks 5, 6, 7, 8, 9, 10, 11

  **References**:
  - `server/cmd/server/main.go:21-68` - 当前main函数

  **Acceptance Criteria**:
  - [ ] ToolRegistry创建和Tool注册
  - [ ] AgentRegistry创建
  - [ ] AgentEngine重构
  - [ ] 服务启动成功

  **QA Scenarios**:
  ```
  Scenario: Server startup with agents
    Tool: Bash (go run)
    Steps:
      1. Run `go run ./cmd/server`
      2. Check logs for agent loading
      3. GET /api/agents to verify
    Expected Result: Server starts, agents loaded
    Evidence: .sisyphus/evidence/task-16-startup.txt
  ```

  **Commit**: NO (groups with Wave 3)

- [ ] 17. **集成测试** — `deep`

  **What to do**:
  - 编写端到端集成测试
  - 测试完整流程：创建Session → 指定Agent → Chat → 切换Agent → 继续Chat
  - 测试Tool隔离：不同Agent使用不同Tool
  - 测试错误场景：Agent不存在、Tool不允许

  **测试场景**:
  ```go
  // integration_test.go
  func TestE2E_SessionWithAgent(t *testing.T) { /* RED */ }
  func TestE2E_AgentSwitch(t *testing.T) { /* RED */ }
  func TestE2E_ToolIsolation(t *testing.T) { /* RED */ }
  func TestE2E_AgentNotFound(t *testing.T) { /* RED */ }
  ```

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖所有Wave 3任务)
  - **Parallel Group**: Wave 3
  - **Blocks**: F1-F4
  - **Blocked By**: Tasks 13, 14, 15, 16

  **References**:
  - `server/internal/session/manager_test.go` - 现有测试模式

  **Acceptance Criteria**:
  - [ ] 集成测试编写
  - [ ] 所有场景覆盖
  - [ ] 测试通过

  **QA Scenarios**:
  ```
  Scenario: Full E2E flow
    Tool: Bash (go test)
    Steps:
      1. Run `go test ./... -v -run TestE2E`
    Expected Result: All E2E tests pass
    Evidence: .sisyphus/evidence/task-17-e2e.txt
  ```

  **Commit**: YES
  - Message: `refactor(api): support agent selection in chat and session creation`
  - Files: api/handlers.go, cmd/server/main.go, integration tests

## Final Verification Wave (MANDATORY)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists. For each "Must NOT Have": search codebase for forbidden patterns. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go test ./... -v`. Review all changed files for: `as any`/`@ts-ignore` equivalents in Go, empty catches, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names.
  Output: `Build [PASS/FAIL] | Lint [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Execute EVERY QA scenario from EVERY task. Test cross-task integration. Test edge cases: empty state, invalid input, agent not found. Save evidence to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built, nothing beyond spec was built. Check "Must NOT do" compliance.
  Output: `Tasks [N/N compliant] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **Wave 1 Completion**: `refactor(config): add AgentConfig and session agent binding schema`
- **Wave 2 Completion**: `refactor(tool,agent): implement ToolRegistry and AgentRegistry with per-agent isolation`
- **Wave 3 Completion**: `refactor(api): support agent selection in chat and session creation`
- **Final**: `refactor: complete session-centric architecture transformation`

---

## Success Criteria

### Verification Commands
```bash
go test ./... -v                     # Expected: All tests pass
go run ./cmd/server                  # Expected: Server starts successfully
curl http://localhost:8080/api/agents  # Expected: Returns agent list
curl -X POST http://localhost:8080/api/sessions -d '{"default_agent_id":"agent-1"}'  # Expected: Session created with agent binding
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass
- [ ] API backward compatible (optional parameters)
- [ ] Documentation updated (AGENTS.md)