# Chat Service Refactor

## MODIFIED Requirements

### Requirement: Tenant Isolation

The use case SHALL enforce tenant isolation on all operations, including session authorization which MUST match both tenant and user and fail closed on missing identity.

#### Scenario: Cross-tenant access denied
- **WHEN** a user attempts to access model from different tenant
- **THEN** the use case returns error
- **AND** error message indicates access denied

#### Scenario: Session authorization matches tenant and user
- **WHEN** `authorizeSession` evaluates access to a session
- **THEN** it MUST require the session `tenant_id` to equal the context tenant id
- **AND** it MUST require the session `user_id` to equal the context user id
- **AND** it MUST deny access when either identity is absent from the context

## ADDED Requirements

### Requirement: Cross-turn summary is persisted
Cross-turn conversation summaries SHALL be written through to MySQL, with Redis used only as a cache, and reads MUST distinguish miss from error.

#### Scenario: UpdateSummary writes through to MySQL
- **WHEN** `UpdateSummary` is called
- **THEN** it MUST persist the summary to MySQL first
- **AND** it MUST update the Redis cache as a cache layer only

#### Scenario: GetSummary distinguishes miss from error
- **WHEN** `GetSummary` finds no cached summary
- **THEN** it MUST fall back to MySQL and backfill the cache on hit
- **AND** it MUST return an empty result for a genuine miss and an error only for a real failure

### Requirement: Memory branch shares pre-processing
The memory branch and the non-memory branch SHALL both execute the shared pre-processing (beforeHooks/middleware).

#### Scenario: Memory branch runs beforeHooks
- **WHEN** a request is routed through the memory branch
- **THEN** it MUST execute the shared beforeHooks/middleware pre-processing
- **AND** it MUST NOT bypass hooks/middleware that the non-memory branch runs
