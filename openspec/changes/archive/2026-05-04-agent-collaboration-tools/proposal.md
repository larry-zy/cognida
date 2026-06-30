## Why

**问题 1：现有协作模式是框架强制的**

当前 Multi-Agent 编排模式（Sequential、Parallel、Supervisor）和协作模式（TaskDecomposer + Dispatcher）都是**框架强制流程**，LLM 无法自主决定何时、如何协作：
- 协作流程写死在代码中，缺乏灵活性
- LLM 无法根据任务复杂度动态选择是否需要协作
- 简单任务也被强制走多 Agent 流程，浪费 Token

**问题 2：缺少 LLM 可调用的协作工具**

LLM 需要在对话中自主决策：
- 这个子任务应该委派给哪个 Agent？
- 我需要向谁咨询来验证信息？
- 当前任务是否应该转交给其他 Agent？

**问题 3：现有 collaboration 包不覆盖此场景**

现有的 `collaboration/` 包提供的是**框架层协作**（任务分解 → 分发 → 聚合），而我们需要的是 **LLM 层协作**（LLM 通过工具自主发起协作）。

## What Changes

- **新增协作工具包** `collaboration/tools.go`：提供 Agent 间协作的工具封装
  - `DelegateTool` - 委派任务给指定 Agent
  - `AskTool` - 向其他 Agent 咨询（不转移控制权）
  - `HandoffTool` - 转移对话控制权
- **扩展 AgentRegistry**：添加 Tool 友好的方法（`GetByName`、`GetDescription` 等）
- **扩展 Builder**：添加 `WithCollaboration()` 方法，支持自动注入协作工具
- **保留现有能力**：orchestration/ 和 collaboration/ 现有功能不变，作为补充

## Capabilities

### New Capabilities
- `agent-collaboration-tools`: Agent 间 LLM 可调用协作工具能力，提供委派、咨询、转移等协作模式

### Modified Capabilities
- `agent-registry`：扩展现有 AgentRegistry，添加 Tool 友好的方法（向后兼容）

## Impact

- **代码层面**：
  - 新增 `collaboration/tools.go`
  - 扩展 `collaboration/task.go` 的 AgentRegistry
  - 扩展 `eino_builder.go` 添加 `WithCollaboration()`
- **API 层面**：Builder 新增 `WithCollaboration()` 方法
- **兼容性**：向后兼容，现有 Agent 代码无需修改
- **依赖**：无新增外部依赖，复用现有 Eino 框架
- **文档**：需要更新文档说明三种协作模式的区别和使用场景

## 协作模式对比

| 层级 | 模式 | 控制方 | 适用场景 |
|------|------|--------|---------|
| L3 - 工具协作 | DelegateTool/AskTool/HandoffTool | LLM | 灵活场景、自主决策 |
| L2 - 框架协作 | TaskDecomposer + Dispatcher | 框架 | 复杂任务、自动分解 |
| L1 - 流程编排 | Sequential/Parallel/Supervisor | 代码 | 固定流程、确定性执行 |
