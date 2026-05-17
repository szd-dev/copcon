# SSE 事件协议规范

## 概述

CopCon 使用 SSE（Server-Sent Events）协议进行实时通信。Agent 引擎在运行过程中按 `step → part` 两级结构逐步产出事件，前端根据事件类型增量构建 UI。

## 协议基础

### 传输格式

```
data: <JSON 字符串>\n\n
```

- 每条消息以 `data: ` 开头
- 消息体为单行 JSON
- 以两个换行符 `\n\n` 结束
- 不显式设置 `event:` 字段，事件类型包含在 JSON 的 `type` 字段中

### 通用结构

```json
{
  "type": "<事件类型>",
  "data": { ... }
}
```

## 事件类型详解

### step_create — 创建新 Step

Agent 循环进入新一轮迭代时发送（第一次迭代不发送）。

**结构：**

```json
{
  "type": "step_create",
  "data": {
    "messageId": "550e8400-e29b-41d4-a716-446655440000",
    "stepIndex": 1
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `messageId` | string (UUID) | 所属消息 ID |
| `stepIndex` | int | 步骤序号，从 1 开始 |

---

### part_create — 创建新 Part

在当前 Step 中创建一个新的 UI 内容片段。

**结构：**

```json
{
  "type": "part_create",
  "data": {
    "messageId": "550e8400-e29b-41d4-a716-446655440000",
    "stepIndex": 0,
    "partIndex": 1,
    "partType": "tool-call",
    "state": "pending",
    "toolCallId": "call_DgGbaJHGHW5Ckz4h",
    "toolName": "code_executor",
    "args": "{\"language\":\"python\",\"code\":\"print('hello')\"}"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `messageId` | string (UUID) | 所属消息 ID |
| `stepIndex` | int | 所属步骤序号 |
| `partIndex` | int | 在当前消息中的唯一序号，从 0 开始递增 |
| `partType` | string | `"text"`, `"reasoning"`, `"tool-call"` |
| `state` | string | 初始状态：text/reasoning 为 `"streaming"`，tool-call 为 `"pending"` |
| `toolCallId` | string | tool-call 类型专用，工具调用唯一标识 |
| `toolName` | string | tool-call 类型专用，工具名称 |
| `args` | string (JSON) | tool-call 类型专用，工具参数 |

---

### part_update — 更新 Part 内容

更新已有 Part 的增量内容或状态。

**text/reasoning 类型（流式增量）：**

```json
{
  "type": "part_update",
  "data": {
    "messageId": "550e8400-e29b-41d4-a716-446655440000",
    "stepIndex": 0,
    "partIndex": 0,
    "partType": "text",
    "textDelta": "快速排序",
    "state": "streaming"
  }
}
```

**text/reasoning 类型（流结束）：**

```json
{
  "type": "part_update",
  "data": {
    "messageId": "550e8400-e29b-41d4-a716-446655440000",
    "stepIndex": 0,
    "partIndex": 0,
    "partType": "text",
    "state": "done"
  }
}
```

**tool-call 类型（执行结果）：**

```json
{
  "type": "part_update",
  "data": {
    "messageId": "550e8400-e29b-41d4-a716-446655440000",
    "stepIndex": 0,
    "partIndex": 1,
    "partType": "tool-call",
    "state": "complete",
    "output": "hello\n"
  }
}
```

tool-call 类型的状态变化：

| state | 说明 |
|-------|------|
| `pending` | 工具调用已注册，等待执行 |
| `running` | 工具正在执行（异步模式） |
| `complete` | 执行成功，`output` 字段有值 |
| `error` | 执行失败，`error` 字段有值 |

**完整字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `messageId` | string (UUID) | 所属消息 ID |
| `stepIndex` | int | 所属步骤序号 |
| `partIndex` | int | Part 序号 |
| `partType` | string | Part 类型 |
| `textDelta` | string | 文本增量（text/reasoning 类型） |
| `state` | string | 当前状态 |
| `output` | string | 工具执行输出（tool-call 类型） |
| `error` | string | 错误信息（tool-call 类型） |

---

### message_done — 消息处理完毕

```json
{
  "type": "message_done",
  "data": {
    "messageId": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `messageId` | string (UUID) | 完成的消息 ID |

收到此事件后，前端应：
1. 将消息标记为"已完成"
2. 停止流式动画
3. 如果 ToolCall Part 还有 `pending` 状态的，需通过轮询获取最终结果

---

### error — 错误事件

```json
{
  "type": "error",
  "data": {
    "error": "agent not found: custom-agent"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `error` | string | 错误描述 |

---

### 异步工具事件

当工具以 `async` 模式执行时，会额外产生以下事件：

**async_tool_started：**

```json
{
  "type": "async_tool_started",
  "data": {
    "message_id": "550e8400-e29b-41d4-a716-446655440000",
    "call_id": "call_abc123",
    "tool_name": "code_executor",
    "session_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

**async_tool_complete：**

```json
{
  "type": "async_tool_complete",
  "data": {
    "message_id": "550e8400-e29b-41d4-a716-446655440000",
    "call_id": "call_abc123",
    "tool_name": "code_executor",
    "result": "hello\n",
    "duration_ms": 1523
  }
}
```

**async_tool_failed：**

```json
{
  "type": "async_tool_failed",
  "data": {
    "message_id": "550e8400-e29b-41d4-a716-446655440000",
    "call_id": "call_abc123",
    "tool_name": "code_executor",
    "error": "execution timed out",
    "duration_ms": 30000
  }
}
```

## 完整流式示例

```
data: {"type":"part_create","data":{"messageId":"m1","stepIndex":0,"partIndex":0,"partType":"reasoning","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"m1","stepIndex":0,"partIndex":0,"partType":"reasoning","textDelta":"用户想","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"m1","stepIndex":0,"partIndex":0,"partType":"reasoning","textDelta":"要一个排序函数","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"m1","stepIndex":0,"partIndex":0,"partType":"reasoning","state":"done"}}

data: {"type":"part_create","data":{"messageId":"m1","stepIndex":0,"partIndex":1,"partType":"text","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"m1","stepIndex":0,"partIndex":1,"partType":"text","textDelta":"以下是快速排序的实现","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"m1","stepIndex":0,"partIndex":1,"partType":"text","textDelta":"：\n\n```python\ndef qs...","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"m1","stepIndex":0,"partIndex":1,"partType":"text","state":"done"}}

data: {"type":"step_create","data":{"messageId":"m1","stepIndex":1}}

data: {"type":"part_create","data":{"messageId":"m1","stepIndex":1,"partIndex":2,"partType":"tool-call","state":"pending","toolCallId":"call_x","toolName":"code_executor","args":"{\"language\":\"python\"}"}}

data: {"type":"part_update","data":{"messageId":"m1","stepIndex":1,"partIndex":2,"partType":"tool-call","state":"complete","output":"[1,1,2,3,6,8,10]\n"}}

data: {"type":"message_done","data":{"messageId":"m1"}}
```

## 前端重连策略

### SSE 连接断开时

CopCon 后端在 SSE 连接断开后，Agent 循环仍然继续执行，结果会被持久化到数据库。前端需要：

**1. 通过 Session Updates 端点获取待处理事件**

```
GET /api/sessions/:sessionId/updates?since=<last_event_id>
```

响应：

```json
{
  "has_updates": true,
  "events": [
    {
      "id": "evt_001",
      "call_id": "call_xyz",
      "tool_name": "code_executor",
      "session_id": "550e8400-e29b-41d4-a716-446655440000",
      "completed_at": "2026-05-17T10:31:00Z"
    }
  ]
}
```

**2. 重新发起空 Chat 请求以重新推送 SSE**

```bash
curl -N -X POST http://localhost:8088/api/sessions/:sessionId/chat \
  -H "Content-Type: application/json" \
  -d '{"content": ""}'
```

传入空的 `content` 时，后端跳过用户消息持久化，直接从当前上下文继续推送事件。

### 推荐重连实现

```typescript
class SSEClient {
  private lastEventId: string = "";

  async connect(sessionId: string) {
    // 1. 先拉取遗漏事件
    const updates = await fetch(
      `/api/sessions/${sessionId}/updates?since=${this.lastEventId}`
    ).then(r => r.json());

    if (updates.has_updates) {
      // 前端自行处理遗漏事件
      updates.events.forEach(this.handleEvent);
    }

    // 2. 建立新 SSE 连接
    const eventSource = new EventSource(
      `/api/sse/sessions/${sessionId}` // 如果将来提供 EventSource 端点
    );
  }
}
```

### 幂等性

前端应基于 `messageId` + `partIndex` 确保同一个 Part 不会被重复创建。收到 `part_create` 时先检查是否已存在，已存在则忽略。

## 废弃事件类型

以下事件类型已废弃，仅保留用于向后兼容：

| 废弃类型 | 替代方案 |
|---------|---------|
| `message` | `part_create` + `part_update` (partType: "text") |
| `reasoning` | `part_create` + `part_update` (partType: "reasoning") |
| `tool_call` | `part_create` (partType: "tool-call") |
| `tool_result` | `part_update` (state: "complete", output: "...") |
| `thought` | 不再使用 |
| `done` | `message_done` |

新开发的前端应只依赖 `step_create`、`part_create`、`part_update`、`message_done` 四种事件类型。