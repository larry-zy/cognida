# Domain Restructuring Specification

## ADDED Requirements

### Requirement: Domain Layer Contains Business Domains Only

The domain layer SHALL only contain business domains, using business language in naming.

#### Scenario: Technical components moved to infrastructure
- **WHEN** reviewing domain layer packages
- **THEN** llm, rag, agent are not present as domain packages
- **AND** they are located in infrastructure or application layer

#### Scenario: Business language used in naming
- **WHEN** domain packages are named
- **THEN** conversation is used instead of chat
- **AND** knowledge is used instead of kb
- **AND** assistant is used for agent capabilities

### Requirement: Services Directory Properly Organized

The domain/services directory SHALL be eliminated or properly organized.

#### Scenario: Services removed or distributed
- **WHEN** reviewing domain layer structure
- **THEN** domain/services directory does not exist
- **OR** it only contains domain service interfaces organized by business domain

### Requirement: Types/Interfaces Separated

Repository and Service interfaces SHALL be properly separated by layer.

#### Scenario: Repository interfaces in domain
- **WHEN** repository interfaces are defined
- **THEN** they are in domain/<context>/repository.go
- **AND** they define data access contracts

#### Scenario: Service interfaces in application
- **WHEN** service interfaces are defined
- **THEN** they are in application/usecases/<context>/interfaces.go
- **AND** they define use case contracts

## REMOVED Requirements

### Requirement: LLM as Domain Package
**Reason**: LLM is technical infrastructure, not a business domain
**Migration**: Move to infrastructure/llm/, define interfaces in consuming domains

### Requirement: RAG as Domain Package
**Reason**: RAG is an application capability, not a business domain
**Migration**: Move to infrastructure/rag/, use through application layer

### Requirement: Agent as Domain Package
**Reason**: Agent is an orchestration pattern, not a business domain
**Migration**: Move to application/usecases/assistant/

### Requirement: Evaluation as Domain Package
**Reason**: Evaluation is a use case for system validation, not core business
**Migration**: Move to application/usecases/evaluation/

### Requirement: Graph Separate from KB
**Reason**: Knowledge graph is an enhancement of knowledge base, not separate domain
**Migration**: Merge into domain/knowledge/ as graph.go
