# 统一任务控制：规划、跨域执行与恢复

本文描述 **2026-10-16 的实现**。`/tasks` 将用户目标、授权输入和预算保存为父任务，再把明确保存并启动的计划委派给现有领域服务。Supervisor 只做澄清、计划提案与事实汇总；它没有执行或审批权限。任务页支持能力表单、计划预检、显式启动、取消、领域详情、真实产物、只读事件流和运行中未来节点重规划。

父任务、不可变计划版本、子任务句柄和事务投递记录共同保存执行事实。跨域衔接使用有版本的生命周期引用，成功来自领域持久回执；候选生成、句柄绑定和模型返回文本均不等于发布成功。

## 启用条件与最短操作路径

先按项目流程执行全部版本化迁移；本次验收库为 **35，dirty=false**。13/14 保存父子任务、计划、outbox、执行预算和租约；15 及后续迁移增加领域回执；35 增加运行中重规划回执。运行时不自动建表。回滚前须停止实例并核对在途领域作业，不能把删除执行元数据当作暂停或恢复。

`BACKGROUND_WORKERS_ENABLED` 默认启用领域与任务 workers。`TASK_CONTROL_EXECUTION_ENABLED=false` 关闭新启动与重规划，已有执行仍核对、遵守期限并传播取消。关闭全部 workers 会停止后台核对；重新启用后从数据库恢复。开关不替代取消请求。

最短使用流程是：在数据生命周期页创建授权资产 → 在任务中心填写目标并选择该资产 → 使用固定能力表单，或让 Supervisor 提案 → 预检并显式保存 → 查看执行准入 → 显式启动 → 读取节点状态、领域详情和产物。计划保存不会自动执行；需要人工审核的领域候选仍在对应领域页审核。

接口位于身份和租户中间件之后。用户不能在请求体中指定身份、审批结果、任意代理、宿主路径或未登记工具。只有注册表列出的能力版本和对应 `parameters` 联合类型可以进入计划；界面不能绕过相同的服务端校验。

## API 与数据契约

以下路径统一以 `/api/v1/governed-tasks` 为前缀。

| 方法与路径 | 行为 |
| --- | --- |
| `GET /capabilities` | 当前可信能力版本、参数类型、工具、权限及执行约束 |
| `POST /validate` | 预检 `PlanSpec`，不创建领域运行 |
| `POST /` | 创建草稿；需要 `Idempotency-Key` |
| `GET /` | 当前用户和租户的任务；`cursor`、`page_size`（1–100） |
| `GET /:id` | 任务元数据与已持久化句柄 |
| `POST /:id/plans` | `{expected_version, plan}`；草稿／就绪任务 CAS 追加计划 |
| `GET /:id/plans/:version` | 不可变计划快照和内容摘要 |
| `POST /supervisor/assess` | 目标、来源与预算检查；缺项返回问题，不调用模型 |
| `POST /:id/supervisor/propose` | `{expected_version, answers}`；只返回澄清／候选／不支持说明 |
| `GET /:id/supervisor/summary` | 从持久事实汇总状态、预留额度和当前可读成果 |
| `GET /:id/events` | 只读 SSE `summary` 事件；终态关闭，刷新可重连 |
| `GET /:id/execution-readiness` | 当前任务／计划版本的逐节点准入结果 |
| `POST /:id/start` | `{expected_version, plan_version}`；需要 `Idempotency-Key`，固定执行计划与期限 |
| `POST /:id/cancel` | `{expected_version}`；保存取消请求，再核对领域真实结果 |
| `POST /:id/replan` | `{expected_version, reason, plan}`；需要 `Idempotency-Key`，只改未预留的未来节点 |

创建请求示例中的引用必须替换为当前用户可访问的真实资产版本：

```json
{
  "goal": "质检并清洗这份数据，保存可审阅计划",
  "input_refs": [{"asset_id": "11111111-1111-4111-8111-111111111111", "version": 1}],
  "budget": {
    "max_nodes": 4,
    "max_concurrency": 2,
    "max_attempts": 6,
    "max_calls": 20,
    "max_cost_micros": 0,
    "timeout_seconds": 600
  }
}
```

计划使用 `schema_version: 1`、`budget` 和 `nodes`。节点声明能力及版本、直接依赖、输入／输出类型、工具、权限、尝试／调用／成本额度、超时及可选类型化参数。输入二选一：`{"asset":{"asset_id":"…","version":1},"kind":"raw_asset"}` 或 `{"from_node":"quality","kind":"raw_asset"}`；后者必须来自直接依赖且输出类型匹配。工具与权限须完整匹配注册表。

请求体上限 256 KiB。未知字段、重复键、大小写及 Unicode 折叠字段别名、尾随对象、过深嵌套均拒绝，目标和数据正文仍支持 Unicode。来源采集和研究可以由可信能力声明零资产输入；这不允许普通能力省略必需数据，也不免除来源权限检查。

## Supervisor 与结果展示

页面流程为 `assess → 创建草稿 → propose → 审阅 → 保存计划`。最多 12 条澄清回答，每条携带 `question_id`、原 `question` 和 `answer`；历史问题 ID 不能被新一轮重复使用。规划器只接收目标、授权引用及类型、预算、可用能力和回答，不读取资产正文。

单次提案最多一次模型调用，45 秒、4,096 输出 token，无工具、自动重试或替代假计划。提案 HTTP 总时限 60 秒，普通接口 30 秒，事件流最多 5 分钟一条连接。模型返回后再次检查版本、授权、预算和 DAG。候选不持久化，重复提案可能再次付费；显式保存仍通过版本 CAS。提案调用未计入父任务执行额度，不宣称已实现累计模型费用结算。

事实汇总不调用模型。领域回执只供内部核对，对外成果重新授权；历史句柄保留，不把已提交成果改写成“未执行”。页面以 SSE 提示变化，再配对读取相同版本的 task/summary；每 4 秒只读轮询作为回退。隐藏页面暂停，终态停止，身份切换丢弃旧响应。领域跳转包括采集运行、离线评测、研究、特征／标注、数仓和 Sandbox。

## 当前接通的能力

实际可用性以 `capabilities` 和 `execution-readiness` 为准。未配置的外部服务、模型或 Sandbox profile 会明确阻断。

| 能力 | 实际执行和产物 | 重要边界 |
| --- | --- | --- |
| `lifecycle.data-quality` | 数据质检和报告 | 固定门槛 70；调用额度为输入数 + 1 |
| `lifecycle.data-cleaning` | trim、dedup、重做质检 | 输入数 + 2；差异与失败报告保留 |
| `lifecycle.data-enrichment` | 规则关键词、摘要和敏感度 | 输入数 + 1；不是任意模型代码 |
| `lifecycle.data-evaluation` | 本地产物完整性与来源检查 | 输入数 + 1；不是模型准确率评测 |
| `governance.feature-engineering` | 固定特征版本的真实物化回执 | 定义绑定来源；精确值和双时间；embedding 需实际模型及可信 revision，按 requirements 预留调用 |
| `governance.tagging`、`classification`、`named_entity`、`qa` | 按固定标签／词条／QA映射生成待审核结果 | 最多 32 条；不自动批准，不覆盖人工审核；规则不伪造模型置信度 |
| `collection.snapshot` | 有界 HTTPS 抓取、不可变候选快照及报告 | 无自动知识入库；新建、重跑和调度都保持候选，历史已提交回执保留 |
| `evaluation.offline-text-v1` | 对输入中的已有答案做固定确定性评分，持久逐题结果 | 显式问题／参考答案／生成答案列；零模型调用，不是在线模型生成评测 |
| `research.deep-research` | 授权 KB／图谱／已配置搜索，规划、来源、主张、证据审查和持久报告 | 最多 3 子问题、1 补充轮、4 模型调用；不是事实保证，未知付费调用不自动重放 |
| `warehouse.transform` | 固定模型版本的全量／增量／回填候选表及血缘 | 明确输入版本；审批后才切换活动版本，不默认扩大外部数仓写权限 |
| `sandbox.execute` | 受控开发 profile 的真实 Python 容器、验证输出与资产回执 | 按配置注册；与一般多租户不可信代码隔离保证有明确区别，见下文 |

类型化参数是封闭联合：`governance`、`feature`、`collection`、`evaluation`、`research`、`warehouse`、`sandbox`，每节点仅允许其能力对应的一项。不能通过模型输出改写固定特征／数仓版本、注入任意 grader/prompt 或新增工具权限。

普通 QA/RAG/Agent/SQL 在线评测入口仍存在，但当前统一任务适配器的 evaluation 能力明确为已有答案离线评分。SQL 写入／ETL 在 `/operations` 使用独立的持久计划、确认绑定、真实提交核对和 staging 切换；旧 Agent 工具返回可审核计划链接，旧确认令牌不能直接写库。统一任务的模型提案不会代替这些审批，也没有通用“批准任意工具”接口。

## 授权、预算与持久恢复

- 创建时最多 64 个根引用；计划最多 32 节点，单节点最多 16 个输入／输出类型。DAG 无环，直接依赖和类型须匹配；质检透传输入不把原始数据冒充已发布数据。
- 每轮提交、恢复和成果读取都复查来源。手工输入、登记的外部来源快照和领域产物可参与；完整父链限制深度 16、节点数 128，检查租户、权限、状态、期限和摘要。未知旧根不因存在资产 ID 就获准执行。
- `access:` 来源保留原访问请求及 owner，按真实 Web／数据源／知识／采集／结果／评测权威重新校验；注册领域 producer 通过真实运行／物化回执核对。生命周期普通 `Resolve` 也检查父链；撤权后历史句柄保留，但派生内容和汇总成果不可借旧引用继续读取。反馈候选另有正式知识读取闸门，撤回后不作为正式检索证据。
- 并发声明上限 8，单节点最多 5 attempts、128 次逻辑调用；具体 adapter 可以更严格。父任务事务预留累计 attempts/calls/cost，重试、旧计划和新计划共同计数。额度是保守预留，不是 HTTP/SQL 次数或真实供应商账单；研究和 embedding 明确不保证金额上限，不能把 `cost=0` 解释成免费调用。
- 节点时限取节点、领域上限与父绝对 deadline 的最小值。编排策略在启动前按 task + 计划摘要固定，实际并发为计划与策略上限的较小值；活动策略变化不悄悄改写在途计划。策略撤权／停用会阻止继续执行。
- 创建、启动、领域提交和重规划各自按 scope、幂等键与摘要绑定；同键不同内容冲突，每次重放仍授权。不能利用通用 HTTP 响应缓存跳过复查。
- 子任务与 outbox 原子建立。父租约和 epoch 使旧 worker 失去写权限；未知提交先 Lookup，以原操作键恢复真实句柄，不能生成新 key 重跑。领域固定单次执行，明确允许的重试由父任务新增 attempt；未知模型调用不按普通重试处理。
- 取消先持久化，再停止／核对领域作业。未知提交用同键 tombstone 阻止迟到提交。已成功作业保留真实提交事实，父任务可为 `partially_completed`；已撤权成果隐藏，取消不会擦除审计历史。

通用 `validate` 的 `origin_revalidated:false` 表示该预检阶段没有替每个 adapter 做执行准入；不等于运行时来源检查缺失。启动应同时检查 `execution-readiness`。旧无完整 provenance 的历史资产需要重新通过受控来源入口生成快照，不能只修改标志后复用。

## 运行中重规划

`POST /:id/replan` 只接受 `running` 或 `reconcile_required` 且未请求取消的任务。请求绑定当前 task version、原因和幂等键。新增不可变计划 revision，不覆盖原计划，也不重置累计预算或绝对期限。

所有已经预留过 child 的节点——包括排队、在途、完成及失败 attempt——必须逐字段保持不变；只允许改未预留的未来节点，仍须通过来源、能力、DAG 和预算校验。事务持锁后再次核对节点冻结边界，避免页面读取后恰好开始的节点被修改。执行 epoch 前移使旧 worker 失效；旧 child/outbox 保留原 plan version、operation key、payload 和回执，新计划通过全历史 children 继续核对。

页面有独立的重规划草稿和高级 JSON 校验，错误输入不替换有效草稿。提交不确定时读取真实版本恢复，不自动重提写命令。来源撤权、CAS 冲突、取消请求或已预留节点变更都会明确拒绝。

## Sandbox 的实际边界

已实现 `DockerRunner` 的 submit/status/cancel/collect、MySQL 持久恢复、真实 Docker 执行 ID、CPU/内存/进程/时间限制、无网络、非 root、只读根和只读输入挂载。输出收集会读取真实字节，拒绝越界路径、链接、归档伪装及特殊文件，核对类型、大小和摘要后才登记带来源的资产；纯 manifest 元数据校验仍不能单独授予登记权限。

默认关闭。需要显式设置 `SANDBOX_ENABLED=true`、`SANDBOX_PROFILE_ACCEPTED=true`、允许的 `SANDBOX_ALLOWED_PRINCIPALS`、绝对 staging 目录以及已登记的摘要固定镜像。受信 profile 的隔离级别为 `controlled_development`；客户端不能声明自己已满足隔离要求。Docker runtime 需通过预检，不允许任意镜像、宿主路径、凭据或容器内 Docker socket。

实际 Docker 和页面验收证明了本机受控开发 profile 的约束与恢复链路；它不构成恶意多租户代码的强隔离认证，也不把不可用 runtime 包装成成功。生产使用需要单独的隔离评估与部署策略。

## 最新验证（2026-10-16）

- 隔离 MySQL 迁移至 35，相关迁移执行 up/down/up；各新增域包含核心、真实数据库和 HTTP 测试及独立 Review。以下为本轮证据，不覆盖未运行的生产环境。
- 重规划 3 组真实 MySQL/HTTP race 回归通过：CAS、根权限、累计预算、取消、deadline、迟到绑定与旧 outbox。真实页面用在途 Sandbox 子任务验证未来节点改版，原 child 句柄和 plan version 保留、未来节点按新版本完成；场景 35.7 秒通过。
- 采集候选、已有答案离线评分、研究持久报告、特征物化、数仓候选、Sandbox 和受控操作都有实际页面链路。离线两样本评分 100/0，不能据此声称在线模型评测已通过；研究仍标 `fact_checked=false`。
- 八种生命周期来源的七阶段页面，以及 TXT/Markdown/PDF/DOCX/XLSX/CSV/HTML/PPTX 文件上传→真实解析→七阶段→审批→报告→Milvus 检索分别验收。文件组 52.7 秒通过（总 53.2 秒），无文字层 PDF 明确失败且无分块。格式、固定样本和质量边界见[验收矩阵](../../../../../apps/cognida-web/e2e/ACCEPTANCE-2026-10-16-EVIDENCE-MATRIX.md)。
- 外部数仓的本机实际引擎、协议 fixture 与云服务验收须分别记录；本地 BigQuery REST/OAuth fixture 不能冒充已连接真实云项目。认证 API 的 TLS fixture 也不冒充生产第三方服务。
- 四份旧确认／采集集成测试已同步新合同并在真实 MySQL/HTTP/Redis 上通过 race：旧 token 拒绝写库、无假成功审计；新建、重跑、定时恢复保持 candidate-only，无 ingestor 调用；停用／删除目标不恢复历史知识文档。

下面保留早期阶段的原始验收记录。其版本号、测试数和清理状态只对应标题日期，不代表当前代码仍停在该阶段。

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

## 特征工程与标注首次扩展记录（2026-10-16，本轮后续扩展前）

新增五类能力使用现有 task/outbox/lease 协议，参数在保存和分发摘要中固定。特征任务单次尝试、最多 300 秒、预留 3 次逻辑调用；标签／标注候选任务最多 32 条，最多 120 秒、预留 34 次逻辑调用。成功仅代表物化已提交或候选已生成，候选保持待人工审核。窗口、精确数值、QA真实映射、审核保护和边界见[领域能力说明](../../../../../docs/implementation/domain-specialists.md)。
