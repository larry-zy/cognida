# sse-auth-lifecycle Specification

## ADDED Requirements

### Requirement: Unified authenticated SSE reader
The frontend SHALL provide a shared `readSSE(url, body, signal)` helper that injects authentication headers, handles non-2xx responses through the unified error/logout path, and forwards the abort signal.

#### Scenario: readSSE injects auth headers
- **WHEN** `readSSE(url, body, signal)` initiates a streaming request
- **THEN** it MUST include the `Authorization` bearer header from the auth store
- **AND** it MUST include the `X-Tenant-ID` header when a current tenant is set

#### Scenario: readSSE forwards abort signal
- **WHEN** the provided `AbortSignal` is aborted
- **THEN** `readSSE` MUST cancel the underlying fetch stream
- **AND** MUST stop yielding further events

#### Scenario: Duplicate stream implementation removed
- **WHEN** inspecting the frontend SSE code
- **THEN** the duplicate implementation in `chat/stream.ts` MUST be removed
- **AND** chat streaming MUST use the shared `readSSE` helper

### Requirement: 401 handling via refresh
On a 401 during a request, the frontend SHALL clear local auth, attempt `refreshAccessToken`, replay the original request on success, and only redirect to login on failure.

#### Scenario: 401 triggers refresh and replay
- **WHEN** a request receives HTTP 401
- **THEN** the client MUST call the previously-unused `refreshAccessToken`
- **AND** on success it MUST replay the original request
- **AND** it MUST NOT unconditionally call `logout` first

#### Scenario: Refresh failure redirects to login
- **WHEN** `refreshAccessToken` fails
- **THEN** the client MUST clear auth and redirect to the login page

### Requirement: SSE stops retrying on client errors
Evaluation-progress SSE consumption SHALL stop reconnecting on 4xx responses instead of retrying indefinitely.

#### Scenario: 4xx stops reconnection
- **WHEN** an evaluation-progress SSE stream returns a 4xx status
- **THEN** the client MUST stop reconnecting
- **AND** it MUST surface the error rather than looping

#### Scenario: Auth-aware progress stream
- **WHEN** the evaluation-progress stream requires authentication
- **THEN** the client MUST send the auth header (via fetch stream or `readSSE`)
- **AND** MUST NOT rely on a bare `EventSource` that cannot carry auth headers

### Requirement: Go SSE cancellation awareness
Go-side SSE producers SHALL detect client disconnection and terminate upstream generation via `ctx.Done()` on every downstream channel send.

#### Scenario: streamAgentChunks honors ctx.Done
- **WHEN** `streamAgentChunks` sends a chunk to the downstream channel
- **THEN** the send MUST be `select { case ch <- chunk: case <-ctx.Done(): return }`
- **AND** when the client disconnects the upstream generation MUST stop

#### Scenario: streamInternal honors ctx.Done
- **WHEN** `streamInternal` sends to its downstream channel
- **THEN** the send MUST include a `<-ctx.Done()` case that returns
- **AND** no chunk MUST be sent after the context is cancelled
