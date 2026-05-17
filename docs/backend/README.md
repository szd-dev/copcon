# CopCon 后端文档

CopCon 是一个用 Go 编写的 AI Agent 基础设施后端。它提供了一套完整的 Agent 引擎、工具系统、会话管理和流式事件推送能力，帮助开发者快速构建 AI 应用。

## 文档结构

```
docs/backend/
├── README.md                          # 本页，文档索引
├── 01-getting-started/
│   ├── quickstart.md                  # 5 分钟快速跑起来
│   ├── installation.md                # 完整安装指南
│   └── hello-world.md                 # 第一个 Agent 程序
├── 02-core-concepts/                  # 核心概念（待补充）
├── 03-agent/                          # Agent 引擎与注册（待补充）
├── 04-tool/                           # 工具系统（待补充）
├── 05-session/                        # 会话管理（待补充）
├── 06-hook/                           # Hook 插件系统（待补充）
├── 07-api/                            # REST API 参考（待补充）
└── 08-deployment/                     # 部署指南（待补充）
```

## 阅读路线

按你的目标选择从哪里开始：

| 目标 | 推荐阅读 |
|---|---|
| 只想跑起来 | [快速开始](01-getting-started/quickstart.md) |
| 加业务逻辑 | [Hello World](01-getting-started/hello-world.md) → Hook 系统 |
| 换模型 / 换 API | [安装指南](01-getting-started/installation.md) → config.yaml 部分 |
| 注册新工具 | [Hello World](01-getting-started/hello-world.md) → Tool 系统 |
| 部署到服务器 | [安装指南](01-getting-started/installation.md) → Docker Compose 部署 |
| 理解整体架构 | 本页 → [核心概念](02-core-concepts/) |

## 架构概览

```
HTTP Request → Gin Handler → Agent Engine → LLM Provider (流式)
                                ↓
                          Hook 链 (插件拦截)
                          Tool 执行 (Code/Shell/File)
                          Session / Context 管理
                                ↓
                          PostgreSQL + Qdrant
```

核心组件：

- **Agent Engine**：Agent 循环引擎，负责会话管理、LLM 调用、工具执行的主控逻辑
- **Tool System**：可扩展的工具注册和执行框架，内置 Code、Shell、File 操作工具
- **Hook System**：核心-外围架构中的插件拦截器，可在引擎生命周期的 10 个节点注入逻辑
- **Session Manager**：基于 PostgreSQL 的会话和消息持久化
- **Context Builder**：上下文窗口管理与组装
- **Memory System**：基于 Qdrant 的向量记忆

## 技术栈

| 组件 | 技术 | 版本 |
|---|---|---|
| 语言 | Go | 1.26+ |
| Web 框架 | Gin | 1.12 |
| ORM | GORM | 1.31 |
| 向量数据库 | Qdrant | 1.17 |
| 关系数据库 | PostgreSQL | 15 |
| LLM SDK | go-openai (openai-go) | v3 |

## 快速链接

- [快速开始](01-getting-started/quickstart.md) — 5 分钟跑起来
- [安装指南](01-getting-started/installation.md) — 环境配置详解
- [Hello World](01-getting-started/hello-world.md) — 写第一个 Agent 程序
- [API 参考](../api/openapi.yaml) — OpenAPI 3.0 规范