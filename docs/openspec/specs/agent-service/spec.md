# Agent Service Refactor

## Purpose
规整自历史遗留的 delta 格式主规格（原文件误含 ADDED/MODIFIED/REMOVED 增量头，结构非法）。此处折叠为合法主规格：保留原 ADDED/MODIFIED 需求正文，丢弃已声明 REMOVED 的「Agent as Domain Package」。归档 architecture-hardening 时其 agent-service delta 会在此叠加。
## Requirements
### Requirement: Domain Restructuring - Technical Components Moved

The domain layer SHALL only contain business domains, using business language in naming.

#### Scenario: Technical components moved to infrastructure
- **WHEN** reviewing domain layer packages
- **THEN** llm, rag, agent are not present as domain packages
- **AND** they are located in infrastructure or application layer

#### Scenario: Business language used in naming
- **WHEN** domain packages are named
- **THEN** assistant is used for agent capabilities
- **AND** conversation is used instead of chat
- **AND** knowledge is used instead of kb

### Requirement: Domain Layer Package Reorganization

The system SHALL organize domain packages by business domain, not technical components.

#### Scenario: Agent moved to application layer
- **WHEN** reviewing application layer
- **THEN** application/usecases/assistant contains agent orchestration logic
- **AND** domain layer does not contain agent package

### Requirement: Agent entity structure
The Agent entity SHALL contain only business state and behavior. Request/Response DTOs SHALL be removed.

#### Scenario: Agent entity contains business attributes only
- **GIVEN** the Agent entity in domain/agent/entity.go
- **WHEN** examining its structure
- **THEN** it contains ID, Name, Description, Type, Status, Config
- **AND** it does NOT contain AgentRequest, AgentResponse, or ChatChunk

### Requirement: Agent use case organization
Agent application logic SHALL be organized into distinct use cases under application/usecases/assistant/.

#### Scenario: Agent use case structure
- **GIVEN** the assistant bounded context
- **WHEN** examining application/usecases/assistant/
- **THEN** it contains use cases like execute.go, research.go, config.go
- **AND** each use case has its own DTOs in dto.go
- **AND** interfaces are defined in interfaces.go

### Requirement: Similarity calculation in domain service
Similarity calculation logic SHALL be moved from application/agent/service.go to domain/services/similarity.go.

#### Scenario: Similarity calculation via domain service
- **GIVEN** a need to calculate similarity
- **WHEN** invoking similarity calculation
- **THEN** it uses domain/services/similarity package
- **AND** CalculateSimilarity method is NOT in application/agent/service.go

### Requirement: Agent orchestrator interface
The AgentOrchestrator interface SHALL remain in domain/agent/repository.go as it represents a domain capability. Agent 实例 SHALL 由注册表驱动而非全局单例：`GetAgentByID` SHALL 改为向 `AgentRegistry` 查询，MUST NOT 用硬编码 switch 分支或包全局构造的单例返回 agent。

#### Scenario: AgentOrchestrator in domain
- **GIVEN** the AgentOrchestrator interface
- **WHEN** locating its definition
- **THEN** it resides in domain/agent/repository.go
- **AND** it defines domain methods like Execute, ExecuteStream, GetTools

#### Scenario: GetAgentByID 注册表化
- **WHEN** 调用 `GetAgentByID(agentID)`
- **THEN** 系统 SHALL 向 `AgentRegistry` 查询返回对应 agent
- **AND** MUST NOT 通过硬编码 switch 分支或包全局单例返回

#### Scenario: orchestrator 从注册表取 agent
- **WHEN** 检查 `cmd/wire`
- **THEN** agent orchestrator SHALL 由注册表查询函数装配（如 `NewRegistryAgentOrchestrator(registry.Get)`）
- **AND** MUST NOT 直接依赖包全局的 `GetAgentByID` 单例装配

### Requirement: Agent DTOs in application layer
Agent-related DTOs (AgenticRAGRequest, AgenticRAGResponse, DeepResearchRequest, etc.) SHALL reside in application/usecases/assistant/dto.go.

#### Scenario: Agent DTOs location
- **GIVEN** agent-related request/response types
- **WHEN** locating their definitions
- **THEN** they reside in application/usecases/assistant/dto.go
- **AND** they are NOT in domain/assistant/entity.go

### Requirement: preset 声明式注册

preset SHALL 以声明式描述注册到 `AgentRegistry`：每个 preset 提供工具名列表、prompt、能力等数据（`AgentSpec`），装配逻辑集中在注册表一处。新增一个 agent SHALL 只需增加一条声明式注册，MUST NOT 要求改动 `GetAgentByID` 的分支或多处命令式装配代码。

#### Scenario: 声明式 spec 注册

- **WHEN** 注册一个 preset agent
- **THEN** preset SHALL 提供 `AgentSpec`（工具名列表 + prompt + 能力）注册到注册表
- **AND** agent 实例 SHALL 由注册表按工具名列表用 `ToolRegistry` 装配

#### Scenario: 新增 agent 不改分支

- **WHEN** 需要新增一个 agent
- **THEN** 新增 SHALL 通过增加一条声明式注册完成
- **AND** MUST NOT 修改 `GetAgentByID` 的 switch/分支代码

