# evaluation-graders Specification

## Purpose
TBD - created by archiving change python-evaluation-service. Update Purpose after archive.
## Requirements
### Requirement: Plugin-based grader system
The system SHALL provide a plugin-based grader architecture for extensible evaluation.

#### Scenario: Register built-in grader
- **WHEN** system starts
- **THEN** all built-in graders are auto-registered
- **AND** graders can be retrieved by name

#### Scenario: Register custom grader
- **WHEN** custom grader class is decorated with @register_grader
- **THEN** grader is added to registry
- **AND** grader becomes available for evaluation

#### Scenario: List available graders
- **WHEN** caller requests grader list
- **THEN** system returns all registered grader names and descriptions

### Requirement: Multiple grader types
The system SHALL support multiple grader implementation patterns.

#### Scenario: Function-based grader
- **WHEN** grader is a simple function
- **THEN** system accepts function with signature (retrieved, relevant, **kwargs) -> float
- **AND** function can be used directly in evaluation

#### Scenario: Class-based grader
- **WHEN** grader is a class extending BaseGrader
- **THEN** system instantiates class with provided config
- **AND** calls score() method for evaluation

#### Scenario: Agentic grader
- **WHEN** grader needs external tools
- **THEN** system provides access to tool registry
- **AND** grader can use tools during evaluation

### Requirement: Custom metrics support
The system SHALL support user-defined custom metrics.

#### Scenario: Custom Python grader
- **WHEN** user adds Python file to custom/ directory
- **THEN** system discovers and registers grader on next reload
- **AND** grader is available for evaluation

#### Scenario: Custom grader with config
- **WHEN** custom grader requires configuration
- **THEN** system accepts config in EvaluationRequest
- **AND** passes config to grader constructor

#### Scenario: Grader hot-reload
- **WHEN** custom grader file is modified
- **THEN** system detects change and reloads grader
- **AND** new grader implementation is used for next evaluation

### Requirement: LLM-as-Judge as grader component
The system SHALL support LLM-as-Judge as a composable grader.

#### Scenario: LLM judge grader registration
- **WHEN** system loads llm.py grader
- **THEN** LLM judge is registered as "llm-judge" grader
- **AND** can be used in evaluation config

#### Scenario: Custom LLM judge dimensions
- **WHEN** config specifies custom dimensions for LLM judge
- **THEN** LLM judge evaluates on specified dimensions
- **AND** returns scores for each dimension

#### Scenario: LLM judge in composite grader
- **WHEN** composite grader includes LLM judge as component
- **THEN** LLM judge is called as part of composite evaluation
- **AND** results are combined with other components

### Requirement: Evaluation strategies
The system SHALL support strategies for controlling grader execution.

#### Scenario: Zero-shot strategy
- **WHEN** strategy is "zero_shot"
- **THEN** system calls specified grader directly
- **AND** returns grader output

#### Scenario: Data-driven strategy
- **WHEN** strategy is "data_driven"
- **THEN** system learns scoring from sample data
- **AND** applies learned scoring to evaluation

#### Scenario: Ensemble strategy
- **WHEN** strategy is "ensemble"
- **THEN** system calls multiple graders
- **AND** combines results using specified aggregation method

### Requirement: Grader validation
The system SHALL validate graders before registration.

#### Scenario: Valid grader
- **WHEN** grader implements required interface
- **THEN** registration succeeds
- **AND** grader is available for use

#### Scenario: Invalid grader signature
- **WHEN** grader function has wrong signature
- **THEN** registration fails with error message
- **AND** system continues with other graders

#### Scenario: Grader with missing dependencies
- **WHEN** grader imports unavailable module
- **THEN** registration fails gracefully
- **AND** error is logged for troubleshooting

### Requirement: Grader metadata
The system SHALL provide metadata about each grader.

#### Scenario: Grader info query
- **WHEN** caller requests grader information
- **THEN** system returns: name, description, parameters, return_type

#### Scenario: Grader compatibility check
- **WHEN** caller checks if grader supports specific metric type
- **THEN** system returns compatibility information

