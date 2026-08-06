## ADDED Requirements

### Requirement: 字段级血缘抽取
系统 SHALL 从 `agent/metricsql/engine.go` 的编译产物抽取字段级血缘，建立血缘链：指标（Metric）→ 逻辑表/度量维度 → 物理表 → 物理列。血缘节点与边 SHALL 落入现有 `graph_*` 图存储后端，MUST 归属租户，重复抽取同一编译产物 MUST 幂等（同键 upsert）。

#### Scenario: 从指标查询编译产物建血缘
- **WHEN** metricsql 引擎完成一次 `Build(bundle, query)` 且血缘抽取开启
- **THEN** 系统为涉及的指标、逻辑表、物理表、物理列创建血缘节点，并建立指标→列的下钻边，节点与 Catalog 资产按物理坐标对齐

#### Scenario: 幂等重复抽取
- **WHEN** 同一 ModelBundle 的相同编译结果被再次抽取
- **THEN** 血缘图中不产生重复节点或重复边，仅更新时间戳

#### Scenario: 血缘对齐 Catalog 资产
- **WHEN** 血缘中的物理列能匹配到已同步的 `asset_column`
- **THEN** 血缘节点携带对应资产 id，使查询可回连资产详情；匹配不到时保留物理坐标并标记 `unresolved`

### Requirement: 血缘查询与影响分析
系统 SHALL 提供 `GET /api/v1/lineage/:asset` 返回指定资产（指标或物理表/列）的上游与下游血缘。查询 SHALL 支持方向（upstream/downstream/both）与深度限制，并 SHALL 提供影响分析：给定物理列变更，列出受影响的下游逻辑表与指标。

#### Scenario: 查询字段上下游
- **WHEN** 用户请求 `GET /api/v1/lineage/:asset?direction=both&depth=3`
- **THEN** 系统返回以该资产为中心、限定深度的血缘子图（节点+边），跨租户血缘不可见

#### Scenario: 列变更影响分析
- **WHEN** 用户请求某物理列的 downstream 影响分析
- **THEN** 系统返回依赖该列的全部下游逻辑表与受治理指标清单，供变更前评估

#### Scenario: 无血缘资产
- **WHEN** 查询一个尚无血缘记录的资产
- **THEN** 系统返回空血缘图并以 200 表示资产存在但无边，而非报错
