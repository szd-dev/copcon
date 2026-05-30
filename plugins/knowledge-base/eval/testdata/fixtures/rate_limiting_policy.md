# 限流策略

## 概述

为保障平台稳定性和公平使用，CopCon 对所有 API 请求实施限流。限流基于滑动窗口算法，按不同的维度和层级设置阈值。

## 限流规则

### API 调用频率限制

| 端点类别 | 基础版 | 专业版 | 企业版 |
|---------|--------|--------|--------|
| 对话 API | 60 次/分钟 | 120 次/分钟 | 500 次/分钟 |
| 知识库 API | 30 次/分钟 | 60 次/分钟 | 200 次/分钟 |
| 管理 API | 20 次/分钟 | 40 次/分钟 | 100 次/分钟 |
| 文件上传 | 10 次/分钟 | 20 次/分钟 | 50 次/分钟 |

### 并发会话限制

| 方案 | 最大并发会话 |
|------|------------|
| 基础版 | 5 |
| 专业版 | 20 |
| 企业版 | 无限制 |

### 文档存储限制

| 方案 | 最大文档数 | 单文档大小 | 总存储空间 |
|------|-----------|-----------|-----------|
| 基础版 | 1,000 | 10 MB | 5 GB |
| 专业版 | 10,000 | 50 MB | 50 GB |
| 企业版 | 无限制 | 100 MB | 按需 |

## 响应头说明

每个 API 响应都包含限流相关的 Header：

```http
HTTP/1.1 200 OK
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 45
X-RateLimit-Reset: 1700000000
```

| Header | 说明 |
|--------|------|
| `X-RateLimit-Limit` | 当前窗口的请求限额 |
| `X-RateLimit-Remaining` | 当前窗口剩余请求次数 |
| `X-RateLimit-Reset` | 限流窗口重置时间（Unix 时间戳） |

## 超限处理

当请求超过限流阈值时，API 返回 429 状态码：

```json
{
  "error": {
    "code": "42901",
    "message": "Rate limit exceeded",
    "details": {
      "limit": 60,
      "remaining": 0,
      "reset_at": "2025-01-15T10:30:00Z",
      "retry_after": 45
    }
  }
}
```

### 客户端最佳实践

```go
// 指数退避重试
func callWithRetry(fn func() error, maxRetries int) error {
    backoff := time.Second
    for i := 0; i < maxRetries; i++ {
        err := fn()
        if err == nil {
            return nil
        }
        if isRateLimitError(err) {
            time.Sleep(backoff)
            backoff = backoff * 2
            if backoff > 30*time.Second {
                backoff = 30 * time.Second
            }
            continue
        }
        return err
    }
    return fmt.Errorf("max retries exceeded")
}
```

## 限流豁免

企业版客户可申请特定端点的限流豁免，适用场景包括：
- 大规模批量操作（数据迁移期间）
- 高频实时应用（如客服系统高峰期）
- CI/CD 自动化流水线

申请方式：联系客户经理，说明业务场景和预期调用量。豁免期限最长 30 天，到期自动恢复默认限流。
