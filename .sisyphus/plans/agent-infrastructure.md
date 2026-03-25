# Agent Infrastructure - First Phase (Core Features)

## TL;DR

> **Quick Summary**: 构建Agent基建核心功能。后端是完整的Agent系统（会话管理+上下文管理+工具调度+记忆管理+Agent循环引擎），对外暴露简洁REST API；前端是纯展示层，仅负责UI交互和流式渲染。
>
> **Deliverables**:
> - React Agent UI组件库（Bubble/Sender/ThoughtChain/FileCard）
> - Golang Gin后端服务（Agent核心引擎）
> - Storybook + Vite Demo展示
> - Docker Compose部署配置
>
> **Estimated Effort**: Medium（4周核心功能）
> **Parallel Execution**: YES - 4 waves
> **Critical Path**: Agent核心引擎 → 会话管理 → 上下文管理 → 工具调度 → 记忆管理 → 前端展示

---

## Context

### Original Request
构建Agent基建，前端使用TypeScript React组件库（可集成到多种系统），后端使用Golang实现。对接OpenAI协议，支持向量数据库作为长期记忆存储。

### Architecture Clarification (Critical)
**后端职责（Agent核心）**：
1. **会话管理** - 创建/恢复/删除/列表会话
2. **上下文管理** - 对话历史、消息裁剪、token管理、上下文构建
3. **工具管理** - MCP工具注册、发现、调度、执行
4. **记忆管理** - 短期记忆（会话内）、长期记忆（Qdrant向量）、检索关联
5. **Agent循环** - LLM调用→工具调用→结果整合→回复生成的完整流程

**前端职责（纯展示层）**：
- 展示会话列表
- 展示消息历史
- 发送用户输入
- 接收并展示流式回复
- 展示工具调用过程（ThoughtChain）

### 架构图
```
┌─────────────────────────────────────────────────────────────┐
│                         Frontend (React UI)                 │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────────┐       │
│  │Bubble   │ │Sender   │ │ThoughtChain│ │FileCard     │       │
│  └────┬────┘ └────┬────┘ └────┬────┘ └──────┬──────┘       │
│       └───────────┴───────────┴──────────────┘              │
│                       │ SSE Stream                           │
└───────────────────────┼─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                    Backend (Gin + Agent Engine)              │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│  │ 会话管理     │  │ 上下文管理   │  │ 工具管理     │       │
│  │ SessionMgr   │  │ ContextMgr   │  │ ToolMgr      │       │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘       │
│         └─────────┬───────┴─────────────────┘                │
│                   ▼                                          │
│         ┌──────────────────┐                                 │
│         │   Agent Engine   │ ◄─── 核心循环                   │
│         │   (Core Loop)    │                                 │
│         └────────┬─────────┘                                 │
│                  │                                           │
│  ┌───────────────┼───────────────┐                           │
│  ▼               ▼               ▼                           │
│ ┌────────┐  ┌────────────┐  ┌────────────┐                   │
│ │Memory  │  │OpenAI API  │  │MCP Tools   │                   │
│ │Manager │  │Client      │  │(Code/Shell/│                   │
│ └────┬───┘  └────────────┘  │File)       │                   │
│      │                      └────────────┘                   │
└──────┼──────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────┐  ┌──────────────┐
│  Qdrant      │  │  PostgreSQL  │
│  (Vector DB) │  │  (Sessions)  │
└──────────────┘  └──────────────┘
```

### Metis Review
**Identified Gaps** (addressed):
- 前端不应直接调用OpenAI API，所有Agent能力内聚在后端
- 后端需要完整的会话/上下文/工具/记忆管理层
- API设计应该对前端隐藏复杂性

---

## Work Objectives

### Core Objective
构建完整的Agent后端引擎，包含会话管理、上下文管理、工具调度、记忆管理、Agent循环；前端仅作为展示层。

### Concrete Deliverables
1. **后端Agent引擎** (server/)
   - 会话管理器 (Session Manager)
   - 上下文管理器 (Context Manager)
   - 工具管理器 (Tool Manager)
   - 记忆管理器 (Memory Manager)
   - Agent核心循环 (Agent Engine)
   - REST API层
2. **前端组件库** (packages/ui/)
   - Bubble, Sender, ThoughtChain, FileCard/Folder
   - Storybook文档
3. **Demo应用** (packages/demo/)
4. **部署配置**
   - Docker Compose
   - PostgreSQL + Qdrant

### Definition of Done
- [ ] Agent引擎可完成完整对话循环（LLM调用→工具调用→回复）
- [ ] 会话持久化到PostgreSQL
- [ ] 对话历史可存储和检索
- [ ] MCP工具可注册和执行
- [ ] 向量记忆可存储和检索
- [ ] 前端可通过简洁API与Agent交互
- [ ] Docker Compose一键启动

### Must Have
- Agent核心循环引擎
- 会话管理（创建/恢复/列表/删除）
- 上下文管理（历史存储、token计算、裁剪策略）
- MCP工具调度
- 短期记忆 + 长期向量记忆
- SSE流式响应

### Must NOT Have (Guardrails)
- 前端不直接调用OpenAI API
- 不暴露底层LLM/工具API给前端
- 首期不包含用户认证
- 首期不包含RAG知识库

---

## Verification Strategy (MANDATORY)

### Test Decision
- **Automated tests**: Tests After（后置测试）

### QA Policy
- **Backend**: Go test + curl（API测试）
- **Frontend**: Playwright（UI测试）
- **Agent Engine**: 集成测试（完整对话循环）

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — foundation + contracts):
├── Task 1: 前后端API合约定义 [quick]
├── Task 2: 后端项目脚手架 [quick]
├── Task 3: 前端组件库脚手架 [quick]
├── Task 4: PostgreSQL Schema设计 [quick]
└── Task 5: Qdrant Collection设计 [quick]

Wave 2 (After Wave 1 — Agent核心模块):
├── Task 6: 会话管理器实现 [deep]
├── Task 7: 上下文管理器实现 [deep]
├── Task 8: 记忆管理器实现 [deep]
├── Task 9: 工具管理器实现 [deep]
├── Task 10: MCP工具实现（Code/Shell/File） [deep]
└── Task 11: Agent核心引擎实现 [ultrabrain]

Wave 3 (After Wave 2 — API + Frontend):
├── Task 12: REST API层实现 [deep]
├── Task 13: SSE流式响应实现 [deep]
├── Task 14: 前端Bubble组件 [visual-engineering]
├── Task 15: 前端Sender组件 [visual-engineering]
├── Task 16: 前端ThoughtChain组件 [visual-engineering]
└── Task 17: 前端FileCard组件 [visual-engineering]

Wave 4 (After Wave 3 — Integration):
├── Task 18: 前端API客户端 [quick]
├── Task 19: Vite Demo应用 [visual-engineering]
├── Task 20: Storybook文档 [visual-engineering]
└── Task 21: Docker Compose部署 [quick]

Final Wave (Verification):
├── Task F1: Agent端到端测试 [deep]
├── Task F2: 会话恢复测试 [deep]
├── Task F3: 工具调用集成测试 [deep]
└── Task F4: 代码质量审查 [unspecified-high]
```

### Dependency Matrix
- **1-5**: None → Wave 2
- **6, 7, 8, 9**: → Task 11 (Agent Engine依赖所有Manager)
- **10**: → Task 9 (工具实现依赖工具管理器接口)
- **11**: → Task 12, 13 (Agent Engine完成后才能实现API)
- **12, 13**: → Task 18, 19
- **14-17**: → Task 19, 20
- **18-21**: → Final

### Agent Dispatch Summary
- **Wave 1**: 5 quick tasks — parallel
- **Wave 2**: 5 deep + 1 ultrabrain — parallel (except Agent Engine waits for Managers)
- **Wave 3**: 2 deep + 4 visual — parallel
- **Wave 4**: 2 quick + 2 visual — parallel
- **Final**: 3 deep + 1 unspecified-high — parallel

---

## TODOs

### Wave 1: Foundation

- [ ] 1. 前后端API合约定义

  **What to do**:
  - 创建OpenAPI 3.0规范文件 `api/openapi.yaml`
  - 定义前端REST API端点（对前端隐藏复杂性）：
    - `POST /api/sessions` - 创建会话
    - `GET /api/sessions` - 获取会话列表
    - `GET /api/sessions/:id` - 获取会话详情
    - `DELETE /api/sessions/:id` - 删除会话
    - `GET /api/sessions/:id/messages` - 获取消息历史
    - `POST /api/sessions/:id/chat` - 发送消息（SSE流式返回）
  - 定义SSE事件格式（message/tool_call/thought/done）
  - 定义请求/响应Schema

  **Must NOT do**:
  - 不要暴露OpenAI原始API
  - 不要暴露工具执行API给前端

  **Recommended Agent Profile**:
  - **Category**: `writing`
    - Reason: API文档编写
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 12, 14-17, 18
  - **Blocked By**: None

  **References**:
  - OpenAPI Spec: `https://swagger.io/specification/` - OpenAPI规范

  **Acceptance Criteria**:
  - [ ] `api/openapi.yaml` 文件创建
  - [ ] 包含所有6个端点定义
  - [ ] SSE事件格式定义完整

  **QA Scenarios**:
  ```
  Scenario: OpenAPI规范验证
    Tool: Bash (swagger-cli)
    Steps:
      1. Run: npx swagger-cli validate api/openapi.yaml
    Expected Result: Valid spec, no errors
    Evidence: .sisyphus/evidence/task-1-openapi-validate.txt
  ```

  **Commit**: YES
  - Message: `chore: define REST API contract for frontend`
  - Files: `api/openapi.yaml`

---

- [ ] 2. 后端项目脚手架

  **What to do**:
  - 创建 `server/` 目录结构：
    ```
    server/
    ├── cmd/server/main.go
    ├── internal/
    │   ├── agent/          # Agent核心引擎
    │   ├── session/        # 会话管理
    │   ├── context/        # 上下文管理
    │   ├── memory/         # 记忆管理
    │   ├── tool/           # 工具管理
    │   ├── llm/            # LLM客户端
    │   ├── api/            # REST API
    │   └── config/         # 配置
    ├── pkg/
    └── go.mod
    ```
  - 初始化Go module：`go mod init github.com/copcon/server`
  - 安装依赖：gin, gorm, go-openai, qdrant-go-client
  - 创建配置加载（config.yaml）
  - 创建基础Gin router

  **Must NOT do**:
  - 不要实现具体业务逻辑

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准Go项目脚手架

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Wave 2所有任务
  - **Blocked By**: None

  **Acceptance Criteria**:
  - [ ] 目录结构创建完成
  - [ ] `go mod init` 完成
  - [ ] server可启动并响应 `/health`

  **QA Scenarios**:
  ```
  Scenario: 后端服务启动
    Tool: Bash
    Steps:
      1. cd server && go build ./cmd/server
      2. ./server &
      3. curl http://localhost:8080/health
    Expected Result: {"status":"ok"}
    Evidence: .sisyphus/evidence/task-2-server-health.txt
  ```

  **Commit**: YES
  - Message: `chore(server): scaffold Go project structure`

---

- [ ] 3. 前端组件库脚手架

  **What to do**:
  - 创建 `packages/ui/` 目录
  - 初始化pnpm workspace
  - 配置Vite library mode
  - 安装依赖：react, antd, @ant-design/x
  - 创建组件入口 `src/index.ts`

  **Must NOT do**:
  - 不要实现具体组件
  - 不要添加网络请求逻辑

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 14-17, 20
  - **Blocked By**: None

  **Acceptance Criteria**:
  - [ ] `packages/ui/` 创建完成
  - [ ] `pnpm build` 可生成ESM/CJS

  **Commit**: YES
  - Message: `chore(ui): scaffold React component library`

---

- [ ] 4. PostgreSQL Schema设计

  **What to do**:
  - 创建 `server/internal/session/schema.sql`
  - 定义表结构：
    ```sql
    CREATE TABLE sessions (
      id UUID PRIMARY KEY,
      title VARCHAR(255),
      created_at TIMESTAMP,
      updated_at TIMESTAMP,
      metadata JSONB
    );
    
    CREATE TABLE messages (
      id UUID PRIMARY KEY,
      session_id UUID REFERENCES sessions(id),
      role VARCHAR(20),  -- user/assistant/tool
      content TEXT,
      tool_calls JSONB,
      tool_call_id VARCHAR(255),
      created_at TIMESTAMP
    );
    
    CREATE INDEX idx_messages_session_id ON messages(session_id);
    ```
  - 创建GORM模型定义

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 6
  - **Blocked By**: None

  **Acceptance Criteria**:
  - [ ] schema.sql创建完成
  - [ ] GORM模型定义完成

  **Commit**: YES
  - Message: `chore(db): define PostgreSQL schema`

---

- [ ] 5. Qdrant Collection设计

  **What to do**:
  - 创建 `docs/vector-design.md`
  - 定义memory collection：
    - 向量维度：1536
    - 距离度量：Cosine
    - Payload：content, session_id, role, timestamp
  - 创建初始化脚本 `scripts/init-qdrant.sh`

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 8
  - **Blocked By**: None

  **Commit**: YES
  - Message: `chore: define Qdrant collection design`

---

### Wave 2: Agent Core Modules

- [ ] 6. 会话管理器实现

  **What to do**:
  - 创建 `server/internal/session/` 目录
  - 实现 `SessionManager` 接口：
    ```go
    type SessionManager interface {
        Create(ctx context.Context) (*Session, error)
        Get(ctx context.Context, id string) (*Session, error)
        List(ctx context.Context) ([]*Session, error)
        Delete(ctx context.Context, id string) error
        UpdateTitle(ctx context.Context, id, title string) error
    }
    ```
  - 实现PostgreSQL存储
  - 实现会话创建时自动生成标题（基于首条消息）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 需要设计会话生命周期管理

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 7, 8, 9)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 11, 12
  - **Blocked By**: Task 2, 4

  **Acceptance Criteria**:
  - [ ] SessionManager接口实现完成
  - [ ] 会话可持久化到PostgreSQL
  - [ ] 单元测试通过

  **QA Scenarios**:
  ```
  Scenario: 会话CRUD
    Tool: Go test
    Steps:
      1. go test ./internal/session/... -v
    Expected Result: 所有测试通过
    Evidence: .sisyphus/evidence/task-6-session-test.txt
  ```

  **Commit**: YES
  - Message: `feat(session): implement session manager`

---

- [ ] 7. 上下文管理器实现

  **What to do**:
  - 创建 `server/internal/context/` 目录
  - 实现 `ContextManager`：
    ```go
    type ContextManager interface {
        // 获取对话历史
        GetHistory(ctx context.Context, sessionID string) ([]Message, error)
        // 添加消息到历史
        AddMessage(ctx context.Context, sessionID string, msg Message) error
        // 构建LLM上下文（含系统提示、历史、当前输入）
        BuildContext(ctx context.Context, sessionID string, userInput string) ([]openai.ChatCompletionMessage, error)
        // Token计算和裁剪
        TrimContext(ctx context.Context, messages []Message, maxTokens int) ([]Message, error)
    }
    ```
  - 实现消息存储（复用session的messages表）
  - 实现token计数（tiktoken或估算）
  - 实现上下文裁剪策略（保留系统提示+最近N条消息）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 上下文管理是Agent核心能力

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 11, 12
  - **Blocked By**: Task 2

  **Acceptance Criteria**:
  - [ ] ContextManager接口实现完成
  - [ ] 消息可存储和检索
  - [ ] 上下文裁剪正确

  **Commit**: YES
  - Message: `feat(context): implement context manager`

---

- [ ] 8. 记忆管理器实现

  **What to do**:
  - 创建 `server/internal/memory/` 目录
  - 实现 `MemoryManager`：
    ```go
    type MemoryManager interface {
        // 存储记忆到向量库
        Store(ctx context.Context, sessionID string, content string, metadata map[string]interface{}) error
        // 检索相关记忆
        Search(ctx context.Context, query string, limit int) ([]Memory, error)
        // 获取会话相关记忆
        GetSessionMemories(ctx context.Context, sessionID string) ([]Memory, error)
        // 清除会话记忆
        ClearSessionMemories(ctx context.Context, sessionID string) error
    }
    ```
  - 实现Qdrant客户端封装
  - 实现OpenAI Embedding调用
  - 短期记忆：会话内的消息历史（ContextManager管理）
  - 长期记忆：重要信息向量存储（MemoryManager管理）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 向量记忆是Agent长期记忆关键

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 11
  - **Blocked By**: Task 2, 5

  **Acceptance Criteria**:
  - [ ] MemoryManager接口实现完成
  - [ ] 向量可存储和检索
  - [ ] OpenAI Embedding集成正常

  **Commit**: YES
  - Message: `feat(memory): implement memory manager with Qdrant`

---

- [ ] 9. 工具管理器实现

  **What to do**:
  - 创建 `server/internal/tool/` 目录
  - 定义 `Tool` 接口：
    ```go
    type Tool interface {
        Name() string
        Description() string
        InputSchema() map[string]interface{}
        Execute(ctx context.Context, args map[string]interface{}) (interface{}, error)
    }
    ```
  - 实现 `ToolManager`：
    ```go
    type ToolManager interface {
        // 注册工具
        Register(tool Tool) error
        // 获取所有工具定义（用于LLM function calling）
        GetToolDefinitions() []openai.Tool
        // 执行工具
        ExecuteTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error)
        // 列出工具
        ListTools() []ToolInfo
    }
    ```
  - 实现工具注册表
  - 实现工具执行调度

  **Recommended Agent Profile**:
  - **Category**: `deep`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 10, 11
  - **Blocked By**: Task 2

  **Acceptance Criteria**:
  - [ ] ToolManager接口实现完成
  - [ ] 工具可注册和执行
  - [ ] 可生成OpenAI function calling格式

  **Commit**: YES
  - Message: `feat(tool): implement tool manager`

---

- [ ] 10. MCP工具实现

  **What to do**:
  - 创建 `server/internal/tools/` 目录
  - 实现三个工具：
    - `code_executor.go` - Python/JS代码执行（Docker沙箱）
    - `shell_executor.go` - Shell命令执行（白名单）
    - `file_ops.go` - 文件读写操作（沙箱限制）
  - 实现Tool接口
  - 注册到ToolManager
  - 安全限制：
    - 代码执行：Docker隔离，30秒超时
    - Shell：命令白名单，10秒超时
    - 文件：只允许工作目录，10MB限制

  **Recommended Agent Profile**:
  - **Category**: `deep`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 11, F3
  - **Blocked By**: Task 9

  **Acceptance Criteria**:
  - [ ] 三个工具实现完成
  - [ ] 安全限制生效
  - [ ] 单元测试通过

  **Commit**: YES
  - Message: `feat(tools): implement code/shell/file tools`

---

- [ ] 11. Agent核心引擎实现

  **What to do**:
  - 创建 `server/internal/agent/` 目录
  - 实现 `AgentEngine` - 核心Agent循环：
    ```go
    type AgentEngine interface {
        // 执行对话（返回SSE流）
        Chat(ctx context.Context, sessionID string, userInput string) (<-chan Event, error)
    }
    ```
  - 实现Agent循环逻辑：
    1. 获取/创建会话
    2. 构建上下文（历史 + 相关记忆 + 用户输入）
    3. 调用LLM
    4. 检测tool_calls
    5. 执行工具
    6. 将工具结果加入上下文
    7. 重复3-6直到LLM返回最终回复
    8. 存储重要信息到记忆
    9. 返回SSE事件流
  - 实现SSE事件类型：
    - `message` - 文本片段
    - `tool_call` - 工具调用开始
    - `tool_result` - 工具执行结果
    - `thought` - 思考过程
    - `done` - 完成

  **Recommended Agent Profile**:
  - **Category**: `ultrabrain`
    - Reason: Agent核心循环是系统大脑，需要复杂逻辑设计

  **Parallelization**:
  - **Can Run In Parallel**: NO (依赖Task 6, 7, 8, 9完成)
  - **Parallel Group**: Wave 2 (last)
  - **Blocks**: Task 12, 13
  - **Blocked By**: Task 6, 7, 8, 9

  **Acceptance Criteria**:
  - [ ] AgentEngine实现完成
  - [ ] 可完成完整对话循环
  - [ ] 可处理工具调用
  - [ ] SSE事件流正确

  **QA Scenarios**:
  ```
  Scenario: Agent对话循环
    Tool: Bash (curl)
    Steps:
      1. Create session: curl -X POST http://localhost:8080/api/sessions
      2. Send message: curl -N -X POST http://localhost:8080/api/sessions/{id}/chat -d '{"content":"1+1=?"}'
    Expected Result: 收到SSE事件流，包含message和done事件
    Evidence: .sisyphus/evidence/task-11-agent-chat.txt

  Scenario: Agent工具调用
    Tool: Bash (curl)
    Steps:
      1. Send: "用Python计算2的10次方"
    Expected Result: 收到tool_call和tool_result事件，最终返回正确答案
    Evidence: .sisyphus/evidence/task-11-agent-tool.txt
  ```

  **Commit**: YES
  - Message: `feat(agent): implement Agent core engine with tool calling loop`

---

### Wave 3: API + Frontend

- [ ] 12. REST API层实现

  **What to do**:
  - 创建 `server/internal/api/` 目录
  - 实现REST API Handler：
    - `POST /api/sessions` - 创建会话
    - `GET /api/sessions` - 获取会话列表
    - `GET /api/sessions/:id` - 获取会话详情
    - `DELETE /api/sessions/:id` - 删除会话
    - `GET /api/sessions/:id/messages` - 获取消息历史
  - 集成SessionManager和ContextManager
  - 实现错误处理和响应格式

  **Recommended Agent Profile**:
  - **Category**: `deep`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 18, 19
  - **Blocked By**: Task 6, 7, 11

  **Acceptance Criteria**:
  - [ ] 所有API端点可用
  - [ ] 响应格式符合OpenAPI定义
  - [ ] 错误处理完善

  **Commit**: YES
  - Message: `feat(api): implement REST API handlers`

---

- [ ] 13. SSE流式响应实现

  **What to do**:
  - 创建 `server/internal/api/chat.go`
  - 实现 `POST /api/sessions/:id/chat` - SSE流式对话
  - 集成AgentEngine的SSE事件流
  - 处理客户端断开
  - 设置正确的HTTP头：
    - `Content-Type: text/event-stream`
    - `Cache-Control: no-cache`
    - `Connection: keep-alive`

  **Recommended Agent Profile**:
  - **Category**: `deep`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 18, 19
  - **Blocked By**: Task 11

  **Acceptance Criteria**:
  - [ ] SSE流式响应正确
  - [ ] 支持中途断开
  - [ ] 事件格式正确

  **Commit**: YES
  - Message: `feat(api): implement SSE streaming chat`

---

- [ ] 14. 前端Bubble对话组件

  **What to do**:
  - 创建 `packages/ui/src/components/Bubble/`
  - 实现 `Bubble.tsx` - 单条消息气泡
  - 支持：role (user/assistant)、content、loading状态
  - 支持：流式内容渲染（打字机效果）
  - 集成 `@ant-design/x-markdown` 渲染Markdown
  - 实现 `Bubble.List.tsx` - 消息列表

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 19
  - **Blocked By**: Task 3

  **Commit**: YES
  - Message: `feat(ui): implement Bubble chat component`

---

- [ ] 15. 前端Sender发送组件

  **What to do**:
  - 创建 `packages/ui/src/components/Sender/`
  - 实现 `Sender.tsx` - 消息输入框
  - 支持：onSubmit回调、placeholder、disabled、loading
  - 支持多行输入、快捷键（Enter发送）
  - 支持发送中状态

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 19
  - **Blocked By**: Task 3

  **Commit**: YES
  - Message: `feat(ui): implement Sender input component`

---

- [ ] 16. 前端ThoughtChain思维链组件

  **What to do**:
  - 创建 `packages/ui/src/components/ThoughtChain/`
  - 实现 `ThoughtChain.tsx` - 工具调用过程展示
  - 支持：steps (title, content, status, duration)
  - 支持折叠/展开、状态显示（pending/running/success/error）

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 19
  - **Blocked By**: Task 3

  **Commit**: YES
  - Message: `feat(ui): implement ThoughtChain component`

---

- [ ] 17. 前端FileCard组件

  **What to do**:
  - 创建 `packages/ui/src/components/FileCard/`
  - 实现 `FileCard.tsx` - 文件卡片
  - 支持：name, size, type, status
  - 实现 `Folder.tsx` - 文件树

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 19
  - **Blocked By**: Task 3

  **Commit**: YES
  - Message: `feat(ui): implement FileCard and Folder components`

---

### Wave 4: Integration

- [ ] 18. 前端API客户端

  **What to do**:
  - 创建 `packages/ui/src/api/`
  - 实现 `agentClient.ts` - 后端API客户端：
    ```typescript
    interface AgentClient {
      createSession(): Promise<Session>
      getSessions(): Promise<Session[]>
      getSession(id: string): Promise<Session>
      deleteSession(id: string): Promise<void>
      getMessages(sessionId: string): Promise<Message[]>
      chat(sessionId: string, content: string, onEvent: (event: SSEEvent) => void): Promise<void>
    }
    ```
  - 实现SSE解析
  - 实现错误处理

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4
  - **Blocks**: Task 19
  - **Blocked By**: Task 1, 12, 13

  **Commit**: YES
  - Message: `feat(ui): implement Agent API client`

---

- [ ] 19. Vite Demo应用

  **What to do**:
  - 创建 `packages/demo/`
  - 使用Vite + React创建Demo应用
  - 集成所有UI组件和API客户端
  - 实现完整对话界面：
    - 左侧：会话列表
    - 中间：对话区域（Bubble + Sender）
    - 工具调用展示（ThoughtChain）
  - 展示SSE流式对话

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4
  - **Blocks**: Final
  - **Blocked By**: Task 14, 15, 16, 17, 18

  **Acceptance Criteria**:
  - [ ] Demo应用可启动
  - [ ] 对话功能完整
  - [ ] SSE流式显示正常

  **Commit**: YES
  - Message: `feat(demo): implement Vite demo application`

---

- [ ] 20. Storybook文档

  **What to do**:
  - 为所有组件创建Storybook stories
  - 配置Storybook主题
  - 添加组件API文档

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4
  - **Blocked By**: Task 14, 15, 16, 17

  **Commit**: YES
  - Message: `docs(ui): add Storybook documentation`

---

- [ ] 21. Docker Compose部署

  **What to do**:
  - 创建 `docker-compose.yaml`
  - 定义服务：
    - `postgres`: PostgreSQL (port 5432)
    - `qdrant`: Qdrant (port 6333, 6334)
    - `server`: Go后端 (port 8080)
  - 创建 `server/Dockerfile`
  - 创建 `.env.example`
  - 创建启动脚本 `scripts/start.sh`

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4
  - **Blocked By**: Task 2

  **Acceptance Criteria**:
  - [ ] `docker compose up -d` 可启动所有服务
  - [ ] 所有服务运行正常

  **Commit**: YES
  - Message: `chore(deploy): add Docker Compose configuration`

---

## Final Verification Wave (MANDATORY)

- [ ] F1. **Agent端到端测试** — `deep`
  验证完整Agent循环：用户输入→LLM调用→工具调用→记忆存储→回复生成

- [ ] F2. **会话恢复测试** — `deep`
  验证会话持久化和恢复功能

- [ ] F3. **工具调用集成测试** — `deep`
  验证MCP工具注册、发现、调度、执行的完整流程

- [ ] F4. **代码质量审查** — `unspecified-high`
  检查代码规范、类型安全、错误处理、架构清晰度

---

## Commit Strategy

- **Wave 1**: `chore(infra): scaffold project structures`
- **Wave 2**: `feat(agent): implement Agent core modules`
- **Wave 3**: `feat(api): implement REST API and UI components`
- **Wave 4**: `feat(demo): implement demo app and deployment`
- **Final**: `test: add integration tests`

---

## Success Criteria

### Verification Commands
```bash
# 创建会话
curl -X POST http://localhost:8080/api/sessions -H "Content-Type: application/json" -d '{}'

# 发送消息（SSE流式）
curl -N -X POST http://localhost:8080/api/sessions/{id}/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"帮我执行一段Python代码：print(1+1)"}'

# 获取会话历史
curl http://localhost:8080/api/sessions/{id}/messages

# Demo应用
cd packages/demo && pnpm dev
```

### Final Checklist
- [ ] Agent引擎可完成完整对话循环
- [ ] 会话可持久化和恢复
- [ ] MCP工具可正常执行
- [ ] 向量记忆可存储检索
- [ ] 前端展示正常
- [ ] Docker Compose一键启动
