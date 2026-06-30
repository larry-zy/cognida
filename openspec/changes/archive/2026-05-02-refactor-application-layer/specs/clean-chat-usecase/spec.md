# Clean Chat UseCase Specification

## ADDED Requirements

### Requirement: Remove Infrastructure Dependency

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

## REMOVED Requirements

### Requirement: Direct Chat Instance Creation
**Reason**: Violates dependency inversion, infrastructure detail in application layer
**Migration**: Inject domain interface through constructor, remove createChatInstance() method

### Requirement: Direct Config Import
**Reason**: Configuration is infrastructure concern
**Migration**: Use interfaces that abstract configuration details
