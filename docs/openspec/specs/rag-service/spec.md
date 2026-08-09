# RAG Service Refactor

## ADDED Requirements

### Requirement: RAG Chat

The system SHALL provide RAG-enhanced chat capabilities.

#### Scenario: RAG chat with retrieval
- **WHEN** a RAG chat request is submitted
- **THEN** the use case retrieves relevant documents
- **AND** optionally reranks documents
- **AND** generates response using retrieved context
- **AND** returns ChatResponse with sources

#### Scenario: RAG chat streaming
- **WHEN** a streaming RAG chat request is submitted
- **THEN** the use case returns a channel of events
- **AND** retrieval happens before streaming starts
- **AND** response is streamed as generated

### Requirement: Document Retrieval

The system SHALL retrieve documents based on query.

#### Scenario: Vector retrieval
- **WHEN** retrieval is requested with vector mode
- **THEN** the use case queries vector index
- **AND** returns top-k similar documents

#### Scenario: Hybrid retrieval
- **WHEN** retrieval is requested with hybrid mode
- **THEN** the use case combines vector and keyword results
- **AND** returns merged and ranked documents

### Requirement: Query Strengthening

The system SHALL support query enhancement for better retrieval.

#### Scenario: Query strengthening
- **WHEN** query strengthening is enabled
- **THEN** the use case enhances original query
- **AND** uses strengthened query for retrieval
- **AND** returns both original and strengthened queries

### Requirement: Graph Integration

The system SHALL integrate knowledge graph with RAG.

#### Scenario: Graph-enhanced retrieval
- **WHEN** graph retrieval is enabled
- **THEN** the use case extracts entities from query
- **AND** retrieves related graph nodes
- **AND** includes graph context in retrieval results

### Requirement: Knowledge Base Management

The system SHALL provide knowledge base management operations through use cases.

#### Scenario: Create knowledge base with settings
- **WHEN** a knowledge base creation request is submitted
- **THEN** the use case validates the request
- **AND** creates both knowledge base and settings
- **AND** returns the created knowledge base

#### Scenario: Update knowledge base with settings
- **WHEN** an update request is submitted
- **THEN** the use case updates knowledge base
- **AND** updates associated settings
- **AND** returns updated knowledge base

### Requirement: Chunk Management

The system SHALL manage knowledge chunks through use cases.

#### Scenario: Create chunk
- **WHEN** a chunk creation request is submitted
- **THEN** the use case validates chunk data
- **AND** creates chunk through repository
- **AND** returns success

#### Scenario: Query chunks by knowledge base
- **WHEN** chunks are requested for a knowledge base
- **THEN** the use case retrieves chunks with pagination
- **AND** returns chunk list and total count

### Requirement: Unified Knowledge Domain

The system SHALL provide a unified knowledge domain combining KB and Graph.

#### Scenario: Knowledge domain contains all knowledge entities
- **WHEN** accessing domain/knowledge package
- **THEN** KnowledgeBase, Knowledge, Chunk entities are available
- **AND** GraphNode, GraphRelation entities are available
- **AND** VectorDocument entities are available

#### Scenario: Knowledge repositories unified
- **WHEN** accessing knowledge repositories
- **THEN** KBRepository, GraphRepository, VectorRepository are in same package
- **AND** they share common interfaces and patterns

### Requirement: Knowledge Retrieval Interface

The system SHALL provide a unified retrieval interface for knowledge.

#### Scenario: Unified retriever interface
- **WHEN** knowledge retrieval is needed
- **THEN** Retriever interface is available in domain/knowledge
- **AND** it supports vector, keyword, and graph retrieval modes
- **AND** it supports hybrid retrieval

### Requirement: Knowledge Domain Organization

The knowledge domain SHALL be organized by functional concern.

#### Scenario: File organization by concern
- **WHEN** examining domain/knowledge directory
- **THEN** entity.go contains core entities (KB, Knowledge, Chunk)
- **AND** graph.go contains graph entities (Node, Relation, GraphData)
- **AND** vector.go contains vector entities (VectorDocument, options)
- **AND** repository.go contains all repository interfaces
- **AND** retriever.go contains retrieval interfaces
- **AND** errors.go contains all error types

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

### Requirement: Knowledge Base Management

The system SHALL manage knowledge bases with integrated graph support.

#### Scenario: Create knowledge base with graph enabled
- **WHEN** a knowledge base is created with graph enabled
- **THEN** both vector and graph storage are initialized
- **AND** graph extraction is configured for the KB

#### Scenario: Delete knowledge base cascades to graph
- **WHEN** a knowledge base is deleted
- **THEN** associated graph data is also deleted
- **AND** vector data is also deleted

### Requirement: Chunk Management with Graph Relations

The system SHALL manage chunks with graph entity associations.

#### Scenario: Chunk associated with graph nodes
- **WHEN** a chunk is processed for graph extraction
- **THEN** extracted entities are linked to the chunk
- **AND** GraphNode.Chunks contains the chunk ID

#### Scenario: Chunk deletion updates graph
- **WHEN** a chunk is deleted
- **THEN** associated graph relations are updated or removed
- **AND** orphaned nodes are cleaned up

## REMOVED Requirements

### Requirement: Service Adapter Layer
**Reason**: Adapter layer adds unnecessary indirection without value
**Migration**: Direct use of individual use cases (ChatUseCase, RetrieveUseCase, etc.)

### Requirement: Unified Service Interface
**Reason**: Forces all RAG operations through single entry point
**Migration**: Use specific use cases for each RAG operation

### Requirement: Separate KB and Graph Domains
**Reason**: Graph is an enhancement of knowledge base, not separate concern
**Migration**: Merge into domain/knowledge/, update all imports
