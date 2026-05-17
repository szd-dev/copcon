# 消息架构设计说明书

## 设计目标

1. **单一事件层级**：SSE 协议只传输 UI 层事件，不传输模型层事件
2. **类型安全**：所有跨层数据的字段类型和命名完全确定，无 `any`/`as` 断言
3. **两条路径一致**：流式路径和刷新路径产出完全相同形状的数据
4. **迭代安全**：Agent Loop 多次迭代的 parts 不会互相冲突
5. **持久化完整**：刷新后 UI 状态与流式结束时完全一致

---

## 一、数据模型

### 1.1 UIMessage（前后端共享的 canonical 格式）

```typescript
// 一条消息 = 一组有序的步骤
interface UIMessage {
  id: string;                          // 消息唯一 ID
  role: 'user' | 'assistant';          // 消息角色（不含 tool）
  steps: Step[];                       // 有序步骤列表
  metadata: {
    createdAt: string;                 // ISO 8601
    model?: string;
    tokenCount?: number;
    durationMs?: number;
  };
}
```

### 1.2 Step（一次 Agent Loop 迭代）

```typescript
// 一次迭代 = 一组有序的内容部件
interface Step {
  parts: Part[];                       // 有序部件列表
  status: 'streaming' | 'done';       // 整步状态
}
```

### 1.3 Part（内容部件）

```typescript
type Part = TextPart | ReasoningPart | ToolCallPart;

interface TextPart {
  type: 'text';
  text: string;                        // 累积的完整文本
  state: 'streaming' | 'done';
}

interface ReasoningPart {
  type: 'reasoning';
  text: string;                        // 累积的完整推理文本
  state: 'streaming' | 'done';
}

interface ToolCallPart {
  type: 'tool-call';
  toolCallId: string;                  // camelCase，单一命名规范
  toolName: string;
  args: string;                        // JSON string
  output: string;                      // 总是 string，后端负责序列化
  error: string;                       // 空字符串表示无错误
  state: 'pending' | 'running' | 'complete' | 'error';
}
```

**设计决策说明**：

- **steps 嵌套**：每次 Agent Loop 迭代是一个独立 Step。前端更新时以 Step 为单位，不同 Step 的 parts 索引互不干扰。
- **不含 StepStartPart**：step-start 不是一个 Part，而是 Step 的边界。它没有内容，不需要作为 Part 存在。
- **不含 role='tool' 消息**：工具结果嵌入 ToolCallPart.output，不存在独立的 tool-role 消息。
- **output 总是 string**：后端在发射 SSE 事件前将 result 对象序列化为 JSON string。前端拿到后可直接展示或 parseToolOutput。
- **字段命名统一 camelCase**：SSE 事件和 API 响应中的字段名统一为 camelCase。后端 Go struct 的 JSON tag 改为 camelCase。

---

## 二、SSE 事件协议

### 2.1 事件总表

**只保留 4 种 UI 层事件**：

| 事件类型 | 语义 | 何时触发 |
|---------|------|---------|
| `step_create` | 创建一个新步骤 | Agent Loop 每次迭代开始时 |
| `part_create` | 在当前步骤内创建一个部件 | 首次产出 reasoning/text/tool-call 时 |
| `part_update` | 更新当前步骤内的部件 | 内容追加、状态变更、output 产出时 |
| `message_done` | 整条消息流式传输结束 | Agent Loop 全部迭代完成后 |

**删除的事件**（不再发射）：

| 删除的事件 | 原因 |
|-----------|------|
| `message` | 模型层事件，由 `part_update(text_delta)` 替代 |
| `reasoning` | 模型层事件，由 `part_update(text_delta)` 替代 |
| `tool_call` | 模型层事件，由 `part_create(tool-call)` 替代 |
| `tool_result` | 模型层事件，由 `part_update(state/output)` 替代 |
| `done` | 由 `message_done` 替代（语义更明确） |
| `thought` | 从未使用 |
| `async_tool_started` | 由 `part_update(state=running)` 替代 |
| `async_tool_complete` | 由 `part_update(state=complete)` 替代 |
| `async_tool_failed` | 由 `part_update(state=error)` 替代 |

### 2.2 step_create 事件

```typescript
interface StepCreateEvent {
  type: 'step_create';
  data: {
    messageId: string;              // 所属消息 ID
    stepIndex: number;              // 步骤索引（0-based）
  };
}
```

**触发时机**：Agent Loop 每次迭代开始时（包括第一次）。

**前端处理**：
- 在 `UIMessage.steps[stepIndex]` 创建新的 `Step { parts: [], status: 'streaming' }`
- 如果 stepIndex 已存在（不应该发生），忽略

### 2.3 part_create 事件

```typescript
interface PartCreateEvent {
  type: 'part_create';
  data: {
    messageId: string;              // 所属消息 ID
    stepIndex: number;              // 所属步骤索引
    partIndex: number;              // 步骤内部件索引
  } & (
    | { partType: 'text'; state: 'streaming' }
    | { partType: 'reasoning'; state: 'streaming' }
    | { partType: 'tool-call'; toolCallId: string; toolName: string; args: string; state: 'pending' }
  );
}
```

**触发时机**：
- `partType='reasoning'`：LLM 首次产出 reasoning_content 时
- `partType='text'`：LLM 首次产出 content 时
- `partType='tool-call'`：LLM 请求调用工具时

**前端处理**：
- 在 `UIMessage.steps[stepIndex].parts[partIndex]` 插入对应 Part
- partIndex 由后端计算，保证步骤内递增

### 2.4 part_update 事件

```typescript
interface PartUpdateEvent {
  type: 'part_update';
  data: {
    messageId: string;              // 所属消息 ID
    stepIndex: number;              // 所属步骤索引
    partIndex: number;              // 步骤内部件索引
  } & (
    | { partType: 'text'; textDelta?: string; state?: 'streaming' | 'done' }
    | { partType: 'reasoning'; textDelta?: string; state?: 'streaming' | 'done' }
    | { partType: 'tool-call'; state?: 'pending' | 'running' | 'complete' | 'error'; output?: string; error?: string }
  );
}
```

**触发时机**：
- `textDelta`：LLM 产出后续 reasoning/content chunk 时
- `state`：部件状态变更时（streaming→done, pending→running→complete/error）
- `output`/`error`：工具执行结果返回时

**前端处理**：
- 找到 `UIMessage.steps[stepIndex].parts[partIndex]`
- 按类型更新：TextPart/ReasoningPart 追加 textDelta、更新 state；ToolCallPart 更新 state/output/error

### 2.5 message_done 事件

```typescript
interface MessageDoneEvent {
  type: 'message_done';
  data: {
    messageId: string;              // 消息 ID
  };
}
```

**触发时机**：Agent Loop 全部迭代完成时。

**前端处理**：
- 将所有 steps 的 status 从 'streaming' 改为 'done'
- 将所有 streaming 状态的 parts 的 state 改为 'done'
- 将所有 pending/running 状态的 tool-call parts 的 state 改为 'complete'（防御性处理）

---

## 三、事件发射时序

### 3.1 完整 Agent Loop 的事件序列

```
用户发送消息
  │
  ▼
step_create { messageId, stepIndex: 0 }            ← 第1次迭代开始
  │
  ├─ [LLM reasoning chunk 1]
  │   part_create { stepIndex:0, partIndex:0, partType:'reasoning', state:'streaming' }
  │   part_update { stepIndex:0, partIndex:0, partType:'reasoning', textDelta:'用户' }
  │
  ├─ [LLM reasoning chunk 2]
  │   part_update { stepIndex:0, partIndex:0, partType:'reasoning', textDelta:'说' }
  │
  ├─ ...更多 reasoning chunks...
  │
  ├─ part_update { stepIndex:0, partIndex:0, partType:'reasoning', state:'done' }
  │
  ├─ [LLM content chunk 1]
  │   part_create { stepIndex:0, partIndex:1, partType:'text', state:'streaming' }
  │   part_update { stepIndex:0, partIndex:1, partType:'text', textDelta:'好的' }
  │
  ├─ ...更多 text chunks...
  │
  ├─ part_update { stepIndex:0, partIndex:1, partType:'text', state:'done' }
  │
  ├─ [LLM tool call]
  │   part_create { stepIndex:0, partIndex:2, partType:'tool-call', toolCallId, toolName, args, state:'pending' }
  │   part_update { stepIndex:0, partIndex:2, partType:'tool-call', state:'running' }
  │
  ├─ [工具执行完成]
  │   part_update { stepIndex:0, partIndex:2, partType:'tool-call', state:'complete', output:'{...}' }
  │
  ▼
step_create { messageId, stepIndex: 1 }            ← 第2次迭代开始
  │
  ├─ [LLM reasoning chunk 1]
  │   part_create { stepIndex:1, partIndex:0, partType:'reasoning', state:'streaming' }
  │   part_update { stepIndex:1, partIndex:0, partType:'reasoning', textDelta:'结果' }
  │
  ├─ ...更多 reasoning/text chunks...
  │
  ├─ part_update { stepIndex:1, partIndex:N, partType:'text', state:'done' }
  │
  ▼
message_done { messageId }                          ← 全部完成
```

### 3.2 与当前系统的关键差异

| 方面 | 当前系统 | 新设计 |
|------|---------|--------|
| 每个LLM chunk发射的事件数 | 2个（legacy + part） | 1个（仅 part_update） |
| 索引体系 | 全局 partIndex，迭代间重置 | stepIndex + partIndex，二级索引 |
| 迭代边界 | step-start（无语义） | step_create（明确创建步骤） |
| 工具调用 | tool_call + part_create 双重发射 | 仅 part_create |
| 工具结果 | tool_result(object) 导致 crash | part_update.output（string，后端序列化） |

---

## 四、后端转换逻辑

### 4.1 模型消息 → SSE 事件（handleStreaming 内部）

**当前**：handleStreaming 直接发射 legacy + part 两套事件
**新设计**：handleStreaming 只调用统一的"发射UI事件"函数

```
LLM delta.content 首次:
  → ensureStep(stepIndex)
  → emit part_create { stepIndex, partIndex:nextInStep, type:'text', state:'streaming' }
  → emit part_update { stepIndex, partIndex, textDelta:content }

LLM delta.content 后续:
  → emit part_update { stepIndex, partIndex, textDelta:content }

LLM delta.reasoning 首次:
  → ensureStep(stepIndex)
  → emit part_create { stepIndex, partIndex:nextInStep, type:'reasoning', state:'streaming' }
  → emit part_update { stepIndex, partIndex, textDelta:reasoning }

LLM delta.reasoning 后续:
  → emit part_update { stepIndex, partIndex, textDelta:reasoning }

流式结束:
  → emit part_update { state:'done' } for each streaming part
```

### 4.2 工具调用 → SSE 事件（handleToolCalls 内部）

```
LLM 请求 tool call:
  → emit part_create { stepIndex, partIndex, type:'tool-call', toolCallId, toolName, args, state:'pending' }
  → emit part_update { stepIndex, partIndex, state:'running' }

工具执行完成:
  → outputJSON = json.Marshal(result)  // 总是序列化为 string
  → emit part_update { stepIndex, partIndex, state:'complete', output:outputJSON }

工具执行失败:
  → emit part_update { stepIndex, partIndex, state:'error', error:errMsg }
```

### 4.3 SSE 事件 → 持久化（persistMessage）

每次迭代结束时，将当前 step 的 parts 完整持久化：

```go
// 持久化的 Parts JSONB 格式（扁平化，不含 step 容器）
type PersistedPart struct {
    Type       string `json:"type"`                  // "text" | "reasoning" | "tool-call"
    Text       string `json:"text,omitempty"`        // text/reasoning 的完整内容
    State      string `json:"state"`                 // 当前状态
    ToolCallID string `json:"toolCallId,omitempty"`  // camelCase!
    ToolName   string `json:"toolName,omitempty"`    // camelCase!
    Args       string `json:"args,omitempty"`
    Output     string `json:"output,omitempty"`      // 总是 string
    Error      string `json:"error,omitempty"`
    StepIndex  int    `json:"stepIndex"`             // 所属步骤索引
}
```

**关键变更**：
1. JSON tag 从 snake_case 改为 camelCase，与前端 TypeScript 一致
2. tool-call 的 state 在持久化时为最终状态（"complete"/"error"），不是 "pending"
3. tool-call 的 output 在持久化时包含序列化后的结果
4. StepIndex 字段让扁平化数据可以重建步骤结构
5. **不再需要单独的 role=tool 消息**：工具结果直接存在 ToolCallPart.output 中

### 4.4 持久化 → API 响应（GetMessages）

```typescript
// API 响应格式
interface GetMessagesResponse {
  messages: APIMessage[];
}

interface APIMessage {
  id: string;
  sessionId: string;
  role: 'user' | 'assistant';       // 不含 tool
  steps: Step[];                     // 直接返回 steps 结构
  metadata: {
    createdAt: string;
    model?: string;
    tokenCount?: number;
    durationMs?: number;
  };
}
```

**转换逻辑**：
1. 从 DB 读取 Message 记录
2. 从 Parts JSONB 重建 steps：按 stepIndex 分组
3. 返回 UIMessage 格式（与流式路径最终状态一致）
4. **不再返回** content、reasoning、tool_calls、tool_call_id 等 legacy 字段

### 4.5 持久化 → LLM 上下文（BuildContext）

```typescript
// 给 LLM 的消息格式（保持 OpenAI API 格式）
interface MessageForLLM {
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  toolCallId?: string;
  toolCalls?: ToolCallForLLM[];
}
```

**转换逻辑**（从 steps 展平为 OpenAI 格式）：
```
UIMessage { role:'user', steps:[{parts:[{type:'text', text:'你好'}]}] }
  → MessageForLLM { role:'user', content:'你好' }

UIMessage { role:'assistant', steps:[
    { parts:[
        {type:'reasoning', text:'让我想想'},   ← 丢弃（UI-only）
        {type:'text', text:'好的'},
        {type:'tool-call', toolCallId, toolName, args, output},
    ]},
    { parts:[
        {type:'text', text:'完成了'},
    ]},
  ]}
  → MessageForLLM { role:'assistant', content:'好的完成了', toolCalls:[{id, name, args}] }
  → MessageForLLM { role:'tool', content:output, toolCallId }
```

**注意**：多个 step 的 text parts 需要拼接为一个 content。这是因为 OpenAI API 的 assistant 消息只有一个 content 字段。

---

## 五、前端处理逻辑

### 5.1 SSE 流式处理

**CopConChatProvider.transformMessage()** 处理每个 SSE 事件：

```
收到 step_create:
  → UIMessage.steps[stepIndex] = { parts: [], status: 'streaming' }

收到 part_create:
  → UIMessage.steps[stepIndex].parts[partIndex] = newPart

收到 part_update:
  → 找到 steps[stepIndex].parts[partIndex]
  → 追加 textDelta / 更新 state / 更新 output / 更新 error

收到 message_done:
  → 所有 steps status → 'done'
  → 所有 streaming parts state → 'done'
  → 所有 pending/running tool-call parts state → 'complete'
```

**不再需要处理**：message, reasoning, tool_call, tool_result, done。

### 5.2 历史加载处理

```typescript
// useAgentChat.loadMessages()
const result = await client.getMessages(sessionId);
// result.messages 已经是 UIMessage[] 格式，包含 steps
// 无需转换，无需 mergeToolMessages
const messageInfos = result.messages.map(msg => ({
  id: msg.id,
  message: msg,
  status: 'success',
}));
chatResult.setMessages(messageInfos);
```

**不再需要**：
- `mergeToolMessages()` — 没有 role=tool 消息需要合并
- snake_case→camelCase 转换 — API 响应已经是 camelCase
- `parseToolOutput()` 在加载时 — output 已经是序列化的 string
- 任何字段映射 — API 响应格式与流式最终状态完全一致

### 5.3 渲染逻辑

```tsx
function renderMessageContent(msg: UIMessage) {
  return (
    <>
      {msg.steps.map((step, stepIndex) => (
        <React.Fragment key={stepIndex}>
          {stepIndex > 0 && <Divider />}
          <StepContent step={step} />
        </React.Fragment>
      ))}
    </>
  );
}

function StepContent({ step }: { step: Step }) {
  const toolCallParts = step.parts.filter(p => p.type === 'tool-call');
  const otherParts = step.parts.filter(p => p.type !== 'tool-call');

  return (
    <>
      {/* 推理和文本内容 */}
      {otherParts.map((part, i) => {
        switch (part.type) {
          case 'reasoning':
            return <Think key={i}><MarkdownContent content={part.text} /></Think>;
          case 'text':
            return <MarkdownContent key={i} content={part.text} />;
        }
      })}
      
      {/* 工具调用链（ThoughtChain） */}
      {toolCallParts.length > 0 && (
        <ThoughtChain items={toolCallParts.map(part => ({
          key: part.toolCallId,
          title: part.toolName,
          status: mapToolCallStatus(part.state),
          description: part.args,
          content: part.output
            ? <MarkdownContent content={parseToolOutput(part.output)} />
            : undefined,
        }))} />
      )}
    </>
  );
}
```

---

## 六、向后兼容

### 6.1 数据库迁移

**现有数据**（legacy 格式）如何处理：

1. **Parts JSONB 已有数据**：按 stepIndex 字段重建 steps。没有 stepIndex 的旧数据归为 stepIndex=0。
2. **Parts JSONB 为空**：从 legacy 字段（content, reasoning, tool_calls）重建 steps，等同于当前的 backfillParts 逻辑，但产出新格式。
3. **role=tool 消息**：保留在数据库中用于 LLM 上下文构建，但 API 响应中不返回。

### 6.2 渐进式迁移

1. **阶段一**：后端同时发射新旧事件，前端只处理新事件
2. **阶段二**：确认新事件稳定后，停止发射旧事件
3. **阶段三**：清理旧事件相关代码

---

## 七、错误处理

### 7.1 前端容错

| 场景 | 处理方式 |
|------|---------|
| step_create 的 stepIndex 不连续 | 跳过缺失的索引，使用实际值 |
| part_create 的 partIndex 已存在 | 替换（不应该发生，但防御性处理） |
| part_update 引用不存在的 step/part | console.warn，忽略该事件 |
| SSE 连接中断 | 使用 message_done 缺失作为判断依据，将所有 streaming 状态设为 done |
| output 字段为空字符串 | 显示"无输出"或隐藏 output 区域 |

### 7.2 后端容错

| 场景 | 处理方式 |
|------|---------|
| SSE 序列化失败 | 记录日志，跳过该事件（当前是静默丢弃） |
| tool-call output 序列化失败 | 将 error 信息作为 output |
| Agent Loop panic | 确保所有已发射的 step 都有确定的状态 |

---

## 八、与 useXChat 的兼容性

### 8.1 核心约束

useXChat 的架构是 **1 次 onRequest = 1 条 assistant MessageInfo**。整个 Agent Loop（可能包含多次迭代）会被归入同一条 MessageInfo。

### 8.2 适配方式

`CopConMessage` 改为包含 `steps: Step[]` 的结构：

```typescript
interface CopConMessage {
  id: string;
  role: 'user' | 'assistant';
  steps: Step[];         // 替代原来的 parts: UIPart[]
  metadata: UIMessageMeta;
}
```

useXChat 的 `transformMessage` 仍然返回单个 `CopConMessage`，但这个消息内部包含多个步骤。渲染时，多个步骤在一条消息气泡内展示，步骤之间用 Divider 分隔。

### 8.3 如果未来需要多消息

如果需要每次迭代为独立消息，需要绕过 useXChat provider 机制：
- 直接使用 XRequest/XStream 解析 SSE
- 手动调用 `chatResult.setMessages()` 管理多条 MessageInfo
- 每收到 `step_create` 时创建新的 MessageInfo

当前设计不排除这种演进路径——UIMessage 的数据结构不变，只是消息的边界从"所有步骤"变为"单步"。
