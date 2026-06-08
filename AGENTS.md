# AGENTS.md

## 执行原则

- 使用中文回复
- 使用中文书写文档
- 对于用户的需求先给出技术方案，用户审核通过后才可以执行

## 开发环境

项目使用 PM2 统一管理前后端服务。

### 前置条件

```bash
# 安装 PM2
sudo npm install -g pm2

# 安装前端依赖
pnpm install
```

### 一键启动

```bash
make dev
```

### 服务管理

```bash
make dev              # 启动所有服务，自动打开实时日志
make restart          # 重启所有服务
make restart-server   # 仅重启后端
make restart-demo     # 仅重启前端
make logs             # 查看实时日志
make status           # 查看运行状态（pm2 list）
make stop             # 停止所有服务
make clean            # 彻底清理所有进程
```

### 服务端口

| 服务 | 端口 | 说明 |
|------|------|------|
| 后端 API | 8088 | Go Gin HTTP 服务 |
| 前端 Demo | 5173 | Vite Dev Server，自动代理 `/api` → `localhost:8088` |

### PM2 配置

配置文件 `ecosystem.config.cjs`（`Makefile` 为其提供快捷入口）：

| 应用名 | 命令 | 工作目录 |
|--------|------|----------|
| `copcon-server` | `go run server/cmd/server/main.go` | 项目根目录 |
| `copcon-demo` | `pnpm dev` | `packages/demo/` |

- 后端 `autorestart: true`，崩溃自动拉起（最多 5 次）
- 前端 `autorestart: false`，由 Vite HMR 自行处理热更新
- 日志文件：`~/.pm2/logs/copcon-server-*.log`、`copcon-demo-*.log`