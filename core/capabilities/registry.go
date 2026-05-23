package capabilities

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

// CapabilityType enumerates the kinds of capabilities the registry manages.
type CapabilityType string

const (
	CapabilityTypeTool   CapabilityType = "tool"
	CapabilityTypeHook   CapabilityType = "hook"
	CapabilityTypeSkill  CapabilityType = "skill"
	CapabilityTypeMemory CapabilityType = "memory"
)

// Capability is the base interface that every registrable capability must implement.
type Capability interface {
	// Name returns a unique identifier for this capability.
	Name() string

	// Type returns the capability type (tool, hook, skill, memory).
	Type() CapabilityType

	// DependsOn returns the names of other capabilities that must be
	// initialized before this one. The registry uses this for
	// dependency resolution and topological ordering.
	DependsOn() []string
}

// ToolCapability extends Capability with the ability to produce a tool.Tool
// instance given its dependencies.
type ToolCapability interface {
	Capability
	NewTool(deps CapabilityDeps) (tool.Tool, error)
}

// HookCapability extends Capability with the ability to produce a hook.Hook
// instance given its dependencies.
type HookCapability interface {
	Capability
	NewHook(deps CapabilityDeps) (hook.Hook, error)
}

// CapabilityDeps collects the dependencies that capabilities may request
// when they are instantiated via NewTool or NewHook.
type CapabilityDeps struct {
	SessionStore  storage.SessionStore
	MessageStore  storage.MessageStore
	TodoStore     storage.TodoStore
	MemoryStore   storage.MemoryStore
	AgentRegistry agent.AgentRegistry
	Engine        interface{} // AgentEngine — typed as interface{} to avoid circular imports
	Logger        *slog.Logger
}

// builtins is the global registry of capabilities. Keys are capability names.
var builtins sync.Map

// Register adds a capability to the global registry. It is safe to call
// from multiple goroutines and from init() functions.
func Register(c Capability) {
	builtins.Store(c.Name(), c)
}

// Get retrieves a capability by name from the global registry.
func Get(name string) (Capability, bool) {
	val, ok := builtins.Load(name)
	if !ok {
		return nil, false
	}
	return val.(Capability), true
}

// ListByType returns all registered capabilities of a given type.
// The returned slice is sorted by name for deterministic ordering.
func ListByType(t CapabilityType) []Capability {
	var result []Capability
	builtins.Range(func(key, value any) bool {
		c := value.(Capability)
		if c.Type() == t {
			result = append(result, c)
		}
		return true
	})
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

// ExpandWildcards expands wildcard patterns in the names list.
// Supported patterns:
//   - "tools.*"   → all capabilities of type "tool"
//   - "hooks.*"   → all capabilities of type "hook"
//   - "skills.*"  → all capabilities of type "skill"
//   - "memory.*"  → all capabilities of type "memory"
//   - "*"         → all registered capabilities
//
// Non-wildcard names are passed through unchanged. The result is deduplicated
// and sorted by name.
func ExpandWildcards(names []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, name := range names {
		switch {
		case name == "*":
			builtins.Range(func(key, value any) bool {
				n := key.(string)
				if !seen[n] {
					seen[n] = true
					result = append(result, n)
				}
				return true
			})
		case name == "tools.*":
			for _, c := range ListByType(CapabilityTypeTool) {
				if !seen[c.Name()] {
					seen[c.Name()] = true
					result = append(result, c.Name())
				}
			}
		case name == "hooks.*":
			for _, c := range ListByType(CapabilityTypeHook) {
				if !seen[c.Name()] {
					seen[c.Name()] = true
					result = append(result, c.Name())
				}
			}
		case name == "skills.*":
			for _, c := range ListByType(CapabilityTypeSkill) {
				if !seen[c.Name()] {
					seen[c.Name()] = true
					result = append(result, c.Name())
				}
			}
		case name == "memory.*":
			for _, c := range ListByType(CapabilityTypeMemory) {
				if !seen[c.Name()] {
					seen[c.Name()] = true
					result = append(result, c.Name())
				}
			}
		default:
			if !seen[name] {
				seen[name] = true
				result = append(result, name)
			}
		}
	}

	sort.Strings(result)
	return result
}

// ResolveDependencies takes a list of capability names (which may include
// wildcards), expands them, transitively resolves all dependencies, and
// returns the capabilities in topological order such that all dependencies
// appear before the capabilities that depend on them.
//
// Returns an error if:
//   - a referenced capability is not registered
//   - a circular dependency is detected
func ResolveDependencies(names []string) ([]Capability, error) {
	expanded := ExpandWildcards(names)

	// Collect all required capabilities (including transitive deps).
	required := make(map[string]Capability)
	if err := collectTransitive(expanded, required); err != nil {
		return nil, err
	}

	inDegree := make(map[string]int)
	dependents := make(map[string][]string)

	for name, cap := range required {
		if _, exists := inDegree[name]; !exists {
			inDegree[name] = 0
		}
		for _, dep := range cap.DependsOn() {
			dependents[dep] = append(dependents[dep], name)
			inDegree[name]++
		}
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	var order []Capability
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		cap, ok := required[name]
		if !ok {
			return nil, fmt.Errorf("capability %q referenced in dependency but not collected", name)
		}
		order = append(order, cap)

		deps := dependents[name]
		sort.Strings(deps)
		for _, dep := range deps {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(required) {
		return nil, fmt.Errorf("circular dependency detected among capabilities")
	}

	return order, nil
}

// collectTransitive walks the dependency graph starting from the given names
// and populates the required map with every reachable capability.
func collectTransitive(names []string, required map[string]Capability) error {
	for _, name := range names {
		if _, already := required[name]; already {
			continue
		}

		cap, ok := Get(name)
		if !ok {
			return fmt.Errorf("capability %q not registered", name)
		}
		required[name] = cap

		if err := collectTransitive(cap.DependsOn(), required); err != nil {
			return err
		}
	}
	return nil
}
