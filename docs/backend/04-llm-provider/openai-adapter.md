# OpenAI Adapter

`OpenAIAdapter` 是 `LLMProvider` 的 OpenAI 实现。它封装 `openai-go` SDK 客户端，将 CopCon 的通用参数转换为 OpenAI 原生格式，并通过流式 API 返回统一的 `StreamChunk`。

## 创建适配器

```go
import (
    "github.com/openai/openai-go/v3"
    "github.com/copcon/server/internal/llm"
)

client := openai.NewClient()
adapter := llm.NewOpenAIAdapter(client, "gpt-4o")
```

构造函数签名：

```go
func NewOpenAIAdapter(client *openai.Client, model string) *OpenAIAdapter
```

`model` 参数是默认模型名。实际调用时，`StreamParams.Model` 会覆盖此值。编译期接口检查：

```go
var _ LLMProvider = (*OpenAIAdapter)(nil)
```

## 请求转换：StreamParams → OpenAI 格式

`Stream` 方法的核心逻辑分三步：消息转换、工具转换、请求构建。

### 消息角色映射

`convertMessages` 将 `llm.Message` 切片转换为 `openai.ChatCompletionMessageParamUnion`：

```go
func convertMessages(messages []llm.Message) []openai.ChatCompletionMessageParamUnion {
    for _, msg := range messages {
        switch msg.Role {
        case llm.RoleSystem:
            // → openai.SystemMessage(msg.Content)
        case llm.RoleUser:
            // → openai.UserMessage(msg.Content)
        case llm.RoleAssistant:
            if len(msg.ToolCalls) > 0 {
                // → openai.AssistantMessage(...) + ToolCalls 子结构
            } else {
                // → openai.AssistantMessage(msg.Content)
            }
        case llm.RoleTool:
            // → openai.ToolMessage(msg.Content, msg.ToolCallID)
        }
    }
}
```

角色映射表：

| `llm.StreamRole` | OpenAI 方法 |
|------------------|------------|
| `RoleSystem` | `openai.SystemMessage(content)` |
| `RoleUser` | `openai.UserMessage(content)` |
| `RoleAssistant`（有 ToolCalls） | `openai.AssistantMessage(content)` + 填充 `ToolCalls` |
| `RoleAssistant`（无 ToolCalls） | `openai.AssistantMessage(content)` |
| `RoleTool` | `openai.ToolMessage(content, toolCallID)` |

default 分支将未知角色当作 `UserMessage` 处理。

### 工具定义转换

`convertTools` 将 `llm.ToolDef` 切片转换为 `openai.ChatCompletionToolUnionParam`：

```go
func convertTools(tools []llm.ToolDef) []openai.ChatCompletionToolUnionParam {
    for _, td := range tools {
        var params shared.FunctionParameters
        json.Unmarshal(td.Parameters, &params)

        result = append(result, openai.ChatCompletionFunctionTool(
            shared.FunctionDefinitionParam{
                Name:        td.Name,
                Description: param.NewOpt(td.Description),
                Parameters:  params,
            },
        ))
    }
    return result
}
```

`ToolDef.Parameters` 是 `json.RawMessage`，在转换时被 unmarshal 为 `shared.FunctionParameters`（底层是 `map[string]any`）。

### 请求构建

```go
req := openai.ChatCompletionNewParams{
    Model:             shared.ChatModel(params.Model),
    Messages:          openaiMsgs,
    Tools:             openaiTools,
    ParallelToolCalls: openai.Bool(true),
}

stream := a.client.Chat.Completions.NewStreaming(ctx, req)
```

启用 `ParallelToolCalls: true` 允许模型在一次响应中并行调用多个工具。

## 流式循环内部

`Stream` 方法启动一个 goroutine 来处理流式响应：

```go
go func() {
    defer close(ch)
    defer close(errc)
    defer stream.Close()

    acc := openai.ChatCompletionAccumulator{}
    toolCallMap := make(map[int]*toolCallAccum)

    for stream.Next() {
        chunk := stream.Current()
        acc.AddChunk(chunk)

        // 检查 context 取消
        select {
        case <-ctx.Done():
            return
        default:
        }

        // 处理每个 choice 的 delta
        ...
    }

    // 流结束后发送最终 chunk（含 usage）
    ...
}()
```

`ChatCompletionAccumulator` 是 openai-go SDK 的工具，它将流式 chunk 累积成完整响应，用于获取 Usage 统计和 final tool calls。

### Delta 处理

每个 chunk 的 `Choices[0].Delta` 包含四种信息：

**1. 文本内容**

```go
if delta.Content != "" {
    chunkOut.Content = delta.Content
}
```

**2. 推理内容**

openai-go SDK 的 Delta 结构体不直接包含 `reasoning_content` 字段（这是 DeepSeek 等 provider 的扩展字段）。适配器从 `delta.RawJSON()` 中手动解开：

```go
var extra deltaExtraFields
if err := json.Unmarshal([]byte(delta.RawJSON()), &extra); err == nil {
    if extra.ReasoningContent != "" {
        chunkOut.ReasoningContent = extra.ReasoningContent
    }
}
```

其中 `deltaExtraFields` 定义为：

```go
type deltaExtraFields struct {
    ReasoningContent string `json:"reasoning_content"`
}
```

**3. 工具调用增量**

工具调用通过 `Index` 累积：

```go
if len(delta.ToolCalls) > 0 {
    for _, tc := range delta.ToolCalls {
        idx := int(tc.Index)
        delta := ToolCallDelta{
            Index:     idx,
            ID:        tc.ID,
            Name:      tc.Function.Name,
            Arguments: tc.Function.Arguments,
        }

        if existing, ok := toolCallMap[idx]; ok {
            // 已有此索引的累积器，追加
            if tc.ID != "" { existing.ID = tc.ID }
            if tc.Function.Name != "" { existing.Name = tc.Function.Name }
            if tc.Function.Arguments != "" { existing.Arguments += tc.Function.Arguments }
            // 转发增量（只发Arguments部分，让调用方能看到实时数据）
            chunkOut.ToolCalls = append(chunkOut.ToolCalls, ToolCallDelta{
                Index: idx, ID: existing.ID, Name: existing.Name,
                Arguments: tc.Function.Arguments,
            })
        } else {
            // 新索引，创建累积器
            toolCallMap[idx] = &toolCallAccum{...}
            chunkOut.ToolCalls = append(chunkOut.ToolCalls, delta)
        }
    }
}
```

**4. 单 chunk 完成工具调用**

某些模型可能在一个 chunk 内完整地给出工具调用（出现在 `Accumulator.JustFinishedToolCall()` 中）。适配器会检测这些已完成调用并补发给调用方：

```go
if finished, ok := acc.JustFinishedToolCall(); ok {
    // 查重：避免重复已经通过增量方式累积的工具调用
    found := false
    for _, existing := range toolCallMap {
        if existing.ID == finished.ID { found = true; break }
    }
    if !found {
        // 作为完整 block 发送
        ...
    }
}
```

### 最终块

流结束时发送一个包含 Usage 统计的最终 chunk：

```go
final := StreamChunk{
    Usage: &Usage{
        PromptTokens:     acc.Usage.PromptTokens,
        CompletionTokens: acc.Usage.CompletionTokens,
        TotalTokens:      acc.Usage.TotalTokens,
    },
    FinishReason: acc.Choices[0].FinishReason,
}
ch <- final
```

## 使用示例

```go
package main

import (
    "context"
    "log"

    openaipkg "github.com/openai/openai-go/v3"
    "github.com/openai/openai-go/v3/option"

    "github.com/copcon/server/internal/llm"
)

func main() {
    client := openaipkg.NewClient(option.WithAPIKey("sk-..."))
    adapter := llm.NewOpenAIAdapter(client, "gpt-4o")

    params := llm.StreamParams{
        Model: "gpt-4o",
        Messages: []llm.Message{
            {Role: llm.RoleSystem, Content: "你是一个有帮助的助手。"},
            {Role: llm.RoleUser, Content: "写一首关于编程的五言绝句。"},
        },
        Temperature: 0.7,
        MaxTokens:   200,
    }

    ch, errc := adapter.Stream(context.Background(), params)

    var fullText string
    for chunk := range ch {
        fullText += chunk.Content
        if chunk.Usage != nil {
            log.Printf("消耗 token: %d", chunk.Usage.TotalTokens)
        }
    }

    if err := <-errc; err != nil {
        log.Fatal(err)
    }

    log.Printf("完整响应: %s", fullText)
}
```

带工具调用的示例：

```go
params := llm.StreamParams{
    Model: "gpt-4o",
    Messages: []llm.Message{
        {Role: llm.RoleUser, Content: "北京今天的天气如何？"},
    },
    Tools: []llm.ToolDef{
        {
            Name:        "get_weather",
            Description: "获取指定城市的天气信息",
            Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
        },
    },
}

ch, errc := adapter.Stream(context.Background(), params)

toolAcc := make(map[int]*toolCallAccum)
for chunk := range ch {
    // 处理文本
    fmt.Print(chunk.Content)
    // 累积工具调用
    for _, delta := range chunk.ToolCalls {
        acc, ok := toolAcc[delta.Index]
        if !ok {
            acc = &toolCallAccum{}
            toolAcc[delta.Index] = acc
        }
        if delta.ID != "" { acc.ID = delta.ID }
        acc.Name += delta.Name
        acc.Arguments += delta.Arguments
    }
}

// 流结束后得到完整工具调用
for _, acc := range toolAcc {
    fmt.Printf("\n工具调用: %s(%s)\n", acc.Name, acc.Arguments)
}
```