# LLMProvider 抽象层

## 概述

`LLMProvider` 定义在 `internal/llm/provider.go`，是 CopCon 后端与 LLM 服务之间的抽象接口。它将 Agent 引擎与具体的 LLM SDK（OpenAI、Anthropic、本地模型等）解耦，使引擎可以在不修改代码的情况下切换到不同的 LLM 后端。

## 接口定义

```go
// LLMProvider 是任何 LLM 后端的接口。
// 实现通过数据 channel 流式返回响应块，通过独立的 error channel 报告错误。
type LLMProvider interface {
    Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error)
}
```

只有一个方法 `Stream`，返回两个 channel：

| Channel | 类型 | 说明 |
|---------|------|------|
| `ch` | `<-chan StreamChunk` | 流式响应块，由 Provider 负责关闭 |
| `errc` | `<-chan error` | 最多接收一个错误，由 Provider 负责关闭 |

调用方使用模式：

```go
ch, errc := provider.Stream(ctx, params)

// 消费所有 chunk
for chunk := range ch {
    // 处理 chunk.Content, chunk.ToolCalls 等
}

// ch 关闭后，检查是否有错误
select {
case err := <-errc:
    // 流式过程中发生的错误
default:
    // 无错误
}
```

> **注意**：调用方**必须**消费 `ch` 直到关闭，否则 Provider 内部的 goroutine 会泄漏。

## StreamParams

```go
type StreamParams struct {
    Model       string    `json:"model"`        // 模型标识符，如 "gpt-4o"
    Messages    []Message `json:"messages"`      // 对话历史（含 system prompt）
    Tools       []ToolDef `json:"tools,omitempty"` // 可用工具定义（nil/空表示无工具）
    Temperature float64   `json:"temperature,omitempty"` // 随机性控制（0.0–2.0，0=用默认值）
    MaxTokens   int       `json:"max_tokens,omitempty"`  // 最大 token 数（0=不限制）
}
```

### Message

```go
type Message struct {
    Role       StreamRole `json:"role"`              // system / user / assistant / tool
    Content    string     `json:"content"`            // 消息正文
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"` // assistant 消息的工具调用
    ToolCallID string     `json:"tool_call_id,omitempty"` // tool 消息关联的工具 ID
    Name       string     `json:"name,omitempty"`     // 可选的参与者名称
}
```

角色常量：

```go
const (
    RoleSystem    StreamRole = "system"
    RoleUser      StreamRole = "user"
    RoleAssistant StreamRole = "assistant"
    RoleTool      StreamRole = "tool"
)
```

### ToolDef

```go
type ToolDef struct {
    Name        string          `json:"name"`        // 函数名称
    Description string          `json:"description"` // 工具描述
    Parameters  json.RawMessage `json:"parameters"`  // JSON Schema 参数定义
}
```

`Parameters` 使用 `json.RawMessage` 而非具体结构体，避免与特定 Provider SDK 的类型耦合。

## StreamChunk

```go
type StreamChunk struct {
    Content          string          `json:"content,omitempty"`          // 文本增量
    ReasoningContent string          `json:"reasoning_content,omitempty"` // 推理/思考增量
    ToolCalls        []ToolCallDelta `json:"tool_calls,omitempty"`       // 工具调用增量
    Usage            *Usage          `json:"usage,omitempty"`            // token 用量（通常最后一块才有）
    FinishReason     string          `json:"finish_reason,omitempty"`    // 结束原因: "stop"/"length"/"tool_calls"
}
```

- `Content` 和 `ReasoningContent` 是增量文本，调用方自行拼接
- `ToolCalls` 是增量的，需按 `Index` 合并（详见 `ToolCallDelta`）
- `Usage` 通常在最终 chunk 中才有值
- `FinishReason` 在最终 chunk 中定义结束原因

### ToolCallDelta

```go
type ToolCallDelta struct {
    Index     int    `json:"index"`              // 工具调用序号（0-based）
    ID        string `json:"id,omitempty"`       // 唯一标识（可能在后续 chunk 才出现）
    Name      string `json:"name,omitempty"`     // 函数名
    Arguments string `json:"arguments,omitempty"` // 参数 JSON 片段
}
```

引擎中的增量合并逻辑（`engine.go`）：

```go
toolCallMap := make(map[int]*toolCallInfo)

for chunk := range ch {
    for _, tc := range chunk.ToolCalls {
        idx := tc.Index
        if existing, ok := toolCallMap[idx]; ok {
            // 合并到已有的 tool call
            if tc.Name != "" {
                existing.Name = tc.Name
            }
            if tc.Arguments != "" {
                existing.Arguments += tc.Arguments  // 拼接参数
            }
            if tc.ID != "" {
                existing.ID = tc.ID
            }
        } else {
            // 新建 tool call entry
            toolCallMap[idx] = &toolCallInfo{
                ID:        tc.ID,
                Name:      tc.Name,
                Arguments: tc.Arguments,
            }
        }
    }
}
```

### Usage

```go
type Usage struct {
    PromptTokens     int64 `json:"prompt_tokens"`
    CompletionTokens int64 `json:"completion_tokens"`
    TotalTokens      int64 `json:"total_tokens"`
}
```

### ToolCall（完整工具调用）

用于非流式场景中的完整工具调用表示：

```go
type ToolCall struct {
    ID       string       `json:"id"`
    Type     string       `json:"type"` // 总是 "function"
    Function FunctionCall `json:"function"`
}

type FunctionCall struct {
    Name      string `json:"name"`      // 函数名
    Arguments string `json:"arguments"` // JSON 编码的参数
}
```

## 在引擎中的使用

```go
// engine.go: handleStreaming()
func (e *engineImpl) handleStreaming(...) (*StreamResult, error) {
    // 1. 选择 Provider（引擎级覆盖优先，否则用 Agent 定义的）
    provider := e.llmProvider
    if provider == nil {
        provider = agentDef.LLMProvider
    }

    // 2. 构造请求参数
    params := llm.StreamParams{
        Model:    agentDef.Model,
        Messages: llmMessages,
        Tools:    tools,
    }

    // 3. 发起流式请求
    ch, errc := provider.Stream(chatCtx.Context(), params)

    // 4. 遍历 chunk，实时 Emit SSE 事件
    for chunk := range ch {
        if chunk.Content != "" {
            // Emit part_create（首次）/ part_update
        }
        if chunk.ReasoningContent != "" {
            // Emit reasoning part_create / part_update
        }
        // ... tool call 增量累积 ...
    }

    // 5. 检查错误
    select {
    case err, ok := <-errc:
        if ok && err != nil {
            return nil, fmt.Errorf("stream error: %w", err)
        }
    default:
    }
}
```

## 为什么需要抽象层

| 原因 | 说明 |
|------|------|
| **Provider 无关** | 引擎不 import 任何 LLM SDK，不依赖 OpenAI 具体类型 |
| **可测试性** | 单元测试中可以注入 mock provider |
| **多 Provider 支持** | 可以同时支持 OpenAI、Anthropic、本地 Ollama 等 |
| **按 Agent 切换** | 不同 Agent 可以使用不同 Provider（通过 `AgentDefinition.LLMProvider`） |
| **全局替换** | 通过 `WithLLMProvider()` 对所有 Agent 统一切换后端 |

## 内置实现：OpenAIAdapter

CopCon 提供了基于 OpenAI SDK 的内置实现（位于 `internal/llm/adapter.go` 或类似文件中），实现了 `LLMProvider` 接口，将 OpenAI 的流式响应转换为标准的 `StreamChunk` 类型。

基本使用：

```go
// 创建 OpenAI provider
provider := llm.NewOpenAIProvider("https://api.openai.com/v1", "sk-xxx")

// 注入引擎
engine := agent.NewAgentEngine(
    registry, sessionMgr, contextMgr, asyncReg,
    agent.WithLLMProvider(provider),
)
```

## 自定义 Provider 实现

实现 `LLMProvider` 接口即可接入任意 LLM 后端：

```go
type MyProvider struct {
    // ...
}

func (p *MyProvider) Stream(
    ctx context.Context,
    params llm.StreamParams,
) (<-chan llm.StreamChunk, <-chan error) {
    ch := make(chan llm.StreamChunk, 64)
    errc := make(chan error, 1)

    go func() {
        defer close(ch)
        defer close(errc)

        // 调用你的 LLM API
        // 将响应转换为 StreamChunk 增量
        for delta := range myAPIResponse {
            select {
            case ch <- llm.StreamChunk{
                Content: delta.Text,
            }:
            case <-ctx.Done():
                return
            }
        }
    }()

    return ch, errc
}
```

注意事项：
- Provider 负责关闭两个 channel
- 遵循 `ctx.Done()` 以支持超时和取消
- channel 应设置合理 buffer（建议 64）以防止 goroutine 阻塞

---

上一篇：[03-hook-system.md](./03-hook-system.md)
下一篇：[05-chat-context.md](./05-chat-context.md)