# Session 级中间件

## 概述

Gin 中间件在 HTTP 请求层面运行，适用于路由级别的鉴权、限流、日志等。但当需要 "按 Session 维度" 实施差异化逻辑时（如每会话独立的速率限制、权限校验），需要在中间件中获取 Session 信息。

本章介绍如何将 Session 级别的策略集成到 Gin 中间件链中，并对比 Session 中间件与 Hook 的适用场景。

## 中间件 vs Hook

| 维度 | Gin 中间件 | Hook |
|------|-----------|------|
| 执行位置 | HTTP 层面，路由前/后 | Agent 引擎内部，生命周期节点 |
| 触发时机 | 请求到达时 | LLM 调用前后、工具执行前后等 |
| 可访问数据 | HTTP Request、路由参数 | ChatContext、Session、Messages、ToolArgs |
| 用途 | 认证、限流、日志、CORS | 业务逻辑注入、消息增强、指标收集 |
| 是否能拒绝请求 | 是（返回 4xx/5xx） | 否（Hook 错误只记录日志，不中断流程） |

**简单判断：**
- 需要在 Agent 处理前就决定是否允许请求 → Gin 中间件
- 需要在 Agent 处理过程中注入逻辑 → Hook

## Session 中间件实现

### 场景：按 Session 维度做速率限制

需求：每个 Session 每分钟最多 10 次 Chat 请求。超过限制返回 429。

```go
package api

import (
    "sync"
    "time"

    "github.com/gin-gonic/gin"
    "golang.org/x/time/rate"
)

// SessionRateLimiter 按 Session ID 管理速率限制器
type SessionRateLimiter struct {
    mu       sync.Mutex
    limiters map[string]*rateLimiterEntry
}

type rateLimiterEntry struct {
    limiter  *rate.Limiter
    lastUsed time.Time
}

func NewSessionRateLimiter() *SessionRateLimiter {
    rl := &SessionRateLimiter{
        limiters: make(map[string]*rateLimiterEntry),
    }

    // 后台清理过期条目（每 5 分钟）
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        for range ticker.C {
            rl.cleanup(10 * time.Minute)
        }
    }()

    return rl
}

func (rl *SessionRateLimiter) Allow(sessionID string, ratePerMin int, burst int) bool {
    rl.mu.Lock()
    entry, exists := rl.limiters[sessionID]
    if !exists {
        // rate.Limit: ratePerMin 次 / 60 秒
        entry = &rateLimiterEntry{
            limiter: rate.NewLimiter(rate.Limit(float64(ratePerMin)/60.0), burst),
        }
        rl.limiters[sessionID] = entry
    }
    entry.lastUsed = time.Now()
    rl.mu.Unlock()

    return entry.limiter.Allow()
}

func (rl *SessionRateLimiter) cleanup(maxAge time.Duration) {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    for id, entry := range rl.limiters {
        if time.Since(entry.lastUsed) > maxAge {
            delete(rl.limiters, id)
        }
    }
}
```

### 集成到 Gin 中间件链

```go
func SessionRateLimitMiddleware(limiter *SessionRateLimiter) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 仅对 Chat 端点生效
        if c.Request.URL.Path != "/api/sessions/:sessionId/chat" {
            c.Next()
            return
        }

        sessionID := c.Param("sessionId")
        if sessionID == "" {
            c.JSON(400, gin.H{"error": "missing session id"})
            c.Abort()
            return
        }

        // 每分钟 10 次，burst 5
        if !limiter.Allow(sessionID, 10, 5) {
            c.JSON(429, gin.H{
                "error":       "rate limit exceeded",
                "retry_after": "60",
            })
            c.Abort()
            return
        }

        c.Next()
    }
}
```

### 注册中间件

```go
// main.go
func main() {
    // ...

    r := gin.Default()

    // 注册 Session 级限流中间件
    rateLimiter := api.NewSessionRateLimiter()
    r.Use(api.SessionRateLimitMiddleware(rateLimiter))

    api.SetupRoutes(r, cfg, sessionMgr, todoMgr, agentEngine, agentRegistry)

    // ...
}
```

## 完整示例：认证 + 授权中间件

```go
// AuthMiddleware 验证 API Key 并注入 Session 上下文
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        apiKey := c.GetHeader("X-API-Key")
        if apiKey == "" {
            c.JSON(401, gin.H{"error": "missing API key"})
            c.Abort()
            return
        }

        // 验证 API Key 并获取用户信息
        user, err := validateAPIKey(apiKey)
        if err != nil {
            c.JSON(403, gin.H{"error": "invalid API key"})
            c.Abort()
            return
        }

        // 注入用户信息到 Gin Context
        c.Set("user_id", user.ID)
        c.Set("user_tier", user.Tier)

        c.Next()
    }
}

// SessionOwnershipMiddleware 验证当前用户是否拥有该 Session
func SessionOwnershipMiddleware(sessionMgr session.SessionManager) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetString("user_id")

        // 创建 Session 时不检查所有权
        if c.Request.Method == "POST" && c.Request.URL.Path == "/api/sessions" {
            c.Next()
            return
        }

        sessionID := c.Param("sessionId")
        if sessionID == "" {
            c.Next()
            return
        }

        chatCtx := iface.NewChatContext(c.Request.Context(), sessionID, "")
        sess, err := sessionMgr.Get(chatCtx)
        if err != nil {
            c.JSON(404, gin.H{"error": "session not found"})
            c.Abort()
            return
        }

        // Session Metadata 中存储了 owner 信息（需在创建时写入）
        ownerID, _ := sess.Metadata["owner_id"].(string)
        if ownerID != "" && ownerID != userID {
            c.JSON(403, gin.H{"error": "access denied"})
            c.Abort()
            return
        }

        c.Next()
    }
}
```

### 中间件链注册

```go
r := gin.Default()

// 中间件执行顺序从上到下
r.Use(
    AuthMiddleware(),                                  // 1. 验证身份
    api.SessionRateLimitMiddleware(rateLimiter),       // 2. 按 Session 限流
    SessionOwnershipMiddleware(sessionMgr),            // 3. 验证所有权
)

api.SetupRoutes(r, cfg, sessionMgr, todoMgr, agentEngine, agentRegistry)
```

## 何时使用 Session 中间件

| 场景 | 推荐方案 |
|------|---------|
| API Key 验证 | Gin 中间件 |
| 按 Session 限流 | Gin 中间件 |
| Session 所有权校验 | Gin 中间件 |
| 修改消息内容 | Hook (`OnMessagePersist`) |
| 注入系统提示词 | Hook (`OnSystemPrompt`) |
| 工具调用前后处理 | Hook (`BeforeToolExecute` / `AfterToolExecute`) |
| 修改 LLM 请求参数 | Hook (`BeforeLLMCall`) |
| 记录 LLM 调用指标 | Hook (`AfterLLMCall`) |
| 请求日志 | Gin 中间件 + slog |

## 注意事项

1. **中间件顺序很重要：** 先认证，再限流，再所有权检查。顺序错可能导致绕过检查。
2. **Hook 不能替代中间件：** Hook 在 Agent 引擎启动后才执行，此时请求已进入处理流程。拒绝请求必须在中间件中完成。
3. **ChatContext 在中间件中的使用：** 中间件中可以创建临时的 ChatContext 用于数据库查询（如 Session 所有权校验），但不要长期持有。
4. **清理过期数据：** 中间件中维护的 Session 级别状态（如限流计数器）需要定期清理，避免内存泄漏。