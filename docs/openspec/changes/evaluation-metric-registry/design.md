## Context

评测主流程当前经 Go worker → Python `/api/v1/evaluation/compute-metrics`。该端点用写死的 if 链识别指标名并返回固定字段的 `ComputeItemResult`;而 `graders/registry.py`(装饰器注册、自动发现、`list_graders`)是理想插件架构却未接入该路径。结果载体在 Python→Go(`types.go`)→MySQL(固定列)→前端(固定类型)全链路固定,新增一个指标要改约 6 处。前端目录 `evaluation-config.ts` 写死并已与后端漂移(`llm_factual`/`llm_safety` 被静默忽略)。

约束:单人开发直接在 `main` 上做;评测表**无 AutoMigrate**,加列需手动 ALTER TABLE;已在主线修复的 bug(语义相似度批量、`llm_reasoning` 解析、零值聚合保留、重试重建请求)不得回退;每阶段可独立提交且不破坏现有功能。

## Goals / Non-Goals

**Goals:**
- 注册表成为"可用指标"的唯一事实来源,每个 grader 携带元数据(name、label、group、requires_reference、requires_contexts、eval_types)。
- `/compute-metrics` 遍历注册表执行,删除写死 if 链。
- 结果动态化:Python `scores{name:value}` → Go `map[string]float64` → MySQL JSON 列 → 前端动态渲染,**同时保留固定字段兼容**。
- 后端"可用指标目录"接口按评测类型过滤,前端目录改为后端驱动。
- 开发者新增指标 = 写一个带元数据的 grader 类,一处生效。
- Go `EvaluationType` 增加 `llm`。

**Non-Goals:**
- 不做终端用户自定义/运行时执行用户代码(无沙箱/无热加载安全面);指标扩展仅限开发者改代码+部署。
- 不重写另一套 gRPC/runner 引擎;本次只统一 worker 实际调用的 compute 路径。
- 不删除现有固定列/字段(平滑迁移,后续单独 change 再考虑清理)。

## Decisions

**决策 1:注册表 = 唯一事实来源,compute 遍历执行。** 每个 grader 声明元数据与 `eval_types`;`/compute-metrics` 按 `request.graders` 从注册表解析并调用。
- 备选:保留 if 链、仅新增目录接口——被否,漂移根因未除,仍要多处改。

**决策 2:动态 `scores` map 承载,固定字段并存。** Python 每项与聚合都输出 `scores: dict[str,float]`;Go 增加 `Scores map[string]float64`;MySQL 增 JSON `scores` 列;前端按 map 渲染。旧固定字段继续写入。
- 备选:直接改固定列为 JSON——被否,破坏兼容与历史数据;并存可平滑迁移、按需回填。

**决策 3:目录接口在 Go 后端暴露,由 Python 注册表元数据驱动。** 契约在 Go 端拼装(符合"UI 契约 Go 端拼装"约定);Go 经一个轻量端点从 Python 拉取 grader 元数据或缓存后按 `eval_type` 过滤返回前端。
- 备选:前端直连 Python——被否,越过主后端、与现有边界不符。

**决策 4:`eval_types` 用枚举集合,含 `llm`(=qa)、`rag`、`agent`,可多值共用。** Go 侧 `EvaluationType` 增加 `llm`;共用指标在多类型目录出现。

**决策 5:批量语义等既有优化保留。** 注册表化改造须保持"批量收集后单次模型加载"的语义相似度路径与零值聚合保留逻辑,不回退。

## Risks / Trade-offs

- [固定字段与 `scores` map 双写不一致] → 由 compute 端统一从同一算子输出同时填充两者;加测试断言两者一致。
- [MySQL 加 JSON 列无 AutoMigrate 被漏执行] → 在 tasks 中显式列出手动 ALTER TABLE 步骤并在 repository 读侧对空列做兼容(NULL→空 map)。
- [前端目录切后端驱动导致老页面短暂不可用] → 分阶段:先上线目录接口(前端仍可回退硬编码),再切换前端;两版并存一个迭代。
- [Python 元数据与 Go/前端契约漂移] → 目录由注册表单一生成,新增测试校验"目录内每项都有可执行 grader"。
- [`llm` 类型引入影响既有 qa 任务] → `llm` 与 `qa` 视为同义/别名,映射保持向后兼容。

## Migration Plan

1. Python:补 grader 元数据 + 注册表化 compute(输出 `scores`,保留固定字段)——独立可提交,行为对旧客户端不变。
2. Go:`types.go` 加 `Scores map`、`EvaluationType=llm`;client/worker 透传;repository 读写 JSON 列。
3. MySQL:手动 ALTER TABLE 增 `scores` JSON 列(评测表无 AutoMigrate);读侧兼容空列。
4. 目录接口:Go 暴露按 `eval_type` 过滤的目录端点。
5. 前端:目录改后端驱动 + 结果按 `scores` map 动态渲染;保留一版回退。
- 回滚:各阶段独立;前端可回退硬编码目录,Go/Python 固定字段仍在,JSON 列可忽略。

## Open Questions

- 目录接口是放在既有评测 handler 下的子路由,还是新独立端点?(倾向子路由)
- Go 是否缓存 Python 元数据(启动拉取)还是每次代理?(倾向启动/定时缓存)
- `scores` 值域统一(0-1 vs 0-100)是否借本次对齐?(倾向记录现状、不在本 change 强改,避免回退风险)
