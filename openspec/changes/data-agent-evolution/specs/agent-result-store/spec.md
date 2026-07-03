## ADDED Requirements

### Requirement: 结果信封契约

系统 SHALL 定义"结果信封"（result envelope）作为工具回灌 LLM 的标准结构，MUST 包含 `result_id`、列名列表、每列 dtype、`row_count`、不超过 N 行的样本行（sample rows）、以及关键聚合值（如 min/max/sum/count 视数据类型而定）。信封 MUST NOT 包含完整结果集的全部原始行。

#### Scenario: 大结果集只回传信封

- **WHEN** `sql_execute` 查询返回 5000 行
- **THEN** 工具 SHALL 把 5000 行完整写入 Result Store 并生成 `result_id`
- **AND** 回灌 LLM 的信封 SHALL 仅含列/ dtype / `row_count=5000` / 前 N 行样本 / 聚合值
- **AND** 信封的样本行数 SHALL NOT 超过配置的 N（默认不超过 20 行）

#### Scenario: 空结果集信封

- **WHEN** 查询返回 0 行
- **THEN** 信封 SHALL 包含 `row_count=0` 与列结构（若可得）
- **AND** SHALL 标注结果为空，供 LLM 据此答复

### Requirement: Redis 后端结果存储

Result Store SHALL 以 Redis 为后端持久化完整结果集，键为 `result_id`。每个结果 MUST 设置 TTL 以自动回收，且 MUST 与会话/请求关联以便审计与权限校验。系统 SHALL 提供按 `result_id` 读取完整结果集的接口，供分析、导出、渲染工具复用。

#### Scenario: 按 result_id 取回完整数据

- **WHEN** `data_export` 收到某 `result_id`
- **THEN** 工具 SHALL 从 Redis 按该键取回完整结果集
- **AND** 若键已过期或不存在，工具 SHALL 返回明确的"结果已过期/不存在"错误而非空导出

#### Scenario: 结果集按 TTL 过期

- **WHEN** 某 `result_id` 存活超过配置的 TTL
- **THEN** Redis SHALL 自动清除该结果
- **AND** 后续按该 `result_id` 的访问 SHALL 返回不存在错误

### Requirement: 跨会话隔离与归属校验

Result Store SHALL 记录每个 `result_id` 的归属会话/请求。工具按 `result_id` 读取时，系统 MUST 校验调用方与结果归属一致，禁止跨会话读取他人结果集。

#### Scenario: 拒绝跨会话读取

- **WHEN** 会话 B 尝试以属于会话 A 的 `result_id` 调用分析工具
- **THEN** Result Store SHALL 拒绝该读取并返回未授权错误
- **AND** 该拒绝 SHALL 记入操作审计

### Requirement: UI 快照独立于 Result Store TTL

Result Store SHALL 是数据的临时按引用后端，受 TTL 约束。为历史回放而随 UI 消息持久化的有界数据快照（见 [generative-ui-rendering](../generative-ui-rendering/spec.md)）MUST 独立于 Result Store 存储与 TTL，MUST NOT 因 `result_id` 过期而失效。Result Store MUST NOT 被用作 UI 历史的持久化后端。

#### Scenario: result_id 过期不影响 UI 快照

- **WHEN** 某 `result_id` 在 Result Store 中因 TTL 过期
- **THEN** 已随 UI 消息持久化的有界快照 SHALL 仍可用于历史重现
- **AND** Result Store 的过期回收 SHALL NOT 触及该快照
