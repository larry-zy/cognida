# agent-tools Delta Specification

## MODIFIED Requirements

### Requirement: sql_execute 回传结果信封

`sql_execute` 工具 SHALL 把完整查询结果写入 [Result Store](../agent-result-store/spec.md) 并回传结果信封（`result_id` + 列 + dtype + `row_count` + 样本 + 聚合），MUST NOT 再将原始行逐字回灌 LLM。既有的 1000 行上限 SHALL 仍作用于底层查询保护，但回灌 LLM 的样本行数受信封 N 上限约束。

`sql_execute` SHALL 支持可选 `database_id` 参数：为空时在当前业务库执行（现状行为）；非空时 SHALL 经 ConnectionManager 路由到对应已注册数据源执行，且 MUST 施加外部数据源只读防护（见 datasource-query-safety）。`database_id` 指向不存在或不可用的数据源时，SHALL 回传明确错误的合成 ToolMessage，MUST NOT 静默回落到业务库。

#### Scenario: 查询结果入 Result Store 并回传信封

- **WHEN** LLM 调用 `sql_execute` 执行一条 SELECT
- **THEN** 工具 SHALL 把结果集写入 Result Store 生成 `result_id`
- **AND** 回灌 LLM 的 ToolMessage SHALL 是结果信封而非原始行

#### Scenario: 后续工具凭 result_id 复用

- **WHEN** LLM 拿到 `result_id` 后调用分析/导出/渲染工具
- **THEN** 这些工具 SHALL 凭 `result_id` 从 Result Store 取回完整数据
- **AND** LLM SHALL NOT 需要在上下文中重述原始行

#### Scenario: 指定 database_id 路由到外部数据源

- **WHEN** LLM 以非空 `database_id` 调用 `sql_execute`
- **THEN** 查询 SHALL 经 ConnectionManager 在对应外部数据源上执行
- **AND** 结果 SHALL 同样写入 Result Store 并回传结果信封

#### Scenario: 无效 database_id 明确报错

- **WHEN** `database_id` 指向不存在或已删除的数据源
- **THEN** 工具 SHALL 回传"数据源不存在/不可用"的合成错误 ToolMessage
- **AND** SHALL NOT 回落到当前业务库执行

### Requirement: get_schema 有界选表回传

`get_schema` 工具在未指定表名时 SHALL NOT 默认返回全库所有表，MUST 改为返回按相关度筛选（见 [agent-context-engineering](../agent-context-engineering/spec.md) 的词法选表）的相关候选子集；提供 `keywords` 时按其相关度选表，二者皆无时返回受上限约束的轻量目录（仅表名+描述，不含列）。指定表名时 SHALL 仍返回该表精确结构。

`get_schema` 的 `database_id` 参数 SHALL 被实装：为空时探查当前业务库（现状行为）；非空时 SHALL 经 ConnectionManager 对相应外部数据源做 schema 探查，有界选表规则（keywords 相关度、轻量目录上限）对外部数据源 SHALL 同样生效。

#### Scenario: 未指定表名时返回候选子集

- **WHEN** LLM 调用 `get_schema` 且未提供 `table_name`，但提供 `keywords`
- **THEN** 工具 SHALL 返回与关键词相关的候选表子集及其完整结构
- **AND** SHALL NOT 返回全库所有表结构

#### Scenario: 无表名无关键词时返回轻量目录

- **WHEN** LLM 调用 `get_schema` 且既无 `table_name` 又无 `keywords`
- **THEN** 工具 SHALL 返回受上限约束的表目录（仅表名+描述）
- **AND** SHALL NOT 一次性注入全部表的列结构

#### Scenario: 指定表名时精确返回

- **WHEN** LLM 调用 `get_schema` 并提供具体 `table_name`
- **THEN** 工具 SHALL 返回该表的精确列/类型/索引结构

#### Scenario: 对外部数据源探查同样有界

- **WHEN** LLM 以非空 `database_id` 调用 `get_schema` 且目标数据源表数量庞大
- **THEN** 工具 SHALL 按相同的有界选表规则返回候选子集或轻量目录
- **AND** SHALL NOT 把外部库全量表结构注入上下文
