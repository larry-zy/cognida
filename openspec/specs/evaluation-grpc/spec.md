# evaluation-grpc Specification

## Purpose
TBD - created by archiving change python-evaluation-service. Update Purpose after archive.
## Requirements
### Requirement: Request message structure
The system SHALL accept EvaluationRequest with specified fields.

#### Scenario: Complete request
- **WHEN** caller provides all fields
- **THEN** system accepts: dataset_id, knowledge_base_id, model_id, config

#### Scenario: Optional config
- **WHEN** config is not provided
- **THEN** system uses default evaluation configuration

### Requirement: Configuration options
The system SHALL support EvaluationConfig with configurable options.

#### Scenario: Top-k configuration
- **WHEN** config specifies top_k
- **THEN** retrieval uses specified top_k value

#### Scenario: Metrics selection
- **WHEN** config specifies metrics list
- **THEN** system only calculates specified metrics

#### Scenario: LLM judge enablement
- **WHEN** config.enable_llm_judge is true
- **THEN** system includes LLM judge in evaluation
- **WHEN** config.enable_llm_judge is false
- **THEN** system skips LLM judge evaluation

### Requirement: Result message structure
The system SHALL return structured EvaluationResult as the output of stateless metric computation. 该结果 SHALL 由无状态指标计算入口返回，MUST NOT 内嵌进度流或编排状态。

#### Scenario: Complete result
- **WHEN** 无状态指标计算完成
- **THEN** result contains: retrieval metrics, generation metrics, llm_judge metrics, semantic metrics
- **AND** 结果 SHALL 为一次性返回，MUST NOT 附带 Progress 流

#### Scenario: Detailed QA results
- **WHEN** config requests detailed results
- **THEN** result includes qa_results array with per-QA scores

