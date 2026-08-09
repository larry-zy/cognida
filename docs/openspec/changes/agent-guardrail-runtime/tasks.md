## 1. framework 层：护栏 Hook 装配点

- [x] 1.1 `eino_builder.go` 新增 `WithInputGuardrail(gs GuardrailService, cfg)` → 转 `BeforeHook`：先 `CheckJailbreak` 再 `CheckInput`，不安全返回 error 中止；可脱敏经 `SanitizeInput` 改写 message 放行
- [x] 1.2 `eino_builder.go` 新增 `WithOutputGuardrail(gs GuardrailService, cfg)` → 转 `AfterHook`：对 `*Response.Content` 跑 `CheckOutput`（含幻觉/PII），命中经 `SanitizeOutput` 脱敏或替换安全兜底文案
- [x] 1.3 护栏 Hook 与既有 before/after hooks 顺序确定：输入护栏 SHALL 在其它 BeforeHook 之前（最先把关）；输出护栏 SHALL 在其它 AfterHook 之后（最后把关）
- [x] 1.4 单元测试：不安全输入被中止、越狱被拦、可脱敏输入改写放行、输出命中脱敏、护栏未装配时 hooks 链行为不变

## 2. framework 层：流式启用护栏时强制缓冲交付

- [x] 2.1 `eino_agent.go` Stream 路径：当会话启用输出护栏时判定强制走缓冲交付（先跑完 + 过 AfterHook + 一次性下发），未启用仍逐块流式
- [x] 2.2 明确边界注释：流式正文一旦下发不可事后愈合，故输出护栏会话必须缓冲（对齐 `eino_agent.go:348-351` 既有边界说明）
- [x] 2.3 单元测试：启用输出护栏的流式会话产出经护栏后一次性下发、未启用会话逐块流式不变

## 3. skill-tool-policy：Permits 升三值门 + approval_required

- [x] 3.1 `tool_policy.go` `Permits(tool)` 返回值扩展：在 allow/deny/scope 三必要条件之上，新增 `approval_required` 第三态（deny/scope 仍优先于审批）
- [x] 3.2 `ToolPolicy` 增加审批标记来源（会话/skill 把哪些写类工具标记为需审批）；未标记时 `Permits` 退化为原 allow/deny（向后兼容）
- [x] 3.3 `gateToolCall` 处理 `approval_required`：不执行工具，生成 `pending_action` 并回灌「待人工确认」ToolMessage
- [x] 3.4 单元测试：deny 优先于审批、scope-denied 优先于审批、未标记退化 allow/deny、标记写类返回 approval_required

## 4. data-operation-tools：策略级写审批复用 pending-confirm

- [x] 4.1 审批 `pending_action` 复用 `sql_mutate.go` `suspendDangerousMutation` 的 `pending_confirm`/`ConfirmToken` 存储与 `uibinding` 确认卡片，不另造机制
- [x] 4.2 人工确认后经既有 `ExecuteConfirmedMutation`/resume 路径执行；策略级审批与 sql_mutate 内建行数阈值门可叠加（两道都过才执行）
- [x] 4.3 `data_export`/`etl_run` 被标记审批时同样生成 `pending_action`（不局限于 sql_mutate）
- [x] 4.4 单元/集成测试：标记审批的写生成 pending_action、确认后 resume 执行、未标记写行为不变、审批 + 行数阈值双门叠加

## 5. framework 层：逐工具输出护栏缝（post-invoke，第二阶段）

- [x] 5.1 `eino_agent.go` `handleToolCall` 工具返回后、回灌前新增可选 post-invoke 检查点，对工具观察做 PII/敏感字段脱敏（复用 `SanitizeOutput`/轻量 PII 过滤）
- [x] 5.2 未启用时零开销、成功观察逐字节不变；脱敏仅作用于命中的字段
- [x] 5.3 单元测试：含 PII 的工具观察被脱敏后才回灌、未启用零改动、脱敏不破坏 result_id 信封结构

## 6. 护栏留痕

- [x] 6.1 护栏拦截/脱敏/审批事件复用 `SetToolBlockRecorder` 同源记录器范式，带 `request_id`/`session_id`/`tenant_id` 落审计
- [x] 6.2 记录 SHALL 含事件类型（input_blocked / jailbreak_blocked / output_redacted / tool_output_redacted / write_approval_required）与关联 `request_id`
- [x] 6.3 单元测试：各类护栏事件均落一条审计、含原因与 rid

## 7. 组合根：护栏配置与装配

- [x] 7.1 新增会话/agent 级护栏配置（输入/越狱/输出/审批各自独立开关），默认全关
- [x] 7.2 组合根按配置注入 `GuardrailService` 与护栏 Hook、挂接护栏审计记录器；未启用的 agent 不装配任何护栏 Hook（零回归）
- [x] 7.3 单元测试：配置开关驱动装配、默认关闭时 agent 构建与执行逐字节不变

## 8. 集成与验收

- [x] 8.1 端到端集成测试：不安全/越狱输入被 BeforeHook 拦截、Agent 不进入 ReAct
- [x] 8.2 端到端集成测试：标记审批的写操作走 `pending_action` → 人工确认 → resume 执行
- [x] 8.3 端到端集成测试：最终输出含敏感/PII 被 AfterHook 脱敏；启用输出护栏的流式会话缓冲交付
- [x] 8.4 零回归回归测：护栏全关时既有 agent（RAG/Text2SQL/Data Agent）行为与产出不变
- [x] 8.5 `go test ./internal/service/agent/... -v` 与 `go test -tags=integration ./internal/... -v` 全绿
- [x] 8.6 Code Review（触发 code-review skill）并修复问题
- [x] 8.7 任务完成后终止启动的服务进程，提交代码
