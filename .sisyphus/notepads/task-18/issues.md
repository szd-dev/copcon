
## Task 18: Migrate agent/ + storage/ to core/

### Adapter conversion overhead
Every call through the adapter converts storage.Message → session.Message (via `session.MessageFromStorage`) and session.Session → storage.Session (via `session.SessionToStorage`). This creates allocation overhead. Future optimization: have the server implementations directly satisfy the core interfaces.

### TestTodoLoopFix cannot live in core/
This test requires gorm.DB, todo.NewTodoManager, and chat_context.NewContextManager - all server-side dependencies. It should be maintained in server/internal/agent/ if needed, or moved to an integration test package.

### engine_test.go mock types are verbose
The mockSessionManager and mockContextManager in engine_test.go have extra methods beyond what iface.SessionManager/ContextManager require (CreateSession, ListSessions, GetHistory, DeleteBySession, etc.). These are leftover from the original session.SessionManager interface and could be cleaned up.
