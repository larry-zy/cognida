## 1. Foundation

- [x] 1.1 Create `domain/services/` package structure
- [x] 1.2 Create `application/usecases/` base directory structure
- [x] 1.3 Create bounded context directories (agent, chat, rag, kb, user, tenant) under usecases
- [x] 1.4 Document migration guidelines in CLAUDE.md or internal/README.md

## 2. Domain Layer Cleanup

- [x] 2.1 Extract similarity calculation from `application/agent/service.go` to `domain/services/similarity.go`
- [x] 2.2 Remove DTOs from `domain/agent/entity.go` (AgentRequest, AgentResponse, ChatChunk) - N/A: These are domain service contracts
- [x] 2.3 Remove DTOs from `domain/chat/entity.go` - N/A: These are repository query parameters
- [x] 2.4 Remove DTOs from `domain/rag/entity.go` - N/A: These are domain service contracts
- [x] 2.5 Create context-specific error packages (domain/agent/errors.go, etc.)
- [x] 2.6 Move AgentOrchestrator interface to domain/agent/repository.go if not already there

## 3. Agent Context Migration

- [x] 3.1 Create `application/usecases/agent/` directory with dto.go, interfaces.go
- [x] 3.2 Move Agent DTOs to `application/usecases/agent/dto.go`
- [x] 3.3 Create use case orchestrators: execute.go, research.go, config.go, progress.go
- [x] 3.4 Move Agent-related service interfaces from `domain/types/interfaces/` to `application/usecases/agent/interfaces.go`
- [x] 3.5 Update imports in `interface/http/handler/agent_handler.go`
- [x] 3.6 Remove old `application/agent/` directory after verification

## 4. Chat Context Migration

- [x] 4.1 Create `application/usecases/chat/` directory with dto.go, interfaces.go
- [x] 4.2 Move Chat/Session/Message DTOs to `application/usecases/chat/dto.go`
- [x] 4.3 Create use case orchestrators: session.go, message.go, chat.go
- [x] 4.4 Move MessageService, SessionService from `domain/types/interfaces/` to `application/usecases/chat/interfaces.go`
- [x] 4.5 Update imports in `interface/http/handler/` for chat-related handlers
- [x] 4.6 Remove old `application/chat/` directory after verification

## 5. RAG Context Migration

- [x] 5.1 Create `application/usecases/rag/` directory with dto.go, interfaces.go
- [x] 5.2 Move RAG DTOs to `application/usecases/rag/dto.go`
- [x] 5.3 Split RAGService into focused use case orchestrators: chat.go, retrieve.go, graph.go, query.go
- [x] 5.4 Update imports for RAG-related code
- [x] 5.5 Remove old `application/rag/` directory after verification

## 6. KB Context Migration

- [x] 6.1 Create `application/usecases/kb/` directory
- [x] 6.2 Move KB DTOs to `application/usecases/kb/dto.go`
- [x] 6.3 Create use case orchestrators for KB operations
- [x] 6.4 Update imports
- [x] 6.5 Remove old `application/kb/` directory after verification

## 7. User Context Migration

- [x] 7.1 Create `application/usecases/user/` directory
- [x] 7.2 Move UserService from `domain/types/interfaces/user.go` to `application/usecases/user/interfaces.go`
- [x] 7.3 Move User DTOs to `application/usecases/user/dto.go`
- [x] 7.4 Create use case orchestrators: auth.go, profile.go
- [x] 7.5 Update imports in handlers and middleware
- [x] 7.6 Remove old `application/user/` directory after verification

## 8. Tenant Context Migration

- [x] 8.1 Create `application/usecases/tenant/` directory
- [x] 8.2 Move Tenant DTOs and interfaces
- [x] 8.3 Create use case orchestrators
- [x] 8.4 Update imports
- [x] 8.5 Remove old `application/tenant/` directory after verification

## 9. Domain Types Interfaces Cleanup

- [x] 9.1 Verify all service interfaces moved from `domain/types/interfaces/`
- [x] 9.2 Move remaining repository interfaces to appropriate domain context
- [x] 9.3 Remove `domain/types/interfaces/` directory if empty - Kept for backward compatibility
- [x] 9.4 Update `domain/types/` to contain only shared types (pagination, etc.)

## 10. Infrastructure Layer Verification

- [x] 10.1 Verify `infrastructure/persistence/` implements correct domain repository interfaces
- [x] 10.2 Verify `infrastructure/llm/` implements domain LLM interfaces
- [x] 10.3 Verify `infrastructure/rag/` implements domain RAG interfaces
- [x] 10.4 Ensure no infrastructure types leak into domain or application layers

## 11. Testing and Validation

- [x] 11.1 Run full test suite and fix any broken tests
- [x] 11.2 Verify all imports are correctly updated
- [x] 11.3 Check for circular dependencies
- [x] 11.4 Verify layer dependencies: Domain ← Application ← Infrastructure ← Interface

## 12. Documentation and Cleanup

- [x] 12.1 Update internal/README.md with new structure
- [x] 12.2 Remove all TODO comments related to migration
- [x] 12.3 Verify no old directories remain
- [x] 12.4 Run `go mod tidy` to clean up dependencies
