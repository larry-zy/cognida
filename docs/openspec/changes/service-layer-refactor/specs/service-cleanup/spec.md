## ADDED Requirements

### Requirement: Service layer follows 3-Layer architecture
The system SHALL ensure all services in `internal/service` follow the 3-Layer architecture: handler → service → model ← repository.

#### Scenario: Service only depends on model layer
- **WHEN** reviewing a service package
- **THEN** the service MUST only import from `internal/model` and external packages
- **AND** the service MUST NOT directly import from `internal/repository`

#### Scenario: Service contains business logic
- **WHEN** reviewing a service implementation
- **THEN** the service MUST contain business logic, not just repository pass-through
- **AND** if a service only passes through to repository, it SHOULD be removed

### Requirement: Service naming consistency
The system SHALL use consistent naming across the service layer.

#### Scenario: Use Service suffix
- **WHEN** creating a new service type
- **THEN** the type name MUST end with "Service" (e.g., `ChatService`, `KnowledgeService`)
- **AND** MUST NOT use "UseCase" suffix

#### Scenario: Package names are singular
- **WHEN** creating a service package
- **THEN** the package name MUST be singular (e.g., `package account`, not `package accounts`)

### Requirement: Interfaces defined in model layer
The system SHALL define all service interfaces in `internal/model`, not in `internal/service`.

#### Scenario: Interface location
- **WHEN** defining a service interface
- **THEN** the interface MUST be placed in `internal/model/<domain>/repository.go` or a dedicated interface file
- **AND** MUST NOT be placed in `internal/service`

#### Scenario: Implementation in service layer
- **WHEN** implementing a service interface
- **THEN** the implementation MUST be in `internal/service/<domain>`
- **AND** MUST implement the interface defined in `internal/model`

### Requirement: Agent service package consolidation
The system SHALL consolidate overly fragmented agent service packages.

#### Scenario: Merge core and framework packages
- **WHEN** reviewing agent service structure
- **THEN** `agent/core` and `agent/framework` SHOULD be merged into a single `agent/agent.go` or `agent/core.go`
- **AND** duplicate abstractions MUST be eliminated

#### Scenario: Remove test subpackage
- **WHEN** reviewing agent service
- **THEN** integration tests MUST be named `*_integration_test.go` within their respective packages
- **AND** the `agent/test` subpackage MUST be removed

## REMOVED Requirements

### Requirement: UseCase naming pattern
**Reason**: Inconsistent with 3-Layer architecture which uses "Service" terminology
**Migration**: Rename all `*UseCase` types to `*Service` and update all references

### Requirement: Interface definitions in service layer
**Reason**: Interfaces belong in model layer per dependency inversion principle
**Migration**: Move all interfaces from `internal/service/*/interfaces.go` to `internal/model/<domain>/`
