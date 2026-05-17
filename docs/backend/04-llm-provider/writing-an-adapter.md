# 编写 LLM Adapter 完整指南

本文以 Anthropic Claude 为例，演示如何为新的模型 provider 实现 `LLMProvider` 接口。

## 前置知识

阅读本文之前，确保你已理解：

- [LLMProvider 接口规范](./interface.md) — `Stream`、`StreamParams`、`StreamChunk` 等类型的定义
- [OpenAI Adapter](./openai-adapter.md) — 现有的参考实现

## 步骤一：创建 adapter 结构体

```go
// /server/internal/llm/claude_adapter.go
package llm

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/anthropics/anthropic-sdk-go/option"
)

var _ LLMProvider = (*ClaudeAdapter)(nil)

type ClaudeAdapter struct {
    client *anthropic.Client
    model  string
}

func NewClaudeAdapter(client *anthropic.Client, model string) *ClaudeAdapter {
    return &ClaudeAdapter{client: client, model: model}
}
```

关键点：

- 使用 `var _ LLMProvider = (*ClaudeAdapter)(nil)` 做编译期接口检查
- 结构体持有 SDK 客户端和默认模型名
- 构造函数保持简洁

## 步骤二：实现 Stream() 方法

`Stream` 方法必须启动一个 goroutine，在其中：

1. 调用 provider SDK 的流式 API
2. 将每一个 provider chunk 转换为 `StreamChunk`
3. 发送到 data channel
4. 流结束时关闭两个 channel
5. 错误发生时发送到 error channel 再返回

### 模板骨架

```go
func (a *ClaudeAdapter) Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error) {
    ch := make(chan StreamChunk)
    errc := make(chan error, 1)

    go func() {
        defer close(ch)
        defer close(errc)

        // 1. 转换请求参数
        // 2. 发起流式请求
        // 3. 循环读取响应
        // 4. 转换并发送 StreamChunk
        // 5. 处理错误
    }()

    return ch, errc
}
```

### Channel 生命周期规则

- `ch` 和 `errc` 必须由 goroutine 关闭（调用方只是读取）
- `errc` 必须带缓冲（`make(chan error, 1)`），防止 goroutine 永久阻塞
- defer 确保即使 panic 也会关闭 channel
- goroutine 必须监听 `ctx.Done()`，在 context 取消时立即退出

## 步骤三：转换请求格式

Claude 的 Messages API 与 OpenAI 的 Chat Completions API 格式不同。我们需要将 `StreamParams` 映射过去。

### 消息转换

Claude 的角色只有 `user` 和 `assistant`，system prompt 作为顶层参数：

```go
func convertToClaudeMessages(messages []Message) (system string, msgs []anthropic.MessageParam) {
    for _, msg := range messages {
        switch msg.Role {
        case RoleSystem:
            system += msg.Content + "\n"
        case RoleUser:
            msgs = append(msgs, anthropic.NewUserMessage(
                anthropic.NewTextBlock(msg.Content),
            ))
        case RoleAssistant:
            if len(msg.ToolCalls) > 0 {
                // assistant 消息中的工具调用
                blocks := []anthropic.ContentBlockParamUnion{}
                if msg.Content != "" {
                    blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
                }
                for _, tc := range msg.ToolCalls {
                    blocks = append(blocks, anthropic.NewToolUseBlock(
                        tc.ID, tc.Function.Name,
                        json.RawMessage(tc.Function.Arguments),
                    ))
                }
                msgs = append(msgs, anthropic.NewAssistantMessage(blocks...))
            } else {
                msgs = append(msgs, anthropic.NewAssistantMessage(
                    anthropic.NewTextBlock(msg.Content),
                ))
            }
        case RoleTool:
            // Claude 中工具结果以 user 消息的 tool_result block 形式呈现
            msgs = append(msgs, anthropic.NewUserMessage(
                anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content),
            ))
        }
    }
    return system, msgs
}
```

### 工具转换

Claude 使用 `ToolUnionParam`，格式与 OpenAI 完全不同：

```go
func convertToClaudeTools(tools []ToolDef) []anthropic.ToolUnionParam {
    result := make([]anthropic.ToolUnionParam, 0, len(tools))
    for _, td := range tools {
        var inputSchema map[string]any
        json.Unmarshal(td.Parameters, &inputSchema)

        result = append(result, anthropic.ToolParam{
            Name:        td.Name,
            Description: anthropic.String(td.Description),
            InputSchema: anthropic.ToolInputSchemaParam{
                Properties: inputSchema["properties"],
            },
        })
    }
    return result
}
```

### 发起请求

```go
system, messages := convertToClaudeMessages(params.Messages)
tools := convertToClaudeTools(params.Tools)

// Claude 不直接支持 Temperature=0（表示"未设置"），跳过即可
var temperature *float64
if params.Temperature > 0 {
    temperature = &params.Temperature
}

stream := a.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
    Model:       anthropic.Model(params.Model),
    System:      anthropic.MessageNewParamsSystemUnion(
        &system,
    ),
    MaxTokens:   int64(params.MaxTokens),
    Messages:    messages,
    Tools:       tools,
    Temperature: anthropic.Float(*temperature),
})
```

## 步骤四：映射响应到 StreamChunk

Claude 的流式事件类型动画丰富，需要按事件类型分发。

```go
var (
    fullContent    string
    fullReasoning  string
    toolUseAcc     = make(map[int]*toolCallAccum)
    finalUsage     *Usage
)

for stream.Next() {
    select {
    case <-ctx.Done():
        return
    default:
    }

    event := stream.Current()

    switch event := event.AsAny().(type) {

    case *anthropic.ContentBlockStartEvent:
        if event.ContentBlock.Type == "tool_use" {
            // 新工具调用开始
            toolUse := event.ContentBlock.ToolUse
            idx := len(toolUseAcc) // Claude 没有 Index，用顺序做索引
            toolUseAcc[idx] = &toolCallAccum{
                ID:   toolUse.ID,
                Name: toolUse.Name,
            }
        }

    case *anthropic.ContentBlockDeltaEvent:
        switch event.Delta.Type {
        case "text_delta":
            fullContent += event.Delta.Text
            ch <- StreamChunk{Content: event.Delta.Text}

        case "input_json_delta":
            // 工具参数的 JSON 片段
            // Claude 的 content block 在同一个事件流中，需要通过 index 定位
            // 这里简化处理：通过 event.Index 定位
            idx := int(event.Index)
            if acc, ok := toolUseAcc[idx]; ok {
                acc.Arguments += event.Delta.PartialJSON
                ch <- StreamChunk{
                    ToolCalls: []ToolCallDelta{
                        {
                            Index:     idx,
                            ID:        acc.ID,
                            Name:      acc.Name,
                            Arguments: event.Delta.PartialJSON,
                        },
                    },
                }
            }

        case "thinking_delta":
            fullReasoning += event.Delta.Thinking
            ch <- StreamChunk{ReasoningContent: event.Delta.Thinking}
        }

    case *anthropic.MessageDeltaEvent:
        // 流结束事件，包含 finish_reason 和 usage
        finalUsage = &Usage{
            CompletionTokens: int64(event.Usage.OutputTokens),
            TotalTokens:      int64(event.Usage.InputTokens + event.Usage.OutputTokens),
            PromptTokens:     int64(event.Usage.InputTokens),
        }

        ch <- StreamChunk{
            FinishReason: string(event.Delta.StopReason),
        }
    }
}

// 流结束后检查错误
if err := stream.Err(); err != nil {
    errc <- fmt.Errorf("claude stream: %w", err)
    return
}

// 发送最终 chunk（含 usage）
if finalUsage != nil {
    ch <- StreamChunk{Usage: finalUsage}
}
```

## 步骤五：错误处理、取消与清理

### Context 取消

goroutine 必须在每次循环中检查 `ctx.Done()`。Context 取消可能发生在：

- Agent Engine 层面超时
- 用户主动中断对话
- 上层 context 传播取消信号

```go
for stream.Next() {
    select {
    case <-ctx.Done():
        return  // defer 会关闭 ch 和 errc，无需额外操作
    default:
    }
    // ... 处理 event
}
```

### SDK 错误

流式循环结束后检查 `stream.Err()`：

```go
if err := stream.Err(); err != nil {
    select {
    case errc <- fmt.Errorf("claude stream: %w", err):
    case <-ctx.Done():
    }
    return
}
```

注意 `errc <- err` 外层的 select：如果 context 已经取消，发送到 errc 可能阻塞（虽然有缓冲，但消费端可能已经不再读取）。

### Panic 保护

如果 adapter 内有 recover 需求，可以加在最外层：

```go
go func() {
    defer close(ch)
    defer close(errc)
    defer func() {
        if r := recover(); r != nil {
            errc <- fmt.Errorf("claude adapter panic: %v", r)
        }
    }()

    // ... 正常逻辑
}()
```

## 完整示例

```go
// /server/internal/llm/claude_adapter.go
package llm

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/anthropics/anthropic-sdk-go"
)

var _ LLMProvider = (*ClaudeAdapter)(nil)

type ClaudeAdapter struct {
    client *anthropic.Client
    model  string
}

func NewClaudeAdapter(client *anthropic.Client, model string) *ClaudeAdapter {
    return &ClaudeAdapter{client: client, model: model}
}

func (a *ClaudeAdapter) Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error) {
    ch := make(chan StreamChunk)
    errc := make(chan error, 1)

    go func() {
        defer close(ch)
        defer close(errc)

        // 转换消息和工具
        system, messages := convertToClaudeMessages(params.Messages)
        tools := convertToClaudeTools(params.Tools)

        // 构建请求
        req := anthropic.MessageNewParams{
            Model:     anthropic.Model(params.Model),
            MaxTokens: int64(params.MaxTokens),
            Messages:  messages,
            Tools:     tools,
        }
        if system != "" {
            req.System = anthropic.MessageNewParamsSystemUnion(&system)
        }
        if params.Temperature > 0 {
            req.Temperature = anthropic.Float(params.Temperature)
        }

        stream := a.client.Messages.NewStreaming(ctx, req)

        toolAcc := make(map[int]*toolCallAccum)
        var finalUsage *Usage

        for stream.Next() {
            select {
            case <-ctx.Done():
                return
            default:
            }

            event := stream.Current()

            switch evt := event.AsAny().(type) {
            case *anthropic.ContentBlockStartEvent:
                if evt.ContentBlock.Type == "tool_use" {
                    tu := evt.ContentBlock.ToolUse
                    idx := int(evt.Index)
                    toolAcc[idx] = &toolCallAccum{ID: tu.ID, Name: tu.Name}
                }

            case *anthropic.ContentBlockDeltaEvent:
                idx := int(evt.Index)
                switch evt.Delta.Type {
                case "text_delta":
                    ch <- StreamChunk{Content: evt.Delta.Text}
                case "input_json_delta":
                    if acc, ok := toolAcc[idx]; ok {
                        acc.Arguments += evt.Delta.PartialJSON
                        ch <- StreamChunk{
                            ToolCalls: []ToolCallDelta{{
                                Index:     idx,
                                ID:        acc.ID,
                                Name:      acc.Name,
                                Arguments: evt.Delta.PartialJSON,
                            }},
                        }
                    }
                case "thinking_delta":
                    ch <- StreamChunk{ReasoningContent: evt.Delta.Thinking}
                }

            case *anthropic.MessageDeltaEvent:
                finalUsage = &Usage{
                    PromptTokens:     int64(evt.Usage.InputTokens),
                    CompletionTokens: int64(evt.Usage.OutputTokens),
                    TotalTokens:      int64(evt.Usage.InputTokens + evt.Usage.OutputTokens),
                }
                ch <- StreamChunk{FinishReason: string(evt.Delta.StopReason)}
            }
        }

        if err := stream.Err(); err != nil {
            select {
            case errc <- fmt.Errorf("claude stream: %w", err):
            case <-ctx.Done():
            }
            return
        }

        if finalUsage != nil {
            ch <- StreamChunk{Usage: finalUsage}
        }
    }()

    return ch, errc
}

// 辅助结构体定义省略，见前文
```

## 注册 Adapter

在 Engine 初始化时注入 adapter：

```go
// 创建 Claude adapter
claudeClient := anthropic.NewClient(
    option.WithAPIKey("sk-ant-..."),
)
claudeAdapter := llm.NewClaudeAdapter(claudeClient, "claude-3-opus-20240229")

// 注入到 Engine
engine := agent.NewAgentEngine(
    agent.WithLLMProvider(claudeAdapter),
    // ...其他配置
)
```

## 编写测试

使用 `MockProvider` 的模式编写 adapter 测试：

```go
func TestClaudeAdapter_ImplementsInterface(t *testing.T) {
    var _ llm.LLMProvider = (*llm.ClaudeAdapter)(nil)
}

func TestClaudeAdapter_Stream(t *testing.T) {
    // 使用真实的测试 API key
    // 或 mock anthropic.Client 的 HTTP transport
}
```

## 检查清单

编写新 adapter 后请确认：

- [ ] 实现 `LLMProvider` 接口（编译期断言）
- [ ] `Stream()` 在独立 goroutine 中运行
- [ ] 两个 channel 都通过 defer 关闭
- [ ] `errc` 是带缓冲的（`make(chan error, 1)`）
- [ ] 消息角色映射完整：system、user、assistant、tool
- [ ] 工具调用按 Index 累积
- [ ] ReasoningContent 正确提取（如 provider 支持）
- [ ] Usage 在流末尾发送
- [ ] Context 取消时 goroutine 正确退出
- [ ] SDK 错误正确转发到 errc