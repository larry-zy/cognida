# LLM Chat UseCase Specification

## ADDED Requirements

### Requirement: Dependency Inversion

The Chat UseCase SHALL depend on domain layer interfaces rather than infrastructure implementations.

#### Scenario: UseCase depends on domain interface
- **WHEN** ChatUseCase is initialized
- **THEN** it accepts a `domain.llm.ChatService` interface
- **AND** does not directly reference `infrastructure/llm/chat`

### Requirement: Chat Execution

The system SHALL execute chat requests through the use case layer.

#### Scenario: Successful chat execution
- **WHEN** a chat request is submitted
- **THEN** the use case delegates to the domain service
- **AND** returns a ChatResponse with content and metadata

#### Scenario: Chat with streaming enabled
- **WHEN** a streaming chat request is submitted
- **THEN** the use case returns a channel of chat chunks
- **AND** each chunk contains partial content

### Requirement: DTO Conversion

The use case SHALL handle conversion between DTOs and domain models.

#### Scenario: Request DTO to domain model
- **WHEN** a ChatRequestDTO is received
- **THEN** the use case converts it to domain ChatRequest
- **AND** passes to domain service

#### Scenario: Domain response to DTO
- **WHEN** a domain ChatResponse is received
- **THEN** the use case converts it to ChatResponseDTO
- **AND** returns to caller

### Requirement: Error Handling

The use case SHALL handle and translate domain errors appropriately.

#### Scenario: Domain service error
- **WHEN** the domain service returns an error
- **THEN** the use case wraps the error with context
- **AND** returns to caller

## REMOVED Requirements

### Requirement: Direct Infrastructure Creation
**Reason**: Violates dependency inversion principle
**Migration**: Use domain interface injected through constructor
