package knowledgebase

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
)

type memoryPersistHookCapability struct{}

func (c *memoryPersistHookCapability) Name() string                      { return capabilities.HookMemoryPersist }
func (c *memoryPersistHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *memoryPersistHookCapability) DependsOn() []string               { return nil }

func (c *memoryPersistHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return nil, nil
}
