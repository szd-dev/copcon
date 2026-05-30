package hooks

import (
	"fmt"
	"sync"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
)

type Tracer interface {
	StartSpan(name string) Span
}

type Span interface {
	End()
	SetAttribute(key, value string)
	SetError(err error)
}

type TracingPlugin struct {
	tracer Tracer

	mu       sync.Mutex
	llmSpan  Span
	toolSpan Span
}

func NewTracingPlugin(tracer Tracer) *TracingPlugin {
	return &TracingPlugin{tracer: tracer}
}

func (p *TracingPlugin) Name() string {
	return "tracing"
}

func (p *TracingPlugin) Points() []hook.HookPoint {
	return []hook.HookPoint{
		hook.BeforeLLMCall,
		hook.AfterLLMCall,
		hook.BeforeToolExecute,
		hook.AfterToolExecute,
		hook.OnToolError,
	}
}

func (p *TracingPlugin) Priority() int {
	return 200
}

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

type tracingHookCapability struct{}

func (c *tracingHookCapability) Name() string                         { return capabilities.HookTracing }
func (c *tracingHookCapability) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeHook }
func (c *tracingHookCapability) DependsOn() []string                  { return nil }
func (c *tracingHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return NewTracingPlugin(nil), nil
}