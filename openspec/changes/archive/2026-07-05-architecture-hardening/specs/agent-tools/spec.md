# agent-tools Specification

## ADDED Requirements

### Requirement: ToolRegistry 实例化依赖注入

工具注册 SHALL 从全局 `GlobalRegistry` + `func init()` 副作用改为实例化 `ToolRegistry`，经 `NewToolRegistry(deps)` 构造并由依赖注入装配。系统 MUST NOT 保留 package 级工具注册单例或 init 副作用注册。

#### Scenario: 实例化注册表替代全局单例

- **WHEN** 装配工具集合
- **THEN** 系统 SHALL 通过 `NewToolRegistry(deps)` 创建 `ToolRegistry` 实例
- **AND** MUST NOT 依赖 `service/agent/tools/registry.go` 中的 `var GlobalRegistry` 单例
- **AND** MUST NOT 依赖 `service/agent/tools/init.go` 的 `func init()` 完成注册

#### Scenario: 工具依赖经构造传入

- **WHEN** 某工具需要 RAG/Graph/SQL/审计等依赖
- **THEN** 该依赖 SHALL 由 `ToolRegistry` 构造时经 deps 传入并下发
- **AND** MUST NOT 经 `SetRAGService`/`SetGraphService` 等运行时 setter 或 `var ragService`/`var opConfig` 包全局提供

### Requirement: 工具测试改为构造注入表驱动

工具单元测试 SHALL 通过 `NewToolRegistry(mockDeps)` 注入测试依赖，MUST NOT 依赖 `Set/Reset` 全局脚手架（如保存并恢复 `opConfig`、调用 `SetGraphService(nil)`）。

#### Scenario: 测试用构造注入替代全局脚手架

- **WHEN** 为某工具编写单元测试
- **THEN** 测试 SHALL 用 `NewToolRegistry`/构造函数注入 mock 依赖
- **AND** MUST NOT 出现 `oldCfg := opConfig` 保存恢复或 `SetGraphService(nil)` 式全局脚手架
