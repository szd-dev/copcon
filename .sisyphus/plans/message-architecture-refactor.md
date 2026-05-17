# 消息架构重构

## TL;DR

> **Quick Summary**: 基于 docs/message-architecture-design.md 设计说明书，将 SSE 协议从混合层级（模型层+UI层事件）重构为纯 UI 层事件（step_create/part_create/part_update/message_done），数据模型从扁平 parts 数组重构为 steps 嵌套结构，修复 6 个已知关键 bug，统一流式路径和刷新路径的数据形状。
> 
> **Deliverables**:
> - 后端：新的 SSE 事件协议（4 种事件，camelCase 字段，二级索引）
> - 后端：UIMessage 含 Steps 的持久化模型，tool-call output 内嵌
> - 后端：GetMessages API 返回 steps 结构
> - 前端：CopConChatProvider 只处理 UI 层事件
> - 前端：刷新路径直接消费 API 响应，无需转换
> - 前端：按 Step 渲染，ThoughtChain 展示工具调用
> - 数据迁移：legacy Parts JSONB 添加 stepIndex，snake_case→camelCase
> 
> **Estimated Effort**: Large
> **Parallel Execution**: YES - 4 waves
> **Critical Path**: T1 → T3 → T7 → T8 → T10 → T12

---

## Context

### Original Request
前后端消息结构重构后更加混乱：消息结构处理错误、响应消息没有处理、刷新后UI展示与初次不同、后端消息时序混乱、reasoning和part_update事件重复、工具调用后前端停处理、工具结果未传递给AI。根本原因：没有底层架构设计。

### Interview Summary
**Key Discussions**:
- 6 个关键 bug 的根因分析：双协议并存、类型断裂、索引冲突、持久化不完整、刷新路径无归一化
- 协议层级问题：reasoning/message 是模型层事件，part_create/part_update 是 UI 层事件，不应混发
- 迭代模型：useXChat 1请求=1消息约束，选择单消息+steps嵌套方案
- 持久化模型：tool-call output 内嵌，不再需要 role=tool 消息
- 字段命名：统一 camelCase

**Research Findings**:
- useXChat 无法从单个 SSE 流产出多条 assistant 消息
- Go `any` 类型 + TS `as string` = 运行时 crash（tool_result 的 result 是 object）
- JSON tag snake_case vs TypeScript camelCase 不匹配
- buildUIParts 持久化 tool-call state 为 "pending"、output 为空
- part_index 在 Agent Loop 迭代间重置，与已有 parts 冲突

### Metis Review
**Identified Gaps** (addressed):
- 异步工具在 SSE 关闭后完成：保留 GetSessionUpdates 轮询机制，异步工具的 part_update 通过轮询端点获取
- JSON tag 重命名导致 legacy 数据反序列化失败：Go Scan 方法兼容两种命名
- Phase 1 双发期间部署顺序：后端先于前端部署
- User 消息也需 steps 格式：单 step，单 text part
- ConvertToModelMessages 必须与 UIMessage 结构变更同步
- 前端在过渡期需同时处理新旧事件格式

---

## Work Objectives

### Core Objective
基于设计说明书重构消息架构，实现纯 UI 层 SSE 协议 + steps 嵌套数据模型 + 流式/刷新路径统一。

### Concrete Deliverables
- `server/internal/domain/entity/event.go` — 新事件类型和数据结构
- `server/internal/domain/entity/ui_message.go` — UIMessage 含 Steps
- `server/internal/domain/entity/convert.go` — 更新 ConvertToModelMessages
- `server/internal/agent/engine.go` — 只发射新事件
- `server/internal/agent/engine_tools.go` — 只发射新事件
- `server/internal/api/handlers.go` — GetMessages 返回 steps
- `server/internal/chat_context/manager.go` — 从 steps 重建 UIMessage
- `server/internal/session/model.go` — PersistedPart 结构
- `packages/ui/src/api/types.ts` — 新 UIMessage/Step/Part 类型
- `packages/ui/src/providers/CopConChatProvider.ts` — 只处理 UI 层事件
- `packages/ui/src/hooks/useAgentChat.ts` — 简化加载逻辑
- `packages/ui/src/utils/messageUtils.ts` — 删除 mergeToolMessages
- `packages/demo/src/App.tsx` — Step 渲染逻辑

### Definition of Done
- [ ] `curl` SSE 输出只含 step_create/part_create/part_update/message_done/error
- [ ] `curl` SSE 字段名全部 camelCase
- [ ] `go test ./internal/domain/entity/... -v` 全部 PASS
- [ ] `cd packages/ui && npx tsc --noEmit` 零错误
- [ ] 流式完成后刷新页面，UI 展示完全一致

### Must Have
- SSE 协议只发射 UI 层事件
- stepIndex + partIndex 二级索引
- output 总是 string（后端序列化）
- 字段命名统一 camelCase
- tool-call output 内嵌在 Parts JSONB
- GetMessages API 返回 steps 结构
- 刷新路径无需额外转换
- Legacy 数据兼容（stepIndex=0，Scan 兼容两种命名）

### Must NOT Have (Guardrails)
- 不删除 role=tool DB 行（BuildContext 仍需要）
- 不修改 ChatContext channel 机制
- 不添加 API 版本控制
- 不构建 snake_case→camelCase 转换工具
- 不在数据管道重构任务中同时改渲染逻辑
- 不在 Phase 1 删除 legacy 事件发射
- 不修改 SSE 信封格式
- 不复用旧 PartCreateData.Data map[string]any 结构

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed.

### Test Decision
- **Infrastructure exists**: YES (Go testing, no frontend test framework)
- **Automated tests**: Tests-after
- **Framework**: Go testing + testify (backend)

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Foundation — 类型定义，互不依赖):
├── Task 1: 后端事件和数据模型定义 [quick]
├── Task 2: 前端类型定义 [quick]
└── Task 3: 后端持久化模型更新 [quick]

Wave 2 (Core Logic — 依赖 Wave 1):
├── Task 4: 后端 handleStreaming 重写 [deep]
├── Task 5: 后端 handleToolCalls + engine_tools 重写 [deep]
├── Task 6: 后端 persistMessage + buildUIParts 重写 [quick]
└── Task 7: 后端 GetMessages API + BuildContext 更新 [unspecified-high]

Wave 3 (Frontend — 依赖 Wave 2):
├── Task 8: 前端 CopConChatProvider 重写 [deep]
└── Task 9: 前端 useAgentChat + messageUtils 简化 [quick]

Wave 4 (Integration — 依赖 Wave 3):
├── Task 10: 前端渲染逻辑更新 [visual-engineering]
├── Task 11: Legacy 数据迁移 + 兼容处理 [deep]
└── Task 12: 端到端验证 + 清理 [unspecified-high]

Wave FINAL (Review):
├── Task F1: Plan compliance audit [oracle]
├── Task F2: Code quality review [unspecified-high]
├── Task F3: Real manual QA [unspecified-high]
└── Task F4: Scope fidelity check [deep]
```

### Dependency Matrix

| Task | Depends On | Blocks | Wave |
|------|-----------|--------|------|
| T1 | — | T4, T5, T6, T7 | 1 |
| T2 | — | T8, T9, T10 | 1 |
| T3 | T1 | T6, T7, T11 | 1 |
| T4 | T1 | T8 | 2 |
| T5 | T1 | T8 | 2 |
| T6 | T1, T3 | T11 | 2 |
| T7 | T1, T3 | T11 | 2 |
| T8 | T2, T4, T5 | T10 | 3 |
| T9 | T2, T8 | T10 | 3 |
| T10 | T8, T9 | T12 | 4 |
| T11 | T6, T7 | T12 | 4 |
| T12 | T10, T11 | F1-F4 | 4 |

### Agent Dispatch Summary

- **Wave 1**: 3 tasks — T1→`quick`, T2→`quick`, T3→`quick`
- **Wave 2**: 4 tasks — T4→`deep`, T5→`deep`, T6→`quick`, T7→`unspecified-high`
- **Wave 3**: 2 tasks — T8→`deep`, T9→`quick`
- **Wave 4**: 3 tasks — T10→`visual-engineering`, T11→`deep`, T12→`unspecified-high`
- **FINAL**: 4 tasks — F1→`oracle`, F2→`unspecified-high`, F3→`unspecified-high`, F4→`deep`

---

## Design Specification (Embedded from docs/message-architecture-design.md)

### Data Model

**UIMessage**:
```typescript
interface UIMessage {
  id: string;
  role: 'user' | 'assistant';
  steps: Step[];
  metadata: { createdAt: string; model?: string; tokenCount?: number; durationMs?: number; };
}
```

**Step**:
```typescript
interface Step {
  parts: Part[];
  status: 'streaming' | 'done';
}
```

**Part** (discriminated union):
```typescript
type Part = TextPart | ReasoningPart | ToolCallPart;
interface TextPart { type: 'text'; text: string; state: 'streaming' | 'done'; }
interface ReasoningPart { type: 'reasoning'; text: string; state: 'streaming' | 'done'; }
interface ToolCallPart {
  type: 'tool-call'; toolCallId: string; toolName: string; args: string;
  output: string; error: string; state: 'pending' | 'running' | 'complete' | 'error';
}
```

### SSE Protocol — 4 Events Only

**step_create**:
```typescript
{ type: 'step_create', data: { messageId: string, stepIndex: number } }
```

**part_create** (discriminated by partType):
```typescript
{ type: 'part_create', data: { messageId, stepIndex, partIndex, partType: 'text', state: 'streaming' } }
{ type: 'part_create', data: { messageId, stepIndex, partIndex, partType: 'reasoning', state: 'streaming' } }
{ type: 'part_create', data: { messageId, stepIndex, partIndex, partType: 'tool-call', toolCallId, toolName, args, state: 'pending' } }
```

**part_update** (discriminated by partType):
```typescript
{ type: 'part_update', data: { messageId, stepIndex, partIndex, partType: 'text', textDelta?, state? } }
{ type: 'part_update', data: { messageId, stepIndex, partIndex, partType: 'reasoning', textDelta?, state? } }
{ type: 'part_update', data: { messageId, stepIndex, partIndex, partType: 'tool-call', state?, output?, error? } }
```

**message_done**:
```typescript
{ type: 'message_done', data: { messageId: string } }
```

### Key Design Rules
1. output 总是 string（后端 json.Marshal 序列化后再发射）
2. 字段命名统一 camelCase（Go JSON tag 和 TypeScript 一致）
3. stepIndex + partIndex 二级索引（partIndex 在每个 step 内从 0 开始）
4. tool-call output 内嵌，不需要 role=tool 消息用于 UI（BuildContext 仍写 role=tool 行供 LLM 用）
5. User 消息也用 steps：`[{ parts: [{ type: 'text', text: '...', state: 'done' }] }]`
6. Legacy 数据 stepIndex 默认 0，Go Scan 兼容 snake_case 和 camelCase
7. 异步工具在 SSE 关闭后完成：保留 GetSessionUpdates 轮询机制不变

---

## TODOs

- [x] 1. 后端事件和数据模型定义

  **What to do**:
  - 在 `event.go` 中定义新事件类型常量：`EventStepCreate`, `EventPartCreate`（重定义）, `EventPartUpdate`（重定义）, `EventMessageDone`（保留）
  - 删除旧事件常量：`EventMessage`, `EventReasoning`, `EventToolCall`, `EventToolResult`, `EventThought`, `EventDone`（标记 Deprecated，暂不删除以保持编译）
  - 定义新事件数据结构（全部 camelCase JSON tag）：
    - `StepCreateData { MessageID string, StepIndex int }`
    - `PartCreateData { MessageID, StepIndex, PartIndex int, PartType string, State string, ToolCallID string, ToolName string, Args string }`（扁平化，不嵌套 Data map）
    - `PartUpdateData { MessageID, StepIndex, PartIndex int, PartType string, TextDelta string, State string, Output string, Error string }`（Output 从 `any` 改为 `string`）
    - `MessageDoneData` 保持不变
  - 在 `ui_message.go` 中更新 `UIMessage` 结构：添加 `Steps []UIStep` 字段
  - 定义 `UIStep` 结构：`{ Parts []UIPart, State UIPartState }`
  - 更新 `UIPart` 的 JSON tag 从 snake_case 改为 camelCase（`tool_call_id` → `toolCallId`，`tool_name` → `toolName`）
  - 扩展 `UIPartState` 枚举：添加 `"pending"`, `"running"`, `"complete"`, `"error"`
  - 在 `UIPart` 中添加 `StepIndex int` 字段（JSON tag: `stepIndex`）
  - 更新 `convert.go` 中的 `ConvertToModelMessages` 以遍历 `msg.Steps[].Parts`（而非 `msg.Parts`），保持输出格式不变
  - 更新 `convert.go` 的 `convertAssistantMessage` 以嵌套遍历 steps，拼接跨 step 的 text parts，收集跨 steps 的 tool-call parts

  **Must NOT do**:
  - 不删除旧事件常量（标记 Deprecated 即可）
  - 不修改 Event struct 的 `Data any` 字段（保持兼容）
  - 不修改 ChatContext 接口
  - 不修改 SSE 信封格式

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 纯类型定义和结构体修改，逻辑简单
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with T2, T3)
  - **Blocks**: T4, T5, T6, T7
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `server/internal/domain/entity/event.go` — 当前所有事件类型和数据结构定义，新定义必须替换/扩展这些
  - `server/internal/domain/entity/ui_message.go` — 当前 UIMessage/UIPart 定义，需添加 Steps 和更新 JSON tag
  - `server/internal/domain/entity/convert.go` — ConvertToModelMessages 当前遍历 msg.Parts，需改为 msg.Steps[].Parts

  **Test References**:
  - `server/internal/domain/entity/convert_test.go` — 现有测试用例，修改后必须仍 PASS

  **WHY Each Reference Matters**:
  - `event.go`: 这是所有事件类型的单一来源，新协议的类型和结构全部定义在此
  - `ui_message.go`: UIMessage 是前后端共享的数据契约，Steps 字段是最核心的结构变更
  - `convert.go`: UIMessage→ModelMessage 转换是给 LLM 构建上下文的关键路径，Steps 嵌套遍历必须正确

  **Acceptance Criteria**:

  - [ ] `go build ./...` 编译通过
  - [ ] `go test ./internal/domain/entity/... -v` 全部 PASS（包括 convert_test.go）
  - [ ] 新事件常量已定义，旧事件常量标记 Deprecated
  - [ ] UIMessage 包含 `Steps []UIStep` 字段
  - [ ] UIPart 的 JSON tag 为 camelCase
  - [ ] UIPartState 包含 pending/running/complete/error
  - [ ] ConvertToModelMessages 正确遍历 Steps 嵌套结构

  **QA Scenarios**:

  ```
  Scenario: ConvertToModelMessages 处理多 step 的 assistant 消息
    Tool: Bash (go test)
    Preconditions: 新的 UIMessage 结构已定义
    Steps:
      1. 在 convert_test.go 中添加测试用例：UIMessage 含 2 个 steps，第一个 step 有 reasoning + text + tool-call，第二个 step 有 text
      2. 运行 go test ./internal/domain/entity/... -v -run TestConvert
      3. 验证输出：assistant message 的 content 为两个 step 的 text 拼接，toolCalls 包含第一个 step 的 tool-call，且有一条 role=tool 的 message
    Expected Result: 测试 PASS，content 拼接正确，tool-call 转换正确
    Failure Indicators: 测试 FAIL，content 为空，或 tool-call 缺失
    Evidence: .sisyphus/evidence/task-1-convert-test.txt

  Scenario: 旧事件常量仍可引用（编译兼容）
    Tool: Bash
    Preconditions: 旧常量标记为 Deprecated
    Steps:
      1. go build ./server/...
      2. 验证编译通过，无 undefined 错误
    Expected Result: 编译成功
    Failure Indicators: 编译失败
    Evidence: .sisyphus/evidence/task-1-build-compat.txt
  ```

  **Commit**: YES (groups with T2, T3)
  - Message: `refactor(domain): define new SSE event types and UIMessage with Steps`
  - Files: `server/internal/domain/entity/event.go, ui_message.go, convert.go, convert_test.go`
  - Pre-commit: `cd server && go test ./internal/domain/entity/... -v`

- [x] 2. 前端类型定义

  **What to do**:
  - 在 `packages/ui/src/api/types.ts` 中定义新类型：
    - `UIMessage { id, role: 'user'|'assistant', steps: Step[], metadata }`
    - `Step { parts: Part[], status: 'streaming'|'done' }`
    - `Part = TextPart | ReasoningPart | ToolCallPart`（discriminated union）
    - `TextPart { type: 'text', text: string, state: 'streaming'|'done' }`
    - `ReasoningPart { type: 'reasoning', text: string, state: 'streaming'|'done' }`
    - `ToolCallPart { type: 'tool-call', toolCallId: string, toolName: string, args: string, output: string, error: string, state: 'pending'|'running'|'complete'|'error' }`
  - 定义新 SSE 事件类型：
    - `StepCreateEvent { type: 'step_create', data: { messageId, stepIndex } }`
    - `PartCreateEvent { type: 'part_create', data: { messageId, stepIndex, partIndex, partType, ...partTypeSpecificFields } }`
    - `PartUpdateEvent { type: 'part_update', data: { messageId, stepIndex, partIndex, partType, ...updateFields } }`
    - `MessageDoneEvent { type: 'message_done', data: { messageId } }`
  - 保留旧的 `Message` interface（过渡期使用，标记 @deprecated）
  - 删除不再需要的旧 SSE 类型：`SSEEvent`, `SSEEventType`, `AsyncToolStartedData` 等
  - 删除旧的扁平 `UIPart`/`UIMessage` 类型（被新类型替代）
  - 删除 `PartCreateEvent`/`PartUpdateEvent` 的旧定义（使用新定义）

  **Must NOT do**:
  - 不删除 `Message` interface（useAgentChat 加载路径过渡期仍需）
  - 不修改 CopConChatProvider（T8 处理）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 纯类型定义，无逻辑
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with T1, T3)
  - **Blocks**: T8, T9, T10
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `packages/ui/src/api/types.ts:102-175` — 当前 UIPart/UIMessage/PartCreateEvent/PartUpdateEvent 定义，新类型替代这些
  - `packages/ui/src/providers/CopConChatProvider.ts:16-32` — CopConMessage 接口，理解当前运行时消息结构

  **WHY Each Reference Matters**:
  - `types.ts`: 这是前端所有类型的单一来源，新类型定义必须完整且与后端一致
  - `CopConChatProvider.ts`: CopConMessage 是运行时实际使用的类型，理解它才能正确迁移

  **Acceptance Criteria**:

  - [ ] `cd packages/ui && npx tsc --noEmit` 零错误
  - [ ] 新 UIMessage/Step/Part 类型已定义
  - [ ] 新 SSE 事件类型已定义（step_create/part_create/part_update/message_done）
  - [ ] 旧 SSEEventType 和旧 PartCreateEvent/PartUpdateEvent 已删除
  - [ ] ToolCallPart.output 类型为 `string`（非 `unknown`/`any`）

  **QA Scenarios**:

  ```
  Scenario: TypeScript 严格模式编译通过
    Tool: Bash
    Preconditions: 新类型定义完成
    Steps:
      1. cd packages/ui && npx tsc --noEmit
      2. 验证零错误
    Expected Result: 编译成功，零错误
    Failure Indicators: 类型错误
    Evidence: .sisyphus/evidence/task-2-tsc.txt

  Scenario: 新类型与后端 JSON 格式对齐
    Tool: Bash
    Preconditions: 后端 event.go 已定义新结构
    Steps:
      1. 对比前端 PartCreateEvent 的字段名与后端 PartCreateData 的 JSON tag
      2. 验证完全一致（camelCase）
    Expected Result: 字段名完全匹配
    Failure Indicators: 任何字段名不一致
    Evidence: .sisyphus/evidence/task-2-field-alignment.txt
  ```

  **Commit**: YES (groups with T1, T3)
  - Message: `refactor(ui): define new UIMessage/Step/Part TypeScript types`
  - Files: `packages/ui/src/api/types.ts`
  - Pre-commit: `cd packages/ui && npx tsc --noEmit`

- [x] 3. 后端持久化模型更新

  **What to do**:
  - 在 `session/model.go` 中定义 `PersistedPart` 结构体替代 `UIParts []map[string]any`：
    ```go
    type PersistedPart struct {
        Type       string `json:"type"`
        Text       string `json:"text,omitempty"`
        State      string `json:"state"`
        ToolCallID string `json:"toolCallId,omitempty"`
        ToolName   string `json:"toolName,omitempty"`
        Args       string `json:"args,omitempty"`
        Output     string `json:"output,omitempty"`
        Error      string `json:"error,omitempty"`
        StepIndex  int    `json:"stepIndex"`
    }
    ```
  - 将 `Message.Parts` 字段类型从 `UIParts` 改为新的 `PersistedParts` 类型
  - 实现 `PersistedParts` 的 `Value()`/`Scan()` GORM 接口（兼容旧的 `[]map[string]any` 格式）
  - `Scan()` 方法必须同时支持 camelCase 和 snake_case JSON key（legacy 兼容）
  - 当 `StepIndex` 字段不存在时默认为 0

  **Must NOT do**:
  - 不删除 `UIParts` 类型定义（其他代码可能引用，标记 Deprecated）
  - 不修改 `Message` 表结构（GORM AutoMigrate 处理 JSONB 类型变更）
  - 不删除 `ToolCalls` 和 `Content`/`Reasoning` 列（BuildContext legacy 路径仍需要）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 结构体定义和 GORM 接口实现
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（但依赖 T1 的 UIMessage/UIPart 类型定义参考）
  - **Parallel Group**: Wave 1 (with T1, T2)
  - **Blocks**: T6, T7, T11
  - **Blocked By**: T1（需要知道 UIPart 的 JSON tag 命名规范）

  **References**:

  **Pattern References**:
  - `server/internal/session/model.go:38-83` — 当前 UIParts 类型和 Value/Scan 实现，新 PersistedParts 必须遵循相同的 GORM 接口模式
  - `server/internal/session/model.go:143-156` — Message struct，Parts 字段需要更新类型

  **API/Type References**:
  - `server/internal/domain/entity/ui_message.go` — UIPart 定义（来自 T1），JSON tag 命名必须与 PersistedPart 一致

  **WHY Each Reference Matters**:
  - `model.go UIParts`: 这是 GORM JSONB 序列化的核心，Scan/Value 实现错误会导致数据读写失败
  - `model.go Message`: Parts 字段类型变更直接影响所有读写该字段的代码

  **Acceptance Criteria**:

  - [ ] `go build ./server/...` 编译通过
  - [ ] `PersistedPart` 结构体定义完成，JSON tag 全部 camelCase
  - [ ] `PersistedParts.Scan()` 能同时解析 camelCase 和 snake_case 的 JSON key
  - [ ] `PersistedParts.Scan()` 在 `stepIndex` 缺失时默认为 0
  - [ ] `Message.Parts` 字段类型更新为 `PersistedParts`

  **QA Scenarios**:

  ```
  Scenario: Scan 兼容 legacy snake_case JSON
    Tool: Bash (go test)
    Preconditions: PersistedParts 实现完成
    Steps:
      1. 编写测试：用 snake_case JSON（如 {"type":"tool-call","tool_call_id":"call_123","tool_name":"read_file","state":"done"}）调用 Scan
      2. 验证 ToolCallID = "call_123", ToolName = "read_file"
      3. 验证 StepIndex = 0（默认值）
    Expected Result: Scan 成功，字段正确映射
    Failure Indicators: ToolCallID 或 ToolName 为空
    Evidence: .sisyphus/evidence/task-3-scan-compat.txt

  Scenario: Scan 正确解析新 camelCase JSON
    Tool: Bash (go test)
    Preconditions: PersistedParts 实现完成
    Steps:
      1. 编写测试：用 camelCase JSON（如 {"type":"tool-call","toolCallId":"call_123","toolName":"read_file","state":"complete","stepIndex":1}）调用 Scan
      2. 验证所有字段正确，StepIndex = 1
    Expected Result: Scan 成功，字段正确
    Failure Indicators: 任何字段为空或不匹配
    Evidence: .sisyphus/evidence/task-3-scan-camelcase.txt
  ```

  **Commit**: YES (groups with T1, T2)
  - Message: `refactor(session): update PersistedPart with camelCase JSON tags and stepIndex`
  - Files: `server/internal/session/model.go`
  - Pre-commit: `cd server && go build ./... && go test ./internal/session/... -v`

- [x] 4. 后端 handleStreaming 重写

  **What to do**:
  - 重写 `engine.go:handleStreaming()` 方法，只发射新 UI 层事件
  - 维护 `stepIndex` 状态变量（由 runAgentLoop 传入，handleStreaming 不管理）
  - 维护 `partIndex` 状态变量（在当前 step 内递增，每次 handleStreaming 调用从 0 开始）
  - 替换所有事件发射：
    - 删除 `EventMessage` 和 `EventReasoning` 发射
    - LLM 首次产出 content：发射 `part_create(partType='text', state='streaming')` + `part_update(textDelta=content)`
    - LLM 后续 content chunk：只发射 `part_update(textDelta=content)`
    - LLM 首次产出 reasoning：发射 `part_create(partType='reasoning', state='streaming')` + `part_update(textDelta=reasoning)`
    - LLM 后续 reasoning chunk：只发射 `part_update(textDelta=reasoning)`
    - 流式结束时：发射 `part_update(state='done')` 标记各 part 完成
  - 删除 `textPartCreated`, `reasoningPartCreated` 等旧状态追踪（替换为 partIndex 计数）
  - 返回的 `StreamResult` 增加 `StepIndex` 字段
  - 确保 `runAgentLoop` 在调用 `handleStreaming` 之前发射 `step_create` 事件

  **Must NOT do**:
  - 不发射任何 legacy 事件（EventMessage, EventReasoning）
  - 不修改 StreamResult 的 Content/ReasoningContent 累积逻辑（仍需用于持久化）
  - 不修改 handleStreaming 的函数签名（除了添加 stepIndex 参数）
  - 不修改 tool_calls 的累积逻辑（仍需用于 handleToolCalls）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 核心流式处理逻辑，需仔细处理状态变量和事件发射顺序
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T5, T6, T7)
  - **Parallel Group**: Wave 2 (with T5, T6, T7)
  - **Blocks**: T8
  - **Blocked By**: T1

  **References**:

  **Pattern References**:
  - `server/internal/agent/engine.go:133-345` — 当前 handleStreaming 实现，包含所有需要替换的双重发射逻辑
  - `server/internal/agent/engine.go:362-419` — runAgentLoop，step_create 的发射位置

  **API/Type References**:
  - `server/internal/domain/entity/event.go` — 新事件类型定义（来自 T1）

  **WHY Each Reference Matters**:
  - `engine.go handleStreaming`: 这是 SSE 事件发射的核心，每一行 Emit 调用都需要审查和替换
  - `engine.go runAgentLoop`: step_create 事件需要在 handleStreaming 之前发射，理解迭代流程是关键

  **Acceptance Criteria**:

  - [ ] handleStreaming 中零 `EventMessage`/`EventReasoning` 发射
  - [ ] 每个 LLM chunk 只发射 1 个事件（part_update），不再双重发射
  - [ ] partIndex 在当前 step 内正确递增
  - [ ] `go build ./server/...` 编译通过

  **QA Scenarios**:

  ```
  Scenario: SSE 输出只含新事件类型
    Tool: Bash (curl + jq)
    Preconditions: 后端运行中，session 存在
    Steps:
      1. curl -s -N -X POST localhost:8080/api/sessions/{SID}/chat -d '{"content":"hello"}'
      2. 管道到 jq -c '.type' | sort | uniq -c
      3. 验证只有 step_create, part_create, part_update, message_done, error
    Expected Result: 无 message/reasoning/tool_call/tool_result/done 事件
    Failure Indicators: 出现任何 legacy 事件类型
    Evidence: .sisyphus/evidence/task-4-sse-types.txt

  Scenario: 每个 LLM chunk 只发射 1 个 part_update
    Tool: Bash (curl)
    Preconditions: 后端运行中
    Steps:
      1. curl -s -N ... | 统计事件数量
      2. 对于纯文本回复，验证 part_create 后只有 part_update 事件（无 message 事件）
    Expected Result: 事件数量约为旧系统的一半
    Failure Indicators: 出现多余事件
    Evidence: .sisyphus/evidence/task-4-event-count.txt
  ```

  **Commit**: YES (groups with T5)
  - Message: `refactor(agent): rewrite handleStreaming for new SSE protocol`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `cd server && go build ./...`

- [x] 5. 后端 handleToolCalls + engine_tools 重写

  **What to do**:
  - 重写 `engine_tools.go:handleToolCalls()` — 只发射新事件：
    - 删除 `EventToolCall` 发射，替换为 `part_create(partType='tool-call', toolCallId, toolName, args, state='pending')`
    - tool-call 的 partIndex 继续当前 step 的 partIndex 递增
    - 删除 `EventDone` 发射，替换为 `EventMessageDone`（仅无 tool-calls 时）
  - 重写 `executeSync()` — 只发射新事件：
    - 删除 `EventToolCall` 和 `EventToolResult` 发射
    - 替换为 `part_update(state='running')` + `part_update(state='complete', output=jsonString)` 或 `part_update(state='error', error=msg)`
    - **关键**：output 字段必须先 `json.Marshal(result)` 转为 string 再发射
  - 重写 `executeConcurrent()` — 同理，只发射 part_update
  - 重写 `executeAsync()` — 只发射 part_update（同步部分），异步完成后的处理保留 GetSessionUpdates 轮询机制不变
  - `runAgentLoop` 中删除旧 `step-start` part_create，替换为在迭代开始时发射 `step_create(stepIndex)`

  **Must NOT do**:
  - 不发射任何 legacy 事件（EventToolCall, EventToolResult, EventDone）
  - 不修改工具执行逻辑本身（只改事件发射）
  - 不修改 role=tool 消息的写入（BuildContext 仍需要）
  - 不修改异步工具的 goroutine 管理和 registry

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 多个执行路径（sync/concurrent/async），需逐一替换事件发射
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T4, T6, T7)
  - **Parallel Group**: Wave 2
  - **Blocks**: T8
  - **Blocked By**: T1

  **References**:

  **Pattern References**:
  - `server/internal/agent/engine_tools.go:76-150` — executeSync 当前实现，包含 legacy + part 双重发射
  - `server/internal/agent/engine_tools.go:152-343` — executeAsync 当前实现
  - `server/internal/agent/engine_tools.go:346-477` — executeConcurrent 当前实现
  - `server/internal/agent/engine_tools.go:483-584` — handleToolCalls 当前实现

  **API/Type References**:
  - `server/internal/domain/entity/event.go` — 新事件类型定义（来自 T1）

  **WHY Each Reference Matters**:
  - `engine_tools.go`: 每个执行路径都有独立的 Emit 调用，需逐一替换。output 序列化是关键改动点。

  **Acceptance Criteria**:

  - [ ] handleToolCalls/executeSync/executeConcurrent/executeAsync 中零 legacy 事件发射
  - [ ] tool-call output 总是 string（`json.Marshal` 序列化后）
  - [ ] `go build ./server/...` 编译通过
  - [ ] step_create 在每次迭代开始时正确发射

  **QA Scenarios**:

  ```
  Scenario: 工具调用只发射 part_create 和 part_update
    Tool: Bash (curl + jq)
    Preconditions: 后端运行中，存在可调用的工具
    Steps:
      1. 发送需要工具调用的消息（如"列出当前目录文件"）
      2. 捕获完整 SSE 流
      3. 过滤 tool-call 相关事件
      4. 验证：只有 part_create(tool-call) 和 part_update(state/output) 事件
    Expected Result: 无 tool_call/tool_result 事件
    Failure Indicators: 出现 legacy 事件
    Evidence: .sisyphus/evidence/task-5-tool-events.txt

  Scenario: tool output 是 string 不是 object
    Tool: Bash (curl + jq)
    Preconditions: 后端运行中
    Steps:
      1. 发送触发工具调用的消息
      2. 过滤含 output 字段的 part_update 事件
      3. jq '.data.output | type' 验证类型
    Expected Result: "string"
    Failure Indicators: "object" 或 "array"
    Evidence: .sisyphus/evidence/task-5-output-type.txt
  ```

  **Commit**: YES (groups with T4)
  - Message: `refactor(agent): rewrite handleToolCalls for new SSE protocol`
  - Files: `server/internal/agent/engine.go, engine_tools.go`
  - Pre-commit: `cd server && go build ./...`

- [x] 6. 后端 persistMessage + buildUIParts 重写

  **What to do**:
  - 重写 `buildUIParts()` 以产出 `PersistedPart` 数组：
    - 每个 part 包含 `stepIndex` 字段
    - tool-call 的 state 存储为最终状态（"complete"/"error"），不是 "pending"
    - tool-call 的 output 包含序列化后的 JSON string（从 role=tool 消息查询）
    - JSON key 全部 camelCase
  - 重写 `persistMessage()` 以使用新 `PersistedParts` 类型
  - 持久化时仍写入 Content/Reasoning/ToolCalls legacy 字段（BuildContext fallback 需要）
  - 持久化时仍写入 role=tool 消息行（BuildContext buildToolResultLookup 需要）

  **Must NOT do**:
  - 不删除 legacy 字段的写入
  - 不删除 role=tool 消息行的写入
  - 不在 buildUIParts 中硬编码 stepIndex（应从 StreamResult 传入）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 逻辑简单，主要是字段映射和 JSON 序列化
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T4, T5, T7)
  - **Parallel Group**: Wave 2
  - **Blocks**: T11
  - **Blocked By**: T1, T3

  **References**:

  **Pattern References**:
  - `server/internal/agent/engine.go:460-525` — 当前 persistMessage + buildUIParts 实现
  - `server/internal/session/model.go` — PersistedPart 定义（来自 T3）

  **WHY Each Reference Matters**:
  - `engine.go persistMessage/buildUIParts`: 这是持久化路径的核心，tool-call state/output 的正确性直接影响刷新后 UI

  **Acceptance Criteria**:

  - [ ] buildUIParts 产出 PersistedPart 数组（非 map[string]any）
  - [ ] tool-call state 为 "complete" 或 "error"（不是 "pending"）
  - [ ] tool-call output 为序列化后的 JSON string
  - [ ] 每个 part 包含 stepIndex
  - [ ] `go build ./server/...` 编译通过

  **QA Scenarios**:

  ```
  Scenario: 持久化后的 Parts 包含正确的 tool-call 状态
    Tool: Bash (go test)
    Preconditions: persistMessage 重写完成
    Steps:
      1. 构造含 tool-call 的 StreamResult
      2. 调用 buildUIParts
      3. 验证 tool-call part 的 state = "complete"，output 非空
    Expected Result: state="complete"，output 为 JSON string
    Failure Indicators: state="pending" 或 output 为空
    Evidence: .sisyphus/evidence/task-6-persist-state.txt
  ```

  **Commit**: YES
  - Message: `refactor(agent): rewrite persistMessage and buildUIParts for Steps model`
  - Files: `server/internal/agent/engine.go`
  - Pre-commit: `cd server && go build ./...`

- [x] 7. 后端 GetMessages API + BuildContext 更新

  **What to do**:
  - 重写 `handlers.go:GetMessages()` 以返回新格式：
    - 从 DB Parts JSONB 重建 steps：按 stepIndex 分组，每组为一个 Step
    - 返回 `{ id, sessionId, role, steps: [...], metadata: {...} }` 格式
    - 不再返回 content/reasoning/tool_calls/tool_call_id legacy 字段
    - 过滤 role=tool 消息（不返回给前端）
  - 更新 `chat_context/manager.go:convertDBMessagesToUI()` 以处理新 PersistedParts 格式
  - 更新 `convertDBPartsToUIPart()` 以从 PersistedParts 读取 camelCase 字段
  - 更新 `synthesizeUIMessage()` 以产出含 Steps 的 UIMessage
  - 确保 `BuildContext` 路径仍然正确：从 UIMessage.Steps 展平为 ModelMessage

  **Must NOT do**:
  - 不删除 BuildContext 的 legacy fallback 路径（仍有旧数据需要处理）
  - 不修改 MessageForLLM 格式（OpenAI API 格式不变）
  - 不删除 buildToolResultLookup（role=tool 行仍存在）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 两条数据路径（GetMessages + BuildContext）都需要更新，且需保持向后兼容
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T4, T5, T6)
  - **Parallel Group**: Wave 2
  - **Blocks**: T11
  - **Blocked By**: T1, T3

  **References**:

  **Pattern References**:
  - `server/internal/api/handlers.go:153-260` — 当前 GetMessages + backfillParts 实现
  - `server/internal/chat_context/manager.go:83-149` — 当前 BuildContext 实现
  - `server/internal/chat_context/manager.go:167-288` — 当前 convertDBMessagesToUI + synthesizeUIMessage

  **WHY Each Reference Matters**:
  - `handlers.go GetMessages`: 这是刷新路径的数据来源，API 响应格式必须与流式路径最终状态一致
  - `chat_context/manager.go`: 这是 LLM 上下文构建路径，必须正确处理 Steps 结构

  **Acceptance Criteria**:

  - [ ] GetMessages 返回 `{ steps: [...] }` 格式（非 `{ parts: [...] }`）
  - [ ] GetMessages 不返回 role=tool 消息
  - [ ] GetMessages 不返回 content/reasoning/tool_calls legacy 字段
  - [ ] BuildContext 从 Steps 结构正确展平为 ModelMessage
  - [ ] `go test ./internal/chat_context/... -v` PASS
  - [ ] `go build ./server/...` 编译通过

  **QA Scenarios**:

  ```
  Scenario: GetMessages 返回 steps 结构
    Tool: Bash (curl + jq)
    Preconditions: 后端运行中，session 有历史消息
    Steps:
      1. curl -s localhost:8080/api/sessions/{SID}/messages | jq '.messages[0] | keys'
      2. 验证包含 "steps" 字段，不包含 "parts"/"content"/"reasoning"/"tool_calls"
    Expected Result: keys 包含 steps，不包含 legacy 字段
    Failure Indicators: 仍有 parts 或 content 字段
    Evidence: .sisyphus/evidence/task-7-api-format.txt

  Scenario: steps 按 stepIndex 正确分组
    Tool: Bash (curl + jq)
    Preconditions: session 有多轮迭代的 assistant 消息
    Steps:
      1. curl -s localhost:8080/api/sessions/{SID}/messages | jq '.messages[] | select(.role=="assistant") | .steps | length'
      2. 验证多轮迭代的消息有多个 steps
    Expected Result: steps.length >= 1
    Failure Indicators: steps 为空或缺失
    Evidence: .sisyphus/evidence/task-7-steps-grouping.txt
  ```

  **Commit**: YES
  - Message: `refactor(api): update GetMessages and BuildContext for Steps`
  - Files: `server/internal/api/handlers.go, server/internal/chat_context/manager.go`
  - Pre-commit: `cd server && go build ./... && go test ./internal/chat_context/... -v`

- [x] 8. 前端 CopConChatProvider 重写

  **What to do**:
  - 重写 `CopConChatProvider.ts` 的 `transformMessage()` 方法，只处理 4 种新事件：
    - `step_create`: 在 `UIMessage.steps[stepIndex]` 创建 `Step { parts: [], status: 'streaming' }`
    - `part_create`: 在 `steps[stepIndex].parts[partIndex]` 插入对应 Part
    - `part_update`: 更新 `steps[stepIndex].parts[partIndex]` 的 textDelta/state/output/error
    - `message_done`: 所有 steps status → 'done'，streaming parts → 'done'，pending/running tool-call → 'complete'
  - 更新 `CopConMessage` 接口：删除 `content`/`reasoning`/`tool_calls`/`tool_call_id` 字段，添加 `steps: Step[]` 和 `metadata`
  - 更新 `transformLocalMessage()` 产出的 user 消息格式：`{ id, role: 'user', steps: [{ parts: [{ type: 'text', text, state: 'done' }] }], metadata }`
  - 删除所有 legacy 事件处理分支（message, reasoning, tool_call, tool_result, done, error 中的 legacy 处理）
  - 删除 `createPart()`/`updatePart()` 私有方法（新逻辑内联在 transformMessage 中，更清晰）
  - 更新 `CopConSSEOutput` 接口保持不变
  - **重要**：transformMessage 中对 steps 的更新必须是 immutable 的（深拷贝 steps 数组），因为 useXChat 用返回值替换整个 message

  **Must NOT do**:
  - 不使用 `as string`/`as any` 类型断言
  - 不处理 legacy 事件（只处理 step_create/part_create/part_update/message_done/error）
  - 不保留 `content`/`reasoning`/`tool_calls` 字段在 CopConMessage 中

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 核心前端逻辑重写，需正确处理 immutable 嵌套更新
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on T4, T5 backend events being correct)
  - **Parallel Group**: Wave 3 (with T9)
  - **Blocks**: T10
  - **Blocked By**: T2, T4, T5

  **References**:

  **Pattern References**:
  - `packages/ui/src/providers/CopConChatProvider.ts:95-360` — 当前 transformMessage 完整实现，包含所有需替换的事件处理
  - `packages/ui/src/providers/CopConChatProvider.ts:113-127` — transformLocalMessage 当前实现
  - `packages/ui/node_modules/@ant-design/x-sdk/es/x-chat/index.js:162-219` — useXChat 的 updateMessage 逻辑，理解 immutable 更新要求

  **API/Type References**:
  - `packages/ui/src/api/types.ts` — 新类型定义（来自 T2）
  - `docs/message-architecture-design.md:Section 5.1` — 前端 SSE 处理的完整设计

  **WHY Each Reference Matters**:
  - `CopConChatProvider.ts`: 这是前端消息处理的核心，每个事件类型的处理都需要重写
  - `useXChat index.js`: 理解 useXChat 如何消费 transformMessage 返回值，确保 immutable 更新正确

  **Acceptance Criteria**:

  - [ ] `cd packages/ui && npx tsc --noEmit` 零错误
  - [ ] transformMessage 只处理 step_create/part_create/part_update/message_done/error
  - [ ] 零 `as string`/`as any` 类型断言
  - [ ] CopConMessage 只包含 id/role/steps/metadata
  - [ ] steps 更新为 immutable（每次 transformMessage 返回新对象）

  **QA Scenarios**:

  ```
  Scenario: TypeScript 严格编译无类型断言
    Tool: Bash
    Preconditions: CopConChatProvider 重写完成
    Steps:
      1. grep -n 'as any\|as string' packages/ui/src/providers/CopConChatProvider.ts
      2. 验证无输出
      3. cd packages/ui && npx tsc --noEmit
    Expected Result: 零匹配，编译通过
    Failure Indicators: 出现类型断言或编译错误
    Evidence: .sisyphus/evidence/task-8-no-type-assertion.txt

  Scenario: 处理 step_create 事件正确创建 Step
    Tool: Bash (unit reasoning)
    Preconditions: Provider 可实例化
    Steps:
      1. 构造 step_create 事件：{ type: 'step_create', data: { messageId: 'm1', stepIndex: 0 } }
      2. 调用 transformMessage({ originMessage: baseMessage, chunk: { data: JSON.stringify(event) } })
      3. 验证返回的 message.steps[0] = { parts: [], status: 'streaming' }
    Expected Result: steps 数组长度为 1，status 为 'streaming'
    Failure Indicators: steps 为空或结构不正确
    Evidence: .sisyphus/evidence/task-8-step-create.txt
  ```

  **Commit**: YES
  - Message: `refactor(ui): rewrite CopConChatProvider for steps-based UI events`
  - Files: `packages/ui/src/providers/CopConChatProvider.ts`
  - Pre-commit: `cd packages/ui && npx tsc --noEmit`

- [x] 9. 前端 useAgentChat + messageUtils 简化

  **What to do**:
  - 简化 `useAgentChat.ts:loadMessages()`：
    - 删除 `mergeToolMessages()` 调用（不再有 role=tool 消息需要合并）
    - 直接使用 API 返回的 UIMessage[] 格式（含 steps），无需转换
    - 删除 `(result.messages || []) as CopConMessage[]` 的类型 cast
  - 简化 `messageUtils.ts`：
    - 删除 `mergeToolMessages()` 函数
    - 保留 `parseToolOutput()` 函数（仍需用于 ToolCallPart.output 的展示解析）
  - 更新 `useAgentChat` 的 `sendMessage` 回调以适配新的 CopConMessage 格式

  **Must NOT do**:
  - 不删除 parseToolOutput（渲染时仍需要）
  - 不修改 useXChat 的调用方式

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 删除代码和简化逻辑
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (after T8)
  - **Parallel Group**: Wave 3 (after T8)
  - **Blocks**: T10
  - **Blocked By**: T2, T8

  **References**:

  **Pattern References**:
  - `packages/ui/src/hooks/useAgentChat.ts:73-98` — 当前 loadMessages 实现
  - `packages/ui/src/utils/messageUtils.ts:56-90` — mergeToolMessages 实现（需删除）

  **WHY Each Reference Matters**:
  - `useAgentChat.ts`: 加载路径的简化直接影响刷新后的数据一致性
  - `messageUtils.ts`: mergeToolMessages 是旧架构的产物，删除它消除潜在的转换错误

  **Acceptance Criteria**:

  - [ ] loadMessages 不调用 mergeToolMessages
  - [ ] loadMessages 不使用 `as CopConMessage[]` 类型断言
  - [ ] mergeToolMessages 函数已删除
  - [ ] parseToolOutput 函数保留
  - [ ] `cd packages/ui && npx tsc --noEmit` 零错误

  **QA Scenarios**:

  ```
  Scenario: TypeScript 编译通过
    Tool: Bash
    Steps:
      1. cd packages/ui && npx tsc --noEmit
    Expected Result: 零错误
    Evidence: .sisyphus/evidence/task-9-tsc.txt

  Scenario: mergeToolMessages 已删除
    Tool: Bash
    Steps:
      1. grep -r 'mergeToolMessages' packages/ui/src/
      2. 验证无结果
    Expected Result: 无匹配
    Failure Indicators: 仍有引用
    Evidence: .sisyphus/evidence/task-9-merge-deleted.txt
  ```

  **Commit**: YES
  - Message: `refactor(ui): simplify useAgentChat and remove mergeToolMessages`
  - Files: `packages/ui/src/hooks/useAgentChat.ts, packages/ui/src/utils/messageUtils.ts`
  - Pre-commit: `cd packages/ui && npx tsc --noEmit`

- [x] 10. 前端渲染逻辑更新

  **What to do**:
  - 重写 `App.tsx:renderMessageContent()` 以使用 steps 结构：
    ```tsx
    function renderMessageContent(msg: CopConMessage) {
      return msg.steps.map((step, stepIndex) => (
        <React.Fragment key={stepIndex}>
          {stepIndex > 0 && <Divider />}
          <StepContent step={step} />
        </React.Fragment>
      ));
    }
    ```
  - 实现 `StepContent` 组件：
    - 遍历 step.parts，按类型渲染：
      - `reasoning` → `<Think>` + MarkdownContent
      - `text` → MarkdownContent
      - `tool-call` → 收集为 ThoughtChain items
    - Tool-call parts 在 step 内收集后统一渲染为 `<ThoughtChain>`
  - 更新 `bubbleItems` 构建逻辑：
    - user 消息：从 `msg.steps[0].parts[0].text` 读取内容（而非 `msg.content`）
    - assistant 消息：loading 检测改为 `!msg.steps.some(s => s.parts.some(p => p.text || p.type === 'tool-call'))`
  - 更新 `mapToolCallStatus` 函数：删除 "done"→"loading" 的错误映射，只保留合法 state
  - 删除对 `msg.content`/`msg.reasoning`/`msg.tool_calls` 的所有引用

  **Must NOT do**:
  - 不修改 CopConChatProvider（T8 负责）
  - 不修改 useAgentChat（T9 负责）
  - 不添加新 UI 组件库（使用现有 antd 组件）

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: 前端 UI 渲染逻辑，需要理解 Ant Design X 组件
  - **Skills**: [`frontend-ui-ux`]

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on T8, T9)
  - **Parallel Group**: Wave 4 (with T11, T12)
  - **Blocks**: T12
  - **Blocked By**: T8, T9

  **References**:

  **Pattern References**:
  - `packages/demo/src/App.tsx:134-174` — 当前 renderMessageContent 实现
  - `packages/demo/src/App.tsx:176-202` — 当前 bubbleItems 构建逻辑

  **WHY Each Reference Matters**:
  - `App.tsx renderMessageContent`: 渲染逻辑是用户直接看到的结果，Steps 结构的渲染必须清晰
  - `App.tsx bubbleItems`: loading 检测逻辑直接影响用户体验

  **Acceptance Criteria**:

  - [ ] 渲染逻辑基于 steps 结构
  - [ ] 多 step 之间有 Divider 分隔
  - [ ] tool-call 使用 ThoughtChain 组件
  - [ ] 无 `msg.content`/`msg.reasoning`/`msg.tool_calls` 引用
  - [ ] `cd packages/demo && npx tsc --noEmit` 零错误

  **QA Scenarios**:

  ```
  Scenario: 单 step 消息正确渲染
    Tool: Playwright
    Preconditions: 应用运行中，有历史消息
    Steps:
      1. 导航到有消息的会话
      2. 截图验证消息显示（含 reasoning 折叠、文本内容、工具调用链）
    Expected Result: 消息正确显示，reasoning 可展开，工具调用有状态
    Evidence: .sisyphus/evidence/task-10-single-step.png

  Scenario: 多 step 消息正确渲染（含 Divider）
    Tool: Playwright
    Preconditions: 应用运行中，有多轮迭代的会话
    Steps:
      1. 发送需要工具调用的消息
      2. 等待完成
      3. 验证显示 Divider 分隔的多个 step
    Expected Result: 多个 step 之间有分隔线
    Evidence: .sisyphus/evidence/task-10-multi-step.png

  Scenario: loading 状态正确显示
    Tool: Playwright
    Preconditions: 应用运行中
    Steps:
      1. 发送消息
      2. 在流式传输中截图
      3. 验证有 loading 指示器
    Expected Result: 消息有 loading 状态
    Evidence: .sisyphus/evidence/task-10-loading.png
  ```

  **Commit**: YES
  - Message: `feat(demo): update rendering for Step-based layout`
  - Files: `packages/demo/src/App.tsx`
  - Pre-commit: `cd packages/demo && npx tsc --noEmit`

- [x] 11. Legacy 数据迁移 + 兼容处理

  **What to do**:
  - 在 `session/model.go` 的 `PersistedParts.Scan()` 中实现双格式兼容：
    - 如果 JSON key 是 snake_case（如 `tool_call_id`），自动映射到 camelCase 字段
    - 如果 `stepIndex` 缺失，默认为 0
  - 在 `chat_context/manager.go` 中确保 `synthesizeUIMessage()` 对旧数据（无 steps、无 stepIndex）正确处理：
    - 从 legacy Content/Reasoning/ToolCalls 字段构建含 steps 的 UIMessage
  - 在 `handlers.go:GetMessages()` 中确保 backfill 逻辑产出新格式（含 steps）：
    - 旧数据（Parts 为空）从 legacy 字段构建 steps
    - 新数据（Parts 有 stepIndex）按 stepIndex 分组
  - 确保前端 `loadMessages()` 能正确消费旧 API 格式和新 API 格式（过渡期）

  **Must NOT do**:
  - 不写 SQL 迁移脚本改写现有 JSONB 数据（Go Scan 兼容即可）
  - 不删除 legacy 字段列（Content, Reasoning, ToolCalls）
  - 不强制前端同时兼容旧 API 和新 API（部署顺序：后端先，前端后）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 需要理解数据格式边界，确保旧数据不丢失
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with T10, T12)
  - **Parallel Group**: Wave 4
  - **Blocks**: T12
  - **Blocked By**: T6, T7

  **References**:

  **Pattern References**:
  - `server/internal/session/model.go` — PersistedParts Scan/Value（来自 T3）
  - `server/internal/api/handlers.go:213-260` — 当前 backfillParts 实现
  - `server/internal/chat_context/manager.go:243-288` — 当前 synthesizeUIMessage 实现

  **WHY Each Reference Matters**:
  - `model.go Scan`: 旧数据兼容的第一道防线，必须正确映射 snake_case 到 camelCase
  - `handlers.go backfillParts`: 旧数据的渲染路径，必须产出 steps 格式

  **Acceptance Criteria**:

  - [ ] 旧数据（snake_case Parts JSONB）加载后 UI 正确显示
  - [ ] 旧数据（空 Parts，有 legacy 字段）加载后 UI 正确显示
  - [ ] 新旧格式消息在同一会话中共存时都能正确显示
  - [ ] `go test ./internal/session/... -v` PASS
  - [ ] `go test ./internal/chat_context/... -v` PASS

  **QA Scenarios**:

  ```
  Scenario: 旧数据（snake_case Parts）正确加载
    Tool: Bash (curl)
    Preconditions: 数据库中有旧格式消息（tool_call_id 字段名）
    Steps:
      1. curl -s localhost:8080/api/sessions/{SID}/messages | jq '.messages[].steps'
      2. 验证每条消息都有 steps 数组
      3. 验证 tool-call part 的 toolCallId 非空
    Expected Result: steps 存在，toolCallId 正确
    Failure Indicators: steps 缺失或 toolCallId 为空
    Evidence: .sisyphus/evidence/task-11-legacy-parts.txt

  Scenario: 旧数据（空 Parts + legacy 字段）正确 backfill
    Tool: Bash (curl)
    Preconditions: 数据库中有 Parts 为空但有 Content/Reasoning/ToolCalls 的消息
    Steps:
      1. curl -s localhost:8080/api/sessions/{SID}/messages | jq '.messages[].steps'
      2. 验证从 legacy 字段构建了 steps
    Expected Result: steps 包含正确的 text/reasoning/tool-call parts
    Failure Indicators: steps 为空
    Evidence: .sisyphus/evidence/task-11-backfill.txt
  ```

  **Commit**: YES
  - Message: `feat(migration): add legacy data compatibility and stepIndex backfill`
  - Files: `server/internal/session/model.go, server/internal/api/handlers.go, server/internal/chat_context/manager.go`
  - Pre-commit: `cd server && go test ./internal/session/... ./internal/chat_context/... -v`

- [x] 12. 端到端验证 + 清理

  **What to do**:
  - 端到端验证：发送消息 → 等待完成 → 刷新页面 → 比较 UI
  - 验证多轮迭代场景：工具调用 → 第二轮 reasoning/text
  - 验证刷新一致性：流式最终状态 vs 刷新后加载状态
  - 清理旧代码：
    - 在 `event.go` 中删除标记为 Deprecated 的旧事件常量
    - 在 `engine.go` 中删除旧事件相关的 import 和变量
    - 在 `types.ts` 中删除 `@deprecated` 标记的旧类型（Message, SSEEvent 等）
    - 在 `CopConChatProvider.ts` 中确认无 legacy 事件处理残留
  - 更新 OpenAPI spec（如果存在）
  - 验证 `go vet ./...` 和 `npx tsc --noEmit` 通过

  **Must NOT do**:
  - 不删除 Content/Reasoning/ToolCalls DB 列（BuildContext fallback 仍需要）
  - 不删除 role=tool 消息行的写入逻辑

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 需要全面的验证和谨慎的清理
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on all previous tasks)
  - **Parallel Group**: Wave 4 (last)
  - **Blocks**: F1-F4
  - **Blocked By**: T10, T11

  **References**:

  **Pattern References**:
  - `docs/message-architecture-design.md` — 完整设计规格，验证时参照
  - `tmp/chat_stream.log` — 之前的 SSE 日志样本，可用于对比

  **Acceptance Criteria**:

  - [ ] 流式完成 + 刷新后 UI 完全一致
  - [ ] 多轮迭代场景正确显示
  - [ ] 旧 Deprecated 常量/类型已删除
  - [ ] `go vet ./...` 零警告
  - [ ] `npx tsc --noEmit` 零错误
  - [ ] 无 `as any`/`as string` 在 CopConChatProvider.ts 中

  **QA Scenarios**:

  ```
  Scenario: 刷新一致性验证
    Tool: Playwright
    Preconditions: 应用运行中
    Steps:
      1. 发送消息"列出当前目录文件"
      2. 等待完整响应（含工具调用）
      3. 截图记录流式最终状态
      4. 刷新页面
      5. 截图记录刷新后状态
      6. 比较两张截图
    Expected Result: 两次截图内容一致
    Failure Indicators: 刷新后内容缺失或结构不同
    Evidence: .sisyphus/evidence/task-12-refresh-parity.png

  Scenario: 多轮迭代正确渲染
    Tool: Playwright
    Preconditions: 应用运行中
    Steps:
      1. 发送需要工具调用的消息
      2. 等待完整响应（应有多轮迭代）
      3. 验证显示多个 step（有 Divider 分隔）
      4. 验证每个 step 的内容正确
    Expected Result: 多个 step 正确显示，有 Divider
    Failure Indicators: 只显示第一个 step，或 step 间无分隔
    Evidence: .sisyphus/evidence/task-12-multi-iteration.png

  Scenario: SSE 输出只有 4 种事件类型
    Tool: Bash (curl + jq)
    Preconditions: 后端运行中
    Steps:
      1. curl -s -N -X POST localhost:8080/api/sessions/{SID}/chat -d '{"content":"hello"}'
      2. jq -c '.type' | sort | uniq -c
      3. 验证只有 step_create, part_create, part_update, message_done
    Expected Result: 无 legacy 事件
    Failure Indicators: 出现 message/reasoning/tool_call/tool_result/done
    Evidence: .sisyphus/evidence/task-12-sse-types.txt

  Scenario: 无 Deprecated 代码残留
    Tool: Bash
    Steps:
      1. grep -rn 'EventMessage\b\|EventReasoning\b\|EventToolCall\b\|EventToolResult\b' server/internal/
      2. grep -rn 'SSEEvent\b\|SSEEventType\b' packages/ui/src/
      3. 验证无匹配（或仅在注释中）
    Expected Result: 无匹配
    Failure Indicators: 仍有引用
    Evidence: .sisyphus/evidence/task-12-deprecated-cleanup.txt
  ```

  **Commit**: YES
  - Message: `chore: end-to-end verification and cleanup deprecated code`
  - Files: various
  - Pre-commit: `cd server && go vet ./... && cd ../packages/ui && npx tsc --noEmit`

---

## Final Verification Wave

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists. For each "Must NOT Have": search codebase for forbidden patterns. Check evidence files. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go test ./... -v` + `cd packages/ui && npx tsc --noEmit`. Review all changed files for: `as any`/`@ts-ignore`, empty catches, console.log in prod, unused imports, `any` types in Go event data. Check AI slop.
  Output: `Build [PASS/FAIL] | Lint [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high` (+ `playwright` skill)
  Start from clean state. Execute EVERY QA scenario from EVERY task. Test cross-task integration. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff. Verify 1:1. Check "Must NOT do" compliance. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **T1**: `refactor(domain): define new SSE event types and UIMessage with Steps` - event.go, ui_message.go
- **T2**: `refactor(ui): define new UIMessage/Step/Part TypeScript types` - types.ts
- **T3**: `refactor(session): update PersistedPart with camelCase JSON tags and stepIndex` - model.go
- **T4+T5**: `refactor(agent): rewrite handleStreaming and handleToolCalls for new SSE protocol` - engine.go, engine_tools.go
- **T6**: `refactor(agent): rewrite persistMessage and buildUIParts for Steps model` - engine.go
- **T7**: `refactor(api): update GetMessages and BuildContext for Steps` - handlers.go, chat_context/manager.go
- **T8**: `refactor(ui): rewrite CopConChatProvider for steps-based UI events` - CopConChatProvider.ts
- **T9**: `refactor(ui): simplify useAgentChat and remove mergeToolMessages` - useAgentChat.ts, messageUtils.ts
- **T10**: `feat(demo): update rendering for Step-based layout` - App.tsx
- **T11**: `feat(migration): add legacy data compatibility and stepIndex backfill` - migration code
- **T12**: `chore: end-to-end verification and cleanup` - various

---

## Success Criteria

### Verification Commands
```bash
# SSE events only contain new types
curl -s -N -X POST localhost:8080/api/sessions/{SID}/chat -d '{"content":"hello"}' | jq -c '.type' | sort | uniq -c
# Expected: only step_create, part_create, part_update, message_done, error

# All SSE fields are camelCase
curl -s -N -X POST localhost:8080/api/sessions/{SID}/chat -d '{"content":"hello"}' | jq '.data | keys' | grep '_'
# Expected: no output

# Tool output is always string
curl -s -N -X POST localhost:8080/api/sessions/{SID}/chat -d '{"content":"list files"}' | jq -c 'select(.data.output != null) | .data.output | type'
# Expected: "string"

# Go tests pass
cd server && go test ./internal/domain/entity/... -v
# Expected: PASS

# TypeScript strict compilation
cd packages/ui && npx tsc --noEmit
# Expected: zero errors

# No `as any` or `as string` casts in provider
grep -n 'as any\|as string' packages/ui/src/providers/CopConChatProvider.ts
# Expected: no output
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All Go tests pass
- [ ] TypeScript strict compilation passes
- [ ] SSE events only 4 types (+ error)
- [ ] SSE fields all camelCase
- [ ] Tool output always string
- [ ] Refresh path produces identical UI as streaming
