# SSE Helper Specification

## ADDED Requirements

### Requirement: SSE sends honor cancellation
SSE helper-based producers SHALL send downstream with cancellation awareness so that a disconnected client terminates upstream work.

#### Scenario: Send selects on ctx.Done
- **WHEN** an SSE producer sends a chunk to its downstream channel
- **THEN** the send MUST be `select { case ch <- chunk: case <-ctx.Done(): return }`
- **AND** no further chunk MUST be sent after the context is cancelled

#### Scenario: Client disconnect terminates upstream
- **WHEN** the HTTP client disconnects during an SSE stream
- **THEN** the request context MUST be cancelled
- **AND** the upstream producer MUST stop generating

### Requirement: SSE errors distinguish normal end from failure
SSE helper stream consumption SHALL distinguish a normal end-of-stream from a real error using `errors.Is(err, io.EOF)`.

#### Scenario: EOF ends stream normally
- **WHEN** the underlying stream returns an error where `errors.Is(err, io.EOF)` is true
- **THEN** the helper MUST end the stream normally
- **AND** it MUST NOT emit an `error` SSE event

#### Scenario: Real error emits error event
- **WHEN** the underlying stream returns a non-EOF error
- **THEN** the helper MUST emit an `error` SSE event (EventTypeError)
- **AND** it MUST NOT treat the error as a normal `done`
