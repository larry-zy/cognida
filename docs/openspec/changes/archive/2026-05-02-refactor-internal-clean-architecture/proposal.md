## Why

The current `internal/` package structure has several Clean Architecture violations that make the codebase difficult to maintain and test:

1. **Domain layer pollution**: Domain entities contain request/response DTOs (`AgentRequest`, `AgentResponse`) that belong in the application layer
2. **Service interfaces in wrong layer**: `UserService`, `SessionService`, etc. are in `domain/types/interfaces/` but should represent application use cases
3. **Business logic misplaced**: Similarity calculations, data transformations, and business rules scattered in application layer
4. **Missing clear boundaries**: No distinction between domain services (business logic) and application services (use case orchestration)

These violations lead to tight coupling, reduced testability, and confusion about where to place new code.

## What Changes

### Layer Restructuring

- **Domain Layer (`internal/domain/`)**: Pure business entities, value objects, and domain services. No DTOs, no request/response types.
- **Application Layer (`internal/application/`)**: Use cases organized by bounded context. Each context has its own subdirectory with use case orchestrators.
- **Infrastructure Layer (`internal/infrastructure/`)**: External service implementations, data access, and technical details.

### Specific Changes

- **BREAKING**: Move service interfaces from `domain/types/interfaces/` to appropriate application use case packages
- **BREAKING**: Remove request/response types from domain entities; move to application use case packages
- **NEW**: Introduce `domain/services/` for domain logic that doesn't belong in entities
- **NEW**: Introduce `application/usecases/` with clear use case orchestrators
- **REFACTOR**: Consolidate scattered application services into context-based use case modules

### Directory Structure After Refactor

```
internal/
├── domain/
│   ├── <context>/          # Bounded context (e.g., agent, chat, rag)
│   │   ├── entity.go       # Core business entities
│   │   ├── value_objects.go
│   │   ├── repository.go   # Repository interfaces
│   │   └── errors.go       # Domain-specific errors
│   └── services/           # Cross-cutting domain services
│       └── similarity.go
├── application/
│   └── usecases/
│       └── <context>/
│           ├── <usecase>.go    # Use case orchestrator
│           ├── dto.go          # Request/response DTOs
│           └── interfaces.go   # Input/output ports
├── infrastructure/
│   ├── persistence/       # Repository implementations
│   ├── llm/              # LLM service implementations
│   └── rag/              # RAG infrastructure implementations
└── interface/
    └── http/             # HTTP handlers
```

## Capabilities

### New Capabilities

- `domain-layer-cleanup`: Establish clean domain layer with pure business entities
- `usecase-layer`: Introduce application use case layer with clear orchestrators
- `domain-services`: Extract domain services for business logic

### Modified Capabilities

- `agent-service`: Restructure agent application logic into use cases
- `chat-service`: Restructure chat application logic into use cases
- `rag-service`: Restructure RAG application logic into use cases

## Impact

### Affected Code

- `internal/domain/agent/` - Remove DTOs, reorganize into pure entities
- `internal/domain/chat/` - Remove DTOs, reorganize into pure entities
- `internal/domain/types/interfaces/` - Move service interfaces to application use cases
- `internal/application/` - Reorganize into usecase-based structure

### Breaking Changes

- Import paths for domain entities will change for some types
- Service interfaces will move from `domain/types/interfaces` to `application/usecases/<context>`
- Request/Response DTOs will move from domain to application use case packages

### Migration Strategy

1. Create new structure alongside existing code
2. Gradually migrate each bounded context
3. Update imports incrementally
4. Remove old directories after migration completes
