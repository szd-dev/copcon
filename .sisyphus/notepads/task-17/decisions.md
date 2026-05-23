# Task 17 Decisions

## Bridge types vs circular dependency
- **Decision**: Define `LegacyMessage/LegacyToolCall/LegacyFunctionCall` bridge types in core/context_builder instead of adding server as a dependency of core
- **Rationale**: Circular module dependencies (core↔server) are architecturally wrong and cause practical issues with `go mod tidy` (network timeouts downloading transitive deps). Bridge types are temporary and will be removed when session types migrate to core/storage.

## Renamed ConvertSessionToolCalls → ConvertLegacyToolCalls
- **Decision**: Rename the function since it now takes `[]LegacyToolCall` instead of `[]session.ToolCall`
- **Rationale**: The function signature changed (input type), so the name should reflect the new type. "Session" in the old name referred to session package types.

## testutil MockChatContext moved to core
- **Decision**: Move MockChatContext to core/testutil/ instead of keeping it as a server import
- **Rationale**: Test utilities needed by core tests must live in core/. The server copy remains for server-side test consumers.