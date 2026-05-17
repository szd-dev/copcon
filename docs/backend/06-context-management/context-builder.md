# ContextBuilder

`ContextBuilder` 是 CopCon 上下文管理系统的核心组件，负责将 UI 层的富消息结构组装为 LLM 可以理解的消息序列。它是一个纯函数组件，不涉及任何持久化或副作用操作。

## 接口定义

```go
type ContextBuilder interface {
    Build(ctx context.Context, messages []entity.UIMessage, systemPrompt string, userInput string) ([]entity.MessageForLLM, error)
}
```

四个入参：

| 参数 | 类型 | 说明 |
|------|------|------|
| `ctx` | `context.Context` | 标准上下文（当前构建过程不使用，保留以备将来扩展） |
| `messages` | `[]entity.UIMessage` | 会话历史消息，经过 `SynthesizeUIMessage` 从数据库转换而来 |
| `systemPrompt` | `string` | 系统提示词，已通过 `OnSystemPrompt` hook 点处理完毕 |
| `userInput` | `string` | 用户当前轮次的输入文本 |

返回值 `[]entity.MessageForLLM` 是引擎最终传给 LLM Provider 的消息序列。

## 纯函数设计

`ContextBuilder` 的实现不持有任何状态，不访问数据库，不产生副作用。所有输入通过参数显式传入，所有输出通过返回值给出。这个设计带来三个好处：

1. **可测试性**：任意构造输入即可验证输出，无需 mock 数据库
2. **确定性**：相同输入始终产生相同输出
3. **无锁并发**：任意 goroutine 可安全并发调用

当前实现 `builder` 是一个空结构体：

```go
type builder struct{}

func New() ContextBuilder {
    return &builder{}
}
```

## 消息组装顺序

`Build` 方法按固定三段式结构组装消息：

```
System Prompt → 历史消息 → 当前用户输入
```

### 第一步：注入 System Prompt

如果 `systemPrompt` 非空，将其包装为 `role="system"` 的 `MessageForLLM` 并作为序列首条消息。

### 第二步：转换历史消息

调用 `entity.ConvertToModelMessages(uiMessages)` 将 UIMessage 切片转换为 ModelMessage 序列，然后将每个 ModelMessage 逐一复制到 `MessageForLLM`：

```go
for _, mm := range modelMessages {
    messages = append(messages, entity.MessageForLLM{
        Role:       mm.Role,
        Content:    mm.Content,
        ToolCallID: mm.ToolCallID,
        ToolCalls:  mm.ToolCalls,
    })
}
```

转换过程中，`reasoning` 和 `step-start` 类型的 Part 会被丢弃，因为它们仅供 UI 展示，不应发送给 LLM。

### 第三步：追加当前用户输入

如果 `userInput` 非空，将其包装为 `role="user"` 的消息追加到序列末尾。

## UIMessage → ModelMessage 转换管道

整个转换链路由 `ConvertToModelMessages` 完成（位于 `internal/domain/entity/convert.go`），主要规则：

1. **User 消息**：所有 `UIPartText` 的 `Text` 字段拼接为一个 `content`
2. **Assistant 消息**：
   - 文本 Parts 拼接为 `Content`
   - 工具调用 Parts 生成 `ToolCalls` 数组
   - 每个工具调用额外生成一条独立的 `role="tool"` 消息（`Content=Output`, `ToolCallID=ToolCallID`）
3. `reasoning` 和 `step-start` Part 在转换中丢弃

## SynthesizeUIMessage：旧格式兼容

历史消息存储在 `session.Message` 结构中（包含扁平的 `Content`, `Reasoning`, `ToolCalls` 字段）。`SynthesizeUIMessage` 函数负责将这些旧格式消息转换为新的 Steps → Parts 层级结构：

```go
func SynthesizeUIMessage(msg session.Message, toolResultByCallID map[string]string) *entity.UIMessage
```

对于 `role="assistant"` 的消息：
- 如有 `Reasoning`，生成 `UIPartReasoning` Part
- 如有 `Content`，生成 `UIPartText` Part
- 每个 `ToolCall` 通过 `toolResultByCallID` 查找对应输出，生成 `UIPartToolCall` Part
- 所有 Part 的 `StepIndex` 设为 0，通过 `GroupPartsByStep` 归入单个 Step

对于不支持的 role，返回 `nil`。

## AfterContextBuild Hook 点

`AfterContextBuild` 是 `ContextBuilder.Build()` 之后触发的 hook 点。此时 `*HookContext.Messages` 指向已组装完毕的消息切片，插件可以在发送给 LLM 之前检查或修改这些消息。

典型用途是 `MemoryPlugin`：在 `AfterContextBuild` 中搜索向量数据库，将相关记忆作为 `role="system"` 的消息插入到消息序列的最前面。