## ADDED Requirements

### Requirement: Multi-stage Dockerfile
The system SHALL provide a multi-stage Dockerfile for production builds.

#### Scenario: Production image build
- **WHEN** the production Docker image is built
- **THEN** the image SHALL be based on a slim Python image
- **AND** the image SHALL contain only runtime dependencies
- **AND** build tools SHALL NOT be included in the final image

### Requirement: Development container
The system SHALL support development mode with hot reload.

#### Scenario: Development container startup
- **WHEN** the development container is started
- **THEN** the application SHALL mount the source code as a volume
- **AND** hot reload SHALL be enabled
- **AND** the debugger SHALL be accessible

### Requirement: Docker Compose configuration
The system SHALL provide a docker-compose.yml for local development.

#### Scenario: Complete stack startup
- **WHEN** docker-compose up is run
- **THEN** the following services SHALL be started:
  - The API application
  - PostgreSQL database (optional)
  - Redis (optional)

#### Scenario: Service networking
- **WHEN** services are started via docker-compose
- **THEN** all services SHALL be on the same network
- **AND** services SHALL communicate using service names as hostnames

### Requirement: Health check in container
The system SHALL include container health checks.

#### Scenario: Container health status
- **WHEN** the container is running
- **THEN** Docker SHALL periodically check the `/health` endpoint
- **AND** the container SHALL be marked healthy if the check succeeds
- **AND** the container SHALL be marked unhealthy if the check fails

### Requirement: Non-root user
The system SHALL run the application as a non-root user in production.

#### Scenario: User security
- **WHEN** the production container starts
- **THEN** the application SHALL run as a user named `appuser`
- **AND** the user SHALL have limited permissions
