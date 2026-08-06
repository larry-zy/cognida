## Why

Data Agent 的 ReAct 循环虽已把工具错误作为 ToolMessage 回灌 LLM（`eino_agent.go` handleToolCall，非硬中断），但属**「无引导的盲目重试」**：回灌的是裸 driver 错误（`sql_execute.go` 仅 `查询执行失败: <err>`），无结构化分级、无 schema 线索；系统提示与 playbook 无修复纪律；瞬时抖动（超时/连接/死锁）也要惊动 LLM 往返；且无重复失败护栏——LLM 可能把 `maxIter=12` 全耗在对同一条错 SQL 反复瞎试上。真实 NL2SQL 失败大头是列名/表名错，一次带上「可用列/表」即可自愈。

同时暴露一个更基础的结构问题：**子代理机制虽已建全（7 个子代理 + 委派信封 + 上下文防火墙），却几乎没被用起来**——主 agent 直接持有全部工具（含写类 `sql_mutate`/`etl_run`），systemPrompt 与 playbook 无一句引导委派，`delegate_*` 工具在列表里等于隐身。结果是「什么都主 agent 干」，复杂任务（多维归因、经营报告）在主循环里硬展开，既污染上下文又难修复。而 skill 现状只是 Markdown 指导文档（`skills/types.go` 无 handler、不能执行 Go），无法把复杂编排真正封装进去。

本提案把两件事一起做：(1) 把软循环从「盲目重试」升级为「有引导的自我修复 + 触发式动态重规划」；(2) 让复杂任务真正下沉子代理——给 skill 增加**可执行 handler**，handler 内可 **inline 编排子代理群**，主循环只见一次 skill 调用与紧凑摘要。**简单任务主 agent 仍直接用工具，不多一跳**；复杂任务封装成带 handler 的 skill 下沉执行。修复随之天然分两层：子代理内**战术修复**（改 SQL 重试）、主 agent/编排层**战略重规划**（换子代理/换口径/反问）。目标：把 Data Agent 从 SIGMOD L2 稳、L3 半推进到 L3 稳。

## What Changes

- **结构化错误分级 + schema 线索**：`sql_execute`/`semantic_query` 执行错误分类为 `error_kind`（`syntax`/`unknown_column`/`unknown_table`/`timeout`/`permission`/`transient`/`other`），对 `unknown_column`/`unknown_table` 解析出错误标识符并附「可用列/表」清单为 `hint`，回灌 LLM 的是**可修复观察**（结构化 `error_kind`+`hint`+`retriable`）而非裸错误。
- **瞬时错工具内自动重试**：`transient`（超时/连接抖动/死锁）在工具内做有限次退避重试（不惊动 LLM），仅在耗尽后才作为可修复观察上抛。
- **修复纪律 prompt（分两层）**：主 agent 与**各子代理** systemPrompt 明确修复协议——「先读 `error_kind`+`hint` 诊断 → 针对性修正重试 → 同类错误连续 2 次未修复则改变策略，禁止盲目重复」；子代理做战术修复，主 agent/编排层做战略重规划。
- **重复失败护栏（触发式动态重规划）**：ReAct 循环（主 + 子代理共用 `execLoop`）追踪同工具/同错误签名连续失败次数，达阈值注入「换策略」重规划提示；再次达上限则优雅收尾，防止耗尽 `maxIter`。
- **skill 可执行 handler**：给 `skills.Skill` 增加可选 handler（`CanInvoke`），把 skill 从「纯指导文档」升级为「可被 `skill_invoke` 直接执行的能力」；工具/skill 执行上下文注入 `collabRegistry`，使 handler 内可拿到子代理注册表。
- **复杂任务下沉 + inline 编排子代理**：把多维归因、经营报告等复杂任务封装成带 handler 的 skill，handler 内 **inline 编排子代理群**（含写类经 Operation 子代理），主循环只收紧凑 handle/摘要，内部逐轮往返与写操作错误细节不回灌主循环。**简单任务不命中 skill，主 agent 照常直接用工具**。

## Capabilities

### New Capabilities
- `agent-error-repair`: Data Agent 跨工具的错误修复契约——结构化 `error_kind` 分级、schema-grounded 修复 `hint`、瞬时错工具内退避重试、ReAct 循环重复失败护栏与触发式动态重规划。
- `agent-skill-runtime`: skill 可执行 handler 运行时——skill 从纯文档升级为可执行能力；工具/skill 执行上下文注入 `collabRegistry`；handler 内 inline 编排子代理群并只回传紧凑摘要；简单任务不下沉、复杂任务下沉的分流契约。

### Modified Capabilities
- `agent-tools`: `sql_execute`（及语义层查询路径）从回灌裸 driver 错误改为回灌结构化可修复观察（`error_kind`/`hint`/`retriable`），并对瞬时错做工具内自动重试；`get_schema` 供修复线索复用其表/列元数据检索。
- `data-agent`: 单一 ReAct 内核新增修复纪律（主 + 子代理 systemPrompt）与重复失败触发的动态重规划编排；确立「简单任务主 agent 直接干、复杂任务经 skill handler 下沉子代理」的分流。
- `data-operation-tools`: 复杂写/ETL/导出经 Operation 子代理在其上下文防火墙内本地诊断重试，主循环仅收 handle/摘要，写操作错误细节不回灌；简单写操作主 agent 仍可直接执行。
- `skill-tool-policy`: skill 在既有工具门/scope 之上新增**可执行 handler** 语义；handler 执行时工具门与 scope 校验 SHALL 依旧全程生效。
- `agent-collaboration`: 子代理除经 LLM `delegate_*` 触发外，SHALL 支持由 skill/工具 handler 通过注入的 `collabRegistry` **inline 触发**，紧凑回传与循环/深度护栏契约不变。

## Impact

- **link-go**：
  - `internal/service/agent/tools/`：`sql_execute.go`（错误分级 + schema 线索 + 瞬时重试）、`semantic_query.go`（同源修复观察）、新增 `sql_error.go`；`skill_tool.go`（`skill_invoke` 支持执行 handler）。
  - `internal/service/agent/skills/`：`types.go`（`Skill` 加 `Handler`/`CanInvoke`）、新增 handler 执行路径；新增复杂任务 skill（深度归因、经营报告）含 handler。
  - `internal/service/agent/framework/`：`eino_agent.go` 重复失败护栏与重规划提示注入；工具执行上下文注入 `collabRegistry`（`eino_builder.go`/执行路径）。
  - `internal/service/agent/presets/data_agent/`：`playbooks.go`、`data_agent.go`（主 + 子代理 systemPrompt 修复纪律、复杂/简单分流引导）。
- **契约**：工具回灌观察结构新增 `error_kind`/`hint`/`retriable`（向后兼容，成功路径不变）；`Skill` 新增可选 `Handler`/`CanInvoke`（无 handler 的 skill 行为不变）。
- **测试**：错误分级/线索/瞬时重试单测、护栏触发单测、skill handler 执行 + inline 编排子代理单测、复杂任务下沉端到端集成测试。
- **依赖**：无新增外部依赖；复用现有 DB 连接、Result Store、子代理协作注册表。
- **无 DB 变更**：不新增表；沿用现有 `agent_operation_audit` 记录操作类修复重试留痕。
