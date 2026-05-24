# Tracing Hook

The tracing hook creates spans for LLM calls and tool executions, giving you a timeline of how long each operation takes and how they relate to each other. It integrates with any distributed tracing backend that supports the OpenTelemetry protocol.

## Purpose

When an agent takes too long to respond, you need to know where the time went. Was it the LLM API call? A slow tool? The tracing hook answers these questions by:

1. Starting a span before each LLM call and ending it after the response.
2. Starting a named span before each tool execution and ending it after completion.
3. Recording errors as span attributes when tool execution fails.

Combined with a tracing backend (Jaeger, Zipkin, Grafana Tempo, etc.), you get a visual timeline of every agent turn.

## Hook Points

| Hook Point | What Happens |
|------------|-------------|
| `BeforeLLMCall` | Starts a span named `agent.llm_call` with session and agent attributes |
| `AfterLLMCall` | Ends the LLM span |
| `BeforeToolExecute` | Starts a span named `agent.tool.<tool_name>` with session, agent, and tool attributes |
| `AfterToolExecute` | Ends the tool span |
| `OnToolError` | Records the error on the tool span, then ends it |

Priority: **200** (same as logging; execution order between them depends on registration order).

## How It Works

### Span Lifecycle

The hook manages spans using a `Tracer` interface. When a span starts, it's stored on the hook struct. When the corresponding end event fires, the span is closed and cleared.

```
BeforeLLMCall    → StartSpan("agent.llm_call")   → store as llmSpan
AfterLLMCall     → llmSpan.End()                  → clear llmSpan

BeforeToolExecute → StartSpan("agent.tool.code_executor") → store as toolSpan
AfterToolExecute  → toolSpan.End()                         → clear toolSpan
OnToolError       → toolSpan.SetError(err) + toolSpan.End() → clear toolSpan
```

### Span Attributes

Each span carries these attributes:

| Attribute | LLM Span | Tool Span |
|-----------|----------|-----------|
| `session_id` | Yes | Yes |
| `agent_id` | Yes | Yes |
| `tool_name` | No | Yes |

### Error Handling

When a tool execution fails, the `OnToolError` hook point fires. The tracing hook records the error on the span before ending it:

```go
case hook.OnToolError:
    if ctx.ToolResult != nil && ctx.ToolResult.Error != "" {
        p.toolSpan.SetError(fmt.Errorf("%s", ctx.ToolResult.Error))
    }
    p.toolSpan.End()
```

This makes errors visible in your tracing backend as red spans with error details.

### Concurrency Safety

The hook uses a `sync.Mutex` to protect span state, since hooks can be called from different goroutines in concurrent agent sessions.

## Tracer Interface

The hook depends on this interface, which you implement by wrapping your tracing library of choice:

```go
type Tracer interface {
    StartSpan(name string) Span
}

type Span interface {
    End()
    SetAttribute(key, value string)
    SetError(err error)
}
```

### Example: OpenTelemetry Adapter

```go
package tracing

import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

type otelTracer struct {
    tracer trace.Tracer
}

func (t *otelTracer) StartSpan(name string) Span {
    ctx, span := t.tracer.Start(context.Background(), name)
    return &otelSpan{span: span, ctx: ctx}
}

type otelSpan struct {
    span trace.Span
    ctx  context.Context
}

func (s *otelSpan) End()                          { s.span.End() }
func (s *otelSpan) SetAttribute(key, value string) { s.span.SetAttributes(attribute.String(key, value)) }
func (s *otelSpan) SetError(err error) {
    s.span.SetStatus(codes.Error, err.Error())
    s.span.RecordError(err)
}
```

### Example: Jaeger Adapter

```go
import "github.com/jaegertracing/jaeger-client-go"

func newJaegerTracer(serviceName string) (Tracer, io.Closer, error) {
    jaegerTracer, closer, err := cfg.New(serviceName, jaegercfg.NullSampler)
    // wrap in the Tracer interface...
}
```

## Configuration

### YAML

```yaml
hooks:
  - name: "tracing"
    type: "opentelemetry"
    enabled: true
    parameters:
      exporter: "jaeger"               # jaeger, zipkin, otlp
      endpoint: "http://localhost:4318" # OTLP HTTP endpoint
      service_name: "copcon"
      sampling_rate: 0.1               # 10% of requests traced
```

### Go

```go
tracer := NewOpenTelemetryTracer("copcon", "http://jaeger:4318")

harness := core.NewHarness(core.HarnessConfig{
    Hooks: []HookSpec{
        {Name: "hooks.tracing", Enabled: true},
    },
    Tracer: tracer,  // pass the tracer to the harness
})
```

### Dependencies

The tracing hook accepts a `Tracer` at construction time. If the tracer is nil (which is the default when created via the capability system), the hook skips all operations. This means:

- You can enable the hook without configuring a tracing backend, and it will be a no-op.
- To get actual traces, you need to provide a tracer implementation.

## Performance Impact

| Aspect | Impact | Notes |
|--------|--------|-------|
| Span creation | Sub-microsecond (in-process) | The expensive part is the exporter, which runs async |
| Mutex contention | Minimal | Lock held only during span start/end, not for the span's lifetime |
| Memory | One span per active LLM/tool call | Spans are short-lived; they're created and destroyed within a single agent turn |
| Network | Depends on exporter | OTLP exporters batch and flush asynchronously |

### Tips

- Use a sampling rate below 1.0 in production. Tracing every request generates a lot of data.
- Set `sampling_rate: 1.0` in development and staging for full visibility.
- The hook creates a new span for every LLM call and every tool call. In a turn with multiple tool calls, you'll see a nested span structure in your tracing backend.
- If you're running without a tracing backend, leave the tracer nil. The no-op path is essentially free.

## Example: Agent with Jaeger Tracing

```yaml
agents:
  - name: "coder"
    model: "gpt-4"
    system_prompt: "You are a coding assistant."
    tools:
      - "code_executor"
      - "file_ops"
    hooks:
      - "logging"
      - "tracing"

hooks:
  - name: "tracing"
    enabled: true
    parameters:
      exporter: "jaeger"
      endpoint: "http://jaeger:4318"
      service_name: "copcon-coder"
      sampling_rate: 0.1
```

With Jaeger running, you can navigate to `http://localhost:16686` and see a timeline of each coder agent turn, showing exactly how long the LLM and each tool took.
