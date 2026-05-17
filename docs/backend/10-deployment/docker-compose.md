# Docker Compose 部署

## 概述

CopCon 提供完整的 Docker Compose 配置，一键启动所有依赖服务。部署包含四个容器：PostgreSQL（会话存储）、Qdrant（向量记忆）、LiteLLM（LLM 代理）、CopCon Server（应用服务）。

## docker-compose.yml 概览

文件位置：`/docker-compose.yaml`

```yaml
services:
  postgres:    # PostgreSQL 15 — 会话和消息持久化
  qdrant:      # Qdrant 1.17 — 向量记忆存储
  litellm:     # LiteLLM — 多模型代理网关
  server:      # CopCon Server — 核心应用
```

### 服务依赖关系

```
server ──depends_on──▶ postgres (health check)
                    ▶ qdrant  (health check)
                    ▶ litellm (health check)

litellm ─extends──▶ OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.
```

## 各服务详解

### PostgreSQL

```yaml
postgres:
  image: postgres:15-alpine
  container_name: copcon-postgres
  environment:
    POSTGRES_USER: agent
    POSTGRES_PASSWORD: agent123
    POSTGRES_DB: agent_infra
  ports:
    - "5432:5432"
  volumes:
    - postgres_data:/var/lib/postgresql/data
  healthcheck:
    test: ["CMD-SHELL", "pg_isready -U agent"]
    interval: 5s
    timeout: 5s
    retries: 5
```

- 使用 Alpine 镜像，体积小
- 数据持久化到 `postgres_data` 命名卷
- 健康检查通过 `pg_isready` 确保数据库就绪后才启动依赖它的服务

### Qdrant

```yaml
qdrant:
  image: qdrant/qdrant:v1.17.0
  container_name: copcon-qdrant
  ports:
    - "6333:6333"    # HTTP API
    - "6334:6334"    # gRPC API
  volumes:
    - qdrant_data:/qdrant/storage
  healthcheck:
    test: ["CMD", "curl", "-f", "http://localhost:6333/health"]
    interval: 5s
    timeout: 5s
    retries: 5
```

- 向量数据持久化到 `qdrant_data` 命名卷
- 暴露 HTTP (6333) 和 gRPC (6334) 端口
- 首次启动后需运行 `scripts/init-qdrant.sh` 初始化 Collection

### LiteLLM

```yaml
litellm:
  image: ghcr.io/berriai/litellm:main-stable
  container_name: copcon-litellm
  ports:
    - "4000:4000"
  environment:
    - LITELLM_MASTER_KEY=${LITELLM_MASTER_KEY:-sk-litellm-key}
    - MOONSHOT_API_KEY=${MOONSHOT_API_KEY:-}
    - OPENAI_API_KEY=${OPENAI_API_KEY:-}
    - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY:-}
    - GEMINI_API_KEY=${GEMINI_API_KEY:-}
    - DEEPSEEK_API_KEY=${DEEPSEEK_API_KEY:-}
  volumes:
    - ./config/litellm-config.yaml:/app/config.yaml:ro
  command: ["--config", "/app/config.yaml", "--port", "4000"]
```

- LiteLLM 作为 LLM 代理，统一对多个模型提供商的 API 调用
- API Keys 通过环境变量注入，默认值留空
- 配置文件以只读方式挂载

### CopCon Server

```yaml
server:
  build:
    context: ./server
    dockerfile: Dockerfile
  container_name: copcon-server
  ports:
    - "8080:8080"
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
  depends_on:
    postgres:
      condition: service_healthy
    qdrant:
      condition: service_healthy
    litellm:
      condition: service_healthy
  volumes:
    - ./server/config.yaml:/app/config.yaml:ro
```

- 从 Dockerfile 构建
- 数据库和 Qdrant 使用 Docker Compose 内部网络的服务名作为 Host
- OPENAI_API_KEY 使用 LiteLLM 主密钥，通过 LiteLLM 代理调用所有模型
- 通过 `depends_on` + `condition: service_healthy` 确保依赖服务就绪后才启动
- `config.yaml` 以只读方式挂载

### 数据卷

```yaml
volumes:
  postgres_data:    # PostgreSQL 数据
  qdrant_data:      # Qdrant 向量数据
  litellm_logs:     # LiteLLM 日志
```

所有数据通过 Docker 命名卷持久化，即使容器删除数据也不会丢失。

## 构建与启动

### 环境准备

创建 `.env` 文件（或在 shell 中 export）：

```bash
# 必需（至少一个 Provider 的 API Key）
export OPENAI_API_KEY="sk-your-openai-key"
export MOONSHOT_API_KEY="your-moonshot-key"

# 可选
export LITELLM_MASTER_KEY="sk-your-custom-litellm-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
export DEEPSEEK_API_KEY="your-deepseek-key"
export GEMINI_API_KEY="your-gemini-key"
```

参考 `.env.example` 查看完整模板。

### 构建并启动

```bash
# 构建镜像并启动所有服务（首次或代码变更后）
docker compose up -d --build

# 仅启动（已有镜像）
docker compose up -d
```

### 验证服务状态

```bash
# 查看所有容器状态
docker compose ps

# 查看日志
docker compose logs -f server
docker compose logs -f litellm

# 验证健康检查
curl http://localhost:8080/health
# → {"status":"ok"}

# 验证 LiteLLM
curl http://localhost:4000/health
# → ... healthy ...
```

### 初始化 Qdrant Collection

```bash
# 首次启动后运行
bash scripts/init-qdrant.sh
```

### 停止服务

```bash
# 停止所有容器（保留数据卷）
docker compose down

# 停止并删除数据卷（⚠ 所有数据丢失）
docker compose down -v
```

## 数据持久化

| 数据 | 存储位置 | 删除方式 |
|------|---------|---------|
| 会话和消息 | `postgres_data` 卷 | `docker compose down -v` |
| 向量记忆 | `qdrant_data` 卷 | `docker compose down -v` |
| LiteLLM 日志 | `litellm_logs` 卷 | `docker compose down -v` |
| 配置文件 | 宿主机文件，只读挂载 | 直接编辑宿主机文件 |

配置文件变更后需重启对应服务：

```bash
docker compose restart server   # config.yaml 变更
docker compose restart litellm  # litellm-config.yaml 变更
```

## 健康检查

所有服务均配置了健康检查：

| 服务 | 检查方式 | 间隔 | 超时 | 重试 |
|------|---------|------|------|------|
| postgres | `pg_isready -U agent` | 5s | 5s | 5 |
| qdrant | `curl http://localhost:6333/health` | 5s | 5s | 5 |
| litellm | `curl http://localhost:4000/health` | 30s | 10s | 3 |
| server | `GET /health` | 由 `depends_on` 保证 | - | - |

## 网络架构

所有容器在同一个 Docker Compose 默认网络中，通过容器名直接通信：

```
外部请求 → server:8080
              ├── postgres:5432 (会话存储)
              ├── qdrant:6333  (向量存储)
              └── litellm:4000 (LLM 代理)
                       └── OpenAI / Moonshot / Claude / Gemini / DeepSeek
```

## 生产环境调整

### 安全加固

```yaml
# 生产环境建议
server:
  environment:
    - DATABASE_PASSWORD=${DB_PASSWORD}     # 从环境变量读取，不硬编码
    - OPENAI_API_KEY=${OPENAI_API_KEY}     # 同上，不写死在配置中
  ports:
    - "127.0.0.1:8080:8080"               # 仅监听本地，通过反向代理暴露
```

### 资源限制

```yaml
server:
  deploy:
    resources:
      limits:
        cpus: '2'
        memory: 2G
      reservations:
        cpus: '0.5'
        memory: 512M
```

### 自动重启

```yaml
server:
  restart: unless-stopped          # 异常退出自动重启
```