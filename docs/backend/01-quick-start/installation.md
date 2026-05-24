# 安装与环境配置

## 系统要求

- Go 1.26+
- Docker 和 Docker Compose
- PostgreSQL 15.x (或通过 Docker 运行)
- Qdrant 1.17.x (可选,用于向量存储)

## 克隆项目

```bash
git clone https://github.com/copcon/copcon.git
cd copcon
```

## 快速启动 (使用 Docker Compose)

### 1. 配置环境变量

```bash
cp .env.example .env
```

编辑 `.env` 文件,填入必要的 API Key:

```env
# OpenAI 兼容的 LLM API
OPENAI_API_KEY=your-api-key-here
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_MODEL=gpt-4

# 数据库配置 (Docker Compose 会自动启动)
DATABASE_URL=postgres://postgres:password@localhost:5432/copcon?sslmode=disable

# Qdrant 向量数据库 (可选)
QDRANT_URL=http://localhost:6333
```

### 2. 启动依赖服务

```bash
docker-compose up -d postgres qdrant
```

验证服务状态:

```bash
docker-compose ps
```

### 3. 运行数据库迁移

```bash
cd server
go run cmd/migrate/main.go up
```

### 4. 启动后端服务

```bash
go run cmd/server/main.go
```

服务将在 `http://localhost:8080` 启动。

### 5. 验证安装

```bash
# 健康检查
curl http://localhost:8080/health

# 预期返回
{"status":"ok"}
```

## 手动配置 (不使用 Docker)

如果你选择手动安装依赖:

### PostgreSQL

```bash
# 安装 PostgreSQL 15+
# macOS
brew install postgresql@15

# Ubuntu
sudo apt-get install postgresql-15

# 创建数据库
createdb copcon

# 或者使用 psql
psql -c "CREATE DATABASE copcon;"
```

### Qdrant (可选)

```bash
# 从官网下载: https://qdrant.tech/downloads/
# 或使用 Docker
docker run -d -p 6333:6333 qdrant/qdrant:v1.17.0
```

### 配置文件

创建 `server/config.yaml`:

```yaml
server:
  port: 8080
  host: localhost

database:
  url: postgres://postgres:password@localhost:5432/copcon?sslmode=disable

llm:
  provider: openai
  api_key: ${OPENAI_API_KEY}
  base_url: ${OPENAI_BASE_URL}
  model: ${OPENAI_MODEL}

vector_store:
  provider: qdrant
  url: ${QDRANT_URL}

log:
  level: info
  format: json
```

## 开发环境

### 热重载

使用 `air` 实现热重载:

```bash
# 安装 air
go install github.com/air-verse/air@latest

# 启动开发服务器
air
```

### 运行测试

```bash
cd server
go test ./... -v
```

## 故障排查

### 端口冲突

如果 8080 端口被占用,修改 `config.yaml`:

```yaml
server:
  port: 8081  # 改为其他端口
```

### 数据库连接失败

1. 确认 PostgreSQL 正在运行:
   ```bash
   pg_isready
   ```

2. 检查连接字符串格式是否正确

3. 确认数据库已创建:
   ```bash
   psql -l | grep copcon
   ```

### LLM API 错误

1. 确认 API Key 有效
2. 检查 base_url 是否正确
3. 验证网络连通性:
   ```bash
   curl -I ${OPENAI_BASE_URL}/models
   ```

## 下一步

- [Hello World - 第一个 Agent 应用](hello-world.md)
- [运行完整 Demo](run-demo.md)
