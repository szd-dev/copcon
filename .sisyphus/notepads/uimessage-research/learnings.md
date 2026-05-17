
## UIMessage Research Learnings (2026-04-19)

### Key Source Files in vercel/ai (commit f20e6b4)
- `packages/ai/src/ui/ui-messages.ts` — All type definitions (UIMessage, UIMessagePart, ToolUIPart, etc.)
- `packages/ai/src/ui/convert-to-model-messages.ts` — Conversion logic from UIMessage to ModelMessage
- `packages/ai/src/ui/process-ui-message-stream.ts` — Stream chunk → part mutation state machine
- `packages/ai/src/ui-message-stream/ui-message-chunks.ts` — Wire protocol chunk types (Zod schemas + TypeScript types)
- `packages/ai/src/ui/last-assistant-message-is-complete-with-tool-calls.ts` — Completeness check using step-start boundaries

### Core Patterns
1. **UIMessage has NO `content` string** — everything is in `parts[]` array
2. **ToolUIPart is a discriminated union on `state`** with 7 variants: input-streaming, input-available, approval-requested, approval-responded, output-available, output-error, output-denied
3. **Tool call + result = ONE part** (inline output/errorText), unlike ModelMessage which splits into assistant tool-call + tool tool-result
4. **step-start parts** divide a single UIMessage into logical steps; convertToModelMessages creates separate assistant+tool message pairs per step
5. **Provider-executed tools** have tool-result inline in assistant content (no separate tool message)
6. **Dynamic tools** (type: 'dynamic-tool') for MCP/runtime tools vs **Static tools** (type: 'tool-${NAME}') for known tools
7. **Streaming protocol** uses fine-grained chunks: text-start/delta/end, reasoning-start/delta/end, tool-input-start/delta/available/error, tool-output-available/error/denied

### Conversion Key Logic
- input-streaming parts are SKIPPED in conversion (no tool-call emitted)
- Each step-start triggers a block flush (assistant + tool messages)
- output-error uses rawInput fallback when input is undefined
- createToolModelOutput() wraps output for provider compatibility (text/error-text/error-json)
