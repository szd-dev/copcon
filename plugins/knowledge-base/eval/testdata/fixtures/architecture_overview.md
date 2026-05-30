# 架构概述

## 系统架构

CopCon 采用模块化的微服务架构，核心设计原则是关注点分离和松耦合。系统由以下核心模块组成：

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Gateway   │───→│  API Server │───→│   Engine    │
│  (Nginx)    │    │  (Go)       │    │  (Go)       │
└─────────────┘    └──────┬──────┘    └──────┬──────┘
                          │                   │
                   ┌──────┴──────┐    ┌──────┴──────┐
                   │   Worker    │    │  ChatContext │
                   │  (Async)    │    │  (State)     │
                   └──────┬──────┘    └─────────────┘
                          │
          ┌───────────────┼───────────────┐
          │               │               │
    ┌─────┴─────┐  ┌─────┴─────┐  ┌─────┴─────┐
    │PostgreSQL │  │   Redis   │  │Vector DB  │
    │  (Data)   │  │ (Cache)   │  │ (Embed)   │
    └───────────┘  └───────────┘  └───────────┘
```

## 核心模块

### core/ — 核心库

独立可复用的 Go 库，包含平台的所有核心逻辑：

- **harness**：系统入口，组装所有组件，对外提供 APIProvider 接口
- **engine**：对话引擎，管理 Agent 生命周期和消息流
- **chatcontext**：对话上下文，贯穿请求全生命周期，提供事件发射和状态管理
- **storage**：存储抽象层，定义 SessionStore、MessageStore 等接口
- **capabilities**：工具和钩子系统，通过 init() 自动注册
- **providers**：存储实现（PostgreSQL、Qdrant、SQLite-vec）

关键约束：core/ 不依赖 server/，保持独立可测试。

### server/ — 应用层

薄应用层，负责 HTTP 传输和配置：

- 路由和中间件
- 请求解析和响应序列化
- SSE 事件流推送
- 配置管理和环境变量

server/ 导入 core/，但 core/ 绝不反向导入 server/。

## 数据流

### 对话请求流程

1. 客户端发送消息到 `/api/v1/chat`
2. Gateway 转发到 API Server
3. Handler 创建 ChatContext，调用 `engine.Chat(chatCtx, input)`
4. Engine 在 goroutine 中执行，通过 `chatCtx.Emit()` 发送事件
5. Handler 通过 `core/chat.HandleChat()` 将事件流转换为 SSE
6. 客户端实时接收流式响应

### 文档导入流程

1. 上传文档到 `/api/v1/knowledge/upload`
2. Worker 接收任务，提取文本内容
3. 文本按 800 token 分块
4. 每个块调用 text-embedding-3-small 生成 1536 维向量
5. 块和向量存入向量数据库（SQLite-vec / Qdrant）
6. 导入完成，文档可用于检索

## 存储设计

### PostgreSQL

存储结构化数据：用户、会话、消息、配置、审计日志。所有查询使用 `db.WithContext(ctx)` 确保超时控制。

### Redis

- 会话级缓存（最近消息上下文）
- 限流计数器
- 任务队列（文档导入、批量操作）

### 向量数据库

存储文档块和嵌入向量，支持：
- 余弦相似度检索
- 元数据过滤（标签、时间范围、来源）
- 混合检索（BM25 + Dense）

## 扩展点

- **存储**：实现 storage 接口即可替换存储后端
- **能力**：实现 Capability 接口注册新工具或钩子
- **Provider**：实现 Provider 接口添加新的 AI 模型供应商
