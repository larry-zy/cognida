# evaluation-worker Specification

## Purpose
TBD - created by archiving change evaluation-system. Update Purpose after archive.
## Requirements
### Requirement: Worker process tasks from queue
The system SHALL run background workers that process evaluation tasks from Redis queue.

#### Scenario: Worker processes task
- **WHEN** task is available in `eval:queue`
- **AND** concurrent slot is available (count < limit)
- **THEN** worker dequeues task using BRPOP
- **AND** worker increments `eval:count`
- **AND** worker begins processing task

#### Scenario: Concurrent limit reached
- **WHEN** worker checks for slot but `eval:count` >= limit (3)
- **THEN** worker waits 1 second
- **AND** worker retries slot acquisition

#### Scenario: Task processing completes
- **WHEN** worker finishes task processing
- **THEN** worker decrements `eval:count`
- **AND** worker marks task as COMPLETED or FAILED

### Requirement: Worker retry on failure
The system SHALL retry failed tasks up to 3 times for recoverable errors.

#### Scenario: Retryable error
- **WHEN** task fails with Python service unavailable error
- **AND** retry_count < 3
- **THEN** system updates retry_count
- **AND** system re-queues task for retry

#### Scenario: Max retries exceeded
- **WHEN** task fails and retry_count >= 3
- **THEN** system marks task as FAILED
- **AND** system stores error message

### Requirement: Worker timeout control
The system SHALL enforce timeout limits for task execution.

#### Scenario: Single QA timeout
- **WHEN** single QA execution exceeds 30 seconds
- **THEN** system cancels that QA execution
- **AND** system marks that QA as failed
- **AND** system continues processing remaining QAs

#### Scenario: Task timeout
- **WHEN** total task execution exceeds 30 minutes
- **THEN** system cancels entire task
- **AND** system marks task as FAILED

### Requirement: Worker graceful shutdown
The system SHALL allow graceful shutdown of workers.

#### Scenario: Shutdown signal
- **WHEN** worker receives stop signal
- **THEN** worker finishes current in-flight tasks
- **AND** worker stops processing new tasks
- **AND** worker exits cleanly

