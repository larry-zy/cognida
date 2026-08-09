## 1. 工具层：错误分级 + schema 线索（sql_error.go）

- [x] 1.1 新增 `tools/sql_error.go`：定义 `error_kind` 枚举常量（syntax/unknown_column/unknown_table/timeout/permission/transient/other）与可修复观察结构体（`ErrorKind`/`Retriable`/`Hint`/`Detail`）
- [x] 1.2 实现 `classifySQLError(err) -> (kind, identifier)`：以 MySQL driver error code 为主（1054/1146/1064/1205/1213/1044/1142）、错误文本模式为辅；`context.DeadlineExceeded`/连接类归 transient；未匹配归 other
- [x] 1.3 实现 `buildRepairObservation(kind, identifier, hint, detail) -> string`：输出紧凑 JSON 观察；`detail` 复用既有外部源脱敏逻辑，不泄露主机/账号
- [x] 1.4 实现 schema 线索检索：对 unknown_column/unknown_table 用解析出的标识符 + 租户/数据源，复用 get_schema 同源元数据取可用列/表候选（截断 top-N）；检索失败降级为无候选通用 hint，绝不 panic
- [x] 1.5 单元测试：各 error_kind 分级、标识符解析、hint 组装、schema 不可得降级、脱敏保持

## 2. 工具层：sql_execute / semantic_query 接入

- [x] 2.1 `sql_execute.go:135` 失败路径改为调用 `classifySQLError` + `buildRepairObservation` 回灌可修复观察，替换裸 `查询执行失败: %w`
- [x] 2.2 在 `sql_execute` 内加瞬时错退避重试：`transient` 走 `maxRetries=2` + 退避（100ms/300ms），耗尽才上抛并标注「已耗尽自动重试」
- [x] 2.3 `semantic_query.go` 失败路径复用同源 `sql_error.go` 分级与观察
- [x] 2.4 单元测试：瞬时错重试后成功（LLM 不见中间失败）、重试耗尽上抛、成功路径信封契约不变（向后兼容回归）

## 3. presets 层：修复纪律 prompt

- [x] 3.1 `playbooks.go` `systemPrompt` 增加修复纪律段落：先读 `error_kind`+`hint` 诊断 → 针对性修正重试 → 同类错误连续 2 次未修复则改变策略（换表/换口径/反问用户），禁止盲目重复
- [x] 3.2 校对各意图 playbook（取数/趋势/归因/报告/通用/歧义）与修复纪律一致，删除任何引导盲目重试的措辞
- [x] 3.3 单元测试/快照：断言 systemPrompt 含修复纪律关键规约

## 4. framework 层：重复失败护栏 + 触发式重规划

- [x] 4.1 `eino_agent.go` `execLoop` 内维护 `map[signature]int`，signature = `toolName + ":" + error_kind`（从可修复观察 JSON 解析，失败退化为 toolName）
- [x] 4.2 达 `replanThreshold=2` 注入换策略重规划提示（换表/换口径/反问用户，勿重复原调用）
- [x] 4.3 达 `windDownThreshold`（累计约 4 次同签名）提前 wind-down，走既有诚实收尾路径，不耗尽 maxIter
- [x] 4.4 任一成功观察对应签名计数归零
- [x] 4.5 单元测试：连续同错触发重规划、再次达上限 wind-down、成功重置计数、不同 error_kind 不累计

## 5. framework 层：执行上下文注入 collabRegistry

- [x] 5.1 在构造工具/skill 执行上下文处（`eino_builder.go` Build / 执行路径）把 `collabRegistry` 只读注入 ctx；提供 `collabFromContext(ctx)` 取用辅助
- [x] 5.2 未注入时取用方 SHALL 得到明确「无子代理可编排」错误（非 panic）；不影响不依赖它的工具/skill
- [x] 5.3 单元测试：注入后 handler 能取到注册表、未注入时安全降级

## 6. skills 层：skill 可执行 handler

- [x] 6.1 `skills/types.go` `Skill` 增加可选字段 `Handler SkillHandler`（`func(ctx, input) (output, error)`）与 `CanInvoke bool`（缺省 false）
- [x] 6.2 `tools/skill_tool.go` `skill_invoke`：命中 skill 若 `CanInvoke && Handler!=nil` 则执行 handler 回传输出，否则维持返回 markdown；handler panic 走 recover 转普通 skill 错误
- [x] 6.3 保证 handler 执行期间工具门/scope 硬拦截照旧生效（`skill-tool-policy` 不被绕过）
- [x] 6.4 单元测试：可执行 skill 命中执行 handler、无 handler skill 行为不变、panic 安全兜底、handler 内被禁工具被拦截

## 7. 复杂任务下沉：skill 内 inline 编排子代理

- [x] 7.1 提供 handler 内 inline 触发子代理的辅助（经 `collabRegistry` 按名取子代理并复用委派内核，紧凑回传 + IsCyclic/MaxDepth 护栏）
- [x] 7.2 新增复杂任务 skill：多维归因（含 handler，inline 编排 SQLAuthor+Analysis）
- [x] 7.3 新增复杂任务 skill：经营报告（含 handler，inline 编排 Insight/Report，写类经 Operation）
- [x] 7.4 校对分流：简单单步任务不命中下沉 skill、主 agent 直连不多跳；systemPrompt/目录不误导简单任务走委派
- [x] 7.5 单元测试：inline 编排后只回摘要、内部往返不回灌、循环被 IsCyclic 拦截、简单任务不下沉

## 8. Operation 子代理：复杂操作修复下沉

- [x] 8.1 确认复杂写/ETL/导出经 Operation 子代理路径；子代理内接收可修复观察并按修复纪律本地重试
- [x] 8.2 保证写操作错误细节（底层 SQL/约束冲突/行数）留在子代理上下文，只回传紧凑 handle/结论，不回灌指挥官主循环
- [x] 8.3 确认本地修复重试时危险分级/dry-run/人机确认协议全程生效，超阈值仍产出 `pending_action_id`
- [x] 8.4 确认简单直接写仍收结构化可修复观察且受脱敏约束；修复重试留痕进 `agent_operation_audit`
- [x] 8.5 单元/集成测试：复杂操作在子代理内本地修复、错误细节不回灌主循环、危险确认协议不被绕过

## 9. 集成与验收

- [x] 9.1 端到端集成测试（真实 DB）：列名错一次自愈、表名错一次自愈、瞬时错自动重试透明成功
- [x] 9.2 端到端集成测试：连续同错触发重规划并改变策略；无法完成时诚实部分收尾且不耗尽 maxIter
- [x] 9.3 端到端集成测试：复杂归因/报告命中下沉 skill、handler inline 编排子代理完成、主循环只见摘要；简单取数主 agent 直连
- [x] 9.4 `go test ./internal/... -v` 与 `go test -tags=integration ./internal/... -v` 全绿
- [x] 9.5 Code Review（触发 code-review skill）并修复问题
- [ ] 9.6 任务完成后终止启动的服务进程，提交代码
