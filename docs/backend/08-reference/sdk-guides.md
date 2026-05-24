# SDK Integration Guides

Practical guides for integrating with CopCon from different languages and environments. CopCon does not ship native SDKs, but its HTTP + SSE API is straightforward to consume from any language.

---

## Go (Library Usage)

The most direct way to use CopCon from Go is embedding the core library. See the [Go API Reference](./go-api.md) for the complete package documentation.

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"

    "github.com/copcon/core"
    "github.com/copcon/core/llm"
    pgstore "github.com/copcon/core/providers/postgres"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "github.com/openai/openai-go/v3"
    "github.com/openai/openai-go/v3/option"
)

func main() {
    log := slog.New(slog.NewTextHandler(os.Stderr, nil))
    db, _ := gorm.Open(postgres.Open("host=localhost port=5432 user=admin password=changeme dbname=copcon sslmode=disable"), &gorm.Config{})
    pg := pgstore.NewStore(db)

    cl := openai.NewClient(option.WithAPIKey(os.Getenv("OPENAI_API_KEY")))
    provider := llm.NewOpenAIAdapter(&cl, "gpt-4o")

    engine, _, err := core.NewAgent(core.AgentQuickConfig{
        Name:         "Assistant",
        Model:        "gpt-4o",
        SystemPrompt: "You are a helpful coding assistant.",
        Tools:        []string{"code_executor", "shell_executor"},
        LLM:          provider,
        SessionStore: pg.Sessions(),
        MessageStore: pg.Messages(),
    })
    if err != nil {
        log.Error("create agent", "error", err)
        os.Exit(1)
    }

    // Create a session via storage
    sess, _ := pg.Sessions().Create(context.Background(), &storage.Session{Title: "Test"})

    // Chat with streaming
    chatCtx := chatcontext.NewChatContext(context.Background(), sess.ID.String(), "")
    go func() {
        defer chatCtx.Close()
        engine.Chat(chatCtx, "Write a hello world in Go")
    }()

    for event := range chatCtx.Events() {
        if event.Type == entity.EventPartUpdate {
            data := event.Data.(entity.PartUpdateData)
            if data.TextDelta != "" {
                fmt.Print(data.TextDelta)
            }
        }
    }
}
```

### Go HTTP Client

For connecting to a running CopCon server:

```go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
)

func main() {
    sessionID := "your-session-uuid"
    body := `{"content":"Hello"}`
    req, _ := http.NewRequest("POST",
        "http://localhost:8080/api/sessions/"+sessionID+"/chat",
        strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    resp, _ := http.DefaultClient.Do(req)
    defer resp.Body.Close()

    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        line := scanner.Text()
        if !strings.HasPrefix(line, "data: ") {
            continue
        }
        var event map[string]any
        json.Unmarshal([]byte(line[6:]), &event)
        fmt.Printf("Event: %s\n", event["type"])
    }
}
```

---

## Python

### Basic Client

```python
import json
import uuid
import requests

BASE_URL = "http://localhost:8080"

class CopConClient:
    def __init__(self, base_url=BASE_URL):
        self.base_url = base_url

    def create_session(self, title="New Chat", agent_id=None):
        data = {"title": title}
        if agent_id:
            data["default_agent_id"] = agent_id
        resp = requests.post(f"{self.base_url}/api/sessions", json=data)
        resp.raise_for_status()
        return resp.json()

    def list_sessions(self, limit=20, offset=0):
        resp = requests.get(
            f"{self.base_url}/api/sessions",
            params={"limit": limit, "offset": offset}
        )
        resp.raise_for_status()
        return resp.json()

    def get_session(self, session_id):
        resp = requests.get(f"{self.base_url}/api/sessions/{session_id}")
        resp.raise_for_status()
        return resp.json()

    def delete_session(self, session_id):
        resp = requests.delete(f"{self.base_url}/api/sessions/{session_id}")
        resp.raise_for_status()
        return resp.status_code == 204

    def get_messages(self, session_id, limit=50):
        resp = requests.get(
            f"{self.base_url}/api/sessions/{session_id}/messages",
            params={"limit": limit}
        )
        resp.raise_for_status()
        return resp.json()

    def chat(self, session_id, message, on_event=None):
        """Stream chat events. Calls on_event(event) for each SSE event."""
        resp = requests.post(
            f"{self.base_url}/api/sessions/{session_id}/chat",
            json={"content": message},
            stream=True
        )
        resp.raise_for_status()

        buffer = ""
        for chunk in resp.iter_content(chunk_size=None):
            buffer += chunk.decode("utf-8")
            while "\n\n" in buffer:
                line, buffer = buffer.split("\n\n", 1)
                if line.startswith("data: "):
                    event = json.loads(line[6:])
                    if on_event:
                        on_event(event)
                    yield event

    def stop_session(self, session_id):
        resp = requests.post(f"{self.base_url}/api/sessions/{session_id}/stop")
        resp.raise_for_status()

    def resume(self, session_id, interrupt_id, action, content=None):
        data = {"interrupt_id": interrupt_id, "action": action}
        if content:
            data["content"] = content
        resp = requests.post(
            f"{self.base_url}/api/sessions/{session_id}/resume",
            json=data
        )
        resp.raise_for_status()
        return resp.json()

    def list_agents(self):
        resp = requests.get(f"{self.base_url}/api/agents")
        resp.raise_for_status()
        return resp.json()

    def get_todos(self, session_id):
        resp = requests.get(f"{self.base_url}/api/sessions/{session_id}/todos")
        resp.raise_for_status()
        return resp.json()

    def get_updates(self, session_id, since=None):
        params = {}
        if since:
            params["since"] = since
        resp = requests.get(
            f"{self.base_url}/api/sessions/{session_id}/updates",
            params=params
        )
        resp.raise_for_status()
        return resp.json()
```

### Usage Examples

```python
client = CopConClient()

# Create a session and chat
session = client.create_session("My Chat")
session_id = session["id"]

# Stream the response
for event in client.chat(session_id, "Write a Python fibonacci function"):
    if event["type"] == "part_update":
        delta = event["data"].get("textDelta", "")
        if delta:
            print(delta, end="", flush=True)
    elif event["type"] == "message_done":
        print("\n--- Done ---")

# Get history
messages = client.get_messages(session_id)
for msg in messages["messages"]:
    print(f"[{msg['role']}]")

# Clean up
client.delete_session(session_id)
```

### HITL Approval

```python
for event in client.chat(session_id, "Delete all temp files"):
    if event["type"] == "part_update":
        data = event["data"]
        if data.get("state") == "waiting_for_input":
            interrupt = data["interrupt"]
            print(f"Agent asks: {interrupt['message']}")
            # Auto-approve
            client.resume(session_id, interrupt["interruptId"], "approve")
```

---

## JavaScript / TypeScript

### Client Class

```typescript
interface SSEEvent {
  type: string;
  data: Record<string, any>;
}

interface Session {
  id: string;
  title: string;
  default_agent_id?: string;
  created_at: string;
  updated_at: string;
  message_count: number;
}

class CopConClient {
  private baseUrl: string;

  constructor(baseUrl = "http://localhost:8080") {
    this.baseUrl = baseUrl;
  }

  async createSession(title = "New Chat", agentId?: string): Promise<Session> {
    const body: Record<string, string> = { title };
    if (agentId) body.default_agent_id = agentId;
    const resp = await fetch(`${this.baseUrl}/api/sessions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return resp.json();
  }

  async listSessions(limit = 20, offset = 0): Promise<any> {
    const resp = await fetch(
      `${this.baseUrl}/api/sessions?limit=${limit}&offset=${offset}`
    );
    return resp.json();
  }

  async getSession(sessionId: string): Promise<Session> {
    const resp = await fetch(`${this.baseUrl}/api/sessions/${sessionId}`);
    return resp.json();
  }

  async deleteSession(sessionId: string): Promise<boolean> {
    const resp = await fetch(`${this.baseUrl}/api/sessions/${sessionId}`, {
      method: "DELETE",
    });
    return resp.status === 204;
  }

  async getMessages(sessionId: string, limit = 50): Promise<any> {
    const resp = await fetch(
      `${this.baseUrl}/api/sessions/${sessionId}/messages?limit=${limit}`
    );
    return resp.json();
  }

  async *chat(sessionId: string, message: string): AsyncGenerator<SSEEvent> {
    const resp = await fetch(`${this.baseUrl}/api/sessions/${sessionId}/chat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content: message }),
    });

    const reader = resp.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const parts = buffer.split("\n\n");
      buffer = parts.pop() || "";

      for (const part of parts) {
        const line = part.trim();
        if (!line.startsWith("data: ")) continue;
        const event: SSEEvent = JSON.parse(line.slice(6));
        yield event;
      }
    }
  }

  async stopSession(sessionId: string): Promise<void> {
    await fetch(`${this.baseUrl}/api/sessions/${sessionId}/stop`, {
      method: "POST",
    });
  }

  async resume(
    sessionId: string,
    interruptId: string,
    action: string,
    content?: Record<string, any>
  ): Promise<any> {
    const body: Record<string, any> = { interrupt_id: interruptId, action };
    if (content) body.content = content;
    const resp = await fetch(
      `${this.baseUrl}/api/sessions/${sessionId}/resume`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }
    );
    return resp.json();
  }

  async listAgents(): Promise<any> {
    const resp = await fetch(`${this.baseUrl}/api/agents`);
    return resp.json();
  }

  async getTodos(sessionId: string): Promise<any> {
    const resp = await fetch(`${this.baseUrl}/api/sessions/${sessionId}/todos`);
    return resp.json();
  }

  async getUpdates(sessionId: string, since?: string): Promise<any> {
    const params = since ? `?since=${since}` : "";
    const resp = await fetch(
      `${this.baseUrl}/api/sessions/${sessionId}/updates${params}`
    );
    return resp.json();
  }
}
```

### React Hook

```tsx
import { useState, useCallback, useRef } from "react";

interface Message {
  id: string;
  role: string;
  steps: Array<{
    parts: Array<{
      type: string;
      text?: string;
      state?: string;
      toolName?: string;
      output?: string;
      error?: string;
    }>;
    state: string;
  }>;
}

export function useChat(client: CopConClient, sessionId: string) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const sendMessage = useCallback(async (content: string) => {
    setIsStreaming(true);
    let currentText = "";

    try {
      for await (const event of client.chat(sessionId, content)) {
        if (event.type === "part_update") {
          const delta = event.data.textDelta;
          if (delta) {
            currentText += delta;
            // Update UI incrementally
          }
        }
        if (event.type === "message_done") {
          // Reload full message list
          const result = await client.getMessages(sessionId);
          setMessages(result.messages);
        }
      }
    } finally {
      setIsStreaming(false);
    }
  }, [client, sessionId]);

  return { messages, isStreaming, sendMessage };
}
```

---

## cURL Quick Reference

Complete cURL examples for every endpoint.

### Health

```bash
curl http://localhost:8080/health
```

### Agents

```bash
# List agents
curl http://localhost:8080/api/agents
```

### Sessions

```bash
# Create session
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"title":"My Chat","default_agent_id":"code-assistant"}'

# Create with defaults (empty body)
curl -X POST http://localhost:8080/api/sessions

# List sessions
curl "http://localhost:8080/api/sessions?limit=10&offset=0"

# Get session
curl http://localhost:8080/api/sessions/SESSION_ID

# Delete session
curl -X DELETE http://localhost:8080/api/sessions/SESSION_ID
```

### Messages

```bash
# Get messages
curl "http://localhost:8080/api/sessions/SESSION_ID/messages?limit=50"
```

### Chat

```bash
# Send message (streaming)
curl -N -X POST http://localhost:8080/api/sessions/SESSION_ID/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"Hello, can you help me?"}'

# With specific agent
curl -N -X POST http://localhost:8080/api/sessions/SESSION_ID/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"Review this code","agent_id":"reviewer"}'

# Reconnect to active stream
curl -N -X POST http://localhost:8080/api/sessions/SESSION_ID/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"","reconnect":true,"last_event_seq":42}'
```

### Control

```bash
# Stop active agent
curl -X POST http://localhost:8080/api/sessions/SESSION_ID/stop

# Resume HITL interrupt
curl -X POST http://localhost:8080/api/sessions/SESSION_ID/resume \
  -H "Content-Type: application/json" \
  -d '{"interrupt_id":"abc-123","action":"approve"}'

# Reject with reason
curl -X POST http://localhost:8080/api/sessions/SESSION_ID/resume \
  -H "Content-Type: application/json" \
  -d '{"interrupt_id":"abc-123","action":"reject","content":{"reason":"Too risky"}}'
```

### Todos and Updates

```bash
# Get todos
curl http://localhost:8080/api/sessions/SESSION_ID/todos

# Poll for async updates
curl "http://localhost:8080/api/sessions/SESSION_ID/updates"
curl "http://localhost:8080/api/sessions/SESSION_ID/updates?since=evt_001"
```

---

## Example Application

A minimal chat application in Python:

```python
#!/usr/bin/env python3
"""Minimal CopCon chat CLI."""

import sys
from copcon_client import CopConClient  # Using the client class above

def main():
    client = CopConClient()
    session = client.create_session("CLI Chat")
    session_id = session["id"]
    print(f"Session: {session_id}")
    print("Type 'quit' to exit, 'stop' to cancel, 'history' to see messages.\n")

    try:
        while True:
            user_input = input("You> ").strip()
            if not user_input:
                continue
            if user_input == "quit":
                break
            if user_input == "stop":
                client.stop_session(session_id)
                print("Agent stopped.")
                continue
            if user_input == "history":
                messages = client.get_messages(session_id)
                for msg in messages["messages"]:
                    role = msg["role"]
                    for step in msg.get("steps", []):
                        for part in step.get("parts", []):
                            if part.get("text"):
                                print(f"[{role}] {part['text']}")
                continue

            print("Agent> ", end="", flush=True)
            for event in client.chat(session_id, user_input):
                if event["type"] == "part_update":
                    delta = event["data"].get("textDelta", "")
                    if delta:
                        print(delta, end="", flush=True)
                    state = event["data"].get("state")
                    if state == "waiting_for_input":
                        interrupt = event["data"]["interrupt"]
                        print(f"\n[Interrupt: {interrupt['message']}]")
                        answer = input("Approve? (yes/no)> ")
                        action = "approve" if answer.lower() == "yes" else "reject"
                        client.resume(session_id, interrupt["interruptId"], action)
                elif event["type"] == "message_done":
                    print()
            print()
    finally:
        client.delete_session(session_id)
        print(f"Session {session_id} deleted.")

if __name__ == "__main__":
    main()
```
