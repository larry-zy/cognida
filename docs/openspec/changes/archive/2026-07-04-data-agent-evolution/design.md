# Design: Data Agent Evolution

## Context

现有 Text2SQL Agent 是**固定 Plan-Execute-Reflect（PER）流水线**、**只读**，且上下文"裸喂"：`get_schema` 无表名时返回全库、`sql_execute` 把上千行原始结果逐字回灌、多轮历史全量重放（`eino_agent.go:245-264`）、无结果存储。分析工具（5 类 MCP）与 A2UI 生成式 UI 已就绪，但 A2UI 仅在流末尾一次性 `compose`（`agent_handler.go:120-127`），无法在循环中多次渲染。目标是把三者统一到一个能**动态编排、按引用管理上下文、安全操作数据**的 Data Agent。完整背景见 `docs/go/data-agent-plan.md`。

关键约束（来自 CLAUDE.md 与用户确认）：
- 操作范围 = 数据导出 + ETL/清洗（派生新对象）+ DML 写库；**明确不含 DDL**。
- 内核换为**单一 ReAct 循环**替代 PER。
- 确认策略 = **分级**：白名单安全操作自动执行，危险操作才走人机确认。
- 业务表无 SQL 迁移文件，新表经 `cmd/migrate-db` 从 GORM model 同步。
- Python 只做计算/分析，Go 为主后端，UI 契约在 Go 端拼装。

## Goals / Non-Goals

### Goals
- 以单一 ReAct 循环统一查询/分析/渲染/操作四类能力。
- 引入 Result Store 实现端到端 data-by-reference，工具回灌 LLM 的是"结果信封"。
- 上下文工程地基：schema 有界选表、观察压缩、历史窗口/摘要、token 预算治理。
- **指标语义层（NL2Semantics）作为查询地基**：受治理的指标/维度语义模型 + 指标引擎生成 SQL，替代裸 NL2SQL 为主路径；Verified/Golden Query 语义缓存优先返回受信结果；复用 Neo4j 知识图谱/血缘做业务术语 grounding。
- **归因/根因作为一等分析能力**：`data_analysis` 分化出趋势/对比/归因/报告解读，归因经 Go 编排 + Python 算法（大小模型协同）。
- 安全操作：`sql_mutate`/`etl_run`/`data_export` + 危险分级 + dry-run + idempotency + 人机确认。
- Skill `allowed_tools`/`disallowed_tools` 升级为执行前硬拦截，叠加会话 scope 最小权限。
- 数据子代理作为上下文防火墙，经运行时委派；补 Insight/Report 分层，形成 Report→Insight→Query 逆向拆解、正向执行的协作。

### Non-Goals
- 不支持 DDL（CREATE/ALTER/DROP TABLE）。
- 不修改任何原始业务表（ETL 只派生 `agent_etl_*` 新对象）。
- 不改动 Python 计算侧算法，仅对齐 `result_id` 契约。
- 不引入新的编排框架，复用既有 Eino agent 循环与 CollaborationContext。
- 旧 Text2SQL preset 保留兼容，不在本次删除。
- 不引入 A2A 等跨进程 agent 间协议；子代理委派为**进程内** CollaborationContext（工具侧 MCP 现状不变）。A2A/跨进程编排列为未来演进。
- 不做**主动式预警/监控**（自动扫描千级指标推异常）；本次为被动响应式，proactive monitoring 列 future roadmap。
- 能力闭环止于"建议"，**不做行动派发/自动执行外部业务动作**（如自动触达、工单派发）。
- 指标语义模型的**编排/建模台**（可视化配置指标口径）不在本次范围；本次仅提供语义模型的存储契约与消费（NL2Semantics + verified query），模型可由 seed/接口写入。

## Decisions

### D1: 单一 ReAct 循环 vs 保留 PER
选**单一 ReAct**。PER 的固定阶段无法表达"查→析→渲→操"的动态顺序与回退。以新 preset `agent-data-agent` 承载，复用 `eino_agent.go` 的 maxIter 循环；旧 preset 保留兼容。**BREAKING**：Text2SQL 主入口迁移到 Data Agent。

### D2: Result Store 作为地基（data-by-reference）
所有数据工具经 `result_id` 传递引用。完整结果存 Redis（TTL + 会话归属校验），LLM 只见信封（列/dtype/row_count/样本/聚合）。压缩统一施加在 `eino_agent.go` 追加 ToolMessage 处，与观察压缩同一拦截点。

上下文治理遵循业界"**卸载/压缩优先、摘要兜底**"次序（Manus / 长程 agent 实践）：先按 `result_id` 卸载重数据、再对观察做有界截断压缩，仅当仍超预算时才对早期历史做**结构化摘要**（保 result_id/结论/未决操作，非自由文本）。Token 预算设**两级阈值**——`pre-rot 触发点`（远早于模型硬窗，规避 context-rot 早退化）触发历史窗口/摘要，`硬上限` 作为分层裁剪的最终边界。

### D3: 渲染即工具（render_ui），流中多次
A2UI 从流末尾 `compose` 改为 `render_ui` 工具，每次调用即时推 `ui` 事件。组件经 `result_id` + RFC6901 JSON Pointer 引用数据，不内联大数据。扩交互组件（Button/Confirm/Form/Filter/Pagination），交互回调作为 follow-up 驱动下一轮。

### D4: 分级安全 + 人机确认
危险分级：白名单安全操作自动执行；危险操作事务内 dry-run 评估影响行数，超阈值产出 `pending_action_id` 暂停，经前端确认卡片 + 携 token 的 resume 后落库。所有写/ETL/导出/拦截记入 `agent_operation_audit`。ETL 强制 `agent_etl_` 前缀红线。

### D5: Skill 硬工具门（ToolPolicy）
`ToolPolicy{Allow, Deny, Scope}` + `Permits(tool)`（deny 优先、非空 allow 即白名单模式、scope 校验）。拦截点在 `eino_agent.go:1006-1018` 工具执行前，被拒返回合成 `{"error":"tool_blocked",...}` ToolMessage。会话 scope（read/write/etl）与 skill 策略共同为放行必要条件。

### D6: 子代理作为上下文防火墙（orchestrator-worker）
复用 CollaborationContext 隔离模式：SchemaExplorer/SQLAuthor 默认 isolated，Analysis/Operation/Viz 默认 summary。指挥官经 `delegate_to_agent` 委派，传结构化信封，只收 handle/摘要——**上下文隔离**是本模式一等目标：子任务在子代理自有窗口内消化重认知，指挥官窗口不随任务复杂度线性膨胀。仅 Operation 持写工具（最小权限）。沿用 IsCyclic 循环检测与 MaxDepth。

结合 2026 orchestrator-worker 主流实践，补三点：
- **委派拓扑**：独立子任务（多数据源/多指标/多维度探查）SHALL 支持**并行 fan-out**，依赖链（探查→写作→分析→渲染）走串行；并行受**并发上限**约束（与 MaxDepth/IsCyclic 并列的资源护栏），避免子代理树暴涨。
- **类型化委派契约**：委派接口本质是 NL prompt，是本模式最大风险面。信封 `{goal,inputs,constraints,return}` 作为**校验型契约**，缺 `goal`/`constraints.scope` 等必填字段即拒绝委派并回灌 LLM，把"正确任务规格"的负担从自由文本拉回到结构校验。
- **治理目录 + 可恢复**：CollaborationRegistry 注册项 SHALL 携治理元数据 `{purpose, data_scope, tools, risk_class}`，形成活体 agent 目录（owner/用途/数据访问级/工具集/风险级），并与 `agent_operation_audit` 串联；子代理工具授予为**每次委派授予、非持久高权**。委派结果按 handle 落 Result Store，单个失败子委派可重试而不牵连并行兄弟。

### D7: 指标语义层 + NL2Semantics（查询地基）
垂类共识——数据 agent 准确率是"语义层质量"的函数，非 prompt 工程；裸 NL2SQL 受幻觉与算术弱之限（对标 Cortex Analyst semantic view / Genie Unity Catalog Metrics / Looker LookML / 数势 NL2Semantics）。引入受治理的**指标语义模型**（逻辑表/维度/度量/指标/关系），查询主路径为「**意图识别 → 拆解指标+维度 → 指标引擎生成 SQL**」，替代裸 NL2SQL。语义模型经 GORM model 落 MySQL（`agent_semantic_*`，`migrate-db` 同步），并复用既有 **Neo4j 知识图谱/血缘**做业务术语 grounding（把"业务语言"喂给 agent 的"教科书"）。Phase 1 的**词法选表降级为回退路径**——语义模型未覆盖的库表仍走词法 `get_schema`，二者并存不互斥。

### D8: Verified / Golden Query 语义缓存
高价值问题预置「问题 → 专家校验的指标查询/SQL」对；提问先查语义缓存，命中即返回受信结果与既有 result_id 契约，未命中才走 D7 生成（对标 BigQuery Verified Queries / AWS Bedrock 语义缓存）。缓存键 = 问题语义 + 语义模型版本，模型版本变更即失效。降低幻觉、延迟与 token。

### D9: 归因/根因作为一等分析能力（大小模型协同）
`data_analysis` 由单一泛化工具**分化为命名能力**：趋势、对比、**归因/根因**（variance decomposition + driver ranking + 隐藏因子）、报告解读。取数价值最低、归因价值最高，是垂类差异化点（investigation depth）。归因由 Go 侧编排 + **Python 侧算法**（归因 API，符合"Python 只做计算/分析"分工）协同，结果回 `result_id` 信封 + 文字洞察，避免用户拿到数不知含义、不知如何下钻。

### D10: 意图路由 + Query/Insight/Report 分层
ReAct 内核入口做**意图分类**（取数/趋势/归因/报告），据此选对应 skill playbook 与工具子集，降延迟提准确、遇歧义反问而非硬猜。子代理体系补 **Insight（洞察）** 与 **Report（报告）** 两类：Report→Insight→Query **逆向拆解、正向执行**——Report 声明需哪些洞察、Insight 声明需哪些数据、Query 取数回填。能力分级从 L1 查询 → L2 归因解释 → 建议（本次闭环止于"建议"，行动派发见 Non-Goals）。

## Risks / Trade-offs

- **迁移风险（BREAKING）**：主入口切到 Data Agent 可能回归旧行为。缓解：旧 preset 保留、灰度切换、契约测试。
- **Result Store 一致性**：Redis TTL 过期导致后续工具取不到数据。缓解：明确"结果已过期"错误、关键结果延长 TTL、导出即时化。
- **选表漏表**：词法/语义模型未覆盖致 SQL 生成失败。缓解：候选集上限内允许 LLM 追加 `get_schema(table_name)` 精确补查；语义模型缺失时回退词法。
- **语义模型缺失/滞后**：无语义模型或口径过时则 NL2Semantics 退化。缓解：语义模型未覆盖库表回退词法 NL2SQL；缓存键含模型版本、变更即失效；缺口可观测提示补建模。
- **归因结果可信度**：小模型/算法归因可能给出误导性驱动因子。缓解：归因回传附样本与置信度、结论文字标注口径、允许用户下钻校验，不将归因结论当作确定性事实。
- **人机确认延迟体验**：危险操作暂停打断流。缓解：仅危险级触发、确认卡片即时渲染、token 有时效。
- **子代理委派开销**：多子代理增加往返与 token 与延迟（管理开销）。缓解：默认 isolated/summary 压缩上下文、MaxDepth 限深 + 并发上限、独立子任务并行摊平延迟、简单单线任务不委派（相当比例系统单代理即可）。
- **委派契约错配**：NL 委派易生成错误任务规格。缓解：校验型信封（缺必填即拒）、返回契约声明期望 `return` 形状、失败按 handle 重试。
- **硬工具门误伤**：策略过严阻断合法调用。缓解：拦截留痕可观测、白名单/scope 可配、被拒信息回灌 LLM 以自我修正。

## Migration Plan

1. **Phase 0（地基）**：Result Store + 观察压缩 + 信封契约，先在旧 PER 上验证不破坏现有行为。
2. **Phase 1**：`get_schema` 有界（词法）选表、历史窗口/摘要、token 预算治理。
3. **Phase 1.5（指标语义层）**：`agent_semantic_*` 语义模型契约 + NL2Semantics 查询路径 + Verified/Golden Query 语义缓存 + Neo4j 术语 grounding；词法选表降级为回退。
4. **Phase 2**：新 `agent-data-agent` ReAct preset + 意图路由，双跑对比旧 preset。
5. **Phase 3**：`render_ui` 工具 + 交互组件 + `ui` 事件即时化。
6. **Phase 3.5（洞察归因）**：`data_analysis` 分化趋势/对比/归因/报告，归因经 Python 算法协同。
7. **Phase 4**：操作工具（mutate/etl/export）+ 危险分级 + dry-run + 审计表（`migrate-db` 同步）。
8. **Phase 5**：人机确认 resume 端点 + 前端确认卡片。
9. **Phase 6**：Skill 硬工具门 + 会话 scope。
10. **Phase 7**：数据子代理（含 Insight/Report）注册与委派。
11. **切换**：主入口迁移到 Data Agent，旧 preset 保留兼容。

## Open Questions

- 危险分级的默认影响行数阈值取值？（建议先设保守默认并可配）
- `pending_action_id` / result TTL 的默认存活时长？
- 会话 scope 的授予来源（用户角色 / 显式开关 / skill 声明）如何确定优先级？
- 并行委派的默认并发上限取值？（建议保守默认如 3，可配）
- token 预算 pre-rot 触发点默认值？（业界经验 128K–200K，需结合所用模型窗口设定）
- 指标语义模型的写入来源（seed 文件 / 内部接口 / 从既有 BI 元数据导入）如何优先？建模台是否后续独立 change？
- Verified/Golden Query 的运营方式（人工录入 / 从历史高频问答沉淀）与审核门槛？
- 归因算法归属：复用 cognida-python 既有 `tools/analytics/` 还是新增归因专用模块？
