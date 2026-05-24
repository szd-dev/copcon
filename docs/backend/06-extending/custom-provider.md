# 自定义 Storage Provider

CopCon 的存储层通过接口抽象，内置了 PostgreSQL（关系数据）和 Qdrant（向量记忆）两个 Provider。当内置 Provider 不满足需求时（例如使用 MySQL、MongoDB 或 Redis），可以实现自定义的 `StoreProvider`。

## 架构概览

```
storage/          ← 纯接口定义（SessionStore, MessageStore, TodoStore, MemoryStore）
providers/
  postgres/       ← PostgreSQL 实现（GORM）
  qdrant/         ← Qdrant 向量数据库实现
```

**核心原则**：新 Provider 放在 `core/providers/` 下或独立模块中。**不要**在 `server/internal/` 中创建新的存储实现。

## StoreProvider 接口

`StoreProvider` 聚合了三个存储接口：

```go
// core/storage/provider.go
type StoreProvider interface {
    Sessions() SessionStore
    Messages() MessageStore
    Todos()    TodoStore
}
```

一个 Provider 实现必须同时提供三个 Store。通常它们共享同一个数据库连接。

## 分接口定义

### SessionStore

```go
type SessionStore interface {
    Create(ctx context.Context, session *Session) (*Session, error)
    Get(ctx context.Context, id uuid.UUID) (*Session, error)
    List(ctx context.Context, limit, offset int) ([]*Session, int64, error)
    Delete(ctx context.Context, id uuid.UUID) error
    UpdateTitle(ctx context.Context, id uuid.UUID, title string) error
    UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]any) error
    GetMessageCount(ctx context.Context, sessionID uuid.UUID) (int64, error)
    AppendMetadata(ctx context.Context, id uuid.UUID, key string, value any) error
}
```

### MessageStore

```go
type MessageStore interface {
    List(ctx context.Context, sessionID uuid.UUID, limit int) ([]*Message, error)
    Add(ctx context.Context, message *Message) error
    Update(ctx context.Context, message *Message) error
    Upsert(ctx context.Context, message *Message) error
    DeleteBySession(ctx context.Context, sessionID uuid.UUID) error
}
```

### TodoStore

```go
type TodoStore interface {
    Create(ctx context.Context, todo *Todo) (*Todo, error)
    Get(ctx context.Context, id uuid.UUID) (*Todo, error)
    List(ctx context.Context, sessionID uuid.UUID) ([]*Todo, error)
    UpdateStatus(ctx context.Context, id uuid.UUID, status TodoStatus) (*Todo, error)
    DeleteBySession(ctx context.Context, sessionID uuid.UUID) error
}
```

### MemoryStore（可选）

```go
type MemoryStore interface {
    Store(ctx context.Context, memory *Memory) error
    Search(ctx context.Context, query []float32, limit int) ([]*Memory, error)
    GetBySession(ctx context.Context, sessionID string, limit int) ([]*Memory, error)
    DeleteBySession(ctx context.Context, sessionID string) error
}
```

`MemoryStore` 是可选的。如果为 `nil`，Memory Hook 会跳过注册，不会报错。

## 数据结构

### Session

```go
type Session struct {
    ID              uuid.UUID
    Title           string
    DefaultAgentID  string
    ParentSessionID *uuid.UUID
    Metadata        map[string]any
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

### Message

```go
type Message struct {
    ID         uuid.UUID
    SessionID  uuid.UUID
    Role       string           // "system", "user", "assistant", "tool"
    Content    string
    Reasoning  string
    ToolCalls  []ToolCall
    ToolCallID string
    Parts      []Part
    Model      string
    TokenCount int
    DurationMs int64
    CreatedAt  time.Time
}
```

### Todo

```go
type Todo struct {
    ID          uuid.UUID
    SessionID   uuid.UUID
    Content     string
    ActiveForm  string
    Status      TodoStatus       // "pending", "in_progress", "completed", "blocked", "failed"
    Priority    string
    DependsOn   []uuid.UUID
    Validation  string
    Result      string
    RetryCount  int
    CompletedAt *time.Time
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

## 完整示例：SQLite Provider

这个示例用 GORM + SQLite 实现一个完整的 `StoreProvider`。

### 项目结构

```
sqlite/
├── store.go       # StoreProvider 入口 + AutoMigrate
├── session.go     # SessionStore 实现
├── message.go     # MessageStore 实现
├── todo.go        # TodoStore 实现
└── models.go      # GORM 模型定义
```

### store.go

```go
package sqlite

import (
    "github.com/copcon/core/storage"
    "gorm.io/gorm"
)

type Store struct {
    SessionStore *SessionStore
    MessageStore *MessageStore
    TodoStore    *TodoStore
}

// 编译时检查接口实现
var _ storage.StoreProvider = (*Store)(nil)

func NewStore(db *gorm.DB) *Store {
    AutoMigrate(db)
    return &Store{
        SessionStore: &SessionStore{db: db},
        MessageStore: &MessageStore{db: db},
        TodoStore:    &TodoStore{db: db},
    }
}

func (s *Store) Sessions() storage.SessionStore { return s.SessionStore }
func (s *Store) Messages() storage.MessageStore { return s.MessageStore }
func (s *Store) Todos() storage.TodoStore       { return s.TodoStore }

func AutoMigrate(db *gorm.DB) error {
    return db.AutoMigrate(&Session{}, &Message{}, &Todo{})
}
```

### session.go

```go
package sqlite

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/copcon/core/storage"
)

var ErrSessionNotFound = errors.New("session not found")

type SessionStore struct {
    db *gorm.DB
}

func (s *SessionStore) Create(ctx context.Context, session *storage.Session) (*storage.Session, error) {
    model := sessionFromStorage(session)
    if model.ID == uuid.Nil {
        model.ID = uuid.New()
    }
    if model.Metadata == nil {
        model.Metadata = make(JSONB)
    }
    if err := s.db.WithContext(ctx).Create(model).Error; err != nil {
        return nil, err
    }
    return sessionToStorage(model), nil
}

func (s *SessionStore) Get(ctx context.Context, id uuid.UUID) (*storage.Session, error) {
    var m Session
    if err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, ErrSessionNotFound
        }
        return nil, err
    }
    return sessionToStorage(&m), nil
}

func (s *SessionStore) List(ctx context.Context, limit, offset int) ([]*storage.Session, int64, error) {
    var sessions []*Session
    var total int64

    if err := s.db.WithContext(ctx).Model(&Session{}).Count(&total).Error; err != nil {
        return nil, 0, err
    }

    if err := s.db.WithContext(ctx).
        Order("updated_at DESC").
        Limit(limit).
        Offset(offset).
        Find(&sessions).Error; err != nil {
        return nil, 0, err
    }

    result := make([]*storage.Session, len(sessions))
    for i, ss := range sessions {
        result[i] = sessionToStorage(ss)
    }
    return result, total, nil
}

func (s *SessionStore) Delete(ctx context.Context, id uuid.UUID) error {
    result := s.db.WithContext(ctx).Delete(&Session{}, "id = ?", id)
    if result.Error != nil {
        return result.Error
    }
    if result.RowsAffected == 0 {
        return ErrSessionNotFound
    }
    return nil
}

func (s *SessionStore) UpdateTitle(ctx context.Context, id uuid.UUID, title string) error {
    result := s.db.WithContext(ctx).Model(&Session{}).Where("id = ?", id).Update("title", title)
    if result.Error != nil {
        return result.Error
    }
    if result.RowsAffected == 0 {
        return ErrSessionNotFound
    }
    return nil
}

func (s *SessionStore) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]any) error {
    result := s.db.WithContext(ctx).Model(&Session{}).Where("id = ?", id).Update("metadata", metadata)
    if result.Error != nil {
        return result.Error
    }
    if result.RowsAffected == 0 {
        return ErrSessionNotFound
    }
    return nil
}

func (s *SessionStore) GetMessageCount(ctx context.Context, sessionID uuid.UUID) (int64, error) {
    var count int64
    if err := s.db.WithContext(ctx).Model(&Message{}).Where("session_id = ?", sessionID).Count(&count).Error; err != nil {
        return 0, err
    }
    return count, nil
}

func (s *SessionStore) AppendMetadata(ctx context.Context, id uuid.UUID, key string, value any) error {
    var m Session
    if err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return ErrSessionNotFound
        }
        return err
    }

    if m.Metadata == nil {
        m.Metadata = make(JSONB)
    }

    existing, ok := m.Metadata[key]
    if !ok {
        m.Metadata[key] = []any{value}
    } else {
        arr, ok := existing.([]any)
        if !ok {
            arr = []any{existing}
        }
        m.Metadata[key] = append(arr, value)
    }

    result := s.db.WithContext(ctx).Model(&Session{}).Where("id = ?", id).Update("metadata", m.Metadata)
    if result.Error != nil {
        return result.Error
    }
    if result.RowsAffected == 0 {
        return ErrSessionNotFound
    }
    return nil
}
```

### 连接管理

```go
package sqlite

import (
    "fmt"

    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

type Config struct {
    Path string // SQLite 数据库文件路径，":memory:" 表示内存数据库
}

func NewConfig(path string) *Config {
    return &Config{Path: path}
}

func Connect(cfg *Config) (*gorm.DB, error) {
    db, err := gorm.Open(sqlite.Open(cfg.Path), &gorm.Config{})
    if err != nil {
        return nil, fmt.Errorf("sqlite connection failed: %w", err)
    }
    return db, nil
}
```

### 使用自定义 Provider

```go
package main

import (
    "log"

    "github.com/copcon/core"
    "github.com/yourorg/copcon-sqlite/sqlite"
)

func main() {
    db, err := sqlite.Connect(&sqlite.Config{Path: "copcon.db"})
    if err != nil {
        log.Fatal(err)
    }

    store := sqlite.NewStore(db)

    harness, err := core.NewHarness(&core.HarnessConfig{
        StoreProvider: store,
        // ... 其他配置
    })
    if err != nil {
        log.Fatal(err)
    }
    defer harness.Close()
}
```

## 迁移策略

### GORM AutoMigrate

最简单的方式，适合开发阶段：

```go
func AutoMigrate(db *gorm.DB) error {
    return db.AutoMigrate(&Session{}, &Message{}, &Todo{})
}
```

GORM AutoMigrate 会创建缺失的表和列，但**不会**删除列或修改列类型。生产环境建议使用版本化迁移。

### 版本化迁移

使用 [golang-migrate](https://github.com/golang-migrate/migrate) 等工具：

```
migrations/
├── 001_create_sessions.up.sql
├── 001_create_sessions.down.sql
├── 002_create_messages.up.sql
├── 002_create_messages.down.sql
└── 003_create_todos.up.sql
    003_create_todos.down.sql
```

```sql
-- 001_create_sessions.up.sql
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) DEFAULT '',
    default_agent_id VARCHAR(64) DEFAULT '',
    parent_session_id UUID REFERENCES sessions(id) ON DELETE RESTRICT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_parent ON sessions(parent_session_id);
CREATE INDEX idx_sessions_updated ON sessions(updated_at DESC);
```

```go
import (
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/sqlite3"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(dbURL, migrationsPath string) error {
    m, err := migrate.New(
        "file://"+migrationsPath,
        dbURL,
    )
    if err != nil {
        return err
    }
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return err
    }
    return nil
}
```

## 事务处理

对于需要原子性的操作，使用 GORM 的事务：

```go
func (s *SessionStore) DeleteWithMessages(ctx context.Context, id uuid.UUID) error {
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // 删除关联消息
        if err := tx.Where("session_id = ?", id).Delete(&Message{}).Error; err != nil {
            return err
        }
        // 删除关联待办
        if err := tx.Where("session_id = ?", id).Delete(&Todo{}).Error; err != nil {
            return err
        }
        // 删除会话
        if err := tx.Delete(&Session{}, "id = ?", id).Error; err != nil {
            return err
        }
        return nil
    })
}
```

## MemoryStore 实现

如果需要自定义向量存储后端（替代 Qdrant），实现 `MemoryStore` 接口：

```go
type MemoryStore interface {
    Store(ctx context.Context, memory *Memory) error
    Search(ctx context.Context, query []float32, limit int) ([]*Memory, error)
    GetBySession(ctx context.Context, sessionID string, limit int) ([]*Memory, error)
    DeleteBySession(ctx context.Context, sessionID string) error
}
```

`Search` 方法接收一个 `[]float32` 向量，返回最相似的 `limit` 条记录。向量编码由调用方负责（Hook 层的 `encodeTextToVector` 函数）。

## 测试自定义 Provider

### 接口合规性检查

```go
// 编译时检查
var (
    _ storage.SessionStore  = (*SessionStore)(nil)
    _ storage.MessageStore  = (*MessageStore)(nil)
    _ storage.TodoStore     = (*TodoStore)(nil)
    _ storage.StoreProvider = (*Store)(nil)
)
```

### CRUD 测试

```go
func TestSessionStore_CRUD(t *testing.T) {
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    require.NoError(t, err)
    store := NewStore(db)

    ctx := context.Background()

    t.Run("create and get", func(t *testing.T) {
        session := &storage.Session{
            Title:          "Test Session",
            DefaultAgentID: "assistant",
        }
        created, err := store.Sessions().Create(ctx, session)
        require.NoError(t, err)
        assert.NotEqual(t, uuid.Nil, created.ID)

        got, err := store.Sessions().Get(ctx, created.ID)
        require.NoError(t, err)
        assert.Equal(t, "Test Session", got.Title)
    })

    t.Run("get not found", func(t *testing.T) {
        _, err := store.Sessions().Get(ctx, uuid.New())
        assert.ErrorIs(t, err, ErrSessionNotFound)
    })

    t.Run("delete", func(t *testing.T) {
        session := &storage.Session{Title: "To Delete"}
        created, err := store.Sessions().Create(ctx, session)
        require.NoError(t, err)

        err = store.Sessions().Delete(ctx, created.ID)
        require.NoError(t, err)

        _, err = store.Sessions().Get(ctx, created.ID)
        assert.ErrorIs(t, err, ErrSessionNotFound)
    })
}
```

## 常见问题

### Q: 自定义 Provider 放在哪里？

A: 有两种选择：
- **同模块**：放在 `core/providers/<name>/` 下，与内置 Provider 并列
- **独立模块**：放在独立仓库（如 `github.com/yourorg/copcon-mysql`），通过 `go get` 引入

不要放在 `server/internal/` 中，这违反了模块边界约束。

### Q: 必须实现 MemoryStore 吗？

A: 不必须。`MemoryStore` 是可选的。如果 Harness 配置中 `MemoryStore` 为 `nil`，Memory Hook 会自动跳过注册。

### Q: 如何处理并发写入？

A: GORM 底层使用连接池，每个请求通过 `db.WithContext(ctx)` 获取独立连接。对于需要串行化的操作，使用数据库事务或乐观锁。

### Q: UUID 类型怎么处理？

A: CopCon 使用 `github.com/google/uuid`。存储层接口中所有 ID 都是 `uuid.UUID` 类型。GORM 模型中使用 `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"` 标注。

## 下一步

- [自定义 Tool](custom-tool.md) - 编写 Agent 可调用的自定义工具
- [自定义 Hook](custom-hook.md) - 在 Agent 生命周期中注入自定义逻辑
- [测试自定义实现](testing-custom-implementations.md) - 测试和基准测试指南
