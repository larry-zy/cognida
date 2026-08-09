## MODIFIED Requirements

### Requirement: sql_execute 回传结果信封

`sql_execute` 工具 SHALL 把完整查询结果写入 [Result Store](../agent-result-store/spec.md) 并回传结果信封（`result_id` + 列 + dtype + `row_count` + 样本 + 聚合），MUST NOT 再将原始行逐字回灌 LLM。既有的 1000 行上限 SHALL 仍作用于底层查询保护，但回灌 LLM 的样本行数受信封 N 上限约束。

当查询**失败**时，`sql_execute` SHALL NOT 回灌裸底层 driver 错误文本，MUST 改为按 [agent-error-repair](../agent-error-repair/spec.md) 回灌结构化可修复观察（`error_kind` + `retriable` + 适用时的 schema-grounded `hint`）；对 `transient` 错误 SHALL 先按 agent-error-repair 做工具内退避重试，耗尽后才上抛。外部数据源的失败观察 SHALL 保持既有脱敏纪律，MUST NOT 泄露底层主机/账号等连接细节。

#### Scenario: 查询结果入 Result Store 并回传信封

- **WHEN** LLM 调用 `sql_execute` 执行一条 SELECT
- **THEN** 工具 SHALL 把结果集写入 Result Store 生成 `result_id`
- **AND** 回灌 LLM 的 ToolMessage SHALL 是结果信封而非原始行

#### Scenario: 后续工具凭 result_id 复用

- **WHEN** LLM 拿到 `result_id` 后调用分析/导出/渲染工具
- **THEN** 这些工具 SHALL 凭 `result_id` 从 Result Store 取回完整数据
- **AND** LLM SHALL NOT 需要在上下文中重述原始行

#### Scenario: 查询失败回灌可修复观察

- **WHEN** `sql_execute` 因列名/表名/语法等确定性错误失败
- **THEN** 回灌 LLM 的观察 SHALL 含 `error_kind` 与 `retriable`
- **AND** 对 `unknown_column`/`unknown_table` SHALL 附 schema-grounded `hint`
- **AND** MUST NOT 仅回灌裸 driver 错误文本

#### Scenario: 外部数据源失败观察保持脱敏

- **WHEN** 外部数据源查询失败并生成可修复观察
- **THEN** 观察 SHALL 仍遵循既有脱敏纪律
- **AND** MUST NOT 泄露底层主机/账号等连接细节
