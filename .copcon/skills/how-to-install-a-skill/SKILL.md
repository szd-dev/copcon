---
name: how-to-install-a-skill
description: 说明如何在 CopCon 中安装一个新的 Skill。包括目录结构、SKILL.md 格式、放置位置和验证方法。
metadata:
  version: "1.0.0"
  author: copcon-team
allowed-tools: Read Write Bash
---

# 如何安装一个新的 Skill

## 什么是 Skill

Skill 是 CopCon 的可插拔能力模块。每个 Skill 是一个包含 `SKILL.md` 文件的目录，Agent 可以通过 Skill 工具自动发现和调用它们。Skill 可以为 Agent 提供专业领域的指令、参考文档、脚本和资源文件。

## 安装步骤

### 1. 创建 Skill 目录

在 `.copcon/skills/` 下创建一个以 Skill 名称命名的子目录。目录名必须符合 `^[a-z0-9]+(-[a-z0-9]+)*$` 规则（小写字母、数字、连字符）。

```bash
mkdir -p .copcon/skills/my-skill
```

可选地，创建资源子目录：

```bash
mkdir -p .copcon/skills/my-skill/references   # 参考文档
mkdir -p .copcon/skills/my-skill/scripts      # 可执行脚本
mkdir -p .copcon/skills/my-skill/assets       # 静态资源
```

### 2. 编写 SKILL.md

在 Skill 目录下创建 `SKILL.md` 文件，格式如下：

```markdown
---
name: my-skill
description: 简短描述这个 Skill 的功能和用途。
metadata:
  version: "1.0.0"
  author: your-name
allowed-tools: Read Write Bash
---

# Skill 标题

## 指令

1. 第一条指令
2. 第二条指令

## 示例

### 示例 1
当用户询问 X 时，执行 Y。
```

#### Frontmatter 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | ✅ | Skill 名称，必须与目录名一致 |
| `description` | ✅ | 简短描述，不能为空 |
| `license` | ❌ | 许可证信息（如 MIT） |
| `metadata` | ❌ | 自定义键值对（如 version、author） |
| `allowed-tools` | ❌ | 空格分隔的工具列表，限制此 Skill 可使用的工具 |

### 3. 添加资源文件（可选）

将参考文档、脚本等放入对应的资源子目录：

```
.copcon/skills/my-skill/
├── SKILL.md
├── references/
│   └── api-docs.md
├── scripts/
│   └── setup.sh
└── assets/
    └── logo.png
```

资源文件会被自动扫描，Agent 可以通过 Skill 工具读取它们。

### 4. 验证安装

重启 CopCon 后端服务后，Skill 会自动被发现和加载：

```bash
make restart-server
```

通过 API 验证：

```bash
curl http://localhost:8088/api/skills | jq '.skills[] | {name, enabled}'
```

或在 Demo 前端中切换到「Skills」标签页查看。

### 5. 启用 / 禁用

Skill 默认启用。可以通过 API 或前端界面切换：

```bash
# 禁用
curl -X POST http://localhost:8088/api/skills/my-skill/disable

# 启用
curl -X POST http://localhost:8088/api/skills/my-skill/enable
```

## Skill 发现路径

CopCon 按以下顺序搜索 Skill（同名 Skill 以先找到的为准）：

1. `config.yaml` 中 `skills.extra_paths` 指定的额外路径
2. `<项目根>/.copcon/skills/`
3. `<项目根>/.agents/skills/`
4. `~/.copcon/skills/`
5. `~/.agents/skills/`

## 完整示例

参考 `.copcon/skills/sample/` 目录中的示例 Skill。

## 注意事项

- 目录名必须与 `SKILL.md` 中的 `name` 字段完全一致
- `SKILL.md` 必须以 `---` 开头和结尾的 YAML frontmatter 开始
- `description` 不能为空
- Skill 修改后需要重启后端服务才能生效
