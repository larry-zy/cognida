# Evaluation UseCase Specification

## ADDED Requirements

### Requirement: Evaluation Orchestration

The Evaluation UseCase SHALL orchestrate the evaluation workflow without implementing business logic.

#### Scenario: Successful evaluation workflow
- **WHEN** an evaluation request is submitted
- **THEN** the use case creates an evaluation task
- **AND** retrieves the dataset
- **AND** executes retrieval for each QA pair
- **AND** executes LLM generation for each QA pair
- **AND** delegates metric calculation to domain service
- **AND** saves results through repository
- **AND** updates task status to success

### Requirement: Dependency on Domain Services

The use case SHALL depend on domain layer interfaces for business operations.

#### Scenario: UseCase uses domain evaluation service
- **WHEN** metric calculation is needed
- **THEN** the use case calls `domain.evaluation.CalculateMetrics()`
- **AND** does not implement PMI, BLEU, ROUGE calculations itself

#### Scenario: UseCase uses domain RAG services
- **WHEN** retrieval is needed
- **THEN** the use case calls `domain.rag.Retriever.Retrieve()`
- **AND** does not directly implement retrieval logic

### Requirement: Progress Tracking

The use case SHALL track and report evaluation progress.

#### Scenario: Update progress after each QA
- **WHEN** a QA pair evaluation completes
- **THEN** the use case increments completed count
- **AND** updates progress through repository

### Requirement: Error Handling

The use case SHALL handle errors gracefully during evaluation.

#### Scenario: Retrieval fails for one QA
- **WHEN** retrieval fails for a specific QA pair
- **THEN** the use case logs the error
- **AND** continues with remaining QA pairs
- **AND** marks the specific result as failed

#### Scenario: Dataset not found
- **WHEN** requested dataset does not exist
- **THEN** the use case updates task status to failed
- **AND** includes error message
- **AND** returns early

### Requirement: Result Aggregation

The use case SHALL aggregate results from domain metric calculation.

#### Scenario: Metrics calculated successfully
- **WHEN** domain service returns metric results
- **THEN** the use case saves results through metrics repository
- **AND** includes metrics in evaluation detail response

## REMOVED Requirements

### Requirement: PMI and Weight Calculation in Application Layer
**Reason**: Business logic belongs in domain layer
**Migration**: Move to `domain.evaluation.CalculateMetrics()`

### Requirement: Direct Infrastructure Dependency
**Reason**: Violates dependency inversion principle
**Migration**: Use domain interfaces for Retriever, LLMChat, Reranker
