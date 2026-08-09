## ADDED Requirements

### Requirement: 护栏接入 ReAct 主链路

系统 SHALL 把既有 `GuardrailService`（`CheckInput`/`CheckJailbreak`/`CheckOutput`/`SanitizeInput`/`SanitizeOutput`）接入 Agent 的 ReAct 主执行链路，使护栏在真实业务请求上强制生效，MUST NOT 仅作为独立 REST 旁路自检接口存在。护栏 SHALL 复用现有挂载缝——输入经 `BeforeHook`、写/导出审批经工具执行前门 `gateToolCall`、最终输出经 `AfterHook`——MUST NOT 为此另造并行执行框架。

#### Scenario: 护栏在主链路生效而非仅旁路

- **WHEN** 一个启用了护栏的 Agent 处理一次真实用户请求
- **THEN** 输入检查/越狱检测 SHALL 在进入 ReAct 前执行
- **AND** 最终输出检查 SHALL 在回答返回用户前执行
- **AND** 二者 SHALL 复用既有 Hook 挂载点，不经独立 REST 端点

### Requirement: 护栏默认关闭且启用零回归

系统 SHALL 提供会话/agent 级护栏开关（输入、越狱、输出、写审批各自独立），默认全部关闭。护栏未启用时，Agent 的构建与执行行为 SHALL 与接入护栏前逐字节一致，MUST NOT 引入任何可观察差异或额外延迟。

#### Scenario: 未启用护栏零回归

- **WHEN** 某 Agent 未开启任何护栏开关
- **THEN** 其请求处理路径 SHALL NOT 装配任何护栏 Hook
- **AND** 其产出与延迟 SHALL 与接入护栏前一致

#### Scenario: 护栏按开关逐项启用

- **WHEN** 仅开启输出护栏、未开启输入护栏
- **THEN** 系统 SHALL 仅装配输出侧护栏
- **AND** 输入侧 SHALL 不做护栏检查

### Requirement: 输入护栏中止与脱敏

启用输入护栏的会话中，系统 SHALL 在消息进入 ReAct 前执行越狱检测与输入检查。判定为不安全或越狱意图的输入 SHALL 中止本次执行并返回明确拒绝，MUST NOT 进入 ReAct 循环。对可脱敏的输入，系统 SHALL 经 `SanitizeInput` 改写后放行，使后续处理使用脱敏后的消息。

#### Scenario: 越狱输入被拦截

- **WHEN** 用户输入被越狱检测判定为攻击意图
- **THEN** 系统 SHALL 中止执行并返回拒绝
- **AND** Agent SHALL NOT 进入 ReAct 循环

#### Scenario: 可脱敏输入改写后放行

- **WHEN** 输入含可脱敏敏感片段但整体安全
- **THEN** 系统 SHALL 经 `SanitizeInput` 改写消息
- **AND** 后续处理 SHALL 使用改写后的消息

### Requirement: 最终输出护栏与流式缓冲交付

启用输出护栏的会话中，系统 SHALL 在最终回答返回用户前对其执行输出检查（含幻觉与 PII），命中时 SHALL 经 `SanitizeOutput` 脱敏或替换为安全兜底文案。由于流式路径正文一旦逐块下发即无法事后愈合，当会话启用输出护栏时，系统 SHALL 对该轮强制走缓冲交付（先完成生成、过护栏、再一次性下发），保证任何字节到达用户前均已过护栏。

#### Scenario: 最终输出命中被脱敏

- **WHEN** 最终回答含 PII 或不安全内容
- **THEN** 系统 SHALL 在返回前脱敏或替换为安全兜底文案

#### Scenario: 启用输出护栏的流式会话缓冲交付

- **WHEN** 一个流式会话启用了输出护栏
- **THEN** 系统 SHALL 缓冲完整回答、过护栏后一次性下发
- **AND** MUST NOT 在护栏判定前把任何正文块下发给用户

#### Scenario: 未启用输出护栏仍可流式

- **WHEN** 会话未启用输出护栏
- **THEN** 系统 SHALL 保持逐块流式交付不变

### Requirement: 逐工具输出脱敏

启用逐工具输出护栏时，系统 SHALL 在工具执行返回后、其观察回灌 LLM 前，对工具观察做 PII/敏感字段脱敏，防止敏感数据经中间工具结果进入 LLM 上下文。该检查点 SHALL 为可选，未启用时 MUST 零开销且工具观察逐字节不变；脱敏 MUST NOT 破坏结果信封（如 `result_id`）结构。

#### Scenario: 含 PII 的工具观察被脱敏后回灌

- **WHEN** 某工具返回的观察含 PII
- **THEN** 系统 SHALL 在回灌 LLM 前脱敏相关字段
- **AND** 脱敏 SHALL NOT 破坏 `result_id` 等信封结构

#### Scenario: 未启用逐工具护栏零开销

- **WHEN** 会话未启用逐工具输出护栏
- **THEN** 工具观察 SHALL 逐字节不变直接回灌

### Requirement: 统一护栏留痕

系统 SHALL 记录护栏产生的拦截/脱敏/审批事件，复用与工具门拦截同源的审计记录器范式。每条记录 MUST 含事件类型（如 `input_blocked`/`jailbreak_blocked`/`output_redacted`/`tool_output_redacted`/`write_approval_required`）、关联 `request_id`、`session_id`、`tenant_id`，供审计与调优，并可经 `request_id` 与日志关联。

#### Scenario: 护栏事件落审计

- **WHEN** 任一护栏拦截/脱敏/审批事件发生
- **THEN** 系统 SHALL 写入一条护栏审计记录
- **AND** 记录 SHALL 含事件类型与关联 `request_id`
