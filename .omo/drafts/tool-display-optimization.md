# Draft: Tool Display Optimization

## Requirements (confirmed)

**User's Request**:
- 优化 tool 的展示
- assistant 消息携带 tool_calls 时，使用思维链 (ThoughtChain) 来展示
- 收到 tool 消息 (role=tool) 时，根据 tool_call_id 更新对应 tool_call 的状态，而不是展示为新气泡
- tool 消息内容应该更新到对应 tool_call 的输出中

**Data Example**:
```json
[
  {
    "role": "assistant",
    "tool_calls": [
      {
        "id": "tool-23002e4743aa4152a0e84d37ffdfb334",
        "type": "function",
        "function": {
          "name": "todolist",
          "arguments": "{\"action\": \"create\", ...}"
        }
      }
    ]
  },
  {
    "role": "tool",
    "tool_call_id": "tool-23002e4743aa4152a0e84d37ffdfb334",
    "content": "{\"success\":false,\"error\":\"...\"}"
  }
]
```

## Current Implementation Analysis

### Message Types
- `CopConMessage` (packages/ui/src/providers/CopConChatProvider.ts):
  - `role`: 'user' | 'assistant' | 'tool'
  - `tool_calls?: Array<{id, name, arguments}>`
  - `tool_call_id?: string`

### Current Rendering (packages/demo/src/App.tsx)
- Messages are rendered as separate bubbles
- `tool` role messages appear as separate tool bubbles (lines 164-178)
- `toolExecutions` from useAgentChat are appended as additional bubbles
- No merging between assistant's tool_calls and tool response messages

### Problem
- assistant 消息和 tool 消息分开显示为独立气泡
- toolExecutions 被单独追加到消息列表末尾
- 没有根据 tool_call_id 将 tool 响应合并到对应的 tool_call

## Technical Decisions

### Approach: Merge Tool Messages into Assistant Message's Tool Calls

**Solution Architecture**:
1. 在渲染前预处理 messages，将 role=tool 的消息合并到前一条 assistant 消息的 tool_calls 中
2. 使用 ThoughtChain 组件展示 tool_calls
3. 每个 tool_call 包含：name, arguments, status, output

**Key Implementation Points**:
1. 创建消息预处理函数 `mergeToolMessages(messages)`
2. 更新 CopConMessage 的 tool_calls 结构，添加 status 和 output 字段
3. 在 App.tsx 的 bubbleItems 生成逻辑中使用 ThoughtChain 展示 tool_calls

### Ant Design X ThoughtChain Component

**Usage Pattern**:
```tsx
import { ThoughtChain } from '@ant-design/x';

const items: ThoughtChainItemType[] = [
  {
    key: 'tool-call-id',
    title: 'Tool Name',
    description: 'Arguments JSON',
    status: 'loading' | 'success' | 'error',
    collapsible: true,
    content: 'Output content',
  }
];

<ThoughtChain items={items} />
```

## Research Findings

### ThoughtChain Component API (from x.ant.design)
- `items`: ThoughtChainItemType[]
- `ThoughtChainItemType`:
  - `key`: unique identifier (use tool_call_id)
  - `title`: tool name
  - `description`: arguments or extra info
  - `status`: 'loading' | 'success' | 'error' | 'abort'
  - `collapsible`: boolean (can expand/collapse)
  - `content`: React.ReactNode (tool output)
  - `icon`: false | React.ReactNode

### Status Mapping
- tool_call received → status: 'loading'
- tool_result received (success) → status: 'success'
- tool_result received (error) → status: 'error'
- request aborted → status: 'abort'

## Open Questions

1. ✅ RESOLVED: Use ThoughtChain for display - YES, it's the right component
2. ✅ RESOLVED: Merge approach - preprocess messages before rendering
3. Need to clarify: Should we keep the current toolExecutions tracking in useAgentChat, or rely only on message-based tracking?

## Scope Boundaries

### INCLUDE:
- 修改 CopConMessage 类型，添加 tool_call status/output
- 创建消息预处理函数 mergeToolMessages
- 更新 App.tsx 渲染逻辑，使用 ThoughtChain 展示 tool_calls
- 更新 CopConChatProvider 的 transformMessage 以处理 tool_result 更新到正确的 tool_call

### EXCLUDE:
- 不修改后端 API
- 不改变消息存储结构
- 不删除现有 UI 组件

## Implementation Plan

### Step 1: Update Types
- Update `CopConMessage.tool_calls` structure:
  ```typescript
  tool_calls?: Array<{
    id: string;
    name: string;
    arguments: string;
    status?: 'loading' | 'success' | 'error' | 'abort';
    output?: string;
  }>
  ```

### Step 2: Update CopConChatProvider
- In `transformMessage`, when receiving `tool_result`:
  - Find the corresponding tool_call by id
  - Update its status and output
  - Don't create a new message

### Step 3: Create Message Merge Utility
- Create `packages/ui/src/utils/messageUtils.ts`
- Implement `mergeToolMessages(messages: CopConMessage[]): CopConMessage[]`
- Logic:
  1. Iterate through messages
  2. For each role=tool message, find previous assistant message
  3. Match by tool_call_id
  4. Update the corresponding tool_call's status and output
  5. Remove the role=tool message from final list

### Step 4: Update App.tsx Rendering
- Import ThoughtChain from @ant-design/x
- Use mergeToolMessages to preprocess messages
- For assistant messages with tool_calls:
  - Render ThoughtChain as header/content
  - Each tool_call becomes a ThoughtChain.Item
- Remove the separate toolExecutions rendering

### Step 5: Clean up useAgentChat
- Remove or deprecate toolExecutions tracking
- Tool status now comes from messages directly