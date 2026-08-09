# generative-ui-rendering Specification

## Purpose
TBD - created by archiving change data-agent-evolution. Update Purpose after archive.
## Requirements
### Requirement: 渲染即工具（render_ui）

系统 SHALL 把 A2UI 生成式 UI 从"流末尾一次性 compose"改造为 Agent 工具 `render_ui`。LLM MUST 能在 ReAct 循环中多次调用 `render_ui`，每次调用 SHALL 即时向前端推送一个 `ui` 事件（UI surface），而非等到整轮结束才渲染。

#### Scenario: 流中多次渲染

- **WHEN** Agent 先出一张指标卡，随后又出一张趋势图
- **THEN** 系统 SHALL 分两次推送 `ui` 事件，用户在第二张渲染前即可看到第一张
- **AND** 每次 `render_ui` 调用 SHALL 对应一个独立的前端 UI surface

#### Scenario: UI 引用结果集而非内联数据

- **WHEN** `render_ui` 渲染一张含 5000 行的表格组件
- **THEN** 组件 SHALL 通过 `result_id` 与 RFC6901 JSON Pointer 引用 Result Store 中的数据
- **AND** SHALL NOT 把全部 5000 行内联进 UI 规格或提示词

### Requirement: 交互式组件与回调驱动多轮

A2UI 组件目录 SHALL 扩展交互式组件（Button/Confirm/Form/Filter/Pagination）。前端组件交互 MUST 能回调后端，把用户动作（点击/提交/筛选/翻页/确认）作为 follow-up 输入驱动 Agent 的下一轮循环。

#### Scenario: 确认卡片驱动 resume

- **WHEN** 用户点击确认卡片（Confirm）上的"确认执行"
- **THEN** 前端 SHALL 发起携带 `pending_action_id` 与 token 的 follow-up resume 请求
- **AND** Agent SHALL 据此继续执行被暂停的危险操作

#### Scenario: 分页组件按需取数

- **WHEN** 用户在 Pagination 组件点击下一页
- **THEN** 前端 SHALL 依据 `result_id` 请求下一页数据
- **AND** 系统 SHALL 从 Result Store 按 cursor 返回对应页，而非重跑查询

### Requirement: 渲染规格校验

系统 SHALL 在推送 `ui` 事件前校验 `render_ui` 产出的 A2UI 规格，MUST 拒绝引用不存在的 `result_id`、越界 JSON Pointer 或目录外组件类型的规格。

#### Scenario: 拒绝非法数据引用

- **WHEN** `render_ui` 产出的规格引用了一个不存在的 `result_id`
- **THEN** 校验 SHALL 失败，系统 SHALL NOT 推送该 `ui` 事件
- **AND** SHALL 向 LLM 回传校验错误以便修正

### Requirement: UI 持久化与历史回放

A2UI 规格 MUST NOT 仅作为一次性 SSE 事件存在。每次 `render_ui` 产出的规格 SHALL 随对应 assistant 消息持久化（MySQL 聊天记录），使会话重开/刷新时既往 UI surface 可原样重现。UI 规格 MUST NOT 写入 [Result Store](../agent-result-store/spec.md)——规格通过 `result_id` 引用数据，二者分离存储。交互式组件的绑定状态（surface ↔ `result_id`、`pending_action_id` + token）SHALL 在会话 TTL 内持久化（Redis），以支撑回调路由。

#### Scenario: 重开会话重现既往 UI

- **WHEN** 用户刷新或重新打开一个既往会话
- **THEN** 系统 SHALL 从聊天记录取回历次 `render_ui` 的 A2UI 规格并重新渲染各 UI surface
- **AND** SHALL NOT 依赖 SSE 事件重放来恢复既往 UI

#### Scenario: 交互绑定状态可路由回调

- **WHEN** 用户在既往渲染的 Confirm/Filter/Pagination 组件上触发交互
- **THEN** 系统 SHALL 依据持久化的绑定状态定位对应 `result_id`/`pending_action_id`
- **AND** 若绑定状态已超会话 TTL，SHALL 返回"会话已过期"而非错误路由

### Requirement: 有界数据快照与过期降级

为使既往 UI 在 Result Store TTL 过期后仍可查看，`render_ui` 在渲染时 SHALL 把组件**实际展示的有界数据**（受上限约束的行数与聚合值）快照进对应消息记录，该快照独立于 Result Store 的 TTL。对于超上限的大/无界数据，系统 MUST NOT 全量快照，而 SHALL 在数据过期后以"数据已过期，可重跑"占位降级。

#### Scenario: 小结果永久可看

- **WHEN** 一张指标卡或封顶 N 行的小表被渲染，随后其 `result_id` 在 Result Store 中过期
- **THEN** 会话重开时系统 SHALL 从消息记录的有界快照重现该 UI
- **AND** SHALL NOT 因 Result Store 过期而丢失该 UI 的数据

#### Scenario: 大数据过期后降级占位

- **WHEN** 一张引用 5000 行的表格，其 `result_id` 已过期
- **THEN** 系统 SHALL NOT 尝试从快照重现全部 5000 行
- **AND** SHALL 渲染"数据已过期，点此重跑"占位而非报错

