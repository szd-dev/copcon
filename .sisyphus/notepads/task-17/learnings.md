# Task 17 Learnings

## Session dependency resolution
- `context_builder/builder.go` imported `server/internal/session` for `session.Message`, `session.ToolCall`, `session.FunctionCall` types
- Core CANNOT import server (circular module dependency) even with replace directives
- Solution: Define `LegacyMessage`, `LegacyToolCall`, `LegacyFunctionCall` bridge types in core/context_builder
- Rename `ConvertSessionToolCalls` → `ConvertLegacyToolCalls` (uses LegacyToolCall instead of session.ToolCall)
- Server side needs adapter functions to convert session types → Legacy types before calling core functions

## Adapter pattern
- Created `adapter.go` files in `server/internal/chat_context/` and `server/internal/tools/` packages
- Each has `sessionMsgToLegacy()` and `sessionToolCallsToLegacy()` conversion functions
- These adapters will be removed once session types are migrated to core/storage or entity packages

## testutil migration
- `tool/manager_test.go` imported `server/internal/testutil` for MockChatContext
- Moved MockChatContext to `core/testutil/chat_context.go` to avoid core→server dependency
- server/internal/testutil/chat_context.go remains unchanged (still used by other server tests)

## Import path sed worked well
- Bulk sed replacement across all server/ .go files for tool, llm, hook, context_builder imports
- Pattern: `|"github.com/copcon/server/internal/TOOL"|"github.com/copcon/core/TOOL"|g`
- No manual per-file edits needed for the bulk of import changes

## Build verification
- `go build ./tool/... ./llm/... ./hook/... ./context_builder/...` in core/ passes
- `go build ./...` in server/ passes
- All core tests pass (tool, llm, hook, context_builder)
- LSP diagnostics: 0 errors in both core/ and server/