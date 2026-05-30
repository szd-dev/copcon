
## Task 13: 已知问题

### vec0 KNN 使用 L2 距离而非 cosine 距离
- vec0 虚拟表的 `WHERE embedding MATCH ?` KNN 搜索始终使用 L2 距离
- 对于非归一化向量，L2 距离排序可能与 cosine 相似度排序不一致
- 当前方案：KNN 用 L2 排序筛选候选 + `vec_distance_cosine()` 精确计算 cosine 距离
- 如果向量是归一化的（如 OpenAI embeddings），L2 排序与 cosine 排序等价
- 未来改进：如果 sqlite-vec 支持在 vec0 定义中指定距离度量，可直接使用 cosine KNN

### FNV-1a 哈希碰撞风险
- chunkID → rowID 使用 FNV-1a 64位哈希，理论上存在碰撞风险
- 对于知识库场景（通常 < 100万 chunks），碰撞概率可忽略
- 如果未来需要绝对无碰撞，可考虑添加自增整数主键到 chunkModel
