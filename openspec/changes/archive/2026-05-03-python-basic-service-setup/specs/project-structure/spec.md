## ADDED Requirements

### Requirement: Standard project directory layout
The system SHALL provide a standardized Python project directory structure that separates source code, tests, and configuration.

#### Scenario: New project initialization
- **WHEN** a developer initializes a new Python service
- **THEN** the project SHALL contain the following directories:
  - `src/` - application source code
  - `tests/` - test files
  - `scripts/` - utility scripts
  - `config/` - configuration files
  - `docs/` - documentation

### Requirement: Source code organization
The system SHALL organize application code under src/ with a clear package hierarchy.

#### Scenario: Source package structure
- **WHEN** the project is created
- **THEN** the following structure SHALL be created under `src/`:
  - `__init__.py` - package marker
  - `main.py` - application entry point
  - `api/` - API route handlers
  - `core/` - core business logic
  - `models/` - data models
  - `services/` - service layer

### Requirement: Configuration files placement
The system SHALL place configuration files at project root for easy access.

#### Scenario: Required configuration files
- **WHEN** the project is initialized
- **THEN** the following files SHALL be created at root:
  - `pyproject.toml` - project metadata and dependencies
  - `uv.lock` - dependency lock file
  - `.env.example` - environment variable template
  - `.gitignore` - git ignore rules
  - `README.md` - project documentation
