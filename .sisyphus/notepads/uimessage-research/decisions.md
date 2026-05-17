
## UIMessage Research Decisions (2026-04-19)

### Decision: Use UIMessage-style parts[] for CopCon ChatContext events
- **Rationale**: The parts[] pattern with in-place state mutation is superior for frontend rendering
- **Alternative considered**: Flat event list with separate tool-result events (like ModelMessage)
- **Why UIMessage wins**: Single source of truth per UI element, streaming state transitions, step boundaries

### Decision: Adopt the 7-state ToolUIPart state machine
- **Rationale**: Vercel's design covers all tool lifecycle phases including approval flows
- **For CopCon**: May simplify to 4 states initially (input-streaming, input-available, output-available, output-error), add approval states later

### Decision: step-start as boundary marker pattern
- **Rationale**: Enables multi-step assistant messages in a single UIMessage while maintaining correct ModelMessage conversion
- **For CopCon**: Our ChatContext.Emit() should support step-start events to divide agent loop iterations
