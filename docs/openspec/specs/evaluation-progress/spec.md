# evaluation-progress Specification

## Purpose
TBD - created by archiving change evaluation-system. Update Purpose after archive.
## Requirements
### Requirement: SSE progress streaming
The system SHALL provide SSE endpoint for real-time progress updates.

#### Scenario: Client connects to stream
- **WHEN** client connects to `/api/v1/evaluation/tasks/{task_id}/stream`
- **THEN** system sets SSE headers (Content-Type: text/event-stream)
- **AND** system begins polling Redis for progress updates

#### Scenario: Progress update event
- **WHEN** worker updates progress in Redis
- **THEN** SSE client receives `event: progress` message
- **AND** data includes: stage, current, total, percentage, message

#### Scenario: Completion event
- **WHEN** task completes (COMPLETED or FAILED)
- **THEN** SSE client receives `event: complete` or `event: error`
- **AND** system closes SSE connection

### Requirement: Progress storage in Redis
The system SHALL store task progress in Redis for polling.

#### Scenario: Worker updates progress
- **WHEN** worker reaches new stage or completes items
- **THEN** system updates `eval:progress:{task_id}` hash in Redis
- **AND** hash includes: stage, current, total, message
- **AND** progress expires after 1 hour

#### Scenario: SSE polls progress
- **WHEN** SSE handler polls for progress
- **THEN** system reads from `eval:progress:{task_id}`
- **AND** system formats data as SSE event
- **AND** system waits 1 second between polls

### Requirement: Progress stages
The system SHALL report progress through defined stages.

#### Scenario: Stage transitions
- **WHEN** task processing progresses
- **THEN** system reports stages in order:
  1. `init` - Initializing task
  2. `loading` - Loading dataset
  3. `retrieval` - Retrieving documents (RAG only)
  4. `generation` - Generating answers
  5. `evaluation` - Computing metrics
  6. `complete` - Task complete

### Requirement: Multiple concurrent connections
The system SHALL support multiple SSE connections to the same task.

#### Scenario: Multiple clients
- **WHEN** multiple clients connect to same task's stream
- **THEN** all clients receive progress updates
- **AND** system does not block other clients

