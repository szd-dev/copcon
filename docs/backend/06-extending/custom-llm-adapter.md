# 自定义 LLM Adapter

CopCon 通过 `LLMProvider` 接口与 LLM 后端解耦。内置了 OpenAI 兼容适配器。如果需要对接其他 LLM 服务（如 Anthropic Claude、Azure OpenAI、本地模型），可以实现自定义适配器。

## LLMProvider 接口

```go
// core/llm/provider.go
type LLMProvider interface {
    Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error)
}
```

只需一个方法。`Stream` 返回两个只读通道：

- `ch`：流式输出的 `StreamChunk`，通道关闭表示流结束
- `errc`：最多发送一个 error，然后关闭

两个通道都由 Provider 创建和关闭。调用方必须读完 `ch` 直到关闭。

## 核心数据类型

### StreamParams

```go
type StreamParams struct {
    Model       string    `json:"model"`          // 模型标识，如 "gpt-4o"
    Messages    []Message `json:"messages"`        // 对话历史
    Tools       []ToolDef `json:"tools,omitempty"` // 工具定义
    Temperature float64   `json:"temperature,omitempty"`
    MaxTokens   int       `json:"max_tokens,omitempty"`
}
```

### Message

```go
type Message struct {
    Role       StreamRole `json:"role"`                   // "system", "user", "assistant", "tool"
    Content    string     `json:"content"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant 消息的工具调用
    ToolCallID string     `json:"tool_call_id,omitempty"` // tool 消息的调用 ID
    Name       string     `json:"name,omitempty"`
}
```

### StreamChunk

```go
type StreamChunk struct {
    Content          string          `json:"content,omitempty"`           // 文本增量
    ReasoningContent string          `json:"reasoning_content,omitempty"` // 推理链增量
    ToolCalls        []ToolCallDelta `json:"tool_calls,omitempty"`       // 工具调用增量
    Usage            *Usage          `json:"usage,omitempty"`             // Token 统计（最后一个 chunk）
    FinishReason     string          `json:"finish_reason,omitempty"`     // "stop", "length", "tool_calls"
}
```

### ToolCallDelta

```go
type ToolCallDelta struct {
    Index     int    `json:"index"`               // 工具调用索引（0-based）
    ID        string `json:"id,omitempty"`         // 调用 ID（可能延后到达）
    Name      string `json:"name,omitempty"`       // 函数名
    Arguments string `json:"arguments,omitempty"`  // JSON 参数片段，需拼接
}
```

### ToolDef

```go
type ToolDef struct {
    Name        string          `json:"name"`                  // 工具名
    Description string          `json:"description"`           // 工具描述
    Parameters  json.RawMessage `json:"parameters"`            // JSON Schema
}
```

## 流式响应处理

流式适配器的核心是：从 LLM SDK 逐个读取 chunk，转换为 `StreamChunk`，写入通道。

### 基本模式

```go
func (a *MyAdapter) Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error) {
    ch := make(chan StreamChunk)
    errc := make(chan error, 1) // 缓冲 1，确保不会阻塞

    go func() {
        defer close(ch)
        defer close(errc)

        // 1. 发起流式请求
        stream, err := a.client.CreateStream(ctx, convertParams(params))
        if err != nil {
            errc <- fmt.Errorf("stream request failed: %w", err)
            return
        }

        // 2. 逐个读取 chunk
        for stream.Next() {
            select {
            case <-ctx.Done():
                return // 上下文取消，退出
            default:
            }

            chunk := convertChunk(stream.Current())
            select {
            case ch <- chunk:
            case <-ctx.Done():
                return
            }
        }

        // 3. 检查流错误
        if err := stream.Err(); err != nil {
            select {
            case errc <- fmt.Errorf("stream error: %w", err):
            case <-ctx.Done():
            }
            return
        }

        // 4. 发送最终 chunk（带 Usage）
        finalChunk := StreamChunk{
            Usage:        &Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
            FinishReason: "stop",
        }
        select {
        case ch <- finalChunk:
        case <-ctx.Done():
        }
    }()

    return ch, errc
}
```

### Tool Call 增量累积

Tool Call 的参数在流式响应中是分片到达的。需要按 `Index` 累积拼接：

```go
type toolCallAccum struct {
    ID        string
    Name      string
    Arguments string
}

// 在流式循环中
toolCallMap := make(map[int]*toolCallAccum)

for _, tc := range delta.ToolCalls {
    idx := tc.Index
    if existing, ok := toolCallMap[idx]; ok {
        // 追加到已有的累积器
        if tc.ID != "" {
            existing.ID = tc.ID
        }
        if tc.Name != "" {
            existing.Name = tc.Name
        }
        if tc.Arguments != "" {
            existing.Arguments += tc.Arguments
        }
        // 转发增量 delta（只包含当前 chunk 的新内容）
        chunkOut.ToolCalls = append(chunkOut.ToolCalls, ToolCallDelta{
            Index:     idx,
            ID:        existing.ID,
            Name:      existing.Name,
            Arguments: tc.Arguments, // 当前增量，非累积值
        })
    } else {
        // 新的工具调用
        toolCallMap[idx] = &toolCallAccum{
            ID:        tc.ID,
            Name:      tc.Name,
            Arguments: tc.Arguments,
        }
        chunkOut.ToolCalls = append(chunkOut.ToolCalls, ToolCallDelta{
            Index: idx, ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments,
        })
    }
}
```

## 示例：Anthropic Claude 适配器

```go
package claude

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/copcon/core/llm"
)

// 编译时检查接口实现
var _ llm.LLMProvider = (*ClaudeAdapter)(nil)

type ClaudeAdapter struct {
    client *anthropic.Client
}

func NewClaudeAdapter(client *anthropic.Client) *ClaudeAdapter {
    return &ClaudeAdapter{client: client}
}

func (a *ClaudeAdapter) Stream(ctx context.Context, params llm.StreamParams) (<-chan llm.StreamChunk, <-chan error) {
    ch := make(chan llm.StreamChunk)
    errc := make(chan error, 1)

    go func() {
        defer close(ch)
        defer close(errc)

        // 转换消息格式
        anthropicMsgs := convertMessages(params.Messages)
        systemPrompt := extractSystemPrompt(params.Messages)

        // 构建请求
        req := anthropic.MessageNewParams{
            Model:     anthropic.Model(params.Model),
            MaxTokens: int64(params.MaxTokens),
            Messages:  anthropicMsgs,
        }
        if systemPrompt != "" {
            req.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
        }
        if params.Temperature > 0 {
            req.Temperature = anthropic.Float(params.Temperature)
        }

        // 转换工具定义
        if len(params.Tools) > 0 {
            req.Tools = convertTools(params.Tools)
        }

        // 发起流式请求
        stream := a.client.Messages.NewStreaming(ctx, req)

        toolCallMap := make(map[int]*toolCallAccum)
        var usage llm.Usage

        for stream.Next() {
            select {
            case <-ctx.Done():
                return
            default:
            }

            event := stream.Current()

            switch variant := event.(type) {
            case anthropic.ContentBlockDeltaEvent:
                chunkOut := llm.StreamChunk{}

                switch delta := variant.Delta.(type) {
                case anthropic.TextDelta:
                    chunkOut.Content = delta.Text
                case anthropic.ToolUseDelta:
                    idx := int(delta.Index)
                    chunkOut.ToolCalls = []llm.ToolCallDelta{{
                        Index:     idx,
                        Arguments: delta.PartialJSON,
                    }}
                    if existing, ok := toolCallMap[idx]; ok {
                        existing.Arguments += delta.PartialJSON
                    }
                }

                sendChunk(ch, ctx, chunkOut)

            case anthropic.ContentBlockStartEvent:
                if toolUse, ok := variant.ContentBlock.(anthropic.ToolUseBlock); ok {
                    idx := int(variant.Index)
                    toolCallMap[idx] = &toolCallAccum{
                        ID:   toolUse.ID,
                        Name: toolUse.Name,
                    }
                    // 发送工具调用开始
                    sendChunk(ch, ctx, llm.StreamChunk{
                        ToolCalls: []llm.ToolCallDelta{{
                            Index: idx,
                            ID:    toolUse.ID,
                            Name:  toolUse.Name,
                        }},
                    })
                }

            case anthropic.MessageStopEvent:
                // 发送最终 chunk
                sendChunk(ch, ctx, llm.StreamChunk{
                    Usage:        &usage,
                    FinishReason: "stop",
                })
            }
        }

        if err := stream.Err(); err != nil {
            select {
            case errc <- fmt.Errorf("claude stream: %w", err):
            case <-ctx.Done():
            }
        }
    }()

    return ch, errc
}

type toolCallAccum struct {
    ID        string
    Name      string
    Arguments string
}

func sendChunk(ch chan<- llm.StreamChunk, ctx context.Context, chunk llm.StreamChunk) {
    select {
    case ch <- chunk:
    case <-ctx.Done():
    }
}

func extractSystemPrompt(messages []llm.Message) string {
    for _, msg := range messages {
        if msg.Role == llm.RoleSystem {
            return msg.Content
        }
    }
    return ""
}

func convertMessages(messages []llm.Message) []anthropic.MessageParam {
    var result []anthropic.MessageParam
    for _, msg := range messages {
        switch msg.Role {
        case llm.RoleUser:
            result = append(result, anthropic.NewUserMessage(
                anthropic.NewTextBlock(msg.Content),
            ))
        case llm.RoleAssistant:
            var blocks []anthropic.ContentBlockParamUnion
            if msg.Content != "" {
                blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
            }
            for _, tc := range msg.ToolCalls {
                blocks = append(blocks, anthropic.NewToolUseBlock(
                    tc.ID, tc.Function.Name, json.RawMessage(tc.Function.Arguments),
                ))
            }
            result = append(result, anthropic.NewAssistantMessage(blocks...))
        case llm.RoleTool:
            result = append(result, anthropic.NewUserMessage(
                anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content),
            ))
        }
    }
    return result
}

func convertTools(tools []llm.ToolDef) []anthropic.ToolUnionParam {
    result := make([]anthropic.ToolUnionParam, 0, len(tools))
    for _, td := range tools {
        var params map[string]any
        if len(td.Parameters) > 0 {
            _ = json.Unmarshal(td.Parameters, &params)
        }
        result = append(result, anthropic.ToolUnionParam{
            OfTool: &anthropic.ToolParam{
                Name:        td.Name,
                Description: anthropic.String(td.Description),
                InputSchema: params,
            },
        })
    }
    return result
}
```

## 示例：Azure OpenAI 适配器

Azure OpenAI 使用与 OpenAI 相同的 API 格式，但认证方式和端点不同。

```go
package azure

import (
    "context"
    "fmt"
    "net/http"

    oai "github.com/openai/openai-go/v3"
    "github.com/openai/openai-go/v3/option"

    "github.com/copcon/core/llm"
)

var _ llm.LLMProvider = (*AzureAdapter)(nil)

type Config struct {
    Endpoint    string // 如 "https://your-resource.openai.azure.com"
    APIKey      string
    Deployment  string // Azure 部署名称
    APIVersion  string // 如 "2024-06-01"
}

type AzureAdapter struct {
    client *oai.Client
    model  string
}

func NewAzureAdapter(cfg Config) *AzureAdapter {
    client := oai.NewClient(
        option.WithBaseURL(fmt.Sprintf("%s/openai/deployments/%s", cfg.Endpoint, cfg.Deployment)),
        option.WithAPIKey(cfg.APIKey),
        option.WithQueryParam("api-version", cfg.APIVersion),
    )
    return &AzureAdapter{client: &client, model: cfg.Deployment}
}

func (a *AzureAdapter) Stream(ctx context.Context, params llm.StreamParams) (<-chan llm.StreamChunk, <-chan error) {
    // Azure OpenAI 的流式响应格式与标准 OpenAI 一致，
    // 可以直接复用 OpenAI 适配器的流处理逻辑。
    openaiAdapter := llm.NewOpenAIAdapter(a.client, a.model)
    return openaiAdapter.Stream(ctx, params)
}
```

## Token 计数与速率限制

### Usage 统计

在流的最后一个 chunk 中发送 `Usage`：

```go
finalChunk := llm.StreamChunk{
    Usage: &llm.Usage{
        PromptTokens:     promptTokens,
        CompletionTokens: completionTokens,
        TotalTokens:      promptTokens + completionTokens,
    },
    FinishReason: "stop",
}
```

如果后端 SDK 不直接提供 token 计数，可以估算：

```go
func estimateTokens(text string) int64 {
    // 粗略估算：英文约 4 字符 = 1 token，中文约 2 字符 = 1 token
    return int64(len(text) / 3)
}
```

### 速率限制处理

在适配器层面处理速率限制，使用指数退避重试：

```go
func (a *MyAdapter) Stream(ctx context.Context, params llm.StreamParams) (<-chan llm.StreamChunk, <-chan error) {
    ch := make(chan llm.StreamChunk)
    errc := make(chan error, 1)

    go func() {
        defer close(ch)
        defer close(errc)

        var lastErr error
        for attempt := 0; attempt < 3; attempt++ {
            if attempt > 0 {
                delay := time.Duration(attempt*attempt) * time.Second
                select {
                case <-time.After(delay):
                case <-ctx.Done():
                    return
                }
            }

            stream, err := a.client.CreateStream(ctx, convertParams(params))
            if err != nil {
                if isRateLimitError(err) {
                    lastErr = err
                    continue // 重试
                }
                errc <- fmt.Errorf("stream request failed: %w", err)
                return
            }

            // 成功，转发流数据
            lastErr = nil
            a.forwardStream(ctx, stream, ch, errc)
            return
        }

        // 重试耗尽
        errc <- fmt.Errorf("rate limited after 3 retries: %w", lastErr)
    }()

    return ch, errc
}

func isRateLimitError(err error) bool {
    // 检查 HTTP 429 状态码
    var apiErr *anthropic.APIError
    if errors.As(err, &apiErr) {
        return apiErr.StatusCode == 429
    }
    return false
}
```

## 错误处理策略

### 错误分类

| 错误类型 | 处理方式 |
|---------|---------|
| 上下文取消 | 直接返回，不发送 error |
| 速率限制 (429) | 指数退避重试 |
| 认证失败 (401/403) | 通过 errc 返回，不重试 |
| 服务器错误 (5xx) | 短暂重试后返回 |
| 无效请求 (400) | 通过 errc 返回，不重试 |
| 网络超时 | 重试一次后返回 |

### 错误包装

始终用 `fmt.Errorf` 包装错误，保留上下文：

```go
errc <- fmt.Errorf("claude stream: %w", err)
```

不要吞掉错误，也不要在错误消息中暴露 API Key 或其他敏感信息。

## 编译时检查

始终添加接口合规性检查：

```go
var _ llm.LLMProvider = (*ClaudeAdapter)(nil)
```

如果接口签名发生变化，编译时就会报错，而不是运行时 panic。

## 注册自定义 LLM Provider

### 通过 Harness 配置

```go
import (
    "github.com/copcon/core"
    "github.com/yourorg/copcon-claude/claude"
)

client := anthropic.NewClient()
adapter := claude.NewClaudeAdapter(&client)

harness, err := core.NewHarness(&core.HarnessConfig{
    LLMProvider: adapter,
    // ... 其他配置
})
```

### 动态选择 Provider

```go
func createProvider(cfg *Config) llm.LLMProvider {
    switch cfg.Provider {
    case "openai":
        client := oai.NewClient()
        return llm.NewOpenAIAdapter(&client, cfg.Model)
    case "claude":
        client := anthropic.NewClient()
        return claude.NewClaudeAdapter(&client)
    case "azure":
        return azure.NewAzureAdapter(azure.Config{
            Endpoint:   cfg.AzureEndpoint,
            APIKey:     cfg.AzureAPIKey,
            Deployment: cfg.AzureDeployment,
            APIVersion: cfg.AzureAPIVersion,
        })
    default:
        panic("unknown LLM provider: " + cfg.Provider)
    }
}
```

## 测试 LLM 适配器

### 使用 MockProvider

内置的 `MockProvider` 返回空流，适合快速验证管线：

```go
func TestWithMockProvider(t *testing.T) {
    provider := llm.NewMockProvider()
    ch, errc := provider.Stream(context.Background(), llm.StreamParams{
        Model: "test",
        Messages: []llm.Message{
            {Role: llm.RoleUser, Content: "hello"},
        },
    })

    // 流立即关闭
    for range ch {}
    for err := range errc {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

### 自定义 Mock

创建返回特定内容的 Mock：

```go
type FixedResponseProvider struct {
    response string
}

func (p *FixedResponseProvider) Stream(ctx context.Context, params llm.StreamParams) (<-chan llm.StreamChunk, <-chan error) {
    ch := make(chan llm.StreamChunk, 1)
    errc := make(chan error, 1)

    ch <- llm.StreamChunk{Content: p.response}
    ch <- llm.StreamChunk{FinishReason: "stop", Usage: &llm.Usage{TotalTokens: 10}}

    close(ch)
    close(errc)
    return ch, errc
}
```

### 适配器测试

```go
func TestClaudeAdapter_InterfaceCompliance(t *testing.T) {
    // 编译时检查
    var _ llm.LLMProvider = (*ClaudeAdapter)(nil)
}

func TestClaudeAdapter_MessageConversion(t *testing.T) {
    messages := []llm.Message{
        {Role: llm.RoleSystem, Content: "You are helpful."},
        {Role: llm.RoleUser, Content: "Hello"},
        {Role: llm.RoleAssistant, Content: "Hi!", ToolCalls: []llm.ToolCall{
            {ID: "call_1", Type: "function", Function: llm.FunctionCall{Name: "search", Arguments: `{"q":"test"}`}},
        }},
        {Role: llm.RoleTool, ToolCallID: "call_1", Content: "result"},
    }

    converted := convertMessages(messages)
    assert.Len(t, converted, 3) // system prompt 被提取，3 条非 system 消息
}
```

## 最佳实践

1. **goroutine 管理**。`Stream` 必须在 goroutine 中执行流式读取，确保 `ch` 和 `errc` 最终关闭
2. **上下文尊重**。在每次写通道前检查 `ctx.Done()`，避免在请求取消后继续工作
3. **通道容量**。`errc` 用缓冲 1 的通道，确保即使调用方不读也不会阻塞 goroutine
4. **幂等关闭**。`close(ch)` 和 `close(errc)` 只能调用一次，用 `defer` 保证
5. **重试边界**。重试 3 次足够，过多重试会让用户等太久
6. **错误可追溯**。所有 error 都用 `fmt.Errorf("adapter_name: %w", err)` 包装

## 下一步

- [自定义 Tool](custom-tool.md) - 编写 Agent 可调用的自定义工具
- [自定义 Provider](custom-provider.md) - 实现自定义存储后端
- [测试自定义实现](testing-custom-implementations.md) - 测试和基准测试指南
