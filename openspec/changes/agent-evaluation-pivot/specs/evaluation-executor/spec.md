## ADDED Requirements

### Requirement: 执行器在组合根完成注册

系统 SHALL 在组合根（wire）为 Worker 注册全部受支持的执行器（QA、RAG、Agent），使各评测类型均可被路由执行，而非仅注册 QA。

#### Scenario: 三类执行器均已注册
- **WHEN** 服务启动并构建评测 Worker
- **THEN** 执行器注册表同时包含 qa、rag、agent 三类执行器
- **AND** 任一类型任务出队后都能命中对应执行器

#### Scenario: Agent 执行器可被解析
- **WHEN** 一个 `agent` 类型任务出队
- **THEN** Worker 从注册表取得 Agent 执行器
- **AND** 不再返回 "executor not found"

### Requirement: 按评测类型正确路由执行器

Worker SHALL 使用与执行器注册键一致的评测类型来查找执行器，保证 `qa`（含历史别名 `llm`）、`rag`、`agent` 均被正确路由。

#### Scenario: QA 及其别名路由一致
- **WHEN** 任务类型为 `qa` 或其历史别名 `llm`
- **THEN** Worker 解析到 QA 执行器并执行
- **AND** 不因归一化前后键不一致而查找失败

#### Scenario: 未知类型明确失败
- **WHEN** 任务类型不属于已注册的任一执行器
- **THEN** Worker 将任务标记为 FAILED 并给出可诊断的错误信息
- **AND** 不静默吞掉该任务
