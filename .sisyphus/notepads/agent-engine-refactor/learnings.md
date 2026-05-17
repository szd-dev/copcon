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
