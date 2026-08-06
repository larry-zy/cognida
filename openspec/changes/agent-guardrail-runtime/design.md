## Context

护栏服务已实现但未接入主链路（已核对代码）：

- 契约在 `internal/model/guardrail/guardrail.go:437` `GuardrailService`：`CheckInput(ctx, input, opts)`、`CheckOutput(ctx, output, input, opts)`、`CheckJailbreak(ctx, input, opts)`、`SanitizeInput`、`SanitizeOutput`。
- 实现在 `internal/service/agent/tools/service.go:19` `GuardrailServiceImpl`，经 `NewGuardrailService(llm)` 组装 `InputFilter`/`OutputFilter`（含 `CheckHallucination`，`filter/output_filter.go:146`）/`JailbreakDetector`。
- **唯一调用方**是 REST 旁路：`handler/guardrail_handler.go` + `router.go`。ReAct 主循环（`framework/eino_agent.go`）**无任何护栏调用点**。

链路上已有三个恰好匹配的挂载缝（已核对代码）：

- `BeforeHook func(ctx, message string) (context.Context, string, error)`（`eino_agent.go:124`）：`preProcess` 顺序执行（`eino_agent.go:159`）；返回 error → 中止执行并传播；返回改写 message → 后续处理用改写值。`Builder.Before`（`eino_builder.go:118`）装配；`IntentClarifier` 已这样挂。
- `gateToolCall(ctx, tool) (payload, ok)`（`tool_policy.go:207`）：所有工具执行路径（run→handleToolCall→invokeTool）统一必经的执行前硬门；`ToolPolicy.Permits(tool) (bool, reason)`（`tool_policy.go:88`）当前仅 allow/deny；拦截经 `blockedToolResult` 合成 `{"error":"tool_blocked",...}` 回灌并 `recordBlockedToolCall` 留痕（`SetToolBlockRecorder` 挂接）。
- `AfterHook func(ctx, *Response) error`（`eino_agent.go:127`）：仅在 `bufferedSink.finish`（`eino_agent.go:351`）收敛；`Builder.After`（`eino_builder.go:124`）装配；`ConclusionGenerator`/`Reflection`/`AutoCompress` 已这样挂。

已有可复用的写审批机制：`sql_mutate` 危险行数超阈值时 `suspendDangerousMutation`（`tools/sql_mutate.go:177`）事务回滚 + 返回 `Status=pending_confirm` + `ConfirmToken`，经 `ExecuteConfirmedMutation`（`sql_mutate.go:226`）恢复；`pendingaction` 存储 + `uibinding` 前端确认卡片已就位。

结论：护栏接入不是造框架，而是**把现成服务装配到现成缝上** + 一处二值门升三值门。

## Goals / Non-Goals

**Goals:**

- 把 `GuardrailService` 接进 ReAct 主链路的输入、写审批、逐工具输出、最终输出四个点，让护栏在真实业务链路强制生效。
- 复用现有 Hook/工具门/pending-confirm/审计范式，零新框架、零新外部依赖、零新表。
- 护栏**默认关闭**、按会话/agent 逐步启用；未启用时行为逐字节不变（零回归）。
- 诚实处理两个真实缺口：流式输出无法事后愈合、逐工具输出无现成缝。

**Non-Goals:**

- 不改 `GuardrailService` 的判定逻辑/过滤器实现（输入/越狱/输出/幻觉/PII 沿用现有）。
- 不做护栏规则的可视化配置台/规则库管理（属后续，本次只做运行时接入 + 会话级开关）。
- 不改工具门的 deny/allow/scope 既有语义与拦截留痕契约——审批是在其之上的第三态。
- 不做跨会话的护栏学习/自适应阈值。

## Decisions

### D1：输入护栏走 BeforeHook，拦截即中止、可脱敏即改写

组合根按会话护栏配置把 `GuardrailService` 包成一个 `BeforeHook`：先 `CheckJailbreak` 再 `CheckInput`；任一判定不安全 → 返回 error 中止执行（回明确拒绝文案），MUST NOT 进入 ReAct。可脱敏场景经 `SanitizeInput` 返回改写后 message 放行。

- **为何 BeforeHook**：它是唯一「消息进入循环前」的缝，且原生支持「返回 error 中止」「返回改写 message」两种语义，与 CheckInput/SanitizeInput 一一对应。
- **替代方案**：在 handler 层调 REST 护栏——但那样每个入口都要重复接、且拦不住 agent 内部生成的子请求，否决。

### D2：写/导出审批走 gateToolCall，Permits 升三值门（不是走 Hook）

`ToolPolicy.Permits` 由 `(bool, reason)` 升为可返回第三态 `approval_required`。当会话/skill 策略把某写类工具（`sql_mutate`/`data_export`/`etl_run`）标记为需审批时，`gateToolCall` 不执行工具，改为生成 `pending_action`（复用 `sql_mutate` 的 `pending_confirm`/`ConfirmToken` + `uibinding` 卡片）并回灌「待人工确认」ToolMessage；人工确认后经既有 `ExecuteConfirmedMutation`/resume 路径执行。

- **为何不用 Hook**：Hook 只有 before-message/after-response 两点、粒度是「整条请求一次」，拦不住「ReAct 第 N 步 Agent 要调 sql_mutate」这种**逐工具**决策。`gateToolCall` 才是逐工具必经门。
- **与既有阈值门的分工**：`gateToolCall` 管**策略级**审批（这个会话所有写都要批）；`sql_mutate` 内建的行数阈值门管**取值级**审批（这一条改动影响行数太大）。二者叠加、互不替代。
- **向后兼容**：未标记审批的工具，`Permits` 退化为原 allow/deny，行为不变。

### D3：最终输出护栏走 AfterHook；流式启用护栏时强制缓冲交付

组合根把 `GuardrailService` 包成 `AfterHook`：对 `*Response.Content` 跑 `CheckOutput`（含幻觉/PII）；命中 → `SanitizeOutput` 脱敏或替换为安全兜底文案。

- **真实缺口（流式）**：`AfterHook` 只在 `bufferedSink.finish` 收敛；流式路径正文已逐块下发、无完整 `*Response` 可事后愈合（`eino_agent.go:348-351` 刻意保留的边界）。**决策**：当会话启用输出护栏时，SHALL 对该轮强制走缓冲交付（先跑完、过护栏、再一次性下发），以「任何字节到达用户前已过护栏」换取放弃逐 token 流式。未启用输出护栏的会话仍可流式。
- **为何不做增量流式扫描**：边流边扫要么引入不可控延迟、要么无法保证已发出的 token 不含敏感内容，正确性上不成立；缓冲交付简单且可证。

### D4：逐工具输出护栏——新增 post-invoke 拦截缝

这是唯一需要新缝的点：工具返回观察当前直接 append 成 ToolMessage 回灌 LLM，无检查点。在 `handleToolCall` 工具执行返回后、回灌前新增一个可选 `post-invoke` 检查点，对工具观察做 PII/敏感字段脱敏（复用 `SanitizeOutput` 或轻量 PII 过滤）。

- **为何需要**：外部数据源/RAG 片段/SQL 结果可能含 PII，最终输出护栏只扫最终回答、扫不到中间每个工具结果；不脱敏则 PII 已进 LLM 上下文。
- **为何可选/可分期**：护栏 MVP 可先上 D1+D2+D3（输入/写审批/最终输出），D4 作为第二阶段；未启用零开销。

### D5：护栏留痕复用工具门审计范式

护栏的拦截/脱敏/审批事件复用 `SetToolBlockRecorder` 同源的记录器形状，带 `request_id`/`session_id`/`tenant_id` 落审计，与 `recordBlockedToolCall` 对齐，保证「为何被拦/被改/待批」可审计、可经 `request_id` 与 Loki 关联。

- **为何复用**：审计范式已成型（结构化 + 异步 + rid 关联），护栏事件与工具门拦截同类，不另造。

### D6：护栏配置放会话/agent 级，默认关闭

新增会话/agent 级护栏开关（输入/越狱/输出/审批各自独立开关）。默认全关 → 现有 agent 零回归；按 agent（如对外 RAG、写能力强的 Data Agent）逐步开启。

- **为何默认关**：护栏引入 LLM 判定往返（输入/输出过滤器本身用 LLM），有延迟与成本；且流式会因 D3 退化为缓冲。按需启用而非一刀切。

## Risks / Trade-offs

- **延迟/成本**：输入/输出过滤器基于 LLM，启用后每轮多 1~2 次判定往返。缓解：会话级开关按需启用；输入护栏可优先只跑越狱检测（轻）。
- **流式体验**：启用输出护栏即失去逐 token 流式（D3）。缓解：明确标注、由启用方权衡；对不需要输出护栏的会话无影响。
- **误杀**：护栏判定可能误拦正常输入/输出。缓解：拦截留痕含原因，便于调优；SanitizeInput/Output 优先脱敏而非硬拒。
- **审批疲劳**：策略级写审批若过宽会让每个写都要人工点。缓解：审批标记按工具/会话精细化，简单写默认不强制审批，仅高危场景开启。

## Migration / Rollout

1. 先落装配与三值门代码，默认关闭 → 全绿且零回归。
2. 单元 + 集成测试覆盖四个挂载点。
3. 先对一个 agent（建议对外 RAG）开启输入 + 输出护栏灰度验证。
4. 再对 Data Agent 开启写审批（策略级）。
5. D4 逐工具输出脱敏作为第二阶段按需推进。
