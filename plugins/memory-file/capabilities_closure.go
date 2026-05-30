package memoryfile

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/tool"
)

// MemoryModule implements capabilities.ModuleCapability, producing
// one hook (file_memory) and three tools (memory_store, memory_recall,
// memory_forget) from a single capability registration.
type MemoryModule struct {
	store *FileMemoryStore
}

func (m *MemoryModule) Name() string                      { return capabilities.CapMemoryFile }
func (m *MemoryModule) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeModule }
func (m *MemoryModule) DependsOn() []string               { return nil }

func (m *MemoryModule) NewHooks(deps capabilities.CapabilityDeps) ([]hook.Hook, error) {
	return []hook.Hook{NewFileMemoryHook(m.store)}, nil
}

func (m *MemoryModule) NewTools(deps capabilities.CapabilityDeps) ([]tool.Tool, error) {
	return []tool.Tool{
		NewMemoryStoreTool(m.store),
		NewMemoryRecallTool(m.store),
		NewMemoryForgetTool(m.store),
	}, nil
}

var _ capabilities.ModuleCapability = (*MemoryModule)(nil)