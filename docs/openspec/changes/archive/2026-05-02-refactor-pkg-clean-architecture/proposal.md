## Why

The `pkg` package currently contains mixed concerns that violate Clean Architecture principles - it holds utilities, HTTP handlers, business errors, and infrastructure implementations without clear separation. This makes the codebase harder to maintain, test, and reason about. Refactoring `pkg` to align with Clean Architecture will establish clear layer boundaries and improve code organization.

## What Changes

- **Delete `pkg/response`** - Move HTTP response utilities to `internal/interface/http/response`
- **Delete `pkg/page`** - Move pagination to `internal/application/dto/page`
- **Delete `pkg/errors`** - Merge business errors with `internal/domain/errors`
- **Delete `pkg/jwt`** - Move JWT implementation to `internal/infrastructure/auth/jwt`
- **Delete `pkg/crypto`** - Move password hashing to `internal/infrastructure/crypto`
- **Delete `pkg/parser`** - Move document parsing to `internal/infrastructure/document/parser`
- **Delete `pkg/convert`** - Move type conversion utilities to `internal/pkg/convert` or delete unused code
- **Delete `pkg/utils.go`** - Move remaining utilities to appropriate locations
- **BREAKING**: All imports referencing `pkg/*` will need to be updated throughout the codebase

## Capabilities

### New Capabilities

None - this is an internal refactoring that does not introduce new capabilities. The existing functionality will be preserved and relocated to appropriate layers.

### Modified Capabilities

None - this refactoring does not change the behavior or requirements of existing capabilities. It only reorganizes code location according to Clean Architecture principles.

## Impact

- **Import paths**: All files importing from `pkg/*` will need updated imports
- **Wire dependency injection**: `cmd/wire/wire.go` may need updates for moved types
- **Tests**: All test files will need updated imports
- **Build**: Go module imports must be updated consistently across the project
- **No external impact**: This is an internal refactoring with no API/contract changes
