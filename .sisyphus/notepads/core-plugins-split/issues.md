# Issues: core-plugins-split

## Potential Issues Identified
1. eval/testdata/ is at repo root (/data/copcon/eval/testdata/), NOT in core/eval/
2. core/providers/doc.go exists at the providers root level - needs to be handled when cleaning up
3. Some hooks/ files that stay in core (todo_injection, memory, logging, tracing) are in the same package as files that move (file_memory, kb_recall, memory_persist)
4. The `init()` pattern in hooks/ and tools/ packages auto-registers capabilities - after moving files, the moved init() functions won't fire unless imported
5. Package naming challenge: memory-file needs files from `package hooks`, `package tools`, `package filememory` - three different packages in one plugin directory

## Resolved Issues
- core/providers/doc.go: deleted via `git rm` (only had 2-line package declaration)
- Package naming: resolved by changing all memory-file files to `package memoryfile`, knowledge-base root to `package knowledgebase`
- eval/testdata: does NOT exist in core/eval/ (confirmed)
- rag/testdata: successfully moved with `git mv core/rag/testdata plugins/rag/testdata`

## Remaining Issues for Later Steps
- Import paths in moved files still reference old core/ paths (will be fixed in Step 3)
- init() functions in moved capability files won't auto-register without blank imports
- Cross-references between moved packages and core/ need updating
- plugins/go.mod needs proper dependency list (currently only has core replace directive)

## Step 4: Resolved Issues
- HookMemory in builtInHooks: was always a plugin capability after steps 1-3 moved memory.go to plugins. Removed from builtInHooks, added to MemoryBundleNames().
- Integration tests: core module can't import plugins (circular dependency). Updated to use Registry with stub plugin capabilities. Real FileMemoryHookInjection test now uses stubs (full functionality test should live in server/ module).
- Register() now returns error: hooks.RegisterAll and tools.RegisterAll ignore errors (duplicates won't happen in normal usage). Could be improved with MustRegister() pattern later.

## Remaining
- TestHarness_FileMemoryHookInjection uses stubs instead of real file_memory hook. Real integration test should be in server/ which can import both core and plugins.
