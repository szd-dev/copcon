# Frontend Ant Design X SDK Migration Plan

## TL;DR

> **Quick Summary**: 将前端 demo 从自定义实现迁移到 Ant Design X SDK 标准模式，使用 `useXChat` + `ChatProvider` + `XRequest` 架构。
> 
> **Deliverables**:
> - 安装 `@ant-design/x-sdk` 依赖
> - 创建自定义 `CopConChatProvider` 适配后端 SSE API
> - 重构 `App.tsx` 使用 `useXChat`
> - 优化性能和类型安全
> 
> **Estimated Effort**: Medium
> **Parallel Execution**: NO - 有依赖顺序
> **Critical Path**: 依赖安装 → Provider 创建 → Hook 替换 → UI 优化

---

## Context

### Original Request
重构前端 demo 应用，使其符合 Ant Design X SDK 最佳实践，解决当前实现中的问题：
1. 未使用 SDK 标准模式
2. SSE 处理位置不当
3. useEffect 依赖问题
4. 性能问题

### Interview Summary
**Key Discussions**:
- 使用 `@ant-design/x-sdk` 的标准模式
- 创建自定义 Provider 适配后端 SSE 格式
- 保持现有 UI 组件结构不变

**Research Findings**:
- 当前缺少 `@ant-design/x-sdk` 依赖
- 自定义 `useAgentChat` 需要替换为 SDK 的 `useXChat`
- SSE 解析逻辑应该在 Provider 中而非 Client 中

### Gap Analysis (from Metis review)
**Identified Gaps** (addressed):
- 后端 SSE 事件格式需要明确（已确认：`{ type: 'message'|'reasoning'|'tool_call'|... , data: {...} }`）
- ToolExecution 处理方式：SDK 支持将工具调用作为消息附件处理
- 需要确认是否需要修改后端 API（不需要，Provider 会适配）

---

## Work Objectives

### Core Objective
将前端迁移到 Ant Design X SDK 标准模式，提升代码质量和可维护性。

### Concrete Deliverables
- `packages/ui/package.json` 添加 `@ant-design/x-sdk` 依赖
- `packages/ui/src/providers/CopConChatProvider.ts` 自定义 Provider
- `packages/ui/src/hooks/useAgentChat.ts` 重构为使用 SDK
- `packages/ui/src/api/agentClient.ts` 简化为仅保留非聊天 API
- `packages/demo/src/App.tsx` 使用新的 Hook 和优化渲染

### Definition of Done
- [x] `pnpm build` 成功
- [x] 类型检查 `tsc --noEmit` 通过
- [x] 所有现有功能正常工作
- [x] 消息发送、接收、流式显示正常
- [x] 工具调用显示正常
- [x] 会话切换正常

### Must Have
- 使用 `@ant-design/x-sdk` 的 `useXChat`
- 使用 `@ant-design/x-sdk` 的 `AbstractChatProvider`
- 使用 `@ant-design/x-sdk` 的 `XRequest`
- 保持所有现有功能

### Must NOT Have (Guardrails)
- 不修改后端 API
- 不删除现有 UI 组件
- 不破坏 TodoList 功能
- 不引入 `as any` 或 `@ts-ignore`

---

## Verification Strategy

### Test Decision
- **Infrastructure exists**: NO
- **Automated tests**: NO
- **Agent-Executed QA**: YES

### QA Policy
每个任务包含 Agent-Executed QA 场景：
- **Frontend/UI**: Playwright 打开页面，验证渲染
- **Build/TypeScript**: Bash 运行构建命令

---

## Execution Strategy

### Sequential Execution (有依赖顺序)

```
Step 1: 安装依赖
    └── 安装 @ant-design/x-sdk

Step 2: 创建 Provider (依赖 Step 1)
    └── 实现 CopConChatProvider

Step 3: 重构 Hook (依赖 Step 2)
    └── 更新 useAgentChat 使用 SDK

Step 4: 简化 Client (与 Step 3 并行)
    └── 移除 chat 方法，保留其他 API

Step 5: 更新 UI (依赖 Step 3, 4)
    └── App.tsx 使用新 Hook，优化渲染

Step 6: 清理和验证 (依赖 Step 5)
    └── 移除旧代码，验证功能
```

### Dependency Matrix

| Task | Depends On |
|------|-----------|
| 1 | — |
| 2 | 1 |
| 3 | 2 |
| 4 | 1 |
| 5 | 3, 4 |
| 6 | 5 |

---

## TODOs

- [x] 1. **安装 @ant-design/x-sdk 依赖**

  **What to do**:
  - 在 `packages/ui/package.json` 添加 `@ant-design/x-sdk` 依赖
  - 在 `packages/demo/package.json` 添加依赖
  - 运行 `pnpm install`

  **Must NOT do**:
  - 不要修改现有依赖版本

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单的依赖添加任务

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Tasks 2, 3, 4

  **References**:
  - `packages/ui/package.json` - 添加依赖位置
  - `packages/demo/package.json` - 添加依赖位置

  **Acceptance Criteria**:
  - [x] `@ant-design/x-sdk` 添加到 devDependencies
  - [x] `pnpm install` 成功
  - [x] 可以 import from '@ant-design/x-sdk'

  **QA Scenarios**:
  ```
  Scenario: Verify SDK installation
    Tool: Bash
    Steps:
      1. cd packages/ui && pnpm ls @ant-design/x-sdk
    Expected Result: 显示已安装的版本号
    Evidence: .sisyphus/evidence/task-1-sdk-install.txt
  ```

  **Commit**: YES
  - Message: `chore(ui): add @ant-design/x-sdk dependency`
  - Files: `packages/ui/package.json`, `packages/demo/package.json`

---

- [x] 2. **创建 CopConChatProvider**

  **What to do**:
  - 创建 `packages/ui/src/providers/CopConChatProvider.ts`
  - 继承 `AbstractChatProvider`
  - 实现 `transformParams`：转换请求参数
  - 实现 `transformLocalMessage`：创建用户消息
  - 实现 `transformMessage`：处理 SSE 响应
  
  **关键代码结构**:
  ```typescript
  import { AbstractChatProvider, XRequest } from '@ant-design/x-sdk';
  
  interface CopConMessage {
    id: string;
    session_id: string;
    role: 'user' | 'assistant' | 'tool';
    content: string;
    reasoning?: string;
    created_at: string;
  }
  
  interface CopConInput {
    content: string;
    agentId?: string;
  }
  
  interface CopConOutput {
    type: 'message' | 'reasoning' | 'tool_call' | 'tool_result' | 'done' | 'error';
    data: Record<string, any>;
  }
  
  export class CopConChatProvider extends AbstractChatProvider<CopConMessage, CopConInput, CopConOutput> {
    // 实现...
  }
  ```

  **Must NOT do**:
  - 不要实现 `request` 方法（由 XRequest 处理）
  - 不要使用 `as any`

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 需要理解 SDK Provider 模式和后端 SSE 格式

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (depends on Task 1)
  - **Blocks**: Task 3

  **References**:
  - Skill: `~/.claude/skills/x-sdk-skills/x-chat-provider.md` - Provider 实现指南
  - `packages/ui/src/api/types.ts` - 现有类型定义
  - `packages/ui/src/api/agentClient.ts:54-115` - 当前 SSE 处理逻辑参考

  **Acceptance Criteria**:
  - [ ] 文件创建成功
  - [ ] 类继承 AbstractChatProvider
  - [ ] 实现三个必需方法
  - [ ] TypeScript 编译通过

  **QA Scenarios**:
  ```
  Scenario: Verify Provider compilation
    Tool: Bash
    Steps:
      1. cd packages/ui && pnpm build
    Expected Result: 构建成功，无错误
    Evidence: .sisyphus/evidence/task-2-provider-build.txt
  ```

  **Commit**: YES
  - Message: `feat(ui): add CopConChatProvider for SSE streaming`
  - Files: `packages/ui/src/providers/CopConChatProvider.ts`

---

- [x] 3. **重构 useAgentChat 使用 SDK**

  **What to do**:
  - 修改 `packages/ui/src/hooks/useAgentChat.ts`
  - 使用 `useXChat` 替代手动状态管理
  - 接收 Provider 实例作为参数
  - 返回与 `useXChat` 兼容的接口
  
  **新的接口设计**:
  ```typescript
  export interface UseAgentChatOptions {
    provider: CopConChatProvider;
    sessionId: string;
  }
  
  export interface UseAgentChatReturn {
    messages: MessageInfo<CopConMessage>[];  // SDK 标准类型
    isRequesting: boolean;
    onRequest: (params: CopConInput) => void;
    abort: () => void;
    reloadMessages: () => Promise<void>;
  }
  ```

  **Must NOT do**:
  - 不要在 hook 中直接处理 SSE（由 Provider 处理）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 需要理解 SDK Hook 模式

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (depends on Task 2)
  - **Blocks**: Task 5

  **References**:
  - Skill: `~/.claude/skills/x-sdk-skills/use-x-chat.md` - Hook 使用指南
  - `packages/ui/src/hooks/useAgentChat.ts` - 当前实现

  **Acceptance Criteria**:
  - [ ] 使用 useXChat
  - [ ] 接口兼容现有使用方式
  - [ ] TypeScript 编译通过

  **QA Scenarios**:
  ```
  Scenario: Verify Hook compilation
    Tool: Bash
    Steps:
      1. cd packages/ui && pnpm build
    Expected Result: 构建成功
    Evidence: .sisyphus/evidence/task-3-hook-build.txt
  ```

  **Commit**: YES
  - Message: `refactor(ui): migrate useAgentChat to useXChat SDK`
  - Files: `packages/ui/src/hooks/useAgentChat.ts`

---

- [x] 4. **简化 AgentClient**

  **What to do**:
  - 修改 `packages/ui/src/api/agentClient.ts`
  - 移除 `chat()` 方法（由 XRequest + Provider 处理）
  - 保留其他 API 方法：createSession, getSessions, getSession, deleteSession, getMessages, getTodos
  - 可选：添加 updateTodo 等方法

  **Must NOT do**:
  - 不要删除必要的 API 方法

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单的代码删除和清理

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Task 3)
  - **Blocks**: Task 5

  **References**:
  - `packages/ui/src/api/agentClient.ts` - 当前实现

  **Acceptance Criteria**:
  - [ ] chat() 方法已移除
  - [ ] 其他方法保留
  - [ ] TypeScript 编译通过

  **Commit**: YES
  - Message: `refactor(ui): remove chat method from AgentClient`
  - Files: `packages/ui/src/api/agentClient.ts`

---

- [x] 5. **更新 App.tsx 使用新架构**

  **What to do**:
  - 修改 `packages/demo/src/App.tsx`
  - 创建 Provider 实例（使用 XRequest）
  - 使用新的 useAgentChat hook
  - 使用 useMemo 优化 bubbleItems 计算
  - 修复 useEffect 依赖
  - 更新 messages 类型适配

  **关键改动**:
  ```typescript
  // 创建 Provider 实例
  const request = XRequest('/api/sessions/${activeKey}/chat');
  const provider = useMemo(() => new CopConChatProvider({ request }), [activeKey]);
  
  // 使用 Hook
  const { messages, isRequesting, onRequest, abort } = useAgentChat({ provider, sessionId: activeKey });
  
  // 优化 bubbleItems
  const bubbleItems = useMemo(() => {
    // ... 构建逻辑
  }, [messages]);
  ```

  **Must NOT do**:
  - 不要删除 TodoList 功能
  - 不要使用 as any

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: 涉及 UI 渲染优化

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (depends on Tasks 3, 4)

  **References**:
  - `packages/demo/src/App.tsx` - 当前实现
  - Skill: `~/.claude/skills/x-sdk-skills/use-x-chat.md` - UI 集成示例

  **Acceptance Criteria**:
  - [ ] 使用新的 Hook
  - [ ] useMemo 优化 bubbleItems
  - [ ] useEffect 依赖正确
  - [ ] 所有功能正常

  **QA Scenarios**:
  ```
  Scenario: Verify UI rendering
    Tool: Bash
    Steps:
      1. cd packages/demo && pnpm build
    Expected Result: 构建成功
    Evidence: .sisyphus/evidence/task-5-ui-build.txt
  
  Scenario: Verify type check
    Tool: Bash
    Steps:
      1. cd packages/demo && npx tsc --noEmit
    Expected Result: 无类型错误
    Evidence: .sisyphus/evidence/task-5-typecheck.txt
  ```

  **Commit**: YES
  - Message: `refactor(demo): migrate App.tsx to Ant Design X SDK`
  - Files: `packages/demo/src/App.tsx`

---

- [x] 6. **更新导出和清理**

  **What to do**:
  - 更新 `packages/ui/src/index.ts` 导出新类型
  - 确保所有导入正确
  - 运行完整构建验证
  - 清理未使用的代码

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单的导出更新和验证

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Final

  **References**:
  - `packages/ui/src/index.ts`

  **Acceptance Criteria**:
  - [ ] 导出正确
  - [ ] pnpm build 成功
  - [ ] 无未使用代码警告

  **QA Scenarios**:
  ```
  Scenario: Final build verification
    Tool: Bash
    Steps:
      1. cd packages/ui && pnpm build
      2. cd ../demo && pnpm build
    Expected Result: 所有构建成功
    Evidence: .sisyphus/evidence/task-6-final-build.txt
  ```

  **Commit**: YES
  - Message: `chore: update exports and cleanup`
  - Files: `packages/ui/src/index.ts`

---

## Final Verification Wave

- [x] F1. **功能完整性审计** — `oracle`
  验证所有功能正常：消息发送、流式接收、工具调用、会话管理、TodoList。

- [x] F2. **代码质量检查** — `unspecified-high`
  运行 `tsc --noEmit`，检查是否有类型错误、未使用变量、`as any`。

- [x] F3. **构建验证** — `quick`
  运行 `pnpm build` 确保 UI 和 Demo 都能构建成功。

---

## Commit Strategy

- **1**: `chore(ui): add @ant-design/x-sdk dependency`
- **2**: `feat(ui): add CopConChatProvider for SSE streaming`
- **3**: `refactor(ui): migrate useAgentChat to useXChat SDK`
- **4**: `refactor(ui): remove chat method from AgentClient`
- **5**: `refactor(demo): migrate App.tsx to Ant Design X SDK`
- **6**: `chore: update exports and cleanup`

---

## Success Criteria

### Verification Commands
```bash
cd packages/ui && pnpm build     # Expected: Build successful
cd packages/demo && pnpm build   # Expected: Build successful
```

### Final Checklist
- [x] 所有 "Must Have" 功能存在
- [x] 无 TypeScript 错误
- [x] 无 `as any` 或 `@ts-ignore`
- [x] 构建成功
- [x] 现有功能保持正常