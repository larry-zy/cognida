# Domain Services

## ADDED Requirements

### Requirement: Domain services for cross-entity logic
Business logic that involves multiple entities or external services SHALL be implemented as domain services in domain/services/.

#### Scenario: Domain service for similarity calculation
- **GIVEN** a need to calculate text similarity
- **WHEN** implementing similarity calculation
- **THEN** it resides in domain/services/similarity.go
- **AND** it provides methods like CalculateSimilarity(text1, text2, method)
- **AND** it is stateless and pure business logic

### Requirement: Domain services are stateless
Domain services SHALL be stateless and contain only business logic. They SHALL NOT hold application state or coordinate workflows.

#### Scenario: Stateless domain service
- **GIVEN** a domain service
- **WHEN** examining its structure
- **THEN** it has no stored state (no fields for repositories, configs, etc.)
- **AND** all methods are pure functions or operate only on their inputs

### Requirement: Domain services interface in domain layer
Domain services SHALL expose their behavior through interfaces defined in the domain layer.

#### Scenario: Domain service interface
- **GIVEN** a domain service for query analysis
- **WHEN** defining its contract
- **THEN** the interface resides in domain/ or domain/services/
- **AND** infrastructure implements this interface

### Requirement: Extract similarity logic from application
Similarity calculation logic currently in application/agent/service.go SHALL be extracted to domain/services/similarity.go.

#### Scenario: Similarity calculation location
- **GIVEN** the need to calculate similarity between two texts
- **WHEN** calling similarity calculation
- **THEN** it is invoked via domain/services/similarity package
- **AND** it is NOT in application/agent/service.go

### Requirement: Domain service for query analysis
Query analysis, decomposition, and enhancement logic SHALL be implemented as a domain service.

#### Scenario: Query analysis domain service
- **GIVEN** a user query
- **WHEN** analyzing the query
- **THEN** the logic resides in domain/services/analysis.go
- **AND** it determines query type, complexity, and suggested tools

## REMOVED Requirements

### Requirement: Business logic in application layer
**Reason**: Business logic belongs in the domain layer, not application use case orchestrators.

**Migration**:
1. Identify business logic in application services (similarity, validation, analysis)
2. Extract to appropriate domain service in domain/services/
3. Update application services to use domain services
4. Remove business logic from application layer
