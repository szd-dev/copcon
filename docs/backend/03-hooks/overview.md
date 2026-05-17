# Hook 系统概览

## 什么是 Hook

Hook（钩子）是 CopCon 的 **核心-外围（Core-Periphery）架构**的关键组成部分。Hook 允许外部代码在 Agent Engine（核心引擎）的关键生命周期节点上插入自定义逻辑，而无需修改核心代码。

简单来说：Hook 就是一段"插播"代码。当引擎走到某个特定阶段时，会暂停一下，依次执行你注册的所有 Hook，然后再继续往下走。

### 为什么需要 Hook 系统

想象以下几种场景：

1. 你想在每次发送给 LLM 的消息中，过滤掉用户的敏感词
2. 你希望在工具执行之前，检查参数是否合法（比如禁止执行危险命令）
3. 你需要在每次 LLM 调用前后记录请求耗时，用于监控
4. 你想根据用户的最后一条消息，从向量数据库里检索相关文档并注入上下文

如果这些逻辑全部硬编码在引擎里，那么代码会变得臃肿、难以维护。Hook 系统把这些"外围逻辑"从核心引擎中剥离出来，让你可以通过实现接口、注册 Hook 的方式来扩展系统能力。

### 核心设计理念

**非侵入式扩展**。Hook 不需要修改核心代码，只需实现接口并注册即可。

**失败不影响主流程**。单个 Hook 出错（返回 error）不会阻塞链路，其他 Hook 依然照常执行。Hook panic 会被 recover 并记录日志，不会导致引擎崩溃。

**多 Hook 有序执行**。同一个 HookPoint 上可以注册多个 Hook，它们按优先级排序执行。优先级高的先跑，优先级相同的按注册时间排序。

**上下文可修改**。每个 Hook 都能拿到 `HookContext`，其中包含该时刻所有可用的上下文数据。对于指针类型的字段（如 `*string`、`*[]MessageForLLM`），Hook 可以直接修改它们，影响后续流程。

---

## 术语区分：Hook、中间件、插件

| 概念 | 在 CopCon 中的含义 |
|------|--------------------|
| **Hook** | 实现 `hook.Hook` 接口的结构体，在特定 HookPoint 执行。最细粒度的扩展单元。 |
| **中间件 (Middleware)** | CopCon 中没有独立的中间件概念。Hook 在功能上涵盖了中间件的作用。 |
| **插件 (Plugin)** | 一个插件通常包含一组相关的 Hook，以及自己的初始化和资源管理。例如 `MemoryPlugin` 同时注册了 `AfterContextBuild` 和 `OnMessagePersist` 两个 HookPoint。 |

简单的关系：**插件 ≥ Hook**。一个插件可以包含多个 Hook，也可以把自己注册为一个复合 Hook（实现 Hook 接口并覆盖多个 Point）。

---

## 四种执行保证

HookRunner 在设计上对 Hook 的执行做了四重保障：

### 1. Panic 恢复

如果某个 Hook 在执行过程中 panic，HookRunner 会捕获并记录日志，然后继续执行剩余的 Hook。整个链路不会因为一个 Hook 的崩溃而中断。

```go
// runner.go 中的实现
defer func() {
    if rec := recover(); rec != nil {
        slog.Error("hook panicked",
            "hook", h.Name(),
            "panic", rec,
            "point", ctx.CurrentPoint,
        )
    }
}()
```

### 2. 错误非阻塞

Hook 的 `Execute` 方法返回 error 时，只会记录一条 Warn 级别的日志。链路不会中断，后续 Hook 照样执行。

```go
if err := h.Execute(ctx); err != nil {
    slog.Warn("hook returned error",
        "hook", h.Name(),
        "error", err,
        "point", ctx.CurrentPoint,
    )
}
```

### 3. Context 取消检测

在执行任何 Hook 之前，Runner 会检查 `ctx.ChatCtx.Context()` 是否已经取消（超时或主动取消）。如果已取消，整个 Hook 链跳过，直接返回。这样避免了在请求已经失效的情况下继续执行无意义的逻辑。

```go
if err := ctx.ChatCtx.Context().Err(); err != nil {
    return
}
```

### 4. 并发安全

HookRunner 内部使用 `sync.Mutex` 保护注册表。注册 Hook 和执行 Hook 可以安全地在不同 goroutine 中并发调用。

---

## 优先级系统

每个 Hook 必须实现 `Priority() int` 方法，返回一个整数表示优先级。

**规则：优先级数值越大，越早执行。**

```text
注册的 Hook 按优先级降序排列：

Priority: 150  ← 最先执行
Priority: 100
Priority:  50
Priority:  10  ← 最后执行
```

如果多个 Hook 返回相同优先级，则按注册时间排序（先注册的先执行）。

### 推荐优先级分层

| 层级 | 优先级范围 | 用途 | 示例 |
|------|-----------|------|------|
| 高优先级 | 100 - 200 | 基础设施类：需要最先介入的通用逻辑 | 日志记录、链路追踪、请求改写 |
| 默认 | 100 | 系统默认值 | MemoryPlugin |
| 中优先级 | 50 - 99 | 上下文增强类：需要时机较早的业务逻辑 | 知识库注入、提示词拼接 |
| 低优先级 | 1 - 49 | 业务逻辑类：最后执行的特定逻辑 | 敏感词过滤、结果转换 |

实际项目中使用 20 及以下、50、100 这三个档位通常就足够了。不需要过于精细的数值划分。

---

## HookPoint 速查表

以下是 CopCon 引擎暴露的全部 10 个 HookPoint：

| HookPoint | 触发时机 | 核心用途 | 可修改字段 |
|-----------|---------|---------|-----------|
| `before_context_build` | 上下文窗口构建之前 | 修改系统提示词 | `SystemPrompt` |
| `after_context_build` | 上下文构建完成后，发送 LLM 前 | 注入外部知识、增删消息 | `Messages` |
| `on_system_prompt` | 系统提示词解析时 | 替换或增强提示词 | `SystemPrompt` |
| `on_message_persist` | 消息持久化之前 | 消息过滤、脱敏、存储 | `Messages` |
| `before_tool_execute` | 工具调用前 | 参数校验、权限控制、参数改写 | `ToolArgs` |
| `after_tool_execute` | 工具执行成功后 | 结果转换、截断、脱敏 | `ToolResult` |
| `on_tool_error` | 工具执行失败时 | 错误处理、重试、提供兜底结果 | `ToolResult` |
| `before_llm_call` | LLM API 请求发送前 | 修改请求参数、追加消息 | `Messages` |
| `after_llm_call` | LLM API 响应返回后 | 响应处理、结果提取、日志记录 | `Messages` |
| `on_session_resolve` | 会话 ID 解析时 | 自定义会话查找逻辑 | 无（需通过接口实现） |

每个 HookPoint 在 `HookContext` 中可用的字段不同。详细说明见 [HookContext 字段参考](./hook-context-fields.md)。

---

## 下一步

- [编写一个 Hook](./writing-a-hook.md) — 从零开始实现一个完整的 Hook
- [Hook 注册与运行](./hook-registry.md) — 了解 HookRunner 的接口和注册规则
- [HookContext 字段参考](./hook-context-fields.md) — 每个字段的详细说明和可用时机
- [示例：敏感词过滤器](./examples/sensitive-filter.md) — 实战示例
- [示例：RAG 知识注入](./examples/rag-knowledge.md) — 实战示例