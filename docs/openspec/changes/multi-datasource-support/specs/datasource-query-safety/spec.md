# datasource-query-safety Specification

## ADDED Requirements

### Requirement: 外部数据源只读拦截

对外部数据源执行的 SQL SHALL 经语句类型校验，仅允许只读查询（SELECT，含 CTE 形式）；`INSERT/UPDATE/DELETE/REPLACE/DDL/SET/USE/GRANT` 等 MUST 被拒绝。语句解析失败时 SHALL 默认拒绝执行（fail-closed）。

#### Scenario: 写语句被拒

- **WHEN** 对外部数据源提交 `UPDATE users SET ...`
- **THEN** 系统 SHALL 拒绝执行并返回"外部数据源仅支持只读查询"错误
- **AND** 该语句 SHALL NOT 触达目标数据库

#### Scenario: 解析失败默认拒绝

- **WHEN** 提交的 SQL 无法被解析器识别语句类型
- **THEN** 系统 SHALL 拒绝执行而非放行

#### Scenario: 普通 SELECT 放行

- **WHEN** 对外部数据源提交合法 SELECT（含 WITH ... SELECT）
- **THEN** 系统 SHALL 放行执行

### Requirement: 强制行数上限

外部数据源查询 SHALL 受强制行数上限约束（沿用现有 1000 行保护）：无 LIMIT 或 LIMIT 超上限的查询 SHALL 被改写为上限值或拒绝，MUST NOT 无界拉取。

#### Scenario: 无 LIMIT 查询被限量

- **WHEN** 提交不含 LIMIT 的 SELECT
- **THEN** 实际执行的查询 SHALL 附带不超过上限的 LIMIT

### Requirement: 查询超时与错误脱敏

外部数据源的每次查询 SHALL 携带超时上下文（默认 30s，可配置），超时即取消。回传给调用方/LLM 的错误信息 MUST NOT 包含 DSN、密码或完整连接串。

#### Scenario: 慢查询被超时取消

- **WHEN** 某外部数据源查询超过配置的超时时间
- **THEN** 系统 SHALL 取消该查询并返回超时错误
- **AND** SHALL NOT 无限占用连接

#### Scenario: 错误信息不泄露凭证

- **WHEN** 连接或查询外部数据源失败
- **THEN** 返回的错误消息 SHALL 经过脱敏，不含密码/DSN
