# SSO 单点登录配置

## 概述

CopCon 支持通过 SSO（单点登录）集成企业身份提供商，实现统一的用户认证和管理。本文档涵盖 SAML 2.0 和 OIDC 两种协议的配置方法。

## SAML 2.0 配置

### 前提条件

- 企业身份提供商（IdP）支持 SAML 2.0
- CopCon 企业版或专业版订阅
- 管理员权限

### 配置步骤

**第一步：在 IdP 中创建应用**

以 Okta 为例：

1. 在 Okta 管理后台创建新的 SAML 应用
2. 配置以下参数：

| 参数 | 值 |
|------|------|
| Audience URI | `https://auth.copcon.io/saml/metadata` |
| SSO URL | `https://auth.copcon.io/saml/acs` |
| Name ID 格式 | EmailAddress |
| 签名算法 | SHA-256 |

3. 添加属性映射：
   - `email` → 用户邮箱
   - `displayName` → 用户姓名
   - `department` → 部门

**第二步：在 CopCon 中配置 SAML**

1. 进入「设置」→「安全」→「SSO」
2. 选择「SAML 2.0」
3. 上传 IdP 元数据 XML 文件或输入元数据 URL
4. 配置属性映射
5. 测试连接
6. 启用 SSO

### 用户映射规则

SSO 登录时，CopCon 根据以下规则映射用户：

- **邮箱匹配**：优先匹配已有账户的邮箱
- **自动创建**：无匹配账户时自动创建新用户
- **角色映射**：根据 IdP 组属性分配 CopCon 角色

```json
{
  "role_mapping": {
    "copcon-admins": "admin",
    "copcon-editors": "editor",
    "copcon-viewers": "viewer"
  }
}
```

## OIDC 配置

### 配置步骤

1. 在身份提供商中创建 OIDC 客户端
2. 设置回调 URL：`https://auth.copcon.io/oidc/callback`
3. 在 CopCon 中配置：
   - Issuer URL
   - Client ID
   - Client Secret
   - Scopes（openid, profile, email）

### 支持的 OIDC 提供商

- Azure AD
- Google Workspace
- Auth0
- Keycloak
- 自建 OIDC 服务（需兼容标准协议）

## 多租户 SSO

企业版支持多租户 SSO 配置，每个组织可使用不同的身份提供商：

```yaml
organizations:
  - name: "组织A"
    sso:
      protocol: saml
      metadata_url: "https://idp.org-a.com/saml/metadata"
  - name: "组织B"
    sso:
      protocol: oidc
      issuer: "https://idp.org-b.com"
      client_id: "xxx"
```

## 故障排查

### 常见问题

**SAML 响应签名验证失败**
- 确认 IdP 签名证书未过期
- 检查签名算法是否为 SHA-256
- 验证 Audience URI 配置正确

**用户登录后角色不正确**
- 检查角色映射配置
- 确认 IdP 发送的组属性名称
- 查看审计日志中的属性值

**登录循环（无限重定向）**
- 确认回调 URL 配置正确
- 检查 Cookie 域设置
- 清除浏览器 Cookie 后重试

## 安全建议

- 强制所有管理员账户使用 SSO + MFA
- 定期审查 SSO 配置和角色映射
- 禁用密码登录（完全依赖 SSO）
- 监控异常登录行为（地域异常、时间异常）
