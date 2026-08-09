## 1. Domain Layer Refactoring

- [x] 1.1 Create `domain/agent/service.go` with service interfaces
- [x] 1.2 Define `HookService` interface (Before, After methods)
- [x] 1.3 Define `ToolRegistry` interface (Register, Get, List, Enable, Disable)
- [x] 1.4 Define `ToolExecutor` interface (Execute, ExecuteStream)
- [x] 1.5 Refactor `domain/agent/entity.go` - mark deprecated types
- [x] 1.6 Add deprecation comments to `ChatChunk`, `ToolCallRecord`, `AgentRequest`, `AgentResponse`
- [x] 1.7 Remove `AgentOrchestrator` interface from `domain/agent/repository.go`
- [x] 1.8 Update `domain/agent/repository.go` to contain only data access interfaces

## 2. Application Layer Restructuring

- [x] 2.1 Add missing DTOs to `application/usecases/agent/dto.go`
- [x] 2.2 Add `ChatChunkDTO` for streaming responses
- [x] 2.3 Add `ToolCallRecordDTO` for tool call records
- [x] 2.4 Add conversion functions `ToDomainAgentRequest` and `FromDomainAgentResponse`
- [x] 2.5 Remove duplicate Agent interface from `application/usecases/agent/eino_agent.go`
- [x] 2.6 Keep `eino_agent.go` only for Eino-specific adapter logic
- [x] 2.7 Move `BaseAgent` from `application/usecases/agent/base_agent.go` to `infrastructure/agent/`
- [x] 2.8 Update imports in files that reference `BaseAgent`
- [x] 2.9 Verify `orchestration/` patterns depend only on Domain layer
- [x] 2.10 Update `application/usecases/agent/eino_builder.go` to use moved `BaseAgent`

## 3. Infrastructure Layer Corrections

- [x] 3.1 Create `infrastructure/agent/base_agent.go` (move from application)
- [x] 3.2 Ensure `base_agent.go` imports only Domain layer and Eino
- [x] 3.3 Refactor `infrastructure/adapter/agent/rag.go` - remove application layer imports
- [x] 3.4 Update `rag.go` to depend on Domain interfaces instead of use cases
- [x] 3.5 Refactor `infrastructure/adapter/agent/tools.go` - remove application layer imports
- [x] 3.6 Create `infrastructure/agent/tools/` directory for tool implementations
- [x] 3.7 Move `rag_query` tool to `infrastructure/agent/tools/rag_query.go`
- [x] 3.8 Move `graph_query` tool to `infrastructure/agent/tools/graph_query.go`
- [x] 3.9 Move `web_search` tool to `infrastructure/agent/tools/web_search.go`
- [x] 3.10 Create `infrastructure/agent/tools/adapter.go` for Eino tool adaptation

## 4. Hooks Implementation

- [x] 4.1 Update `infrastructure/agent/hooks/base.go` to implement `domain.HookService`
- [x] 4.2 Update `infrastructure/agent/hooks/conclusion.go` to implement `domain.HookService`
- [x] 4.3 Update `infrastructure/agent/hooks/clarification.go` to implement `domain.HookService`
- [x] 4.4 Update `infrastructure/agent/hooks/doc.go` with correct package documentation
- [x] 4.5 Verify hooks integrate correctly with `eino_builder.go`

## 5. Tools Registry Implementation

- [x] 5.1 Create `infrastructure/agent/registry.go` implementing `domain.ToolRegistry`
- [x] 5.2 Move tool registry logic from `application/usecases/agent/tools/registry.go`
- [x] 5.3 Implement `ToolExecutor` in `infrastructure/agent/executor.go`
- [x] 5.4 Update `application/usecases/agent/tools/` to depend on Domain interfaces
- [x] 5.5 Remove tool implementations from `application/usecases/agent/tools/`
- [x] 5.6 Keep only tool wrappers/factories in `application/usecases/agent/tools/`

## 6. Test Updates

- [ ] 6.1 Update `application/usecases/agent/eino_agent_test.go` for new structure
- [ ] 6.2 Update `application/usecases/agent/eino_middleware_test.go` for new structure
- [ ] 6.3 Update `application/usecases/agent/test/agent_integration_test.go`
- [ ] 6.4 Update `application/usecases/agent/orchestration/*_test.go` files
- [ ] 6.5 Add tests for new `infrastructure/agent/base_agent.go`
- [ ] 6.6 Add tests for `infrastructure/agent/registry.go`
- [ ] 6.7 Verify all tests pass with `go test ./internal/...`

## 7. Dependency Verification

- [ ] 7.1 Run `go mod tidy` to clean up dependencies
- [ ] 7.2 Verify no infrastructure → application imports with `go mod graph | grep infrastructure`
- [ ] 7.3 Run `go vet ./internal/...` to check for issues
- [ ] 7.4 Run `golangci-lint` if available
- [ ] 7.5 Build the project to ensure no compilation errors

## 8. Documentation Updates

- [ ] 8.1 Update `CLAUDE.md` files with new structure if needed
- [ ] 8.2 Update `internal/domain/CLAUDE.md` with service interface guidance
- [ ] 8.3 Update `internal/application/CLAUDE.md` with use case patterns
- [ ] 8.4 Update `internal/infrastructure/CLAUDE.md` with implementation patterns

## 9. Cleanup

- [ ] 9.1 Remove deprecated types from `domain/agent/entity.go`
- [ ] 9.2 Remove old unused files after migration is complete
- [ ] 9.3 Remove deprecated code markers
- [ ] 9.4 Final test run to ensure everything works

## 10. Validation

- [ ] 10.1 Run full test suite: `go test ./...`
- [ ] 10.2 Build project: `go build ./...`
- [ ] 10.3 Check for circular dependencies
- [ ] 10.4 Verify agent execution end-to-end if possible
- [ ] 10.5 Review changes against original proposal
