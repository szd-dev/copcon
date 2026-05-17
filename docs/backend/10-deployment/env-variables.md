# 环境变量参考

## 概述

CopCon 支持通过环境变量覆盖 `config.yaml` 中的配置。环境变量的优先级高于配置文件，适合 Docker Compose 部署和 CI/CD 场景。

## 加载机制

配置加载流程（`config.Load()`）：

1. 读取 `CONFIG_PATH` 环境变量确定配置文件路径，默认为 `config.yaml`
2. 解析 YAML 配置文件
3. 用环境变量覆盖对应的配置字段
4. 校验配置完整性

注意：`OPENAI_API_KEY` 是唯一可以通过环境变量覆盖的配置项。其余字段（数据库、Qdrant 等）的环境变量在 `config.go` 中未映射，它们通过 `os.Getenv()` 在各自的使用点直接读取（如 `main.go` 中的 `buildDSN`）。

## 完整环境变量表

### LLM / OpenAI 配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `OPENAI_API_KEY` | 配置文件中的 `openai.api_key` | OpenAI API 密钥。环境变量优先于配置文件 |
| `OPENAI_BASE_URL` | 配置文件中的 `openai.base_url` | OpenAI API 基础 URL。如果使用 LiteLLM 代理，设为 `http://litellm:4000/v1` |
| `CONFIG_PATH` | `config.yaml` | 配置文件路径 |

### 数据库配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DATABASE_HOST` | `localhost` | PostgreSQL 主机地址 |
| `DATABASE_PORT` | `5432` | PostgreSQL 端口 |
| `DATABASE_USER` | `admin` | 数据库用户名 |
| `DATABASE_PASSWORD` | `changeme` | 数据库密码 |
| `DATABASE_DBNAME` | `agent_infra` | 数据库名称 |

连接字符串格式：`host=<HOST> port=<PORT> user=<USER> password=<PASSWORD> dbname=<DBNAME> sslmode=disable`

### Qdrant 配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `QDRANT_HOST` | `localhost` | Qdrant 服务主机地址 |
| `QDRANT_PORT` | `6333` | Qdrant HTTP API 端口 |
| `QDRANT_COLLECTION` | `copcon` | Collection 名称（在代码中硬编码） |

### LiteLLM 配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `LITELLM_MASTER_KEY` | `sk-litellm-key` | LiteLLM 主密钥，也是 Server 访问 LiteLLM 的 API Key |
| `LITELLM_SALT_KEY` | `sk-litellm-salt` | LiteLLM 加密盐值 |
| `MOONSHOT_API_KEY` | - | Moonshot / Kimi API 密钥 |
| `ANTHROPIC_API_KEY` | - | Anthropic Claude API 密钥 |
| `GEMINI_API_KEY` | - | Google Gemini API 密钥 |
| `DEEPSEEK_API_KEY` | - | DeepSeek API 密钥 |

## 加载优先级

```
环境变量  >  config.yaml
```

以 `OPENAI_API_KEY` 为例：
1. 如果设置了 `export OPENAI_API_KEY="sk-abc"`，则使用 `"sk-abc"`，忽略 config.yaml 中的值
2. 如果未设置环境变量，则使用 config.yaml 中 `openai.api_key` 的值

## 配置示例

### 本地开发

```bash
export OPENAI_API_KEY="sk-your-openai-api-key"
export DATABASE_HOST="localhost"
export DATABASE_PORT="5432"
export DATABASE_USER="admin"
export DATABASE_PASSWORD="changeme"
export DATABASE_DBNAME="copcon"
export QDRANT_HOST="localhost"
export QDRANT_PORT="6333"

cd server && go run ./cmd/server
```

### Docker Compose（docker-compose.yaml 中配置）

```yaml
server:
  environment:
    - CONFIG_PATH=/app/config.yaml
    - DATABASE_HOST=postgres
    - DATABASE_PORT=5432
    - DATABASE_USER=agent
    - DATABASE_PASSWORD=agent123
    - DATABASE_DBNAME=agent_infra
    - QDRANT_HOST=qdrant
    - QDRANT_PORT=6333
    - OPENAI_API_KEY=${LITELLM_MASTER_KEY:-sk-litellm-key}
    - OPENAI_BASE_URL=http://litellm:4000/v1
```

### 生产环境（通过 .env 文件）

```bash
# .env (不提交到 Git)
OPENAI_API_KEY=sk-prod-key-xxxxx
MOONSHOT_API_KEY=moonshot-prod-key-xxxxx
LITELLM_MASTER_KEY=sk-prod-litellm-xxxxx
DATABASE_PASSWORD=secure-db-password
```

```bash
# 启动时加载
set -a && source .env && set +a
docker compose up -d
```

## config.yaml 参考

环境变量的值通常与 config.yaml 中的对应字段一致：

```yaml
server:
  port: "8088"              # Gin 监听端口

database:
  host: "localhost"         # 对应 DATABASE_HOST
  port: 5432                # 对应 DATABASE_PORT
  user: "admin"             # 对应 DATABASE_USER
  password: "changeme"      # 对应 DATABASE_PASSWORD
  dbname: "copcon"          # 对应 DATABASE_DBNAME

openai:
  api_key: ""               # 对应 OPENAI_API_KEY
  base_url: "https://api.360.cn/v1"  # 对应 OPENAI_BASE_URL
  model: "z-ai/glm-5"

qdrant:
  host: "localhost"         # 对应 QDRANT_HOST
  port: 6333                # 对应 QDRANT_PORT

default_agent_id: "code-assistant"

agents:
  - id: "code-assistant"
    name: "Code Assistant"
    model: "z-ai/glm-5"
    system_prompt: "You are a helpful coding assistant..."
    tools: ["code_executor", "shell_executor", "file_ops", "todolist"]
```

## 安全建议

- 永远不要在 config.yaml 中硬编码 API Key（`api_key: ""` 留空，通过环境变量注入）
- `.env` 文件应加入 `.gitignore`
- 生产环境中使用 Docker Secrets 或 Kubernetes Secrets 管理敏感配置
- 定期轮换 API Key
- 为 LiteLLM 设置自定义 `LITELLM_MASTER_KEY` 而不是使用默认值