# 新员工入职手册

## 欢迎

欢迎加入 CopCon 团队！这份手册将帮助你快速了解公司文化、工作流程和必备工具。我们相信每一位新成员都是团队的重要补充，期待你在这里发挥自己的价值。

## 第一周任务清单

### Day 1：环境搭建

1. 领取工牌和设备（笔记本电脑、显示器）
2. 完成 IT 账号注册（邮箱、Slack、VPN）
3. 安装开发环境：Go 1.21+、Docker、VS Code / JetBrains
4. 克隆核心代码仓库，完成首次构建
5. 阅读项目架构概述文档

### Day 2-3：团队融入

1. 与直属经理进行一对一面谈，明确短期目标
2. 参加团队站会，了解当前项目进展
3. 认识跨部门关键协作人（产品、设计、QA）
4. 了解团队编码规范和 Git 工作流
5. 完成第一个 Pull Request（通常是文档修复或小功能）

### Day 4-5：业务理解

1. 阅读 API 认证指南和核心模块文档
2. 了解产品路线图和近期迭代目标
3. 参加产品演示会，理解用户场景
4. 完成首次代码 Review，学习团队 Review 标准
5. 撰写第一篇技术笔记（记录本周学习心得）

## 开发工具链

### 必备工具

| 工具 | 用途 | 安装指南 |
|------|------|---------|
| Go 1.21+ | 后端开发 | `brew install go` |
| Docker 24+ | 本地环境 | Docker Desktop |
| golangci-lint | 代码检查 | `brew install golangci-lint` |
| pre-commit | Git hooks | `brew install pre-commit` |
| PostgreSQL | 数据库 | Docker 或本地安装 |

### Git 工作流

我们采用 GitHub Flow：

1. 从 `main` 创建功能分支：`feat/feature-name`
2. 在分支上开发和提交
3. 推送到远程，创建 Pull Request
4. 至少 2 位同事 Review 后合并
5. 合并后自动部署到 staging 环境

提交信息格式：`type(scope): description`，例如 `feat(core): add agent factory spec`。

## 公司文化

### 核心价值观

- **务实创新**：解决真实问题，不做花哨但不实用的东西
- **开放协作**：代码开源，知识共享，鼓励跨团队交流
- **持续学习**：每周技术分享会，鼓励探索新技术
- **用户至上**：所有决策从用户价值出发

### 日常工作节奏

- 每日站会 9:30-9:45
- 周二和周四下午为深度工作时间（减少会议）
- 周五下午技术分享或团队活动
- 弹性工作时间，核心时段 10:00-16:00

## 资源链接

- 内部 Wiki：`wiki.copcon.internal`
- 设计系统：`design.copcon.internal`
- API 文档：`docs.copcon.io/api`
- 状态监控：`status.copcon.io`