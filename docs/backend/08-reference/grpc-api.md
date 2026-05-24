# gRPC API

CopCon does not currently ship a gRPC API. The server exposes only HTTP REST endpoints with SSE for streaming (see [HTTP API](./http-api.md) and [SSE Streaming](./websocket-api.md)).

## Current Transport Layer

| Transport | Status | Use Case |
|-----------|--------|----------|
| HTTP + JSON | Stable | All CRUD operations |
| SSE (over HTTP) | Stable | Streaming agent responses |
| WebSocket | Not available | |
| gRPC | Not available | |

## When to Consider gRPC

If your integration requires gRPC, these scenarios make it a good fit:

- High-throughput service-to-service communication in a microservices architecture
- Strict schema enforcement and code generation across multiple languages
- Bidirectional streaming for real-time agent control
- Lower payload overhead with Protocol Buffers serialization

## Adding gRPC Support

CopCon's architecture supports adding a gRPC transport layer without modifying the core engine. The key integration point is the `APIProvider` interface:

```go
type APIProvider interface {
    Store() storage.StoreProvider
    Engine() agent.AgentEngine
    Registry() agent.AgentRegistry
    ActiveSessions() chat.ActiveSessions
}
```

A gRPC server would:

1. Define `.proto` files mirroring the REST API resources (Sessions, Messages, Chat)
2. Implement a streaming RPC for chat that consumes `chatcontext.ChatContext.Events()`
3. Register the server alongside the existing Gin HTTP server in `cmd/server/main.go`

### Proto Skeleton

```protobuf
syntax = "proto3";

package copcon.v1;

service AgentService {
  // Session management
  rpc CreateSession(CreateSessionRequest) returns (Session);
  rpc ListSessions(ListSessionsRequest) returns (SessionList);
  rpc GetSession(GetSessionRequest) returns (Session);
  rpc DeleteSession(DeleteSessionRequest) returns (google.protobuf.Empty);

  // Messages
  rpc GetMessages(GetMessagesRequest) returns (MessageList);

  // Streaming chat
  rpc Chat(ChatRequest) returns (stream ChatEvent);

  // Agent control
  rpc StopSession(StopSessionRequest) returns (google.protobuf.Empty);
  rpc ResumeSession(ResumeSessionRequest) returns (ResumeResponse);

  // HITL
  rpc GetSessionTodos(GetSessionTodosRequest) returns (TodoList);
  rpc GetSessionUpdates(GetSessionUpdatesRequest) returns (UpdateList);
}

message ChatRequest {
  string session_id = 1;
  string content = 2;
  string agent_id = 3;
}

message ChatEvent {
  string type = 1;
  bytes data = 2;  // JSON-encoded event data
}
```

### Implementation Approach

```go
func (s *grpcServer) Chat(req *pb.ChatRequest, stream pb.AgentService_ChatServer) error {
    chatCtx := chatcontext.NewChatContext(stream.Context(), req.SessionId, req.AgentId)
    chatCtx.SetStore(s.activeSessions)
    s.activeSessions.Put(req.SessionId, chatCtx)

    go func() {
        defer chatCtx.Close()
        s.engine.Chat(chatCtx, req.Content)
    }()

    sub, ok := chatCtx.Subscribe(0)
    if !ok {
        return status.Error(codes.Internal, "failed to subscribe to events")
    }

    for {
        select {
        case event, ok := <-sub.Events:
            if !ok {
                return nil
            }
            data, _ := json.Marshal(event)
            if err := stream.Send(&pb.ChatEvent{
                Type: string(event.Type),
                Data: data,
            }); err != nil {
                return err
            }
        case <-stream.Context().Done():
            return stream.Context().Err()
        }
    }
}
```

## Roadmap

gRPC support is not on the immediate roadmap. If you need it, consider these options:

1. **Build a gRPC gateway** alongside the HTTP server using the `APIProvider` interface
2. **Use grpc-gateway** to auto-generate a REST proxy from proto definitions
3. **Contribute** a gRPC transport implementation to the project

The core library (`github.com/copcon/core`) is transport-agnostic. Adding a new transport does not require changes to the engine, storage, or capability layers.
