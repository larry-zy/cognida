# Knowledge Domain Specification

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

## MODIFIED Requirements

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

### Requirement: Separate KB and Graph Domains
**Reason**: Graph is an enhancement of knowledge base, not separate concern
**Migration**: Merge into domain/knowledge/, update all imports
