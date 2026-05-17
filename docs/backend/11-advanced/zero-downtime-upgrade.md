# 零停机升级

## 概述

CopCon 的架构支持在不中断服务的情况下进行升级和配置更新。核心机制包括：Hook 热重载、原子性的 HookRunner 替换、以及基于信号量的优雅关闭。

## 架构概览

```
请求到达时的执行路径：

HTTP Request → Gin Handler → AgentEngine
                                  ├── AgentRegistry (atomic 替换)
                                  ├── HookRunner   (atomic 替换)
                                  │     └── Hooks (动态注册/取消)
                                  ├── ToolManager
                                  └── LLM Provider
```

可热更新的组件：
- **Hook** — 动态注册新的 Hook 或替换现有逻辑，无需重启
- **AgentRegistry** — 从数据库动态加载 Agent 定义
- **LLM Provider** — 切换模型或 API 配置

## Hook 热重载策略

### 原理

`HookRunner` 的 `Run` 方法在执行前会获取注册 Hook 的快照（`hooksSnapshot`），然后再逐个执行。这意味着：

1. 正在执行的 Agent 循环使用的是旧 Hook 集合
2. 新注册的 Hook 只对新开始的 Agent 循环生效
3. 单个 Hook 的 panic 或错误不会影响其他 Hook 的执行

```go
// runner.go 中的关键代码片段
func (r *hookRunner) Run(point HookPoint, ctx *HookContext) {
    if err := ctx.ChatCtx.Context().Err(); err != nil {
        return // 上下文已取消，跳过
    }

    r.mu.Lock()
    // 获取快照（复制一份），释放锁后执行
    candidates := make([]hookEntry, len(r.entries))
    copy(candidates, r.entries)
    r.mu.Unlock()

    // 过滤并排序，然后逐个执行
    // ...
}
```

### 实现 Hook 热重载

```go
// HotReloadManager 管理 Hook 的运行时更新
type HotReloadManager struct {
    runner      hook.HookRunner
    pluginDir   string
    fileWatcher *fsnotify.Watcher
}

func NewHotReloadManager(runner hook.HookRunner, pluginDir string) *HotReloadManager {
    return &HotReloadManager{
        runner:    runner,
        pluginDir: pluginDir,
    }
}

func (h *HotReloadManager) Start(ctx context.Context) error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }
    h.fileWatcher = watcher

    // 监听插件目录
    if err := watcher.Add(h.pluginDir); err != nil {
        return err
    }

    go func() {
        for {
            select {
            case event := <-watcher.Events:
                if event.Op&fsnotify.Create != 0 || event.Op&fsnotify.Write != 0 {
                    // 动态加载新插件
                    if err := h.reloadPlugin(event.Name); err != nil {
                        slog.Error("failed to reload plugin",
                            "file", event.Name,
                            "error", err,
                        )
                    }
                }
            case err := <-watcher.Errors:
                slog.Error("file watcher error", "error", err)
            case <-ctx.Done():
                watcher.Close()
                return
            }
        }
    }()

    return nil
}

func (h *HotReloadManager) reloadPlugin(filePath string) error {
    // 从 .go 文件动态加载（通过 go plugin 或 重新解析配置）
    // Go 的 plugin 包在 macOS 上有限制，生产建议使用配置重新解析方式

    // 简化的配置重新解析方式：
    data, err := os.ReadFile(filePath)
    if err != nil {
        return err
    }

    var pluginConfig PluginConfig
    if err := yaml.Unmarshal(data, &pluginConfig); err != nil {
        return err
    }

    // 创建新 Hook 实例
    newHook := createHookFromConfig(pluginConfig)

    // 如果 Hook 支持版本号，先取消旧版本再注册新版本
    h.runner.Register(newHook)

    slog.Info("plugin reloaded", "name", newHook.Name())
    return nil
}
```

## 原子性 HookRunner 替换

### 场景

当更新背后的 HookRunner 实例时（例如添加新的内置钩子），需确保正在进行中的 Agent 循环不受影响。

```go
// SwitchableRunner 提供原子性替换 HookRunner
type SwitchableRunner struct {
    mu      sync.RWMutex
    current hook.HookRunner
}

func NewSwitchableRunner(initial hook.HookRunner) *SwitchableRunner {
    return &SwitchableRunner{current: initial}
}

// Register 代理到当前 runner
func (s *SwitchableRunner) Register(h hook.Hook) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    s.current.Register(h)
}

// Run 使用当前 runner（快照）
func (s *SwitchableRunner) Run(point hook.HookPoint, ctx *hook.HookContext) {
    s.mu.RLock()
    runner := s.current
    s.mu.RUnlock()
    runner.Run(point, ctx)
}

// On 使用当前 runner（快照）
func (s *SwitchableRunner) On(point hook.HookPoint, chatCtx iface.ChatContextInterface, logger *slog.Logger, extras ...hook.HookExtra) {
    s.mu.RLock()
    runner := s.current
    s.mu.RUnlock()
    runner.On(point, chatCtx, logger, extras...)
}

// Swap 原子性地替换为新的 HookRunner
// 正在执行的 Agent 循环继续使用旧 runner，新循环使用新 runner
func (s *SwitchableRunner) Swap(newRunner hook.HookRunner) {
    s.mu.Lock()
    defer s.mu.Unlock()

    old := s.current
    s.current = newRunner

    slog.Info("hook runner swapped",
        "old_hook_count", len(old.(*hookRunner).entries()), // 需要暴露接口
        "new_hook_count", len(newRunner.(*hookRunner).entries()),
    )
}
```

### 集成到 main.go

```go
// 创建可热替换的 Runner
runner := hook.NewHookRunner()
switchable := NewSwitchableRunner(runner)

// 注册初始 Hooks
runner.Register(plugins.NewTodoInjectionHook(todoMgr))
runner.Register(memoryplugin.NewMemoryPlugin(memoryMgr))

// 使用 switchable runner 创建引擎
agentEngine := agent.NewAgentEngine(
    agentRegistry, sessionMgr, contextMgr, asyncRegistry,
    agent.WithHookRunner(switchable),
)

// 启动热更新监听
// ...
```

## 优雅关闭

### 当前实现的问题

CopCon 当前的 `main.go` 缺少优雅关闭逻辑。`r.Run()` 是阻塞调用，`SIGTERM` 会导致所有进行中的 Agent 循环被中断。

### 改进方案

```go
func main() {
    // ... 初始化代码 ...

    srv := &http.Server{
        Addr:    ":" + cfg.Server.Port,
        Handler: r,
        // 设置读写超时（SSE 连接除外）
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 0,  // SSE 连接必须为 0（无限）
        IdleTimeout:  120 * time.Second,
    }

    // 在 goroutine 中启动
    go func() {
        logger.Info("Server starting", "port", cfg.Server.Port)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("Server failed", "error", err)
            os.Exit(1)
        }
    }()

    // 等待中断信号
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    logger.Info("Received shutdown signal")

    // 1. 停止接收新请求（设置 30s 超时）
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // 2. 关闭 HTTP Server
    if err := srv.Shutdown(ctx); err != nil {
        logger.Error("Server shutdown error", "error", err)
    }

    // 3. 等待所有进行中的 Agent 循环完成
    //    可以通过 WaitGroup 或 Context 实现
    //    （需要引擎暴露完成信号）

    // 4. 关闭数据库连接池
    sqlDB, _ := db.DB()
    sqlDB.Close()

    // 5. 关闭 Qdrant 连接
    //    memoryMgr.Close()

    logger.Info("Server shutdown complete")
}
```

### Docker 配置

```yaml
# docker-compose.yaml
server:
  stop_grace_period: 30s         # 给 30 秒完成优雅关闭
```

### 处理进行中的 SSE 连接

SSE 连接是长连接，优雅关闭时需妥善处理：

```go
// 在 Shutdown 前通知所有活跃 SSE 连接
func (s *Server) notifyShutdown() {
    // 每个活跃的 ChatContext 发送关闭事件
    for _, chatCtx := range s.activeSessions() {
        chatCtx.Emit(entity.Event{
            Type: "error",
            Data: entity.ErrorData{Error: "server shutting down"},
        })
        chatCtx.Close()
    }
}
```

## 插件版本化

### 方案

为 Hook 实现添加版本信息，支持平滑切换：

```go
type VersionedHook struct {
    base    hook.Hook
    version string
    replaced []string  // 替换的旧 Hook 名称
}

func (h *VersionedHook) Name() string {
    return fmt.Sprintf("%s@%s", h.base.Name(), h.version)
}

// 注册新版本时注销旧版本
func (r *HookRunner) Replace(oldName, newHook hook.Hook) {
    r.mu.Lock()
    defer r.mu.Unlock()

    // 移除旧条目（按名称匹配）
    kept := make([]hookEntry, 0)
    for _, entry := range r.entries {
        if entry.hook.Name() != oldName {
            kept = append(kept, entry)
        }
    }
    r.entries = kept

    // 添加新条目
    r.entries = append(r.entries, hookEntry{
        hook:      newHook,
        createdAt: time.Now(),
    })
}
```

## 部署流程

### Docker Compose 滚动更新

```bash
# 1. 构建新镜像
docker compose build server

# 2. 滚动重启（create new → start healthy → stop old）
docker compose up -d --no-deps --scale server=2 server
# ── 需要先修改 docker-compose.yaml 支持多实例

# 3. 健康检查通过后缩容旧实例
docker compose up -d --no-deps --scale server=1 server

# 4. 验证
curl http://localhost:8080/health
```

**注意：** 当前 CopCon 单实例部署。多实例需要：
- PostgreSQL 和 Qdrant 支持多连接
- Agent Engine 的并发安全（已满足，`sync.RWMutex` 保护关键路径）
- 无状态化设计（所有状态在 DB/Qdrant 中，无本地内存状态）

## 总结

| 组件 | 热更新方案 | 影响 |
|------|-----------|------|
| Hook | `HookRunner.Register()` & `SwitchableRunner.Swap()` | 新请求生效，进行中请求不受影响 |
| AgentRegistry | 实现自定义 Registry + 数据库动态加载 | 新请求生效 |
| LLM Provider | 通过 AgentRegistry 间接更新 | Agent 级别生效 |
| Server 二进制 | Docker 滚动更新 + 优雅关闭 | 短暂中断（通过负载均衡解决） |
| Config | 环境变量 / ConfigMap + 重启 | 需重启 |