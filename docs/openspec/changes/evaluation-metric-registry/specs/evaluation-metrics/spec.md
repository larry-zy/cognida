## MODIFIED Requirements

### Requirement: Compute metrics via HTTP API

The system SHALL provide an HTTP API for batch metrics computation that iterates the requested graders through the registry and carries results as a dynamic scores map, while preserving existing fixed fields for backward compatibility.

#### Scenario: Successful metrics computation
- **WHEN** Go Worker POSTs to `/api/v1/evaluation/compute-metrics` with valid QA items
- **THEN** Python service returns aggregate metrics
- **AND** Python service returns per-item metrics
- **AND** response includes success: true

#### Scenario: Registry-driven computation
- **WHEN** the request lists graders in `request.graders`
- **THEN** the system resolves each grader from the registry and invokes it
- **AND** the system does not use a hardcoded `if` chain of metric names

#### Scenario: Dynamic scores carrier
- **WHEN** metrics are computed for an item
- **THEN** per-item results include a dynamic `scores` map keyed by metric name
- **AND** aggregate results include a dynamic `scores` map keyed by metric name
- **AND** existing fixed fields remain populated for backward compatibility

#### Scenario: Invalid request
- **WHEN** request contains invalid graders or missing fields
- **THEN** system returns 400 error
- **AND** system returns validation error details

### Requirement: Aggregated metrics reporting

The system SHALL aggregate metrics across all evaluated QA pairs, including dynamically registered metrics, and SHALL retain a metric in the aggregate whenever its grader ran, even when every score is legitimately zero.

#### Scenario: Average metrics calculation
- **WHEN** evaluation completes
- **THEN** system calculates the average of each computed metric across all QA pairs
- **AND** returns the aggregated values in the dynamic `scores` map

#### Scenario: Zero-valued metric retained
- **WHEN** a grader ran for every item but produced a legitimate score of 0
- **THEN** the metric is still present in the aggregate with value 0
- **AND** the metric is not dropped by a positivity check

#### Scenario: Per-QA result details
- **WHEN** caller requests detailed results
- **THEN** system returns individual results for each QA pair
- **AND** includes question, reference, generated, and the dynamic scores map
