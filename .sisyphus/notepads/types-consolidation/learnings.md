# Types Consolidation Learnings

## Completed: Consolidated types from ui/src/api/types.ts and CopConChatProvider.ts into chat-core/src/types.ts

### What was done:
1. Created `packages/chat-core/src/types.ts` with all types consolidated:
   - Copied all types from `ui/src/api/types.ts` (except deprecated `Message`)
   - Merged `UIMessage` → `CopConMessage` (same shape, renamed)
   - Added `CopConInput`, `CopConSSEOutput` from `CopConChatProvider.ts`
   - Added new types: `SessionStatus`, `SessionState`, `ChatSessionCallbacks`
2. Updated `packages/chat-core/src/index.ts` with barrel re-exports for all 26 types

### Build status:
- `vite build` succeeds
- `tsc --emitDeclarationOnly` has 13 pre-existing errors in `agent-client.ts` and `sse-parser.ts` (DOM types like `fetch`, `TextDecoder`, `ReadableStreamDefaultReader` — `lib: ["ES2022"]` excludes these)
- Zero errors from types.ts or index.ts

### Key constraints respected:
- No DOM lib types used
- No framework imports (@ant-design, react)
- Field names preserved as-is (camelCase + snake_case for backend alignment)
- `InterruptPayload` field names kept exact (`interruptId`, `interruptType`)
- No JSDoc comments added