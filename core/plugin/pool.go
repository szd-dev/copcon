package plugin

import (
	"strings"
	"sync"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/tool"
)

// ToolPool is a global, thread-safe registry for tools.
// Key is tool.Name(), following namespace.tool.name convention.
type ToolPool struct {
	mu      sync.RWMutex
	tools   map[string]tool.Tool
	enabled map[string]bool
}

func NewToolPool() *ToolPool {
	return &ToolPool{
		tools:   make(map[string]tool.Tool),
		enabled: make(map[string]bool),
	}
}

func (p *ToolPool) Register(t tool.Tool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tools[t.Name()] = t
	p.enabled[t.Name()] = true
}

func (p *ToolPool) Get(name string) (tool.Tool, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	t, ok := p.tools[name]
	return t, ok
}

func (p *ToolPool) SetEnabled(name string, enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled[name] = enabled
}

func (p *ToolPool) IsEnabled(name string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.enabled[name]
}

// Select returns tools matching the given patterns.
//
// Pattern rules:
//   - "memory.tool.memory_store" → exact match
//   - "memory.*"                → namespace prefix match (all tools starting with "memory.")
//   - "*"                       → match all tools
//
// Results are deduplicated; order is not guaranteed.
func (p *ToolPool) Select(patterns []string) []tool.Tool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	seen := make(map[string]struct{})
	result := make([]tool.Tool, 0)

	for _, pattern := range patterns {
		switch {
		case pattern == "*":
			for name, t := range p.tools {
				if !p.enabled[name] {
					continue
				}
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					result = append(result, t)
				}
			}
		case strings.HasSuffix(pattern, ".*"):
			prefix := pattern[:len(pattern)-1] // "memory.*" → "memory."
			for name, t := range p.tools {
				if !p.enabled[name] {
					continue
				}
				if strings.HasPrefix(name, prefix) {
					if _, ok := seen[name]; !ok {
						seen[name] = struct{}{}
						result = append(result, t)
					}
				}
			}
		default:
			if t, ok := p.tools[pattern]; ok {
				if !p.enabled[pattern] {
					continue
				}
				if _, dup := seen[pattern]; !dup {
					seen[pattern] = struct{}{}
					result = append(result, t)
				}
			}
		}
	}

	return result
}

// HookPool is a global, thread-safe registry for hooks.
type HookPool struct {
	mu    sync.RWMutex
	hooks []hook.Hook
}

func NewHookPool() *HookPool {
	return &HookPool{hooks: make([]hook.Hook, 0)}
}

func (p *HookPool) Register(h hook.Hook) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hooks = append(p.hooks, h)
}

func (p *HookPool) All() []hook.Hook {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]hook.Hook, len(p.hooks))
	copy(out, p.hooks)
	return out
}
