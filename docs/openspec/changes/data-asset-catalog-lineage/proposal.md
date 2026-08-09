## Why

当前"智能数据中台"缺失数据资产层的两根承重柱：**元数据目录（Catalog）**与**数据血缘（Lineage）**。`datasource` 只在请求时临时做 `ListTables/DescribeTable` schema 探查，元数据不落库、不可搜索、无业务语义；`graph_*` 是文档知识图谱而非数据资产血缘。结果是 Text2SQL / `semantic_query` 选表选字段全靠模型现场探查，准确率和可解释性受限。沉淀资产 + 建立字段级血缘，能直接反哺 Agent 取数、并为后续指标平台与治理闭环打地基。

## What Changes

- 新增 **Catalog 元数据中心**：把 datasource 的 schema 探查沉淀为持久化资产（`asset_table` / `asset_column`），承载 owner、tag、数据分级、采样/profiling 统计与业务术语（glossary）；提供资产搜索/发现 API。
- 新增**元数据同步任务**：按数据源幂等抽取表/列结构，复用 `datasource/sampler.go` 做列级 profiling（行数、空值率、基数、样本值），支持增量刷新。
- 新增**字段级血缘 Lineage**：从 `agent/metricsql/engine.go` 编译产物抽取血缘链（指标 → 逻辑表 → 物理表 → 列），**复用现有 `graph_*` 图存储后端**落血缘节点与边，避免另起存储。
- 新增**血缘查询 API** `GET /api/v1/lineage/:asset`：返回资产上下游 + 影响分析（impact analysis）。
- **反哺 Agent**：Catalog 资产与业务术语作为选表选字段的检索上下文，喂给 Text2SQL / `semantic_query`，提升取数准确率。
- 全部新增能力遵循现有租户隔离、审计中间件与 handler→service→repository 分层。

## Capabilities

### New Capabilities
- `data-catalog`: 数据资产元数据中心——表/列资产持久化、owner/tag/分级、列级 profiling、业务术语表，以及资产搜索/发现 API。
- `metadata-sync`: 元数据同步——按数据源幂等抽取 schema 并做列级 profiling 的任务能力（含增量刷新）。
- `data-lineage`: 数据血缘——从 metricsql 编译产物抽取字段级血缘、落图存储、提供上下游与影响分析查询。

### Modified Capabilities
<!-- Catalog 资产作为检索上下文喂给 agent-tools 属于消费侧集成，通过任务在实现层接线，不改动 agent-tools/agent-semantic-layer 的既有 spec 级需求，故此处留空。 -->

## Impact

- **新增代码**：`internal/service/catalog/`（service + sync + profiler）、`internal/service/lineage/`（extractor + query）、`internal/model/catalog/`、`internal/model/lineage/`、`internal/repository/mysql/asset_*_repo.go`、`internal/handler/catalog_handler.go`、`internal/handler/lineage_handler.go`。
- **复用/接线**：`datasource/sampler.go`（profiling 取样）、`datasource/driver.go`（schema 探查）、`agent/metricsql/engine.go`（血缘抽取钩子）、`graph_*` 图存储后端（血缘落地）、`agent/tools`（Catalog 作为选表上下文）。
- **数据库**：新增 `asset_table` / `asset_column` / `business_glossary` 等业务表，通过 `cmd/migrate-db` 从 GORM model 幂等同步；血缘边走图存储 `ensureSchema` 懒加载。
- **API**：新增 `/api/v1/catalog/*`（认证+租户）与 `/api/v1/lineage/*`（认证+租户）路由，注册进 `router.go`。
- **迁移/兼容**：纯新增，无破坏性变更；datasource 既有 `ListTables/DescribeTable` 保留，Catalog 在其之上叠加持久层。
