# tenant-isolation Specification

## ADDED Requirements

### Requirement: Handler passes authenticated tenant
HTTP handlers SHALL derive the tenant boundary from the authenticated context via `GetTenantID(c)` and MUST NOT trust any `tenant_id` supplied in the request body or query.

#### Scenario: Handler uses context tenant
- **WHEN** a knowledge/document/chunk handler processes a read, update, or delete request
- **THEN** the handler MUST obtain the tenant id from `GetTenantID(c)`
- **AND** it MUST pass that tenant id down to the service call
- **AND** it MUST NOT read `tenant_id` from the request payload for authorization

#### Scenario: Missing tenant in context is rejected
- **WHEN** the authenticated context has no tenant id
- **THEN** the handler MUST reject the request as unauthorized
- **AND** it MUST NOT default the tenant id to any fixed value

### Requirement: Service enforces tenant ownership
Service methods that read, update, or delete tenant-owned resources SHALL accept an explicit `tenantID` parameter and MUST enforce ownership before returning or mutating any record.

#### Scenario: FindByID requires tenant
- **WHEN** `knowledgeBaseService.FindByID(ctx, id, tenantID)` is called
- **THEN** the method SHALL only return the record whose `tenant_id` equals `tenantID`
- **AND** if the record belongs to another tenant it MUST return a not-found / access-denied error rather than the record

#### Scenario: GetChunks requires tenant
- **WHEN** `knowledgeBaseService.GetChunks(ctx, kbID, tenantID, ...)` is called
- **THEN** the method SHALL scope results to the knowledge base owned by `tenantID`
- **AND** chunks of a knowledge base owned by another tenant MUST NOT be returned

#### Scenario: Update and Delete verify ownership
- **WHEN** an update or delete is requested for a resource id
- **THEN** the service SHALL verify the resource `tenant_id` matches the caller tenant before mutating
- **AND** a mismatch MUST result in an access-denied error and no mutation

### Requirement: Repository enforces tenant boundary in SQL
Repository queries for tenant-owned resources SHALL include a `tenant_id` predicate as a defense-in-depth boundary, independent of upper-layer checks.

#### Scenario: Read query scoped by tenant
- **WHEN** a repository loads a resource by id on behalf of a tenant
- **THEN** the SQL SHALL be `WHERE id = ? AND tenant_id = ?`
- **AND** a row belonging to a different tenant MUST NOT be selected even if the id matches

#### Scenario: Write query scoped by tenant
- **WHEN** a repository updates or deletes a resource by id on behalf of a tenant
- **THEN** the SQL SHALL constrain by both `id` and `tenant_id`
- **AND** zero rows MUST be affected when the tenant does not own the row

### Requirement: Cross-tenant access is denied
The system SHALL deny any attempt by an authenticated user to access another tenant's data through id-based endpoints (IDOR protection).

#### Scenario: IDOR attempt returns not found
- **WHEN** a user authenticated for tenant A requests a resource id owned by tenant B
- **THEN** the system MUST respond as if the resource does not exist (or access denied)
- **AND** it MUST NOT leak the resource content or existence details of tenant B
