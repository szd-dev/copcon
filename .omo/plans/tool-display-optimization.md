# Tool 展示优化 - ThoughtChain 集成

## TL;DR

> **快速摘要**: 将 tool 结果消息合并到父级 assistant 消息的 tool_calls 数组中，使用 ThoughtChain 组件展示，而不是作为单独的 tool 气泡。
> 
> **交付物**:
> - 更新 CopConMessage 类型，添加 tool_call status/output 字段
> - 历史工具结果的消息合并工具
> - assistant 消息中 tool_calls 的 ThoughtChain 渲染
> - 移除单独的 toolExecutions 状态/展示
> 
> **预估工作量**: 中等
> **并行执行**: 是 - 3 个波次
> **关键路径**: 类型更新 → Provider 转换 → UI 渲染

---

## 背景

### 原始需求
用户期望优化 tool 的展示：
- assistant 消息携带 tool_calls 时，使用思维链 (ThoughtChain) 来展示
- 收到 tool 消息 (role=tool) 时，根据 tool_call_id 更新对应 tool_call 的状态，而不是展示为新气泡

### 讨论摘要
**关键讨论**:
- 当前实现将 tool 消息和 toolExecutions 渲染为单独气泡
- @ant-design/x 的 ThoughtChain 组件专为此场景设计
- 后端在 SSE 事件中发送 tool_call_id 用于关联结果和调用
- 需要合并历史消息并处理实时 SSE 更新

**研究发现**:
- ThoughtChain API: `{key, title, status, description, content, collapsible}` 状态值: 'loading' | 'success' | 'error' | 'abort'
- 模式: 构建 tool_call_id → tool 消息映射，合并到父级 tool_calls
- CopConChatProvider.transformMessage 当前忽略 tool_result 事件

### Metis 审查
**已识别的差距** (已处理):
- 后端验证需求: 确认 tool_result.id 匹配 tool_call.id 格式
- 边缘情况: 顺序竞态条件、孤立结果、多个 tool calls
- 验收标准: 多个调用、错误处理、历史加载
- 范围蔓延: 无交互控件 (取消/重试)，仅展示

**应用的护栏**:
- 不修改后端 API
- 无 tool 交互控件 (仅展示)
- 必须保持与无 tool_calls 消息的向后兼容
- ThoughtChain 位置: assistant 气泡内部，内容之前

---

## 工作目标

### 核心目标
在 assistant 消息中使用 ThoughtChain 展示 tool calls，tool 结果更新 tool_call 的 status/output 而不是作为单独气泡显示。

### 具体交付物
1. 更新 `CopConMessage.tool_calls` 类型，添加 `status` 和 `output` 字段
2. 消息预处理工具 `mergeToolMessages()` 
3. 更新 `CopConChatProvider.transformMessage` 处理 tool_result
4. 更新 `App.tsx` 为有 tool_calls 的 assistant 消息渲染 ThoughtChain
5. 废弃 useAgentChat 中单独的 `toolExecutions` 状态

### 完成定义
- [ ] Tool calls 在 assistant 消息气泡中显示为 ThoughtChain
- [ ] Tool 结果更新 tool_call status/output (无单独气泡)
- [ ] 带有 tool 结果的历史消息在会话切换时正确加载
- [ ] 一条消息中的多个 tool calls 正确渲染
- [ ] 错误的 tool 结果在 ThoughtChain 中显示错误状态

### 必须有
- tool_calls 的 ThoughtChain 展示
- 基于 tool_call_id 的合并
- 状态转换: loading → success/error

### 不可有 (护栏)
- 不修改后端 API
- 无 tool 交互控件 (取消、重试、编辑)
- 不为 role=tool 消息渲染单独的 tool 气泡
- 不使用嵌套 ThoughtChain (仅扁平结构)
- 不使用 `as any` 或 `@ts-ignore`

---

## 验证策略 (强制)

> **零人工干预** — 所有验证由 agent 执行。

### 测试决策
- **基础设施存在**: 是 (packages/demo dev server)
- **自动化测试**: 否 (通过浏览器手动 QA)
- **Agent 执行 QA**: 是 (Playwright 用于 UI 验证)

### QA 策略
每个任务包含使用 Playwright 的 agent 执行 QA 场景，验证:
- ThoughtChain 正确渲染
- 状态转换工作正常
- Tool 结果正确合并
- 多个 tool calls 正确显示

---

## 执行策略

### 并行执行波次

```
Wave 1 (立即开始 — 类型 + 工具更新):
├── Task 1: 更新 CopConMessage.tool_calls 类型 [quick]
├── Task 2: 创建 mergeToolMessages 工具函数 [quick]
└── Task 3: 更新 CopConChatProvider.transformMessage [quick]

Wave 2 (Wave 1 之后 — 集成):
├── Task 4: 更新 useAgentChat 消息处理 [unspecified-low]
├── Task 5: 更新 App.tsx ThoughtChain 渲染 [visual-engineering]
└── Task 6: 移除 toolExecutions 单独展示 [quick]

Wave FINAL (所有任务之后 — 验证):
├── Task F1: Playwright 可视化 QA (思维链展示)
├── Task F2: 代码质量审查 (类型检查，无 as any)
├── Task F3: 边缘情况测试 (多个调用、错误)
└── Task F4: 范围保真检查
-> 展示结果 -> 获取用户明确确认

关键路径: Task 1 → Task 3 → Task 5 → Task 6 → F1-F4
并行加速: 比顺序执行快约 50%
最大并发: 3 (Wave 1)
```

### 依赖矩阵

- **1**: — — 3, 5
- **2**: — — 4
- **3**: 1 — 5
- **4**: 2 — 5
- **5**: 1, 3, 4 — F1
- **6**: 5 — F1
- **F1**: 5, 6 — —
- **F2**: ALL — —
- **F3**: 5 — —
- **F4**: ALL — —

---

## TODOs

- [ ] 1. **更新 CopConMessage.tool_calls 类型**

  **做什么**:
  - 在 CopConMessage 接口的 tool_calls 项中添加 `status` 和 `output` 字段
  - 在 `packages/ui/src/providers/CopConChatProvider.ts` 中更新类型
  - 从 `packages/ui/src/index.ts` 导出更新后的类型

  **不要做**:
  - 不要修改现有的 id, name, arguments 字段
  - 不要将 status/output 设为必填 (保持可选以向后兼容)

  **推荐 Agent 配置**:
  - **Category**: `quick`
    - 原因: 简单的类型定义更新
  - **Skills**: []

  **并行化**:
  - **可并行运行**: 是
  - **并行组**: Wave 1 (与 Tasks 2, 3)
  - **阻塞**: Task 3, Task 5
  - **被阻塞**: 无

  **参考**:
  - `packages/ui/src/providers/CopConChatProvider.ts:13-18` - 当前 tool_calls 类型定义
  - `packages/ui/src/api/types.ts:60-68` - ToolExecution 类型作为状态参考

  **验收标准**:
  - [ ] CopConMessage.tool_calls 项有 `status?: 'loading' | 'success' | 'error' | 'abort'`
  - [ ] CopConMessage.tool_calls 项有 `output?: string`
  - [ ] 类型编译无错误

  **QA 场景**:
  ```
  场景: 类型定义编译通过
    工具: Bash
    步骤:
      1. cd packages/ui && pnpm tsc --noEmit
    预期结果: 无 TypeScript 错误
    证据: .sisyphus/evidence/task-1-type-check.log
  ```

  **提交**: 否 (与最终提交合并)

- [ ] 2. **创建 mergeToolMessages 工具函数**

  **做什么**:
  - 创建 `packages/ui/src/utils/messageUtils.ts`
  - 实现 `mergeToolMessages(messages: CopConMessage[]): CopConMessage[]`
  - 逻辑:
    1. 构建 tool_call_id → tool 消息的映射
    2. 对于每个有 tool_calls 的 assistant 消息，附加匹配的 tool 结果
    3. 从最终数组中过滤掉 role=tool 消息
  - 从 `packages/ui/src/index.ts` 导出

  **不要做**:
  - 不要修改原始 messages 数组 (返回新数组)
  - 不要在合并过程中丢失任何消息数据

  **推荐 Agent 配置**:
  - **Category**: `quick`
    - 原因: 纯工具函数，无副作用
  - **Skills**: []

  **并行化**:
  - **可并行运行**: 是
  - **并行组**: Wave 1 (与 Tasks 1, 3)
  - **阻塞**: Task 4
  - **被阻塞**: 无

  **参考**:
  - `packages/ui/src/providers/CopConChatProvider.ts:7-20` - CopConMessage 类型
  - Ant Design X 模式: 构建 tool_call_id 映射，然后合并

  **验收标准**:
  - [ ] 函数接受 CopConMessage[] 并返回 CopConMessage[]
  - [ ] Tool 结果附加到父级 assistant 的 tool_calls
  - [ ] role=tool 消息从输出中移除
  - [ ] 优雅处理缺失的 tool_call_id (记录警告，跳过)

  **QA 场景**:
  ```
  场景: 正确合并 tool 消息
    工具: Bash (node REPL)
    步骤:
      1. 导入 mergeToolMessages
      2. 用示例消息测试: [有 tool_calls 的 assistant, tool 结果]
      3. 验证 tool 结果附加到 tool_calls[0].output
      4. 验证输出中只有 1 条消息 (tool 已移除)
    预期结果: Tool 结果已合并，tool 消息已移除
    证据: .sisyphus/evidence/task-2-merge-test.log
  ```

  **提交**: 否 (与最终提交合并)

- [ ] 3. **更新 CopConChatProvider.transformMessage**

  **做什么**:
  - 修改 CopConChatProvider 中的 `transformMessage` 以处理 `tool_result` 事件
  - 当 tool_result 到达时:
    1. 在当前消息的 tool_calls 中按 id 查找匹配的 tool_call
    2. 更新其状态 (根据结果内容为 'success' 或 'error')
    3. 用结果数据设置 output 字段
  - 处理边缘情况: tool_result 在 tool_call 之前到达 (创建占位符)

  **不要做**:
  - 不要为 tool_result 创建新消息
  - 不要忽略 tool_result 事件 (当前行为)

  **推荐 Agent 配置**:
  - **Category**: `quick`
    - 原因: 修改 transformMessage 中现有的 switch case
  - **Skills**: []

  **并行化**:
  - **可并行运行**: 是
  - **并行组**: Wave 1 (与 Tasks 1, 2)
  - **阻塞**: Task 5
  - **被阻塞**: Task 1 (需要更新的类型)

  **参考**:
  - `packages/ui/src/providers/CopConChatProvider.ts:88-142` - transformMessage 方法
  - `packages/ui/src/providers/CopConChatProvider.ts:129-130` - 当前 tool_result 处理 (被忽略)

  **验收标准**:
  - [ ] tool_result 事件更新匹配 tool_call 的状态
  - [ ] tool_result 事件设置 tool_call 的 output
  - [ ] 错误检测: 如果结果包含 error 字段，status = 'error'
  - [ ] 优雅处理缺失的 tool_call (记录警告)

  **QA 场景**:
  ```
  场景: tool_result 更新 tool_call 状态
    工具: Bash (单元测试风格)
    步骤:
      1. 创建 provider 实例
      2. 模拟 tool_call 事件 → 验证 tool_calls 数组有 status 为 undefined 的项
      3. 模拟 tool_result 事件 → 验证 tool_calls[0].status = 'success'
      4. 验证 tool_calls[0].output 已设置
    预期结果: 状态从 undefined 转换为 'success'
    证据: .sisyphus/evidence/task-3-transform-test.log
  ```

  **提交**: 否 (与最终提交合并)

- [ ] 4. **更新 useAgentChat 消息处理**

  **做什么**:
  - 在设置历史消息时导入并使用 `mergeToolMessages`
  - 在 `loadMessages` effect 中，在 `setMessages` 之前应用合并
  - 暂时保留 toolExecutions 状态 (将在 Task 6 中移除)
  - 更新返回类型注释以注明 toolExecutions 将被废弃

  **不要做**:
  - 不要现在移除 toolExecutions 追踪 (在 Task 6 完成)
  - 不要改变 hook 的公共 API 签名

  **推荐 Agent 配置**:
  - **Category**: `unspecified-low`
    - 原因: 修改现有 hook，集成工具函数
  - **Skills**: []

  **并行化**:
  - **可并行运行**: 是
  - **并行组**: Wave 2 (与 Tasks 5, 6)
  - **阻塞**: Task 5
  - **被阻塞**: Task 2 (需要 mergeToolMessages)

  **参考**:
  - `packages/ui/src/hooks/useAgentChat.ts:144-166` - loadMessages effect
  - `packages/ui/src/hooks/useAgentChat.ts:180-186` - Return 语句

  **验收标准**:
  - [ ] 历史消息在设置前被合并
  - [ ] Tool 结果在加载时出现在 assistant 消息的 tool_calls 中
  - [ ] 无 TypeScript 错误
  - [ ] Hook 仍然返回 toolExecutions (向后兼容)

  **QA 场景**:
  ```
  场景: 历史消息加载时合并 tool 结果
    工具: Playwright
    前置条件: 存在历史中有 tool calls 的会话
    步骤:
      1. 导航到 demo 应用
      2. 选择有历史 tool calls 的会话
      3. 验证 ThoughtChain 显示 'success' 状态 (不是 'loading')
      4. 验证 tool 输出在展开内容中可见
    预期结果: 历史 tools 显示完成状态
    证据: .sisyphus/evidence/task-4-historical-load.png
  ```

  **提交**: 否 (与最终提交合并)

- [ ] 5. **更新 App.tsx ThoughtChain 渲染**

  **做什么**:
  - 从 '@ant-design/x' 导入 `ThoughtChain`
  - 对于有 tool_calls 的 assistant 消息:
    - 在内容之前渲染 ThoughtChain (作为 header 或内容上方)
    - 将 tool_calls 映射为 ThoughtChainItemType[]
    - 根据 tool_call 字段设置 status, title, description, content
  - 配置 ThoughtChain:
    - `line="dashed"` 用于视觉分隔
    - 对有 output 的项设置 `collapsible`
    - 状态图标: loading, success, error
  - 移除单独的 toolExecutions.forEach 渲染块 (164-178 行)

  **不要做**:
  - 不要在 content 内渲染 tool_calls (作为单独的 ThoughtChain 渲染)
  - 不要创建自定义状态指示器 (使用 ThoughtChain 内置)
  - 不要将 role=tool 消息渲染为单独气泡

  **推荐 Agent 配置**:
  - **Category**: `visual-engineering`
    - 原因: UI 组件集成和视觉打磨
  - **Skills**: [`/frontend-ui-ux`]
    - `/frontend-ui-ux`: ThoughtChain 样式的前端 UI/UX 模式

  **并行化**:
  - **可并行运行**: 是
  - **并行组**: Wave 2 (与 Tasks 4, 6)
  - **阻塞**: Task 6, Task F1
  - **被阻塞**: Task 1 (需要更新的类型), Task 3 (需要转换逻辑), Task 4 (需要合并的消息)

  **参考**:
  - `packages/demo/src/App.tsx:126-178` - 当前 bubbleItems 生成
  - `packages/demo/src/App.tsx:138-159` - Think 组件使用模式
  - Ant Design X 文档: ThoughtChain API 包含 status, collapsible, content

  **验收标准**:
  - [ ] ThoughtChain 出现在有 tool_calls 的 assistant 气泡中
  - [ ] 状态图标正确显示 (loading → success/error)
  - [ ] Tool 输出在 ThoughtChain 项展开时可见
  - [ ] 消息列表中无单独 tool 气泡
  - [ ] ThoughtChain 样式匹配 Think 组件样式

  **QA 场景**:
  ```
  场景: ThoughtChain 在 assistant 消息中渲染
    工具: Playwright
    步骤:
      1. 导航到 demo 应用
      2. 发送触发 tool call 的消息
      3. 验证 ThoughtChain 出现在 assistant 气泡内部
      4. 验证状态显示 'loading' 带有转圈图标
      5. 等待 tool 结果
      6. 验证状态变为 'success' 带有对勾图标
      7. 点击展开，验证输出可见
    预期结果: ThoughtChain 带有状态转换
    证据: .sisyphus/evidence/task-5-thoughtchain.png

  场景: 无单独 tool 气泡
    工具: Playwright
    步骤:
      1. tool call 完成后检查 DOM
      2. 搜索 role="tool" 的 bubble 项
      3. 验证零匹配 (tool calls 已合并到 assistant)
    预期结果: 无独立的 tool 气泡
    证据: .sisyphus/evidence/task-5-no-tool-bubbles.png
  ```

  **提交**: 否 (与最终提交合并)

- [ ] 6. **移除 toolExecutions 单独展示**

  **做什么**:
  - 移除 App.tsx 中的 `toolExecutions.forEach` 块 (164-178 行)
  - 从 useAgentChatReturn 接口移除 `toolExecutions`
  - 从 useAgentChat hook 移除 toolExecutions 状态和回调
  - 更新 App.tsx 不解构 toolExecutions
  - 保留 ToolExecution 类型导出以备将来使用

  **不要做**:
  - 不要移除 ToolExecution 类型定义 (可能有用)
  - 不要破坏可能使用 toolExecutions 的其他组件

  **推荐 Agent 配置**:
  - **Category**: `quick`
    - 原因: 移除未使用的状态和渲染代码
  - **Skills**: []

  **并行化**:
  - **可并行运行**: 是
  - **并行组**: Wave 2 (与 Tasks 4, 5)
  - **阻塞**: Task F1
  - **被阻塞**: Task 5 (需要先完成 ThoughtChain 渲染)

  **参考**:
  - `packages/ui/src/hooks/useAgentChat.ts:19-30` - Return 接口
  - `packages/ui/src/hooks/useAgentChat.ts:41-98` - toolExecutions 状态和回调
  - `packages/demo/src/App.tsx:34-43` - 解构 toolExecutions
  - `packages/demo/src/App.tsx:164-178` - toolExecutions 渲染块

  **验收标准**:
  - [ ] toolExecutions 从 useAgentChatReturn 移除
  - [ ] App.tsx 编译无 toolExecutions 引用
  - [ ] 无 toolExecutions 缺失导致的运行时错误

  **QA 场景**:
  ```
  场景: 应用编译运行无 toolExecutions
    工具: Playwright
    步骤:
      1. 启动 demo 服务器
      2. 导航到应用
      3. 验证无控制台错误
      4. 发送消息，验证聊天工作正常
    预期结果: 无错误，聊天功能正常
    证据: .sisyphus/evidence/task-6-no-errors.png
  ```

  **提交**: 否 (与最终提交合并)

---

## 最终验证波 (强制)

- [ ] F1. **可视化 QA** — `visual-engineering` + `playwright` skill
  启动 demo 服务器，导航到有 tool calls 的会话。验证:
  - ThoughtChain 出现在 assistant 气泡内部
  - 状态图标正确 (loading → success/error)
  - Tool 输出在展开时可见
  - 无单独 tool 气泡存在
  输出: `ThoughtChain [YES] | Status Icons [PASS] | No Tool Bubbles [PASS] | VERDICT`

- [ ] F2. **代码质量审查** — `quick`
  在 packages/ui 和 packages/demo 中运行 `pnpm build` + `tsc --noEmit`。检查:
  - 无 `as any` 或 `@ts-ignore`
  - 无未使用的导入
  - tool_calls 更新的类型安全
  输出: `Build [PASS] | Types [PASS] | No as any [PASS] | VERDICT`

- [ ] F3. **边缘情况测试** — `quick`
  测试场景:
  - 一条消息中多个 tool calls
  - 带错误的 tool 结果
  - 会话切换时的历史 tool calls
  输出: `Multiple Calls [PASS] | Error Handling [PASS] | Historical Load [PASS] | VERDICT`

- [ ] F4. **范围保真检查** — `quick`
  验证:
  - 无后端更改
  - 无 tool 交互控件添加
  - toolExecutions 状态已移除/废弃
  输出: `Backend Changes [NONE] | Interaction Controls [NONE] | Scope [COMPLIANT] | VERDICT`

---

## 提交策略

- 所有任务完成后 **单次提交**
- 消息: `feat(ui): display tool calls as ThoughtChain in assistant messages`
- 提交前: `pnpm build && pnpm tsc --noEmit`

---

## 成功标准

### 验证命令
```bash
cd packages/ui && pnpm build       # 预期: 构建成功
cd packages/demo && pnpm build     # 预期: 构建成功
cd packages/ui && pnpm tsc --noEmit # 预期: 无错误
```

### 最终检查清单
- [ ] 所有 "必须有" 已实现
- [ ] 所有 "不可有" 已避免
- [ ] ThoughtChain 正确渲染
- [ ] Tool 结果按 tool_call_id 合并
- [ ] 无单独 tool 气泡