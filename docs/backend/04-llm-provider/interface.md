# LLMProvider 接口规范

`LLMProvider` 是 CopCon 与任何大语言模型进行通信的统一抽象层。所有模型后端（OpenAI、Anthropic、本地模型等）都通过实现此接口接入系统。

## 接口定义

```go
type LLMProvider interface {
    Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error)
}
```

`Stream` 方法发起一次流式补全请求，返回两个 channel：

- `ch`（`<-chan StreamChunk`）：接收模型响应中每个增量块，当流结束时关闭。
- `errc`（`<-chan error`）：最多接收一个错误，之后关闭。无错误时空置。

调用方必须持续从 `ch` 读取直到其关闭。`errc` 可以在 `ch` 耗尽后检查，也可以通过 `select` 并发监听。

## StreamParams

`StreamParams` 封装了一次流式请求所需的全部参数。

```go
type StreamParams struct {
    Model       string    `json:"model"`
    Messages    []Message `json:"messages"`
    Tools       []ToolDef `json:"tools,omitempty"`
    Temperature float64   `json:"temperature,omitempty"`
    MaxTokens   int       `json:"max_tokens,omitempty"`
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `Model` | `string` | 模型标识符，例如 `"gpt-4o"`、`"claude-3-opus-20240229"` |
| `Messages` | `[]Message` | 完整的对话历史，包含系统提示词 |
| `Tools` | `[]ToolDef` | 可用工具/函数定义列表。空或 nil 表示无可用工具 |
| `Temperature` | `float64` | 随机性控制（0.0~2.0）。值为 0 表示未设置，使用模型默认值 |
| `MaxTokens` | `int` | 最大输出 token 数。值为 0 表示不设限 |

`Message` 表示对话中的单条消息：

```go
type Message struct {
    Role       StreamRole  `json:"role"`         // system / user / assistant / tool
    Content    string      `json:"content"`       // 消息正文
    ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`   // assistant 消息中的工具调用
    ToolCallID string      `json:"tool_call_id,omitempty"` // tool 消息中的调用 ID
    Name       string      `json:"name,omitempty"`         // 可选参与者名称
}
```

`ToolDef` 描述一个可供模型调用的工具：

```go
type ToolDef struct {
    Name        string          `json:"name"`         // 函数名
    Description string          `json:"description"`  // 功能说明
    Parameters  json.RawMessage `json:"parameters"`   // JSON Schema 参数定义
}
```

## StreamChunk

`StreamChunk` 是流式响应中的一个增量块。

```go
type StreamChunk struct {
    Content          string          `json:"content,omitempty"`
    ReasoningContent string          `json:"reasoning_content,omitempty"`
    ToolCalls        []ToolCallDelta `json:"tool_calls,omitempty"`
    Usage            *Usage          `json:"usage,omitempty"`
    FinishReason     string          `json:"finish_reason,omitempty"`
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `Content` | `string` | 文本内容的增量。无文本时为空（如纯工具调用块） |
| `ReasoningContent` | `string` | 模型内部推理/思维链的增量。不支持此功能的 provider 返回空 |
| `ToolCalls` | `[]ToolCallDelta` | 增量工具调用。一个块中可能包含多个工具调用 |
| `Usage` | `*Usage` | 本次请求的 token 统计。中间块通常为 nil，仅最后一块填充 |
| `FinishReason` | `string` | 模型停止原因。中间块为空 |

## 双通道模式

CopCon 采用 **数据通道 + 错误通道** 的并发模式，而不是让 Stream 返回单个带错误的 channel。

```go
func (p *MyProvider) Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error) {
    ch := make(chan StreamChunk)
    errc := make(chan error, 1) // 带缓冲，防止 goroutine 泄露

    go func() {
        defer close(ch)
        defer close(errc)

        // ... 发送 StreamChunk ...

        if err != nil {
            errc <- err  // 非阻塞（缓冲大小为 1）
            return
        }
    }()

    return ch, errc
}
```

调用方使用方式：

```go
ch, errc := provider.Stream(ctx, params)

for chunk := range ch {
    fmt.Println(chunk.Content)
}

// ch 关闭后检查错误
select {
case err := <-errc:
    if err != nil {
        log.Fatal(err)
    }
default:
}
```

这种设计的好处：

1. **类型清晰**：数据通道只传数据，错误通道只传错误，不会出现在 StreamChunk 中嵌入错误信息的混乱局面。
2. **生命周期明确**：两个 channel 都由 provider 创建的 goroutine 负责关闭，调用方只读取。
3. **context 取消安全**：goroutine 通过 `ctx.Done()` 感知取消，避免 goroutine 泄露。

## ToolCallDelta

`ToolCallDelta` 是流式传输中工具调用的增量更新。

```go
type ToolCallDelta struct {
    Index     int    `json:"index"`
    ID        string `json:"id,omitempty"`
    Name      string `json:"name,omitempty"`
    Arguments string `json:"arguments,omitempty"`
}
```

增量按索引累积。为什么需要 `Index` 字段？

模型可能在一次响应中开始多个并行工具调用，每个调用被分配到唯一的索引（从 0 开始）。同一个索引的多条 delta 属于同一个工具调用，调用方需要按索引聚合。

| 字段 | 说明 |
|------|------|
| `Index` | 工具调用索引（从 0 开始），同一索引的增量属于同一个调用 |
| `ID` | 工具调用唯一标识。可能在 Name/Arguments 之后的 chunk 才出现 |
| `Name` | 函数名，可能分片到达 |
| `Arguments` | JSON 参数片段，需与同一索引之前的所有 Arguments 片段拼接 |

累积模式：

```go
accMap := make(map[int]*ToolCallAccum)

for chunk := range ch {
    for _, delta := range chunk.ToolCalls {
        acc, ok := accMap[delta.Index]
        if !ok {
            acc = &ToolCallAccum{}
            accMap[delta.Index] = acc
        }
        if delta.ID != "" {
            acc.ID = delta.ID
        }
        if delta.Name != "" {
            acc.Name = delta.Name
        }
        acc.Arguments += delta.Arguments
    }
}
```

## Usage

`Usage` 记录一次请求的 token 使用统计。

```go
type Usage struct {
    PromptTokens     int64 `json:"prompt_tokens"`
    CompletionTokens int64 `json:"completion_tokens"`
    TotalTokens      int64 `json:"total_tokens"`
}
```

| 字段 | 说明 |
|------|------|
| `PromptTokens` | 输入 token 数 |
| `CompletionTokens` | 输出 token 数 |
| `TotalTokens` | 总计 token 数（prompt + completion） |

`Usage` 通常在流的最后一个 chunk 中填充。中间 chunk 的 `Usage` 为 nil。

## 完整消费模式

一个完整的流消费模式：

```go
func consumeStream(provider llm.LLMProvider, params llm.StreamParams) error {
    ctx := context.Background()
    ch, errc := provider.Stream(ctx, params)

    var fullContent string
    var toolCallAcc = make(map[int]*llm.ToolCallAccum)

    for chunk := range ch {
        fullContent += chunk.Content

        for _, delta := range chunk.ToolCalls {
            acc, ok := toolCallAcc[delta.Index]
            if !ok {
                acc = &ToolCallAccum{}
                toolCallAcc[delta.Index] = acc
            }
            if delta.ID != "" {
                acc.ID = delta.ID
            }
            acc.Name += delta.Name
            acc.Arguments += delta.Arguments
        }

        // Usage 只在最后 chunk 出现
        if chunk.Usage != nil {
            fmt.Printf("Token: prompt=%d completion=%d total=%d\n",
                chunk.Usage.PromptTokens,
                chunk.Usage.CompletionTokens,
                chunk.Usage.TotalTokens,
            )
        }
    }

    select {
    case err := <-errc:
        return err
    default:
    }

    return nil
}
```