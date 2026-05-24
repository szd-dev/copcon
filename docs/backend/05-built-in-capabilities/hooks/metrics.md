# Metrics Hook

The metrics hook is a planned capability that will expose agent and tool performance metrics to Prometheus or other monitoring backends. It is listed here for reference, but **no implementation exists in the current codebase**.

## Planned Purpose

Once implemented, the metrics hook will track quantitative data about agent behavior: how many LLM calls are made, how long they take, how often tools succeed or fail, token usage, and session throughput. This complements the logging hook (which records events) and the tracing hook (which records timelines) with aggregate numbers that are useful for dashboards and alerts.

## Planned Hook Points

Based on the configuration schema and the pattern set by the logging and tracing hooks, the metrics hook would likely listen to these points:

| Hook Point | Planned Metrics |
|------------|----------------|
| `BeforeLLMCall` | Increment `llm_calls_total` counter |
| `AfterLLMCall` | Record `llm_call_duration_seconds` histogram, `llm_tokens_used` counter |
| `BeforeToolExecute` | Increment `tool_calls_total` counter by tool name |
| `AfterToolExecute` | Record `tool_call_duration_seconds` histogram by tool name |
| `OnToolError` | Increment `tool_errors_total` counter by tool name |

## Planned Configuration

```yaml
hooks:
  - name: "metrics"
    type: "prometheus"
    enabled: true
    parameters:
      port: 9090
      path: "/metrics"
      labels:
        - "session_id"
        - "agent_id"
        - "tool_name"
```

## How to Track Metrics Today

While the metrics hook isn't implemented yet, you can still collect metrics using these approaches:

### Option 1: Custom Hook

Write a custom hook that implements the `hook.Hook` interface and records Prometheus metrics:

```go
package metrics

import (
    "github.com/copcon/core/hook"
    "github.com/prometheus/client_golang/prometheus"
)

type MetricsHook struct {
    llmCalls    prometheus.Counter
    llmDuration prometheus.Histogram
    toolCalls   *prometheus.CounterVec
}

func (h *MetricsHook) Name() string      { return "metrics" }
func (h *MetricsHook) Priority() int      { return 300 }
func (h *MetricsHook) Points() []hook.HookPoint {
    return []hook.HookPoint{
        hook.BeforeLLMCall,
        hook.AfterLLMCall,
        hook.BeforeToolExecute,
        hook.AfterToolExecute,
    }
}

func (h *MetricsHook) Execute(ctx *hook.HookContext) error {
    switch ctx.CurrentPoint {
    case hook.BeforeLLMCall:
        h.llmCalls.Inc()
    case hook.AfterLLMCall:
        // record duration (requires timing mechanism)
    case hook.BeforeToolExecute:
        h.toolCalls.WithLabelValues(ctx.ToolName).Inc()
    }
    return nil
}
```

Register it as a capability and enable it in your Harness config. See the [Custom Hook Guide](../../06-extending/custom-hook.md) for details.

### Option 2: Log Aggregation

If the [logging hook](logging.md) is enabled, you can extract metrics from structured logs using a tool like Vector, Fluentd, or Logstash. Parse `before_llm_call` and `after_llm_call` entries to compute call counts and durations.

### Option 3: Tracing Backend Metrics

The [tracing hook](tracing.md) with Jaeger or Grafana Tempo can produce aggregate metrics from trace data. Most tracing backends support metric extraction from spans.

## Status

The metrics hook is tracked as a future enhancement. When it's implemented, this document will be updated with the actual interface, configuration, and usage details.