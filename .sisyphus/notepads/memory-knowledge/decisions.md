# Decisions — memory-knowledge plan

## 2026-05-29 Session Start

### Architecture
- Two independent capability bundles: `memory` (MD files) and `knowledge_base` (RAG)
- sqlite-vec as sole KnowledgeStore backend this phase (pure Go, no CGO)
- OpenAI Embedder reuses existing LLMProvider (no new SDK dependency)
- StoreProvider.Knowledge() returns nil when not configured (backward compatible)
- MemoryBundleNames: [hooks.file_memory, tools.memory_store, tools.memory_recall, tools.memory_forget]
- KnowledgeBaseBundleNames: [hooks.kb_recall, hooks.memory_persist]

## 2026-05-30 F1 Compliance Audit

### VERDICT: APPROVE
- Must Have: 10/10 | Must NOT Have: 11/11 | Tasks: 43/43 | Deliverables: 12/12
- All builds pass (core, server, chat-core, demo)
- All tests pass (core eval, core unit, server API, chat-core 54/54)

### Architectural Deviation (Acceptable)
- Plan specified `StoreProvider.Memory()` method; implementation uses `StoreConfig.Memory` field instead.
- Rationale: MemoryStore is optional/configured separately, not a core StoreProvider concern. Same pattern as FileMemory/KnowledgeStore/Embedder in StoreConfig.
- Plan intent (MemoryStore accessible) is fulfilled; this is a structural choice, not a functional gap.
