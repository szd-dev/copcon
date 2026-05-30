package memoryfile

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/tool"
)

type fileMemoryHookCapabilityClosure struct {
	store *FileMemoryStore
}

func (c *fileMemoryHookCapabilityClosure) Name() string                      { return capabilities.HookFileMemory }
func (c *fileMemoryHookCapabilityClosure) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *fileMemoryHookCapabilityClosure) DependsOn() []string               { return nil }

func (c *fileMemoryHookCapabilityClosure) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return NewFileMemoryHook(c.store), nil
}

type memoryHookCapabilityClosure struct {
	store *FileMemoryStore
}

func (c *memoryHookCapabilityClosure) Name() string                      { return capabilities.HookMemory }
func (c *memoryHookCapabilityClosure) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *memoryHookCapabilityClosure) DependsOn() []string               { return nil }

func (c *memoryHookCapabilityClosure) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return NewMemoryPlugin(newMemoryManagerFromStore(c.store)), nil
}

func newMemoryManagerFromStore(store *FileMemoryStore) MemoryManager {
	if store == nil {
		return nil
	}
	return &memoryManagerAdapter{store: store}
}

type memoryStoreCapabilityClosure struct {
	store *FileMemoryStore
}

func (c *memoryStoreCapabilityClosure) Name() string                      { return capabilities.ToolMemoryStore }
func (c *memoryStoreCapabilityClosure) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *memoryStoreCapabilityClosure) DependsOn() []string               { return []string{capabilities.HookFileMemory} }

func (c *memoryStoreCapabilityClosure) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	return NewMemoryStoreTool(c.store), nil
}

type memoryRecallCapabilityClosure struct {
	store *FileMemoryStore
}

func (c *memoryRecallCapabilityClosure) Name() string                      { return capabilities.ToolMemoryRecall }
func (c *memoryRecallCapabilityClosure) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *memoryRecallCapabilityClosure) DependsOn() []string               { return []string{capabilities.HookFileMemory} }

func (c *memoryRecallCapabilityClosure) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	return NewMemoryRecallTool(c.store), nil
}

type memoryForgetCapabilityClosure struct {
	store *FileMemoryStore
}

func (c *memoryForgetCapabilityClosure) Name() string                      { return capabilities.ToolMemoryForget }
func (c *memoryForgetCapabilityClosure) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *memoryForgetCapabilityClosure) DependsOn() []string               { return []string{capabilities.HookFileMemory} }

func (c *memoryForgetCapabilityClosure) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	return NewMemoryForgetTool(c.store), nil
}