# agent-hooks Specification

## Purpose
TBD - created by archiving change agent-layer-cleanup. Update Purpose after archive.
## Requirements
### Requirement: Domain layer defines Hook service interface
The system SHALL define the Hook service interface in the Domain layer.

#### Scenario: HookService interface exists in domain
- **WHEN** inspecting `domain/agent/service.go`
- **THEN** `HookService` interface defines methods:
  - `Before(ctx context.Context, message string) (context.Context, string, error)`
  - `After(ctx context.Context, resp interface{}) error`

### Requirement: Infrastructure layer implements concrete Hooks
The system SHALL implement specific hooks in the Infrastructure layer using the Domain interface.

#### Scenario: ConclusionGenerator hook exists
- **WHEN** inspecting `infrastructure/agent/hooks/conclusion.go`
- **THEN** file implements `HookService` interface
- **AND** file provides domain-specific conclusion generation logic

#### Scenario: IntentClarifier hook exists
- **WHEN** inspecting `infrastructure/agent/hooks/clarification.go`
- **THEN** file implements `HookService` interface
- **AND** file provides intent clarification logic

### Requirement: Hooks are configurable via Domain config
The system SHALL allow hook configuration through Domain layer config structures.

#### Scenario: HookConfig enables/disables hooks
- **WHEN** `AgentConfig.HookConfig.EnableConclusion` is true
- **THEN** ConclusionGenerator hook is active during agent execution

#### Scenario: HookConfig contains hook parameters
- **WHEN** `AgentConfig.HookConfig.DataTools` is set
- **THEN** ConclusionGenerator uses specified tools for data analysis

### Requirement: Hooks follow error handling contract
The system SHALL ensure hooks handle errors according to Domain contract.

#### Scenario: Before hook can return error
- **WHEN** a Before hook returns an error
- **THEN** agent execution stops and error is propagated

#### Scenario: Before hook can modify message
- **WHEN** a Before hook returns modified message
- **THEN** subsequent processing uses the modified message

#### Scenario: After hook error is recoverable
- **WHEN** an After hook returns an error
- **THEN** error is logged but response is still returned (non-blocking)

### Requirement: Hooks integrate with Agent builder
The system SHALL provide a fluent API for configuring hooks in the Agent builder.

#### Scenario: Builder accepts conclusion hook
- **WHEN** calling `builder.WithConclusion(generator)`
- **THEN** generator is added to after-hooks list

#### Scenario: Builder accepts clarification hook
- **WHEN** calling `builder.WithClarification(clarifier)`
- **THEN** clarifier is added to before-hooks list

