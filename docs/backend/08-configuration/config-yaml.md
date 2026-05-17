# config.yaml 配置参考

`config.yaml` 是 CopCon 的纯文本配置文件，由 `config.Load()` 在启动时加载，定义服务端口、数据库连接、LLM 后端、Agent 列表等全局设置。

**文件位置**: `server/config.yaml`（可通过 `CONFIG_PATH` 环境变量覆盖）

## 完整结构

```yaml
server:
  port: "8080"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "copcon"

openai:
  api_key: ""
  base_url: ""
  model: "gpt-4o"

qdrant:
  host: "localhost"
  port: 6333

default_agent_id: "code-assistant"

agents:
  - id: "code-assistant"
    name: "Code Assistant"
    model: "gpt-4o"
    system_prompt: "You are a helpful coding assistant..."
    tools: ["code_executor", "shell_executor", "file_ops", "todolist"]
  - id: "chat-assistant"
    name: "Chat Assistant"
    model: "gpt-4o"
    system_prompt: "You are a friendly chat assistant..."
    tools: []
```

## 顶层字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `server` | `ServerConfig` | 是 | HTTP 服务配置 |
| `database` | `DatabaseConfig` | 是 | PostgreSQL 连接配置 |
| `openai` | `OpenAIConfig` | 是 | OpenAI 兼容 API 配置 |
| `qdrant` | `QdrantConfig` | 是 | Qdrant 向量数据库配置 |
| `agents` | `[]AgentConfig` | 是 | Agent 定义列表 |
| `default_agent_id` | `string` | 否 | 默认 Agent ID |

## ServerConfig

```yaml
server:
  port: "8088"
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `port` | `string` | Gin HTTP 服务监听端口 |

## DatabaseConfig

```yaml
database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "copcon"
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `host` | `string` | PostgreSQL 主机地址 |
| `port` | `int` | PostgreSQL 端口 |
| `user` | `string` | 数据库用户名 |
| `password` | `string` | 数据库密码 |
| `dbname` | `string` | 数据库名称 |

GORM 根据这些字段构建 DSN 连接字符串。

## OpenAIConfig

```yaml
openai:
  api_key: "sk-xxx"
  base_url: "https://api.openai.com/v1"
  model: "gpt-4o"
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `api_key` | `string` | API 密钥（可被环境变量覆盖） |
| `base_url` | `string` | API 基础 URL，兼容任意 OpenAI 格式的服务 |
| `model` | `string` | 全局默认模型名 |

`api_key` 可通过 `OPENAI_API_KEY` 环境变量覆盖（见下方）。`base_url` 支持切换到兼容服务（如 360 智脑、DeepSeek 等）。

## QdrantConfig

```yaml
qdrant:
  host: "localhost"
  port: 6333
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `host` | `string` | Qdrant 服务地址 |
| `port` | `int` | Qdrant HTTP API 端口 |

## AgentConfig

每个 Agent 定义包括身份、模型、提示词和可用工具：

```yaml
agents:
  - id: "code-assistant"
    name: "Code Assistant"
    model: "z-ai/glm-5"
    system_prompt: "You are a helpful coding assistant..."
    tools: ["code_executor", "shell_executor", "file_ops", "todolist"]
    base_url: ""   # 可选，覆盖全局 openai.base_url
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | `string` | 是 | Agent 唯一标识，不可重复 |
| `name` | `string` | 是 | 展示名称 |
| `model` | `string` | 是 | 此 Agent 使用的模型（覆盖全局 `openai.model`） |
| `system_prompt` | `string` | 是 | 系统提示词 |
| `tools` | `[]string` | 否 | 可用工具名称列表，空数组表示无工具 |
| `base_url` | `string` | 否 | 此 Agent 专用的 API 地址（覆盖全局 `openai.base_url`） |

### 工具配置

`tools` 字段指定 Agent 可用的工具名称。每个名称必须对应 `ToolRegistry` 中已注册的工具。引擎在创建 Agent 时验证所有工具存在性，未找到的工具会导致初始化失败：

```go
for _, toolName := range agentConfig.Tools {
    if _, err := toolRegistry.Get(toolName); err != nil {
        return nil, fmt.Errorf("agent %s: tool not found: %s", agentConfig.ID, toolName)
    }
}
```

当前可用的工具名称：`code_executor`, `shell_executor`, `file_ops`, `todolist`。

## default_agent_id

指定默认 Agent。当请求未指定 Agent ID 且 Session 无默认 Agent 时，使用此 Agent：

```yaml
default_agent_id: "code-assistant"
```

`config.Load()` 会验证 `default_agent_id` 对应的 Agent 是否存在于 `agents` 列表中，不存在时返回错误。

## 环境变量覆盖

`config.Load()` 在解析 YAML 后检查环境变量，允许运行时覆盖配置文件中的值：

```go
if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
    cfg.OpenAI.APIKey = apiKey
}
```

| 环境变量 | 覆盖字段 | 说明 |
|---------|---------|------|
| `OPENAI_API_KEY` | `openai.api_key` | API 密钥（推荐使用，避免硬编码到文件） |
| `CONFIG_PATH` | 配置文件路径 | 加载非默认位置的配置文件 |

数据库和 Qdrant 的连接信息也通常通过环境变量配置（由 Docker Compose 注入）：

| 环境变量 | 对应字段 | 默认值 |
|---------|---------|--------|
| `DATABASE_HOST` | `database.host` | `localhost` |
| `DATABASE_PORT` | `database.port` | `5432` |
| `DATABASE_USER` | `database.user` | `admin` |
| `DATABASE_PASSWORD` | `database.password` | `changeme` |
| `DATABASE_DBNAME` | `database.dbname` | `agent_infra` |
| `QDRANT_HOST` | `qdrant.host` | `localhost` |
| `QDRANT_PORT` | `qdrant.port` | `6333` |

注意：数据库和 Qdrant 的环境变量覆盖不在 `config.Load()` 中实现，而是在连接初始化时由各 Manager 自行处理。

## 配置验证

`config.Load()` 调用 `validate()` 执行以下检查：

1. Agent ID 不可重复
2. `default_agent_id` 如果非空，必须在 `agents` 列表中

验证失败返回具体错误信息，引擎不会以无效配置启动。

## 获取特定 Agent 配置

```go
func (c *Config) GetAgent(id string) (AgentConfig, error)
```

通过 Agent ID 查找配置。未找到时返回错误。此方法用于读取原始配置结构，与 `AgentRegistry.Get()` 返回的 `AgentDefinition`（包含已初始化的 ToolManager 和 LLMProvider）是不同层次的操作。

## 完整配置示例

```yaml
server:
  port: "8080"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "copcon"

openai:
  api_key: ""
  base_url: "https://api.360.cn/v1"
  model: "z-ai/glm-5"

qdrant:
  host: "localhost"
  port: 6333

default_agent_id: "code-assistant"

agents:
  - id: "code-assistant"
    name: "Code Assistant"
    model: "z-ai/glm-5"
    system_prompt: "You are a helpful coding assistant. You can write, analyze, and debug code. You have access to code execution, shell commands, and file operations. Always provide clear explanations and best practices."
    tools: ["code_executor", "shell_executor", "file_ops", "todolist"]

  - id: "chat-assistant"
    name: "Chat Assistant"
    model: "z-ai/glm-5"
    system_prompt: "You are a friendly chat assistant. You engage in natural conversations, answer questions, and provide helpful information. You don't have access to tools, so focus on text-based responses."
    tools: []
```