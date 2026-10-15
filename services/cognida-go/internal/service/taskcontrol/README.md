# 统一任务控制：Supervisor 与计划审阅

已交付父任务、不可变计划版本、子任务句柄、事务投递记录，以及 Supervisor 规划服务、HTTP API 和 `/tasks` 任务规划页面。Supervisor 调用现有模型生成澄清问题或候选计划，服务端重新预检后交由用户保存。响应固定 `execution_enabled: false`；保存计划不会创建领域运行、派发任务、发布数据或运行容器。

## 启用与 API

按项目迁移流程先应用 `migrations/000013_task_control.{up,down}.sql`，运行时不自动建表。迁移新增 `task_control_tasks`、`task_control_plans`、`task_control_children`、`task_control_outbox` 四张表，不迁移旧任务或修改原领域表。

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

汇总保留计划、投递状态和实际绑定句柄；领域状态尚未接入时返回 `domain_status: "unknown"`，不会把句柄绑定当作成功。原始回执不对外返回，结果引用仍需通过当前资产访问校验。页面没有执行和审批入口。

## 校验与持久化保证

- 初始输入最多 64 个版本引用；节点最多 32 个，每节点最多 16 个输入和 16 个输出类型。计划不能扩张任务创建时的输入集合或预算。
- 预检检查能力版本、必要数据输入、输入／输出类型、直接依赖、无环、工具和权限。质检只透传首个数据输入，不能把原始数据声明成已发布数据；清洗不能只绑定质检报告。
- 并发声明上限 8；每节点尝试次数上限 5，单次调用数上限 128。尝试、调用、成本及执行秒数按全部节点及重试保守求和。成本以统一计价单位的百万分之一表示；这是计划额度，不是实际费用结算或运行时计量。
- 写入／ETL 能力必须在可信注册表中标为需要审批；预检只返回 `approval_required`，不签发或复用审批。
- 创建草稿的同作用域、同键、同内容返回同一任务；同键不同内容冲突。每次重放仍检查当前用户权限和资产访问。该接口不使用通用响应缓存。
- 追加计划使用父任务版本 CAS，只允许未派发子任务的草稿／就绪任务；新版本回到 `draft`。历史快照不覆盖，任务不能继承旧计划授权。
- 仓储在事务内创建子任务和 outbox；操作键固定任务、计划、节点及 attempt。Claim 后不能直接重放；未知提交保留待核对状态。取消待投递任务与取消已进入提交临界点的任务不同，迟到的真实提交回执被保留，绑定句柄不代表领域任务完成。

## 当前验收边界与后续工作

当前实际 resolver 调用 `lifecycle.Resolve`，复查租户／用户权限、资产状态、有效期、敏感级别和存储摘要。响应显式标记 `authorization_scope: "lifecycle_asset"`、`origin_revalidated: false`。它尚未递归检查原知识文档禁用／删除、外部数据源撤权或派生来源失效传播；这属于实施清单 6.7，在可执行控制器启用前需要补齐。读取已有任务／计划只返回所属任务的元数据，不重新读取资产正文。

尚无跨域执行 adapter、派发与恢复 worker、运行时预算扣减、审批 API、执行／取消 API 或 SSE。当前页面覆盖规划、任务列表、历史计划和事实汇总，尚非完整运行工作台。现有领域 API 的执行行为不因本批变化；投递记录原语不能替代后台对账与取消传播。后续顺序为能力接入及控制器、多 Agent 协作与任务分发验证、最后 Sandbox，清单 2.2、2.5–2.7 保持未完成。

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
