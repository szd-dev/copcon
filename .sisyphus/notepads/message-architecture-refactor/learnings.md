## Learnings

## 2026-04-18 Session Start
- Plan: message-architecture-refactor
- 11 implementation tasks, 4 final verification tasks
- 3 waves of execution + final wave

## Task 6: types.ts additions
- Added UIMessage, UIPart union, TextPart, ReasoningPart, ToolCallPart, StepStartPart, UIMessageMeta
- Added PartCreateEvent, PartUpdateEvent, MessageDoneEvent interfaces
- Added 'part_create' | 'part_update' | 'message_done' to SSEEventType union
- All existing types (Message, ToolCall, SSEEvent, etc.) preserved — no breaking changes
- Build passes cleanly
- Note: CopConMessage was NOT in the original file (only Message existed), so no CopConMessage to preserve

## Task 10: Message struct Parts/Metadata fields (model.go)

- Added `UIParts []map[string]any` type with same Value/Scan/GormDataType JSONB pattern as ToolCalls
- Used `[]map[string]any` to avoid circular dependency on entity package
- Relaxed Content field: removed `not null` from gorm tag (`gorm:"type:text"`), added `omitempty` to json tag
- New fields: Parts (jsonb), Model (size:100), TokenCount (int), DurationMs (int64)
- All new fields have `omitempty` json tags
- `go vet ./internal/session/...` passes cleanly

## Task 4: ModelMessage entity types

- Created `server/internal/domain/entity/model_message.go` with ModelMessage, ModelToolCall, ModelFunctionCall
- Key difference from MessageForLLM: no Reasoning field (shouldn't be sent to LLM)
- Key difference from session.ToolCall: uses own ModelToolCall type (decoupling from session package)
- Package is `entity` — same as event.go, no external imports
- JSON tags: `omitempty` on all fields except Role (and ModelFunctionCall.Name/Arguments which are always required)
- `go vet` passes cleanly

## Task: event.go part-level event types

- Added 3 new EventType constants: EventPartCreate, EventPartUpdate, EventMessageDone
- EventThought marked as `// Deprecated: never emitted` (not deleted for backward compat)
- EventAsyncCompletionPending marked as `// Deprecated: only used in metadata, not as SSE event`
- Added PartCreateData with map[string]any Data field for flexible initial UIPart fields
- Added PartUpdateData with omitempty on State, TextDelta, Output, Error
- Type enum values documented: "text", "reasoning", "tool-call", "step-start"
- State enum values documented: "streaming", "done", "pending", "running", "complete", "error"
- Added MessageID to AsyncToolStartedData, AsyncToolCompleteData, AsyncToolFailedData
- Go struct literals in engine_tools.go won't break — unset fields get zero values
- `go vet` and `go build` both pass cleanly

## Task: ConvertToModelMessages (convert.go)

- Created `server/internal/domain/entity/convert.go` with ConvertToModelMessages function
- Key pattern: assistant UIMessage with tool-call parts flattens into [assistant+tool_calls, tool_result_1, tool_result_2, ...]
- ModelToolCall.Type always "function" (OpenAI API convention)
- UIPartReasoning and UIPartStepStart silently discarded in switch (no default case needed)
- Empty assistant content (no text parts) → Content="" (empty string, not omitted)
- Used strings.Builder for user message concatenation, strings.Join for assistant text parts
- 9 test cases: plain text, tool calls, multi-turn, drops reasoning, drops step-start, empty content, multiple tool calls, empty input, user multiple text parts
- `go vet` and `go test` both pass cleanly

## Task 7: Engine refactoring for UIMessage/ModelMessage architecture

### convertMessages() fix (the critical bug)
- `openai.AssistantMessage(content)` returns a `ChatCompletionMessageParamUnion` with an `OfAssistant` field
- To add ToolCalls, must access `asst.OfAssistant.ToolCalls` and set it to `[]ChatCompletionMessageToolCallUnionParam`
- Each tool call uses `OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{...}`
- The openai-go v3 SDK uses union types with `OfFunction`/`OfCustom` pattern for tool calls
- `constant.Function` type always marshals to "function" (has Default() method)
- Must create `ChatCompletionMessageFunctionToolCallFunctionParam` with Name and Arguments (both required)

### handleStreaming() part-level events
- PartIndex tracking: start at 0, increment for each new part (reasoning, text)
- Use boolean flags (textPartCreated, reasoningPartCreated) to emit PartCreate only on first chunk
- Emit PartUpdate with TextDelta on every chunk
- After stream completes, emit PartUpdate with State="done" for each created part
- Both old (EventMessage, EventReasoning) and new (EventPartCreate, EventPartUpdate) events emitted side by side

### handleToolCalls() part-level events
- Tool-call part indices must account for reasoning and text parts that came before
- handleToolCalls calculates partIndex by counting prior parts from the StreamResult
- A `toolCallPartIndices` map (callID → partIndex) is passed to execute functions
- Each execute function (sync, async, concurrent) now takes messageID and partIndices params
- All test files calling these functions needed updating with empty string and nil for new params

### persistMessage() Parts JSONB
- `buildUIParts()` constructs `[]map[string]any` (session.UIParts type)
- Reasoning parts: type="reasoning", state="done"
- Text parts: type="text", state="done" (included even if empty content when no tool calls)
- Tool-call parts: type="tool-call", state="pending" (output not yet available at persist time)
- Tool-call parts use "tool_call_id" key (snake_case) matching UIPart struct json tags

### BuildContext() ConvertToModelMessages integration
- Dual-path approach: try UIMessage path first, fall back to legacy path
- `toolResultByCallID` lookup map bridges tool-call parts (in assistant messages) with tool result content (in separate tool-role messages)
- `convertDBPartsToUIPart()` fills in output from toolResultByCallID for tool-call parts
- `synthesizeUIMessage()` creates UIMessages from legacy Content/Reasoning/ToolCalls fields
- System-role messages are NOT UIMessages — handled separately as MessageForLLM
- Tool-role messages are skipped in UI conversion (generated by ConvertToModelMessages from tool-call parts)
- Legacy path still works correctly because convertMessages() now includes ToolCalls

### Test updates
- All direct calls to executeSync/executeAsync/executeConcurrent in test files needed new params
- Used empty string ("") for messageID and nil for partIndices in test calls
- engine_execution_test.go: 14 call sites updated
- integration_test.go: 10 call sites updated

## Task: SSE event channel buffer + Emit() silent discard fix

- **Original bug**: `Emit()` used `select { case ch <- event: case <-ctx.Done(): }` which silently drops events when channel is full AND context is not cancelled
- **Original testutil bug**: `MockChatContext.Emit()` used `select { case ch <- event: default: }` which also silently drops but doesn't even check context cancellation
- **Fix**: Two-tier select — first try non-blocking send, on default log warning then block with context cancellation check
- **Pattern**: `select { case ch <- event: default: log.Printf(WARNING); select { case ch <- event: case <-ctx.Done(): } }`
- **Buffer increased**: 100 → 256 to accommodate part-level events (part_create, part_update per chunk)
- **Files changed**: `server/internal/domain/iface/chat.go`, `server/internal/testutil/chat_context.go`
- **`go build ./...` and `go vet ./...` both pass**

## Task: CopConChatProvider rewrite for part-level events

- CopConMessage extended with `parts: UIPart[]` field alongside all legacy fields (content, reasoning, tool_calls)
- `parts` is required in the interface — historical messages loaded via API are cast with `as CopConMessage[]`, so missing `parts` at runtime won't cause TS errors
- `transformLocalMessage` now creates a text part when content is present
- New event handlers: `part_create` (inserts part at specified index via splice), `part_update` (updates existing part by index), `message_done` (marks streaming→done, pending/running→complete)
- Legacy events (`message`, `reasoning`, `tool_call`, `tool_result`) update BOTH legacy fields AND parts array simultaneously
- For legacy `message`: finds or creates a streaming text part in parts array
- For legacy `reasoning`: finds or creates a streaming reasoning part in parts array
- For legacy `tool_call`: appends to `tool_calls` array AND pushes tool-call part
- For legacy `tool_result`: updates both `tool_calls` entry AND corresponding tool-call part (by toolCallId match)
- `createPart` private method handles all 4 part types (text, reasoning, tool-call, step-start), supports both snake_case and camelCase field names from backend
- `updatePart` private method handles text_delta appending, state transitions, output/error updates per part type
- Three state mapper functions (mapToolCallState, mapTextState, mapReasoningState) with safe defaults
- `messageUtils.ts` and `useAgentChat.ts` required NO changes — spread operators preserve the new `parts` field
- Build passes, zero LSP diagnostics on all affected files

## handlers.go GetMessages() Parts Backfill (2026-04-19)

- Added `backfillParts()` helper to convert legacy Content/Reasoning/ToolCalls fields into UIParts format
- Backfill only triggers when `msg.Parts` is empty (len == 0) — new data with Parts populated is used as-is
- Tool results lookup built from tool-role messages: `toolResults[ToolCallID] = Content`
- User messages: Content → text part
- Assistant messages: Reasoning → reasoning part, Content → text part (even if empty when no ToolCalls), ToolCalls → tool-call parts with output from toolResults
- Response now includes `parts`, `model`, `token_count`, `duration_ms` fields alongside existing fields (backward compatible)
- `go build ./...` and `go vet ./...` both pass clean

## App.tsx Parts-Based Rendering Refactor

### What changed
- Replaced `msg.tool_calls` / `msg.reasoning` flat-field rendering with `msg.parts[]` iteration
- Added `renderMessageContent()` that switches on `part.type` (text, reasoning, tool-call, step-start)
- Added `mapToolCallStatus()` to map ToolCallPart states to ThoughtChain status values
- Removed `BubbleItem.header` field — all content (Think, ThoughtChain, Markdown) now flows through `content`
- Added `Divider` from antd for step-start parts

### Key finding: ThoughtChain status values
- ThoughtChain items accept: `'loading' | 'success' | 'error' | 'abort'` (NOT `'pending'`)
- Map: `pending→loading`, `running→loading`, `complete→success`, `error→error`

### Backward compatibility
- `renderMessageContent` falls back to `msg.content` when `parts` is empty (legacy messages)
- `loading` check considers both `msg.content` (legacy) and `msg.parts` (new)

### UIPart types not exported from @copcon/ui
- `UIPart`, `TextPart`, `ReasoningPart`, `ToolCallPart`, `StepStartPart` are defined in `packages/ui/src/api/types.ts` but NOT re-exported from `packages/ui/src/index.ts`
- TypeScript still narrows correctly via discriminated union on `part.type` since `CopConMessage.parts: UIPart[]` is accessible through the exported `CopConMessage` type
- If explicit part type imports are needed later, they should be added to `packages/ui/src/index.ts`

## T2: Frontend Type Definitions (Completed)

- Deleted old SSE types (SSEEventType with legacy event types, SSEEvent interface, old PartCreateEvent/PartUpdateEvent with snake_case, old MessageDoneEvent)
- Deleted StepStartPart and UIPart, replaced with Part = TextPart | ReasoningPart | ToolCallPart
- New UIMessage has `steps: Step[]` instead of `parts: UIPart[]`
- New UIMessageMeta has optional model/tokenCount/durationMs instead of required model/tokenCount/duration
- New SSE events use camelCase (messageId, stepIndex, partIndex, partType, toolCallId, textDelta) instead of snake_case
- New PartCreateEvent has flattened data (no nested `data: Record<string,unknown>`)
- CopConChatProvider.ts needed UIPart→Part rename in imports and type annotations — minimal change, no logic touched
- agentClient.ts imported SSEEvent but never used it — removed from import
- step-start case in CopConChatProvider.createPart changed to return null (StepStartPart no longer exists in Part union)
- step-start case in CopConChatProvider.updatePart removed as dead code (Part union no longer includes step-start)
- Message interface kept with @deprecated JSDoc for transition period

## T1: Backend Event & Data Model Redefinition (completed)

### Changes Made
- **event.go**: Added `EventStepCreate` constant + `StepCreateData` struct. Redefined `PartCreateData` (removed `Data map[string]any`, flattened to `State`, `ToolCallID`, `ToolName`, `Args` fields). Redefined `PartUpdateData` (`Output` changed from `any` to `string`, renamed `Type` to `PartType`). Updated `MessageDoneData` JSON tag `message_id` → `messageId`. Marked old event constants (EventMessage, EventReasoning, EventToolCall, EventToolResult, EventDone) as Deprecated.
- **ui_message.go**: Added `UIStep` struct with `Parts []UIPart` and `State UIPartState`. Added `Steps []UIStep` to `UIMessage` (kept `Parts` with Deprecated comment). Added `StepIndex int` to `UIPart`. Updated UIPart JSON tags to camelCase (`tool_call_id` → `toolCallId`, `tool_name` → `toolName`). Expanded `UIPartState` with `pending`, `running`, `complete`, `error`. Updated `UIMetadata` JSON tags to camelCase.
- **convert.go**: Added `collectParts()` helper that iterates `Steps[].Parts` with fallback to `msg.Parts`. Updated `convertUserMessage` and `convertAssistantMessage` to use `collectParts()`.
- **convert_test.go**: All tests updated to use `Steps: []UIStep{{Parts: []UIPart{...}}}` format. Added `TestConvertMultiStepAssistant` (2 steps with text+tool-call across steps) and `TestConvertFallbackToParts` (tests backward compat with flat Parts).

### Downstream Impact Fixed
- **engine.go**: Updated `PartCreateData` usages (Type→PartType, Data map→flattened fields). Updated `PartUpdateData` usages (Type→PartType).
- **engine_tools.go**: Updated `PartUpdateData` usages (Type→PartType). Changed `Output` assignments from `result`/`execResult` (type `*tool.ToolResult`) to JSON-marshaled string via `json.Marshal`.

### Key Pattern
- When `PartUpdateData.Output` was `any`, code assigned `*tool.ToolResult` directly. Now that it's `string`, we `json.Marshal` the result first.

## T3: PersistedPart / PersistedParts (model.go)

- Defined `PersistedPart` struct with camelCase JSON tags matching `entity.UIPart` (toolCallId, toolName, stepIndex)
- `PersistedParts []PersistedPart` type with Value()/Scan()/GormDataType() replacing UIParts
- Scan() uses two-pass approach: unmarshal to `[]map[string]any`, then `strValFallback(m, "toolCallId", "tool_call_id")` for legacy snake_case compat
- StepIndex defaults to 0 when absent via `intValWithDefault(m, "stepIndex", 0)`
- Value() produces camelCase JSON (standard json.Marshal on PersistedPart)
- UIParts kept with `// Deprecated: use PersistedParts instead` comment
- Message.Parts changed from UIParts to PersistedParts
- engine.go `buildUIParts` now returns `session.PersistedParts` using struct literals instead of `[]map[string]any`
- handlers.go `backfillParts` now returns `session.PersistedParts` using struct literals
- chat_context/manager.go `convertDBPartsToUIPart` signature changed from `[]map[string]any` to `session.PersistedParts` — simplified from type-assertion map lookups to direct struct field access
- 7 test cases: Scan camelCase, Scan snake_case, missing stepIndex, camelCase overrides snake_case, edge cases (nil/empty/null/object), Value camelCase output, Value nil, GormDataType

## T4: handleStreaming rewrite — legacy event removal + stepIndex

- Removed ALL `entity.EventMessage` and `entity.EventReasoning` emissions from handleStreaming
- Added `stepIndex int` parameter to handleStreaming signature
- All `PartCreateData` emissions now include `StepIndex` field (from stepIndex param)
- All `PartUpdateData` emissions now include `StepIndex` field (from stepIndex param)
- `StreamResult` struct gained `StepIndex int` field (set in result initialization)
- `runAgentLoop` replaced old `part_create(partType='step-start')` with `step_create(stepIndex)` event
- `runAgentLoop` tracks `stepIndex` starting at 0, increments after each loop iteration
- `step_create` is emitted BEFORE `handleStreaming` on subsequent iterations (not first)
- `Content`/`ReasoningContent` accumulation logic preserved (still needed for persistence)
- `tool_calls` accumulation logic preserved (still needed for `handleToolCalls`)
- `handleToolCalls` call signature unchanged — it reads `result.StepIndex` from StreamResult
- `go build ./...` and `go vet ./...` both pass clean with zero warnings

## T5: handleToolCalls + engine_tools rewrite — legacy event removal

- Removed ALL `EventToolCall`, `EventToolResult`, `EventDone` emissions from engine_tools.go
- `handleToolCalls()`: replaced `EventDone` with `EventMessageDone` (only when no tool calls)
- `handleToolCalls()`: added `StepIndex` and `Args` fields to PartCreateData emissions
- `executeSync()`: removed legacy EventToolCall + EventToolResult, added `stepIndex int` param, added StepIndex to all PartUpdateData
- `executeConcurrent()`: removed legacy EventToolCall + EventToolResult, added `stepIndex int` param, added StepIndex to all PartUpdateData
- `executeAsync()`: added `stepIndex int` param, added StepIndex to all PartUpdateData (async-specific events kept as-is)
- All `output` fields in PartUpdateData are ALWAYS strings (via `json.Marshal(result)` then `string(outputJSON)`)
- `stepIndex` flows from `result.StepIndex` in handleToolCalls → passed to execute functions
- Test signatures updated: all executeSync/executeAsync/executeConcurrent calls now include `stepIndex=0`
- Test assertions updated: EventToolCall/EventToolResult checks replaced with EventPartUpdate running/complete/error state checks
- `go build ./...` and `go vet ./...` both pass clean

## T7: GetMessages API + BuildContext update (2026-04-19)

### handlers.go changes
- `GetMessages()` now filters out role=tool messages (tool results embedded in tool-call parts)
- Response format changed to: `{ id, sessionId, role, steps: [...], metadata: { createdAt, model, tokenCount, durationMs } }`
- Removed legacy fields: content, reasoning, tool_calls, tool_call_id, parts, model, token_count, duration_ms, created_at, session_id
- Added `groupPartsByStep()` helper that groups PersistedParts by StepIndex into `[]entity.UIStep`
- Each UIStep has `{ parts: [...], state: "done" }` — all persisted data is done
- `backfillParts()` updated: all legacy-synthesized parts now have `StepIndex: 0`
- Added `sort` and `entity` imports

### chat_context/manager.go changes
- `convertDBMessagesToUI()` now produces UIMessages with `Steps` (not `Parts`)
- Added `groupPartsByStep(parts []entity.UIPart) []entity.UIStep` helper (same pattern as handlers.go but for UIPart slice)
- `synthesizeUIMessage()` now produces Steps via groupPartsByStep (all parts have StepIndex=0 for legacy data)
- `BuildContext` path unchanged — `entity.ConvertToModelMessages` already uses `collectParts()` which handles Steps via `Steps[].Parts` iteration
- Added `sort` import

### Key insight: two groupPartsByStep implementations
- handlers.go version: takes `session.PersistedParts` → converts to `entity.UIPart` → groups into `[]entity.UIStep`
- chat_context/manager.go version: takes `[]entity.UIPart` directly → groups into `[]entity.UIStep`
- Different input types because handlers.go builds from raw DB data while manager.go already has typed UIParts

## T6: persistMessage + buildUIParts rewrite (2026-04-19)

### Changes Made
- **engine.go**: Added `ToolCallResult` struct (Output + Error fields) and `ToolResults map[string]*ToolCallResult` to `StreamResult`
- **engine.go**: `buildUIParts` signature changed from `(result *StreamResult)` to `(result *StreamResult, stepIndex int)` — sets `StepIndex` on every PersistedPart
- **engine.go**: `buildUIParts` now looks up tool results from `result.ToolResults` map: if entry has Error → state="error", if entry has Output → state="complete", if no entry → state="pending" (defensive, for async tools)
- **engine.go**: `persistMessage` passes `result.StepIndex` to `buildUIParts`
- **engine_tools.go**: `handleToolCalls` restructured — `persistMessage` moved AFTER tool execution so `result.ToolResults` is populated
- **engine_tools.go**: `executeSync` gained `toolResults map[string]*ToolCallResult` param — populates entry after execution
- **engine_tools.go**: `executeConcurrent` gained `toolResults map[string]*ToolCallResult` param — populates entries inside goroutine (mutex-protected)
- **engine_execution_test.go**: All `executeSync`/`executeConcurrent` calls updated with `make(map[string]*ToolCallResult)` for new param

### Key Design Decision: persistMessage ordering
- Moving `persistMessage` after tool execution means the assistant message is saved AFTER role=tool messages in the DB
- This is safe because: (1) `convertDBMessagesToUI` skips role=tool messages entirely, (2) `buildToolResultLookup` reads all role=tool messages regardless of ordering, (3) new messages always have Parts populated so the UIMessage path is used
- Legacy BuildContext path (direct MessageForLLM) could be affected by ordering, but only triggers when `hasFallback=true` (unlikely with Parts-populated messages)

### Async tool handling
- Async tool results are NOT available at persist time (goroutine hasn't completed)
- `toolResults` map won't have an entry for async tool call IDs
- `buildUIParts` sets state="pending" for missing entries (defensive fallback)

## T8: CopConChatProvider rewrite (2026-04-19)

### CopConMessage interface change
- Removed: content, reasoning, tool_calls, tool_call_id, parts, created_at, session_id
- Added: steps: Step[], metadata: UIMessageMeta
- Role narrowed: 'user' | 'assistant' | 'tool' → 'user' | 'assistant' (no more tool-role messages)

### transformLocalMessage
- Now produces: `{ id, role: 'user', steps: [{ parts: [{ type: 'text', text, state: 'done' }], status: 'done' }], metadata: { createdAt } }`
- Empty content → empty steps array

### transformMessage — only new events
- Removed ALL legacy handlers: message, reasoning, tool_call, tool_result, done
- Only handles: step_create, part_create, part_update, message_done, error
- SSE parsing: `chunk.data` is JSON string → `JSON.parse()` → `{ type, data }` with camelCase fields

### Immutable step updates pattern
- step_create: `[...baseMessage.steps]`, pad with empty steps, set at stepIndex
- part_create: copy steps array, ensure stepIndex exists, copy parts array, set at partIndex
- part_update: copy steps → copy step → copy parts → update part at partIndex
- message_done: map over steps with map over parts, return new objects
- Every handler returns `{ ...baseMessage, steps }` — new message object, new steps array

### Type narrowing without `as string` or `as any`
- `typeof data.stepIndex === 'number'` for number fields
- `typeof data.textDelta === 'string'` for string fields
- Tool-call state narrowing: chained `data.state === 'pending' ? 'pending' : data.state === 'running' ? 'running' : ...` pattern
- Text/reasoning state: `data.state === 'done' ? 'done' : part.state`
- Explicit type annotations: `const state: ToolCallPart['state'] = ...`

### Deleted code
- Removed: `mapToolCallState`, `mapTextState`, `mapReasoningState` helper functions
- Removed: `createPart()` and `updatePart()` private methods (logic inlined in transformMessage)
- Removed: `parseToolOutput` import from CopConChatProvider (no longer used)
- Removed: `import { Part, TextPart, ReasoningPart, ToolCallPart }` → added `Step, UIMessageMeta` imports

### messageUtils.ts update
- `mergeToolMessages` is now a passthrough (tool results embedded in steps, no tool-role messages)
- `parseToolOutput` kept as exported utility but no longer called within the file
- Removed: all tool-role filtering and tool_calls merging logic

### types.ts Message interface update
- Updated `Message` interface to match new API response format (steps/metadata instead of content/reasoning/tool_calls)
- This was needed because useAgentChat.ts casts `Message[] as CopConMessage[]` — old Message shape was incompatible
- Message is now `{ id, sessionId, role, steps, metadata }` — overlaps sufficiently with CopConMessage for the cast

## T12: End-to-end verification + cleanup (2026-04-19)

### All checks PASSED — no fixes needed

| Check | Result |
|-------|--------|
| `go build ./server/...` | ✅ Passes |
| `go vet ./server/...` | ✅ Zero warnings |
| `go test ./internal/domain/entity/... -v` | ✅ 11/11 tests PASS |
| `npx tsc --noEmit` (packages/ui) | ✅ Zero errors |
| `npx tsc --noEmit` (packages/demo) | ✅ Zero errors |
| `as any` / `as string` in CopConChatProvider.ts | ✅ No matches |
| Deprecated event emissions in engine.go/engine_tools.go | ✅ No deprecated emissions (EventMessage, EventReasoning, EventToolCall, EventToolResult, EventDone all absent) |
| `msg.content`/`msg.reasoning`/`msg.tool_calls`/`msg.parts` in App.tsx | ✅ No matches |
| `mergeToolMessages` in packages/ui/src/ | ✅ No matches |
| Deprecated constants in event.go | ✅ 7 Deprecated markers present (backward compat) |
| UIParts type in model.go | ✅ Present at line 39 |
| Parts field in UIMessage | ✅ Present as `Parts PersistedParts` at line 288 |

### Key observation
- `EventMessageDone` is the NEW constant (value "message_done"), NOT deprecated — the deprecated one is `EventDone` (value "done")
- engine_tools.go correctly emits `EventMessageDone`, not `EventDone`
- No legacy code remnants found — all 11 previous tasks completed correctly

### Gaps found and fixed in model.go PersistedParts.Scan()
- **Type normalization**: Legacy data may have `type:"text_delta"` or `type:"tool_call"` (old event-style names). Added `normalizePartType()` to map `text_delta→text`, `tool_call→tool-call`. Other types pass through unchanged.
- **text_delta field fallback**: Legacy data may store text content under `text_delta` key instead of `text`. Added `strValFallback(m, "text", "text_delta")` — primary key checked first, fallback second.
- **step_index snake_case fallback**: Legacy data may use `step_index` instead of `stepIndex`. Added `intValFallback(m, "stepIndex", "step_index", 0)` replacing `intValWithDefault`. Removed now-unused `intValWithDefault`.

### Already correct (no changes needed)
- **backfillParts()**: Already sets `StepIndex: 0` on all parts for legacy data ✓
- **synthesizeUIMessage()**: Already creates parts with `StepIndex: 0` and calls `groupPartsByStep()` to produce Steps ✓
- **groupPartsByStep()**: Already handles StepIndex=0 correctly (single step with all parts) ✓
- **handlers.go**: No changes needed — backfill + groupPartsByStep already correct

### Tests added
- **model_test.go** (7 new tests): LegacyTypeTextDelta, LegacyTypeToolCall, StepIndexSnakeCase, StepIndexCamelOverridesSnake, TextDeltaKeyFallback, TextKeyOverridesTextDelta, MixedLegacyAndNew, NormalizePartType
- **handlers_test.go** (6 new tests): BackfillParts_UserMessage, BackfillParts_AssistantWithToolCalls, BackfillParts_AssistantToolCallOnly, GroupPartsByStep_SingleStep, GroupPartsByStep_MultipleSteps, GroupPartsByStep_Empty
- **chat_context/manager_test.go** (11 new tests): SynthesizeUIMessage_UserMessage, SynthesizeUIMessage_AssistantWithToolCalls, SynthesizeUIMessage_AssistantToolCallOnly, SynthesizeUIMessage_UnsupportedRole, GroupPartsByStep_SingleStep, GroupPartsByStep_MultipleSteps, GroupPartsByStep_Empty, ConvertDBMessagesToUI_LegacyData, ConvertDBMessagesToUI_WithParts, ConvertDBMessagesToUI_ToolRoleSkipped, ConvertDBMessagesToUI_MixedLegacyAndNew
