# CopCon 后端技术文档

## 简介

本文档详细介绍了 CopCon 后端的架构设计、核心概念、开发指南和部署方案。

## 文档结构

```
├── 01-quick-start/          快速入门
├── 02-core-concepts/        核心概念
├── 03-core-library/         核心库使用
├── 04-server-app/           服务端应用
├── 05-built-in-capabilities/ 内置能力详解
├── 06-extending/            扩展开发指南
├── 07-deployment/           部署指南
└── 08-reference/            API 参考
```

## 推荐阅读路线

### 🚀 快速体验 (5分钟)
1. [安装与环境配置](01-quick-start/installation.md)
2. [第一个 Agent 应用](01-quick-start/hello-world.md)
3. [运行完整 Demo](01-quick-start/run-demo.md)

### 📚 深入理解核心架构
1. [架构概览](02-core-concepts/architecture.md)
2. [Harness 配置](02-core-concepts/harness.md)
3. [能力系统](02-core-concepts/capabilities.md)
4. [SSD 流式传输](02-core-concepts/streaming.md)

### 🔧 作为独立库使用
1. [核心库独立使用](03-core-library/as-library.md)
2. [自定义 Provider](03-core-library/custom-provider.md)
3. [多 Agent 协作](03-core-library/multi-agent.md)

### 🌐 服务端应用开发
1. [API 概览](04-server-app/api-overview.md)
2. [配置详解](04-server-app/configuration.md)
3. [自定义 Handler](04-server-app/customization.md)

### 🧩 使用内置能力
1. [Tools 概览](05-built-in-capabilities/tools/overview.md)
2. [Hooks 概览](05-built-in-capabilities/hooks/overview.md)

### 🔌 扩展开发
1. [自定义 Tool](06-extending/custom-tool.md)
2. [自定义 Hook](06-extending/custom-hook.md)
3. [自定义 LLM Adapter](06-extending/custom-llm-adapter.md)

### 🚢 部署上线
1. [Docker Compose 部署](07-deployment/docker-compose.md)
2. [生产环境检查清单](07-deployment/production-checklist.md)

## 核心架构

```
┌─────────────────────────────────────┐
│         core (独立库)                │
│  ┌──────────┐  ┌──────────────────┐ │
│  │ Agent    │  │ Capabilities     │ │
│  │ Engine   │  │ ┌──────────────┐ │ │
│  │ Loop     │  │ │ Tools & Hooks│ │ │
│  └──────────┘  │ └──────────────┘ │ │
│  ┌──────────┐  └──────────────────┘ │
│  │ SSE      │  ┌──────────────────┐ │
│  │ Stream   │  │ Storage          │ │
│  └──────────┘  │ - SessionStore   │ │
│                │ - MessageStore   │ │
│                │ - TodoStore      │ │
│                └──────────────────┘ │
└─────────────────────────────────────┘
              ▲
              │ 依赖
              │
┌─────────────────────────────────────┐
│       server (薄应用)                │
│  ┌──────────┐  ┌──────────────────┐ │
│  │ REST API │  │ Config Loader    │ │
│  └──────────┘  └──────────────────┘ │
└─────────────────────────────────────┘
```

## 技术栈

| 类别 | 技术 | 版本 |
|------|------|------|
| 语言 | Go | 1.26+ |
| Web 框架 | Gin | 1.12.0 |
| ORM | GORM | 1.31.1 |
| 向量数据库 | Qdrant | 1.17.x |
| 关系数据库 | PostgreSQL | 15.x |
| LLM SDK | go-openai | v3.29+ |
| 前端 | React + TypeScript | 19.x + 5.x |

## 快速链接

- [OpenAPI 3.0 规范](../../api/openapi.yaml)
- [前端组件库文档](../../packages/ui/README.md)
- [项目根 README](../../README.md)

## 反馈与贡献

- 📝 文档勘误: [提交 Issue](https://github.com/copcon/copcon/issues)
- 🤝 贡献指南: [CONTRIBUTING.md](../../CONTRIBUTING.md)
- 💬 讨论交流: [GitHub Discussions](https://github.com/copcon/copcon/discussions)

---

**下一步**: [开始安装](01-quick-start/installation.md) ➡️
