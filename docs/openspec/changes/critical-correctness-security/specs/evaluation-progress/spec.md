# evaluation-progress Specification

## MODIFIED Requirements

### Requirement: SSE progress streaming
The system SHALL provide SSE endpoint for real-time progress updates, and progress produced internally MUST actually be delivered to the connected client through an `asyncio.Queue` bridge.

#### Scenario: Client connects to stream
- **WHEN** client connects to `/api/v1/evaluation/tasks/{task_id}/stream`
- **THEN** system sets SSE headers (Content-Type: text/event-stream)
- **AND** system begins delivering progress updates for the task

#### Scenario: Progress is yielded to the client
- **WHEN** the worker produces a `Progress` update
- **THEN** `service.py` MUST place it on an `asyncio.Queue`
- **AND** the gRPC/SSE stream side MUST `await` the queue and yield the progress to the client
- **AND** the client MUST NOT be starved of updates that were produced internally

#### Scenario: Completion event
- **WHEN** task completes (COMPLETED or FAILED)
- **THEN** SSE client receives `event: complete` or `event: error`
- **AND** system closes the SSE connection

## ADDED Requirements

### Requirement: Stream terminates on sentinel
The progress stream SHALL terminate cleanly when a completion sentinel is received on the queue.

#### Scenario: Sentinel ends the stream
- **WHEN** a completion/termination sentinel is placed on the progress queue
- **THEN** the stream consumer MUST stop awaiting further items
- **AND** the connection MUST be closed
