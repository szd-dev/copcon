# SSE Streaming API

CopCon uses Server-Sent Events (SSE) for real-time streaming of agent responses. There is no WebSocket or gRPC streaming endpoint. SSE provides simpler integration, better HTTP compatibility, and native browser support without requiring connection state management.

## Why SSE, Not WebSocket

- SSE is a pure HTTP protocol. No upgrade handshake, no protocol switching
- Native browser support via the `EventSource` API
- Works through standard HTTP infrastructure (proxies, load balancers, CDN)
- Unidirectional (server to client) matches the chat streaming use case
- Automatic browser reconnection with `Last-Event-ID` header

## Connection Establishment

### New Chat

```
POST /api/sessions/:sessionId/chat
Content-Type: application/json

{
  "content": "Your message here",
  "agent_id": "optional-agent-id"
}
```

The server responds with `Content-Type: text/event-stream` and begins streaming events.

### Reconnection

If the SSE connection drops, reconnect by posting the same endpoint with reconnection parameters:

```
POST /api/sessions/:sessionId/chat
Content-Type: application/json

{
  "content": "",
  "reconnect": true,
  "last_event_seq": 42
}
```

The server checks the ring buffer (1024 events). If `last_event_seq` is still within the buffer window, all events from that sequence onward are replayed. If the sequence has been evicted, a single `events_lost` event is sent.

### Active Session Guard

Only one active agent loop can run per session at a time. If you try to start a new chat while an agent is still running, the server returns:

```
data: {"type":"error","data":{"error":"session already has an active agent"}}
```

Use the `POST /api/sessions/:sessionId/stop` endpoint to cancel an active agent before starting a new one.

---

## Event Format

All events follow the SSE data-only format (no `event:` field, only `data:`):

```
data: <JSON>\n\n
```

Each JSON payload is an `Event` structure:

```json
{
  "type": "<event_type>",
  "data": { ... }
}
```

---

## Event Types

### Current Step/Part Model

These are the primary event types you should handle in new integrations.

#### `step_create`

A new reasoning/tool-call step begins. Multi-step responses have one `step_create` per iteration of the agent loop.

```json
{
  "type": "step_create",
  "data": {
    "messageId": "uuid-of-the-assistant-message",
    "stepIndex": 1
  }
}
```

#### `part_create`

A new content part is created within a step. Parts can be `text`, `reasoning`, or `tool-call`.

```json
{
  "type": "part_create",
  "data": {
    "messageId": "uuid-of-the-assistant-message",
    "stepIndex": 0,
    "partIndex": 0,
    "partType": "text",
    "state": "streaming",
    "toolCallId": "",
    "toolName": "",
    "args": ""
  }
}
```

For `tool-call` parts:

```json
{
  "type": "part_create",
  "data": {
    "messageId": "uuid-of-the-assistant-message",
    "stepIndex": 0,
    "partIndex": 1,
    "partType": "tool-call",
    "state": "pending",
    "toolCallId": "call_abc123",
    "toolName": "code_executor",
    "args": "{\"language\":\"python\",\"code\":\"print('hello')\"}"
  }
}
```

#### `part_update`

Incremental update to an existing part. For text/reasoning parts, `textDelta` contains the new text fragment. For tool-call parts, `output`, `error`, or `state` changes indicate execution progress.

**Text delta:**

```json
{
  "type": "part_update",
  "data": {
    "messageId": "uuid-of-the-assistant-message",
    "stepIndex": 0,
    "partIndex": 0,
    "partType": "text",
    "textDelta": "Here is a"
  }
}
```

**Reasoning delta:**

```json
{
  "type": "part_update",
  "data": {
    "messageId": "uuid-of-the-assistant-message",
    "stepIndex": 0,
    "partIndex": 0,
    "partType": "reasoning",
    "textDelta": "The user wants..."
  }
}
```

**Part completion:**

```json
{
  "type": "part_update",
  "data": {
    "messageId": "uuid-of-the-assistant-message",
    "stepIndex": 0,
    "partIndex": 0,
    "partType": "text",
    "state": "done"
  }
}
```

**Tool result:**

```json
{
  "type": "part_update",
  "data": {
    "messageId": "uuid-of-the-assistant-message",
    "stepIndex": 0,
    "partIndex": 1,
    "partType": "tool-call",
    "state": "complete",
    "output": "hello\n"
  }
}
```

**Tool error:**

```json
{
  "type": "part_update",
  "data": {
    "messageId": "uuid-of-the-assistant-message",
    "stepIndex": 0,
    "partIndex": 1,
    "partType": "tool-call",
    "state": "error",
    "error": "execution timed out"
  }
}
```

**HITL interrupt (waiting for input):**

```json
{
  "type": "part_update",
  "data": {
    "messageId": "uuid-of-the-assistant-message",
    "stepIndex": 0,
    "partIndex": 1,
    "partType": "tool-call",
    "state": "waiting_for_input",
    "interrupt": {
      "interruptId": "abc-123-def",
      "interruptType": "approval",
      "message": "Are you sure you want to delete these files?",
      "summary": "Delete 5 temp files",
      "inputSchema": {
        "type": "object",
        "properties": {
          "reason": { "type": "string" }
        }
      }
    }
  }
}
```

#### `message_done`

The entire assistant message is complete. No more events for this message will follow.

```json
{
  "type": "message_done",
  "data": {
    "messageId": "uuid-of-the-assistant-message"
  }
}
```

#### `error`

An error occurred during the agent loop. The stream may or may not continue after an error event.

```json
{
  "type": "error",
  "data": {
    "error": "step limit exceeded"
  }
}
```

### Async Tool Events

#### `async_tool_started`

A tool is executing in the background (async mode). The main stream continues, and this event signals the start of background execution.

```json
{
  "type": "async_tool_started",
  "data": {
    "message_id": "uuid-of-the-assistant-message",
    "call_id": "call_abc123",
    "tool_name": "code_executor",
    "session_id": "session-uuid"
  }
}
```

#### `async_tool_complete`

An async tool finished successfully.

```json
{
  "type": "async_tool_complete",
  "data": {
    "message_id": "uuid-of-the-assistant-message",
    "call_id": "call_abc123",
    "tool_name": "code_executor",
    "result": { "stdout": "hello\n", "exitCode": 0 },
    "duration_ms": 3500
  }
}
```

#### `async_tool_failed`

An async tool failed.

```json
{
  "type": "async_tool_failed",
  "data": {
    "message_id": "uuid-of-the-assistant-message",
    "call_id": "call_abc123",
    "tool_name": "code_executor",
    "error": "execution timed out after 30s",
    "duration_ms": 30000
  }
}
```

### Legacy Events (Deprecated)

These event types are retained for backward compatibility but should not be used in new integrations. They may be removed in a future version.

| Legacy Event | Replaced By |
|-------------|-------------|
| `message` | `part_create` + `part_update` with `partType: "text"` |
| `reasoning` | `part_create` with `partType: "reasoning"` |
| `tool_call` | `part_create` with `partType: "tool-call"` |
| `tool_result` | `part_update` with `output` field |
| `done` | `message_done` |
| `thought` | Never emitted, removed |

---

## Part State Lifecycle

```
streaming → done        (text/reasoning completed)
pending   → running     (tool execution started)
running   → complete    (tool execution succeeded)
running   → error       (tool execution failed)
running   → waiting_for_input  (HITL interrupt)
waiting_for_input → complete/error (after user response)
```

---

## Typical Stream Sequence

A simple text response:

```
data: {"type":"step_create","data":{"messageId":"msg-1","stepIndex":0}}
data: {"type":"part_create","data":{"messageId":"msg-1","stepIndex":0,"partIndex":0,"partType":"text","state":"streaming"}}
data: {"type":"part_update","data":{"messageId":"msg-1","stepIndex":0,"partIndex":0,"partType":"text","textDelta":"Hello"}}
data: {"type":"part_update","data":{"messageId":"msg-1","stepIndex":0,"partIndex":0,"partType":"text","textDelta":" world"}}
data: {"type":"part_update","data":{"messageId":"msg-1","stepIndex":0,"partIndex":0,"partType":"text","state":"done"}}
data: {"type":"message_done","data":{"messageId":"msg-1"}}
```

A response with tool calls (multi-step):

```
data: {"type":"step_create","data":{"messageId":"msg-2","stepIndex":0}}
data: {"type":"part_create","data":{"messageId":"msg-2","stepIndex":0,"partIndex":0,"partType":"reasoning","state":"streaming"}}
data: {"type":"part_update","data":{"messageId":"msg-2","stepIndex":0,"partIndex":0,"partType":"reasoning","textDelta":"I should run this code"}}
data: {"type":"part_update","data":{"messageId":"msg-2","stepIndex":0,"partIndex":0,"partType":"reasoning","state":"done"}}
data: {"type":"part_create","data":{"messageId":"msg-2","stepIndex":0,"partIndex":1,"partType":"text","state":"streaming"}}
data: {"type":"part_update","data":{"messageId":"msg-2","stepIndex":0,"partIndex":1,"partType":"text","textDelta":"Let me check"}}
data: {"type":"part_update","data":{"messageId":"msg-2","stepIndex":0,"partIndex":1,"partType":"text","state":"done"}}
data: {"type":"part_create","data":{"messageId":"msg-2","stepIndex":0,"partIndex":2,"partType":"tool-call","state":"pending","toolCallId":"call-1","toolName":"code_executor","args":"{...}"}}
data: {"type":"part_update","data":{"messageId":"msg-2","stepIndex":0,"partIndex":2,"partType":"tool-call","state":"running"}}
data: {"type":"part_update","data":{"messageId":"msg-2","stepIndex":0,"partIndex":2,"partType":"tool-call","state":"complete","output":"hello\n"}}
data: {"type":"step_create","data":{"messageId":"msg-2","stepIndex":1}}
data: {"type":"part_create","data":{"messageId":"msg-2","stepIndex":1,"partIndex":0,"partType":"text","state":"streaming"}}
data: {"type":"part_update","data":{"messageId":"msg-2","stepIndex":1,"partIndex":0,"partType":"text","textDelta":"The output is"}}
data: {"type":"part_update","data":{"messageId":"msg-2","stepIndex":1,"partIndex":0,"partType":"text","textDelta":" hello"}}
data: {"type":"part_update","data":{"messageId":"msg-2","stepIndex":1,"partIndex":0,"partType":"text","state":"done"}}
data: {"type":"message_done","data":{"messageId":"msg-2"}}
```

---

## Client Implementation

### JavaScript (fetch)

```javascript
async function streamChat(sessionId, message) {
  const response = await fetch(`/api/sessions/${sessionId}/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: message })
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n\n');
    buffer = lines.pop() || '';

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue;
      const event = JSON.parse(line.slice(6));
      handleEvent(event);
    }
  }
}

function handleEvent(event) {
  switch (event.type) {
    case 'part_create':
      console.log(`New ${event.data.partType} part`);
      break;
    case 'part_update':
      if (event.data.textDelta) {
        process.stdout.write(event.data.textDelta);
      }
      if (event.data.state === 'waiting_for_input') {
        showInterruptPrompt(event.data.interrupt);
      }
      break;
    case 'message_done':
      console.log('\nMessage complete');
      break;
    case 'error':
      console.error('Error:', event.data.error);
      break;
  }
}
```

### Python (requests)

```python
import json
import requests

def stream_chat(session_id, message):
    url = f"http://localhost:8080/api/sessions/{session_id}/chat"
    data = {"content": message}
    response = requests.post(url, json=data, stream=True)

    buffer = ""
    for chunk in response.iter_content(chunk_size=None):
        buffer += chunk.decode("utf-8")
        while "\n\n" in buffer:
            line, buffer = buffer.split("\n\n", 1)
            if line.startswith("data: "):
                event = json.loads(line[6:])
                handle_event(event)

def handle_event(event):
    if event["type"] == "part_update":
        delta = event["data"].get("textDelta", "")
        if delta:
            print(delta, end="", flush=True)
    elif event["type"] == "message_done":
        print("\nDone.")
    elif event["type"] == "error":
        print(f"\nError: {event['data']['error']}")
```

### Go (net/http)

```go
func streamChat(ctx context.Context, sessionID, message string) error {
    body := fmt.Sprintf(`{"content":"%s"}`, message)
    req, _ := http.NewRequestWithContext(ctx, "POST",
        fmt.Sprintf("http://localhost:8080/api/sessions/%s/chat", sessionID),
        strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        line := scanner.Text()
        if !strings.HasPrefix(line, "data: ") {
            continue
        }
        var event entity.Event
        if err := json.Unmarshal([]byte(line[6:]), &event); err != nil {
            continue
        }
        // Process event...
    }
    return scanner.Err()
}
```

---

## Reconnection Strategy

1. Store the last event sequence number locally
2. On connection drop, wait 1-3 seconds
3. Reconnect with `reconnect: true` and `last_event_seq`
4. If you receive `events_lost`, reload the full message history via `GET /api/sessions/:sessionId/messages`
5. Continue streaming from the reconnect response

The ring buffer holds 1024 events. For sessions with long agent loops (many tool calls), events older than the buffer window are evicted. Use the messages endpoint as a fallback for full history recovery.

---

## Handling HITL Interrupts

When a `part_update` arrives with `state: "waiting_for_input"`:

1. Display the interrupt prompt to the user
2. Collect their response
3. POST to `POST /api/sessions/:sessionId/resume` with `interrupt_id`, `action`, and optional `content`
4. The original SSE stream resumes with the tool result

The `interrupt` object in the event data contains:

| Field | Type | Description |
|-------|------|-------------|
| `interruptId` | string | ID to use in the resume request |
| `interruptType` | string | "approval" or "question" |
| `message` | string | Prompt text shown to user |
| `summary` | string | Short summary of what's being asked |
| `inputSchema` | object | JSON Schema for the expected response format |

---

## Handling Async Tools

When a `part_update` arrives with `state: "complete"` for an async tool, the main stream may end before the tool finishes. Use `GET /api/sessions/:sessionId/updates` to poll for completion.

Alternatively, the async tool events (`async_tool_started`, `async_tool_complete`, `async_tool_failed`) are emitted during the stream if the tool completes before the stream ends.