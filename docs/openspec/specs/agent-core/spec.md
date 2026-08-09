# agent-core Specification

## Purpose
TBD - created by archiving change agent-layer-cleanup. Update Purpose after archive.
## Requirements
### Requirement: Domain layer defines core Agent entities
The system SHALL define core Agent entities in the Domain layer without dependencies on external frameworks or application layer concepts.

#### Scenario: Agent entity contains only domain concepts
- **WHEN** inspecting `domain/agent/entity.go`
- **THEN** file contains only: `Agent`, `AgentType`, `AgentStatus`, `AgentConfig`, `HookConfig`, `SearchConfig`
- **AND** file does NOT contain: `ChatChunk`, `ToolCallRecord`, `AgentRequest`, `AgentResponse`

### Requirement: Domain layer defines repository interfaces
The system SHALL define data access interfaces in the Domain layer for Infrastructure layer to implement.

#### Scenario: AgentRepository interface exists
- **WHEN** inspecting `domain/agent/repository.go`
- **THEN** `AgentRepository` interface defines methods: `SaveConfig`, `LoadConfig`, `SaveExecutionRecord`, `FindExecutionRecords`

### Requirement: Domain layer defines service interfaces
The system SHALL define domain service interfaces for capabilities that don't naturally belong to any single entity.

#### Scenario: AgentExecutor service interface exists
- **WHEN** inspecting `domain/agent/service.go`
- **THEN** `AgentExecutor` interface defines methods for executing agent logic
- **AND** `HookService` interface defines lifecycle hook methods

### Requirement: Application layer contains DTOs
The system SHALL define Data Transfer Objects in the Application layer for request/response handling.

#### Scenario: DTOs exist in application layer
- **WHEN** inspecting `application/usecases/agent/dto.go`
- **THEN** file contains: `AgenticRAGRequest`, `AgenticRAGResponse`, `ChatChunkDTO`, `ToolCallRecordDTO`

### Requirement: Conversion functions exist between Domain and DTO
The system SHALL provide conversion functions between Domain entities and Application DTOs.

#### Scenario: ToDomainAgentRequest converts DTO to domain
- **WHEN** calling `ToDomainAgentRequest(&AgenticRAGRequest{...})`
- **THEN** returns `*agent.AgentRequest` with mapped fields

#### Scenario: FromDomainAgentResponse converts domain to DTO
- **WHEN** calling `FromDomainAgentResponse(&agent.AgentResponse{...})`
- **THEN** returns `*AgenticRAGResponse` with mapped fields

### Requirement: Infrastructure layer implements Domain interfaces
The system SHALL implement Domain-defined interfaces in the Infrastructure layer.

#### Scenario: BaseAgent implements domain interface
- **WHEN** inspecting `infrastructure/agent/base_agent.go`
- **THEN** `BaseAgent` struct implements Domain-defined Agent interface
- **AND** file imports only: Domain layer, Eino framework (external)

### Requirement: Infrastructure layer does not depend on Application layer
The system SHALL NOT allow Infrastructure layer to import Application layer packages.

#### Scenario: No infrastructure → application imports
- **WHEN** running `go mod graph | grep infrastructure`
- **THEN** output does NOT contain paths ending in `application/usecases`

### Requirement: Application layer uses Domain interfaces via dependency injection
The system SHALL inject Domain interface implementations into Application layer use cases.

#### Scenario: ExecuteUseCase accepts domain interface
- **WHEN** creating `NewExecuteUseCase(orchestrator AgentOrchestrator)`
- **THEN** use case stores the domain interface for later use

### Requirement: eino_agent 执行主干去上帝对象化

`eino_agent` 的执行 SHALL 收敛为单一执行主干，MUST NOT 把 memory/tool/streaming 三个正交维度以笛卡尔积展开成多个 `chatWith*`/`streamWith*` 变体。当前 `chatWithMemory`/`chatWithMemoryAndTools`/`chatWithMemoryOnly`/`chatWithTools`/`chatWithoutTools`/`streamWithTools`/`streamWithoutTools` 等变体 SHALL 被消除。

#### Scenario: 单一执行主干

- **WHEN** 检查 `service/agent/framework/eino_agent.go`
- **THEN** 存在单一执行主干入口
- **AND** MUST NOT 存在 `chatWithMemoryAndTools`/`chatWithMemoryOnly`/`chatWithTools`/`chatWithoutTools`/`streamWithTools`/`streamWithoutTools` 等按维度展开的重复变体

### Requirement: memory/tool-loop/streaming 组件可插拔

memory、tool-loop、streaming 三个维度 SHALL 抽为可插拔组件（策略/组合），由执行主干组合调用，使新增一个维度的行为不再乘出新变体。

#### Scenario: 三维度独立可插拔

- **WHEN** 检查 eino_agent 执行组件划分
- **THEN** memory 上下文构建、tool 执行循环、streaming 输出 SHALL 各为独立组件
- **AND** 执行主干 SHALL 通过组合这些组件承载不同请求，MUST NOT 为组合逐个复制主干代码

#### Scenario: 拆解保持行为一致

- **WHEN** 对 memory×tool×streaming 各组合执行同一请求
- **THEN** 拆解后的执行主干 SHALL 产出与拆解前一致的响应/流式行为
- **AND** 组合矩阵回归测试 SHALL 全部通过

