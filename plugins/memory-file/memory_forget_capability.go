package memoryfile

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/tool"
)

type memoryForgetType struct{}

func (c *memoryForgetType) Name() string                      { return capabilities.ToolMemoryForget }
func (c *memoryForgetType) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *memoryForgetType) DependsOn() []string               { return []string{capabilities.HookFileMemory} }

func (c *memoryForgetType) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	return nil, nil
}
