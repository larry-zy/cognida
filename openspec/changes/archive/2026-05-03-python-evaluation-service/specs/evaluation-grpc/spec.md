# Evaluation gRPC Interface Capability Specification

## ADDED Requirements

### Requirement: ExecuteEvaluation RPC method
The system SHALL provide ExecuteEvaluation RPC method that accepts request and returns stream of responses.

#### Scenario: Valid request
- **WHEN** client calls ExecuteEvaluation with valid EvaluationRequest
- **THEN** system returns stream of EvaluationResponse messages
- **AND** stream contains progress updates followed by final result

#### Scenario: Missing required fields
- **WHEN** request missing dataset_id or knowledge_base_id
- **THEN** system returns error immediately without starting stream

### Requirement: Streaming response format
The system SHALL use oneof pattern for streaming responses.

#### Scenario: Progress response
- **WHEN** sending progress update
- **THEN** response contains Progress message with stage, current, total, message fields

#### Scenario: Result response
- **WHEN** evaluation completes successfully
- **THEN** response contains EvaluationResult with all calculated metrics

#### Scenario: Error response
- **WHEN** evaluation fails
- **THEN** response contains Error message with error description

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
The system SHALL return structured EvaluationResult.

#### Scenario: Complete result
- **WHEN** evaluation completes
- **THEN** result contains: retrieval metrics, generation metrics, llm_judge metrics, semantic metrics

#### Scenario: Detailed QA results
- **WHEN** config requests detailed results
- **THEN** result includes qa_results array with per-QA scores
