# Webhook 集成指南

## 概述

CopCon 支持 Webhook 通知，当平台发生指定事件时，自动向你的服务器发送 HTTP POST 请求。Webhook 可用于实现实时同步、自动化工作流和第三方系统集成。

## 配置 Webhook

### 创建 Webhook 端点

1. 登录控制台，进入「设置」→「Webhook」
2. 点击「添加端点」
3. 填写接收 URL（必须是 HTTPS）
4. 选择要订阅的事件类型
5. 设置签名密钥（用于验证请求来源）

### 端点要求

- 必须支持 HTTPS（TLS 1.2+）
- 必须在 10 秒内返回 2xx 状态码
- 建议实现幂等处理（相同事件可能发送多次）

## 支持的事件

| 事件类型 | 触发条件 | Payload 包含 |
|---------|---------|-------------|
| `conversation.created` | 新会话创建 | 会话 ID、用户 ID、时间戳 |
| `conversation.completed` | 会话结束 | 会话 ID、消息数、满意度评分 |
| `knowledge.updated` | 知识库文档更新 | 文档 ID、变更类型、操作人 |
| `knowledge.deleted` | 文档被删除 | 文档 ID、删除原因 |
| `user.created` | 新用户注册 | 用户 ID、邮箱、注册来源 |
| `alert.triggered` | 告警触发 | 告警类型、严重级别、详情 |
| `deployment.completed` | 部署完成 | 版本号、环境、状态 |

## 请求格式

```json
{
  "id": "evt_xxxxxxxxxxxx",
  "type": "conversation.completed",
  "timestamp": "2025-01-15T10:30:00Z",
  "data": {
    "session_id": "sess_abc123",
    "message_count": 8,
    "satisfaction_score": 4.5,
    "duration_seconds": 120
  }
}
```

## 签名验证

每个 Webhook 请求包含签名头，用于验证请求确实来自 CopCon：

```http
X-CopCon-Signature: sha256=abcdef1234567890...
X-CopCon-Timestamp: 1700000000
```

验证代码示例：

```go
func verifySignature(payload []byte, sig string, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    expected := hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(sig), []byte("sha256="+expected))
}
```

## 重试机制

如果端点返回非 2xx 状态码或超时，系统将按以下策略重试：

| 重试次数 | 间隔 |
|---------|------|
| 第 1 次 | 1 分钟 |
| 第 2 次 | 5 分钟 |
| 第 3 次 | 15 分钟 |
| 第 4 次 | 1 小时 |
| 第 5 次 | 6 小时 |

5 次重试全部失败后，事件进入死信队列，可通过控制台手动重新发送。

## 最佳实践

- **异步处理**：收到 Webhook 后立即返回 200，业务逻辑放入队列异步执行
- **去重处理**：使用事件 ID 进行幂等检查
- **日志记录**：记录所有收到的 Webhook，便于排查问题
- **监控告警**：设置端点响应时间和错误率监控
- **安全防护**：严格验证签名，忽略未签名的请求
