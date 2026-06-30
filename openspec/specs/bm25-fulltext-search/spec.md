# bm25-fulltext-search Specification

## Purpose
TBD - created by archiving change milvus-sdk-upgrade. Update Purpose after archive.
## Requirements
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

### Requirement: Milvus Collection 支持 BM25 全文搜索
系统 SHALL 在 Milvus Collection 中配置 BM25 Function，实现文本到稀疏向量的自动转换和 BM25 评分。

#### Scenario: Collection Schema 包含必需字段
- **WHEN** 创建 Milvus Collection Schema
- **THEN** Schema SHALL 包含 `text` 字段（VARCHAR，enable_analyzer=true）
- **AND** Schema SHALL 包含 `sparse` 字段（SPARSE_FLOAT_VECTOR）
- **AND** Schema SHALL 包含 BM25 Function（input_field_names=["text"], output_field_names=["sparse"]）

### Requirement: BM25 索引配置
系统 SHALL 为稀疏向量字段创建 `SPARSE_INVERTED_INDEX` 索引，使用 BM25 作为 metric_type。

#### Scenario: 索引参数配置
- **WHEN** 创建稀疏向量索引
- **THEN** index_type SHALL 为 `SPARSE_INVERTED_INDEX` 或 `AUTOINDEX`
- **AND** metric_type SHALL 为 `BM25`
- **AND** inverted_index_algo SHALL 为 `DAAT_MAXSCORE`（默认）
- **AND** bm25_k1 参数 SHALL 可配置（范围 [1.2, 2.0]，默认 1.2）
- **AND** bm25_b 参数 SHALL 可配置（范围 [0, 1]，默认 0.75）

### Requirement: 文本数据自动向量化
系统 SHALL 在插入文本数据时，自动使用 BM25 Function 生成稀疏向量。

#### Scenario: 插入文本数据
- **WHEN** 插入包含 `text` 字段的数据
- **THEN** 系统 SHALL 自动调用 BM25 Function 生成稀疏向量
- **AND** 生成的稀疏向量 SHALL 存储在 `sparse` 字段
- **AND** 用户不需要手动提供稀疏向量

### Requirement: BM25 全文检索
系统 SHALL 支持使用原始文本查询进行 BM25 检索，自动将查询转换为稀疏向量并计算 BM25 相关性分数。

#### Scenario: 文本查询
- **WHEN** 使用文本查询执行检索
- **THEN** 系统 SHALL 自动将查询文本转换为稀疏向量
- **AND** 系统 SHALL 使用 BM25 算法计算每个文档的相关性分数
- **AND** 结果 SHALL 按 BM25 分数降序排列

#### Scenario: 查询参数支持
- **WHEN** 执行 BM25 检索
- **THEN** drop_ratio_search 参数 SHALL 可配置（范围 [0, 1]，默认 0.2）
- **AND** topK 参数 SHALL 控制返回结果数量

### Requirement: 中文分词支持
系统 SHALL 使用 Milvus 内置的 standard 分析器支持中文分词。

#### Scenario: 中文文本检索
- **WHEN** 查询或文档包含中文文本
- **THEN** 系统 SHALL 使用 standard 分析器进行分词
- **AND** 分词结果 SHALL 用于 BM25 评分计算

