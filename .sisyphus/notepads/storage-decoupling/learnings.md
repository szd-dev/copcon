# Task 12 Learnings

## TodoStore Decoupling Pattern

- `storage.TodoStore` uses `context.Context` + `uuid.UUID` calling convention
- `todo.TodoManager` uses `iface.ChatContextInterface` + `string` calling convention
- When switching consumers from `TodoManager` to `TodoStore`, need to:
  1. Parse `chatCtx.SessionID()` to `uuid.UUID` for `TodoStore.List`
  2. Use `chatCtx.Context()` instead of `chatCtx` directly
  3. Convert return types from `[]*storage.Todo` to whatever the consumer needs

## NewTodoManager Return Signature

- Changed from `NewTodoManager(db) TodoManager` to `NewTodoManager(db) (TodoManager, storage.TodoStore)`
- Returns both interfaces from the same concrete `*todoManager`
- Callers that only need `TodoManager` use `todoMgr, _ := todo.NewTodoManager(db)`
- Callers that need `TodoStore` use the second return value

## TodoTool Type Assertion Pattern

- `TodoTool` stores `storage.TodoStore` as its declared dependency (for CapabilityDeps prep)
- Internally uses `todoMgr()` method that type-asserts to `todo.TodoManager`
- This is transitional — `TodoStore` doesn't have all business-logic methods (Start, Complete, Fail, Delete)
- `TodoStore.UpdateStatus` only updates status field, can't set result/retry_count/completed_at
- In Phase 3, CapabilityDeps will provide a better interface design

## Test Stub Updates

- When changing from `todo.TodoManager` to `storage.TodoStore` in hook, test stubs must implement `TodoStore` interface
- `storage.TodoStore.List` takes `uuid.UUID` for sessionID, so test sessionIDs must be valid UUIDs
- Hook gracefully handles invalid UUIDs (logs warning, returns nil)
