# agent-semantic-layer Specification

## Purpose
TBD - created by archiving change data-agent-evolution. Update Purpose after archive.
## Requirements
### Requirement: 指标语义模型契约

系统 SHALL 提供受治理的指标语义模型作为查询地基，定义逻辑表、维度、度量、指标与表间关系。语义模型 SHALL 经 GORM model 落 MySQL（`agent_semantic_*` 前缀），由 `cmd/migrate-db` 从 model 同步表结构。指标口径（如 revenue/churn 的计算逻辑）MUST 集中定义于语义模型，SHALL NOT 由 LLM 每次自行推断。

#### Scenario: 指标口径集中定义

- **WHEN** 用户询问"本月营收"
- **THEN** 系统 SHALL 采用语义模型中 `revenue` 指标的既定口径
- **AND** SHALL NOT 让 LLM 自由拼装营收计算逻辑

#### Scenario: 语义模型经 migrate-db 同步

- **WHEN** 新增或修改 `agent_semantic_*` GORM model
- **THEN** 表结构 SHALL 由 `cmd/migrate-db` 从 model 幂等同步
- **AND** SHALL NOT 依赖手写 SQL 迁移

### Requirement: NL2Semantics 查询主路径

查询能力 SHALL 以 NL2Semantics 为主路径：先意图识别，再将请求拆解为语义模型中的指标 + 维度，最后由指标引擎生成 SQL，替代裸 NL2SQL。当语义模型未覆盖目标库表时，系统 SHALL 回退到 Phase 1 的词法 `get_schema` NL2SQL 路径。

#### Scenario: 覆盖表走 NL2Semantics

- **WHEN** 提问命中语义模型已建模的指标与维度
- **THEN** 系统 SHALL 经指标引擎按既定口径生成 SQL
- **AND** SHALL NOT 让 LLM 裸生成聚合逻辑

#### Scenario: 未覆盖表回退词法 NL2SQL

- **WHEN** 提问涉及语义模型未覆盖的库表
- **THEN** 系统 SHALL 回退到词法 `get_schema` 选表 + NL2SQL
- **AND** SHALL 在结果中可观测标注走了回退路径

### Requirement: Verified / Golden Query 语义缓存

系统 SHALL 维护「问题 → 专家校验的指标查询/SQL」对（Verified/Golden Query）。提问 SHALL 先查语义缓存，命中则直接返回受信结果并沿用 `result_id` 信封契约，未命中才走 NL2Semantics 生成。缓存键 MUST 含语义模型版本，模型版本变更时相关缓存 SHALL 失效。

#### Scenario: 命中缓存返回受信结果

- **WHEN** 提问语义等价于一条已校验的 Verified Query
- **THEN** 系统 SHALL 返回该 Verified Query 的结果
- **AND** SHALL NOT 重新走 LLM 生成

#### Scenario: 模型版本变更缓存失效

- **WHEN** 语义模型版本从 v1 升到 v2
- **THEN** 基于 v1 的缓存条目 SHALL 失效
- **AND** 相应提问 SHALL 重新走生成或重新校验

### Requirement: 知识图谱业务术语 grounding

NL2Semantics 的意图识别与指标/维度拆解 SHALL 复用既有 Neo4j 知识图谱与血缘，把业务术语（同义词、口径、上下游关系）作为 grounding 上下文注入，降低"听不懂业务语言"的失败。

#### Scenario: 业务同义词映射到指标

- **WHEN** 用户用业务口语（如"流水"）指代已建模指标（如 `gmv`）
- **THEN** 系统 SHALL 借知识图谱把术语映射到对应指标
- **AND** SHALL 在歧义无法消解时反问而非硬猜

