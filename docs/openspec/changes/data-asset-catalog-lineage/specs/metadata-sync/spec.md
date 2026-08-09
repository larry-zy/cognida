## ADDED Requirements

### Requirement: 幂等元数据同步
系统 SHALL 提供按数据源抽取表/列结构并写入 Catalog 资产的同步能力。同步 MUST 幂等：重复同步同一数据源仅更新变化项，不产生重复资产。同步 SHALL 复用 `datasource/driver.go` 的 `ListTables/DescribeTable` 探查 schema。

#### Scenario: 首次同步数据源
- **WHEN** 授权用户 `POST /api/v1/catalog/sync` 指定 datasource_id
- **THEN** 系统抽取该源全部表与列结构，创建对应 `asset_table`/`asset_column`，并返回本次新增/更新/删除的资产数量

#### Scenario: 重复同步幂等
- **WHEN** 对同一数据源在结构未变时再次触发同步
- **THEN** 资产总数不变，仅更新 last_synced_at，无重复行

#### Scenario: 结构漂移处理
- **WHEN** 源表新增列、删除列或改类型后再次同步
- **THEN** 系统新增/软删除/更新对应列资产，软删除的列保留历史但标记 `deprecated`，不物理删除以保全血缘引用

### Requirement: 列级 Profiling 集成
同步流程 SHALL 可选地对列做 profiling，复用 `datasource/sampler.go` 采样计算空值率、去重基数与样本值，写入列资产。Profiling MUST 基于采样而非全表扫描，且失败不阻断结构同步。

#### Scenario: 同步带 profiling
- **WHEN** 触发同步时传入 `profile=true`
- **THEN** 系统对每列基于采样写入 profiling 统计，并记录采样规模

#### Scenario: profiling 失败降级
- **WHEN** 某列采样失败
- **THEN** 该列结构照常落库、profiling 留空，同步整体成功并在结果中列出告警

### Requirement: 增量刷新与触发方式
系统 SHALL 支持手动触发同步，并 SHALL 为后续定时调度预留可复用的服务方法（业务逻辑与 HTTP 触发解耦）。同步过程 MUST 纳入审计，记录触发人、数据源与变更摘要。

#### Scenario: 同步纳入审计
- **WHEN** 用户触发一次同步
- **THEN** 审计日志记录 request_id、租户、datasource_id 与本次变更摘要（新增/更新/删除数）
