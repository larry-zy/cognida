## ADDED Requirements

### Requirement: BM25 全文搜索
系统 SHALL 支持 BM25 算法进行全文搜索，允许用户使用原始文本查询检索相关文档。

#### Scenario: 成功的 BM25 搜索
- **WHEN** 用户使用查询文本 "机器学习算法" 进行 BM25 搜索
- **THEN** 系统 SHALL 返回包含该关键词的文档列表
- **AND** 结果 SHALL 按 BM25 相关性分数降序排列
- **AND** 每个结果 SHALL 包含文档内容、ChunkID、相关性分数

#### Scenario: BM25 搜索无结果
- **WHEN** 用户查询的文本在所有文档中都不存在
- **THEN** 系统 SHALL 返回空结果列表
- **AND** 不应抛出错误

#### Scenario: BM25 搜索带过滤条件
- **WHEN** 用户使用查询文本 "算法" 并指定 tenant_id 过滤条件
- **THEN** 系统 SHALL 仅返回指定租户的相关文档
- **AND** 结果 SHALL 同时满足文本匹配和过滤条件

### Requirement: BM25 Schema 配置
系统 SHALL 支持 BM25 功能的 Collection Schema 配置，包含必需的字段和函数。

#### Scenario: 创建 BM25-enabled Collection
- **WHEN** 创建新知识库并启用 BM25
- **THEN** 系统 SHALL 在 Schema 中添加 `text` 字段 (VARCHAR, enable_analyzer=true)
- **AND** 系统 SHALL 在 Schema 中添加 `sparse` 字段 (SPARSE_FLOAT_VECTOR)
- **AND** 系统 SHALL 配置 BM25 Function 将 `text` 映射到 `sparse`
- **AND** 系统 SHALL 创建 BM25 索引 (metric_type=BM25, inverted_index_algo=DAAT_MAXSCORE)

#### Scenario: BM25 索引参数配置
- **WHEN** 创建 BM25 索引
- **THEN** 系统 SHALL 使用默认参数 bm25_k1=1.2 (词频饱和度)
- **AND** 系统 SHALL 使用默认参数 bm25_b=0.75 (文档长度归一化)
- **AND** 系统 SHALL 设置 drop_ratio_search=0.2

### Requirement: entity.Text() API 支持
系统 SHALL 使用 Milvus SDK v2.6+ 的 `entity.Text()` API 进行文本查询。

#### Scenario: 使用 entity.Text() 构造查询
- **WHEN** 执行 BM25 搜索时
- **THEN** 系统 SHALL 使用 `entity.Text(query)` 构造搜索向量
- **AND** 系统 SHALL 在 `sparse` 字段上执行搜索
- **AND** 系统 SHALL 不需要手动将文本转换为稀疏向量

### Requirement: BM25 搜索降级处理
系统 SHALL 在 BM25 搜索失败时提供降级方案。

#### Scenario: Milvus 未配置 BM25 时的降级
- **WHEN** BM25 搜索失败（如 Collection 不存在 BM25 索引）
- **THEN** 系统 SHALL 返回空结果而非抛出错误
- **AND** 系统 SHALL 记录警告日志说明失败原因
- **AND** 上层 SHALL 可以 fallback 到数据库关键词匹配

#### Scenario: Milvus 连接失败时的处理
- **WHEN** Milvus 客户端连接失败
- **THEN** 系统 SHALL 返回错误
- **AND** 上层 SHALL fallback 到纯数据库检索

### Requirement: BM25 结果解析
系统 SHALL 正确解析 Milvus SDK v2.6+ 的搜索结果。

#### Scenario: 解析 BM25 搜索结果
- **WHEN** Milvus 返回 BM25 搜索结果
- **THEN** 系统 SHALL 从 ResultSet 中提取所有输出字段
- **AND** 系统 SHALL 正确映射字段值到 SearchResult 结构
- **AND** 系统 SHALL 保留 BM25 分数用于排序

#### Scenario: 处理字段缺失
- **WHEN** 某个输出字段在结果中不存在
- **THEN** 系统 SHALL 使用零值或默认值
- **AND** 不应导致整个搜索失败
