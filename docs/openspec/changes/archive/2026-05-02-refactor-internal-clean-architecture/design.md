## Context

The current `internal/` package structure follows a rough Clean Architecture but has significant violations that have accumulated over time:

1. **Domain layer pollution**: Domain entities like `AgentRequest`, `AgentResponse`, `ChatChunk` contain application-layer DTOs
2. **Service interfaces in domain**: `UserService`, `SessionService` in `domain/types/interfaces/` should be application use cases
3. **Mixed responsibilities**: Application layer contains both business logic (similarity calculations) and orchestration
4. **Inconsistent organization**: Some modules are feature-based (agent, chat), others are layer-based (repository)

### Current State Analysis

```
internal/domain/agent/
├── entity.go        # Contains AgentRequest, AgentResponse (should be in app)
├── repository.go    # Contains AgentOrchestrator interface (mixed abstraction)

internal/domain/types/interfaces/
├── user.go          # UserService interface (should be application use case)
├── session.go       # SessionService interface (should be application use case)
└── message.go       # MessageService interface (should be application use case)

internal/application/
├── agent/service.go # Contains similarity calculation (domain logic)
└── repository/      # Empty directory - wrong placement
```

### Constraints

- Must maintain backward compatibility during migration
- Cannot break existing API contracts
- Need to support incremental migration
- Tests must continue to pass

## Goals / Non-Goals

**Goals:**

1. Establish clear layer boundaries following Clean Architecture principles
2. Domain layer contains only business entities, value objects, and domain services
3. Application layer organizes by use cases within bounded contexts
4. Infrastructure depends only on domain interfaces
5. Enable independent testing of each layer

**Non-Goals:**

- Changing external API contracts (HTTP handlers remain compatible)
- Migrating to a different architectural pattern (e.g., hexagonal, onion)
- Complete rewrite - this is a restructuring, not a new implementation
- Changing the technology stack (still Go, still using Gin, GORM, etc.)

## Decisions

### Decision 1: Bounded Context Organization

**Choice**: Organize by bounded contexts (agent, chat, rag, kb, user, tenant) rather than pure layers.

**Rationale**:
- Feature-based organization is more intuitive for developers
- Each bounded context has its own entities, use cases, and interfaces
- Aligns with Domain-Driven Design principles
- Easier to scale as features grow

**Alternatives considered**:
- Pure layer organization (all entities in one folder) - rejected due to poor discoverability
- Mixed organization (current state) - rejected due to confusion

### Decision 2: Service Interface Placement

**Choice**: Move service interfaces from `domain/types/interfaces/` to `application/usecases/<context>/interfaces.go`.

**Rationale**:
- Service interfaces represent application use cases, not domain concepts
- Keeps interfaces close to their implementations
- Domain interfaces should only contain repository interfaces

**Alternatives considered**:
- Keep in domain - rejected because services orchestrate, they don't define business rules
- Create separate interfaces package - rejected as over-engineering

### Decision 3: Domain Services vs Application Services

**Choice**: Introduce `domain/services/` for business logic that spans multiple entities.

**Rationale**:
- Some logic doesn't belong in entities but is still business logic
- Examples: similarity calculation, query analysis, fact extraction
- Application services should only orchestrate, not contain business rules

**Structure**:
```
domain/services/
├── similarity.go     # Text similarity algorithms
├── analysis.go       # Query analysis logic
└── validation.go     # Business validation rules
```

### Decision 4: Migration Strategy

**Choice**: Incremental migration with parallel structure.

**Rationale**:
- Can't afford to break everything at once
- Allows testing each bounded context independently
- Rollback is safer if something goes wrong

**Process**:
1. Create new structure for one bounded context
2. Migrate code while keeping old path working
3. Update imports gradually
4. Remove old code once fully migrated

## Risks / Trade-offs

### Risk 1: Import Breakage During Migration

**Risk**: Moving types will break imports across the codebase.

**Mitigation**:
- Use gofix-like tooling to update imports automatically
- Create temporary re-exports in old locations
- Run full test suite after each bounded context migration

### Risk 2: Confusion During Transition

**Risk**: Having both old and new structures simultaneously causes confusion.

**Mitigation**:
- Add `// TODO: remove after migration` comments
- Document migration progress in README
- Use feature flags if necessary

### Risk 3: Over-Abstraction

**Risk**: Creating too many layers and indirections.

**Mitigation**:
- Keep domain services minimal
- Don't create interfaces for everything
- YAGNI principle - only add layers when needed

### Risk 4: Repository Interface Placement

**Risk**: Domain repository interfaces may reference too much application detail.

**Mitigation**:
- Keep repository interfaces focused on persistence operations
- Use domain entities in repository interfaces, not DTOs
- Complex queries go in application layer, orchestrate simple repo calls

## Migration Plan

### Phase 1: Foundation (Week 1)

1. Create new directory structure alongside existing code
2. Set up `domain/services/` package
3. Create `application/usecases/` base structure
4. Document migration guidelines

### Phase 2: Incremental Migration (Weeks 2-4)

Migrate one bounded context at a time:
1. **Week 2**: `agent` context
2. **Week 3**: `chat` and `rag` contexts
3. **Week 4**: `kb`, `user`, `tenant` contexts

For each context:
1. Extract domain services from application layer
2. Move DTOs from domain to application usecases
3. Reorganize use case interfaces
4. Update imports
5. Verify tests pass

### Phase 3: Cleanup (Week 5)

1. Remove old directories
2. Update documentation
3. Final verification

### Rollback Strategy

- Each phase can be rolled back independently
- Git branches for each bounded context migration
- Keep old code until migration is verified

## Open Questions

1. **How to handle cross-cutting concerns like authentication?**
   - Option: Create `application/usecases/auth/` or middleware package?
   - **Decision**: Use middleware in `interface/http/middleware/` for HTTP-level, use case for application-level

2. **Should domain entities have methods or use domain services?**
   - Option: Rich entities with methods vs anemic entities with services
   - **Decision**: Lean toward rich entities, but use domain services for operations that span multiple entities

3. **How to handle shared types like pagination?**
   - Option: Put in `domain/types/` or create `shared/types/`?
   - **Decision**: Keep in `domain/types/` as they're used across bounded contexts
