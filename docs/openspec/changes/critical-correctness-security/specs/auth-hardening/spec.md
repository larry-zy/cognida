# auth-hardening Specification

## ADDED Requirements

### Requirement: Remove empty API-Key auth branch
The authentication middleware SHALL NOT contain a branch that authorizes any non-empty `X-API-Key` header without validating it against a persisted key store.

#### Scenario: X-API-Key branch removed
- **WHEN** inspecting `internal/handler/middleware/auth.go`
- **THEN** there MUST NOT be a branch that sets `user_id`/`tenant_id` solely because `X-API-Key` is non-empty
- **AND** a request presenting only an unverified `X-API-Key` MUST receive HTTP 401

#### Scenario: Request without valid credentials is rejected
- **WHEN** a request arrives with no valid `Authorization` bearer token and DEV_MODE is not enabled
- **THEN** the middleware MUST respond with HTTP 401
- **AND** it MUST NOT assign a default user or tenant

### Requirement: JWT secret startup validation
The system SHALL validate the JWT signing secret at startup and MUST refuse to start when it is missing, equal to a placeholder, or shorter than the minimum length.

#### Scenario: Missing secret fails startup
- **WHEN** the application starts and `JWT_SECRET` is unset or empty
- **THEN** the process MUST `log.Fatal` and exit
- **AND** it MUST NOT fall back to a default secret

#### Scenario: Placeholder secret fails startup
- **WHEN** `JWT_SECRET` equals the placeholder value `"your-secret-key"`
- **THEN** the process MUST `log.Fatal` and exit

#### Scenario: Short secret fails startup
- **WHEN** `JWT_SECRET` is shorter than 32 bytes
- **THEN** the process MUST `log.Fatal` and exit

### Requirement: CORS allowlist
The CORS middleware SHALL reflect the request `Origin` only when it matches a configured allowlist, and MUST NOT combine a wildcard/reflected origin with credentialed responses.

#### Scenario: Allowed origin is reflected
- **WHEN** a request `Origin` is present in the configured `AllowedOrigins`
- **THEN** the middleware SHALL set `Access-Control-Allow-Origin` to that origin
- **AND** MAY set `Access-Control-Allow-Credentials: true`

#### Scenario: Disallowed origin is not credentialed
- **WHEN** a request `Origin` is not in the allowlist
- **THEN** the middleware MUST NOT reflect that origin with `Access-Control-Allow-Credentials: true`
- **AND** the default `AllowedOrigins` MUST NOT be `["*"]`

### Requirement: Session authorization fail-closed
Session authorization SHALL enforce both tenant and user match and MUST deny access when identity is absent from the context.

#### Scenario: Tenant mismatch denied
- **WHEN** `authorizeSession` is evaluated and the session `tenant_id` does not equal the context tenant id
- **THEN** authorization MUST fail even if the user id matches

#### Scenario: Missing identity denied
- **WHEN** the context lacks a tenant id or user id during session authorization
- **THEN** authorization MUST fail closed (deny)
- **AND** it MUST NOT proceed with a default identity
