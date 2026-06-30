# agent-core Specification

## Purpose
TBD - created by archiving change agent-layer-cleanup. Update Purpose after archive.
## Requirements
### Requirement: Domain layer defines core Agent entities
The system SHALL define core Agent entities in the Domain layer without dependencies on external frameworks or application layer concepts.

#### Scenario: Agent entity contains only domain concepts
- **WHEN** inspecting `domain/agent/entity.go`
- **THEN** file contains only: `Agent`, `AgentType`, `AgentStatus`, `AgentConfig`, `HookConfig`, `SearchConfig`
- **AND** file does NOT contain: `ChatChunk`, `ToolCallRecord`, `AgentRequest`, `AgentResponse`

### Requirement: Domain layer defines repository interfaces
The system SHALL define data access interfaces in the Domain layer for Infrastructure layer to implement.

#### Scenario: AgentRepository interface exists
- **WHEN** inspecting `domain/agent/repository.go`
- **THEN** `AgentRepository` interface defines methods: `SaveConfig`, `LoadConfig`, `SaveExecutionRecord`, `FindExecutionRecords`

### Requirement: Domain layer defines service interfaces
The system SHALL define domain service interfaces for capabilities that don't naturally belong to any single entity.

#### Scenario: AgentExecutor service interface exists
- **WHEN** inspecting `domain/agent/service.go`
- **THEN** `AgentExecutor` interface defines methods for executing agent logic
- **AND** `HookService` interface defines lifecycle hook methods

### Requirement: Application layer contains DTOs
The system SHALL define Data Transfer Objects in the Application layer for request/response handling.

#### Scenario: DTOs exist in application layer
- **WHEN** inspecting `application/usecases/agent/dto.go`
- **THEN** file contains: `AgenticRAGRequest`, `AgenticRAGResponse`, `ChatChunkDTO`, `ToolCallRecordDTO`

### Requirement: Conversion functions exist between Domain and DTO
The system SHALL provide conversion functions between Domain entities and Application DTOs.

#### Scenario: ToDomainAgentRequest converts DTO to domain
- **WHEN** calling `ToDomainAgentRequest(&AgenticRAGRequest{...})`
- **THEN** returns `*agent.AgentRequest` with mapped fields

#### Scenario: FromDomainAgentResponse converts domain to DTO
- **WHEN** calling `FromDomainAgentResponse(&agent.AgentResponse{...})`
- **THEN** returns `*AgenticRAGResponse` with mapped fields

### Requirement: Infrastructure layer implements Domain interfaces
The system SHALL implement Domain-defined interfaces in the Infrastructure layer.

#### Scenario: BaseAgent implements domain interface
- **WHEN** inspecting `infrastructure/agent/base_agent.go`
- **THEN** `BaseAgent` struct implements Domain-defined Agent interface
- **AND** file imports only: Domain layer, Eino framework (external)

### Requirement: Infrastructure layer does not depend on Application layer
The system SHALL NOT allow Infrastructure layer to import Application layer packages.

#### Scenario: No infrastructure → application imports
- **WHEN** running `go mod graph | grep infrastructure`
- **THEN** output does NOT contain paths ending in `application/usecases`

### Requirement: Application layer uses Domain interfaces via dependency injection
The system SHALL inject Domain interface implementations into Application layer use cases.

#### Scenario: ExecuteUseCase accepts domain interface
- **WHEN** creating `NewExecuteUseCase(orchestrator AgentOrchestrator)`
- **THEN** use case stores the domain interface for later use

