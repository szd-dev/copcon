# Error Handling

Complete reference for error codes, error response formats, and recommended handling patterns for the CopCon API.

---

## Error Response Format

All error responses return JSON with an `error` field containing a human-readable message:

```json
{
  "error": "session not found"
}
```

Some endpoints may return additional context in the error string:

```json
{
  "error": "tool call not found: call_abc123"
}
```

There is no structured error code in the HTTP response body. Use the HTTP status code to determine the error category.

---

## HTTP Status Codes

### 400 Bad Request

The request is malformed or missing required fields.

| Trigger | Example |
|---------|---------|
| Invalid JSON body | `{"error": "invalid request body"}` |
| Invalid session ID format | `{"error": "invalid session id"}` |
| Missing required fields | `{"error": "invalid request"}` |
| Empty content in chat | `{"error": "content is required"}` (from core, returned as SSE) |

### 404 Not Found

The requested resource does not exist.

| Trigger | Example |
|---------|---------|
| Session not found | `{"error": "session not found"}` |
| No active agent | `{"error": "no active agent for this session"}` |
| Interrupt not found | `{"error": "interrupt not found"}` |

### 409 Conflict

The request conflicts with the current state.

| Trigger | Example |
|---------|---------|
| Resume on inactive session | `{"error": "no active agent for this session"}` |

### 500 Internal Server Error

Unexpected server-side failures.

| Trigger | Example |
|---------|---------|
| Database errors | `{"error": "<database error message>"}` |
| Storage failures | `{"error": "failed to retrieve todos"}` |

---

## SSE Error Events

Errors during the agent loop are delivered as SSE events rather than HTTP error responses, since the HTTP stream is already open.

```json
{
  "type": "error",
  "data": {
    "error": "step limit exceeded"
  }
}
```

### Common Agent Error Messages

| Error Message | Cause |
|---------------|-------|
| `step limit exceeded` | Agent loop ran for 50 steps without finishing |
| `max subagent depth exceeded` | Nested delegation exceeded 3 levels |
| `session not found` | Session ID does not exist in the database |
| `invalid session ID` | UUID parse failure |
| `no agent specified and no default agent` | No default agent configured |
| `stream error: <detail>` | LLM provider returned an error |
| `build context: <detail>` | Failed to assemble context for LLM call |
| `session closed while waiting for input` | Session was stopped while a HITL interrupt was pending |
| `events_lost` | Reconnection failed, events evicted from buffer |

---

## Core Library Errors

When using the Go library directly, errors follow Go conventions with wrapped errors.

### Engine Errors

```go
// Max subagent depth
fmt.Errorf("max subagent depth exceeded")

// Session lookup
fmt.Errorf("invalid session ID: %w", err)
fmt.Errorf("get session: %w", err)

// Agent resolution
fmt.Errorf("no agent specified and no default agent: %w", err)
fmt.Errorf("get agent: %w", err)

// Message persistence
fmt.Errorf("add user message: %w", err)
fmt.Errorf("add assistant message: %w", err)
fmt.Errorf("update assistant message: %w", err)

// LLM
fmt.Errorf("stream error: %w", err)
fmt.Errorf("build context: %w", err)
```

### Registry Errors

```go
var ErrAgentNotFound  = errors.New("agent not found")
var ErrNoDefaultAgent = errors.New("no default agent configured")
```

### Tool Errors

```go
var ErrToolNotFound      = errors.New("tool not found")
var ErrToolAlreadyExists = errors.New("tool already exists")
```

### Interrupt Errors

```go
var ErrInterruptNotFound = errors.New("interrupt not found")
```

---

## Error Categories

### Transient Errors

These errors may resolve on retry:

| Error | Retry Strategy |
|-------|---------------|
| LLM provider timeouts | Exponential backoff, 3 retries |
| Database connection errors | Retry with backoff |
| `session already has an active agent` | Wait and retry, or stop first |

### Permanent Errors

These errors will not resolve on retry:

| Error | Action |
|-------|--------|
| `invalid session id` | Fix the request parameter |
| `session not found` | Create a new session |
| `agent not found` | Check agent ID against `/api/agents` |
| `interrupt not found` | Interrupt may have already been resolved or timed out |

### User Errors

These require user intervention:

| Error | Action |
|-------|--------|
| `content is required` | Provide message content |
| `invalid request body` | Fix JSON format |
| `step limit exceeded` | Simplify the request or increase `maxSteps` |

---

## Retry Strategies

### HTTP Requests

For transient HTTP errors (5xx, network timeouts):

```python
import time
import requests

def request_with_retry(method, url, max_retries=3, **kwargs):
    for attempt in range(max_retries):
        try:
            resp = requests.request(method, url, **kwargs)
            if resp.status_code < 500:
                return resp
        except requests.ConnectionError:
            pass
        wait = (2 ** attempt) + 0.5
        time.sleep(wait)
    raise Exception(f"Request failed after {max_retries} retries")
```

### SSE Stream Reconnection

```python
def stream_with_reconnect(client, session_id, message, last_seq=0):
    max_attempts = 5
    for attempt in range(max_attempts):
        try:
            if last_seq > 0:
                # Reconnect from last known sequence
                events = client.chat(session_id, "",
                    reconnect=True, last_event_seq=last_seq)
            else:
                events = client.chat(session_id, message)

            for event in events:
                # Track sequence for reconnection
                last_seq += 1
                yield event
                if event["type"] == "message_done":
                    return

        except (requests.ConnectionError, requests.Timeout):
            wait = min((2 ** attempt), 10)
            time.sleep(wait)
            continue

    raise Exception("Stream reconnection failed")
```

### Go Library Error Handling

```go
func chatWithRetry(ctx context.Context, engine agent.AgentEngine, chatCtx iface.ChatContextInterface, message string, maxRetries int) error {
    var lastErr error
    for i := 0; i < maxRetries; i++ {
        if err := ctx.Err(); err != nil {
            return err
        }
        err := engine.Chat(chatCtx, message)
        if err == nil {
            return nil
        }
        lastErr = err
        time.Sleep(time.Duration(math.Pow(2, float64(i))) * time.Second)
    }
    return fmt.Errorf("chat failed after %d retries: %w", maxRetries, lastErr)
}
```

---

## Circuit Breaker Pattern

For high-throughput integrations, wrap the CopCon client in a circuit breaker:

```python
import time
from enum import Enum

class CircuitState(Enum):
    CLOSED = "closed"       # Normal operation
    OPEN = "open"           # Failing, reject requests
    HALF_OPEN = "half_open" # Testing if service recovered

class CircuitBreaker:
    def __init__(self, failure_threshold=5, recovery_timeout=30):
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self.failure_count = 0
        self.last_failure_time = 0
        self.state = CircuitState.CLOSED

    def call(self, func, *args, **kwargs):
        if self.state == CircuitState.OPEN:
            if time.time() - self.last_failure_time > self.recovery_timeout:
                self.state = CircuitState.HALF_OPEN
            else:
                raise Exception("Circuit breaker is open")

        try:
            result = func(*args, **kwargs)
            self._on_success()
            return result
        except Exception as e:
            self._on_failure()
            raise

    def _on_success(self):
        self.failure_count = 0
        self.state = CircuitState.CLOSED

    def _on_failure(self):
        self.failure_count += 1
        self.last_failure_time = time.time()
        if self.failure_count >= self.failure_threshold:
            self.state = CircuitState.OPEN
```

---

## Logging Errors

### Server-Side

The CopCon server logs errors using structured logging (`slog`). Key log patterns:

| Log Message | Level | Meaning |
|-------------|-------|---------|
| `llm_stream_error` | ERROR | LLM provider stream failed |
| `incremental_persist_delta_failed` | WARN | Checkpoint save failed (non-fatal) |
| `incremental_persist_done_failed` | WARN | Part completion save failed |
| `hook panicked` | ERROR | Hook caused a panic (recovered) |
| `hook returned error` | WARN | Hook returned non-nil error |
| `harness: failed to create cross-agent tool` | WARN | Cross-agent tool registration failed |

### Client-Side

Recommended logging approach:

```python
import logging

logger = logging.getLogger("copcon")

def handle_event(event):
    event_type = event.get("type")
    if event_type == "error":
        error_msg = event["data"].get("error", "unknown")
        logger.error("Agent error: %s", error_msg)
    elif event_type == "part_update":
        data = event["data"]
        if data.get("state") == "error":
            logger.error("Tool error: %s", data.get("error"))
        elif data.get("state") == "waiting_for_input":
            logger.info("HITL interrupt: %s", data.get("interrupt", {}).get("message"))
```

---

## Common Error Scenarios

### Session Already Active

```
POST /api/sessions/{id}/chat → Error in SSE stream
{"type":"error","data":{"error":"session already has an active agent"}}
```

**Fix:** Call `POST /api/sessions/{id}/stop` first, then retry.

### Stale Interrupt ID

```
POST /api/sessions/{id}/resume → 404
{"error":"interrupt not found"}
```

**Fix:** The interrupt was already resolved or the agent session ended. Check `GET /api/sessions/{id}/messages` for the current state.

### Events Lost on Reconnect

```
POST /api/sessions/{id}/chat (reconnect) → SSE stream
{"type":"events_lost"}
```

**Fix:** Reload full message history via `GET /api/sessions/{id}/messages` and resume from there.

### LLM Provider Errors

```
SSE stream → {"type":"error","data":{"error":"stream error: context length exceeded"}}
```

**Fix:** This comes from the LLM provider. Options:
- Use a model with larger context window
- Clear conversation history by creating a new session
- Reduce the complexity of the request
