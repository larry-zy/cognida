## ADDED Requirements

### Requirement: 语义模型建模写入口

系统 SHALL 提供受治理语义模型的建模写入口，使语义模型可经 REST 增删改与发布，替代此前「`UpsertBundle` 仅测试可达」的空壳状态。写入口 SHALL 提供 List/Get/Create/Update/Publish/Deprecate，并 SHALL 对提交的模型做两级校验：结构完整性（必填字段、状态合法）与引用一致性（维度/度量/指标/关系引用的逻辑表与列名在模型内可解析）；SHALL NOT 校验表达式语义（`Metric.Expr`/`Measure.Expr`/`Relation.JoinCondition` 属受信管理员 SQL，逐字采用）。`Update`/`Publish`/`Deprecate` MUST bump 模型 `Version` 以失效受信查询缓存。系统 SHALL 另提供幂等的冷启动 seed，向目标租户灌入 status=active 的初始模型，使 `semantic_query` 立即可行使治理主路。

#### Scenario: 经 REST 建模并发布

- **WHEN** 管理员经 `POST /semantic-models` 提交结构合法、引用一致的模型并 `POST /:id/publish`
- **THEN** 系统 SHALL 落库为 active 模型并进入查询主路径
- **AND** 后续命中该模型的提问 SHALL 走治理口径 SQL 而非回退

#### Scenario: 引用不一致被拒

- **WHEN** 提交的维度引用了模型内不存在的逻辑表或列
- **THEN** 系统 SHALL 以校验错误（400）拒绝，标明不一致的引用
- **AND** SHALL NOT 落库半成品模型

#### Scenario: 发布 bump 版本失效缓存

- **WHEN** 已发布模型被 `Update` 或重新 `Publish`
- **THEN** 模型 `Version` SHALL 递增
- **AND** 基于旧版本的受信查询缓存条目 SHALL 失效

### Requirement: 治理覆盖率可观测

系统 SHALL 对每次 `semantic_query` 落一条治理覆盖埋点，记录命中态（`covered`/`cache_hit`/`fallback`）、未覆盖的名称清单与 `request_id`（关联全链路审计与 Loki 原始日志）。埋点 SHALL 为 best-effort：写入失败 MUST NOT 阻断取数主路。系统 SHALL 提供按语义模型聚合的覆盖率读取（`GET /semantic-coverage`），返回每模型的 `covered`/`cache_hit`/`fallback` 计数与命中率 `HitRatio=(covered+cache_hit)/total`，且 `Total` MUST 恒等于三桶之和（未知 outcome 不计入任一桶与分母）。覆盖能力未启用时，读取 SHALL 返回 `{items:[], enabled:false}` 而非报错。

#### Scenario: 命中与回退均可观测

- **WHEN** 一次提问被语义模型完整覆盖并生成治理 SQL
- **THEN** 系统 SHALL 落一条 `outcome=covered` 的埋点
- **AND** 后续同签名提问命中受信缓存时 SHALL 落 `outcome=cache_hit`
- **AND** 涉及未覆盖名称回退时 SHALL 落 `outcome=fallback` 并记录未覆盖名称

#### Scenario: 埋点失败不阻断主路

- **WHEN** 覆盖埋点写入因存储异常失败
- **THEN** `semantic_query` SHALL 照常返回取数结果
- **AND** SHALL NOT 因埋点失败而报错

#### Scenario: 按模型聚合命中率

- **WHEN** 查询 `GET /semantic-coverage`
- **THEN** 系统 SHALL 返回每语义模型的三态计数与 `HitRatio`
- **AND** 每模型的 `Total` SHALL 等于三态计数之和

## MODIFIED Requirements

### Requirement: NL2Semantics 查询主路径

查询能力 SHALL 以 NL2Semantics 为主路径：先意图识别，再将请求拆解为语义模型中的指标 + 维度，最后由指标引擎生成 SQL，替代裸 NL2SQL。当语义模型未覆盖目标库表时，系统 SHALL 回退到 Phase 1 的词法 `get_schema` NL2SQL 路径。主路径命中与回退 SHALL 落治理覆盖埋点（见「治理覆盖率可观测」），使「走了治理口径还是回退」可结构化度量而非仅结果内文字标注。语义引擎 MUST 在任一名称不可解析时整体判定为未覆盖（不产出半成品 SQL），据此触发回退。

#### Scenario: 覆盖表走 NL2Semantics

- **WHEN** 提问命中语义模型已建模的指标与维度
- **THEN** 系统 SHALL 经指标引擎按既定口径生成 SQL
- **AND** SHALL NOT 让 LLM 裸生成聚合逻辑
- **AND** SHALL 落 `outcome=covered` 覆盖埋点

#### Scenario: 未覆盖表回退词法 NL2SQL

- **WHEN** 提问涉及语义模型未覆盖的库表或名称
- **THEN** 系统 SHALL 回退到词法 `get_schema` 选表 + NL2SQL
- **AND** SHALL 在结果中可观测标注走了回退路径
- **AND** SHALL 落 `outcome=fallback` 覆盖埋点并记录未覆盖名称

### Requirement: 知识图谱业务术语 grounding

NL2Semantics 的意图识别与指标/维度拆解 SHALL 复用既有 Neo4j 知识图谱与血缘做术语接地，把业务术语映射到受治理的指标/维度/度量名。接地 SHALL 为确定性、可观测的两层：(1) 模型内同义词——用 `ModelBundle` 的 name/synonyms 建归一化倒排索引精确匹配；(2) 图谱回落——模型内未命中时，经 `GraphAdapter` 复用 `knowledge.GraphRepository`，沿 SIMILAR_TO/RELATED_TO/BELONGS_TO 边把术语解析为图谱中的同义/相关名，再回落到模型内索引。每个候选 SHALL 标注来源（`name`/`synonym`/`graph`）与经由术语（`Via`），并按可信度排序（name > synonym > graph）。一个术语命中多个不同受治理概念时 SHALL 判为歧义、只回传候选交上层反问，MUST NOT 硬选。图谱端口不可用（未连 Neo4j）时，接地 SHALL 退化为仅模型内同义词而非报错。图谱第二层所依赖的「业务口语 → 规范名」桥接证据 SHALL 可经 seed 注入（与运行时检索命名空间 tenant/kb_id 一致）。

#### Scenario: 业务口语经图谱回落到指标

- **WHEN** 用户用模型内未登记的业务口语（如"流水"）指代已建模指标（如"营收"）
- **THEN** 系统 SHALL 经知识图谱 SIMILAR_TO 桥接把术语回落到对应指标
- **AND** 候选来源 SHALL 标注为 `graph` 且记录经由的图谱别名

#### Scenario: 歧义反问而非硬猜

- **WHEN** 一个术语命中多个不同的受治理概念
- **THEN** 系统 SHALL 回传全部候选并交上层反问用户
- **AND** SHALL NOT 擅自选定其一

#### Scenario: 无图谱时退化

- **WHEN** 未连接 Neo4j 或图谱端口不可用
- **THEN** 接地 SHALL 仅用模型内同义词工作
- **AND** SHALL NOT 因缺图谱而报错
