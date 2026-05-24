# Security

This guide covers authentication, authorization, input validation, rate limiting, encryption, and hardening for the CopCon server in production.

## Current State

The server ships without authentication enabled. The `gin.Default()` setup has no auth middleware, and all API endpoints are publicly accessible. This is intentional for the reference implementation. Before deploying to any shared environment, add the controls described here.

## Authentication

### API Key Authentication

The simplest approach for service-to-service communication. Each client gets a unique key passed via the `X-API-Key` header.

**Middleware:**

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

**Setup:**

```go
keys := map[string]bool{
    os.Getenv("API_KEY_WEB"):    true,
    os.Getenv("API_KEY_MOBILE"): true,
}

r.Use(APIKeyAuth(keys))
```

**Key rotation:** Support multiple valid keys simultaneously. Add the new key, deploy, then remove the old key in a follow-up deploy. This avoids downtime.

**Key storage:** Never hardcode keys in config files. Use environment variables or a secrets manager (AWS Secrets Manager, HashiCorp Vault).

### JWT Authentication

For user-facing applications where you need identity, roles, and session expiry.

**Middleware:**

```go
import "github.com/golang-jwt/jwt/v5"

func JWTAuth(secret string) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "missing authorization header",
            })
            return
        }

        tokenString := strings.TrimPrefix(authHeader, "Bearer ")
        if tokenString == authHeader {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "invalid authorization format, expected Bearer token",
            })
            return
        }

        token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
            if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
            }
            return []byte(secret), nil
        })

        if err != nil || !token.Valid {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "invalid token",
            })
            return
        }

        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "invalid token claims",
            })
            return
        }

        // Pass identity to handlers
        c.Set("user_id", claims["sub"])
        c.Set("roles", claims["roles"])
        c.Next()
    }
}
```

**Token issuance** is not handled by the CopCon server. It should be done by your identity provider (Auth0, Keycloak, Firebase Auth) or a dedicated auth service.

### OAuth 2.0

For third-party login flows, implement the OAuth 2.0 authorization code flow:

1. Redirect users to the OAuth provider's consent screen.
2. The provider redirects back with an authorization code.
3. Exchange the code for an access token.
4. Validate the token on each request (using JWT middleware above).

Libraries like `golang.org/x/oauth2` handle the token exchange. The CopCon server only needs to validate the resulting JWT.

## Authorization and RBAC

### Role-Based Access Control

Once you have user identity (from JWT claims), enforce permissions:

```go
type Role string

const (
    RoleAdmin  Role = "admin"
    RoleUser   Role = "user"
    RoleViewer Role = "viewer"
)

func RequireRole(allowedRoles ...Role) gin.HandlerFunc {
    roleSet := make(map[Role]bool)
    for _, r := range allowedRoles {
        roleSet[r] = true
    }

    return func(c *gin.Context) {
        rolesVal, exists := c.Get("roles")
        if !exists {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "no roles found"})
            return
        }

        roles, ok := rolesVal.([]interface{})
        if !ok {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid roles format"})
            return
        }

        for _, r := range roles {
            if roleSet[Role(r.(string))] {
                c.Next()
                return
            }
        }

        c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
    }
}
```

**Usage:**

```go
sessions.DELETE("/:sessionId",
    JWTAuth(secret),
    RequireRole(RoleAdmin),
    handler.DeleteSession,
)
```

### Session Ownership

For multi-user setups, verify that a user can only access their own sessions:

```go
func SessionOwnership(sessionStore storage.SessionStore) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID, _ := c.Get("user_id")
        sessionID := c.Param("sessionId")

        sessUUID, err := uuid.Parse(sessionID)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
            return
        }

        sess, err := sessionStore.Get(c.Request.Context(), sessUUID)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "session not found"})
            return
        }

        ownerID, ok := sess.Metadata["owner_id"].(string)
        if ok && ownerID != userID.(string) {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not your session"})
            return
        }

        c.Next()
    }
}
```

## Input Validation and Sanitization

### Request Body Validation

Use Gin's `ShouldBindJSON` with struct tags for validation:

```go
type CreateSessionRequest struct {
    Title          string `json:"title" binding:"max=100"`
    DefaultAgentID string `json:"default_agent_id" binding:"omitempty,alphanum"`
}
```

Common binding tags:

| Tag | Purpose |
|-----|---------|
| `required` | Field must be present and non-empty |
| `omitempty` | Skip validation if empty |
| `min=N`, `max=N` | String length or numeric range |
| `alphanum` | Only alphanumeric characters |
| `email` | Valid email format |
| `oneof=a b c` | Must be one of the listed values |
| `uuid` | Valid UUID format |

### UUID Validation

Always validate UUID path parameters before using them:

```go
sessionID := c.Param("sessionId")
sessUUID, err := uuid.Parse(sessionID)
if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
    return
}
```

### Content Length Limits

Cap the size of user messages to prevent abuse:

```go
const maxContentLength = 10000

if len(req.Content) > maxContentLength {
    c.JSON(http.StatusBadRequest, gin.H{
        "error": fmt.Sprintf("content exceeds maximum length of %d characters", maxContentLength),
    })
    return
}
```

### SQL Injection

The server uses GORM, which parameterizes queries by default. As long as you use GORM's query builders and don't pass raw strings into `db.Raw()` or `db.Exec()` with string concatenation, you're protected.

```go
// Safe: GORM parameterizes this
db.Where("session_id = ?", sessionID).Find(&messages)

// Dangerous: raw string interpolation
db.Raw("SELECT * FROM messages WHERE session_id = " + sessionID) // NEVER do this
```

### XSS Prevention

The server returns JSON, not HTML. XSS is primarily a client-side concern. However, you should still sanitize user input before storing it to prevent stored XSS when the content is later rendered in a web UI.

For the API layer, the simplest defense is setting the `Content-Type: application/json` header (which Gin does automatically), preventing browsers from interpreting responses as HTML.

## Rate Limiting and DDoS Protection

### Per-IP Rate Limiting

See [Middleware: Rate Limiting](./middleware.md#rate-limiting) for implementation details. Recommended limits:

| Endpoint | Rate Limit | Burst |
|----------|-----------|-------|
| `POST /api/sessions` | 10 req/s | 20 |
| `POST /api/sessions/:id/chat` | 2 req/s | 5 |
| `GET /api/sessions` | 20 req/s | 40 |
| All other endpoints | 20 req/s | 40 |

Chat is the most expensive endpoint because it triggers LLM API calls. Limit it aggressively.

### LLM API Cost Protection

Each chat request costs money. Protect against runaway usage:

```go
func TokenBudgetMiddleware(sessionStore storage.SessionStore, maxTokensPerSession int) gin.HandlerFunc {
    return func(c *gin.Context) {
        sessionID := c.Param("sessionId")
        sessUUID, err := uuid.Parse(sessionID)
        if err != nil {
            c.Next()
            return
        }

        // Count existing messages to estimate token usage
        msgs, err := sessionStore.List(c.Request.Context(), sessUUID, 0)
        if err != nil {
            c.Next()
            return
        }

        // Simple heuristic: estimate 500 tokens per message
        estimatedTokens := len(msgs) * 500
        if estimatedTokens > maxTokensPerSession {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "error": "session token budget exceeded",
            })
            return
        }

        c.Next()
    }
}
```

### Load Balancer Level

For serious DDoS protection, handle rate limiting at the infrastructure level:

- **Cloudflare**: Enable rate limiting rules in the dashboard.
- **AWS ALB + WAF**: Use AWS WAF rate-based rules.
- **Nginx**: Add `limit_req` directives.

```nginx
limit_req_zone $binary_remote_addr zone=api:10m rate=20r/s;

location /api/ {
    limit_req zone=api burst=40 nodelay;
    proxy_pass http://server;
}
```

## Encryption

### In Transit (TLS)

Never run the CopCon server without TLS in production. Use a reverse proxy to terminate TLS:

```nginx
server {
    listen 443 ssl http2;
    server_name api.example.com;

    ssl_certificate     /etc/ssl/certs/api.example.com.pem;
    ssl_certificate_key /etc/ssl/private/api.example.com.key;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    location / {
        proxy_pass http://127.0.0.1:8088;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE support
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 300s;
    }
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name api.example.com;
    return 301 https://$host$request_uri;
}
```

For container environments, use an ingress controller (Kubernetes Ingress, AWS ALB) that handles TLS termination.

### At Rest

**PostgreSQL encryption:**

- Enable Transparent Data Encryption (TDE) in PostgreSQL if your hosting supports it.
- Use encrypted EBS volumes on AWS, or encrypted disks on GCP/Azure.
- For self-hosted Postgres, use LUKS full-disk encryption on the data volume.

**Backup encryption:**

```bash
pg_dump -h db-host -U admin -d copcon | gpg --symmetric --cipher-algo AES256 -o backup.gpg
```

**S3 storage for backups:** Enable server-side encryption (SSE-S3 or SSE-KMS).

## Security Headers

Add security headers via middleware:

```go
func SecurityHeadersMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-XSS-Protection", "1; mode=block")
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
        c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        c.RemoveHeader("Server") // Don't advertise Gin version
        c.Next()
    }
}
```

Apply globally:

```go
r.Use(SecurityHeadersMiddleware())
```

## Compliance

### GDPR

If you process personal data of EU residents:

1. **Data minimization.** Only store what you need. Don't log full user messages at INFO level.
2. **Right to deletion.** Implement a data deletion endpoint that removes a user's sessions, messages, and todos.
3. **Consent.** Track when users consented to data processing. Store this in session metadata.
4. **Data export.** Provide an endpoint that exports all data for a given user.
5. **Retention.** Set up automated cleanup of old sessions.

```go
// Example: delete all data for a user
func DeleteUserData(sessionStore storage.SessionStore, messageStore storage.MessageStore, userID string) error {
    // Query sessions owned by this user
    // Delete messages, then sessions
    // Delete from Qdrant if memory is enabled
    return nil
}
```

### SOC 2

For SOC 2 compliance:

1. **Access control.** All API access must be authenticated and authorized.
2. **Audit logging.** Log all administrative actions (session creation, deletion, config changes).
3. **Encryption.** TLS in transit, encryption at rest.
4. **Incident response.** Have an alert on abnormal access patterns (e.g., a single IP hitting the chat endpoint 100 times in a minute).
5. **Change management.** All config changes go through version control and review.

## Security Audit Checklist

Run through this checklist before deploying to production.

### Authentication

- [ ] Authentication middleware is enabled on all `/api/*` routes
- [ ] API keys are stored in environment variables or a secrets manager, not in config files
- [ ] JWT secret is at least 256 bits (32 bytes) of random data
- [ ] Token expiry is configured (recommend 24 hours or less)
- [ ] Refresh token rotation is implemented if using JWT

### Authorization

- [ ] Users can only access their own sessions (session ownership check)
- [ ] Admin-only endpoints (e.g., delete all sessions) require an admin role
- [ ] Agent configuration changes are restricted to admins

### Input Validation

- [ ] All path parameters are validated (UUID format for session IDs)
- [ ] Request body size is capped (100KB recommended)
- [ ] User message content has a maximum length
- [ ] All JSON inputs are validated with `ShouldBindJSON` and struct tags

### Network

- [ ] TLS is enabled (via reverse proxy or ingress)
- [ ] The server listens on `127.0.0.1` when behind a proxy, not `0.0.0.0`
- [ ] CORS is configured with specific origins, not `*`
- [ ] Rate limiting is applied, especially on the chat endpoint

### Database

- [ ] Database password is not the default (`changeme`)
- [ ] SSL is enabled between the server and Postgres (or they're on the same private network)
- [ ] The Postgres user is not a superuser
- [ ] Connection pool limits are set to prevent connection exhaustion

### Logging and Monitoring

- [ ] User message content is not logged at INFO or above
- [ ] API keys and tokens are redacted from logs
- [ ] Failed authentication attempts are logged
- [ ] An alert fires on unusual error rates or rate limit breaches

### Infrastructure

- [ ] Config files don't contain secrets
- [ ] Docker images use non-root users
- [ ] Health check endpoint is accessible without auth (for load balancers)
- [ ] Backups are encrypted and tested for restore
- [ ] Container resource limits are set (CPU, memory)

## Docker Hardening

The current Dockerfile runs as root. For production, add a non-root user:

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:3.19
WORKDIR /app

# Create non-root user
RUN adduser -D -u 1000 appuser

COPY --from=builder /app/server .
COPY config.yaml .

# Set ownership and switch user
RUN chown -R appuser:appuser /app
USER appuser

EXPOSE 8080
CMD ["./server"]
```

## Securing LLM Tool Execution

The server can give agents access to tools like `code_executor` and `shell_executor`. These are powerful and dangerous.

### Tool Sandboxing

The core library's `code_executor` and `shell_executor` run in isolated containers. Ensure:

1. **Network isolation.** Tool containers have no network access (`network: none`).
2. **Resource limits.** Cap CPU, memory, and execution time.
3. **Filesystem restrictions.** The sandbox filesystem is ephemeral and read-only except for a writable `/tmp`.
4. **No privilege escalation.** Containers run as non-root with `no-new-privileges`.

### Tool Whitelisting

Not every agent needs every tool. Configure tools per agent in `config.yaml`:

```yaml
agents:
  - id: "chat-assistant"
    tools: []  # No tools, text-only conversations

  - id: "code-assistant"
    tools: ["code_executor", "file_ops", "todolist"]  # No shell_executor
```

## Incident Response

### Detecting Abuse

Watch for these patterns:

- A single IP exceeding rate limits consistently
- Sessions with abnormally high message counts
- Chat requests with extremely long content (buffer overflow attempts)
- Requests with unusual headers or malformed JSON (injection attempts)

### Responding to a Breach

1. **Revoke compromised credentials.** Rotate API keys and JWT secrets immediately.
2. **Block offending IPs.** Add them to your WAF or load balancer deny list.
3. **Audit recent activity.** Check logs for unauthorized session access.
4. **Notify affected users.** If user data was accessed, follow your disclosure policy.
5. **Patch the vulnerability.** Deploy the fix and verify it's effective.

## Next Steps

- [Middleware](./middleware.md) for auth middleware implementations
- [Configuration](./configuration.md) for secure config practices
- [Logging](./logging.md) for audit logging and redaction