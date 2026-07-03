# Data Agent 演进方案（Text2SQL → 可渲染 / 分析 / 操作的数据智能体）

> 状态：设计方案（待实施）
> 目标：把只读的 Text2SQL Agent 升级为真正的 **Data Agent**——能查询、分析、渲染、**操作**数据的动态智能体。
> 关联文档：[text2sql-architecture.md](./text2sql-architecture.md)、[genui-implementation.md](./genui-implementation.md)、[data-agent-market-research.md](../data-agent-market-research.md)

---

## 1. 现状盘点

| 能力 | 现状 | 位置 |
|------|------|------|
| **查询** 数据 | ✅ Text2SQL，只读 SQL | `internal/service/agent/presets/text2sql/`，工具 `sql_execute`、`get_schema` |
| **分析** 数据 | ✅ 5 个 MCP 分析工具（describe/trend/anomaly/correlation/insight），Python 算、Go 路由 | `link-python/tools/analytics/`，Go 侧 `tools/data_analysis.go` |
| **渲染** 数据 | ✅ A2UI 生成式 UI，SQL+分析结果真实融合，7 种组件 | `internal/service/agent/genui/`，前端 `A2UIRenderer.vue` |
| **操作** 数据 | ❌ 完全没有（无写 SQL、无导出、无确认流） | — |

### 结构性约束（必须先破）
- **固定 3 段流水线** Plan→Execute→Reflect（`text2sql.go:155-221`），LLM 无法自主决定"先分析、再渲染、再操作"的动态流。
- 渲染是 handler 在流末尾**一次性**调 `genui.Compose`（`agent_handler.go:120-127`）。
- 上下文基本"裸喂"（见第 4 节实测）。

### 关键决策（已与需求方确认）
- **内核**：换成单一 ReAct 动态工具编排循环（替换固定 PER）。
- **操作范围**：数据导出 + ETL/清洗（派生新表/视图，不动原始表）+ DML 写库（增删改）；**不含 DDL 建表改表**。
- **确认策略**：分级——危险操作才需人工确认，白名单安全操作可直接执行。

---

## 2. 目标能力模型

```
                    ┌─────────────────────────────┐
                    │        Data Agent           │
                    │   (单一 ReAct 编排循环)      │
                    └──────────────┬──────────────┘
          ┌───────────────┬────────┼────────┬───────────────┐
          ▼               ▼        ▼         ▼               ▼
       查询 Query     分析 Analyze  渲染 Render  操作 Operate   (上下文工程贯穿)
     sql_execute   data_analysis  render_ui   sql_mutate      Result Store
     get_schema    (5 类分析)      (A2UI)      data_export     Schema 检索
                                              etl_run          Scratchpad/预算

  横切：Skill（领域 playbook + 工具策略层）  ·  Subagent（专职子代理 = 上下文防火墙）
```

---

## 3. 上下文工程（Phase 0 · 地基 · 优先）

> 一个能"操作数据"的 Agent 循环更长、工具更多、结果集更大，裸喂必爆窗并诱发数值幻觉。上下文工程是所有其它 Phase 的公共底座。

### C1. Result Store —— 端到端 data-by-reference（最重要）
把 genUI 已有的"数据按引用"理念**贯通到 LLM 推理侧**。
- 新增 `internal/service/agent/resultstore/`（Redis 后端，符合存储约定）。`sql_execute` / `etl_run` 把**全量结果集**写入，返回 `result_id`。
- **回灌 context 的不是行，而是"结果信封"**：`result_id` + 列名 + dtype + `row_count` + 前 N 行样本（如 5 行）+ 关键聚合（min/max/null 数）。LLM 只看信封。
- 下游工具（`data_analysis` / `render_ui` / `sql_mutate` / `data_export`）吃 `result_id` → 直接从 store 取全量，**不经 LLM**。
- 收益：窗口占用从 ~500KB/轮 降到 ~1KB/轮；消除数值幻觉；A2UI 变成真·按引用渲染。

### C2. Schema 检索（不再 dump 全库）
- 三层：①**目录卡**（表名+一行注释+行数，极简，可常驻）；②**语义选表**——把 表/列/注释 向量化进 Milvus（已有基建），按问题检索 Top-K 表，只注入相关表 DDL；③`get_schema` 保留做按需下钻。
- 加一步 **schema linking**：问题实体 → 表/列映射，写入 scratchpad。
- Schema 快照缓存（Redis）。

### C3. 结构化 Scratchpad + 观察压缩
- ReAct 循环内维护**结构化工作记忆**（已发现的表、执行过的 SQL 日志、`result_id` 列表、待确认操作），与原始 message 列表分离。
- **观察压缩**：旧工具结果消息被用过后，替换为"一行摘要 + handle"，保持循环精简（改造 `eino_agent.go` 的 messages 累加逻辑，`eino_agent.go:1012-1018`）。

### C4. 多轮历史：窗口 + 摘要 + 会话工作集
- 滚动窗口（近 K 轮逐字）+ 旧轮次 LLM 压缩成"对话摘要"；复用现有 `ContextBuilder` / `WithAutoCompress` 钩子。
- 维护紧凑的**会话工作集**（已知表、上次 `result_id`、上次 SQL、数据域）常驻，而非回放全文（现状为全量回放，`eino_agent.go:245-264`）。

### C5. 分层动态提示词 + Token 预算治理
- 提示词分层：静态核心（角色/能力/分级安全规则）+ 动态注入（方言、选中的 schema 切片、工具目录、会话工作集、按任务类型选的 few-shot）。当前 text2sql 提示词是纯静态字符串（`text2sql.go:22-87`），改成模板化组装。
- **Token 预算分配器**：总窗口 → 给 system/schema/history/scratchpad/live-output 按优先级分配；超预算按优先级压缩/驱逐；输出各段 token 遥测。

---

## 4. Skill 体系集成

现有 skill 系统（`internal/service/agent/skills/`）已具备：SKILL.md（YAML frontmatter + markdown 正文）、注册表 + 相关度匹配（`registry.go:273-356`）、`AutoSkillMiddleware`（推理前自动注入 Top-3，阈值 0.3）、`ManualSkillMiddleware`（`skill_invoke` 工具）、`allowed_tools/disallowed_tools` 工具白/黑名单。已存在 `skills/data-analysis/SKILL.md`。

Data Agent 把 skill 当作**领域 playbook + 工具策略层**：

### S1. 数据领域 playbook（纯指导）
- 每个业务分析范式一个 skill：`financial-reporting`（涉及哪些表、KPI 口径、MoM/YoY 算法）、`cohort-analysis`、`funnel-analysis`、`data-cleaning-playbook`。
- 靠 `when_to_use` + tags 命中 matcher；`AutoSkillMiddleware` 推理前注入 Top-K → 成为 C5 分层提示词的一层（占独立 token 预算）。
- SKILL.md 的 `examples/` 目录供 C5 few-shot 选择。

### S2. 工具策略层（least privilege）—— 需补强
- skill 的 `allowed_tools/disallowed_tools` 用作**操作权限门**：如 `readonly-analysis` 禁用 `sql_mutate/etl_run`；`safe-mutation` 描述分级确认协议并放开写工具。
- **现状 gap**：allowed_tools 目前是"policy 层、未强制"（探查结论）。Data Agent 能改数据，**必须把它变成硬约束**——工具执行前按激活 skill 的白/黑名单拦截。落到 Phase 8 治理。
- skill 可调节 Phase 4 危险阈值（如 `production-db` skill 把所有写操作提到"危险"级）。

### S3. 与内核/预算的关系
- `maxSkills`（默认 3）纳入 token 预算治理（C5）；skill 注入体量参与分段遥测。
- 数据域 skill 以"纯指导 + allowed_tools 策略"为主，不自执行工具。

---

## 5. Subagent 架构

现有基建已支持**静态编排 + 运行时委派**：`orchestration/`（Sequential/Parallel/Conditional/Supervisor/Route）、运行时 `DelegateTool/AskTool/HandoffTool`（`framework/collab_tools.go`）、`CollaborationRegistry`、`CollaborationContext`（含环检测 `IsCyclic`、深度上限 `MaxDepth`、上下文模式 none/summary/recent/full/isolated）。

Data Agent 采用**单一 ReAct 主控 + 选择性委派**的混合模式：

### SA1. 主控 + 专职子代理
- 顶层一个 **Data Agent（ReAct 指挥官）**，面向用户、持有编排。
- 在 `CollaborationRegistry` 注册专职子代理，经 `DelegateTool` 运行时委派：
  - **SchemaExplorerAgent** — 重的 schema linking / 选表（C2），isolated 上下文，只回压缩后的表候选清单。
  - **SQLAuthorAgent** — 复杂 SQL 撰写（多表 join / 窗口 / CTE），自带重试环。
  - **AnalysisAgent** — 驱动 5 个分析工具并解读结果。
  - **OperationAgent** — 写 / ETL 专家，独占危险分级 + dry-run + 确认协议（Phase 3/4）。
  - **VizAgent** — 选型并生成 A2UI 布局（Phase 2/5）。

### SA2. 子代理 = 上下文防火墙（核心动机）
- 每个子代理以 `mode=isolated/summary` 运行，其**冗长中间推理**（schema dump、失败的 SQL 尝试、分析中间量）**不污染指挥官窗口**，只回压缩结论。这是对 Phase 0 上下文工程的直接补强——用委派换取上下文隔离。
- 判据：默认单循环工具调用；**仅当子任务需独立长推理、需最小权限隔离（写操作）、或可并行 fan-out 时**才委派子代理。

### SA3. 最小权限 + 安全
- **只有 OperationAgent 持有写工具**（`sql_mutate/etl_run`），经 S2 的 skill 工具门强制；其余子代理只读。
- 复用 `CollaborationContext` 的 `IsCyclic` + `MaxDepth` 防环 / 防深递归。

### SA4. 并行与流式
- 多部分问题（"对比 A/B 区并预测"）→ 指挥官 `Parallel` 委派多个 AnalysisAgent 后合并（`parallel.go` 聚合）。
- 子代理流已能透传到父 SSE（`sequential.go:47-84`）；需给 chunk 打上子代理名，前端时间线展示 `SchemaExplorer → SQLAuthor → Operation` 的委派链。

---

## 6. 现状上下文实测（风险依据）

| 维度 | 现状 | 风险 | 位置 |
|------|------|------|------|
| Schema 检索 | 不带表名返回**全库**所有表所有列，无检索无预算 | 中（表多则不可控） | `get_schema.go:103-108` |
| SQL 结果 | 上限 1000 行，**原始 JSON 逐字回灌**，每轮累加 | **高** | `sql_execute.go:26-27,91-97`；`eino_agent.go:1012-1018` |
| 分析结果 | 全量 JSON（相关性矩阵/预测数组）逐字回灌 | **高** | `data_analysis.go:139-155` |
| 多轮历史 | 默认全量回放，无摘要/窗口 | **高** | `eino_agent.go:245-264` |
| 系统提示词 | 纯静态字符串，无 schema/上下文注入 | 低 | `text2sql.go:22-87` |
| Scratchpad | 仅 in-memory messages 数组，单次运行内有效 | 低 | `eino_agent.go:826-1032` |
| Result 引用 | **无 result store**，全量数据穿过 LLM | **高** | genUI 仅渲染侧按引用 |

---

## 7. 分阶段计划

### Phase 0 —— 上下文工程（地基，先行）
C1 Result Store → C2 语义选表 → C3 Scratchpad/观察压缩 → C4 历史窗口/摘要 → C5 分层提示词/预算。详见第 3 节。

### Phase 1 —— Agent 内核：单一 ReAct 循环（替换 PER）
- 新建 `internal/service/agent/presets/data_agent/data_agent.go`，用 eino `ToolCallingChatModel` 做动态循环（`maxIter` ≈ 8–10），持有全部工具。
- 系统提示词定义四类能力（查/析/渲/操）+ 分级确认规则 + ETL 只写派生表红线。
- 注册新 agentID `agent-data-agent`；`text2sql` preset 保留但不再是主入口。
- 初始化挂到 `service/agent/initializer/init.go`。
- 作为 Subagent 委派的**指挥官**（详见 Phase 7）：`WithCollaboration(registry).EnableDelegate()`。
- 复杂度 >200 行 → 多 Agent 并行开发。
- 依赖 C1/C3/C5。

### Phase 2 —— 渲染即工具（流中多次渲染）
- 新增 `tools/render_ui.go`，内部复用 `genui.Compose`，输入 `result_id`（从 store 取数）。
- 改造 `agent_handler.go:120-127`：从"末尾一次性 compose"改为识别 `render_ui` 工具输出 → 即时发 `ui` SSE 事件；支持一会话多 UI surface。
- 依赖 C1。

### Phase 3 —— 操作工具集（分级安全，不含 DDL）
- `tools/sql_mutate.go` —— INSERT/UPDATE/DELETE。SQL 解析分级：无 WHERE 的 UPDATE/DELETE、影响行数超阈值 → **危险**，走确认；其余可直接执行。事务内 dry-run 拿 affected rows，带 `idempotency_key`。
- `tools/data_export.go` —— 结果集导出 CSV/Excel，读 `result_id`，低风险直执行。
- `tools/etl_run.go` —— `CREATE TABLE AS SELECT` / 视图做清洗转换，**只允许派生新对象**（前缀如 `agent_etl_*`），红线拦截对原始业务表的写入。
- 审计表 `agent_operation_audit`（GORM model + `go run ./cmd/migrate-db` 同步，遵循 CLAUDE.md 表结构同步规则）。
- 依赖 C1。

### Phase 4 —— 分级确认协议（Human-in-the-loop）
- 危险操作 → Agent 产出 pending action → 前端渲染 A2UI `Confirm` 卡片 → 用户确认 → 携 action token 的 follow-up 请求 resume → commit。
- pending-action 存 Redis（符合存储约定）；安全操作跳过此环。

### Phase 5 —— 交互式 A2UI
- `genui/spec.go` Catalog 扩 `Button`/`Confirm`/`Form`/`Filter`/`Pagination` + 行内 action；`validate.go` 同步白名单。
- 前端 `A2UINode.vue` 支持组件回调 → 发起 follow-up query（下钻、分页、确认多轮联动）。

### Phase 6 —— Skill 体系（领域 playbook + 工具策略）
- 新增数据域 SKILL.md：`financial-reporting` / `cohort-analysis` / `funnel-analysis` / `data-cleaning-playbook` / `readonly-analysis` / `safe-mutation`（详见第 4 节 S1/S2）。
- Data Agent 挂 `AutoSkillMiddleware`（Top-K 注入，纳入 C5 token 预算）+ `ManualSkillMiddleware`（`skill_invoke`）。
- skill 的 `allowed_tools/disallowed_tools` 参与 Phase 8 的硬工具门。

### Phase 7 —— Subagent 委派（专职子代理 = 上下文防火墙）
- 在 `CollaborationRegistry` 注册 SchemaExplorer / SQLAuthor / Analysis / Operation / Viz 五类子代理（详见第 5 节）。
- 指挥官启用 `DelegateTool`；子代理默认 `mode=isolated/summary` 隔离上下文，只回压缩结论。
- 复用 `CollaborationContext` 的 `IsCyclic` + `MaxDepth`；chunk 打子代理名，前端展示委派链。
- OperationAgent 独占写工具，落实最小权限。

### Phase 8 —— 治理 / 可观测
表 allowlist、行数上限、事务包裹、审计落库、per-tool read/write/etl scope、限流、request_id 全链路、token 分段遥测。**补强 skill `allowed_tools` 为硬约束**（工具执行前拦截，见第 4 节 S2 gap）。

### Phase 9 —— 测试 + Review + 提交
- 单测（工具分级判定逻辑、结果信封生成、schema 选表）。
- 集成测试（真实 MySQL 写入/回滚/ETL 派生表）。
- API 测试（SSE 确认 resume 流）。
- 触发 `code-review` skill。
- 文档更新为 `data-agent-architecture.md`。
- 任务完成后终止所有服务进程（CLAUDE.md 强制规则）。

---

## 8. 依赖与推荐顺序

```
Phase 0  上下文工程（C1 Result Store → C2 选表 → C3/C4/C5）   ← 地基，先行
Phase 1  ReAct 内核（单一循环 + 委派指挥官）  ┐ 依赖 C1/C3/C5
Phase 2  渲染即工具（读 result_id）           ┘ 依赖 C1
Phase 3  操作工具（写/读 result_id）            依赖 C1
Phase 4  分级确认协议
Phase 5  交互式 A2UI
Phase 6  Skill 体系（playbook + 工具策略）      依赖 C5 预算
Phase 7  Subagent 委派（上下文防火墙）          依赖 Phase 1/3，补强 Phase 0
Phase 8  治理/审计/可观测（含 skill 硬工具门）
Phase 9  测试 + Review + 提交
```

**关键依赖**：C1（Result Store）是所有其它 Phase 的公共底座，**第一个做**。内核（Phase 1）一落地就跑在按引用的上下文上，后续操作/渲染全部复用同一 store，不返工。

**建议里程碑**：先做 **C1 + Phase 1 + Phase 2**（动态内核 + 按引用上下文 + 流中渲染，风险可控、可立即验证），再攻 Phase 3/4 的写操作安全。

---

## 9. 涉及文件索引

**Go 后端**
- 内核/preset：`internal/service/agent/presets/data_agent/`（新）、`presets/text2sql/text2sql.go`（保留）
- 框架：`internal/service/agent/framework/eino_agent.go`（观察压缩、消息累加改造）、`collab_tools.go` + `collaboration_registry.go`（子代理委派，复用）
- 编排：`internal/service/agent/orchestration/`（Sequential/Parallel/Supervisor，复用于并行 fan-out）
- 工具：`internal/service/agent/tools/`（新增 `render_ui.go`、`sql_mutate.go`、`data_export.go`、`etl_run.go`；现有 `sql_execute.go`、`get_schema.go`、`data_analysis.go` 接 result_id）
- 上下文：`internal/service/agent/resultstore/`（新）、`internal/service/agent/context/`（预算/压缩，新）
- Skill：`internal/service/agent/skills/`（`middleware.go` 挂载、`agent_integration.go` 注入、`allowed_tools` 硬门补强）；skill 内容目录 `skills/`（新增数据域 SKILL.md）
- 渲染：`internal/service/agent/genui/`（`spec.go` Catalog 扩展、`validate.go`）
- Handler：`internal/handler/agent_handler.go`（SSE `ui` 事件即时化、confirm resume、子代理名标注）
- 初始化：`internal/service/agent/initializer/init.go`、`cmd/server/main.go`（注册子代理到 CollaborationRegistry）
- 审计表：新增 GORM model + `cmd/migrate-db` 同步

**前端 link-web**
- `src/components/agent/A2UINode.vue`（交互组件回调）、`A2UIRenderer.vue`、`a2ui-context.ts`
- `src/views/ai/AICenterView.vue`（多 UI surface、确认卡片）

**Python link-python**（计算侧不变，按 result_id 契约对接）
- `link-python/tools/analytics/`、`services/analytics/`

---

## 10. 附录 A —— Subagent Prompt 与委派契约

### A0. 委派信封（指挥官 → 子代理）
现有 `DelegateTool` 的入参是 `{agent_name, task}`（`collab_tools.go`）。约定 `task` 承载一个**紧凑 JSON 信封**，避免把原始数据塞进委派串：

```json
{
  "goal": "本次子任务的一句话目标",
  "inputs": { "result_id": "rs_xxx", "sql": null, "question": "用户原始问题(可选)" },
  "constraints": { "scope": "read|write|etl", "max_rows": 1000 },
  "return": "期望回传的结构(见各子代理 output 契约)"
}
```

**通用回传原则**：子代理**只回压缩结论 + handle（result_id / surface_id / pending_action_id）**，绝不回原始行。指挥官上下文只增长 ~几百 token/次委派。`CollaborationContext.mode` 默认 `isolated`（重探索类）或 `summary`（需原问题背景类）。

构建方式统一走现有 builder：
```go
infraagent.New(toolModel).Name("<Name>").Prompt(<prompt>).
    Tools(<该子代理允许的工具>).WithMaxIterations(<n>).Build(ctx)
// 注册：registry.Register(id, agent, capabilities, description)
// description 直接决定指挥官何时委派它 —— 要写清"何时用/输入/产出"
```

### A1. SchemaExplorerAgent
- **职责**：schema linking 与选表（C2），把"全库"收敛成 3–8 张相关表。
- **工具**：`get_schema`、（可选）schema 向量检索工具。**只读**。`maxIter=3`，`mode=isolated`。
- **Prompt 要点**：
  > 你是库表勘探专家。给定问题，先语义选表再取列，**只保留与问题相关的表/列**，输出候选表清单与连接键。不要生成 SQL，不要臆造不存在的表列。
- **output 契约**：
  ```json
  { "tables": [{"name":"orders","columns":["id","amount","created_at"],"why":"金额与下单时间"}],
    "joins": [{"left":"orders.user_id","right":"users.id"}],
    "notes": "created_at 为 UTC" }
  ```

### A2. SQLAuthorAgent
- **职责**：撰写并执行只读查询 SQL（多表 join / 窗口 / CTE），把结果落 Result Store。
- **工具**：`get_schema`、`sql_execute`。**只读**。`maxIter=5`，`mode=isolated`。
- **Prompt 要点**：
  > 你是 SQL 撰写专家。基于给定表结构生成方言正确的**只读** SQL 并用 `sql_execute` 执行；失败则读报错自我修正。**只回 result_id 与信封摘要，不回原始行**。
- **output 契约**：
  ```json
  { "result_id": "rs_abc", "sql": "SELECT ...", "dialect": "mysql",
    "envelope": {"columns":["month","sales"],"row_count":12,"sample":[{"month":"2024-01","sales":100}]},
    "rationale": "按月聚合销售额" }
  ```

### A3. AnalysisAgent
- **职责**：对 `result_id` 驱动 5 类分析（describe/trend/anomaly/correlation/insight）并解读。
- **工具**：`data_analysis`。**只读**（从 store 取数）。`maxIter=4`，`mode=summary`。
- **Prompt 要点**：
  > 你是数据分析专家。根据问题选择合适分析类型，调用 `data_analysis(result_id, analysis_type, ...)`，**用自然语言给出关键结论**，数值一律引用工具返回，禁止编造。
- **output 契约**：
  ```json
  { "analysis_type": "trend", "metrics_ref": "rs_abc#metrics",
    "key_findings": ["销售额线性上升，R²=0.94","近 3 期增速放缓"],
    "insight_summary": "整体上行但动能减弱" }
  ```

### A4. OperationAgent（唯一持写工具）
- **职责**：写库 / ETL 专家，独占**危险分级 + dry-run + 确认协议**（Phase 3/4）。
- **工具**：`sql_mutate`、`etl_run`、`data_export`。`maxIter=4`，`mode=summary`。**受 skill 硬工具门约束（附录 B）**。
- **Prompt 要点**：
  > 你是数据操作专家。任何写操作先解析危险级别：无 WHERE 的 UPDATE/DELETE、影响行数超阈值、写原始业务表 → **危险**，必须走 dry-run 生成预览并产出 `pending_action_id` 等待人工确认；安全操作可直接执行。ETL 只允许派生 `agent_etl_*` 新对象，红线禁止改原始表。
- **output 契约**：
  ```json
  { "operation": "UPDATE", "danger_level": "danger",
    "dry_run": {"affected_rows": 320, "preview_ref": "rs_prev"},
    "pending_action_id": "act_789", "status": "await_confirm" }
  ```

### A5. VizAgent
- **职责**：为 `result_id`(+分析) 选型并生成 A2UI 布局。
- **工具**：`render_ui`（内部复用 `genui.Compose`，按引用取数）。`maxIter=2`，`mode=summary`。
- **Prompt 要点**：
  > 你是可视化专家。依据数据形态（时序→折线、KPI→MetricCard、明细→Table）与问题意图选择布局，调用 `render_ui(result_id, question)`。**只决定布局与绑定路径，绝不输出数据值**（数值由前端按 JSON Pointer 从 dataModel 取）。
- **output 契约**：`{ "ui_surface_id": "ui_01", "gen_mode": "llm", "components": <n> }`

### A6. 指挥官侧委派提示
指挥官系统提示词需内建"何时委派"规则（配合各子代理 `description`）：
> 简单单步查询直接用工具；**遇到 (a) 需大量选表探索 → SchemaExplorer；(b) 复杂多表 SQL → SQLAuthor；(c) 深度分析 → Analysis；(d) 任何写/ETL/导出 → Operation；(e) 需可视化 → Viz**。可对独立子问题并行委派多个 Analysis 后合并。始终只依据子代理回传的 handle/摘要继续推理。

---

## 11. 附录 B —— Skill 硬工具门实现

**目标**：把 skill 的 `allowed_tools/disallowed_tools` 从"文档提示"升级为**执行前硬拦截**，与会话级 scope（read/write/etl）叠加，实现最小权限。

### B1. 策略解析（激活 skill → 有效工具集）
激活 skill 由 `AutoSkillMiddleware`/`ManualSkillMiddleware` 放入 context（现有 `contextWithSkills`）。新增解析器：

```go
type ToolPolicy struct {
    Allow    map[string]bool // 非空 => 白名单模式
    Deny     map[string]bool // 黑名单，优先级最高
    Scope    string          // 会话级：read|write|etl
}

// 合并规则：deny 覆盖 allow；任一 skill 声明 allowed_tools 即进入白名单模式（取并集）
func ResolveToolPolicy(active []*skills.Skill, scope string) ToolPolicy { ... }

func (p ToolPolicy) Permits(tool string) (bool, string) {
    if p.Deny[tool]                      { return false, "被激活 skill 的 disallowed_tools 禁止" }
    if len(p.Allow) > 0 && !p.Allow[tool]{ return false, "不在激活 skill 的 allowed_tools 白名单内" }
    if !scopeAllows(p.Scope, tool)       { return false, "超出会话 scope=" + p.Scope }
    return true, ""
}
```
`scopeAllows`：把工具映射到类别（`sql_mutate/etl_run`=write/etl，`sql_execute/data_analysis/render_ui/get_schema`=read），scope 不足则拒。

### B2. 拦截点（工具执行前）
现状工具在 eino 循环里执行后 append `ToolMessage`（`eino_agent.go:1006-1018`）。在**调用工具之前**插入门禁——被拒时**不执行**，回一条合成 `ToolMessage` 让 LLM 自适应改道：

```go
for _, tc := range toolCalls {
    if ok, reason := policy.Permits(tc.Function.Name); !ok {
        blocked := fmt.Sprintf(`{"error":"tool_blocked","tool":%q,"reason":%q}`, tc.Function.Name, reason)
        messages = append(messages,
            &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{tc}},
            schema.ToolMessage(blocked, tc.ID))
        emitAudit(ctx, tc, reason)   // 落 agent_operation_audit
        continue                      // 跳过真正执行
    }
    // ... 正常执行并 append 结果
}
```

### B3. 与 Subagent 的组合（最小权限）
- 委派时把 policy 随 `CollaborationContext` 下传；子代理构建即只挂各自允许的工具（如 SchemaExplorer 不挂 `sql_mutate`），**双保险**：结构上不给 + 运行时门禁。
- OperationAgent 的写权限来自激活 `safe-mutation` skill 放开 + 会话 `scope=write`；缺任一则 `sql_mutate` 被 B2 拦截。

### B4. 预置数据域 skill 示例
```yaml
# skills/readonly-analysis/SKILL.md
name: readonly-analysis
when_to_use: 纯查询与分析、明确不允许改动数据时
disallowed_tools: [sql_mutate, etl_run]
tags: [sql, analytics, readonly]
```
```yaml
# skills/safe-mutation/SKILL.md
name: safe-mutation
when_to_use: 需要写库/ETL 且要求分级确认时
allowed_tools: [sql_execute, get_schema, sql_mutate, etl_run, data_export]
tags: [mutation, etl, safety]
# 正文：描述危险分级、dry-run、pending 确认流程 —— 同时塑造行为 + 放开工具
```

### B5. 测试要点（Phase 9）
- 单测 `ResolveToolPolicy`：白名单/黑名单/scope 叠加与优先级。
- 单测 `Permits`：`safe-mutation` 放开而 `readonly-analysis` 拦截 `sql_mutate`。
- 集成：激活 `readonly-analysis` 时 Agent 试图改数据 → 被拦截且审计落库，LLM 收到 `tool_blocked` 后改走只读路径。
