## Why

Text2SQL Agent 现为**固定 Plan-Execute-Reflect 流水线**、**只读**，且上下文"裸喂"（全库 schema、上千行原始结果、全量历史逐字回灌 LLM），无法支撑"查询 → 分析 → 渲染 → 操作"的动态数据工作流，长循环必然爆窗并诱发数值幻觉。分析工具（5 类 MCP）与 A2UI 生成式 UI 均已就绪，欠缺的是把它们统一到一个能**动态编排、按引用管理上下文、并安全操作数据**的 Data Agent。完整设计见 `docs/go/data-agent-plan.md`。

## What Changes

- 内核：固定 PER 流水线 → **单一 ReAct 动态编排循环**（新 preset `agent-data-agent`），入口做**意图分类路由**（取数/趋势/归因/报告），LLM 自主决定查/析/渲/操顺序。**BREAKING**：Text2SQL 主入口迁移到 Data Agent（旧 preset 保留兼容）。
- 上下文工程（地基）：引入 **Result Store** 实现端到端 data-by-reference——工具回灌 LLM 的是"结果信封（result_id + 列 + 行数 + 样本 + 聚合）"而非原始行；新增 schema 有界（词法）选表、观察压缩、历史窗口/摘要、token 预算治理。
- 查询地基（垂类正统）：引入**指标语义层（NL2Semantics）**——受治理的指标/维度语义模型 + 指标引擎生成 SQL，替代裸 NL2SQL 为主路径；**Verified/Golden Query 语义缓存**优先返回受信结果；复用 Neo4j 知识图谱/血缘做业务术语 grounding；语义模型未覆盖时回退词法 NL2SQL。
- 洞察归因（垂类差异化）：`data_analysis` 分化为趋势/对比/**归因-根因**/报告解读，归因经 Go 编排 + link-python 算法（大小模型协同，driver ranking + variance decomposition）。
- 渲染即工具：A2UI 从"流末尾一次性 compose"改为 `render_ui` 工具，支持**流中多次渲染**与交互式组件（Button/Confirm/Form/Filter/Pagination）。
- 操作数据（新）：新增 `sql_mutate`（增删改）、`etl_run`（派生 `agent_etl_*` 新对象，红线禁改原始表）、`data_export`（CSV/Excel）；配**危险分级 + 事务内 dry-run + idempotency_key**。
- 人机确认：危险操作产出 `pending_action_id`，经前端确认卡片 + 携 token 的 follow-up **resume** 后才落库。
- Skill 硬工具门：skill 的 `allowed_tools/disallowed_tools` 从"文档提示"升级为**执行前硬拦截**，叠加会话 scope（read/write/etl）实现最小权限。
- Subagent 委派：注册 SchemaExplorer/SQLAuthor/Analysis/Operation/Viz + 分层 Insight/Report 子代理作为**上下文防火墙**（isolated/summary 模式），Report→Insight→Query 逆向拆解正向执行，指挥官经 `delegate_to_agent` 运行时委派、只收 handle/摘要；支持独立子任务并行 fan-out（并发上限护栏）与治理目录。
- 审计：新增 `agent_operation_audit` 表（GORM model + `migrate-db` 同步）记录所有写/被拦截操作。

## Capabilities

### New Capabilities
- `data-agent`: 单一 ReAct Data Agent 编排器，统一查询/分析/渲染/操作四类能力，入口意图路由，并作为子代理委派的指挥官。
- `agent-semantic-layer`: 指标语义模型契约 + NL2Semantics 查询主路径 + Verified/Golden Query 语义缓存 + Neo4j 术语 grounding；未覆盖回退词法 NL2SQL。
- `agent-insight-attribution`: 意图分类路由、归因/根因（大小模型协同）、Query/Insight/Report 分层协作。
- `agent-result-store`: data-by-reference 结果存储（Redis 后端）与"结果信封"契约，供全部数据工具读写。
- `data-operation-tools`: 写库 / ETL / 导出工具，含危险分级、dry-run、派生表红线与人机确认协议。
- `generative-ui-rendering`: A2UI 作为 Agent 工具（`render_ui`），流中多次渲染 + 交互式组件回调驱动多轮。
- `agent-context-engineering`: schema 有界（词法）选表、观察压缩、历史窗口/摘要、分层提示词与 token 预算治理。
- `skill-tool-policy`: skill `allowed_tools/disallowed_tools` 执行前硬拦截，叠加会话 scope 实现最小权限。

### Modified Capabilities
- `agent-tools`: `sql_execute`/`get_schema` 改为写入 Result Store 并回传 result_id 信封（不再把原始行灌回 LLM）；工具执行前受 `skill-tool-policy` 门禁。
- `agent-collaboration`: 新增数据域子代理注册（含 Insight/Report）、治理目录、并行委派与并发护栏，及"校验型委派信封 + 紧凑 handle 回传"契约，子代理默认 isolated/summary 上下文隔离。

## Impact

- **link-go**：`internal/service/agent/` 新增 `presets/data_agent/`、`resultstore/`、`context/`；改造 `framework/eino_agent.go`（观察压缩 + 工具门）、`genui/`（Catalog 扩展 + render_ui）、`tools/`（新增 mutate/etl/export/render_ui，改造 sql_execute/get_schema）、`skills/`（硬工具门）、`initializer/init.go` 与 `cmd/server/main.go`（注册子代理）。
- **HTTP/SSE**：`internal/handler/agent_handler.go` 的 `ui` 事件即时化、confirm-resume 端点、子代理名标注。
- **数据库**：新增 `agent_operation_audit` 表与 `agent_semantic_*` 语义模型表（`cmd/migrate-db` 同步）。
- **link-web**：`components/agent/A2UINode.vue`（交互回调）、`A2UIRenderer.vue`、`views/ai/AICenterView.vue`（多 UI surface + 确认卡片）。
- **link-python**：计算侧算法不变，`tools/analytics/` 按 result_id 契约对接归因 API。
- **依赖**：Redis（Result Store / pending-action / verified-query 缓存）、Neo4j（知识图谱/血缘做语义 grounding）。指标语义层为词法选表之上的主查询路径，Milvus 向量选表列 future。
