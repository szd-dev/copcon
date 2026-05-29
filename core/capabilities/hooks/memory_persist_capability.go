package hooks

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/providers/embedding"
)

type memoryPersistHookCapability struct{}

func (c *memoryPersistHookCapability) Name() string                      { return "hooks.memory_persist" }
func (c *memoryPersistHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *memoryPersistHookCapability) DependsOn() []string               { return nil }

func (c *memoryPersistHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	if deps.Embedder == nil || deps.MemoryStore == nil {
		return nil, nil
	}

	emb, ok := deps.Embedder.(embedding.Embedder)
	if !ok {
		return nil, nil
	}

	return NewMemoryPersistHook(emb, deps.MemoryStore), nil
}

func init() {
	capabilities.Register(&memoryPersistHookCapability{})
}
