package capabilities

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCap is a minimal Capability implementation for testing.
type stubCap struct {
	name      string
	capType   CapabilityType
	deps      []string
}

func (s *stubCap) Name() string            { return s.name }
func (s *stubCap) Type() CapabilityType     { return s.capType }
func (s *stubCap) DependsOn() []string      { return s.deps }

// resetBuiltins clears the global builtins map between tests.
func resetBuiltins() {
	builtins.Range(func(key, _ any) bool {
		builtins.Delete(key)
		return true
	})
}

func TestRegisterAndGet(t *testing.T) {
	resetBuiltins()

	cap := &stubCap{name: "test-tool", capType: CapabilityTypeTool, deps: nil}
	Register(cap)

	got, ok := Get("test-tool")
	assert.True(t, ok)
	assert.Equal(t, "test-tool", got.Name())
	assert.Equal(t, CapabilityTypeTool, got.Type())
}

func TestGet_NotFound(t *testing.T) {
	resetBuiltins()

	_, ok := Get("nonexistent")
	assert.False(t, ok)
}

func TestListByType(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "alpha", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "beta", capType: CapabilityTypeHook, deps: nil})
	Register(&stubCap{name: "gamma", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "delta", capType: CapabilityTypeSkill, deps: nil})

	tools := ListByType(CapabilityTypeTool)
	require.Len(t, tools, 2)
	assert.Equal(t, "alpha", tools[0].Name())
	assert.Equal(t, "gamma", tools[1].Name())

	hooks := ListByType(CapabilityTypeHook)
	require.Len(t, hooks, 1)
	assert.Equal(t, "beta", hooks[0].Name())

	skills := ListByType(CapabilityTypeSkill)
	require.Len(t, skills, 1)
	assert.Equal(t, "delta", skills[0].Name())

	memories := ListByType(CapabilityTypeMemory)
	assert.Len(t, memories, 0)
}

func TestExpandWildcards_ToolsStar(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "tool-a", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "tool-b", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "hook-a", capType: CapabilityTypeHook, deps: nil})

	result := ExpandWildcards([]string{"tools.*"})
	assert.Equal(t, []string{"tool-a", "tool-b"}, result)
}

func TestExpandWildcards_HooksStar(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "hook-x", capType: CapabilityTypeHook, deps: nil})
	Register(&stubCap{name: "tool-x", capType: CapabilityTypeTool, deps: nil})

	result := ExpandWildcards([]string{"hooks.*"})
	assert.Equal(t, []string{"hook-x"}, result)
}

func TestExpandWildcards_SkillsStar(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "skill-a", capType: CapabilityTypeSkill, deps: nil})
	Register(&stubCap{name: "tool-a", capType: CapabilityTypeTool, deps: nil})

	result := ExpandWildcards([]string{"skills.*"})
	assert.Equal(t, []string{"skill-a"}, result)
}

func TestExpandWildcards_MemoryStar(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "mem-a", capType: CapabilityTypeMemory, deps: nil})

	result := ExpandWildcards([]string{"memory.*"})
	assert.Equal(t, []string{"mem-a"}, result)
}

func TestExpandWildcards_StarAll(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "tool-a", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "hook-a", capType: CapabilityTypeHook, deps: nil})
	Register(&stubCap{name: "skill-a", capType: CapabilityTypeSkill, deps: nil})

	result := ExpandWildcards([]string{"*"})
	assert.Equal(t, []string{"hook-a", "skill-a", "tool-a"}, result)
}

func TestExpandWildcards_PlainNames(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "tool-a", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "tool-b", capType: CapabilityTypeTool, deps: nil})

	result := ExpandWildcards([]string{"tool-a", "tool-b"})
	assert.Equal(t, []string{"tool-a", "tool-b"}, result)
}

func TestExpandWildcards_MixedWildcardsAndNames(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "tool-a", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "hook-a", capType: CapabilityTypeHook, deps: nil})

	result := ExpandWildcards([]string{"tools.*", "hook-a"})
	assert.Equal(t, []string{"hook-a", "tool-a"}, result)
}

func TestExpandWildcards_Dedup(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "tool-a", capType: CapabilityTypeTool, deps: nil})

	result := ExpandWildcards([]string{"tool-a", "tools.*", "tool-a"})
	assert.Equal(t, []string{"tool-a"}, result)
}

func TestExpandWildcards_Empty(t *testing.T) {
	resetBuiltins()

	result := ExpandWildcards([]string{})
	assert.Nil(t, result)
}

func TestResolveDependencies_NoDeps(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "standalone", capType: CapabilityTypeTool, deps: nil})

	result, err := ResolveDependencies([]string{"standalone"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "standalone", result[0].Name())
}

func TestResolveDependencies_Chain(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "base", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "mid", capType: CapabilityTypeTool, deps: []string{"base"}})
	Register(&stubCap{name: "top", capType: CapabilityTypeTool, deps: []string{"mid"}})

	result, err := ResolveDependencies([]string{"top"})
	require.NoError(t, err)
	require.Len(t, result, 3)

	names := capNames(result)
	assert.Equal(t, []string{"base", "mid", "top"}, names)
}

func TestResolveDependencies_DAG(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "a", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "b", capType: CapabilityTypeHook, deps: nil})
	Register(&stubCap{name: "c", capType: CapabilityTypeTool, deps: []string{"a", "b"}})
	Register(&stubCap{name: "d", capType: CapabilityTypeHook, deps: []string{"c"}})

	result, err := ResolveDependencies([]string{"d"})
	require.NoError(t, err)
	require.Len(t, result, 4)

	names := capNames(result)
	assert.Equal(t, "a", names[0])
	assert.Equal(t, "b", names[1])
	assert.Less(t, indexOf(names, "a"), indexOf(names, "c"))
	assert.Less(t, indexOf(names, "b"), indexOf(names, "c"))
	assert.Less(t, indexOf(names, "c"), indexOf(names, "d"))
}

func TestResolveDependencies_MultipleRoots(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "a", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "b", capType: CapabilityTypeHook, deps: nil})
	Register(&stubCap{name: "c", capType: CapabilityTypeTool, deps: []string{"a", "b"}})

	result, err := ResolveDependencies([]string{"c"})
	require.NoError(t, err)
	require.Len(t, result, 3)

	names := capNames(result)
	assert.Less(t, indexOf(names, "a"), indexOf(names, "c"))
	assert.Less(t, indexOf(names, "b"), indexOf(names, "c"))
}

func TestResolveDependencies_Circular(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "a", capType: CapabilityTypeTool, deps: []string{"b"}})
	Register(&stubCap{name: "b", capType: CapabilityTypeHook, deps: []string{"a"}})

	_, err := ResolveDependencies([]string{"a"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestResolveDependencies_SelfCircular(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "self", capType: CapabilityTypeTool, deps: []string{"self"}})

	_, err := ResolveDependencies([]string{"self"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestResolveDependencies_ThreeWayCircular(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "a", capType: CapabilityTypeTool, deps: []string{"c"}})
	Register(&stubCap{name: "b", capType: CapabilityTypeHook, deps: []string{"a"}})
	Register(&stubCap{name: "c", capType: CapabilityTypeSkill, deps: []string{"b"}})

	_, err := ResolveDependencies([]string{"a"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestResolveDependencies_MissingCapability(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "a", capType: CapabilityTypeTool, deps: []string{"nonexistent"}})

	_, err := ResolveDependencies([]string{"a"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestResolveDependencies_WithWildcards(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "tool-a", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "tool-b", capType: CapabilityTypeTool, deps: []string{"tool-a"}})
	Register(&stubCap{name: "hook-a", capType: CapabilityTypeHook, deps: nil})

	result, err := ResolveDependencies([]string{"tools.*"})
	require.NoError(t, err)
	require.Len(t, result, 2)

	names := capNames(result)
	assert.Equal(t, []string{"tool-a", "tool-b"}, names)
}

func TestResolveDependencies_Empty(t *testing.T) {
	resetBuiltins()

	result, err := ResolveDependencies([]string{})
	require.NoError(t, err)
	assert.Len(t, result, 0)
}

func TestResolveDependencies_MultipleNamesWithSharedDeps(t *testing.T) {
	resetBuiltins()

	Register(&stubCap{name: "base", capType: CapabilityTypeTool, deps: nil})
	Register(&stubCap{name: "tool-a", capType: CapabilityTypeTool, deps: []string{"base"}})
	Register(&stubCap{name: "tool-b", capType: CapabilityTypeTool, deps: []string{"base"}})

	result, err := ResolveDependencies([]string{"tool-a", "tool-b"})
	require.NoError(t, err)
	require.Len(t, result, 3)

	names := capNames(result)
	assert.Equal(t, "base", names[0])
	assert.Less(t, indexOf(names, "base"), indexOf(names, "tool-a"))
	assert.Less(t, indexOf(names, "base"), indexOf(names, "tool-b"))
}

func capNames(caps []Capability) []string {
	names := make([]string, len(caps))
	for i, c := range caps {
		names[i] = c.Name()
	}
	return names
}

func indexOf(slice []string, val string) int {
	for i, s := range slice {
		if s == val {
			return i
		}
	}
	return -1
}
