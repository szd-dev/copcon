# 故障排查指南

本指南列出 CopCon 常见问题的诊断步骤和解决方法。

## 常见错误与解决

### 启动失败

#### "Failed to connect to database"

```
Error: Failed to connect
  host=localhost port=5432 user=admin password=changeme dbname=copcon
  pq: connection refused
```

**原因**: PostgreSQL 未运行,或连接参数错误。

**排查步骤**:

```bash
# 1. PostgreSQL 是否运行?
pg_isready -h localhost -p 5432
# 如果返回 "no response", PostgreSQL 未启动

# Docker 环境
docker compose ps postgres
docker compose logs postgres

# 2. 连接参数是否匹配 config.yaml?
# 检查 database.host, database.port, database.user, database.password, database.dbname

# 3. 环境变量是否覆盖了配置?
env | grep DATABASE_

# 4. 数据库是否存在?
psql -h localhost -U admin -d postgres -c "SELECT datname FROM pg_database WHERE datname='copcon';"

# 5. 用户权限?
psql -h localhost -U admin -d postgres -c "\du"
```

**解决**:
- 启动 PostgreSQL: `docker compose up -d postgres` 或 `sudo systemctl start postgresql`
- 创建数据库: `cd server && go run cmd/init-db/main.go`
- 修正连接参数: 检查 config.yaml 和环境变量的一致性

#### "Config file not found"

```
Error: open config.yaml: no such file or directory
```

**原因**: 配置文件路径不正确。

**排查**:

```bash
# CONFIG_PATH 环境变量
echo $CONFIG_PATH

# 文件是否存在?
ls -la /etc/copcon/config.yaml
ls -la ./config.yaml
```

**解决**:
- 设置 `CONFIG_PATH=/etc/copcon/config.yaml`
- 或将 config.yaml 放到工作目录

#### "duplicate agent ID"

```
Error: duplicate agent ID: assistant
```

**原因**: config.yaml 中有重复的 agent ID。

**解决**: 检查 `agents` 列表,确保每个 `id` 唯一。

### 运行中问题

#### SSE 连接断开

客户端收到 SSE 事件后连接中断。

**常见原因**:
1. 反向代理缓冲了 SSE 流(Nginx `proxy_buffering on`)
2. 请求超时(代理层或 LLM 层)
3. LLM API 返回错误导致连接关闭
4. 网络中间层(防火墙/WAF)干预长连接

**排查**:

```bash
# 1. 直接访问 CopCon(绕过代理)测试
curl -N http://localhost:8080/api/sessions/{id}/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "hello"}'

# 如果直接连接正常,问题在代理层

# 2. 检查 Nginx 配置
nginx -T | grep proxy_buffering
# 应为 proxy_buffering off

# 3. 检查超时配置
nginx -T | grep proxy_read_timeout
# 建议至少 300s

# 4. 查看 CopCon 日志
journalctl -u copcon -f | grep "sse\|stream\|error"
```

**解决**:
- Nginx: 添加 `proxy_buffering off;` 和 `proxy_read_timeout 300s;`
- Cloud Run: 设置 `--timeout=300`
- Kubernetes Ingress: 添加注解 `nginx.ingress.kubernetes.io/proxy-buffering: "off"`

#### LLM API 调用失败

```
Error: LLM request failed: 429 Too Many Requests
```

**原因**: LLM API 限流或 Key 无效。

**排查**:

```bash
# 1. 测试 API Key
curl -s https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY" | jq '.error'

# 2. 测试连通性(经过 LiteLLM)
curl -s http://litellm:4000/v1/models \
  -H "Authorization: Bearer $LITELLM_MASTER_KEY"

# 3. 查看 LiteLLM 日志
docker compose logs litellm --tail 100

# 4. 检查 base_url 配置
grep base_url config.yaml
```

**解决**:
- 429 限流: 降低并发,或增加 API Key 的 rate limit
- 401 无效 Key: 更换 API Key
- 网络不通: 检查 DNS 和防火墙
- LiteLLM 代理: 确保 LiteLLM 健康检查通过

#### 数据库连接池耗尽

```
Error: pq: sorry, too many clients already
```

**原因**: 活跃连接数超过 PostgreSQL `max_connections`,或 GORM 连接池配置不当。

**排查**:

```bash
# 1. PostgreSQL 当前连接数
psql -c "SELECT count(*) FROM pg_stat_activity WHERE datname='copcon';"

# 2. PostgreSQL 最大连接数
psql -c "SHOW max_connections;"

# 3. CopCon 进程的数据库连接
# 查看指标
curl -s http://localhost:8080/metrics | grep copcon_db_connections

# 4. 连接来源分布
psql -c "SELECT state, count(*) FROM pg_stat_activity WHERE datname='copcon' GROUP BY state;"
```

**解决**:
- 增加 PostgreSQL `max_connections`(需要重启)
- 减小 GORM 连接池大小(`max_connections` 和 `max_idle_connections` 在 config.yaml 中)
- 检查是否有慢查询占用连接(`pg_stat_activity` 中 `state=active` 且 `query_start` 很久前)
- 检查是否有连接泄漏(未正确关闭)

#### Qdrant 不可用

```
Error: Qdrant health check failed: connection refused
```

**排查**:

```bash
curl -s http://localhost:6333/health
# 正常: {"status":"ok"}

curl -s http://localhost:6333/collections/agent_memory | jq '.result.status'
# 正常: "green"
```

**解决**:
- CopCon 的记忆 Hook 在 Qdrant 不可用时静默跳过,不会导致服务崩溃
- 如果记忆功能重要,重启 Qdrant: `docker compose restart qdrant`
- 如果 collection 丢失,重新初始化: `bash scripts/init-qdrant.sh`

## 性能问题诊断

### 响应慢

**症状**: API 响应延迟 P95 > 1 秒,用户感知明显。

**排查步骤**:

```
延迟来自哪里?
├── CopCon 本身?
│   ├── 查看 http_request_duration_seconds 指标
│   └── 如果非聊天端点慢 → 检查数据库查询
│   └── 如果 /health 慢 → 进程可能资源不足
│
├── LLM API?
│   ├── 查看 copcon_llm_request_duration_seconds 指标
│   └── P95 > 5s → LLM 侧问题,考虑换模型或增加超时
│   └── 频繁 429 → 限流,需要调整并发策略
│
├── 数据库?
│   ├── 查看 copcon_db_query_duration_seconds 指标
│   ├── 检查 pg_stat_activity 的慢查询
│   └── 检查索引是否命中(EXPLAIN ANALYZE)
│
└── 网络?
    ├── curl 测试 CopCon → LLM API 延迟
    └── curl 测试 CopCon → PostgreSQL 延迟
```

### 内存持续增长

**症状**: 内存使用曲线持续上升,不回落。

**排查**:

```bash
# 1. 查看内存趋势(Grafana 仪表盘)
# process_resident_memory_bytes 持续上升?

# 2. Goroutine 数量
curl -s http://localhost:8080/metrics | grep go_goroutines
# 持续增长 → goroutine 泄漏

# 3. 活跃 SSE 连接数
curl -s http://localhost:8080/metrics | grep copcon_sse_connections_active
# 持续增长 → 连接未正确关闭

# 4. Go 内存详情
curl -s http://localhost:8080/metrics | grep go_memstats
```

**常见内存泄漏原因**:
- SSE 连接未关闭: 客户端断开后 CopCon 未检测到
- Goroutine 泄漏: Agent 循环中 goroutine 未退出
- ChatContext 未释放: 活跃会话的 ChatContext 在内存中
- 数据库连接未归还: 查询异常时连接未释放回池

**解决**:
- 确保客户端正确关闭 SSE 连接
- 检查 ChatContext 的超时机制
- 添加 goroutine 监控和清理逻辑
- 设置进程内存上限(Docker/Kubernetes `resources.limits.memory`)

### CPU 使用率高

**排查**:

```bash
# 1. 查看进程 CPU
top -p $(pgrep copcon-server)

# 2. Go runtime CPU profile(如果启用了 pprof)
curl -s http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof

# 3. 查看活跃 goroutine
curl -s http://localhost:8080/debug/pprof/goroutine > goroutine.prof
go tool pprof goroutine.prof
```

## 连接问题

### Docker 网络不通

**症状**: CopCon 容器无法连接 postgres/qdrant 容器。

```bash
# 查看容器网络
docker network ls
docker network inspect copcon_default

# 从 CopCon 容器内测试
docker exec copcon-server wget -q --spider http://postgres:5432 2>&1 || echo "Cannot reach postgres"
docker exec copcon-server wget -q --spider http://qdrant:6333/health 2>&1 || echo "Cannot reach qdrant"
```

**解决**:
- 确保 `depends_on` 和 `condition: service_healthy` 配置正确
- 确保所有服务在同一 Docker 网络中
- 环境变量使用服务名(如 `DATABASE_HOST=postgres`),而非 IP 地址

### Kubernetes Pod 无法连接 Service

```bash
# Pod 内测试
kubectl -n copcon exec -it copcon-server-xxx -- sh
wget -q --spider http://copcon-postgres:5432 2>&1
wget -q --spider http://copcon-qdrant:6333/health 2>&1

# 检查 Service
kubectl -n copcon get svc
kubectl -n copcon get endpoints copcon-postgres
kubectl -n copcon get endpoints copcon-qdrant
```

**解决**:
- 确保 Service selector 匹配 Pod labels
- 确保 Endpoints 有条目(Pod 注册到了 Service)
- 检查 NetworkPolicy 是否阻止了通信

## 日志分析技巧

### 查看实时日志

```bash
# systemd
journalctl -u copcon -f --since "5 minutes ago"

# Docker
docker compose logs -f server --tail 100

# Kubernetes
kubectl -n copcon logs -l app.kubernetes.io/name=copcon-server -f --tail=100
```

### 搜索错误

```bash
# systemd
journalctl -u copcon --since "1 hour ago" | grep "level=ERROR\|error\|panic\|fatal"

# Docker
docker compose logs server --since 1h | grep "error"

# Kubernetes
kubectl -n copcon logs -l app.kubernetes.io/name=copcon-server --since=1h | grep "error"
```

### 结构化日志查询

如果日志输出为 JSON 格式:

```bash
# 查看所有错误
cat /var/log/copcon/copcon.log | jq 'select(.level=="ERROR")'

# 查看特定会话
cat /var/log/copcon/copcon.log | jq 'select(.session_id=="550e8400-...")'

# 查看慢请求
cat /var/log/copcon/copcon.log | jq 'select(.duration_ms > 5000)'
```

## Debug 模式

### 启用详细日志

临时启用 debug 日志(注意: debug 日志量很大,可能包含敏感数据):

```bash
# 环境变量方式
export COPCON_LOG_LEVEL=debug
sudo systemctl restart copcon

# 或在 config.yaml 中修改
log:
  level: "debug"  # 仅在排查问题时临时启用
```

### Go pprof(如果启用)

如果 CopCon 启用了 pprof 端点(通常在 debug 端口):

```bash
# CPU profile
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30

# 内存 profile
go tool pprof http://localhost:8080/debug/pprof/heap

# Goroutine
go tool pprof http://localhost:8080/debug/pprof/goroutine

# 交互式分析
(pprof) top 10
(pprof) list funcName
(pprof) web  # 生成火焰图(需要 graphviz)
```

## 问题报告模板

提交 Issue 时请包含:

```
## 环境
- CopCon 版本: v2.0.0
- 部署方式: Docker Compose / Kubernetes / 二进制
- Go 版本: 1.26
- PostgreSQL 版本: 15.x
- Qdrant 版本: 1.17.x (或 "未使用")
- 操作系统: Ubuntu 22.04

## 现象
[描述具体现象,包含错误日志]

## 复现步骤
1. ...
2. ...

## 日志
[粘贴相关日志,注意脱敏 API Key]

## 已尝试的排查
[列出你已做的排查步骤]
```

## 获取帮助

- GitHub Issues: https://github.com/copcon/copcon/issues
- GitHub Discussions: https://github.com/copcon/copcon/discussions
- 文档: [docs/backend/](../README.md)

## 下一步

- [监控与可观测性](monitoring.md)
- [备份与恢复](backups.md)
- [升级与迁移](upgrade.md)