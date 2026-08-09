## ADDED Requirements

### Requirement: 策略级写审批复用待确认机制

系统 SHALL 支持在既有「取值级危险确认」（如 `sql_mutate` 按影响行数阈值触发的 `pending_confirm`）之上，叠加**策略级写审批**：当会话或 skill 策略把某写/导出/ETL 工具（`sql_mutate`/`data_export`/`etl_run`）标记为需审批时，系统 SHALL 在执行前生成待确认动作。策略级审批 MUST 复用同一套待确认存储、`ConfirmToken` 与前端确认卡片机制，MUST NOT 另造并行审批通道；人工确认后 SHALL 经既有确认恢复路径执行。简单只读操作与未被标记审批的写操作行为 SHALL 不变。

#### Scenario: 标记审批的写操作先生成待确认动作

- **WHEN** 会话把 `sql_mutate` 标记为需审批，Agent 触发一次写入
- **THEN** 系统 SHALL 在执行前生成 `pending_action` 与 `ConfirmToken`
- **AND** SHALL 复用既有确认卡片而非新建审批通道

#### Scenario: 确认后经既有恢复路径执行

- **WHEN** 用户对策略级审批的待确认动作点确认
- **THEN** 系统 SHALL 经既有确认恢复路径执行原写操作

#### Scenario: 导出与 ETL 同样支持策略级审批

- **WHEN** `data_export` 或 `etl_run` 被标记为需审批
- **THEN** 系统 SHALL 同样生成 `pending_action` 而非直接执行

#### Scenario: 未标记审批的写操作行为不变

- **WHEN** 某写操作未被标记审批且未触及危险行数阈值
- **THEN** 系统 SHALL 直接执行，行为与接入审批前一致
