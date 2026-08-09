## Why

评测(evaluation)当前有两套并行引擎:被 Go worker 主流程真正调用的 `fastapi_app.py` `/compute-metrics` 用写死的 if 链判断指标名并返回固定字段;理想的插件注册表 `graders/registry.py` 却没接到主流程。结果载体全链路固定字段(Python `ComputeItemResult` → Go `types.go` → MySQL 固定列 → 前端类型),新增一个指标要改约 6 处;前端指标目录写死在 `evaluation-config.ts` 并已与后端漂移(勾选 `llm_factual`/`llm_safety` 被静默忽略)。这既挡住了"用户在任务里自选指标",也挡住了"开发者在代码里新增指标"。

## What Changes

- 让 grader 注册表成为"可用指标"的唯一事实来源:每个 grader 声明元数据(`name`、`label`、适用评测类型、分组、是否需要参考答案/检索上下文)。
- `/compute-metrics` 端点改为遍历 `request.graders` 走注册表执行,删除写死的 if 链。
- 结果载体改为动态:Python 输出 `scores{name: value}` 动态 map,Go 用 `map[string]float64`,MySQL 增加 JSON 列(**保留现有固定字段以平滑迁移,不破坏兼容**),前端按 map 动态渲染。
- 新增后端"可用指标目录"接口,按评测类型过滤;前端目录改为后端驱动,消除漂移。
- 每个指标声明其适用的评测类型(llm/qa、rag、agent,部分跨类型共用);创建某类型任务时只展示该类型可用指标。
- Go `EvaluationType` 增加独立的 `llm` 类型。
- 开发者新增指标 = 写一个带元数据的 grader 类,一处生效:自动出现在对应类型的可选目录、被计算、被存储、被展示。
- **非破坏约束**:已在主线修复的 bug(语义相似度批量计算、llm_reasoning 解析、零值聚合保留、重试重建请求)不得回退。

## Capabilities

### New Capabilities
- `evaluation-metric-catalog`: 后端驱动的"可用指标目录"能力——按评测类型(llm/qa、rag、agent)过滤,向前端暴露指标元数据(name、label、分组、是否需参考答案/检索上下文),作为前端渲染可选指标的唯一来源。

### Modified Capabilities
- `evaluation-graders`: grader 需携带元数据(适用评测类型、分组、requires_reference/requires_contexts、label);注册表成为 `/compute-metrics` 计算路径的唯一事实来源,而非旁路。
- `evaluation-metrics`: `/compute-metrics` 改为遍历注册的 grader 执行(取代写死 if 链);计算结果以动态 `scores` map 承载并贯穿 Python→Go→MySQL(JSON 列)→前端,固定字段仅作兼容保留。

## Impact

- **cognida-python**: `services/evaluation/fastapi_app.py`(compute 路径重写)、`graders/registry.py` 与各 grader(补元数据)、`metrics/*`(接入注册表)、新增目录接口。
- **cognida-go**: `internal/service/evaluation/types.go`(动态 `map[string]float64` 载体 + `EvaluationType=llm`)、worker/repository(JSON 列读写)、新增目录 handler/service 透传。
- **MySQL**: `evaluation_qa_results` 等增加 JSON `scores` 列(保留固定列);评测表无 AutoMigrate,需手动 ALTER TABLE。
- **cognida-web**: `views/evaluation/evaluation-config.ts` 由写死改为拉取后端目录;结果展示按动态 map 渲染。
- **分阶段交付**:每阶段可独立提交且不破坏现有功能。
