# Skill 能力实现

## TL;DR

> **Quick Summary**: 实现 Skill 能力插件（plugins/skill/），遵循 Agent Skills Specification 标准，支持 Agent 在运行时加载和使用专业化 Skill 指令。核心类型定义在 core，具体实现作为插件，Server 层显式注册。
>
> **Deliverables**:
> - `core/capabilities/skill/types.go` — Skill 核心类型定义
> - `plugins/skill/` — 完整 Skill 插件（解析器、发现器、Hook、Tool、注册入口）
> - `server/internal/config/config.go` — SkillConfig 配置扩展
> - `server/cmd/server/main.go` — Server 层注册调用
>
> **Estimated Effort**: Medium
> **Parallel Execution**: YES — 3 waves
> **Critical Path**: Task 1 (类型) → Task 4 (解析器) → Task 7 (发现器) → Task 8 (Module) → Task 9 (注册) → Task 10 (集成)

---

## Context

### Original Request
用户要求新增 Skill 能力，涉及三个层面：
1. 对 SKILL.md 文件的解析
2. Skill 说明的注入（Hook，在 agent 会话开始时注入 skill 列表）
3. Skill 的信息获取（Tool，agent 运行时查询/激活 skill）

### Interview Summary
**Key Discussions**:
- Skill 代码位置：plugins/skill/ 作为插件，core/capabilities/skill/types.go 仅放核心类型
- 注册入口：Server 层显式调用 `skill.RegisterCapabilities()`
- 发现路径：默认路径（.copcon/skills/ 等）+ config.yaml 配置路径，配置优先
- 实现模式：ModuleCapability 产出 1 个 Hook + 1 个 Tool
- Skill 格式：遵循 Agent Skills Specification（YAML Frontmatter + Markdown Body）

**Research Findings**:
- Agent Skills Specification (SKILL.md) 是行业标准，25+ 框架采用
- 性能方面：渐进式披露（L1 元数据 → L2 指令 → L3 资源），避免 token 浪费
- 标准字段：name, description, license, metadata, allowed-tools
- 标准目录：skill-name/SKILL.md + 可选 scripts/, references/, assets/

### Metis Review
**Identified Gaps** (addressed):
- **CapabilityTypeSkill 是空壳**：harness 中 `CapabilityTypeSkill` 类型无处理分支，SkillModule 必须使用 `CapabilityTypeModule`（ModuleCapability 接口检查会先于 type switch 捕获）
- Tool 拆分方案：采用单工具 + action 参数（list/get/search），避免工具爆炸
- Per-agent 过滤：本次不做，所有 agent 共享所有 skill
- 解析时机：Build 时解析（Discover 在 NewHooks 中调用），运行时仅内存查询
- Allowed-tools：信息传递（注入到 skill 描述中），不强制执行
- 边缘场景：编码校验、并发安全、空目录、缺失 SKILL.md、大文件截断

---

## Work Objectives

### Core Objective
实现完整的 Skill 能力插件，使 Agent 能够在运行时发现、查询和激活 Skill 指令。

### Concrete Deliverables
- `core/capabilities/skill/types.go` — Skill, SkillSummary, ResourceFile 类型
- `core/capabilities/constants.go` — 新增 CapSkillsModule 常量
- `plugins/skill/parser.go` — SKILL.md 解析器
- `plugins/skill/discover.go` — 多路径 Skill 发现 + 去重
- `plugins/skill/hook.go` — SkillInfoHook（OnSystemPrompt 注入）
- `plugins/skill/tool.go` — skill 工具（list/get/search）
- `plugins/skill/capability.go` — SkillModule（ModuleCapability）
- `plugins/skill/register.go` — 注册入口
- `server/internal/config/config.go` — SkillConfig 扩展
- `server/cmd/server/main.go` — Server 层注册调用

### Definition of Done
- [ ] `go build ./...` 通过（core + plugins + server）
- [ ] `go test ./plugins/skill/...` 所有测试通过
- [ ] 在 `.copcon/skills/` 下放置示例 SKILL.md，启动 server 后 skill 列表出现在 system prompt 中
- [ ] Agent 可通过 `skill` 工具查询和获取 skill 指令

### Must Have
- SKILL.md 解析（YAML frontmatter + Markdown body）
- 多路径 Skill 发现 + 同名去重
- System prompt 注入 skill 列表（L1 元数据）
- skill 工具（list / get / search）
- 配置支持（SkillConfig.Enabled + ExtraPaths）

### Must NOT Have (Guardrails)
- **MCP 集成**：本次不集成 MCP 服务器
- **Skill 热加载**：仅在 Build 时解析，不支持运行时动态添加
- **Per-agent 过滤**：所有 agent 看到所有 skill
- **Allowed-tools 强制执行**：仅作为信息传递，不拦截工具调用
- **Skill 市场/远程下载**：仅本地文件系统
- **AI slop**：不过度抽象，不引入不必要的接口层
- **CapabilityTypeSkill 误用**：SkillModule.Type() 必须返回 `CapabilityTypeModule`，因为 harness 不处理 `CapabilityTypeSkill` 类型

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES (testify v1.11.1)
- **Automated tests**: YES (TDD)
- **Framework**: testify (assert + require)
- **TDD Flow**: Each task follows RED (failing test) → GREEN (minimal impl) → REFACTOR
- **Test files**: parser_test.go, discover_test.go, hook_test.go, tool_test.go — each written BEFORE implementation

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **CLI**: Use Bash (go test) — Run tests, assert output
- **API/Backend**: Use Bash (curl) — Send requests, assert status + response fields
- **Library/Module**: Use Bash (go test -run) — Run specific test, verify assertions

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — foundation):
├── Task 1: Core 类型定义 (core/capabilities/skill/types.go) [quick]
├── Task 2: 常量定义 (core/capabilities/constants.go) [quick]
└── Task 3: Config 扩展 (server/internal/config/config.go) [quick]

Wave 2 (After Wave 1 — core implementation, MAX PARALLEL):
├── Task 4: 解析器 (plugins/skill/parser.go) [quick]
├── Task 5: SkillInfoHook (plugins/skill/hook.go) [quick]
└── Task 6: skill 工具 (plugins/skill/tool.go) [quick]

Wave 3 (After Wave 2 — integration, MAX PARALLEL):
├── Task 7: 发现器 (plugins/skill/discover.go) depends: 4 [quick]
├── Task 8: SkillModule (plugins/skill/capability.go) depends: 5, 6, 7 [quick]
└── Task 9: 注册入口 (plugins/skill/register.go) depends: 8 [quick]

Wave 4 (After Wave 3 — server integration):
├── Task 10: Server 集成 (server/cmd/server/main.go) depends: 3, 9 [quick]
└── Task 11: 测试 (plugins/skill/*_test.go) depends: 4-9 [quick]

Critical Path: Task 1 → Task 4 → Task 7 → Task 8 → Task 9 → Task 10
Parallel Speedup: ~60% faster than sequential
Max Concurrent: 3 (Waves 2 & 3)
```

---

## TODOs

- [x] 1. Core 类型定义

  **What to do**:
  - 创建 `core/capabilities/skill/types.go`
  - 定义 `Skill` 结构体：Name, Description, License, Metadata, AllowedTools, Instructions, DirPath, Source, ResourceFiles
  - 定义 `ResourceFile` 结构体：Name, Path, Category
  - 定义 `SkillSummary` 结构体：Name, Description, Source（L1 级别摘要）

  **Must NOT do**:
  - 不引入任何插件依赖（core 包应保持纯净）
  - 不定义接口（仅数据结构）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 纯数据结构定义，无复杂逻辑
  - **Skills**: `[]`
  - **Skills Evaluated but Omitted**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3)
  - **Blocks**: Tasks 4, 5, 6, 7
  - **Blocked By**: None (can start immediately)

  **References**:
  - `plugins/knowledge-base/types/knowledge.go` — 现有插件类型定义模式（struct 组织方式）
  - `plugins/memory-file/types/memory.go` — 现有插件类型定义模式
  - **WHY**: 这两个文件展示了项目中的类型定义惯例——简洁的 struct，无过度抽象

  **Acceptance Criteria**:
  - [ ] 文件创建：`core/capabilities/skill/types.go`
  - [ ] `go build ./core/...` → PASS

  **QA Scenarios**:

  ```
  Scenario: 类型编译通过
    Tool: Bash (go build)
    Steps:
      1. Run: go build ./core/capabilities/skill/
    Expected Result: 编译成功，无错误
    Failure Indicators: 编译错误信息
    Evidence: .sisyphus/evidence/task-1-build.txt

  Scenario: 类型可被 plugins 引用
    Tool: Bash (go build)
    Steps:
      1. 在 plugins/skill/ 下创建临时文件 import "github.com/copcon/core/capabilities/skill"
      2. 使用 Skill, SkillSummary, ResourceFile 类型
      3. Run: go build
    Expected Result: 编译成功
    Failure Indicators: import 失败或类型未找到
    Evidence: .sisyphus/evidence/task-1-import.txt
  ```

  **Commit**: YES (groups in Commit 1)
  - Message: `feat(skill): add core skill types`
  - Files: `core/capabilities/skill/types.go`

- [x] 2. 常量定义

  **What to do**:
  - 在 `core/capabilities/constants.go` 中添加 `CapSkillsModule = "modules.skills"` 常量
  - 放在现有 Module 常量区块（`CapMemoryFile`、`CapKBModule` 附近）

  **Must NOT do**:
  - 不修改现有常量
  - 不添加 `WildcardSkills`（已存在）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单行常量添加
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3)
  - **Blocks**: Task 8
  - **Blocked By**: None

  **References**:
  - `core/capabilities/constants.go:15-18` — 现有 Module 常量区块（`CapMemoryFile`, `CapKBModule`）
  - **WHY**: 新常量应放在同一区块，保持代码组织一致

  **Acceptance Criteria**:
  - [ ] 常量添加：`core/capabilities/constants.go` 中 `CapSkillsModule = "modules.skills"`
  - [ ] `go build ./core/...` → PASS

  **QA Scenarios**:

  ```
  Scenario: 常量可被引用
    Tool: Bash (go test)
    Steps:
      1. Run: go test ./core/capabilities/ -run TestWildcardSkills -v
    Expected Result: 测试通过（`skills.*` 通配符能展开 `modules.skills`）
    Failure Indicators: 测试失败
    Evidence: .sisyphus/evidence/task-2-constant.txt
  ```

  **Commit**: YES (groups in Commit 1)
  - Message: `feat(skill): add CapSkillsModule constant`
  - Files: `core/capabilities/constants.go`

- [x] 3. Config 扩展

  **What to do**:
  - 在 `server/internal/config/config.go` 的 `Config` 结构体中添加 `Skills SkillConfig` 字段
  - 定义 `SkillConfig` 结构体：`Enabled bool` + `ExtraPaths []string`
  - 使用 `yaml:"skills,omitempty"` 标签

  **Must NOT do**:
  - 不修改现有配置字段
  - 不添加复杂的配置验证（保持简单）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单结构体添加
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2)
  - **Blocks**: Task 10
  - **Blocked By**: None

  **References**:
  - `server/internal/config/config.go:59-67` — MemoryConfig 结构体模式（Enabled + 可选字段）
  - `server/internal/config/config.go:85-100` — KnowledgeConfig 结构体模式
  - **WHY**: 遵循现有配置结构的命名和标签风格

  **Acceptance Criteria**:
  - [ ] `SkillConfig` 结构体定义完成
  - [ ] `Config.Skills` 字段添加完成
  - [ ] `go build ./server/...` → PASS

  **QA Scenarios**:

  ```
  Scenario: 解析带 skills 配置的 YAML
    Tool: Bash (go test -run)
    Steps:
      1. 创建临时 config.yaml 包含 skills.enabled: true + extra_paths
      2. 调用 config.Load() 解析
      3. Assert cfg.Skills.Enabled == true
      4. Assert cfg.Skills.ExtraPaths 包含配置的路径
    Expected Result: 配置正确解析
    Failure Indicators: 字段为空或解析失败
    Evidence: .sisyphus/evidence/task-3-config.yaml
  ```

  **Commit**: YES (groups in Commit 3)
  - Message: `feat(skill): add skill config section`
  - Files: `server/internal/config/config.go`

- [x] 4. SKILL.md 解析器

  **What to do**:
  - 创建 `plugins/skill/parser.go`
  - 实现 `ParseSkill(dirPath string) (*Skill, error)` 函数
  - 解析流程：
    1. 验证目录名格式 `[a-z0-9]+(-[a-z0-9]+)*`
    2. 读取 `dirPath/SKILL.md`
    3. 分割 YAML frontmatter（`---` 分隔符）
    4. 解析 frontmatter 到 map
    5. 提取 body 作为 Instructions
    6. 校验 `name` 与目录名一致
    7. 校验 `description` 非空
    8. 扫描 `scripts/`, `references/`, `assets/` 子目录收集 ResourceFile 列表
  - 使用 `gopkg.in/yaml.v3` 解析 frontmatter（与项目现有 config 解析一致）
  - 文件不存在时返回明确错误，格式不合法时返回具体错误信息

  **Must NOT do**:
  - 不引入新的 YAML 库（复用 `gopkg.in/yaml.v3`）
  - 不处理 MCP 相关字段（mcpServers 等）
  - 不做网络请求

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准文件解析，逻辑清晰
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 5, 6)
  - **Blocks**: Task 7
  - **Blocked By**: Task 1

  **References**:
  - `server/internal/config/config.go:108-115` — YAML 解析模式（`yaml.Unmarshal`）
  - `plugins/memory-file/frontmatter.go` — 现有的 frontmatter 解析实现
  - **WHY**: 复用项目的 YAML 解析方式，参考已有的 frontmatter 处理模式

  **Acceptance Criteria**:
  - [ ] `ParseSkill()` 实现完成
  - [ ] 正常 SKILL.md 解析成功
  - [ ] 缺少 SKILL.md 返回错误
  - [ ] 目录名不匹配返回错误
  - [ ] description 为空返回错误
  - [ ] 资源文件扫描正确

  **QA Scenarios**:

  ```
  Scenario: 正常 SKILL.md 解析
    Tool: Bash (go test -run)
    Preconditions: t.TempDir() 创建临时 skill 目录，写入合法 SKILL.md
    Steps:
      1. 创建临时目录 `test-skill/`
      2. 写入 SKILL.md（含 frontmatter + body）
      3. 调用 ParseSkill(tempDir)
      4. Assert skill.Name == "test-skill"
      5. Assert skill.Description 非空
      6. Assert skill.Instructions 非空
    Expected Result: 返回完整的 Skill 结构体
    Failure Indicators: 返回 error 或字段为空
    Evidence: .sisyphus/evidence/task-4-parse-ok.txt

  Scenario: 缺失 SKILL.md 文件
    Tool: Bash (go test -run)
    Preconditions: 创建空目录
    Steps:
      1. 创建临时空目录
      2. 调用 ParseSkill(emptyDir)
      3. Assert error 非 nil
      4. Assert error 包含 "SKILL.md"
    Expected Result: 返回明确错误
    Failure Indicators: 返回 nil error
    Evidence: .sisyphus/evidence/task-4-missing.txt

  Scenario: name 与目录名不匹配
    Tool: Bash (go test -run)
    Steps:
      1. 创建目录 `my-skill/`
      2. SKILL.md 中 name 写为 `other-name`
      3. 调用 ParseSkill()
      4. Assert error 非 nil
    Expected Result: 返回 name mismatch 错误
    Failure Indicators: 静默成功
    Evidence: .sisyphus/evidence/task-4-mismatch.txt
  ```

  **Commit**: YES (groups in Commit 2)
  - Message: `feat(skill): add SKILL.md parser`
  - Files: `plugins/skill/parser.go`
  - Pre-commit: `go test ./plugins/skill/ -run TestParse`

- [x] 5. SkillInfoHook

  **What to do**:
  - 创建 `plugins/skill/hook.go`
  - 实现 `SkillInfoHook` 结构体，实现 `hook.Hook` 接口
  - `Name()` 返回 `"skill_info"`
  - `Points()` 返回 `[]hook.HookPoint{hook.OnSystemPrompt}`
  - `Priority()` 返回 `60`（在 todo_injection(50) 之后，memory(80) 之前）
  - `Execute()` 在 `*ctx.SystemPrompt` 末尾追加 Skill 列表（L1 级别，仅 name + description）
  - 注入格式：
    ```
    ## Available Skills
    - **skill-name**: description text
    ```
  - 无 skill 时静默跳过（不追加任何内容）

  **Must NOT do**:
  - 不注入完整 SKILL.md body（仅 L1 元数据）
  - 不在 Execute 中做 I/O 操作（skill 数据已在构造时传入）
  - 不修改 SystemPrompt 以外的字段

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准 Hook 实现，模式明确
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 4, 6)
  - **Blocks**: Task 8
  - **Blocked By**: Task 1

  **References**:
  - `plugins/knowledge-base/kb_info_hook.go` — KBInfoHook 实现（OnSystemPrompt，注入 KB 信息）
  - `plugins/memory-file/file_memory_hook.go` — FileMemoryHook 实现（OnSystemPrompt，注入记忆）
  - `core/hook/hook.go:143-167` — Hook 接口定义
  - **WHY**: kb_info_hook 和 file_memory_hook 都是 OnSystemPrompt 注入的 Hook，SkillInfoHook 模式完全相同

  **Acceptance Criteria**:
  - [ ] `SkillInfoHook` 实现 `hook.Hook` 接口
  - [ ] 有 skill 时 system prompt 末尾包含 skill 列表
  - [ ] 无 skill 时 system prompt 不变
  - [ ] 仅注入 name + description（不含 body）

  **QA Scenarios**:

  ```
  Scenario: 有 skill 时注入列表
    Tool: Bash (go test -run)
    Steps:
      1. 创建 SkillInfoHook([]*Skill{{Name: "test", Description: "A test skill"}})
      2. 构造 HookContext，SystemPrompt 初始为 "You are a helpful assistant."
      3. 调用 Execute()
      4. Assert SystemPrompt 包含 "## Available Skills"
      5. Assert SystemPrompt 包含 "test: A test skill"
      6. Assert SystemPrompt 不包含 SKILL.md body 内容
    Expected Result: system prompt 正确追加 skill 列表
    Failure Indicators: 列表未出现或格式错误
    Evidence: .sisyphus/evidence/task-5-inject.txt

  Scenario: 无 skill 时不变
    Tool: Bash (go test -run)
    Steps:
      1. 创建 SkillInfoHook([]*Skill{})
      2. 构造 HookContext，SystemPrompt 初始为 "original prompt"
      3. 调用 Execute()
      4. Assert SystemPrompt == "original prompt"
    Expected Result: system prompt 保持不变
    Failure Indicators: 追加了空内容
    Evidence: .sisyphus/evidence/task-5-empty.txt
  ```

  **Commit**: YES (groups in Commit 2)
  - Message: `feat(skill): add SkillInfoHook for system prompt injection`
  - Files: `plugins/skill/hook.go`
  - Pre-commit: `go test ./plugins/skill/ -run TestSkillInfoHook`

- [x] 6. skill 工具

  **What to do**:
  - 创建 `plugins/skill/tool.go`
  - 实现 `SkillTool` 结构体，实现 `tool.Tool` 接口
  - `Name()` 返回 `"skill"`
  - `Description()` 返回工具描述
  - `InputSchema()` 定义 action 参数（enum: list/get/search）+ name/query 参数
  - `Execute()` 根据 action 分发：
    - `list`：返回所有 skill 的 L1 摘要（name + description）
    - `get`：根据 name 查找 skill，返回完整指令（L2）+ 资源文件列表
    - `search`：在 name + description 中模糊匹配 query，返回匹配的 skill 列表
  - 未知 action 返回错误
  - 不存在的 skill name 返回 "skill not found" 错误

  **Must NOT do**:
  - 不在 Execute 中读取文件（仅返回已解析的 Instructions 和资源文件路径）
  - 不实现 MCP 相关功能
  - 不缓存或修改 skill 数据

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准 Tool 实现，模式明确
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 4, 5)
  - **Blocks**: Task 8
  - **Blocked By**: Task 1

  **References**:
  - `plugins/knowledge-base/kb_search_tool.go` — KBSearchTool 实现（Tool 接口模式）
  - `plugins/memory-file/memory_recall_tool.go` — MemoryRecallTool 实现
  - `core/tool/manager.go:30-35` — Tool 接口定义
  - **WHY**: kb_search_tool 展示了带参数的 Tool 实现模式，SkillTool 结构类似

  **Acceptance Criteria**:
  - [ ] `SkillTool` 实现 `tool.Tool` 接口
  - [ ] `list` action 返回所有 skill 摘要
  - [ ] `get` action 返回完整指令
  - [ ] `search` action 返回匹配结果
  - [ ] 未知 action 返回错误
  - [ ] 不存在的 skill name 返回 "not found"

  **QA Scenarios**:

  ```
  Scenario: list 返回所有 skill
    Tool: Bash (go test -run)
    Steps:
      1. 创建 SkillTool([]*Skill{{Name: "a", Description: "desc a"}, {Name: "b", Description: "desc b"}})
      2. 调用 Execute(action="list")
      3. Assert result.Success == true
      4. Assert result.Data 包含 "a" 和 "b"
    Expected Result: 返回两个 skill 的摘要
    Failure Indicators: 缺失 skill 或格式错误
    Evidence: .sisyphus/evidence/task-6-list.txt

  Scenario: get 返回完整指令
    Tool: Bash (go test -run)
    Steps:
      1. 创建 SkillTool([]*Skill{{Name: "test", Instructions: "# Full instructions"}})
      2. 调用 Execute(action="get", name="test")
      3. Assert result.Success == true
      4. Assert result.Data 包含 "# Full instructions"
    Expected Result: 返回完整 skill 指令
    Failure Indicators: 返回空或错误
    Evidence: .sisyphus/evidence/task-6-get.txt

  Scenario: get 不存在的 skill
    Tool: Bash (go test -run)
    Steps:
      1. 创建 SkillTool([]*Skill{})
      2. 调用 Execute(action="get", name="nonexistent")
      3. Assert result.Success == false
      4. Assert result.Error 包含 "not found"
    Expected Result: 返回错误
    Failure Indicators: 返回 success=true
    Evidence: .sisyphus/evidence/task-6-notfound.txt

  Scenario: search 模糊匹配
    Tool: Bash (go test -run)
    Steps:
      1. 创建 SkillTool([]*Skill{{Name: "code-review", Description: "review code"}, {Name: "deploy", Description: "deploy app"}})
      2. 调用 Execute(action="search", query="review")
      3. Assert result.Success == true
      4. Assert result.Data 包含 "code-review"
      5. Assert result.Data 不包含 "deploy"
    Expected Result: 仅返回匹配的 skill
    Failure Indicators: 返回不匹配的 skill
    Evidence: .sisyphus/evidence/task-6-search.txt
  ```

  **Commit**: YES (groups in Commit 2)
  - Message: `feat(skill): add skill tool for runtime skill queries`
  - Files: `plugins/skill/tool.go`
  - Pre-commit: `go test ./plugins/skill/ -run TestSkillTool`

- [x] 7. 多路径发现器

  **What to do**:
  - 创建 `plugins/skill/discover.go`
  - 实现 `Discoverer` 结构体，管理优先级排序的搜索路径列表
  - `NewDiscoverer(projectRoot string, extraPaths []string)` 构造默认路径 + 配置路径
  - 默认路径（低优先级）：`.copcon/skills/`, `.agents/skills/`, `~/.copcon/skills/`, `~/.agents/skills/`
  - 配置路径（高优先级）：`extraPaths` 插入到默认路径之前
  - `Discover()` 方法：
    1. 遍历所有路径
    2. 对每个路径，扫描子目录（跳过隐藏目录和文件）
    3. 对每个子目录调用 `ParseSkill()`
    4. 解析失败时 warn 日志 + 跳过（不中断整体流程）
    5. 同名 Skill 按路径优先级去重（索引小的路径优先）
  - 路径不存在时静默跳过

  **Must NOT do**:
  - 不递归扫描（仅一层子目录）
  - 不处理符号链接
  - 不做并发扫描（保持简单）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 文件系统扫描 + 去重逻辑
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 8, 9)
  - **Blocks**: Task 8
  - **Blocked By**: Task 4

  **References**:
  - `plugins/memory-file/dir.go` — 目录扫描模式
  - `plugins/skill/parser.go` — ParseSkill 函数（依赖）
  - **WHY**: dir.go 展示了项目中的目录遍历模式

  **Acceptance Criteria**:
  - [ ] `Discover()` 扫描所有路径
  - [ ] 同名 Skill 按优先级去重
  - [ ] 解析失败的 Skill 被跳过（不中断）
  - [ ] 路径不存在时静默跳过
  - [ ] 配置路径优先级高于默认路径

  **QA Scenarios**:

  ```
  Scenario: 多路径发现 + 去重
    Tool: Bash (go test -run)
    Steps:
      1. 创建两个临时目录：high/ 和 low/
      2. 在 high/ 和 low/ 下各创建 test-skill/SKILL.md（description 不同）
      3. 创建 Discoverer(paths: [high, low])
      4. 调用 Discover()
      5. Assert 返回 1 个 skill
      6. Assert skill.Description == high 版本
      7. Assert skill.Source == high 路径
    Expected Result: 高优先级路径的 skill 胜出
    Failure Indicators: 返回 2 个 skill 或使用了低优先级版本
    Evidence: .sisyphus/evidence/task-7-dedup.txt

  Scenario: 空目录
    Tool: Bash (go test -run)
    Steps:
      1. 创建空临时目录
      2. 创建 Discoverer(paths: [emptyDir])
      3. 调用 Discover()
      4. Assert len(skills) == 0
      5. Assert err == nil
    Expected Result: 返回空列表，无错误
    Failure Indicators: 返回 error
    Evidence: .sisyphus/evidence/task-7-empty.txt

  Scenario: 路径不存在
    Tool: Bash (go test -run)
    Steps:
      1. 创建 Discoverer(paths: ["/nonexistent/path"])
      2. 调用 Discover()
      3. Assert len(skills) == 0
      4. Assert err == nil
    Expected Result: 静默跳过，无错误
    Failure Indicators: 返回 error 或 panic
    Evidence: .sisyphus/evidence/task-7-missing-path.txt
  ```

  **Commit**: YES (groups in Commit 2)
  - Message: `feat(skill): add multi-path skill discoverer`
  - Files: `plugins/skill/discover.go`
  - Pre-commit: `go test ./plugins/skill/ -run TestDiscover`

- [x] 8. SkillModule (ModuleCapability)

  **What to do**:
  - 创建 `plugins/skill/capability.go`
  - 实现 `SkillModule` 结构体，实现 `capabilities.ModuleCapability` 接口
  - `Name()` 返回 `"modules.skills"`
  - **关键**: `Type()` 返回 `capabilities.CapabilityTypeModule`（不是 CapabilityTypeSkill！harness 不处理 CapabilityTypeSkill 类型）
  - `DependsOn()` 返回 `nil`（无依赖）
  - `NewHooks()` 调用 `Discoverer.Discover()` 解析所有 skill，缓存到 `m.skills`，返回 `[]hook.Hook{NewSkillInfoHook(skills)}`
  - `NewTools()` 返回 `[]tool.Tool{NewSkillTool(m.skills)}`
  - 提供 `NewSkillModule(cfg Config)` 构造函数

  **Must NOT do**:
  - **绝不**让 `Type()` 返回 `CapabilityTypeSkill`（harness 中无处理分支，会导致静默忽略）
  - 不在 NewHooks/NewTools 中做重型操作（Discover 是唯一 I/O，在 Build 时执行）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准的 ModuleCapability 实现，模式明确
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 7, 9)
  - **Blocks**: Task 9
  - **Blocked By**: Tasks 5, 6, 7

  **References**:
  - `plugins/knowledge-base/capabilities_closure.go` — KBModule（ModuleCapability 参考实现）
  - `plugins/memory-file/capabilities_closure.go` — MemoryModule（ModuleCapability 参考实现）
  - `core/capabilities/registry.go:64-68` — ModuleCapability 接口定义
  - `core/harness.go:197-222` — harness 中 ModuleCapability 的处理流程
  - **WHY**: 两个现有 ModuleCapability 实现展示了完整的模式，SkillModule 完全遵循

  **Acceptance Criteria**:
  - [ ] `SkillModule` 实现 `ModuleCapability` 接口
  - [ ] `Type()` 返回 `CapabilityTypeModule`
  - [ ] `NewHooks()` 返回 1 个 SkillInfoHook
  - [ ] `NewTools()` 返回 1 个 SkillTool
  - [ ] 编译时接口校验 `var _ ModuleCapability = (*SkillModule)(nil)`

  **QA Scenarios**:

  ```
  Scenario: Module 接口实现完整
    Tool: Bash (go test -run)
    Steps:
      1. 创建 SkillModule
      2. Assert module.Name() == "modules.skills"
      3. Assert module.Type() == CapabilityTypeModule
      4. Assert len(module.DependsOn()) == 0
      5. 调用 NewHooks(), Assert len(hooks) == 1
      6. 调用 NewTools(), Assert len(tools) == 1
    Expected Result: 所有接口方法正常工作
    Failure Indicators: 返回错误或空结果
    Evidence: .sisyphus/evidence/task-8-module.txt
  ```

  **Commit**: YES (groups in Commit 2)
  - Message: `feat(skill): add SkillModule capability`
  - Files: `plugins/skill/capability.go`
  - Pre-commit: `go test ./plugins/skill/ -run TestSkillModule`

- [x] 9. 注册入口

  **What to do**:
  - 创建 `plugins/skill/register.go`
  - 实现 `RegisterCapabilities(r *capabilities.Registry, cfg Config)` 函数
  - 内部调用 `r.Register(NewSkillModule(cfg))`
  - 这是供 Server 层调用的唯一入口

  **Must NOT do**:
  - 不在此处做任何初始化或解析
  - 不创建全局变量

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单行注册函数
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 7, 8)
  - **Blocks**: Task 10
  - **Blocked By**: Task 8

  **References**:
  - `plugins/knowledge-base/register.go` — KB 插件注册入口
  - `plugins/memory-file/register.go` — Memory 插件注册入口
  - **WHY**: 两个现有插件的 register.go 展示了完全相同的注册模式

  **Acceptance Criteria**:
  - [ ] `RegisterCapabilities()` 函数实现
  - [ ] `go build ./plugins/skill/...` → PASS

  **QA Scenarios**:

  ```
  Scenario: 注册到 Registry
    Tool: Bash (go test -run)
    Steps:
      1. 创建 capabilities.NewRegistry()
      2. 调用 RegisterCapabilities(r, Config{ProjectRoot: "/tmp"})
      3. cap, ok := r.Get("modules.skills")
      4. Assert ok == true
      5. Assert cap.Type() == CapabilityTypeModule
    Expected Result: skill 模块成功注册
    Failure Indicators: Get 返回 false
    Evidence: .sisyphus/evidence/task-9-register.txt
  ```

  **Commit**: YES (groups in Commit 2)
  - Message: `feat(skill): add RegisterCapabilities entry point`
  - Files: `plugins/skill/register.go`

- [x] 10. Server 集成

  **What to do**:
  - 修改 `server/cmd/server/main.go`
  - 在 Harness 构建之前，检查 `cfg.Skills.Enabled`
  - 如果启用，构造 `skill.Config{ProjectRoot, ExtraPaths}` 并调用 `skill.RegisterCapabilities(capRegistry, skillCfg)`
  - 添加 `import "github.com/copcon/plugins/skill"` 导入

  **Must NOT do**:
  - 不修改 Harness 核心逻辑
  - 不在 main.go 中做 skill 解析

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 几行代码的集成调用
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (with Task 11)
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: Tasks 3, 9

  **References**:
  - `server/cmd/server/main.go` — 现有 server 启动流程，找到 Harness 构建和插件注册的位置
  - **WHY**: 需要理解现有 server 启动流程以找到正确的插入点

  **Acceptance Criteria**:
  - [ ] `cfg.Skills.Enabled == true` 时调用 `skill.RegisterCapabilities()`
  - [ ] `cfg.Skills.Enabled == false` 时跳过
  - [ ] `go build ./server/...` → PASS

  **QA Scenarios**:

  ```
  Scenario: skills enabled 时注册
    Tool: Bash (go test)
    Steps:
      1. 配置 skills.enabled: true
      2. 启动 server
      3. 检查 CapabilityRegistry 中是否有 modules.skills
    Expected Result: skill 模块已注册
    Failure Indicators: 注册表中无 modules.skills
    Evidence: .sisyphus/evidence/task-10-enabled.txt

  Scenario: skills disabled 时跳过
    Tool: Bash (go test)
    Steps:
      1. 配置 skills.enabled: false
      2. 启动 server
      3. 检查 CapabilityRegistry 中是否有 modules.skills
    Expected Result: 无 skill 模块注册
    Failure Indicators: 仍然注册了模块
    Evidence: .sisyphus/evidence/task-10-disabled.txt
  ```

  **Commit**: YES (groups in Commit 3)
  - Message: `feat(skill): integrate skill plugin into server startup`
  - Files: `server/cmd/server/main.go`

- [x] 11. 测试

  **What to do**:
  - 创建 `plugins/skill/parser_test.go` — 解析器测试（正常、缺失、不匹配、空 description）
  - 创建 `plugins/skill/discover_test.go` — 发现器测试（多路径、去重、空目录、不存在路径）
  - 创建 `plugins/skill/hook_test.go` — Hook 测试（有 skill、无 skill、格式验证）
  - 创建 `plugins/skill/tool_test.go` — Tool 测试（list、get、search、not found、invalid action）
  - 使用 testify/assert + require
  - 使用 `t.TempDir()` 创建临时 skill 目录
  - 手写 mock（不引入 mockgen）
  - 遵循 `TestXxx_Scenario` 命名约定

  **Must NOT do**:
  - 不引入新的测试依赖
  - 不创建集成测试文件（本次仅单元测试）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准测试编写，模式已明确
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: NO (with Task 10)
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: Tasks 4, 5, 6, 7, 8, 9

  **References**:
  - `plugins/memory-file/extraction_hook_test.go` — 测试结构模式（1 func = 1 scenario）
  - `plugins/memory-file/facts_test.go` — t.TempDir() + testify 模式
  - `core/testutil/chat_context.go` — MockChatContext（如需 mock ChatContextInterface）
  - **WHY**: 遵循项目现有测试惯例

  **Acceptance Criteria**:
  - [ ] 测试文件创建：parser_test.go, discover_test.go, hook_test.go, tool_test.go
  - [ ] `go test ./plugins/skill/... -v -count=1` → PASS
  - [ ] 覆盖率 ≥ 80%

  **QA Scenarios**:

  ```
  Scenario: 所有测试通过
    Tool: Bash (go test)
    Steps:
      1. Run: go test ./plugins/skill/... -v -count=1
      2. Assert exit code == 0
      3. Assert 无 FAIL 标记
    Expected Result: 所有测试通过
    Failure Indicators: 任何测试失败
    Evidence: .sisyphus/evidence/task-11-tests.txt
  ```

  **Commit**: YES (groups in Commit 2)
  - Message: `test(skill): add unit tests for parser, discoverer, hook, and tool`
  - Files: `plugins/skill/*_test.go`
  - Pre-commit: `go test ./plugins/skill/... -count=1`

---

## Final Verification Wave

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists. For each "Must NOT Have": search codebase for forbidden patterns. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go build ./...` + `go vet ./...`. Review all changed files for: empty catches, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Execute every QA scenario from every task. Test cross-task integration. Test edge cases: empty dir, missing SKILL.md, invalid YAML, duplicate names, large body.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff. Verify 1:1. Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **Commit 1**: `feat(skill): add core skill types and constants` — core/capabilities/skill/types.go, core/capabilities/constants.go
- **Commit 2**: `feat(skill): add skill plugin with parser, hook, and tool` — plugins/skill/*
- **Commit 3**: `feat(skill): add skill config and server integration` — server/internal/config/config.go, server/cmd/server/main.go

---

## Success Criteria

### Verification Commands
```bash
go build ./...                          # Expected: no errors
go test ./plugins/skill/... -v -count=1 # Expected: all tests pass
go test ./core/capabilities/... -v      # Expected: existing tests still pass
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass
- [ ] `go build ./...` succeeds
- [ ] Skill list appears in system prompt when skill is enabled