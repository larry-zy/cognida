## Why

护栏能力其实**已经建好了一半却没接进来**：`internal/model/guardrail` 定义了 `GuardrailService`（`CheckInput`/`CheckJailbreak`/`CheckOutput`/`SanitizeInput`/`SanitizeOutput`），`service/agent/tools/service.go` 有完整实现 `GuardrailServiceImpl`（输入过滤 + 越狱检测 + 输出过滤/幻觉检查/PII），但**唯一调用方只有 REST 端点** `handler/guardrail_handler.go` + `router.go`——它是一个「你主动来问我某段文本安不安全」的旁路接口，**从未接进 Agent 的 ReAct 主循环**。也就是说：真正跑业务的 Data Agent/RAG/Text2SQL 在收到用户输入、调用写工具、产出最终回答时，**没有任何护栏在链路上生效**。

同时链路上其实**已有三个恰好合适的挂载缝**，无需另造框架：

- **输入侧**：`framework` 的 `BeforeHook func(ctx, message) (ctx, message, error)`（`eino_agent.go:124`）——返回 error 即中止本次执行、返回改写后的 message 即替换后续处理输入，正好承载 `CheckInput`/`CheckJailbreak`（拦截）与 `SanitizeInput`（脱敏改写）。已有 `IntentClarifier` 就是这么挂的。
- **写/导出审批**：每次工具执行前必经的 `gateToolCall`（`tool_policy.go:207`）——现有 `ToolPolicy.Permits(tool)` 只有 allow/deny 二值，且已有 `sql_mutate` 内建的 `pending_confirm` 挂起/`ConfirmToken` 恢复机制可复用，正好承载「写/导出/ETL 需人工审批」这一策略级门。
- **最终输出侧**：`AfterHook func(ctx, *Response) error`（`eino_agent.go:127`，`bufferedSink.finish` 收敛），正好承载 `CheckOutput`/幻觉检查/PII 脱敏。

问题本质：**护栏服务是现成的、挂载缝是现成的，缺的只是「把服务接到缝上」这一层装配 + 一个把二值门升级为三值门（allow/deny/approval）的小改**。本提案就做这件事，让护栏从「旁路自检接口」变成「主链路强制生效」，补上「能不能上生产」的开关。

真实缺口只有两个，本提案一并覆盖并诚实标注：(1) `AfterHook` 只在**缓冲（非流式）路径**收敛——流式输出正文已逐块下发、无法事后愈合（`eino_agent.go:348-351` 明确保留的边界）；(2) **逐个工具输出**的检查/脱敏当前无缝，需在 `handleToolCall` 新增一个 post-invoke 拦截点。

## What Changes

- **护栏装配进 Agent 运行时**：组合根把已有 `GuardrailService` 按会话护栏配置装配为 `BeforeHook`（输入检查/越狱检测/输入脱敏）与 `AfterHook`（输出检查/幻觉/PII 脱敏），复用现有 `Builder.Before/After`；不改护栏服务本身的判定逻辑。
- **输入护栏（BeforeHook）**：命中不安全输入或越狱意图 SHALL 中止执行并回明确拒绝；可脱敏输入经 `SanitizeInput` 改写后放行；护栏未启用时行为与今天完全一致（零回归）。
- **工具门升级为三值 + 写/导出审批（gateToolCall）**：`ToolPolicy.Permits` 从 `(bool, reason)` 升级为可返回第三态 `approval_required`；被判定需审批的写/导出/ETL 调用 SHALL NOT 直接执行，改为生成 `pending_action`（复用 `sql_mutate` 的 `pending_confirm`/`ConfirmToken` 机制与 `uibinding`）并回灌「待确认」ToolMessage，人工确认后经既有 resume 路径执行。
- **最终输出护栏（AfterHook，缓冲路径）**：最终回答经 `CheckOutput`/幻觉/PII 判定；命中 SHALL 脱敏或替换为安全兜底文案。**流式会话若启用输出护栏，SHALL 强制走缓冲交付**（先扫后发），以保证「任何字节到达用户前已过护栏」。
- **逐工具输出护栏缝（新增 post-invoke 拦截点）**：在 `handleToolCall` 工具返回后、回灌 LLM 前新增一个可选检查点，对工具观察做 PII/敏感字段脱敏；未启用时零开销。
- **统一护栏留痕**：护栏拦截/脱敏/审批事件复用 `SetToolBlockRecorder` 同源的审计范式，带 `request_id`/`session_id`/`tenant_id` 落审计，与工具门拦截留痕对齐。

## Capabilities

### New Capabilities
- `agent-guardrail-runtime`: Agent 运行时护栏契约——把既有 `GuardrailService` 接入 ReAct 主链路的三/四个挂载点（输入 BeforeHook、写审批 gateToolCall、逐工具输出 post-invoke、最终输出 AfterHook），会话级护栏开关、启用/未启用的零回归契约、流式输出护栏的缓冲交付规则、统一护栏留痕。

### Modified Capabilities
- `agent-hooks`: 新增「护栏 BeforeHook/AfterHook」装配契约——BeforeHook 承载输入检查/越狱/脱敏（可中止、可改写 message），AfterHook 承载输出检查/幻觉/PII 脱敏（缓冲路径收敛，流式启用护栏时强制缓冲交付）。
- `skill-tool-policy`: `ToolPolicy.Permits` 由 allow/deny 二值门升级为可返回 `approval_required` 的三值门；scope/deny/allow 既有语义与拦截留痕不变，审批为在其之上的第三态。
- `data-operation-tools`: 写/导出/ETL 在既有「危险行数阈值 → `pending_confirm`」之上，新增**策略级审批**（会话/skill 标记写类需人工确认）——复用同一 `pending_action`/`ConfirmToken`/resume 机制；简单只读及未标记审批的写操作行为不变。

## Impact

- **link-go**：
  - `internal/service/agent/framework/`：`eino_builder.go`（新增 `WithInputGuardrail`/`WithOutputGuardrail` 装配，转 `BeforeHook`/`AfterHook`）；`eino_agent.go`（`handleToolCall` 新增 post-invoke 输出护栏缝；输出护栏启用时流式→缓冲交付判定）；`tool_policy.go`（`Permits` 升三值 + `approval_required` 分支、审批 `pending_action` 合成与留痕）。
  - `internal/service/agent/tools/`：`service.go`（`GuardrailServiceImpl` 复用，不改判定）；写工具审批复用 `sql_mutate.go` 的 `suspendDangerousMutation`/`ExecuteConfirmedMutation` 与 `pendingaction`/`uibinding`。
  - 组合根（`cmd/server` / agent 装配处）：按会话/agent 护栏配置注入 `GuardrailService` 与护栏 Hook；挂接护栏审计记录器。
- **契约**：`ToolPolicy.Permits` 返回值语义扩展（新增 `approval_required` 第三态，向后兼容——未标记审批时退化为原 allow/deny）；护栏 Hook 为可选装配，未启用的 Agent 行为逐字节不变。
- **配置**：新增会话/agent 级护栏开关（输入/输出/越狱/审批各自可开关）；默认关闭以保零回归，按 agent 逐步启用。
- **测试**：输入拦截/越狱/脱敏 BeforeHook 单测、`Permits` 三值门单测、写审批 `pending_action` 生成与 resume 单测、输出护栏脱敏 + 流式强制缓冲单测、逐工具输出脱敏单测、护栏未启用零回归回归测、端到端（不安全输入被拒/写操作走审批确认/最终输出脱敏）。
- **依赖**：无新增外部依赖；复用现有 `GuardrailService`、`pendingaction`、`uibinding`、审计记录器、Hook/工具门框架。
- **无 DB 变更**：不新增表；审批复用现有 `pending_action` 存储与 `agent_operation_audit` 留痕。
