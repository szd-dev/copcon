# 测试自定义实现

自定义 Tool、Hook、Provider 和 LLM Adapter 都需要测试。本文档提供统一的测试指南，涵盖单元测试、集成测试、Mock 策略、性能基准和 CI/CD 集成。

## 单元测试模式

### 框架与断言

CopCon 使用 `testify/assert` 和 `testify/require`：

```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
```

- `require`：失败时立即终止测试（用于关键前置条件）
- `assert`：失败时记录但继续执行（用于非关键检查）

### 接口合规性检查

每种自定义实现都应在编译时验证接口合规性：

```go
// Tool
var _ tool.Tool = (*MyTool)(nil)

// Hook
var _ hook.Hook = (*MyHook)(nil)

// StoreProvider
var _ storage.StoreProvider = (*MyStore)(nil)
var _ storage.SessionStore = (*MySessionStore)(nil)
var _ storage.MessageStore = (*MyMessageStore)(nil)
var _ storage.TodoStore = (*MyTodoStore)(nil)

// LLM Provider
var _ llm.LLMProvider = (*MyAdapter)(nil)
```

如果接口签名变了，编译就会报错，而不是运行时 panic。

## 测试自定义 Tool

### Mock ChatContext

Tool 的 `Execute` 方法接收 `iface.ChatContextInterface`。测试时需要 mock：

```go
package mytool_test

import (
    "context"
    "testing"

    "github.com/copcon/core/iface"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// mockChatCtx 实现 iface.ChatContextInterface 的最小子集
type mockChatCtx struct {
    ctx       context.Context
    sessionID string
    agentID   string
}

func (m *mockChatCtx) Context() context.Context  { return m.ctx }
func (m *mockChatCtx) SessionID() string          { return m.sessionID }
func (m *mockChatCtx) AgentID() string            { return m.agentID }
func (m *mockChatCtx) Emit(eventType string, data any) {}
func (m *mockChatCtx) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
    return &iface.InputResponse{Action: "approve", Content: "test"}, nil
}
```

### 参数验证测试

```go
func TestMyTool_ArgumentValidation(t *testing.T) {
    myTool := NewMyTool()
    ctx := &mockChatCtx{ctx: context.Background(), sessionID: "test", agentID: "agent"}

    tests := []struct {
        name    string
        args    map[string]any
        wantErr string
    }{
        {
            name:    "missing required param",
            args:    map[string]any{},
            wantErr: "query is required",
        },
        {
            name:    "empty string param",
            args:    map[string]any{"query": ""},
            wantErr: "query is required",
        },
        {
            name:    "invalid enum value",
            args:    map[string]any{"query": "test", "category": "invalid"},
            wantErr: "invalid category",
        },
        {
            name:    "out of range numeric",
            args:    map[string]any{"query": "test", "limit": float64(-1)},
            wantErr: "limit must be",
        },
        {
            name: "valid arguments",
            args: map[string]any{"query": "golang testing", "limit": float64(5)},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := myTool.Execute(ctx, tt.args)
            require.NoError(t, err) // 系统级不应有 error

            if tt.wantErr != "" {
                assert.False(t, result.Success)
                assert.Contains(t, result.Error, tt.wantErr)
            } else {
                assert.True(t, result.Success)
            }
        })
    }
}
```

### Schema 一致性测试

确保 Schema 与实际参数处理一致：

```go
func TestMyTool_SchemaConsistency(t *testing.T) {
    myTool := NewMyTool()
    schema := myTool.InputSchema()

    props, ok := schema["properties"].(map[string]any)
    require.True(t, ok, "schema must have properties")

    required, ok := schema["required"].([]string)
    require.True(t, ok, "schema must have required")

    // 验证 required 中的字段都在 properties 中定义了
    for _, field := range required {
        _, exists := props[field]
        assert.True(t, exists, "required field %q must appear in properties", field)
    }
}
```

## 测试自定义 Hook

### 单元测试 Hook 逻辑

```go
func TestRateLimitHook_AllowsUnderLimit(t *testing.T) {
    hook := NewRateLimitHook(3, time.Minute)

    ctx := &hook.HookContext{
        SessionID:    "session-1",
        CurrentPoint: hook.BeforeLLMCall,
        Logger:       slog.Default(),
    }

    // 前 3 次应该通过
    for i := 0; i < 3; i++ {
        err := hook.Execute(ctx)
        assert.NoError(t, err)
    }

    // 第 4 次应该被限制
    err := hook.Execute(ctx)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "rate limit")
}
```

### 测试 Hook 修改上下文

```go
func TestDataMaskingHook_MasksSensitiveData(t *testing.T) {
    h := NewDataMaskingHook()

    messages := []entity.MessageForLLM{
        {Role: "user", Content: "My card is 6222021234567890 and phone is 13812345678"},
    }

    ctx := &hook.HookContext{
        CurrentPoint: hook.OnMessagePersist,
        Messages:     &messages,
        Logger:       slog.Default(),
    }

    err := h.Execute(ctx)
    require.NoError(t, err)

    // 手机号中间 4 位应该被遮盖
    assert.Contains(t, messages[0].Content, "138****5678")
    // 银行卡号中间位应该被遮盖
    assert.NotContains(t, messages[0].Content, "6222021234567890")
}
```

### 测试 Hook 优先级

```go
func TestHookExecutionOrder(t *testing.T) {
    runner := hook.NewHookRunner()

    var order []string

    runner.Register(&testHook{name: "low", priority: 50, onExecute: func() { order = append(order, "low") }})
    runner.Register(&testHook{name: "high", priority: 300, onExecute: func() { order = append(order, "high") }})
    runner.Register(&testHook{name: "mid", priority: 200, onExecute: func() { order = append(order, "mid") }})

    chatCtx := &mockChatCtx{ctx: context.Background(), sessionID: "test", agentID: "test"}
    runner.On(hook.BeforeLLMCall, chatCtx, slog.Default())

    assert.Equal(t, []string{"high", "mid", "low"}, order)
}

type testHook struct {
    name      string
    priority  int
    onExecute func()
}

func (h *testHook) Name() string                   { return h.name }
func (h *testHook) Points() []hook.HookPoint        { return []hook.HookPoint{hook.BeforeLLMCall} }
func (h *testHook) Priority() int                   { return h.priority }
func (h *testHook) Execute(_ *hook.HookContext) error { h.onExecute(); return nil }
```

## 测试自定义 Provider

### 内存数据库测试

使用 SQLite 内存模式或 GORM 的 DryRun 模式：

```go
func setupTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    require.NoError(t, err)
    require.NoError(t, AutoMigrate(db))
    return db
}

func TestSessionStore_CRUD(t *testing.T) {
    db := setupTestDB(t)
    store := NewStore(db).Sessions()
    ctx := context.Background()

    // Create
    session, err := store.Create(ctx, &storage.Session{
        Title:          "Test",
        DefaultAgentID: "agent",
    })
    require.NoError(t, err)
    assert.NotEqual(t, uuid.Nil, session.ID)

    // Get
    got, err := store.Get(ctx, session.ID)
    require.NoError(t, err)
    assert.Equal(t, "Test", got.Title)

    // UpdateTitle
    err = store.UpdateTitle(ctx, session.ID, "Updated")
    require.NoError(t, err)
    got, err = store.Get(ctx, session.ID)
    require.NoError(t, err)
    assert.Equal(t, "Updated", got.Title)

    // Delete
    err = store.Delete(ctx, session.ID)
    require.NoError(t, err)
    _, err = store.Get(ctx, session.ID)
    assert.ErrorIs(t, err, ErrSessionNotFound)
}
```

### 分页测试

```go
func TestSessionStore_Pagination(t *testing.T) {
    db := setupTestDB(t)
    store := NewStore(db).Sessions()
    ctx := context.Background()

    // 创建 25 条记录
    for i := 0; i < 25; i++ {
        _, err := store.Create(ctx, &storage.Session{
            Title: fmt.Sprintf("Session %d", i),
        })
        require.NoError(t, err)
    }

    // 第一页
    sessions, total, err := store.List(ctx, 10, 0)
    require.NoError(t, err)
    assert.Equal(t, int64(25), total)
    assert.Len(t, sessions, 10)

    // 第二页
    sessions, total, err = store.List(ctx, 10, 10)
    require.NoError(t, err)
    assert.Equal(t, int64(25), total)
    assert.Len(t, sessions, 10)

    // 最后一页
    sessions, total, err = store.List(ctx, 10, 20)
    require.NoError(t, err)
    assert.Equal(t, int64(25), total)
    assert.Len(t, sessions, 5)
}
```

### 并发测试

```go
func TestSessionStore_ConcurrentWrites(t *testing.T) {
    db := setupTestDB(t)
    store := NewStore(db).Sessions()
    ctx := context.Background()

    var wg sync.WaitGroup
    errors := make(chan error, 100)

    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(n int) {
            defer wg.Done()
            _, err := store.Create(ctx, &storage.Session{
                Title: fmt.Sprintf("Concurrent %d", n),
            })
            if err != nil {
                errors <- err
            }
        }(i)
    }

    wg.Wait()
    close(errors)

    for err := range errors {
        t.Errorf("concurrent write failed: %v", err)
    }

    sessions, total, err := store.List(ctx, 200, 0)
    require.NoError(t, err)
    assert.Equal(t, int64(100), total)
    assert.Len(t, sessions, 100)
}
```

## 集成测试

### Testcontainers 模式

集成测试需要真实的数据库实例。使用 [testcontainers-go](https://golang.testcontainers.org/) 自动管理容器生命周期：

```go
package postgres_test

import (
    "context"
    "testing"
    "time"

    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/wait"
    "gorm.io/driver/postgres"
    gormio "gorm.io/gorm"

    mypg "github.com/yourorg/copcon-mypg"
    "github.com/stretchr/testify/require"
)

func setupPostgresContainer(t *testing.T) *gormio.DB {
    t.Helper()
    ctx := context.Background()

    c, err := postgres.Run(ctx,
        "postgres:15-alpine",
        postgres.WithDatabase("copcon_test"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2).
                WithStartupTimeout(5*time.Second)),
    )
    require.NoError(t, err)

    t.Cleanup(func() { c.Terminate(ctx) })

    connStr, err := c.ConnectionString(ctx, "sslmode=disable")
    require.NoError(t, err)

    db, err := gormio.Open(postgres.Open(connStr), &gormio.Config{})
    require.NoError(t, err)

    return db
}

func TestPostgresStore_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    db := setupPostgresContainer(t)
    store := mypg.NewStore(db)
    ctx := context.Background()

    t.Run("session lifecycle", func(t *testing.T) {
        created, err := store.Sessions().Create(ctx, &storage.Session{
            Title: "Integration Test",
        })
        require.NoError(t, err)

        got, err := store.Sessions().Get(ctx, created.ID)
        require.NoError(t, err)
        require.Equal(t, "Integration Test", got.Title)
    })
}
```

### 环境变量控制

通过环境变量区分单元测试和集成测试：

```go
func TestMain(m *testing.M) {
    if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
        fmt.Println("Skipping integration tests. Set RUN_INTEGRATION_TESTS=1 to run them.")
        os.Exit(0)
    }
    os.Exit(m.Run())
}
```

运行方式：

```bash
# 只跑单元测试
go test ./...

# 包含集成测试
RUN_INTEGRATION_TESTS=1 go test ./...
```

## Mock 策略

### Store Mock

创建轻量的内存实现用于测试：

```go
type MemorySessionStore struct {
    mu       sync.RWMutex
    sessions map[uuid.UUID]*storage.Session
}

func NewMemorySessionStore() *MemorySessionStore {
    return &MemorySessionStore{sessions: make(map[uuid.UUID]*storage.Session)}
}

func (s *MemorySessionStore) Create(_ context.Context, session *storage.Session) (*storage.Session, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if session.ID == uuid.Nil {
        session.ID = uuid.New()
    }
    session.CreatedAt = time.Now()
    session.UpdatedAt = time.Now()
    s.sessions[session.ID] = session
    return session, nil
}

func (s *MemorySessionStore) Get(_ context.Context, id uuid.UUID) (*storage.Session, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    session, ok := s.sessions[id]
    if !ok {
        return nil, fmt.Errorf("session not found: %s", id)
    }
    return session, nil
}

// ... 其他方法类似
```

### LLM Provider Mock

```go
type FixedResponseProvider struct {
    chunks []llm.StreamChunk
}

func (p *FixedResponseProvider) Stream(ctx context.Context, params llm.StreamParams) (<-chan llm.StreamChunk, <-chan error) {
    ch := make(chan llm.StreamChunk, len(p.chunks)+1)
    errc := make(chan error, 1)

    for _, chunk := range p.chunks {
        ch <- chunk
    }
    ch <- llm.StreamChunk{FinishReason: "stop", Usage: &llm.Usage{TotalTokens: 10}}

    close(ch)
    close(errc)
    return ch, errc
}

// 使用示例：模拟带工具调用的响应
func TestWithToolCallResponse(t *testing.T) {
    provider := &FixedResponseProvider{
        chunks: []llm.StreamChunk{
            {Content: "I'll search for that."},
            {
                ToolCalls: []llm.ToolCallDelta{
                    {Index: 0, ID: "call_1", Name: "search", Arguments: `{"q":"test"}`},
                },
            },
        },
    }
    // ...
}
```

### 全量 Mock StoreProvider

```go
type MockStoreProvider struct {
    Sessions_ *MemorySessionStore
    Messages_ *MemoryMessageStore
    Todos_    *MemoryTodoStore
}

func NewMockStoreProvider() *MockStoreProvider {
    return &MockStoreProvider{
        Sessions_: NewMemorySessionStore(),
        Messages_: NewMemoryMessageStore(),
        Todos_:    NewMemoryTodoStore(),
    }
}

func (p *MockStoreProvider) Sessions() storage.SessionStore { return p.Sessions_ }
func (p *MockStoreProvider) Messages() storage.MessageStore { return p.Messages_ }
func (p *MockStoreProvider) Todos() storage.TodoStore       { return p.Todos_ }
```

## 性能基准测试

### Store 基准

```go
func BenchmarkSessionStore_Create(b *testing.B) {
    db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    AutoMigrate(db)
    store := NewStore(db).Sessions()
    ctx := context.Background()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        store.Create(ctx, &storage.Session{
            Title: fmt.Sprintf("Benchmark %d", i),
        })
    }
}

func BenchmarkSessionStore_Get(b *testing.B) {
    db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    AutoMigrate(db)
    store := NewStore(db).Sessions()
    ctx := context.Background()

    // 预创建一条记录
    session, _ := store.Create(ctx, &storage.Session{Title: "Benchmark"})

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        store.Get(ctx, session.ID)
    }
}

func BenchmarkMessageStore_List(b *testing.B) {
    db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    AutoMigrate(db)
    store := NewStore(db)
    ctx := context.Background()

    session, _ := store.Sessions().Create(ctx, &storage.Session{Title: "Benchmark"})

    // 预创建 1000 条消息
    for i := 0; i < 1000; i++ {
        store.Messages().Add(ctx, &storage.Message{
            SessionID: session.ID,
            Role:      "user",
            Content:   fmt.Sprintf("Message %d", i),
        })
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        store.Messages().List(ctx, session.ID, 50)
    }
}
```

运行基准测试：

```bash
go test -bench=. -benchmem ./providers/mypg/
```

### Tool 基准

```go
func BenchmarkWeatherTool_Execute(b *testing.B) {
    tool := NewWeatherTool("test-key")
    ctx := &mockChatCtx{ctx: context.Background()}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        tool.Execute(ctx, map[string]any{"city": "Tokyo"})
    }
}
```

## CI/CD 集成

### GitHub Actions 配置

```yaml
name: Test

on: [push, pull_request]

jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - name: Run unit tests
        run: |
          cd core && go test ./... -count=1 -short
          cd server && go test ./internal/... -count=1 -short

  integration:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15-alpine
        env:
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
          POSTGRES_DB: copcon_test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - name: Run integration tests
        env:
          RUN_INTEGRATION_TESTS: "1"
          DATABASE_HOST: localhost
          DATABASE_PORT: 5432
          DATABASE_USER: test
          DATABASE_PASSWORD: test
          DATABASE_DBNAME: copcon_test
        run: |
          cd server && go test -run "Integration" -v

  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - name: Run benchmarks
        run: |
          cd core && go test -bench=. -benchmem ./providers/...
```

### Makefile 目标

```makefile
.PHONY: test test-unit test-integration test-bench

test: test-unit test-integration

test-unit:
	cd core && go test ./... -count=1 -short
	cd server && go test ./internal/... -count=1 -short

test-integration:
	RUN_INTEGRATION_TESTS=1 cd server && go test -run "Integration" -v

test-bench:
	cd core && go test -bench=. -benchmem ./...

test-coverage:
	cd core && go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

## 测试清单

每种自定义实现的测试要点：

### Tool 测试

- [ ] 接口合规性（`var _ tool.Tool = ...`）
- [ ] 必填参数缺失时的错误返回
- [ ] 参数类型错误时的错误返回
- [ ] 参数范围/枚举校验
- [ ] 成功路径返回正确结果
- [ ] Schema 与实际参数处理一致
- [ ] 上下文取消时的行为
- [ ] 超时时的行为

### Hook 测试

- [ ] 接口合规性（`var _ hook.Hook = ...`）
- [ ] 各 HookPoint 的正确行为
- [ ] 不关注的 HookPoint 被忽略
- [ ] 上下文修改生效（`*Messages`、`*SystemPrompt` 等）
- [ ] nil 字段不触发 panic
- [ ] 并发安全（如果 Hook 有可变状态）
- [ ] 优先级排序正确

### Provider 测试

- [ ] 接口合规性（`var _ storage.StoreProvider = ...`）
- [ ] 所有分接口合规性
- [ ] CRUD 操作正确性
- [ ] 分页和排序
- [ ] 不存在记录的错误处理
- [ ] 并发写入安全
- [ ] 连接失败时的错误信息
- [ ] 迁移幂等性

### LLM Adapter 测试

- [ ] 接口合规性（`var _ llm.LLMProvider = ...`）
- [ ] 流式响应正确转发
- [ ] Tool Call 增量累积正确
- [ ] Usage 统计在最终 chunk 中
- [ ] 上下文取消时 goroutine 不泄漏
- [ ] 通道正确关闭
- [ ] 错误通过 errc 传递
- [ ] 消息格式转换正确

## 下一步

- [自定义 Tool](custom-tool.md)
- [自定义 Hook](custom-hook.md)
- [自定义 Provider](custom-provider.md)
- [自定义 LLM Adapter](custom-llm-adapter.md)
