
## Task 18: Migrate agent/ + storage/ to core/

### Key Decision: iface.SessionManager and iface.ContextManager

Added `SessionManager` and `ContextManager` interfaces to `core/iface/chat.go` using `storage.*` value types. This was necessary because core/agent/ cannot import server/ packages (circular dependency). The original `session.SessionManager` and `chat_context.ContextManager` in server/ use GORM model types (`*session.Session`, `*session.Message`), while the core interfaces use pure value types (`*storage.Session`, `*storage.Message`).

### Key Decision: Adapter Pattern in server/

Created `server/internal/adapter/agent_adapter.go` with `SessionManagerAdapter` and `ContextManagerAdapter` that wrap the server's implementations and convert between GORM models and storage value types using the existing `session.SessionToStorage()`, `session.MessageFromStorage()` conversion functions.

### Key Decision: storage types replace session types in agent/

Changed agent/ to use `storage.Part` instead of `session.PersistedPart`, `storage.ToolCall` instead of `session.ToolCall`, `storage.Message` instead of `session.Message`, etc. The agent package no longer depends on GORM model types.

### Key Decision: TestTodoLoopFix removed from core/

The `TestTodoLoopFix` test in engine_test.go depends on server-specific packages (gorm, todo.NewTodoManager, chat_context.NewContextManager). This test was removed from the core/ copy with a comment explaining why.
