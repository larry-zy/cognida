## MODIFIED Requirements

### Requirement: Grader metadata

The system SHALL provide metadata about each grader, including the evaluation types it applies to, so callers can discover, filter, and render graders without hardcoding.

#### Scenario: Grader info query
- **WHEN** caller requests grader information
- **THEN** system returns: `name`, `label`, `description`, `group`, `requires_reference`, `requires_contexts`, and `eval_types`

#### Scenario: Grader declares applicable evaluation types
- **WHEN** a grader is registered
- **THEN** it declares one or more applicable evaluation types among `llm`/`qa`, `rag`, `agent`
- **AND** a grader applicable to several types lists all of them

#### Scenario: Grader compatibility check
- **WHEN** caller checks if a grader supports a specific evaluation type
- **THEN** system returns whether that type is in the grader's `eval_types`

## ADDED Requirements

### Requirement: Registry as single source of truth for compute path

The system SHALL drive metric computation through the grader registry rather than hardcoded metric-name branches, so that adding a grader class makes the metric available for computation without further code changes on the compute path.

#### Scenario: Compute uses registered grader
- **WHEN** a computation request references a metric name
- **THEN** the system looks up the corresponding grader in the registry and invokes it
- **AND** the system does not rely on a hardcoded `if` chain of metric names

#### Scenario: New grader is immediately computable
- **WHEN** a developer adds a new grader class with metadata to the codebase
- **THEN** the metric becomes computable on the compute path with no other edits
- **AND** the metric appears in the catalog for its declared evaluation types

#### Scenario: Unknown metric requested
- **WHEN** a computation request references a metric with no registered grader
- **THEN** the system skips it and reports it as unsupported rather than silently ignoring it
