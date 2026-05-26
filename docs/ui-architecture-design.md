# UI 跨框架架构设计

## 1. 背景与问题

### 现状

当前 `@copcon/ui` 是一个 React + Ant Design X 强耦合的组件库。其依赖关系如下：

| 组件 | 框架依赖 |
|---|---|
| `SubagentCard` | `@ant-design/x` (Think, ThoughtChain) + `x-markdown` + `antd` + `@ant-design/icons` |
| `HumanInteraction` | `antd` (Button, Card, Form, Input, Select, InputNumber) + `@ant-design/icons` |
| `TodoItem` | `@ant-design/icons` |
| `useAgentChat` | React hooks + `@ant-design/x-sdk` (XRequest, useXChat) |
| `useSubagentSSE` | React hooks + `@ant-design/x-sdk` (XRequest) |
| `CopConChatProvider` | 继承 `@ant-design/x-sdk` 的 `AbstractChatProvider`（但 `transformMessage` 逻辑本身无框架依赖） |
| `AgentClient` | 无（纯 fetch，已框架无关） |
| `api/types.ts` | 无（纯 TypeScript 类型定义） |

### 问题

1. **生态锁定**：无法在 Vue、Svelte 等非 React 生态中使用 CopCon 的 Agent 能力
2. **UI 耦合**：预置组件将"数据结构 → UI"的映射关系硬编码，用户无法深度定制渲染行为（如按工具类型分发不同渲染器、HITL 使用 Drawer 而非 inline 表单）
3. **demo 冗余**：`packages/demo/src/App.tsx` 已自行用 `@ant-design/x` 原始组件组合了完整的聊天 UI，并未使用 `@copcon/ui` 导出的组件

### 目标

设计一套架构，使 CopCon 的前端 AI Agent 能力可以：

- 在 React、Vue、Svelte 等多种框架生态下被引用
- 允许用户深度定制 UI 渲染，而不受预置组件的限制
- 核心业务逻辑（SSE 协议解析、消息状态管理、重连等）不随框架重复实现

---

## 2. 方案选型

### 2.1 候选方案对比

| 方案 | 框架覆盖 | SSE/流式 | Markdown | 复杂交互(HITL/工具链) | SSR | 维护成本 |
|---|---|---|---|---|---|---|
| Web Components (Lit/Stencil) | 全平台 | 可工作但需手动防抖 | Shadow DOM 内限制多 | 表单参与被 Shadow DOM 破坏 | 痛苦 | 低 |
| Mitosis (Builder.io) | 20+ 目标 | 不支持自定义 hooks | 无法使用框架特定库 | 无法编译 Context/状态机 | 受限 | 中 |
| Headless Core + Framework Adapters | 任意框架 | Core 统一处理 | 每框架用各自渲染器 | 每框架用原生表单方案 | 原生 | 中 |
| 嵌入式 Widget | 全平台 | 可嵌入 | 内置 | 无法深度定制 | 不适用 | 低 |

### 2.2 行业验证

Headless Core + Framework Adapters 模式已被多个生产级库验证：

| 库 | Core 包 | Adapters | 规模 |
|---|---|---|---|
| **TanStack Query** | `@tanstack/query-core` (0 deps) | React, Vue, Solid, Svelte, Angular, Lit | 58.2M 周下载 |
| **Vercel AI SDK** | `ai` (AbstractChat + ChatTransport) | React, Vue, Svelte, Angular | 数百万周下载 |
| **Ark UI / Zag.js** | `zag.js` (状态机) | React, Solid, Vue, Svelte | — |
| **Floating UI** | `@floating-ui/core` (纯数学) | React, Vue, DOM, React Native | — |

### 2.3 结论

选择 **Headless Core + Framework Adapters + Headless Hooks** 三层架构。

核心原因：

1. CopCon 的 SSE 协议是自定义的（`step_create` / `part_create` / `part_update` / `message_done`），通用框架的 SSE 处理不匹配
2. AI 聊天的复杂交互（流式 Markdown、工具调用可视化、HITL 表单、思考链）需要框架原生的组件生态支持
3. `CopConChatProvider.transformMessage()` 已经是纯函数，提取成本极低
4. Vercel AI SDK 的 `AbstractChat + ChatState + ChatTransport` 模式为 chat 场景提供了最佳的参考架构

---

## 3. 总体架构

### 3.1 分层设计

```
┌─────────────────────────────────────────────────────────┐
│  Layer 4: 视觉组件（可选，不发布）                          │
│  位置：packages/demo/src/components/                      │
│  作用：参考实现，展示如何组合 headless hooks 构建 UI        │
├─────────────────────────────────────────────────────────┤
│  Layer 3: Headless Hooks（发布 @copcon/headless-hooks）   │
│  作用：封装每种消息 part 的交互逻辑 + 状态 + ARIA           │
│  特点：纯函数为主，不渲染任何 DOM                           │
├─────────────────────────────────────────────────────────┤
│  Layer 2: Framework Adapters（发布 @copcon/chat-react 等）│
│  作用：将 Layer 1 的事件桥接到框架响应式系统                │
│  特点：极薄（50-200 行），每个框架一个包                    │
├─────────────────────────────────────────────────────────┤
│  Layer 1: Core（发布 @copcon/chat-core）                  │
│  作用：SSE 协议解析、消息转换、会话管理、重连               │
│  特点：纯 TypeScript，零框架依赖，零 DOM 依赖              │
└─────────────────────────────────────────────────────────┘
```

### 3.2 发布包结构

```
packages/
├── chat-core/              ← 发布 (@copcon/chat-core)
│   ├── types.ts              类型定义
│   ├── agent-client.ts       REST API 客户端
│   ├── sse-parser.ts         SSE 流解析器
│   ├── message-reducer.ts    SSE 事件 → CopConMessage 纯转换
│   ├── chat-session.ts       主会话管理（含重连）
│   ├── subagent-stream.ts    子 Agent 流监听
│   ├── utils.ts              工具函数
│   └── index.ts
│
├── chat-react/             ← 发布 (@copcon/chat-react)
│   ├── use-chat.ts           useChat() hook
│   ├── react-chat-state.ts   React 响应式桥接
│   └── index.ts
│
├── chat-vue/               ← 发布 (@copcon/chat-vue)（按需实现）
│   ├── use-chat.ts           useChat() composable
│   ├── vue-chat-state.ts     Vue 响应式桥接
│   └── index.ts
│
├── chat-svelte/            ← 发布 (@copcon/chat-svelte)（按需实现）
│   ├── chat.svelte.ts        Chat class
│   ├── svelte-chat-state.ts  Svelte 响应式桥接
│   └── index.ts
│
├── headless-hooks/         ← 发布 (@copcon/headless-hooks)
│   ├── use-thinking-chain.ts 思维链状态（展开/折叠/流式状态）
│   ├── use-tool-call.ts      工具调用状态（状态/输出解析/审批）
│   ├── use-message-list.ts   消息列表（滚动/分组）
│   ├── use-chat-input.ts     输入管理（发送/附件）
│   ├── use-hitl-form.ts      HITL 表单（验证/提交）
│   └── index.ts
│
├── ui/                     ← 删除（合并入 demo）
│
└── demo/                   ← 不发布，参考实现
    ├── src/
    │   ├── App.tsx             应用壳
    │   ├── components/         视觉组件（基于 headless hooks 构建）
    │   │   ├── ThinkingBlock.tsx
    │   │   ├── ToolCallCard.tsx
    │   │   ├── HumanInteraction.tsx
    │   │   ├── StreamMarkdown.tsx
    │   │   ├── SubagentCard.tsx
    │   │   └── TodoList.tsx
    │   └── renderers/          Part 渲染器注册表
    │       └── default-renderers.ts
    └── ...
```

### 3.3 用户消费路径

**浅度用户**（快速集成）：

```typescript
// 安装 chat-react + 参考 demo 代码
import { useChat } from '@copcon/chat-react';
import { AgentClient } from '@copcon/chat-core';
// 复制 demo 组件，按需修改
```

**深度用户**（完全定制）：

```typescript
// 只用 chat-core + headless-hooks
import { AgentClient, ChatSession } from '@copcon/chat-core';
import { useThinkingChain, useToolCall } from '@copcon/headless-hooks';
// 自己渲染所有 DOM
```

---

## 4. `@copcon/chat-core` 设计

### 4.1 类型系统

从现有 `api/types.ts` 提取，合并重复类型（`UIMessage` 和 `CopConMessage` 实质相同）。

#### 消息结构

```typescript
// Part 类型（Discriminated Union）
interface TextPart {
  type: 'text';
  text: string;
  state: 'streaming' | 'done';
}

interface ReasoningPart {
  type: 'reasoning';
  text: string;
  state: 'streaming' | 'done';
}

interface ToolCallPart {
  type: 'tool-call';
  toolCallId: string;
  toolName: string;
  args: string;      // 后端返回的 JSON 字符串
  output: string;    // 后端返回的 JSON 字符串
  error: string;
  state: 'pending' | 'running' | 'complete' | 'error' | 'waiting_for_input';
  interrupt?: InterruptPayload;
}

type Part = TextPart | ReasoningPart | ToolCallPart;

interface Step {
  parts: Part[];
  status: 'streaming' | 'done';
}

interface CopConMessage {
  id: string;
  role: 'user' | 'assistant';
  steps: Step[];
  metadata: UIMessageMeta;
}
```

#### HITL 中断

```typescript
interface InterruptPayload {
  interruptId: string;
  interruptType: 'approval' | 'question';
  message: string;
  summary?: string;
  inputSchema?: Record<string, unknown>;
}
```

#### 会话状态

```typescript
type SessionStatus = 'idle' | 'streaming' | 'reconnecting' | 'error';

interface SessionState {
  status: SessionStatus;
  error: Error | undefined;
}
```

#### 回调接口（框架适配的桥接点）

```typescript
interface ChatSessionCallbacks {
  onMessagesChange: (messages: CopConMessage[]) => void;
  onStateChange: (state: SessionState) => void;
}
```

### 4.2 SSE 协议解析（`sse-parser.ts`）

#### 后端协议要点

后端 SSE 只发 `data:` 行，不带 `event:` 或 `id:` 字段。事件类型在 JSON payload 的 `type` 字段中。

#### 事件目录

| 事件类型 | 触发时机 | 关键字段 |
|---|---|---|
| `step_create` | 新 step 开始（step 0 隐式，不发射此事件） | `messageId`, `stepIndex` |
| `part_create` | 新 part 创建（text/reasoning/tool-call） | `messageId`, `stepIndex`, `partIndex`, `partType`, `state`, 可选: `toolCallId`, `toolName`, `args` |
| `part_update` | part 内容/状态变化 | `messageId`, `stepIndex`, `partIndex`, `partType`, 可选: `textDelta`, `state`, `output`, `error`, `interrupt` |
| `message_done` | Agent 完成一轮对话 | `messageId` |
| `error` | 不可恢复错误 | `error` |
| `events_lost` | 重连时 ring buffer 已淘汰旧事件 | (无) |
| `async_tool_started` | 异步工具开始执行 | `message_id`, `call_id`, `tool_name` |
| `async_tool_complete` | 异步工具成功 | `message_id`, `call_id`, `result`, `duration_ms` |
| `async_tool_failed` | 异步工具失败 | `message_id`, `call_id`, `error`, `duration_ms` |

#### 协议注意事项

1. **step 0 隐式**：第一个 `part_create` 到达时 step 0 自动创建，不会收到 `step_create` 事件
2. **`textDelta` 是增量的**：需要累加，不能替换
3. **`args` 和 `output` 是 JSON 字符串**：客户端需要 `JSON.parse()`
4. **camelCase / snake_case 混用**：step/part 事件用 camelCase（`messageId`），async tool 事件用 snake_case（`message_id`）
5. **消息 ID**：每个有意义的事件都携带 `messageId`，第一个 `part_create` 就包含。无需客户端生成或替换
6. **Ring buffer 容量 1024**：重连时 seq 差超过 1024 会收到 `events_lost`

#### SSE Parser 职责

SSE parser 是通用的流解析工具，只做：

- `data:` 行分割
- 按空行识别事件边界
- JSON parse

不关心事件语义。语义处理交给 message reducer。

### 4.3 消息转换器（`message-reducer.ts`）

这是系统的核心——从 `CopConChatProvider.transformMessage()` 提取的纯函数，**零副作用**，**零框架依赖**。

#### 核心函数

| 函数 | 职责 |
|---|---|
| `applySSEChunk(message, rawData) → newMessage` | 将一个 SSE chunk 应用到当前消息，返回不可变更新后的新消息 |
| `createUserMessage(content) → newMessage` | 创建用户消息 |
| `mergeMessages(fetched, local) → merged[]` | 合并 API 消息和本地消息（按 ID 去重） |

#### `applySSEChunk` 内部调度

| SSE 事件 | 处理逻辑 |
|---|---|
| `step_create` | 扩展 steps 数组到 stepIndex，插入空 step |
| `part_create` | 根据 partType 创建 TextPart / ReasoningPart / ToolCallPart，插入到对应位置 |
| `part_update` | 累加 textDelta（text/reasoning）、更新 state（tool-call）、填充 output/error/interrupt |
| `message_done` | 将所有 streaming→done，所有 pending/running→complete |
| `error` | 不修改消息结构，仅记录 |

#### 设计原则

- **不可变更新**：每次返回新对象，不修改原对象。`ensureSteps()` / `ensureParts()` 做数组扩展时拷贝
- **防御性解析**：所有字段做类型检查（`typeof data.xxx === 'string'`），类型不匹配时使用默认值
- **step 0 兜底**：`part_create` 在 stepIndex=0 时，即使没有 `step_create`，`ensureSteps()` 也会自动创建 step

### 4.4 会话管理

#### ChatSession（主会话）

从 `useAgentChat` 提取的业务逻辑，封装为 Class + Callback 模式。

**公共 API：**

| 方法 | 职责 |
|---|---|
| `new ChatSession(config)` | 构造实例，config 包含 AgentClient、sessionId、回调 |
| `start()` | 加载历史消息 + 建立 SSE 连接 |
| `sendMessage(content)` | 发送消息（乐观添加用户消息 → 建立 SSE 连接） |
| `abort()` | 中止当前请求 |
| `loadMessages()` | 从 API 加载历史消息 |
| `destroy()` | 清理资源 |

**重连（在 Core 层处理）：**

```
SSE onError
  → client.reconnect(sessionId, lastSeq + 1)
    → 204 响应：从 API 拉全量消息，完成
    → SSE body：逐条 parse 补漏事件
    → 失败：从 API 拉全量 + mergeMessages() 合并本地消息 + 重建连接
```

重连放在 Core 层的原因：

- 重连是协议行为（seq 追踪、ring buffer 限制、events_lost 处理），与框架无关
- 每个框架的 adapter 都需要重连能力，不应重复实现
- `agentClient.reconnect()` 是纯 fetch，Core 可直接调用

#### SubagentStream（子 Agent 流）

从 `useSubagentSSE` 提取，比 ChatSession 更简单：

- 一次性 SSE 连接，无重连
- 无消息加载和合并逻辑
- 只监听流并转换消息

**公共 API：**

| 方法 | 职责 |
|---|---|
| `new SubagentStream(config)` | 构造实例 |
| `start()` | 建立 SSE 连接 |
| `destroy()` | 清理资源（中止 fetch） |

两个类共享底层的 `sse-parser.ts` 和 `message-reducer.ts`，不在类层面强制统一。

---

## 5. Framework Adapters 设计

### 5.1 桥接模式

每个 adapter 的职责只有一个：将 Core 的回调事件桥接到框架的响应式系统。

| 框架 | 桥接机制 |
|---|---|
| **React** | `useState` 存储 + `useEffect` 生命周期管理 |
| **Vue** | `ref()` / `shallowRef()` + `onScopeDispose` |
| **Svelte** | `$state` runes |

### 5.2 React Adapter 示例

```typescript
import { useState, useEffect, useRef } from 'react';
import { ChatSession, AgentClient } from '@copcon/chat-core';

export function useChat(options: UseChatOptions) {
  const [messages, setMessages] = useState<CopConMessage[]>([]);
  const [state, setState] = useState<SessionState>({ status: 'idle' });
  const sessionRef = useRef<ChatSession | null>(null);

  useEffect(() => {
    const session = new ChatSession({
      client: options.client,
      sessionId: options.sessionId,
      callbacks: {
        onMessagesChange: setMessages,
        onStateChange: setState,
      },
    });
    session.start();
    sessionRef.current = session;
    return () => session.destroy();
  }, [options.client, options.sessionId]);

  return {
    messages,
    status: state.status,
    error: state.error,
    sendMessage: (content: string) => sessionRef.current?.sendMessage(content),
    abort: () => sessionRef.current?.abort(),
  };
}
```

Adapter 层的特征：

- 极薄（50-100 行）
- 只做 `Core 回调 → 框架响应式` 的桥接
- 不包含任何业务逻辑

---

## 6. `@copcon/headless-hooks` 设计

### 6.1 核心原则

Headless hooks 封装的是**每种消息 part 的交互逻辑**（状态管理、ARIA 无障碍属性），但不决定它长什么样。

关键区分：

| | 包含 | 不包含 |
|---|---|---|
| **Headless hooks** | 状态（展开/折叠）、副作用（自动滚动）、ARIA 属性、数据转换 | 任何 JSX/DOM/样式 |
| **视觉组件** | DOM 结构、CSS 样式、动画 | 交互状态的管理逻辑 |

### 6.2 框架无关策略

Headless hooks 以**纯函数 + 状态对象**为主，不含任何 `useState`/`useEffect`：

```typescript
// headless-hooks — 不是 React hook，是普通函数

// 思维链控制器
function createThinkingChainController(part: ReasoningPart, options?: {
  defaultExpanded?: boolean;
  autoCollapse?: boolean;
}) {
  let expanded = options?.defaultExpanded ?? true;

  return {
    get expanded() { return expanded; },
    toggle() { expanded = !expanded; },
    get isStreaming() { return part.state === 'streaming'; },
    get text() { return part.text; },
    getContainerProps() { return { role: 'region' as const, 'aria-label': 'Thinking' }; },
    getToggleProps() { return { 'aria-expanded': expanded }; },
    getContentProps() { return { hidden: !expanded }; },
  };
}

// 工具调用控制器
function createToolCallController(part: ToolCallPart) {
  return {
    get toolName() { return part.toolName; },
    get status() { return part.state; },
    get parsedArgs() { /* JSON.parse(part.args) with error handling */ },
    get parsedOutput() { /* parseToolOutput(part.output) */ },
    get needsApproval() { return part.state === 'waiting_for_input'; },
    get interrupt() { return part.interrupt; },
    getStatusProps() { return { 'aria-live': 'polite' as const }; },
  };
}

// 消息列表控制器
function createMessageListController(options?: {
  autoScroll?: boolean;
}) {
  return {
    // 框架侧提供 containerRef，这里提供逻辑
    shouldAutoScroll(isAtBottom: boolean) { return isAtBottom && options?.autoScroll !== false; },
    getItemProps(msg: CopConMessage) { return { 'data-role': msg.role }; },
  };
}
```

用户在各自框架中使用：

```typescript
// React
const ctrl = useMemo(() => createThinkingChainController(part), [part]);

// Vue
const ctrl = reactive(createThinkingChainController(part));

// Svelte
const ctrl = $derived(createThinkingChainController(part));
```

### 6.3 按需添加框架适配层

如果某个 hook 确实需要框架特定的副作用管理（如 `useEffect` 管理自动滚动），按需增加薄封装：

```
@copcon/headless-hooks          ← 纯函数（框架无关）
@copcon/headless-hooks/react    ← React hook 封装（按需，非默认）
```

遵循 YAGNI 原则——先导出纯函数，等有真实需求时再增加适配层。

---

## 7. 视觉组件的定制能力

### 7.1 定制机制

视觉组件不发布为包，而是放在 `packages/demo/` 中作为参考实现。用户有两种使用方式：

| 方式 | 适用场景 | 工作量 |
|---|---|---|
| **复制 + 修改** | 想快速开始，微调样式 | 低 |
| **从头构建** | 完全自定义 UI，只用 headless hooks | 中-高 |

### 7.2 定制场景示例

**场景：隐藏 Todo**

用户不渲染 TodoList 组件即可。零成本。

**场景：按工具类型分发不同渲染器**

```typescript
// 用户自定义：根据 toolName 选择不同 UI
const toolRenderers = {
  'shell_executor': ShellToolRenderer,
  'code_executor': CodeToolRenderer,
  'web_search': SearchResultRenderer,
};

function MyToolCallRenderer({ part }) {
  const ctrl = createToolCallController(part);
  const Component = toolRenderers[ctrl.toolName] ?? FallbackToolCard;
  return <Component {...ctrl} />;
}
```

**场景：HITL 用 Drawer 而非 inline 表单**

```typescript
function DrawerApproval({ part }) {
  const ctrl = createToolCallController(part);
  return (
    <Drawer open={ctrl.needsApproval}>
      <p>{ctrl.interrupt?.message}</p>
      <Button onClick={() => approve(part.interrupt.interruptId)}>批准</Button>
    </Drawer>
  );
}
```

**场景：在消息流中插入自定义内容**

```typescript
function MyMessageList({ messages }) {
  return messages.map((msg, i) => (
    <>
      <MessageRenderer message={msg} />
      {i === 3 && <PromoCard />}
    </>
  ));
}
```

### 7.3 `@copcon/ui` 的处理

现有 `@copcon/ui` 包废弃并删除：

| 原因 | 说明 |
|---|---|
| demo 未使用其组件 | `App.tsx` 直接用 `@ant-design/x` 组合，`SubagentCard` / `TodoItem` 从未被消费 |
| 与 headless 架构矛盾 | 发布视觉组件隐含"请直接使用"的承诺，限制了用户的定制空间 |
| 组件逻辑简单 | `TodoList` 30 行 JSX，`HumanInteraction` 是标准表单，不值得独立发包 |

组件代码迁入 `packages/demo/src/components/`，作为参考实现。

---

## 8. 迁移计划

### 第一阶段：提取 Core（基础设施）

| 任务 | 来源 | 目标 | 工作量 |
|---|---|---|---|
| 类型定义提取 | `packages/ui/src/api/types.ts` | `packages/chat-core/src/types.ts` | 0.5 天 |
| AgentClient 搬迁 | `packages/ui/src/api/agentClient.ts` | `packages/chat-core/src/agent-client.ts` | 0 天（零改动） |
| 工具函数搬迁 | `packages/ui/src/utils/messageUtils.ts` | `packages/chat-core/src/utils.ts` | 0 天 |
| SSE Parser 提取 | `useAgentChat.parseReconnectSSE()` | `packages/chat-core/src/sse-parser.ts` | 0.5 天 |
| Message Reducer 提取 | `CopConChatProvider.transformMessage()` | `packages/chat-core/src/message-reducer.ts` | 1 天 |
| ChatSession 类 | `useAgentChat` 业务逻辑 | `packages/chat-core/src/chat-session.ts` | 1-2 天 |
| SubagentStream 类 | `useSubagentSSE` 业务逻辑 | `packages/chat-core/src/subagent-stream.ts` | 0.5 天 |

### 第二阶段：Framework Adapter

| 任务 | 目标 | 工作量 |
|---|---|---|
| React Adapter (`chat-react`) | `packages/chat-react/` | 0.5 天 |
| 重构 demo 使用 `chat-react` | `packages/demo/` | 0.5 天 |

### 第三阶段：Headless Hooks + Demo 组件

| 任务 | 目标 | 工作量 |
|---|---|---|
| Headless hooks 包 | `packages/headless-hooks/` | 1-2 天 |
| Demo 视觉组件重构为 headless hooks 消费者 | `packages/demo/src/components/` | 2-3 天 |
| 删除 `@copcon/ui` 包 | — | 0.5 天 |

### 第四阶段：多框架扩展（按需）

| 任务 | 目标 | 工作量 |
|---|---|---|
| Vue Adapter (`chat-vue`) | `packages/chat-vue/` | 0.5-1 天 |
| Vue 组件参考实现 | 独立 demo 或文档 | 1-2 周 |
| Svelte Adapter (`chat-svelte`) | `packages/chat-svelte/` | 0.5-1 天 |
| Svelte 组件参考实现 | 独立 demo 或文档 | 1-2 周 |

---

## 9. 架构决策记录

| # | 决策点 | 结论 | 理由 |
|---|---|---|---|
| 1 | Core 的形态 | 纯函数 transform + 分离的 Class 管理（Vercel AI SDK 模式） | `CopConChatProvider.transformMessage()` 已是纯函数；Chat 是天然有状态的场景，Class 封装会话生命周期更直观 |
| 2 | 协议脏活的分层 | SSE Parser（通用流解析）+ Message Reducer（协议语义） | 关注点分离；SSE Parser 可复用，Message Reducer 可独立测试 |
| 3 | 重连逻辑归属 | Core 层 | 重连是协议行为（seq 追踪、ring buffer），与框架无关 |
| 4 | ChatSession vs SubagentStream | 两个独立类，共享 reducer + parser | 生命周期差异大（重连/消息合并 vs 一次性只读流），强行统一会产生条件分支 |
| 5 | Headless hooks 跨框架 | 纯函数为主，按需加框架适配层 | YAGNI；大部分 headless 逻辑是纯计算，天然框架无关 |
| 6 | 消息 ID 策略 | 直接使用后端 messageId | 每个事件（包括第一个 `part_create`）都携带 messageId，无需客户端生成/替换 |
| 7 | `@copcon/ui` 处理 | 废弃删除，组件迁入 demo | demo 从未使用其组件；发布视觉组件与 headless 架构精神矛盾 |
