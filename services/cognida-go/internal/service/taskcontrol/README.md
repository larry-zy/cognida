# 统一任务控制：Supervisor、受控执行与恢复

已交付父任务、不可变计划版本、子任务句柄、事务投递记录，以及 Supervisor 规划服务、HTTP API 和 `/tasks` 页面。Supervisor 生成候选，服务端预检后由用户保存；保存不派发任务。执行前通过 `execution-readiness` 检查固定操作与完整来源链，用户显式启动后由持久控制器调用已有领域服务。当前接通质检、预置清洗、规则增强和本地产物完整性评估。

## 启用与 API

按项目流程执行迁移至 **14**，运行时不自动建表。版本 13 新增任务、计划、子任务及 outbox；版本 14 增加执行计划、期限、终态、领域观测、预算预留及 `task_control_leases`。回滚 14 前须停止实例、核对并排空在途任务；回滚会移除执行元数据，不能当作暂停后恢复。

`BACKGROUND_WORKERS_ENABLED` 默认启用领域与任务 workers。`TASK_CONTROL_EXECUTION_ENABLED=false` 仅关闭新启动；已有执行继续核对、遵守期限并传播取消。停止所有后台处理用 `BACKGROUND_WORKERS_ENABLED=false`，同时禁止新启动；恢复 workers 后从数据库接管。开关不替代显式取消任务。

接口位于认证及租户中间件之后；用户与租户来自服务端上下文，请求不能提供身份、审批状态、执行参数或宿主路径。

| 方法 | 路径 | 行为 |
| --- | --- | --- |
| GET | `/api/v1/governed-tasks/capabilities` | 返回可信能力版本、工具、权限和输入／输出约束 |
| POST | `/api/v1/governed-tasks/validate` | 预检 `PlanSpec`，不写入领域运行 |
| POST | `/api/v1/governed-tasks` | 创建草稿；必须携带 `Idempotency-Key` |
| GET | `/api/v1/governed-tasks` | 当前用户及租户的任务；`cursor`、`page_size`（1–100） |
| GET | `/api/v1/governed-tasks/:id` | 读取所属任务的元数据 |
| POST | `/api/v1/governed-tasks/:id/plans` | `{ "expected_version": 1, "plan": ... }`，CAS 追加不可变计划 |
| GET | `/api/v1/governed-tasks/:id/plans/:version` | 读取计划快照及内容摘要 |
| POST | `/api/v1/governed-tasks/supervisor/assess` | 检查目标、输入和预算；缺项返回问题，不调用模型或写库 |
| POST | `/api/v1/governed-tasks/:id/supervisor/propose` | `{ "expected_version": 1, "answers": [] }`；调用模型生成澄清／候选／不支持说明，不保存候选 |
| GET | `/api/v1/governed-tasks/:id/supervisor/summary` | 根据已持久化计划及子任务句柄汇总事实，不调用模型 |
| GET | `/api/v1/governed-tasks/:id/execution-readiness` | 只读检查任务／计划版本、能否启动及逐节点阻断原因 |
| POST | `/api/v1/governed-tasks/:id/start` | `{ "expected_version": 2, "plan_version": 1 }`，必须有 `Idempotency-Key`；固定计划与绝对期限 |
| POST | `/api/v1/governed-tasks/:id/cancel` | `{ "expected_version": 3 }`，记录取消请求，随后核对真实终态 |

任务创建请求示例（引用必须替换为当前用户可访问的真实生命周期资产版本）：

```json
{
  "goal": "质检并清洗这份数据，保存可审阅计划",
  "input_refs": [{ "asset_id": "11111111-1111-4111-8111-111111111111", "version": 1 }],
  "budget": {
    "max_nodes": 4,
    "max_concurrency": 2,
    "max_attempts": 6,
    "max_calls": 20,
    "max_cost_micros": 1000000,
    "timeout_seconds": 600
  }
}
```

计划使用 `schema_version: 1`、相同结构的 `budget` 和 `nodes`。每个节点声明 `id`、`capability`、`capability_version`、`depends_on`、`inputs`、`output_kinds`、`tools`、`permissions`、`max_attempts`、`max_calls_per_attempt`、`max_cost_micros_per_attempt`、`timeout_seconds`。输入只能二选一：`{"asset": {"asset_id": "...", "version": 1}, "kind": "raw_asset"}` 或 `{"from_node": "quality", "kind": "raw_asset"}`；后者必须来自直接依赖且类型匹配。工具与权限必须完整匹配能力注册表，客户端不能删减权限或自行添加工具。

请求体上限 256 KiB，JSON 字段严格匹配小写 ASCII 名称；未知字段、重复键、大小写及 Unicode 折叠别名、尾随对象和过深嵌套均拒绝。目标文本仍支持 Unicode。

## Supervisor 规划流程

页面依次完成 `assess → 创建草稿 → propose → 审阅 → 显式保存计划`。澄清回答携带 `question_id`、`question` 与 `answer`，保留问题正文以供后续轮次理解；最多 12 条。缺少必需输入时不消耗模型请求。模型不可用或输出不合约时明确报错，不生成替代计划。

规划器只接收目标、授权输入引用及类型、用户预算、当前权限允许的能力和澄清历史，不读取资产正文。每次提案最多一次模型调用，45 秒、4,096 输出 token；无工具执行、自动重试或审批能力。提案 HTTP 请求另设 60 秒总时限，其他任务接口保持 30 秒。输出严格限定为澄清、计划或不支持三类。模型返回后重新检查任务版本、输入访问、预算、能力与 DAG；不能扩张原始输入集合、工具和权限。

提案是只读候选，重复请求可能再次调用模型，不具备持久化的请求去重或累计模型费用结算。保存沿用计划版本 CAS；历史任务深链和刷新读取已保存快照，不重新调用模型。未保存候选离开页面时提示确认；切换身份清除旧上下文并忽略旧请求结果。

汇总保留计划、投递状态和实际绑定句柄；领域状态来自持久化观测，句柄绑定不表示完成。原始回执不对外返回，结果引用再次检查完整来源链。页面显示固定操作、预检阻断、启动、取消、节点进度、领域深链及产物；每 4 秒只读刷新，终态停止、隐藏页面暂停，身份变化丢弃旧响应。审批入口尚未接通。

## 校验与持久化保证

- 初始输入最多 64 个版本引用；节点最多 32 个，每节点最多 16 个输入和 16 个输出类型。计划不能扩张任务创建时的输入集合或预算。
- 预检检查能力版本、必要数据输入、输入／输出类型、直接依赖、无环、工具和权限。质检只透传首个数据输入，不能把原始数据声明成已发布数据；清洗不能只绑定质检报告。
- 并发声明上限 8；每节点尝试次数上限 5，单次调用数上限 128。尝试、调用、成本及执行秒数按全部节点及重试保守求和。成本以统一计价单位的百万分之一表示；这是计划额度，不是实际费用结算或运行时计量。
- 写入／ETL 能力必须在可信注册表中标为需要审批；预检只返回 `approval_required`，不签发或复用审批。
- 创建草稿的同作用域、同键、同内容返回同一任务；同键不同内容冲突。每次重放仍检查当前用户权限和资产访问。该接口不使用通用响应缓存。
- 追加计划使用父任务版本 CAS，只允许未派发子任务的草稿／就绪任务；新版本回到 `draft`。历史快照不覆盖，任务不能继承旧计划授权。
- 仓储在事务内创建子任务和 outbox；操作键固定任务、计划、节点及 attempt。Claim 后不能直接重放；未知提交保留待核对状态。取消待投递任务与取消已进入提交临界点的任务不同，迟到的真实提交回执被保留，绑定句柄不代表领域任务完成。

## 受控执行与恢复

| 能力 | 固定操作 | 每次逻辑调用预算下限 |
| --- | --- | --- |
| `lifecycle.data-quality` | 数据质检，门槛 70 | 输入数 + 1 |
| `lifecycle.data-cleaning` | trim + dedup，保留首次记录，重做质检，门槛 70 | 输入数 + 2 |
| `lifecycle.data-enrichment` | 规则关键词、摘要、敏感度；最多 10 关键词、300 字符摘要 | 输入数 + 1 |
| `lifecycle.data-evaluation` | 本地产物完整性与来源检查，不是模型准确率／语义评测 | 输入数 + 1 |

节点最长 300 秒，实际时限取节点、领域服务和父任务绝对期限的最小值。调用数表示输入解析与固定算子的逻辑额度，不是 HTTP／SQL 次数。每次尝试在事务中按保存的 Node 预留调用／成本，页面显示预留而非实际费用；本批固定算子不调用付费模型。领域 stage 固定单次尝试，重试由父任务控制，只有明确失败／超时才可增加 attempt。

- 启动原子固定执行计划与期限；同键同计划重放返回原执行，不重置期限或重跑。启动后不能追加计划。
- 只分发依赖全部成功的节点，并限制活跃尝试数。输出绑定要求唯一、真实、授权的同类型版本，不猜测多个结果中的一个。
- 父任务使用 40 秒数据库租约、20 秒单次核对期限。所有执行写操作事务内检查 owner、epoch 和到期时间；旧 worker 不能覆盖新 owner，续租不改变用户任务版本。
- outbox 与 child 原子建立，操作键绑定租户、用户、任务、计划、节点、attempt。领域 Submit 同键同摘要原子幂等。未知提交先 Lookup；权威确认不存在后，只以同键同载荷重提。
- 取消先持久化，再取消领域作业。未知提交不存在时写同键 cancelled tombstone，阻止迟到 Submit。pending 不再投递；已成功作业保留事实，父任务以 `partially_completed` 保留当前可访问成果。
- 每轮核对及提交前复查来源；已完成节点也不能跳过父任务完整来源链检查。结果再次授权；失败／隔离不会汇总为成功。
- 页面读取只查询事实，不触发分发。领域 worker 根据自己的持久记录恢复队列，父任务恢复只核对相同领域句柄。

## 当前验收边界与后续工作

通用计划预检仍标记 `authorization_scope: "lifecycle_asset"`、`origin_revalidated: false`，只证明资产级检查。执行预检另行递归检查完整来源链：当前仅放行 `ImportWithKey` 的手工输入（`lifecycle` 来源、`upload:` 根）及可验证派生版本，限制深度 16、节点数 128，复查身份、状态、期限、敏感级别及摘要。外部 Web／数据源／知识／采集／结果／评测来源及旧未知根明确阻止启动；原来源撤权传播仍属 6.7。

本批的结果过滤作用于执行汇总与最终可用成果集合。普通任务读取／列表和历史记录仍保留已持久化的引用标识；原生命周期资产读取接口沿用资产自身权限，不递归传播祖先失效。此处验证的是控制器停止后续分发及汇总隐藏失效成果，不代表全平台历史派生资产均已不可读取；统一撤回、禁用与缓存失效继续由 6.7 实施。

本批实现 lifecycle 四类固定适配，不包括 collection、governance、模型 evaluation、research、发布／任意分析与自定义代码。审批绑定、运行中重规划、SSE、付费模型实际费用计量及旧八类来源联合执行验收继续实施。清单 2.2、2.5–2.7 按完整要求保持未勾选；域内多节点协作不等于跨域全链路完成。

Sandbox 本批仅有 `model/sandbox.Runner` 接口和 `service/sandbox` 的纯校验：可信 profile、输入授权、镜像／代码摘要、资源声明、执行绑定及 manifest 元数据。它没有 Docker runner、实际磁盘内容验证、归档检查、资源限制执行或候选资产登记。声明通过不能证明容器隔离已生效；3.2–3.7 保持未完成。

## 验证记录（2026-10-15）

- 隔离 MySQL 库完成全量迁移至 13、回滚一步至 12、重放至 13，`dirty=false`。
- taskcontrol 模型／服务、Sandbox、HTTP 参数及幂等中间件的单测与 race 检查通过；覆盖状态转换、预算、DAG、权限、严格 JSON 和类型推导。
- `go test -tags=integration ./internal/repository/mysql ./internal/handler -run '^TestTaskControl' -count=1 -v`：4 项真实仓储场景及 1 项真实 MySQL HTTP 场景通过，包括并发幂等、CAS、事务回滚、隔离、取消及迟到绑定。
- HTTP 集成使用真实仓储和中间件，资产授权 resolver 使用明确的 fixture；其撤权用例只证明接口会重新调用 resolver，不能证明真实来源撤权传播。未把它记为逐来源页面验收。
- 持久化／迁移、Sandbox、计划服务／HTTP／接线分别完成独立 Review，并补上实际能力推导与 Unicode 字段别名回归。
- Supervisor 服务、严格模型适配器、HTTP 与接线的 race 回归通过，taskcontrol 服务语句覆盖率为 89.1%；通过独立 Review。
- `go test -race -tags=integration ./internal/handler -run '^TestTaskSupervisor' -count=1 -v`：4 项真实 MySQL/HTTP 契约场景通过，使用明确的 planner/resolver fixture 验证预览无写入、CAS、授权、撤权、取消和事实汇总。
- 显式设置 `SUPERVISOR_REAL_LLM=1` 后，第 5 项用合成目标与引用调用实际配置模型，经 HTTP 生成有效候选并核对数据库没有计划和子任务写入。此项不读取真实业务资产正文，也不证明领域执行已接通。
- [真实页面验收](../../../../../apps/cognida-web/e2e/ACCEPTANCE-2026-10-15-SUPERVISOR.md)通过输入创建→实际模型候选→人工保存→深链刷新，保存前后均无子任务，桌面和手机截图已检查。前端 26 项定向测试、类型检查与生产构建通过。

集成测试要求显式设置 `COGNIDA_TASKCONTROL_TEST_DSN`。迁移 CLI 使用 `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`，而非仅 `MYSQL_DSN`；运行前须核对为隔离测试库。测试不会自动启用业务执行器。

## 受控执行验证（2026-10-15）

- 隔离 MySQL 迁移 14 完成 up → down 1 → up，`dirty=false`；10 项仓储 race 集成覆盖幂等启动、租约过期／独立进程退出接管、迟到写 fencing、取消、预算、终态和非 UTC 驱动时区。
- `go test ./internal/...` 全部通过；runtime helper、领域 adapter 与生命周期控制器有单元及 race 回归。
- 新增 9 组 HTTP／Runtime 集成使用真实 MySQL 与明确的故障 adapter，验证只读预检、跨身份、CAS、幂等、未知恢复、取消 tombstone、取消竞态、重试／超时、并发依赖、部分成功及完整来源撤权；不冒充真实 Python 计算。
- 适配器集成使用真实 MySQL、Redis、本地对象存储及确定性计算 fixture，验证领域持久化、worker、产物、重建读取、并发提交、取消及跨身份隔离。
- 真实 Go／Redis／Python 和 Playwright 证据见[任务执行验收](../../../../../apps/cognida-web/e2e/ACCEPTANCE-2026-10-15-TASK-RUNTIME.md)。
- 本批 3 场真实页面、41 项前端单测、类型检查及构建通过。真实 Go 在首个领域成功而父任务未完成时 SIGKILL，再用同一二进制／数据库／Redis 恢复，4 节点全部 attempt 1、领域句柄及原有回执不重复。此项不证明在途 Python 计算重放。
- 最后停止真实 Go worker 后再次执行仓储／HTTP／adapter 的 MySQL 与 Redis race 集成，全部通过；实际模型规划测试本批显式跳过，沿用前批证据。独立 Review 完成，测试服务、隔离数据库和临时凭据已清理。
