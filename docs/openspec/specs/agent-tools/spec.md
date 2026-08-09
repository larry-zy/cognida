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

### Requirement: sql_execute 回传结果信封

`sql_execute` 工具 SHALL 把完整查询结果写入 [Result Store](../agent-result-store/spec.md) 并回传结果信封（`result_id` + 列 + dtype + `row_count` + 样本 + 聚合），MUST NOT 再将原始行逐字回灌 LLM。既有的 1000 行上限 SHALL 仍作用于底层查询保护，但回灌 LLM 的样本行数受信封 N 上限约束。

#### Scenario: 查询结果入 Result Store 并回传信封

- **WHEN** LLM 调用 `sql_execute` 执行一条 SELECT
- **THEN** 工具 SHALL 把结果集写入 Result Store 生成 `result_id`
- **AND** 回灌 LLM 的 ToolMessage SHALL 是结果信封而非原始行

#### Scenario: 后续工具凭 result_id 复用

- **WHEN** LLM 拿到 `result_id` 后调用分析/导出/渲染工具
- **THEN** 这些工具 SHALL 凭 `result_id` 从 Result Store 取回完整数据
- **AND** LLM SHALL NOT 需要在上下文中重述原始行

### Requirement: get_schema 有界选表回传

`get_schema` 工具在未指定表名时 SHALL NOT 默认返回全库所有表，MUST 改为返回按相关度筛选（见 [agent-context-engineering](../agent-context-engineering/spec.md) 的词法选表）的相关候选子集；提供 `keywords` 时按其相关度选表，二者皆无时返回受上限约束的轻量目录（仅表名+描述，不含列）。指定表名时 SHALL 仍返回该表精确结构。

#### Scenario: 未指定表名时返回候选子集

- **WHEN** LLM 调用 `get_schema` 且未提供 `table_name`，但提供 `keywords`
- **THEN** 工具 SHALL 返回与关键词相关的候选表子集及其完整结构
- **AND** SHALL NOT 返回全库所有表结构

#### Scenario: 无表名无关键词时返回轻量目录

- **WHEN** LLM 调用 `get_schema` 且既无 `table_name` 又无 `keywords`
- **THEN** 工具 SHALL 返回受上限约束的表目录（仅表名+描述）
- **AND** SHALL NOT 一次性注入全部表的列结构

#### Scenario: 指定表名时精确返回

- **WHEN** LLM 调用 `get_schema` 并提供具体 `table_name`
- **THEN** 工具 SHALL 返回该表的精确列/类型/索引结构

### Requirement: 工具执行受硬门禁约束

所有工具的执行 SHALL 在执行前受 [skill-tool-policy](../skill-tool-policy/spec.md) 的硬工具门与会话 scope 约束。被拒调用 MUST NOT 触达底层执行，且 SHALL 以合成错误 ToolMessage 回灌 LLM。

#### Scenario: 被门禁拦截的工具不执行

- **WHEN** 某工具调用未通过 skill 策略或会话 scope 校验
- **THEN** 工具的底层执行 SHALL NOT 被触发
- **AND** 系统 SHALL 回传 `tool_blocked` 合成 ToolMessage

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

