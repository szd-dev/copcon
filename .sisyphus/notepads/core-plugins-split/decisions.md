# Decisions: core-plugins-split

## From Plan
- go.mod strategy: Separate modules (plugins/ gets its own go.mod, all share go.work)
- Capability Registry: Instance-based (no global state)
- OpenAI adapter: Stays in core/llm/
- Embedding: Entire module moves to plugins/embedding-openai/
- Todo: Stays in core (built-in tool)
- Migration: All at once (one atomic commit)

## Package Names After Move
- Keep existing package names where possible (postgres, sqlite, etc.)
- Memory/Knowledge hooks+tools files from different packages → need sub-packages or unification
- Plan says: "keep package names separate for now"

## Step 4: Instance-based Registry (Completed)
- Replaced global sync.Map with instance-based Registry struct using map[string]Capability + sync.RWMutex
- Register() now returns error on duplicates (was void before)
- All init() functions removed from core/hooks, core/tools, plugins/memory-file, plugins/knowledge-base
- Created hooks.RegisterAll(r), tools.RegisterAll(r) for explicit registration
- Created memoryfile.RegisterCapabilities(r), knowledgebase.RegisterCapabilities(r) for plugin registration
- HarnessConfig.Registry field added (auto-created if nil in Build())
- HookMemory removed from builtInHooks (it's a plugin capability now), added to MemoryBundleNames()
- Integration tests updated: core can't import plugins, so test uses Registry with stub plugin capabilities
- registerCrossAgentTool updated to take Registry instance as first parameter
- CapRegistry() accessor added to Harness for callers to access the capability registry
