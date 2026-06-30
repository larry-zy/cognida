## ADDED Requirements

### Requirement: PKG Directory Contains Only Pure Utilities
The `pkg/` directory SHALL contain only pure, reusable utility packages that have no dependencies on project-specific code. Infrastructure-related code SHALL be moved to `internal/infrastructure/`.

#### Scenario: Pure utility packages remain in pkg
- **GIVEN** the pkg directory restructuring
- **WHEN** listing packages in `pkg/`
- **THEN** it MAY contain: convert, crypto, errors, jwt, page, parser, response
- **AND** each package MUST be self-contained without internal project dependencies

#### Scenario: Infrastructure code moved out of pkg
- **GIVEN** the pkg directory restructuring
- **WHEN** inspecting `pkg/` directory contents
- **THEN** gorm.go MUST NOT exist in pkg/
- **AND** middleware/ directory MUST NOT exist in pkg/
- **AND** these items MUST be in `internal/infrastructure/`

### Requirement: GORM Utilities in Infrastructure Layer
Database-related utilities using GORM SHALL be located in the infrastructure layer, reflecting their role as external service adapters.

#### Scenario: GORM file location
- **GIVEN** the infrastructure layer exists
- **WHEN** looking for GORM utilities
- **THEN** gorm.go MUST be located at `internal/infrastructure/persistence/gorm.go` or similar infrastructure path
- **AND** MUST NOT be in `pkg/`

### Requirement: HTTP Middleware in Interface Layer
HTTP middleware SHALL be located in the interface layer, as they are part of the interface adapters that handle external requests.

#### Scenario: Middleware directory location
- **GIVEN** the interface layer structure
- **WHEN** looking for HTTP middleware
- **THEN** middleware MUST be at `internal/interface/http/middleware/`
- **AND** MUST NOT be in `pkg/middleware/`

#### Scenario: All imports updated to new location
- **GIVEN** middleware has been moved
- **WHEN** searching for imports of old pkg/middleware path
- **THEN** no results MUST be found
- **AND** all imports MUST reference the new interface layer path

### Requirement: PKG Packages Are Reusable
Each package in `pkg/` SHALL be designed such that it could be extracted and used in a separate Go project without modification.

#### Scenario: No internal dependencies
- **GIVEN** any package in `pkg/`
- **WHEN** inspecting its imports
- **THEN** it MUST NOT import from `internal/`
- **AND** it MUST NOT import from other project-specific packages
