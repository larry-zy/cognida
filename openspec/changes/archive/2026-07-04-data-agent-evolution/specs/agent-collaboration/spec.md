## ADDED Requirements

### Requirement: 数据域子代理注册

系统 SHALL 注册专职数据子代理并纳入 CollaborationRegistry：SchemaExplorer（get_schema）、SQLAuthor（get_schema + sql_execute）、Analysis（data_analysis 趋势/对比/归因/报告解读）、Operation（sql_mutate + etl_run + data_export，唯一持写工具者）、Viz（render_ui），以及分层协作的 Insight（洞察编排，委派 Analysis/Query）与 Report（报告编排，委派 Insight）。每个子代理 MUST 声明自身最小工具集与最大迭代次数。

#### Scenario: 子代理最小工具集

- **WHEN** SchemaExplorer 子代理被注册
- **THEN** 其可用工具集 SHALL 仅含 `get_schema`
- **AND** SHALL NOT 具备 `sql_mutate` 等写工具

#### Scenario: 仅 Operation 持写工具

- **WHEN** 指挥官需要执行写/ETL/导出
- **THEN** 系统 SHALL 仅允许委派给 Operation 子代理
- **AND** 其余子代理 SHALL NOT 被授予写工具

### Requirement: 子代理治理目录

CollaborationRegistry 每个注册项 SHALL 携治理元数据 `{purpose, data_scope, tools, risk_class}`，构成活体 agent 目录（用途/数据访问级/工具集/风险级），供审计与最小权限校验。子代理工具授予 SHALL 为每次委派授予、MUST NOT 跨会话累积持久高权。

#### Scenario: 注册项声明治理元数据

- **WHEN** Operation 子代理被注册
- **THEN** 其目录项 SHALL 声明 `risk_class` 为写级、`tools` 含 `sql_mutate`/`etl_run`/`data_export`
- **AND** 该元数据 SHALL 可被 `agent_operation_audit` 关联留痕

#### Scenario: 工具授予不持久化

- **WHEN** 一次写委派完成
- **THEN** Operation 的写工具授予 SHALL 随该次委派结束而失效
- **AND** SHALL NOT 在后续无关委派中默认保留

### Requirement: 委派拓扑与并发护栏

指挥官 SHALL 支持对**独立子任务并行 fan-out 委派**（如多数据源/多指标同时探查），对**依赖链串行委派**。并行委派 MUST 受**并发上限**约束，与 IsCyclic、MaxDepth 并列为资源护栏。单个失败子委派 SHALL 可按其 handle 重试而不牵连并行兄弟。

#### Scenario: 独立子任务并行委派

- **WHEN** 指挥官需对三个独立数据源分别探查 schema
- **THEN** 系统 SHALL 允许并行委派三个 SchemaExplorer
- **AND** 并行数 SHALL NOT 超过配置的并发上限

#### Scenario: 失败子委派独立重试

- **WHEN** 并行委派中某一子代理失败
- **THEN** 系统 SHALL 允许仅重试该失败委派
- **AND** SHALL NOT 要求重跑已成功的并行兄弟

### Requirement: 委派信封与紧凑回传契约

指挥官经 `delegate_to_agent` 委派子任务时，SHALL 传递结构化委派信封 `{goal, inputs:{result_id,sql,question}, constraints:{scope,max_rows}, return}`，且该信封 SHALL 作为校验型契约——缺 `goal` 或 `constraints.scope` 等必填字段时 MUST 拒绝委派并回灌错误供 LLM 自我修正。子代理 MUST 只回传紧凑 handle 或摘要（如 `result_id` + 结论），MUST NOT 把内部逐轮工具输出回灌指挥官上下文。

#### Scenario: 缺必填字段的委派被拒

- **WHEN** 指挥官发起的委派缺 `constraints.scope`
- **THEN** 系统 SHALL 拒绝该委派
- **AND** SHALL 返回契约校验错误而非以默认值放行

#### Scenario: 委派携带约束信封

- **WHEN** 指挥官委派 SQLAuthor 生成并执行查询
- **THEN** 委派 SHALL 携带 `goal`、`inputs`、`constraints{scope,max_rows}` 与期望 `return`
- **AND** 子代理 SHALL 在 `constraints` 约束内工作

#### Scenario: 子代理只回传 handle

- **WHEN** SQLAuthor 完成查询
- **THEN** 其回传 SHALL 为 `result_id` + 结果信封摘要
- **AND** 指挥官上下文 SHALL NOT 追加 SQLAuthor 的内部工具往返明细

### Requirement: 子代理默认上下文隔离

数据域子代理 SHALL 默认采用 isolated 或 summary 上下文模式作为"上下文防火墙"：探查/写作类（SchemaExplorer/SQLAuthor）默认 isolated，分析/操作/渲染类（Analysis/Operation/Viz）默认 summary。系统 MUST 沿用既有 CollaborationContext 的循环检测（IsCyclic）与 MaxDepth 约束。

#### Scenario: 隔离模式不泄漏指挥官全历史

- **WHEN** SchemaExplorer 以 isolated 模式被委派
- **THEN** 子代理 SHALL NOT 接收指挥官的完整对话历史
- **AND** 仅接收委派信封所载的必要输入

#### Scenario: 循环委派被检测拦截

- **WHEN** 委派链形成环（如 A→B→A）
- **THEN** 系统 SHALL 依据 IsCyclic 检测拦截该委派
- **AND** SHALL 返回循环检测错误而非无限递归
