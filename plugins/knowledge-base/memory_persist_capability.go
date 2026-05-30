package knowledgebase

import (
	"fmt"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/plugins/embedding-openai"
	"github.com/copcon/plugins/memory-file"
)

type memoryPersistHookCapability struct{}

func (c *memoryPersistHookCapability) Name() string                      { return capabilities.HookMemoryPersist }
func (c *memoryPersistHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *memoryPersistHookCapability) DependsOn() []string               { return nil }

func (c *memoryPersistHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	if deps.Embedder == nil {
		return nil, fmt.Errorf("%w: Embedder not configured", capabilities.ErrDependencyUnavailable)
	}
	if deps.MemoryStore == nil {
		return nil, fmt.Errorf("%w: MemoryStore not configured", capabilities.ErrDependencyUnavailable)
	}

	emb, ok := deps.Embedder.(embedding.Embedder)
	if !ok {
		return nil, fmt.Errorf("%w: Embedder type assertion failed", capabilities.ErrDependencyUnavailable)
	}

	ms, ok := deps.MemoryStore.(memoryfile.MemoryStore)
	if !ok {
		return nil, fmt.Errorf("%w: MemoryStore type assertion failed", capabilities.ErrDependencyUnavailable)
	}

	return NewMemoryPersistHook(emb, ms), nil
}
