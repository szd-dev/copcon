# 升级与迁移指南

本指南说明如何安全升级 CopCon,包括版本兼容性、数据库迁移、回滚步骤和注意事项。

## 版本兼容性

### 版本号规则

CopCon 遵循语义化版本(SemVer):

- **主版本(Major)**: 不兼容的 API 变更,需要迁移
- **次版本(Minor)**: 新功能,向后兼容
- **修订版本(Patch)**: Bug 修复,向后兼容

### 兼容矩阵

| 从版本 | 到版本 | 数据库迁移 | 配置变更 | 破坏性变更 |
|--------|--------|-----------|---------|-----------|
| v1.0 → v1.1 | 无 | 无 | 无 |
| v1.1 → v1.2 | 无 | AgentSpec 新增 `timeout` 字段(可选) | 无 |
| v1.x → v2.0 | **有** | **有** | **有**,详见下文 |

### v1.x 到 v2.0 破坏性变更

这是目前最重要的迁移路径:

| 变更项 | v1.x | v2.0 | 影响 |
|--------|------|------|------|
| Agent 标识 | `name` 字段 | `id` 字段 | config.yaml 中所有 agent 定义需改用 `id` |
| 默认 Agent | `default` 字段 | `default_agent_id` 字段 | 配置字段名变化 |
| 数据库连接 | `database.url` (DSN 字符串) | 分离的 `database.host/port/user/password/dbname` | config.yaml 格式变化 |
| 存储接口 | 单一 `Store` 接口 | 分离的 `SessionStore/MessageStore/TodoStore` | 自定义 Provider 需适配 |
| Qdrant | 必需 | 可选 | Qdrant 不可用时记忆功能静默跳过 |
| 工具注册 | 手动导入 | 自动注册(init()) | server/ 不再需要手动导入 capabilities |

## 升级流程

### 前置准备

1. **阅读 Release Notes**: 确认是否有破坏性变更
2. **备份数据库**: 详见 [备份指南](backups.md)
3. **备份配置文件**: `cp config.yaml config.yaml.bak`
4. **准备回滚方案**: 保留旧版本二进制或镜像

### 升级步骤(通用)

```
准备 → 备份 → 测试 → 升级 → 验证 → 监控
```

#### 步骤一: 在测试环境验证

```bash
# 1. 部署新版本到测试环境
docker compose -f docker-compose.test.yaml up -d

# 或 Kubernetes 测试命名空间
kubectl apply -f k8s/ -n copcon-test

# 2. 运行集成测试
cd server && go test -run "Integration" -v

# 3. 手动验证核心功能
# 创建会话、发送消息、验证 SSE 流
```

#### 步骤二: 配置迁移

根据版本的配置变更,修改 config.yaml:

```bash
# 比较新旧默认配置
diff config.yaml.example.new config.yaml

# 手动合并变更
# 不要直接替换,只合并新增字段
```

v1.x → v2.0 配置迁移示例:

```yaml
# v1.x 格式
default: "assistant"
agents:
  - name: "assistant"
    model: "gpt-4"
    ...

# v2.0 格式
default_agent_id: "assistant"
agents:
  - id: "assistant"
    name: "Assistant"
    model: "gpt-4"
    ...
```

#### 步骤三: 数据库迁移

```bash
# 数据库表结构在服务器启动时由 GORM AutoMigrate 自动更新
# 新表、新列、新索引会自动创建,不会删除已有数据
# 只需启动新版本服务器即可
```

#### 步骤四: 执行升级

**Docker Compose**:

```bash
# 拉取新镜像
docker compose pull server

# 停止旧版本(保留数据库运行)
docker compose stop server

# 启动新版本
docker compose up -d server

# 验证
docker compose logs server -f
curl http://localhost:8080/health
```

**Kubernetes**:

```bash
# 滚动更新
kubectl -n copcon set image deployment/copcon-server \
  server=ghcr.io/copcon/copcon-server:v2.0.0

# 观察滚动更新
kubectl -n copcon rollout status deployment/copcon-server

# 如果有问题,立即回滚
kubectl -n copcon rollout undo deployment/copcon-server
```

**二进制**:

```bash
# 停止旧服务
sudo systemctl stop copcon

# 替换二进制
sudo cp copcon-server /usr/local/bin/copcon-server

# 启动新服务
sudo systemctl start copcon

# 验证
curl http://localhost:8080/health
```

#### 步骤五: 验证升级

```bash
# 1. 健康检查
curl http://localhost:8080/health

# 2. 创建会话
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"title": "Post-upgrade test"}'

# 3. 发送消息(验证 LLM 连通)
curl -X POST http://localhost:8080/api/sessions/{id}/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "hello"}'

# 4. 查看 Agent 列表(验证新 Agent 定义生效)
curl http://localhost:8080/api/agents

# 5. 查看日志有无异常
journalctl -u copcon --since "5 minutes ago" | grep -i error
```

#### 步骤六: 持续监控

升级后 24 小时内关注:
- 错误率是否超过基线
- 内存使用是否稳定
- SSE 连接是否正常
- 数据库连接池是否健康

## 回滚流程

### 快速回滚

升级失败时立即回滚到旧版本:

**Docker Compose**:

```bash
# 指定旧版本镜像
docker compose up -d server \
  --image ghcr.io/copcon/copcon-server:v1.2.0

# 或修改 docker-compose.yaml 中的 image tag 后
docker compose up -d server
```

**Kubernetes**:

```bash
# 回滚到上一版本
kubectl -n copcon rollout undo deployment/copcon-server

# 回滚到指定版本
kubectl -n copcon rollout undo deployment/copcon-server --to-revision=2

# 查看回滚状态
kubectl -n copcon rollout status deployment/copcon-server
```

**二进制**:

```bash
sudo systemctl stop copcon
sudo cp copcon-server-v1.2.0.backup /usr/local/bin/copcon-server
sudo systemctl start copcon
```

### 数据库回滚

如果新版本修改了数据库 schema,回滚时可能需要恢复数据库:

```bash
# 从备份恢复(如果 schema 变更破坏了兼容性)
zcat /data/backups/postgres/copcon_20250115_030000.sql.gz | \
  PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -U $DB_USER -d copcon
```

**重要**: 数据库回滚意味着升级后的数据丢失。如果用户在升级后创建了新会话,回滚后这些会话不存在。

### 配置回滚

```bash
# 恢复备份配置
sudo cp config.yaml.bak /etc/copcon/config.yaml
sudo systemctl restart copcon
```

## 数据库 Schema 迁移

### 当前 Schema

CopCon v2.0 的数据库表结构:

```sql
-- sessions 表
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    metadata JSONB DEFAULT '{}'
);

-- messages 表
CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('user', 'assistant', 'tool', 'system')),
    content TEXT NOT NULL,
    tool_calls JSONB,
    tool_call_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);
```

### 迁移方式

CopCon 使用 `CREATE TABLE IF NOT EXISTS` 和 `CREATE INDEX IF NOT EXISTS`,这意味着:
- 新表: 自动创建
- 已有表: 保持不变,不删除数据
- 新列: 需要 ALTER TABLE 语句添加(如果 schema 变更需要新列)
- 删除列: 不自动处理,需要手动迁移

#### 添加新列的手动迁移

如果新版本需要新增列(如 `model` 字段到 messages 表):

```sql
ALTER TABLE messages ADD COLUMN IF NOT EXISTS model VARCHAR(100);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS token_count INTEGER;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS duration_ms INTEGER;
```

#### 迁移脚本模板

```bash
#!/bin/bash
# migrate-v2.sh - v1.x 到 v2.0 数据迁移

echo "Starting v1.x → v2.0 migration..."

# 1. 添加新列(如果不存在)
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -U $DB_USER -d copcon <<EOF
ALTER TABLE messages ADD COLUMN IF NOT EXISTS model VARCHAR(100);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS token_count INTEGER DEFAULT 0;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS duration_ms INTEGER DEFAULT 0;

-- sessions 表添加 default_agent_id 列
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS default_agent_id VARCHAR(100);
EOF

echo "Schema migration complete."

# 2. 数据迁移(如果需要)
# 例如: 从 v1.x 的 sessions.metadata 中提取 default_agent_id
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -U $DB_USER -d copcon <<EOF
UPDATE sessions SET default_agent_id = 'assistant' WHERE default_agent_id IS NULL;
EOF

echo "Data migration complete."
```

### AutoMigrate 机制

如果 config.yaml 中启用了 `auto_migrate: true`,Harness 启动时会使用 GORM AutoMigrate 自动处理:
- 创建新表
- 添加新列(仅添加,不删除或修改已有列)
- 创建新索引

AutoMigrate 不会删除任何数据或列。如果你需要删除列或做破坏性 schema 变更,必须手动操作。

## 配置变更

### 配置合并策略

升级时不要用新版本默认配置直接替换生产配置。应该合并:

```bash
# 查看新版本增加了哪些配置项
diff config.yaml.example.v2.0 config.yaml.production

# 合并步骤:
# 1. 保留生产配置的所有值
# 2. 添加新版本的新配置项(使用默认值)
# 3. 修改有破坏性变更的字段名
```

### 环境变量优先级

环境变量覆盖 config.yaml,所以即使 config.yaml 中有旧字段名,只要环境变量正确,服务也能正常运行。但这不是长期方案,应该更新 config.yaml。

## 升级测试流程

### 升级前测试

在测试环境完整模拟升级流程:

1. 从生产数据库恢复一份到测试环境
2. 应用新版本配置
3. 运行新版本 CopCon
4. 执行功能测试(创建会话、发送消息、SSE 流、Agent 刑名)
5. 执行性能测试(延迟、并发)
6. 验证数据完整性(会话数、消息数是否匹配)

### 测试清单

- [ ] 新版本健康检查通过
- [ ] 已有会话可以正常访问
- [ ] 新会话创建正常
- [ ] SSE 流式响应正常
- [ ] Agent 定义正确加载
- [ ] 工具调用正常(code_executor, shell_executor)
- [ ] 记忆功能正常(Qdrant 检索)
- [ ] 数据库迁移无数据丢失
- [ ] 回滚流程可以正常执行

## 常见升级问题

### "agent ID not found"

升级 v1.x → v2.0 后出现:

```
Error: default agent ID not found: assistant
```

**原因**: v2.0 使用 `id` 字段,而 config.yaml 仍然使用 v1.x 的 `name` 字段。

**解决**: 将所有 agent 定义中的 `name` 改为 `id`,并添加 `name` 作为显示名称。

### "database connection string format changed"

**原因**: v2.0 将 `database.url` DSN 字符串拆分为独立字段。

**解决**: 将 DSN 解析为各字段:

```yaml
# v1.x
database:
  url: "postgres://admin:changeme@localhost:5432/copcon?sslmode=disable"

# v2.0
database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "copcon"
```

### "capabilities import error"

v2.0 中内置 capabilities 通过 `init()` 自动注册。如果 server/ 中仍手动导入:

```go
// WRONG for v2.0
import "github.com/copcon/core/capabilities/tools"

// CORRECT for v2.0 - just use Harness
import "github.com/copcon/core"
h := core.NewHarness(cfg)  // capabilities auto-registered
```

## 下一步

- [安装指南](installation.md)
- [故障排查](troubleshooting.md)
- [备份与恢复](backups.md)