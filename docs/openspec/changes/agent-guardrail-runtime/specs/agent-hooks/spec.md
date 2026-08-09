## ADDED Requirements

### Requirement: 护栏 BeforeHook 承载输入检查

系统 SHALL 支持把 `GuardrailService` 的输入检查（`CheckInput`）与越狱检测（`CheckJailbreak`）装配为一个 `BeforeHook`，复用既有 Before 挂载点与错误/改写契约：判定不安全 SHALL 经返回 error 中止执行，可脱敏 SHALL 经返回改写后的 message 放行。护栏 BeforeHook SHALL 在其它业务 BeforeHook（如意图澄清）之前执行，做第一道把关。

#### Scenario: 护栏 BeforeHook 装配

- **WHEN** 组合根为某 Agent 启用输入护栏
- **THEN** 系统 SHALL 把 `GuardrailService` 输入检查装配为 `BeforeHook`
- **AND** 该 hook SHALL 位于 before-hooks 链最前

#### Scenario: 护栏 BeforeHook 中止走既有 error 契约

- **WHEN** 护栏 BeforeHook 判定输入不安全
- **THEN** 其 SHALL 返回 error
- **AND** Agent 执行 SHALL 依既有 Before hook error 契约停止并传播

### Requirement: 护栏 AfterHook 承载输出检查

系统 SHALL 支持把 `GuardrailService` 的输出检查（`CheckOutput`，含幻觉与 PII）装配为一个 `AfterHook`，复用既有 After 挂载点：命中 SHALL 经 `SanitizeOutput` 脱敏或替换安全兜底文案后再返回。护栏 AfterHook SHALL 在其它 AfterHook 之后执行，做最后一道把关。由于 `AfterHook` 仅在缓冲路径收敛，启用输出护栏的流式会话 SHALL 强制走缓冲交付（见 agent-guardrail-runtime 规约）。

#### Scenario: 护栏 AfterHook 装配

- **WHEN** 组合根为某 Agent 启用输出护栏
- **THEN** 系统 SHALL 把 `GuardrailService` 输出检查装配为 `AfterHook`
- **AND** 该 hook SHALL 位于 after-hooks 链最后

#### Scenario: 护栏 AfterHook 非阻塞脱敏

- **WHEN** 护栏 AfterHook 判定最终输出含敏感内容
- **THEN** 其 SHALL 脱敏或替换为安全兜底文案
- **AND** 返回给用户的 SHALL 是脱敏后的响应
