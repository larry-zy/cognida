# Internal Package Structure

This directory follows **Clean Architecture** principles with clear layer separation.

## Architecture Overview

```
internal/
├── domain/                    # Enterprise business rules
│   ├── <context>/            # Bounded contexts (agent, chat, rag, kb, user, tenant)
│   │   ├── entity.go         # Core business entities
│   │   ├── repository.go     # Repository interfaces (data access contracts)
│   │   └── errors.go         # Domain-specific errors
│   └── services/             # Cross-cutting domain services
│       └── similarity.go     # Example: similarity calculation service
│
├── application/              # Application business rules (use cases)
│   └── usecases/
│       └── <context>/        # One per bounded context
│           ├── <usecase>.go  # Use case orchestrators
│           ├── dto.go        # Request/response DTOs
│           └── interfaces.go # Use case interfaces for dependency injection
│
├── infrastructure/           # External interfaces (DB, APIs, etc.)
│   ├── persistence/          # Database implementations
│   ├── llm/                  # LLM service implementations
│   └── rag/                  # RAG infrastructure implementations
│
└── interface/                # Frameworks & drivers (HTTP, gRPC, etc.)
    └── http/                 # HTTP handlers, middleware, routers
```

## Layer Dependencies

**Dependency Rule:** Dependencies point inward. Outer layers depend on inner layers.

```
Interface → Application → Domain ← Infrastructure
```

- **Domain**: No dependencies on other layers. Pure business logic.
- **Application**: Depends only on Domain. Orchestrates use cases.
- **Infrastructure**: Implements Domain interfaces. Depends only on Domain.
- **Interface**: Depends on Application. Handles HTTP/RPC/etc.

## Bounded Contexts

Each bounded context represents a business domain:

| Context | Description |
|---------|-------------|
| `agent` | AI agent orchestration (AgenticRAG, Deep Research) |
| `chat` | Chat sessions and message management |
| `rag` | Retrieval Augmented Generation |
| `kb` | Knowledge base management |
| `user` | User authentication and profiles |
| `tenant` | Multi-tenant management |

## Migration Guidelines

### When Adding New Code

1. **Business Entity?** → `domain/<context>/entity.go`
2. **Data Access?** → Define interface in `domain/<context>/repository.go`, implement in `infrastructure/persistence/`
3. **Use Case/Orchestration?** → `application/usecases/<context>/<usecase>.go`
4. **External API Integration?** → `infrastructure/`
5. **HTTP Handler?** → `interface/http/handler/`

### Things to Avoid

- ❌ DTOs in domain entities (put them in `application/usecases/<context>/dto.go`)
- ❌ Business logic in application layer (put domain services in `domain/services/`)
- ❌ Infrastructure types in domain (use interfaces, implement in infrastructure)
- ❌ Importing infrastructure packages from domain or application

## Examples

### Domain Entity

```go
// domain/agent/entity.go
package agent

type Agent struct {
    ID          string
    Name        string
    Type        AgentType
    Status      AgentStatus
    Config      *AgentConfig
}
```

### Domain Repository Interface

```go
// domain/agent/repository.go
package agent

type AgentRepository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id string) (*Agent, error)
}
```

### Application Use Case

```go
// application/usecases/agent/execute.go
package agent

type ExecuteUseCase struct {
    agentRepo domain.AgentRepository
    // ... other dependencies
}

func (uc *ExecuteUseCase) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
    // Orchestrate the flow, delegate to domain services
}
```

### Infrastructure Implementation

```go
// infrastructure/persistence/mysql/agent_repo.go
package mysql

type AgentRepository struct {
    db *gorm.DB
}

func (r *AgentRepository) Save(ctx context.Context, agent *domain.Agent) error {
    // MySQL implementation
}
```

## Current Migration Status

This structure is being actively migrated. Some old patterns may still exist.
See `openspec/changes/refactor-internal-clean-architecture/` for migration progress.
