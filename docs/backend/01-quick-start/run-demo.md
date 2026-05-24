# 运行完整 Demo

本章节介绍如何运行包含前后端的完整 Demo 应用。

## 前置条件

- 完成[安装与环境配置](installation.md)
- 后端服务已启动

## 启动前端 Demo

### 安装依赖

```bash
cd packages/demo
npm install
```

### 启动开发服务器

```bash
npm run dev
```

Demo 将在 `http://localhost:5173` 启动。

## 功能演示

### 1. 会话管理

- 创建新会话
- 查看历史会话
- 切换会话

### 2. 对话交互

- 发送文本消息
- 接收流式响应
- 查看完整对话历史

### 3. 工具调用

- 代码执行示例
- 文件操作示例
- Shell 命令示例

### 4. Todo 任务

- Agent 自动创建任务
- 查看任务进度
- 任务完成状态

## 使用 React 组件库

CopCon 提供了开箱即用的 React 组件:

```bash
cd packages/ui
npm install
npm run storybook
```

Storybook 将在 `http://localhost:6006` 启动,你可以:

- 浏览所有可用组件
- 查看组件文档
- 交互示例

### 核心组件

#### `<Chat>` - 聊天视图

```jsx
import { Chat } from '@copcon/ui';

function MyChat() {
  return (
    <Chat 
      sessionId="your-session-id"
      onMessage={(msg) => console.log(msg)}
    />
  );
}
```

#### `<SessionList>` - 会话列表

```jsx
import { SessionList } from '@copcon/ui';

function MySessionList() {
  return (
    <SessionList 
      onSelect={(session) => console.log(session)}
    />
  );
}
```

#### `<Message>` - 消息组件

```jsx
import { Message } from '@copcon/ui';

function MyMessage() {
  return (
    <Message
      role="assistant"
      content="你好！"
      timestamp={new Date()}
    />
  );
}
```

#### `<ToolCall>` - 工具调用展示

```jsx
import { ToolCall } from '@copcon/ui';

function MyToolCall() {
  return (
    <ToolCall
      toolName="code_executor"
      input={{ code: "print('hello')" }}
      output="hello"
      status="completed"
    />
  );
}
```

## 集成到你的应用

详见 [packages/ui/README.md](../../../packages/ui/README.md)

## 故障排查

### 前端无法连接后端

1. 确认后端服务在 `http://localhost:8080` 运行
2. 检查 CORS 配置
3. 查看浏览器开发者工具 Network 标签

### 页面空白

1. 运行 `npm run build` 查看构建错误
2. 检查浏览器控制台错误
3. 确认环境变量已配置

## 下一步

- [架构概览](../02-core-concepts/architecture.md)
- [API 参考](../08-reference/http-api.md)
- [部署指南](../07-deployment/docker-compose.md)
