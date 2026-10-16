## ADDED Requirements

### Requirement: Governed candidate construction
The system SHALL accept a fixed published QA standard and actual prepared predecessor input, construct an immutable-source SFT draft through the shared domain service, and persist the real candidate and completion receipt atomically.

#### Scenario: Upstream preparation feeds SFT
- **WHEN** preparation succeeds and its authorized output is bound to the SFT node
- **THEN** the candidate uses that exact content and standard snapshot and emits a review-required manifest with its real dataset ID.

### Requirement: Human review and independent access
The system MUST retain independent HF/SFT pages, APIs and tools and SHALL NOT grant the Supervisor review, publish or training authority.

#### Scenario: Candidate awaits review
- **WHEN** a parent task completes SFT construction
- **THEN** the dataset remains draft, the page can open it by persisted receipt, and an authorized human can review and publish through the existing service.

### Requirement: Replay and cancellation consistency
The system MUST enforce scope/key/hash idempotency, cancel-before-submit fencing, deadline checks and current source authorization.

#### Scenario: Replay after success
- **WHEN** a completed submission is replayed after controller restart
- **THEN** the same candidate and output manifest are returned without duplicating data or discarding an actual success on late cancellation.
