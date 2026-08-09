# data-agent Delta Specification

## ADDED Requirements

### Requirement: 会话数据源上下文

Data Agent 会话 SHALL 支持携带可选的数据源上下文（`datasource_id`）：指定后，本会话内查询类工具（`get_schema`/`sql_execute`）的默认 `database_id` SHALL 为该数据源；未指定时保持现状（当前业务库）。指定外部数据源的会话中，操作类能力（`sql_mutate`/`etl_run`）MUST NOT 作用于该外部数据源。

#### Scenario: 会话级数据源默认值

- **WHEN** 用户在会话中选定某外部数据源后提问"各区域销售额是多少"
- **THEN** Agent 的 `get_schema`/`sql_execute` 调用 SHALL 默认携带该数据源的 `database_id`
- **AND** 查询 SHALL 在该外部数据源上执行

#### Scenario: 未指定数据源保持现状

- **WHEN** 会话未携带任何数据源上下文
- **THEN** 查询类工具 SHALL 在当前业务库执行，行为与本变更前一致

#### Scenario: 外部数据源会话屏蔽写操作

- **WHEN** 会话选定外部数据源且 LLM 尝试对其执行写操作
- **THEN** 系统 SHALL 拒绝该调用并回传只读约束的合成错误 ToolMessage
- **AND** 写语句 SHALL NOT 触达外部数据源
