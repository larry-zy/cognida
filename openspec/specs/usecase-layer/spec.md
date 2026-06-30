# Use Case Layer

## ADDED Requirements

### Requirement: Evaluation UseCase Orchestration

The Evaluation UseCase SHALL orchestrate the evaluation workflow without implementing business logic.

#### Scenario: Successful evaluation workflow
- **WHEN** an evaluation request is submitted
- **THEN** the use case creates an evaluation task
- **AND** retrieves the dataset
- **AND** executes retrieval for each QA pair
- **AND** executes LLM generation for each QA pair
- **AND** delegates metric calculation to domain service
- **AND** saves results through repository
- **AND** updates task status to success

### Requirement: Evaluation Dependency on Domain Services

The use case SHALL depend on domain layer interfaces for business operations.

#### Scenario: UseCase uses domain evaluation service
- **WHEN** metric calculation is needed
- **THEN** the use case calls `domain.evaluation.CalculateMetrics()`
- **AND** does not implement PMI, BLEU, ROUGE calculations itself

#### Scenario: UseCase uses domain RAG services
- **WHEN** retrieval is needed
- **THEN** the use case calls `domain.rag.Retriever.Retrieve()`
- **AND** does not directly implement retrieval logic

### Requirement: Evaluation Progress Tracking

The use case SHALL track and report evaluation progress.

#### Scenario: Update progress after each QA
- **WHEN** a QA pair evaluation completes
- **THEN** the use case increments completed count
- **AND** updates progress through repository

### Requirement: Evaluation Error Handling

The use case SHALL handle errors gracefully during evaluation.

#### Scenario: Retrieval fails for one QA
- **WHEN** retrieval fails for a specific QA pair
- **THEN** the use case logs the error
- **AND** continues with remaining QA pairs
- **AND** marks the specific result as failed

#### Scenario: Dataset not found
- **WHEN** requested dataset does not exist
- **THEN** the use case updates task status to failed
- **AND** includes error message
- **AND** returns early

### Requirement: Evaluation Result Aggregation

The use case SHALL aggregate results from domain metric calculation.

#### Scenario: Metrics calculated successfully
- **WHEN** domain service returns metric results
- **THEN** the use case saves results through metrics repository
- **AND** includes metrics in evaluation detail response

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

## REMOVED Requirements

### Requirement: PMI and Weight Calculation in Application Layer
**Reason**: Business logic belongs in domain layer
**Migration**: Move to `domain.evaluation.CalculateMetrics()`

### Requirement: Direct Infrastructure Dependency
**Reason**: Violates dependency inversion principle
**Migration**: Use domain interfaces for Retriever, LLMChat, Reranker

### Requirement: Service Adapter Layer
**Reason**: Adapter layer adds unnecessary indirection without value
**Migration**: Direct use of use cases by consumers, remove service.go adapter

### Requirement: GORM DB Exposure
**Reason**: Exposes infrastructure detail through use case
**Migration**: Use repository interfaces for transaction handling
