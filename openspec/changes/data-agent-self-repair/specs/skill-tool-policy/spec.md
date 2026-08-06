## MODIFIED Requirements

### Requirement: 执行前硬工具门

系统 SHALL 把 skill 的 `allowed_tools`/`disallowed_tools` 从文档提示升级为**执行前硬拦截**。在 Agent 循环执行任一工具调用之前，系统 MUST 依据当前生效的工具策略判定是否放行；被拒绝的调用 MUST NOT 执行，且 SHALL 以合成 ToolMessage（`{"error":"tool_blocked",...}`）回灌 LLM。该硬门 SHALL 同样覆盖由**可执行 skill handler 或工具 handler inline 发起**的工具/子代理调用（见 [agent-skill-runtime](../agent-skill-runtime/spec.md)），MUST NOT 因走 handler 路径而绕过。

#### Scenario: disallowed 工具被拦截

- **WHEN** 命中的 skill 将 `sql_mutate` 列入 `disallowed_tools`，而 LLM 尝试调用 `sql_mutate`
- **THEN** 系统 SHALL 在执行前拦截该调用
- **AND** SHALL 回传 `{"error":"tool_blocked",...}` 合成 ToolMessage 而非执行写入

#### Scenario: 白名单模式仅放行 allowed

- **WHEN** skill 设置了非空 `allowed_tools` 列表
- **THEN** 系统 SHALL 进入白名单模式，仅放行列表内工具
- **AND** 任何不在 `allowed_tools` 的调用 SHALL 被拦截

#### Scenario: deny 优先于 allow

- **WHEN** 同一工具同时出现在 `allowed_tools` 与 `disallowed_tools`
- **THEN** 系统 SHALL 以拒绝为准（deny wins）

#### Scenario: handler 内 inline 调用同受硬门

- **WHEN** 可执行 skill 的 handler inline 发起一个被 scope/策略禁止的工具或写子代理调用
- **THEN** 系统 SHALL 在执行前依据工具门/scope 拦截
- **AND** MUST NOT 因该调用源自 handler 而放行
