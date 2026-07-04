# agent-insight-attribution Specification

## Purpose
TBD - created by archiving change data-agent-evolution. Update Purpose after archive.
## Requirements
### Requirement: 意图分类路由

Data Agent ReAct 内核入口 SHALL 对用户请求做意图分类（取数 / 趋势 / 归因 / 报告），据分类选择对应 skill playbook 与工具子集。当分类存在多个同等可能的解释时，系统 SHALL 向用户发起有针对性的反问，而非硬猜执行。

#### Scenario: 归因类问题路由到归因能力

- **WHEN** 用户问"华北区销售额为何下降 20%"
- **THEN** 系统 SHALL 判定为归因意图并选归因 playbook
- **AND** SHALL NOT 仅返回一条汇总取数

#### Scenario: 歧义意图触发反问

- **WHEN** 请求可同等解释为取数或趋势
- **THEN** 系统 SHALL 发起澄清反问
- **AND** SHALL NOT 在未澄清时硬选一路执行

### Requirement: 归因/根因作为一等分析能力

`data_analysis` SHALL 分化为命名分析能力：趋势、对比、归因/根因、报告解读。归因/根因 SHALL 支持 variance decomposition 与 driver ranking，由 Go 侧编排 + link-python 侧算法（归因 API）协同（大小模型协同）。归因结果 SHALL 回传 `result_id` 信封 + 文字洞察，并附样本与口径标注。

#### Scenario: 归因经大小模型协同

- **WHEN** 指挥官发起归因分析
- **THEN** Go 侧 SHALL 编排调用 Python 归因算法而非让 LLM 心算驱动因子
- **AND** 结果 SHALL 含 driver ranking 与文字洞察

#### Scenario: 归因结果不被当作确定性事实

- **WHEN** 归因产出驱动因子排名
- **THEN** 回传 SHALL 附样本与置信/口径标注
- **AND** SHALL 支持用户下钻校验

### Requirement: Query/Insight/Report 分层协作

子代理体系 SHALL 补 Insight（洞察）与 Report（报告）两类，与 Query 类形成 Report → Insight → Query 的**逆向拆解、正向执行**协作：Report 声明所需洞察，Insight 声明所需数据，Query 取数回填。上层子代理 MUST 只接收下层的 handle/摘要，SHALL NOT 内联下层逐轮明细。

#### Scenario: 报告请求逆向拆解

- **WHEN** 用户请求"做一份业绩复盘"
- **THEN** Report 子代理 SHALL 向 Insight 声明所需洞察项
- **AND** Insight 子代理 SHALL 向 Query 声明所需数据集

#### Scenario: 分层只回传摘要

- **WHEN** Query 完成取数
- **THEN** 其回传给 Insight 的 SHALL 为 `result_id` + 信封摘要
- **AND** Report 上下文 SHALL NOT 追加 Query 的内部工具往返明细

