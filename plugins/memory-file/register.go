package memoryfile

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/llm"
)

func RegisterCapabilities(r *capabilities.Registry, store *FileMemoryStore, llmProvider llm.LLMProvider, summaryLLM llm.LLMProvider) {
	r.Register(&MemoryModule{store: store, llm: llmProvider, summaryLLM: summaryLLM})
}