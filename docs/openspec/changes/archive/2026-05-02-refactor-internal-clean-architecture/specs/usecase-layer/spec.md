# Use Case Layer

## ADDED Requirements

### Requirement: Use cases organized by bounded context
Application layer SHALL be organized as application/usecases/<context>/ where each context represents a business domain.

#### Scenario: Use case directory structure
- **GIVEN** the application layer
- **WHEN** examining its structure
- **THEN** each bounded context has its own directory (agent, chat, rag, kb, user, tenant)
- **AND** each directory contains use case orchestrators and DTOs

### Requirement: Use case orchestrator pattern
Each use case SHALL be implemented as an orchestrator that coordinates domain services and repositories.

#### Scenario: Use case orchestrator structure
- **GIVEN** a use case like AgentExecute
- **WHEN** examining its implementation
- **THEN** it resides in application/usecases/agent/execute.go
- **AND** it orchestrates domain services and repositories
- **AND** it does NOT contain business logic

### Requirement: DTOs co-located with use cases
Request and response DTOs SHALL reside in the same package as their use case orchestrator.

#### Scenario: DTO package placement
- **GIVEN** a use case requiring input/output
- **WHEN** defining its DTOs
- **THEN** they reside in application/usecases/<context>/dto.go
- **AND** they are NOT in the domain layer

### Requirement: Use case interfaces define input/output ports
Each use case SHALL define its interface in a separate interfaces.go file for dependency injection.

#### Scenario: Use case interface definition
- **GIVEN** a use case like ChatUseCase
- **WHEN** defining its contract
- **THEN** the interface resides in application/usecases/chat/interfaces.go
- **AND** it defines only the public methods of the use case

### Requirement: Use case does not contain business logic
Use case orchestrators SHALL NOT contain business logic. Business logic belongs in domain entities or domain services.

#### Scenario: Use case delegates to domain
- **GIVEN** a use case like RAGChat
- **WHEN** executing a chat request
- **THEN** similarity calculation is delegated to domain service
- **AND** query validation is delegated to domain
- **AND** the use case only orchestrates the flow

## MODIFIED Requirements

### Requirement: Application service interfaces
Service interfaces previously in domain/types/interfaces/ SHALL now be defined as use case interfaces in application/usecases/<context>/interfaces.go.

#### Scenario: UserService as use case interface
- **GIVEN** the UserService interface
- **WHEN** locating its definition
- **THEN** it resides in application/usecases/user/interfaces.go
- **AND** it defines methods like Register, Login, Logout
- **AND** it is NOT in domain/types/interfaces/user.go

### Requirement: DTO separation from domain
Request/response types previously in domain entities SHALL be moved to application use case packages.

#### Scenario: AgentRequest in application layer
- **GIVEN** the AgentRequest type
- **WHEN** examining its location
- **THEN** it resides in application/usecases/agent/dto.go
- **AND** it is NOT in domain/agent/entity.go
