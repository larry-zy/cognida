# evaluation-executor Specification

## Purpose
TBD - created by archiving change evaluation-system. Update Purpose after archive.
## Requirements
### Requirement: Agent evaluation execution
The system SHALL support Agent-based evaluation for agent-type tasks.

#### Scenario: Successful Agent evaluation
- **WHEN** task has agent_id configured
- **THEN** system loads Agent by agent_id
- **AND** system calls Agent.Chat() for each QA pair
- **AND** system collects generated responses

#### Scenario: Agent not found
- **WHEN** configured agent_id does not exist
- **THEN** system marks task as FAILED
- **AND** system returns error "Agent not found"

### Requirement: RAG evaluation execution
The system SHALL support RAG evaluation for RAG-type tasks.

#### Scenario: Successful RAG evaluation
- **WHEN** task has kb_id and model_id configured
- **THEN** system calls Retriever.Retrieve() with query and top_k
- **AND** system builds prompt with retrieved context
- **AND** system calls LLMChat.Chat() to generate answer
- **AND** system stores retrieved chunks for metrics

#### Scenario: Retrieval failure
- **WHEN** retrieval operation fails
- **THEN** system marks that QA as failed
- **AND** system continues processing remaining QAs

### Requirement: QA evaluation execution
The system SHALL support direct QA evaluation for qa-type tasks.

#### Scenario: Successful QA evaluation
- **WHEN** task has model_id but no kb_id
- **THEN** system builds prompt with question only
- **AND** system calls LLMChat.Chat() to generate answer
- **AND** system stores generated response for metrics

### Requirement: Result collection
The system SHALL collect QA results for all evaluation types.

#### Scenario: Collect results
- **WHEN** executor completes processing
- **THEN** system returns list of QAResult with:
  - question
  - reference_answer
  - generated_answer
  - retrieved_chunks (for RAG)
  - success status
  - error message (if failed)

### Requirement: Go worker 为唯一权威编排

Go worker SHALL 是评测编排的唯一权威：任务状态、进度、编排流程 SHALL 全部由 Go worker 承担。Python 侧 MUST NOT 承担评测的编排或进度状态机。

#### Scenario: 编排职责归 Go worker

- **WHEN** 执行一次评测任务
- **THEN** 任务状态与进度推进 SHALL 由 Go worker 管理
- **AND** Python 侧 MUST NOT 持有并行的编排状态机

#### Scenario: Python 侧无进度状态机

- **WHEN** 检查 `link-python/services/evaluation`
- **THEN** MUST NOT 存在 `runner.py` 的 `ProgressStage` 编排状态机
- **AND** MUST NOT 存在 `EvaluationRunner.run` 式有状态编排入口

### Requirement: Python 评测收敛为无状态计算

Python 评测 SHALL 收敛为无状态 `compute_*` 指标计算 + FastAPI 薄壳（:18888）。Go worker 调 Python 只 SHALL 获取指标结果，MUST NOT 获取"进度流"或委托编排。

#### Scenario: Python 只做无状态计算

- **WHEN** Go worker 需要 Python 计算评测指标
- **THEN** 调用 SHALL 命中无状态 `compute_*` 计算入口返回指标
- **AND** 调用 MUST NOT 触发 Python 侧的进度流或状态编排

