# Interface Layer Refactoring Design

## Context

### Current State
The `internal/interface/http/` layer has accumulated technical debt:
- `handlers.go` bundles 5 different handlers (AuthHandler, SessionHandler, MessageHandler, ChatHandler, TenantHandler) violating single responsibility principle
- Response formats are inconsistent: some use `gin.H`, others use custom `{code, message, data}` format, and a few use the `response` package
- SSE utility functions (`sendSSE`) are duplicated in multiple handler files
- `evaluation_handler_test.go` only tests JSON parsing, not actual handler logic
- Middleware file contains multiple concerns that could be better organized
- Request ID generation uses a weak random string implementation

### Constraints
- API contracts must remain stable (no breaking changes for clients)
- Existing routes and endpoints must continue to work
- Wire dependency injection configuration must be updated
- Integration tests should pass without modification

### Stakeholders
- Backend developers working on the codebase
- API consumers (frontend, mobile apps) depending on stable endpoints

## Goals / Non-Goals

**Goals:**
- Reorganize handler files following single responsibility principle (one handler type per file)
- Standardize all responses to use the `response` package helpers
- Extract SSE utilities to a shared package
- Clean up unused/dead code and TODO comments
- Improve middleware organization and request ID generation
- Ensure Clean Architecture dependency compliance

**Non-Goals:**
- Changing API contracts or request/response schemas for external clients
- Modifying business logic (that belongs in application layer)
- Changing the routing structure or URL patterns
- Performance optimization (this is a structural refactor)

## Decisions

### 1. Handler File Split Strategy
**Decision:** Split `handlers.go` into separate files by handler type, each with its own `dto.go` for request/response types.

**Rationale:**
- One handler per file follows Go conventions and improves code navigation
- Separating DTOs into their own files keeps handler logic focused
- Easier to review changes in pull requests

**Alternatives considered:**
- Keep handlers in one file but better organized: Still violates SRP and file gets too large
- Create subdirectories per handler: Over-complication for the number of handlers we have

### 2. Response Format Standardization
**Decision:** Use the existing `response` package as the single source of truth, removing all `gin.H` and custom response wrappers.

**Rationale:**
- The `response` package already has good helper functions
- Consistent responses make frontend integration easier
- Centralized error handling and status code mapping

**Migration approach:**
- Replace `c.JSON(http.StatusOK, gin.H{"error": err})` with `response.InternalError(c, err.Error())`
- Replace `c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})` with `response.OK(c, data)`
- Keep HTTP status codes semantic (use 4xx for client errors, 5xx for server errors)

### 3. SSE Helper Package
**Decision:** Create `internal/interface/http/sse/sse.go` with centralized SSE utilities.

**Rationale:**
- Eliminates code duplication (sendSSE currently duplicated in handlers.go and used elsewhere)
- Consistent SSE event format across all streaming endpoints
- Easier to add SSE-specific features (reconnection, heartbeat, etc.)

**Implementation:**
```go
// sse.go
package sse

import (
    "encoding/json"
    "fmt"
    "net/http"
)

func SetSSEHeaders(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
}

func SendSSE(w http.ResponseWriter, eventType string, data interface{}) {
    fmt.Fprintf(w, "event: %s\n", eventType)
    jsonData, _ := json.Marshal(data)
    fmt.Fprintf(w, "data: %s\n\n", jsonData)
    if f, ok := w.(http.Flusher); ok {
        f.Flush()
    }
}
```

### 4. Middleware Organization
**Decision:** Split `middleware/auth.go` into separate files by middleware type when file grows beyond 300 lines.

**Rationale:**
- Currently auth.go is ~250 lines - approaching the threshold
- Each middleware type (Auth, Tenant, CORS, Recovery, Logger, Trace) is independent
- Improves discoverability

**Files to create if needed:**
- `middleware/auth.go` - AuthMiddleware (keep)
- `middleware/tenant.go` - TenantMiddleware (extract from auth.go)
- `middleware/cors.go` - CORSMiddleware (extract from auth.go)
- `middleware/recovery.go` - RecoveryMiddleware (extract from auth.go)
- `middleware/logger.go` - LoggerMiddleware (extract from auth.go)
- `middleware/trace.go` - TraceMiddleware (extract from auth.go)

### 5. Request ID Generation
**Decision:** Replace custom `randomString` with `github.com/google/uuid` for request ID generation.

**Rationale:**
- Current implementation has poor randomness (modulo bias)
- UUID v7 or v4 provides globally unique identifiers
- Industry standard approach

**Migration:**
```go
import "github.com/google/uuid"

func generateRequestID() string {
    return uuid.New().String()
}
```

### 6. Wire Provider Updates
**Decision:** Update `cmd/wire/provider.go` to reflect new handler constructor signatures after file split.

**Rationale:**
- Wire needs explicit provider functions for dependency injection
- File split doesn't change constructors, but we need to verify

## Target Structure

After refactoring, the structure will be:

```
internal/interface/http/
├── router/
│   └── router.go
├── middleware/
│   ├── auth.go       # AuthMiddleware
│   ├── cors.go       # CORSMiddleware
│   ├── recovery.go   # RecoveryMiddleware
│   ├── logger.go     # LoggerMiddleware
│   └── trace.go      # TraceMiddleware (with improved request ID)
├── handler/
│   ├── auth_handler.go         # AuthHandler + constructor
│   ├── auth_handler_dto.go     # AuthHandler DTOs
│   ├── session_handler.go      # SessionHandler + constructor
│   ├── session_handler_dto.go  # SessionHandler DTOs
│   ├── message_handler.go      # MessageHandler + constructor
│   ├── message_handler_dto.go  # MessageHandler DTOs
│   ├── chat_handler.go         # ChatHandler + constructor
│   ├── chat_handler_dto.go     # ChatHandler DTOs
│   ├── tenant_handler.go       # TenantHandler + constructor
│   ├── tenant_handler_dto.go   # TenantHandler DTOs
│   ├── agent_handler.go        # Already separate
│   ├── evaluation_handler.go   # Already separate
│   ├── evaluation_handler_dto.go # New: extract DTOs
│   ├── graph_handler.go        # Already separate
│   ├── graph_handler_dto.go    # New: extract DTOs
│   ├── kb_handler.go           # Already separate
│   ├── kb_handler_dto.go       # New: extract DTOs
│   └── model_handler.go        # Already separate
├── response/
│   └── response.go
└── sse/
    └── sse.go                  # New: SSE utilities
```

## Risks / Trade-offs

### Risk: Breaking existing Wire configuration
- **Risk:** Splitting handler files might break Wire dependency injection
- **Mitigation:** Run `wire generate` after each file split to catch issues early

### Risk: Response format changes could break clients
- **Risk:** Some clients may depend on exact response format
- **Mitigation:** The change is mostly internal - we're standardizing to what many handlers already do. Verify with integration tests.

### Risk: Large diff may be hard to review
- **Risk:** Refactoring many files at once creates a large PR
- **Mitigation:** Break into logical chunks (handlers, response, SSE, middleware) for easier review

### Trade-off: More files vs. better organization
- **Trade-off:** We'll have ~30 files instead of ~10
- **Justification:** Better organization and SRP outweigh file count concerns. Go tooling handles this well.

## Migration Plan

### Phase 1: Create SSE Helper (No Breaking Changes)
1. Create `internal/interface/http/sse/sse.go`
2. Add SSE helper functions
3. Update one handler (e.g., AgentHandler) to use new SSE package
4. Verify tests pass

### Phase 2: Split Handler Files
1. Create individual handler files from `handlers.go`
2. Extract DTOs to separate `*_dto.go` files
3. Update imports in router.go
4. Run `wire generate` and fix any issues
5. Delete original `handlers.go`

### Phase 3: Standardize Response Format
1. Update all handlers to use `response` package helpers
2. Remove inline `gin.H` responses
3. Remove custom `{code, message, data}` wrappers
4. Verify all tests pass

### Phase 4: Extract Inline DTOs
1. Extract inline request/response structs from remaining handlers to `*_dto.go` files
2. For agent_handler.go, evaluation_handler.go, graph_handler.go, model_handler.go, kb_handler.go

### Phase 5: Middleware and Utilities
1. Improve request ID generation with UUID
2. Split middleware/auth.go if it's grown too large
3. Remove unused functions

### Phase 6: Cleanup
1. Remove TODO comments for unimplemented routes
2. Delete or improve `evaluation_handler_test.go`
3. Remove commented-out code

### Rollback Strategy
- Each phase is independently revertable via git
- No database migrations means clean rollback
- API contracts unchanged means no client-side rollback needed

## Open Questions

1. **Should we change the `{code, message, data}` format to match `response` package exactly?**
   - Some handlers use `{code: 0, message: "success", data: {...}}` while others use direct HTTP status codes
   - **Decision:** Standardize on `response` package format which uses HTTP status codes primarily

2. **Should DTO files be in a separate `dto/` subdirectory?**
   - Could do `handler/dto/auth_dto.go` instead of `handler/auth_handler_dto.go`
   - **Decision:** Keep `*_handler_dto.go` pattern for clearer file ownership

3. **Should we keep the Middlewares struct in router.go?**
   - Currently unused, references middleware types
   - **Decision:** Remove if unused, keep if it helps with organization
