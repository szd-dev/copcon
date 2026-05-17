# 示例：敏感词过滤器

## 场景

用户输入中可能包含敏感信息（密码、手机号、身份证号等），在消息发送给 LLM 之前需要清理这些内容。

## 为什么选择 AfterContextBuild

有两个候选 HookPoint：

- `OnMessagePersist`：在消息持久化时执行。问题是它作用于存储层，LLM 实际收到的是原始消息。
- `AfterContextBuild`：在上下文构建完成、即将发送给 LLM 之前执行。此时消息已经组装好，Hook 可以直接修改 `Messages` 数组。

选择 `AfterContextBuild`，因为我们要确保 LLM 收到的是处理后的消息。

## 优先级考虑

敏感词过滤属于业务逻辑层，不需要最早执行。设为 20，让其他系统层 Hook（日志、上下文增强）先执行。

## 完整代码

```go
package filter

import (
    "log/slog"
    "regexp"
    "strings"

    "github.com/copcon/server/internal/hook"
)

// SensitiveContentFilter 在消息发送给 LLM 之前，检查并替换敏感词。
type SensitiveContentFilter struct {
    // patterns 是需要过滤的正则模式列表
    patterns []*regexp.Regexp
    // replacement 是替换敏感内容的字符串
    replacement string
    // logger 用于记录过滤事件
    logger *slog.Logger
}

// NewSensitiveContentFilter 创建一个新的敏感词过滤器。
// words 是敏感词列表（普通字符串匹配，大小写不敏感）。
// patterns 是正则表达式模式列表。
func NewSensitiveContentFilter(
    words []string,
    patterns []string,
    replacement string,
) *SensitiveContentFilter {
    // 将普通敏感词转为正则模式：匹配整个单词，忽略大小写
    allPatterns := make([]*regexp.Regexp, 0, len(words)+len(patterns))
    for _, word := range words {
        if word == "" {
            continue
        }
        // 构造匹配整个单词的模式
        re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
        allPatterns = append(allPatterns, re)
    }
    for _, pattern := range patterns {
        if pattern == "" {
            continue
        }
        re, err := regexp.Compile(pattern)
        if err != nil {
            // 编译失败的正则跳过，记录日志
            slog.Warn("invalid regex pattern", "pattern", pattern, "error", err)
            continue
        }
        allPatterns = append(allPatterns, re)
    }

    if replacement == "" {
        replacement = "[已过滤]"
    }

    return &SensitiveContentFilter{
        patterns:    allPatterns,
        replacement: replacement,
        logger:       slog.Default(),
    }
}

// Name 返回 Hook 标识符。
func (f *SensitiveContentFilter) Name() string {
    return "sensitive_content_filter"
}

// Points 返回 AfterContextBuild，在消息发送给 LLM 之前执行。
func (f *SensitiveContentFilter) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.AfterContextBuild}
}

// Priority 返回 20，属于业务逻辑层。
func (f *SensitiveContentFilter) Priority() int {
    return 20
}

// Execute 遍历 messages 中所有用户消息，将匹配到的敏感内容替换为 replacement。
func (f *SensitiveContentFilter) Execute(ctx *hook.HookContext) error {
    if ctx.Messages == nil || len(*ctx.Messages) == 0 {
        return nil
    }

    if len(f.patterns) == 0 {
        return nil
    }

    replacedCount := 0

    for i, msg := range *ctx.Messages {
        // 只过滤用户消息
        if msg.Role != "user" && msg.Role != "" {
            continue
        }

        original := msg.Content
        filtered := original

        for _, re := range f.patterns {
            filtered = re.ReplaceAllString(filtered, f.replacement)
        }

        if filtered != original {
            (*ctx.Messages)[i].Content = filtered
            replacedCount++
        }
    }

    if replacedCount > 0 {
        f.logger.Info("sensitive content replaced",
            "session_id", ctx.SessionID,
            "messages_affected", replacedCount,
            "patterns", len(f.patterns),
        )
    }

    return nil
}
```

## 注册代码

```go
package main

import (
    "github.com/copcon/server/internal/hook"
    "your-project/filter"
)

func main() {
    runner := hook.NewHookRunner()

    // 创建过滤器
    filter := filter.NewSensitiveContentFilter(
        // 普通敏感词（按单词边界匹配，大小写不敏感）
        []string{
            "password", "secret", "token", "api_key",
            "身份证", "银行卡号",
        },
        // 正则模式（更复杂的匹配规则）
        []string{
            `\d{15}(\d{2}[0-9Xx])?`,         // 身份证号
            `\d{16,19}`,                       // 银行卡号
            `1[3-9]\d{9}`,                     // 手机号
        },
        "[已过滤]",
    )

    runner.Register(filter)

    // 将 runner 传入引擎
    // engine := agent.NewEngine(agent.WithHookRunner(runner), ...)
}
```

## 执行流程说明

1. 用户发送消息："我的密码是 abc123"
2. 引擎构建上下文，调用 `BeforeContextBuild` Hook
3. 引擎调用 `AfterContextBuild` Hook
4. `SensitiveContentFilter` 的 `Execute` 被调用
5. 遍历 `Messages`，找到用户消息，用正则匹配 `password`
6. 替换为："我的[已过滤]是 abc123"
7. 后续 Hook 看到的是修改后的消息
8. LLM 收到修改后的消息

## 设计说明

**为什么只过滤 `user` 角色的消息？** 用户消息是敏感信息的来源。助手消息和系统消息通常不会有这类内容。但你可以根据需要扩展这个规则。

**正则模式下为什么编译失败时只记录日志不 panic？** Hook 中的任何错误都不应该中断引擎。编译正则失败说明配置有问题，这是运维层面需要修复的，不应该导致线上请求失败。

**为什么用 `regexp.QuoteMeta`？** 普通敏感词直接放进正则会出问题（比如包含 `.`、`*` 等特殊字符）。`QuoteMeta` 转义这些特殊字符，确保它们按字面意思匹配。

## 扩展建议

- 添加白名单功能：某些场景下临时关闭过滤
- 根据 `AgentID` 区分不同的过滤规则
- 使用 `ctx.ChatCtx.Emit()` 向客户端发送警告事件
- 将命中日志持久化到审计系统