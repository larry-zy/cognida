## Context

"智能数据中台"已有 datasource（MySQL/PG，含 sampler/driver）、semantic 语义层（semantic-models + `agent/metricsql/engine.go` 编译引擎）、知识图谱（`graph_*` 存储后端，由 `graphMetaRepository.ensureSchema` 懒加载建表）、quality、audit、租户隔离。但数据资产不落库、无血缘，Agent 选表选字段靠现场探查。

本变更新增数据资产层 Phase 1：Catalog（元数据中心）+ Lineage（字段级血缘）。约束：遵循 `handler → service → model ← repository` 分层；业务表走 GORM model + `cmd/migrate-db` 幂等同步（评测/业务表主流程无手写 SQL 迁移）；图谱表由内部 model 懒加载，不进 migrate-db；全链路 request_id + audit；租户隔离。

## Goals / Non-Goals

**Goals:**
- 把 datasource schema 探查沉淀为可搜索、带治理属性（owner/tag/分级）与 profiling 的持久资产。
- 复用 sampler 做列级 profiling，采样而非全表扫描。
- 从 metricsql 编译产物抽字段级血缘，复用 `graph_*` 图存储落地，提供上下游 + 影响分析查询。
- Catalog 资产可作为选表选字段上下文喂给 Text2SQL / semantic_query（接线层，不改其 spec）。

**Non-Goals:**
- 不做定时调度/DAG（仅预留可复用 service 方法，HTTP 手动触发；调度归后续 Phase）。
- 不做跨源联邦查询、不新增 datasource driver（属 Phase 3）。
- 不做指标对外取数 API（属 Phase 2 指标平台服务化）。
- 不做列级权限/脱敏策略中心（属 Phase 4 治理闭环）；本期数据分级仅作资产标注，不做强制访问控制。

## Decisions

### D1: 血缘落"图存储"而非关系表
选择复用现有 `graph_*` 图存储后端存血缘节点/边，而非新建 MySQL 邻接表。理由：血缘本质是有向图，上下游遍历/影响分析是图查询，图后端天然胜任；且团队已有 `graph.go` + `ensureSchema` 懒加载模式可复用，避免重复造递归 CTE。
- **Alternatives**：(a) MySQL 自关联 `lineage_edge` 表 + 递归 CTE——深度遍历性能差、代码复杂；(b) Neo4j 独立库——存储约定里图谱归 Neo4j，但为血缘单开实例过重。→ 复用现有图存储后端，血缘用独立节点/边 label（`lineage_*`）与知识图谱隔离命名空间。

### D2: Catalog 资产落 MySQL 业务表，走 migrate-db
`asset_table` / `asset_column` / `business_glossary` 作为元数据/配置类数据，按存储约定归 MySQL，用 GORM model + `cmd/migrate-db` 幂等同步。资产与图存储中的血缘节点通过"物理坐标（datasource_id + schema + table + column）"对齐，血缘节点携带 asset_id 反连。
- **Alternatives**：全放图存储——搜索/分页/租户过滤等关系型查询不便。→ 资产在 MySQL，血缘在图，用物理坐标桥接。

### D3: 血缘抽取采用"编译产物钩子 + 显式触发"双通道
在 `metricsql/engine.go` 的 `Build` 产出处提供一个可选的血缘抽取回调（extractor），运行时可开关；同时提供显式 `POST /api/v1/lineage/rebuild` 对已发布 semantic-models 批量重建血缘。理由：既能实时增量捕获，又能对存量模型补建，且回调关闭时对 metricsql 零影响。
- **Alternatives**：仅离线批量解析 SQL 文本——需重新解析 SQL、丢失 bundle 结构信息。→ 直接吃编译产物（已含指标/维度/表映射），信息最全。

### D4: Profiling 复用 sampler、失败降级
列 profiling 走 `datasource/sampler.go` 采样，计算空值率/基数/样本值，采样规模有上限；任何列/源失败只记告警不阻断结构落库。理由：保证资产可用性优先于统计完整性；避免对生产库压测。

### D5: 同步幂等 + 软删除保血缘
同步以物理坐标为幂等键 upsert；源端删列时资产标记 `deprecated` 软删除而非物理删除，避免悬挂血缘引用。

## Risks / Trade-offs

- **图存储被血缘与知识图谱共用产生耦合/命名冲突** → 血缘用独立 label 前缀（`lineage_*`）与租户维度隔离；ensureSchema 时确保血缘 schema 独立初始化。
- **profiling 采样对生产库产生负载** → 采样规模上限 + 复用现有 sampler 的限流；profiling 默认关闭，`profile=true` 显式开启。
- **血缘抽取回调拖慢 metricsql 主链路** → extractor 异步执行、开关可关；主查询路径不等待血缘落库。
- **Catalog 与真实 schema 漂移（同步不及时导致资产过时）** → 资产带 last_synced_at，检索结果暴露时效；预留定时刷新的 service 方法。
- **血缘节点与 Catalog 资产对齐失败（unresolved）** → 保留物理坐标 + `unresolved` 标记，同步后可回填，不丢边。

## Migration Plan

1. 新增 GORM model（catalog/lineage）→ `cd cognida-go && set -a && source .env && set +a && go run ./cmd/migrate-db` 幂等建业务表。
2. 图存储血缘 schema 由 lineage repository 的 ensureSchema 懒加载首次调用时建立。
3. 分层落地：model → repository → service（catalog/sync/profiler、lineage extractor/query）→ handler → router 注册（`/api/v1/catalog/*`、`/api/v1/lineage/*`，认证+租户中间件）→ wire 接线。
4. metricsql extractor 回调默认关闭，验证无误后开启；存量模型经 `rebuild` 补建。
5. **回滚**：新增能力可整体摘除路由与 wire 注入即停用；业务表与图 label 为纯新增，不影响既有流程；无破坏性变更。

## Open Questions

- 数据分级枚举取值是否需与后续 guardrail 脱敏策略统一编码？（Phase 4 前先用独立枚举，留映射位。）
- 血缘是否需纳入非 semantic 来源（如直接 `sql_execute` 的 Ad-hoc SQL）？本期只覆盖 metricsql 治理链路，Ad-hoc 血缘留待后续。
- Catalog 喂给 Agent 的检索接口形态（工具 vs 直接注入 context）留待实现期与 agent-tools 现状对齐。
