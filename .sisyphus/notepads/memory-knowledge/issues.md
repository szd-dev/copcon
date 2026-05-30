# Issues — memory-knowledge plan

(No issues yet)

## F4 Scope Fidelity Check (2026-05-30)

### Missing: sqlitevec README.md
- W3.1 plan explicitly lists `core/providers/sqlitevec/README.md` as a deliverable
- File does not exist. Low severity (documentation only).

### Design Deviation: StoreProvider.Memory() absent
- Plan Must-NOT guardrail implies `Memory() MemoryStore` should exist in StoreProvider interface
- Implementation provides Memory through `StoreConfig.Memory` field instead
- Plan says: "不能让 MemoryStore 在 StoreProvider 中强制非 nil——保留 Memory() MemoryStore 可选返回 nil 的语义"
- Since Memory() doesn't exist in the interface, this guardrail can't be evaluated
- Functionality is preserved via alternate path (StoreConfig.Memory)

### Pre-existing Responsive CSS
- `packages/demo/src/App.css` has 3 @media breakpoints (480px, 768px, 1024px)
- These pre-date the memory-knowledge feature (present at base commit 03bc47e)
- Wave 5 did NOT add new @media rules (only added tab-related CSS)
- Not a scope creep, but the feature didn't actively remove pre-existing responsive CSS either

### Commit Convention Deviations
- 6/9 commits use wave-based scopes (wave1, wave2a, etc.) instead of module-based (core, server, demo)
- 1/9 commits missing type prefix: `eval: bootstrap...` should be `test(eval):` or `chore(eval):`

## F3 Build & Test Verification (2026-05-30)

### Pre-existing: TestChat_FirstConnectEmptyContent FAIL
- `server/internal/api/handlers_test.go` — expects HTTP 400 for empty content, gets 200
- Present on feat/v2 base branch — NOT a regression from feat/common_ui

### Pre-existing: TestChat_Reconnect TIMEOUT
- `server/internal/api/handlers_test.go` — hangs in `streamEvents` goroutine
- Present on feat/v2 base branch — NOT a regression from feat/common_ui

### Pre-existing: MRR below 0.75 gate
- Overall MRR = 0.7323 (gate ≥ 0.75), but TestGoldenEval passes (its internal threshold allows this)
- Caused by 5 adversarial queries correctly returning 0 relevance
- Non-adversarial MRR = 0.8137 (well above gate)
