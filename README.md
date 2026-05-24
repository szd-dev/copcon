# CopCon

**企业级 AI Agent 基础设施**

CopCon 读作"卡普空"——企业协作的 AI Agent 引擎。

专注于为企业服务的精简化 Agent 能力,可以快速将 Agent 集成到你的业务系统中。

## 核心能力

### 🤖 智能 Agent 引擎
- 完整的 LLM 对话循环
- 流式输出(SSE)
- 工具调用与执行
- 上下文管理

### 🧰 丰富的内置工具
- 代码执行(Code Executor)
- Shell 命令执行
- 文件操作
- Todo 任务管理
- 人机交互(HITL)

### 🔌 可扩展架构
- Hook 系统:在 Agent 生命周期的各阶段注入自定义逻辑
- Tool 系统:快速注册业务工具
- LLM Provider:支持 OpenAI 兼容接口

### 🧠 长期记忆
- 基于 Qdrant 的向量记忆
- 对话历史智能检索
- 自动记忆持久化

### 📦 开箱即用
- 完整的会话管理 API
- PostgreSQL 持久化
- Docker Compose 一键部署
- React 前端组件库

## 快速开始

```bash
# 克隆项目
git clone https://github.com/copcon/copcon.git
cd copcon

# 启动依赖服务
docker compose up -d postgres qdrant

# 配置环境变量
cp .env.example .env
# 编辑 .env 填入你的 API Key

# 启动后端服务
cd server
go run cmd/server/main.go
```

完整文档请查看 [docs/backend/](docs/backend/README.md)

## 项目结构

```
copcon/
├── core/              # Agent 引擎核心库(可独立使用)
├── server/            # 参考应用(薄封装层)
├── packages/ui/       # React 组件库
├── packages/demo/     # 演示应用
└── api/               # OpenAPI 规范
```

## 技术栈

- **后端**: Go 1.26 + Gin + GORM
- **数据库**: PostgreSQL 15 + Qdrant 1.17
- **前端**: React 19 + TypeScript + Vite
- **LLM**: OpenAI API 兼容

## 文档

- [后端开发文档](docs/backend/README.md) - 完整的技术指南
- [API 参考](api/openapi.yaml) - OpenAPI 3.0 规范
- [前端文档](packages/ui/README.md) - React 组件库使用指南

## 贡献

欢迎提交 Issue 和 Pull Request!

## 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。
