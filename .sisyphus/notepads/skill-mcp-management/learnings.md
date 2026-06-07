# Learnings - MCP Plugin Management

## 2026-06-07: MCPPlugin 结构体重命名与状态管理

- `mcpPlugin` 重命名为 `MCPPlugin`（导出），允许外部类型断言访问公共方法
- 测试文件中有 3 处 `*mcpPlugin` 类型断言需同步更新
- `enabledServers` map 在 `Init()` 中初始化，key 为 server name，value 默认 true
- `discoverTools()` 中启用检查放在连接检查之前，避免无效连接尝试
- `RemoveServer` 使用切片删除 `append(p.configs[:i], p.configs[i+1:]...)` 模式
- `Servers()` 返回 configs 副本（copy），防止外部修改内部状态
- `AddServer` 不触发自动连接，连接由外部管理

## QA Verification (2026-06-07)

- All 17 API handler tests pass (Skills: 7, MCP: 11)
- All plugin tests pass (skill: 33, mcp: 28, core: 25)
- Env field correctly excluded from MCPServerInfo API response (security verified)
- persistConfig uses atomic write (temp file + os.Rename) and preserves env from existing config
- Frontend-backend API contract fully aligned: all 8 client methods match backend routes
- Popconfirm wraps all destructive operations (enable/disable toggle, delete)
- Nil provider check returns 503 consistently in all handlers
