# 部署指南

## 系统要求

### 最低配置

| 组件 | 要求 |
|------|------|
| CPU | 4 核 |
| 内存 | 8 GB |
| 磁盘 | 50 GB SSD |
| 操作系统 | Ubuntu 22.04 / CentOS 8 / Debian 12 |
| Docker | 24.0+ |
| Docker Compose | 2.20+ |

### 推荐配置（生产环境）

| 组件 | 要求 |
|------|------|
| CPU | 16 核 |
| 内存 | 32 GB |
| 磁盘 | 500 GB NVMe SSD |
| 网络 | 1 Gbps |

## 部署模式

### Docker Compose 部署（推荐）

适用于中小规模部署，单机运行所有组件。

```bash
# 克隆配置仓库
git clone https://github.com/copcon/deploy.git
cd deploy/compose

# 配置环境变量
cp .env.example .env
# 编辑 .env 文件，设置必要的配置项

# 启动所有服务
docker compose up -d

# 验证服务状态
docker compose ps
curl http://localhost:8080/health
```

### Kubernetes 部署

适用于大规模生产环境，支持水平扩展和高可用。

```bash
# 添加 Helm 仓库
helm repo add copcon https://charts.copcon.io
helm repo update

# 创建命名空间
kubectl create namespace copcon

# 安装
helm install copcon copcon/copcon \
  --namespace copcon \
  --values values-production.yaml

# 验证
kubectl get pods -n copcon
```

Helm Chart 支持以下自定义配置：
- 副本数和资源限制
- 存储类和持久卷大小
- Ingress 和 TLS 证书
- 环境变量注入
- 亲和性和反亲和性规则

### 独立部署

各组件可独立部署，适用于需要灵活定制的场景。核心组件包括：

1. **API Server**：处理所有 API 请求，无状态设计
2. **Worker**：执行异步任务（文档导入、向量计算等）
3. **PostgreSQL**：主数据库，存储会话和配置数据
4. **Redis**：缓存和消息队列
5. **向量数据库**：存储嵌入向量，支持 SQLite-vec 或 Qdrant

## 环境变量

| 变量名 | 说明 | 默认值 |
|-------|------|--------|
| `COPCON_ENV` | 运行环境 | `development` |
| `COPCON_PORT` | API 服务端口 | `8080` |
| `DATABASE_URL` | PostgreSQL 连接串 | 必填 |
| `REDIS_URL` | Redis 连接串 | `localhost:6379` |
| `VECTOR_STORE` | 向量存储类型 | `sqlite-vec` |
| `OPENAI_API_KEY` | OpenAI API 密钥 | 必填 |
| `LOG_LEVEL` | 日志级别 | `info` |

## 健康检查

部署完成后，通过以下端点验证服务健康状态：

```bash
# 基础健康检查
curl http://localhost:8080/health

# 详细状态（含依赖组件）
curl http://localhost:8080/health/detailed

# 就绪检查（Kubernetes）
curl http://localhost:8080/ready
```

健康检查返回各组件状态：数据库连接、Redis 连通性、向量存储可用性、磁盘空间等。

## 数据备份

建议配置定时备份任务：

```bash
# PostgreSQL 备份
pg_dump -h $DATABASE_HOST -U admin copcon > backup_$(date +%Y%m%d).sql

# 向量数据备份（SQLite-vec）
cp /data/copcon/vectors.db /backup/vectors_$(date +%Y%m%d).db
```

备份频率建议：数据库每日备份，向量数据每周全量备份。备份文件加密后存储到异地。
