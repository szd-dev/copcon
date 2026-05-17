# Issues - Agent Engine Refactor

## 2026-04-06 Session Start

### Known Issues (From Plan)

1. **Test Coverage Gap**: Existing tests don't cover `runAgentLoop` internal logic
   - Impact: HIGH - Risk of breaking behavior during refactor
   - Solution: Create Mock OpenAI Stream, add unit tests first

2. **Tool List Redundancy**: `GetOpenAITools()` called every loop iteration
   - Impact: MEDIUM - Performance issue
   - Solution: Move outside loop

3. **Message Persistence Duplication**: Lines 258-266 vs 277-284
   - Impact: LOW - Code duplication
   - Solution: Unified `persistMessage()` helper

### Resolved Issues

(None yet)

### Open Questions

(None yet)
