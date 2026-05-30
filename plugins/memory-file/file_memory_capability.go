package memoryfile

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
)

type fileMemoryHookCapability struct{}

func (c *fileMemoryHookCapability) Name() string                      { return capabilities.HookFileMemory }
func (c *fileMemoryHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *fileMemoryHookCapability) DependsOn() []string               { return nil }

func (c *fileMemoryHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return nil, nil
}
