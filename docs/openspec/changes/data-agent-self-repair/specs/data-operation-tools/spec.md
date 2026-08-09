## ADDED Requirements

### Requirement: 复杂操作类错误修复下沉子代理

经 Operation 子代理执行的复杂/多步写/ETL/导出（`sql_mutate`/`etl_run`/`data_export`），其执行失败修复 SHALL 由该子代理在其 summary 上下文防火墙内本地完成：子代理接收结构化可修复观察（`error_kind`/`hint`/`retriable`，见 [agent-error-repair](../agent-error-repair/spec.md)），在本地上下文内诊断并按修复纪律重试，只向指挥官回传紧凑 handle/结论。写操作的失败错误细节（含底层 SQL、约束冲突、行数等）MUST NOT 回灌指挥官主循环，以免污染主上下文并放大写风险。简单单步写操作若由主 agent 直接执行，SHALL 仍收到结构化可修复观察，但其错误细节暴露面 SHALL 受 [agent-error-repair](../agent-error-repair/spec.md) 脱敏纪律约束。危险分级、dry-run 与人机确认协议在子代理本地修复重试时 SHALL 依旧全程生效。

#### Scenario: 复杂操作类错误在子代理内本地修复

- **WHEN** 经 Operation 子代理执行的复杂写/ETL/导出失败
- **THEN** Operation 子代理 SHALL 在其本地上下文内接收可修复观察并按修复纪律处置
- **AND** 指挥官主循环 SHALL 只收到紧凑 handle/结论

#### Scenario: 简单直接写操作仍受修复契约

- **WHEN** 主 agent 直接执行一个简单单步写操作并失败
- **THEN** 回灌的观察 SHALL 是结构化可修复观察（含 `error_kind`）
- **AND** 其错误细节 SHALL 受既有脱敏纪律约束

#### Scenario: 写操作错误细节不回灌主循环

- **WHEN** 写操作因约束冲突/语法等失败
- **THEN** 失败的底层错误细节 SHALL 留在 Operation 子代理上下文内
- **AND** MUST NOT 回灌指挥官主循环

#### Scenario: 本地修复重试仍受危险分级约束

- **WHEN** Operation 子代理在本地修复后重试写操作
- **THEN** 危险分级/dry-run/人机确认协议 SHALL 依旧全程生效
- **AND** 超危险阈值的操作 SHALL 仍产出 `pending_action_id` 等待确认
