package capabilities

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

// ErrDependencyUnavailable is returned by a capability's NewHook or NewTool
// when a required dependency is not available. Build() catches this error
// and skips the capability gracefully (logging an info message) instead
// of failing.
var ErrDependencyUnavailable = errors.New("dependency unavailable")

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
	SessionStore        storage.SessionStore
	MessageStore        storage.MessageStore
	TodoStore           storage.TodoStore
	MemoryStore         interface{} // was: storage.MemoryStore — moved to plugins/memory-file
	FileMemoryStore     interface{} // filememory.FileMemoryStore — typed as interface{} to avoid circular imports
	AgentRegistry       agent.AgentRegistry
	Engine              interface{} // AgentEngine — typed as interface{} to avoid circular imports
	Logger              *slog.Logger
	KnowledgeStore      interface{} // storage.KnowledgeStore — typed as interface{} to avoid circular imports
	Embedder            interface{} // embedding.Embedder — typed as interface{} to avoid circular imports
	AgentKnowledgeBases map[string][]string // agentID → KB IDs
}

// Registry is an instance-based capability registry. It replaces the former
// global sync.Map approach, allowing multiple independent registries and
// eliminating init()-based side effects.
type Registry struct {
	builtins map[string]Capability
	mu       sync.RWMutex
}

// NewRegistry creates a new empty capability registry.
func NewRegistry() *Registry {
	return &Registry{builtins: make(map[string]Capability)}
}

// Register adds a capability to the registry. Returns an error if a
// capability with the same name is already registered.
func (r *Registry) Register(c Capability) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.builtins[c.Name()]; exists {
		return fmt.Errorf("capability %q already registered", c.Name())
	}
	r.builtins[c.Name()] = c
	return nil
}

// Get retrieves a capability by name from the registry.
func (r *Registry) Get(name string) (Capability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.builtins[name]
	return c, ok
}

// ListByType returns all registered capabilities of a given type.
// The returned slice is sorted by name for deterministic ordering.
func (r *Registry) ListByType(t CapabilityType) []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []Capability
	for _, c := range r.builtins {
		if c.Type() == t {
			result = append(result, c)
		}
	}
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
func (r *Registry) ExpandWildcards(names []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, name := range names {
		switch {
		case name == WildcardAll:
			r.mu.RLock()
			for n := range r.builtins {
				if !seen[n] {
					seen[n] = true
					result = append(result, n)
				}
			}
			r.mu.RUnlock()
		case name == WildcardTools:
			for _, c := range r.ListByType(CapabilityTypeTool) {
				if !seen[c.Name()] {
					seen[c.Name()] = true
					result = append(result, c.Name())
				}
			}
		case name == WildcardHooks:
			for _, c := range r.ListByType(CapabilityTypeHook) {
				if !seen[c.Name()] {
					seen[c.Name()] = true
					result = append(result, c.Name())
				}
			}
		case name == WildcardSkills:
			for _, c := range r.ListByType(CapabilityTypeSkill) {
				if !seen[c.Name()] {
					seen[c.Name()] = true
					result = append(result, c.Name())
				}
			}
		case name == WildcardMemory:
			for _, c := range r.ListByType(CapabilityTypeMemory) {
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
func (r *Registry) ResolveDependencies(names []string) ([]Capability, error) {
	expanded := r.ExpandWildcards(names)

	// Collect all required capabilities (including transitive deps).
	required := make(map[string]Capability)
	if err := r.collectTransitive(expanded, required); err != nil {
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
func (r *Registry) collectTransitive(names []string, required map[string]Capability) error {
	for _, name := range names {
		if _, already := required[name]; already {
			continue
		}

		cap, ok := r.Get(name)
		if !ok {
			return fmt.Errorf("capability %q not registered", name)
		}
		required[name] = cap

		if err := r.collectTransitive(cap.DependsOn(), required); err != nil {
			return err
		}
	}
	return nil
}
