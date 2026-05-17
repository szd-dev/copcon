# 追踪插件

`TracingPlugin` 为 Agent 引擎提供基于 Span 的链路追踪能力。它定义了自己的 `Tracer` 和 `Span` 接口（而非直接依赖 OpenTelemetry），通过适配器模式支持任意追踪后端。

**文件位置**: `server/internal/plugins/tracing/tracing_plugin.go`

## Tracer / Span 接口

TracingPlugin 定义了两个简洁的抽象接口：

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

这个设计将 CopCon 的追踪需求与具体实现解耦。任意满足这两个接口的追踪后端（OpenTelemetry、Datadog、内存 recorder）都可以作为 `Tracer` 注入。

| 方法 | 说明 |
|------|------|
| `StartSpan(name)` | 创建并启动一个命名 Span |
| `End()` | 标记 Span 完成，必须只调用一次 |
| `SetAttribute(k, v)` | 附加键值属性到 Span |
| `SetError(err)` | 在 Span 上记录错误 |

## Hook 元数据

| 属性 | 值 | 说明 |
|------|-----|------|
| `Name()` | `"tracing"` | 标识符 |
| `Points()` | `[BeforeLLMCall, AfterLLMCall, BeforeToolExecute, AfterToolExecute, OnToolError]` | 5 个 hook 点 |
| `Priority()` | `200` | 高优先级 |

## Span 生命周期

TracingPlugin 管理两类 Span：LLM 调用 Span 和工具调用 Span。每个 Span 在 `Before` hook 创建，在 `After`（或 `OnToolError`）hook 结束。

### agent.llm_call Span

追踪 LLM API 调用的完整生命周期：

```
BeforeLLMCall → StartSpan("agent.llm_call")
    ├─ SetAttribute("session_id", ...)
    ├─ SetAttribute("agent_id", ...)
    └─ 等待 LLM 响应...

AfterLLMCall → llmSpan.End()
```

创建的 Span 名称固定为 `"agent.llm_call"`。

### agent.tool.{name} Span

追踪每个工具调用的完整生命周期：

```
BeforeToolExecute → StartSpan("agent.tool.{toolName}")
    ├─ SetAttribute("session_id", ...)
    ├─ SetAttribute("agent_id", ...)
    ├─ SetAttribute("tool_name", ...)
    └─ 等待工具执行...

AfterToolExecute → toolSpan.End()

或异常情况：

OnToolError → toolSpan.SetError(error) → toolSpan.End()
```

Span 名称格式为 `"agent.tool."` + 工具名称，例如 `"agent.tool.code_executor"`。

## Span 属性

每个 Span 自动附加以下属性：

| 属性 | 来源 | Span 类型 | 说明 |
|------|------|-----------|------|
| `session_id` | `ctx.SessionID` | 全部 | 会话标识 |
| `agent_id` | `ctx.AgentID` | 全部 | Agent 标识 |
| `tool_name` | `ctx.ToolName` | tool span | 工具名称 |

## Nil Tracer 零开销模式

当 `TracingPlugin` 以 nil tracer 创建时，所有 `Execute` 调用直接返回 nil，不产生任何 Span 创建或内存分配：

```go
func (p *TracingPlugin) Execute(ctx *hook.HookContext) error {
    if p.tracer == nil {
        return nil
    }
    // ...
}
```

这意味着追踪功能可以在不修改代码的情况下按需启用或关闭。

## 并发安全

TracingPlugin 使用 `sync.Mutex` 保护内部 Span 引用：

```go
type TracingPlugin struct {
    tracer   Tracer
    mu       sync.Mutex
    llmSpan  Span
    toolSpan Span
}
```

LLM Span 和工具 Span 分别存储。锁保护确保 `BeforeLLMCall` 创建的 Span 能被 `AfterLLMCall` 安全地读取和结束。

## 生产环境 OpenTelemetry 集成

### 实现 Tracer 适配器

```go
type OTelTracer struct {
    tracer trace.Tracer
}

func NewOTelTracer(tracer trace.Tracer) *OTelTracer {
    return &OTelTracer{tracer: tracer}
}

func (t *OTelTracer) StartSpan(name string) tracing.Span {
    ctx, span := t.tracer.Start(context.Background(), name)
    return &OTelSpan{span: span}
}

type OTelSpan struct {
    span trace.Span
}

func (s *OTelSpan) End() {
    s.span.End()
}

func (s *OTelSpan) SetAttribute(key, value string) {
    s.span.SetAttributes(attribute.String(key, value))
}

func (s *OTelSpan) SetError(err error) {
    s.span.RecordError(err)
    s.span.SetStatus(codes.Error, err.Error())
}
```

### 注册示例

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// 初始化 OTel
exp, _ := otlptracegrpc.New(context.Background())
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
otel.SetTracerProvider(tp)

// 创建适配器
otTracer := otel.Tracer("copcon-agent")
tracingTracer := NewOTelTracer(otTracer)

// 创建 TracingPlugin
tracingPlugin := tracing.NewTracingPlugin(tracingTracer)

// 注册
runner.Register(tracingPlugin)
```

### 测试用内存 Tracer

测试环境可以使用内存 Tracer，无需外部依赖：

```go
type InMemorySpan struct {
    Name       string
    Attributes map[string]string
}

type InMemoryTracer struct {
    Spans []*InMemorySpan
}

// ... 实现 Tracer / Span 接口 ...

// 测试中使用
memTracer := &InMemoryTracer{}
tracingPlugin := tracing.NewTracingPlugin(memTracer)
// 验证 memTracer.Spans 中的 Span 数据
```