# 安装指南

CopCon 后端的完整安装和环境配置说明。

## Go 版本

需要 **Go 1.26 或更高版本**。项目在 `go.mod` 中声明了 `go 1.26`，低版本编译器会直接拒绝编译。

检查你的版本：

```bash
go version
# 应该输出 go version go1.26.x linux/amd64 或类似内容
```

## PostgreSQL 设置

### 方式一：Docker Compose（推荐）

项目根目录提供了 `docker-compose.yaml`：

```bash
# 启动 PostgreSQL 和 Qdrant（两个依赖）
docker compose up -d postgres qdrant
```

这会启动：
- PostgreSQL 15，端口 `5432`，用户 `agent`，密码 `agent123`，库 `agent_infra`
- Qdrant 1.17，端口 `6333`

默认的数据库连接参数在 `config.yaml` 和 `docker-compose.yaml` 中是对齐的。

### 方式二：手动安装

如果你有自己的 PostgreSQL，创建一个数据库：

```sql
CREATE USER agent WITH PASSWORD 'agent123';
CREATE DATABASE agent_infra OWNER agent;
```

然后修改 `config.yaml` 中的数据库配置指向你的实例。

### 初始化

GORM 的 AutoMigrate 会在服务启动时自动创建表，不需要手动执行 SQL。

## 环境变量

所有环境变量都会覆盖 `config.yaml` 中的对应值：

| 变量 | 说明 | 默认值 |
|---|---|---|
| `OPENAI_API_KEY` | OpenAI 兼容 API 密钥 | 无，必填 |
| `OPENAI_BASE_URL` | API 基础地址。留空用 OpenAI 官方，也可以用任何兼容服务 | 空 |
| `DATABASE_HOST` | PostgreSQL 主机 | `localhost` |
| `DATABASE_PORT` | PostgreSQL 端口 | `5432` |
| `DATABASE_USER` | 数据库用户名 | `admin` |
| `DATABASE_PASSWORD` | 数据库密码 | `changeme` |
| `DATABASE_DBNAME` | 数据库名称 | `agent_infra` |
| `QDRANT_HOST` | Qdrant 主机 | `localhost` |
| `QDRANT_PORT` | Qdrant 端口 | `6333` |
| `CONFIG_PATH` | 配置文件路径 | `config.yaml`（当前工作目录） |

**注意**：`docker-compose.yaml` 中的数据库默认值是 `agent/agent123/agent_infra`，而 `config.yaml` 默认值是 `admin/changeme/agent_infra`。如果你用 Docker Compose 启动依赖但不用 Docker 运行服务端，记得把 `config.yaml` 里的数据库参数对齐，或者通过环境变量覆盖。

## config.yaml 详解

```yaml
server:
  port: "8088"                    # HTTP 监听端口

database:
  host: "localhost"               # PostgreSQL 主机
  port: 5432                      # PostgreSQL 端口
  user: "admin"                   # 数据库用户
  password: "changeme"            # 数据库密码
  dbname: "copcon"                # 数据库名

openai:
  api_key: ""                     # API Key，空值时会读环境变量 OPENAI_API_KEY
  base_url: ""                    # 空值 = OpenAI 官方；填值 = 自定义端点
  model: "gpt-4o"                 # 默认模型（会被 agent 级别的配置覆盖）

qdrant:
  host: "localhost"               # Qdrant 主机（未配置时为 no-op）
  port: 6333                      # Qdrant 端口

default_agent_id: "code-assistant"  # 创建会话时的默认 Agent

agents:
  - id: "code-assistant"          # Agent 唯一标识
    name: "Code Assistant"        # 显示名称
    model: "gpt-4o"               # 该 Agent 使用的模型
    base_url: ""                  # 可选：覆盖全局 base_url
    system_prompt: "You are a helpful coding assistant..."
    tools:                        # 该 Agent 可用的工具列表
      - "code_executor"
      - "shell_executor"
      - "file_ops"
      - "todolist"

  - id: "chat-assistant"
    name: "Chat Assistant"
    model: "gpt-4o"
    system_prompt: "You are a friendly chat assistant..."
    tools: []                     # 空列表 = 纯对话，无工具
```

### 工具列表参考

内置工具：

| 工具 ID | 说明 |
|---|---|
| `code_executor` | 执行代码片段 |
| `shell_executor` | 执行 Shell 命令 |
| `file_ops` | 文件读写操作 |
| `todolist` | 任务列表管理 |
| `get_tool_status` | 查询异步工具状态 |
| `get_tool_result` | 获取异步工具结果 |
| `cancel_tool` | 取消异步工具 |
| `list_async_tools` | 列出异步工具 |

### 切换模型

换模型只需要改 `config.yaml` 中的 `openai.model` 或每个 agent 的 `model` 字段。例如：

```yaml
openai:
  api_key: "sk-xxx"
  base_url: "https://api.deepseek.com/v1"
  model: "deepseek-chat"

agents:
  - id: "chat-assistant"
    name: "Chat Assistant"
    model: "deepseek-chat"
    system_prompt: "..."
    tools: []
```

只要目标服务兼容 OpenAI 的 `/v1/chat/completions` 格式就行。

## 安装验证

```bash
cd server

# 拉取依赖
go mod tidy

# 编译验证
go build ./cmd/server

# 编译通过就说明环境没问题
echo "安装成功"
```

如果 `go build` 失败，通常是因为 Go 版本不对（需要 1.26+）或者网络不通（`go mod tidy` 拉取失败）。

## Docker 部署

项目提供了完整的 Docker Compose 配置，一键启动所有服务：

```bash
cd /path/to/copcon

# 启动所有服务（PostgreSQL + Qdrant + LiteLLM + 服务器）
docker compose up -d
```

这会构建 `server/Dockerfile` 镜像并启动整个栈。服务配置通过环境变量注入（见 `docker-compose.yaml` 中 `server` 服务的 `environment` 节）。

## 下一步

- [快速开始](quickstart.md) — 验证安装并发送第一个请求
- [Hello World](hello-world.md) — 写一个自定义 Agent 程序