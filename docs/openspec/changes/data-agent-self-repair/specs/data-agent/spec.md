## ADDED Requirements

### Requirement: 自我修复纪律

Data Agent 的系统提示 SHALL 声明明确的错误修复协议：拿到含 `error_kind`/`hint` 的失败观察时，SHALL 先据此诊断根因，再针对性修正后重试，MUST NOT 不加分析地重复同一失败调用。当同类错误连续两次修正仍未解决时，提示 SHALL 要求 LLM 改变策略（换表/换口径/改用其他能力/反问用户澄清），而非继续原路径。各意图 playbook（取数/趋势/归因/报告/通用）SHALL 与该修复纪律一致，不得引导盲目重试。

#### Scenario: 依据 hint 修正后重试

- **WHEN** LLM 观察到 `error_kind = "unknown_column"` 且 `hint` 含可用列
- **THEN** LLM SHALL 按 `hint` 改写 SQL 后重试
- **AND** SHALL NOT 原样重复失败的 SQL

#### Scenario: 两次未修复则改变策略

- **WHEN** 同类错误连续两次修正仍失败
- **THEN** LLM SHALL 改变策略（换表/换口径/反问用户）
- **AND** SHALL NOT 继续第三次同路径重试

### Requirement: 触发式动态重规划

Data Agent 的 ReAct 内核 SHALL 支持由重复失败触发的动态重规划：当循环护栏（见 [agent-error-repair](../agent-error-repair/spec.md)）判定同一失败签名达到阈值时，编排 SHALL 接收注入的重规划提示并据此调整后续动作，而非机械沿用原 playbook 路径。重规划 SHALL 在既有 `maxIter`/token 预算约束内进行，MUST NOT 突破全局循环上限。

#### Scenario: 收到重规划提示后调整路径

- **WHEN** 循环护栏注入换策略重规划提示
- **THEN** Data Agent SHALL 在后续步骤改变工具选择或口径
- **AND** SHALL 在 `maxIter`/token 预算内完成

#### Scenario: 重规划失败仍诚实收尾

- **WHEN** 重规划后仍无法完成且预算耗尽
- **THEN** Data Agent SHALL 基于已有观察给出部分结论
- **AND** SHALL 诚实说明未完成的部分与原因

### Requirement: 复杂任务下沉与简单任务直连分流

Data Agent SHALL 按任务复杂度分流：简单任务（单步取数/单图渲染/单步分析）主 agent SHALL 直接调用相应工具完成，MUST NOT 强制引入委派跳数；复杂任务（多维归因、经营报告等多步编排）SHALL 经命中的可执行 skill 在其 handler 内 inline 编排子代理群完成（见 [agent-skill-runtime](../agent-skill-runtime/spec.md)），只向主循环回传紧凑 handle/摘要。修复据此分两层：子代理内做战术修复（依 `error_kind`/`hint` 改写重试），主 agent/编排层做战略重规划（换子代理/换口径/反问）。主 + 各子代理 systemPrompt SHALL 一致声明修复纪律。

#### Scenario: 简单任务主 agent 直连

- **WHEN** 用户请求一次简单单步取数
- **THEN** 主 agent SHALL 直接调用查询工具完成
- **AND** SHALL NOT 因下沉机制引入额外委派跳数

#### Scenario: 复杂任务经 skill handler 下沉

- **WHEN** 用户请求多维归因这类复杂任务并命中下沉 skill
- **THEN** 该 skill handler SHALL inline 编排子代理群完成
- **AND** 主循环 SHALL 只收到紧凑 handle/摘要

#### Scenario: 修复分两层各司其职

- **WHEN** 下沉执行中某子代理遇 `unknown_column` 错误
- **THEN** 子代理 SHALL 依 `hint` 本地战术修复重试
- **AND** 仅当子代理回报无法完成时，编排层/主 agent 才 SHALL 做战略重规划
