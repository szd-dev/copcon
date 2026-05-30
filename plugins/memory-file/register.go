package memoryfile

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

func RegisterCapabilities(r *capabilities.Registry, store *FileMemoryStore, emb kbtypes.Embedder) {
	r.Register(&MemoryModule{store: store})
	r.Register(&memoryPersistHookCapabilityClosure{emb: emb, store: store})
}

type memoryPersistHookCapabilityClosure struct {
	emb   kbtypes.Embedder
	store MemoryStore
}

func (c *memoryPersistHookCapabilityClosure) Name() string                      { return capabilities.HookMemoryPersist }
func (c *memoryPersistHookCapabilityClosure) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *memoryPersistHookCapabilityClosure) DependsOn() []string               { return nil }
func (c *memoryPersistHookCapabilityClosure) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return NewMemoryPersistHook(c.emb, c.store), nil
}