# HTTP Response Standardization Specification

## Purpose
Define the standard response format and structure for all HTTP handlers in the interface layer to ensure consistency and maintainability.

## ADDED Requirements

### Requirement: Unified Response Structure
All HTTP handlers SHALL use a unified response structure defined in the `response` package. The response structure SHALL consist of a status code, message, and optional data field.

#### Scenario: Success response with data
- **GIVEN** a handler successfully processes a request with data to return
- **WHEN** the handler calls `response.Success(data)`
- **THEN** the response SHALL have HTTP status 200
- **AND** the response body SHALL contain `{"code": 0, "message": "success", "data": <data>}`

#### Scenario: Success response with custom message
- **GIVEN** a handler successfully processes a request with a custom success message
- **WHEN** the handler calls `response.SuccessWithMessage(message, data)`
- **THEN** the response SHALL have HTTP status 200
- **AND** the response body SHALL contain `{"code": 0, "message": <message>, "data": <data>}`

#### Scenario: Error response
- **GIVEN** a handler encounters an error during processing
- **WHEN** the handler calls `response.Error(code, message)`
- **THEN** the response SHALL have appropriate HTTP status code (4xx or 5xx)
- **AND** the response body SHALL contain `{"code": <code>, "message": <message>}`

### Requirement: Handler Response Consistency
All HTTP handlers SHALL use response package helpers instead of directly constructing `gin.H` responses or custom response structures.

#### Scenario: No direct gin.H responses in handlers
- **GIVEN** any handler file in `internal/interface/http/handler/`
- **WHEN** inspecting the code for `c.JSON(http.Status*, gin.H{...})` patterns
- **THEN** these patterns MUST NOT exist
- **AND** all responses MUST use `response.*()` helpers instead

#### Scenario: No custom code/message/data wrappers
- **GIVEN** any handler file in `internal/interface/http/handler/`
- **WHEN** inspecting for manually constructed `{"code": ..., "message": ..., "data": ...}` responses
- **THEN** these patterns MUST NOT exist
- **AND** all responses MUST use the standardized `response` package

### Requirement: Paginated Response Format
Handlers that return paginated lists SHALL use the `response.PageSuccess()` helper with consistent pagination metadata.

#### Scenario: Paginated list response
- **GIVEN** a handler returns a paginated list of items
- **WHEN** the handler calls `response.PageSuccess(total, list, page, size)`
- **THEN** the response SHALL contain `{"code": 0, "message": "success", "data": {"total": <total>, "list": <list>, "page": <page>, "size": <size>}}`

#### Scenario: Paginated JSON helper
- **GIVEN** a handler returns a paginated list
- **WHEN** the handler calls `response.PageSuccessJSON(c, total, list, page, size)`
- **THEN** the response SHALL be sent with HTTP status 200
- **AND** the response body SHALL match the paginated format

### Requirement: HTTP Status Code Mapping
Response helpers SHALL map business codes to appropriate HTTP status codes following REST conventions.

#### Scenario: Success codes map to 2xx
- **GIVEN** a successful operation
- **WHEN** using `response.Success()` or `response.SuccessWithMessage()`
- **THEN** the HTTP status code SHALL be 200 OK
- **AND** created resources MAY use 201 Created via `response.Created()`

#### Scenario: Client error codes map to 4xx
- **GIVEN** a client error (invalid input, unauthorized, etc.)
- **WHEN** using `response.BadRequest()`, `response.Unauthorized()`, or `response.Forbidden()`
- **THEN** the HTTP status code SHALL be 400, 401, or 403 respectively

#### Scenario: Not found maps to 404
- **GIVEN** a requested resource is not found
- **WHEN** using `response.NotFound()`
- **THEN** the HTTP status code SHALL be 404

#### Scenario: Server error maps to 5xx
- **GIVEN** an internal server error occurs
- **WHEN** using `response.InternalError()`
- **THEN** the HTTP status code SHALL be 500

### Requirement: SSE Response Format
Handlers that use Server-Sent Events (SSE) SHALL use the standardized SSE helper functions with consistent event types.

#### Scenario: SSE content event
- **GIVEN** a handler streams content chunks via SSE
- **WHEN** calling `sendSSE(w, "content", chunk)`
- **THEN** the event SHALL be formatted as `event: content\ndata: <chunk-json>\n\n`

#### Scenario: SSE done event
- **GIVEN** a handler completes an SSE stream
- **WHEN** calling `sendSSE(w, "done", chunk)`
- **THEN** the event SHALL signal stream completion to the client
