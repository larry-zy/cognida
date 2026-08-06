## 1. 数据模型与迁移 (Catalog)

- [ ] 1.1 新增 `internal/model/catalog/entity.go`：`AssetTable`（租户、datasource_id、schema、table_name、type、row_count、last_synced_at、deprecated）、`AssetColumn`（外键 table、col_name、data_type、nullable、is_pk/is_fk、null_rate、distinct_count、sample_values、profiled、deprecated）、`BusinessGlossary`（term、synonyms、关联 asset）
- [ ] 1.2 定义 catalog 领域接口 `internal/model/catalog/repository.go`（Repository 契约）与 `errors.go`
- [ ] 1.3 在 `internal/repository/mysql/` 实现 `asset_table_repo.go` / `asset_column_repo.go` / `glossary_repo.go`（cursor 分页、租户过滤、按物理坐标 upsert、tag/关键字/术语检索）
- [ ] 1.4 运行 `cd link-go && set -a && source .env && set +a && go run ./cmd/migrate-db` 幂等同步业务表，确认表结构

## 2. 元数据同步与 Profiling (metadata-sync)

- [ ] 2.1 新增 `internal/service/catalog/sync.go`：复用 `datasource/driver.go` 的 `ListTables/DescribeTable` 抽取 schema，按物理坐标幂等 upsert 资产，源端删列做软删除
- [ ] 2.2 新增 `internal/service/catalog/profiler.go`：复用 `datasource/sampler.go` 采样计算空值率/基数/样本值，失败降级不阻断结构落库，采样规模有上限
- [ ] 2.3 同步方法与 HTTP 触发解耦（预留可复用 service 方法供后续调度），同步写入 audit（触发人/datasource/变更摘要）
- [ ] 2.4 单元测试：幂等重复同步无重复行、结构漂移软删除、profiling 失败降级

## 3. Catalog Service 与 API

- [ ] 3.1 新增 `internal/service/catalog/service.go`：List/Get/Search 资产、更新治理属性（owner/tag/分级，纳入审计）、glossary CRUD 与关联
- [ ] 3.2 新增 `internal/handler/catalog_handler.go` + `setupCatalogRoutes`：`GET /api/v1/catalog/tables`（列表/搜索）、`GET /:id`、`PUT /:id`（治理属性）、`POST /catalog/sync`、glossary 路由；认证+租户中间件
- [ ] 3.3 在 `internal/handler/router/router.go` 注册 catalog 路由；wire（`cmd/wire/wire.go` + `wire_gen.go`）接线 catalog service/handler
- [ ] 3.4 handler/service 单元测试 + 租户隔离用例（跨租户资产不可见）

## 4. 血缘模型与图存储 (data-lineage)

- [ ] 4.1 新增 `internal/model/lineage/`：血缘节点（Metric/LogicalTable/PhysicalTable/PhysicalColumn）与边类型，独立 `lineage_*` label 命名空间，携带租户与 asset_id 对齐字段
- [ ] 4.2 在 lineage repository 实现图存储读写：ensureSchema 懒加载血缘 schema，节点/边按物理坐标幂等 upsert，与知识图谱命名空间隔离
- [ ] 4.3 血缘节点与 `asset_column` 按物理坐标对齐回填 asset_id，匹配不到标记 `unresolved`

## 5. 血缘抽取与查询

- [ ] 5.1 在 `agent/metricsql/engine.go` 的 `Build` 产出处接入可选血缘 extractor 回调（默认关闭、异步执行、不阻塞主查询）
- [ ] 5.2 新增 `internal/service/lineage/extractor.go`：从编译产物抽取 指标→逻辑表→物理表→列 血缘链并幂等落图
- [ ] 5.3 新增 `internal/service/lineage/query.go`：上下游遍历（direction/depth）、影响分析（给定列列出下游逻辑表+指标）
- [ ] 5.4 新增 `internal/handler/lineage_handler.go`：`GET /api/v1/lineage/:asset`（direction/depth）、影响分析、`POST /lineage/rebuild`（存量模型补建）；router + wire 接线
- [ ] 5.5 单元测试：幂等重复抽取、无血缘资产返回空图 200、影响分析正确性、跨租户隔离

## 6. 反哺 Agent（接线）

- [ ] 6.1 将 Catalog 资产/术语作为选表选字段检索上下文，接入 Text2SQL / `semantic_query`（agent tools 消费侧，不改其既有 spec）
- [ ] 6.2 集成测试：真实 MySQL/PG 数据源跑通「同步→检索→Text2SQL 命中资产」链路

## 7. 集成测试与验收

- [ ] 7.1 集成测试（真实库，`-tags=integration`）：catalog 同步幂等 + profiling、glossary 检索命中
- [ ] 7.2 集成测试：metricsql Build → 血缘落图 → `GET /lineage/:asset` 上下游 + 影响分析
- [ ] 7.3 API 测试：catalog 与 lineage 全部路由的认证/租户/分页/错误码
- [ ] 7.4 触发 code-review skill 修复问题；任务完成后终止所有开启的服务进程
