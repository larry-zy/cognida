## 1. Create New Directory Structure

- [x] 1.1 Create `internal/interface/http/response/` directory
- [x] 1.2 Create `internal/application/dto/page/` directory
- [x] 1.3 Create `internal/infrastructure/auth/jwt/` directory
- [x] 1.4 Create `internal/infrastructure/crypto/` directory
- [x] 1.5 Create `internal/infrastructure/document/parser/` directory
- [x] 1.6 Create `internal/pkg/convert/` directory

## 2. Move Response Package

- [x] 2.1 Copy `pkg/response/response.go` to `internal/interface/http/response/response.go`
- [x] 2.2 Update package declaration to `package response`
- [x] 2.3 Verify no additional files in response package

## 3. Move Page Package

- [x] 3.1 Copy `pkg/page/page.go` to `internal/application/dto/page/page.go`
- [x] 3.2 Update package declaration to `package page`
- [x] 3.3 Verify no additional files in page package

## 4. Merge Errors Package

- [x] 4.1 Read `pkg/errors/errors.go` and `internal/domain/errors/errors.go`
- [x] 4.2 Merge error definitions into `internal/domain/errors/errors.go`
- [x] 4.3 Ensure all error codes are preserved
- [x] 4.4 Ensure all error types (BizError, predefined errors) are present

## 5. Move JWT Package

- [x] 5.1 Copy `pkg/jwt/jwt.go` to `internal/infrastructure/auth/jwt/jwt.go`
- [x] 5.2 Update package declaration to `package jwt`
- [x] 5.3 Verify no additional files in jwt package

## 6. Move Crypto Package

- [x] 6.1 Copy `pkg/crypto/password.go` to `internal/infrastructure/crypto/password.go`
- [x] 6.2 Update package declaration to `package crypto`
- [x] 6.3 Verify no additional files in crypto package

## 7. Move Parser Package

- [x] 7.1 Copy `pkg/parser/` directory to `internal/infrastructure/document/parser/`
- [x] 7.2 Update all package declarations in parser files
- [x] 7.3 Verify all parser files are moved (parser_factory.go, document_parser.go, pdf_parser.go, parser_test.go)

## 8. Move Convert Package

- [x] 8.1 Copy `pkg/convert/convert.go` to `internal/pkg/convert/convert.go`
- [x] 8.2 Update package declaration to `package convert`
- [x] 8.3 Verify no additional files in convert package

## 9. Handle Utils.go

- [x] 9.1 Audit `pkg/utils.go` for actual usage
- [x] 9.2 Move UUID/time utilities to `internal/pkg/utils/` if used
- [x] 9.3 Delete unused functions if any

## 10. Update Imports - Response

- [x] 10.1 Find all imports of `github.com/yourusername/link/pkg/response`
- [x] 10.2 Replace with `github.com/yourusername/link/internal/interface/http/response`
- [x] 10.3 Verify interface/http handlers import new location

## 11. Update Imports - Page

- [x] 11.1 Find all imports of `github.com/yourusername/link/pkg/page`
- [x] 11.2 Replace with `github.com/yourusername/link/internal/application/dto/page`
- [x] 11.3 Verify application/usecases import new location

## 12. Update Imports - Errors

- [x] 12.1 Find all imports of `github.com/yourusername/link/pkg/errors`
- [x] 12.2 Replace with `github.com/yourusername/link/internal/domain/errors`
- [x] 12.3 Verify all layers import from domain/errors

## 13. Update Imports - JWT

- [x] 13.1 Find all imports of `github.com/yourusername/link/pkg/jwt`
- [x] 13.2 Replace with `github.com/yourusername/link/internal/infrastructure/auth/jwt`
- [x] 13.3 Verify interface middleware and infrastructure import new location

## 14. Update Imports - Crypto

- [x] 14.1 Find all imports of `github.com/yourusername/link/pkg/crypto`
- [x] 14.2 Replace with `github.com/yourusername/link/internal/infrastructure/crypto`
- [x] 14.3 Verify application/usecases/user import new location

## 15. Update Imports - Parser

- [x] 15.1 Find all imports of `github.com/yourusername/link/pkg/parser`
- [x] 15.2 Replace with `github.com/yourusername/link/internal/infrastructure/document/parser`
- [x] 15.3 Verify infrastructure code imports new location

## 16. Update Imports - Convert

- [x] 16.1 Find all imports of `github.com/yourusername/link/pkg/convert`
- [x] 16.2 Replace with `github.com/yourusername/link/internal/pkg/convert`
- [x] 16.3 Verify all locations using convert update imports

## 17. Update Imports - Utils

- [x] 17.1 Find all imports of `github.com/yourusername/link/pkg` (root utils)
- [x] 17.2 Replace with appropriate new location or delete if unused
- [x] 17.3 Verify no remaining pkg imports

## 18. Update Wire Configuration

- [x] 18.1 Check `cmd/wire/wire.go` for pkg references
- [x] 18.2 Update any provider functions that use moved types
- [x] 18.3 Regenerate wire code with `wire gen ./cmd/wire`

## 19. Verify Build

- [x] 19.1 Run `go mod tidy`
- [x] 19.2 Run `go build ./...` and fix any compilation errors
- [x] 19.3 Run `go test ./...` and fix any test failures

## 20. Cleanup

- [x] 20.1 Delete `pkg/response/` directory
- [x] 20.2 Delete `pkg/page/` directory
- [x] 20.3 Delete `pkg/errors/` directory
- [x] 20.4 Delete `pkg/jwt/` directory
- [x] 20.5 Delete `pkg/crypto/` directory
- [x] 20.6 Delete `pkg/parser/` directory
- [x] 20.7 Delete `pkg/convert/` directory
- [x] 20.8 Delete `pkg/utils.go`
- [x] 20.9 Delete `pkg/` directory if empty
- [x] 20.10 Final verification: run `go build ./...` and `go test ./...`
