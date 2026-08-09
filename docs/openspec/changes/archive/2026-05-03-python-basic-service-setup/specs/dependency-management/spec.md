## ADDED Requirements

### Requirement: uv as package manager
The system SHALL use uv as the primary package manager for dependency management.

#### Scenario: Dependency installation
- **WHEN** a developer runs `uv sync`
- **THEN** all dependencies from pyproject.toml SHALL be installed
- **AND** a virtual environment SHALL be created automatically

### Requirement: Dependency groups
The system SHALL support separate dependency groups for different environments.

#### Scenario: Development dependencies
- **WHEN** the project is created
- **THEN** the following dependency groups SHALL be defined in pyproject.toml:
  - `dev` - development tools (ruff, mypy, pytest)
  - `test` - testing dependencies
  - `prod` - production dependencies only

#### Scenario: Installing specific group
- **WHEN** a developer runs `uv sync --group dev`
- **THEN** only the dev group dependencies SHALL be installed

### Requirement: Lock file
The system SHALL maintain a lock file for reproducible builds.

#### Scenario: Lock file generation
- **WHEN** dependencies are added or updated
- **THEN** uv.lock SHALL be updated automatically
- **AND** the lock file SHALL contain exact version numbers

### Requirement: Python version specification
The system SHALL specify the minimum Python version required.

#### Scenario: Python version constraint
- **WHEN** the project is created
- **THEN** pyproject.toml SHALL specify `requires-python = ">=3.11"`
