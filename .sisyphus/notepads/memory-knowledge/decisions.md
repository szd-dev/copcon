# Decisions — memory-knowledge plan

## 2026-05-29 Session Start

### Architecture
- Two independent capability bundles: `memory` (MD files) and `knowledge_base` (RAG)
- sqlite-vec as sole KnowledgeStore backend this phase (pure Go, no CGO)
- OpenAI Embedder reuses existing LLMProvider (no new SDK dependency)
- StoreProvider.Knowledge() returns nil when not configured (backward compatible)
- MemoryBundleNames: [hooks.file_memory, tools.memory_store, tools.memory_recall, tools.memory_forget]
- KnowledgeBaseBundleNames: [hooks.kb_recall, hooks.memory_persist]
