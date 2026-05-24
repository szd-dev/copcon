# Server 应用概览

Server 是一个轻量级的 REST API 服务，提供完整的 Agent 管理、会话管理和对话功能。它基于 core 库构建，作为 CopCon 的示例应用。

## 架构

Server 采用简洁的分层架构：

```
┌──────────────────────────────────────┐
│ HTTP Layer (server/internal/api)      │
├──────────────────────────────────────┤
│ Service Layer (server/internal/session) │
├──────────────────────────────────────┤
│ Domain Layer (server/internal/domain)   │
└──────────────────────────────────────┘
```

## 核心模块

### 1. HTTP API (`internal/api`)

提供 RESTful API 接口，包括：

- **会话管理**
  - 创建会话
  - 获取会话列表
  - 获取/更新/删除会话
  - 会话配置管理

- **Agent 管理**
  - Agent 配置
  - Agent 生命周期管理
  - Agent 状态监控

- **消息管理**
  - 发送消息
  - 获取消息历史
  - 流式响应处理

- **工具管理**
  - 工具注册
  - 工具状态查询
  - 工具调用监控

### 2. 会话服务 (`internal/session`)

处理业务逻辑，包括：

- 会话的创建、更新、删除
- Agent 配置管理
- 上下文管理
- 消息历史记录
- 流式事件推送

### 3. 领域模型 (`internal/domain`)

定义核心实体和接口：

- `Session`: 会话实体
- `Agent`: Agent 实体
- `Message`: 消息实体
- `ChatContext`: 对话上下文
- `ToolCall`: 工具调用

## 主要特性

### 1. 多租户支持

Server 原生支持多租户，通过 session_id 隔离：

```go
type Session struct {
    ID          string
    UserID      string
    AgentConfig string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 2. 流式响应

基于 Server-Sent Events (SSE) 实现实时流式响应：

```go
func (s *Service) SendMessage(ctx context.Context, sessionID string, message string) error {
    // 创建事件流
    stream := s.engine.Chat(ctx, message)
    
    // 推送事件到客户端
    for event := range stream {
        s.pushEvent(sessionID, event)
    }
    
    return nil
}
```

### 3. Agent 热更新

支持在不重启服务的情况下更新 Agent 配置：

```yaml
agents:
  - name: code-assistant
    model: gpt-4
    system_prompt: "You are a helpful coding assistant..."
    tools:
      - name: code_execution
        config: {...}
```

### 4. 工具调用监控

实时跟踪工具的调用状态：

- 工具调用开始
- 执行中
- 执行完成
- 结果返回

## API 端点

### 会话管理

| 方法 | 端点 | 描述 |
|------|-------|------|
| POST | `/api/sessions` | 创建新会话 |
| GET | `/api/sessions` | 获取会话列表 |
| GET | `/api/sessions/{id}` | 获取会话详情 |
| PUT | `/api/sessions/{id}` | 更新会话 |
| DELETE | `/api/sessions/{id}` | 删除会话 |

### Agent 管理

| 方法 | 端点 | 描述 |
|------|-------|------|
| GET | `/api/agents` | 获取 Agent 列表 |
| PUT | `/api/agents/{name}` | 更新 Agent 配置 |
| POST | `/api/agents/{name}/restart` | 重启 Agent |

### 消息管理

| 方法 | 端点 | 描述 |
|------|-------|------|
| POST | `/api/sessions/{id}/messages` | 发送消息 |
| GET | `/api/sessions/{id}/messages` | 获取消息历史 |
| DELETE | `/api/sessions/{id}/messages` | 清空消息 |

### 工具管理

| 方法 | 端点 | 描述 |
|------|-------|------|
| GET | `/api/tools` | 获取可用工具列表 |
| GET | `/api/tools/{name}/status` | 获取工具状态 |

## 配置详解

Server 使用 YAML 格式的配置文件：

```yaml
server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 300s

database:
  driver: postgres
  dsn: ${DATABASE_URL}
  max_open_conns: 100
  max_idle_conns: 10

logging:
  level: info
  format: json
  output: stdout

authentication:
  enabled: false
  secret_key: ${JWT_SECRET}
  token_expiry: 24h

features:
  streaming: true
  multi_tenant: true
  hot_reload: true
```

## 部署选项

### 1. 独立部署

作为独立服务部署：

```yaml
version: '3.8'
services:
  server:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://user:pass@db:5432/copcon
  db:
    image: postgres:15
    environment:
      - POSTGRES_USER=user
      - POSTGRES_PASSWORD=pass
      - POSTGRES_DB=copcon
```

### 2. 反向代理

通过 Nginx 等反向代理提供服务：

```nginx
upstream server {
    server server:8080;
}

server {
    listen 80;
    server_name api.example.com;
    
    location /api/ {
        proxy_pass http://server;
        proxy_http_version 1.1;
        proxy_set_header Connection '';
        proxy_buffering off;
        proxy_cache off;
    }
}
```

### 3. 容器编排

使用 Kubernetes 进行容器编排：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: server
spec:
  replicas: 3
  selector:
    matchLabels:
      app: server
  template:
    metadata:
      labels:
        app: server
    spec:
      containers:
      - name: server
        image: copcon/server:latest
        ports:
        - containerPort: 8080
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: database-secret
              key: url
---
apiVersion: v1
kind: Service
metadata:
  name: server
spec:
  selector:
    app: server
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
```

## 监控和日志

### 健康检查

Server 提供健康检查端点：

```go
// GET /health
{
    "status": "healthy",
    "version": "1.0.0",
    "uptime": "24h",
    "sessions": {
        "active": 42,
        "total": 1000
    }
}
```

### 指标导出

支持 Prometheus 指标导出：

```go
// GET /metrics
# HELP server_requests_total Total number of requests
# TYPE server_requests_total counter
server_requests_total{method="GET",endpoint="/api/sessions"} 1000
server_requests_total{method="POST",endpoint="/api/sessions"} 500

# HELP server_request_duration_seconds Request duration in seconds
# TYPE server_request_duration_seconds histogram
server_request_duration_seconds_bucket{le="0.1"} 100
server_request_duration_seconds_bucket{le="0.5"} 400
```

### 结构化日志

输出 JSON 格式的结构化日志：

```json
{
  "level": "info",
  "timestamp": "2026-05-25T10:30:00Z",
  "message": "Session created",
  "session_id": "abc123",
  "user_id": "user456",
  "duration_ms": 15
}
```

## 安全考虑

### 1. 身份认证

支持多种认证方式：

- JWT Token
- API Key
- OAuth 2.0

### 2. 权限控制

基于角色的访问控制 (RBAC)：

```go
type Permission struct {
    Action string  // "create", "read", "update", "delete"
    Resource string  // "session", "agent", "message"
}

type Role struct {
    Name        string
    Permissions []Permission
}
```

### 3. 输入验证

所有 API 输入都经过严格验证：

```go
func CreateSessionSchema() *Schema {
    return &Schema{
        Fields: map[string]Field{
            "title": {
                Type:     StringType,
                Required: true,
                MaxLen:   100,
            },
            "agent_id": {
                Type:     StringType,
                Required: true,
                Pattern:  `^[a-z0-9-]+$`,
            },
        },
    }
}
```

## 性能优化

### 1. 连接池

使用数据库连接池提高性能：

```go
db, err := sql.Open("postgres", dsn)
db.SetMaxOpenConns(100)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(time.Hour)
```

### 2. 缓存

使用 Redis 缓存热点数据：

```go
type Cache struct {
    client *redis.Client
    ttl    time.Duration
}

func (c *Cache) GetSession(sessionID string) (*Session, error) {
    // 先查缓存
    if session, ok := c.client.Get(sessionID); ok {
        return session, nil
    }
    
    // 缓存未命中，查询数据库
    session, err := c.db.GetSession(sessionID)
    if err != nil {
        return nil, err
    }
    
    // 写入缓存
    c.client.Set(sessionID, session, c.ttl)
    
    return session, nil
}
```

### 3. 消息队列

使用消息队列异步处理耗时操作：

```go
type MessageQueue struct {
    producer kafka.Producer
    consumer kafka.Consumer
}

func (q *MessageQueue) PublishMessage(msg *Message) error {
    return q.producer.Produce(&KafkaMessage{
        Topic: "messages",
        Value: msg,
    })
}
```

## 故障排查

### 常见问题

1. **数据库连接失败**
   - 检查数据库连接字符串
   - 确认数据库服务正常运行
   - 检查网络连通性

2. **内存使用过高**
   - 增加 GC 频率
   - 减小连接池大小
   - 清理过期会话

3. **响应延迟**
   - 启用缓存
   - 优化数据库查询
   - 增加服务器资源

## 下一步

- 查看 [API 详细文档](./api-reference.md)
- 学习 [自定义 Handler](./customization.md)
- 了解 [部署最佳实践](../07-deployment/production-checklist.md)
