# 生产环境部署清单

## 概述

将 CopCon 投入生产环境前，请逐项检查以下配置，确保系统稳定、安全和可观测。

## 1. 数据库

### 连接池配置

GORM 使用 `database/sql` 连接池，通过 GORM 配置控制：

```go
import (
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

sqlDB, _ := db.DB()

// 最大打开连接数
sqlDB.SetMaxOpenConns(25)

// 最大空闲连接数
sqlDB.SetMaxIdleConns(10)

// 连接最大存活时间
sqlDB.SetConnMaxLifetime(5 * time.Minute)

// 空闲连接最大存活时间
sqlDB.SetConnMaxIdleTime(1 * time.Minute)
```

**建议配置：**

| 参数 | 建议值 | 说明 |
|------|--------|------|
| `MaxOpenConns` | 20-50 | 根据 CPU 核心数和并发量调整 |
| `MaxIdleConns` | 10-20 | 保持一定空闲连接避免频繁建立 |
| `ConnMaxLifetime` | 5min | 防止连接泄漏 |
| `ConnMaxIdleTime` | 1min | 空闲连接及时回收 |

### 备份策略

```bash
# 定时全量备份 (crontab: 每天凌晨 2 点)
0 2 * * * pg_dump -h localhost -U agent agent_infra > /backups/copcon_$(date +\%Y\%m\%d).sql

# WAL 归档（生产推荐）
# postgresql.conf:
wal_level = replica
archive_mode = on
archive_command = 'cp %p /wal_archive/%f'
```

**推荐方案：** 使用 pgBackRest 或 pg_dump + WAL 归档，RPO < 5 分钟。

> ⚠ 目前 CopCon 的 `sslmode=disable`。生产环境必须启用 TLS（见第 3 节）。

### 自动迁移

CopCon 启动时执行 `db.AutoMigrate(&session.Session{}, &session.Message{}, &session.Todo{})`，自动创建或更新表结构。

**注意：** AutoMigrate 仅创建列，不删除列，不修改列约束。重大 schema 变更需手动执行迁移脚本。

---

## 2. Qdrant

### Collection 初始化

首次部署后必须运行初始化脚本：

```bash
bash scripts/init-qdrant.sh
```

该脚本创建名为 `copcon` 的 Collection，配置向量维度和索引参数。

### 内存调优

Qdrant 默认将索引映射到内存。根据数据集大小调整：

```yaml
# docker-compose.yaml
qdrant:
  environment:
    - QDRANT__STORAGE__OPTIMIZERS__DEFAULT_SEGMENT_NUMBER=2
    - QDRANT__STORAGE__OPTIMIZERS__MEMEORY_THRESHOLD=50000
```

**建议：**

| 向量记录数 | 建议内存 | 说明 |
|-----------|---------|------|
| < 10万 | 512MB | 默认配置即可 |
| 10万 - 100万 | 2GB | 调整 `MEMEORY_THRESHOLD` |
| > 100万 | 4GB+ | 考虑使用磁盘索引 |

### 持久化

```yaml
volumes:
  - qdrant_data:/qdrant/storage
```

确保 `qdrant_data` 卷有足够的磁盘空间。每条 1536 维向量约占用 6KB，计算存储需求：

```
存储空间 ≈ 向量数量 × 6KB × 1.3 (索引开销)
```

---

## 3. SSL/TLS

CopCon Server 本身不内置 TLS 支持。生产环境应通过反向代理（Nginx / Caddy）终止 TLS。

### Nginx 配置示例

```nginx
server {
    listen 443 ssl http2;
    server_name api.your-domain.com;

    ssl_certificate     /etc/ssl/certs/server.crt;
    ssl_certificate_key /etc/ssl/keys/server.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        # SSE 支持
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
    }
}
```

**关键配置：**
- `proxy_buffering off` — SSE 流式推送必须关闭缓冲
- `proxy_read_timeout 3600s` — 长时间 SSE 连接的超时
- `proxy_set_header Connection "upgrade"` — WebSocket / SSE 升级支持

**PostgreSQL TLS：** 将 DSN 中的 `sslmode=disable` 改为 `sslmode=require` 或 `sslmode=verify-full`。

---

## 4. 速率限制

CopCon 本身不内置速率限制器。应通过以下方案实现：

### 方案 A：反向代理限流（推荐）

使用 Nginx `limit_req` 或 Caddy `rate_limit` 模块。

```nginx
# 定义限流区域：每秒 5 个请求，burst 10
limit_req_zone $binary_remote_addr zone=chat_api:10m rate=5r/s;

location /api/sessions/ {
    limit_req zone=chat_api burst=10 nodelay;
    proxy_pass http://127.0.0.1:8080;
}
```

### 方案 B：Gin 中间件（每 Session 限流）

参考 [Session Middleware](../11-advanced/session-middleware.md) 章节，实现自定义 Gin 中间件在 API 层做速率限制。

### 方案 C：LiteLLM 限流

LiteLLM 内置速率限制和费用追踪。在 `litellm-config.yaml` 中配置：

```yaml
litellm_settings:
  request_timeout: 600
  set_verbose: true
  num_retries: 3

router_settings:
  routing_strategy: "usage-based-routing"
  allowed_fails: 3
  num_retries: 3
```

---

## 5. 监控

### slog 结构化日志

CopCon 使用 Go 标准库 `log/slog` 输出结构化日志到 stderr：

```
time=2026-05-17T10:30:00.000Z level=INFO msg="llm_response" session_id=xxx reasoning_len=150 content_len=512 tool_calls=1 prompt_tokens=1200 completion_tokens=400 total_tokens=1600
time=2026-05-17T10:30:01.000Z level=ERROR msg="llm_stream_error" session_id=xxx error="context deadline exceeded"
```

**日志集成：**
- 容器化部署：日志输出到 stderr，由 Docker 日志驱动收集
- 生产推荐：使用 Loki + Grafana 或 ELK Stack 聚合日志
- 设置日志级别：`slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))`

### Hook 指标

通过 Hook 系统可以在关键生命周期点埋点收集指标：

```go
// 示例：性能监控 Hook
type MetricsHook struct{}

func (h *MetricsHook) Execute(ctx *hook.HookContext) error {
    if ctx.CurrentPoint == hook.AfterLLMCall {
        // 记录 LLM 调用延迟、Token 消耗
        metrics.RecordLLMCall(ctx.SessionID, ctx.AgentID, latency, tokens)
    }
    if ctx.CurrentPoint == hook.AfterToolExecute {
        // 记录工具调用延迟
        metrics.RecordToolCall(ctx.SessionID, ctx.ToolName, latency)
    }
    return nil
}
```

### 健康检查端点

```
GET /health → {"status": "ok"}
```

此端点检查服务进程存活。如需深度检查（数据库、Qdrant 连接），需自行扩展。

---

## 6. 优雅关闭

### 实现方案

```go
func main() {
    // ... 启动代码 ...

    r := gin.Default()
    // ... 路由注册 ...

    srv := &http.Server{
        Addr:    ":" + cfg.Server.Port,
        Handler: r,
    }

    // 在 goroutine 中启动，主线程等待信号
    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("listen: %s\n", err)
        }
    }()

    // 等待中断信号
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    log.Println("Shutting down server...")

    // 设置关闭超时
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // 关闭 HTTP 服务器
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatalf("Server forced to shutdown: %v", err)
    }

    // 关闭数据库连接
    sqlDB, _ := db.DB()
    sqlDB.Close()

    log.Println("Server exited")
}
```

**关键配置：**
- `Shutdown` 超时：30 秒（应大于最长请求处理时间）
- `signal.Notify`：捕获 SIGINT (Ctrl+C) 和 SIGTERM (docker stop)
- 关闭顺序：HTTP Server → 数据库连接池 → 退出

### Docker Compose 停止行为

```yaml
server:
  stop_grace_period: 30s     # 给 30 秒优雅关闭
```

---

## 7. 安全检查清单

- [ ] `OPENAI_API_KEY` 不写在 config.yaml 中，通过环境变量注入
- [ ] API 服务通过反向代理暴露，不直接监听 `0.0.0.0`
- [ ] PostgreSQL `sslmode` 设为 `require` 或 `verify-full`
- [ ] CORS 配置仅允许受信域名
- [ ] 所有外部端口（除 443）在防火墙封闭
- [ ] `.env` 文件不在镜像中
- [ ] 定期更新基础镜像（`postgres:15-alpine`, `qdrant/qdrant:latest`）