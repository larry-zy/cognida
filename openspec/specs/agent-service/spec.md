# Agent Service Refactor

## ADDED Requirements

### Requirement: Domain Restructuring - Technical Components Moved

The domain layer SHALL only contain business domains, using business language in naming.

#### Scenario: Technical components moved to infrastructure
- **WHEN** reviewing domain layer packages
- **THEN** llm, rag, agent are not present as domain packages
- **AND** they are located in infrastructure or application layer

#### Scenario: Business language used in naming
- **WHEN** domain packages are named
- **THEN** assistant is used for agent capabilities
- **AND** conversation is used instead of chat
- **AND** knowledge is used instead of kb

### Requirement: Domain Layer Package Reorganization

The system shall organize domain packages by business domain, not technical components.

#### Scenario: Agent moved to application layer
- **WHEN** reviewing application layer
- **THEN** application/usecases/assistant contains agent orchestration logic
- **AND** domain layer does not contain agent package

## MODIFIED Requirements

### Requirement: Agent entity structure
The Agent entity SHALL contain only business state and behavior. Request/Response DTOs SHALL be removed.

#### Scenario: Agent entity contains business attributes only
- **GIVEN** the Agent entity in domain/agent/entity.go
- **WHEN** examining its structure
- **THEN** it contains ID, Name, Description, Type, Status, Config
- **AND** it does NOT contain AgentRequest, AgentResponse, or ChatChunk

### Requirement: Agent use case organization
Agent application logic SHALL be organized into distinct use cases under application/usecases/assistant/.

#### Scenario: Agent use case structure
- **GIVEN** the assistant bounded context
- **WHEN** examining application/usecases/assistant/
- **THEN** it contains use cases like execute.go, research.go, config.go
- **AND** each use case has its own DTOs in dto.go
- **AND** interfaces are defined in interfaces.go

### Requirement: Similarity calculation in domain service
Similarity calculation logic SHALL be moved from application/agent/service.go to domain/services/similarity.go.

#### Scenario: Similarity calculation via domain service
- **GIVEN** a need to calculate similarity
- **WHEN** invoking similarity calculation
- **THEN** it uses domain/services/similarity package
- **AND** CalculateSimilarity method is NOT in application/agent/service.go

### Requirement: Agent orchestrator interface
The AgentOrchestrator interface SHALL remain in domain/agent/repository.go as it represents a domain capability.

#### Scenario: AgentOrchestrator in domain
- **GIVEN** the AgentOrchestrator interface
- **WHEN** locating its definition
- **THEN** it resides in domain/agent/repository.go
- **AND** it defines domain methods like Execute, ExecuteStream, GetTools

### Requirement: Agent DTOs in application layer
Agent-related DTOs (AgenticRAGRequest, AgenticRAGResponse, DeepResearchRequest, etc.) SHALL reside in application/usecases/assistant/dto.go.

#### Scenario: Agent DTOs location
- **GIVEN** agent-related request/response types
- **WHEN** locating their definitions
- **THEN** they reside in application/usecases/assistant/dto.go
- **AND** they are NOT in domain/assistant/entity.go

## REMOVED Requirements

### Requirement: Agent as Domain Package
**Reason**: Agent is an orchestration pattern, not a business domain
**Migration**: Move to application/usecases/assistant/
