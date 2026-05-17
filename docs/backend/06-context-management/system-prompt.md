# 系统提示词

系统提示词（System Prompt）是 CopCon Agent 的行为锚点。它定义了 Agent 的身份、能力和行为准则，在每次 LLM 调用时作为第一条消息注入上下文窗口。

## 提示词来源

系统提示词的来源是 `AgentDefinition.SystemPrompt` 字段，该字段在 Agent 注册时从 `config.yaml` 中读取：

```yaml
agents:
  - id: "code-assistant"
    name: "Code Assistant"
    system_prompt: "You are a helpful coding assistant..."
```

当 `NewAgentRegistry` 创建 Agent 定义时，提示词被直接写入 `AgentDefinition`：

```go
agent := AgentDefinition{
    ID:           agentConfig.ID,
    Name:         agentConfig.Name,
    SystemPrompt: agentConfig.SystemPrompt,
    // ...
}
```

## 提示词解析流程

一次 LLM 调用的完整提示词处理流程：

```
1. 引擎从 AgentRegistry 解析 AgentDefinition
   └─ 获取 agent.SystemPrompt (原始文本)

2. 引擎触发 OnSystemPrompt hook 点
   └─ HookContext.SystemPrompt 指向提示词指针

3. 所有注册到 OnSystemPrompt 的 hook 依次执行
   └─ 每个 hook 可以修改 *ctx.SystemPrompt

4. 处理后的 systemPrompt 传入 ContextBuilder.Build()
   └─ Build 将其作为 role="system" 消息放在序列首位

5. 引擎触发 BeforeLLMCall hook 点
   └─ 此时 Messages 已包含最终的 system 消息

6. 消息序列发送给 LLM
```

## OnSystemPrompt Hook 点

`OnSystemPrompt` hook 点的 `HookContext` 中填充的关键字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `SystemPrompt` | `*string` | 指向当前解析出的系统提示词。Hook 可直接修改 `*ctx.SystemPrompt` 来替换或追加内容 |
| `ChatCtx` | `ChatContextInterface` | 会话上下文，可获取 SessionID 等信息 |
| `SessionID` | `string` | 当前会话 ID |
| `AgentID` | `string` | 当前 Agent ID |

Hook 返回 `nil` 表示成功，即使 `SystemPrompt` 为 nil 也不影响管道继续执行。返回非 nil 错误会被 HookRunner 记录日志但不中断管道。

### 典型用例：TodoInjectionHook

`TodoInjectionHook` 在 `OnSystemPrompt` 点将当前会话的 todo 列表追加到系统提示词末尾：

```go
func (h *TodoInjectionHook) Execute(ctx *hook.HookContext) error {
    if ctx.SystemPrompt == nil {
        return nil  // 无提示词，跳过
    }
    todos, err := h.todoMgr.List(ctx.ChatCtx)
    if err != nil {
        h.logger.Warn("failed to fetch todos", "session_id", ctx.SessionID, "error", err)
        return nil  // 获取失败不阻塞管道
    }
    if len(todos) > 0 {
        *ctx.SystemPrompt = *ctx.SystemPrompt + "\n\n" + formatTodoState(todos)
    }
    return nil
}
```

## 默认提示词格式

默认提示词没有强制的格式要求。从 `config.yaml` 示例来看，推荐使用英文、清晰的角色定义和工具可用性说明：

```yaml
system_prompt: "You are a helpful coding assistant. You can write, analyze, and debug code. You have access to code execution, shell commands, and file operations. Always provide clear explanations and best practices."
```

提示词的最终内容由 `OnSystemPrompt` hook 链决定，这意味着插件可以动态追加上下文（todo 列表、向量记忆等），与基础提示词拼接为最终文本。

## 多 Agent 提示词路由

多 Agent 场景下，每个 Agent 拥有独立的 `SystemPrompt`。路由逻辑在引擎的 `Chat` 方法中执行：

1. 通过 `chatCtx.AgentID()` 获取请求指定的 Agent ID
2. 若为空，从当前 Session 的 `DefaultAgentID` 回退
3. 若仍为空，使用 `AgentRegistry.Default()`
4. 从 `AgentRegistry.Get(id)` 取得 `AgentDefinition`
5. 提取 `definition.SystemPrompt` 作为当前请求的提示词

这个流程确保不同 Agent 使用各自的提示词，且 `OnSystemPrompt` hook 对所有 Agent 的提示词生效（例如 todo 列表对 code-assistant 和 chat-assistant 都会注入）。