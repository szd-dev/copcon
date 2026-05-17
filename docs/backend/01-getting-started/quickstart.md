# 快速开始

5 分钟把 CopCon 后端跑起来。

## 前提条件

- Go 1.26 或更高版本
- PostgreSQL 15 运行中（可以用 Docker Compose 启动）
- 一个可用的 OpenAI 兼容 API Key

## 第一步：克隆并启动依赖

```bash
git clone <your-repo-url>
cd copcon

# 启动 PostgreSQL 和 Qdrant（可选）
docker compose up -d postgres qdrant
```

如果不想装 Docker，也可以自己装个 PostgreSQL 并建好库。

## 第二步：配置 API Key

创建环境变量或在 `server/config.yaml` 中配置：

```bash
export OPENAI_API_KEY=sk-your-key-here
```

`config.yaml` 里也可以写，但环境变量会覆盖文件中的值：

```yaml
# server/config.yaml
openai:
  api_key: ""           # 留空就用环境变量
  base_url: ""          # 留空就用 OpenAI 官方地址
  model: "gpt-4o"       # 也可以用其他模型
```

## 第三步：启动服务

```bash
cd server
go mod tidy
go run ./cmd/server
```

看到以下输出就说明启动成功了：

```
2025/01/01 12:00:00 INFO Registered tools in registry count=8
2025/01/01 12:00:00 INFO Loaded agents count=2
2025/01/01 12:00:00 INFO Server starting port=8088
```

## 第四步：发送第一个请求

先建一个会话：

```bash
curl -s -X POST http://localhost:8088/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello World", "default_agent_id": "code-assistant"}' | jq
```

返回：

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "title": "Hello World",
  "default_agent_id": "code-assistant",
  "created_at": "2025-01-01T12:00:00Z",
  "updated_at": "2025-01-01T12:00:00Z",
  "message_count": 0
}
```

记下这个 `id`，然后用它发消息：

```bash
curl -N -X POST http://localhost:8088/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890/chat \
  -H "Content-Type: application/json" \
  -d '{"content": "写一段 Go 代码打印 Hello World"}'
```

## 预期的 SSE 响应

你会看到一段 SSE（Server-Sent Events）流：

```
data: {"type":"step_create","data":{"messageId":"msg-xxx","stepIndex":0}}

data: {"type":"part_create","data":{"messageId":"msg-xxx","stepIndex":0,"partIndex":0,"partType":"text","state":"streaming"}}

data: {"type":"part_update","data":{"messageId":"msg-xxx","stepIndex":0,"partIndex":0,"partType":"text","textDelta":"好的"}}

data: {"type":"part_update","data":{"messageId":"msg-xxx","stepIndex":0,"partIndex":0,"partType":"text","textDelta":"，给"}}

data: {"type":"part_update","data":{"messageId":"msg-xxx","stepIndex":0,"partIndex":0,"partType":"text","textDelta":"你写"}}

...

data: {"type":"part_update","data":{"messageId":"msg-xxx","stepIndex":0,"partIndex":0,"partType":"text","state":"done"}}

data: {"type":"message_done","data":{"messageId":"msg-xxx"}}
```

如果 Agent 调了工具（比如执行代码），你还会看到 `tool-call` 类型的 part 和 `step_create` 事件。

## 事件类型说明

| 事件 | 含义 |
|---|---|
| `step_create` | 开始新一轮 Agent 循环（工具调用后会再开新 step） |
| `part_create` | 创建新的内容片段（text / reasoning / tool-call） |
| `part_update` | 更新某个片段的内容 | 状态 |
| `message_done` | 整个消息处理完成 |

## 下一步

- 了解详细的安装配置：[安装指南](installation.md)
- 从头写一个 Agent 程序：[Hello World](hello-world.md)