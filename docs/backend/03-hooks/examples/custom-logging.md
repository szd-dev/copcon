# 示例：自定义日志与指标收集

## 场景

你希望在每次 LLM 调用前后记录详细的请求信息：消息长度、估算 token 数、请求耗时、响应长度。这些数据用于监控和成本分析。

## 为什么选择 BeforeLLMCall + AfterLLMCall

`BeforeLLMCall` 在 LLM API 请求发出前触发，此时 `Messages` 已包含完整的上下文。`AfterLLMCall` 在 LLM 响应返回后触发，可以看到请求结果。

这两个 Point 成对使用，可以精确测量单次 LLM 调用的完整生命周期。

## 优先级考虑

日志和监控属于最高优先级的基础设施层，需要最先执行。设为 200，确保在整个 Hook 链的最前面。

## 为什么用 sync.Map 存储时间戳

`BeforeLLMCall` 和 `AfterLLMCall` 是同一个 Hook 实例在不同时刻的两次调用。需要一个地方存储 `开始时间`，让第二次调用能算出差值。`sync.Map` 以 `SessionID` 为键存储时间戳，因为同一个 Session 在不同 goroutine 中可能并发调用 LLM。

## 完整代码

```go
package metrics

import (
    "log/slog"
    "sync"
    "time"
    "unicode/utf8"

    "github.com/copcon/server/internal/hook"
)

// LLMMetricsHook 在 LLM 调用前后记录性能指标。
type LLMMetricsHook struct {
    // timers 记录每个 session 的 LLM 调用开始时间。
    // key 是 session_id，value 是 time.Time。
    timers sync.Map

    logger *slog.Logger
}

// NewLLMMetricsHook 创建一个 LLM 指标收集 Hook。
func NewLLMMetricsHook() *LLMMetricsHook {
    return &LLMMetricsHook{
        logger: slog.Default(),
    }
}

// Name 返回 Hook 标识符。
func (h *LLMMetricsHook) Name() string {
    return "llm_metrics"
}

// Points 返回 BeforeLLMCall 和 AfterLLMCall。
func (h *LLMMetricsHook) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.BeforeLLMCall, hook.AfterLLMCall}
}

// Priority 返回 200，最高优先级。
func (h *LLMMetricsHook) Priority() int {
    return 200
}

// Execute 根据 CurrentPoint 分发到不同的处理逻辑。
func (h *LLMMetricsHook) Execute(ctx *hook.HookContext) error {
    switch ctx.CurrentPoint {
    case hook.BeforeLLMCall:
        return h.onBeforeCall(ctx)
    case hook.AfterLLMCall:
        return h.onAfterCall(ctx)
    default:
        return nil
    }
}

// onBeforeCall 在 LLM 请求发送前记录请求指标。
func (h *LLMMetricsHook) onBeforeCall(ctx *hook.HookContext) error {
    // 存储开始时间
    h.timers.Store(ctx.SessionID, time.Now())

    if ctx.Messages == nil {
        return nil
    }

    messages := *ctx.Messages

    // 计算消息数
    msgCount := len(messages)

    // 计算总字符数和估算 token 数
    totalChars := 0
    for _, msg := range messages {
        totalChars += utf8.RuneCountInString(msg.Content)
    }

    // 粗略估算：中文约 1.5 字符/token，英文约 4 字符/token
    // 这里使用保守估计：3 字符/token
    estimatedTokens := totalChars / 3
    if estimatedTokens < 1 {
        estimatedTokens = 1
    }

    h.logger.Info("llm request prepared",
        "session_id", ctx.SessionID,
        "agent_id", ctx.AgentID,
        "message_count", msgCount,
        "total_chars", totalChars,
        "estimated_tokens", estimatedTokens,
    )

    return nil
}

// onAfterCall 在 LLM 响应返回后记录响应指标。
func (h *LLMMetricsHook) onAfterCall(ctx *hook.HookContext) error {
    // 获取开始时间
    start, exists := h.timers.LoadAndDelete(ctx.SessionID)
    if !exists {
        return nil
    }

    startTime, ok := start.(time.Time)
    if !ok {
        return nil
    }

    latency := time.Since(startTime)

    // 计算响应消息的指标
    msgCount := 0
    totalChars := 0

    if ctx.Messages != nil {
        messages := *ctx.Messages
        msgCount = len(messages)
        for _, msg := range messages {
            totalChars += utf8.RuneCountInString(msg.Content)
        }
    }

    h.logger.Info("llm response received",
        "session_id", ctx.SessionID,
        "agent_id", ctx.AgentID,
        "latency_ms", latency.Milliseconds(),
        "latency_seconds", latency.Seconds(),
        "response_message_count", msgCount,
        "response_total_chars", totalChars,
    )

    // 扩展：可以将指标发送到 Prometheus、InfluxDB 等监控系统
    // prometheus.RecordLLMLatency(latency)
    // prometheus.RecordLLMMessageCount(msgCount)

    return nil
}
```

## 注册代码

```go
package main

import (
    "github.com/copcon/server/internal/hook"
    "your-project/metrics"
)

func main() {
    runner := hook.NewHookRunner()

    // 创建指标收集 Hook
    metricsHook := metrics.NewLLMMetricsHook()
    runner.Register(metricsHook)

    // 将 runner 传入引擎
    // engine := agent.NewEngine(agent.WithHookRunner(runner), ...)
}
```

## 执行流程说明

### BeforeLLMCall

1. 引擎准备发送 LLM 请求
2. 触发 `BeforeLLMCall` HookPoint
3. `onBeforeCall` 记录当前时间存入 `sync.Map`
4. 遍历 `Messages`，统计消息数和字符数
5. 估算 token 数（字符数 / 3）
6. 输出 Info 日志

### AfterLLMCall

1. LLM 返回响应
2. 触发 `AfterLLMCall` HookPoint
3. `onAfterCall` 从 `sync.Map` 取出开始时间
4. 计算延迟（`time.Since`）
5. 统计响应消息数和字符数
6. 输出 Info 日志

### 日志输出示例

```text
INFO llm request prepared session_id=sess_abc123 agent_id=gpt4 message_count=12 total_chars=3400 estimated_tokens=1133
INFO llm response received session_id=sess_abc123 agent_id=gpt4 latency_ms=2340 latency_seconds=2.34 response_message_count=13 response_total_chars=4200
```

## 设计说明

**为什么用 `sync.Map` 而不是普通 map？** `BeforeLLMCall` 和 `AfterLLMCall` 可能在同一个 Session 的不同 goroutine 中交错执行。`sync.Map` 保证并发安全，`LoadAndDelete` 保证原子性，不会出现时间戳被错误使用的情况。

**为什么用 `LoadAndDelete` 而不是 `Load`？** 删除已使用的时间戳防止内存泄漏。如果某个 LLM 调用因为异常原因没有触发 `AfterLLMCall`，残留的时间戳会在下一次 LLM 调用时被覆盖（同一个 session），但清理更好的信号。

**token 估算为什么不精确？** 精确的 token 数需要用 tokenizer（如 `tiktoken`）计算。粗略估算对日志场景足够，如果需要精确的计费用 token 数，可以从 LLM API 的 `usage` 字段中获取。

## 扩展：集成 Prometheus

如果你已经部署了 Prometheus，可以把指标推送到 Prometheus：

```go
// 在 onAfterCall 方法的末尾添加
promLLMLatency.WithLabelValues(ctx.AgentID).Observe(latency.Seconds())
promLLMMessageCount.WithLabelValues(ctx.AgentID, "response").Observe(float64(msgCount))
```

## 扩展：按 Agent 分组统计

`ctx.AgentID` 标识了使用哪个 Agent（如 `gpt-4o` vs `claude-3.5`）。你可以在日志中利用这个字段按 Agent 分组统计，比较不同 LLM 的延迟和消息量。