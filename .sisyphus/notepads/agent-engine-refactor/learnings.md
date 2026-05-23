# Learnings - Agent Engine Refactor

## 2026-04-06 Session Start

### Current Code Structure (engine.go)

**runAgentLoop() - Lines 71-293 (222 lines)**

Key sections identified:
1. **Lines 72-102**: Session retrieval, agent resolution (3-layer fallback), user message persistence
2. **Line 104**: Loop start `for {`
3. **Lines 107-118**: Todo state injection inside loop
4. **Lines 120-138**: Context building, message conversion, logging
5. **Line 134**: Tool list retrieval (INSIDE loop - should be outside)
6. **Lines 140-145**: LLM request params construction
7. **Lines 147-237**: Streaming loop - OpenAI stream handling
   - Lines 164-170: Content delta handling
   - Lines 172-181: Reasoning delta handling
   - Lines 183-206: Tool call delta accumulation
   - Lines 208-223: JustFinishedToolCall fallback
   - Lines 226-230: Tool call map to slice conversion
   - Lines 232-237: Stream error handling
8. **Lines 239-256**: Response logging
9. **Lines 258-274**: Tool call handling path (with tool calls)
10. **Lines 277-291**: Final message path (no tool calls)

### Existing Helper Methods

- `executeToolCall()` - Lines 295-338
- `convertMessages()` - Lines 340-357
- `convertToolCalls()` - Lines 359-372
- `parseArgs()` - Lines 374-380
- `formatTodoState()` - Lines 382-422

### Data Structures

- `deltaExtraFields` - Lines 26-28
- `toolCallInfo` - Lines 30-35

### Dependencies

- OpenAI SDK: `github.com/openai/openai-go/v3`
- Session: `github.com/copcon/server/internal/session`
- Context: `github.com/copcon/server/internal/chat_context`
- Todo: `github.com/copcon/server/internal/todo`
- Tool: `github.com/copcon/server/internal/tool`

### Guardrails (Must NOT Change)

- `Chat()` method signature (Line 61)
- Event types in `entity.Event*`
- Event emission order
- Message persistence timing
- No ContextManager → TodoManager dependency

## 2026-04-06 Task 2: Streaming Accumulation Tests

### MockOpenAIStream Helpers (from mock_openai_test.go)

The mock provides these key methods:
- `AddContentChunk(content string)` - Adds chunk with content delta
- `AddReasoningChunk(content string)` - Adds chunk with reasoning_content in RawJSON
- `AddToolCallDelta(idx int, id, name, args string)` - Adds tool call delta
- `AddFinishChunk(reason string)` - Adds chunk with finish_reason
- `AddEmptyChunk()` - Adds heartbeat/keepalive chunk

### Accumulation Patterns (from engine.go)

1. **Content accumulation (lines 164-170)**:
   ```go
   if delta.Content != "" {
       content += delta.Content
   }
   ```

2. **Reasoning accumulation (lines 172-181)**:
   ```go
   var extra deltaExtraFields
   if err := json.Unmarshal([]byte(delta.RawJSON()), &extra); err == nil {
       if extra.ReasoningContent != "" {
           reasoningContent += extra.ReasoningContent
       }
   }
   ```

3. **Tool call merging (lines 183-206)**:
   - Uses `toolCallMap[int]*toolCallInfo` keyed by `tc.Index`
   - Merges: Name, Arguments (concatenated), ID across deltas
   - Same index = same tool call, accumulate deltas

### Test Coverage Added

- Content delta accumulation (multiple chunks → single string)
- Reasoning delta accumulation (DeepSeek-style reasoning_content)
- Tool call delta merging (ID, name, arguments across multiple deltas)
- Multiple tool calls with separate indices
- Mixed content/reasoning/tool calls in single stream
- Empty chunks handling (heartbeats)

## Task 11: Decouple agent/registry from config.Config

### Changes Made
1. **registry.go**: Changed `NewAgentRegistry(cfg *config.Config, toolRegistry tool.ToolRegistry) (AgentRegistry, error)` → `NewAgentRegistry(defaultAgentID string) AgentRegistry`
   - Removed `config` import entirely
   - Removed `openai` and `option` imports (only used by deprecated config-based loop)
   - Removed `fmt` and `log/slog` imports (only used by deprecated loop)
   - Removed the entire config-based agent factory creation loop (lines 90-157)
   - Function no longer returns error (nothing can fail in the simplified constructor)
2. **main.go**: Changed `agentRegistry, err := agent.NewAgentRegistry(cfg, toolRegistry)` → `agentRegistry := agent.NewAgentRegistry(cfg.DefaultAgentID)`
   - Removed error handling block for NewAgentRegistry
3. **registry_test.go**: Rewrote tests to use RegisterFactory pattern instead of config-based creation
   - Removed `config` import and all config struct construction
   - Removed TestAgentRegistryValidateTools (validated tools during config-based creation, no longer applicable)
   - All other tests adapted to use RegisterFactory
4. **hook_composition_test.go**: Removed unused `iface` import (pre-existing issue blocking test compilation)

### Verification
- `grep -E '"github.com/copcon/server/internal/config"|config\.' internal/agent/registry.go` → no matches (PASS)
- `NewAgentRegistry` signature: `func NewAgentRegistry(defaultAgentID string) AgentRegistry` (PASS)
- `go build ./...` → succeeds (PASS)
- All registry tests pass (PASS)

## F2 Code Quality Review (2026-05-23)

### Build Failure: core/harness.go:320-327
- Variables declared as concrete types (`*noopSessionManager`, `*noopContextManager`, `*tool.AsyncToolRegistry`) but assigned interface values from `h.config`
- Fix: declare as interface types: `var sessionMgr iface.SessionManager = &noopSessionManager{}`
- This is a pattern issue — when you have a "default noop + optional override", the variable must be the interface type

### Vet Failure: core/agent/engine_test.go:31
- `mockSessionManager.CreateSession` uses `opts ...interface{}` but interface requires `opts ...iface.SessionCreateOption`
- Mock signatures must match interface signatures exactly, including variadic parameter types

### Code Quality: core/capabilities/registry.go
- Clean file overall: proper topological sort, wildcard expansion, compile-time interface checks
- `Engine interface{}` on line 61 is intentional to avoid circular imports (commented)
- Unchecked type assertions are safe since data comes from controlled `Register()` calls

### No TODO/FIXME found in changed files
