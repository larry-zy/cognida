# Pkg Package Layer Alignment

This spec defines the proper layer placement for code currently in the `pkg` package. Since this is an internal refactoring, there are no behavior changes - only code relocation.

## ADDED Requirements

### Requirement: HTTP utilities reside in interface layer
All HTTP-specific utilities including response helpers SHALL reside in `internal/interface/http/` package.

#### Scenario: Response utilities in interface layer
- **GIVEN** HTTP response utilities such as Success, Fail, JSON helpers
- **WHEN** locating their implementation
- **THEN** they reside in `internal/interface/http/response/`
- **AND** they are NOT in any `pkg/` package

### Requirement: Pagination types in application DTO layer
Pagination request and response types SHALL reside in `internal/application/dto/page/` package as data transfer objects.

#### Scenario: Pagination types in application layer
- **GIVEN** pagination request (Req) and response (Resp) types
- **WHEN** locating their definition
- **THEN** they reside in `internal/application/dto/page/`
- **AND** they are imported by both interface and application layers

### Requirement: Business errors in domain layer
All business error types and codes SHALL reside in `internal/domain/errors/` package.

#### Scenario: Business errors consolidated in domain
- **GIVEN** business error types such as BizError
- **WHEN** locating error definitions
- **THEN** they reside in `internal/domain/errors/`
- **AND** all error codes (1-9999) are defined in one place

### Requirement: Authentication in infrastructure layer
JWT implementation and other authentication utilities SHALL reside in `internal/infrastructure/auth/` package.

#### Scenario: JWT in infrastructure layer
- **GIVEN** JWT token generation and parsing utilities
- **WHEN** locating their implementation
- **THEN** they reside in `internal/infrastructure/auth/jwt/`
- **AND** they are NOT referenced by domain layer

### Requirement: Cryptography in infrastructure layer
Password hashing and cryptographic utilities SHALL reside in `internal/infrastructure/crypto/` package.

#### Scenario: Password hashing in infrastructure layer
- **GIVEN** Argon2id password hashing implementation
- **WHEN** locating the crypto utilities
- **THEN** they reside in `internal/infrastructure/crypto/`
- **AND** external crypto dependencies are contained

### Requirement: Document parsing in infrastructure layer
Document parsing implementations SHALL reside in `internal/infrastructure/document/parser/` package.

#### Scenario: Parser implementations in infrastructure layer
- **GIVEN** PDF, DOCX, and other document parsers
- **WHEN** locating parser implementations
- **THEN** they reside in `internal/infrastructure/document/parser/`
- **AND** they are NOT in a top-level `pkg/` package

### Requirement: Shared utilities in internal/pkg
Pure utility functions without external dependencies MAY reside in `internal/pkg/` for cross-layer sharing.

#### Scenario: Type conversion in internal/pkg
- **GIVEN** type conversion utilities (convert package)
- **WHEN** locating shared utilities
- **THEN** they reside in `internal/pkg/convert/`
- **AND** they have no external dependencies

## REMOVED Requirements

### Requirement: Top-level pkg package
**Reason**: The `pkg` package violates Clean Architecture layering by mixing concerns from different layers.

**Migration**:
1. Identify all code in `pkg/` and determine its proper layer
2. Move code to appropriate `internal/` location based on decisions in this spec
3. Update all import references throughout the codebase
4. Delete the `pkg/` directory
