# Chat Service Refactor

## ADDED Requirements

### Requirement: Clean Chat UseCase

The Chat UseCase SHALL NOT directly depend on infrastructure layer implementations.

#### Scenario: No direct infrastructure import
- **WHEN** ChatUseCase is implemented
- **THEN** it SHALL NOT import `infrastructure/llm/chat`
- **AND** SHALL NOT import `infrastructure/config`
- **AND** SHALL only depend on domain interfaces

### Requirement: Domain Interface Dependency

The Chat UseCase SHALL depend on domain layer interfaces for chat operations.

#### Scenario: Initialize with domain interface
- **WHEN** ChatUseCase is created
- **THEN** it accepts a `domain.llm.ChatService` interface
- **AND** stores the interface for method calls

#### Scenario: Chat execution through interface
- **WHEN** a chat request is made
- **THEN** the use case calls the domain interface method
- **AND** does not create infrastructure instances directly

### Requirement: Agent Integration

The use case SHALL support optional agent integration.

#### Scenario: Chat with agent enabled
- **WHEN** agent is available and tool calling is enabled
- **THEN** the use case delegates to agent orchestrator
- **AND** returns agent's response

#### Scenario: Chat without agent
- **WHEN** agent is not available or tool calling is disabled
- **THEN** the use case uses standard chat service
- **AND** returns chat response

### Requirement: Streaming Support

The use case SHALL support both sync and streaming chat modes.

#### Scenario: Sync chat
- **WHEN** a non-streaming chat request is made
- **THEN** the use case returns complete ChatResponse

#### Scenario: Streaming chat
- **WHEN** a streaming chat request is made
- **THEN** the use case returns a channel of StreamChatEvent
- **AND** events are streamed as they arrive

### Requirement: LLM Chat UseCase Dependency Inversion

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

### Requirement: LLM Model Configuration CRUD

The system SHALL provide full CRUD operations for LLM model configurations.

#### Scenario: Create model configuration
- **WHEN** a valid CreateModelRequestDTO is submitted
- **THEN** the use case validates the request
- **AND** creates the model configuration through repository
- **AND** returns the created ModelResponseDTO

#### Scenario: Update model configuration
- **WHEN** an UpdateModelRequestDTO is submitted
- **THEN** the use case retrieves existing configuration
- **AND** applies updates to fields that are set
- **AND** saves through repository
- **AND** returns updated ModelResponseDTO

#### Scenario: Delete model configuration
- **WHEN** a delete request is submitted with valid model ID
- **THEN** the use case deletes the configuration
- **AND** returns success

#### Scenario: Get model configuration
- **WHEN** a get request is submitted with valid model ID
- **THEN** the use case retrieves from repository
- **AND** returns ModelResponseDTO

#### Scenario: List model configurations
- **WHEN** a list request is submitted with tenant ID
- **THEN** the use case retrieves models with pagination
- **AND** applies filters (type, enabled status)
- **AND** returns ListModelsResponseDTO

### Requirement: Model Instance Creation

The system SHALL create model instances from configurations.

#### Scenario: Create chat model instance
- **WHEN** a chat model instance is requested
- **THEN** the use case validates tenant access
- **AND** validates model type is chat
- **AND** uses factory to create instance
- **AND** returns the chat repository

#### Scenario: Create embedding model instance
- **WHEN** an embedding model instance is requested
- **THEN** the use case validates tenant access
- **AND** validates model type is embedding
- **AND** uses factory to create instance
- **AND** returns the embedding repository

#### Scenario: Create rerank model instance
- **WHEN** a rerank model instance is requested
- **THEN** the use case validates tenant access
- **AND** validates model type is rerank
- **AND** uses factory to create instance
- **AND** returns the rerank repository

### Requirement: Default Model Selection

The system SHALL support default model per tenant and type.

#### Scenario: Get default model
- **WHEN** default model is requested for tenant and type
- **THEN** the use case queries repository for default
- **AND** returns ModelResponseDTO

### Requirement: Tenant Isolation

The use case SHALL enforce tenant isolation on all operations.

#### Scenario: Cross-tenant access denied
- **WHEN** a user attempts to access model from different tenant
- **THEN** the use case returns error
- **AND** error message indicates access denied

### Requirement: LLM Embedding UseCase Dependency Inversion

The Embedding UseCase SHALL depend on domain layer interfaces rather than infrastructure implementations.

#### Scenario: UseCase depends on domain interface
- **WHEN** EmbeddingUseCase is initialized
- **THEN** it accepts a `domain.llm.EmbeddingService` interface
- **AND** does not directly reference infrastructure implementations

### Requirement: Single Text Embedding

The system SHALL generate embeddings for single text input.

#### Scenario: Successful embedding generation
- **WHEN** an EmbeddingRequestDTO with single text is submitted
- **THEN** the use case delegates to domain service
- **AND** returns EmbeddingResponseDTO with vector data

### Requirement: Batch Text Embedding

The system SHALL generate embeddings for multiple texts efficiently.

#### Scenario: Successful batch embedding
- **WHEN** an EmbeddingRequestDTO with multiple texts is submitted
- **THEN** the use case calls batch embedding on domain service
- **AND** returns EmbeddingResponseDTO with all vectors

### Requirement: Dimension Query

The system SHALL provide embedding dimension information.

#### Scenario: Query embedding dimension
- **WHEN** dimension is requested
- **THEN** the use case queries the domain service
- **AND** returns the vector dimension

### Requirement: Model Validation

The use case SHALL validate that embedding model is initialized.

#### Scenario: Model not initialized
- **WHEN** embedding request is submitted but model is not initialized
- **THEN** the use case returns an error
- **AND** error message indicates model not configured

### Requirement: Conversation Domain Naming

The system SHALL use conversation terminology instead of chat.

#### Scenario: Domain package renamed
- **WHEN** accessing the former chat domain
- **THEN** domain/conversation package is used
- **AND** it contains Session and Message entities

#### Scenario: UseCase package renamed
- **WHEN** accessing conversation use cases
- **THEN** application/usecases/conversation package is used
- **AND** it contains ConversationUseCase, SessionUseCase, MessageUseCase

### Requirement: Conversation Context Management

The system SHALL manage conversation context through Session entity.

#### Scenario: Session tracks conversation state
- **WHEN** a conversation session is created
- **THEN** Session entity contains title, message count, status
- **AND** Session is linked to tenant and user

#### Scenario: Messages belong to session
- **WHEN** messages are created
- **THEN** each Message references its Session
- **AND** messages maintain order through timestamps

### Requirement: Knowledge-Enhanced Conversations

The system SHALL support RAG-enhanced conversations.

#### Scenario: Message with knowledge references
- **WHEN** a message is generated with RAG
- **THEN** Message.KnowledgeReferences contains source information
- **AND** retrieved chunks are referenced

#### Scenario: Message with agent steps
- **WHEN** a message is generated by agent
- **THEN** Message.AgentSteps contains tool call history
- **AND** execution flow is traceable

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
The SessionService interface SHALL be moved from domain/types/interfaces/session.go to application/usecases/conversation/interfaces.go.

#### Scenario: SessionService in application layer
- **GIVEN** the SessionService interface
- **WHEN** locating its definition
- **THEN** it resides in application/usecases/conversation/interfaces.go
- **AND** it defines use case methods like CreateSession, GetSession, ListSessions
- **AND** it is NOT in domain/types/interfaces/session.go

### Requirement: Session Management

The system SHALL manage conversation sessions with improved naming.

#### Scenario: Create conversation session
- **WHEN** a new conversation starts
- **THEN** a Session entity is created
- **AND** it contains initial title and status

#### Scenario: List conversation sessions
- **WHEN** user requests conversation history
- **THEN** sessions are filtered by user and tenant
- **AND** pagination is supported

### Requirement: Message Management

The system SHALL manage messages within conversation context.

#### Scenario: Add message to conversation
- **WHEN** a user or assistant sends a message
- **THEN** Message is created with Session reference
- **AND** message order is preserved

#### Scenario: Retrieve conversation history
- **WHEN** conversation context is needed
- **THEN** messages are retrieved in order
- **AND** full context is available

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

## REMOVED Requirements

### Requirement: Direct Chat Instance Creation
**Reason**: Violates dependency inversion, infrastructure detail in application layer
**Migration**: Inject domain interface through constructor, remove createChatInstance() method

### Requirement: Direct Config Import
**Reason**: Configuration is infrastructure concern
**Migration**: Use interfaces that abstract configuration details

### Requirement: Direct Infrastructure Creation
**Reason**: Violates dependency inversion principle
**Migration**: Use domain interface injected through constructor

### Requirement: Chat Domain Terminology
**Reason**: Chat describes technical action, Conversation describes business concept
**Migration**: Rename all chat references to conversation, update API paths
