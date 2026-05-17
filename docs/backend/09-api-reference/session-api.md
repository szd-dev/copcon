# Session API

## 概述

Session（会话）是 CopCon 中对话的容器，每个 Session 包含对话元数据、消息历史和执行上下文。Session API 提供完整的 CRUD 操作。

## 端点一览

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/sessions` | 创建新会话 |
| GET | `/api/sessions` | 获取会话列表 |
| GET | `/api/sessions/:sessionId` | 获取单个会话详情 |
| DELETE | `/api/sessions/:sessionId` | 删除会话及关联消息 |

---

## POST /api/sessions — 创建会话

创建一个新的会话，返回会话 ID 和元数据。

### 请求体

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `title` | string | 否 | `"New Chat"` | 会话标题 |
| `default_agent_id` | string | 否 | 配置中的 `default_agent_id` | 该会话默认使用的 Agent |

### 请求示例

```bash
curl -X POST http://localhost:8088/api/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Python 代码审查",
    "default_agent_id": "code-assistant"
  }'
```

请求体为空也是允许的（向后兼容）：

```bash
curl -X POST http://localhost:8088/api/sessions \
  -H "Content-Type: application/json" \
  -d '{}'
```

### 成功响应 (201 Created)

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "title": "Python 代码审查",
  "default_agent_id": "code-assistant",
  "created_at": "2026-05-17T10:30:00Z",
  "updated_at": "2026-05-17T10:30:00Z",
  "message_count": 0
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string (UUID) | 会话唯一标识 |
| `title` | string | 会话标题 |
| `default_agent_id` | string | 默认 Agent ID |
| `created_at` | string (ISO 8601) | 创建时间 |
| `updated_at` | string (ISO 8601) | 最后更新时间 |
| `message_count` | int | 消息总数 |

### 错误响应 (400 Bad Request)

```json
{
  "error": "invalid request body"
}
```

---

## GET /api/sessions — 获取会话列表

按更新时间倒序返回会话列表，支持分页。

### 查询参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `limit` | int | 20 | 每页返回数量 |
| `offset` | int | 0 | 偏移量 |

### 请求示例

```bash
curl "http://localhost:8088/api/sessions?limit=10&offset=0"
```

### 成功响应 (200 OK)

```json
{
  "sessions": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "title": "Python 代码审查",
      "created_at": "2026-05-17T10:30:00Z",
      "updated_at": "2026-05-17T10:35:00Z",
      "message_count": 12
    },
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "title": "Shell 脚本调试",
      "created_at": "2026-05-17T09:00:00Z",
      "updated_at": "2026-05-17T09:05:00Z",
      "message_count": 4
    }
  ],
  "total": 42
}
```

| 字段 | 说明 |
|------|------|
| `sessions` | 会话数组，按 `updated_at` 倒序 |
| `total` | 符合条件的会话总数 |

---

## GET /api/sessions/:sessionId — 获取会话详情

获取单个会话的完整信息。

### 路径参数

| 参数 | 说明 |
|------|------|
| `sessionId` | 会话 UUID |

### 请求示例

```bash
curl http://localhost:8088/api/sessions/550e8400-e29b-41d4-a716-446655440000
```

### 成功响应 (200 OK)

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "title": "Python 代码审查",
  "created_at": "2026-05-17T10:30:00Z",
  "updated_at": "2026-05-17T10:35:00Z",
  "message_count": 12
}
```

### 错误响应 (404 Not Found)

```json
{
  "error": "session not found"
}
```

---

## DELETE /api/sessions/:sessionId — 删除会话

删除指定会话及其所有关联的消息和 Todo 记录。删除采用数据库级联（`ON DELETE CASCADE`），消息和 Todo 自动清除。

删除前会取消该 Session 下所有正在执行的异步工具。

### 路径参数

| 参数 | 说明 |
|------|------|
| `sessionId` | 会话 UUID |

### 请求示例

```bash
curl -X DELETE http://localhost:8088/api/sessions/550e8400-e29b-41d4-a716-446655440000
```

### 成功响应 (204 No Content)

无响应体。

### 错误响应 (404 Not Found)

```json
{
  "error": "session not found"
}
```

---

## 完整交互示例

```bash
# 1. 创建会话
curl -X POST http://localhost:8088/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"title": "测试会话", "default_agent_id": "code-assistant"}'
# → 返回 session ID: "xxx-yyy-zzz"

# 2. 查看会话列表
curl "http://localhost:8088/api/sessions?limit=5"

# 3. 查看会话详情
curl http://localhost:8088/api/sessions/xxx-yyy-zzz

# 4. 删除会话
curl -X DELETE http://localhost:8088/api/sessions/xxx-yyy-zzz
```

## 注意事项

- Session ID 使用 UUID v4 格式
- 删除操作不可逆，关联的消息和历史永久丢失
- `default_agent_id` 在创建时指定后不会自动变更，如需修改请重新创建 Session
- 不提供 PATCH 更新接口，标题更新可通过 `UpdateTitle` Manager 方法实现（暂未暴露为 HTTP 端点）