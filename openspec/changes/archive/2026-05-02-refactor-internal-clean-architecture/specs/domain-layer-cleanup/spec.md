# Domain Layer Cleanup

## ADDED Requirements

### Requirement: Domain entities contain only business state
The domain layer SHALL contain only business entities with their state and behavior. Entities SHALL NOT contain request/response DTOs, application-specific types, or infrastructure concerns.

#### Scenario: Entity contains only business attributes
- **GIVEN** a domain entity such as Agent
- **WHEN** examining its structure
- **THEN** it contains only business attributes (ID, name, type, status, config)
- **AND** it does NOT contain request/response types like AgentRequest or AgentResponse

### Requirement: Domain repository interfaces are pure
The domain layer SHALL define repository interfaces that represent data access contracts without coupling to implementation details.

#### Scenario: Repository interface uses domain entities
- **GIVEN** a repository interface in domain layer
- **WHEN** defining its methods
- **THEN** all parameters and return values are domain entities or primitive types
- **AND** no DTOs from application layer are referenced

### Requirement: Service interfaces removed from domain
Service interfaces (UserService, SessionService, MessageService) SHALL be removed from domain/types/interfaces/ and moved to application use case layer.

#### Scenario: Service interface in application layer
- **GIVEN** a service interface like UserService
- **WHEN** locating its definition
- **THEN** it resides in application/usecases/user/interfaces.go
- **AND** it is NOT in domain/types/interfaces/

### Requirement: Domain services for cross-entity business logic
Business logic that spans multiple entities SHALL be extracted into domain services in the domain/services/ package.

#### Scenario: Similarity calculation in domain service
- **GIVEN** a need to calculate text similarity
- **WHEN** implementing this logic
- **THEN** it resides in domain/services/similarity.go
- **AND** it is NOT in the application layer

### Requirement: Domain errors package
Each bounded context SHALL have its own errors package defining domain-specific error types.

#### Scenario: Context-specific error types
- **GIVEN** the agent bounded context
- **WHEN** defining error types
- **THEN** they reside in domain/agent/errors.go
- **AND** they use domain-specific error codes

## REMOVED Requirements

### Requirement: DTOs in domain entities
**Reason**: DTOs are application-layer concerns for data transfer, not domain business concepts.

**Migration**:
1. Identify all DTOs in domain entities (AgentRequest, AgentResponse, ChatChunk, etc.)
2. Move them to application/usecases/<context>/dto.go
3. Update all imports referencing these types
4. Remove from domain entities
