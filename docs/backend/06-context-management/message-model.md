# 消息模型

CopCon 的消息系统分为三个层次：前端的 `UIMessage` 展示层、中介的 `ModelMessage` 转换层、以及最终传给 LLM 的 `MessageForLLM` 提交层。每层服务于不同的消费者，通过清晰的转换管道衔接。

## UIMessage：前端展示层

`UIMessage` 是面向 UI 渲染的富消息结构，定义在 `internal/domain/entity/ui_message.go`。它采用 Steps → Parts 的层级模型来描述消息内容。

```go
type UIMessage struct {
    ID        string     `json:"id"`
    SessionID string     `json:"session_id"`
    Role      string     `json:"role"`    // "user" 或 "assistant"
    Steps     []UIStep   `json:"steps"`
    Parts     []UIPart   `json:"parts"`   // 已弃用，保留兼容
    Metadata  UIMetadata `json:"metadata"`
}
```

### Steps → Parts 层级

每条消息按执行步骤组织为多个 `UIStep`，每个步骤包含多个 `UIPart`。这个设计支持一个 assistant 消息内包含"思考→文本回复→工具调用→工具结果"的完整流程：

```
UIMessage
  └─ Step 0 (state: done)
       ├─ UIPart { type: "reasoning", text: "我需要先查看文件..." }
       ├─ UIPart { type: "tool-call", toolName: "read_file", args: "..." }
       ├─ UIPart { type: "text",     text: "文件内容如下..." }
       └─ UIPart { type: "step-start" }
```

### UIPart 类型

| 类型常量 | 含义 | 使用字段 |
|---------|------|---------|
| `UIPartText` | 文本内容 | `Text` |
| `UIPartReasoning` | 推理/思考过程 | `Text` |
| `UIPartToolCall` | 工具调用（含输入输出） | `ToolName`, `Args`, `Output` |
| `UIPartStepStart` | 步骤开始标记 | 无 |

### UIPartState：流式状态

每个 Part 携带状态信息，支持实时流式渲染：

| 状态 | 含义 |
|------|------|
| `pending` | 等待开始 |
| `streaming` | 正在流式传输 |
| `running` | 正在执行 |
| `done` | 传输完成 |
| `complete` | 执行完成 |
| `error` | 执行出错 |

## ModelMessage：OpenAI 兼容格式

`ModelMessage` 定义在 `internal/domain/entity/model_message.go`，是 OpenAI Chat Completion API 消息格式的 Go 映射。它解耦了内部逻辑与 go-openai SDK。

```go
type ModelMessage struct {
    Role       string          `json:"role"`
    Content    string          `json:"content,omitempty"`
    ToolCalls  []ModelToolCall `json:"tool_calls,omitempty"`
    ToolCallID string          `json:"tool_call_id,omitempty"`
    Name       string          `json:"name,omitempty"`
}
```

### Role 取值

| Role | 含义 | 必填字段 |
|------|------|---------|
| `system` | 系统指令 | `Content` |
| `user` | 用户输入 | `Content` |
| `assistant` | 模型回复（可能含工具调用） | `Content` 或 `ToolCalls` |
| `tool` | 工具执行结果 | `ToolCallID`, `Content` |

### ModelToolCall 结构

```go
type ModelToolCall struct {
    ID       string            `json:"id,omitempty"`
    Type     string            `json:"type,omitempty"`   // 默认 "function"
    Function ModelFunctionCall `json:"function"`
}

type ModelFunctionCall struct {
    Name      string `json:"name"`
    Arguments string `json:"arguments"`
}
```

## 转换管道：PersistedParts → UIMessage → ModelMessage

消息从数据库到 LLM 的完整转换流程：

```
数据库 (PersistedParts)
    │
    ▼ SynthesizeUIMessage() — 从旧格式合成 UIMessage
UIMessage (Steps → Parts)
    │
    ▼ ConvertToModelMessages() — entity/convert.go
ModelMessage (OpenAI 格式)
    │
    ▼ ContextBuilder.Build() — 拼装最终序列
MessageForLLM (提交给 LLM)
```

### ConvertToModelMessages 转换规则

`internal/domain/entity/convert.go` 中的 `ConvertToModelMessages` 函数执行以下转换：

1. **User 消息**：提取所有 `UIPartText` 的 `Text` 字段并拼接，生成 `role="user"` 的 ModelMessage。

2. **Assistant 消息**（关键规则）：
   - 文本 Parts 拼接为 `Content`
   - 工具调用 Parts 生成 `ModelToolCall` 条目挂载到 assistant 消息上
   - 每个工具调用同时生成一个独立的 `role="tool"` ModelMessage（`Content=Output`, `ToolCallID=ToolCallID`, `Name=ToolName`）
   - `reasoning` 和 `step-start` 类型的 Part 在转换时丢弃（仅 UI 使用）

3. **输出序列格式**：
   ```
   [assistant (content + tool_calls), tool (result 1), tool (result 2), ...]
   ```

### collectParts 兼容逻辑

转换优先使用 `Steps[].Parts`，若 Steps 为空则回退到顶层的 `Parts` 切片，保证向后兼容。

## MessageForLLM：最终提交格式

`MessageForLLM` 定义在 `internal/domain/entity/message_for_llm.go`，是将 ModelMessage 展平后的最终格式：

```go
type MessageForLLM struct {
    Role       string          `json:"role"`
    Content    string          `json:"content"`
    Reasoning  string          `json:"reasoning,omitempty"`
    ToolCallID string          `json:"tool_call_id,omitempty"`
    ToolCalls  []ModelToolCall `json:"tool_calls,omitempty"`
}
```

与 `ModelMessage` 的区别在于 `MessageForLLM` 额外保留了 `Reasoning` 字段，且结构更扁平。它由 `ContextBuilder.Build()` 直接输出，是引擎传给 LLM Provider 的最终消息序列。