# Task 29 Learnings

## Capability name vs tool name mapping
- Capabilities are registered by name (e.g., `tools.todo`, `tools.code_executor`)
- The actual tool's `Name()` method returns a different name (e.g., `todolist`, `code_executor`)
- The ToolRegistry keys by `tool.Name()`, not the capability name
- Solution: Build a `capToToolName` map during Build() and pass it to agent factories

## Delegate tool circular dependency
- `tools.delegate` capability requires `Engine` in its `CapabilityDeps`
- Engine isn't created until after all tools are registered
- Solution: Skip `tools.delegate` and `tools.read_sub_session` during initial capability resolution (step 3-4), register them in step 9 after engine creation

## Capability init() registration
- Capabilities self-register via `init()` functions in their packages
- Tests must blank-import the packages to trigger registration: `_ "github.com/copcon/core/capabilities/tools"` and `_ "github.com/copcon/core/capabilities/hooks"`

## No-op store implementations
- No existing no-op store implementations existed in the codebase
- Created: noopSessionStore, noopMessageStore, noopTodoStore, noopMemoryStore
- Also needed: noopSessionManager, noopContextManager (for iface adapters)
- Storage interfaces use `uuid.UUID` not `interface{}` — must use correct types

## BuildContext return type
- `iface.ContextManager.BuildContext` returns `[]entity.MessageForLLM`, not `[]interface{}`
