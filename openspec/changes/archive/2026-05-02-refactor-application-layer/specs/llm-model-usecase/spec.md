# LLM Model UseCase Specification

## ADDED Requirements

### Requirement: Model Configuration CRUD

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
