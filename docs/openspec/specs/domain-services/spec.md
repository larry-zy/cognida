# Domain Services

## ADDED Requirements

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

### Requirement: Domain services for cross-entity logic

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

### Requirement: Separate KB and Graph Domains
**Reason**: Graph is an enhancement of knowledge base, not separate concern
**Migration**: Merge into domain/knowledge/, update all imports
