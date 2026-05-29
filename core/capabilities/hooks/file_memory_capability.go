package hooks

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
)

type fileMemoryHookCapability struct{}

func (c *fileMemoryHookCapability) Name() string                      { return "hooks.file_memory" }
func (c *fileMemoryHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *fileMemoryHookCapability) DependsOn() []string               { return nil }

func (c *fileMemoryHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	if deps.FileMemoryStore == nil {
		return nil, nil
	}

	type basePather interface{ BasePath() string }
	store, ok := deps.FileMemoryStore.(basePather)
	if !ok {
		return nil, nil
	}

	return NewFileMemoryHook(store), nil
}

func init() {
	capabilities.Register(&fileMemoryHookCapability{})
}
