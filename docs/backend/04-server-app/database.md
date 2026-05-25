# Database

CopCon uses PostgreSQL as its primary data store, with Qdrant as an optional vector database for long-term memory. This guide covers setup, migrations, connection pooling, backups, and tuning.

## Supported Databases

| Database | Role | Status |
|----------|------|--------|
| PostgreSQL 15+ | Primary store (sessions, messages, todos) | Supported (recommended for production) |
| SQLite | Embedded store (sessions, messages, todos) | Supported (auto-detected, zero-config) |
| Qdrant 1.17+ | Vector memory store | Optional (skipped if unavailable) |
| In-memory | Not supported by the server | Available only in unit tests via mock stores |

The server supports both PostgreSQL and SQLite as storage backends. PostgreSQL is recommended for production deployments. SQLite is auto-detected when no PostgreSQL configuration is present, providing a zero-config option for development and lightweight deployments.

The `core/providers/postgres` and `core/providers/sqlite` packages each provide all three store interfaces (SessionStore, MessageStore, TodoStore) backed by the same database connection.

## PostgreSQL Setup

### Quick Start with Docker Compose

The project includes a Docker Compose file that sets up Postgres with health checks:

```yaml
# docker-compose.yaml
services:
  postgres:
    image: postgres:15-alpine
    container_name: copcon-postgres
    environment:
      POSTGRES_USER: agent
      POSTGRES_PASSWORD: agent123
      POSTGRES_DB: agent_infra
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U agent"]
      interval: 5s
      timeout: 5s
      retries: 5
```

Start it:

```bash
docker compose up -d postgres
```

### Manual Setup

If you prefer to run Postgres outside Docker:

1. Install PostgreSQL 15 or later.
2. Create the database and user:

```sql
CREATE USER copcon_admin WITH PASSWORD 'your_password';
CREATE DATABASE copcon OWNER copcon_admin;
GRANT ALL PRIVILEGES ON DATABASE copcon TO copcon_admin;
```

Tables are created automatically on server startup via GORM `AutoMigrate`. No manual initialization step is needed.

### Connection String Format

The server constructs the DSN from config fields:

```
host=<host> port=<port> user=<user> password=<password> dbname=<dbname> sslmode=disable
```

Example for the default config:

```
host=localhost port=5432 user=admin password=changeme dbname=copcon sslmode=disable
```

For Docker Compose, the server uses service names:

```
host=postgres port=5432 user=agent password=agent123 dbname=agent_infra sslmode=disable
```

> The `sslmode=disable` suffix is currently hardcoded in `DatabaseConfig.DSN()`. To enable SSL, modify this method in `server/internal/config/config.go` to read an `sslmode` config field, or add `sslmode=require` for production.

## SQLite

SQLite is available as a zero-configuration alternative to PostgreSQL. It's auto-detected when no `database.host` is configured.

### Configuration

Use the SQLite config template:

```bash
cp server/config.yaml.sqlite.template server/config.yaml
```

Or manually set the database type:

```yaml
database:
  type: sqlite
  sqlite_path: "data/copcon.db"  # optional, defaults to data/copcon.db
```

### Auto-Detection

If `database.host` is empty and `database.type` is not set, the server automatically uses SQLite with the default path `data/copcon.db`. No external database service is required.

### PRAGMA Defaults

The SQLite connection is configured with these PRAGMA settings for reliability:

| PRAGMA | Value | Purpose |
|--------|-------|---------|
| `journal_mode` | WAL | Write-Ahead Logging for better concurrency |
| `busy_timeout` | 5000 | Wait up to 5s for locked database |
| `foreign_keys` | 1 | Enable foreign key constraints |
| `synchronous` | NORMAL | Balance between safety and performance |

Connection pool is set to `MaxOpenConns=1` for single-writer safety.

### SQLite Auto-Migration

When using SQLite, tables are created automatically on server startup via GORM `AutoMigrate`. No manual initialization step is needed.

## Schema and Migrations

### Auto-Migration

The PostgreSQL provider uses GORM's `AutoMigrate` on startup. When `pgstore.NewStore(db)` is called, it runs:

```go
func AutoMigrate(db *gorm.DB) error {
    return db.AutoMigrate(&Session{}, &Message{}, &Todo{})
}
```

This creates or updates tables to match the current Go model definitions. It adds missing columns and indexes but never drops data. For a fresh database, this is sufficient.

### Schema Details

The three core tables:

**sessions**

| Column | Type | Notes |
|--------|------|-------|
| `id` | `uuid` | Primary key, auto-generated via `gen_random_uuid()` |
| `title` | `varchar(255)` | Session title |
| `default_agent_id` | `varchar(64)` | Agent assigned to this session |
| `parent_session_id` | `uuid` | Self-referencing FK for session trees |
| `created_at` | `timestamptz` | Auto-set on insert |
| `updated_at` | `timestamptz` | Auto-set on update (GORM auto-timestamps) |
| `metadata` | `jsonb` | Flexible key-value store, defaults to `{}` |

**messages**

| Column | Type | Notes |
|--------|------|-------|
| `id` | `uuid` | Primary key |
| `session_id` | `uuid` | FK to sessions, CASCADE on delete |
| `role` | `varchar(20)` | `user`, `assistant`, `tool`, or `system` |
| `content` | `text` | Message body |
| `reasoning` | `text` | Chain-of-thought content (optional) |
| `tool_calls` | `jsonb` | Array of tool invocation objects |
| `tool_call_id` | `varchar(255)` | Links tool result messages to their calls |
| `parts` | `jsonb` | Structured message parts for rich rendering |
| `model` | `varchar(100)` | LLM model that produced this message |
| `token_count` | `integer` | Token usage |
| `duration_ms` | `integer` | Response time in milliseconds |
| `created_at` | `timestamptz` | Auto-set on insert |

Indexes: `idx_messages_session_id` on `session_id`, `idx_messages_created_at` on `created_at DESC`.

**todos**

| Column | Type | Notes |
|--------|------|-------|
| `id` | `uuid` | Primary key |
| `session_id` | `uuid` | FK to sessions, CASCADE on delete |
| `content` | `text` | Task description |
| `active_form` | `varchar(255)` | Present-tense form ("Writing tests") |
| `status` | `varchar(20)` | `pending`, `in_progress`, `completed`, `blocked`, `failed` |
| `depends_on` | `uuid[]` | Array of UUIDs this task depends on |
| `validation` | `text` | How to verify completion |
| `result` | `text` | Outcome description |
| `retry_count` | `integer` | Number of retries, defaults to 0 |
| `created_at` | `timestamptz` | Auto-set |
| `updated_at` | `timestamptz` | Auto-set |
| `completed_at` | `timestamptz` | Set on completion |

### Manual Migration Files

Incremental SQL migrations live in `server/migrations/`. These handle changes that GORM's AutoMigrate can't do (like adding constraints or triggers that GORM doesn't model).

Currently there is one migration:

**001_add_parent_session_id.sql**

```sql
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS parent_session_id UUID REFERENCES sessions(id);

CREATE INDEX IF NOT EXISTS idx_sessions_parent_session_id ON sessions(parent_session_id);
```

To run migrations manually:

```bash
psql -h localhost -U admin -d copcon -f server/migrations/001_add_parent_session_id.sql
```

Or programmatically, before starting the server:

Tables are auto-created on server startup via GORM `AutoMigrate`.

### Migration Strategy for New Changes

1. **Add column.** GORM's `AutoMigrate` handles simple column additions. Just add the field to the Go model.
2. **Add index.** GORM can create indexes via struct tags (`gorm:"index"`). For complex indexes, use a SQL migration file.
3. **Add constraint or trigger.** Always use a SQL migration file under `server/migrations/`.
4. **Rename or drop column.** GORM won't do this. Write a migration file and test it carefully.

Follow the naming convention: `NNN_descriptive_name.sql` where `NNN` is a zero-padded sequence number.

## Connection Pooling

GORM wraps a `database/sql` connection pool. The server currently uses GORM's default pool settings. For production, you should tune these.

### Recommended Pool Settings

Add pool configuration after opening the GORM connection in `cmd/server/main.go`:

```go
db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
if err != nil {
    log.Error("fatal", "error", err)
    os.Exit(1)
}

sqlDB, err := db.DB()
if err != nil {
    log.Error("fatal", "error", err)
    os.Exit(1)
}

// Connection pool tuning
sqlDB.SetMaxOpenConns(25)        // Max concurrent connections
sqlDB.SetMaxIdleConns(10)        // Keep 10 warm connections
sqlDB.SetConnMaxLifetime(time.Hour)  // Recycle connections hourly
sqlDB.SetConnMaxIdleTime(30 * time.Minute) // Drop idle connections after 30 min
```

### Tuning Guidelines

| Setting | Dev | Staging | Production | Rationale |
|---------|-----|---------|------------|-----------|
| `MaxOpenConns` | 5 | 15 | 25 | Match your Postgres `max_connections` budget. Leave room for other clients. |
| `MaxIdleConns` | 2 | 5 | 10 | Reduces connection startup latency. Keep roughly half of MaxOpenConns. |
| `ConnMaxLifetime` | 30min | 30min | 1h | Prevents stale connections. Shorter in dev for faster feedback. |
| `ConnMaxIdleTime` | 15min | 15min | 30min | Drops unused connections to free Postgres resources. |

## Qdrant (Memory Store)

Qdrant stores vector embeddings for the long-term memory feature. It's optional. If `MemoryStore` is nil in the Harness config, the memory hook skips registration silently. No errors, just no memory features.

### Setup

Docker Compose includes Qdrant:

```yaml
qdrant:
  image: qdrant/qdrant:v1.17.0
  ports:
    - "6333:6333"
    - "6334:6334"
  volumes:
    - qdrant_data:/qdrant/storage
```

The gRPC port (6334) is used by the Go client. The REST port (6333) serves the dashboard and health checks.

### Wiring into the Server

The current `main.go` does not connect Qdrant. To enable memory, add the client initialization before building the Harness:

```go
import (
    "github.com/qdrant/go-client/qdrant"
    qdrantstore "github.com/copcon/core/providers/qdrant"
)

// After creating the pg store...

qdrantClient, err := qdrant.NewClient(&qdrant.Config{
    Host: cfg.Qdrant.Host,
    Port: cfg.Qdrant.Port,
})
chk(log, err)

memoryStore := qdrantstore.NewMemoryStore(qdrantClient, "copcon_memories")

h := core.NewHarness(core.HarnessConfig{
    Store: core.StoreConfig{
        Provider:     pg,
        MemoryStore:  memoryStore,  // nil = skip memory features
    },
    // ...
})
```

## Backup and Restore

### PostgreSQL

**pg_dump (logical backup)**

```bash
# Full backup
pg_dump -h localhost -U admin -d copcon -F c -f copcon_backup.dump

# Restore
pg_restore -h localhost -U admin -d copcon -c copcon_backup.dump
```

**Automated daily backups with cron**

```bash
# Add to crontab
0 2 * * * pg_dump -h db-host -U copcon_admin -d copcon -F c -f /backups/copcon_$(date +\%Y\%m\%d).dump
```

**Continuous archiving (WAL)**

For production databases that can't tolerate data loss between daily dumps, enable WAL archiving in `postgresql.conf`:

```
wal_level = replica
archive_mode = on
archive_command = 'cp %p /backups/wal/%f'
```

This gives you point-in-time recovery.

### Qdrant

Qdrant stores data in `/qdrant/storage` inside the container. With the Docker Compose volume mount, this persists across container restarts.

For explicit backups:

```bash
# Create a snapshot via the REST API
curl -X POST 'http://localhost:6333/collections/copcon_memories/snapshots'

# Download the snapshot
curl 'http://localhost:6333/collections/copcon_memories/snapshots' -o snapshot.tar
```

## Performance Tuning

### PostgreSQL

**Key settings in `postgresql.conf` for the CopCon workload:**

| Setting | Recommended | Why |
|---------|-------------|-----|
| `shared_buffers` | 256MB (or 25% of RAM) | Caches table data in memory |
| `work_mem` | 16MB | Enough for JSONB queries and sorting |
| `maintenance_work_mem` | 128MB | Fast auto-vacuum and migrations |
| `effective_cache_size` | 1GB (or 50% of RAM) | Helps the planner choose index scans |
| `random_page_cost` | 1.1 (on SSD) | SSDs make random reads cheap |

**Index strategy:**

The current indexes cover the common query patterns. For heavy list operations, consider adding:

```sql
-- If you filter sessions by agent
CREATE INDEX idx_sessions_default_agent_id ON sessions(default_agent_id);

-- If you query messages by role
CREATE INDEX idx_messages_role ON messages(role) WHERE role = 'assistant';

-- For text search in content (if you add search features)
CREATE INDEX idx_messages_content_trgm ON messages USING gin (content gin_trgm_ops);
```

### Query Patterns

The server's most frequent queries:

1. **List sessions.** `SELECT * FROM sessions ORDER BY updated_at DESC LIMIT ? OFFSET ?`
2. **Get messages.** `SELECT * FROM messages WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`
3. **Create session.** `INSERT INTO sessions (...)`
4. **Add message.** `INSERT INTO messages (...)`

All of these hit the existing indexes. The `idx_sessions_updated_at` and `idx_messages_created_at` indexes ensure list queries stay fast even with thousands of records.

### Connection Context

Always pass `context.Context` through to GORM. The `core/providers/postgres` store uses `db.WithContext(ctx)` on every query. This means:

- Requests that take too long get cancelled when the client disconnects.
- Timeouts propagate from the HTTP handler through to the database.
- No orphan queries sitting in the connection pool.

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| "connection refused" at startup | Postgres not running or wrong host/port | Check `database.*` config, ensure Postgres is healthy (`pg_isready`) |
| "database copcon does not exist" | Database not created yet | Create it manually (see SQL in Manual Setup). If using SQLite, ensure the `data/` directory is writable. |
| Slow session list with many records | Missing index on `updated_at` | Ensure `idx_sessions_updated_at` exists |
| GORM migration adds wrong columns | Stale model definition | Check `core/providers/postgres/models.go` matches your schema |
| Qdrant connection failures | Qdrant not running, wrong port | Check `qdrant.*` config, or set `MemoryStore` to nil to disable |

## Next Steps

- [Configuration](./configuration.md) for database config fields
- [Logging](./logging.md) for monitoring database queries
- [Security](./security.md) for database hardening