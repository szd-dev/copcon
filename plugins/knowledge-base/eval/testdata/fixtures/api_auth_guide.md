# API 认证指南

## 简介

CopCon API 使用基于 Token 的认证机制来保护所有接口调用。本文档详细说明了如何获取、使用和管理 API 认证凭证，确保你的集成安全可靠。

## 认证方式

### Bearer Token 认证

所有 API 请求必须在 HTTP Header 中携带有效的 Bearer Token：

```http
GET /api/v1/sessions HTTP/1.1
Host: api.copcon.io
Authorization: Bearer sk_live_xxxxxxxxxxxxxxxxxxxx
Content-Type: application/json
```

Token 以 `sk_live_`（生产环境）或 `sk_test_`（测试环境）开头。

### API Key 认证

对于服务端到服务端的调用，你也可以使用 API Key：

```go
client := copcon.NewClient(copcon.Config{
    APIKey: "ck_live_xxxxxxxxxxxxxxxxxxxx",
    BaseURL: "https://api.copcon.io",
})
```

## 获取凭证

1. 登录 CopCon 控制台
2. 进入「设置」→「API 凭证」
3. 点击「创建新凭证」
4. 选择凭证类型（Token 或 API Key）
5. 设置权限范围和过期时间
6. 复制并妥善保存凭证（创建后仅显示一次）

## Token 刷新机制

Access Token 默认有效期为 3600 秒（1 小时）。当 Token 过期时，使用 Refresh Token 获取新的 Access Token：

```bash
curl -X POST https://api.copcon.io/auth/token \
  -H "Content-Type: application/json" \
  -d '{
    "grant_type": "refresh_token",
    "refresh_token": "rt_xxxxxxxxxxxxxxxxxxxx"
  }'
```

Refresh Token 有效期为 30 天。超过有效期需要重新进行授权流程。

## 安全最佳实践

- **永远不要在客户端代码中硬编码 Token**。使用环境变量或密钥管理服务。
- **定期轮换凭证**。建议每 90 天更换一次 API Key。
- **为不同环境使用不同凭证**。开发和生产环境严格隔离。
- **设置最小权限范围**。仅授予应用所需的 API 权限。
- **启用 IP 白名单**。限制凭证只能从指定 IP 地址使用。
- **监控异常调用**。配置告警规则，当调用频率或来源异常时立即通知。

## 错误码

| 错误码 | 说明 | 处理方式 |
|-------|------|---------|
| 40101 | Token 无效 | 检查 Token 格式和有效性 |
| 40102 | Token 已过期 | 使用 Refresh Token 刷新 |
| 40103 | 权限不足 | 检查凭证权限范围 |
| 40104 | IP 不在白名单 | 添加调用方 IP 到白名单 |
| 42901 | 请求频率超限 | 参考限流策略降低调用频率 |

## 多因素认证

对于高权限操作（如删除资源、修改安全设置），API 层面支持 MFA 验证。发起请求时需额外提供 TOTP 验证码：

```http
X-MFA-Code: 123456
```

未提供 MFA 验证码的高权限请求将被拒绝，返回 40105 错误码。
