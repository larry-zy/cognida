# evaluation-graders Specification

## MODIFIED Requirements

### Requirement: LLM-as-Judge as grader component
The system SHALL support LLM-as-Judge as a composable grader that truly invokes the model through `LLMClient.generate_json`, uses a single unified score scale, and MUST NOT silently fix a constant score when the call or parse fails.

#### Scenario: LLM judge grader registration
- **WHEN** system loads llm.py grader
- **THEN** LLM judge is registered as "llm_judge" grader
- **AND** can be used in evaluation config

#### Scenario: Judge invokes model via generate_json
- **WHEN** the `llm_judge` grader in `graders/builtin/llm.py` evaluates an answer
- **THEN** it MUST call `LLMClient.generate_json(...)` to obtain structured dimension scores
- **AND** it MUST NOT call a non-existent method whose failure is swallowed

#### Scenario: Failure is surfaced not silently fixed
- **WHEN** the LLM call or JSON parse fails
- **THEN** the grader MUST surface an explicit error / failure marker
- **AND** it MUST NOT return a hardcoded `total_score` of 3.0 as if successful

#### Scenario: Unified score scale
- **WHEN** the LLM judge returns dimension scores and `total_score`
- **THEN** all dimensions and the total MUST use a single unified scale
- **AND** downstream consumers MUST read scores on that same scale

#### Scenario: Custom LLM judge dimensions
- **WHEN** config specifies custom dimensions for LLM judge
- **THEN** LLM judge evaluates on specified dimensions
- **AND** returns scores for each dimension

## ADDED Requirements

### Requirement: Runner reads dimension scores correctly
The evaluation runner SHALL call the LLM-judge with the correct signature per item and read structured dimension scores from the result.

#### Scenario: compute_llm_judge_metrics_async reads dimension_scores
- **WHEN** `compute_llm_judge_metrics_async` evaluates items
- **THEN** it MUST call the judge per item with the correct signature
- **AND** it MUST read `result["dimension_scores"]` from the returned structure

### Requirement: Grader registration failures are logged
Grader registration failures SHALL be logged and MUST NOT silently mark the registry as ready.

#### Scenario: Registration failure logged
- **WHEN** a grader fails to register at startup
- **THEN** the failure MUST be logged with the grader name and reason
- **AND** the system MUST NOT silently fix the registry state to "ready"
