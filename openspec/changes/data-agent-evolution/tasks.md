# Tasks: Data Agent Evolution

## Phase 0 — 上下文地基：Result Store + 观察压缩
- [x] 1.1 新增 `internal/service/agent/resultstore/`：定义结果信封结构体（result_id/columns/dtypes/row_count/samples/aggregates）与接口（Put/Get/归属校验）
- [x] 1.2 实现 Redis 后端：键 `result_id`、TTL、会话/请求归属写入（Redis 不可用降级为进程内 MemoryStore）
- [x] 1.3 归属校验：跨会话读取拒绝并返回未授权错误（OwnerKey=tenant:session）
- [x] 1.4 在 `eino_agent.go` 追加 ToolMessage 处统一施加观察压缩（compactObservation，超限截断并提示按 result_id 取用）；sql_execute 改为回传信封，genUI 以有界样本快照渲染并在 Meta 暴露 result_id/truncated
- [x] 1.5 单元测试：信封生成、TTL 过期取不到、跨会话拒绝、样本 N 上限
- [x] 1.6 集成测试：真实 Redis 往返 + 大结果集只回传信封

## Phase 1 — Schema 语义选表 + 历史治理 + Token 预算
- [x] 2.1 改造 `tools/get_schema.go`：拆分 `FetchSchema`（HTTP/前端浏览器，保留全库全结构）与 `getSchema`（agent 工具，有界选表）。工具路径无表名时以**词法相关度**（`keywords` 对全部表描述卡打分，表名命中×3）返回候选子集（Milvus 向量检索为后续升级，暂不引入）；指定表名时跳过存在性查询直接精确返回（无列即视为不存在，返回 0 表）
- [x] 2.2 无命中/无关键词回退：`catalogResult` 返回受 `maxCatalogTables` 上限约束的轻量目录（仅表名+描述，不含列），超限截断并提示用 keywords 收敛；禁止无上限全库详细回退
- [x] 2.3 新增 `internal/service/agent/context/`（`window.go`）：`Windower` 施加历史窗口，超 `KeepRecentTurns` 的早期轮次由 `Summarizer` 压缩为紧凑摘要；`ExtractiveSummarizer` 为无 LLM 的确定性默认实现；`collectResultIDs`+`ensureResultIDsPresent` 兜底保证仍被引用的 result_id 与结论落进摘要（供 Phase 2 preset 装配）
- [x] 2.4 分层动态提示词 + token 预算治理（`budget.go`/`layers.go`）：规范四层 System/Safety(Pinned) → Capability → SkillPlaybook → Memory（优先级递减）；`Assemble` 无条件保留 Pinned 层，其余按优先级填充剩余预算、边界层截断、低优先层丢弃，Pinned 超预算时置 `OverBudget` 仍保留安全层
- [x] 2.5 单元测试：选表相关度/无命中回退（`tools/get_schema_test.go`）、超窗摘要保留 result_id（`context/window_test.go`）、超预算裁剪优先级/安全层保留（`context/budget_test.go`+`layers_test.go`）

## Phase 1.5 — 指标语义层（NL2Semantics）
- [x] 2a.1 新增 `agent_semantic_*` GORM model（逻辑表/维度/度量/指标/关系）+ 语义模型 repository；`cd link-go && set -a && source .env && set +a && go run ./cmd/migrate-db` 同步
- [x] 2a.2 NL2Semantics 查询路径：意图识别 → 拆解指标+维度 → 指标引擎生成 SQL；语义模型未覆盖时回退 Phase 1 词法 `get_schema` NL2SQL（可观测标注回退）
- [x] 2a.3 Verified/Golden Query 语义缓存：键=问题语义+语义模型版本，命中返回受信结果沿用 result_id 信封，模型版本变更失效
- [x] 2a.4 Neo4j 知识图谱/血缘做业务术语 grounding：同义词/口径映射注入意图识别，歧义反问不硬猜
- [x] 2a.5 单元测试：覆盖表走 NL2Semantics、未覆盖回退词法、缓存命中/版本失效、术语映射
- [x] 2a.6 集成测试：真实 DB + 真实语义模型端到端取数，与裸 NL2SQL 对比口径一致性

## Phase 2 — 单一 ReAct 内核（新 preset）
- [x] 3.1 新增 `internal/service/agent/presets/data_agent/`：ReAct 循环 preset `agent-data-agent`，入口做**意图分类路由**（取数/趋势/归因/报告→选 playbook+工具子集，歧义反问），动态工具编排，受 maxIter + token 预算约束
- [x] 3.2 注册四类能力工具于 preset（查/析/渲/操），能力间经 result_id 传递
- [x] 3.3 达 maxIter / token 耗尽时的收尾逻辑（返回部分结果 + 说明）
- [x] 3.4 在 `initializer/init.go` 与 `cmd/server/main.go` 注册新 preset
- [x] 3.5 单元测试：动态顺序、maxIter 终止、预算终止
- [x] 3.6 集成测试：查→析→渲端到端（旧 PER 双跑对比）；渲（render_ui）为 Phase 3 能力，按 present-if-registered 编排，Phase 4.2 落地后由同一用例自动覆盖

## Phase 3 — 渲染即工具 render_ui
- [x] 4.1 扩展 `genui/spec.go` Catalog：新增交互组件 Button/Confirm/Form/Filter/Pagination
- [x] 4.2 新增 `tools/render_ui.go`：产出 A2UI 规格，经 result_id + RFC6901 Pointer 引用数据
- [x] 4.3 校验（复用 `genui/validate.go`）：拒绝非法 result_id / 越界 Pointer / 目录外组件，校验失败不推 ui 事件并回灌 LLM
- [x] 4.4 改造 `agent_handler.go`：`ui` 事件即时化（每次 render_ui 即推，替代 120-127 末尾一次性 compose）
- [x] 4.5 UI 持久化：A2UI 规格随 assistant 消息存 MySQL，会话重开从消息记录重现 UI surface（不依赖 SSE 重放）
- [x] 4.6 有界数据快照：render_ui 渲染时把组件实际展示的有界行 + 聚合快照进消息记录（独立于 Result Store TTL）；大/无界数据不全量快照
- [x] 4.7 过期降级：result_id 过期时，小结果走快照重现、大数据渲染"数据已过期，可重跑"占位
- [x] 4.8 交互绑定状态（surface ↔ result_id / pending_action_id + token）存 Redis 会话 TTL，支撑回调路由；超 TTL 返回"会话已过期"
- [x] 4.9 link-web：`A2UINode.vue` / `A2UIRenderer.vue` 交互回调，`AICenterView.vue` 多 UI surface；优先自研 Ui* 组件
- [x] 4.10 单元测试：多次渲染独立 surface、大表引用不内联、非法引用拒绝、快照重现、大数据过期降级占位

## Phase 3.5 — 洞察归因（分化分析能力）
- [x] 4a.1 `data_analysis` 分化命名能力：趋势/对比/归因-根因/报告解读；能力选择受意图路由驱动
- [x] 4a.2 归因/根因：Go 侧编排 + link-python 归因算法（variance decomposition + driver ranking + 隐藏因子），大小模型协同
- [x] 4a.3 归因结果回传 result_id 信封 + 文字洞察，附样本与口径/置信标注，支持下钻校验
- [x] 4a.4 link-python `tools/analytics/` 按 result_id 契约对接归因 API（计算侧算法不变）
- [x] 4a.5 单元测试：意图→能力路由、归因回传契约、口径标注、下钻引用
- [x] 4a.6 集成测试：真实数据归因端到端（driver ranking 稳定性）

## Phase 4 — 操作工具 + 危险分级 + 审计
- [x] 5.1 新增 `agent_operation_audit` GORM model；`cd link-go && set -a && source .env && set +a && go run ./cmd/migrate-db` 同步
- [x] 5.2 新增 `tools/sql_mutate.go`：DML only（拒 DDL）、idempotency_key、事务内 dry-run 评估影响行数、红线原始表拒改
- [x] 5.3 新增 `tools/etl_run.go`：派生 `agent_etl_*` 新对象、前缀强校验、支持 result_id/SQL 输入源
- [x] 5.4 新增 `tools/data_export.go`：按 result_id 导出 CSV/Excel，返回文件引用
- [x] 5.5 危险分级判定：白名单安全自动执行；危险操作产出 pending_action_id 暂停
- [x] 5.6 所有写/ETL/导出/拒绝写入 `agent_operation_audit`
- [x] 5.7 单元测试：拒 DDL、幂等去重、前缀校验、红线拒改、影响行数阈值触发暂停
- [x] 5.8 集成测试：真实 DB dry-run + 审计留痕 + 派生表不动原表

## Phase 5 — 人机确认 resume
- [x] 6.1 pending action 存储（Redis）：pending_action_id + token + TTL
- [x] 6.2 `agent_handler.go` 新增 confirm-resume 端点：校验 token 匹配/未过期后提交事务，不匹配则拒绝并失效
- [x] 6.3 Confirm 组件回调 → 携 pending_action_id + token 的 follow-up resume
- [x] 6.4 单元/集成测试：确认落库、token 不匹配拒绝、过期失效

## Phase 6 — Skill 硬工具门 + 会话 scope
- [x] 7.1 新增 `ToolPolicy{Allow,Deny,Scope}` + `Permits(tool)`（deny 优先、非空 allow 白名单模式、scope 校验）
- [x] 7.2 拦截点：`eino_agent.go` 工具执行前（~1006-1018），被拒返回合成 `{"error":"tool_blocked",...}` ToolMessage
- [x] 7.3 从命中 skill 的 allowed_tools/disallowed_tools 构建 Deny/Allow；叠加会话 scope（read/write/etl）
- [x] 7.4 拦截留痕（被拒工具/原因/skill/会话/request_id）
- [x] 7.5 单元测试：disallowed 拦截、白名单模式、deny 优先、只读会话拦写、scope 与策略同为必要条件

## Phase 7 — 数据域子代理委派（orchestrator-worker）
- [x] 8.1 在 `initializer/init.go` 创建 `CollaborationRegistry` 并注册子代理：SchemaExplorer/SQLAuthor/Analysis/Operation/Viz + 分层的 Insight/Report，各声明最小工具集 + maxIter；指挥官 preset `.WithCollaboration(registry, EnableDelegate())` 激活 `delegate_to_agent`
- [x] 8.1a Insight/Report 分层协作：Report→Insight→Query 逆向拆解、正向执行，上层只收下层 handle/摘要
- [x] 8.2 仅 Operation 持写工具；其余不授写工具；工具授予每次委派授予、非持久高权
- [x] 8.3 委派信封作为**校验型契约** `{goal,inputs,constraints{scope,max_rows},return}`，缺必填字段拒绝并回灌 LLM；子代理只回传 handle/摘要
- [x] 8.4 默认上下文模式：探查/写作 isolated，分析/操作/渲染 summary；沿用 IsCyclic + MaxDepth
- [x] 8.5 委派拓扑：独立子任务并行 fan-out（受**并发上限**护栏），依赖链串行；委派结果按 handle 落 Result Store，失败子委派可独立重试
- [x] 8.6 治理目录：注册项携 `{purpose,data_scope,tools,risk_class}`，与 `agent_operation_audit` 串联
- [x] 8.7 单元测试：最小工具集、隔离不泄漏全历史、只回传 handle、循环委派拦截、缺字段委派被拒、并发上限约束、失败子委派独立重试

## Phase 8 — 切换与收尾
- [x] 9.1 主入口迁移到 `agent-data-agent`（旧 preset 保留兼容）
- [x] 9.2 link-python `tools/analytics/` 按 result_id 契约对接（计算侧算法不变）
- [x] 9.3 全量回归：go test ./internal/... + 集成测试 + API 测试
- [x] 9.4 code-review skill 通过后提交
- [x] 9.5 任务完成后终止所有开启的服务进程
