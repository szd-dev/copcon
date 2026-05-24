# Custom Handlers

The CopCon server uses Gin HTTP handlers that delegate business logic to the core library. This guide explains the handler architecture, how to write custom handlers, and patterns for authentication, error handling, and streaming.

## Handler Architecture

### Request Lifecycle

Every API request follows this path:

```
Client Request
    |
    v
Gin Router (route matching)
    |
    v
Gin Middleware Chain (logging, recovery, CORS, auth)
    |
    v
Handler Method (e.g., Handler.CreateSession)
    |
    v
Store Interface (SessionStore, MessageStore, TodoStore)
    |
    v
PostgreSQL Provider (core/providers/postgres)
    |
    v
HTTP Response (JSON or SSE stream)
```

For chat requests, the path extends through the agent engine:

```
Handler.Chat
    |
    v
chat.HandleChat (framework-agnostic SSE helper)
    |
    v
AgentEngine.Chat (core agent loop)
    |
    v
ChatContext.Emit (event stream)
    |
    v
SSE Writer (text/event-stream to client)
```

### Handler Structure

The `Handler` struct in `server/internal/api/handlers.go` holds all dependencies:

```go
type Handler struct {
    config        *config.Config
    sessionStore  storage.SessionStore
    messageStore  storage.MessageStore
    todoStore     storage.TodoStore
    agent         agent.AgentEngine
    agentRegistry agent.AgentRegistry
    chatStore     chat.ActiveSessions
}
```

It's created by `NewHandler`, which extracts stores and engines from the `core.APIProvider`:

```go
func NewHandler(cfg *config.Config, h core.APIProvider) *Handler {
    return &Handler{
        config:        cfg,
        sessionStore:  h.Store().Sessions(),
        messageStore:  h.Store().Messages(),
        todoStore:     h.Store().Todos(),
        agent:         h.Engine(),
        agentRegistry: h.Registry(),
        chatStore:     h.ActiveSessions(),
    }
}
```

This design keeps the handler thin. It doesn't implement business logic; it translates between HTTP and the core library's interfaces.

### Route Registration

Routes are registered in `SetupRoutes`:

```go
func SetupRoutes(r *gin.Engine, cfg *config.Config, h core.APIProvider) {
    handler := NewHandler(cfg, h)

    api := r.Group("/api")
    {
        api.GET("/agents", handler.ListAgents)

        sessions := api.Group("/sessions")
        {
            sessions.POST("", handler.CreateSession)
            sessions.GET("", handler.ListSessions)
            sessions.GET("/:sessionId", handler.GetSession)
            sessions.DELETE("/:sessionId", handler.DeleteSession)
            sessions.GET("/:sessionId/messages", handler.GetMessages)
            sessions.POST("/:sessionId/chat", handler.Chat)
            sessions.POST("/:sessionId/stop", handler.StopSession)
            sessions.POST("/:sessionId/resume", handler.ResumeSession)
            sessions.GET("/:sessionId/todos", handler.GetSessionTodos)
            sessions.GET("/:sessionId/updates", handler.GetSessionUpdates)
        }
    }
}
```

To add new routes, either extend this function or create a separate registration function.

## Writing a Custom Handler

### Example: Stats Endpoint

Let's add an endpoint that returns aggregate statistics about sessions and messages.

**1. Add the handler method.**

Create a new file `server/internal/api/stats.go` (or add to `handlers.go`):

```go
package api

import (
    "net/http"

    "github.com/gin-gonic/gin"
)

func (h *Handler) GetStats(c *gin.Context) {
    ctx := c.Request.Context()

    // Use the session store to get a count
    _, totalSessions, err := h.sessionStore.List(ctx, 1, 0)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count sessions"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "total_sessions": totalSessions,
    })
}
```

**2. Register the route.**

Add it to `SetupRoutes`:

```go
api.GET("/stats", handler.GetStats)
```

**3. Test it.**

```go
func TestGetStats(t *testing.T) {
    handler, cleanup := setupTestHandler(t)
    defer cleanup()

    router := gin.New()
    router.GET("/api/stats", handler.GetStats)

    req, _ := http.NewRequest("GET", "/api/stats", nil)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var response map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &response)
    assert.Contains(t, response, "total_sessions")
}
```

### Example: Custom API Endpoint with Validation

An endpoint that validates input before proceeding:

```go
type CreateTagRequest struct {
    SessionID string `json:"session_id" binding:"required"`
    Tag       string `json:"tag" binding:"required,min=1,max=50"`
}

func (h *Handler) CreateTag(c *gin.Context) {
    var req CreateTagRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": fmt.Sprintf("validation failed: %s", err.Error()),
        })
        return
    }

    // Validate session exists
    sessUUID, err := uuid.Parse(req.SessionID)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
        return
    }

    _, err = h.sessionStore.Get(c.Request.Context(), sessUUID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
        return
    }

    // Store the tag using session metadata
    err = h.sessionStore.AppendMetadata(c.Request.Context(), sessUUID, "tag:"+req.Tag, true)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save tag"})
        return
    }

    c.JSON(http.StatusCreated, gin.H{
        "session_id": req.SessionID,
        "tag":        req.Tag,
    })
}
```

Key patterns here:

- `ShouldBindJSON` for input validation with struct tags
- UUID parsing before database lookups
- Existence checks before mutations
- Error responses with clear messages

## Request/Response Interceptors

### Adding Context to Every Request

Use middleware to inject shared data (request IDs, user info) into the Gin context:

```go
func RequestContextMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Set("request_id", uuid.New().String())
        c.Set("started_at", time.Now())
        c.Next()
    }
}
```

Then in handlers:

```go
requestID, _ := c.Get("request_id")
startedAt, _ := c.Get("started_at")
log.Info("request", "id", requestID, "duration", time.Since(startedAt.(time.Time)))
```

### Response Wrapping

To wrap all API responses in a consistent envelope, use a post-handler middleware:

```go
func ResponseEnvelope() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()

        // Only wrap JSON responses that haven't been written yet
        if c.Writer.Written() {
            return
        }

        // Wrap the response
        // Note: this requires a custom response writer to capture the body
    }
}
```

For simple cases, just use a helper function:

```go
func respondOK(c *gin.Context, data any) {
    c.JSON(http.StatusOK, gin.H{
        "code":    0,
        "message": "success",
        "data":    data,
    })
}

func respondError(c *gin.Context, status int, msg string) {
    c.JSON(status, gin.H{
        "code":    status * 1000,
        "message": msg,
        "data":    nil,
    })
}
```

## Error Handling Patterns

### Handler-Level Errors

The existing handlers follow a simple pattern: check the error and return an appropriate HTTP status:

```go
if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
}
```

Status code mapping:

| Condition | HTTP Status | Example |
|-----------|-------------|---------|
| Invalid input (bad JSON, wrong format) | 400 | `c.JSON(http.StatusBadRequest, ...)` |
| Resource not found | 404 | `c.JSON(http.StatusNotFound, ...)` |
| Conflict (active agent already running) | 409 | `c.JSON(http.StatusConflict, ...)` |
| Internal server error | 500 | `c.JSON(http.StatusInternalServerError, ...)` |

### Centralized Error Handler

For larger applications, centralize error handling:

```go
type APIError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

func (e *APIError) Error() string {
    return e.Message
}

func ErrBadRequest(msg string) *APIError {
    return &APIError{Code: 400001, Message: msg}
}

func ErrNotFound(msg string) *APIError {
    return &APIError{Code: 404001, Message: msg}
}

func ErrorHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()

        for _, err := range c.Errors {
            switch e := err.Err.(type) {
            case *APIError:
                c.JSON(e.Code/1000, e)
            default:
                c.JSON(http.StatusInternalServerError, gin.H{
                    "code":    500001,
                    "message": "internal server error",
                })
            }
            return
        }
    }
}
```

### Don't Leak Internal Errors

Never expose raw error messages from the database or LLM API to clients. Wrap them:

```go
// Bad: leaks database internals
c.JSON(500, gin.H{"error": err.Error()})

// Good: generic message, log the details
log.Error("database error", "error", err, "session_id", id)
c.JSON(500, gin.H{"error": "failed to retrieve session"})
```

## Authentication Integration

### API Key Middleware

Add API key checking before your handlers:

```go
func APIKeyAuth(validKeys map[string]bool) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.GetHeader("X-API-Key")
        if key == "" {
            key = c.Query("api_key")
        }

        if !validKeys[key] {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "invalid or missing API key",
            })
            return
        }
        c.Next()
    }
}
```

Apply it to route groups:

```go
api := r.Group("/api")
api.Use(APIKeyAuth(validKeys))
```

### JWT Middleware

```go
func JWTAuth(secret string) gin.HandlerFunc {
    return func(c *gin.Context) {
        auth := c.GetHeader("Authorization")
        if auth == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
            return
        }

        tokenString := strings.TrimPrefix(auth, "Bearer ")
        token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
            if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
            }
            return []byte(secret), nil
        })

        if err != nil || !token.Valid {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
            return
        }

        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
            return
        }

        c.Set("user_id", claims["sub"])
        c.Set("roles", claims["roles"])
        c.Next()
    }
}
```

## Streaming Endpoints

### How Chat Streaming Works

The `/api/sessions/:sessionId/chat` endpoint uses Server-Sent Events (SSE). The handler sets up the response headers and delegates to the framework-agnostic `chat.HandleChat`:

```go
func (h *Handler) Chat(c *gin.Context) {
    var req chat.ChatRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
        return
    }
    req.SessionID = c.Param("sessionId")

    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")

    flusher, ok := c.Writer.(http.Flusher)
    if !ok {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
        return
    }

    chat.HandleChat(c.Request.Context(), c.Writer, flusher, req, h.agent, h.chatStore)
}
```

The three SSE headers are required:
- `Content-Type: text/event-stream` tells the client this is an SSE stream
- `Cache-Control: no-cache` prevents proxy caching
- `Connection: keep-alive` keeps the TCP connection open

### Example: Custom SSE Endpoint

To create a custom streaming endpoint (e.g., streaming tool execution logs):

```go
func (h *Handler) StreamToolLogs(c *gin.Context) {
    sessionID := c.Param("sessionId")

    // Set SSE headers
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")

    flusher, _ := c.Writer.(http.Flusher)

    // Get active chat context
    chatCtx, found := h.chatStore.Get(sessionID)
    if !found {
        c.JSON(http.StatusNotFound, gin.H{"error": "no active session"})
        return
    }

    // Stream events
    for event := range chatCtx.Events() {
        data, _ := json.Marshal(event)
        fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
        flusher.Flush()

        if event.Type == entity.EventDone {
            break
        }
    }
}
```

### Reconnection Handling

The chat handler supports reconnection. When a client reconnects with `last_event_seq`, it receives only events after that sequence number. This is handled by the ring buffer inside `ChatContext`.

The existing `Chat` handler checks the request for `reconnect: true` and handles it transparently through `chat.HandleChat`.

## Organizing Handler Code

As your API grows, split handlers into separate files by domain:

```
server/internal/api/
    handlers.go         # SetupRoutes, NewHandler
    sessions.go         # CreateSession, ListSessions, GetSession, DeleteSession
    messages.go         # GetMessages
    chat.go             # Chat, StopSession, ResumeSession
    agents.go           # ListAgents
    render.go           # GroupPartsByStep, BackfillParts
    stats.go            # GetStats (new)
```

Each file is in the same `api` package and operates on the same `Handler` struct. Gin doesn't care which file a handler method lives in.

## Testing Handlers

The test file `server/internal/api/handlers_test.go` shows the pattern:

1. Create mock implementations of store interfaces.
2. Build a `testHarness` that satisfies `core.APIProvider`.
3. Create a `Handler` with `NewHandler`.
4. Set up a `gin.New()` router with the specific route under test.
5. Use `httptest.NewRecorder()` to capture responses.
6. Assert status codes and response bodies.

For SSE tests, parse the response body looking for `data: ` lines:

```go
func parseSSEEvents(t *testing.T, body string) []map[string]interface{} {
    t.Helper()
    var events []map[string]interface{}
    lines := strings.Split(body, "\n")
    for _, line := range lines {
        if strings.HasPrefix(line, "data: ") {
            var event map[string]interface{}
            err := json.Unmarshal([]byte(line[6:]), &event)
            require.NoError(t, err)
            events = append(events, event)
        }
    }
    return events
}
```

## Next Steps

- [Middleware](./middleware.md) for request logging, CORS, and auth
- [API Reference](./api-reference.md) for the full endpoint catalog
- [Security](./security.md) for auth and authorization patterns