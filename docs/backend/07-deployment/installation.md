# 安装指南

本指南覆盖 CopCon 的所有安装方式,从开发环境到生产部署。

## 系统要求

### 最低配置

| 资源 | 要求 | 说明 |
|------|------|------|
| 操作系统 | Linux / macOS / Windows | 生产环境推荐 Linux (Ubuntu 22.04+, Debian 12+) |
| CPU | 2 核 | SSE 流式响应对单核性能敏感 |
| 内存 | 2 GB | 服务器本体 ~200MB,余量留给 PostgreSQL |
| 磁盘 | 10 GB | 含日志和数据库初始空间 |
| Go | 1.26+ | 仅源码编译时需要 |

### 推荐生产配置

| 资源 | 配置 | 说明 |
|------|------|------|
| CPU | 4 核+ | 支撑高并发 SSE 连接 |
| 内存 | 8 GB | 数据库连接池 + 缓存 + Agent 上下文 |
| 磁盘 | 50 GB+ SSD | 数据库 I/O 对延迟影响显著 |
| 网络 | 低延迟出口 | LLM API 调用对网络延迟敏感 |

### 外部依赖

| 依赖 | 版本 | 必需 | 说明 |
|------|------|------|------|
| PostgreSQL | 15.x+ | 是 | 会话、消息、任务持久化 |
| Qdrant | 1.17.x | 否 | 向量记忆存储,未配置时记忆功能静默跳过 |
| LLM API | OpenAI 兼容 | 是 | 支持直连或经 LiteLLM 代理 |

## 安装方式一: 预编译二进制

最简单的生产部署方式,无需 Go 工具链。

### 下载

从 [GitHub Releases](https://github.com/copcon/copcon/releases) 下载对应平台二进制:

```bash
# Linux amd64
curl -L -o copcon-server https://github.com/copcon/copcon/releases/latest/download/copcon-server-linux-amd64
chmod +x copcon-server

# macOS arm64
curl -L -o copcon-server https://github.com/copcon/copcon/releases/latest/download/copcon-server-darwin-arm64
chmod +x copcon-server
```

### 安装到系统路径

```bash
sudo mv copcon-server /usr/local/bin/copcon-server
sudo chown root:root /usr/local/bin/copcon-server
```

### 创建运行用户

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin copcon
sudo mkdir -p /etc/copcon /var/lib/copcon /var/log/copcon
sudo chown copcon:copcon /var/lib/copcon /var/log/copcon
```

### 放置配置文件

```bash
sudo cp config.yaml /etc/copcon/config.yaml
sudo chown copcon:copcon /etc/copcon/config.yaml
sudo chmod 600 /etc/copcon/config.yaml  # 含密钥,限制读取权限
```

### 验证

```bash
copcon-server --version
copcon-server --config /etc/copcon/config.yaml --validate
```

## 安装方式二: Docker

适合快速部署和容器化环境。详见 [Docker 部署指南](docker.md)。

```bash
# 拉取镜像
docker pull ghcr.io/copcon/copcon-server:latest

# 运行
docker run -d \
  --name copcon-server \
  -p 8080:8080 \
  -e CONFIG_PATH=/app/config.yaml \
  -v ./config.yaml:/app/config.yaml:ro \
  ghcr.io/copcon/copcon-server:latest
```

## 安装方式三: 源码编译

适合需要自定义修改或开发调试的场景。

### 前置条件

```bash
# 安装 Go 1.26+
go version

# 克隆仓库
git clone https://github.com/copcon/copcon.git
cd copcon
```

### 编译服务器

```bash
# 编译 server 模块
cd server
go build -o copcon-server ./cmd/server

# 也可以编译数据库初始化工具
go build -o copcon-init-db ./cmd/init-db
```

### 交叉编译

```bash
# Linux amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o copcon-server-linux-amd64 ./cmd/server

# Linux arm64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o copcon-server-linux-arm64 ./cmd/server

# macOS arm64
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o copcon-server-darwin-arm64 ./cmd/server
```

### 安装到 $GOPATH/bin

```bash
cd server
go install ./cmd/server
# 二进制位于 $(go env GOPATH)/bin/server
```

## 安装方式四: Docker Compose (全栈)

一键拉起 CopCon 及其所有依赖,适合开发和测试环境。

```bash
# 克隆仓库
git clone https://github.com/copcon/copcon.git
cd copcon

# 配置环境变量
cp .env.example .env
# 编辑 .env,至少填入一个 LLM API Key

# 启动全部服务
docker compose up -d

# 仅启动依赖(自己跑 Go 二进制)
docker compose up -d postgres qdrant
```

## 初始配置

无论用哪种安装方式,都需要完成以下配置步骤。

### 1. 准备配置文件

最小配置 `config.yaml`:

```yaml
server:
  port: "8080"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"    # 生产环境务必替换,或用环境变量覆盖
  dbname: "copcon"

openai:
  api_key: "${OPENAI_API_KEY}"
  base_url: "https://api.openai.com/v1"
  model: "gpt-4o"

qdrant:
  host: "localhost"
  port: 6333

default_agent_id: "assistant"

agents:
  - id: "assistant"
    name: "Assistant"
    model: "gpt-4o"
    system_prompt: "You are a helpful assistant."
    tools: []
```

### 2. 环境变量覆盖

敏感配置不要写进配置文件,用环境变量传入:

```bash
export OPENAI_API_KEY="sk-your-key-here"
export DATABASE_PASSWORD="your-strong-password"
```

| 环境变量 | 对应配置 | 必需 |
|---------|---------|------|
| `CONFIG_PATH` | 配置文件路径 | 否,默认 `config.yaml` |
| `OPENAI_API_KEY` | `openai.api_key` | 是 |
| `OPENAI_BASE_URL` | `openai.base_url` | 否 |
| `DATABASE_HOST` | `database.host` | 否 |
| `DATABASE_PORT` | `database.port` | 否 |
| `DATABASE_USER` | `database.user` | 否 |
| `DATABASE_PASSWORD` | `database.password` | 是 |
| `DATABASE_DBNAME` | `database.dbname` | 否 |
| `QDRANT_HOST` | `qdrant.host` | 否 |
| `QDRANT_PORT` | `qdrant.port` | 否 |

### 3. 初始化数据库

```bash
# 方式 A: 使用 Go 工具
cd server && go run cmd/init-db/main.go

# 方式 B: 使用 shell 脚本
bash scripts/init-db.sh

# 方式 C: 手动 psql
PGPASSWORD=changeme psql -h localhost -U admin -d copcon -f server/internal/session/schema.sql
```

初始化会创建 `sessions` 和 `messages` 表,以及相关索引和触发器。

### 4. 初始化 Qdrant (可选)

```bash
# 方式 A: 使用 shell 脚本
bash scripts/init-qdrant.sh

# 方式 B: 手动 curl
curl -X PUT "http://localhost:6333/collections/agent_memory" \
  -H "Content-Type: application/json" \
  -d '{
    "vectors": { "size": 1536, "distance": "Cosine" }
  }'
```

跳过此步骤不会报错,记忆 Hook 会静默跳过注册。

## 首次运行验证

### 1. 启动服务

```bash
# 二进制方式
cd server && go run cmd/server/main.go

# 或使用编译好的二进制
./copcon-server --config /etc/copcon/config.yaml
```

### 2. 健康检查

```bash
curl http://localhost:8080/health
# 期望: {"status":"ok"}
```

### 3. 创建会话

```bash
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"title": "Test Session"}'
```

记录返回的 `id`。

### 4. 发送消息

```bash
curl -X POST http://localhost:8080/api/sessions/{session-id}/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello!"}'
```

### 5. 验证 SSE 流

```bash
# 使用 curl 观察 SSE 事件流
curl -N http://localhost:8080/api/sessions/{session-id}/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Tell me a joke"}'
```

应该看到 `data:` 开头的流式事件逐行到达。

## systemd 服务配置 (Linux)

生产环境建议用 systemd 管理 CopCon 进程:

```ini
# /etc/systemd/system/copcon.service
[Unit]
Description=CopCon Agent Server
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=copcon
Group=copcon
ExecStart=/usr/local/bin/copcon-server
Environment=CONFIG_PATH=/etc/copcon/config.yaml
EnvironmentFile=/etc/copcon/env
Restart=on-failure
RestartSec=5
StartLimitBurst=3
StartLimitIntervalSec=60

# 安全加固
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/copcon /var/lib/copcon
PrivateTmp=true

# 资源限制
LimitNOFILE=65536
MemoryMax=2G

[Install]
WantedBy=multi-user.target
```

环境变量文件 `/etc/copcon/env`:

```bash
OPENAI_API_KEY=sk-your-key
DATABASE_PASSWORD=your-strong-password
```

启用并启动:

```bash
sudo systemctl daemon-reload
sudo systemctl enable copcon
sudo systemctl start copcon
sudo systemctl status copcon
```

查看日志:

```bash
journalctl -u copcon -f
```

## 版本确认与升级

```bash
# 查看当前版本
copcon-server --version

# 升级时替换二进制后重启
sudo systemctl restart copcon
```

升级注意事项详见 [升级与迁移指南](upgrade.md)。

## 下一步

- [Docker 部署](docker.md)
- [Kubernetes 部署](kubernetes.md)
- [生产检查清单](production-checklist.md)
