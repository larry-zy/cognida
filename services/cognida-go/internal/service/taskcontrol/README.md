# 统一任务控制：草稿与计划预检

本批交付父任务、不可变计划版本、子任务句柄、事务投递记录，以及草稿和预检 HTTP API。计划通过校验只表示可以保存供审阅；响应固定 `execution_enabled: false`，不会创建领域运行、调度任务、调用模型、发布数据或运行容器。

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

尚无跨域执行 adapter、Supervisor、派发与恢复 worker、运行时预算扣减、审批 API、执行／取消 API、SSE 或统一任务页面。现有领域 API 的执行行为不因本批变化。清单 2.2、2.4–2.7 仍未完成；投递记录原语不能替代后台对账与取消传播。

Sandbox 本批仅有 `model/sandbox.Runner` 接口和 `service/sandbox` 的纯校验：可信 profile、输入授权、镜像／代码摘要、资源声明、执行绑定及 manifest 元数据。它没有 Docker runner、实际磁盘内容验证、归档检查、资源限制执行或候选资产登记。声明通过不能证明容器隔离已生效；3.2–3.7 保持未完成。

## 验证记录（2026-10-15）

- 隔离 MySQL 库完成全量迁移至 13、回滚一步至 12、重放至 13，`dirty=false`。
- taskcontrol 模型／服务、Sandbox、HTTP 参数及幂等中间件的单测与 race 检查通过；覆盖状态转换、预算、DAG、权限、严格 JSON 和类型推导。
- `go test -tags=integration ./internal/repository/mysql ./internal/handler -run '^TestTaskControl' -count=1 -v`：4 项真实仓储场景及 1 项真实 MySQL HTTP 场景通过，包括并发幂等、CAS、事务回滚、隔离、取消及迟到绑定。
- HTTP 集成使用真实仓储和中间件，资产授权 resolver 使用明确的 fixture；其撤权用例只证明接口会重新调用 resolver，不能证明真实来源撤权传播。未把它记为逐来源页面验收。
- 持久化／迁移、Sandbox、计划服务／HTTP／接线分别完成独立 Review，并补上实际能力推导与 Unicode 字段别名回归。

集成测试要求显式设置 `COGNIDA_TASKCONTROL_TEST_DSN`。迁移 CLI 使用 `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`，而非仅 `MYSQL_DSN`；运行前须核对为隔离测试库。测试不会自动启用业务执行器。
