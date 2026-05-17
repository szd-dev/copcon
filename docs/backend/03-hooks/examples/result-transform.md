# 示例：工具结果转换

## 场景

工具执行后会返回结果给 LLM。你希望在结果返回前做处理：

1. **截断长结果**：shell 命令可能输出几十万行，全发给 LLM 不仅浪费 token，还会让 LLM 注意力分散。只保留头尾。
2. **脱敏结果**：工具输出中可能包含密钥、证书等敏感信息。
3. **格式化结果**：原始 JSON 输出不便于 LLM 理解，需要转换格式。

## 为什么选择 AfterToolExecute

`AfterToolExecute` 在工具成功执行后、结果返回给引擎前触发。此时 `ToolResult` 已经有值，你可以读取和修改它的 `Data`、`Success`、`Error` 字段。

## 优先级考虑

结果转换属于业务逻辑层，设为 20。在基础设施层的日志和监控 Hook 之后执行。

## 完整代码

```go
package transform

import (
    "encoding/json"
    "fmt"
    "log/slog"
    "regexp"
    "strings"
    "unicode/utf8"

    "github.com/copcon/server/internal/hook"
    "github.com/copcon/server/internal/tool"
)

// ResultTransformConfig 定义每种工具的转换配置。
type ResultTransformConfig struct {
    // ToolName 是工具名称（如 "shell"、"file_read"）。
    // 空字符串表示默认配置，匹配所有未被单独配置的工具。
    ToolName string

    // MaxChars 是结果的最大字符数。超过时会被截断。
    // 0 表示不限制。
    MaxChars int

    // TruncateStrategy 是截断策略："head"、"tail"、"head_tail"。
    TruncateStrategy string

    // SecretPatterns 是需要脱敏的正则模式列表。
    SecretPatterns []string

    // StripJSON 表示是否尝试格式化 JSON 为更可读的形式。
    StripJSON bool
}

// DefaultConfig 返回全局默认配置。
func DefaultConfig() ResultTransformConfig {
    return ResultTransformConfig{
        MaxChars:         8000,
        TruncateStrategy: "head_tail",
        SecretPatterns: []string{
            `-----BEGIN [A-Z ]+ PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+ PRIVATE KEY-----`,
            `Bearer\s+[a-zA-Z0-9\-._~+/]+=*`,
            `password["\s:=]+[^\s,}]+`,
        },
        StripJSON: true,
    }
}

// ResultTransformHook 在工具执行后转换结果。
type ResultTransformHook struct {
    // configs 是按工具名索引的转换配置
    configs map[string]ResultTransformConfig
    // defaultConfig 是未单独配置的工具使用的默认配置
    defaultConfig ResultTransformConfig

    // compiledPatterns 是预编译的正则（按工具名 + 全局索引）
    compiledPatterns map[string][]*regexp.Regexp

    logger *slog.Logger
}

// NewResultTransformHook 创建结果转换 Hook。
// configs 可以为 nil，此时所有工具使用默认配置。
func NewResultTransformHook(configs []ResultTransformConfig) *ResultTransformHook {
    h := &ResultTransformHook{
        configs:          make(map[string]ResultTransformConfig),
        compiledPatterns: make(map[string][]*regexp.Regexp),
        logger:           slog.Default(),
    }

    for _, cfg := range configs {
        if cfg.ToolName == "" {
            h.defaultConfig = cfg
            continue
        }
        h.configs[cfg.ToolName] = cfg

        // 预编译正则
        var patterns []*regexp.Regexp
        for _, p := range cfg.SecretPatterns {
            re, err := regexp.Compile(p)
            if err != nil {
                slog.Warn("invalid regex pattern",
                    "tool", cfg.ToolName,
                    "pattern", p,
                    "error", err,
                )
                continue
            }
            patterns = append(patterns, re)
        }
        h.compiledPatterns[cfg.ToolName] = patterns
    }

    // 如果没有显式设置默认配置，使用全局默认
    if h.defaultConfig.MaxChars == 0 {
        h.defaultConfig = DefaultConfig()
    }

    // 预编译默认配置的正则
    var defaultPatterns []*regexp.Regexp
    for _, p := range h.defaultConfig.SecretPatterns {
        re, err := regexp.Compile(p)
        if err != nil {
            continue
        }
        defaultPatterns = append(defaultPatterns, re)
    }
    h.compiledPatterns["__default__"] = defaultPatterns

    return h
}

// Name 返回 Hook 标识符。
func (h *ResultTransformHook) Name() string {
    return "result_transform"
}

// Points 返回 AfterToolExecute，在工具执行成功后触发。
func (h *ResultTransformHook) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.AfterToolExecute}
}

// Priority 返回 20，属于业务逻辑层。
func (h *ResultTransformHook) Priority() int {
    return 20
}

// Execute 根据工具名获取配置并执行转换。
func (h *ResultTransformHook) Execute(ctx *hook.HookContext) error {
    if ctx.ToolResult == nil {
        return nil
    }

    // 获取该工具的配置（没有则使用默认）
    cfg, exists := h.configs[ctx.ToolName]
    if !exists {
        cfg = h.defaultConfig
    }

    // 将 Data 转为字符串
    dataStr := h.dataToString(ctx.ToolResult.Data)
    originalLen := utf8.RuneCountInString(dataStr)

    // 脱敏
    patterns := h.compiledPatterns[ctx.ToolName]
    if len(patterns) == 0 {
        patterns = h.compiledPatterns["__default__"]
    }
    for _, re := range patterns {
        dataStr = re.ReplaceAllString(dataStr, "[已脱敏]")
    }

    // 截断
    if cfg.MaxChars > 0 {
        dataStr = h.truncate(dataStr, cfg.MaxChars, cfg.TruncateStrategy)
    }

    // 尝试 JSON 格式化
    if cfg.StripJSON {
        dataStr = h.maybeFormatJSON(dataStr)
    }

    // 回写结果
    ctx.ToolResult.Data = dataStr

    if originalLen > cfg.MaxChars || len(patterns) > 0 {
        h.logger.Info("tool result transformed",
            "session_id", ctx.SessionID,
            "tool", ctx.ToolName,
            "original_chars", originalLen,
            "final_chars", utf8.RuneCountInString(dataStr),
            "redacted", len(patterns) > 0,
            "truncated", originalLen > cfg.MaxChars,
        )
    }

    return nil
}

// dataToString 将 Data 转为字符串。
func (h *ResultTransformHook) dataToString(data any) string {
    switch v := data.(type) {
    case string:
        return v
    case []byte:
        return string(v)
    case fmt.Stringer:
        return v.String()
    default:
        // 尝试 JSON 序列化
        b, err := json.Marshal(v)
        if err != nil {
            return fmt.Sprintf("%v", v)
        }
        return string(b)
    }
}

// truncate 按策略截断字符串。
// head: 保留开头 MaxChars 个字符
// tail: 保留末尾 MaxChars 个字符
// head_tail: 保留开头和末尾各 MaxChars/2 个字符
func (h *ResultTransformHook) truncate(s string, maxChars int, strategy string) string {
    runes := []rune(s)
    if len(runes) <= maxChars {
        return s
    }

    separator := fmt.Sprintf(
        "\n\n[... 中间 %d 个字符已省略 ...]\n\n",
        len(runes)-maxChars,
    )

    switch strategy {
    case "head":
        return string(runes[:maxChars]) + separator

    case "tail":
        return separator + string(runes[len(runes)-maxChars:])

    case "head_tail":
        half := maxChars / 2
        head := string(runes[:half])
        tail := string(runes[len(runes)-half:])
        return head + separator + tail

    default:
        // 默认行为：保留开头
        return string(runes[:maxChars]) + separator
    }
}

// maybeFormatJSON 尝试将 JSON 字符串格式化，使其更可读。
func (h *ResultTransformHook) maybeFormatJSON(s string) string {
    // 尝试解析为 JSON
    var obj any
    err := json.Unmarshal([]byte(s), &obj)
    if err != nil {
        return s
    }

    // 格式化输出
    formatted, err := json.MarshalIndent(obj, "", "  ")
    if err != nil {
        return s
    }

    return string(formatted)
}

// RedactSecrets 公开的辅助方法：对字符串进行脱敏处理。
// 可以在其他代码中复用。
func RedactSecrets(s string, patterns []*regexp.Regexp) string {
    result := s
    for _, re := range patterns {
        result = re.ReplaceAllString(result, "[已脱敏]")
    }
    return result
}

// TruncateString 公开的辅助方法：截断字符串。
func TruncateString(s string, maxLen int) string {
    runes := []rune(s)
    if len(runes) <= maxLen {
        return s
    }
    return string(runes[:maxLen]) + fmt.Sprintf(
        "\n\n[... 已截断 %d 个字符 ...]",
        len(runes)-maxLen,
    )
}

// Ensure non-empty import usage to avoid compilation error in example context.
var _ = tool.ToolResult{}
var _ = strings.TrimSpace
```

## 注册代码

```go
package main

import (
    "github.com/copcon/server/internal/hook"
    "your-project/transform"
)

func main() {
    runner := hook.NewHookRunner()

    // 创建结果转换 Hook，针对不同工具使用不同配置
    transformHook := transform.NewResultTransformHook([]transform.ResultTransformConfig{
        {
            ToolName:         "shell",
            MaxChars:         4000,
            TruncateStrategy: "head_tail",
            SecretPatterns: []string{
                `-----BEGIN [A-Z ]+ PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+ PRIVATE KEY-----`,
                `Bearer\s+[a-zA-Z0-9\-._~+/]+=*`,
                `password["\s:=]+[^\s,}]+`,
            },
        },
        {
            ToolName:         "file_read",
            MaxChars:         8000,
            TruncateStrategy: "tail",
            StripJSON:        false,
        },
        {
            ToolName:         "http_request",
            MaxChars:         5000,
            TruncateStrategy: "head",
            StripJSON:        true,
        },
    })

    runner.Register(transformHook)

    // 将 runner 传入引擎
    // engine := agent.NewEngine(agent.WithHookRunner(runner), ...)
}
```

## 执行流程说明

### Shell 命令输出截断

1. Shell 工具执行完毕，输出 50000 个字符
2. 触发 `AfterToolExecute`
3. `dataToString` 将 `Data` 转为字符串
4. 脱敏检查：找到 `-----BEGIN RSA PRIVATE KEY-----...`，替换为 `[已脱敏]`
5. 截断：按 `head_tail` 策略保留开头 2000 和末尾 2000 个字符
6. 中间插入提示：`[... 中间 46000 个字符已省略 ...]`
7. `ctx.ToolResult.Data` 被更新为处理后的字符串
8. LLM 收到截断和脱敏后的结果

### Token 节省对比

假设 shell 命令输出了 50000 个字符（约 16000 token）：

| 处理方式 | 字符数 | 估算 token 数 | 节省 |
|---------|--------|-------------|------|
| 原始输出 | 50000 | ~16000 | - |
| 截断 + 脱敏后 | ~4100 | ~1360 | ~91% |

巨大的 token 节省意味着更低的 API 成本、更快的响应速度和更少的上下文污染。

## 设计说明

**为什么默认使用 `head_tail` 策略？** 命令输出的开头和末尾通常是最有价值的信息（开头是命令结果概述，末尾是最终状态）。纯 `head` 可能丢失关键的错误信息，纯 `tail` 可能丢失概览信息。`head_tail` 各取一半，兼顾两者。

**为什么使用 rune 而不是 byte 做截断？** `utf8.RuneCountInString` 按 Unicode 字符（而非字节）计数。中文一个字是一个 rune，而不是 3 个 byte。用 byte 截断可能导致中文字符被截断成乱码。

**为什么 JSON 格式化的结果可能更大？** 是的，`json.MarshalIndent` 会增加空格和换行。但可读性的提升对 LLM 理解结果有巨大好处。如果你更看重 token 节省，可以设置 `StripJSON: false` 关闭格式化。

**脱敏的正则为什么这么复杂？** 这些模式是示例。实际项目中需要根据你使用的密钥格式定制。一个常见做法是在 base64 解码后做模式匹配，再对原始结果做替换。

## 扩展建议

- 添加 Compress 策略：用 LLM 对过长结果做摘要后再截断
- 添加 Extract 策略：只提取特定结构（如 JSON path）的数据
- 按 AgentID 区分配置：不同 LLM 的 token 限制不同
- 添加缓存：相同命令的相同结果不重复处理
- 将截断统计作为监控指标推送到 Prometheus