# evaluation-grpc Specification

## MODIFIED Requirements

### Requirement: Result message structure
The system SHALL return structured EvaluationResult as the output of stateless metric computation. 该结果 SHALL 由无状态指标计算入口返回，MUST NOT 内嵌进度流或编排状态。

#### Scenario: Complete result
- **WHEN** 无状态指标计算完成
- **THEN** result contains: retrieval metrics, generation metrics, llm_judge metrics, semantic metrics
- **AND** 结果 SHALL 为一次性返回，MUST NOT 附带 Progress 流

#### Scenario: Detailed QA results
- **WHEN** config requests detailed results
- **THEN** result includes qa_results array with per-QA scores

## REMOVED Requirements

### Requirement: ExecuteEvaluation RPC method
**Reason**: `ExecuteEvaluation` 是 Python 侧越界的第二套有状态编排入口，与 Go worker 权威编排重复；评测 gRPC 面收敛为无状态指标计算，Python 不再承担编排/进度。
**Migration**: 调用方改为通过 Go worker 触发评测（任务状态与进度由 Go worker 管理），Python 侧仅提供无状态 `compute_*` 指标计算；移除前需 `grep ExecuteEvaluation link-go` 确认无 Go 侧调用，并同步删除 `proto` 中该 RPC、`link-python` 的 servicer 实现与 `runner.py` 状态机。

### Requirement: Streaming response format
**Reason**: 流式响应（Progress/Result/Error 的 oneof）依附于已移除的 `ExecuteEvaluation` 有状态流式编排；无状态指标计算一次性返回结果，不再需要流式进度格式。
**Migration**: 进度语义由 Go worker 承担并对外暴露；Python 计算返回一次性 EvaluationResult，调用方不再消费 Progress 流。
