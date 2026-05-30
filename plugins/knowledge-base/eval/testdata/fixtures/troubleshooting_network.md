# 网络故障排查指南

## 概述

本文档提供 CopCon 平台常见网络问题的诊断和解决方法。如果你在使用平台时遇到连接问题，请按照以下步骤逐步排查。

## 常见问题与解决方案

### 问题 1：API 请求超时

**症状**：调用 API 时返回超时错误，HTTP 状态码 504 或连接超时。

**排查步骤**：

1. **检查网络连通性**

```bash
# 测试到 API 服务器的网络连通
ping api.copcon.io
traceroute api.copcon.io

# 测试 HTTPS 连接
curl -v https://api.copcon.io/health
```

2. **检查 DNS 解析**

```bash
nslookup api.copcon.io
dig api.copcon.io
```

如果 DNS 解析失败或返回错误 IP，尝试切换到公共 DNS（8.8.8.8 或 114.114.114.114）。

3. **检查防火墙规则**

确保以下出站端口已开放：
- TCP 443（HTTPS）
- TCP 80（HTTP，自动跳转到 HTTPS）

4. **检查代理设置**

如果你在公司网络内，可能需要配置代理：

```bash
export HTTPS_PROXY=http://proxy.company.com:8080
export NO_PROXY=localhost,127.0.0.1
```

5. **检查限流配置**

确认请求频率未超过 API 限制。查看响应头中的 `X-RateLimit-Remaining` 字段。

### 问题 2：WebSocket 连接断开

**症状**：实时对话功能中断，消息延迟或丢失。

**排查步骤**：

1. 确认网络环境支持 WebSocket 协议
2. 检查是否有中间代理修改了 `Connection` 或 `Upgrade` 头
3. 验证心跳机制正常（每 30 秒发送一次 ping）
4. 检查客户端是否正确处理重连逻辑

```javascript
// 推荐的重连策略
const ws = new WebSocket('wss://api.copcon.io/ws');
ws.onclose = (event) => {
  const delay = Math.min(1000 * Math.pow(2, retryCount), 30000);
  setTimeout(() => reconnect(), delay);
};
```

### 问题 3：跨域请求被拒绝

**症状**：浏览器控制台显示 CORS 错误。

**解决方案**：

1. 在控制台配置允许的域名
2. 服务端已预配置常见域名，自定义域名需在「安全设置」中添加
3. 本地开发使用代理而非直接跨域调用

### 问题 4：国内网络访问慢

**症状**：从中国大陆访问 API 响应慢，延迟超过 2 秒。

**排查**：

1. 确认使用了国内端点（api.copcon.cn）
2. 检查是否误连了海外节点
3. 如使用 CDN，确认 CDN 配置正确
4. 联系技术支持开启加速通道

## 日志收集

排查网络问题时，请收集以下信息：

- 完整的请求 URL 和 HTTP 方法
- 请求和响应 Header
- 错误信息和堆栈跟踪
- 网络环境描述（公司网络/家庭网络/移动网络）
- `curl` 测试结果

提供以上信息可显著加快问题定位速度。对于 P0 级问题，请直接拨打技术支持热线。
