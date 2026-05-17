# Message API

## 概述

Message API 用于获取会话的消息历史。返回的消息采用 `steps/parts` 结构，与前端 `UIMessage` 格式一一对应，前端可直接渲染。

## 端点

```
GET /api/sessions/:sessionId/messages
```

## 查询参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `limit` | int | 50 | 返回的最大消息数 |

消息按 `created_at ASC` 排序（从旧到新），`limit` 控制返回条数上限。

注意：当前版本不支持 `offset` 分页。如需加载更多历史消息，可将所有消息缓存在前端，或在后续版本中通过游标分页实现。

## 请求示例

```bash
curl "http://localhost:8088/api/sessions/550e8400-e29b-41d4-a716-446655440000/messages?limit=20"
```

## 响应格式

### 成功响应 (200 OK)

```json
{
  "messages": [
    {
      "id": "aaa-bbb-ccc-001",
      "sessionId": "550e8400-e29b-41d4-a716-446655440000",
      "role": "user",
      "steps": [
        {
          "parts": [
            {
              "type": "text",
              "stepIndex": 0,
              "text": "用 Python 写一个快速排序",
              "state": "done"
            }
          ],
          "state": "done"
        }
      ],
      "metadata": {
        "createdAt": "2026-05-17T10:30:00Z",
        "model": "",
        "tokenCount": 0,
        "durationMs": 0
      }
    },
    {
      "id": "aaa-bbb-ccc-002",
      "sessionId": "550e8400-e29b-41d4-a716-446655440000",
      "role": "assistant",
      "steps": [
        {
          "parts": [
            {
              "type": "reasoning",
              "stepIndex": 0,
              "text": "用户需要一个快速排序的实现，我应该提供清晰的代码和解释。",
              "state": "done"
            },
            {
              "type": "text",
              "stepIndex": 0,
              "text": "以下是快速排序的 Python 实现：\n\n```python\ndef quicksort(arr):\n    if len(arr) <= 1:\n        return arr\n    pivot = arr[len(arr) // 2]\n    left = [x for x in arr if x < pivot]\n    middle = [x for x in arr if x == pivot]\n    right = [x for x in arr if x > pivot]\n    return quicksort(left) + middle + quicksort(right)\n```\n\n时间复杂度：平均 O(n log n)，最坏 O(n²)。",
              "state": "done"
            }
          ],
          "state": "done"
        },
        {
          "parts": [
            {
              "type": "tool-call",
              "stepIndex": 1,
              "toolCallId": "call_xyz789",
              "toolName": "code_executor",
              "args": "{\"language\":\"python\",\"code\":\"def quicksort(arr):\\n    if len(arr) <= 1:\\n        return arr\\n    pivot = arr[len(arr) // 2]\\n    left = [x for x in arr if x < pivot]\\n    middle = [x for x in arr if x == pivot]\\n    right = [x for x in arr if x > pivot]\\n    return quicksort(left) + middle + quicksort(right)\\n\\nprint(quicksort([3,6,8,10,1,2,1]))\"}",
              "output": "[1, 1, 2, 3, 6, 8, 10]\n",
              "state": "complete"
            }
          ],
          "state": "done"
        }
      ],
      "metadata": {
        "createdAt": "2026-05-17T10:30:05Z",
        "model": "gpt-4o",
        "tokenCount": 342,
        "durationMs": 2200
      }
    }
  ]
}
```

## UIMessage 结构说明

### 顶层字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string (UUID) | 消息唯一标识 |
| `sessionId` | string (UUID) | 所属会话 ID |
| `role` | string | `"user"` 或 `"assistant"` |
| `steps` | UIStep[] | 步骤数组，每个步骤包含一组 Parts |
| `metadata` | UIMetadata | 消息元数据 |

### UIMetadata

| 字段 | 类型 | 说明 |
|------|------|------|
| `createdAt` | string (ISO 8601) | 消息创建时间 |
| `model` | string | 使用的模型名称（assistant 消息） |
| `tokenCount` | int | Token 消耗量 |
| `durationMs` | int64 | 处理耗时（毫秒） |

### UIStep

| 字段 | 类型 | 说明 |
|------|------|------|
| `parts` | UIPart[] | 该步骤包含的内容片段 |
| `state` | string | 步骤状态（持久化数据固定为 `"done"`） |

### UIPart

Part 类型通过 `type` 字段区分：

| type 值 | 描述 | 特征字段 |
|---------|------|---------|
| `text` | 模型输出的文本内容 | `text` |
| `reasoning` | 推理 / 思考过程 | `text` |
| `tool-call` | 工具调用 | `toolCallId`, `toolName`, `args`, `output`, `error` |
| `step-start` | 步骤开始标记 | 无 |

Part 状态（`state`）：

| 值 | 含义 |
|---|------|
| `done` | 文本 / 推理内容已完成 |
| `streaming` | 正在流式输出（仅在 SSE 中） |
| `pending` | 工具调用等待中 |
| `running` | 工具正在执行（仅在 SSE 中） |
| `complete` | 工具调用成功完成 |
| `error` | 工具调用失败 |

持久化消息中所有 text 和 reasoning 类型的 Part 状态为 `done`，tool-call 类型的状态为 `complete` 或 `error`。

## tool 消息过滤

系统中存在 `role="tool"` 的内部消息（工具执行结果）。在 GET messages 响应中，tool 消息已被自动过滤。工具执行结果通过 tool-call part 的 `output` 字段嵌入到对应的 assistant 消息中，无需额外请求。

## 向后兼容

当 `parts` JSONB 列为空（旧数据）时，API 会自动从旧的 `content`、`reasoning`、`tool_calls` 字段反向构建 Parts 数据（`backfillParts`），确保旧数据在新结构下仍可正常展示。

转换规则：
- 旧 `content` → `type: "text"`  Part
- 旧 `reasoning` → `type: "reasoning"` Part
- 旧 `tool_calls` → `type: "tool-call"` Part（`state: "complete"` 若有结果）

## 错误响应

### 无效 sessionId (400)

```json
{
  "error": "invalid session id"
}
```

### 数据库查询错误 (500)

```json
{
  "error": "<数据库错误描述>"
}
```