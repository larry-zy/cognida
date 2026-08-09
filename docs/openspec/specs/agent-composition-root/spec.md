# agent-composition-root Specification

## Purpose
TBD - created by archiving change architecture-hardening. Update Purpose after archive.
## Requirements
### Requirement: 工具注册通过显式构造注入装配

系统 SHALL 通过 `NewToolRegistry(deps)` 显式构造函数装配工具注册表，deps 携带工具所需的领域依赖（RAG/Graph/SQL/审计等）。系统 MUST NOT 使用 package 级可变全局单例（如 `GlobalRegistry`）或 `func init()` 副作用注册工具。

#### Scenario: 构造函数装配工具注册表

- **WHEN** 装配 agent 边界的工具集合
- **THEN** 系统 SHALL 调用 `NewToolRegistry(deps)` 传入显式依赖构造注册表
- **AND** 返回的注册表实例 SHALL 承载全部工具，MUST NOT 依赖包加载时序

#### Scenario: 无 init 副作用注册

- **WHEN** 检查 `service/agent/tools` 包
- **THEN** 包内 MUST NOT 存在带工具注册副作用的 `func init()`
- **AND** MUST NOT 存在 `var GlobalRegistry` 等 package 级可变全局注册表

### Requirement: 工具依赖经构造传入而非运行时 setter

系统 SHALL 通过构造函数把工具依赖注入，MUST NOT 提供 `SetRAGService`/`SetGraphService` 等运行时 setter，也 MUST NOT 使用 `var ragService`/`var sqlDB`/`var opConfig`/`var dsProvider` 等 package 级可变依赖变量。

#### Scenario: 移除运行时 setter

- **WHEN** 检查 `service/agent/tools` 包的导出符号
- **THEN** MUST NOT 存在 `SetRAGService`、`SetGraphService` 及其它 `SetXxx` 依赖 setter
- **AND** 工具所需依赖 SHALL 全部经构造函数参数传入

#### Scenario: 缺失依赖在构造期暴露

- **WHEN** 构造工具注册表时未提供某必需依赖
- **THEN** 系统 SHALL 在构造期返回错误或明确失败
- **AND** MUST NOT 在运行期以 nil 依赖静默降级

### Requirement: agent 注册表通过显式构造注入装配

系统 SHALL 通过 `NewAgentRegistry(deps)` 显式构造 agent 注册表，agent 实例 MUST NOT 以 package 级全局单例（如 `var *AgentInstance`）持有。

#### Scenario: 构造函数装配 agent 注册表

- **WHEN** 装配 agent 集合
- **THEN** 系统 SHALL 调用 `NewAgentRegistry(deps)` 构造注册表
- **AND** agent 实例 SHALL 由注册表持有，MUST NOT 存在 package 级 `var` agent 单例

### Requirement: wire 依赖注入覆盖到 agent 边界

系统 SHALL 由 wire provider 装配 `ToolRegistry` 与 `AgentRegistry`，并经 builder/context 传递给 agent 执行，使 agent 边界的依赖图在 wire 中显式可见。

#### Scenario: wire provider 装配注册表

- **WHEN** 检查 `cmd/wire/wire.go`
- **THEN** 存在装配 `ToolRegistry` 与 `AgentRegistry` 的 provider
- **AND** agent orchestrator SHALL 从注册表获取 agent，而非包全局函数

#### Scenario: 同进程可装配多套配置

- **WHEN** 用两组不同 deps 分别调用 `NewToolRegistry`/`NewAgentRegistry`
- **THEN** 两个注册表实例 SHALL 互不共享全局态
- **AND** 各自的工具/agent 装配 SHALL 独立生效

