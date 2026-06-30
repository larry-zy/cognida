## ADDED Requirements

### Requirement: Orchestration patterns defined in Application layer
The system SHALL define Agent orchestration patterns in the Application layer as use cases, not in Domain layer.

#### Scenario: Sequential orchestration exists
- **WHEN** inspecting `application/usecases/agent/orchestration/sequential.go`
- **THEN** file defines `Sequential` function that executes agents in order
- **AND** file depends only on Domain layer interfaces

#### Scenario: Parallel orchestration exists
- **WHEN** inspecting `application/usecases/agent/orchestration/parallel.go`
- **THEN** file defines `Parallel` function that executes agents concurrently
- **AND** file depends only on Domain layer interfaces

#### Scenario: Supervisor orchestration exists
- **WHEN** inspecting `application/usecases/agent/orchestration/supervisor.go`
- **THEN** file defines `Supervisor` pattern for routing requests
- **AND** file depends only on Domain layer interfaces

### Requirement: Orchestration patterns implement common interface
The system SHALL provide a common interface that all orchestration patterns implement.

#### Scenario: Orchestrator interface exists
- **WHEN** inspecting `application/usecases/agent/orchestration/`
- **THEN** all patterns implement a common `AgentExecutor` interface
- **AND** interface has methods: `Execute(ctx, req) (resp, error)`, `ExecuteStream(ctx, req) (<-chan, error)`

### Requirement: Orchestration patterns are composable
The system SHALL allow nesting orchestration patterns.

#### Scenario: Nested orchestration works
- **WHEN** calling `Sequential(Parallel(agent1, agent2), agent3)`
- **THEN** agents 1 and 2 execute in parallel, then agent3 executes after both complete

### Requirement: Orchestration layer uses Domain interfaces
The system SHALL ensure orchestration patterns depend only on Domain layer interfaces.

#### Scenario: Orchestration has correct dependencies
- **WHEN** inspecting imports in `application/usecases/agent/orchestration/*.go`
- **THEN** files import only: `link/internal/domain/agent`, standard library, Eino framework

## REMOVED Requirements

### Requirement: AgentOrchestrator interface in domain layer
**Reason**: Orchestration is an application-level concern (coordination of multiple agents), not a core domain concept
**Migration**: Use orchestration patterns in `application/usecases/agent/orchestration/`
