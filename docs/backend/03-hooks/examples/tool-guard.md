# 示例：工具调用守卫

## 场景

Agent 可以调用多种工具：`shell`（执行命令）、`file_write`（写文件）、`http_request`（发请求）等。你希望在工具执行之前检查参数，阻止危险操作，比如：

- Shell 命令中包含 `rm -rf`
- 文件写入路径指向系统目录
- HTTP 请求的目标是内网地址

## 为什么选择 BeforeToolExecute

`BeforeToolExecute` 在工具调用前触发，此时 `ToolName` 和 `ToolArgs` 都已确定，你可以读取甚至修改参数。如果检测到危险操作，有两个选择：

1. **改写参数**：将危险参数替换为安全值（本示例的做法）
2. **拒绝执行**：在 Hook 中无法直接"阻止"执行，但你可以修改参数使工具执行无害操作

## 优先级考虑

安全检查应该在所有 Hook 中优先级最高。设为 150，确保在任何业务逻辑之前执行。

## 完整代码

```go
package guard

import (
    "fmt"
    "log/slog"
    "net"
    "strings"

    "github.com/copcon/server/internal/hook"
)

// 危险的 shell 命令模式列表。
// 这些模式会在 command 参数中进行子字符串匹配。
var dangerousShellPatterns = []string{
    "rm -rf",
    "rm -r",
    "mkfs",
    "dd if=",
    "> /dev/sda",
    "chmod 777",
    "chown root",
    ":(){ :|:& };:", // fork bomb
    "shutdown",
    "reboot",
    "halt",
    "poweroff",
    "wget",  // 通常需要限制下载来源
    "curl",  // 通常需要限制请求目标
}

// 危险的文件写入路径前缀列表。
var dangerousFilePaths = []string{
    "/etc/",
    "/boot/",
    "/proc/",
    "/sys/",
    "/dev/",
    "/bin/",
    "/sbin/",
    "/usr/bin/",
    "/usr/sbin/",
    "/lib/",
    "/lib64/",
}

// 内网 CIDR 列表。
var privateCIDRs = []string{
    "10.0.0.0/8",
    "172.16.0.0/12",
    "192.168.0.0/16",
    "127.0.0.0/8",
    "169.254.0.0/16",
}

// ToolGuardHook 在工具执行前检查参数安全性。
type ToolGuardHook struct {
    // blockedCommands 是被完全禁止的命令列表
    blockedCommands []string
    // blockedPaths 是被禁止写入的文件路径前缀
    blockedPaths []string
    // blockedNetworks 是被禁止访问的网络 CIDR
    blockedNetworks []*net.IPNet

    logger *slog.Logger
}

// NewToolGuardHook 创建工具守卫 Hook。
// blockedCommands、blockedPaths、blockedNetworks 可以传入自定义的禁止列表。
// 如果传空值，使用默认列表。
func NewToolGuardHook(
    blockedCommands []string,
    blockedPaths []string,
    blockedNetworks []string,
) *ToolGuardHook {
    if len(blockedCommands) == 0 {
        blockedCommands = dangerousShellPatterns
    }
    if len(blockedPaths) == 0 {
        blockedPaths = dangerousFilePaths
    }
    if len(blockedNetworks) == 0 {
        blockedNetworks = privateCIDRs
    }

    guard := &ToolGuardHook{
        blockedCommands: blockedCommands,
        blockedPaths:    blockedPaths,
        logger:          slog.Default(),
    }

    // 解析 CIDR 列表
    for _, cidr := range blockedNetworks {
        _, network, err := net.ParseCIDR(cidr)
        if err != nil {
            slog.Warn("invalid CIDR in blocked networks",
                "cidr", cidr, "error", err,
            )
            continue
        }
        guard.blockedNetworks = append(guard.blockedNetworks, network)
    }

    return guard
}

// Name 返回 Hook 标识符。
func (g *ToolGuardHook) Name() string {
    return "tool_guard"
}

// Points 返回 BeforeToolExecute，在工具执行前拦截。
func (g *ToolGuardHook) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.BeforeToolExecute}
}

// Priority 返回 150，高于默认优先级。
func (g *ToolGuardHook) Priority() int {
    return 150
}

// Execute 根据工具类型分发到对应的检查逻辑。
func (g *ToolGuardHook) Execute(ctx *hook.HookContext) error {
    switch ctx.ToolName {
    case "shell", "execute_command", "run_command":
        return g.guardShellCommand(ctx)
    case "file_write", "write_file", "create_file":
        return g.guardFileWrite(ctx)
    case "http_request", "fetch_url", "web_request":
        return g.guardHTTPRequest(ctx)
    default:
        // 不认识的工具，放行
        return nil
    }
}

// guardShellCommand 检查 shell 命令参数是否包含危险模式。
func (g *ToolGuardHook) guardShellCommand(ctx *hook.HookContext) error {
    cmd, ok := g.extractCommand(ctx.ToolArgs)
    if !ok {
        return nil
    }

    cmdLower := strings.ToLower(cmd)

    for _, pattern := range g.blockedCommands {
        if strings.Contains(cmdLower, pattern) {
            g.logger.Warn("dangerous shell command blocked",
                "session_id", ctx.SessionID,
                "tool", ctx.ToolName,
                "command", cmd,
                "matched_pattern", pattern,
            )

            // 改写参数：让工具执行无害的 echo 命令，输出拒绝信息
            ctx.ToolArgs["command"] = fmt.Sprintf(
                "echo '[ToolGuard] 危险命令已被拦截: %s'", pattern,
            )

            return nil
        }
    }

    return nil
}

// guardFileWrite 检查文件写入路径是否在禁止列表中。
func (g *ToolGuardHook) guardFileWrite(ctx *hook.HookContext) error {
    // 尝试多个可能的参数名
    path := g.extractFilePath(ctx.ToolArgs)
    if path == "" {
        return nil
    }

    for _, blocked := range g.blockedPaths {
        if strings.HasPrefix(path, blocked) {
            g.logger.Warn("file write to protected path blocked",
                "session_id", ctx.SessionID,
                "tool", ctx.ToolName,
                "file_path", path,
                "blocked_prefix", blocked,
            )

            // 改写路径为 /tmp/
            safePath := "/tmp/blocked_" + strings.ReplaceAll(path, "/", "_")
            ctx.ToolArgs["file_path"] = safePath

            return nil
        }
    }

    return nil
}

// guardHTTPRequest 检查 HTTP 请求目标是否为内网地址。
func (g *ToolGuardHook) guardHTTPRequest(ctx *hook.HookContext) error {
    url, ok := g.extractURL(ctx.ToolArgs)
    if !ok {
        return nil
    }

    // 尝试解析主机名
    // 这是一个简化实现。实际项目中应该提取 URL 中的 host:port，然后解析域名。
    host := g.extractHost(url)
    if host == "" {
        return nil
    }

    ip := net.ParseIP(host)
    if ip == nil {
        // host 不是 IP，是域名，放行。
        // 实际项目中应该做 DNS 解析后检查。
        return nil
    }

    for _, network := range g.blockedNetworks {
        if network.Contains(ip) {
            g.logger.Warn("http request to private network blocked",
                "session_id", ctx.SessionID,
                "tool", ctx.ToolName,
                "url", url,
                "ip", ip.String(),
                "blocked_cidr", network.String(),
            )

            // 将 URL 替换为安全的 localhost
            ctx.ToolArgs["url"] = "http://localhost/"
            return nil
        }
    }

    return nil
}

// extractCommand 从 ToolArgs 中提取 shell 命令参数。
func (g *ToolGuardHook) extractCommand(args map[string]any) (string, bool) {
    // 不同工具实现可能用不同的 key 名
    for _, key := range []string{"command", "cmd", "script", "input"} {
        if val, ok := args[key]; ok {
            if cmd, ok := val.(string); ok {
                return cmd, true
            }
        }
    }
    return "", false
}

// extractFilePath 从 ToolArgs 中提取文件路径参数。
func (g *ToolGuardHook) extractFilePath(args map[string]any) string {
    for _, key := range []string{"file_path", "path", "filepath", "filename", "destination"} {
        if val, ok := args[key]; ok {
            if path, ok := val.(string); ok {
                return path
            }
        }
    }
    return ""
}

// extractURL 从 ToolArgs 中提取 URL 参数。
func (g *ToolGuardHook) extractURL(args map[string]any) (string, bool) {
    for _, key := range []string{"url", "uri", "endpoint", "address"} {
        if val, ok := args[key]; ok {
            if url, ok := val.(string); ok {
                return url, true
            }
        }
    }
    return "", false
}

// extractHost 从 URL 中提取主机部分。
func (g *ToolGuardHook) extractHost(rawURL string) string {
    // 移除协议前缀
    s := rawURL
    for _, prefix := range []string{"https://", "http://"} {
        s = strings.TrimPrefix(s, prefix)
    }

    // 去掉路径和端口
    if idx := strings.Index(s, "/"); idx >= 0 {
        s = s[:idx]
    }
    if idx := strings.Index(s, ":"); idx >= 0 {
        s = s[:idx]
    }

    return s
}
```

## 注册代码

```go
package main

import (
    "github.com/copcon/server/internal/hook"
    "your-project/guard"
)

func main() {
    runner := hook.NewHookRunner()

    // 创建工具守卫（使用默认禁止列表）
    guard := guard.NewToolGuardHook(nil, nil, nil)
    runner.Register(guard)

    // 自定义禁止列表
    // guard := guard.NewToolGuardHook(
    //     []string{"rm -rf", "mkfs", "docker run --privileged"},
    //     []string{"/etc/", "/var/run/"},
    //     []string{"10.0.0.0/8", "192.168.1.0/24"},
    // )

    // 将 runner 传入引擎
    // engine := agent.NewEngine(agent.WithHookRunner(runner), ...)
}
```

## 执行流程说明

### Shell 命令拦截

1. Agent 决定执行命令：`rm -rf /tmp/test/`
2. 触发 `BeforeToolExecute`
3. `guardShellCommand` 提取 `command` 参数
4. 匹配到模式 `rm -rf`
5. 记录 Warn 日志
6. 改写 `command` 参数为：`echo '[ToolGuard] 危险命令已被拦截: rm -rf'`
7. 工具执行改写后的命令，输出拦截提示

### 文件写入拦截

1. Agent 决定写文件到 `/etc/config.ini`
2. 触发检查，匹配前缀 `/etc/`
3. 改写路径为 `/tmp/blocked__etc_config.ini`

### HTTP 请求拦截

1. Agent 决定请求 `http://192.168.1.100/admin`
2. 解析 IP `192.168.1.100`
3. 匹配到 CIDR `192.168.0.0/16`
4. 改写 URL 为 `http://localhost/`

## 设计说明

**为什么不直接阻止执行，而是改写参数？** Hook 系统不支持阻断语义。`Execute` 返回 error 只会记录日志，不会中断工具执行。因此采用"改写参数"的策略，将危险操作转为无害操作。

**为什么只做了简单的子字符串匹配？** 这是示例代码。生产环境中应该使用更精确的解析（如 shell 命令 AST 解析、URL 解析等），避免误判。例如 `echo "rm -rf"` 不应该被拦截。

**为什么参数 key 名有多种可能性？** 不同团队实现的工具可能用不同的参数名。`extractCommand` 方法尝试多个常见的 key 名，提高兼容性。在你的项目中，应该统一工具参数命名规范。

## 扩展建议

- 添加白名单机制：特定 Session 或 Agent 可以绕过检查
- 集成审批流程：危险操作需要管理员确认后才能执行
- 使用 `ctx.ChatCtx.Emit()` 发送拦截事件到前端
- 将拦截日志写入审计数据库
- 支持正则模式匹配