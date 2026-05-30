package tools

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/tool"
)

type memoryStoreCapability struct{}

func (c *memoryStoreCapability) Name() string                      { return capabilities.ToolMemoryStore }
func (c *memoryStoreCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *memoryStoreCapability) DependsOn() []string               { return []string{capabilities.HookFileMemory} }

func (c *memoryStoreCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	if deps.FileMemoryStore == nil {
		return nil, nil
	}

	store, ok := deps.FileMemoryStore.(MemoryStoreAPI)
	if !ok {
		return nil, nil
	}

	return NewMemoryStoreTool(store), nil
}

func init() {
	capabilities.Register(&memoryStoreCapability{})
}
