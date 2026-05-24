# 配置指南

本指南详细介绍如何配置 CopCon 的核心库,包括 Harness 配置、存储、LLM Provider、工具和 Hook 等。

## 配置文件 (HarnessConfig)

Harness 是 CopCon 的核心,通过 `HarnessConfig` 进行配置:

```go
type HarnessConfig struct {
    Store     StoreConfig    `yaml:"store"`     // 存储配置
    Default   string         `yaml:"default"`   // 默认 Agent 名称
    Agents    []AgentSpec    `yaml:"agents"`    // Agent 定义列表
    Tools     []ToolSpec     `yaml:"tools"`     // 全局工具
    Hooks     []HookSpec     `yaml:"hooks"`     // 全局 Hook
}
```

## Agent 配置

### AgentSpec 结构

```go
type AgentSpec struct {
    Name         string    `yaml:"name"`          // Agent 唯一名称
    Model        string    `yaml:"model"`         // LLM 模型名称 (gpt-4, gpt-3.5-turbo)
    Endpoint     string    `yaml:"endpoint,omitempty"` // 自定义 endpoint (可选)
    SystemPrompt string    `yaml:"system_prompt"` // 系统提示词
    Temperature  float64   `yaml:"temperature,omitempty"` // 温度 (0.0-2.0)
    MaxTokens    int       `yaml:"max_tokens,omitempty"` // 最大响应 token 数
    Tools        []string  `yaml:"tools,omitempty"` // 工具列表
    Hooks        []string  `yaml:"hooks,omitempty"` // Hook 列表
    Timeout      string    `yaml:"timeout,omitempty"` // 超时时间 (如: "30s")
}
```

### 示例配置

```yaml
agents:
  - name: "assistant"
    model: "gpt-4"
    system_prompt: |
      你是一个友好的AI助手。
      你可以帮助用户回答问题、提供建议和执行任务。
      请用简洁、清晰的语言回答。
    temperature: 0.7
    max_tokens: 2000
    tools:
      - "search"
      - "code_executor"
    hooks:
      - "logging"
    timeout: "60s"
  
  - name: "coder"
    model: "gpt-4"
    system_prompt: |
      你是一个专业的程序员。
      你擅长编写高质量、可维护的代码。
    temperature: 0.3
    max_tokens: 4000
    tools:
      - "code_executor"
      - "file_ops"
      - "shell_executor"
    hooks:
      - "logging"
      - "tracing"
```

## 存储配置

### PostgreSQL (推荐)

```yaml
store:
  type: "postgres"
  
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "password"
  dbname: "copcon"
  sslmode: "disable"
  timezone: "Asia/Shanghai"
  
  # 连接池配置
  max_connections: 100
  max_idle_connections: 10
  connection_timeout: "30s"
  
  # 迁移配置
  auto_migrate: true
  drop_tables: false   # 危险! 仅用于开发
  
  # 缓存配置 (可选)
  cache_type: "redis"
  cache_host: "localhost"
  cache_port: 6379
```

### MongoDB

```yaml
store:
  type: "mongodb"
  
  uri: "mongodb://localhost:27017"
  database: "copcon"
  
  # 集合前缀 (可选)
  collection_prefix: "copcon_"
  
  # 连接池
  max_connections: 100
  min_connections: 10
```

### SQLite (开发/测试)

```yaml
store:
  type: "sqlite"
  
  # 数据库文件路径
  path: "./data/copcon.db"
  
  # 内存数据库 (测试用)
  # path: ":memory:"
  
  # 性能优化
  journal_mode: "WAL"
  cache_size: 2000
```

### Redis (仅用于缓存,非主存储)

```yaml
store:
  type: "redis"
  
  host: "localhost"
  port: 6379
  password: ""
  db: 0
  
  # 过期时间
  ttl: "1h"
  
  # 最大内存
  max_memory: 256mb
  eviction_policy: allkeys-lfu
```

## LLM Provider 配置

### OpenAI

```yaml
# 环境变量中设置
OPENAI_API_KEY=sk-...
OPENAI_ORG=org-...  # 可选

# 或使用自定义配置
provider:
  type: "openai"
  
  models:
    gpt-4:
      model: "gpt-4"
      max_context_tokens: 128000
      max_output_tokens: 4096
      timeout: "120s"
    
    gpt-3.5-turbo:
      model: "gpt-3.5-turbo"
      max_context_tokens: 16385
      max_output_tokens: 4096
      timeout: "60s"
  
  # 速率限制
  rate_limit:
    requests_per_minute: 60
    tokens_per_minute: 90000
    concurrent_requests: 5
```

### Azure OpenAI

```yaml
provider:
  type: "azure-openai"
  
  endpoint: "https://your-resource.openai.azure.com"
  api_key: "your-api-key"
  api_version: "2024-02-15-preview"
  
  models:
    gpt-4-deployment:
      deployment: "gpt-4-deployment"
      max_context_tokens: 128000
      max_output_tokens: 4096
```

### Ollama (本地部署)

```yaml
provider:
  type: "ollama"
  
  base_url: "http://localhost:11434"
  timeout: "300s"
  
  models:
    llama2:
      model: "llama2"
      max_context_tokens: 4096
      max_output_tokens: 2048
    
    codellama:
      model: "codellama"
      max_context_tokens: 16384
      max_output_tokens: 8192
  
  # 本地模型限制
  max_concurrent: 1
```

## 工具配置

### 全局工具

```yaml
tools:
  - name: "search"
    type: "web_search"
    description: "搜索互联网信息"
    parameters:
      max_results: 5
      timeout: "10s"
      safe_mode: true
  
  - name: "code_executor"
    type: "docker_isolated"
    description: "执行代码 (隔离环境)"
    parameters:
      docker_image: "copcon/sandbox:latest"
      timeout: "30s"
      memory_limit: "512mb"
      network: "none"  # 禁止网络
  
  - name: "file_ops"
    type: "local"
    description: "文件操作"
    parameters:
      root_dir: "/data/copcon"  # 允许访问的目录
      allowed_extensions:
        - ".txt"
        - ".md"
        - ".js"
        - ".py"
      max_size: "10mb"
```

### 工具安全配置

```yaml
security:
  tools:
    # 黑名单: 禁止使用的工具
    blacklist:
      - "shell_executor"     # 禁止在用户环境下执行
    
    # 白名单: 允许访问的网络 (如果使用网络功能)
    network_allowed:
      - "*.github.com"
      - "api.openai.com"
    
    # 资源限制
    max_execution_time: "60s"
    max_output_size: "10kb"
    max_files_per_session: 100
```

## Hook 配置

### 全局 Hook

```yaml
hooks:
  - name: "logging"
    type: "file_logger"
    description: "记录所有对话和工具调用"
    enabled: true
    parameters:
      # 日志文件
      file: "./logs/copcon.log"
      level: "info"  # debug, info, warn, error
      max_size: "100mb"
      max_backups: 30
      
      # 可选: 发送到日志聚合服务
      elasticsearch:
        url: "http://localhost:9200"
        index: "copcon-logs"
  
  - name: "tracing"
    type: "opentelemetry"
    description: "链路追踪"
    enabled: true
    parameters:
      exporter: "jaeger"
      endpoint: "http://localhost:4318"  # OTLP HTTP endpoint
      service_name: "copcon"
      
      # 采样率 (0.0-1.0)
      sampling_rate: 0.1
  
  - name: "memory"
    type: "vector_store"
    description: "长期记忆"
    enabled: true
    parameters:
      # 向量存储配置
      vector_db: "chromadb"
      collection: "conversations"
      embedding_model: "BAAI/bge-base-zh-v1.5"
      
      # 检索配置
      top_k: 5
      min_similarity: 0.7
      
      # 存储策略
      retention_days: 90
      max_memories_per_session: 1000
```

## 性能配置

```yaml
# 并发和队列
concurrency:
  max_concurrent_agents: 10
  max_concurrent_tool_calls: 5
  tool_queue_size: 100
  
# 资源限制
limits:
  max_context_tokens: 128000
  max_output_tokens: 4000
  max_tool_args_length: 1000
  
# 重试策略
retry:
  max_attempts: 3
  backoff_multiplier: 2.0
  initial_delay: "1s"
  max_delay: "30s"
```

## 环境变量优先级

CopCon 支持通过环境变量覆盖配置:

| 环境变量 | 配置项 | 说明 |
|---------|-------|------|
| `COPCON_CONFIG` | - | 指定配置文件路径 |
| `OPENAI_API_KEY` | provider.openai.api_key | OpenAI API 密钥 |
| `AZURE_OPENAI_API_KEY` | provider.azure.api_key | Azure OpenAI API 密钥 |
| `DATABASE_URL` | store.dsn | 数据库连接 URL |
| `COPCON_LOG_LEVEL` | hooks.logging.level | 日志级别 |
| `COPCON_DEBUG` | debug | 启用调试模式 |

## 加载配置

### 从文件加载

```go
package main

import (
    "log"
    
    "github.com/copcon/core"
    "github.com/copcon/core/config"
)

func main() {
    // 加载配置文件
    cfg, err := config.Load("config.yaml")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    
    // 创建 Harness
    harness, err := core.NewHarness(cfg)
    if err != nil {
        log.Fatalf("Failed to create harness: %v", err)
    }
    
    // ...
}
```

### 动态加载 (热重启)

```go
import "github.com/copcon/core/config"

// 监听配置文件变化
watcher := config.NewFileWatcher("config.yaml")

go watcher.Watch(func(cfg *config.Config) {
    log.Println("Config updated, reloading...")
    
    if err := harness.Reload(cfg); err != nil {
        log.Printf("Reload failed: %v", err)
    } else {
        log.Println("Reload successful")
    }
})
```

## 配置文件示例: 完整配置

```yaml
# config.yaml - 完整配置示例

# 存储配置
store:
  type: "postgres"
  
  host: "${DATABASE_HOST:localhost}"
  port: 5432
  user: "${DATABASE_USER:postgres}"
  password: "${DATABASE_PASSWORD:password}"
  dbname: "copcon"
  sslmode: "disable"
  timezone: "Asia/Shanghai"
  
  max_connections: 100
  max_idle_connections: 10
  
  auto_migrate: true
  drop_tables: false

# 默认 Agent (未指定时使用)
default: "assistant"

# Agent 配置
agents:
  - name: "assistant"
    model: "gpt-4"
    system_prompt: |
      你是一个友好的AI助手。
      请用简洁、清晰的语言回答。
    temperature: 0.7
    max_tokens: 2000
    tools:
      - "search"
      - "file_ops"
    hooks:
      - "logging"
      - "tracing"
    timeout: "120s"
  
  - name: "coder"
    model: "gpt-4"
    system_prompt: |
      你是一个专业的程序员。
      你精通: Python, Go, JavaScript, SQL 等。
    temperature: 0.3
    max_tokens: 4000
    tools:
      - "code_executor"
      - "file_ops"
      - "shell_executor"
    hooks:
      - "logging"
      - "tracing"

# 全局工具
tools:
  - name: "search"
    type: "web_search"
    enabled: true
    description: "搜索互联网信息"
    parameters:
      provider: "serper"  # 使用 Serper API
      api_key: "${SERPER_API_KEY}"
      max_results: 5
      timeout: "15s"
      safe_mode: true
  
  - name: "code_executor"
    type: "docker_isolated"
    enabled: true
    description: "在隔离环境中执行代码"
    parameters:
      image: "copcon/sandbox:latest"
      timeout: "30s"
      memory_limit: "512mb"
      cpu_limit: 1.0
      network: "none"
      
      # 支持的语言
      languages:
        - "python3"
        - "javascript"
        - "bash"
  
  - name: "file_ops"
    type: "local"
    enabled: true
    description: "读写文件"
    parameters:
      root_dir: "/data/copcon"
      allowed_extensions:
        - ".txt"
        - ".md"
        - ".json"
        - ".py"
        - ".js"
        - ".go"
      max_size: "10mb"
  
  - name: "shell_executor"
    type: "docker_isolated"
    enabled: true
    description: "执行 shell 命令 (隔离)"
    parameters:
      image: "ubuntu:22.04"
      timeout: "30s"
      max_output_length: 5000

# 全局 Hook
hooks:
  - name: "logging"
    type: "file_logger"
    enabled: true
    description: "记录对话和工具调用"
    parameters:
      file: "./logs/copcon.log"
      level: "info"
      max_size: "100mb"
      max_backups: 30
      max_age_days: 90
  
  - name: "tracing"
    type: "opentelemetry"
    enabled: true
    description: "链路追踪和性能监控"
    parameters:
      exporter: "jaeger"
      endpoint: "http://jaeger:4318"
      service_name: "copcon"
      sampling_rate: 1.0
  
  - name: "memory"
    type: "vector_store"
    enabled: true
    description: "长期记忆和语义搜索"
    parameters:
      # 向量存储
      vector_db: "chromadb"
      collection_name: "conversations"
      persist_directory: "./data/chroma"
      
      # 嵌入模型
      embedding_model: "BAAI/bge-base-zh-v1.5"
      embedding_dimension: 768
      
      # 检索配置
      top_k: 5
      min_similarity: 0.75
      
      # 保留策略
      retention_days: 90
      max_memories_per_session: 1000

# 安全配置
security:
  tools:
    # 工具黑名单 (禁止使用)
    blacklist:
      - "dangerous_tool_name"
    
    # 资源限制
    default_timeout: "60s"
    max_output_size: "10kb"
  
  # 内容安全
  content_filter:
    enabled: true
    keywords_blacklist:
      - "暴力"
      - "色情"
    max_input_length: 8000

# 性能配置
performance:
  concurrency:
    max_concurrent_agents: 20
    max_concurrent_tool_calls: 10
    tool_queue_size: 200
  
  # 重试
  retry:
    max_attempts: 3
    backoff_multiplier: 2.0
    initial_delay: "1s"
  
  # 限流
  rate_limit:
    enabled: true
    requests_per_minute: 60
    tokens_per_minute: 90000

# 监控
monitoring:
  # Prometheus 指标
  prometheus:
    enabled: true
    port: 9090
    path: "/metrics"
  
  health_check:
    enabled: true
    path: "/health"
    interval: "30s"
```

## 最佳实践

### 1. 使用环境变量

敏感信息使用环境变量,不要硬编码在配置文件中:

```yaml
# ❌ 错误: 硬编码密钥
database:
  password: "my-secret-password"

# ✅ 正确: 使用环境变量
database:
  password: "${DATABASE_PASSWORD}"
```

### 2. 配置分层

针对不同环境使用不同配置:

```bash
# 开发环境
config.dev.yaml

# 测试环境
config.test.yaml

# 生产环境
config.prod.yaml
```

加载时指定:

```go
env := os.Getenv("COPCON_ENV")
configFile := fmt.Sprintf("config.%s.yaml", env)
cfg, err := config.Load(configFile)
```

### 3. 配置文件版本控制

```yaml
# config.yaml
version: "1.0"  # 配置版本
```

### 4. 默认配置和覆盖

```go
package config

type Config struct {
    // ...
}

var defaultConfig = Config{
    Store: StoreConfig{
        AutoMigrate: true,
    },
    Concurrency: ConcurrencyConfig{
        MaxConcurrentAgents: 10,
    },
}

func New() *Config {
    cfg := defaultConfig
    // 加载用户配置覆盖默认值
    return &cfg
}
```

## 验证配置

```go
if err := cfg.Validate(); err != nil {
    log.Fatalf("Invalid configuration: %v", err)
}

// 检查必需的配置
if cfg.Store.Type == "" {
    log.Fatal("Store type is required")
}

if len(cfg.Agents) == 0 {
    log.Warn("No agents configured, using default")
    cfg.Agents = []AgentSpec{defaultAgent}
}
```

## 常见问题

### Q: 配置变更后需要重启服务吗?

A: 大部分配置支持热更新,但某些配置 (如存储配置) 需要重启。

### Q: 如何配置自定义模型?

A: 在 `provider.models` 中添加自定义模型配置即可。

### Q: 日志文件过大如何处理?

A: 配置日志文件的最大大小和轮转策略:

```yaml
logging:
  max_size: "100mb"
  max_backups: 30
  max_age_days: 90
```

## 下一步

- [内置工具概览](../05-built-in-capabilities/tools/overview.md)
- [内置 Hook 概览](../05-built-in-capabilities/hooks/overview.md)
- [部署指南](../07-deployment/docker-compose.md)
