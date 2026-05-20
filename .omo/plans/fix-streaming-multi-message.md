# 修复流式响应多消息渲染问题

## TL;DR

> **问题**：Agent 多轮交互时，后端每次 LLM 调用生成不同的 `message_id`，导致前端无法正确累积内容到同一条消息。
> **方案**：后端统一 `message_id` 生成时机，前端修复消息累积逻辑。
> **影响**：后端 1 个文件修改，前端 2 个文件修改。

**Deliverables**:
- 后端：修改 `engine.go`，将 `messageID` 生成移到循环外
- 前端：修复 `CopConChatProvider.ts` 消息处理逻辑
- 前端：修复 `App.tsx` 工具组件布局

**Estimated Effort**: Small
**Parallel Execution**: NO - 有依赖关系（后端改完前端才能测试）
**Critical Path**: 后端修改 → 前端修改 → 测试验证

---

## Context

### Original Request

用户报告前端页面流式响应存在 bug：
1. 工具一直卡在调用状态
2. 没有后续的新消息
3. 工具组件位置展示不正常（应该在气泡下面，现在在右边）

### Interview Summary

**Key Discussions**:
- 日志分析：一次会话有 3 个不同的 `message_id`（`98d5ab3c`, `0fee6763`, `a0003d63`）
- 原因定位：后端 `runAgentLoop` 每次循环迭代生成新 `messageID`
- 行业调研：ChatGPT/Claude/Cursor 等主流产品采用"单气泡 + 可折叠区域"模式
- 方案确认：修改后端统一 `message_id`，而非前端忽略

**Research Findings**:
- ChatGPT：单气泡 + 可折叠思考区域
- Claude：`type: "thinking"` 独立块，可折叠
- Ant Design X：`ThoughtChain` 组件专门用于展示工具调用链

### Root Cause Analysis

**后端问题**（`server/internal/agent/engine.go:288`）：
```go
for {
    messageID := uuid.New().String()  // 每次循环都生成新的 ID
    ...
}
```

**前端问题**（`packages/ui/src/providers/CopConChatProvider.ts:106-113`）：
```typescript
const baseMessage: CopConMessage = originMessage || {
  id: chunkMessageId || `assistant-${Date.now()}`,
  ...
};
```
当 `message_id` 变化时，新内容被错误地累积到旧消息。

---

## Work Objectives

### Core Objective

修复流式响应在 Agent 多轮交互时的渲染问题，确保所有内容正确累积到一条 assistant 消息中。

### Concrete Deliverables

1. 后端：`server/internal/agent/engine.go` - 修改 `runAgentLoop`
2. 前端：`packages/ui/src/providers/CopConChatProvider.ts` - 优化消息处理
3. 前端：`packages/demo/src/App.tsx` - 修复工具组件布局

### Definition of Done

- [ ] 后端：一次用户请求对应一个统一的 `message_id`
- [ ] 前端：所有内容正确累积到一条消息
- [ ] 前端：工具状态正确显示（loading → success/error）
- [ ] 前端：工具组件位置正确（气泡下方，而非右侧）

### Must Have

- 后端必须在循环开始前生成 `messageID`
- 前端必须正确处理多个 `tool_call` 和 `tool_result`
- 工具调用过程必须可折叠展示

### Must NOT Have (Guardrails)

- 不要改变数据库存储结构
- 不要引入新的依赖
- 不要破坏现有的 API 兼容性

---

## Verification Strategy

### Test Decision

- **Infrastructure exists**: YES (Go test framework)
- **Automated tests**: Tests-after（修改后补充测试）
- **Agent-Executed QA**: ALWAYS（手动测试 + 自动化验证）

### QA Policy

Every task MUST include agent-executed QA scenarios.

---

## Execution Strategy

### Sequential Execution (有依赖关系)

```
Step 1: 后端修改 (engine.go)
    ↓
Step 2: 前端修改 (CopConChatProvider.ts)
    ↓
Step 3: UI布局修复 (App.tsx)
    ↓
Step 4: 集成测试验证
```

---

## TODOs

- [x] 1. 修改后端 message_id 生成逻辑

  **What to do**:
  - 将 `messageID := uuid.New().String()` 从循环内移到循环外
  - 确保整个 Agent 循环使用同一个 `messageID`
  - 发送 `done` 事件时使用该 `messageID`

  **File**: `server/internal/agent/engine.go`

  **Code Change**:
  ```go
  // Before:
  func (e *AgentEngine) runAgentLoop(...) error {
      ...
      for {
          messageID := uuid.New().String()  // 移到循环外
          ...
      }
  }

  // After:
  func (e *AgentEngine) runAgentLoop(...) error {
      ...
      messageID := uuid.New().String()  // 移到循环开始前
      for {
          ...
      }
  }
  ```

  **Must NOT do**:
  - 不要改变 `toolCallInfo.MessageID` 的使用方式
  - 不要修改 SSE 事件格式

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单的重构，只需要移动一行代码的位置
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Blocks**: Task 2, 3, 4
  - **Blocked By**: None

  **References**:
  - `server/internal/agent/engine.go:276-317` - `runAgentLoop` 函数
  - `server/internal/agent/engine.go:288` - 当前的 `messageID` 生成位置
  - `server/internal/agent/engine_tools.go:411-414` - `done` 事件发送

  **Acceptance Criteria**:
  - [ ] `messageID` 在循环外生成
  - [ ] 所有 SSE 事件使用相同的 `messageID`
  - [ ] `go test ./internal/agent/... -v` 通过

  **QA Scenarios**:
  ```
  Scenario: 验证统一的 message_id
    Tool: Bash (curl)
    Preconditions: 后端服务启动
    Steps:
      1. curl -N -X POST http://localhost:8080/api/sessions/{session-id}/chat \
         -H "Content-Type: application/json" \
         -d '{"content":"列出/data目录"}' | tee /tmp/test_output.log
      2. grep -o '"message_id":"[^"]*"' /tmp/test_output.log | sort -u
    Expected Result: 只有一个唯一的 message_id
    Evidence: /tmp/test_output.log
  ```

  **Commit**: YES
  - Message: `fix(agent): use single message_id per user request`
  - Files: `server/internal/agent/engine.go`

---

- [x] 2. 优化前端消息处理逻辑

  **What to do**:
  - 优化 `transformMessage` 方法，确保 `message_id` 变化时不会丢失内容
  - 添加防御性代码，处理 `tool_result` 匹配失败的情况
  - 确保多个 `tool_call` 正确累积到 `tool_calls` 数组

  **File**: `packages/ui/src/providers/CopConChatProvider.ts`

  **Code Analysis**:
  当前代码在 `message_id` 变化时：
  1. `chunkMessageId` 是新的 ID
  2. `originMessage` 是旧消息（包含之前的内容）
  3. `baseMessage = originMessage || {id: chunkMessageId}` 使用旧消息
  4. 新内容被追加到旧消息（这是正确的）
  
  问题在于 `tool_result` 匹配：
  - 当第二轮的 `tool_result` 到达时
  - `baseMessage.tool_calls` 可能不包含对应的 `tool_call`
  - 匹配失败，状态不更新

  **Solution**:
  后端统一 `message_id` 后，这个问题应该自动解决。
  但仍需添加防御性代码：
  ```typescript
  case 'tool_result': {
    const toolCallId = data.id as string;
    const toolCalls = baseMessage.tool_calls;

    if (!toolCalls) {
      console.warn('[CopConChatProvider] tool_result received but no tool_calls in message');
      // 创建 tool_calls 数组并添加新的 tool_call
      return {
        ...baseMessage,
        tool_calls: [{
          id: toolCallId,
          type: 'function' as const,
          function: { name: (data.tool_name as string) || '', arguments: '{}' },
          status: 'success' as const,
          output: (data.result as string) || '',
        }],
      };
    }

    const toolCallIndex = toolCalls.findIndex(tc => tc.id === toolCallId);
    if (toolCallIndex === -1) {
      console.warn(`[CopConChatProvider] tool_result for unknown tool_call_id: ${toolCallId}`);
      // 添加缺失的 tool_call
      return {
        ...baseMessage,
        tool_calls: [...toolCalls, {
          id: toolCallId,
          type: 'function' as const,
          function: { name: (data.tool_name as string) || '', arguments: '{}' },
          status: 'success' as const,
          output: (data.result as string) || '',
        }],
      };
    }

    // 正常更新
    ...
  }
  ```

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 添加防御性代码，逻辑简单
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Blocks**: Task 4
  - **Blocked By**: Task 1

  **References**:
  - `packages/ui/src/providers/CopConChatProvider.ts:154-183` - `tool_result` 处理逻辑

  **Acceptance Criteria**:
  - [ ] `tool_result` 匹配失败时有防御性处理
  - [ ] TypeScript 编译通过（`pnpm tsc --noEmit`）
  - [ ] 不影响正常流程

  **QA Scenarios**:
  ```
  Scenario: 验证前端消息累积
    Tool: Playwright
    Preconditions: 前端和后端服务启动
    Steps:
      1. 打开 http://localhost:5173
      2. 创建新会话
      3. 发送消息 "列出/data目录，然后再列出/tmp目录"
      4. 等待响应完成（tool_calls 状态变为 success）
      5. 截图验证消息结构
    Expected Result: 
      - 只有一条 assistant 消息
      - ThoughtChain 显示两个工具调用
      - 两个工具状态都是 success
    Evidence: .sisyphus/evidence/task-2-message-accumulation.png
  ```

  **Commit**: YES
  - Message: `fix(ui): add defensive handling for tool_result`
  - Files: `packages/ui/src/providers/CopConChatProvider.ts`

---

- [x] 3. 修复工具组件布局

  **What to do**:
  - 将 `ThoughtChain` 从 `header` 属性移到 `content` 属性中
  - 或使用 `footer` 属性，确保显示在气泡下方

  **File**: `packages/demo/src/App.tsx`

  **Code Analysis**:
  当前代码：
  ```typescript
  bubbleItems.push({
    key: msg.id,
    role: msg.role === 'user' ? 'user' : 'ai',
    content: msg.content || (msg.reasoning ? ' ' : ''),
    loading: ...,
    header,  // ThoughtChain 在这里
  });
  ```
  
  `@ant-design/x` 的 `Bubble` 组件：
  - `header`: 在气泡上方或右侧（取决于配置）
  - `content`: 在气泡内部
  - `footer`: 在气泡下方

  **Solution**:
  将 `ThoughtChain` 放在 `content` 中，与消息内容一起：
  ```typescript
  bubbleItems.push({
    key: msg.id,
    role: msg.role === 'user' ? 'user' : 'ai',
    content: (
      <>
        {header}  {/* ThoughtChain */}
        <MarkdownContent content={msg.content || ''} />
      </>
    ),
    loading: ...,
  });
  ```

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: UI 布局调整，需要理解组件结构
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Blocks**: Task 4
  - **Blocked By**: Task 1

  **References**:
  - `packages/demo/src/App.tsx:125-198` - 消息渲染逻辑
  - `packages/ui/node_modules/@ant-design/x/es/bubble/interface.d.ts` - Bubble 组件 API

  **Acceptance Criteria**:
  - [ ] ThoughtChain 显示在气泡内容区域
  - [ ] 消息内容在 ThoughtChain 下方
  - [ ] 布局符合预期（工具调用在上方，消息内容在下方）

  **QA Scenarios**:
  ```
  Scenario: 验证工具组件布局
    Tool: Playwright
    Preconditions: 前端和后端服务启动
    Steps:
      1. 打开 http://localhost:5173
      2. 创建新会话
      3. 发送消息 "列出/data目录"
      4. 等待工具调用完成
      5. 截图验证布局
    Expected Result: 
      - ThoughtChain 在消息气泡内部
      - 工具调用在上方
      - 消息内容在下方
    Evidence: .sisyphus/evidence/task-3-layout-fix.png
  ```

  **Commit**: YES
  - Message: `fix(demo): move ThoughtChain to content area`
  - Files: `packages/demo/src/App.tsx`

---

- [x] 4. 集成测试验证

  **What to do**:
  - 运行完整的端到端测试
  - 验证所有修复点
  - 确保没有回归问题

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 执行测试验证
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Blocks**: None
  - **Blocked By**: Task 1, 2, 3

  **QA Scenarios**:
  ```
  Scenario: 完整流程验证
    Tool: Playwright
    Preconditions: 所有服务启动
    Steps:
      1. 打开 http://localhost:5173
      2. 创建新会话
      3. 发送消息 "列出/data目录，然后告诉我有什么"
      4. 等待完整响应
      5. 验证：
         - message_id 统一
         - tool_calls 状态正确
         - 布局正确
         - 最终消息内容完整
      6. 截图保存
    Expected Result: 所有验证点通过
    Evidence: .sisyphus/evidence/task-4-full-test.png
  ```

  **Commit**: NO

---

## Final Verification Wave (MANDATORY)

- [x] F1. **Plan Compliance Audit** — `oracle`
  验证所有修改符合计划要求。

- [x] F2. **Code Quality Review** — `unspecified-high`
  运行 `pnpm tsc --noEmit` 和 `go vet ./...`，确保代码质量。

- [x] F3. **Real Manual QA** — `unspecified-high`
  执行所有 QA 场景，验证功能正确。

- [x] F4. **Scope Fidelity Check** — `deep`
  确认修改范围符合计划，没有引入额外变更。

---

## Commit Strategy

- **1**: `fix(agent): use single message_id per user request` - server/internal/agent/engine.go
- **2**: `fix(ui): add defensive handling for tool_result` - packages/ui/src/providers/CopConChatProvider.ts
- **3**: `fix(demo): move ThoughtChain to content area` - packages/demo/src/App.tsx

---

## Success Criteria

### Verification Commands

```bash
# 后端测试
cd server && go test ./internal/agent/... -v

# 前端类型检查
cd packages/ui && pnpm tsc --noEmit
cd packages/demo && pnpm tsc --noEmit

# 集成测试
curl -N -X POST http://localhost:8080/api/sessions/{id}/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"测试消息"}' | grep -o '"message_id":"[^"]*"' | sort -u
# Expected: 只输出一个 message_id
```

### Final Checklist

- [ ] 后端 message_id 统一生成
- [ ] 前端消息累积正确
- [ ] 工具状态正确显示
- [ ] 布局符合预期
- [ ] 所有测试通过
