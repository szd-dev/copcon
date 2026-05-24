# 自定义 Provider

如果内置的存储提供者 (PostgreSQL, MongoDB, SQLite) 不满足你的需求,可以实现自定义的 Storage Provider。

## Store 接口

CopCon 定义了统一的存储接口:

```go
type Store interface {
    // Session 管理
    CreateSession(ctx context.Context, session *Session) error
    GetSession(ctx context.Context, id string) (*Session, error)
    ListSessions(ctx context.Context, filter *SessionFilter) ([]*Session, error)
    UpdateSession(ctx context.Context, session *Session) error
    DeleteSession(ctx context.Context, id string) error
    
    // Message 管理
    AppendMessage(ctx context.Context, sessionID string, msg *Message) error
    GetMessages(ctx context.Context, sessionID string) ([]*Message, error)
    DeleteMessages(ctx context.Context, sessionID string) error
    
    // Tool 结果管理
    SaveToolResult(ctx context.Context, result *ToolResult) error
    GetToolResults(ctx context.Context, sessionID string) ([]*ToolResult, error)
    
    // Todo 管理 (可选)
    SaveTodo(ctx context.Context, todo *Todo) error
    GetTodos(ctx context.Context, sessionID string) ([]*Todo, error)
    
    // 关闭存储
    Close() error
}
```

## 数据结构

```go
type Session struct {
    ID         string                 `json:"id"`
    AgentName  string                 `json:"agent_name"`
    Title      string                 `json:"title"`
    Metadata   map[string]interface{} `json:"metadata"`
    CreatedAt  time.Time              `json:"created_at"`
    UpdatedAt  time.Time              `json:"updated_at"`
}

type Message struct {
    ID         string    `json:"id"`
    SessionID  string    `json:"session_id"`
    Role       string    `json:"role"`        // "user", "assistant", "system", "tool"
    Content    string    `json:"content"`
    Model      string    `json:"model,omitempty"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    CreatedAt  time.Time `json:"created_at"`
}

type ToolCall struct {
    ID       string                 `json:"id"`
    Name     string                 `json:"name"`
    Arguments map[string]interface{} `json:"arguments"`
}

type ToolResult struct {
    ID         string    `json:"id"`
    SessionID  string    `json:"session_id"`
    MessageID  string    `json:"message_id"`
    ToolCallID string    `json:"tool_call_id"`
    ToolName   string    `json:"tool_name"`
    Result     string    `json:"result"`
    Error      string    `json:"error,omitempty"`
    Status     string    `json:"status"` // "success", "error", "timeout"
    CreatedAt  time.Time `json:"created_at"`
}

type Todo struct {
    ID          string    `json:"id"`
    SessionID   string    `json:"session_id"`
    Content     string    `json:"content"`
    Status      string    `json:"status"` // "pending", "in_progress", "completed", "failed"
    Priority    int       `json:"priority"`
    CreatedAt   time.Time `json:"created_at"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type SessionFilter struct {
    AgentName  string
    Limit      int
    Offset     int
    SortBy     string
    SortDesc   bool
}
```

## 实现示例: Redis Provider

```go
package redis

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "github.com/go-redis/redis/v8"
    "github.com/copcon/core"
    "github.com/copcon/core/storage"
)

type Config struct {
    Addr     string
    Password string
    DB       int
}

type RedisStore struct {
    client *redis.Client
}

func NewStore(cfg Config) (*RedisStore, error) {
    client := redis.NewClient(&redis.Options{
        Addr:     cfg.Addr,
        Password: cfg.Password,
        DB:       cfg.DB,
    })
    
    // 测试连接
    ctx := context.Background()
    if err := client.Ping(ctx).Err(); err != nil {
        return nil, fmt.Errorf("redis connection failed: %w", err)
    }
    
    return &RedisStore{client: client}, nil
}

func (s *RedisStore) CreateSession(ctx context.Context, session *storage.Session) error {
    data, err := json.Marshal(session)
    if err != nil {
        return err
    }
    
    key := fmt.Sprintf("session:%s", session.ID)
    return s.client.Set(ctx, key, data, 0).Err()
}

func (s *RedisStore) GetSession(ctx context.Context, id string) (*storage.Session, error) {
    key := fmt.Sprintf("session:%s", id)
    data, err := s.client.Get(ctx, key).Result()
    if err == redis.Nil {
        return nil, core.ErrNotFound
    }
    if err != nil {
        return nil, err
    }
    
    var session storage.Session
    if err := json.Unmarshal([]byte(data), &session); err != nil {
        return nil, err
    }
    
    return &session, nil
}

func (s *RedisStore) ListSessions(ctx context.Context, filter *storage.SessionFilter) ([]*storage.Session, error) {
    // Redis 不擅长复杂查询,这里简化实现
    pattern := "session:*"
    keys, err := s.client.Keys(ctx, pattern).Result()
    if err != nil {
        return nil, err
    }
    
    var sessions []*storage.Session
    for _, key := range keys {
        data, err := s.client.Get(ctx, key).Result()
        if err != nil {
            continue
        }
        
        var session storage.Session
        if err := json.Unmarshal([]byte(data), &session); err != nil {
            continue
        }
        
        // 应用过滤
        if filter.AgentName != "" && session.AgentName != filter.AgentName {
            continue
        }
        
        sessions = append(sessions, &session)
    }
    
    // 应用分页
    if filter.Limit > 0 {
        start := filter.Offset
        end := start + filter.Limit
        if start >= len(sessions) {
            return []*storage.Session{}, nil
        }
        if end > len(sessions) {
            end = len(sessions)
        }
        sessions = sessions[start:end]
    }
    
    return sessions, nil
}

func (s *RedisStore) UpdateSession(ctx context.Context, session *storage.Session) error {
    session.UpdatedAt = time.Now()
    return s.CreateSession(ctx, session)
}

func (s *RedisStore) DeleteSession(ctx context.Context, id string) error {
    // 删除 session
    key := fmt.Sprintf("session:%s", id)
    if err := s.client.Del(ctx, key).Err(); err != nil {
        return err
    }
    
    // 删除关联的 messages
    msgKey := fmt.Sprintf("messages:%s", id)
    if err := s.client.Del(ctx, msgKey).Err(); err != nil {
        return err
    }
    
    // 删除关联的 tool results
    toolKey := fmt.Sprintf("tool_results:%s", id)
    if err := s.client.Del(ctx, toolKey).Err(); err != nil {
        return err
    }
    
    return nil
}

func (s *RedisStore) AppendMessage(ctx context.Context, sessionID string, msg *storage.Message) error {
    key := fmt.Sprintf("messages:%s", sessionID)
    
    data, err := json.Marshal(msg)
    if err != nil {
        return err
    }
    
    return s.client.RPush(ctx, key, data).Err()
}

func (s *RedisStore) GetMessages(ctx context.Context, sessionID string) ([]*storage.Message, error) {
    key := fmt.Sprintf("messages:%s", sessionID)
    
    data, err := s.client.LRange(ctx, key, 0, -1).Result()
    if err != nil {
        return nil, err
    }
    
    var messages []*storage.Message
    for _, d := range data {
        var msg storage.Message
        if err := json.Unmarshal([]byte(d), &msg); err != nil {
            return nil, err
        }
        messages = append(messages, &msg)
    }
    
    return messages, nil
}

func (s *RedisStore) DeleteMessages(ctx context.Context, sessionID string) error {
    key := fmt.Sprintf("messages:%s", sessionID)
    return s.client.Del(ctx, key).Err()
}

func (s *RedisStore) SaveToolResult(ctx context.Context, result *storage.ToolResult) error {
    key := fmt.Sprintf("tool_results:%s", result.SessionID)
    
    data, err := json.Marshal(result)
    if err != nil {
        return err
    }
    
    return s.client.RPush(ctx, key, data).Err()
}

func (s *RedisStore) GetToolResults(ctx context.Context, sessionID string) ([]*storage.ToolResult, error) {
    key := fmt.Sprintf("tool_results:%s", sessionID)
    
    data, err := s.client.LRange(ctx, key, 0, -1).Result()
    if err != nil {
        return nil, err
    }
    
    var results []*storage.ToolResult
    for _, d := range data {
        var result storage.ToolResult
        if err := json.Unmarshal([]byte(d), &result); err != nil {
            return nil, err
        }
        results = append(results, &result)
    }
    
    return results, nil
}

func (s *RedisStore) SaveTodo(ctx context.Context, todo *storage.Todo) error {
    key := fmt.Sprintf("todos:%s:%s", todo.SessionID, todo.ID)
    
    data, err := json.Marshal(todo)
    if err != nil {
        return err
    }
    
    return s.client.Set(ctx, key, data, 0).Err()
}

func (s *RedisStore) GetTodos(ctx context.Context, sessionID string) ([]*storage.Todo, error) {
    pattern := fmt.Sprintf("todos:%s:*", sessionID)
    keys, err := s.client.Keys(ctx, pattern).Result()
    if err != nil {
        return nil, err
    }
    
    var todos []*storage.Todo
    for _, key := range keys {
        data, err := s.client.Get(ctx, key).Result()
        if err != nil {
            continue
        }
        
        var todo storage.Todo
        if err := json.Unmarshal([]byte(data), &todo); err != nil {
            continue
        }
        
        todos = append(todos, &todo)
    }
    
    return todos, nil
}

func (s *RedisStore) Close() error {
    return s.client.Close()
}
```

## 使用自定义 Provider

```go
import "github.com/yourorg/copcon-redis"

cfg := redis.Config{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
}

store, err := redis.NewStore(cfg)
if err != nil {
    panic(err)
}
defer store.Close()

// 使用 Harness
harnessCfg := &core.HarnessConfig{
    Agents: []core.AgentSpec{...},
}

harness, err := core.NewHarnessWithStore(harnessCfg, store)
```

## 性能优化建议

### 1. 连接池

```go
client := redis.NewClient(&redis.Options{
    Addr:         cfg.Addr,
    Password:     cfg.Password,
    DB:           cfg.DB,
    PoolSize:     100,        // 连接池大小
    MinIdleConns: 10,         // 最小空闲连接
    MaxRetries:   3,          // 最大重试次数
})
```

### 2. 批量操作

```go
func (s *RedisStore) GetMultipleSessions(ctx context.Context, ids []string) ([]*storage.Session, error) {
    // 使用 Pipeline
    pipe := s.client.Pipeline()
    
    results := make([]*redis.StringCmd, len(ids))
    for i, id := range ids {
        key := fmt.Sprintf("session:%s", id)
        results[i] = pipe.Get(ctx, key)
    }
    
    _, err := pipe.Exec(ctx)
    if err != nil && err != redis.Nil {
        return nil, err
    }
    
    var sessions []*storage.Session
    for _, cmd := range results {
        data, err := cmd.Result()
        if err == redis.Nil {
            continue
        }
        if err != nil {
            return nil, err
        }
        
        var session storage.Session
        if err := json.Unmarshal([]byte(data), &session); err != nil {
            return nil, err
        }
        
        sessions = append(sessions, &session)
    }
    
    return sessions, nil
}
```

### 3. 缓存策略

```go
// 热数据缓存 (最近访问的 session)
type CachedStore struct {
    cache memory.Store        // 内存缓存
    redis redis.Store         // Redis 持久化
}

func (s *CachedStore) GetSession(ctx context.Context, id string) (*storage.Session, error) {
    // 先查缓存
    if session, err := s.cache.GetSession(ctx, id); err == nil {
        return session, nil
    }
    
    // 缓存未命中,查 Redis
    session, err := s.redis.GetSession(ctx, id)
    if err != nil {
        return nil, err
    }
    
    // 写入缓存
    s.cache.CreateSession(ctx, session)
    
    return session, nil
}
```

## 测试

```go
func TestRedisStore(t *testing.T) {
    cfg := redis.Config{
        Addr:     "localhost:6379",
        Password: "",
        DB:       0,
    }
    
    store, err := redis.NewStore(cfg)
    require.NoError(t, err)
    defer store.Close()
    
    ctx := context.Background()
    
    // 测试 Session CRUD
    session := &storage.Session{
        ID:        "test-123",
        AgentName: "assistant",
        CreatedAt: time.Now(),
    }
    
    err = store.CreateSession(ctx, session)
    require.NoError(t, err)
    
    got, err := store.GetSession(ctx, "test-123")
    require.NoError(t, err)
    assert.Equal(t, session.ID, got.ID)
    
    // 清理
    err = store.DeleteSession(ctx, "test-123")
    require.NoError(t, err)
}
```

## 常见问题

### Q: 如何处理并发写入?

A: 使用 Redis 的乐观锁:
```go
// 使用 WATCH + MULTI/EXEC
tx := s.client.Watch(ctx, func(tx *redis.Tx) error {
    // 在事务中执行写入
    _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
        pipe.Set(ctx, key, data, 0)
        return nil
    })
    return err
}, key)
```

### Q: 如何保证数据一致性?

A: 对于强一致性要求的场景,考虑使用分布式锁或切换到支持事务的存储 (如 PostgreSQL)。

### Q: 如何处理大数据量?

A: 使用 Redis 的分页查询:
```go
func (s *RedisStore) ListMessages(ctx context.Context, sessionID string, limit, offset int) ([]*storage.Message, error) {
    key := fmt.Sprintf("messages:%s", sessionID)
    data, err := s.client.LRange(ctx, key, int64(offset), int64(offset+limit-1)).Result()
    // ...
}
```

## 下一步

- [多 Agent 协作](multi-agent.md)
- [配置指南](configuration.md)
- [自定义工具](../06-extending/custom-tool.md)
