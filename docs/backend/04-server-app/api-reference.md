# API 参考

CopCon Server 提供完整的 RESTful API，所有端点都返回 JSON 格式的响应。

## 基础配置

- **基础 URL**: `http://localhost:8080`
- **认证方式**: Bearer Token (可选)
- **Content-Type**: `application/json`

## 通用响应格式

所有 API 响应都遵循以下格式：

### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": { ... }
}
```

### 错误响应

```json
{
  "code": 400001,
  "message": "Validation error: title is required",
  "data": null
}
```

## 错误码定义

| 错误码 | 描述 | HTTP 状态码 |
|---------|------|-------------|
| 0 | 成功 | 200 |
| 400001 | 参数验证失败 | 400 |
| 400002 | 无效请求格式 | 400 |
| 401001 | 未授权访问 | 401 |
| 401002 | Token 已过期 | 401 |
| 403001 | 权限不足 | 403 |
| 404001 | 资源不存在 | 404 |
| 404002 | 会话不存在 | 404 |
| 404003 | Agent 不存在 | 404 |
| 409001 | 资源已存在 | 409 |
| 429001 | 请求过于频繁 | 429 |
| 500001 | 服务器内部错误 | 500 |
| 500002 | 数据库错误 | 500 |
| 503001 | 服务不可用 | 503 |

---

## 会话管理 API

### 创建会话

创建新的对话会话。

#### 请求

```http
POST /api/sessions
Content-Type: application/json

{
  "title": "我的对话",
  "agent_id": "code-assistant",
  "metadata": {
    "user_id": "user123",
    "project": "my-project"
  }
}
```

#### 字段说明

| 字段 | 类型 | 必需 | 描述 |
|-------|------|------|------|
| title | string | 是 | 会话标题 (最大 100 字符) |
| agent_id | string | 是 | Agent ID (仅支持字母、数字和连字符) |
| metadata | object | 否 | 自定义元数据 |

#### 响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "sess_abc123",
    "title": "我的对话",
    "agent_id": "code-assistant",
    "status": "active",
    "created_at": "2026-05-25T10:30:00Z",
    "updated_at": "2026-05-25T10:30:00Z"
  }
}
```

#### 示例 (curl)

```bash
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "title": "我的对话",
    "agent_id": "code-assistant"
  }'
```

---

### 获取会话列表

检索所有会话（支持分页和过滤）。

#### 请求

```http
GET /api/sessions?page=1&size=20&status=active
Authorization: Bearer <token>
```

#### 查询参数

| 参数 | 类型 | 默认值 | 描述 |
|------|------|---------|------|
| page | integer | 1 | 页码 |
| size | integer | 20 | 每页数量 (最大 100) |
| status | string | - | 过滤状态 (active, inactive, archived) |
| agent_id | string | - | 过滤 Agent ID |
| created_after | string | - | 创建时间之后 (ISO 8601) |
| created_before | string | - | 创建时间之前 (ISO 8601) |
| metadata.key | string | - | 按元数据键过滤 |

#### 响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "id": "sess_abc123",
        "title": "我的对话",
        "agent_id": "code-assistant",
        "status": "active",
        "created_at": "2026-05-25T10:30:00Z",
        "updated_at": "2026-05-25T10:30:00Z"
      }
    ],
    "total": 100,
    "page": 1,
    "size": 20
  }
}
```

---

### 获取会话详情

获取指定会话的详细信息，包括配置和历史消息计数。

#### 请求

```http
GET /api/sessions/{session_id}
```

#### 响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "sess_abc123",
    "title": "我的对话",
    "agent_id": "code-assistant",
    "status": "active",
    "metadata": {
      "user_id": "user123"
    },
    "message_count": 42,
    "total_tokens": 15000,
    "created_at": "2026-05-25T10:30:00Z",
    "updated_at": "2026-05-25T10:35:00Z"
  }
}
```

---

### 更新会话

更新会话配置或元数据（不支持更新消息）。

#### 请求

```http
PATCH /api/sessions/{session_id}
Content-Type: application/json

{
  "title": "更新的标题",
  "status": "archived",
  "metadata": {
    "user_id": "user123",
    "priority": "high"
  }
}
```

#### 字段说明

| 字段 | 类型 | 必需 | 描述 |
|-------|------|------|------|
| title | string | 否 | 新标题 |
| status | string | 否 | 新状态 (active, inactive, archived) |
| metadata | object | 否 | 新元数据 |

#### 响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "sess_abc123",
    "title": "更新的标题",
    "status": "archived",
    "metadata": {
      "user_id": "user123",
      "priority": "high"
    },
    "updated_at": "2026-05-25T10:40:00Z"
  }
}
```

---

### 删除会话

删除会话及其所有相关消息。

#### 请求

```http
DELETE /api/sessions/{session_id}
```

#### 响应

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

---

## 消息 API

### 发送消息

向指定会话发送消息并获取流式响应。

#### 请求

```http
POST /api/sessions/{session_id}/messages
Content-Type: application/json
Accept: text/event-stream

{
  "content": "请帮我写一个 Python 函数",
  "stream": true
}
```

#### 字段说明

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| content | string | 是 | 消息内容 (最大 10000 字符) |
| stream | boolean | 否 | 是否启用流式响应 (默认 true) |

#### 流式响应 (SSE)

```
event: message
data: {"type": "message", "content": "好的，", "tokens": 8}

event: message
data: {"type": "message", "content": "我来帮你", "tokens": 16}

event: tool_call
data: {"type": "tool_call", "tool": "code_generation", "status": "running"}

event: message
data: {"type": "message", "content": "```python\ndef hello():\n    print('Hello')\n```", "tokens": 150}

event: done
data: {"type": "done", "total_tokens": 174}
```

#### SSE 事件类型

| 事件类型 | 描述 |
|---------|------|
| message | 文本消息片段 |
| tool_call | 工具调用状态 |
| done | 响应完成 |
| error | 发生错误 |

#### 示例 (Python)

```python
import requests

url = "http://localhost:8080/api/sessions/sess_abc123/messages"
data = {"content": "请帮我写一个 Python 函数", "stream": True}

response = requests.post(url, json=data, stream=True)

for line in response.iter_lines():
    if line.startswith(b"data: "):
        event_data = json.loads(line[6:])
        
        if event_data["type"] == "message":
            print(event_data["content"], end="", flush=True)
        elif event_data["type"] == "tool_call":
            print(f"\n🔧 工具调用: {event_data['tool']}")
        elif event_data["type"] == "done":
            print(f"\n✅ 完成，总 token: {event_data['total_tokens']}")
```

---

### 获取消息历史

获取指定会话的所有历史消息。

#### 请求

```http
GET /api/sessions/{session_id}/messages?page=1&size=50
```

#### 响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "id": "msg_001",
        "role": "user",
        "content": "请帮我写一个 Python 函数",
        "created_at": "2026-05-25T10:30:00Z"
      },
      {
        "id": "msg_002",
        "role": "assistant",
        "content": "好的，我来帮你...",
        "tool_calls": [
          {
            "tool": "code_generation",
            "status": "completed",
            "result": "def hello():\n    print('Hello')\n"
          }
        ],
        "tokens": 174,
        "created_at": "2026-05-25T10:30:05Z"
      }
    ],
    "total": 42,
    "page": 1,
    "size": 50
  }
}
```

---

## Agent API

### 获取 Agent 列表

获取所有可用的 Agent。

#### 请求

```http
GET /api/agents
```

#### 响应

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "name": "code-assistant",
      "model": "gpt-4",
      "description": "代码助手，帮助编写和调试代码",
      "tools": ["code_execution", "file_operations"],
      "created_at": "2026-05-25T00:00:00Z"
    }
  ]
}
```

---

### 更新 Agent 配置

更新指定 Agent 的配置（不重启服务）。

#### 请求

```http
PUT /api/agents/{name}
Content-Type: application/json

{
  "model": "gpt-4-turbo",
  "description": "高级代码助手",
  "tools": ["code_execution", "file_operations", "web_search"]
}
```

#### 响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "name": "code-assistant",
    "model": "gpt-4-turbo",
    "description": "高级代码助手",
    "tools": ["code_execution", "file_operations", "web_search"]
  }
}
```

---

## 工具 API

### 获取工具列表

获取所有可用工具。

#### 请求

```http
GET /api/tools
```

#### 响应

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "name": "code_execution",
      "description": "执行多种语言的代码",
      "parameters": {
        "language": "string",
        "code": "string"
      },
      "output_type": "object"
    }
  ]
}
```

---

## 流式响应实现指南

### Server-Sent Events (SSE) 使用

CopCon 使用 SSE 协议实现流式响应，相比 WebSocket 更简单且兼容性更好。

#### 服务器端设置

```go
func handleSSE(w http.ResponseWriter, r *http.Request) {
    // 设置必要的响应头
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    // 创建事件通道
    events := make(chan Event)
    
    // 异步发送事件
    go func() {
        for i := 0; i < 100; i++ {
            events <- Event{Type: "message", Content: fmt.Sprintf("片段 %d", i)}
            time.Sleep(50 * time.Millisecond)
        }
        events <- Event{Type: "done"}
    }()
    
    // 将事件发送 SSE 协议格式化
    for event := range events {
        fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Content)
        w.(http.Flusher).Flush()
        
        if event.Type == "done" {
            break
        }
    }
}
```

#### 客户端使用

```javascript
const eventSource = new EventSource('/api/sessions/sess_abc123/messages');

eventSource.onmessage = (event) => {
    const data = JSON.parse(event.data);
    
    if (data.type === 'message') {
        console.log('收到消息:', data.content);
    } else if (data.type === 'done') {
        eventSource.close();
    }
};

eventSource.onerror = (error) => {
    console.error('SSE 错误:', error);
};
```

#### 错误处理

客户端应该实现自动重连机制：

```javascript
function connect() {
    const eventSource = new EventSource('/api/sessions/sess_abc123/messages');
    
    eventSource.onopen = () => {
        console.log('连接已建立');
    };
    
    eventSource.onerror = () => {
        console.log('连接断开，3 秒后重连...');
        eventSource.close();
        setTimeout(connect, 3000);
    };
}

connect();
```

---

## 认证和授权

### 启用认证

在配置文件中启用认证：

```yaml
authentication:
  enabled: true
  mode: jwt  # 或 "api_key"
```

### JWT Token 认证

#### 获取 Token

```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "secret"
  }'
```

响应：

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 86400
}
```

#### 使用 Token

```bash
curl -X GET http://localhost:8080/api/sessions \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

### API Key 认证

```yaml
authentication:
  enabled: true
  mode: api_key
  key: your-secret-api-key
```

#### 使用 API Key

```bash
curl -X GET http://localhost:8080/api/sessions \
  -H "X-API-Key: your-secret-api-key"
```

---

## 最佳实践

1. **连接超时**: 设置合理的 HTTP 超时时间（建议 30 秒）
2. **重试机制**: 对网络错误实现指数退避重试
3. **连接池**: 使用 HTTP 连接池提高性能
4. **请求体大小**: 单个请求体不超过 100KB
5. **并发限制**: 每个客户端同时不超过 10 个并发请求

## 常见问题

### 1. 为什么某些 API 返回 403?

- 检查是否启用了认证
- 检查 Token 是否正确且未过期
- 确认是否有相应权限

### 2. 流式响应无法接收?

- 确认客户端支持 SSE 协议
- 检查浏览器/HTTP 客户端的缓存设置
- 避免使用不支持流式的 CDN 或代理

### 3. 请求超时?

- 检查网络连接
- 增加客户端的超时时间
- 确认服务器负载情况

## 下一步

- 查看 [自定义 Handler](./customization.md) 扩展 API
- 学习 [部署最佳实践](../07-deployment/production-checklist.md)
