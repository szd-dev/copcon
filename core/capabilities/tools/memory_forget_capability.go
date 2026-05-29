package tools

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/tool"
)

type memoryForgetType struct{}

func (c *memoryForgetType) Name() string                      { return "tools.memory_forget" }
func (c *memoryForgetType) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *memoryForgetType) DependsOn() []string               { return []string{"hooks.file_memory"} }

func (c *memoryForgetType) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	if deps.FileMemoryStore == nil {
		return nil, nil
	}

	store, ok := deps.FileMemoryStore.(MemoryStoreAPI)
	if !ok {
		return nil, nil
	}

	return NewMemoryForgetTool(store), nil
}

func init() {
	capabilities.Register(&memoryForgetType{})
}
