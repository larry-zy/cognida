## ADDED Requirements

### Requirement: Backend-driven metric catalog

The system SHALL expose a backend endpoint that returns the catalog of available metrics as the single source of truth for the frontend, so the frontend does not hardcode metric lists.

#### Scenario: List available metrics
- **WHEN** the frontend requests the metric catalog
- **THEN** the backend returns each available metric with its metadata: `name`, `label`, `group`, `requires_reference`, `requires_contexts`, and `eval_types`
- **AND** the catalog is derived from the grader registry, not a static list

#### Scenario: No drift between catalog and compute
- **WHEN** a metric appears in the catalog
- **THEN** the `/compute-metrics` endpoint has a registered grader able to compute it
- **AND** a metric with no registered grader never appears in the catalog

### Requirement: Filter catalog by evaluation type

The system SHALL filter the metric catalog by evaluation type (`llm`/`qa`, `rag`, `agent`) so that creating a task of a given type only surfaces metrics applicable to that type.

#### Scenario: Type-filtered catalog
- **WHEN** the frontend requests the catalog for evaluation type `rag`
- **THEN** the backend returns only metrics whose `eval_types` include `rag`
- **AND** metrics not applicable to `rag` are excluded

#### Scenario: Shared metrics appear across types
- **WHEN** a metric declares multiple applicable evaluation types
- **THEN** the metric appears in the filtered catalog for each of those types

#### Scenario: Unknown type
- **WHEN** the frontend requests the catalog for an unrecognized evaluation type
- **THEN** the backend returns a validation error rather than an unfiltered list
