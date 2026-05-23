# Task 26 & 27: init() Self-Registration Evidence

## Verification Results

### go vet
```
cd core && go vet ./capabilities/...
# (no output — passes)
```

### LSP diagnostics
- tools/ directory: 0 errors across 9 files
- hooks/ directory: 0 errors across 6 files

### Registry verification (temporary test, removed after verification)
All 7 tool capabilities registered:
- tools.async
- tools.code_executor
- tools.delegate
- tools.file_ops
- tools.hitl
- tools.shell_executor
- tools.todo

All 4 hook capabilities registered:
- hooks.logging
- hooks.memory
- hooks.todo_injection
- hooks.tracing

### Specific checks
- `registry.Get("tools.code_executor")` → valid ToolCapability ✓
- `registry.Get("hooks.logging")` → valid HookCapability ✓
- `tools.todo` DependsOn: `["hooks.todo_injection"]` ✓
- `tools.delegate` NewTool returns error when Engine is nil ✓

## Files Modified

### Tools (7 capabilities in 6 files)
1. `core/capabilities/tools/code_executor.go` — +codeExecutorCapability, +shellExecutorCapability
2. `core/capabilities/tools/file_ops.go` — +fileOpsCapability
3. `core/capabilities/tools/todo.go` — +todoCapability, +todoManagerAdapter (bridges TodoStore→TodoManager)
4. `core/capabilities/tools/async.go` — +asyncCapability
5. `core/capabilities/tools/hitl.go` — +hitlCapability
6. `core/capabilities/tools/delegate.go` — +delegateCapability

### Hooks (4 capabilities in 4 files)
1. `core/capabilities/hooks/todo_injection.go` — +todoInjectionHookCapability
2. `core/capabilities/hooks/logging.go` — +loggingHookCapability
3. `core/capabilities/hooks/memory.go` — +memoryHookCapability, +memoryManagerAdapter (bridges MemoryStore→MemoryManager)
4. `core/capabilities/hooks/tracing.go` — +tracingHookCapability

## Design Decisions

1. **todoManagerAdapter**: TodoStore lacks DeleteByID, Complete(result), Fail(reason). Adapter uses UpdateStatus for Start/Complete/Fail (result/reason not persisted). Delete returns error.

2. **memoryManagerAdapter**: MemoryStore.Store/Search match MemoryManager interface directly. Adapter delegates with context extraction from ChatContextInterface.

3. **delegateCapability.NewTool**: Casts deps.Engine to agent.AgentEngine. Returns error if nil/wrong type. SessionManager/ContextManager passed as nil (not in CapabilityDeps).

4. **asyncCapability.NewTool**: Creates a new AsyncToolRegistry for each tool instance. This is a placeholder — the real registry should be shared across async tools.

5. **tracingCapability.NewHook**: Passes nil tracer since no Tracer is in CapabilityDeps. The TracingPlugin already handles nil tracer gracefully.
