## ADDED Requirements

### Requirement: Clean Architecture Layer Separation
The system SHALL organize internal code into four distinct layers following Clean Architecture principles: Domain, Application, Infrastructure, and Interface. Each layer SHALL have a single responsibility and clear dependency direction (outer layers depend on inner layers only).

#### Scenario: Domain layer independence
- **GIVEN** the domain layer exists at `internal/domain/`
- **WHEN** inspecting domain package imports
- **THEN** it MUST NOT import any package from application, infrastructure, or interface layers
- **AND** it MAY ONLY import standard library and external dependencies

#### Scenario: Application layer depends on Domain
- **GIVEN** the application layer exists at `internal/application/`
- **WHEN** inspecting application package imports
- **THEN** it MAY import from domain layer
- **BUT** it MUST NOT import from infrastructure or interface layers

### Requirement: Legacy Service Directory Removal
The system SHALL NOT contain the legacy `internal/application/service/` directory. All functionality previously in this directory SHALL be migrated to appropriate Clean Architecture layers.

#### Scenario: No legacy service directory exists
- **GIVEN** a clean repository state
- **WHEN** listing directories under `internal/application/`
- **THEN** the `service/` directory MUST NOT exist
- **AND** agent functionality MUST be at `internal/application/agent/`
- **AND** rag functionality MUST be at `internal/application/rag/`

#### Scenario: All imports updated
- **GIVEN** the legacy service directory has been removed
- **WHEN** searching for imports of `application/service`
- **THEN** no results MUST be found in the codebase

### Requirement: Wire Dependency Injection Configuration
The system SHALL use Wire for compile-time dependency injection. All providers SHALL be defined in `cmd/wire/provider.go` and the wire generator SHALL successfully produce `wire_gen.go`.

#### Scenario: Wire generation succeeds
- **GIVEN** all code follows Clean Architecture
- **WHEN** running `wire` in `cmd/wire/` directory
- **THEN** `wire_gen.go` MUST be generated without errors
- **AND** the generated code MUST compile successfully

#### Scenario: All layers registered in Wire
- **GIVEN** the Wire provider configuration
- **WHEN** inspecting provider functions
- **THEN** each layer (domain repository, application service, infrastructure implementation, interface handler) MUST have corresponding provider functions
- **AND** dependencies MUST be correctly wired

### Requirement: No Circular Dependencies
The system SHALL NOT contain circular dependencies between layers or packages within the same layer.

#### Scenario: Build succeeds without circular import errors
- **GIVEN** the complete codebase
- **WHEN** running `go build ./...`
- **THEN** the build MUST complete successfully
- **AND** NO "import cycle not allowed" errors MUST occur
