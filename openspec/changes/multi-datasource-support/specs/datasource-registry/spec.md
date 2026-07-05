# datasource-registry Specification

## ADDED Requirements

### Requirement: 数据源 CRUD

系统 SHALL 提供数据源的注册、查询、编辑、删除 API（`/datasources`）。数据源 MUST 至少包含名称、类型（Phase 1 仅 `mysql`）、host、port、库名、用户名、密码及可选 `extra` 参数；`name` MUST 唯一。删除数据源时 SHALL 同时失效其连接缓存。

#### Scenario: 注册新数据源

- **WHEN** 用户提交合法的数据源配置（类型 mysql、含主机/端口/库名/账号密码）
- **THEN** 系统 SHALL 持久化该数据源并返回其 `id`
- **AND** 响应 SHALL NOT 包含任何密码字段

#### Scenario: 名称重复被拒

- **WHEN** 用户注册与现有数据源同名的数据源
- **THEN** 系统 SHALL 返回明确的唯一性冲突错误

#### Scenario: 删除数据源后连接失效

- **WHEN** 用户删除某数据源
- **THEN** 系统 SHALL 删除其元数据并关闭/移除对应的缓存连接池
- **AND** 后续以该 `database_id` 的查询 SHALL 返回"数据源不存在"错误

### Requirement: 凭证加密存储与不回显

数据源密码 SHALL 以 AES-GCM 加密后存储（密钥来自 `DATASOURCE_SECRET_KEY` 环境变量），MUST NOT 明文落库。所有 API 响应 MUST NOT 回显密码（含密文）。编辑数据源时密码字段留空 SHALL 表示"不修改原密码"。

#### Scenario: 密码加密落库

- **WHEN** 数据源创建或密码更新
- **THEN** 数据库中 SHALL 仅存在加密后的密码列
- **AND** 明文密码 SHALL NOT 出现在数据库、日志或 API 响应中

#### Scenario: 编辑时留空不改密码

- **WHEN** 用户编辑数据源且密码字段为空
- **THEN** 系统 SHALL 保留原加密密码不变

#### Scenario: 解密失败可恢复

- **WHEN** 因密钥变更导致某数据源密码解密失败
- **THEN** 系统 SHALL 将该数据源标记为需重新录入凭证并返回明确错误
- **AND** SHALL NOT 导致进程崩溃或影响其他数据源

### Requirement: 测试连接

系统 SHALL 提供"测试连接"接口：按给定配置（或已存数据源 id）建立临时连接执行 ping，在超时内返回成功或失败原因。测试连接 MUST NOT 写入连接缓存。

#### Scenario: 保存前验证配置

- **WHEN** 用户在保存前用表单配置调用测试连接
- **THEN** 系统 SHALL 用该配置临时建连并 ping
- **AND** 成功返回可连接，失败返回脱敏后的原因（不含 DSN/密码）

#### Scenario: 目标不可达时限时失败

- **WHEN** 目标数据库不可达
- **THEN** 测试连接 SHALL 在配置的超时内返回失败而非无限阻塞

### Requirement: 外部数据源 schema 探查

系统 SHALL 提供对已注册数据源的 schema 探查能力：列出表清单及指定表的列/类型结构，供前端浏览与 text2sql 选表使用。探查 SHALL 经由对应 driver 的 `information_schema` 查询实现。

#### Scenario: 列出数据源的表

- **WHEN** 用户请求某数据源的表清单
- **THEN** 系统 SHALL 返回该数据源库内的表名（及注释，若有）

#### Scenario: 查看指定表结构

- **WHEN** 用户请求某数据源下指定表的结构
- **THEN** 系统 SHALL 返回该表的列名/类型/可空性/键信息
