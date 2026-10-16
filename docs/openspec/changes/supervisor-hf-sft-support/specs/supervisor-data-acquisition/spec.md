## ADDED Requirements

### Requirement: Durable bounded HF acquisition
The system SHALL expose a fixed-commit HF import as a persistent Supervisor root capability with explicit limits, typed parameters, current authorization and actual raw asset outputs.

#### Scenario: Root import completes
- **WHEN** a user saves and starts a valid fixed-file HF plan
- **THEN** its child receipt, actual manifest and raw asset persist and unblock dependent nodes only after successful domain completion.

### Requirement: Atomic cancellation and uncertain recovery
The system MUST bind scope, operation key and payload hash atomically, fence cancellation before submission, and never repeat a claimed download after an uncertain restart.

#### Scenario: Cancel arrives first
- **WHEN** cancellation is persisted before a delayed Submit
- **THEN** Submit returns the cancelled receipt and does not download or create an asset.

#### Scenario: Worker dies after claim
- **WHEN** another worker reads an uncompleted claimed import
- **THEN** it does not redownload; it exposes the persisted state until cancellation or the fixed deadline determines the result.
