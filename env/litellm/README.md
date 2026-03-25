# LiteLLM 开发环境部署

统一的 LLM API 网关，将各种 LLM 提供商转换为 OpenAI 兼容格式。

## 快速开始

### 1. 配置环境变量

```bash
# 复制环境变量模板
cp .env.example .env

# 编辑 .env 文件，填入你的 API Keys
vim .env
```

### 2. 启动服务

```bash
# 启动 LiteLLM
docker compose up -d

# 查看日志
docker compose logs -f
```

### 3. 访问服务

| 服务 | 地址 | 说明 |
|-----|------|------|
| API 端点 | http://localhost:4000/v1 | OpenAI 兼容 API |
| Admin UI | http://localhost:4000/ui | 管理界面 |
| Swagger | http://localhost:4000/docs | API 文档 |
| Health | http://localhost:4000/health | 健康检查 |

## 使用示例

### Python (OpenAI SDK)

```python
from openai import OpenAI

client = OpenAI(
    api_key="sk-litellm-dev-key",  # LITELLM_MASTER_KEY
    base_url="http://localhost:4000/v1"
)

# 使用 Kimi K2
response = client.chat.completions.create(
    model="kimi-k2",
    messages=[{"role": "user", "content": "你好！"}]
)

# 切换到 GPT-4o，只需改模型名
response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "你好！"}]
)
```

### Go (go-openai)

```go
import openai "github.com/sashabaranov/go-openai"

config := openai.DefaultConfig("sk-litellm-dev-key")
config.BaseURL = "http://localhost:4000/v1"
client := openai.NewClientWithConfig(config)

// 使用任意模型
resp, _ := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
    Model:    "kimi-k2.5",
    Messages: []openai.ChatCompletionMessage{
        {Role: "user", Content: "你好！"},
    },
})
```

### cURL

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-litellm-dev-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "kimi-k2",
    "messages": [{"role": "user", "content": "你好！"}]
  }'
```

## 支持的模型

在 `litellm-config.yaml` 中配置的模型：

| 模型名 | 后端 | 说明 |
|-------|------|------|
| `kimi-k2` | Moonshot AI | Kimi K2 模型 |
| `kimi-k2.5` | Moonshot AI | Kimi K2.5 模型 |
| `kimi-k2-turbo` | Moonshot AI | Kimi K2 Turbo |
| `gpt-4o` | OpenAI | GPT-4o |
| `gpt-4o-mini` | OpenAI | GPT-4o Mini |
| `claude-sonnet` | Anthropic | Claude Sonnet 4 |
| `claude-haiku` | Anthropic | Claude 3.5 Haiku |
| `gemini-pro` | Google | Gemini 2.5 Pro |
| `gemini-flash` | Google | Gemini 2.5 Flash |
| `deepseek-chat` | DeepSeek | DeepSeek Chat |
| `deepseek-reasoner` | DeepSeek | DeepSeek R1 |

## 工具调用 (Tool Calling)

LiteLLM 自动处理不同提供商的工具调用格式差异：

```python
tools = [{
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "获取天气信息",
        "parameters": {
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "城市名"}
            },
            "required": ["city"]
        }
    }
}]

# Kimi 和 OpenAI 使用相同的代码
response = client.chat.completions.create(
    model="kimi-k2",  # 或 gpt-4o
    messages=[{"role": "user", "content": "北京今天天气怎么样？"}],
    tools=tools
)
```

## 配置说明

- `litellm-config.yaml` - 模型和路由配置
- `.env` - API Keys 和环境变量

添加新模型只需在 `litellm-config.yaml` 中添加配置项。

## 故障排查

```bash
# 查看日志
docker compose logs -f litellm

# 检查健康状态
curl http://localhost:4000/health

# 列出可用模型
curl http://localhost:4000/v1/models \
  -H "Authorization: Bearer sk-litellm-dev-key"
```