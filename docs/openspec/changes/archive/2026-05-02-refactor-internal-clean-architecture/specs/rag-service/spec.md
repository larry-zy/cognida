# RAG Service Refactor

## MODIFIED Requirements

### Requirement: RAG domain layer cleanup
The RAG domain layer SHALL contain only business entities, value objects, and repository interfaces. Request/response types SHALL be removed.

#### Scenario: RAG entities contain business attributes only
- **GIVEN** RAG domain entities in domain/rag/
- **WHEN** examining their structure
- **THEN** they contain only business attributes (Document, GraphNode, GraphRelation, etc.)
- **AND** they do NOT contain application request/response DTOs

### Requirement: RAG repository interfaces remain in domain
Repository interfaces for RAG (Retriever, Reranker, Pipeline, GraphRepository, etc.) SHALL remain in domain/rag/repository.go.

#### Scenario: RAG repositories in domain
- **GIVEN** RAG repository interfaces
- **WHEN** locating their definitions
- **THEN** they reside in domain/rag/repository.go
- **AND** they define data access contracts using domain entities

### Requirement: RAG use case organization
RAG application logic SHALL be organized into use cases under application/usecases/rag/.

#### Scenario: RAG use case structure
- **GIVEN** the RAG bounded context
- **WHEN** examining application/usecases/rag/
- **THEN** it contains use cases like chat.go, retrieve.go, graph.go, query.go
- **AND** DTOs reside in dto.go
- **AND** interfaces are defined in interfaces.go

### Requirement: RAG DTOs in application layer
RAG-related DTOs (ChatRequest, ChatResponse, RetrieveRequest, etc.) SHALL reside in application/usecases/rag/dto.go.

#### Scenario: RAG DTOs location
- **GIVEN** RAG request/response types
- **WHEN** locating their definitions
- **THEN** they reside in application/usecases/rag/dto.go
- **AND** they are NOT in the domain layer

### Requirement: RAG service reorganization
The current RAGService in application/rag/service.go SHALL be split into focused use case orchestrators.

#### Scenario: RAG use case separation
- **GIVEN** the RAGService with multiple responsibilities
- **WHEN** refactoring into use cases
- **THEN** Chat logic goes to application/usecases/rag/chat.go
- **AND** Retrieve logic goes to application/usecases/rag/retrieve.go
- **AND** Graph operations go to application/usecases/rag/graph.go
- **AND** Query enhancement goes to application/usecases/rag/query.go

### Requirement: LLM chat interface in domain
The LLMChat interface SHALL remain in domain/rag/repository.go as it represents an external service contract.

#### Scenario: LLMChat interface location
- **GIVEN** the LLMChat interface
- **WHEN** locating its definition
- **THEN** it resides in domain/rag/repository.go
- **AND** it defines methods like Chat, ChatStream
- **AND** infrastructure/llm/chat/ implements this interface
