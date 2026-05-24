# Hello World - 第一个 Agent 应用

本教程将带你创建一个最简单的 Agent 应用,让它能够回答用户的问题。

## 目标

在 5 分钟内创建一个:
- ✅ 能接收用户消息的 Agent
- ✅ 调用 LLM 生成回复
- ✅ 通过 SSE 流式返回结果

## 前置要求

- 已完成[安装与环境配置](installation.md)
- 后端服务正在运行

## 创建会话

首先创建一个会话:

```bash
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "title": "我的第一个会话",
    "agent_id": "code-assistant"
  }'
```

**响应示例:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "title": "我的第一个会话",
  "agent_id": "code-assistant",
  "created_at": "2026-05-25T10:30:00Z"
}
```

记住返回的 `id`,下一步会用到。

## 发送消息

使用上一步获得的会话 ID,发送一条消息:

```bash
# 将会话 ID 替换为你实际获得的 ID
SESSION_ID=550e8400-e29b-41d4-a716-446655440000

curl -N -X POST http://localhost:8080/api/sessions/${SESSION_ID}/chat \
  -H "Content-Type: application/json" \
  -d '{
    "content": "你好,请介绍一下你自己"
  }'
```

**预期输出 (SSE 流):**

```
event: message
data: {"type":"message","content":"你"}

event: message
data: {"type":"message","content":"好"}

event: message
data: {"type":"message","content":"！"}

event: message
data: {"type":"message","content":"我是"}

... (持续流式输出)

event: done
data: {"type":"done","message_id":"msg-123"}
```

## 查看消息历史

获取会话中的所有消息:

```bash
curl http://localhost:8080/api/sessions/${SESSION_ID}/messages
```

**响应示例:**

```json
{
  "messages": [
    {
      "id": "msg-123",
      "role": "user",
      "content": "你好,请介绍一下你自己"
    },
    {
      "id": "msg-124",
      "role": "assistant",
      "content": "你好！我是由 CopCon 驱动的智能助手,可以帮助你编写代码、解答问题、执行命令等。我可以..."
    }
  ]
}
```

## 使用工具

让我们测试代码执行能力:

```bash
curl -N -X POST http://localhost:8080/api/sessions/${SESSION_ID}/chat \
  -H "Content-Type: application/json" \
  -d '{
    "content": "请计算 1+1 的结果"
  }'
```

**预期输出:**

```
event: tool_call
data: {"type":"tool_call","tool":"code_executor","input":{"language":"python","code":"print(1+1)"}}

event: tool_result
data: {"type":"tool_result","tool":"code_executor","output":"2\n"}

event: message
data: {"type":"message","content":"1"}

event: message
data: {"type":"message","content":"+"}

event: message
data: {"type":"message","content":"1"}

event: message
data: {"type":"message","content":" 的结果是"}

event: message
data: {"type":"message","content":" 2"}

event: done
data: {"type":"done","message_id":"msg-125"}
```

## 使用 Python SDK

如果你更喜欢 Python,可以使用官方 SDK:

```bash
pip install copcon-sdk
```

```python
from copcon import CopConClient

client = CopConClient(base_url="http://localhost:8080")

# 创建会话
session = client.sessions.create(
    title="Python 示例",
    agent_id="code-assistant"
)

# 发送消息并流式接收
for event in client.chat(
    session_id=session.id,
    content="你好,请用 Python 写一个 Hello World"
):
    if event.type == "message":
        print(event.content, end="", flush=True)
    elif event.type == "tool_call":
        print(f"\n[调用工具: {event.tool}]")
    elif event.type == "done":
        print("\n[完成]")
```

## 使用 JavaScript/TypeScript

```typescript
import { CopConClient } from 'copcon-sdk';

const client = new CopConClient({ baseURL: 'http://localhost:8080' });

// 创建会话
const session = await client.sessions.create({
  title: 'TypeScript 示例',
  agentId: 'code-assistant'
});

// 发送消息并流式接收
const stream = await client.chat({
  sessionId: session.id,
  content: '你好,请用 TypeScript 写一个类型定义'
});

for await (const event of stream) {
  if (event.type === 'message') {
    process.stdout.write(event.content);
  } else if (event.type === 'tool_call') {
    console.log(`\n[调用工具: ${event.tool}]`);
  } else if (event.type === 'done') {
    console.log('\n[完成]');
  }
}
```

## 使用前端组件

CopCon 提供了 React 组件库,让你可以快速搭建聊天界面:

```jsx
import { ChatView, SessionList } from '@copcon/ui';

function App() {
  return (
    <div style={{ display: 'flex', height: '100vh' }}>
      <div style={{ width: '300px', borderRight: '1px solid #ccc' }}>
        <SessionList />
      </div>
      <div style={{ flex: 1 }}>
        <ChatView />
      </div>
    </div>
  );
}
```

## 常见问题

### Q: Agent 没有响应?

1. 检查后端服务是否在运行
2. 查看日志: `docker-compose logs -f server`
3. 确认 OpenAI API Key 有效

### Q: 工具调用失败?

1. 确认 Docker 容器正常运行
2. 代码执行需要 `code_executor` 容器可用
3. 查看工具相关日志

### Q: 如何更换 Agent?

在创建会话时指定 `agent_id`:

```bash
curl -X POST http://localhost:8080/api/sessions \
  -d '{"title": "新会话", "agent_id": "chat-assistant"}'
```

查看可用 Agents:

```bash
curl http://localhost:8080/api/agents
```

## 下一步

- [运行完整 Demo](run-demo.md) - 体验完整的前后端集成
- [架构概览](../02-core-concepts/architecture.md) - 了解 CopCon 核心架构
- [Harness 配置](../02-core-concepts/harness.md) - 学习如何配置 Agent
