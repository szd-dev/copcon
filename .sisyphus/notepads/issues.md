
## F2 Code Quality Review Issues (2026-05-26)

### Real Issues
1. **chat-session.ts:210** — Empty `catch {}` swallowing `loadMessages()` error in reconnect fallback path. Should at minimum log the error or surface it via callback.
2. **message-reducer.ts:236** — `async_tool_started`, `async_tool_complete`, `async_tool_failed` events are unhandled (TODO). Currently returns baseMessage unchanged. These events are silently dropped.

### Minor
3. **demo/App.tsx:365** — Empty `catch {}` around `client.stop()` on cancel. Non-critical but should log.

### Non-Issues (Acceptable Patterns)
- SSE JSON.parse empty catches (chat-session.ts:109,174; subagent-stream.ts:75) — standard pattern, SSE streams contain non-JSON lines
- Demo chunk size warning (828KB) — Ant Design X bundle size, expected
