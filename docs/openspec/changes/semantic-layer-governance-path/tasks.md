# 指标语义层：跑通治理主路 - Tasks

> **目标**：把语义层从「空壳（恒回退）」变为「治理主路可行使、可观测、可接地」。
> 闸门：`go build ./...` + 相关单测/集成测试绿灯；治理命中经 `GET /semantic-coverage` 可见。

---

## Phase 1: 建模写入口（补进料线）

- [x] 1.1 新增 `service/semantic` 应用服务：Create/Update/Publish/Deprecate/List/Get，含结构 + 引用一致性校验（`ErrValidation`/`ErrNameConflict`）
- [x] 1.2 `Update/Publish/Deprecate` 调 `BumpVersion`，失效受信查询缓存（缓存键含模型版本）
- [x] 1.3 新增 `handler/semantic_handler.go`：REST 增删改发布 + 错误映射（not-found→404，校验/冲突→400，其余→500）
- [x] 1.4 `router.go` 挂 `/semantic-models`（GET/POST、`/:id` GET/PUT、`/:id/publish`、`/:id/deprecate`）
- [x] 1.5 wire 装配 `ProvideSemanticModelService`/`ProvideSemanticHandler`
- [x] 1.6 `cmd/seed-semantic` 幂等灌入 tenant=1 两套 active 模型（电商销售 / 商品销售，全 to-one 扇出安全）
- [x] 1.7 单测：`service/semantic`、`handler/semantic_handler_test.go` 绿灯

## Phase 2: 治理覆盖率可观测（补油表）

- [x] 2.1 定义覆盖端口 `model/semantic/coverage.go`：`CoverageOutcome`/`CoverageEvent`/`CoverageModelStat` + Sink/Reporter/Repository 接口
- [x] 2.2 `repository/mysql/semantic_coverage_repo.go`：`agent_semantic_coverage_logs` 表 + `Record`（截断 uncovered 到列宽）+ `Stats`（DB GROUP BY，应用端归并 per-model，维持 Total=三桶之和）
- [x] 2.3 `cmd/migrate-db` 注册 `SemanticCoverageLogModel`；`cmd/server` 注入 `CoverageSink`
- [x] 2.4 `semantic_query` 各出口埋点：nil bundle→`fallback`、命中缓存→`cache_hit`、引擎命中→`covered`（并写受信缓存）、非覆盖→`fallback`(+uncovered)；best-effort 吞错
- [x] 2.5 `GET /semantic-coverage` 按模型聚合命中率；`ErrCoverageDisabled` 时返回 `{items:[],enabled:false}`
- [x] 2.6 集成测试：`semantic_coverage_integration_test.go`、`semantic_seed_integration_test.go`（真实库）

## Phase 3: 图谱接地（补第二层证据）

- [x] 3.1 `cmd/server` 接线 `TermGrounding = NewGraphAdapter(graphRepo, "")`；Neo4j 不可用时端口为 nil，接地退化为仅模型内同义词
- [x] 3.2 `cmd/seed-graph-terms` 注入「口语词 → 规范名」SIMILAR_TO 桥接（tenant=1、kb_id=""，与运行时命名空间一致；10 条桥接）
- [x] 3.3 集成测试 `graph_adapter_integration_test.go`：真实 Neo4j 验证「流水」经图谱回落「营收」，`Source=graph`、`Via` 记录经由别名（**已跑通 PASS**）

## Phase 4: 数据源路由硬化（收口最后一段接缝）

- [x] 4.1 在 `LogicalTable.DatabaseID` 写入电商演示库数据源 ID（`ecommerceDatasourceID`，可 env 覆盖），`cmd/seed-semantic` 重灌并核验 5 张逻辑表均绑定
- [x] 4.2 `SemanticQueryResult` 增 `database_id` 字段，`semanticQuery` 经 `bundleDatabaseID(bundle)` 从逻辑表推导并在 covered / cache_hit 两条出口透传
- [x] 4.3 更新 semantic_query 工具 prompt：`covered=true` 且 `database_id` 非空时必须原样传给 `sql_execute`（而非依赖会话隐式选库）
- [x] 4.4 单测：`bundleDatabaseID`（单库/全空/部分一致/跨库不一致）+ covered/cache_hit 透传 + 未绑定旧模型回落空值（向后兼容）

## Phase 5: 端到端验证 + 收尾

> 采用**确定性真实库端到端集成测试**替代人工起服务点按（更可复现、可回归、不依赖 LLM 抽样）：
> `semantic_e2e_integration_test.go::TestSemanticGovernanceE2E` 打通「seed 模型 → semanticQuery 命中治理口径直出 SQL + 数据源绑定 → 经真实 ConnectionManager 打向 ecommerce_demo 执行 → 取真实行 → 覆盖埋点落表且可聚合读回」整条接缝。

- [x] 5.1 端到端夹具：`agentctx` 注入 tenant=1 会话 + 真实 `SemanticRepository`/`ConnectionManager`（`DATASOURCE_SECRET_KEY` 解密数据源凭证），前置缺失则 `t.Skip`
- [x] 5.2 「按城市看营收」→ 断言 `covered=true` + 治理 SQL（`SUM(orders.pay_amount)`，非回退），经 `sql_execute(database_id=res.DatabaseID)` 打真实库取到 10 行（列含 城市/营收）
- [x] 5.3 覆盖可观测：`covered` 埋点落 `agent_semantic_coverage_logs`，`Stats` 聚合断言电商销售模型 `covered≥1` 且 `Total=三桶之和`、`HitRatio` 恒等；越界口径「毛利率」断言 `fallback`+uncovered（`cache_hit` 透传路径由 `semantic_query_test.go` 单测覆盖）
- [x] 5.4 图谱接地：`termgrounding::TestGraphGroundingIntegration` 真实 Neo4j 验证口语词「流水」经图谱回落「营收」，`Source=graph`、`Via` 记录经由别名
- [x] 5.5 `go build ./...` OK + 单测（agent/tools·metricsql·termgrounding·semanticcache·semantic·handler）全绿 + 集成测试（tools E2E·termgrounding·mysql 覆盖/seed 五例）全 PASS；本会话未起常驻服务进程，无需清理
