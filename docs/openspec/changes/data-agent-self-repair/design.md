## Context

Data Agent 是单 ReAct（Reason-Act-Observe）循环内核（`framework/eino_agent.go`），四类能力查/析/渲/操统一编排，`maxIter=12`、`tokenBudget=120000`。

现状（已核对代码，非硬中断）：

- `eino_agent.go` `handleToolCall`（约 807-881）已把工具错误封成 `schema.ToolMessage` 回灌 LLM——`obs = fmt.Sprintf("Error: %v", execErr)`，属**软循环**；但没有失败计数、没有重规划触发。
- `tools/sql_execute.go:135` 直接 `fmt.Errorf("查询执行失败: %w", err)` 透传裸 driver 错误（外部源已脱敏，131-134）；无分级、无 schema 线索、无瞬时重试。
- `presets/data_agent/playbooks.go` 的 `systemPrompt` 与各意图 playbook 无任何报错处置纪律。

问题本质是「**无引导的盲目重试**」：错误信号弱 → LLM 只能瞎猜 → 可能把 12 轮预算耗在同一条错 SQL 上。真实 NL2SQL 失败大头是列名/表名错，一次带上「可用列/表」即可自愈。

第二个现状（已核对代码）：子代理机制建全但未被激励——主 agent 经 `capabilityGroups`（`data_agent.go:28`）直接持有全部工具（含写类），systemPrompt/playbook 无一句引导委派，`delegate_*` 隐身；skill 现状是纯 Markdown（`skills/types.go` 无 handler、不能执行 Go），`collabRegistry` 只喂给了 framework 的 `delegate_*` 工具，skill/工具 handler 拿不到子代理注册表。结论：复杂任务无法真正下沉，只能在主循环里硬展开。

## Goals / Non-Goals

**Goals:**

- 工具失败回灌**结构化可修复观察**（`error_kind`/`retriable`/`hint`）而非裸 driver 错误。
- `unknown_column`/`unknown_table` 附 schema-grounded `hint`（可用列/表），支撑一次重试内自愈。
- `transient` 错误在工具内退避重试，不惊动 LLM。
- 主 + 子代理 systemPrompt 加修复纪律；ReAct 循环加重复失败护栏 + 触发式重规划。
- 给 skill 加**可执行 handler**、给工具/skill 执行上下文注入 `collabRegistry`，使复杂任务能封装成 skill 并在 handler 内 **inline 编排子代理群**。
- 复杂写/ETL/导出经 Operation 子代理本地修复，写操作错误细节不回灌主循环；**简单任务主 agent 仍直接用工具，不多一跳**。

**Non-Goals:**

- 不改成 DAG/多循环规划器——保持单 ReAct 内核，只在软循环上加引导层。
- **不做主 agent 纯编排重构**——不摘除主 agent 的执行类工具，简单任务照旧主 agent 自己干；只把复杂任务经 skill handler 下沉。
- 不新增外部依赖、不新增 DB 表；沿用现有 DB 连接、Result Store、`agent_operation_audit`。
- 不改成功路径的结果信封契约与无 handler skill 的既有行为（向后兼容）。
- 不做跨会话学习/记忆化修复（属 L5，超本次范围）。

## Decisions

### D1：错误分级放在工具层，用独立 `sql_error.go` 辅助

新增 `tools/sql_error.go`：`classifySQLError(err) -> (kind, identifier)` + `buildRepairObservation(kind, identifier, hint) -> string(JSON)`。分级基于 driver error code + 错误文本模式匹配（MySQL：1054 unknown column、1146 unknown table、1064 syntax、1205/1213 deadlock、1044/1142 permission；`context.DeadlineExceeded`/连接类 → transient）。

- **为何工具层而非 framework 层**：只有工具持有 driver 原始错误与 schema 访问能力；framework 层只见字符串。framework 只需按签名做失败计数，不需要懂 SQL。
- **替代方案**：在 `handleToolCall` 里正则解析 `Error: ...` 字符串再分级——脆弱且丢失 driver code，否决。

### D2：可修复观察用 JSON 结构回灌，成功路径不变

失败时 ToolMessage 内容改为紧凑 JSON：`{"error_kind":"unknown_column","retriable":true,"hint":{"table":"orders","available_columns":[...]},"detail":"<脱敏摘要>"}`。成功仍回结果信封。

- **为何 JSON**：LLM 易解析、字段稳定、便于 prompt 里引用 `error_kind`/`hint`。
- **脱敏**：`detail` 复用既有外部源脱敏逻辑，MUST NOT 泄露主机/账号。

### D3：schema 线索复用 get_schema 的元数据检索

`unknown_column`/`unknown_table` 时，用 D1 解析出的标识符 + 当前租户/数据源，调用与 `get_schema` 同源的表/列元数据检索取候选清单。检索失败则降级为无候选的通用 `hint`，MUST NOT panic。

- **为何复用**：避免重复实现 schema 访问；口径与 `get_schema` 一致。
- **性能**：仅失败路径触发，正常查询零开销；候选清单截断（如列 top-N）防止 hint 过长。

### D4：瞬时重试在工具内，有限次 + 退避

`transient` 在 `sql_execute` 内 `maxRetries=2`、退避（如 100ms/300ms）自动重试；耗尽才作为可修复观察上抛并标注「已耗尽自动重试」。

- **为何工具内**：瞬时抖动不该触发 LLM 生成往返（省 token、降延迟）。
- **上限**：明确 `maxRetries` 常量防级联放大；只读 SELECT 重试安全（幂等）。

### D5：framework 护栏按「工具名+错误签名」计数，阈值注入重规划提示

`execLoop` 内维护 `map[signature]int`，signature = `toolName + ":" + error_kind`（error_kind 从可修复观察 JSON 解析，解析失败退化为 toolName）。

- 连续同签名失败达 `replanThreshold=2` → 注入一条 system/user 重规划提示（「换策略：换表/换口径/反问用户，勿重复原调用」）。
- 再次达 `windDownThreshold=2`（即累计约 4 次同签名）→ 提前 wind-down，走既有诚实收尾路径。
- 任一成功观察 → 该签名计数归零。

- **为何在 framework**：跨工具调用的状态只有循环持有；prompt 层无法计数。
- **替代方案**：只靠 prompt 纪律不加护栏——LLM 不总遵守，无法保证不耗尽 maxIter，否决。二者互补：prompt 是软引导，护栏是硬保底。

### D6：复杂操作下沉 Operation 子代理，简单操作主 agent 仍直接干

复杂写/ETL/导出经 Operation 子代理（summary 上下文）：子代理接收可修复观察、按修复纪律本地重试，只回传紧凑 handle/结论；写操作错误细节（底层 SQL/约束冲突/行数）留在子代理上下文，不回灌指挥官主循环；危险分级/dry-run/确认协议在本地重试时全程生效。简单单步写操作主 agent 仍可直接执行（工具门 + scope 照旧兜底）。

- **为何下沉复杂而非全部**：写操作错误细节回灌主循环既污染上下文又放大写风险；但强制所有写都委派会让简单操作固定多一跳。以「复杂/多步」为界，兼顾干净与低延迟。
- **护栏协同**：D5 护栏对「委派 Operation 子代理」这一步仍按签名计数，防止对同一失败操作反复委派。

### D7：skill 可执行 handler + 执行上下文注入 collabRegistry

给 `skills.Skill` 增加可选字段 `Handler SkillHandler` 与 `CanInvoke bool`（`skills/types.go`）。`skill_invoke` 工具（`tools/skill_tool.go`）在命中 skill 时：若 `CanInvoke && Handler != nil` 则**执行 handler** 并回传其输出；否则维持既有行为（返回 markdown 指导）。handler 签名 `func(ctx, input) (output, error)`，通过 ctx 拿到注入的 `collabRegistry`——在 framework 构造工具执行上下文时（`eino_builder.go`/执行路径）把 `collabRegistry` 放进 ctx，使 handler 与工具能 inline 触发子代理。

- **为何在 skill 层加 handler 而非只加编排工具**：skill 天然是「复杂任务方法论」的载体，剧本 + 可执行体同处一地，新增/调整一个复杂能力只动一个 skill，比把编排逻辑写死在工具里灵活。
- **向后兼容**：无 handler 的 skill 完全不受影响，仍是纯指导文档；`CanInvoke` 缺省 false。
- **替代方案**：只走「skill 剧本引导 LLM 调 `delegate_*`」（零改动）——主循环仍见委派往返、不是真 inline，否决为起步方案但保留为回退。

### D8：复杂任务 skill handler 内 inline 编排子代理，修复分两层

复杂任务（多维归因、经营报告）封装成带 handler 的 skill，handler 内经注入的 `collabRegistry` 拿到子代理实例、**inline 编排**（串/并行）SQLAuthor/Analysis/Operation/Insight 等，跑完只回传 `result_id` + 结论摘要给主循环，内部逐轮往返不回灌。修复据此天然分两层：

| 层 | 谁 | 干什么 |
|---|---|---|
| 战术修复 | 子代理内 `execLoop` | 拿 `error_kind`+`hint` 改 SQL 重试、瞬时错退避——本地闭环 |
| 战略重规划 | 主 agent / skill handler 编排层 | 子代理回报搞不定时换子代理/换口径/反问；D5 护栏在此层计数 |

- **inline 触发 vs LLM delegate**：inline 由 handler 代码直接调子代理（复用 `agent-collaboration` 的紧凑回传与 IsCyclic/MaxDepth 护栏），主循环只见一次 skill 调用，上下文最干净；LLM `delegate_*` 仍保留给主 agent 临场委派。
- **护栏复用**：inline 编排走的仍是子代理的同一 `execLoop`，D1–D5 修复层对 inline 子代理天然生效，无需另写。

## Risks / Trade-offs

- **错误分级误判（driver 文本随版本变化）** → 以 driver error code 为主、文本模式为辅；未匹配一律归 `other` 并保留脱敏摘要，绝不因分级失败而崩溃。
- **schema 线索检索增加失败路径延迟** → 仅失败时触发、候选截断、检索失败即降级；不影响成功路径。
- **瞬时重试可能延长单次工具耗时** → `maxRetries` 与退避上限受控（最坏 +~400ms），且只对幂等 SELECT。
- **护栏阈值过紧误伤合法多次尝试 / 过松失效** → 阈值设常量便于调参；signature 含 `error_kind` 使「不同错」不累计，降低误伤。
- **prompt 修复纪律与护栏重复触发** → 二者分层互补而非冲突：prompt 先软引导，护栏是达阈值的硬保底；成功即双双重置。
- **Operation 下沉后主循环可观测性下降** → 子代理回传的 handle/结论仍进审计（`agent_operation_audit`），排障凭 request_id 全链路追踪，不依赖主循环上下文。
- **skill handler 引入可执行代码，扩大攻击面/出错面** → handler 执行时工具门与 scope 校验 SHALL 依旧全程生效（`skill-tool-policy` 不被绕过）；`CanInvoke` 显式开关，缺省 false；handler panic SHALL recover 为普通 skill 错误回灌，不崩整个循环。
- **collabRegistry 注入 ctx 增加耦合** → 只读注入、handler 侧按需取用；未注入时 handler 取不到即降级为「无子代理可编排」错误，不影响不依赖它的 skill/工具。
- **inline 编排绕开 LLM 可能掩盖决策** → inline 子代理仍复用同一 `execLoop` 与审计留痕；handler 只固化「编排骨架」，具体查询/分析仍由子代理 LLM 决策，非硬编码结果。

## Migration Plan

- 纯增量、向后兼容：成功路径信封契约不变，无 handler skill 行为不变，无 DB 变更、无新依赖。
- 分层落地顺序：tools 层（分级+线索+重试）→ presets 层（prompt 纪律）→ framework 层（护栏 + collabRegistry 注入）→ skill runtime（handler 支持）→ 复杂任务 skill 下沉。每层独立可测、可单独回滚。
- 回滚：护栏阈值设极大值即等价关闭；prompt 纪律段落可摘除；skill `CanInvoke` 置 false 即回退为纯文档；工具层失败仍走既有 wind-down，无破坏性。

## Open Questions

- `replanThreshold`/`windDownThreshold`/`maxRetries` 的具体数值是否需要按意图（取数/归因/报告）差异化，还是全局常量即可——初版先全局常量，后续按实测调。
- `semantic_query.go` 是否与 `sql_execute` 完全共用 `sql_error.go` 分级，还是需要语义层特有的 `error_kind`（如口径歧义）——初版共用，语义层特有分类按需再加。
- 「复杂任务」判定边界（何时命中下沉 skill、何时主 agent 直接干）由 skill 匹配阈值 + 意图路由决定还是显式规则——初版靠 skill 目录 + `FallbackInjectThreshold` 命中，先做归因/报告两个 skill 验证，再定通用判定。
- handler 内 inline 编排是否需要独立于主循环的 token 预算——初版沿用子代理各自 `maxIter`，不额外设预算，观察后再定。
