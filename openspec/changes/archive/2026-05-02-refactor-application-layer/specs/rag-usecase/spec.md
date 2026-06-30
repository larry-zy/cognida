# RAG UseCase Specification

## MODIFIED Requirements

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

## REMOVED Requirements

### Requirement: Service Adapter Layer
**Reason**: Adapter layer adds unnecessary indirection without value
**Migration**: Direct use of individual use cases (ChatUseCase, RetrieveUseCase, etc.)

### Requirement: Unified Service Interface
**Reason**: Forces all RAG operations through single entry point
**Migration**: Use specific use cases for each RAG operation
