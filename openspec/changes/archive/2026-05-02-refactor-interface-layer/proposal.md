## Why

The `internal/interface/http/` layer has accumulated technical debt and architectural inconsistencies that violate Clean Architecture principles. Handler files contain mixed responsibilities (e.g., `handlers.go` bundles AuthHandler, SessionHandler, MessageHandler, ChatHandler, and TenantHandler), response formats are inconsistent across handlers (some use `gin.H`, others use custom `{code, message, data}` format), and SSE utility functions are duplicated. This makes maintenance difficult and creates confusion for developers.

## What Changes

### Restructure Handler Files
- Split `handlers.go` into separate files: `auth_handler.go`, `session_handler.go`, `message_handler.go`, `chat_handler.go`, `tenant_handler.go`
- Create separate `dto.go` files for request/response DTOs currently defined inline in handler files
- Move SSE utility functions to a shared `sse.go` file

### Standardize Response Format
- Consolidate to a single response format using the `response` package
- Update all handlers to use `response.Success()`, `response.Error()`, and other helper functions
- Remove inconsistent `{code, message, data}` wrappers in favor of standard HTTP status codes

### Clean Up Middleware
- Reorganize middleware into separate files by concern
- Improve `request_id` generation using proper UUID library
- Consolidate duplicate Apply() methods into cleaner interface

### Remove Unused/Dead Code
- Remove unused handlers and routes marked as TODO
- Clean up commented-out route definitions
- Remove test files that don't test actual functionality (e.g., `evaluation_handler_test.go` only tests JSON parsing)

### Fix Architectural Violations
- Ensure handlers only depend on application layer use cases
- Remove direct domain/infrastructure imports from handlers where they exist
- Verify all interfaces properly implement Clean Architecture dependency rule

## Capabilities

### New Capabilities
- `http-response-standardization`: Unified response format across all HTTP handlers
- `sse-helper`: Server-Sent Events utility functions for streaming responses

### Modified Capabilities
None - this refactoring maintains existing API contracts and only changes internal implementation.

## Impact

### Affected Code
- `internal/interface/http/handler/*.go` - All handler files will be reorganized
- `internal/interface/http/middleware/auth.go` - May be split into multiple files
- `internal/interface/http/response/response.go` - Enhanced with additional helpers if needed
- `internal/interface/http/router/router.go` - Updated to use restructured handlers

### External APIs
- No breaking changes to external API contracts
- Response formats will be standardized but remain backwards compatible where possible

### Dependencies
- May add `github.com/google/uuid` for proper request ID generation
- No new external service dependencies

### Testing
- Existing unit tests will be updated to match new structure
- New tests will be added for SSE helper functions
- Integration tests should pass without modification (API contracts unchanged)
