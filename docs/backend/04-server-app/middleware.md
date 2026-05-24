# Middleware

CopCon uses Gin middleware for cross-cutting concerns like logging, recovery, CORS, and authentication. This guide covers the built-in middleware, how to write custom middleware, and practical patterns for production use.

## Built-in Middleware

The server currently uses `gin.Default()`, which includes two middleware handlers:

### Logger

Gin's built-in Logger writes a line for every HTTP request:

```
[GIN] 2026/05/24 - 10:30:00 | 200 |     1.234ms |       127.0.0.1 | POST     "/api/sessions/abc123/chat"
```

This covers method, status, latency, client IP, and path. It's useful for quick debugging but lacks structured fields.

### Recovery

Gin's Recovery middleware catches panics inside handlers, writes a stack trace to stderr, and returns a 500 response. Without it, a panic would crash the entire server process.

```go
// What Recovery does internally:
defer func() {
    if r := recover(); r != nil {
        c.AbortWithStatus(500)
    }
}()
```

## Middleware Execution Order

Gin executes middleware in the order it's added. For `gin.Default()`, the chain is:

```
Request → Logger → Recovery → Handler → Response
```

When you add custom middleware, the chain grows:

```
Request → CORS → RateLimit → Auth → Logger → Recovery → Handler → Response
```

The order matters. Put cross-cutting concerns first:

1. **CORS** (before anything, so preflight OPTIONS requests get handled)
2. **Rate limiting** (before auth, so blocked requests don't hit your auth logic)
3. **Authentication** (before handler, so handlers can assume the user is validated)
4. **Logging** (before recovery, so panics get logged)
5. **Recovery** (last before handler, so it catches all panics)

## Switching to Custom Middleware

`gin.Default()` is convenient for development. For production, switch to `gin.New()` and add exactly the middleware you need:

```go
r := gin.New()
r.Use(gin.Recovery())
r.Use(RequestIDMiddleware())
r.Use(SlogLoggerMiddleware(log))
r.Use(CORSMiddleware(corsConfig))
```

This gives you full control over the chain and avoids the default Logger in favor of structured slog output.

## Writing Custom Middleware

### Anatomy of a Gin Middleware

A middleware is a function that returns a `gin.HandlerFunc`:

```go
func MyMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Pre-handler logic: runs before the next handler

        c.Next() // Call the next handler in the chain

        // Post-handler logic: runs after the handler returns
    }
}
```

- Code before `c.Next()` runs before the handler.
- Code after `c.Next()` runs after the handler returns.
- Calling `c.Abort()` stops the chain and returns immediately.
- Calling `c.AbortWithStatus(code)` sets the status and stops.

### Example: Request ID

Attaches a unique ID to every request for tracing:

```go
func RequestIDMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        id := c.GetHeader("X-Request-ID")
        if id == "" {
            id = uuid.New().String()
        }
        c.Set("request_id", id)
        c.Header("X-Request-ID", id)
        c.Next()
    }
}
```

### Example: Request Timing

Logs how long each request takes:

```go
func TimingMiddleware(log *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()
        duration := time.Since(start)

        log.Info("request",
            "method", c.Request.Method,
            "path", c.Request.URL.Path,
            "status", c.Writer.Status(),
            "duration_ms", duration.Milliseconds(),
        )
    }
}
```

### Example: Structured Logging Middleware

Replaces Gin's default Logger with slog:

```go
func SlogLoggerMiddleware(log *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        query := c.Request.URL.RawQuery

        c.Next()

        latency := time.Since(start)
        status := c.Writer.Status()

        attrs := []any{
            "status", status,
            "method", c.Request.Method,
            "path", path,
            "latency_ms", latency.Milliseconds(),
            "client_ip", c.ClientIP(),
            "body_size", c.Writer.Size(),
        }
        if query != "" {
            attrs = append(attrs, "query", query)
        }
        if len(c.Errors) > 0 {
            attrs = append(attrs, "errors", c.Errors.ByType(gin.ErrorTypePrivate).String())
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

## CORS Configuration

Cross-Origin Resource Sharing is necessary when your frontend runs on a different domain or port than the server.

### Basic CORS Middleware

```go
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
    originsMap := make(map[string]bool)
    for _, o := range allowedOrigins {
        originsMap[o] = true
    }

    return func(c *gin.Context) {
        origin := c.GetHeader("Origin")

        if originsMap[origin] {
            c.Header("Access-Control-Allow-Origin", origin)
        }

        c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
        c.Header("Access-Control-Allow-Credentials", "true")
        c.Header("Access-Control-Max-Age", "86400")

        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(http.StatusNoContent)
            return
        }

        c.Next()
    }
}
```

### Usage

```go
r.Use(CORSMiddleware([]string{
    "http://localhost:3000",
    "https://app.example.com",
}))
```

For development, allow all origins:

```go
r.Use(CORSMiddleware([]string{"*"}))
```

Never use `*` in production.

### Using gin-contrib/cors

For more features (regex matching, dynamic origins), use the community package:

```go
import "github.com/gin-contrib/cors"

r.Use(cors.New(cors.Config{
    AllowOrigins:     []string{"https://app.example.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Content-Type", "Authorization"},
    ExposeHeaders:    []string{"X-Request-ID"},
    AllowCredentials: true,
    MaxAge:           24 * time.Hour,
}))
```

## Rate Limiting

### Simple Per-IP Rate Limiter

```go
import "golang.org/x/time/rate"

func RateLimitMiddleware(rps float64, burst int) gin.HandlerFunc {
    limiters := sync.Map{}

    getLimiter := func(key string) *rate.Limiter {
        if l, ok := limiters.Load(key); ok {
            return l.(*rate.Limiter)
        }
        l := rate.NewLimiter(rate.Limit(rps), burst)
        limiters.Store(key, l)
        return l
    }

    return func(c *gin.Context) {
        ip := c.ClientIP()
        limiter := getLimiter(ip)

        if !limiter.Allow() {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "error": "rate limit exceeded",
            })
            return
        }
        c.Next()
    }
}
```

Usage:

```go
// 10 requests per second, burst of 20
r.Use(RateLimitMiddleware(10, 20))
```

### Endpoint-Specific Rate Limiting

Apply stricter limits to expensive endpoints:

```go
chatLimiter := RateLimitMiddleware(2, 5)  // 2 req/s for chat
sessionsLimiter := RateLimitMiddleware(10, 20)

sessions.POST("/:sessionId/chat", chatLimiter, handler.Chat)
sessions.POST("", sessionsLimiter, handler.CreateSession)
```

### Distributed Rate Limiting with Redis

For multi-instance deployments, use a shared counter in Redis:

```go
import "github.com/redis/go-redis/v9"

func RedisRateLimitMiddleware(rdb *redis.Client, keyPrefix string, limit int, window time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        ip := c.ClientIP()
        key := fmt.Sprintf("%s:%s", keyPrefix, ip)

        ctx := c.Request.Context()
        count, err := rdb.Incr(ctx, key).Result()
        if err != nil {
            // Fail open: if Redis is down, allow the request
            c.Next()
            return
        }

        if count == 1 {
            rdb.Expire(ctx, key, window)
        }

        if count > int64(limit) {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "error": "rate limit exceeded",
            })
            return
        }

        c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
        c.Header("X-RateLimit-Remaining", strconv.Itoa(int(int64(limit)-count)))
        c.Next()
    }
}
```

## Request Validation

### Body Size Limit

Prevent oversized request bodies from consuming memory:

```go
func MaxBodySizeMiddleware(maxBytes int64) gin.HandlerFunc {
    return func(c *gin.Context) {
        if c.Request.ContentLength > maxBytes {
            c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
                "error": "request body too large",
            })
            return
        }
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
        c.Next()
    }
}
```

Usage:

```go
// 100KB max request body
r.Use(MaxBodySizeMiddleware(100 * 1024))
```

### Session ID Validation

Many endpoints take `:sessionId` as a path parameter. Centralize the UUID check:

```go
func ValidateSessionID() gin.HandlerFunc {
    return func(c *gin.Context) {
        id := c.Param("sessionId")
        if _, err := uuid.Parse(id); err != nil {
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
                "error": "invalid session id",
            })
            return
        }
        c.Next()
    }
}
```

Apply to the sessions group:

```go
sessions := api.Group("/sessions")
sessions.Use(ValidateSessionID())
```

## Authentication Middleware

See [Custom Handlers: Authentication Integration](./custom-handlers.md#authentication-integration) for detailed examples of API key and JWT middleware.

Quick setup:

```go
// API key for simple setups
r.Use(APIKeyAuth(validKeys))

// JWT for full auth systems
r.Use(JWTAuth(jwtSecret))

// Per-route: only protect certain endpoints
protected := r.Group("/api")
protected.Use(JWTAuth(jwtSecret))
{
    protected.POST("/sessions", handler.CreateSession)
    protected.DELETE("/:sessionId", handler.DeleteSession)
}
```

## Middleware for SSE Endpoints

SSE endpoints need special handling. Proxies and load balancers can buffer SSE responses, breaking real-time updates.

### Disable Proxy Buffering

```go
func SSEHeadersMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Accel-Buffering", "no") // Nginx
        c.Header("Cache-Control", "no-cache")
        c.Header("Connection", "keep-alive")
        c.Next()
    }
}
```

Apply to chat routes:

```go
sessions.POST("/:sessionId/chat", SSEHeadersMiddleware(), handler.Chat)
```

For Nginx, also add `proxy_buffering off;` to the location block:

```nginx
location /api/ {
    proxy_pass http://server;
    proxy_buffering off;
    proxy_http_version 1.1;
    proxy_set_header Connection '';
}
```

## Health Check Middleware

The server already has a simple health check:

```go
r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
```

For more detailed health checks that verify database connectivity:

```go
func HealthCheckMiddleware(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        sqlDB, err := db.DB()
        if err != nil {
            c.JSON(http.StatusServiceUnavailable, gin.H{
                "status": "unhealthy",
                "error":  "database connection error",
            })
            return
        }

        if err := sqlDB.Ping(); err != nil {
            c.JSON(http.StatusServiceUnavailable, gin.H{
                "status": "unhealthy",
                "error":  "database ping failed",
            })
            return
        }

        c.JSON(http.StatusOK, gin.H{"status": "healthy"})
    }
}
```

## Complete Middleware Chain

Here's a production-ready middleware setup for `cmd/server/main.go`:

```go
r := gin.New()

// 1. Recovery (catch panics)
r.Use(gin.Recovery())

// 2. Request ID (for tracing)
r.Use(RequestIDMiddleware())

// 3. Structured logging (replace default Logger)
r.Use(SlogLoggerMiddleware(log))

// 4. CORS
r.Use(CORSMiddleware([]string{"https://app.example.com"}))

// 5. Body size limit
r.Use(MaxBodySizeMiddleware(100 * 1024))

// 6. Rate limiting
r.Use(RateLimitMiddleware(20, 40))

// 7. Health check (no auth required)
r.GET("/health", HealthCheckMiddleware(db))

// 8. API routes with optional auth
api := r.Group("/api")
api.Use(APIKeyAuth(validKeys))
api.Use(ValidateSessionID())
{
    // ... route registration
}
```

## Testing Middleware

Test middleware the same way you test handlers: create a test router, make a request, check the response.

```go
func TestRateLimitMiddleware(t *testing.T) {
    r := gin.New()
    r.Use(RateLimitMiddleware(1, 1))
    r.GET("/test", func(c *gin.Context) { c.Status(200) })

    // First request succeeds
    w1 := httptest.NewRecorder()
    r.ServeHTTP(w1, httptest.NewRequest("GET", "/test", nil))
    assert.Equal(t, 200, w1.Code)

    // Second request is rate limited
    w2 := httptest.NewRecorder()
    r.ServeHTTP(w2, httptest.NewRequest("GET", "/test", nil))
    assert.Equal(t, 429, w2.Code)
}
```

## Next Steps

- [Custom Handlers](./custom-handlers.md) for writing handler logic
- [Security](./security.md) for auth and hardening
- [Logging](./logging.md) for structured logging patterns