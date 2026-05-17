# Learnings - Async Tool Execution v2

## 2025-04-06: AsyncToolRegistry Implementation

### Pattern: sync.Map vs sync.RWMutex
- Used `sync.Map` for AsyncToolRegistry instead of `sync.RWMutex` + map
- `sync.Map` is ideal for:
  - Write-once-read-many scenarios (tool states are registered once, then read)
  - Concurrent iteration via `Range()` method
  - No need for explicit lock/unlock management
- Reference: `manager.go:53-56` uses `sync.RWMutex` + `map[string]Tool` for tool registry where locks are needed for iteration
- Decision: `sync.Map` is cleaner for this use case since we need concurrent-safe iteration

### Pattern: AsyncToolState fields
- `CallID`: Unique identifier from OpenAI tool call
- `ToolName`: Human-readable tool name
- `Status`: Lifecycle state (running/completed/failed/cancelled)
- `StartTime`/`EndTime`: Duration tracking
- `Result`: any type to accommodate different tool outputs
- `Error`: string (not error type) for JSON serialization friendliness
- `SessionID`: For grouping tools by session (enables CancelSession)
- `CancelFunc`: `context.CancelFunc` for cancellation support

### Pattern: CancelSession implementation
- Uses `Range()` to iterate over all states
- Filters by `SessionID` AND `Status == StatusRunning`
- Returns count of cancelled tools
- Important: Must check `CancelFunc != nil` before calling

### Build verification
- `go build ./internal/tool/...` passed successfully
- No external dependencies beyond standard library

---

## 2026-04-06: Async Event Types Addition

### Pattern: Event Type Definition
- Event types are string constants grouped in a single const block
- Each event type follows snake_case naming (e.g., `async_tool_started`)
- Group related events with a comment separator

### Pattern: Event Data Structures
- Each event type has a corresponding `XxxData` struct
- Structs use JSON tags matching the field names
- Common fields across async events: `call_id`, `tool_name`, `session_id`
- Duration stored as `int64` in milliseconds (`duration_ms`)

### Go Conventions in this Codebase
- Chinese docstrings for struct types (existing pattern)
- JSON tags use snake_case
- `any` type used for flexible result fields

### Added Event Types (lines 16-19 in event.go)
- `EventAsyncToolStarted` - Emitted when async tool starts execution
- `EventAsyncToolComplete` - Emitted when async tool succeeds
- `EventAsyncToolFailed` - Emitted when async tool fails
- `EventAsyncCompletionPending` - For frontend polling of async state

### Build verification
- `go build ./internal/domain/entity/...` passed successfully

---

## 2026-04-06: Semaphore Addition to AgentEngine

### What was done:
- Added `golang.org/x/sync/semaphore` import to `server/internal/agent/engine.go`
- Added `concurrencySem *semaphore.Weighted` field to AgentEngine struct
- Initialized semaphore with limit 5 in NewAgentEngine constructor

### Key findings:
- `golang.org/x/sync` was already in go.mod (v0.19.0) as an indirect dependency
- No need to run `go get` - dependency already available
- Build passed without any changes to constructor parameters

### Field naming choice:
- Used `concurrencySem` instead of just `semaphore` for clarity
- Descriptive name makes the purpose self-documenting

### Next steps (not part of this task):
- Tasks 7-9 will implement the actual execution logic using this semaphore
- Need to acquire/release semaphore around tool execution calls

---

## 2026-04-06: Empty Input Handling in prepareAgentLoop

### What was done:
- Modified `prepareAgentLoop` method in `server/internal/agent/engine.go` (lines 117-128)
- Wrapped user message persistence in conditional: `if userInput != ""`
- Added comment explaining the async completion trigger scenario

### Key architectural insight:
- Empty input is a legitimate use case, not an error
- Async tool completions can trigger new agent loop rounds without user input
- SSE-offline scenario: frontend polls → discovers pending event → triggers empty input session
- Context already has the async tool result persisted by the goroutine

### Pattern: Conditional persistence
- Check for empty input BEFORE database write
- Skip user message persistence entirely for empty input (not an error case)
- Keep all subsequent logic unchanged (agent resolution, return statement)

### Comment necessity:
- Non-obvious architectural pattern requires explicit documentation
- Code `if userInput != ""` alone doesn't convey WHY empty input is allowed
- Prevents future maintainers from incorrectly "fixing" this as a bug
- Documents SSE reconnection and frontend polling flow

### Build verification:
- `go build ./internal/agent/...` passed successfully
- No changes to method signature or subsequent logic

---

## 2026-04-06: execution_mode Parameter Injection in GetOpenAITools

### What was done:
- Modified `GetOpenAITools()` method in `server/internal/tool/manager.go` (lines 174-210)
- Injected `execution_mode` parameter into each tool's InputSchema
- Parameter: type="string", enum=["sync","concurrent","async"], default="sync"

### Key pattern: Defensive schema copying
- Must create a copy of the tool's InputSchema before modifying
- Original schema is returned by reference from `t.InputSchema()`
- Modifying directly would affect the tool's internal state
- Simple `for k, v := range` copy is sufficient (shallow copy ok for this use case)

### Parameter structure (JSON Schema):
```go
props["execution_mode"] = map[string]any{
    "type":        "string",
    "enum":        []string{"sync", "concurrent", "async"},
    "default":     "sync",
    "description": "Execution mode: 'sync' (wait for result), 'concurrent' (parallel with other tools), 'async' (background execution). Default: sync.",
}
```

### Important design decision:
- Parameter NOT added to `required` array (uses default value)
- This allows backward compatibility - existing tool calls work without specifying execution_mode
- Comment `// Not required - uses default value` documents this decision

### Nil-safe handling:
- Check `schema == nil` → create empty map
- Check `schemaCopy["properties"] == nil` → create empty properties map
- Type assertion `props := schemaCopy["properties"].(map[string]any)` is safe after nil check

### Build verification:
- `go build ./internal/tool/...` passed successfully

---

## 2026-04-06: Execution Mode Dispatcher Implementation

### What was done:
- Added `ExecutionMode` type with constants: `sync`, `concurrent`, `async`
- Created `parseExecutionMode` function to extract and remove `execution_mode` from args
- Refactored `executeToolCall` into dispatcher pattern
- Created `executeSync` method with the original sync execution logic

### Pattern: Dispatcher Pattern for Tool Execution
- `executeToolCall` is now a thin dispatcher that:
  1. Parses args from JSON
  2. Extracts and removes `execution_mode` from args
  3. Routes to appropriate execution method based on mode
- All modes currently route to `executeSync` (placeholders for Tasks 8-9)

### Key Design Decisions:
- `parseExecutionMode` mutates args map in place (deletes `execution_mode` key)
- Returns both mode AND modified args for clean chaining: `mode, args := parseExecutionMode(args)`
- Invalid mode values fall back to `sync` (trust model output, don't error)
- `execution_mode` is removed before passing to tool (tools shouldn't see it)

### Code Structure:
```go
// Type and constants (after ErrNoSession var)
type ExecutionMode string
const (
    ExecutionModeSync       ExecutionMode = "sync"
    ExecutionModeConcurrent ExecutionMode = "concurrent"
    ExecutionModeAsync      ExecutionMode = "async"
)

// Dispatcher in executeToolCall
switch mode {
case ExecutionModeSync:
    return e.executeSync(chatCtx, toolMgr, tc, args)
case ExecutionModeConcurrent:
    return e.executeSync(...) // placeholder for Task 8
case ExecutionModeAsync:
    return e.executeSync(...) // placeholder for Task 9
}
```

### Build verification:
- `go build ./internal/agent/...` passed successfully
- No LSP errors (only pre-existing hints in test files)

---

## 2026-04-06: Async Execution Implementation (executeAsync)

### What was done:
- Added `asyncRegistry *tool.AsyncToolRegistry` field to AgentEngine struct
- Initialized `asyncRegistry` in NewAgentEngine constructor
- Created `executeAsync` method with full async execution flow
- Updated dispatcher to call `executeAsync` for `ExecutionModeAsync` mode

### Pattern: Async Execution Flow
```
1. Create context with 5-minute timeout
2. Register in asyncRegistry (sessionID, callID, toolName, cancelFunc)
3. Emit EventAsyncToolStarted immediately
4. Launch goroutine that:
   a. Defer Unregister from registry
   b. Defer cancel() to clean up context
   c. Acquire semaphore (fail if timeout/cancelled)
   d. Defer Release semaphore
   e. Recover from panics
   f. Execute tool
   g. On success: Complete in registry, emit EventAsyncToolComplete, persist result
   h. On error: Fail in registry, emit EventAsyncToolFailed, persist error
5. Return nil immediately (doesn't wait)
```

### Key Design Decisions:
- **Immediate return**: Main loop returns nil immediately after launching goroutine
- **Semaphore acquisition in goroutine**: Acquire happens inside goroutine, not before
  - If acquisition fails (timeout/cancel), tool is marked as failed
  - This prevents blocking the main loop on semaphore
- **Panic recovery**: Uses `debug.Stack()` to capture stack trace in error message
- **Double defer**: Both `Unregister` and `cancel` are deferred for guaranteed cleanup
- **Result persistence**: Both success and error results are persisted to session.Message

### Pattern: Context Hierarchy
```go
ctx, cancel := context.WithTimeout(chatCtx.Context(), 5*time.Minute)
```
- Derives from `chatCtx.Context()` (request context)
- 5-minute timeout enforced at tool level
- Cancel propagates to tool execution
- `defer cancel()` ensures cleanup even on panic

### Pattern: Event Emission
- `EventAsyncToolStarted`: Emitted synchronously before goroutine (immediate feedback)
- `EventAsyncToolComplete`/`EventAsyncToolFailed`: Emitted in goroutine after execution
- Duration calculated from `startTime` captured before goroutine

### Error Handling:
- Semaphore acquisition failure → `asyncRegistry.Fail()` + emit failed event
- Tool execution error → `asyncRegistry.Fail()` + emit failed event + persist error
- Panic → `asyncRegistry.Fail()` + emit failed event (with stack trace)
- Persistence error → logged (doesn't fail the async operation)

### Build verification:
- `go build ./internal/agent/...` passed successfully
- Only pre-existing LSP hint about unused `safeExecuteTool` method

---

## 2026-04-06: Concurrent Execution Implementation (executeConcurrent)

### What was done:
- Added `sort` and `sync` imports to engine.go
- Created `parsedToolCall` struct to hold tool calls with parsed args
- Created `toolExecutionResult` struct to hold execution results
- Created `executeConcurrent` method for batch concurrent execution
- Modified `handleToolCalls` to group tool calls by execution mode and dispatch appropriately

### Pattern: Concurrent Execution with Semaphore
```
1. Parse all tool calls upfront (extract execution_mode, remove from args)
2. Group by execution mode: sync, concurrent, async
3. Launch goroutines for concurrent tools with:
   a. Acquire semaphore (blocks if 5 already running)
   b. Emit EventToolCall
   c. Execute tool
   d. Store result in thread-safe slice (mutex protected)
   e. Emit EventToolResult
   f. Release semaphore
4. Wait for all goroutines via sync.WaitGroup
5. Sort results by tool call ID
6. Persist all results in order
```

### Key Design Decisions:
- **Semaphore in goroutine**: Acquire happens inside goroutine, not before launch
- **Mutex for result collection**: `sync.Mutex` protects the results slice
- **Wait for all completion**: Uses `sync.WaitGroup` to block until all finish
- **One failure doesn't stop others**: Each goroutine handles its own error
- **Sorted results**: `sort.Slice` ensures consistent ordering by tool call ID

### Pattern: Tool Call Grouping in handleToolCalls
```go
var (
    syncToolCalls       []toolCallInfo
    concurrentToolCalls []parsedToolCall
    asyncToolCalls      []toolCallInfo
)

for _, tc := range result.ToolCalls {
    args := parseArgs(tc.Arguments)
    mode, args := parseExecutionMode(args)
    
    switch mode {
    case ExecutionModeSync:
        syncToolCalls = append(syncToolCalls, tc)
    case ExecutionModeConcurrent:
        concurrentToolCalls = append(concurrentToolCalls, parsedToolCall{tc: tc, args: args})
    case ExecutionModeAsync:
        asyncToolCalls = append(asyncToolCalls, tc)
    }
}

// Execute each group appropriately
for _, tc := range syncToolCalls { e.executeSync(...) }
if len(concurrentToolCalls) > 0 { e.executeConcurrent(...) }
for _, tc := range asyncToolCalls { e.executeAsync(...) }
```

### Type Handling:
- `toolMgr.Execute()` returns `(*tool.ToolResult, error)`
- `entity.ToolResultData.Result` is `any` type
- Use `any` for result storage to accommodate `*tool.ToolResult` directly

### Build verification:
- `go build ./internal/agent/...` passed successfully
- Only informational LSP hints about unused functions (executeToolCall, safeExecuteTool)

---

## 2026-04-06: GetToolStatusTool Implementation

### What was done:
- Created `server/internal/tools/async_tools.go`
- Implemented `GetToolStatusTool` struct implementing Tool interface
- Also includes `CancelToolTool` (bonus from parallel task)

### Pattern: Tool Implementation Structure
```go
type GetToolStatusTool struct {
    asyncRegistry *tool.AsyncToolRegistry
}

func NewGetToolStatusTool(asyncRegistry *tool.AsyncToolRegistry) *GetToolStatusTool
func (t *GetToolStatusTool) Name() string        // "get_tool_status"
func (t *GetToolStatusTool) Description() string // Chinese description
func (t *GetToolStatusTool) InputSchema() map[string]any
func (t *GetToolStatusTool) Execute(chatCtx, args) (*tool.ToolResult, error)
```

### Key Design Decisions:
- **In-memory registry**: Uses `AsyncToolRegistry.GetStatus()` not database
- **Safe response fields**: Excludes `CancelFunc` from response (sensitive)
- **Optional fields**: Only includes `end_time`, `duration`, `result`, `error` if present
- **Time formatting**: Uses `time.RFC3339` for JSON serialization
- **Duration as string**: `EndTime.Sub(StartTime).String()` for human-readable format

### Response Structure:
```json
{
  "call_id": "toolu_abc123",
  "tool_name": "execute_code",
  "status": "completed",
  "start_time": "2026-04-06T15:30:00Z",
  "end_time": "2026-04-06T15:30:05Z",
  "duration": "5.123s",
  "result": {...}
}
```

### Pattern: Shared Helper Functions
- `successResult()` and `errorResult()` defined in `todo_tool.go`
- Reused across all tools in the `tools` package
- No need to redeclare in each tool file

### Build verification:
- `go build ./internal/tools/...` passed successfully
- No LSP errors in tools package

---

## 2026-04-06: GetToolResultTool Implementation

### What was done:
- Added `GetToolResultTool` to `server/internal/tools/async_tools.go`
- Implements Tool interface for retrieving completed async tool results
- Tool name: "get_tool_result"

### Key Design Decisions:
- **Only returns result for completed tools**: Returns error if status is not `StatusCompleted`
- **Direct result return**: Returns `state.Result` directly in `Data` field (no wrapper)
- **No JSON marshaling**: Unlike `GetToolStatusTool`, returns raw result without JSON encoding
- **Strict status check**: Returns error with status info for non-completed tools

### Execute Logic:
```go
1. Extract call_id from args (required)
2. Query registry.GetStatus(callID)
3. If not found → error "tool not found: %s"
4. If status != StatusCompleted → error "tool is not completed (status: %s)"
5. Return result directly in ToolResult.Data
```

### Error Responses:
- Missing call_id: `"call_id is required"`
- Tool not found: `"tool not found: %s"`
- Not completed: `"tool is not completed (status: %s)"`

### Difference from GetToolStatusTool:
| Aspect | GetToolStatusTool | GetToolResultTool |
|--------|-------------------|-------------------|
| Purpose | Query any status | Get result only |
| Returns for running | Full status info | Error |
| Returns for failed | Full status + error | Error |
| Returns for completed | Full status + result | Result only |
| Response format | JSON wrapped | Raw result |

### Build verification:
- `go build ./internal/tools/...` passed successfully
- No LSP diagnostics

---

## 2026-04-07: Async Tools Test Implementation

### What was done:
- Created `server/internal/tools/async_tools_test.go` with 21 test cases
- All tests pass with race detection enabled

### Test Coverage:
- **GetToolStatusTool**: 5 tests (running, completed, failed, cancelled, not found, missing call_id)
- **GetToolResultTool**: 5 tests (completed, running, failed, not found, missing call_id)
- **CancelToolTool**: 4 tests (running, completed, not found, missing call_id)
- **ListAsyncToolsTool**: 3 tests (multiple tools, empty list, with duration)
- **Metadata tests**: 3 tests (names, descriptions, input schemas)

### Key Test Patterns:

1. **Helper function for JSON response parsing**:
```go
func parseResponse(t *testing.T, result *tool.ToolResult) map[string]any {
    data := result.Data.(map[string]any)
    responseStr := data["response"].(string)
    var parsed map[string]any
    err := json.Unmarshal([]byte(responseStr), &parsed)
    require.NoError(t, err)
    return parsed
}
```
   - Needed because `successResult()` JSON-encodes and wraps data in `{"response": "..."}`
   - Tests for tools using `successResult()` must parse the nested JSON

2. **GetToolResultTool returns raw data**:
   - Unlike other tools, `GetToolResultTool` returns `state.Result` directly in `result.Data`
   - No JSON wrapping, no "response" key
   - Tests can access `result.Data.(map[string]any)` directly

3. **Context cancellation verification**:
```go
ctx, cancel := context.WithCancel(context.Background())
registry.Register(sessionID, callID, "execute_code", cancel)
assert.NoError(t, ctx.Err())  // Not cancelled yet
registry.Cancel(callID)
assert.Error(t, ctx.Err())    // Now cancelled
```

4. **JSON number handling**:
   - JSON unmarshal converts integers to `float64`
   - Must use `int(data["count"].(float64))` for assertions

### No database needed:
- `AsyncToolRegistry` is in-memory (`sync.Map`)
- Tests don't require PostgreSQL connection
- Fast execution (~1 second for all 21 tests)

### Build verification:
- `go test ./internal/tools/... -v -race -cover` passes
- Coverage: 21.9% of statements in tools package

---

## 2026-04-07: Execution Mode Tests Implementation

### What was done:
- Created `server/internal/agent/engine_execution_test.go` with 12 comprehensive test cases
- All tests pass with race detection enabled (`-race` flag)

### Test Coverage:
- **TestSyncExecution**: Sequential execution and result persistence
- **TestConcurrentExecution**: Parallel execution with overlapping timestamps
- **TestConcurrencyLimit**: Semaphore limiting to max 5 concurrent tools
- **TestAsyncExecution**: Immediate return behavior of async tools
- **TestAsyncCompletionSSEOnline**: Event emission for SSE-connected clients
- **TestAsyncCompletionSSEOffline**: Registry persistence when SSE disconnected
- **TestErrorIsolation**: One tool failure doesn't affect others
- **TestResultOrdering**: Results sorted by tool call ID
- **TestPanicRecovery**: Panicked tool doesn't crash server
- **TestGoroutineLeak**: No goroutine leaks after execution
- **TestExecutionModeParsing**: Mode extraction from arguments
- **TestContextCancellation**: Tools respect context cancellation

### Key Test Patterns:

1. **Mock tool with controllable execution**:
```go
type mockControllableTool struct {
    name        string
    duration    time.Duration
    result      *tool.ToolResult
    err         error
    shouldPanic bool
    execCount   atomic.Int32
    startTimes  []time.Time
}
```
- Uses `atomic.Int32` for thread-safe execution counting
- Uses `[]time.Time` with mutex for timestamp tracking
- Supports panic simulation for recovery testing

2. **Concurrency verification via timestamps**:
```go
maxStartDiff := max(
    startTimes1[0].Sub(startTimes2[0]).Abs(),
    startTimes1[0].Sub(startTimes3[0]).Abs(),
)
assert.Less(t, maxStartDiff, 10*time.Millisecond)
```
- Overlapping start times prove true concurrency

3. **Semaphore limiting verification**:
```go
// 7 tools × 100ms each with limit 5 = ~200ms (2 batches)
assert.GreaterOrEqual(t, duration, 150*time.Millisecond)
assert.Less(t, duration, 350*time.Millisecond)
```
- Duration proves batching, not unlimited parallelism

4. **Goroutine leak detection**:
```go
runtime.GC()
time.Sleep(100*time.Millisecond)
baseGoroutines := runtime.NumGoroutine()
// ... execute tools ...
runtime.GC()
time.Sleep(200*time.Millisecond)
finalGoroutines := runtime.NumGoroutine()
assert.LessOrEqual(t, finalGoroutines - baseGoroutines, 2)
```
- GC and sleep allow goroutines to settle
- Small variance allowed for test infrastructure

5. **Event tracking with timeout**:
```go
func trackEvents(eventChan <-chan entity.Event, timeout time.Duration) []entity.Event {
    events := make([]entity.Event, 0)
    timeoutChan := time.After(timeout)
    for {
        select {
        case event, ok := <-eventChan:
            if !ok { return events }
            events = append(events, event)
        case <-timeoutChan:
            return events
        }
    }
}
```

6. **Result type assertion**:
```go
// Result is *tool.ToolResult, not map
if result, ok := data.Result.(*tool.ToolResult); ok {
    assert.True(t, result.Success)
}
```

### No database needed:
- Uses mock implementations from `engine_test.go`
- `mockToolManagerWithTools` for tool registration
- `mockOrderedContextManager` for message ordering verification

### Build verification:
- `go test ./internal/agent/... -v -race -run "TestSync|TestConcurrent|..."` passes
- All 12 tests pass with race detection
- No LSP diagnostics
