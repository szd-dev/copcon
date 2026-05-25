# Docker 部署指南

本指南详细说明如何用 Docker 和 Docker Compose 部署 CopCon,涵盖单机开发和生产级配置。

## 官方镜像

CopCon 提供官方 Docker 镜像:

```
ghcr.io/copcon/copcon-server:latest
ghcr.io/copcon/copcon-server:v2.0.0   # 指定版本
ghcr.io/copcon/copcon-server:sha-abc123  # 指定 commit
```

镜像基于 Alpine Linux,仅包含编译好的二进制和默认配置,体积约 20MB。

## Dockerfile 解析

项目自带的多阶段构建 Dockerfile:

```dockerfile
# 阶段一: 编译
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server
# CGO_ENABLED=0 is compatible with the pure-Go SQLite driver (modernc.org/sqlite),
# no C compiler required in the build or runtime image

# 阶段二: 运行
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/server .
COPY config.yaml .
EXPOSE 8080
CMD ["./server"]
```

### 自定义构建

如果需要修改编译参数或添加 CA 证书:

```dockerfile
FROM golang:1.26-alpine AS builder

# 企业环境可能需要自定义 CA
RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 添加编译信息
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
  -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
  -o server ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /app/server .
COPY config.yaml .

EXPOSE 8080

# 非 root 用户运行
RUN adduser -D -H copcon
USER copcon

CMD ["./server"]
```

## Docker Compose 部署

项目根目录提供了完整的 `docker-compose.yaml`,包含四个服务:

| 服务 | 镜像 | 端口 | 说明 |
|------|------|------|------|
| postgres | postgres:15-alpine | 5432 | 会话和消息存储 |
| qdrant | qdrant/qdrant:v1.17.0 | 6333, 6334 | 向量记忆 |
| server | 本地构建 | 8080 | CopCon 服务 |

### 启动全栈

```bash
# 配置环境变量
# 编辑 server/config.yaml，填入你的 API Key（可从 config.yaml.template 复制）

# 启动所有服务
docker compose up -d

# 查看状态
docker compose ps

# 查看日志
docker compose logs -f server
```

### 仅启动依赖

如果你想在宿主机上运行 CopCon 二进制:

```bash
docker compose up -d postgres qdrant

# 然后本地运行
cd server && go run cmd/server/main.go
```

### SQLite-only deployment (no external dependencies)

If you don't need PostgreSQL or Qdrant, you can run the server with just SQLite:

```bash
cp server/config.yaml.sqlite.template server/config.yaml
# Edit config.yaml to set your API key
cd server && go run cmd/server/main.go
```

No Docker services are needed. Data is stored in `data/copcon.db`.

### 停止和清理

```bash
# 停止服务,保留数据卷
docker compose down

# 停止并删除数据卷(数据会丢失!)
docker compose down -v
```

## 数据持久化

Docker Compose 定义了三个数据卷:

```yaml
volumes:
  postgres_data:   # PostgreSQL 数据
  qdrant_data:     # Qdrant 向量数据
```

生产环境必须确保这些卷映射到持久存储。默认使用 Docker 命名卷,数据存在 `/var/lib/docker/volumes/` 下。

### 绑定挂载到宿主机

```yaml
services:
  postgres:
    volumes:
      - /data/copcon/postgres:/var/lib/postgresql/data  # 替代命名卷
  qdrant:
    volumes:
      - /data/copcon/qdrant:/qdrant/storage
```

这样做的好处:
- 直接访问备份文件
- 不依赖 Docker 卷驱动
- 方便迁移

### 配置文件挂载

配置文件以只读方式挂载,防止容器内进程意外修改:

```yaml
server:
  volumes:
    - ./server/config.yaml:/app/config.yaml:ro
```

## 环境变量

CopCon 服务容器的环境变量配置:

```yaml
server:
  environment:
    - CONFIG_PATH=/app/config.yaml
    - DATABASE_HOST=postgres          # Docker 网络中的服务名
    - DATABASE_PORT=5432
    - DATABASE_USER=agent
    - DATABASE_PASSWORD=agent123
    - DATABASE_DBNAME=agent_infra
    - QDRANT_HOST=qdrant
    - QDRANT_PORT=6333
```

要点:
- 容器间通信使用 Docker Compose 服务名(如 `postgres`, `qdrant`)
- LLM API Key 和 Base URL 在 `server/config.yaml` 中配置，挂载到 `/app/config.yaml:ro`

## 生产级 Docker Compose

开发用 `docker-compose.yaml` 不适合直接上生产。以下是一个加固版本:

```yaml
version: "3.9"

services:
  postgres:
    image: postgres:15-alpine
    restart: always
    environment:
      POSTGRES_USER: ${DB_USER}
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_DB: copcon
    volumes:
      - /data/copcon/postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${DB_USER}"]
      interval: 10s
      timeout: 5s
      retries: 5
    deploy:
      resources:
        limits:
          memory: 2G
          cpus: "2"
    networks:
      - backend

  qdrant:
    image: qdrant/qdrant:v1.17.0
    restart: always
    volumes:
      - /data/copcon/qdrant:/qdrant/storage
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:6333/health"]
      interval: 10s
      timeout: 5s
      retries: 5
    deploy:
      resources:
        limits:
          memory: 1G
          cpus: "1"
    networks:
      - backend

  server:
    image: ghcr.io/copcon/copcon-server:latest
    # 如果自己构建:
    # build:
    #   context: ./server
    #   dockerfile: Dockerfile
    restart: always
    ports:
      - "127.0.0.1:8080:8080"  # 仅本机可访问,由反向代理转发
    environment:
      CONFIG_PATH: /app/config.yaml
      DATABASE_HOST: postgres
      DATABASE_PORT: "5432"
      DATABASE_USER: ${DB_USER}
      DATABASE_PASSWORD: ${DB_PASSWORD}
      DATABASE_DBNAME: copcon
      QDRANT_HOST: qdrant
      QDRANT_PORT: "6333"
      OPENAI_API_KEY: ${OPENAI_API_KEY}
      OPENAI_BASE_URL: ${OPENAI_BASE_URL:-https://api.openai.com/v1}
    depends_on:
      postgres:
        condition: service_healthy
      qdrant:
        condition: service_healthy
    volumes:
      - /etc/copcon/config.yaml:/app/config.yaml:ro
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 10s
    deploy:
      resources:
        limits:
          memory: 1G
          cpus: "2"
    logging:
      driver: json-file
      options:
        max-size: "50m"
        max-file: "5"
    networks:
      - backend

networks:
  backend:
    driver: bridge
```

与开发版的关键差异:

| 方面 | 开发版 | 生产版 |
|------|--------|--------|
| 端口绑定 | `0.0.0.0:8080` | `127.0.0.1:8080` |
| 重启策略 | `unless-stopped` | `always` |
| 资源限制 | 无 | CPU/内存上限 |
| 日志管理 | 默认 | 轮转,50MB x 5 |
| 网络 | 默认 | 自定义 bridge |
| 密码 | 明文 | 环境变量 |
| 健康检查 | 有 | 有,间隔更长 |

## 网络配置

### 容器间通信

Docker Compose 自动创建网络,服务间通过服务名互访:

```
server → postgres:5432
server → qdrant:6333
```

### 对外暴露

CopCon 服务端口 `8080` 需要通过反向代理对外提供:

```
客户端 → Nginx/Caddy(443) → copcon-server(8080)
```

生产环境不要直接暴露 CopCon 端口。用 `127.0.0.1:8080:8080` 限制为仅本机访问,由 Nginx 转发。

### Nginx 反向代理配置

```nginx
server {
    listen 443 ssl http2;
    server_name copcon.example.com;

    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;

    # SSE 需要
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 300s;  # LLM 可能慢,给够超时

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # SSE 端点特别处理
    location /api/sessions/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;
    }
}
```

SSE 的关键配置是 `proxy_buffering off`,否则 Nginx 会缓冲流式响应,客户端收不到实时更新。

## 镜像安全

### 扫描漏洞

```bash
# 使用 Trivy
trivy image ghcr.io/copcon/copcon-server:latest

# 使用 Docker Scout
docker scout cves ghcr.io/copcon/copcon-server:latest
```

### 签名验证

```bash
# 验证镜像签名(如果启用了 cosign)
cosign verify ghcr.io/copcon/copcon-server:latest
```

### 最佳实践清单

1. **不要用 `latest` 标签部署生产**。锁定具体版本号。
2. **定期更新基础镜像**。Alpine 镜像每月检查更新。
3. **以非 root 用户运行**。在 Dockerfile 中添加 `USER copcon`。
4. **只读根文件系统**。`docker run --read-only --tmpfs /tmp`。
5. **限制能力**。`docker run --cap-drop=ALL --cap-add=NET_BIND_SERVICE`。
6. **不挂载 Docker socket**。`/var/run/docker.sock` 不应出现在 CopCon 容器中。

## 日志管理

### Docker 日志驱动

```yaml
server:
  logging:
    driver: json-file
    options:
      max-size: "50m"
      max-file: "5"
```

### 集中日志收集

如果使用 ELK 或 Loki:

```yaml
server:
  logging:
    driver: fluentd
    options:
      fluentd-address: localhost:24224
      tag: copcon.server
```

## 健康检查

Docker Compose 已定义健康检查。手动验证:

```bash
# 检查容器健康状态
docker inspect --format='{{.State.Health.Status}}' copcon-server

# 手动触发健康检查
docker exec copcon-server wget -q --spider http://localhost:8080/health
```

## 更新策略

### 滚动更新

```bash
# 拉取新镜像
docker compose pull server

# 重建并启动(仅更新有变化的容器)
docker compose up -d --no-deps server
```

### 零停机更新

单机 Docker Compose 无法做到零停机。如果需要零停机,考虑:
- 多实例 + 负载均衡
- 蓝绿部署脚本
- 迁移到 Kubernetes

## 下一步

- [Kubernetes 部署](kubernetes.md)
- [生产检查清单](production-checklist.md)
- [备份与恢复](backups.md)
