# Chat API

## 概述

Chat API 是 CopCon 的核心接口，用于向 Agent 发送消息并通过 SSE（Server-Sent Events）流式接收响应。后端会在 Agent 循环中依次处理 LLM 流式输出、工具调用执行，并将每个细粒度事件实时推送给前端。

## 端点

```
POST /api/sessions/:sessionId/chat
```

## 请求格式

**Header:**

```
Content-Type: application/json
```

**Body:**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `content` | string | 是 | 用户输入的消息内容 |
| `agent_id` | string | 否 | 指定处理此请求的 Agent ID。留空则使用 Session 创建时指定的 default_agent_id |

### 请求示例

```json
{
  "content": "写一个快速排序的 Python 实现，并解释时间复杂度",
  "agent_id": "code-assistant"
}
```

## SSE 响应格式

CopCon 返回标准 SSE（`text/event-stream`）格式。每个事件是一行 `data: <JSON>\n\n`，前端直接按行解析即可。

### 响应头

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

### 事件分类

| 事件类型 | 说明 | 触发时机 |
|---------|------|---------|
| `step_create` | 创建新的执行步骤 | Agent 循环进入新一轮迭代（工具调用后） |
| `part_create` | 创建一个 UI 内容片段 | 开始输出文本、推理内容或工具调用 |
| `part_update` | 更新已有片段的内容或状态 | 流式增量、工具完成或状态变更 |
| `message_done` | 消息处理完毕，流结束 | 当前消息全部处理完成 |
| `error` | 处理过程中发生错误 | 异常中断 |

### step_create

```json
{
  "type": "step_create",
  "data": {
    "messageId": "550e8400-e29b-41d4-a716-446655440000",
    "stepIndex": 1
  }
}
```

每个 chat 请求的第一个 step（stepIndex=0）不发送 `step_create` 事件，从 stepIndex=1 开始发送。

### part_create

```json
{
  "type": "part_create",
  "data": {
    "messageId": "550e8400-e29b-41d4-a716-446655440000",
    "stepIndex": 1,
    "partIndex": 0,
    "partType": "tool-call",
    "state": "pending",
    "toolCallId": "call_abc123",
    "toolName": "code_executor",
    "args": "{\"language\":\"python\",\"code\":\"print('hello')\"}"
  }
}
```

`partType` 可选值：
- `text` — 文本输出
- `reasoning` — 推理 / 思考过程
- `tool-call` — 工具调用

### part_update

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

Part 状态流转：

```
text:       streaming → done
reasoning:  streaming → done
tool-call:  pending → running → complete (或 error)
```

工具调用完成后，`part_update` 携带最终状态和输出：

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

### message_done

```json
{
  "type": "message_done",
  "data": {
    "messageId": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

收到此事件后前端应标记该消息为已完成。

### error

```json
{
  "type": "error",
  "data": {
    "error": "session not found"
  }
}
```

## 完整流式示例

以下是 `curl` 调用和完整的 SSE 输出示例。

### curl 命令

```bash
curl -N -X POST http://localhost:8088/api/sessions/550e8400-e29b-41d4-a716-446655440000/chat \
  -H "Content-Type: application/json" \
  -d '{
    "content": "用 Python 写一个 hello world",
    "agent_id": "code-assistant"
  }'
```

### SSE 输出

```
data: {"type":"part_create","data":{"messageId":"aaa-bbb-ccc","stepIndex":0,"partIndex":0,"partType":"text","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"aaa-bbb-ccc","stepIndex":0,"partIndex":0,"partType":"text","textDelta":"以下是一个简单的 Python Hello World 程序：","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"aaa-bbb-ccc","stepIndex":0,"partIndex":0,"partType":"text","textDelta":"\n\n```python\nprint(\"Hello,","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"aaa-bbb-ccc","stepIndex":0,"partIndex":0,"partType":"text","textDelta":" World!\")\n```","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"aaa-bbb-ccc","stepIndex":0,"partIndex":0,"partType":"text","state":"done"}}

data: {"type":"message_done","data":{"messageId":"aaa-bbb-ccc"}}
```

## 错误响应

### 请求体格式错误 (400)

```json
{
  "error": "invalid request"
}
```

### 流式不支持 (500)

当运行环境不支持 HTTP Flusher 时返回：

```json
{
  "error": "streaming not supported"
}
```

### Agent 执行错误

Agent 引擎在处理过程中遇到的错误会通过 SSE `error` 事件推送，不会中断 SSE 流。

常见错误场景：
- Session 不存在
- Agent 未找到
- LLM API 调用失败
- 工具执行异常

## 超时与连接处理

### 连接生命周期

1. 客户端建立 HTTP POST 连接
2. 服务端校验请求后立即启动 Agent 循环（goroutine 异步执行）
3. 主线程阻塞在事件通道上，逐个写出 SSE 事件
4. 当 Agent 循环结束（`chatCtx.Close()`）或通道关闭时，连接自动断开

### 超时策略

| 超时类型 | 默认值 | 说明 |
|---------|--------|------|
| HTTP 连接超时 | 无限制 | 流式连接不设超时，直到流结束 |
| Agent 执行 | 由 LLM Provider 决定 | 底层由 OpenAI SDK 的超时配置控制 |
| 工具执行 | 同步模式无限制，异步模式由 `execution_mode` 控制 | 异步工具可通过 `cancel_tool` 取消 |

### 断开与重连

如果 SSE 连接意外断开（网络问题、代理超时等）：

- 后端 Agent 循环继续执行（goroutine 不受影响），结果会被持久化
- 前端应通过 `GET /api/sessions/:sessionId/updates?since=<last_event_id>` 轮询待处理事件
- 也可以重新调用 `POST /api/sessions/:sessionId/chat`（传入空的 `content`），后端会重新推送 SSE

### 连接数限制

单个 Session 同时只能有一个活跃的 Agent 循环。如果在同一个 Session 上并发发送多个 chat 请求，会产生竞态。前端应确保同一 Session 上一次请求结束后再发起下一次。