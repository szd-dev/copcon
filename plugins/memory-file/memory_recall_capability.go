package memoryfile

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/tool"
)

type memoryRecallCapability struct{}

func (c *memoryRecallCapability) Name() string                      { return capabilities.ToolMemoryRecall }
func (c *memoryRecallCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *memoryRecallCapability) DependsOn() []string               { return []string{capabilities.HookFileMemory} }

func (c *memoryRecallCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	if deps.FileMemoryStore == nil {
		return nil, nil
	}

	store, ok := deps.FileMemoryStore.(MemoryStoreAPI)
	if !ok {
		return nil, nil
	}

	return NewMemoryRecallTool(store), nil
}
