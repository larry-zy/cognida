## ADDED Requirements

### Requirement: Environment-based configuration
The system SHALL support multiple environment configurations (dev, test, prod).

#### Scenario: Environment loading
- **WHEN** the application starts
- **THEN** the system SHALL load environment variables from a .env file
- **AND** the .env file SHALL match the current APP_ENV environment

### Requirement: Pydantic settings
The system SHALL use pydantic-settings for configuration validation.

#### Scenario: Configuration validation
- **WHEN** the application starts
- **THEN** all required configuration values SHALL be validated
- **AND** if a required value is missing, the application SHALL fail to start with a clear error message

### Requirement: Type-safe configuration
The system SHALL provide type-safe access to configuration values.

#### Scenario: Configuration access
- **WHEN** a developer accesses a configuration value
- **THEN** the value SHALL be properly typed
- **AND** the type SHALL be enforced at application startup

### Requirement: Sensitive data handling
The system SHALL prevent logging of sensitive configuration values.

#### Scenario: Secret masking
- **WHEN** configuration is logged or displayed
- **THEN** values marked as secret SHALL be masked
- **AND** only the last 4 characters SHALL be shown

### Requirement: Configuration schema
The system SHALL provide a standard set of configuration options.

#### Scenario: Required configuration
- **WHEN** the application starts
- **THEN** the following configuration SHALL be required:
  - APP_NAME - application name
  - APP_ENV - environment (dev/test/prod)
  - LOG_LEVEL - logging level

#### Scenario: Optional configuration
- **WHEN** the application starts
- **THEN** the following configuration SHALL be optional:
  - API_PORT - API server port (default: 8000)
  - DATABASE_URL - database connection string
  - REDIS_URL - Redis connection string
