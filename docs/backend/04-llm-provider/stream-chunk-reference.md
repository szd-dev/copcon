# StreamChunk 参考手册

`StreamChunk` 是 CopCon 流式架构的核心数据类型。每次 LLM 返回的增量信息都封装在 `StreamChunk` 中，由调用方逐块消费和聚合。

## 结构定义

```go
type StreamChunk struct {
    Content          string          `json:"content,omitempty"`
    ReasoningContent string          `json:"reasoning_content,omitempty"`
    ToolCalls        []ToolCallDelta `json:"tool_calls,omitempty"`
    Usage            *Usage          `json:"usage,omitempty"`
    FinishReason     string          `json:"finish_reason,omitempty"`
}
```

## Content — 文本增量

**类型**：`string`

**含义**：模型输出的主文本内容的增量片段。调用方应将所有 chunk 的 Content 拼接起来获取完整文本。

**示例**：

| Chunk | Content | 累积文本 |
|-------|---------|---------|
| 1 | `"今天"` | `"今天"` |
| 2 | `"北京的"` | `"今天北京的"` |
| 3 | `"天气很好。"` | `"今天北京的天气很好。"` |

**空值场景**：
- 模型正在进行纯工具调用（无文字输出）
- 流的最末尾 Usage chunk
- 某些 chunk 仅包含 ReasoningContent

## ReasoningContent — 推理/思维链增量

**类型**：`string`

**含义**：模型内部的推理过程/思维链。这是部分模型（DeepSeek-R1、o1 系列）的专有功能。Provider 通过 SDK 的扩展字段提取。

**示例**（DeepSeek-R1）：

| Chunk | ReasoningContent | 说明 |
|-------|-----------------|------|
| 1 | `"我需要"` | 推理开始 |
| 2 | `"先分析"` | 继续推理 |
| 3 | `"用户的需求..."` | 推理结束 |

**不支持此功能的 provider（如标准 GPT-4o）总是返回空字符串。**

提取方式（以 OpenAI Adapter 为例）：

```go
// Delta 结构体不含此字段，从 RawJSON 中解包
var extra struct {
    ReasoningContent string `json:"reasoning_content"`
}
json.Unmarshal([]byte(delta.RawJSON()), &extra)
chunkOut.ReasoningContent = extra.ReasoningContent
```

## ToolCalls — 工具调用增量

**类型**：`[]ToolCallDelta`

**含义**：当前 chunk 中的工具调用增量。每次可能包含零个、一个或多个 `ToolCallDelta`。

```go
type ToolCallDelta struct {
    Index     int    `json:"index"`
    ID        string `json:"id,omitempty"`
    Name      string `json:"name,omitempty"`
    Arguments string `json:"arguments,omitempty"`
}
```

### 增量累积模式

模型流式返回工具调用时，无法在一个 chunk 中给出完整信息。调用方需要**按 Index 累积**：

```
时间轴：Chunk#1 ─── Chunk#2 ─── Chunk#3 ─── Chunk#4
         │            │            │            │
Tool#0:  Name=get_    Name=get_   Args={"ci    Args="ty":
         weather      weather     ty":"Bei    "Bei
                                  jing        jing"}
```

累积代码：

```go
accMap := make(map[int]*ToolCallAccum)

for chunk := range ch {
    for _, delta := range chunk.ToolCalls {
        acc, ok := accMap[delta.Index]
        if !ok {
            acc = &ToolCallAccum{Index: delta.Index}
            accMap[delta.Index] = acc
        }
        if delta.ID != "" {
            acc.ID = delta.ID
        }
        acc.Name += delta.Name       // 拼接 Name
        acc.Arguments += delta.Arguments  // 拼接 Arguments
    }
}
```

### 为什么 ToolCallDelta 有 Index 字段

模型能在一个响应中发起**多个并行工具调用**。每个调用被分配一个从 0 开始的索引。流式传输时，多个调用的增量交错到达：

```
Chunk#1: [Index=0, Name="get_weather"]  [Index=1, Name="get_time"]
Chunk#2: [Index=0, Args="{\"city\":"}] [Index=1, Args="{\"tz\":"}]
Chunk#3: [Index=0, Args="\"Beijing\"}] [Index=1, Args="\"UTC+8\"}]
```

没有 Index，调用方无法区分哪个参数片段属于哪个工具调用。

### ID 的可能到达时机

`ID` 字段可能在 Name 和 Arguments **之后**才出现。例如 OpenAI 的流式响应中，工具调用 ID 不一定在第一个 chunk 中给出：

```go
// 安全的累积方式：不假设 ID 与 Name 同时到达
if delta.ID != "" {
    acc.ID = delta.ID
}
```

## Usage — Token 统计

**类型**：`*Usage`

**含义**：本次请求的 token 使用量。**只在流的最后一个 chunk 中非 nil**，中间 chunk 的 Usage 均为 nil。

```go
type Usage struct {
    PromptTokens     int64 `json:"prompt_tokens"`
    CompletionTokens int64 `json:"completion_tokens"`
    TotalTokens      int64 `json:"total_tokens"`
}
```

**示例值**：

```json
{
    "prompt_tokens": 150,
    "completion_tokens": 82,
    "total_tokens": 232
}
```

**消费模式**：

```go
for chunk := range ch {
    // 中间 chunk 处理
    fmt.Print(chunk.Content)

    if chunk.Usage != nil {
        log.Printf("Token: input=%d output=%d total=%d",
            chunk.Usage.PromptTokens,
            chunk.Usage.CompletionTokens,
            chunk.Usage.TotalTokens,
        )
    }
}
```

## FinishReason — 停止原因

**类型**：`string`

**含义**：模型停止生成的原因。**中间 chunk 为空**,仅在最后一个（或倒数第二个）chunk 中出现。

### 常见值及含义

| 值 | 含义 | 说明 |
|----|------|------|
| `"stop"` | 自然结束 | 模型正常完成回复 |
| `"length"` | 达到长度限制 | 达到 `MaxTokens` 限制，回复被截断 |
| `"tool_calls"` | 工具调用 | 模型决定调用工具，等待用户提交工具结果 |
| `"content_filter"` | 内容过滤 | 被模型的安全过滤机制拦截 |
| `"function_call"` | 函数调用 | 旧版函数调用，等同于 `tool_calls` |

**示例流结尾**：

```
Chunk#N-1: Content="。", ToolCalls=nil, Usage=nil, FinishReason=""
Chunk#N:   Content="",   ToolCalls=nil, Usage={...},  FinishReason="stop"
```

**OpenAI Adapter 中 FinishReason 出现的两个时机**：

1. 流中间：当模型决定调用工具时，OpenAI 会发送 `FinishReason="tool_calls"` 的 chunk
2. 流末尾：`Accumulator` 汇总后，最终 chunk 的 `FinishReason` 表示整条消息的停止原因

## 完整流示例

假设用户问"北京今天天气怎么样？"，模型调用 `get_weather` 工具：

```
Chunk#1: {Content: "",        ReasoningContent: "用户想知道", ToolCalls: nil,               Usage: nil, FinishReason: ""}
Chunk#2: {Content: "",        ReasoningContent: "北京天气...",  ToolCalls: nil,               Usage: nil, FinishReason: ""}
Chunk#3: {Content: "好的，",   ReasoningContent: "",           ToolCalls: nil,               Usage: nil, FinishReason: ""}
Chunk#4: {Content: "让我查一下", ReasoningContent: "",          ToolCalls: nil,               Usage: nil, FinishReason: ""}
Chunk#5: {Content: "",        ReasoningContent: "",           ToolCalls: [{Index:0, ...}],  Usage: nil, FinishReason: ""}
Chunk#6: {Content: "",        ReasoningContent: "",           ToolCalls: [{Index:0, ...}],  Usage: nil, FinishReason: ""}
Chunk#7: {Content: "",        ReasoningContent: "",           ToolCalls: [{Index:0, ...}],  Usage: nil, FinishReason: "tool_calls"}
Chunk#8: {Content: "",        ReasoningContent: "",           ToolCalls: nil,               Usage: {150,82,232},  FinishReason: "tool_calls"}
```

## 消费方参考实现

CopCon Agent Engine 中的 `handleStreaming` 方法展示了完整的消费模式：

```go
func (e *engineImpl) handleStreaming(
    chatCtx iface.ChatContextInterface,
    ch <-chan llm.StreamChunk,
    errc <-chan error,
) (*StreamResult, error) {
    result := &StreamResult{}
    toolAcc := make(map[int]*toolCallAccum)

    for chunk := range ch {
        // 1. 累积文本
        result.Content += chunk.Content
        result.ReasoningContent += chunk.ReasoningContent

        // 2. 实时发送文本事件
        if chunk.Content != "" {
            chatCtx.Emit(entity.Event{
                Type: entity.EventMessageDelta,
                Data: entity.MessageDeltaData{Content: chunk.Content},
            })
        }
        if chunk.ReasoningContent != "" {
            chatCtx.Emit(entity.Event{
                Type: entity.EventMessageDelta,
                Data: entity.MessageDeltaData{ReasoningContent: chunk.ReasoningContent},
            })
        }

        // 3. 累积工具调用
        for _, delta := range chunk.ToolCalls {
            acc, ok := toolAcc[delta.Index]
            if !ok {
                acc = &toolCallAccum{Index: delta.Index}
                toolAcc[delta.Index] = acc
            }
            if delta.ID != "" { acc.ID = delta.ID }
            acc.Name += delta.Name
            acc.Arguments += delta.Arguments
        }

        // 4. 记录 Usage（最后 chunk 才有）
        if chunk.Usage != nil {
            result.Usage = chunk.Usage
        }
    }

    // 5. 流结束后检查错误
    select {
    case err := <-errc:
        return nil, err
    default:
    }

    // 6. 整理工具调用
    for _, acc := range toolAcc {
        result.ToolCalls = append(result.ToolCalls, toolCallInfo{
            ID:        acc.ID,
            Name:      acc.Name,
            Arguments: acc.Arguments,
        })
    }

    return result, nil
}
```