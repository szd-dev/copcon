# 知识库管理指南

## 概述

知识库是 CopCon Agent 获取领域知识的核心来源。本文档说明如何创建、配置和管理知识库，确保 Agent 能高效准确地检索信息。

## 创建知识库

### 控制台创建

1. 进入「知识库」页面
2. 点击「新建知识库」
3. 输入名称和描述
4. 选择嵌入模型（默认 text-embedding-3-small, 1536 维）
5. 配置分块策略
6. 创建完成

### API 创建

```bash
curl -X POST https://api.copcon.io/api/v1/knowledge \
  -H "Authorization: Bearer sk_live_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "产品文档",
    "description": "CopCon 产品相关文档",
    "embedding_model": "text-embedding-3-small",
    "chunk_config": {
      "max_tokens": 800,
      "overlap_tokens": 100,
      "separator": "\n\n"
    }
  }'
```

## 文档导入

### 支持的格式

| 格式 | 扩展名 | 说明 |
|------|--------|------|
| Markdown | .md | 推荐，解析效果最好 |
| PDF | .pdf | 支持文本提取，扫描件需 OCR |
| Word | .docx | 提取文本和基本格式 |
| 纯文本 | .txt | 简单文本，无格式 |
| HTML | .html | 提取正文内容 |
| CSV | .csv | 每行作为独立文档 |

### 分块策略

文档导入时自动分块，影响检索质量的关键参数：

- **max_tokens**：单个块的最大 token 数。默认 800。较大的值保留更多上下文，但可能降低检索精度；较小的值提高精度但可能丢失上下文。
- **overlap_tokens**：相邻块的重叠 token 数。默认 100。重叠确保跨块边界的信息不会丢失。
- **separator**：分块优先分隔符。默认双换行（段落级分隔）。

### 批量导入

```bash
# 批量导入目录下所有文件
curl -X POST https://api.copcon.io/api/v1/knowledge/kb_abc123/import \
  -H "Authorization: Bearer sk_live_xxx" \
  -F "files=@./docs/" \
  -F "batch_size=50"
```

批量导入支持最多 1000 个文件，单文件最大 50MB（专业版）/ 100MB（企业版）。

## 检索配置

### 检索模式

| 模式 | 说明 | 适用场景 |
|------|------|---------|
| dense | 纯向量检索 | 语义匹配场景 |
| sparse | BM25 关键词检索 | 精确关键词匹配 |
| hybrid | 混合检索 | 推荐模式，兼顾语义和关键词 |

### 重排序

启用重排序后，检索结果先粗排召回（top-20），再通过 reranker 模型精排（top-5），显著提升 MRR：

```json
{
  "retrieval": {
    "mode": "hybrid",
    "top_k": 20,
    "rerank": true,
    "rerank_top_k": 5
  }
}
```

### 元数据过滤

支持按文档元数据筛选检索范围：

```json
{
  "query": "退款政策",
  "filters": {
    "category": "政策文档",
    "created_after": "2025-01-01",
    "tags": ["客户服务", "财务"]
  }
}
```

## 知识库维护

### 更新文档

```bash
# 更新单个文档
curl -X PUT https://api.copcon.io/api/v1/knowledge/kb_abc123/docs/doc_xyz \
  -H "Authorization: Bearer sk_live_xxx" \
  -F "file=@updated_doc.md"
```

更新文档时，系统自动重新分块和索引，旧版本标记为已归档。

### 删除文档

删除文档后，相关向量索引在 5 分钟内清理完毕。删除操作不可逆，建议先归档。

### 版本控制

企业版支持知识库版本管理：

- 每次导入或更新创建新版本
- 可回滚到任意历史版本
- 版本间差异对比
- 版本标签和备注

## 质量评估

定期评估知识库检索质量，确保 Agent 回答的准确性：

- **Recall@5**：前 5 个结果中包含正确答案的比例，目标 ≥ 0.80
- **MRR**：第一个正确结果的排名倒数的均值，目标 ≥ 0.75
- **覆盖率**：查询能返回相关结果的比例，目标 ≥ 0.90

使用 `eval/testdata/golden_set.jsonl` 中的标准测试集进行自动化评估。
