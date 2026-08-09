# 指标语义层：跑通治理主路（建模写入口 + 覆盖可观测 + 图谱接地） - Proposal

## Why

指标语义层的「引擎」与「实体」此前已落地并提交（metricsql 引擎、`agent_semantic_*` 模型、termgrounding 接地器），但整条治理主路是一具**空壳**——缺三段接缝，导致 `semantic_query` 恒返回 `covered=false`、每次回退词法 NL2SQL，语义层从未被真正行使：

1. **没有建模写入口**：`SemanticRepository.UpsertBundle` 仅测试可达，生产无任何路径把语义模型灌进库。库里没有生效模型 → `resolveBundle` 永远拿到空 → 直接回退。
2. **治理是否命中不可观测**：无从回答「语义层到底覆盖了多少提问、命中率多少、哪些名称未覆盖」，治理效果无法度量，回归也无抓手。
3. **接地只有模型内同义词、图谱层无数据**：`termgrounding` 的第二层（知识图谱 SIMILAR_TO 回落）端口已接线，但图谱里没有「业务口语 → 规范名」的桥接证据，口语词（如「流水」）无从解析到指标（「营收」）。

现在做，是因为语义层是「LLM 裸拼 SQL → 引擎按固定口径拼 SQL」的护城河地基；不补齐这三段接缝，后续的 Verified/Golden 缓存、口径治理、命中率优化都无处附着。

## What Changes

- **补建模写入口**：新增语义模型应用服务（`service/semantic`）+ REST 处理器（`handler/semantic_handler.go`），提供 List/Get/Create/Update/Publish/Deprecate；`Update/Publish/Deprecate` bump `Version` 以失效受信查询缓存。校验分两级：结构完整性 + 引用一致性（不校验表达式语义，表达式属受信管理员 SQL）。
- **补冷启动灌模**：`cmd/seed-semantic` 幂等灌入 tenant=1 的两套 active 模型（电商销售 grain=order、商品销售 grain=order_item），全 JOIN 为事实表出发的 to-one（扇出安全），让 `semantic_query` 立即可走治理主路。
- **补覆盖可观测**：新增 `CoverageSink/CoverageReporter/CoverageRepository`（`agent_semantic_coverage_logs` 表）；`semantic_query` 每次落一条覆盖埋点（`covered`/`cache_hit`/`fallback` + 未覆盖名称 + `request_id`，关联审计/Loki），best-effort 不阻断主路；新增 `GET /semantic-coverage` 按模型聚合命中率（`HitRatio=(covered+cache_hit)/total`）。
- **补图谱接地证据**：新增 `cmd/seed-graph-terms`，向 Neo4j 注入「业务口语 → 受治理规范名」的 SIMILAR_TO 桥接（tenant=1、kb_id=""，与运行时 `GraphAdapter` 命名空间一致），使模型内未登记的口语词经图谱第二层回落到指标/维度，`Source=graph` 可观测。
- **数据源路由硬化（收口接缝）**：`semantic_query` 生成的是裸物理 SQL，交 `sql_execute` 时依赖「会话已选定电商演示库数据源」这一隐式前置。将其显式化：语义模型/逻辑表携带数据源绑定并经 `SemanticQueryResult` 透传，避免选错库时静默打错数据源。

## Capabilities

### Modified Capabilities
- `agent-semantic-layer`: 新增受治理语义模型的建模写入口（REST + 冷启动 seed）与治理覆盖率可观测（埋点 + 按模型聚合）；细化知识图谱业务术语 grounding 为「模型内同义词 → 图谱 SIMILAR_TO 回落」的确定性两层接地，并补齐图谱层桥接数据的注入路径。

## Impact

- **cognida-go 新增**：`internal/service/semantic/`、`internal/handler/semantic_handler.go(+test)`、`internal/model/semantic/coverage.go`、`internal/repository/mysql/semantic_coverage_repo.go(+integration test)`、`cmd/seed-semantic/`、`cmd/seed-graph-terms/`、`internal/service/agent/termgrounding/graph_adapter_integration_test.go`。
- **cognida-go 改动**：`cmd/server/main.go`（注入 `CoverageSink`、接线 `TermGrounding` 图谱适配器）、`cmd/wire/{wire.go,wire_gen.go}`、`internal/handler/router/router.go`（`/semantic-models`、`/semantic-coverage` 路由）、`cmd/migrate-db/main.go`（注册覆盖埋点表）、`internal/service/agent/tools/{deps.go,init.go,semantic_query.go}`（注入 `CoverageSink`、接地器）。
- **数据/存储**：MySQL `link` 库新增 `agent_semantic_coverage_logs`（经 `cmd/migrate-db` 同步）；Neo4j 注入术语接地图谱（tenant=1、kb_id=""）。
- **非破坏面**：对外查询语义不变——命中语义层走治理口径 SQL，未覆盖仍回退词法 NL2SQL；覆盖埋点 best-effort，失败不影响取数主路。
- **范围外**：前端建模 UI（本变更只补后端治理主路，建模仍经 REST/seed）。
