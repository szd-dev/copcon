# 扩缩容指南

本指南说明如何根据负载变化调整 CopCon 的部署规模,涵盖水平扩缩、垂直扩缩、负载均衡和性能调优。

## 扩缩容决策树

```
需要扩容吗?
├── CPU 使用率持续 > 70%? → 垂直扩容(加 CPU)或水平扩容(加实例)
├── 内存使用率持续 > 80%? → 垂直扩容(加内存)
├── 请求延迟上升? → 查明瓶颈
│   ├── LLM API 慢? → 不能靠扩容解决,考虑缓存或换模型
│   ├── 数据库慢? → 增加连接池或优化查询
│   └── CopCon 本身 CPU 高? → 水平扩容
├── SSE 连接数增长? → 水平扩容
└── 吞吐量不够? → 水平扩容
```

## 水平扩缩

### 原理

CopCon Server 是无状态的(活跃 ChatContext 除外),可以通过增加实例来分摊负载:

```
                  ┌──────────────┐
                  │ Load Balancer│
                  └──────┬───────┘
                         │
            ┌────────────┼────────────┐
            ▼            ▼            ▼
       ┌─────────┐ ┌─────────┐ ┌─────────┐
       │ Pod 1   │ │ Pod 2   │ │ Pod 3   │
       │ server  │ │ server  │ │ server  │
       └─────────┘ └─────────┘ └─────────┘
            │            │            │
            └────────────┼────────────┘
                         │
              ┌──────────┴──────────┐
              ▼                     ▼
       ┌────────────┐        ┌───────────┐
       │ PostgreSQL │        │  Qdrant   │
       └────────────┘        └───────────┘
```

### 会话亲和性

CopCon 的 SSE 请求需要会话亲和(Sticky Session),原因:
- 活跃 ChatContext 存储在创建它的 Pod 内存中
- 同一会话的后续请求(如 stop/resume)必须路由到同一个 Pod
- 新会话的创建请求可以路由到任意 Pod

**配置方式**:

Nginx:

```nginx
upstream copcon {
    # 基于 session ID 的会话亲和
    hash $request_uri consistent;
    server pod1:8080;
    server pod2:8080;
    server pod3:8080;
}
```

Kubernetes (Ingress):

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    nginx.ingress.kubernetes.io/affinity: "cookie"
    nginx.ingress.kubernetes.io/affinity-mode: "persistent"
    nginx.ingress.kubernetes.io/session-cookie-name: "COPCON_SID"
    nginx.ingress.kubernetes.io/session-cookie-path: "/"
```

ALB (AWS):

```bash
aws elbv2 create-target-group \
  --name copcon-tg \
  --target-type ip \
  --stickiness-enabled \
  --stickiness-type lb_cookie \
  --stickiness-duration-seconds 86400
```

### Kubernetes HPA 配置

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: copcon-server-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: copcon-server
  minReplicas: 2
  maxReplicas: 20
  metrics:
    # CPU 触发
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70

    # 内存触发
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80

    # 自定义指标: 活跃 SSE 连接数
    - type: Pods
      pods:
        metric:
          name: copcon_sse_connections_active
        target:
          type: AverageValue
          averageValue: "200"  # 每个 Pod 200 个活跃连接时扩容
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Percent
          value: 50        # 每次扩容最多增加 50%
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300  # 缩容前等 5 分钟
      policies:
        - type: Percent
          value: 10        # 每次缩容最多减少 10%
          periodSeconds: 120
```

### 手动扩缩容

```bash
# Kubernetes
kubectl -n copcon scale deployment/copcon-server --replicas=5

# Docker Compose(不支持自动扩缩,需手动调整)
# 修改 docker-compose.yaml 或启动多个实例
```

### 扩容注意事项

1. **会话亲和**: 扩容后新 Pod 只接收新会话,已有会话仍路由到旧 Pod
2. **数据库连接**: 每个 Pod 都会建立连接池,扩容会增加数据库总连接数
3. **LLM API 限流**: 更多实例意味着更多并发 LLM 请求,可能触发 API 限流
4. **缩容断连**: 缩容关闭 Pod 时,该 Pod 上的活跃 ChatContext 会丢失,客户端需要重连

## 垂直扩缩

### 何时垂直扩容

- 单个请求处理需要更多内存(大上下文、长对话)
- CPU 成为瓶颈(Goroutine 调度慢)
- 不方便水平扩容时(如单机部署)

### 资源推荐

| 场景 | CPU | 内存 | 说明 |
|------|-----|------|------|
| 开发/测试 | 1 核 | 1 GB | 足够,偶尔卡顿 |
| 小团队(< 20 人) | 2 核 | 2 GB | 日常使用流畅 |
| 中等规模(20-100 人) | 4 核 | 4 GB | 推荐起始配置 |
| 大规模(100+ 人) | 8 核+ | 8 GB+ | 配合水平扩缩 |

### Kubernetes 垂直扩容

```yaml
# 方式一: 修改 resources
resources:
  requests:
    cpu: "1"
    memory: "1Gi"
  limits:
    cpu: "4"
    memory: "4Gi"

# 方式二: VPA (Vertical Pod Autoscaler)
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: copcon-server-vpa
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: copcon-server
  updatePolicy:
    updateMode: "Auto"  # 自动调整 Pod 资源
  resourcePolicy:
    containerPolicies:
      - containerName: server
        minAllowed:
          cpu: "500m"
          memory: "256Mi"
        maxAllowed:
          cpu: "8"
          memory: "8Gi"
```

### Docker 垂直扩容

```yaml
server:
  deploy:
    resources:
      limits:
        cpus: "4"
        memory: 4G
      reservations:
        cpus: "1"
        memory: 1G
```

## 负载均衡

### 负载均衡选择

| 负载均衡器 | 适合场景 | SSE 支持 | 会话亲和 |
|-----------|---------|---------|---------|
| Nginx | 自建,小中规模 | 需配置 `proxy_buffering off` | cookie/hash |
| AWS ALB | AWS 部署 | 原生支持 | cookie |
| GCP Cloud LB | GCP 部署 | 原生支持 | cookie |
| HAProxy | 高性能场景 | 原生支持 | cookie |
| Caddy | 简单部署 | 原生支持 | cookie |

### SSE 长连接调优

SSE 连接持续时间长(数秒到数分钟),需要特别调优:

**Nginx**:

```nginx
# 每个 worker 的最大连接数
events {
    worker_connections 10240;  # 默认 512,不够
}

http {
    # 保持连接
    keepalive_timeout 300s;
    keepalive_requests 10000;

    upstream copcon {
        # 长连接到后端
        keepalive 64;
        server pod1:8080;
        server pod2:8080;
    }

    server {
        location / {
            proxy_pass http://copcon;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_buffering off;        # SSE 必须
            proxy_cache off;
            proxy_read_timeout 300s;    # LLM 可能慢
        }
    }
}
```

**系统级调优 (Linux)**:

```bash
# /etc/sysctl.conf
net.core.somaxconn = 65535          # TCP 连接队列
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.ip_local_port_range = 1024 65535
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_keepalive_intvl = 30
net.ipv4.tcp_keepalive_probes = 3

# 应用
sudo sysctl -p
```

## 数据库连接池

### GORM 连接池配置

```yaml
# config.yaml
database:
  host: "localhost"
  port: 5432
  user: "copcon"
  password: "${DATABASE_PASSWORD}"
  dbname: "copcon"

  # 连接池
  max_connections: 100        # 最大打开连接数
  max_idle_connections: 10    # 最大空闲连接数
  connection_max_lifetime: 1h # 连接最大存活时间
  connection_max_idle_time: 15m # 空闲连接最大存活时间
```

### 连接池大小计算

公式: 总连接数 = Pod 数 x 每个 Pod 的 max_connections

| Pod 数 | 每 Pod 连接数 | 总连接数 | PostgreSQL max_connections |
|--------|-------------|---------|--------------------------|
| 2 | 25 | 50 | 100 (留余量) |
| 5 | 25 | 125 | 200 |
| 10 | 25 | 250 | 400 |

经验法则:
- PostgreSQL `max_connections` 设为总连接数的 1.5 倍(预留管理连接)
- 每个 Pod 的 `max_connections` 不要太大,25-50 通常足够
- `max_idle_connections` 设为 `max_connections` 的 1/5 到 1/3

### 连接池监控

```bash
# PostgreSQL 活跃连接
psql -c "SELECT count(*), state FROM pg_stat_activity GROUP BY state;"

# 连接池使用率
curl -s http://localhost:8080/metrics | grep copcon_db_connections
```

## 缓存策略

### 会话缓存

会话列表和消息历史是热点数据。缓存可以减少数据库查询:

```
客户端 → CopCon Server
          ├── Redis 缓存 (命中) → 直接返回
          └── 缓存未命中 → PostgreSQL → 写入缓存
```

如果配置了 Redis 缓存:

```yaml
store:
  cache_type: "redis"
  cache_host: "localhost"
  cache_port: 6379
  cache_ttl: "5m"
```

### LLM 响应缓存

相似请求的 LLM 响应可以缓存,减少 API 调用和延迟:

```yaml
# LiteLLM 配置
litellm_settings:
  cache: true
  cache_params:
    type: "redis"
    host: "redis:6379"
    ttl: 600  # 10 分钟
```

### Qdrant 查询缓存

Qdrant 内置查询缓存,默认启用。通过索引优化查询性能:

```bash
# 确保索引存在
curl -X PUT "http://$QDRANT_HOST:6333/collections/agent_memory/index" \
  -H "Content-Type: application/json" \
  -d '{"field_name": "session_id", "field_schema": "keyword"}'
```

## 性能调优

### Go 运行时调优

```bash
# GOMAXPROCS: 默认等于 CPU 核数,容器中需要特殊处理
# 在 Kubernetes 中,建议使用 automaxprocs
export GOMAXPROCS=$(nproc)

# GOMEMLIMIT: 设置 Go 软内存上限
export GOMEMLIMIT=1GiB
```

在 Go 代码中使用 [automaxprocs](https://github.com/uber-go/automaxprocs):

```go
import _ "go.uber.org/automaxprocs"
```

### 数据库优化

```sql
-- 检查慢查询
SELECT query, calls, mean_exec_time, max_exec_time
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;

-- 确保索引被使用
EXPLAIN ANALYZE SELECT * FROM messages WHERE session_id = 'xxx';

-- 定期清理和统计
VACUUM ANALYZE sessions;
VACUUM ANALYZE messages;
```

### PostgreSQL 配置调优

```ini
# postgresql.conf - 中等规模推荐值
shared_buffers = 1GB           # 内存的 25%
effective_cache_size = 3GB     # 内存的 75%
work_mem = 16MB                # 排序和哈希操作内存
maintenance_work_mem = 256MB   # 维护操作内存
max_connections = 200          # 根据实际需求
random_page_cost = 1.1         # SSD 存储用低值
effective_io_concurrency = 200 # SSD 并发 IO
```

### CopCon 配置调优

```yaml
# 性能相关配置
concurrency:
  max_concurrent_agents: 20     # 同时处理的 Agent 对话数
  max_concurrent_tool_calls: 10  # 同时执行的工具调用数
  tool_queue_size: 200          # 工具调用队列大小

retry:
  max_attempts: 3               # LLM 调用重试次数
  backoff_multiplier: 2.0
  initial_delay: "1s"
  max_delay: "30s"

rate_limit:
  enabled: true
  requests_per_minute: 60       # 每分钟最大请求数
  tokens_per_minute: 90000      # 每分钟最大 token 数
```

## 容量规划

### 基准数据

基于 CopCon v2.0 的基准测试(单 Pod,2 vCPU/2GB):

| 指标 | 值 |
|------|-----|
| 并发 SSE 连接 | ~500 |
| 会话创建 QPS | ~200 |
| 消息发送 QPS | ~50 (受 LLM 延迟限制) |
| 空载内存 | ~200MB |
| 50 连接内存 | ~500MB |
| 500 连接内存 | ~1.5GB |

### 扩容触发建议

| 指标 | 阈值 | 动作 |
|------|------|------|
| CPU > 70% 持续 5 分钟 | 水平扩容 |
| 内存 > 80% 持续 5 分钟 | 检查泄漏,考虑垂直扩容 |
| SSE 连接数 > 400/Pod | 水平扩容 |
| P95 延迟 > 2s | 排查瓶颈,可能扩容 |
| DB 连接池 > 85% | 增大连接池或扩容 |

## 下一步

- [Kubernetes 部署](kubernetes.md)
- [监控与可观测性](monitoring.md)
- [生产检查清单](production-checklist.md)