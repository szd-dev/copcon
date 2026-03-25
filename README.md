# Agent Infrastructure

一个完整的Agent基建系统，包含React UI组件库和Golang后端引擎。

## 项目结构

```
.
├── api/openapi.yaml              # OpenAPI 3.0 REST API规范
├── docker-compose.yaml           # Docker Compose部署配置
├── pnpm-workspace.yaml           # pnpm monorepo配置
│
├── server/                       # Go后端
│   ├── cmd/server/main.go        # 入口文件
│   ├── internal/
│   │   ├── agent/engine.go       # Agent核心引擎
│   │   ├── session/              # 会话管理
│   │   ├── context/              # 上下文管理
│   │   ├── memory/               # 记忆管理（Qdrant）
│   │   ├── tool/                 # 工具管理
│   │   ├── tools/                # 具体工具实现
│   │   ├── api/handlers.go       # REST API
│   │   └── config/               # 配置
│   ├── config.yaml               # 配置文件
│   ├── Dockerfile                # Docker镜像
│   └── go.mod                    # Go依赖
│
├── packages/
│   ├── ui/                       # React组件库
│   │   ├── src/
│   │   │   ├── components/       # Bubble, Sender, ThoughtChain, FileCard
│   │   │   ├── api/              # AgentClient
│   │   │   └── hooks/            # useAgentChat
│   │   ├── .storybook/           # Storybook配置
│   │   └── package.json
│   │
│   └── demo/                     # Vite Demo应用
│       ├── src/App.tsx
│       └── package.json
│
└── docs/
    ├── vector-design.md          # Qdrant设计文档
    └── version-summary.md        # 版本总结
```

## 技术栈

| 层级 | 技术 | 版本 |
|-----|------|------|
| 前端框架 | React + TypeScript | 19.x + 5.x |
| UI组件 | @ant-design/x | 2.4.0 |
| 构建工具 | Vite | 6.x |
| 文档 | Storybook | 8.x |
| 后端框架 | Gin | 1.12.0 |
| ORM | GORM | 1.31.1 |
| 向量DB | Qdrant | 1.17.x |
| 业务DB | PostgreSQL | 15.x |
| OpenAI客户端 | go-openai | 1.38.0 |

## 架构设计

### 后端架构

```
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
│         └────────┬─────────┘                                 │
│                  │                                           │
│  ┌───────────────┼───────────────┐                           │
│  ▼               ▼               ▼                           │
│ ┌────────┐  ┌────────────┐  ┌────────────┐                   │
│ │Memory  │  │OpenAI API  │  │MCP Tools   │                   │
│ │Manager │  │Client      │  │(Code/Shell/│                   │
│ │        │  │            │  │File)       │                   │
│ └────┬───┘  └────────────┘  └────────────┘                   │
│      │                                                      │
└──────┼──────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────┐  ┌──────────────┐
│  Qdrant      │  │  PostgreSQL  │
│  (Vector DB) │  │  (Sessions)  │
└──────────────┘  └──────────────┘
```

### 前端组件库

| 组件 | 功能 |
|-----|------|
| Bubble | 消息气泡，支持用户/AI/工具角色 |
| BubbleList | 消息列表，支持流式渲染 |
| Sender | 消息发送框，支持快捷键 |
| ThoughtChain | 工具调用过程可视化 |
| FileCard | 文件卡片展示 |
| Folder | 文件树组件 |

## 快速开始

### 环境要求

- Go 1.26+
- Node.js 20+
- Docker & Docker Compose
- PostgreSQL 15+
- Qdrant 1.17+

### 启动服务

```bash
# 1. 启动依赖服务
docker compose up -d

# 2. 初始化Qdrant Collection
bash scripts/init-qdrant.sh

# 3. 配置环境变量
export OPENAI_API_KEY=your-api-key

# 4. 启动后端
cd server
go mod tidy
go run ./cmd/server

# 5. 启动前端Demo (支持远程访问)
cd packages/demo
pnpm install
pnpm dev
# 访问: http://<your-ip>:5173

# 6. 查看组件文档 (支持远程访问)
cd packages/ui
pnpm storybook
# 访问: http://<your-ip>:6006
```

### 远程访问

Demo和Storybook开发服务器已配置支持远程访问：

| 服务 | 端口 | 访问地址 |
|-----|------|---------|
| Demo应用 | 5173 | `http://<your-ip>:5173` |
| Storybook | 6006 | `http://<your-ip>:6006` |
| 后端API | 8080 | `http://<your-ip>:8080` |

### 环境变量

| 变量 | 说明 | 默认值 |
|-----|------|--------|
| OPENAI_API_KEY | OpenAI API密钥 | - |
| DATABASE_HOST | PostgreSQL主机 | localhost |
| DATABASE_PORT | PostgreSQL端口 | 5432 |
| DATABASE_USER | 数据库用户 | admin |
| DATABASE_PASSWORD | 数据库密码 | changeme |
| DATABASE_DBNAME | 数据库名 | agent_infra |
| QDRANT_HOST | Qdrant主机 | localhost |
| QDRANT_PORT | Qdrant端口 | 6333 |

## API 文档

### 端点列表

| 方法 | 路径 | 说明 |
|-----|------|------|
| POST | /api/sessions | 创建会话 |
| GET | /api/sessions | 获取会话列表 |
| GET | /api/sessions/:id | 获取会话详情 |
| DELETE | /api/sessions/:id | 删除会话 |
| GET | /api/sessions/:id/messages | 获取消息历史 |
| POST | /api/sessions/:id/chat | 发送消息(SSE流式) |

### SSE 事件类型

| 事件 | 说明 |
|-----|------|
| message | 文本内容片段 |
| tool_call | 工具调用开始 |
| tool_result | 工具执行结果 |
| thought | 思考过程 |
| done | 完成 |
| error | 错误 |

### 示例请求

```bash
# 创建会话
curl -X POST http://localhost:8080/api/sessions

# 发送消息
curl -N -X POST http://localhost:8080/api/sessions/{session-id}/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"你好，请帮我执行一段Python代码"}'
```

## 测试

```bash
# 运行所有Go测试
cd server
go test ./... -v

# 运行特定包测试
go test ./internal/session/... -v
go test ./internal/tool/... -v
go test ./internal/tools/... -v
```

### 测试覆盖

- ✅ Session管理器 (6/6)
- ✅ Tool管理器 (5/5)
- ✅ 具体工具实现 (6/6)

## 使用组件库

### 安装

```bash
pnpm add @agent-infra/ui
```

### 使用示例

```tsx
import { Bubble, Sender, useAgentChat, AgentClient } from '@agent-infra/ui';

const client = new AgentClient({ baseUrl: 'http://localhost:8080' });

function ChatApp() {
  const { messages, isLoading, sendMessage } = useAgentChat({
    client,
    sessionId: 'your-session-id',
  });

  return (
    <div>
      <Bubble.List items={messages.map(m => ({
        role: m.role,
        content: m.content,
      }))} />
      <Sender onSubmit={sendMessage} loading={isLoading} />
    </div>
  );
}
```

## 开发指南

### 后端开发

```bash
cd server

# 安装依赖
go mod tidy

# 运行开发服务器
go run ./cmd/server

# 构建
go build ./cmd/server

# 运行测试
go test ./... -v
```

### 前端开发

```bash
cd packages/ui

# 安装依赖
pnpm install

# 开发模式
pnpm dev

# 构建
pnpm build

# Storybook
pnpm storybook
```

## 部署

### Docker Compose

```bash
docker compose up -d
```

### 配置文件

```yaml
# config.yaml
server:
  port: "8080"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "agent_infra"

openai:
  api_key: ""
  base_url: ""
  model: "gpt-4o"

qdrant:
  host: "localhost"
  port: 6333
```

## License

MIT