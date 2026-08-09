# data-agent Specification

## Purpose
TBD - created by archiving change data-agent-evolution. Update Purpose after archive.
## Requirements
### Requirement: 单一 ReAct 编排循环

系统 SHALL 提供一个 Data Agent preset（`agent-data-agent`），以单一 ReAct（Reason-Act-Observe）循环替代固定的 Plan-Execute-Reflect 流水线。LLM MUST 在每一步自主决定下一个动作（查询 / 分析 / 渲染 / 操作 / 结束），而非按预设阶段顺序执行。循环 MUST 受最大迭代次数（maxIter）与 token 预算共同约束。

#### Scenario: LLM 动态决定工具顺序

- **WHEN** 用户请求"查出上月各区域销售额并画趋势图"
- **THEN** Agent SHALL 先调用 `sql_execute` 获取数据，再调用 `data_analysis` 或直接 `render_ui` 绘图，顺序由 LLM 依据观察结果决定
- **AND** 系统 SHALL NOT 强制经过固定的 Plan → Execute → Reflect 三阶段

#### Scenario: 达到最大迭代次数时终止

- **WHEN** ReAct 循环连续执行到 maxIter 上限仍未产出最终答复
- **THEN** Agent SHALL 停止继续调用工具
- **AND** SHALL 向用户返回已获得的部分结果与"已达最大步数"的说明

#### Scenario: token 预算耗尽时终止

- **WHEN** 循环累计消耗接近配置的 token 预算上限
- **THEN** Agent SHALL 停止发起新的工具调用并进入收尾
- **AND** SHALL 依据当前 Scratchpad 中的结果信封生成最终答复

### Requirement: 四类能力统一编排

Data Agent SHALL 在同一会话循环内统一编排四类能力：查询（SQL 读）、分析（统计/洞察）、渲染（生成式 UI）、操作（写库/ETL/导出）。系统 MUST 通过工具注册向 LLM 暴露这四类能力，且各能力间通过 Result Store 的 `result_id` 传递数据引用而非原始行。

#### Scenario: 跨能力引用同一结果集

- **WHEN** `sql_execute` 产出结果信封（含 `result_id`）后，LLM 决定对其做分析
- **THEN** Agent SHALL 以该 `result_id` 调用 `data_analysis`，而非把原始行重新贴回提示词
- **AND** 分析工具 SHALL 从 Result Store 按 `result_id` 取回完整数据

#### Scenario: 无写权限场景屏蔽操作能力

- **WHEN** 会话 scope 为只读（read）
- **THEN** Data Agent SHALL NOT 向 LLM 暴露或允许执行 `sql_mutate`/`etl_run`
- **AND** 若 LLM 仍尝试调用，系统 SHALL 依据 [skill-tool-policy](../skill-tool-policy/spec.md) 硬门禁拦截

### Requirement: 作为子代理委派指挥官

Data Agent SHALL 作为指挥官（commander），可经运行时委派工具 `delegate_to_agent` 将子任务派发给专职数据子代理（SchemaExplorer/SQLAuthor/Analysis/Operation/Viz），且仅接收子代理回传的紧凑 handle 或摘要，而非其完整对话历史。

#### Scenario: 委派探查 schema 并只收摘要

- **WHEN** 指挥官需要理解一批陌生表的结构
- **THEN** 指挥官 SHALL 通过 `delegate_to_agent` 委派 SchemaExplorer 子代理
- **AND** 指挥官上下文 SHALL 仅追加子代理回传的结构摘要，而非其内部逐轮工具输出

