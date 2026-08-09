# Chat Service Refactor

## MODIFIED Requirements

### Requirement: Chat entity structure
The Chat and Session entities SHALL contain only business state. Message-related DTOs SHALL be removed from domain.

#### Scenario: Chat entities contain business attributes
- **GIVEN** the Chat and Session entities in domain/chat/entity.go
- **WHEN** examining their structure
- **THEN** they contain only business attributes (ID, UserID, Title, Status, etc.)
- **AND** they do NOT contain request/response DTOs

### Requirement: Message service interface moved
The MessageService interface SHALL be moved from domain/types/interfaces/message.go to application/usecases/chat/interfaces.go.

#### Scenario: MessageService in application layer
- **GIVEN** the MessageService interface
- **WHEN** locating its definition
- **THEN** it resides in application/usecases/chat/interfaces.go
- **AND** it defines use case methods like CreateMessage, ListMessages, DeleteMessage
- **AND** it is NOT in domain/types/interfaces/message.go

### Requirement: Session service interface moved
The SessionService interface SHALL be moved from domain/types/interfaces/session.go to application/usecases/chat/interfaces.go.

#### Scenario: SessionService in application layer
- **GIVEN** the SessionService interface
- **WHEN** locating its definition
- **THEN** it resides in application/usecases/chat/interfaces.go
- **AND** it defines use case methods like CreateSession, GetSession, ListSessions
- **AND** it is NOT in domain/types/interfaces/session.go

### Requirement: Chat use case organization
Chat application logic SHALL be organized into use cases under application/usecases/chat/.

#### Scenario: Chat use case structure
- **GIVEN** the chat bounded context
- **WHEN** examining application/usecases/chat/
- **THEN** it contains use cases like session.go, message.go, chat.go
- **AND** DTOs reside in dto.go
- **AND** interfaces are defined in interfaces.go

### Requirement: Chat repository interfaces remain in domain
Repository interfaces for chat SHALL remain in domain/chat/repository.go.

#### Scenario: Chat repositories in domain
- **GIVEN** repository interfaces for chat
- **WHEN** locating their definitions
- **THEN** they reside in domain/chat/repository.go
- **AND** they define data access contracts using domain entities
