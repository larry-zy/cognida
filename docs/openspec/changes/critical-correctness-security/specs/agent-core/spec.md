# agent-core Specification

## ADDED Requirements

### Requirement: Stream error distinguishes EOF from real errors
The agent streaming loop SHALL distinguish normal end-of-stream from real errors using `errors.Is(err, io.EOF)` and MUST report non-EOF errors.

#### Scenario: EOF treated as normal completion
- **WHEN** `eino_agent.go` receives an error from the stream where `errors.Is(err, io.EOF)` is true
- **THEN** it MUST treat it as normal completion
- **AND** it MUST NOT report it as a failure

#### Scenario: Non-EOF error is reported
- **WHEN** the stream returns an error where `errors.Is(err, io.EOF)` is false
- **THEN** the agent MUST report the error upstream
- **AND** it MUST NOT silently swallow it as end-of-stream

### Requirement: StreamReader lifecycle without deferred leak
The agent SHALL close each `StreamReader` per iteration and MUST NOT accumulate `defer Close()` inside a loop.

#### Scenario: Reader closed each iteration
- **WHEN** the agent obtains a `StreamReader` within a loop iteration
- **THEN** it MUST close that reader before the next iteration
- **AND** it MUST NOT rely on a loop-body `defer` that only fires at function return

### Requirement: RAG streaming does not drop chunks
The RAG streaming pipeline SHALL send chunks with a blocking send that also honors cancellation, and MUST NOT drop chunks via a `default` branch on a full buffer.

#### Scenario: Blocking send replaces default-drop
- **WHEN** `pipeline.go` sends a retrieved chunk to the downstream channel
- **THEN** it MUST use `select { case ch <- chunk: case <-ctx.Done(): return }`
- **AND** it MUST NOT use `select { case ch <- chunk: default: }` that silently drops the chunk

#### Scenario: Cancellation stops the pipeline
- **WHEN** the context is cancelled during streaming
- **THEN** the pipeline MUST stop sending and return the context error

### Requirement: Agent SSE producers honor cancellation
Agent SSE producers SHALL detect client disconnection and terminate upstream work via `ctx.Done()` on every downstream channel send.

#### Scenario: streamAgentChunks stops on cancel
- **WHEN** `streamAgentChunks` sends a chunk downstream
- **THEN** the send MUST be `select { case ch <- chunk: case <-ctx.Done(): return }`
- **AND** upstream generation MUST stop when the client disconnects
