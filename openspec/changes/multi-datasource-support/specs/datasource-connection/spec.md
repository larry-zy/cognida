# datasource-connection Specification

## ADDED Requirements

### Requirement: ConnectionManager 懒加载连接池

系统 SHALL 提供 ConnectionManager，按 `datasource_id` 惰性创建并缓存到外部数据源的 `database/sql` 连接池；首次使用前 MUST NOT 建立连接。对外部数据源 MUST 使用 `database/sql` 原生连接，MUST NOT 使用 GORM 或执行任何 schema 迁移。

#### Scenario: 首次使用才建连

- **WHEN** 某数据源注册后从未被查询
- **THEN** 系统 SHALL NOT 与其建立任何连接
- **AND** 首次以其 `database_id` 发起查询时才创建连接池并缓存

#### Scenario: 复用缓存连接池

- **WHEN** 同一数据源被连续多次查询
- **THEN** 后续查询 SHALL 复用已缓存的连接池而非重复建池

### Requirement: 保守池参数与空闲回收

外部数据源连接池 SHALL 使用保守参数（默认 `MaxOpenConns≤4`、`MaxIdleConns≤2`、`ConnMaxIdleTime≈5min`、`ConnMaxLifetime≈30min`，可配置），且长期未使用的缓存条目 SHALL 被回收关闭，避免占用用户数据库资源。

#### Scenario: 并发查询受池上限约束

- **WHEN** 对同一外部数据源同时发起超过 MaxOpenConns 的查询
- **THEN** 超出的查询 SHALL 等待空闲连接而非无限开新连接

#### Scenario: 长期空闲被回收

- **WHEN** 某数据源连接池超过回收阈值未被使用
- **THEN** 系统 SHALL 关闭该池并移出缓存
- **AND** 下次使用时按懒加载重新创建

### Requirement: 配置变更失效缓存

数据源配置（host/端口/账号/密码等）更新后，其已缓存的连接池 SHALL 失效并以新配置重建；实现 SHALL 以数据源的版本标识（如 `updated_at`）在取用时比对判断。

#### Scenario: 改密码后旧连接不再使用

- **WHEN** 用户更新了某数据源的密码
- **THEN** 下一次以该 `database_id` 的查询 SHALL 关闭旧连接池并用新凭证重建
- **AND** SHALL NOT 继续使用旧凭证的连接

### Requirement: driver Strategy 扩展点

连接与 schema 探查 SHALL 经 driver 接口（BuildDSN / Ping / ListTables / DescribeTable 等）实现，driver 按数据源 `type` 从工厂获取。Phase 1 SHALL 提供 MySQL driver；新增数据库类型 SHALL 仅需新增 driver 实现并注册，MUST NOT 修改 ConnectionManager 与上层消费方。

#### Scenario: 未支持的类型被明确拒绝

- **WHEN** 请求的数据源 `type` 没有已注册的 driver
- **THEN** 系统 SHALL 返回"暂不支持的数据源类型"错误而非崩溃或误连
