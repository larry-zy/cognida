## Context

The project already follows Clean Architecture with clear layer separation:
- `internal/domain` - Domain entities and repository interfaces
- `internal/application/usecases` - Application business logic
- `internal/infrastructure` - External service implementations
- `internal/interface` - HTTP handlers and middleware

However, the `pkg` directory remains a legacy collection of utilities from before the Clean Architecture adoption. It contains:
- HTTP response utilities (`pkg/response`) - interface layer concern
- Pagination types (`pkg/page`) - application/dto layer concern
- Business errors (`pkg/errors`) - domain layer concern
- JWT implementation (`pkg/jwt`) - infrastructure layer concern
- Password hashing (`pkg/crypto`) - infrastructure layer concern
- Document parsing (`pkg/parser`) - infrastructure layer concern
- Type conversions (`pkg/convert`) - shared utility concern
- Common utilities (`pkg/utils.go`) - mixed concerns

**Current State**: The `pkg` package creates confusion about where to place new code and violates dependency rules (e.g., infrastructure code in a package that could be imported by any layer).

**Constraints**:
- Must maintain backward compatibility of behavior - no functional changes
- Must update all imports consistently across the codebase
- Wire dependency injection configuration must remain functional

## Goals / Non-Goals

**Goals:**
1. Eliminate the `pkg` package entirely by relocating all code to appropriate Clean Architecture layers
2. Establish clear patterns for utility placement (what goes where)
3. Update all imports to reference new locations
4. Maintain all existing functionality without behavioral changes

**Non-Goals:**
1. No changes to HTTP API contracts or responses
2. No changes to business logic or error handling behavior
3. No new functionality - this is pure relocation

## Decisions

### 1. `pkg/response` → `internal/interface/http/response`

**Decision**: Move HTTP response utilities to the interface layer.

**Rationale**: The response package contains Gin-specific JSON response helpers. These are interface/presentation layer concerns, closely tied to the HTTP framework.

**Alternatives considered**:
- Keep in `pkg`: Rejected - violates layering, allows infrastructure to leak
- Create `internal/pkg/response`: Rejected - still unclear which layer should use it

### 2. `pkg/page` → `internal/application/dto/page`

**Decision**: Move pagination types to application DTO layer.

**Rationale**: Pagination is a data transfer concern between interface and application layers. It belongs with other DTOs.

**Alternatives considered**:
- Move to `internal/domain`: Rejected - pagination is not a domain concept
- Move to `internal/interface/http`: Rejected - pagination should be reusable by other interfaces (e.g., gRPC in future)

### 3. `pkg/errors` → `internal/domain/errors` (merge)

**Decision**: Merge `pkg/errors` into `internal/domain/errors`.

**Rationale**: Business errors (BizError) are domain concerns. The existing `internal/domain/errors` already has similar error types.

**Action**: Combine error definitions, keeping all error codes and messages. Ensure all references use `internal/domain/errors`.

### 4. `pkg/jwt` → `internal/infrastructure/auth/jwt`

**Decision**: Move JWT implementation to infrastructure layer under auth package.

**Rationale**: JWT is an external concern (jwt library) - a classic infrastructure implementation detail. The domain should define authentication interfaces, infrastructure provides JWT implementation.

**Alternatives considered**:
- Create `internal/infrastructure/security/jwt`: Rejected - auth is more descriptive
- Keep in `pkg`: Rejected - allows application layer to depend on concrete implementation

### 5. `pkg/crypto` → `internal/infrastructure/crypto`

**Decision**: Move password hashing to infrastructure layer.

**Rationale**: Cryptography is an infrastructure concern. The Argon2id implementation depends on external libraries.

### 6. `pkg/parser` → `internal/infrastructure/document/parser`

**Decision**: Move document parsing to infrastructure layer under document package.

**Rationale**: Document parsing involves I/O and external libraries (PDF, etc.) - pure infrastructure.

**Note**: There's already `internal/infrastructure/document/chunker` - this fits naturally alongside it.

### 7. `pkg/convert` → `internal/pkg/convert`

**Decision**: Move type conversion utilities to `internal/pkg/convert`.

**Rationale**: Type conversion is a pure utility without external dependencies. It can be safely used by any layer. The `internal/pkg` pattern is appropriate for truly shared utilities.

**Alternative considered**: Delete unused code - but the convert package is used in multiple places for SQL scan/value implementations.

### 8. `pkg/utils.go` content distribution

**Decision**: Distribute contents based on function:
- UUID/time utilities → `internal/pkg/uuid` or merge with convert
- Or delete if unused after audit

**Rationale**: Need to audit actual usage. Many utility functions may be redundant.

### 9. Import path updates

**Decision**: Use automated find-replace for import updates.

**Pattern**: `github.com/yourusername/link/pkg/*` → `github.com/yourusername/link/internal/...`

**Verification**: Run `go build ./...` after changes to catch any missed imports.

## Risks / Trade-offs

### Risk: Missed import references
**Risk**: Some files may import from `pkg` that aren't caught by simple grep.
**Mitigation**: Run `go build ./...` and `go test ./...` after refactoring to catch compilation errors.

### Risk: Wire dependency injection breaks
**Risk**: Wire generates code based on type locations - moved types may break wire generation.
**Mitigation**: Regenerate wire with `wire gen` after refactoring.

### Risk: Circular dependencies
**Risk**: Moving code could create unexpected circular imports between layers.
**Mitigation**: The chosen destinations follow dependency direction (domain → application → infrastructure → interface). Verify with `go list -f '{{.ImportPath}} {{.Imports}}'` if needed.

### Risk: External consumers
**Risk**: If other projects import this project's `pkg` package, they will break.
**Mitigation**: This is an internal project. If external consumers exist, they should use the proper layer APIs anyway.

## Migration Plan

### Phase 1: Create new locations
1. Create all target directories
2. Move files to new locations (copy first, don't delete yet)
3. Update package declarations if needed

### Phase 2: Update imports
1. Find all imports of `github.com/yourusername/link/pkg/*`
2. Replace with new paths
3. Run `go mod tidy`

### Phase 3: Verify
1. Run `go build ./...`
2. Run `go test ./...`
3. Regenerate wire: `wire gen ./cmd/wire`
4. Build and test again

### Phase 4: Cleanup
1. Delete original `pkg` directory
2. Final verification

### Rollback strategy
Keep the `pkg` directory until after full verification. Git provides easy rollback if issues arise.

## Open Questions

1. **Should `internal/pkg/convert` and `internal/pkg/uuid` be merged?**
   - Audit usage first. If small, merge. If distinct use cases, keep separate.

2. **Should we create domain interfaces for auth before moving JWT?**
   - Ideally yes, but out of scope for this refactoring. Can be done as follow-up.

3. **What about `pkg/parser` - should it be abstracted behind an interface?**
   - Good practice but out of scope. Current goal: correct location by layer.
