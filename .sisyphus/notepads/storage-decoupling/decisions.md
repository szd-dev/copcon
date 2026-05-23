# Task 12 Decisions

## Decision: TodoTool uses type assertion for business logic methods

**Context:** `TodoStore` interface only has CRUD methods (Create, Get, List, UpdateStatus, DeleteBySession). TodoTool needs business logic operations (CreateTodo with dedup/auto-start, Start with state machine, Complete with result, Fail with retry count, Delete by ID).

**Options:**
1. Reimplement all business logic in TodoTool using TodoStore methods
2. Type-assert `storage.TodoStore` to `todo.TodoManager` internally
3. Keep `todo.TodoManager` as the stored type

**Chosen:** Option 2 — type assertion via `todoMgr()` helper method.

**Rationale:**
- Preserves business logic (satisfies "Do NOT change Todo business logic")
- Establishes `storage.TodoStore` as the declared dependency (satisfies "TodoTool uses TodoStore")
- Transitional pattern for Phase 3 CapabilityDeps
- Avoids duplicating state machine/validation logic in the tool layer

**Trade-off:** Runtime type assertion could fail if a different `TodoStore` implementation is used. This is acceptable because in production, `todoManager` always implements both interfaces, and Phase 3 will replace this pattern.

## Decision: NewTodoManager returns both interfaces

**Context:** `main.go` needs both `TodoManager` (for API layer) and `TodoStore` (for hook/tool).

**Chosen:** Return `(TodoManager, storage.TodoStore)` from `NewTodoManager`.

**Rationale:** Avoids runtime type assertions in `main.go`. The same concrete object satisfies both interfaces. Clean compile-time guarantee.
