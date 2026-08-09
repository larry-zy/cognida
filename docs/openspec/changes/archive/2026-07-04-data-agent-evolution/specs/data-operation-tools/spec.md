## ADDED Requirements

### Requirement: 写库工具（sql_mutate）

系统 SHALL 提供 `sql_mutate` 工具，支持 INSERT/UPDATE/DELETE（DML）。工具 MUST NOT 执行 DDL（CREATE/ALTER/DROP TABLE 等）。每次写入 MUST 携带 `idempotency_key` 保证幂等，MUST 先在事务内执行 dry-run 评估影响行数，且 MUST NOT 修改被列为红线的原始业务表。

#### Scenario: 拒绝 DDL 语句

- **WHEN** LLM 以 `sql_mutate` 提交 `ALTER TABLE orders ADD COLUMN ...`
- **THEN** 工具 SHALL 拒绝执行并返回"不支持 DDL"错误
- **AND** 该拒绝 SHALL 记入操作审计

#### Scenario: 事务内 dry-run 评估影响

- **WHEN** LLM 提交一条 `DELETE FROM agent_etl_tmp WHERE ...`
- **THEN** 工具 SHALL 在事务内先评估将影响的行数
- **AND** 若影响行数达到危险阈值，SHALL 触发人机确认协议而非直接提交

#### Scenario: 重复 idempotency_key 不重复写入

- **WHEN** 同一 `idempotency_key` 的写请求被重复提交
- **THEN** 工具 SHALL 识别为重复并返回首次结果
- **AND** SHALL NOT 二次落库

#### Scenario: 禁止修改红线原始表

- **WHEN** `sql_mutate` 目标为配置中列为红线的原始业务表
- **THEN** 工具 SHALL 拒绝执行并返回"禁止修改原始表"错误

### Requirement: ETL 派生工具（etl_run）

系统 SHALL 提供 `etl_run` 工具执行清洗/派生任务。派生产物 MUST 写入以 `agent_etl_` 为前缀的新对象（表/视图），MUST NOT 覆盖或修改任何原始业务表。工具 SHALL 支持从 `result_id` 或 SQL 作为输入源。

#### Scenario: 派生新表而非改源表

- **WHEN** 用户请求"把上月订单去重后另存一张干净表"
- **THEN** `etl_run` SHALL 创建 `agent_etl_orders_clean_*` 之类的新对象
- **AND** SHALL NOT 对原始 `orders` 表做任何写入

#### Scenario: 派生前缀强校验

- **WHEN** `etl_run` 的目标对象名不以 `agent_etl_` 前缀开头
- **THEN** 工具 SHALL 拒绝执行并返回前缀约束错误

### Requirement: 数据导出工具（data_export）

系统 SHALL 提供 `data_export` 工具，按 `result_id` 从 Result Store 取回完整结果集并导出为 CSV 或 Excel。导出 MUST 基于已存结果引用，MUST NOT 要求 LLM 在上下文中重述原始行。

#### Scenario: 按 result_id 导出 CSV

- **WHEN** LLM 以某 `result_id` 与 `format=csv` 调用 `data_export`
- **THEN** 工具 SHALL 从 Result Store 取回完整数据并产出 CSV 文件/下载引用
- **AND** 返回给 LLM 的 SHALL 是文件引用而非文件内容本身

### Requirement: 危险分级与人机确认协议

系统 SHALL 对操作工具划分危险分级：白名单安全操作（如导出、派生小表）自动执行；危险操作（大范围 DML、超阈值影响行数）MUST 先产出 `pending_action_id` 并暂停，等待前端确认卡片经携带 token 的 follow-up **resume** 后方可落库。

#### Scenario: 安全操作自动执行

- **WHEN** 操作被判定为白名单安全级（如 `data_export`）
- **THEN** 系统 SHALL 直接执行，无需人工确认

#### Scenario: 危险操作产出 pending_action_id 并等待确认

- **WHEN** 一条 `UPDATE` 影响行数超过危险阈值
- **THEN** 工具 SHALL 暂停执行并返回 `pending_action_id`
- **AND** 系统 SHALL 通过 UI 渲染确认卡片，仅在收到携带匹配 token 的 resume 请求后才提交事务

#### Scenario: 确认 token 不匹配则拒绝落库

- **WHEN** resume 请求携带的 token 与 `pending_action_id` 不匹配或已过期
- **THEN** 系统 SHALL 拒绝落库并使该 pending action 失效

### Requirement: 操作审计

系统 SHALL 新增 `agent_operation_audit` 表（GORM model，经 `migrate-db` 同步），记录所有写/ETL/导出以及被拦截的操作，MUST 包含操作类型、目标、SQL/参数、`idempotency_key`、结果（成功/拒绝/待确认）、会话与请求标识。

#### Scenario: 写操作留痕

- **WHEN** 任一 `sql_mutate`/`etl_run`/`data_export` 执行或被拒
- **THEN** 系统 SHALL 向 `agent_operation_audit` 写入一条记录
- **AND** 记录 SHALL 含结果状态与关联 `request_id`
