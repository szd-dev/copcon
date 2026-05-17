// Package tracing provides a span-based tracing plugin for the agent engine
// hook system. It defines the Tracer and Span interfaces (not OpenTelemetry)
// and a TracingPlugin that creates spans at key lifecycle points.
//
// The plugin creates spans at these hook points:
//   - BeforeLLMCall / AfterLLMCall: "agent.llm_call"
//   - BeforeToolExecute / AfterToolExecute / OnToolError: "agent.tool.{name}"
//
// Each span is decorated with session_id and agent_id attributes.
// When no Tracer is configured, the plugin is a zero-cost no-op.
package tracing

import (
	"fmt"
	"sync"

	"github.com/copcon/server/internal/hook"
)

// Tracer is a factory for creating Spans. Implementations of this interface
// wrap concrete tracing backends (e.g., OpenTelemetry, Datadog, or a simple
// in-memory recorder for tests).
type Tracer interface {
	// StartSpan creates and starts a new span with the given name.
	// The returned Span must have its End method called when the
	// traced operation completes.
	StartSpan(name string) Span
}

// Span represents a single traced operation. Callers must call End
// exactly once when the operation completes.
type Span interface {
	// End marks the span as complete. Must be called once.
	End()

	// SetAttribute attaches a key-value attribute to the span.
	SetAttribute(key, value string)

	// SetError records an error on the span.
	SetError(err error)
}

// TracingPlugin creates and manages spans at key hook points in the
// agent engine lifecycle. It implements hook.Hook.
type TracingPlugin struct {
	tracer Tracer

	mu       sync.Mutex
	llmSpan  Span
	toolSpan Span
}

// NewTracingPlugin creates a new TracingPlugin. If tracer is nil, the
// plugin operates as a no-op — all Execute calls return immediately
// without allocation.
func NewTracingPlugin(tracer Tracer) *TracingPlugin {
	return &TracingPlugin{tracer: tracer}
}

// Name returns a human-readable identifier for this hook.
func (p *TracingPlugin) Name() string {
	return "tracing"
}

// Points returns the set of hook points at which this plugin is active.
func (p *TracingPlugin) Points() []hook.HookPoint {
	return []hook.HookPoint{
		hook.BeforeLLMCall,
		hook.AfterLLMCall,
		hook.BeforeToolExecute,
		hook.AfterToolExecute,
		hook.OnToolError,
	}
}

// Priority returns the execution order for this hook. 200 ensures tracing
// runs after most business-logic hooks but before high-priority infrastructure
// hooks.
func (p *TracingPlugin) Priority() int {
	return 200
}

// Execute dispatches the hook point to the appropriate span lifecycle
// operation. When the tracer is nil, this is a no-op.
func (p *TracingPlugin) Execute(ctx *hook.HookContext) error {
	if p.tracer == nil {
		return nil
	}

	switch ctx.CurrentPoint {
	case hook.BeforeLLMCall:
		span := p.tracer.StartSpan("agent.llm_call")
		span.SetAttribute("session_id", ctx.SessionID)
		span.SetAttribute("agent_id", ctx.AgentID)

		p.mu.Lock()
		p.llmSpan = span
		p.mu.Unlock()

	case hook.AfterLLMCall:
		p.mu.Lock()
		if p.llmSpan != nil {
			p.llmSpan.End()
			p.llmSpan = nil
		}
		p.mu.Unlock()

	case hook.BeforeToolExecute:
		name := fmt.Sprintf("agent.tool.%s", ctx.ToolName)
		span := p.tracer.StartSpan(name)
		span.SetAttribute("session_id", ctx.SessionID)
		span.SetAttribute("agent_id", ctx.AgentID)
		span.SetAttribute("tool_name", ctx.ToolName)

		p.mu.Lock()
		p.toolSpan = span
		p.mu.Unlock()

	case hook.AfterToolExecute:
		p.mu.Lock()
		if p.toolSpan != nil {
			p.toolSpan.End()
			p.toolSpan = nil
		}
		p.mu.Unlock()

	case hook.OnToolError:
		p.mu.Lock()
		if p.toolSpan != nil {
			if ctx.ToolResult != nil && ctx.ToolResult.Error != "" {
				p.toolSpan.SetError(fmt.Errorf("%s", ctx.ToolResult.Error))
			}
			p.toolSpan.End()
			p.toolSpan = nil
		}
		p.mu.Unlock()
	}

	return nil
}
