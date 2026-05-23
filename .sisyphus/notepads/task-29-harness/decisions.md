# Task 29 Decisions

## Harness uses noopSessionManager/noopContextManager internally
- The Harness builds an AgentEngine but doesn't wire real session/context managers
- This is intentional: Harness only creates the wiring structure; the consuming app (e.g., server) provides real implementations via adapters
- The Engine can be replaced or reconfigured later with proper managers

## capToToolName map passed through factory closures
- The capability→tool name mapping is built once during Build() and captured in the agent factory closure
- This avoids re-resolving capability names on every factory call

## Cross-agent tools always registered if available
- delegate_to and read_sub_session are always registered if their capabilities exist in the global registry
- Not gated by explicit inclusion in any agent's Tools list — they're always available in the ToolRegistry
