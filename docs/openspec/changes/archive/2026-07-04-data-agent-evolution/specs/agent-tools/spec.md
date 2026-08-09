## ADDED Requirements

### Requirement: sql_execute 回传结果信封

`sql_execute` 工具 SHALL 把完整查询结果写入 [Result Store](../agent-result-store/spec.md) 并回传结果信封（`result_id` + 列 + dtype + `row_count` + 样本 + 聚合），MUST NOT 再将原始行逐字回灌 LLM。既有的 1000 行上限 SHALL 仍作用于底层查询保护，但回灌 LLM 的样本行数受信封 N 上限约束。

#### Scenario: 查询结果入 Result Store 并回传信封

- **WHEN** LLM 调用 `sql_execute` 执行一条 SELECT
- **THEN** 工具 SHALL 把结果集写入 Result Store 生成 `result_id`
- **AND** 回灌 LLM 的 ToolMessage SHALL 是结果信封而非原始行

#### Scenario: 后续工具凭 result_id 复用

- **WHEN** LLM 拿到 `result_id` 后调用分析/导出/渲染工具
- **THEN** 这些工具 SHALL 凭 `result_id` 从 Result Store 取回完整数据
- **AND** LLM SHALL NOT 需要在上下文中重述原始行

### Requirement: get_schema 有界选表回传

`get_schema` 工具在未指定表名时 SHALL NOT 默认返回全库所有表，MUST 改为返回按相关度筛选（见 [agent-context-engineering](../agent-context-engineering/spec.md) 的词法选表）的相关候选子集；提供 `keywords` 时按其相关度选表，二者皆无时返回受上限约束的轻量目录（仅表名+描述，不含列）。指定表名时 SHALL 仍返回该表精确结构。

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

### Requirement: 工具执行受硬门禁约束

所有工具的执行 SHALL 在执行前受 [skill-tool-policy](../skill-tool-policy/spec.md) 的硬工具门与会话 scope 约束。被拒调用 MUST NOT 触达底层执行，且 SHALL 以合成错误 ToolMessage 回灌 LLM。

#### Scenario: 被门禁拦截的工具不执行

- **WHEN** 某工具调用未通过 skill 策略或会话 scope 校验
- **THEN** 工具的底层执行 SHALL NOT 被触发
- **AND** 系统 SHALL 回传 `tool_blocked` 合成 ToolMessage
