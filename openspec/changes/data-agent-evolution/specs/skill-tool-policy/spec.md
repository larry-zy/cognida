## ADDED Requirements

### Requirement: 执行前硬工具门

系统 SHALL 把 skill 的 `allowed_tools`/`disallowed_tools` 从文档提示升级为**执行前硬拦截**。在 Agent 循环执行任一工具调用之前，系统 MUST 依据当前生效的工具策略判定是否放行；被拒绝的调用 MUST NOT 执行，且 SHALL 以合成 ToolMessage（`{"error":"tool_blocked",...}`）回灌 LLM。

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

### Requirement: 会话 scope 最小权限

系统 SHALL 在 skill 工具门之上叠加会话级 scope（read / write / etl），MUST 实现最小权限：scope 未授予的能力，即便工具已注册也不得执行。scope 校验 SHALL 与 skill 策略共同构成放行的必要条件。

#### Scenario: 只读会话拦截写工具

- **WHEN** 会话 scope 为 `read`，LLM 尝试调用 `sql_mutate` 或 `etl_run`
- **THEN** 系统 SHALL 依据 scope 拦截该调用
- **AND** 无论 skill 策略是否放行，写工具 SHALL NOT 执行

#### Scenario: scope 与 skill 策略同为必要条件

- **WHEN** 工具通过了 skill `allowed_tools` 校验，但会话 scope 未授予对应能力
- **THEN** 系统 SHALL 仍拦截该调用
- **AND** 仅当 skill 策略与 scope 双双放行时工具才执行

### Requirement: 拦截留痕

系统 SHALL 记录被工具门/scope 拦截的调用，MUST 包含被拒工具名、拒绝原因（disallowed / not-in-allowlist / scope-denied）、生效 skill 与会话标识，供审计与调优。

#### Scenario: 拦截写入审计

- **WHEN** 任一工具调用被硬门或 scope 拦截
- **THEN** 系统 SHALL 写入一条拦截审计记录
- **AND** 记录 SHALL 含拒绝原因与关联 `request_id`
