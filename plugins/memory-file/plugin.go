package memoryfile

import (
	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/tool"
)

// toolNameWrapper wraps a tool.Tool and overrides its Name().
type toolNameWrapper struct {
	tool.Tool
	newName string
}

func (w *toolNameWrapper) Name() string { return w.newName }

// hookNameWrapper wraps a hook.Hook and overrides its Name().
type hookNameWrapper struct {
	hook.Hook
	newName string
}

func (w *hookNameWrapper) Name() string { return w.newName }

// memoryPlugin implements plugin.Plugin for the memory-file plugin.
type memoryPlugin struct {
	store      *FileMemoryStore
	llm        llm.LLMProvider
	summaryLLM llm.LLMProvider

	// Internal references for Init() injection.
	factHook *FactExtractionHook
}

// NewPlugin creates a new memory-file Plugin.
// summaryLLM is optional; when nil, MemorySummaryHook is not created.
func NewPlugin(fmStore *FileMemoryStore, llmProvider llm.LLMProvider, summaryLLM llm.LLMProvider) plugin.Plugin {
	return &memoryPlugin{
		store:      fmStore,
		llm:        llmProvider,
		summaryLLM: summaryLLM,
	}
}

func (p *memoryPlugin) Name() string { return "memory" }

func (p *memoryPlugin) Tools() []tool.Tool {
	return []tool.Tool{
		&toolNameWrapper{Tool: NewMemoryStoreTool(p.store), newName: "memory.tool.memory_store"},
		&toolNameWrapper{Tool: NewMemoryRecallTool(p.store, p.llm), newName: "memory.tool.memory_recall"},
		&toolNameWrapper{Tool: NewMemoryForgetTool(p.store), newName: "memory.tool.memory_forget"},
	}
}

func (p *memoryPlugin) Hooks() []hook.Hook {
	hooks := []hook.Hook{
		&hookNameWrapper{Hook: NewFileMemoryHook(p.store), newName: "memory.hook.file_memory"},
		&hookNameWrapper{Hook: NewMemoryRecallHook(p.store, p.llm), newName: "memory.hook.memory_recall"},
	}

	if p.factHook != nil {
		hooks = append(hooks, &hookNameWrapper{Hook: p.factHook, newName: "memory.hook.fact_extraction"})
	}

	if p.summaryLLM != nil {
		summarizer := NewFileSummarizer(p.store, p.summaryLLM, DefaultSummarizerConfig())
		hooks = append(hooks, &hookNameWrapper{Hook: NewMemorySummaryHook(summarizer), newName: "memory.hook.memory_summary"})
	}

	return hooks
}

func (p *memoryPlugin) Init(deps plugin.PluginDeps) error {
	p.factHook = NewFactExtractionHook(p.store, p.llm, deps.MessageStore, "")
	return nil
}

// GetStore exposes the underlying FileMemoryStore for API layer usage.
func (p *memoryPlugin) GetStore() *FileMemoryStore {
	return p.store
}

var _ plugin.Plugin = (*memoryPlugin)(nil)
