# HTTP REST API Reference

Complete reference for the CopCon Server HTTP API. The server exposes a RESTful API on top of the core engine, backed by Gin.

## Base Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| Base URL | `http://localhost:8080` | Configurable via `config.yaml` |
| Content-Type | `application/json` | For all non-streaming endpoints |
| Auth | Optional | See [Authentication](./authentication.md) |

## Health Check

```
GET /health
```

Returns server status.

**Response:**

```json
{
  "status": "ok"
}
```

---

## Agents

### List Agents

```
GET /api/agents
```

Returns all registered agents.

**Response:** `200 OK`

```json
{
  "agents": [
    {
      "id": "code-assistant",
      "name": "Code Assistant",
      "model": "gpt-4o"
    },
    {
      "id": "reviewer",
      "name": "Code Reviewer",
      "model": "gpt-4o"
    }
  ]
}
```

**cURL:**

```bash
curl http://localhost:8080/api/agents
```

---

## Sessions

### Create Session

```
POST /api/sessions
```

Creates a new conversation session.

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| title | string | No | Session title. Defaults to "New Chat" |
| default_agent_id | string | No | Agent to use. Falls back to config default |

```json
{
  "title": "Code Review Session",
  "default_agent_id": "code-assistant"
}
```

The request body is optional. An empty body creates a session with defaults.

**Response:** `201 Created`

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "title": "Code Review Session",
  "default_agent_id": "code-assistant",
  "created_at": "2026-05-25T10:30:00Z",
  "updated_at": "2026-05-25T10:30:00Z",
  "message_count": 0
}
```

**cURL:**

```bash
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"title": "Code Review Session", "default_agent_id": "code-assistant"}'
```

---

### List Sessions

```
GET /api/sessions
```

Returns a paginated list of sessions.

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| limit | integer | 20 | Max sessions to return |
| offset | integer | 0 | Pagination offset |

**Response:** `200 OK`

```json
{
  "sessions": [
    {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "title": "Code Review Session",
      "created_at": "2026-05-25T10:30:00Z",
      "updated_at": "2026-05-25T10:30:00Z",
      "message_count": 5
    }
  ],
  "total": 42
}
```

**cURL:**

```bash
curl "http://localhost:8080/api/sessions?limit=10&offset=0"
```

---

### Get Session

```
GET /api/sessions/:sessionId
```

Returns details for a specific session.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| sessionId | string (UUID) | Session identifier |

**Response:** `200 OK`

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "title": "Code Review Session",
  "created_at": "2026-05-25T10:30:00Z",
  "updated_at": "2026-05-25T10:35:00Z",
  "message_count": 5
}
```

**Error Responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid session id"}` |
| 404 | `{"error": "session not found"}` |

**cURL:**

```bash
curl http://localhost:8080/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

---

### Delete Session

```
DELETE /api/sessions/:sessionId
```

Deletes a session and all associated messages.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| sessionId | string (UUID) | Session identifier |

**Response:** `204 No Content`

**Error Responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid session id"}` |
| 404 | `{"error": "session not found"}` |

**cURL:**

```bash
curl -X DELETE http://localhost:8080/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

---

### Get Session Messages

```
GET /api/sessions/:sessionId/messages
```

Returns messages for a session, grouped by steps with rich part data.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| sessionId | string (UUID) | Session identifier |

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| limit | integer | 50 | Max messages to return |

**Response:** `200 OK`

```json
{
  "messages": [
    {
      "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
      "sessionId": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "role": "user",
      "steps": [
        {
          "parts": [
            {
              "type": "text",
              "text": "Write a hello world function in Python",
              "state": "done",
              "stepIndex": 0
            }
          ],
          "state": "done"
        }
      ],
      "metadata": {
        "createdAt": "2026-05-25T10:30:00Z",
        "model": "",
        "tokenCount": 0,
        "durationMs": 0
      }
    },
    {
      "id": "c3d4e5f6-a7b8-9012-cdef-123456789012",
      "sessionId": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "role": "assistant",
      "steps": [
        {
          "parts": [
            {
              "type": "reasoning",
              "text": "The user wants a simple Python function...",
              "state": "done",
              "stepIndex": 0
            },
            {
              "type": "text",
              "text": "Here's a hello world function:\n\n```python\ndef hello():\n    print('Hello, World!')\n```",
              "state": "done",
              "stepIndex": 0
            }
          ],
          "state": "done"
        }
      ],
      "metadata": {
        "createdAt": "2026-05-25T10:30:05Z",
        "model": "gpt-4o",
        "tokenCount": 150,
        "durationMs": 2300
      }
    }
  ]
}
```

Tool messages (`role: "tool"`) are not returned directly. Their content is embedded in the corresponding `tool-call` part's `output` field.

For messages without `Parts` data (legacy), the server backfills parts from `Content` and `ToolCalls` fields.

**Error Responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid session id"}` |
| 500 | `{"error": "<database error>"}` |

**cURL:**

```bash
curl http://localhost:8080/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890/messages?limit=20
```

---

### Chat (SSE Stream)

```
POST /api/sessions/:sessionId/chat
```

Sends a message and receives a streaming response via Server-Sent Events.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| sessionId | string (UUID) | Session identifier |

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| content | string | Yes | User message text |
| agent_id | string | No | Override the session's default agent |
| reconnect | boolean | No | Reconnect to an active stream. Default: false |
| last_event_seq | integer | No | Last received sequence number (for reconnection) |

```json
{
  "content": "Write a Python function that sorts a list"
}
```

**Response Headers:**

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

**Response:** `200 OK` (SSE stream)

Each SSE line follows the format:

```
data: <JSON-encoded Event>\n\n
```

See the [WebSocket/SSE API](./websocket-api.md) for the complete event schema.

**Error Responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid request"}` |
| 500 | `{"error": "streaming not supported"}` |

**cURL:**

```bash
curl -N -X POST http://localhost:8080/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890/chat \
  -H "Content-Type: application/json" \
  -d '{"content": "Write a hello world function"}'
```

**Reconnection:**

```bash
curl -N -X POST http://localhost:8080/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890/chat \
  -H "Content-Type: application/json" \
  -d '{"content": "", "reconnect": true, "last_event_seq": 42}'
```

When reconnecting, set `content` to empty string, `reconnect` to `true`, and `last_event_seq` to the last sequence number you received. If the sequence is still within the ring buffer (1024 events), you receive all events from that point forward. If evicted, you receive a `{"type": "events_lost"}` event.

---

### Stop Session

```
POST /api/sessions/:sessionId/stop
```

Stops an active agent execution for a session.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| sessionId | string (UUID) | Session identifier |

**Request Body:** None

**Response:** `204 No Content`

**Error Responses:**

| Status | Body |
|--------|------|
| 404 | `{"error": "no active agent for this session"}` |

**cURL:**

```bash
curl -X POST http://localhost:8080/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890/stop
```

---

### Resume Session (HITL)

```
POST /api/sessions/:sessionId/resume
```

Resolves a human-in-the-loop interrupt. Use this when an agent has paused execution waiting for user approval or input.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| sessionId | string (UUID) | Session identifier |

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| interrupt_id | string | Yes | The interrupt ID from the `waiting_for_input` event |
| action | string | Yes | User action (e.g., "approve", "reject", "answer") |
| content | object | No | Additional response data |

```json
{
  "interrupt_id": "abc-123-def",
  "action": "approve",
  "content": {
    "reason": "Looks good"
  }
}
```

**Response:** `200 OK`

```json
{
  "status": "resolved"
}
```

**Error Responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid request body"}` |
| 404 | `{"error": "interrupt not found"}` |
| 409 | `{"error": "no active agent for this session"}` |

**cURL:**

```bash
curl -X POST http://localhost:8080/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890/resume \
  -H "Content-Type: application/json" \
  -d '{"interrupt_id": "abc-123-def", "action": "approve"}'
```

---

### Get Session Todos

```
GET /api/sessions/:sessionId/todos
```

Returns all todo items for a session.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| sessionId | string (UUID) | Session identifier |

**Response:** `200 OK`

```json
{
  "todos": [
    {
      "id": "d4e5f6a7-b8c9-0123-defa-234567890123",
      "session_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "content": "Implement user authentication",
      "active_form": "Implementing user authentication",
      "status": "in_progress",
      "priority": "high",
      "depends_on": [],
      "validation": "Auth flow works end-to-end",
      "result": "",
      "retry_count": 0,
      "completed_at": null,
      "created_at": "2026-05-25T10:30:00Z",
      "updated_at": "2026-05-25T10:35:00Z"
    }
  ]
}
```

**Error Responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid session id"}` |
| 404 | `{"error": "session not found"}` |
| 500 | `{"error": "failed to retrieve todos"}` |

**cURL:**

```bash
curl http://localhost:8080/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890/todos
```

---

### Get Session Updates

```
GET /api/sessions/:sessionId/updates
```

Polls for async tool completion events. Use this to check if background tool executions have finished.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| sessionId | string (UUID) | Session identifier |

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| since | string | Only return events with IDs after this value |

**Response:** `200 OK`

```json
{
  "has_updates": true,
  "events": [
    {
      "id": "evt_001",
      "type": "async_tool_complete",
      "call_id": "call_abc123",
      "tool_name": "code_executor",
      "session_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "completed_at": "2026-05-25T10:35:00Z"
    }
  ]
}
```

**Error Responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid session id"}` |
| 404 | `{"error": "session not found"}` |

**cURL:**

```bash
curl "http://localhost:8080/api/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890/updates?since=evt_000"
```

---

## Complete Route Table

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/api/agents` | List all agents |
| POST | `/api/sessions` | Create session |
| GET | `/api/sessions` | List sessions |
| GET | `/api/sessions/:sessionId` | Get session details |
| DELETE | `/api/sessions/:sessionId` | Delete session |
| GET | `/api/sessions/:sessionId/messages` | Get session messages |
| POST | `/api/sessions/:sessionId/chat` | Send message (SSE stream) |
| POST | `/api/sessions/:sessionId/stop` | Stop active agent |
| POST | `/api/sessions/:sessionId/resume` | Resolve HITL interrupt |
| GET | `/api/sessions/:sessionId/todos` | Get session todos |
| GET | `/api/sessions/:sessionId/updates` | Poll async tool completions |

---

## Common Patterns

### Full Chat Flow

```bash
# 1. Create a session
SESSION=$(curl -s -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"title": "My Chat"}')
SESSION_ID=$(echo $SESSION | jq -r '.id')

# 2. Send a message and stream the response
curl -N -X POST "http://localhost:8080/api/sessions/$SESSION_ID/chat" \
  -H "Content-Type: application/json" \
  -d '{"content": "Hello, can you help me?"}'

# 3. Get message history
curl "http://localhost:8080/api/sessions/$SESSION_ID/messages"

# 4. Clean up
curl -X DELETE "http://localhost:8080/api/sessions/$SESSION_ID"
```

### HITL Approval Flow

```bash
# 1. Start a chat that triggers a confirm_action tool
curl -N -X POST "http://localhost:8080/api/sessions/$SESSION_ID/chat" \
  -H "Content-Type: application/json" \
  -d '{"content": "Delete all temp files"}'

# The stream emits a part_update with state "waiting_for_input"
# containing an interrupt object:
# {
#   "interruptId": "abc-123",
#   "interruptType": "approval",
#   "message": "Are you sure you want to delete all temp files?",
#   "summary": "Delete temp files",
#   "inputSchema": { ... }
# }

# 2. Approve the action
curl -X POST "http://localhost:8080/api/sessions/$SESSION_ID/resume" \
  -H "Content-Type: application/json" \
  -d '{"interrupt_id": "abc-123", "action": "approve"}'

# The original SSE stream resumes with the tool result
```

### Async Tool Polling

```bash
# 1. Start a chat with an async tool
curl -N -X POST "http://localhost:8080/api/sessions/$SESSION_ID/chat" \
  -H "Content-Type: application/json" \
  -d '{"content": "Run the long computation in background"}'

# The stream emits async_tool_started, then message_done
# The tool runs in background

# 2. Poll for completion
curl "http://localhost:8080/api/sessions/$SESSION_ID/updates"
# {"has_updates": true, "events": [...]}
```
