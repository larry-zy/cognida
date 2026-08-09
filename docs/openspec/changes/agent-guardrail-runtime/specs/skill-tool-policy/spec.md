## MODIFIED Requirements

### Requirement: 执行前硬工具门

系统 SHALL 把 skill 的 `allowed_tools`/`disallowed_tools` 从文档提示升级为**执行前硬拦截**。在 Agent 循环执行任一工具调用之前，系统 MUST 依据当前生效的工具策略判定是否放行；判定结果 SHALL 为三态之一——**放行**、**拒绝**、**需审批**（`approval_required`）。被拒绝的调用 MUST NOT 执行，且 SHALL 以合成 ToolMessage（`{"error":"tool_blocked",...}`）回灌 LLM。被判定为需审批的调用 SHALL NOT 直接执行，改为进入人机确认流程（见「写/导出审批第三态」要求）。判定的必要条件顺序 SHALL 为：deny 优先 → 白名单模式 → scope 校验 → 审批标记；deny 与 scope 拒绝 SHALL 优先于审批。

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

#### Scenario: deny 与 scope 优先于审批

- **WHEN** 某工具既被 deny/scope 拒绝、又被标记为需审批
- **THEN** 系统 SHALL 直接拒绝（tool_blocked）
- **AND** SHALL NOT 进入审批流程

## ADDED Requirements

### Requirement: 写/导出审批第三态

系统 SHALL 支持把写类工具（`sql_mutate`/`data_export`/`etl_run`）由会话或 skill 策略标记为**需人工审批**。当工具门判定为 `approval_required` 时，系统 SHALL NOT 直接执行该工具，MUST 生成一个待确认动作（`pending_action`）并回灌「待人工确认」ToolMessage；人工确认后 SHALL 经既有确认恢复路径执行。未被标记审批的工具，工具门判定 SHALL 退化为原 放行/拒绝 二态，行为不变（向后兼容）。该策略级审批 SHALL 与工具自身内建的取值级危险确认（如按影响行数阈值）叠加生效，二者均通过后方可执行。

#### Scenario: 标记审批的写工具生成待确认动作

- **WHEN** 会话把 `sql_mutate` 标记为需审批，LLM 尝试调用 `sql_mutate`
- **THEN** 系统 SHALL 判定为 `approval_required` 且不执行写入
- **AND** SHALL 生成 `pending_action` 并回灌待确认 ToolMessage

#### Scenario: 未标记审批退化二态

- **WHEN** 某工具未被任何策略标记为需审批
- **THEN** 工具门判定 SHALL 仅为放行或拒绝
- **AND** 行为与升级前一致

#### Scenario: 策略级审批与取值级危险门叠加

- **WHEN** 一次写操作既被策略标记需审批、又触及内建危险行数阈值
- **THEN** 系统 SHALL 要求两道确认均通过后才执行

### Requirement: 审批与拦截留痕对齐

系统 SHALL 把 `approval_required` 审批事件与既有工具门拦截留痕对齐记录，MUST 含被审批工具名、审批原因、生效 skill/会话标识与关联 `request_id`，复用同源审计记录器范式。

#### Scenario: 审批事件写入审计

- **WHEN** 一次工具调用被判定为 `approval_required`
- **THEN** 系统 SHALL 写入一条审计记录
- **AND** 记录 SHALL 含审批原因与关联 `request_id`
