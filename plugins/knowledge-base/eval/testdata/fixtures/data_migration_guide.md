# 数据迁移指南

## 概述

本文档指导你将现有数据从其他平台迁移到 CopCon。我们支持从多种来源迁移知识库、对话历史和用户配置数据。

## 迁移前准备

### 评估迁移范围

1. **知识库文档**：统计文档数量、格式类型和总大小
2. **对话历史**：确认需要保留的历史会话范围
3. **用户和权限**：导出用户列表和角色配置
4. **集成配置**：记录 API Key、Webhook 和第三方集成

### 环境准备

```bash
# 安装迁移工具
pip install copcon-migrate

# 验证安装
copcon-migrate --version

# 配置目标环境
copcon-migrate config set \
  --api-url https://api.copcon.io \
  --api-key sk_live_xxxxxxxxxxxx
```

## 迁移流程

### 第一步：导出源数据

支持的数据源格式：

| 来源 | 导出方式 | 支持的数据类型 |
|------|---------|--------------|
| 自建知识库 | CSV/JSON 导出 | 文档、元数据 |
| Notion | API 批量导出 | 页面、数据库 |
| Confluence | REST API | 页面、附件 |
| 飞书文档 | 开放平台 API | 文档、表格 |
| 通用 | Markdown 文件 | 文档 |

### 第二步：数据转换

```bash
# 将源数据转换为 CopCon 格式
copcon-migrate convert \
  --source notion \
  --input ./export/ \
  --output ./converted/ \
  --format markdown

# 验证转换结果
copcon-migrate validate ./converted/
```

转换过程包括：
- 格式标准化（统一为 Markdown）
- 元数据映射（标签、分类、时间戳）
- 附件处理（图片上传到对象存储）
- 链接修正（更新内部引用）

### 第三步：批量导入

```bash
# 执行导入
copcon-migrate import \
  --input ./converted/ \
  --batch-size 100 \
  --parallel 4 \
  --dry-run  # 先试运行

# 正式导入（去掉 dry-run）
copcon-migrate import \
  --input ./converted/ \
  --batch-size 100 \
  --parallel 4
```

### 第四步：验证迁移结果

```bash
# 检查文档完整性
copcon-migrate verify --check-documents

# 检查向量索引
copcon-migrate verify --check-vectors

# 抽样检索测试
copcon-migrate verify --test-queries ./test_queries.json
```

## 常见问题

### 大规模文档迁移

超过 10,000 篇文档时建议分批迁移，每批 1,000 篇。导入过程中系统会自动重建向量索引，大批量导入可能导致索引构建延迟增大。

### 迁移回滚

导入操作支持回滚，每次导入创建一个快照点：

```bash
# 查看导入历史
copcon-migrate snapshots list

# 回滚到指定快照
copcon-migrate snapshots rollback --id snap_20250115_001
```

### 迁移期间服务影响

文档导入期间 API 服务正常可用。新导入的文档在向量索引构建完成后才可被检索到，通常延迟为 1-5 分钟（取决于文档数量）。
