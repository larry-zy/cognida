# Application & Domain Layer Refactoring - Implementation Tasks

## 0. Phase 0: Move LLM and RAG to Infrastructure (Preparation)

- [x] 0.1 Create `infrastructure/llm/` directory
- [x] 0.2 Move `domain/llm/entity.go` to `infrastructure/llm/model.go`
- [x] 0.3 Move `domain/llm/repository.go` to `infrastructure/llm/`
- [x] 0.4 Create `infrastructure/rag/` directory
- [x] 0.5 Extract non-domain entities from `domain/rag/` to infrastructure/rag
- [x] 0.6 Update all import paths referencing old domain/llm and domain/rag
- [x] 0.7 Run tests to verify infrastructure move

## 1. Phase 1: Merge KB and Graph into Knowledge Domain

- [ ] 1.1 Create `domain/knowledge/` directory
- [ ] 1.2 Merge `domain/kb/entity.go` and `domain/graph/entity.go` into `knowledge/entity.go`
- [ ] 1.3 Create `knowledge/graph.go` with GraphNode, GraphRelation, GraphData
- [ ] 1.4 Create `knowledge/vector.go` with VectorDocument and related types
- [ ] 1.5 Create `knowledge/repository.go` merging all repository interfaces
- [ ] 1.6 Create `knowledge/retriever.go` with Retriever interface
- [ ] 1.7 Create `knowledge/errors.go` with all error types
- [ ] 1.8 Update all imports from domain/kb and domain/graph to domain/knowledge
- [ ] 1.9 Delete old `domain/kb/` and `domain/graph/` directories
- [ ] 1.10 Run tests to verify knowledge domain merge

## 2. Phase 2: Rename Chat to Conversation

- [ ] 2.1 Rename `domain/chat/` directory to `domain/conversation/`
- [ ] 2.2 Update package declarations in all files
- [ ] 2.3 Rename `application/usecases/chat/` to `application/usecases/conversation/`
- [ ] 2.4 Update all imports referencing old chat package
- [ ] 2.5 Update wire dependency injection configuration
- [ ] 2.6 Run tests to verify conversation rename

## 3. Phase 3: Move Agent to Application Layer

- [ ] 3.1 Create `application/usecases/assistant/` directory
- [ ] 3.2 Move Agent entity definitions to appropriate location (domain or usecase)
- [ ] 3.3 Create assistant usecase files from domain/agent patterns
- [ ] 3.4 Update imports and references
- [ ] 3.5 Delete old `domain/agent/` directory
- [ ] 3.6 Run tests to verify agent move

## 4. Phase 4: Move Evaluation to Application Layer

- [ ] 4.1 Move `domain/evaluation/` to `application/usecases/evaluation/`
- [ ] 4.2 Keep metric definitions in shared location if needed
- [ ] 4.3 Restructure as usecase (orchestration) rather than domain
- [ ] 4.4 Update all imports and references
- [ ] 4.5 Run tests to verify evaluation move

## 5. Phase 5: Clean Up Services and Types/Interfaces

- [ ] 5.1 Review all files in `domain/services/`
- [ ] 5.2 Move interfaces to appropriate domain packages or delete
- [ ] 5.3 Review `domain/types/interfaces/` contents
- [ ] 5.4 Keep Repository interfaces in domain
- [ ] 5.5 Move Service interfaces to application/usecases
- [ ] 5.6 Delete empty `domain/services/` and `domain/types/interfaces/` directories
- [ ] 5.7 Create `domain/shared/` for truly shared types
- [ ] 5.8 Run tests to verify cleanup

## 6. Phase 6: Move Chunker to Infrastructure

- [ ] 6.1 Create `infrastructure/document/chunker/` directory
- [ ] 6.2 Move `application/chunker/semantic.go` to new location
- [ ] 6.3 Update package declaration and imports
- [ ] 6.4 Update all import paths referencing `internal/application/chunker`
- [ ] 6.5 Delete empty `application/chunker/` directory
- [ ] 6.6 Run tests to verify chunker move

## 7. Phase 7: Split LLM Service into UseCases

- [ ] 7.1 Create `application/usecases/llm/` directory
- [ ] 7.2 Create `chat_usecase.go` from llm/service.go ChatService
- [ ] 7.3 Create `embedding_usecase.go` from EmbeddingService
- [ ] 7.4 Create `rerank_usecase.go` from RerankService
- [ ] 7.5 Create `model_usecase.go` from ModelService
- [ ] 7.6 Update wire configuration
- [ ] 7.7 Update handler imports
- [ ] 7.8 Delete `application/llm/service.go`
- [ ] 7.9 Run tests to verify LLM usecase split

## 8. Phase 8: Refactor Conversation UseCase (Dependency Inversion)

- [ ] 8.1 Define ChatService interface in appropriate domain location
- [ ] 8.2 Refactor conversation_usecase.go to use domain interface
- [ ] 8.3 Remove infrastructure/llm/chat import
- [ ] 8.4 Remove infrastructure/config import
- [ ] 8.5 Remove createChatInstance() method
- [ ] 8.6 Update constructor to accept domain interface
- [ ] 8.7 Update wire configuration
- [ ] 8.8 Run tests to verify conversation refactoring

## 9. Phase 9: Knowledge Graph Business Logic to Domain

- [ ] 9.1 Create `domain/knowledge/graph_service.go` for business logic
- [ ] 9.2 Move PMI/Weight calculation from application to domain
- [ ] 9.3 Move graph merge logic from application to domain
- [ ] 9.4 Create `infrastructure/graph/llm_extractor.go` for LLM calls
- [ ] 9.5 Move LLM entity/relation extraction to infrastructure
- [ ] 9.6 Refactor application/graph to pure orchestration
- [ ] 9.7 Update wire configuration
- [ ] 9.8 Run tests to verify knowledge graph refactoring

## 10. Phase 10: Rename KB UseCase to Knowledge

- [ ] 10.1 Rename `application/usecases/kb/` to `application/usecases/knowledge/`
- [ ] 10.2 Update package declarations
- [ ] 10.3 Update all imports
- [ ] 10.4 Update wire configuration
- [ ] 10.5 Run tests to verify knowledge usecase rename

## 11. Phase 11: Clean Up Adapters and Empty Directories

- [ ] 11.1 Verify no external callers of `usecases/knowledge/service.go` (was kb/service.go)
- [ ] 11.2 Delete `usecases/knowledge/service.go` adapter
- [ ] 11.3 Verify no external callers of `usecases/rag/service.go`
- [ ] 11.4 Delete `usecases/rag/service.go` adapter
- [ ] 11.5 Delete empty `application/repository/` directory
- [ ] 11.6 Verify no remaining infrastructure imports in application layer
- [ ] 11.7 Run full test suite

## 12. Phase 12: Final Verification and Documentation

- [ ] 12.1 Verify domain layer only contains business domains
- [ ] 12.2 Verify application layer only orchestrates use cases
- [ ] 12.3 Verify infrastructure contains all technical implementations
- [ ] 12.4 Run full test suite
- [ ] 12.5 Check for circular dependencies
- [ ] 12.6 Update CLAUDE.md with new structure
- [ ] 12.7 Update README with new architecture
- [ ] 12.8 Create architecture decision records if needed
