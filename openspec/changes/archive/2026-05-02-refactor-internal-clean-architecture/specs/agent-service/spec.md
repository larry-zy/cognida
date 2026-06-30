# Agent Service Refactor

## MODIFIED Requirements

### Requirement: Agent entity structure
The Agent entity SHALL contain only business state and behavior. Request/Response DTOs SHALL be removed.

#### Scenario: Agent entity contains business attributes only
- **GIVEN** the Agent entity in domain/agent/entity.go
- **WHEN** examining its structure
- **THEN** it contains ID, Name, Description, Type, Status, Config
- **AND** it does NOT contain AgentRequest, AgentResponse, or ChatChunk

### Requirement: Agent use case organization
Agent application logic SHALL be organized into distinct use cases under application/usecases/agent/.

#### Scenario: Agent use case structure
- **GIVEN** the agent bounded context
- **WHEN** examining application/usecases/agent/
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
Agent-related DTOs (AgenticRAGRequest, AgenticRAGResponse, DeepResearchRequest, etc.) SHALL reside in application/usecases/agent/dto.go.

#### Scenario: Agent DTOs location
- **GIVEN** agent-related request/response types
- **WHEN** locating their definitions
- **THEN** they reside in application/usecases/agent/dto.go
- **AND** they are NOT in domain/agent/entity.go
