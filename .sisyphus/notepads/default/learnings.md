
## Task 9: AsyncToolRegistry → AsyncToolTracker interface

- Actual method signatures in AsyncToolRegistry differ from the plan spec (e.g., `Register` takes `cancelFunc context.CancelFunc` not `args map[string]any`; `Fail` takes `errMsg string` not `err error`; `GetStatus` returns `(*AsyncToolState, error)` not `(AsyncToolStatus, bool)`; `Cancel` returns `bool` not `error`; `CancelSession` returns `int` not `error`; `ListBySession` returns `[]*AsyncToolState` not `[]AsyncToolInfo`). Always read the actual code first.
- When abstracting a concrete type to an interface, test files using `*ConcreteType` params don't need changes since `*ConcreteType` satisfies the interface — Go handles the implicit conversion.
- `main.go` needed no changes because `*AsyncToolRegistry` already satisfies `AsyncToolTracker` — the constructor calls just work.
- The `async_tools.go` tool implementations still use `*tool.AsyncToolRegistry` directly in their struct fields and constructors — these were NOT in scope for this task but could be updated in a follow-up.
