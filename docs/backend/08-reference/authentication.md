# Authentication

CopCon supports optional authentication for API access. When authentication is not configured, all endpoints are publicly accessible. This is suitable for development and internal deployments where network-level access control is sufficient.

---

## Authentication Modes

| Mode | Config Value | Header | Use Case |
|------|-------------|--------|----------|
| None (default) | `authentication.enabled: false` | None | Development, internal networks |
| API Key | `authentication.mode: api_key` | `X-API-Key` | Service-to-service, simple setups |
| JWT | `authentication.mode: jwt` | `Authorization: Bearer <token>` | Multi-user, production |

---

## Configuration

Authentication is configured in `config.yaml`:

```yaml
authentication:
  enabled: true
  mode: jwt          # "jwt" or "api_key"
  key: your-secret   # For api_key mode: the expected key value
  jwt_secret: your-jwt-secret   # For jwt mode: signing key
  jwt_expiry: 86400              # For jwt mode: token lifetime in seconds
```

Environment variable overrides:

```bash
AUTH_MODE=jwt
AUTH_JWT_SECRET=your-jwt-secret
AUTH_API_KEY=your-api-key
```

---

## API Key Authentication

The simplest authentication mode. All requests must include a static key in the `X-API-Key` header.

### Setup

```yaml
authentication:
  enabled: true
  mode: api_key
  key: "sk-copcon-abc123xyz"
```

### Usage

```bash
curl http://localhost:8080/api/sessions \
  -H "X-API-Key: sk-copcon-abc123xyz"
```

### Validation Rules

- The key is compared as an exact string match
- Key must be non-empty when `mode: api_key`
- Requests without the header or with an incorrect key receive `401 Unauthorized`
- The key applies to all `/api/*` endpoints; `/health` remains unauthenticated

### Key Rotation

To rotate an API key:

1. Add the new key to configuration
2. Restart or signal the server to reload config
3. Update all clients to use the new key
4. Remove the old key

For zero-downtime rotation, support multiple keys by configuring a comma-separated list:

```yaml
authentication:
  mode: api_key
  key: "old-key,new-key"
```

---

## JWT Authentication

Token-based authentication using JSON Web Tokens. Suitable for multi-user and production deployments.

### Obtaining a Token

```
POST /auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "secret"
}
```

**Response:**

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 86400
}
```

### Using the Token

```bash
curl http://localhost:8080/api/sessions \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

### Token Format

CopCon uses standard JWT claims:

| Claim | Description |
|-------|-------------|
| `sub` | User identifier |
| `exp` | Expiration timestamp |
| `iat` | Issued-at timestamp |
| `iss` | Issuer (`copcon`) |

### Token Refresh

Tokens expire based on `jwt_expiry` (default: 86400 seconds = 24 hours). Before expiration, obtain a new token via `POST /auth/login`.

Recommended approach:

1. Store the token and its expiration time locally
2. Check expiration before each request
3. If expired or about to expire (within 5 minutes), fetch a new token
4. Retry the original request with the new token

```python
import time
import requests

class AuthenticatedClient:
    def __init__(self, base_url, username, password):
        self.base_url = base_url
        self.username = username
        self.password = password
        self.token = None
        self.token_expiry = 0

    def _ensure_token(self):
        if self.token and time.time() < self.token_expiry - 300:
            return
        resp = requests.post(f"{self.base_url}/auth/login", json={
            "username": self.username,
            "password": self.password
        })
        resp.raise_for_status()
        data = resp.json()
        self.token = data["token"]
        self.token_expiry = time.time() + data["expires_in"]

    def request(self, method, path, **kwargs):
        self._ensure_token()
        headers = kwargs.pop("headers", {})
        headers["Authorization"] = f"Bearer {self.token}"
        return requests.request(method, f"{self.base_url}{path}",
                                headers=headers, **kwargs)
```

---

## Multi-Tenant Authentication

For multi-tenant deployments, the JWT token can carry tenant information:

```json
{
  "sub": "user-123",
  "tenant": "acme-corp",
  "role": "admin",
  "exp": 1716700000
}
```

Tenant isolation is enforced at the storage layer. Sessions created under one tenant are not visible to another. The server extracts the tenant ID from the JWT claims and filters all database queries accordingly.

### Tenant-Aware Request Flow

1. Client authenticates and receives a JWT with `tenant` claim
2. All subsequent requests include the JWT in the `Authorization` header
3. The server middleware extracts the tenant ID
4. Storage queries are automatically scoped to the tenant
5. Cross-tenant access returns `404 Not Found` (not `403 Forbidden`, to avoid information leakage)

---

## Service-to-Service Authentication

For backend services communicating with CopCon:

### API Key (Recommended)

```yaml
# Service A's configuration
copcon:
  base_url: "http://copcon-internal:8080"
  api_key: "sk-copcon-service-a"
```

```go
req.Header.Set("X-API-Key", "sk-copcon-service-a")
```

### JWT with Service Account

For services that need longer-lived tokens:

1. Create a service account with a shared secret
2. Generate a long-lived JWT (30-day expiry)
3. Include the service account ID in the `sub` claim
4. Store the token securely (environment variable, secret manager)

```go
// Generating a service token
token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
    "sub": "service-pipeline",
    "iss": "copcon",
    "exp": time.Now().Add(30 * 24 * time.Hour).Unix(),
})
tokenString, _ := token.SignedString([]byte(jwtSecret))
```

---

## Unauthenticated Endpoints

Regardless of authentication configuration:

| Endpoint | Auth Required |
|----------|--------------|
| `GET /health` | No |
| `POST /auth/login` | No |
| All `/api/*` endpoints | Yes (when auth enabled) |

---

## Security Best Practices

1. **Always use HTTPS in production.** Authentication headers are sent in cleartext over HTTP.
2. **Rotate secrets regularly.** Change API keys and JWT secrets on a defined schedule.
3. **Use strong secrets.** At least 32 characters for API keys, at least 256 bits for JWT signing keys.
4. **Set short token lifetimes.** Use the shortest practical `jwt_expiry` for your use case.
5. **Validate tokens on every request.** Don't cache authentication results beyond the request scope.
6. **Log authentication failures.** Monitor for brute-force attempts on the login endpoint.
7. **Rate limit the login endpoint.** Prevent credential stuffing attacks.
8. **Don't commit secrets.** Use environment variables or secret management tools, not config files in version control.

---

## Current Implementation Status

The current CopCon server does not include built-in authentication middleware. The `config.yaml` fields for authentication are defined but the middleware is not wired into the Gin router by default. To add authentication:

1. Implement a Gin middleware that validates `X-API-Key` or `Authorization` headers
2. Register the middleware on the `/api` route group in `SetupRoutes`
3. Add the `/auth/login` endpoint for JWT token issuance

Example middleware skeleton:

```go
func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
    if !cfg.Authentication.Enabled {
        return func(c *gin.Context) { c.Next() }
    }

    return func(c *gin.Context) {
        switch cfg.Authentication.Mode {
        case "api_key":
            key := c.GetHeader("X-API-Key")
            if key != cfg.Authentication.Key {
                c.AbortWithStatusJSON(401, gin.H{"error": "invalid API key"})
                return
            }
        case "jwt":
            auth := c.GetHeader("Authorization")
            if !strings.HasPrefix(auth, "Bearer ") {
                c.AbortWithStatusJSON(401, gin.H{"error": "missing token"})
                return
            }
            tokenStr := strings.TrimPrefix(auth, "Bearer ")
            token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
                return []byte(cfg.Authentication.JWTSecret), nil
            })
            if err != nil || !token.Valid {
                c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
                return
            }
            c.Set("user", token.Claims)
        }
        c.Next()
    }
}
```

Register in `SetupRoutes`:

```go
func SetupRoutes(r *gin.Engine, cfg *config.Config, h core.APIProvider) {
    handler := NewHandler(cfg, h)
    api := r.Group("/api")
    if cfg.Authentication.Enabled {
        api.Use(AuthMiddleware(cfg))
    }
    // ... register routes
}
```
