## ADDED Requirements

### Requirement: skill 可执行 handler

系统 SHALL 允许 skill 从纯指导文档升级为**可执行能力**：`skills.Skill` SHALL 支持可选字段 `Handler`（`func(ctx, input) (output, error)`）与 `CanInvoke`（布尔）。当命中的 skill `CanInvoke` 为真且 `Handler` 非空时，`skill_invoke` SHALL 执行该 handler 并回传其输出，MUST NOT 仅返回 markdown 文本；否则 SHALL 维持既有行为返回指导文档。无 handler 的 skill 行为 SHALL 完全不变（向后兼容），`CanInvoke` 缺省为假。handler panic SHALL 被 recover 为普通 skill 错误回灌 LLM，MUST NOT 崩溃整个 Agent 循环。

#### Scenario: 可执行 skill 命中即执行 handler

- **WHEN** LLM 通过 `skill_invoke` 命中一个 `CanInvoke=true` 且有 `Handler` 的 skill
- **THEN** 系统 SHALL 执行该 handler 并回传其输出
- **AND** SHALL NOT 仅返回 skill 的 markdown 内容

#### Scenario: 无 handler skill 行为不变

- **WHEN** 命中的 skill 无 `Handler`（`CanInvoke` 缺省）
- **THEN** 系统 SHALL 返回其 markdown 指导内容
- **AND** 既有纯文档 skill 的行为 SHALL 保持不变

#### Scenario: handler panic 安全兜底

- **WHEN** skill handler 执行中 panic
- **THEN** 系统 SHALL recover 并作为普通 skill 错误回灌 LLM
- **AND** MUST NOT 崩溃整个 Agent 循环

### Requirement: 执行上下文注入协作注册表

系统 SHALL 在构造工具/skill 执行上下文时把 `collabRegistry`（子代理注册表）注入 ctx，使 skill handler 与工具能在执行体内取到子代理并 inline 触发。注入 SHALL 为只读传递；当 ctx 未携带 `collabRegistry` 时，依赖它的 handler SHALL 降级为明确的「无子代理可编排」错误，MUST NOT panic，且不影响不依赖它的 skill/工具。

#### Scenario: handler 从 ctx 取到协作注册表

- **WHEN** 一个可执行 skill handler 在执行中需要编排子代理
- **THEN** 它 SHALL 能从 ctx 取到注入的 `collabRegistry`
- **AND** 据此按名获取已注册子代理

#### Scenario: 未注入时安全降级

- **WHEN** 执行上下文未携带 `collabRegistry` 而 handler 依赖它
- **THEN** handler SHALL 返回明确的「无子代理可编排」错误
- **AND** MUST NOT panic，且不影响不依赖它的能力

### Requirement: 复杂任务 handler 内 inline 编排子代理

系统 SHALL 支持把复杂任务（如多维归因、经营报告）封装成带 handler 的 skill，handler 内经注入的 `collabRegistry` **inline 编排子代理群**（串行/并行），并只向主循环回传紧凑 handle/摘要（如 `result_id` + 结论），内部逐轮工具往返 MUST NOT 回灌指挥官主循环。inline 编排 SHALL 复用子代理既有 `execLoop` 与 [agent-collaboration](../agent-collaboration/spec.md) 的紧凑回传、IsCyclic、MaxDepth 护栏。简单任务 SHALL NOT 被强制下沉——未命中下沉 skill 时主 agent 仍直接用工具，不引入额外委派跳数。

#### Scenario: 复杂任务 inline 编排后只回摘要

- **WHEN** 一个复杂任务 skill handler inline 编排 SQLAuthor+Analysis 完成归因
- **THEN** 回传主循环的 SHALL 是 `result_id` + 结论摘要
- **AND** 内部逐轮工具往返 SHALL NOT 回灌主循环

#### Scenario: 简单任务不下沉不多跳

- **WHEN** 用户请求简单单步取数，未命中下沉 skill
- **THEN** 主 agent SHALL 直接调用查询工具完成
- **AND** SHALL NOT 因下沉机制而引入额外委派跳数

#### Scenario: inline 编排复用协作护栏

- **WHEN** handler inline 编排的子代理链形成环或超深度
- **THEN** 系统 SHALL 依据 IsCyclic/MaxDepth 拦截
- **AND** 编排子代理的失败/成功回传 SHALL 遵循既有紧凑契约

### Requirement: 可执行 skill 全程受工具门与 scope 约束

skill handler 执行期间（含其 inline 编排的子代理）SHALL 依旧全程受 [skill-tool-policy](../skill-tool-policy/spec.md) 的工具门与会话 scope 约束，MUST NOT 因走 handler 路径而绕过硬拦截。写类操作在 handler 内 inline 委派时 SHALL 仍经 Operation 子代理并受危险分级/确认协议约束。

#### Scenario: handler 内工具门不被绕过

- **WHEN** 一个可执行 skill 的 handler 尝试 inline 触发 scope 未授予的写工具
- **THEN** 系统 SHALL 依据工具门/scope 拦截
- **AND** MUST NOT 因走 handler 路径而放行

#### Scenario: handler 内写操作仍走确认协议

- **WHEN** handler inline 编排 Operation 子代理执行超危险阈值的写
- **THEN** 系统 SHALL 仍产出 `pending_action_id` 等待确认
- **AND** SHALL NOT 因 inline 路径跳过人机确认
