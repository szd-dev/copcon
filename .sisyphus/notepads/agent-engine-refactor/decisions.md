# Decisions - Agent Engine Refactor

## 2026-04-06 Session Start

### Architecture Decisions

1. **Todo Injection Location**: Keep in `AgentEngine.buildLLMContext()`, NOT in `ContextManager.BuildContext()`
   - Reason: Avoids circular dependency (ContextManager → TodoManager)
   - Alternative rejected: Moving to ContextManager would create import cycle

2. **Tool List Retrieval**: Move outside loop (Line 134 → before Line 104)
   - Reason: Tools are static per agent, no need to re-fetch each iteration
   - Performance: Saves redundant `GetOpenAITools()` calls

3. **Message Persistence**: Unify into `persistMessage()` helper
   - Lines 258-266 and 277-284 are nearly identical
   - Single helper handles both cases (with/without tool calls)

4. **No LLMProvider Abstraction**: Keep OpenAI SDK direct usage
   - Reason: Out of scope, adds complexity
   - Future consideration if multi-provider support needed

5. **No State Machine**: Keep simple `for` loop structure
   - Reason: Current loop is clear, state machine over-engineering

### Test Strategy

1. **Mock OpenAI Stream**: Create `MockOpenAIStream` for unit testing
   - Simulates streaming behavior without real API
   - Allows testing delta accumulation, tool call merging

2. **TDD Approach**: Write tests BEFORE refactoring
   - Ensures behavior preservation
   - Documents expected behavior

### Refactor Phases

1. **Phase 1 - Initialization**: `prepareAgentLoop()` helper
2. **Phase 2 - Loop**: Context building, streaming, tool calls
3. **Phase 3 - Tool Execution**: `handleToolCalls()` helper
