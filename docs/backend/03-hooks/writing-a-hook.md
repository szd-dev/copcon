# 编写一个 Hook

本文从零开始，带你一步步实现一个真正可用的 Hook。

## 前置知识

编写 Hook 之前，你需要了解：

- `hook.Hook` 接口（定义在 `server/internal/hook/hook.go`）
- `hook.HookContext` 结构体（Hook 执行时的上下文数据）
- `hook.HookRunner` 接口（注册和运行 Hook 的入口）

如果你还不熟悉这些概念，建议先阅读 [Hook 系统概览](./overview.md)。

---

## 五分钟速览

Hook 本质上是实现 `hook.Hook` 接口的一个结构体：

```go
type Hook interface {
    Name() string
    Points() []HookPoint
    Priority() int
    Execute(ctx *HookContext) error
}
```

完成以下五步就能写出一个 Hook：

| 步骤 | 做什么 | 关键点 |
|------|--------|--------|
| 1 | 定义结构体 | 把依赖注入进去 |
| 2 | 选择 HookPoint | 决定在哪个时机执行 |
| 3 | 设置优先级 | 数字越大越先执行 |
| 4 | 实现 Execute | 读取和修改 HookContext |
| 5 | 注册到 HookRunner | 在程序启动时调用 Register |

---

## 第一步：定义结构体

Hook 是一个结构体，需要的依赖通过构造函数注入。

```go
package myhooks

import (
    "log/slog"
    "github.com/copcon/server/internal/hook"
)

// SensitiveContentFilter 在消息发送前检查并过滤敏感内容。
type SensitiveContentFilter struct {
    // badWords 是需要过滤的敏感词列表
    badWords []string
    // logger 用于记录过滤日志
    logger   *slog.Logger
}

// NewSensitiveContentFilter 创建一个新的过滤器实例。
func NewSensitiveContentFilter(badWords []string) *SensitiveContentFilter {
    return &SensitiveContentFilter{
        badWords: badWords,
        logger:   slog.Default(),
    }
}
```

**为什么用结构体而不是函数？**

因为 Hook 通常需要持有依赖（数据库连接、配置、外部服务等）。结构体让这些依赖自然地挂在实例上，不必通过全局变量传递。这也让测试变得简单，你可以注入 mock 依赖。

---

## 第二步：实现 Hook 接口 — Name、Points、Priority

接着给结构体添加三个简单方法：

```go
// Name 返回可读的标识符，用于日志和调试。
func (f *SensitiveContentFilter) Name() string {
    return "sensitive_content_filter"
}

// Points 返回这个 Hook 希望在哪些 HookPoint 上执行。
// 一个 Hook 可以注册到多个 Point。这里只在消息构建完成后执行。
func (f *SensitiveContentFilter) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.AfterContextBuild}
}

// Priority 返回执行优先级。50 是中优先级，不抢先也不拖后。
func (f *SensitiveContentFilter) Priority() int {
    return 50
}
```

### Name 的命名约定

`Name()` 没有严格的格式要求，但建议使用 `snake_case`，长度控制在 50 个字符以内。名字会在日志中出现，所以起个好认的名字能大大减少排查问题的时间。

### 选择 HookPoint

10 个 HookPoint 各有用处。这里列出最常见的几个选择场景：

| 你想做什么 | 选择 |
|-----------|------|
| 修改系统提示词 | `OnSystemPrompt` 或 `BeforeContextBuild` |
| 往消息里插入内容 | `AfterContextBuild` 或 `BeforeLLMCall` |
| 保存消息到外部存储 | `OnMessagePersist` |
| 检查工具参数 | `BeforeToolExecute` |
| 改写工具结果 | `AfterToolExecute` |
| 处理工具错误 | `OnToolError` |
| 记录请求耗时 | `BeforeLLMCall` + `AfterLLMCall` 配合使用 |

完整列表见 [HookPoint 速查表](./overview.md#hookpoint-速查表)。

---

## 第三步：设置优先级

优先级的规则很简单：**数字越大，越先执行**。

```text
场景：三个 Hook 都注册在 AfterContextBuild 上

Hook A: Priority = 200  ← 第 1 个执行（日志记录，必须最先）
Hook B: Priority = 100  ← 第 2 个执行（默认优先级）
Hook C: Priority =  20  ← 第 3 个执行（用户自定义逻辑，最后）
```

### 推荐优先级分层

| 层级 | 优先级范围 | 适用场景 |
|------|-----------|---------|
| 基础设施层 | 100 - 200 | 日志、监控、链路追踪。必须最早执行，因为其他 Hook 依赖它们收集的数据。 |
| 上下文增强层 | 50 - 99 | 知识库注入、提示词拼接、消息预处理。需要在业务逻辑之前完成。 |
| 业务逻辑层 | 1 - 49 | 用户自己的业务逻辑，如过滤、校验、转换等。 |

真实项目中通常不需要这么精细。用 20、50、100 三档就够了：

- **100**：如果是基础设施（日志、监控）
- **50**：上下文增强或系统级 Hook
- **20**：用户自定义业务逻辑

### 同优先级时怎么办

同优先级的 Hook 按注册先后顺序执行。先 Register 的先跑。如果不确定优先级该设多少，用默认值 100 就好。

---

## 第四步：实现 Execute

`Execute` 是整个 Hook 的核心。它接收 `*HookContext`，读取或修改其中的字段，然后返回 `error`。

### 代码结构模板

```go
func (f *SensitiveContentFilter) Execute(ctx *hook.HookContext) error {
    // 1. 检查必要的字段是否为 nil
    if ctx.Messages == nil || len(*ctx.Messages) == 0 {
        return nil  // 没有消息可处理，正常返回
    }

    // 2. 基于 CurrentPoint 做判断（如果注册了多个 Point）
    switch ctx.CurrentPoint {
    case hook.AfterContextBuild:
        return f.filterMessages(ctx)
    default:
        return nil
    }
}

func (f *SensitiveContentFilter) filterMessages(ctx *hook.HookContext) error {
    for i, msg := range *ctx.Messages {
        (*ctx.Messages)[i].Content = f.replaceSensitiveWords(msg.Content)
    }

    // 3. 记录日志
    f.logger.Info("sensitive content filtered",
        "session_id", ctx.SessionID,
    )

    // 4. 返回 nil 表示成功。返回 error 不会中断链路。
    return nil
}
```

### 关键原则

**字段为 nil 时务必检查。** 不是每个 HookPoint 都会填充所有字段。例如 `AfterContextBuild` 上 `ToolName` 一定是 `""`，`ToolArgs` 一定是 `nil`。如果你不检查就直接用，会 panic。

**不要依赖执行顺序。** 即使你设置了最高优先级，也不要在 `Execute` 中假设"我是第一个被调用的"。永远把输入视为不确定的状态，做好防御处理。

**返回 error 不会中断链路。** 你返回的 error 只会被记录为 Warn 日志，后续 Hook 照样跑。如果你需要"阻断"后续 Hook，目前 Hook 系统不支持这个语义。设计上假定所有 Hook 都是可选的增强逻辑，不存在"必须通过才能继续"的 Hook。

### 什么是"链式副作用"

如果 Hook A 修改了 `Messages`，Hook B 看到的是修改后的 `Messages`。指针类型的字段在所有 Hook 之间是共享的。

```text
原始 Messages: [msg1, msg2]

Hook A (Priority=100) 执行，追加 msg3
Messages 变为: [msg1, msg2, msg3]

Hook B (Priority=50) 执行，删除 msg2
Messages 变为: [msg1, msg3]
```

这种"链式副作用"是做上下文注入的核心机制。不过要注意，除非你在文档中明确说明，否则不要假设其他 Hook 的行为。

---

## 第五步：注册到 HookRunner

写完 Hook 代码后，需要在程序初始化时把它注册到 `HookRunner` 中。

```go
package main

import (
    "github.com/copcon/server/internal/hook"
    "your-project/myhooks"
)

func main() {
    // 创建 HookRunner
    runner := hook.NewHookRunner()

    // 创建 Hook 实例
    filter := myhooks.NewSensitiveContentFilter([]string{"password", "secret"})

    // 注册
    runner.Register(filter)

    // runner 现在可以在引擎中使用
    // engine := agent.NewEngine(runner, ...)
}
```

注册是并发安全的，你可以在运行时注册新的 Hook，但通常推荐在 `main()` 中一次性完成所有注册，这样逻辑更容易追踪。

---

## 完整示例

下面是一个完整的、可直接编译的 Hook。它在 `AfterContextBuild` 上执行，将所有消息中的敏感词替换为 `[已过滤]`。

```go
package sensitivehook

import (
    "log/slog"
    "strings"

    "github.com/copcon/server/internal/hook"
)

// SensitiveWordFilter 在消息发送给 LLM 之前过滤敏感词。
type SensitiveWordFilter struct {
    words  []string
    logger *slog.Logger
}

func NewSensitiveWordFilter(words []string) *SensitiveWordFilter {
    return &SensitiveWordFilter{
        words:  words,
        logger:  slog.Default(),
    }
}

func (f *SensitiveWordFilter) Name() string {
    return "sensitive_word_filter"
}

func (f *SensitiveWordFilter) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.AfterContextBuild}
}

func (f *SensitiveWordFilter) Priority() int {
    return 20
}

func (f *SensitiveWordFilter) Execute(ctx *hook.HookContext) error {
    if ctx.Messages == nil || len(*ctx.Messages) == 0 {
        return nil
    }

    for i, msg := range *ctx.Messages {
        filtered := msg.Content
        for _, word := range f.words {
            filtered = strings.ReplaceAll(filtered, word, "[已过滤]")
        }
        (*ctx.Messages)[i].Content = filtered
    }

    f.logger.Debug("sensitive words filtered", "session_id", ctx.SessionID)

    return nil
}
```

用法：

```go
runner := hook.NewHookRunner()
filter := sensitivehook.NewSensitiveWordFilter([]string{"password", "secret", "token"})
runner.Register(filter)
```

---

## 常见问题

### Q: 我的 Hook 注册了多个 HookPoint，Execute 怎么区分当前是哪个？

用 `ctx.CurrentPoint` 做 switch：

```go
func (h *MyHook) Execute(ctx *hook.HookContext) error {
    switch ctx.CurrentPoint {
    case hook.AfterContextBuild:
        return h.handleAfterBuild(ctx)
    case hook.OnMessagePersist:
        return h.handlePersist(ctx)
    default:
        return nil
    }
}
```

### Q: 我想在一个 HookPoint 上注册多次同一个 Hook，可以吗？

技术上可以，但不推荐。如果需要多次执行同一逻辑，在不同的 HookPoint 上注册就好。

### Q: 如果我在 Execute 里 panic 了会怎样？

HookRunner 会 recover 并记录 Error 日志，然后继续执行下一个 Hook。即使 panic 也不会导致引擎崩溃。