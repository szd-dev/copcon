# Logging

The CopCon server uses Go's `log/slog` package for structured logging. This guide covers log levels, formats, destinations, redaction, and integration with external tools.

## Current Setup

The server initializes a single logger in `cmd/server/main.go`:

```go
log := slog.New(slog.NewTextHandler(os.Stderr, nil))
```

This produces text-formatted log lines to stderr. The logger is passed into the Harness and used throughout the request lifecycle.

The Gin framework adds its own Logger and Recovery middleware when you use `gin.Default()`, which writes access logs to stdout and panic stacks to stderr.

## Log Levels

`slog` defines four levels, from most to least verbose:

| Level | When to Use | Example |
|-------|-------------|---------|
| `DEBUG` | Development only. Verbose internals, query traces, full request/response bodies. | `log.Debug("GORM query", "sql", query, "duration", dur)` |
| `INFO` | Normal operations. Server start, session creation, agent dispatch. | `log.Info("Server starting", "port", "8088")` |
| `WARN` | Recoverable problems. Retries, degraded features, configuration oddities. | `log.Warn("Qdrant unavailable, memory features disabled")` |
| `ERROR` | Failures that break a request or feature. Database errors, LLM API failures. | `log.Error("fatal", "error", err)` |

The current server uses `INFO` as the default level with no way to change it at runtime. To make the level configurable, swap the handler:

```go
level := slog.LevelInfo
if os.Getenv("COPCON_LOG_LEVEL") == "debug" {
    level = slog.LevelDebug
}

handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: level,
})
log := slog.New(handler)
```

## Log Formats

### Text Format (Default)

```
time=2026-05-24T10:30:00.000Z level=info msg="Server starting" port=8088
time=2026-05-24T10:30:01.200Z level=error msg="fatal" error="connection refused"
```

Good for local development and reading logs in a terminal. Each key-value pair is space-separated.

### JSON Format

```go
log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
```

Output:

```json
{"time":"2026-05-24T10:30:00.000Z","level":"INFO","msg":"Server starting","port":"8088"}
{"time":"2026-05-24T10:30:01.200Z","level":"ERROR","msg":"fatal","error":"connection refused"}
```

Use JSON in production. It's the standard format for log aggregation tools (ELK, Loki, Datadog).

### Structured Fields

Always use structured key-value pairs instead of string formatting:

```go
// Good: structured, searchable
log.Info("session created", "session_id", id, "agent_id", agentID, "duration_ms", dur)

// Bad: formatted string, not searchable
log.Info(fmt.Sprintf("session %s created with agent %s in %dms", id, agentID, dur))
```

Structured fields let you filter and aggregate in your log platform. You can search `session_id=abc123` instead of regexing through formatted strings.

## Log Destinations

### stderr (Default)

The default configuration writes to `os.Stderr`. This is the right choice for containers, where stdout/stderr are captured by the container runtime.

### File Output

To write logs to a file, wrap `os.Stderr` with a file writer:

```go
logFile, err := os.OpenFile("/var/log/copcon/server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
if err != nil {
    log.Fatal(err)
}
defer logFile.Close()

log := slog.New(slog.NewJSONHandler(logFile, nil))
```

For log rotation, use a library like `natefinch/lumberjack`:

```go
import "gopkg.in/natefinch/lumberjack.v2"

writer := &lumberjack.Logger{
    Filename:   "/var/log/copcon/server.log",
    MaxSize:    100, // megabytes
    MaxBackups: 10,
    MaxAge:     30,  // days
    Compress:   true,
}

log := slog.New(slog.NewJSONHandler(writer, nil))
```

### Dual Output (stderr + file)

Write to both stderr and a file using `io.MultiWriter`:

```go
multiWriter := io.MultiWriter(os.Stderr, writer)
log := slog.New(slog.NewJSONHandler(multiWriter, nil))
```

This keeps container logs visible in `docker logs` while also persisting to a file.

## Gin Access Logs

Gin's built-in Logger middleware writes HTTP access logs in its own format:

```
[GIN] 2026/05/24 - 10:30:00 | 200 |     1.234ms |       127.0.0.1 | POST     "/api/sessions/abc123/chat"
```

To make Gin logs consistent with slog, replace the default Logger middleware:

```go
r := gin.New() // Use gin.New(), not gin.Default()
r.Use(gin.Recovery()) // Keep Recovery
r.Use(SlogLoggerMiddleware(log)) // Custom middleware

func SlogLoggerMiddleware(log *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        raw := c.Request.URL.RawQuery

        c.Next()

        latency := time.Since(start)
        status := c.Writer.Status()

        attrs := []any{
            "status", status,
            "method", c.Request.Method,
            "path", path,
            "latency_ms", latency.Milliseconds(),
            "client_ip", c.ClientIP(),
        }
        if raw != "" {
            attrs = append(attrs, "query", raw)
        }
        if len(c.Errors) > 0 {
            attrs = append(attrs, "errors", c.Errors.String())
        }

        if status >= 500 {
            log.Error("request completed", attrs...)
        } else if status >= 400 {
            log.Warn("request completed", attrs...)
        } else {
            log.Info("request completed", attrs...)
        }
    }
}
```

## Log Correlation and Tracing

### Request IDs

Add a request ID middleware to correlate all logs within a single request:

```go
func RequestIDMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        requestID := c.GetHeader("X-Request-ID")
        if requestID == "" {
            requestID = uuid.New().String()
        }
        c.Set("request_id", requestID)
        c.Header("X-Request-ID", requestID)
        c.Next()
    }
}
```

Then in your handlers, attach the request ID to logs:

```go
func (h *Handler) CreateSession(c *gin.Context) {
    requestID, _ := c.Get("request_id")
    log := h.logger.With("request_id", requestID)
    log.Info("creating session")
    // ...
}
```

### Session Correlation

For chat-related operations, include the session ID:

```go
log.Info("chat started", "session_id", sessionID, "agent_id", agentID)
log.Info("chat completed", "session_id", sessionID, "duration_ms", dur, "token_count", tokens)
```

### Distributed Tracing with OpenTelemetry

The core library supports OpenTelemetry hooks. To wire trace IDs into your server logs:

1. Configure the tracing hook in your Harness setup.
2. Extract the trace ID from the context and add it to log entries.

```go
import "go.opentelemetry.io/otel/trace"

func WithTraceID(ctx context.Context, log *slog.Logger) *slog.Logger {
    span := trace.SpanFromContext(ctx)
    if span.SpanContext().IsValid() {
        return log.With(
            "trace_id", span.SpanContext().TraceID().String(),
            "span_id", span.SpanContext().SpanID().String(),
        )
    }
    return log
}
```

## Sensitive Data Redaction

### What to Redact

CopCon processes user messages that may contain sensitive data. Never log:

- Full message content (especially user input)
- API keys and tokens
- Database passwords
- Personal information

### Redaction Middleware

Add a custom slog handler that filters known sensitive keys:

```go
type redactingHandler struct {
    inner    slog.Handler
    redacted map[string]bool
}

func (h *redactingHandler) Handle(ctx context.Context, r slog.Record) error {
    sanitized := r.Clone()
    r.Attrs(func(a slog.Attr) bool {
        if h.redacted[a.Key] {
            sanitized.AddAttrs(slog.String(a.Key, "[REDACTED]"))
        } else {
            sanitized.AddAttrs(a)
        }
        return true
    })
    return h.inner.Handle(ctx, sanitized)
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return h.inner.Enabled(ctx, level)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &redactingHandler{inner: h.inner.WithAttrs(attrs), redacted: h.redacted}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
    return &redactingHandler{inner: h.inner.WithGroup(name), redacted: h.redacted}
}
```

Usage:

```go
baseHandler := slog.NewJSONHandler(os.Stderr, nil)
redacted := &redactingHandler{
    inner: baseHandler,
    redacted: map[string]bool{
        "api_key":     true,
        "password":    true,
        "token":       true,
        "content":     true,  // redact user message content
        "api_key":     true,
    },
}
log := slog.New(redacted)
```

### Field-Level Redaction in Handlers

In handler code, never pass raw user content to the logger:

```go
// Bad: logs user message
log.Info("received chat", "content", userInput)

// Good: log only metadata
log.Info("received chat", "session_id", sessionID, "content_length", len(userInput))
```

## Debug Logging

### Enabling Debug Logs

Set the environment variable and update the handler:

```bash
COPCON_LOG_LEVEL=debug ./server
```

### GORM Query Logging

To see every SQL query, enable GORM's logger:

```go
import "gorm.io/gorm/logger"

db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Info), // logs all queries
})
```

For production, use a conditional logger that only logs slow queries:

```go
db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Warn), // only slow queries and errors
})
```

### SSE Event Debugging

To trace SSE events flowing through the chat handler, add debug logging in the `HandleChat` wrapper:

```go
func (h *Handler) Chat(c *gin.Context) {
    log := h.logger.With("session_id", c.Param("sessionId"))
    log.Debug("chat request received", "content_length", len(req.Content))
    // ... existing SSE setup
    log.Debug("chat SSE stream completed")
}
```

## Integration with Log Aggregation

### ELK Stack (Elasticsearch, Logstash, Kibana)

1. Set the log format to JSON.
2. Configure Filebeat or Logstash to tail the log file or read from Docker stdout.

```yaml
# filebeat.yml
filebeat.inputs:
  - type: container
    paths:
      - /var/lib/docker/containers/*/*.log
    processors:
      - add_docker_metadata:

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
  index: "copcon-%{+yyyy.MM.dd}"
```

### Grafana Loki

Loki works well with structured JSON logs. Use the `loki` output in Promtail:

```yaml
# promtail.yml
scrape_configs:
  - job_name: copcon
    docker_sd_configs:
      - host: unix:///var/run/docker.sock
        refresh_interval: 5s
        filters:
          - name: label
            values: ["com.docker.compose.service=copcon-server"]
    relabel_configs:
      - source_labels: ['__meta_docker_container_name']
        target_label: 'container'
    pipeline_stages:
      - json:
          expressions:
            level: level
            msg: msg
            session_id: session_id
```

### Datadog

If you're on Datadog, the JSON log format works out of the box with the Datadog Agent. Set the service name via a log attribute:

```go
log := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("service", "copcon-server", "env", "prod")
```

## Best Practices

1. **Use JSON in production.** Text is fine for dev, but JSON is what aggregation tools expect.
2. **Always include context.** A log line without a session ID or request ID is hard to trace.
3. **Never log user content at INFO or above.** Use DEBUG, and only in dev.
4. **Log at the boundary.** Log when requests enter and leave your system, not at every internal function call.
5. **Use structured fields, not string formatting.** This makes logs searchable and aggregatable.
6. **Keep log volume reasonable.** Debug logs in production will fill disks and cost money. Set levels appropriately.
7. **Monitor slow queries via GORM logger.** Set `LogMode(logger.Warn)` to catch queries taking longer than 200ms.

## Next Steps

- [Configuration](./configuration.md) for log-related config fields
- [Middleware](./middleware.md) for request logging middleware
- [Security](./security.md) for data redaction policies