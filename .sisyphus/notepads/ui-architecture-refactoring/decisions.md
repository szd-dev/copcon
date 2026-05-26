# Decisions

## 2026-05-26 Session Start
- Architecture: Headless Core + Framework Adapters + Headless Hooks
- Reference: TanStack Query, Vercel AI SDK, Ark UI patterns
- chat-core: pure TS, 0 runtime deps
- chat-react: thin adapter using useSyncExternalStore
- headless-hooks: framework-agnostic controllers (not React hooks)
- async_tool_* events: out of scope, TODO comment only
- Reconnect: one attempt + fallback, no retry/backoff/jitter
