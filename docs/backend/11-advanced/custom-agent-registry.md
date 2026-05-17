# 自定义 Agent 注册中心

## 概述

CopCon 默认从 `config.yaml` 加载 Agent 定义。通过实现 `AgentRegistry` 接口，可以构建自定义的注册中心，从数据库、远程配置中心、或动态注册表加载 Agent。

## AgentRegistry 接口

定义位置：`server/internal/agent/registry.go`

```go
type AgentRegistry interface {
    // Get 根据 ID 获取 Agent 定义
    Get(id string) (AgentDefinition, error)

    // List 列出所有可用 Agent 的摘要信息
    List() []AgentInfo

    // Default 获取默认 Agent
    Default() (AgentDefinition, error)
}
```

### AgentDefinition

```go
type AgentDefinition struct {
    ID           string          // Agent 唯一标识
    Name         string          // 显示名称
    Model        string          // 使用的模型（如 "gpt-4o"）
    SystemPrompt string          // 系统提示词
    ToolManager  tool.ToolManager // 该 Agent 可用的工具
    LLMProvider  llm.LLMProvider  // LLM 调用提供者
}
```

### AgentInfo

```go
type AgentInfo struct {
    ID    string
    Name  string
    Model string
}
```

## 默认实现：config-file 注册中心

当前 `agentRegistry` 的实现（`registry.go:42-146`）从 `config.yaml` 加载 Agent：

```go
func NewAgentRegistry(cfg *config.Config, toolRegistry tool.ToolRegistry) (AgentRegistry, error) {
    registry := &agentRegistry{
        agents:       make(map[string]AgentDefinition),
        defaultAgent: cfg.DefaultAgentID,
    }

    for _, agentConfig := range cfg.Agents {
        // 校验工具是否存在
        // 创建 ToolManager（仅包含该 Agent 声明的工具）
        // 创建 OpenAI Client
        // 构建 AgentDefinition
        registry.agents[agentConfig.ID] = agent
    }

    return registry, nil
}
```

它使用 `sync.RWMutex` 保证并发安全。

## 实现自定义注册中心

### 示例 1：PostgreSQL 支持的 Agent 注册中心

将 Agent 配置存储在数据库中，支持运行时动态变更而不重启服务。

```go
package registry

import (
    "sync"
    "time"

    "gorm.io/gorm"
    "github.com/openai/openai-go/v3"
    "github.com/openai/openai-go/v3/option"

    "github.com/copcon/server/internal/agent"
    "github.com/copcon/server/internal/llm"
    "github.com/copcon/server/internal/tool"
)

// AgentRecord 数据库模型
type AgentRecord struct {
    ID           string    `gorm:"primaryKey"`
    Name         string
    Model        string
    SystemPrompt string    `gorm:"type:text"`
    Tools        string    `gorm:"type:text"` // JSON 数组: ["code_executor","file_ops"]
    APIKey       string    // 该 Agent 专用的 API Key（加密存储）
    BaseURL      string
    IsDefault    bool
    IsActive     bool      `gorm:"default:true"`
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

func (AgentRecord) TableName() string {
    return "agents"
}

// DBAgentRegistry 从 PostgreSQL 加载 Agent
type DBAgentRegistry struct {
    mu           sync.RWMutex
    db           *gorm.DB
    toolRegistry tool.ToolRegistry
    cache        map[string]agent.AgentDefinition
    defaultAgent string
    lastReload   time.Time
}

func NewDBAgentRegistry(db *gorm.DB, toolRegistry tool.ToolRegistry) (*DBAgentRegistry, error) {
    r := &DBAgentRegistry{
        db:           db,
        toolRegistry: toolRegistry,
        cache:        make(map[string]agent.AgentDefinition),
    }

    // 自动迁移
    if err := db.AutoMigrate(&AgentRecord{}); err != nil {
        return nil, err
    }

    // 初始加载
    if err := r.reload(); err != nil {
        return nil, err
    }

    return r, nil
}

// reload 从数据库重新加载所有 Agent 定义到内存缓存
func (r *DBAgentRegistry) reload() error {
    var records []AgentRecord
    if err := r.db.Where("is_active = ?", true).Find(&records).Error; err != nil {
        return err
    }

    newCache := make(map[string]agent.AgentDefinition)
    var defaultID string

    for _, record := range records {
        // 解析工具列表
        var toolNames []string
        json.Unmarshal([]byte(record.Tools), &toolNames)

        // 创建 ToolManager
        toolMgr := tool.NewToolManager()
        for _, name := range toolNames {
            t, err := r.toolRegistry.Get(name)
            if err != nil {
                // 跳过不存在的工具（打日志即可）
                continue
            }
            toolMgr.Register(t)
        }

        // 创建 LLM Provider
        apiKey := record.APIKey
        if apiKey == "" {
            // 如果该 Agent 没有专用 Key，回退到全局 Key
            // 从环境变量或全局配置读取
        }

        opts := []option.RequestOption{option.WithAPIKey(apiKey)}
        if record.BaseURL != "" {
            opts = append(opts, option.WithBaseURL(record.BaseURL))
        }
        client := openai.NewClient(opts...)

        newCache[record.ID] = agent.AgentDefinition{
            ID:           record.ID,
            Name:         record.Name,
            Model:        record.Model,
            SystemPrompt: record.SystemPrompt,
            ToolManager:  toolMgr,
            LLMProvider:  llm.NewOpenAIAdapter(&client, record.Model),
        }

        if record.IsDefault {
            defaultID = record.ID
        }
    }

    r.mu.Lock()
    r.cache = newCache
    r.defaultAgent = defaultID
    r.lastReload = time.Now()
    r.mu.Unlock()

    return nil
}

func (r *DBAgentRegistry) Get(id string) (agent.AgentDefinition, error) {
    r.mu.RLock()
    def, ok := r.cache[id]
    r.mu.RUnlock()

    if !ok {
        return agent.AgentDefinition{}, agent.ErrAgentNotFound
    }
    return def, nil
}

func (r *DBAgentRegistry) List() []agent.AgentInfo {
    r.mu.RLock()
    defer r.mu.RUnlock()

    infos := make([]agent.AgentInfo, 0, len(r.cache))
    for _, def := range r.cache {
        infos = append(infos, agent.AgentInfo{
            ID:    def.ID,
            Name:  def.Name,
            Model: def.Model,
        })
    }
    return infos
}

func (r *DBAgentRegistry) Default() (agent.AgentDefinition, error) {
    r.mu.RLock()
    defaultID := r.defaultAgent
    r.mu.RUnlock()

    if defaultID == "" {
        return agent.AgentDefinition{}, agent.ErrNoDefaultAgent
    }
    return r.Get(defaultID)
}

// Reload 手动触发热重载（可通过 API 端点暴露）
func (r *DBAgentRegistry) Reload() error {
    return r.reload()
}
```

### 使用方式

```go
// 在 main.go 中替换 agent.NewAgentRegistry
registry, err := NewDBAgentRegistry(db, toolRegistry)
if err != nil {
    log.Fatal(err)
}

agentEngine := agent.NewAgentEngine(registry, sessionMgr, contextMgr, asyncRegistry)
```

### 数据库插入示例

```sql
INSERT INTO agents (id, name, model, system_prompt, tools, api_key, base_url, is_default, is_active)
VALUES (
    'code-assistant',
    'Code Assistant',
    'gpt-4o',
    'You are a helpful coding assistant.',
    '["code_executor", "shell_executor", "file_ops", "todolist"]',
    '',  -- 留空则使用全局 OPENAI_API_KEY
    '',  -- 留空则使用全局 OPENAI_BASE_URL
    true,
    true
);
```

### 示例 2：远程配置中心注册中心

从 etcd / Consul / Nacos 实时获取 Agent 配置：

```go
type RemoteAgentRegistry struct {
    mu      sync.RWMutex
    client  *etcd.Client  // 或 Consul API
    prefix  string        // e.g., "/copcon/agents/"
    agents  map[string]agent.AgentDefinition
    default string
}

func (r *RemoteAgentRegistry) watch() {
    // 使用 etcd Watch 监听配置变更
    watchChan := r.client.Watch(context.Background(), r.prefix, etcd.WithPrefix())
    for resp := range watchChan {
        for _, ev := range resp.Events {
            if ev.Type == etcd.EventTypePut {
                r.addOrUpdateAgent(ev.Kv.Key, ev.Kv.Value)
            } else if ev.Type == etcd.EventTypeDelete {
                r.removeAgent(ev.Kv.Key)
            }
        }
    }
}
```

## 动态 Agent 加载策略

### 方案对比

| 方案 | 配置变更生效 | 适用场景 |
|------|-------------|---------|
| config.yaml | 重启服务 | 静态配置，Agent 不变 |
| PostgreSQL | 调用 Reload API | 运营后台管理，无需重启 |
| 远程配置中心 | 实时（Watch） | 多实例同步，自动下发 |
| 混合方案 | 按需 | 基础配置在 config.yaml，动态部分在 DB |

### 注意

- 动态加载时，`AgentDefinition` 中的 `ToolManager` 和 `LLMProvider` 需要重新创建（涉及新的 HTTP Client 和连接）
- 如果频繁更新，考虑使用对象池避免过多的 Client 创建
- 旧 Client 的连接不会被立即释放，由 Go GC 回收

## 集成到现有系统

无论使用哪种注册中心实现，只需要在 `main.go` 中替换注册中心的初始化即可：

```go
// before: config file registry
agentRegistry, err := agent.NewAgentRegistry(cfg, toolRegistry)

// after: DB-backed registry
agentRegistry, err := NewDBAgentRegistry(db, toolRegistry)
```

其余代码（API Handler、Agent Engine）完全不需要修改，因为它们仅依赖 `AgentRegistry` 接口。