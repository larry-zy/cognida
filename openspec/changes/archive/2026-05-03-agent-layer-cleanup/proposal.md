## Why

当前 Agent 模块的代码分层不符合 Clean Architecture 原则，存在依赖方向错误、职责混乱、重复定义等问题：
- Infrastructure 层依赖 Application 层，违反依赖倒置原则
- Domain 和 Application 层定义重复（如 `AgentRequest/Response`, `ChatChunk`）
- 具体实现（如 `BaseAgent`）放在 Application 层而非 Infrastructure
- 工具实现和编排模式位置不当，导致层级职责模糊

这些问题增加了维护成本，限制了代码的可测试性和可扩展性。

## What Changes

### Domain 层 (`internal/domain/agent/`)
- **精简 `entity.go`**：移除运行时类型（`ChatChunk`, `ToolCallRecord` 等），保留核心领域实体
- **重构 `repository.go`**：保留 `AgentRepository`，将 `ToolRegistry`, `ToolExecutor` 移至应用层，移除 `AgentOrchestrator`（编排是应用层职责）
- **新增 `service.go`**：定义领域服务接口（如 `AgentExecutor`, `HookService`）

### Application 层 (`internal/application/usecases/agent/`)
- **统一 Agent 接口**：移除 `eino_agent.go` 中重复的 `Agent` 接口定义，统一使用 Domain 层概念
- **移动 `BaseAgent`**：将 `base_agent.go` 移至 `infrastructure/agent/`
- **重构 `orchestration/`**：确保编排模式依赖 Domain 层接口
- **保留 `dto.go`**：作为应用层 DTO 定义，包含转换函数

### Infrastructure 层 (`internal/infrastructure/`)
- **修正 `adapter/agent/`**：移除对 Application 层的依赖，改为依赖 Domain 层接口
- **实现 `agent/`**：放置 `BaseAgent` 实现，实现 Domain 层定义的服务接口
- **实现 `hooks/`**：实现 Domain 层定义的 `HookService` 接口

### Tools 重构
- **保留接口**：在 Domain 层定义工具接口
- **移动实现**：将具体工具实现从 Application 层移至 Infrastructure 层

## Capabilities

### New Capabilities
- `agent-core`: 核心 Agent 接口和实体定义
- `agent-orchestration`: Agent 编排模式（Sequential, Parallel, Supervisor 等）
- `agent-hooks`: Agent 生命周期钩子系统
- `agent-tools`: 工具注册和执行机制

### Modified Capabilities
无（此重构是内部架构调整，不改变对外能力）

## Impact

### 受影响的代码
- `internal/domain/agent/entity.go` - 精简实体定义
- `internal/domain/agent/repository.go` - 重构为领域服务接口
- `internal/application/usecases/agent/` - 大量重构
- `internal/infrastructure/adapter/agent/` - 修正依赖方向
- `internal/infrastructure/agent/` - 新增/重构实现

### 受影响的测试
- `internal/application/usecases/agent/*_test.go` - 需要更新以适应新架构

### 依赖变化
- Infrastructure 层不再依赖 Application 层
- Application 层仅依赖 Domain 层

### 风险
- **中等风险**：涉及多层重构，需要谨慎处理迁移路径
- **缓解措施**：保持向后兼容的过渡期，逐步废弃旧接口
