# agent-service Specification

## MODIFIED Requirements

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

## ADDED Requirements

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
