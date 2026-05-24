# Rate Limiting

CopCon does not ship with built-in rate limiting middleware. This document describes the recommended approach to rate limiting for production deployments and provides practical implementation guidance.

---

## Why Rate Limiting Matters

LLM-backed APIs have unique cost characteristics. Each chat request can trigger multiple LLM calls (one per agent loop step), each with significant token cost and latency. Without rate limiting:

- A single client can exhaust your LLM API quota
- Concurrent requests across sessions can overwhelm the LLM provider
- Background async tools can accumulate without bound
- Costs scale linearly with uncontrolled usage

---

## Recommended Limits

| Resource | Limit | Scope | Rationale |
|----------|-------|-------|-----------|
| Chat requests | 10 per minute | Per session | Prevent rapid-fire messages while agent is processing |
| Concurrent sessions | 5 | Per client | Limit parallel agent loops |
| Message size | 10,000 characters | Per request | Prevent oversized context windows |
| SSE connections | 1 per session | Per session | Only one active agent per session |
| Login attempts | 5 per minute | Per IP | Prevent credential brute-forcing |
| Total API requests | 100 per minute | Per API key / user | General abuse prevention |

These are starting points. Adjust based on your LLM provider's rate limits and your cost budget.

---

## LLM Provider Constraints

The most important rate limits come from your LLM provider, not CopCon itself.

### OpenAI

| Tier | RPM (Requests Per Minute) | TPM (Tokens Per Minute) |
|------|---------------------------|-------------------------|
| Free | 3 | 40,000 |
| Tier 1 | 500 | 200,000 |
| Tier 2 | 5,000 | 2,000,000 |

CopCon's agent loop can make multiple LLM calls per user message (one per step). A single chat request with tool calls might trigger 3-10 LLM calls. Factor this into your rate calculations.

### Concurrency

CopCon's engine uses a concurrency semaphore (default: 5) to limit parallel tool executions within a single agent loop. This is configured via `agent.WithConcurrency(n)`.

---

## Implementation

### Gin Middleware (Server-Side)

Add rate limiting middleware to the CopCon server. This example uses a token bucket algorithm:

```go
package middleware

import (
    "net/http"
    "sync"
    "time"

    "github.com/gin-gonic/gin"
    "golang.org/x/time/rate"
)

type RateLimiter struct {
    limiters sync.Map
    rate     rate.Limit
    burst    int
}

func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
    return &RateLimiter{rate: r, burst: burst}
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
    if l, ok := rl.limiters.Load(key); ok {
        return l.(*rate.Limiter)
    }
    l := rate.NewLimiter(rl.rate, rl.burst)
    actual, _ := rl.limiters.LoadOrStore(key, l)
    return actual.(*rate.Limiter)
}

func (rl *RateLimiter) Middleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.ClientIP()
        limiter := rl.getLimiter(key)

        if !limiter.Allow() {
            c.Header("Retry-After", "60")
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "error": "rate limit exceeded",
            })
            return
        }
        c.Next()
    }
}
```

Register in `SetupRoutes`:

```go
func SetupRoutes(r *gin.Engine, cfg *config.Config, h core.APIProvider) {
    handler := NewHandler(cfg, h)

    // Rate limit all API endpoints
    apiLimiter := middleware.NewRateLimiter(rate.Every(time.Second), 10)  // 10 req/sec
    api := r.Group("/api")
    api.Use(apiLimiter.Middleware())

    // Stricter limit on chat endpoint
    chatLimiter := middleware.NewRateLimiter(rate.Every(6*time.Second), 1)  // 10 req/min
    sessions := api.Group("/sessions")
    sessions.POST("/:sessionId/chat", chatLimiter.Middleware(), handler.Chat)

    // ... rest of routes
}
```

### Per-Session Concurrency

CopCon already enforces one active agent per session at the application level:

```go
// In chat.HandleChat
if _, active := store.Get(req.SessionID); active {
    return fmt.Errorf("session already has an active agent")
}
```

This prevents parallel chat requests to the same session. If you need to limit concurrent sessions per user, add middleware that tracks active sessions per API key.

---

## Rate Limit Headers

Return standard rate limit headers so clients can adjust their behavior:

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum requests allowed in the window |
| `X-RateLimit-Remaining` | Requests remaining in the current window |
| `X-RateLimit-Reset` | Timestamp when the window resets (Unix epoch) |
| `Retry-After` | Seconds until the client should retry (on 429) |

Example middleware with headers:

```go
func RateLimitWithHeaders(limiter *RateLimiter) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.ClientIP()
        l := limiter.getLimiter(key)

        // Set rate limit headers
        c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limiter.burst))
        c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", l.Tokens()))

        if !l.Allow() {
            c.Header("Retry-After", "60")
            c.AbortWithStatusJSON(429, gin.H{"error": "rate limit exceeded"})
            return
        }
        c.Next()
    }
}
```

---

## Client-Side Backoff

### Exponential Backoff

When receiving a `429 Too Many Requests` response:

```python
import time

def request_with_backoff(func, *args, max_retries=5, **kwargs):
    for attempt in range(max_retries):
        resp = func(*args, **kwargs)
        if resp.status_code != 429:
            return resp

        retry_after = int(resp.headers.get("Retry-After", "5"))
        wait = min(retry_after * (2 ** attempt), 60)
        time.sleep(wait)

    raise Exception("Max retries exceeded")
```

### Jitter

Add random jitter to avoid thundering herd:

```python
import random

def backoff_with_jitter(attempt, base=2, max_wait=60):
    wait = min(base ** attempt, max_wait)
    jitter = random.uniform(0, wait * 0.1)
    return wait + jitter
```

### Per-Endpoint Strategy

```python
BACKOFF_CONFIG = {
    "chat":      {"max_retries": 3, "base_wait": 2},   # Chat can take longer
    "sessions":  {"max_retries": 5, "base_wait": 1},   # CRUD is fast
    "resume":    {"max_retries": 2, "base_wait": 1},   # HITL should be quick
}
```

---

## Bulk Operation Quotas

Bulk operations (creating many sessions, listing all messages) should have separate quotas.

### Session Creation

Limit session creation to prevent database flooding:

```go
sessionLimiter := middleware.NewRateLimiter(rate.Every(10*time.Second), 1)  // 6 per minute
sessions.POST("", sessionLimiter.Middleware(), handler.CreateSession)
```

### Message History

Large message histories can be expensive to query. Use pagination and limit the maximum page size:

```go
// In handlers.go, cap the limit parameter
limit := 50
if l := c.Query("limit"); l != "" {
    fmt.Sscanf(l, "%d", &limit)
    if limit > 100 {
        limit = 100  // Hard cap
    }
}
```

---

## Monitoring Rate Limit Usage

### Server Metrics

Track these metrics to understand usage patterns:

| Metric | Type | Labels |
|--------|------|--------|
| `copcon_rate_limit_rejected_total` | Counter | `endpoint`, `client_ip` |
| `copcon_llm_calls_total` | Counter | `model`, `agent_id` |
| `copcon_llm_tokens_total` | Counter | `model`, `type` (prompt/completion) |
| `copcon_active_sessions` | Gauge | |
| `copcon_chat_duration_seconds` | Histogram | `agent_id` |

### Alerting Thresholds

| Alert | Condition | Action |
|-------|-----------|--------|
| High rejection rate | `rejected_total / requests_total > 0.1` | Increase limits or investigate abuse |
| LLM quota approaching | `tokens_total > 80% of provider limit` | Scale back concurrent sessions |
| Long chat durations | `p99(duration) > 60s` | Check LLM provider latency |

---

## Reverse Proxy Rate Limiting

If you run CopCon behind a reverse proxy (nginx, Envoy, Cloudflare), you can offload rate limiting there.

### nginx

```nginx
limit_req_zone $binary_remote_addr zone=api:10m rate=10r/m;
limit_req_zone $binary_remote_addr zone=chat:10m rate=6r/m;

server {
    location /api/sessions/ {
        limit_req zone=api burst=20 nodelay;
        proxy_pass http://copcon:8080;
    }

    location /api/sessions/*/chat {
        limit_req zone=chat burst=3 nodelay;
        proxy_pass http://copcon:8080;
    }
}
```

### Cloudflare

Configure rate limiting rules in the Cloudflare dashboard:

- Rule: Path starts with `/api/sessions` and contains `/chat`
- Rate: 10 requests per minute per IP
- Action: Block for 60 seconds

---

## Cost Control

Rate limiting is your primary tool for cost control with LLM providers.

### Estimating Costs

```
Cost per chat = sum(steps) × (prompt_tokens × input_price + completion_tokens × output_price)
```

For GPT-4o (as of 2026):
- Input: $2.50 / 1M tokens
- Output: $10.00 / 1M tokens

A typical agent loop with 3 steps, averaging 2,000 prompt tokens and 500 completion tokens per step:

```
3 × (2000 × $0.0000025 + 500 × $0.00001) = 3 × $0.01 = $0.03 per chat
```

### Budget Enforcement

Set daily or monthly budget caps at the application level:

```go
type BudgetEnforcer struct {
    dailySpend   atomic.Int64
    dailyLimit   int64  // in cents
    resetOn      time.Time
}

func (b *BudgetEnforcer) Middleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        if time.Now().After(b.resetOn) {
            b.dailySpend.Store(0)
            b.resetOn = time.Now().AddDate(0, 0, 1)
        }
        if b.dailySpend.Load() >= b.dailyLimit {
            c.AbortWithStatusJSON(503, gin.H{"error": "daily budget exceeded"})
            return
        }
        c.Next()
    }
}
```

Track spending by recording token usage from LLM responses and accumulating costs per API key or tenant.
