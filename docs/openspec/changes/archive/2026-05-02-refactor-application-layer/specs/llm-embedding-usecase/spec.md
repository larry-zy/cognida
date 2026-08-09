# LLM Embedding UseCase Specification

## ADDED Requirements

### Requirement: Dependency Inversion

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
