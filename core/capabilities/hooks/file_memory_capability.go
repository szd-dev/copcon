package hooks

import (
	"fmt"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
)

type fileMemoryHookCapability struct{}

func (c *fileMemoryHookCapability) Name() string                      { return "hooks.file_memory" }
func (c *fileMemoryHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *fileMemoryHookCapability) DependsOn() []string               { return nil }

func (c *fileMemoryHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	if deps.FileMemoryStore == nil {
		return nil, fmt.Errorf("%w: FileMemoryStore not configured", capabilities.ErrDependencyUnavailable)
	}

	type basePather interface{ BasePath() string }
	store, ok := deps.FileMemoryStore.(basePather)
	if !ok {
		return nil, fmt.Errorf("%w: FileMemoryStore does not implement BasePath()", capabilities.ErrDependencyUnavailable)
	}

	return NewFileMemoryHook(store), nil
}

func init() {
	capabilities.Register(&fileMemoryHookCapability{})
}
