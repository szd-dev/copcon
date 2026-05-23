# Task 17 Issues

## Empty directories in server/
- server/internal/{tool,llm,hook,context_builder}/ directories are now empty (no Go files)
- These can be safely removed with `rmdir` but leaving them for now
- Go toolchain ignores empty directories, no build impact

## Server testutil duplication
- core/testutil/chat_context.go and server/internal/testutil/chat_context.go both contain MockChatContext
- This is intentional duplication during migration; server tests still reference server/internal/testutil
- Can be consolidated once all server test files migrate to core/testutil