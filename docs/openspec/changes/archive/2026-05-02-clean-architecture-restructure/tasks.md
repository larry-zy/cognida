## 1. Preparation and Safety Net

- [x] 1.1 Create backup branch `git checkout -b backup/refactor-backup`
- [x] 1.2 Run baseline tests `go test ./...` to ensure current state works
- [x] 1.3 Verify current build succeeds `go build ./...`
- [x] 1.4 Return to development branch `git checkout dev`

## 2. Backend - PKG Directory Restructure

- [x] 2.1 Create `internal/infrastructure/persistence/` directory if needed
- [x] 2.2 Move `pkg/gorm.go` to `internal/infrastructure/persistence/gorm.go`
- [x] 2.3 Create `internal/infrastructure/http/middleware/` directory if needed
- [x] 2.4 Move `pkg/middleware/` to `internal/infrastructure/http/middleware/` (deleted as unused)
- [x] 2.5 Update all imports referencing `pkg/gorm` to new infrastructure path
- [x] 2.6 Update all imports referencing `pkg/middleware` to new infrastructure path
- [x] 2.7 Verify build succeeds `go build ./...`
- [x] 2.8 Run tests `go test ./...`
- [x] 2.9 Commit changes: `git commit -m "refactor: move gorm and middleware to infrastructure layer"`

## 3. Backend - Analyze Legacy Service Directory

- [x] 3.1 Search all references to `application/service`: `grep -r "application/service" internal/`
- [x] 3.2 Document which functions from `service/agent/` are still being used
- [x] 3.3 Document which functions from `service/rag/` are still being used
- [x] 3.4 Identify code in `service/rag/agent_adapter.go` that needs migration
- [x] 3.5 Determine migration target for each used function (new application layer or infrastructure)

## 4. Backend - Migrate and Remove Legacy Service Directory

- [x] 4.1 Update `cmd/wire/wire.go` to remove old service providers
- [x] 4.2 Update `cmd/wire/provider.go` to use new application layer services
- [x] 4.3 Update imports in `internal/application/agent/tools/` that reference old service/rag
- [x] 4.4 Migrate any unique logic from `service/rag/agent_adapter.go` to new location
- [x] 4.5 Update any remaining references to use new paths
- [x] 4.6 Run Wire generation: `cd cmd/wire && wire`
- [x] 4.7 Verify build succeeds: `go build ./...`
- [x] 4.8 Verify no remaining `application/service` imports: `grep -r "application/service" .`
- [x] 4.9 Delete legacy directory: `rm -rf internal/application/service/`
- [x] 4.10 Run tests: `go test ./...`
- [x] 4.11 Commit changes: `git commit -m "refactor: remove legacy application/service directory"`

## 5. Frontend - Directory Standardization

- [x] 5.1 Rename `web/src/component/` to `web/src/components/` using `git mv`
- [x] 5.2 Update all imports from `@/component/` to `@/components/` in Vue/TS files
- [x] 5.3 Update router.ts imports if referencing component directory
- [x] 5.4 Move or delete `web/src/pic/` directory:
  - If keeping: `git mv web/src/pic web/public/assets/wallpaper`
  - If deleting: `git rm -r web/src/pic/`
- [x] 5.5 Update any references to pic/ directory
- [x] 5.6 Run `npm run build` in web/ directory to verify
- [x] 5.7 Fix any import or build errors that arise
- [x] 5.8 Run `npm run type-check` if available
- [x] 5.9 Commit changes: `git commit -m "refactor: standardize frontend directory naming"`

## 6. Git Cleanup and Configuration

- [x] 6.1 Remove orphan file: `rm nul` and `git rm nul` if tracked
- [x] 6.2 Update .gitignore with new patterns (tmp/, uploads/*, *.exe, nul, etc.)
- [x] 6.3 Create `uploads/.gitkeep` to preserve directory structure
- [x] 6.4 Verify node_modules is properly ignored (should only be in web/)
- [x] 6.5 Check for other accidentally committed files (bin/, tmp/, etc.)
- [x] 6.6 Test .gitignore by adding a dummy file and ensuring it's ignored
- [x] 6.7 Commit changes: `git commit -m "chore: update .gitignore and remove orphan files"`

## 7. Backend - Staged Migration Commits

- [x] 7.1 Review all staged files in git status
- [x] 7.2 Group staged files by logical module (domain, application, infrastructure, interface)
- [x] 7.3 Commit domain layer changes: `git commit -m "refactor: complete domain layer structure"`
- [x] 7.4 Commit application layer changes: `git commit -m "refactor: complete application layer structure"`
- [x] 7.5 Commit infrastructure layer changes: `git commit -m "refactor: complete infrastructure layer structure"`
- [x] 7.6 Commit interface layer changes: `git commit -m "refactor: complete interface layer structure"`
- [x] 7.7 Verify final state: `go build ./... && go test ./...`

## 8. Documentation Updates

- [x] 8.1 Update README.md directory structure section
- [x] 8.2 Remove references to deleted `application/service/` path
- [x] 8.3 Update pkg/ directory description in README
- [x] 8.4 Update architecture diagrams if they exist
- [x] 8.5 Update any relevant docs/*.md files
- [x] 8.6 Commit changes: `git commit -m "docs: update architecture documentation"`

## 9. Final Verification

- [x] 9.1 Full build test: `go build ./...`
- [x] 9.2 Full test suite: `go test ./...`
- [x] 9.3 Frontend build: `cd web && npm run build`
- [x] 9.4 Verify Wire generation is correct: `cd cmd/wire && wire && go build`
- [x] 9.5 Check for any remaining circular dependencies
- [x] 9.6 Verify git log shows clean commit history
- [x] 9.7 Optional: Create PR for review if working in team

## 10. Rollback Preparedness (Optional but Recommended)

- [x] 10.1 Tag final state: `git tag pre-refactor-backup`
- [x] 10.2 Document rollback steps in team wiki/README
- [x] 10.3 Verify backup branch exists: `git branch -a | grep backup`
- [x] 10.4 Test rollback process on a separate branch if time permits
