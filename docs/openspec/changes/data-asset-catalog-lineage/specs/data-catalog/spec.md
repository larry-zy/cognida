## ADDED Requirements

### Requirement: 表资产持久化与检索
系统 SHALL 将数据源的表结构沉淀为持久化表资产（`asset_table`），每条资产 MUST 归属单一租户，并记录物理坐标（datasource_id、schema、table_name）、类型（table/view）、行数估算、最近同步时间。系统 SHALL 提供按租户隔离的资产列表、详情与关键字搜索能力。

#### Scenario: 列出某数据源的表资产
- **WHEN** 认证用户请求 `GET /api/v1/catalog/tables?datasource_id=<id>`
- **THEN** 系统返回该租户下、该数据源已同步的表资产分页列表（cursor 分页），跨租户资产不可见

#### Scenario: 关键字搜索表资产
- **WHEN** 用户请求 `GET /api/v1/catalog/tables?q=order`
- **THEN** 系统在当前租户范围内按表名、备注、业务术语匹配返回资产，并高亮命中来源

#### Scenario: 查看表资产详情
- **WHEN** 用户请求 `GET /api/v1/catalog/tables/:id`
- **THEN** 系统返回该表资产及其列资产列表、owner、tag、数据分级与最近 profiling 统计

### Requirement: 列资产与 Profiling 统计
系统 SHALL 为每张表持久化列资产（`asset_column`），记录列名、数据类型、是否可空、主键/外键标记，以及列级 profiling 统计（空值率、去重基数、样本值）。Profiling 统计 SHALL 复用 `datasource/sampler.go` 的采样结果，不得对生产表做全表扫描。

#### Scenario: 展示列 profiling
- **WHEN** 用户查看某表资产详情
- **THEN** 每个列资产返回其类型、可空性与最近一次 profiling 的空值率、基数和样本值；若尚未 profiling 则相应字段为空并标记 `profiled=false`

#### Scenario: 采样受限不阻塞
- **WHEN** 某数据源不可达或采样超时
- **THEN** 列资产仍按 schema 结构落库，profiling 字段留空且记录同步告警，不影响资产可检索

### Requirement: 资产治理属性
系统 SHALL 支持为表/列资产维护治理属性：owner、tag（多值）、数据分级（如 public/internal/confidential/secret）。这些属性 SHALL 可被更新且纳入审计。

#### Scenario: 设置资产 owner 与分级
- **WHEN** 授权用户 `PUT /api/v1/catalog/tables/:id` 更新 owner 与数据分级
- **THEN** 系统持久化变更、写入审计日志，并在后续检索结果中反映新属性

#### Scenario: 按 tag 过滤资产
- **WHEN** 用户请求 `GET /api/v1/catalog/tables?tag=core`
- **THEN** 系统仅返回携带该 tag 的资产

### Requirement: 业务术语表（Glossary）
系统 SHALL 提供业务术语表能力，允许定义业务术语及其同义词，并将术语关联到具体表/列资产。术语 SHALL 参与资产检索匹配，作为 NL→资产的桥梁。

#### Scenario: 术语关联字段
- **WHEN** 授权用户创建术语「客单价」并关联到某列资产
- **THEN** 之后以「客单价」搜索资产时，该列资产命中并标注命中来源为 glossary

#### Scenario: 术语同义词命中
- **WHEN** 术语「GMV」配置了同义词「成交额」，用户以「成交额」搜索
- **THEN** 系统返回关联「GMV」的资产
