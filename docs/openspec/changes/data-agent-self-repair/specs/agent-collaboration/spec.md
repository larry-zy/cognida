## ADDED Requirements

### Requirement: 子代理 handler 内 inline 触发

系统 SHALL 允许子代理除经 LLM `delegate_to_agent`/`delegate_parallel` 工具触发外，还可由 skill/工具 handler 通过注入的 `collabRegistry` **inline 触发**（见 [agent-skill-runtime](../agent-skill-runtime/spec.md)）。inline 触发 SHALL 复用既有委派执行内核：紧凑 handle/摘要回传、IsCyclic 循环检测、MaxDepth 深度护栏、上下文防火墙与治理元数据校验 MUST 全程一致，MUST NOT 因走 inline 路径而绕过任一护栏。inline 触发的子代理内部逐轮工具往返 SHALL NOT 回灌指挥官主循环。

#### Scenario: handler inline 触发子代理走同一护栏

- **WHEN** 一个 skill handler 经 `collabRegistry` inline 触发 SQLAuthor 子代理
- **THEN** 系统 SHALL 复用既有委派内核执行
- **AND** IsCyclic/MaxDepth/紧凑回传契约 SHALL 与 LLM 委派路径完全一致

#### Scenario: inline 触发仍只回传紧凑摘要

- **WHEN** inline 触发的子代理完成子任务
- **THEN** 其回传 SHALL 为 `result_id` + 结论摘要
- **AND** 内部逐轮工具往返 SHALL NOT 回灌主循环

#### Scenario: inline 触发的循环被拦截

- **WHEN** handler inline 编排使子代理链形成环（如 A→B→A）
- **THEN** 系统 SHALL 依 IsCyclic 拦截该次触发
- **AND** SHALL NOT 因源自 handler 而无限递归
