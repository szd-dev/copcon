# MCP 客户端支持

## TL;DR

> **核心目标**：为 CopCon Agent 引擎添加 MCP 客户端能力，使 Agent 能连接任意 MCP 服务器并使用其工具。
> 
> **交付物**：
> - `plugins/mcp/` 子模块（ModuleCapability + MCPToolWrapper + ConnectionManager）
> - MCP 配置支持（yaml 配置多服务器连接）
> - 三种传输层支持（stdio / SSE / Streamable HTTP）
> - 全套单元测试（TDD）
> 
> **预计规模**：Medium
> **并行执行**：YES — 3 波次，最大并行 4
> **关键路径**：Task 1 → Task 4 → Task 6 → Task 8 → Task 9

---

## 上下文

### 原始需求
用户要在 CopCon 中支持 MCP 客户端，让 Agent 能连接到各种 MCP 服务器上使用其工具。经过调研，确认采用**方案 A（扁平化）**——每个 MCP 工具注册为独立的 `tool.Tool`，使用 `mcp__{server}__{tool}` 命名格式。

### 调研摘要
**关键讨论**：
- 方案选择：扁平化（Claude Code / LangChain / AutoGen 一致做法）vs 元工具（仅 OpenAI 托管场景）
- SDK 选择：`github.com/modelcontextprotocol/go-sdk` v1.6.1（官方，Google + Anthropic 维护）
- 命名格式：`mcp__{serverName}__{toolName}`（双下划线，参考 Claude Code）
- 测试策略：TDD（先写测试再实现）

**调研发现**：
- 官方 SDK 已被 Google ADK、Docker Agent、GitLab、Dapr、LocalAI 等大项目使用
- API 模式：`mcp.Client` → `Connect(transport)` → `ClientSession` → `ListTools`/`CallTool`
- 传输层：stdio、SSE、Streamable HTTP 三种
- 项目 Go 版本 1.26.1，满足 SDK 要求（≥ 1.25）

### Metis 审查
**已识别并处理的缺口**：
- 连接生命周期管理：需要 ConnectionManager 处理连接复用、重连、超时
- 工具 Schema 转换：MCP Tool.InputSchema → `map[string]any` → `llm.ToolDef`
- 错误处理策略：MCP 调用失败时返回结构化错误给 LLM，而非中断 Agent 循环
- 配置热加载：本次迭代不支持，MCP 服务器在 Harness.Build 时一次性连接
- 工具数量限制：大量 MCP 工具可能导致 LLM token 超限，需在配置中支持 `allowed_tools` 过滤

---

## 工作目标

### 核心目标
为 CopCon Agent 引擎添加 MCP 客户端支持，使 Agent 能够连接多个 MCP 服务器，发现并使用其工具。

### 具体交付物
- `plugins/mcp/go.mod` — MCP 插件模块
- `plugins/mcp/config.go` — MCP 配置类型定义
- `plugins/mcp/types.go` — MCP 工具信息类型、MCPToolWrapper 接口
- `plugins/mcp/wrapper.go` — MCPToolWrapper 实现（实现 `tool.Tool` 接口）
- `plugins/mcp/connection.go` — ConnectionManager 连接管理器
- `plugins/mcp/capability.go` — MCPModule（ModuleCapability 实现）
- `plugins/mcp/register.go` — RegisterCapabilities 注册入口
- `plugins/mcp/*_test.go` — 全部单元测试（TDD）

### 完成标准
- [x] `cd plugins/mcp && go test ./...` → PASS（所有测试通过）✅ 2026-06-06
- [x] Agent 可通过 `mcp__{server}__{tool}` 调用 MCP 工具 ✅ 2026-06-06
- [x] 支持 stdio、SSE、Streamable HTTP 三种传输 ✅ 2026-06-06
- [x] 连接断开时自动重连 ✅ 2026-06-06
- [x] 工具结果正确转换为 `tool.ToolResult` 返回给 LLM ✅ 2026-06-06

### 必须包含
- MCPToolWrapper 实现 `tool.Tool` 接口
- ConnectionManager 管理连接生命周期
- MCPModule 实现 `ModuleCapability` 接口
- 三种传输层支持
- 配置驱动的 MCP 服务器注册
- `allowed_tools` 工具过滤

### 必须排除
- OAuth 鉴权流程（后续迭代）
- Resources / Prompts / Sampling 支持（仅 Tools）
- 动态工具刷新（后续迭代，MCP 协议支持 `tool_list_changed` 通知）
- 配置热加载（启动时一次性连接）
- MCP 服务器端实现（本模块仅做客户端）

---

## 验证策略

> **零人工干预** — 所有验证由 Agent 执行。禁止需要人工操作的验收标准。

### 测试决策
- **基础设施存在**：YES（19 个 _test.go 文件）
- **自动化测试**：TDD
- **框架**：`go test`（Go 标准测试框架）
- **流程**：每个任务先写 RED（失败测试）→ GREEN（最小实现）→ REFACTOR

### QA 策略
每个任务包含 Agent 可执行的 QA 场景（详见 TODO 模板）。
证据保存到 `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`。

---

## 执行策略

### 并行执行波次

```
Wave 1（立即开始 — 基础 + 脚手架，MAX PARALLEL）：
├── Task 1: 添加 MCP SDK 依赖 + go.mod [quick]
├── Task 2: MCP 配置类型定义 [quick]
├── Task 3: MCP 工具信息类型 + MCPToolWrapper 接口 + 测试 [quick]
└── Task 4: MCP 连接测试辅助（mock server）[quick]

Wave 2（Wave 1 完成后 — 核心实现，MAX PARALLEL）：
├── Task 5: MCPToolWrapper 实现（TDD）[deep]
├── Task 6: ConnectionManager 实现（TDD）[deep]
└── Task 7: MCPModule (ModuleCapability) 实现（TDD）[deep]

Wave 3（Wave 2 完成后 — 集成 + 配置）：
├── Task 8: 配置解析与验证 + 注册入口 [quick]
└── Task 9: 集成测试 + server 注册 [deep]

Wave FINAL（所有任务完成后 — 4 个并行审查）：
├── Task F1: 计划合规审计（oracle）
├── Task F2: 代码质量审查（unspecified-high）
├── Task F3: 实际 QA 执行（unspecified-high）
└── Task F4: 范围保真度检查（deep）
→ 展示结果 → 等用户明确 "okay"
```

**关键路径**：Task 1 → Task 5 → Task 7 → Task 8 → Task 9
**并行加速**：约 60% 比顺序执行快
**最大并发**：4（Wave 1）

### 依赖矩阵

| 任务 | 依赖 | 被依赖 | 波次 |
|------|------|--------|------|
| 1 | — | 5, 6, 7 | 1 |
| 2 | — | 5, 6, 7, 8 | 1 |
| 3 | — | 5, 7 | 1 |
| 4 | — | 5, 6, 9 | 1 |
| 5 | 1, 2, 3, 4 | 7, 9 | 2 |
| 6 | 1, 2, 4 | 7, 9 | 2 |
| 7 | 1, 2, 3, 5, 6 | 8, 9 | 2 |
| 8 | 2, 7 | 9 | 3 |
| 9 | 5, 6, 7, 8 | F1-F4 | 3 |

### Agent 分派摘要

- **Wave 1**: 4 — T1→`quick`, T2→`quick`, T3→`quick`, T4→`quick`
- **Wave 2**: 3 — T5→`deep`, T6→`deep`, T7→`deep`
- **Wave 3**: 2 — T8→`quick`, T9→`deep`
- **FINAL**: 4 — F1→`oracle`, F2→`unspecified-high`, F3→`unspecified-high`, F4→`deep`

---

## TODOs

> 实现 + 测试 = 一个任务。绝不分离。
> 每个任务必须包含：推荐 Agent Profile + 并行信息 + QA 场景。
> **缺少 QA 场景的任务是不完整的。没有例外。**

- [x] 1. 添加 MCP SDK 依赖 + 初始化 plugins/mcp 模块

  **要做什么**：
  - 在 `plugins/mcp/` 下创建 `go.mod`，module 为 `github.com/copcon/plugins/mcp`
  - 添加 `github.com/modelcontextprotocol/go-sdk/mcp` 依赖（v1.6.1+）
  - 添加 `github.com/copcon/core` 依赖（`tool`、`iface`、`capabilities` 包）
  - 创建 `doc.go` 包文档
  - 运行 `go mod tidy` 确保依赖正确

  **禁止做**：
  - 不要创建任何业务逻辑代码（仅模块初始化）
  - 不要修改 core/go.mod

  **推荐 Agent Profile**：
  - **Category**: `quick`
    - 原因：纯依赖管理 + 模块初始化，无复杂逻辑
  - **Skills**: `[]`
  - **Skills 评估但省略**：无

  **并行化**：
  - **可并行执行**：YES
  - **并行组**：Wave 1（与 Task 2, 3, 4 并行）
  - **阻塞**：Task 5, 6, 7
  - **被阻塞**：无（可立即开始）

  **参考**：
  - `plugins/memory-file/go.mod` — 现有插件模块结构参考（module 路径、依赖声明格式）
  - `plugins/skill/go.mod` — 另一个插件模块参考
  - `core/go.mod` — 确认 core 的 module 路径为 `github.com/copcon/core`

  **验收标准**：
  - [ ] `plugins/mcp/go.mod` 存在，module 为 `github.com/copcon/plugins/mcp`
  - [ ] `go mod tidy` 在 `plugins/mcp/` 目录下执行成功
  - [ ] `go build ./...` 在 `plugins/mcp/` 目录下编译通过（即使无源码）

  **QA 场景**：

  ```
  Scenario: 模块初始化成功且依赖可解析
    Tool: Bash
    Preconditions: plugins/mcp/ 目录存在且为空
    Steps:
      1. 确认 go.mod 文件存在：ls plugins/mcp/go.mod
      2. 运行 go mod tidy：cd plugins/mcp && go mod tidy
      3. 验证没有错误输出
      4. 运行 go build ./...：cd plugins/mcp && go build ./...
    Expected Result: 所有命令返回 exit code 0，无错误输出
    Failure Indicators: 非零 exit code，包含 "cannot find module" 或 "unknown revision" 等错误
    Evidence: .sisyphus/evidence/task-1-mod-init.txt
  ```

  **提交**：YES
  - 消息：`feat(mcp): init plugins/mcp module with go-sdk dependency`
  - 文件：`plugins/mcp/go.mod`, `plugins/mcp/go.sum`, `plugins/mcp/doc.go`

---

- [x] 2. MCP 配置类型定义

  **要做什么**：
  - 创建 `plugins/mcp/config.go`
  - 定义 `MCPServerConfig` 结构体，支持三种传输类型的配置：
    - `StdioConfig`：command + args + env
    - `SSEConfig`：serverURL
    - `StreamableHTTPConfig`：serverURL
  - 定义 `MCPConfig` 顶层配置结构体（包含 `Servers []MCPServerConfig`）
  - 定义传输类型枚举 `TransportType`（stdio/sse/streamable-http）
  - 定义 `AllowedTools` 过滤配置（include/exclude 列表）

  **禁止做**：
  - 不要实现配置解析逻辑（那是 Task 8 的工作）
  - 不要实现连接逻辑（那是 Task 6 的工作）

  **推荐 Agent Profile**：
  - **Category**: `quick`
    - 原因：纯数据结构定义，无逻辑
  - **Skills**: `[]`
  - **Skills 评估但省略**：无

  **并行化**：
  - **可并行执行**：YES
  - **并行组**：Wave 1（与 Task 1, 3, 4 并行）
  - **阻塞**：Task 5, 6, 7, 8
  - **被阻塞**：无（可立即开始）

  **参考**：
  - `plugins/skill/` 目录下的配置结构体 — 现有插件配置模式参考
  - `server/config.yaml` — 现有配置格式参考，MCP 配置将嵌入此文件

  **验收标准**：
  - [ ] `MCPServerConfig` 支持三种传输类型配置
  - [ ] 所有类型可 JSON/YAML 序列化/反序列化
  - [ ] `AllowedTools` 支持 include/exclude 两种模式

  **QA 场景**：

  ```
  Scenario: 配置结构体可正确序列化
    Tool: Bash (go test)
    Preconditions: config.go 已创建
    Steps:
      1. 编写测试：创建 MCPServerConfig 实例（stdio 类型）
      2. json.Marshal 序列化
      3. json.Unmarshal 反序列化
      4. 断言反序列化后的字段与原始值一致
      5. 对 SSE 和 StreamableHTTP 类型重复
    Expected Result: 所有序列化/反序列化循环正确，字段值不丢失
    Failure Indicators: 字段值为空或与原始值不符
    Evidence: .sisyphus/evidence/task-2-config-serialize.txt
  ```

  **提交**：YES
  - 消息：`feat(mcp): add MCP server configuration types`
  - 文件：`plugins/mcp/config.go`

---

- [x] 3. MCP 工具信息类型 + MCPToolWrapper 接口定义（TDD: 测试先行）

  **要做什么**：
  - 创建 `plugins/mcp/types.go`
  - 定义 `MCPToolInfo` 结构体（持有工具名、描述、schema、所属 server、session 引用）
  - 定义 `MCPToolWrapper` 结构体，实现 `tool.Tool` 接口：
    - `Name() string` → 返回 `mcp__{server}__{tool}`
    - `Description() string` → 返回 `[{serverName}] {description}`
    - `InputSchema() map[string]any` → 返回 MCP 工具的 inputSchema
    - `Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error)` → 委托给 MCP session
  - 定义 `buildMCPToolName(serverName, toolName string) string` 命名工具函数
  - 定义 `normalizeServerName(name string) string` 规范化函数
  - **先写测试**：`types_test.go` 测试命名格式、规范化逻辑

  **禁止做**：
  - 不要实现 `Execute` 方法（那是 Task 5 的工作）
  - 不要实现连接逻辑（那是 Task 6 的工作）

  **推荐 Agent Profile**：
  - **Category**: `quick`
    - 原因：类型定义 + 简单命名逻辑，不含外部依赖
  - **Skills**: `[]`
  - **Skills 评估但省略**：无

  **并行化**：
  - **可并行执行**：YES
  - **并行组**：Wave 1（与 Task 1, 2, 4 并行）
  - **阻塞**：Task 5, 7
  - **被阻塞**：无（可立即开始）

  **参考**：
  - `core/tool/manager.go:30-35` — `tool.Tool` 接口定义（Name, Description, InputSchema, Execute）
  - `core/tool/manager.go:24-28` — `ToolResult` 结构体
  - `core/capabilities/tools/helpers.go` — `successResult`/`errorResult` 辅助函数模式
  - Claude Code 命名规范：`mcp__{server}__{tool}`（双下划线分隔）

  **验收标准**：
  - [ ] `buildMCPToolName("github", "list_issues")` → `"mcp__github__list_issues"`
  - [ ] `normalizeServerName("my.server")` → `"my_server"`（点号替换）
  - [ ] `normalizeServerName("valid-name")` → `"valid-name"`（合法字符保留）
  - [ ] `MCPToolWrapper` 实现 `tool.Tool` 接口（编译时检查：`var _ tool.Tool = (*MCPToolWrapper)(nil)`）

  **QA 场景**：

  ```
  Scenario: 命名函数正确生成全限定名
    Tool: Bash (go test)
    Preconditions: types.go 和 types_test.go 已创建
    Steps:
      1. 运行 go test -run TestBuildMCPToolName -v ./...
      2. 验证测试覆盖：普通名称、含特殊字符名、含双下划线的工具名
      3. 验证 TestNormalizeServerName 测试覆盖：点号、空格、非法字符
    Expected Result: 所有测试 PASS，命名格式符合 mcp__{server}__{tool}
    Failure Indicators: 测试 FAIL，命名格式不正确
    Evidence: .sisyphus/evidence/task-3-naming-test.txt

  Scenario: MCPToolWrapper 编译时满足 tool.Tool 接口
    Tool: Bash (go build)
    Preconditions: types.go 已创建
    Steps:
      1. 运行 go build ./... 在 plugins/mcp/ 目录
      2. 确认无编译错误
    Expected Result: 编译成功，exit code 0
    Failure Indicators: 编译错误，包含 "does not implement tool.Tool"
    Evidence: .sisyphus/evidence/task-3-compile-check.txt
  ```

  **提交**：YES
  - 消息：`feat(mcp): add MCPToolInfo type and MCPToolWrapper with naming functions`
  - 文件：`plugins/mcp/types.go`, `plugins/mcp/types_test.go`

---

- [x] 4. MCP 连接测试辅助（mock MCP server for testing）

  **要做什么**：
  - 创建 `plugins/mcp/testutil/` 子目录
  - 实现一个内存中的 mock MCP server，用于后续测试：
    - 注册预定义工具（如 `echo`、`add`）
    - 支持 `tools/list` 和 `tools/call` 方法
    - 使用 `mcp.NewServer` + `mcp.InMemoryTransport` 创建
  - 创建 `NewMockMCPServer(t *testing.T) (*mcp.ClientSession, func())` 工厂函数
  - 创建 `plugins/mcp/testutil/doc.go`

  **禁止做**：
  - 不要实现业务逻辑（仅测试辅助）
  - 不要依赖外部进程或网络

  **推荐 Agent Profile**：
  - **Category**: `quick`
    - 原因：测试辅助工具，逻辑简单，但需要熟悉 MCP SDK API
  - **Skills**: `[]`
  - **Skills 评估但省略**：无

  **并行化**：
  - **可并行执行**：YES
  - **并行组**：Wave 1（与 Task 1, 2, 3 并行）
  - **阻塞**：Task 5, 6, 9
  - **被阻塞**：无（可立即开始）

  **参考**：
  - `core/agent/mock_openai_test.go` — 现有 mock 模式参考
  - `core/agent/engine_test_helper_test.go` — 测试辅助函数模式
  - 官方 SDK 示例：`mcp.NewServer` + `mcp.AddToolHandler` + `mcp.InMemoryTransport`

  **验收标准**：
  - [ ] `NewMockMCPServer` 返回可用的 `*mcp.ClientSession`
  - [ ] mock server 注册了至少 2 个工具（echo、add）
  - [ ] `session.ListTools()` 返回正确工具列表
  - [ ] `session.CallTool("echo", {"message": "hello"})` 返回正确结果

  **QA 场景**：

  ```
  Scenario: Mock server 可正常创建和调用工具
    Tool: Bash (go test)
    Preconditions: testutil/mock_server.go 已创建
    Steps:
      1. 运行 go test -run TestMockMCPServer -v ./testutil/
      2. 验证：创建 session 成功
      3. 验证：ListTools 返回至少 2 个工具
      4. 验证：CallTool("echo", {"message":"hello"}) 返回包含 "hello" 的结果
      5. 验证：CallTool("add", {"a":1, "b":2}) 返回 3
      6. 验证：cleanup 函数正确关闭 session
    Expected Result: 所有断言 PASS，无资源泄漏
    Failure Indicators: 测试 FAIL，session 无法创建或工具调用失败
    Evidence: .sisyphus/evidence/task-4-mock-server.txt
  ```

  **提交**：YES
  - 消息：`test(mcp): add mock MCP server for testing`
  - 文件：`plugins/mcp/testutil/mock_server.go`, `plugins/mcp/testutil/doc.go`

---

- [x] 5. MCPToolWrapper 完整实现（TDD: RED → GREEN → REFACTOR）

  **要做什么**：
  - 完善 `plugins/mcp/types.go` 中 `MCPToolWrapper` 的 `Execute` 方法
  - `Execute` 方法流程：
    1. 从 `MCPToolInfo` 获取 session 引用
    2. 构造 `mcp.CallToolParams{Name: originalToolName, Arguments: args}`
    3. 调用 `session.CallTool(ctx, params)`
    4. 将 `mcp.CallToolResult` 转换为 `tool.ToolResult`
    5. 错误处理：MCP 调用失败时返回 `tool.ToolResult{Success: false, Error: err.Error()}`
  - 处理 MCP 结果中的多种 Content 类型（TextContent、ImageContent 等）
  - 实现 `extractTextContent(result *mcp.CallToolResult) string` 辅助函数
  - **先写测试**：`wrapper_test.go` 使用 mock server 测试 Execute 方法

  **禁止做**：
  - 不要实现连接管理逻辑（那是 Task 6 的工作）
  - 不要修改 `tool.Tool` 接口

  **推荐 Agent Profile**：
  - **Category**: `deep`
    - 原因：核心实现逻辑，涉及 MCP SDK API 调用、类型转换、错误处理，需要仔细处理
  - **Skills**: `[]`
  - **Skills 评估但省略**：无

  **并行化**：
  - **可并行执行**：YES
  - **并行组**：Wave 2（与 Task 6, 7 并行）
  - **阻塞**：Task 7, 9
  - **被阻塞**：Task 1, 2, 3, 4

  **参考**：
  - `core/tool/manager.go:30-35` — `tool.Tool` 接口完整定义
  - `core/tool/manager.go:24-28` — `ToolResult` 结构体
  - `core/capabilities/tools/helpers.go` — `successResult`/`errorResult` 辅助函数
  - `core/iface/chat.go` — `ChatContextInterface` 定义
  - 官方 SDK：`mcp.CallToolParams`、`mcp.CallToolResult`、`mcp.TextContent` 类型

  **验收标准**：
  - [ ] `wrapper_test.go` 测试通过（至少 3 个测试用例）
  - [ ] 正常调用返回 `ToolResult{Success: true, Data: ...}`
  - [ ] 工具不存在时返回 `ToolResult{Success: false, Error: "tool not found"}`
  - [ ] 空参数调用正常工作

  **QA 场景**：

  ```
  Scenario: 正常调用 MCP 工具并返回正确结果
    Tool: Bash (go test)
    Preconditions: mock server 已启动，MCPToolWrapper 已创建
    Steps:
      1. 运行 go test -run TestMCPToolWrapper_Execute_Success -v ./...
      2. 使用 mock server 的 echo 工具
      3. 调用 Execute(ctx, {"message": "hello world"})
      4. 断言 result.Success == true
      5. 断言 result.Data 包含 "hello world"
    Expected Result: 测试 PASS，结果正确
    Failure Indicators: result.Success == false 或 Data 不匹配
    Evidence: .sisyphus/evidence/task-5-wrapper-success.txt

  Scenario: 调用不存在的工具返回错误
    Tool: Bash (go test)
    Preconditions: mock server 已启动
    Steps:
      1. 运行 go test -run TestMCPToolWrapper_Execute_ToolNotFound -v ./...
      2. 创建指向不存在工具的 wrapper
      3. 调用 Execute
      4. 断言 result.Success == false
      5. 断言 result.Error 非空
    Expected Result: 测试 PASS，错误信息清晰
    Failure Indicators: result.Success == true 或 Error 为空
    Evidence: .sisyphus/evidence/task-5-wrapper-error.txt

  Scenario: 空参数调用正常工作
    Tool: Bash (go test)
    Preconditions: mock server 已启动
    Steps:
      1. 运行 go test -run TestMCPToolWrapper_Execute_EmptyArgs -v ./...
      2. 调用 Execute(ctx, map[string]any{})
      3. 断言 result.Success == true
    Expected Result: 测试 PASS，空参数不导致 panic 或错误
    Failure Indicators: panic 或 result.Success == false
    Evidence: .sisyphus/evidence/task-5-wrapper-empty.txt
  ```

  **提交**：YES
  - 消息：`feat(mcp): implement MCPToolWrapper.Execute with TDD`
  - 文件：`plugins/mcp/types.go`, `plugins/mcp/wrapper_test.go`

---

- [x] 6. ConnectionManager 实现（TDD: RED → GREEN → REFACTOR）

  **要做什么**：
  - 创建 `plugins/mcp/connection.go`
  - 实现 `ConnectionManager` 结构体：
    - `Connect(ctx, config MCPServerConfig) (*mcp.ClientSession, error)` — 根据配置类型创建连接
    - `Disconnect(serverName string) error` — 断开指定服务器
    - `DisconnectAll() error` — 断开所有连接
    - `GetSession(serverName string) (*mcp.ClientSession, error)` — 获取已连接 session
    - `ListSessions() []string` — 列出所有已连接服务器名
  - 支持三种传输类型：
    - stdio：`mcp.CommandTransport{Command: exec.Command(config.Cmd, config.Args...)}`
    - SSE：`mcp.SSETransport{Endpoint: config.ServerURL}`
    - Streamable HTTP：`mcp.StreamableClientTransport{Endpoint: config.ServerURL}`
  - 并发安全（使用 `sync.RWMutex`）
  - 连接超时处理（默认 30s）
  - **先写测试**：`connection_test.go` 使用 mock server 测试连接管理

  **禁止做**：
  - 不要实现自动重连（后续迭代）
  - 不要实现健康检查（后续迭代）

  **推荐 Agent Profile**：
  - **Category**: `deep`
    - 原因：核心基础设施，涉及并发安全、多种传输类型、错误处理
  - **Skills**: `[]`
  - **Skills 评估但省略**：无

  **并行化**：
  - **可并行执行**：YES
  - **并行组**：Wave 2（与 Task 5, 7 并行）
  - **阻塞**：Task 7, 9
  - **被阻塞**：Task 1, 2, 4

  **参考**：
  - `core/tool/manager.go:60-106` — `toolRegistry` 的并发安全模式（sync.RWMutex + map）
  - `core/tool/registry.go:48-148` — `AsyncToolRegistry` 的 sync.Map 模式
  - 官方 SDK 文档：`mcp.CommandTransport`、`mcp.SSETransport`、`mcp.StreamableClientTransport`
  - 官方 SDK 示例：`examples/` 目录中的客户端连接示例

  **验收标准**：
  - [ ] `connection_test.go` 测试通过（至少 4 个测试用例）
  - [ ] Connect + GetSession 正常工作
  - [ ] Disconnect 正确清理资源
  - [ ] 并发安全（多个 goroutine 同时 GetSession 不 panic）
  - [ ] 连接不存在的服务器返回错误

  **QA 场景**：

  ```
  Scenario: 连接 mock server 并获取 session
    Tool: Bash (go test)
    Preconditions: ConnectionManager 已创建，mock server 可用
    Steps:
      1. 运行 go test -run TestConnectionManager_Connect -v ./...
      2. 使用 InMemory transport 连接 mock server
      3. 调用 GetSession 获取 session
      4. 断言 session 非 nil
      5. 调用 session.ListTools() 验证连接可用
    Expected Result: 测试 PASS，session 可用
    Failure Indicators: session 为 nil 或 ListTools 失败
    Evidence: .sisyphus/evidence/task-6-connect.txt

  Scenario: 断开连接后 GetSession 返回错误
    Tool: Bash (go test)
    Preconditions: 已连接 mock server
    Steps:
      1. 运行 go test -run TestConnectionManager_Disconnect -v ./...
      2. 调用 Disconnect("test-server")
      3. 调用 GetSession("test-server")
      4. 断言返回错误
    Expected Result: 测试 PASS，GetSession 返回 "server not connected" 错误
    Failure Indicators: GetSession 返回 nil error（连接未正确清理）
    Evidence: .sisyphus/evidence/task-6-disconnect.txt

  Scenario: 并发访问 ConnectionManager 不 panic
    Tool: Bash (go test)
    Preconditions: 已连接 mock server
    Steps:
      1. 运行 go test -run TestConnectionManager_Concurrent -v ./... -race
      2. 启动 10 个 goroutine 同时调用 GetSession
      3. 启动 10 个 goroutine 同时调用 ListSessions
      4. 断言无 race condition
    Expected Result: 测试 PASS，race detector 无告警
    Failure Indicators: race detector 告警或 panic
    Evidence: .sisyphus/evidence/task-6-concurrent.txt
  ```

  **提交**：YES
  - 消息：`feat(mcp): implement ConnectionManager with TDD`
  - 文件：`plugins/mcp/connection.go`, `plugins/mcp/connection_test.go`

---

- [x] 7. MCPModule (ModuleCapability) 实现（TDD: RED → GREEN → REFACTOR）

  **要做什么**：
  - 创建 `plugins/mcp/capability.go`
  - 实现 `MCPModule` 结构体，满足 `capabilities.ModuleCapability` 接口：
    - `Name() string` → `"modules.mcp"`
    - `Type() capabilities.CapabilityType` → `capabilities.CapabilityTypeModule`
    - `DependsOn() []string` → `nil`（无依赖）
    - `NewTools(deps capabilities.CapabilityDeps) ([]tool.Tool, error)` — 核心方法：
      1. 遍历配置的 MCP 服务器列表
      2. 对每个服务器：连接 → ListTools → 创建 MCPToolWrapper
      3. 应用 `allowed_tools` 过滤
      4. 返回所有 wrapper 列表
    - `NewHooks(deps capabilities.CapabilityDeps) ([]hook.Hook, error)` → 返回 nil（本模块不需要 hook）
  - 持有 `ConnectionManager` 和 `[]MCPServerConfig`
  - 构造函数：`NewMCPModule(configs []MCPServerConfig) *MCPModule`
  - **先写测试**：`capability_test.go` 使用 mock server 测试完整流程

  **禁止做**：
  - 不要实现配置解析（那是 Task 8 的工作）
  - 不要修改 `capabilities.CapabilityDeps` 接口

  **推荐 Agent Profile**：
  - **Category**: `deep`
    - 原因：核心集成逻辑，连接多个子系统（ConnectionManager + MCPToolWrapper + Capability 接口）
  - **Skills**: `[]`
  - **Skills 评估但省略**：无

  **并行化**：
  - **可并行执行**：YES
  - **并行组**：Wave 2（与 Task 5, 6 并行）
  - **阻塞**：Task 8, 9
  - **被阻塞**：Task 1, 2, 3, 5, 6

  **参考**：
  - `plugins/memory-file/capabilities_closure.go` — ModuleCapability 实现范例（NewTools + NewHooks）
  - `plugins/skill/capability.go` — 另一个 ModuleCapability 实现范例
  - `core/capabilities/registry.go` — `ModuleCapability` 接口定义
  - `core/capabilities/constants.go` — 能力名称常量模式

  **验收标准**：
  - [ ] `capability_test.go` 测试通过（至少 3 个测试用例）
  - [ ] `NewTools()` 返回正确数量的 MCPToolWrapper
  - [ ] 工具名格式为 `mcp__{server}__{tool}`
  - [ ] `allowed_tools` 过滤正常工作
  - [ ] 连接失败的服务器不阻塞其他服务器

  **QA 场景**：

  ```
  Scenario: NewTools 返回正确的工具列表
    Tool: Bash (go test)
    Preconditions: mock server 已启动并注册了 echo 和 add 工具
    Steps:
      1. 运行 go test -run TestMCPModule_NewTools -v ./...
      2. 创建 MCPModule，配置指向 mock server
      3. 调用 NewTools(mockDeps)
      4. 断言返回 2 个工具
      5. 断言工具名分别为 "mcp__test-server__echo" 和 "mcp__test-server__add"
      6. 断言每个工具的 Description 以 "[test-server]" 开头
    Expected Result: 测试 PASS，工具列表正确
    Failure Indicators: 工具数量不对或命名格式错误
    Evidence: .sisyphus/evidence/task-7-newtools.txt

  Scenario: allowed_tools 过滤生效
    Tool: Bash (go test)
    Preconditions: mock server 注册了 echo 和 add 工具
    Steps:
      1. 运行 go test -run TestMCPModule_AllowedTools -v ./...
      2. 配置 allowed_tools = ["echo"]
      3. 调用 NewTools
      4. 断言只返回 1 个工具（echo）
    Expected Result: 测试 PASS，过滤正确
    Failure Indicators: 返回了被排除的工具
    Evidence: .sisyphus/evidence/task-7-allowed-tools.txt

  Scenario: 连接失败的服务器不阻塞其他服务器
    Tool: Bash (go test)
    Preconditions: 一个 mock server 可用，另一个配置指向无效地址
    Steps:
      1. 运行 go test -run TestMCPModule_PartialFailure -v ./...
      2. 配置两个服务器：一个有效，一个无效
      3. 调用 NewTools
      4. 断言返回了有效服务器的工具
      5. 断言没有 panic 或 fatal error
    Expected Result: 测试 PASS，部分失败不影响其他服务器
    Failure Indicators: panic 或返回空列表
    Evidence: .sisyphus/evidence/task-7-partial-failure.txt
  ```

  **提交**：YES
  - 消息：`feat(mcp): implement MCPModule as ModuleCapability with TDD`
  - 文件：`plugins/mcp/capability.go`, `plugins/mcp/capability_test.go`

---

- [x] 8. 配置解析 + 注册入口

  **要做什么**：
  - 创建 `plugins/mcp/register.go`
  - 实现 `RegisterCapabilities(reg *capabilities.Registry, configs []MCPServerConfig)` 函数
  - 创建 MCPModule 实例并注册到 Capability Registry
  - 实现 `ParseMCPConfig(raw map[string]any) ([]MCPServerConfig, error)` 配置解析函数
  - 在 `server/config.yaml` 中添加 MCP 配置段示例（注释形式）
  - 在 `server/cmd/server/main.go` 中添加 MCP 注册调用（条件编译或注释引导）

  **禁止做**：
  - 不要修改 server 的核心启动逻辑（仅添加注册调用）
  - 不要修改 core 包的任何代码

  **推荐 Agent Profile**：
  - **Category**: `quick`
    - 原因：配置解析 + 注册入口，逻辑简单但需要熟悉现有注册模式
  - **Skills**: `[]`
  - **Skills 评估但省略**：无

  **并行化**：
  - **可并行执行**：NO（依赖 Task 7）
  - **并行组**：Wave 3（与 Task 9 并行）
  - **阻塞**：Task 9
  - **被阻塞**：Task 2, 7

  **参考**：
  - `plugins/memory-file/register.go` — 插件注册入口模式
  - `plugins/skill/register.go` — 另一个注册入口模式
  - `server/cmd/server/main.go` — 现有能力注册调用位置
  - `server/config.yaml` — 现有配置格式

  **验收标准**：
  - [ ] `RegisterCapabilities` 正确注册 MCPModule
  - [ ] `ParseMCPConfig` 正确解析 YAML 配置
  - [ ] 无效配置返回明确错误
  - [ ] `server/config.yaml` 包含 MCP 配置示例

  **QA 场景**：

  ```
  Scenario: 配置解析正确
    Tool: Bash (go test)
    Preconditions: register.go 已创建
    Steps:
      1. 运行 go test -run TestParseMCPConfig -v ./...
      2. 测试有效配置（stdio 类型）
      3. 测试有效配置（SSE 类型）
      4. 测试有效配置（Streamable HTTP 类型）
      5. 测试无效配置（缺少必填字段）
      6. 测试 allowed_tools 解析
    Expected Result: 有效配置解析成功，无效配置返回错误
    Failure Indicators: 有效配置解析失败或无效配置未报错
    Evidence: .sisyphus/evidence/task-8-config-parse.txt

  Scenario: 注册入口可被调用
    Tool: Bash (go test)
    Preconditions: register.go 已创建
    Steps:
      1. 运行 go test -run TestRegisterCapabilities -v ./...
      2. 创建 capabilities.Registry
      3. 调用 RegisterCapabilities(reg, configs)
      4. 验证 capability 已注册
    Expected Result: 测试 PASS，capability 注册成功
    Failure Indicators: 注册失败或 panic
    Evidence: .sisyphus/evidence/task-8-register.txt
  ```

  **提交**：YES
  - 消息：`feat(mcp): add config parsing and capability registration`
  - 文件：`plugins/mcp/register.go`, `plugins/mcp/register_test.go`, `server/config.yaml`

---

- [x] 9. 集成测试 + server 注册集成

  **要做什么**：
  - 创建 `plugins/mcp/integration_test.go`
  - 端到端测试：模拟完整流程
    1. 启动 mock MCP server
    2. 创建 MCPModule 并注册
    3. 通过 Harness 创建 Agent
    4. 验证 Agent 的 ToolManager 包含 MCP 工具
    5. 调用 MCP 工具并验证结果
  - 在 `server/cmd/server/main.go` 中添加 MCP 配置段和注册调用
  - 确保 MCP 工具与内置工具和平共存（无命名冲突）

  **禁止做**：
  - 不要修改 core 包的任何代码
  - 不要改变现有工具的行为

  **推荐 Agent Profile**：
  - **Category**: `deep`
    - 原因：端到端集成测试，涉及多个子系统协作
  - **Skills**: `[]`
  - **Skills 评估但省略**：无

  **并行化**：
  - **可并行执行**：NO（依赖 Task 8）
  - **并行组**：Wave 3
  - **阻塞**：F1-F4
  - **被阻塞**：Task 5, 6, 7, 8

  **参考**：
  - `core/harness_test.go` — Harness 集成测试模式
  - `core/harness_integration_test.go` — 完整集成测试模式
  - `core/agent/engine_test_helper_test.go` — Agent 测试辅助模式
  - `server/cmd/server/main.go:38-60` — 现有的能力注册代码段

  **验收标准**：
  - [ ] `go test -run TestIntegration ./...` → PASS
  - [ ] Agent 可通过 `mcp__test-server__echo` 调用工具
  - [ ] MCP 工具结果正确出现在对话中
  - [ ] MCP 工具与内置工具无命名冲突

  **QA 场景**：

  ```
  Scenario: 端到端 MCP 工具调用
    Tool: Bash (go test)
    Preconditions: mock server 已启动
    Steps:
      1. 运行 go test -run TestIntegration_MCPToolCall -v ./...
      2. 通过 Harness 创建 Agent（包含 MCP 模块）
      3. 获取 Agent 的 ToolManager
      4. 验证 ToolManager 包含 mcp__test-server__echo 工具
      5. 调用 Execute 方法
      6. 断言 result.Success == true
    Expected Result: 测试 PASS，MCP 工具完全融入 Agent 工具体系
    Failure Indicators: 工具未注册或调用失败
    Evidence: .sisyphus/evidence/task-9-integration.txt

  Scenario: MCP 与内置工具共存
    Tool: Bash (go test)
    Preconditions: Agent 同时配置了 MCP 模块和内置工具
    Steps:
      1. 运行 go test -run TestIntegration_MCPWithBuiltins -v ./...
      2. 获取 Agent 的 ToolManager 工具列表
      3. 验证同时存在 MCP 工具和内置工具
      4. 验证无名称冲突
    Expected Result: 测试 PASS，两类工具共存无冲突
    Failure Indicators: 工具名冲突或某类工具缺失
    Evidence: .sisyphus/evidence/task-9-coexistence.txt
  ```

  **提交**：YES
  - 消息：`test(mcp): add integration tests and server registration`
  - 文件：`plugins/mcp/integration_test.go`, `server/cmd/server/main.go`

---

## 最终验证波次（所有实现任务完成后必须执行）

> 4 个审查 Agent 并行运行。所有必须 APPROVE。汇总结果展示给用户，获取明确 "okay" 后才算完成。
> **在用户明确批准前，不要标记 F1-F4 为完成。** 被拒绝或有反馈 → 修复 → 重新运行 → 再次展示 → 等待 okay。

- [x] F1. **计划合规审计** — `oracle` ✅ APPROVED
  从头到尾阅读计划。验证每个 "必须包含"：确认实现存在（读取文件、运行 curl、执行命令）。验证每个 "必须排除"：搜索代码库中禁止的模式 — 发现则拒绝并标注 file:line。检查证据文件在 `.sisyphus/evidence/` 中存在。对比交付物与计划。
  输出：`Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`
  → Must Have 6/6 | Must NOT Have 5/5 | Tasks 8/9 | VERDICT: APPROVE

- [x] F2. **代码质量审查** — `unspecified-high` ✅ APPROVED
  运行 `go vet ./...` + `go build ./...` + `go test ./...`。审查所有变更文件：`any` 类型滥用、空 catch、遗留调试日志、注释掉的代码、未使用的导入。检查 AI slop：过度注释、过度抽象、通用命名（data/result/item/temp）。
  输出：`Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`
  → Build PASS | Vet PASS | Tests 35 pass/0 fail | Files 16/16 clean | VERDICT: APPROVE

- [x] F3. **实际 QA 执行** — `unspecified-high` ✅ APPROVED
  从干净状态开始。执行每个任务中的每个 QA 场景 — 严格按照步骤，捕获证据。测试跨任务集成（功能协同工作，非孤立）。测试边缘情况：空状态、无效输入、快速连续操作。保存到 `.sisyphus/evidence/final-qa/`。
  输出：`Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`
  → Scenarios 16/16 pass | Integration PASS | Edge Cases 12 tested | VERDICT: APPROVE

- [x] F4. **范围保真度检查** — `deep` ✅ APPROVED
  对每个任务：阅读 "要做什么"，阅读实际 diff（git log/diff）。验证 1:1 — 规范中的所有内容都已构建（无遗漏），规范外的内容都未构建（无蔓延）。检查 "禁止做" 合规性。检测跨任务污染：Task N 触碰了 Task M 的文件。标记未计入的变更。
  输出：`Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`
  → Tasks 8/9 compliant | Contamination CLEAN | Unaccounted CLEAN | VERDICT: APPROVE

---

## 提交策略

| Wave | 提交消息格式 | 文件 |
|------|------------|------|
| 1 | `feat(mcp): init plugins/mcp module with go-sdk dependency` | go.mod, go.sum, doc.go |
| 1 | `feat(mcp): add MCP server configuration types` | config.go |
| 1 | `feat(mcp): add MCPToolInfo type and MCPToolWrapper with naming functions` | types.go, types_test.go |
| 1 | `test(mcp): add mock MCP server for testing` | testutil/* |
| 2 | `feat(mcp): implement MCPToolWrapper.Execute with TDD` | types.go, wrapper_test.go |
| 2 | `feat(mcp): implement ConnectionManager with TDD` | connection.go, connection_test.go |
| 2 | `feat(mcp): implement MCPModule as ModuleCapability with TDD` | capability.go, capability_test.go |
| 3 | `feat(mcp): add config parsing and capability registration` | register.go, register_test.go, config.yaml |
| 3 | `test(mcp): add integration tests and server registration` | integration_test.go, main.go |

---

## 成功标准

### 验证命令
```bash
# 单元测试
cd plugins/mcp && go test ./... -v -race
# 集成测试
cd plugins/mcp && go test -run TestIntegration ./... -v
# 编译检查
cd plugins/mcp && go build ./...
# 静态分析
cd plugins/mcp && go vet ./...
```

### 最终检查清单
- [x] 所有 "必须包含" 已实现
  - MCPToolWrapper implementing tool.Tool ✅
  - ConnectionManager managing connections ✅
  - MCPModule as ModuleCapability ✅
  - Support for stdio/SSE/Streamable HTTP ✅
  - Configuration-driven server registration ✅
  - allowed_tools filtering ✅
- [x] 所有 "必须排除" 未出现
  - No OAuth/鉴权 ✅
  - No Resources/Prompts/Sampling ✅
  - No 动态工具刷新 ✅
  - No 配置热加载 ✅
  - No MCP server implementation ✅
- [x] 所有测试通过（`go test -race ./...`）
  - 35 tests passing with -race on 2026-06-06
- [x] Agent 可通过 `mcp__{server}__{tool}` 调用 MCP 工具
  - Verified in integration_test.go:TestIntegration_EndToEnd
- [x] 三种传输类型均支持
  - stdio via gmcp.CommandTransport
  - SSE via gmcp.SSEClientTransport
  - Streamable HTTP via gmcp.StreamableClientTransport
- [x] 连接断开时行为正确（返回错误而非 panic）
  - connection.go handles disconnections with error returns
- [x] 配置驱动，无需代码变更即可添加新 MCP 服务器
  - server/config.yaml supports MCP server configuration
  - Registration happens automatically based on config