# agent-tools Specification

## Purpose
TBD - created by archiving change agent-layer-cleanup. Update Purpose after archive.
## Requirements
### Requirement: Domain layer defines Tool service interfaces
The system SHALL define tool-related service interfaces in the Domain layer.

#### Scenario: ToolRegistry interface exists in domain
- **WHEN** inspecting `domain/agent/service.go`
- **THEN** `ToolRegistry` interface defines methods:
  - `Register(tool Tool) error`
  - `Get(name string) (Tool, bool)`
  - `List() []Tool`
  - `Enable(name string) error`
  - `Disable(name string) error`

#### Scenario: ToolExecutor interface exists in domain
- **WHEN** inspecting `domain/agent/service.go`
- **THEN** `ToolExecutor` interface defines methods:
  - `Execute(ctx context.Context, name string, input string) (string, error)`
  - `ExecuteStream(ctx context.Context, name string, input string) (<-chan string, error)`

### Requirement: Domain layer defines Tool entity
The system SHALL define the Tool entity in the Domain layer with core attributes.

#### Scenario: Tool entity contains core attributes
- **WHEN** inspecting `domain/agent/entity.go`
- **THEN** `Tool` struct contains: `ID`, `Name`, `Description`, `Type`, `Enabled`, `Config`, `CreatedAt`, `UpdatedAt`

### Requirement: Infrastructure layer implements concrete tools
The system SHALL implement specific tools in the Infrastructure layer.

#### Scenario: RAGQueryTool exists
- **WHEN** inspecting `infrastructure/agent/tools/rag_query.go`
- **THEN** file implements a tool that queries the RAG system
- **AND** file depends on Domain interfaces, not Application use cases

#### Scenario: GraphQueryTool exists
- **WHEN** inspecting `infrastructure/agent/tools/graph_query.go`
- **THEN** file implements a tool that queries the knowledge graph
- **AND** file depends on Domain interfaces

#### Scenario: WebSearchTool exists
- **WHEN** inspecting `infrastructure/agent/tools/web_search.go`
- **THEN** file implements a web search tool
- **AND** file depends on Domain interfaces

### Requirement: Tool registry implementation exists in infrastructure
The system SHALL provide a concrete implementation of ToolRegistry in Infrastructure layer.

#### Scenario: RegistryImpl implements ToolRegistry
- **WHEN** inspecting `infrastructure/agent/registry.go`
- **THEN** file implements `domain.ToolRegistry` interface
- **AND** file manages tool lifecycle (registration, enable/disable)

### Requirement: Tools adapt Eino framework to Domain interface
The system SHALL adapt Eino framework tool types to Domain tool interface.

#### Scenario: Eino tool adapter exists
- **WHEN** inspecting `infrastructure/agent/tools/adapter.go`
- **THEN** file converts `tool.BaseTool` (Eino) to `Tool` (Domain)
- **AND** file handles both InvokableTool and StreamableTool types

### Requirement: Application layer uses tools via Domain interface
The system SHALL ensure Application layer interacts with tools through Domain interfaces only.

#### Scenario: Use case accepts ToolRegistry interface
- **WHEN** inspecting `application/usecases/agent/`
- **THEN** use cases accept `domain.ToolRegistry` as dependency
- **AND** use cases do NOT import `infrastructure` packages

