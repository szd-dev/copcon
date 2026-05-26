# UI 跨框架架构重构

## TL;DR

> **Quick Summary**: 将 `@copcon/ui`（React + Ant Design X 强耦合）重构为 Headless Core + Framework Adapters + Headless Hooks 三层跨框架架构，删除 `@copcon/ui`，视觉组件迁入 demo 作为参考实现。
>
> **Deliverables**:
> - `@copcon/chat-core` — 纯 TS 核心（SSE 解析、消息转换、会话管理、重连）
> - `@copcon/chat-react` — React 适配层（useChat hook，<200 行）
> - `@copcon/headless-hooks` — 框架无关的交互逻辑（思维链、工具调用、消息列表）
> - 重构后的 demo — 使用 chat-react + headless-hooks 的参考实现
> - `@copcon/ui` 废弃删除
>
> **Estimated Effort**: Large
> **Parallel Execution**: YES — 4 waves
> **Critical Path**: T5 (message-reducer) → T9 (ChatSession) → T11 (useChat) → T16 (App.tsx rewrite)

---

## Context

### Original Request
用户希望 UI 组件库不再强依赖 React 和 Ant Design 生态，能在多种框架生态下被引用。

### Interview Summary
**Key Discussions**:
- 评估了 4 种方案：Web Components (Lit/Stencil)、Mitosis、Headless Core+Adapters、嵌入式 Widget
- 选定 Headless Core + Framework Adapters，参考 TanStack Query (58.2M/wk)、Vercel AI SDK、Ark UI 模式
- 深入分析了 Go 后端 SSE 协议（9 种事件类型、step 0 隐式、messageId 全链路携带）
- 确认 demo 实际使用了 `@copcon/ui`（TodoList, HumanInteraction, useAgentChat, AgentClient, 多个类型），迁移并非简单换 import
- 测试策略：tests-after + vitest，聚焦 chat-core 纯函数

**Research Findings**:
- `CopConChatProvider.transformMessage()` 313 行中仅 class 声明依赖 x-sdk —— 提取成本极低
- `AgentClient` 已是纯 fetch，零改动
- `useAgentChat` 338 行中约 60% 是框架无关业务逻辑（SSE 解析、重连、消息合并）
- 后端 step_create 对 stepIndex=0 不发射（隐式 step 0），但所有事件都携带 messageId

### Metis Review
**Identified Gaps** (addressed):
- ~~Demo 不使用 @copcon/ui~~ → 已修正：Demo 使用了 TodoList/HumanInteraction/useAgentChat/AgentClient/6 个类型，迁移非平凡
- Phase 顺序错误 → 已调整为：core → tests → adapter → hooks → demo → cleanup
- `events_lost` 无处理策略 → ChatSession 重连流程显式处理：收到 events_lost 立即 fallback 到全量拉取
- `async_tool_*` 事件未定义处理 → 显式标记为 out-of-scope，reducer 中 return baseMessage + TODO 注释
- 204 reconnect 假设存疑 → ChatSession 中检测空 body 响应（200/204），统一 fallback
- filler-parts 模式需保留或显式变更 → 保留现有行为，测试覆盖

---

## Work Objectives

### Core Objective
将 `@copcon/ui` 的框架无关业务逻辑提取为 `@copcon/chat-core`，提供 React 适配层，创建 headless hooks 包，重构 demo 为参考实现，最终删除 `@copcon/ui`。

### Concrete Deliverables
- `packages/chat-core/` — 纯 TS 包（0 runtime deps）
- `packages/chat-react/` — React adapter（<200 行）
- `packages/headless-hooks/` — 框架无关交互 hooks
- `packages/demo/src/components/` — 参考实现视觉组件
- `packages/ui/` — 删除

### Definition of Done
- [ ] `cd packages/chat-core && pnpm build` → 成功，0 @ant-design 依赖
- [ ] `cd packages/chat-core && pnpm test` → 所有测试通过
- [ ] `cd packages/chat-react && pnpm build` → 成功
- [ ] `cd packages/demo && pnpm build` → 成功
- [ ] `grep -r "@copcon/ui" packages/demo/` → 0 matches
- [ ] `packages/ui/` 目录已删除

### Must Have
- chat-core 零框架依赖（package.json dependencies 为空）
- 消息转换逻辑与现有 CopConChatProvider.transformMessage() 行为完全等价
- 保留 filler-parts 模式（partIndex > parts.length 时填充空 text part）
- 保留 step 0 隐式创建行为
- ChatSession 重连流程：onError → reconnect(seq+1) → 正常/204/空body/events_lost → fallback 全量拉取
- SubagentStream 使用 reconnect: true + last_event_seq: 0 获取全部事件

### Must NOT Have (Guardrails)
- chat-core 不得 import 任何 @ant-design/* 或 react 包
- chat-react 不得超过 200 行（业务逻辑泄漏检测）
- 不得修改 Go 后端代码
- 不得添加 async_tool_* 事件的消息状态跟踪（保持忽略，加 TODO 注释）
- 不得添加重连重试/退避/抖动策略（保持 current 一次尝试 + fallback）
- 不得创建 chat-vue 或 chat-svelte 的 stub 包
- 不得在 headless hooks 中添加表单验证逻辑（仅提供状态 + ARIA）
- 不得添加 JSDoc 注释（提取优先，文档后续）
- chat-core 测试文件不得 import react、vue 或任何 DOM API

---

## Verification Strategy

### Test Decision
- **Infrastructure exists**: NO (JS/TS 侧无测试基础设施)
- **Automated tests**: Tests-after (vitest)
- **Framework**: vitest
- **Scope**: chat-core 纯函数（message-reducer、sse-parser）+ chat-react hook 基本测试

### QA Policy
每个 task 包含 agent-executed QA scenarios：
- chat-core：`pnpm build` 成功 + `pnpm test` 通过
- chat-react：`pnpm build` 成功
- demo：`pnpm build` 成功 + 验证无 @copcon/ui 引用
- 集成：使用 `ast_grep_search` 验证无非法 import

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Foundation — can start immediately, all parallel):
├── T1: chat-core package scaffold + tsconfig [quick]
├── T2: Type extraction + consolidation [quick]
├── T3: AgentClient migration [quick]
└── T4: Utils + SSE Parser extraction [quick]

Wave 2 (Core Logic — after Wave 1):
├── T5: Message Reducer (applySSEChunk) [deep]
├── T6: ChatSession class [deep]
└── T7: SubagentStream class [unspecified-high]

Wave 3 (Testing — after Wave 2):
├── T8: Vitest setup + message-reducer tests [unspecified-high]
└── T9: SSE parser tests [unspecified-high]

Wave 4 (Adapters + Headless Hooks — after Wave 3, parallel):
├── T10: chat-react adapter (useChat) [unspecified-high]
├── T11: createThinkingChainController [quick]
├── T12: createToolCallController [quick]
└── T13: createMessageListController + createHitlFormController [quick]

Wave 5 (Demo Components — after Wave 4, most parallel):
├── T14: StreamMarkdown component [quick]
├── T15: ThinkingBlock reference component [quick]
├── T16: ToolCallCard reference component [unspecified-low]
├── T17: HumanInteraction reference component [unspecified-low]
├── T18: TodoList + TodoItem reference components [quick]
└── T19: SubagentCard reference component [unspecified-low]

Wave 6 (Integration — after Wave 5):
└── T20: App.tsx rewrite + demo integration [deep]

Wave 7 (Cleanup — after Wave 6):
├── T21: Delete @copcon/ui + workspace cleanup [quick]
└── T22: Cross-package integration tests + build verification [unspecified-high]

Wave FINAL (after all tasks — 3 parallel reviews):
├── F1: Plan compliance audit (oracle)
├── F2: Code quality review (unspecified-high)
└── F3: Scope fidelity check (deep)
→ Present results → Get explicit user okay

Critical Path: T2 → T5 → T6 → T10 → T20 → T21 → F1-F3
Parallel Speedup: ~60% faster than sequential
Max Concurrent: 6 (Wave 5)
```

### Dependency Matrix

- T1: — → T2,T3,T4
- T2: T1 → T5,T6,T7
- T3: T1 → T6,T7
- T4: T1 → T5
- T5: T2,T4 → T6,T7,T8,T9
- T6: T3,T2,T5 → T10
- T7: T3,T2,T5 → T19
- T8: T5 → T9 (informational)
- T9: T8 → T10 (gate)
- T10: T6,T9 → T20
- T11: T2 → T15
- T12: T2 → T16,T17
- T13: T2 → T17,T18
- T14: — → T15,T16,T19
- T15: T11,T14 → T19,T20
- T16: T12,T14 → T19,T20
- T17: T12,T13,T14 → T20
- T18: T13 → T20
- T19: T7,T14,T15,T16 → T20
- T20: T10,T15,T16,T17,T18,T19 → T21
- T21: T20 → T22
- T22: T21 → F1,F2,F3

### Agent Dispatch Summary

- Wave 1: T1→quick, T2→quick, T3→quick, T4→quick
- Wave 2: T5→deep, T6→deep, T7→unspecified-high
- Wave 3: T8→unspecified-high, T9→unspecified-high
- Wave 4: T10→unspecified-high, T11→quick, T12→quick, T13→quick
- Wave 5: T14→quick, T15→quick, T16→unspecified-low, T17→unspecified-low, T18→quick, T19→unspecified-low
- Wave 6: T20→deep
- Wave 7: T21→quick, T22→unspecified-high
- FINAL: F1→oracle, F2→unspecified-high, F3→deep

---

## TODOs

- [x] 1. chat-core Package Scaffold + tsconfig

  **What to do**:
  - Create `packages/chat-core/` directory structure
  - Initialize `package.json` with: name `@copcon/chat-core`, type `module`, zero runtime deps, devDependencies limited to `typescript` + `vitest`
  - Create `tsconfig.json` (target ES2022, strict, declaration emit)
  - Create `vite.config.ts` (library mode, entry `src/index.ts`)
  - Create empty `src/index.ts` barrel export
  - Add `packages/copcon-chat-core` to `pnpm-workspace.yaml`

  **Must NOT do**:
  - Do NOT add any runtime dependencies to package.json
  - Do NOT add any @ant-design, react, or DOM-related packages

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: [] (no specialized skills needed)

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with T2, T3, T4)
  - **Blocks**: T2, T3, T4, T5, T6, T7
  - **Blocked By**: None

  **References**:
  - `packages/ui/package.json` — reference for pnpm workspace package structure (name pattern, exports field, files field)
  - `packages/ui/tsconfig.json` — reference for TypeScript config settings
  - `packages/ui/vite.config.ts` — reference for Vite library mode build config
  - `pnpm-workspace.yaml` — file to modify (add new package path)

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-core && pnpm build` → exit 0 (builds empty package successfully)
  - [ ] `cat packages/chat-core/package.json | jq '.dependencies // {}'` → `{}` (zero dependencies)
  - [ ] `cat pnpm-workspace.yaml` → contains `packages/chat-core`

  **QA Scenarios**:
  ```
  Scenario: Package builds successfully
    Tool: Bash
    Steps:
      1. cd packages/chat-core && pnpm install
      2. pnpm build
    Expected Result: exit code 0
    Evidence: .sisyphus/evidence/task-1-build.txt
  ```

  **Commit**: YES (groups with T2, T3, T4)
  - Message: `refactor(ui): extract chat-core package scaffold and foundational modules`

---

- [x] 2. Type Extraction + Consolidation

  **What to do**:
  - Create `packages/chat-core/src/types.ts`
  - Copy from `packages/ui/src/api/types.ts`: CopConMessage (renamed from UIMessage), Session, Todo, InterruptPayload, TextPart, ReasoningPart, ToolCallPart, Part, Step, UIMessageMeta, all SSE event types
  - Copy from `packages/ui/src/providers/CopConChatProvider.ts`: CopConInput, CopConSSEOutput (simplified)
  - Add `SessionStatus` type: `'idle' | 'streaming' | 'reconnecting' | 'error'`
  - Add `SessionState` interface: `{ status: SessionStatus; error: Error | undefined }`
  - Add `ChatSessionCallbacks` interface: `onMessagesChange`, `onStateChange`
  - Delete deprecated `Message` type (use CopConMessage instead)
  - Reconcile `UIMessage` vs `CopConMessage` into single `CopConMessage`
  - Update `src/index.ts` to re-export all types

  **Must NOT do**:
  - Do NOT rename fields (keep backend alignment: `messageId`, `stepIndex` etc.)
  - Do NOT add JSDoc comments

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with T1, T3, T4)
  - **Blocks**: T5, T6, T7, T11, T12, T13
  - **Blocked By**: T1

  **References**:
  - `packages/ui/src/api/types.ts` — source file to extract types from (all lines)
  - `packages/ui/src/providers/CopConChatProvider.ts:18-39` — CopConMessage, CopConInput, CopConSSEOutput type definitions
  - `packages/ui/src/hooks/useAgentChat.ts:12-30` — UseAgentChatOptions, UseAgentChatReturn (inform shape for ChatSessionCallbacks)

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-core && pnpm build` → exit 0
  - [ ] All types exported from `src/index.ts`
  - [ ] No deprecated `Message` type remains
  - [ ] No duplicate `UIMessage` type (merged into `CopConMessage`)

  **QA Scenarios**:
  ```
  Scenario: Types compile and export correctly
    Tool: Bash
    Steps:
      1. cd packages/chat-core && npx tsc --noEmit
    Expected Result: exit code 0, no type errors
    Evidence: .sisyphus/evidence/task-2-types.txt
  ```

  **Commit**: YES (groups with T1, T3, T4)

---

- [x] 3. AgentClient Migration

  **What to do**:
  - Copy `packages/ui/src/api/agentClient.ts` to `packages/chat-core/src/agent-client.ts`
  - Update imports: `import type { Session, Message, Todo } from './types'` (local import)
  - Update `getMessages()` return type: keep current `Message` type for now (backend returns this shape), add a mapping note for future
  - Update `src/index.ts` to export `AgentClient` and `AgentClientConfig`
  - Zero logic changes — file is already framework-agnostic

  **Must NOT do**:
  - Do NOT change any method signatures or logic
  - Do NOT change the reconnect() method's raw fetch approach (documented reason in comments L88-92)

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with T1, T2, T4)
  - **Blocks**: T6, T7
  - **Blocked By**: T1

  **References**:
  - `packages/ui/src/api/agentClient.ts` — source file (copy verbatim, 157 lines)
  - Important: L88-92 comment explains WHY reconnect uses raw fetch instead of XRequest — this constraint is preserved by dropping XRequest entirely

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-core && pnpm build` → exit 0
  - [ ] File is identical to source except import paths

  **QA Scenarios**:
  ```
  Scenario: AgentClient compiles in new location
    Tool: Bash
    Steps:
      1. cd packages/chat-core && npx tsc --noEmit
    Expected Result: exit code 0
    Evidence: .sisyphus/evidence/task-3-agent-client.txt

  Scenario: No framework imports leaked
    Tool: Bash
    Steps:
      1. grep -r "react\|@ant-design" packages/chat-core/src/agent-client.ts || echo "CLEAN"
    Expected Result: CLEAN
    Evidence: .sisyphus/evidence/task-3-no-framework-imports.txt
  ```

  **Commit**: YES (groups with T1, T2, T4)

---

- [x] 4. Utils + SSE Parser Extraction

  **What to do**:
  - Copy `packages/ui/src/utils/messageUtils.ts` to `packages/chat-core/src/utils.ts` (parseToolOutput function)
  - Create `packages/chat-core/src/sse-parser.ts`:
    - `parseSSEStream(reader, onChunk)` — extract from `useAgentChat.ts:169-201` (parseReconnectSSE function)
    - `parseSSERaw(rawData)` — extract JSON parse + type check from `CopConChatProvider.ts:98-101`
  - Handles: `data:` line splitting, blank line event boundaries, trailing buffer
  - Update `src/index.ts` to export `parseToolOutput`, `parseSSEStream`, `parseSSERaw`

  **Must NOT do**:
  - Do NOT add event semantic handling in SSE parser (that's message-reducer's job)
  - Do NOT change the SSE parsing logic from the existing implementation

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with T1, T2, T3)
  - **Blocks**: T5
  - **Blocked By**: T1

  **References**:
  - `packages/ui/src/utils/messageUtils.ts` — parseToolOutput (45 lines, copy verbatim)
  - `packages/ui/src/hooks/useAgentChat.ts:169-201` — parseReconnectSSE (the SSE parsing logic to extract)
  - `packages/ui/src/providers/CopConChatProvider.ts:98-101` — JSON parse + type check (the parseSSERaw logic)

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-core && pnpm build` → exit 0
  - [ ] parseSSEStream handles: normal events, multi-line data, trailing buffer without blank line

  **QA Scenarios**:
  ```
  Scenario: SSE parser handles standard event
    Tool: Bash
    Steps:
      1. cd packages/chat-core && npx tsc --noEmit
    Expected Result: exit code 0
    Evidence: .sisyphus/evidence/task-4-utils-parser.txt
  ```

  **Commit**: YES (groups with T1, T2, T3)

---

- [x] 5. Message Reducer (applySSEChunk)

  **What to do**:
  - Create `packages/chat-core/src/message-reducer.ts`
  - Extract `applySSEChunk(originMessage, rawData) → CopConMessage` from `CopConChatProvider.transformMessage()` (L94-313)
  - Extract `createUserMessage(content) → CopConMessage` from `CopConChatProvider.transformLocalMessage()` (L81-92)
  - Extract `mergeMessages(fetched, local) → CopConMessage[]` from `useAgentChat.ts:238-253`
  - Internal handlers: `handleStepCreate`, `handlePartCreate`, `handlePartUpdate`, `handleMessageDone`
  - Helper functions: `ensureSteps(steps, stepIndex)`, `ensureParts(parts, partIndex)`, `normalizeToolCallState(raw, fallback)`
  - Handle ALL 9 SSE event types in switch: step_create → handle, part_create → handle, part_update → handle, message_done → handle, error → no-op (return unchanged), events_lost → no-op (handled at session level), async_tool_* → no-op + TODO comment
  - CRITICAL: Preserve filler-parts pattern (L188-198: `while (parts.length <= partIndex) parts.push({ type: 'text', text: '', state: 'streaming' })`)
  - CRITICAL: Preserve step 0 implicit behavior (no step_create for index 0)
  - All updates must be immutable (return new objects, never mutate input)
  - Update `src/index.ts` to export `applySSEChunk`, `createUserMessage`, `mergeMessages`

  **Must NOT do**:
  - Do NOT import from @ant-design/x-sdk (no AbstractChatProvider)
  - Do NOT modify SSE protocol semantics
  - Do NOT handle async_tool_* events (add `// TODO: handle async_tool events` comment)
  - Do NOT add JSDoc

  **Recommended Agent Profile**:
  - **Category**: `deep` — complex pure logic extraction requiring exact behavioral equivalence
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T6, T7 after T2+T4 complete)
  - **Parallel Group**: Wave 2 (with T6, T7)
  - **Blocks**: T6, T7, T8, T9, T10
  - **Blocked By**: T2, T4

  **References**:
  - `packages/ui/src/providers/CopConChatProvider.ts:94-313` — transformMessage: THE primary source. Extract all logic verbatim, removing only the class wrapper and AbstractChatProvider dependency
  - `packages/ui/src/providers/CopConChatProvider.ts:81-92` — transformLocalMessage: extract as createUserMessage
  - `packages/ui/src/hooks/useAgentChat.ts:238-253` — merge algorithm (fetched + local, dedup by id)
  - `packages/chat-core/src/types.ts` — types to use (CopConMessage, Part, Step, etc.)
  - **Key behavioral contracts to preserve**:
    - filler parts: L188-198 of CopConChatProvider (ensureParts fills with empty text parts)
    - step 0: no step_create event for stepIndex=0, rely on ensureSteps to create
    - tool-call state normalization: L162-168, L247-253 (normalizeToolCallState)
    - textDelta is incremental (L231-235: `part.text + textDelta`, NOT replacement)

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-core && pnpm build` → exit 0
  - [ ] `applySSEChunk` with `step_create` event → extends steps array correctly
  - [ ] `applySSEChunk` with `part_create` (text) → inserts TextPart at correct position
  - [ ] `applySSEChunk` with `part_create` (tool-call) → inserts ToolCallPart with normalized state
  - [ ] `applySSEChunk` with `part_update` (textDelta) → accumulates text incrementally
  - [ ] `applySSEChunk` with `part_update` (state done) → updates state
  - [ ] `applySSEChunk` with `message_done` → finalizes all streaming→done, pending/running→complete
  - [ ] `applySSEChunk` with `error` → returns message unchanged
  - [ ] Original message object is never mutated (immutable output)

  **QA Scenarios**:
  ```
  Scenario: applySSEChunk handles step_create + part_create + part_update sequence
    Tool: Bash (vitest preview)
    Steps:
      1. cd packages/chat-core && npx tsc --noEmit
    Expected Result: exit code 0
    Evidence: .sisyphus/evidence/task-5-reducer-compile.txt

  Scenario: Filler parts preserved when partIndex exceeds parts.length
    Tool: Bash
    Steps:
      1. Write inline node script that calls applySSEChunk with partIndex=2 when parts is empty
      2. Assert parts[0] and parts[1] are empty TextParts, parts[2] is the created part
    Expected Result: Behavior matches CopConChatProvider.transformMessage line 195-197
    Evidence: .sisyphus/evidence/task-5-filler-parts.txt
  ```

  **Commit**: YES
  - Message: `refactor(core): implement message-reducer with SSE event transformation`

---

- [x] 6. ChatSession Class

  **What to do**:
  - Create `packages/chat-core/src/chat-session.ts`
  - Class `ChatSession` with constructor taking `ChatSessionConfig { client, sessionId, callbacks }`
  - Public API: `start()`, `sendMessage(content)`, `abort()`, `loadMessages()`, `destroy()`
  - Private state: messages array, currentAssistantIndex, seq counter, abortController, isRequesting, isReconnecting, isStreamComplete
  - **start()**: loadMessages() + connectStream()
  - **sendMessage(content)**: optimistic add user message via createUserMessage() → connectStream with content → set state to 'streaming'
  - **connectStream(content?)**: POST to `/api/sessions/{id}/chat`, parse SSE body using parseSSEStream, apply each chunk via applySSEChunk to current assistant message, call callbacks.onMessagesChange on each update
  - **abort()**: abort SSE fetch via AbortController, set state to 'idle'
  - **loadMessages()**: client.getMessages(sessionId), map to CopConMessage[], call callbacks.onMessagesChange
  - **Reconnect flow** (private method `handleReconnect()`):
    - Triggered by SSE onError (non-AbortError, not already reconnecting)
    - Call `client.reconnect(sessionId, lastSeq + 1)`
    - If response body exists (SSE stream): parseSSEStream + applySSEChunk for each event
    - If response has no body or empty body: call loadMessages() (full fetch fallback)
    - On `events_lost` event: immediately fallback to loadMessages() + mergeMessages
    - On reconnect failure: loadMessages() + mergeMessages, reset seq, create fresh connection
  - **Seq tracking**: increment `seq` on every SSE chunk received (same as current lastReceivedSeqRef)
  - **destroy()**: abort any active stream, clear state
  - Update `src/index.ts` to export `ChatSession`, `ChatSessionConfig`

  **Must NOT do**:
  - Do NOT add retry/backoff/jitter to reconnect
  - Do NOT add async_tool_* event handling
  - Do NOT use XRequest or @ant-design/x-sdk (use raw fetch + parseSSEStream)
  - Do NOT change the seq counting approach (client-side counter)

  **Recommended Agent Profile**:
  - **Category**: `deep` — complex state machine with reconnect flow
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T7, after T3+T5 complete)
  - **Parallel Group**: Wave 2 (with T5, T7)
  - **Blocks**: T10
  - **Blocked By**: T3, T5

  **References**:
  - `packages/ui/src/hooks/useAgentChat.ts:60-107` — session init + XRequest creation (extract SSE transport setup, discard XRequest contract)
  - `packages/ui/src/hooks/useAgentChat.ts:115-152` — handleReconnect function (extract the full reconnect state machine)
  - `packages/ui/src/hooks/useAgentChat.ts:156-167` — refreshMessagesFromAPI (extract as loadMessages)
  - `packages/ui/src/hooks/useAgentChat.ts:169-201` — parseReconnectSSE (already extracted in sse-parser.ts, reuse)
  - `packages/ui/src/hooks/useAgentChat.ts:203-233` — applySSEChunk in reconnect path (reuse message-reducer)
  - `packages/ui/src/hooks/useAgentChat.ts:235-290` — refreshMessagesAndCreateFreshProvider (extract merge logic)
  - `packages/ui/src/hooks/useAgentChat.ts:82-93` — onUpdate seq tracking + isStreamComplete detection
  - `packages/ui/src/hooks/useAgentChat.ts:292-320` — load messages on session change effect (extract as loadMessages call)
  - `packages/chat-core/src/message-reducer.ts` — applySSEChunk (use this for all chunk transformations)
  - `packages/chat-core/src/sse-parser.ts` — parseSSEStream (use this for all SSE parsing)
  - `packages/chat-core/src/agent-client.ts` — AgentClient (use for API calls)

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-core && pnpm build` → exit 0
  - [ ] ChatSession can be instantiated with config
  - [ ] start() calls loadMessages + connectStream
  - [ ] sendMessage(content) creates user message + connects SSE
  - [ ] abort() cancels in-flight fetch
  - [ ] destroy() cleans up resources
  - [ ] Reconnect: on error → calls handleReconnect → fallback path works

  **QA Scenarios**:
  ```
  Scenario: ChatSession compiles and instantiates
    Tool: Bash
    Steps:
      1. cd packages/chat-core && npx tsc --noEmit
    Expected Result: exit code 0
    Evidence: .sisyphus/evidence/task-6-session-compile.txt

  Scenario: No XRequest dependency
    Tool: Bash
    Steps:
      1. grep -r "XRequest\|@ant-design\|useXChat" packages/chat-core/src/chat-session.ts || echo "CLEAN"
    Expected Result: CLEAN
    Evidence: .sisyphus/evidence/task-6-no-xrequest.txt
  ```

  **Commit**: YES
  - Message: `feat(core): implement ChatSession with reconnect state machine`

---

- [x] 7. SubagentStream Class

  **What to do**:
  - Create `packages/chat-core/src/subagent-stream.ts`
  - Class `SubagentStream` with constructor taking `SubagentStreamConfig { client, sessionId, callbacks: { onMessagesChange, onStreamingChange, onError } }`
  - Public API: `start()`, `destroy()`
  - Private state: messages array, currentMessage (in-flight assistant message), seq counter, abortController
  - **start()**: POST to `/api/sessions/{id}/chat` with `{ reconnect: true, last_event_seq: 0 }` — this gets ALL events from ring buffer beginning (per current useSubagentSSE behavior)
  - Parse SSE body using parseSSEStream, apply each chunk via applySSEChunk
  - On `message_done`: finalize currentMessage into messages array, set streaming=false
  - On error: if AbortError → ignore (clean shutdown), else → call onError
  - **destroy()**: abort fetch via AbortController
  - Update `src/index.ts` to export `SubagentStream`, `SubagentStreamConfig`

  **Must NOT do**:
  - Do NOT add reconnect/restore logic (SubagentStream is one-shot per current design)
  - Do NOT add message loading from API (subagent reads from stream only)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — simpler than ChatSession but needs precision
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T5, T6, after T3+T5 complete)
  - **Parallel Group**: Wave 2 (with T5, T6)
  - **Blocks**: T19
  - **Blocked By**: T3, T5

  **References**:
  - `packages/ui/src/hooks/useSubagentSSE.ts:39-137` — the full implementation. Key points:
    - L55-58: dummy XRequest (DISCARD — not needed in core)
    - L62-125: XRequest with callbacks (extract SSE transport + applySSEChunk calls)
    - L71-79: onSuccess → finalize currentMessage into messages
    - L81-88: onError → AbortError ignore, else setError
    - L89-121: onUpdate → applySSEChunk + message_done detection
    - L127-132: request.run with reconnect:true, last_event_seq:0 (REPLICATE this behavior)
  - `packages/chat-core/src/message-reducer.ts` — applySSEChunk
  - `packages/chat-core/src/sse-parser.ts` — parseSSEStream
  - `packages/chat-core/src/agent-client.ts` — AgentClient

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-core && pnpm build` → exit 0
  - [ ] SubagentStream connects with reconnect:true + last_event_seq:0
  - [ ] On message_done: currentMessage moves to messages array
  - [ ] destroy() aborts fetch cleanly

  **QA Scenarios**:
  ```
  Scenario: SubagentStream compiles
    Tool: Bash
    Steps:
      1. cd packages/chat-core && npx tsc --noEmit
    Expected Result: exit code 0
    Evidence: .sisyphus/evidence/task-7-subagent-compile.txt

  Scenario: Uses reconnect mode
    Tool: Bash
    Steps:
      1. grep -n "reconnect.*true\|last_event_seq.*0" packages/chat-core/src/subagent-stream.ts
    Expected Result: find both patterns present
    Evidence: .sisyphus/evidence/task-7-reconnect-mode.txt
  ```

  **Commit**: YES
  - Message: `feat(core): implement SubagentStream for subagent SSE monitoring`

---

- [x] 8. Vitest Setup + message-reducer Tests

  **What to do**:
  - Install `vitest` as devDependency in `packages/chat-core/package.json`
  - Create `vitest.config.ts` in chat-core root
  - Create `packages/chat-core/src/message-reducer.test.ts` with tests:
    - `step_create`: extends steps array, creates empty step at stepIndex
    - `part_create` (text): inserts TextPart with state 'streaming' at correct [stepIndex][partIndex]
    - `part_create` (reasoning): inserts ReasoningPart with state 'streaming'
    - `part_create` (tool-call): inserts ToolCallPart with normalized state
    - `part_update` (textDelta): accumulates text incrementally (NOT replace)
    - `part_update` (state done): updates part state to 'done'
    - `part_update` (tool-call output): sets output field
    - `part_update` (tool-call error): sets error field
    - `part_update` (tool-call interrupt): sets interrupt payload + state waiting_for_input
    - `message_done`: finalizes all streaming→done, pending/running→complete
    - `error`: returns message unchanged
    - filler-parts: partIndex > parts.length → fills gaps with empty TextParts
    - step 0 implicit: part_create with stepIndex=0 without prior step_create works
    - immutable: original message is never mutated (compare references)
    - createUserMessage: returns correct structure
    - mergeMessages: dedup by id, fetched preferred, local-only preserved
  - Add `"test": "vitest run"` script to package.json

  **Must NOT do**:
  - Do NOT import react, vue, or any DOM API in test files
  - Do NOT test async_tool_* handling (they're no-ops)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — test writing requires careful coverage analysis
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T9)
  - **Parallel Group**: Wave 3 (with T9)
  - **Blocks**: T9 (informational dependency)
  - **Blocked By**: T5

  **References**:
  - `packages/chat-core/src/message-reducer.ts` — the code under test
  - `packages/chat-core/src/types.ts` — types used in test fixtures
  - `packages/ui/src/providers/CopConChatProvider.ts:94-313` — source logic (for behavioral reference in test design)

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-core && pnpm test` → all tests pass (expect 15+ test cases)
  - [ ] No framework imports in test files
  - [ ] Tests cover: all 5 main event types + filler-parts + step 0 + immutability + createUserMessage + mergeMessages

  **QA Scenarios**:
  ```
  Scenario: All tests pass
    Tool: Bash
    Steps:
      1. cd packages/chat-core && pnpm test
    Expected Result: exit code 0, 15+ tests passed
    Evidence: .sisyphus/evidence/task-8-reducer-tests.txt

  Scenario: No framework imports in tests
    Tool: Bash
    Steps:
      1. grep -rn "react\|vue\|svelte\|document\|window" packages/chat-core/src/message-reducer.test.ts || echo "CLEAN"
    Expected Result: CLEAN
    Evidence: .sisyphus/evidence/task-8-no-framework-test-imports.txt
  ```

  **Commit**: YES (groups with T9)
  - Message: `test(core): add vitest infrastructure and message-reducer tests`

---

- [x] 9. SSE Parser Tests

  **What to do**:
  - Create `packages/chat-core/src/sse-parser.test.ts` with tests:
    - `parseSSEStream`: single event parsing (data: line + blank line)
    - `parseSSEStream`: multiple consecutive events
    - `parseSSEStream`: multi-line data (data: ... \n data: ...)
    - `parseSSEStream`: trailing buffer without final blank line
    - `parseSSEStream`: empty stream (no events)
    - `parseSSEStream`: malformed data (non-JSON) — onChunk not called for bad lines
    - `parseSSERaw`: valid JSON → returns parsed type + data
    - `parseSSERaw`: non-JSON string → returns undefined
    - `parseSSERaw`: JSON without type field → returns undefined
    - `parseToolOutput`: success JSON → status 'success'
    - `parseToolOutput`: error JSON → status 'error'
    - `parseToolOutput`: plain string → status 'success'
    - `parseToolOutput`: empty string → status 'success', output ''

  **Must NOT do**:
  - Do NOT import framework code

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T8)
  - **Parallel Group**: Wave 3 (with T8)
  - **Blocks**: T10 (gate — tests must pass before adapter is built)
  - **Blocked By**: T4, T5

  **References**:
  - `packages/chat-core/src/sse-parser.ts` — code under test
  - `packages/chat-core/src/utils.ts` — parseToolOutput under test

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-core && pnpm test` → all tests pass (expect 13+ test cases)
  - [ ] Total chat-core tests: 28+ (T8's ~15 + T9's ~13)

  **QA Scenarios**:
  ```
  Scenario: All SSE parser + utils tests pass
    Tool: Bash
    Steps:
      1. cd packages/chat-core && pnpm test
    Expected Result: exit code 0, 28+ tests total
    Evidence: .sisyphus/evidence/task-9-parser-tests.txt
  ```

  **Commit**: YES (groups with T8)

---

- [x] 10. chat-react Adapter (useChat)

  **What to do**:
  - Create `packages/chat-react/` directory
  - Initialize `package.json`: name `@copcon/chat-react`, dependencies: `@copcon/chat-core` (workspace:*), peerDependencies: `react ^18 || ^19`, devDependencies: `react`, `react-dom`, `@types/react`, `@types/react-dom`, `typescript`, `vitest`, `@vitejs/plugin-react`, `vite`
  - Add to `pnpm-workspace.yaml`
  - Create `tsconfig.json`, `vite.config.ts` (library mode)
  - Create `src/react-chat-state.ts` (~60 lines):
    - `ReactChatState` class: private state fields + listener Set
    - `subscribe(listener): () => void`
    - `getSnapshot()` returns `{ messages, state }`
    - Setters that notify listeners
  - Create `src/use-chat.ts` (~80 lines):
    - `useChat(options: { client, sessionId })` hook
    - Creates ChatSession instance with ReactChatState callbacks
    - Uses `useSyncExternalStore` to bridge ChatState → React
    - `useEffect` for lifecycle (start/destroy)
    - Returns `{ messages, status, error, sendMessage, abort }`
  - Create `src/use-subagent-stream.ts` (~50 lines):
    - `useSubagentStream(options: { client, sessionId })` hook
    - Creates SubagentStream instance
    - `useState` for messages / isStreaming / error
    - `useEffect` for lifecycle
    - Returns `{ messages, isStreaming, error }`
  - Create `src/index.ts` barrel export

  **Must NOT do**:
  - Do NOT exceed 200 total lines (count all .ts files in src/)
  - Do NOT import @ant-design/* packages
  - Do NOT re-implement SSE parsing or message transformation (delegate to ChatSession)
  - Do NOT add reconnect logic (ChatSession handles it)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — bridge pattern requires care but is straightforward
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T11, T12, T13)
  - **Parallel Group**: Wave 4 (with T11, T12, T13)
  - **Blocks**: T20
  - **Blocked By**: T6, T9 (tests must pass first)

  **References**:
  - `packages/chat-core/src/chat-session.ts` — ChatSession to instantiate
  - `packages/chat-core/src/subagent-stream.ts` — SubagentStream to instantiate
  - `packages/chat-core/src/types.ts` — ChatSessionCallbacks interface
  - `packages/ui/src/hooks/useAgentChat.ts` — current React hook to replicate the PUBLIC API shape of (messages, isRequesting, sendMessage, abort)
  - `packages/ui/src/hooks/useSubagentSSE.ts` — current subagent hook to replicate (messages, isStreaming, error)

  **Acceptance Criteria**:
  - [ ] `cd packages/chat-react && pnpm build` → exit 0
  - [ ] Total lines in `packages/chat-react/src/*.ts` < 200 (verify with `wc -l`)
  - [ ] `useChat` returns: `{ messages, status, error, sendMessage, abort }`
  - [ ] `useSubagentStream` returns: `{ messages, isStreaming, error }`
  - [ ] No @ant-design imports

  **QA Scenarios**:
  ```
  Scenario: chat-react builds successfully
    Tool: Bash
    Steps:
      1. cd packages/chat-react && pnpm install && pnpm build
    Expected Result: exit code 0
    Evidence: .sisyphus/evidence/task-10-react-build.txt

  Scenario: Line count under 200
    Tool: Bash
    Steps:
      1. wc -l packages/chat-react/src/*.ts
    Expected Result: total < 200
    Evidence: .sisyphus/evidence/task-10-line-count.txt

  Scenario: No forbidden imports
    Tool: Bash
    Steps:
      1. grep -rn "@ant-design\|useXChat\|XRequest" packages/chat-react/src/ || echo "CLEAN"
    Expected Result: CLEAN
    Evidence: .sisyphus/evidence/task-10-no-forbidden-imports.txt
  ```

  **Commit**: YES
  - Message: `feat(react): implement chat-react adapter (useChat hook)`

---

- [x] 11. createThinkingChainController

  **What to do**:
  - Create `packages/headless-hooks/` directory
  - Initialize `package.json`: name `@copcon/headless-hooks`, dependencies: `@copcon/chat-core` (workspace:*), zero other deps
  - Add to `pnpm-workspace.yaml`
  - Create `tsconfig.json`, `vite.config.ts`
  - Create `src/use-thinking-chain.ts`:
    - `createThinkingChainController(part: ReasoningPart, options?: { defaultExpanded?: boolean; autoCollapse?: boolean })`
    - State: expanded (boolean)
    - Returns: `{ expanded, toggle(), isStreaming, text, getContainerProps(), getToggleProps(), getContentProps() }`
    - getContainerProps: `{ role: 'region', 'aria-label': 'Thinking' }`
    - getToggleProps: `{ 'aria-expanded': expanded }`
    - getContentProps: `{ hidden: !expanded }`

  **Must NOT do**:
  - Do NOT use useState, useEffect, or any React/Vue/Svelte API
  - Do NOT render any DOM

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with T10, T12, T13)
  - **Blocks**: T15
  - **Blocked By**: T2

  **References**:
  - `packages/chat-core/src/types.ts` — ReasoningPart type
  - `packages/ui/src/components/SubagentCard/index.tsx:42-46` — current Think component usage (the behavior to replicate as headless)

  **Acceptance Criteria**:
  - [ ] `cd packages/headless-hooks && pnpm build` → exit 0
  - [ ] No React/Vue/Svelte imports
  - [ ] Function returns expanded/toggle/isStreaming/text + ARIA props

  **QA Scenarios**:
  ```
  Scenario: Builds without framework deps
    Tool: Bash
    Steps:
      1. cd packages/headless-hooks && pnpm install && pnpm build
    Expected Result: exit code 0
    Evidence: .sisyphus/evidence/task-11-hooks-build.txt

  Scenario: No framework imports
    Tool: Bash
    Steps:
      1. grep -rn "react\|vue\|svelte\|useState\|useEffect" packages/headless-hooks/src/ || echo "CLEAN"
    Expected Result: CLEAN
    Evidence: .sisyphus/evidence/task-11-no-framework.txt
  ```

  **Commit**: YES (groups with T12, T13)

---

- [x] 12. createToolCallController

  **What to do**:
  - Create `packages/headless-hooks/src/use-tool-call.ts`:
    - `createToolCallController(part: ToolCallPart, callbacks?: { onApprove?, onDecline? })`
    - Returns: `{ toolName, status, parsedArgs, parsedOutput, error, needsApproval, interrupt, approve(), decline(), getStatusProps() }`
    - `parsedArgs`: JSON.parse(part.args) with try/catch fallback (returns raw string on parse error)
    - `parsedOutput`: parseToolOutput(part.output) from @copcon/chat-core/utils
    - `needsApproval`: part.state === 'waiting_for_input'
    - `approve()` / `decline()`: call callbacks if provided
    - `getStatusProps()`: `{ 'aria-live': 'polite' }`

  **Must NOT do**:
  - Do NOT add form validation
  - Do NOT render DOM

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with T10, T11, T13)
  - **Blocks**: T16, T17
  - **Blocked By**: T2

  **References**:
  - `packages/chat-core/src/types.ts` — ToolCallPart, InterruptPayload
  - `packages/chat-core/src/utils.ts` — parseToolOutput
  - `packages/ui/src/components/HumanInteraction/index.tsx` — current HITL rendering (the behavior to replicate as headless)
  - `packages/ui/src/components/SubagentCard/index.tsx:13-25` — mapToolCallStatus (status mapping logic)

  **Acceptance Criteria**:
  - [ ] `cd packages/headless-hooks && pnpm build` → exit 0
  - [ ] parsedArgs handles malformed JSON gracefully
  - [ ] needsApproval is true when state === 'waiting_for_input'

  **QA Scenarios**:
  ```
  Scenario: Handles malformed args JSON
    Tool: Bash
    Steps:
      1. Write inline node script calling createToolCallController with args "not valid json"
      2. Assert parsedArgs === "not valid json" (fallback to raw string)
    Expected Result: No exception thrown, parsedArgs is raw string
    Evidence: .sisyphus/evidence/task-12-malformed-args.txt
  ```

  **Commit**: YES (groups with T11, T13)

---

- [x] 13. createMessageListController + createHitlFormController

  **What to do**:
  - Create `packages/headless-hooks/src/use-message-list.ts`:
    - `createMessageListController(options?: { autoScroll?: boolean })`
    - Returns: `{ shouldAutoScroll(isAtBottom), getItemProps(msg) }`
  - Create `packages/headless-hooks/src/use-hitl-form.ts`:
    - `createHitlFormController(interrupt: InterruptPayload, callbacks: { onSubmit, onCancel })`
    - Returns: `{ schema, required, properties, getFormFieldProps(name), handleSubmit(formData), handleCancel(), getContainerProps() }`
    - `getFormFieldProps(name)`: returns type-aware props (enum→select options, number→input number, boolean→yes/no, default→text input)
  - Create `src/index.ts` barrel export for all hooks

  **Must NOT do**:
  - Do NOT add form validation beyond what current HumanInteraction does
  - Do NOT use any framework reactivity APIs

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with T10, T11, T12)
  - **Blocks**: T17, T18
  - **Blocked By**: T2

  **References**:
  - `packages/chat-core/src/types.ts` — InterruptPayload, CopConMessage
  - `packages/ui/src/components/HumanInteraction/index.tsx:46-89` — form field rendering logic (the behavior to extract: enum/number/boolean/default dispatch)

  **Acceptance Criteria**:
  - [ ] `cd packages/headless-hooks && pnpm build` → exit 0
  - [ ] getFormFieldProps correctly dispatches enum/number/boolean/default types

  **QA Scenarios**:
  ```
  Scenario: HITL form field dispatch
    Tool: Bash
    Steps:
      1. Write inline node script: createHitlFormController with schema having enum, number, boolean, string fields
      2. Assert getFormFieldProps returns correct type hints for each
    Expected Result: enum → options array, number → type 'number', boolean → yes/no options, string → type 'text'
    Evidence: .sisyphus/evidence/task-13-hitl-form.txt
  ```

  **Commit**: YES (groups with T11, T12)

---

- [x] 14. StreamMarkdown Component

  **What to do**:
  - Create `packages/demo/src/components/StreamMarkdown.tsx`
  - Use `@ant-design/x-markdown` (`<XMarkdown>`) — already a demo dependency
  - Props: `{ content: string; isStreaming?: boolean }`
  - Simple wrapper: `<XMarkdown content={content} />`

  **Must NOT do**:
  - Do NOT implement custom markdown parsing

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5 (with T15, T16, T17, T18, T19)
  - **Blocks**: T15, T16, T19
  - **Blocked By**: None (only needs demo deps)

  **References**:
  - `packages/demo/src/App.tsx:14-16` — current MarkdownContent component (replace with StreamMarkdown export)

  **Acceptance Criteria**:
  - [ ] Component renders XMarkdown with content prop
  - [ ] `cd packages/demo && pnpm build` → exit 0 (after all Wave 5 tasks)

  **Commit**: YES (groups with T15-T19, T20)

---

- [x] 15. ThinkingBlock Reference Component

  **What to do**:
  - Create `packages/demo/src/components/ThinkingBlock.tsx`
  - Import `createThinkingChainController` from `@copcon/headless-hooks`
  - Import `StreamMarkdown` from local components
  - Use Ant Design's collapse or custom div with toggle
  - Props: `{ part: ReasoningPart }`
  - Use controller: `const ctrl = useMemo(() => createThinkingChainController(part), [part])`
  - Render: collapsible container + StreamMarkdown for content

  **Must NOT do**:
  - Do NOT import from @copcon/ui

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5 (with T14, T16, T17, T18, T19)
  - **Blocks**: T19, T20
  - **Blocked By**: T11, T14

  **References**:
  - `packages/demo/src/App.tsx:44-48` — current Think rendering (replacement target)
  - `packages/headless-hooks/src/use-thinking-chain.ts` — headless controller
  - `packages/demo/src/components/StreamMarkdown.tsx` — markdown renderer

  **Acceptance Criteria**:
  - [ ] Component renders using headless hook (no direct state management in component)
  - [ ] Toggle expand/collapse works

  **Commit**: YES (groups with T14, T16-T20)

---

- [x] 16. ToolCallCard Reference Component

  **What to do**:
  - Create `packages/demo/src/components/ToolCallCard.tsx`
  - Import `createToolCallController` from `@copcon/headless-hooks`
  - Use Ant Design Card + Tag + Descriptions or similar layout
  - Props: `{ part: ToolCallPart }`
  - Use controller: `const ctrl = useMemo(() => createToolCallController(part), [part])`
  - Render: tool name + status badge + args display + output/error

  **Must NOT do**:
  - Do NOT import from @copcon/ui

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low` — moderate complexity component
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5 (with T14, T15, T17, T18, T19)
  - **Blocks**: T19, T20
  - **Blocked By**: T12, T14

  **References**:
  - `packages/demo/src/App.tsx:50-79` — current ThoughtChain rendering (replacement target)
  - `packages/ui/src/components/SubagentCard/index.tsx:13-25` — mapToolCallStatus reference
  - `packages/headless-hooks/src/use-tool-call.ts` — headless controller

  **Acceptance Criteria**:
  - [ ] Component renders tool name, status, args, output
  - [ ] Error state shows error message

  **Commit**: YES (groups with T14, T15, T17-T20)

---

- [x] 17. HumanInteraction Reference Component

  **What to do**:
  - Create `packages/demo/src/components/HumanInteraction.tsx`
  - Import `createToolCallController` + `createHitlFormController` from `@copcon/headless-hooks`
  - Use Ant Design Form/Card/Button/Select/Input/InputNumber
  - Props: `{ part: ToolCallPart; sessionId: string }`
  - For approval interrupts: show approve/decline buttons
  - For question interrupts: render dynamic form from inputSchema
  - On respond: call `client.resume(sessionId, interrupt.interruptId, action, content)`

  **Must NOT do**:
  - Do NOT import from @copcon/ui

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5 (with T14, T15, T16, T18, T19)
  - **Blocks**: T20
  - **Blocked By**: T12, T13, T14

  **References**:
  - `packages/ui/src/components/HumanInteraction/index.tsx` — current implementation (replicate behavior, not import patterns)
  - `packages/headless-hooks/src/use-tool-call.ts` — tool call controller
  - `packages/headless-hooks/src/use-hitl-form.ts` — HITL form controller
  - `packages/demo/src/App.tsx:51-60` — current HITL usage in demo

  **Acceptance Criteria**:
  - [ ] Approval mode: renders approve + decline buttons
  - [ ] Question mode: renders dynamic form based on inputSchema
  - [ ] Form submission calls client.resume with correct params

  **Commit**: YES (groups with T14-T16, T18-T20)

---

- [x] 18. TodoList + TodoItem Reference Components

  **What to do**:
  - Create `packages/demo/src/components/TodoList.tsx` (combines TodoList + TodoItem from @copcon/ui)
  - Import `parseToolOutput` from `@copcon/chat-core`
  - Use Ant Design Tag + List or custom div layout
  - Props: `{ todos: Todo[]; onStatusChange?: (id, status) => void; readonly?: boolean }`
  - Render: sorted list (in_progress → pending → blocked → failed → completed)
  - Each item: status icon + content + optional activeForm/result
  - Use Iconify or inline SVG for status icons (replacing @ant-design/icons)

  **Must NOT do**:
  - Do NOT import from @copcon/ui or @ant-design/icons

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5 (with T14, T15, T16, T17, T19)
  - **Blocks**: T20
  - **Blocked By**: T13 (createHitlFormController — for shared patterns, though TodoList is simpler)

  **References**:
  - `packages/ui/src/components/TodoItem/index.tsx` — current TodoItem (90 lines)
  - `packages/ui/src/components/TodoList/index.tsx` — current TodoList (78 lines, includes sort logic L11-17)
  - `packages/demo/src/App.tsx:448` — current TodoList usage

  **Acceptance Criteria**:
  - [ ] Component renders sorted todo list
  - [ ] Status icons display without @ant-design/icons dependency
  - [ ] onStatusChange callback fires on click

  **Commit**: YES (groups with T14-T17, T19, T20)

---

- [x] 19. SubagentCard Reference Component

  **What to do**:
  - Create `packages/demo/src/components/SubagentCard.tsx`
  - Import `useSubagentStream` from `@copcon/chat-react`
  - Import ThinkingBlock, ToolCallCard, StreamMarkdown from local components
  - Use Ant Design Collapse + Badge
  - Props: `{ subSessionId: string; agentName?: string; autoExpand?: boolean }`
  - Use `useSubagentStream({ client, sessionId: subSessionId })` to get messages
  - Render: collapsible card with header (name + status badge) + message content
  - For each message step: render ThinkingBlock/ToolCallCard/StreamMarkdown based on part type

  **Must NOT do**:
  - Do NOT import from @copcon/ui

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5 (with T14, T15, T16, T17, T18)
  - **Blocks**: T20
  - **Blocked By**: T7, T10, T14, T15, T16

  **References**:
  - `packages/ui/src/components/SubagentCard/index.tsx` — current implementation (194 lines, replicate behavior)
  - `packages/chat-react/src/use-subagent-stream.ts` — useSubagentStream hook (from T10)
  - `packages/demo/src/components/ThinkingBlock.tsx` — local component (T15)
  - `packages/demo/src/components/ToolCallCard.tsx` — local component (T16)
  - `packages/demo/src/components/StreamMarkdown.tsx` — local component (T14)

  **Acceptance Criteria**:
  - [ ] Component renders collapsible card with subagent status
  - [ ] Message parts render with correct sub-components
  - [ ] No @copcon/ui imports

  **Commit**: YES (groups with T14-T18, T20)

---

- [x] 20. App.tsx Rewrite + Demo Integration

  **What to do**:
  - Rewrite `packages/demo/src/App.tsx` to use new packages:
    - Replace `import { AgentClient, useAgentChat, ... } from '@copcon/ui'` with imports from `@copcon/chat-core`, `@copcon/chat-react`, `@copcon/headless-hooks`
    - Replace inline `StepContent` / `renderMessageContent` with `ThinkingBlock`, `ToolCallCard`, `HumanInteraction`, `StreamMarkdown` components
    - Replace inline TodoList usage with `TodoList` component from demo
    - Replace `Think` / `ThoughtChain` from @ant-design/x with custom components using headless hooks
    - Keep `Bubble.List`, `Conversations`, `Sender`, `Welcome`, `XProvider` from @ant-design/x and antd (UI primitives, not @copcon/ui)
  - Update `packages/demo/package.json`:
    - Add `@copcon/chat-core: "workspace:*"`, `@copcon/chat-react: "workspace:*"`, `@copcon/headless-hooks: "workspace:*"`
    - Remove `@copcon/ui` dependency
  - Verify all rendering paths: text → StreamMarkdown, reasoning → ThinkingBlock, tool-call → ToolCallCard, HITL interrupt → HumanInteraction, todos → TodoList, subagent → SubagentCard

  **Must NOT do**:
  - Do NOT import from @copcon/ui
  - Do NOT change demo functionality (same UX, different implementation)
  - Do NOT remove @ant-design/x UI primitives (Bubble, Conversations, Sender, Welcome, XProvider) — these are the demo's UI choices

  **Recommended Agent Profile**:
  - **Category**: `deep` — complex integration combining all new packages
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (final integration)
  - **Parallel Group**: Wave 6 (sequential, after Wave 5)
  - **Blocks**: T21
  - **Blocked By**: T10, T15, T16, T17, T18, T19

  **References**:
  - `packages/demo/src/App.tsx` — current 472-line implementation (full behavioral spec)
  - `packages/chat-core/src/index.ts` — AgentClient, types
  - `packages/chat-react/src/index.ts` — useChat, useSubagentStream
  - `packages/headless-hooks/src/index.ts` — all controllers
  - `packages/demo/src/components/` — all 6 reference components (T14-T19)

  **Acceptance Criteria**:
  - [ ] `cd packages/demo && pnpm build` → exit 0
  - [ ] `grep -rn "@copcon/ui" packages/demo/src/` → 0 matches
  - [ ] Demo imports from all 3 new packages
  - [ ] All message part types render correctly

  **QA Scenarios**:
  ```
  Scenario: Demo builds with no @copcon/ui references
    Tool: Bash
    Steps:
      1. cd packages/demo && pnpm install && pnpm build
      2. grep -rn "@copcon/ui" packages/demo/src/ || echo "CLEAN"
    Expected Result: Build exit 0, grep returns CLEAN
    Evidence: .sisyphus/evidence/task-20-demo-build.txt

  Scenario: New package imports present
    Tool: Bash
    Steps:
      1. grep -rn "@copcon/chat-core\|@copcon/chat-react\|@copcon/headless-hooks" packages/demo/src/ | wc -l
    Expected Result: count >= 3
    Evidence: .sisyphus/evidence/task-20-imports.txt
  ```

  **Commit**: YES
  - Message: `refactor(demo): rewrite App.tsx using chat-react + headless hooks`

---

- [x] 21. Delete @copcon/ui + Workspace Cleanup

  **What to do**:
  - `rm -rf packages/ui`
  - Remove `packages/ui` entry from `pnpm-workspace.yaml`
  - Verify zero remaining references to `@copcon/ui` anywhere in workspace (`grep -r` all .ts, .tsx, .json files)
  - Run `pnpm install` from workspace root to clean lockfile

  **Must NOT do**:
  - Do NOT leave any @copcon/ui references

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T22)
  - **Parallel Group**: Wave 7 (with T22)
  - **Blocks**: F1, F2, F3
  - **Blocked By**: T20

  **References**:
  - `packages/ui/` — directory to delete
  - `pnpm-workspace.yaml` — remove packages/ui entry

  **Acceptance Criteria**:
  - [ ] `ls packages/ui 2>/dev/null || echo "DELETED"` → DELETED
  - [ ] `grep -r "@copcon/ui" . --include="*.ts" --include="*.tsx" --include="*.json" --exclude-dir=node_modules || echo "CLEAN"` → CLEAN
  - [ ] `pnpm-workspace.yaml` does not contain `packages/ui`

  **QA Scenarios**:
  ```
  Scenario: Full deletion verified
    Tool: Bash
    Steps:
      1. ls packages/ui 2>/dev/null || echo "DELETED"
      2. grep -rn "@copcon/ui" . --include="*.ts" --include="*.tsx" --include="*.json" --exclude-dir=node_modules || echo "CLEAN"
    Expected Result: DELETED + CLEAN
    Evidence: .sisyphus/evidence/task-21-cleanup.txt

  Scenario: Demo still builds after deletion
    Tool: Bash
    Steps:
      1. cd packages/demo && pnpm build
    Expected Result: exit code 0
    Evidence: .sisyphus/evidence/task-21-demo-builds.txt
  ```

  **Commit**: YES (groups with T22)

---

- [x] 22. Cross-package Integration Verification

  **What to do**:
  - Run full build chain in dependency order: chat-core → chat-react → headless-hooks → demo
  - Run chat-core tests
  - Verify dependency purity:
    - chat-core: `grep -rn "@ant-design\|from 'react'" packages/chat-core/src/` → CLEAN
    - chat-react: `wc -l packages/chat-react/src/*.ts` → total < 200
    - headless-hooks: `grep -rn "react\|vue\|svelte" packages/headless-hooks/src/` → CLEAN
  - Verify cross-package imports: each package imports from correct workspace packages
  - Report final summary

  **Must NOT do**:
  - Do NOT fix issues — report only

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T21)
  - **Parallel Group**: Wave 7 (with T21)
  - **Blocks**: F1, F2, F3
  - **Blocked By**: T20

  **Acceptance Criteria**:
  - [ ] All 4 packages build successfully
  - [ ] chat-core tests pass (28+ test cases)
  - [ ] chat-core: zero framework imports
  - [ ] chat-react: < 200 lines
  - [ ] headless-hooks: zero framework imports

  **QA Scenarios**:
  ```
  Scenario: Full build chain + dependency purity
    Tool: Bash
    Steps:
      1. cd packages/chat-core && pnpm build && pnpm test
      2. cd packages/chat-react && pnpm build
      3. cd packages/headless-hooks && pnpm build
      4. cd packages/demo && pnpm build
      5. grep -rn "@ant-design\|from 'react'" packages/chat-core/src/ || echo "core CLEAN"
      6. grep -rn "react\|vue\|svelte" packages/headless-hooks/src/ || echo "hooks CLEAN"
      7. wc -l packages/chat-react/src/*.ts
    Expected Result: All builds exit 0, all tests pass, core + hooks CLEAN, react < 200 lines
    Evidence: .sisyphus/evidence/task-22-verification.txt
  ```

  **Commit**: YES (groups with T21)
  - Message: `chore: remove @copcon/ui and verify workspace integrity`

---

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, run build command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check that chat-core has zero @ant-design/react imports. Verify filler-parts and step 0 implicit behaviors are preserved and tested.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `pnpm build` in chat-core, chat-react, headless-hooks, demo. Run `pnpm test` in chat-core. Review all new files for: hardcoded strings, empty catches, console.log in prod code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names. Verify chat-react is under 200 lines.
  Output: `Build [PASS/FAIL] | Tests [N pass/N fail] | chat-react lines [N] | Files [N clean/N issues] | VERDICT`

- [ ] F3. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual files created/modified. Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check that NO async_tool_* handling was added to message-reducer. Check that NO retry/backoff was added to reconnect. Check NO chat-vue/chat-svelte stubs exist. Verify @copcon/ui is fully deleted.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- T1-T4: `refactor(ui): extract chat-core package scaffold and foundational modules`
- T5: `refactor(core): implement message-reducer with SSE event transformation`
- T6: `feat(core): implement ChatSession with reconnect state machine`
- T7: `feat(core): implement SubagentStream for subagent SSE monitoring`
- T8-T9: `test(core): add vitest infrastructure and pure function tests`
- T10: `feat(react): implement chat-react adapter (useChat hook)`
- T11-T13: `feat(hooks): implement headless hooks package`
- T14-T19: `feat(demo): build reference components using headless hooks`
- T20: `refactor(demo): rewrite App.tsx using chat-react + headless hooks`
- T21-T22: `chore: remove @copcon/ui and verify workspace integrity`

---

## Success Criteria

### Verification Commands
```bash
# Core: zero framework deps
cd packages/chat-core && cat package.json | jq '.dependencies // {} | keys'  # Expected: []
cd packages/chat-core && pnpm build  # Expected: exit 0
cd packages/chat-core && pnpm test   # Expected: all pass

# React adapter: thin
wc -l packages/chat-react/src/*.ts  # Expected: total < 200
cd packages/chat-react && pnpm build  # Expected: exit 0

# Headless hooks: pure functions
cd packages/headless-hooks && pnpm build  # Expected: exit 0

# Demo: no @copcon/ui dependency
grep -r "@copcon/ui" packages/demo/src/  # Expected: 0 matches
cd packages/demo && pnpm build  # Expected: exit 0

# @copcon/ui deleted
ls packages/ui 2>/dev/null || echo "DELETED"  # Expected: DELETED

# No forbidden imports in chat-core
grep -r "@ant-design\|from 'react'" packages/chat-core/src/  # Expected: 0 matches
```

### Final Checklist
- [ ] All "Must Have" present and verified
- [ ] All "Must NOT Have" absent (search verified)
- [ ] All tests pass in chat-core
- [ ] Demo builds successfully
- [ ] @copcon/ui fully removed
- [ ] No regression in existing demo functionality
