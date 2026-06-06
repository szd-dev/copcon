package plugin

import (
	"fmt"
	"sync"
	"testing"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTool struct {
	name string
}

func (s *stubTool) Name() string                                                          { return s.name }
func (s *stubTool) Description() string                                                   { return "" }
func (s *stubTool) InputSchema() map[string]any                                           { return nil }
func (s *stubTool) Execute(_ iface.ChatContextInterface, _ map[string]any) (*tool.ToolResult, error) { return nil, nil }

type stubHook struct {
	name string
}

func (s *stubHook) Name() string                       { return s.name }
func (s *stubHook) Points() []hook.HookPoint           { return nil }
func (s *stubHook) Priority() int                      { return 100 }
func (s *stubHook) Execute(_ *hook.HookContext) error { return nil }

// --- Helpers ---

func toolNames(tools []tool.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name()
	}
	return names
}

func newPopulatedPool() *ToolPool {
	p := NewToolPool()
	p.Register(&stubTool{name: "memory.tool.memory_store"})
	p.Register(&stubTool{name: "memory.tool.memory_recall"})
	p.Register(&stubTool{name: "memory.tool.memory_forget"})
	p.Register(&stubTool{name: "mcp.tool.github__list_repos"})
	p.Register(&stubTool{name: "mcp.tool.slack__send_message"})
	return p
}

// --- ToolPool tests ---

func TestToolPool_RegisterAndGet(t *testing.T) {
	p := NewToolPool()
	t1 := &stubTool{name: "builtin.tool.code_executor"}
	p.Register(t1)

	got, ok := p.Get("builtin.tool.code_executor")
	require.True(t, ok)
	assert.Equal(t, "builtin.tool.code_executor", got.Name())

	_, ok = p.Get("nonexistent")
	assert.False(t, ok)
}

func TestToolPool_SelectWildcardNamespace(t *testing.T) {
	p := newPopulatedPool()

	result := p.Select([]string{"memory.*"})
	assert.Len(t, result, 3)
	assert.ElementsMatch(t, []string{
		"memory.tool.memory_store",
		"memory.tool.memory_recall",
		"memory.tool.memory_forget",
	}, toolNames(result))
}

func TestToolPool_SelectExactMatch(t *testing.T) {
	p := newPopulatedPool()

	result := p.Select([]string{"memory.tool.memory_store"})
	require.Len(t, result, 1)
	assert.Equal(t, "memory.tool.memory_store", result[0].Name())
}

func TestToolPool_SelectGlobalWildcard(t *testing.T) {
	p := newPopulatedPool()

	result := p.Select([]string{"*"})
	assert.Len(t, result, 5)
	assert.ElementsMatch(t, []string{
		"memory.tool.memory_store",
		"memory.tool.memory_recall",
		"memory.tool.memory_forget",
		"mcp.tool.github__list_repos",
		"mcp.tool.slack__send_message",
	}, toolNames(result))
}

func TestToolPool_SelectMixedPatterns(t *testing.T) {
	p := newPopulatedPool()

	result := p.Select([]string{"builtin.*", "memory.tool.memory_store"})
	assert.Len(t, result, 1)
	assert.Equal(t, "memory.tool.memory_store", result[0].Name())
}

func TestToolPool_SelectDedup(t *testing.T) {
	p := newPopulatedPool()

	result := p.Select([]string{"memory.*", "memory.tool.memory_store", "memory.*"})
	names := toolNames(result)
	assert.Len(t, names, 3)
	assert.ElementsMatch(t, []string{
		"memory.tool.memory_store",
		"memory.tool.memory_recall",
		"memory.tool.memory_forget",
	}, names)
}

func TestToolPool_SelectEmpty(t *testing.T) {
	p := newPopulatedPool()

	result := p.Select([]string{})
	assert.Len(t, result, 0)
}

func TestToolPool_SelectNonExistent(t *testing.T) {
	p := newPopulatedPool()

	result := p.Select([]string{"nonexistent.tool.foo"})
	assert.Len(t, result, 0)
}

func TestToolPool_ConcurrentAccess(t *testing.T) {
	p := NewToolPool()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p.Register(&stubTool{name: fmt.Sprintf("concurrent.tool.%d", idx)})
		}(i)
	}
	wg.Wait()

	result := p.Select([]string{"*"})
	assert.Len(t, result, 100)
}

// --- HookPool tests ---

func TestHookPool_RegisterAndAll(t *testing.T) {
	p := NewHookPool()
	h1 := &stubHook{name: "builtin.hook.logging"}
	h2 := &stubHook{name: "memory.hook.file_memory"}

	p.Register(h1)
	p.Register(h2)

	all := p.All()
	require.Len(t, all, 2)
	names := make([]string, len(all))
	for i, h := range all {
		names[i] = h.Name()
	}
	assert.ElementsMatch(t, []string{"builtin.hook.logging", "memory.hook.file_memory"}, names)
}

func TestHookPool_AllReturnsCopy(t *testing.T) {
	p := NewHookPool()
	p.Register(&stubHook{name: "a"})

	all := p.All()
	all[0] = &stubHook{name: "mutated"}

	original := p.All()
	assert.Equal(t, "a", original[0].Name())
}

func TestHookPool_ConcurrentAccess(t *testing.T) {
	p := NewHookPool()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Register(&stubHook{name: "hook"})
		}()
	}
	wg.Wait()

	all := p.All()
	assert.Len(t, all, 100)
}
