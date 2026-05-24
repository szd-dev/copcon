# Configuration

The CopCon server reads its settings from a YAML file, with select overrides from environment variables. This page covers every config field, how precedence works, and example setups for different environments.

## Config File Location

By default, the server loads `config.yaml` from its working directory. Point it elsewhere with the `CONFIG_PATH` environment variable:

```bash
CONFIG_PATH=/etc/copcon/config.yaml ./server
```

In Docker, the compose file mounts your config at `/app/config.yaml` and sets `CONFIG_PATH=/app/config.yaml`.

## Full Reference

Here is every field the server recognizes, with defaults where applicable.

```yaml
server:
  port: "8088"           # TCP port to bind (string, not integer)

database:
  host: "localhost"      # PostgreSQL host
  port: 5432             # PostgreSQL port (integer)
  user: "admin"          # Database user
  password: "changeme"   # Database password
  dbname: "copcon"       # Database name

openai:
  api_key: ""            # OpenAI-compatible API key
  base_url: "https://api.openai.com/v1"  # API base URL
  model: "gpt-4o"        # Default model name

qdrant:
  host: "localhost"      # Qdrant vector DB host
  port: 6333             # Qdrant gRPC port

default_agent_id: ""     # Which agent is selected when clients omit agent_id

agents:                  # List of agent definitions
  - id: "code-assistant"
    name: "Code Assistant"
    model: "gpt-4o"      # Overrides openai.model for this agent
    system_prompt: "You are a helpful coding assistant."
    tools: ["code_executor", "shell_executor", "file_ops", "todolist"]
    base_url: ""         # Per-agent API endpoint override (optional)
```

### Field Details

#### server

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | string | `"8088"` | The port the HTTP server listens on. Stored as a string because Gin's `Run()` accepts a string argument. |

#### database

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `"localhost"` | PostgreSQL host. Use the service name (`postgres`) when running in Docker Compose. |
| `port` | integer | `5432` | PostgreSQL port. |
| `user` | string | `"admin"` | Database user. |
| `password` | string | `"changeme"` | Database password. Always override this in production via environment variable. |
| `dbname` | string | `"copcon"` | Database name. Must exist before the server starts. |

The server constructs a DSN internally:

```
host=<host> port=<port> user=<user> password=<password> dbname=<dbname> sslmode=disable
```

Currently `sslmode` is hardcoded to `disable`. To enable SSL, extend the `DatabaseConfig.DSN()` method in `server/internal/config/config.go`.

#### openai

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `api_key` | string | `""` | API key for the OpenAI-compatible endpoint. Override with `OPENAI_API_KEY` env var. |
| `base_url` | string | `"https://api.openai.com/v1"` | Base URL for the LLM API. Change this to point at LiteLLM, Azure, or any OpenAI-compatible provider. |
| `model` | string | `"gpt-4o"` | Default model sent to the API. Individual agents can override this. |

#### qdrant

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `"localhost"` | Qdrant host. Memory features are optional; if Qdrant is unavailable, the memory hook skips registration. |
| `port` | integer | `6333` | Qdrant gRPC port. |

#### agents

Each entry in the `agents` list defines an agent the server can dispatch.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique identifier. Used in API requests to select this agent. No duplicates allowed. |
| `name` | string | yes | Human-readable name. |
| `model` | string | yes | LLM model to use for this agent. |
| `system_prompt` | string | yes | System prompt injected into every conversation this agent handles. |
| `tools` | []string | no | List of built-in tool names to enable. Options: `code_executor`, `shell_executor`, `file_ops`, `todolist`. |
| `base_url` | string | no | Per-agent API endpoint override. Falls back to `openai.base_url` if empty. |

The `default_agent_id` field must match the `id` of one of the agents listed, or be empty. If it is set to a nonexistent ID, the server refuses to start.

## Environment Variable Overrides

Environment variables take precedence over the config file. This lets you keep secrets out of YAML.

| Variable | Overrides | Notes |
|----------|-----------|-------|
| `CONFIG_PATH` | Config file path | Defaults to `config.yaml` |
| `OPENAI_API_KEY` | `openai.api_key` | Always prefer this over putting the key in YAML |
| `DATABASE_HOST` | (not yet wired) | Planned; currently only `docker-compose.yaml` passes it as a convention |
| `DATABASE_PORT` | (not yet wired) | Planned |
| `DATABASE_USER` | (not yet wired) | Planned |
| `DATABASE_PASSWORD` | (not yet wired) | Planned |
| `DATABASE_DBNAME` | (not yet wired) | Planned |

> **Note:** The current config loader only wires `CONFIG_PATH` and `OPENAI_API_KEY` as env var overrides. The `DATABASE_*` variables shown in `docker-compose.yaml` are placeholders that need to be wired into `config.Load()` by extending the override block. This is a straightforward change: add `if v := os.Getenv("DATABASE_HOST"); v != "" { cfg.Database.Host = v }` and so on for each field.

## Configuration Precedence

From highest to lowest:

1. **Environment variables** (`OPENAI_API_KEY`, etc.)
2. **Config file** (`config.yaml`)
3. **Hardcoded defaults** (in Go struct field tags)

The loader reads the file first, then applies any non-empty env vars on top.

## Validation Rules

The config loader validates these rules at startup:

1. **No duplicate agent IDs.** Two agents with `id: "assistant"` will cause the server to exit with an error.
2. **Default agent must exist.** If `default_agent_id` is set and no agent with that ID exists, startup fails.
3. **Required fields.** The server will fail to connect if `database.*` fields are wrong, or if `openai.api_key` is missing and the `OPENAI_API_KEY` env var is not set.

Validation happens in `config.Load()` before the server opens any connections.

## Example Configs

### Development

Minimal setup for local hacking. Uses default Postgres credentials and a local LLM proxy.

```yaml
server:
  port: "8088"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "copcon"

openai:
  api_key: "dev-key"
  base_url: "http://localhost:4000/v1"
  model: "gpt-4o"

qdrant:
  host: "localhost"
  port: 6333

default_agent_id: "code-assistant"

agents:
  - id: "code-assistant"
    name: "Code Assistant"
    model: "gpt-4o"
    system_prompt: "You are a coding assistant. Write clean, tested code."
    tools: ["code_executor", "shell_executor", "file_ops", "todolist"]
```

### Staging

Staging mirrors production but with less aggressive pooling and debug-friendly logging.

```yaml
server:
  port: "8088"

database:
  host: "staging-db.internal"
  port: 5432
  user: "copcon_app"
  password: "${DATABASE_PASSWORD}"
  dbname: "copcon_staging"

openai:
  api_key: "${OPENAI_API_KEY}"
  base_url: "https://api.openai.com/v1"
  model: "gpt-4o"

qdrant:
  host: "staging-qdrant.internal"
  port: 6333

default_agent_id: "code-assistant"

agents:
  - id: "code-assistant"
    name: "Code Assistant"
    model: "gpt-4o"
    system_prompt: "You are a coding assistant in a staging environment. Be cautious with destructive operations."
    tools: ["code_executor", "file_ops", "todolist"]
  - id: "chat-assistant"
    name: "Chat Assistant"
    model: "gpt-4o"
    system_prompt: "You are a friendly chat assistant."
    tools: []
```

### Production

Production config uses environment variables for all secrets and runs behind a load balancer.

```yaml
server:
  port: "8080"

database:
  host: "${DATABASE_HOST}"
  port: 5432
  user: "${DATABASE_USER}"
  password: "${DATABASE_PASSWORD}"
  dbname: "${DATABASE_DBNAME}"

openai:
  api_key: "${OPENAI_API_KEY}"
  base_url: "${OPENAI_BASE_URL}"
  model: "gpt-4o"

qdrant:
  host: "${QDRANT_HOST}"
  port: 6333

default_agent_id: "code-assistant"

agents:
  - id: "code-assistant"
    name: "Code Assistant"
    model: "gpt-4o"
    system_prompt: "You are a production coding assistant. Prioritize correctness and safety."
    tools: ["code_executor", "file_ops", "todolist"]
  - id: "reviewer"
    name: "Code Reviewer"
    model: "gpt-4o"
    system_prompt: "You review code for bugs, security issues, and style problems."
    tools: ["file_ops"]
```

## Hot Reload

The current server does not support hot reload of the config file. Changing `config.yaml` requires restarting the server process.

If you need live agent configuration changes, you have two options:

1. **Rolling restart.** In a Kubernetes Deployment, update the ConfigMap and trigger a rolling restart. Pods drain active connections before terminating.
2. **Custom file watcher.** Extend `main.go` with an `fsnotify` watcher that calls `harness.Reload()` when the config file changes. This requires implementing a `Reload` method on the Harness.

## Extending the Config

To add new config fields:

1. Add the field to the appropriate struct in `server/internal/config/config.go`.
2. Add a YAML tag for deserialization.
3. Wire any env var overrides in `config.Load()`.
4. Add validation in `config.validate()` if needed.
5. Update `config.yaml.template` with the new field and a comment.

Example: adding a `logging` section.

```go
type Config struct {
    Server   ServerConfig   `yaml:"server"`
    Database DatabaseConfig `yaml:"database"`
    OpenAI   OpenAIConfig   `yaml:"openai"`
    Qdrant   QdrantConfig   `yaml:"qdrant"`
    Agents   []AgentConfig  `yaml:"agents"`
    DefaultAgentID string   `yaml:"default_agent_id"`
    Logging  LoggingConfig  `yaml:"logging"`  // new
}

type LoggingConfig struct {
    Level  string `yaml:"level"`   // debug, info, warn, error
    Format string `yaml:"format"`  // json, text
}
```

Then in `Load()`:

```go
if v := os.Getenv("COPCON_LOG_LEVEL"); v != "" {
    cfg.Logging.Level = v
}
```

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| Server exits with "duplicate agent ID" | Two agents share the same `id` | Make each `id` unique |
| Server exits with "default agent ID not found" | `default_agent_id` references an agent not in the `agents` list | Correct the ID or remove the field |
| "connection refused" on startup | Database host/port wrong, or Postgres not running | Check `database.*` fields and that Postgres is healthy |
| 401 errors from LLM API | `openai.api_key` is empty or invalid | Set `OPENAI_API_KEY` or fill in `api_key` in config |
| Config not updating after edit | No hot reload; server uses the config it loaded at startup | Restart the server |
