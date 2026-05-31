

## Task: Memory 页面从 Session 绑定改为 Agent 绑定

### 修改范围
- `packages/chat-core/src/types.ts`: 新增 `Agent` 接口；`Session` 增加 `default_agent_id?: string`；`Memory` 的 `session_id` 改为 `agent_id`
- `packages/chat-core/src/index.ts`: 导出 `Agent` 类型
- `packages/chat-core/src/agent-client.ts`: 新增 `getAgents()`；`getSessionMemories` → `getAgentMemories`；`deleteSessionMemory` → `deleteAgentMemory`；URL 从 `/api/sessions/...` 改为 `/api/agents/...`
- `packages/chat-core/src/agent-client.test.ts`: 删除旧 memory 测试，新增 `getAgents`、`getAgentMemories`、`deleteAgentMemory` 测试
- `packages/demo/src/pages/MemoryPage.tsx`: 下拉框从 session 列表改为 agent 列表；状态和方法全改为 agent-based
- `packages/demo/src/components/memory/MemoryPanel.tsx`: prop `sessionId` → `agentId`；方法调用同步更新
- `packages/demo/src/pages/ChatPage.tsx`: 从当前 session 取 `default_agent_id` 传给 `MemoryPanel`

### 关键踩坑
- **pnpm workspace 跨包类型不同步**: demo 的 tsconfig 使用 `moduleResolution: "bundler"`，通过 package.json `types` 字段解析到 `./dist/index.d.ts`。修改 chat-core 源码后，**必须先 build chat-core**（`pnpm --filter @copcon/chat-core build`），否则 demo 的 `tsc` 会使用旧的 dist 类型定义，报大量 `Property does not exist` 错误。
- 前端构建通过，测试全部通过（chat-core 55 tests passed，demo build 成功）。
