## ADDED Requirements

### Requirement: Complete publication parameters
The system SHALL expose expert and template-stage options and validate required authorized publication targets before queued execution.

#### Scenario: Text asset publication
- **WHEN** a user selects text input and a publication stage
- **THEN** a target knowledge base is required, persisted in the correct stage options, and used after approval

### Requirement: Goal-driven analysis
The system SHALL execute nonempty analysis/report goals against authorized immutable input, retaining deterministic statistics and evidence references while labelling model interpretation as unverified.

#### Scenario: Missing model
- **WHEN** a goal requires model analysis but the model is unavailable
- **THEN** the task fails visibly instead of merely echoing the goal with generic statistics

### Requirement: Deployable service topology
The production topology SHALL run MCP and authenticated lifecycle computation with readiness reflecting required service availability.

#### Scenario: Invalid internal authentication configuration
- **WHEN** production starts without required shared authentication
- **THEN** startup fails or readiness is unhealthy rather than silently omitting computation endpoints
