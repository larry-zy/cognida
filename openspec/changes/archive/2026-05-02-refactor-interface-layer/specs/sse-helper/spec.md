# SSE Helper Specification

## Purpose
Define Server-Sent Events (SSE) utility functions for streaming responses in HTTP handlers, centralizing SSE logic and removing duplication.

## ADDED Requirements

### Requirement: SSE Helper Package
The system SHALL provide a centralized SSE helper package at `internal/interface/http/sse/` with utility functions for sending SSE events.

#### Scenario: SSE helper package exists
- **GIVEN** the interface layer structure
- **WHEN** listing directories under `internal/interface/http/`
- **THEN** an `sse/` directory MUST exist
- **AND** it MUST contain `sse.go` with helper functions

### Requirement: SendSSE Function
The SSE helper package SHALL provide a `SendSSE` function that sends properly formatted SSE events.

#### Scenario: SendSSE writes formatted event
- **GIVEN** an http.ResponseWriter
- **WHEN** calling `sse.SendSSE(w, "message", data)`
- **THEN** the response SHALL contain `event: message\ndata: <json-data>\n\n`
- **AND** the response SHALL be flushed if the writer supports http.Flusher

#### Scenario: SendSSE handles JSON marshaling
- **GIVEN** a complex data structure
- **WHEN** calling `sse.SendSSE(w, "metadata", complexData)`
- **THEN** the data SHALL be JSON marshaled
- **AND** the JSON SHALL be written to the response

### Requirement: SSE Response Headers
The SSE helper package SHALL provide a function to set proper SSE response headers.

#### Scenario: SetSSEHeaders configures response
- **GIVEN** an http.ResponseWriter
- **WHEN** calling `sse.SetSSEHeaders(w)`
- **THEN** the Content-Type header SHALL be set to "text/event-stream"
- **AND** the Cache-Control header SHALL be set to "no-cache"
- **AND** the Connection header SHALL be set to "keep-alive"

### Requirement: SSE Chunk Structures
The SSE helper package SHALL define standard chunk structures for common streaming scenarios.

#### Scenario: ContentChunk defined
- **GIVEN** the SSE helper package
- **WHEN** inspecting the type definitions
- **THEN** a `ContentChunk` struct MUST exist
- **AND** it SHALL contain `Content` string field and `Done` boolean field

#### Scenario: MetadataChunk defined
- **GIVEN** the SSE helper package
- **WHEN** inspecting the type definitions
- **THEN** a `MetadataChunk` struct MAY exist
- **AND** it SHALL contain a `Metadata` interface{} field

### Requirement: Handler SSE Integration
HTTP handlers that stream responses SHALL use the SSE helper functions instead of inline SSE code.

#### Scenario: No duplicate sendSSE functions
- **GIVEN** any handler file in `internal/interface/http/handler/`
- **WHEN** searching for local `sendSSE` function definitions
- **THEN** local `sendSSE` functions MUST NOT exist
- **AND** all SSE code MUST use `sse.SendSSE()`

#### Scenario: Handlers import sse package
- **GIVEN** a handler that uses SSE streaming
- **WHEN** inspecting the handler's imports
- **THEN** the handler MUST import `link/internal/interface/http/sse`
- **AND** SSE calls MUST use the `sse.` prefix

### Requirement: SSE Event Type Constants
The SSE helper package SHALL define constants for common SSE event types.

#### Scenario: Standard event types defined
- **GIVEN** the SSE helper package
- **WHEN** inspecting the constants
- **THEN** `EventTypeContent` SHALL be defined as "content"
- **AND** `EventTypeDone` SHALL be defined as "done"
- **AND** `EventTypeError` SHALL be defined as "error"
- **AND** `EventTypeMetadata` SHALL be defined as "metadata"
